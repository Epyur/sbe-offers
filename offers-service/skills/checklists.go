package skills

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Чек-листы. Возвращают список замечаний на русском — их показывает менеджер,
// поэтому формулировка человеческая, а не код ошибки.
//
// Источник правил: references/ai-tells.md (copywriting 2.3.2). Список шаблонов
// — выборка того, что реально встречается в русском тексте КП; английские
// заставки из оригинала не переносились.
//
// ВАЖНО про регулярки: `\b` в Go считает границей слова только латиницу и
// цифры — на кириллице он не срабатывает, и шаблон молча не находит ничего.
// Поэтому границы слова здесь не используются вовсе.

var (
	// «Это не дорого, это выгодно» / «не потому что X, а потому что Y».
	reContrast = regexp.MustCompile(`(?i)не\s+\S+(?:\s+\S+)?\s*[,—–-]\s*(?:это|а|но)\s+\S+`)
	// «Без выезда, без шаблонов, без ожидания» — два отрицания подряд через
	// запятую; одиночное «без остановки производства» замечанием не считается.
	reNegation = regexp.MustCompile(`(?i)без\s+[^,.;!?]{1,40},\s*без\s+[^,.;!?]{1,40}`)
	// «Результат? Втрое быстрее» — короткий вопрос, за которым сразу ответ.
	reFakeQuestion = regexp.MustCompile(`(?m)^\s*[A-ZА-ЯЁ][^.!?]{1,30}\?\s*\S`)
	// Заставки.
	reStock = regexp.MustCompile(`(?i)(в современном|в нынешних условиях|хотите узнать|давайте разберёмся|в мире, где|ни для кого не секрет)`)
	// Длинное тире.
	reEmDash = regexp.MustCompile(`\s—\s`)
	// Восклицательный знак.
	reExclaim = regexp.MustCompile(`!`)
	// Эмодзи (основные диапазоны).
	reEmoji = regexp.MustCompile(`[\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}\x{2190}-\x{21FF}\x{2B00}-\x{2BFF}]`)
	// Число в тексте: 1 234,56 / 40 / 40 % / 40%.
	reNumber = regexp.MustCompile(`\d[\d\s\x{00a0}]*(?:[.,]\d+)?\s*%?`)
)

// CheckAiTells — замечания к тексту по правилам «не писать как модель».
// Пустой список значит «чисто».
func CheckAiTells(text string) []string {
	var out []string
	if reContrast.MatchString(text) {
		out = append(out, "контрастный оборот «не X, а Y»: скажите Y прямо и назовите причину")
	}
	if reNegation.MatchString(text) {
		out = append(out, "перечисление «без X, без Y»: напишите, что происходит на деле")
	}
	if reFakeQuestion.MatchString(text) {
		out = append(out, "вопрос с ответом («Результат? …»): уберите вопрос, оставьте ответ")
	}
	if loc := reStock.FindString(text); loc != "" {
		out = append(out, "заставка «"+loc+"»: начните с задачи заказчика")
	}
	if reEmDash.MatchString(text) && len([]rune(text)) < 80 {
		out = append(out, "длинное тире в короткой фразе: точка или запятая")
	}
	if reExclaim.MatchString(text) {
		out = append(out, "восклицательный знак: в предложении его не бывает")
	}
	if loc := reEmoji.FindString(text); loc != "" {
		out = append(out, "эмодзи «"+loc+"»: в документе для заказчика их не ставят")
	}
	return out
}

// CheckClaimNumbers — числа в тексте, которых нет среди разрешённых.
//
// Правило проекта: модель может использовать только те числа, что уже есть в
// предложении. Иначе она посчитает «экономию 40%» от себя, и это разойдётся
// со сметой. Разрешённые числа передаются строками в формате документа.
func CheckClaimNumbers(text string, allowed []string) []string {
	norm := func(s string) string {
		s = strings.ReplaceAll(s, "\u00a0", " ")
		s = strings.ReplaceAll(s, " ", "")
		s = strings.ReplaceAll(s, ",", ".")
		return strings.TrimSuffix(s, "%")
	}
	ok := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		ok[norm(a)] = true
	}
	seen := make(map[string]bool)
	var out []string
	for _, raw := range reNumber.FindAllString(text, -1) {
		candidate := norm(raw)
		if candidate == "" || ok[candidate] || seen[candidate] {
			continue
		}
		// Мелкие целые без процента — это обычно единицы измерения, классы и
		// артикулы («2 мм», «B1»), их не помечаем.
		if !strings.Contains(raw, "%") && !strings.Contains(candidate, ".") {
			if n, err := strconv.Atoi(candidate); err == nil && n < 100 {
				continue
			}
		}
		seen[candidate] = true
		out = append(out, fmt.Sprintf("число %s нет в расчёте — уточните или уберите", strings.TrimSpace(raw)))
	}
	return out
}

// CheckSectionStructure — грубая проверка формы секции: пусто / слишком длинно
// / преимущества не списком.
func CheckSectionStructure(text string, kind SectionKind) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return []string{"секция пустая"}
	}
	var out []string
	words := len(strings.Fields(trimmed))
	switch kind {
	case SectionIntro, SectionConditions:
		if words > 120 {
			out = append(out, fmt.Sprintf("вводный текст слишком длинный (%d слов): это 2–4 предложения", words))
		}
	case SectionSystem:
		if words > 250 {
			out = append(out, fmt.Sprintf("описание системы слишком длинное (%d слов)", words))
		}
	case SectionAdvantages:
		bullets := 0
		for _, line := range strings.Split(trimmed, "\n") {
			l := strings.TrimSpace(line)
			if strings.HasPrefix(l, "•") || strings.HasPrefix(l, "-") || strings.HasPrefix(l, "–") {
				bullets++
			}
		}
		if bullets < 2 {
			out = append(out, "преимущества должны идти списком: 3–5 пунктов, каждый с новой строки и с •")
		}
	}
	return out
}

// FormatMoney — сумма в формате документа: «1 234,56».
//
// Один экземпляр на весь сервис. До этого тот же код был скопирован в docx.go
// и в боте, и они разошлись: `1.005` печаталось как «1,00».
func FormatMoney(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	// Множитель 1000 снимает артефакт представления: 1.005 * 100 в двоичном
	// виде — 100.49999999999999, и округление по нему дало бы 1.00.
	scaled := float64(int64(v*1000+0.5)) / 10
	cents := int64(scaled + 0.5)
	whole := cents / 100
	frac := cents % 100
	digits := strconv.FormatInt(whole, 10)
	var parts []string
	for len(digits) > 3 {
		parts = append([]string{digits[len(digits)-3:]}, parts...)
		digits = digits[:len(digits)-3]
	}
	parts = append([]string{digits}, parts...)
	out := strings.Join(parts, " ") + "," + fmt.Sprintf("%02d", frac)
	if neg {
		return "-" + out
	}
	return out
}

// formatQty — количество без хвостов плавающей точки: 5520, 221, 10.5.
func formatQty(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}
