package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// materialTagName — тег карточки Базы знаний, которая описывает материал.
// Вместе с commercialPropose он отличает материал от системы.
const materialTagName = "material"

// Сопоставление строк таблицы с материалами (Task 4 плана 2026-10-30).
//
// Порядок строго по возрастанию доверия:
//  1. точный ЕКН — номер уже привязан к материалу, это факт, а не догадка;
//  2. точное название после нормализации — единственное совпадение;
//  3. модель — только для того, что осталось, и только как ПРЕДЛОЖЕНИЕ:
//     такое решение обязательно подтверждается человеком.
//
// Молча применять догадку нельзя: ошибка сопоставления попадёт в КП с ценой.

// MatchMethod — как найдено соответствие. Строки уходят в интерфейс, чтобы
// человек видел, чему верить без проверки, а что надо посмотреть.
const (
	matchByEkn  = "ekn"
	matchByName = "name"
	matchByAI   = "ai"
	matchNone   = "none"
)

// MatchResult — что решили по одной строке таблицы.
type MatchResult struct {
	Row SheetRow `json:"row"`
	// MaterialID — найденный материал; 0 значит «не найден».
	MaterialID int64  `json:"materialId"`
	SKU        string `json:"sku"`
	Name       string `json:"name"`
	Method     string `json:"method"`
	// Reason — почему не нашли. Показывается человеку, а не пишется в журнал.
	Reason string `json:"reason,omitempty"`
	// NeedsConfirm — решение принято моделью и требует подтверждения.
	NeedsConfirm bool `json:"needsConfirm"`
}

// MaterialRef — то немногое, что нужно сопоставлению от материала.
type MaterialRef struct {
	ID   int64
	SKU  string
	Name string
	// Ekn — номера, уже привязанные к этому материалу.
	Ekn []string
}

// matchRows сопоставляет строки с материалами.
//
// complete может быть nil — тогда шаг модели пропускается и всё непонятое
// остаётся в matchNone. Так проверяются первые два шага без сети.
func matchRows(ctx context.Context, rows []SheetRow, materials []MaterialRef, complete Completer) []MatchResult {
	byEkn := make(map[string]MaterialRef, len(materials))
	for _, m := range materials {
		for _, e := range m.Ekn {
			key := normalizeEkn(e)
			if key != "" {
				byEkn[key] = m
			}
		}
	}
	byName := make(map[string][]MaterialRef, len(materials))
	for _, m := range materials {
		key := normalizeMaterialName(m.Name)
		if key != "" {
			byName[key] = append(byName[key], m)
		}
	}

	results := make([]MatchResult, 0, len(rows))
	for _, row := range rows {
		res := MatchResult{Row: row, Method: matchNone}
		switch {
		case row.MissingName && normalizeEkn(row.Ekn) == "":
			res.Reason = "нет ни названия, ни ЕКН"
		case normalizeEkn(row.Ekn) != "" && byEkn[normalizeEkn(row.Ekn)].ID != 0:
			m := byEkn[normalizeEkn(row.Ekn)]
			res.MaterialID, res.SKU, res.Name, res.Method = m.ID, m.SKU, m.Name, matchByEkn
		default:
			matches := byName[normalizeMaterialName(row.Name)]
			switch len(matches) {
			case 1:
				m := matches[0]
				res.MaterialID, res.SKU, res.Name, res.Method = m.ID, m.SKU, m.Name, matchByName
			case 0:
				res.Reason = "среди материалов нет такого названия"
			default:
				res.Reason = fmt.Sprintf("под название подходит несколько материалов (%d): уточните", len(matches))
			}
		}
		results = append(results, res)
	}

	if complete != nil {
		askModelForUnmatched(ctx, results, materials, complete)
	}
	return results
}

// askModelForUnmatched даёт модели шанс на строки, которые не разобрали
// первые два шага. Любой ответ модели помечается как требующий подтверждения.
func askModelForUnmatched(ctx context.Context, results []MatchResult, materials []MaterialRef, complete Completer) {
	for i := range results {
		if results[i].Method != matchNone || results[i].Row.MissingName {
			continue
		}
		m, ok := askModelToMatch(ctx, results[i].Row.Name, materials, complete)
		if !ok {
			continue
		}
		results[i].MaterialID = m.ID
		results[i].SKU = m.SKU
		results[i].Name = m.Name
		results[i].Method = matchByAI
		results[i].NeedsConfirm = true
		results[i].Reason = ""
	}
}

// askModelToMatch спрашивает модель, какой материал соответствует названию.
//
// Модель обязана ответить ИНДЕКСОМ из списка или «нет»: свободный текст не
// разбирается, иначе она начнёт придумывать материалы, которых нет.
func askModelToMatch(ctx context.Context, rowName string, materials []MaterialRef, complete Completer) (MaterialRef, bool) {
	name := strings.TrimSpace(rowName)
	if name == "" || len(materials) == 0 {
		return MaterialRef{}, false
	}
	var list strings.Builder
	for i, m := range materials {
		list.WriteString(fmt.Sprintf("%d. %s\n", i+1, m.Name))
	}
	system := "Ты сопоставляешь строку из таблицы менеджера с материалами справочника.\n" +
		"Ответь ТОЛЬКО номером материала из списка или словом «нет», если подходящего нет.\n" +
		"Не придумывай материалы, которых нет в списке. Если не уверен — ответь «нет»."
	user := "Строка из таблицы: " + name + "\n\nМатериалы:\n" + list.String()

	answer, err := complete.Complete(ctx, system, user, 40)
	if err != nil {
		return MaterialRef{}, false
	}
	return pickMaterialByAnswer(answer, materials)
}

// pickMaterialByAnswer разбирает ответ модели: номер из списка или «нет».
func pickMaterialByAnswer(answer string, materials []MaterialRef) (MaterialRef, bool) {
	trimmed := strings.TrimSpace(answer)
	if trimmed == "" {
		return MaterialRef{}, false
	}
	// Берём первое число в ответе: модель любит обрамлять его текстом.
	digits := make([]rune, 0, 4)
	for _, r := range trimmed {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
			continue
		}
		if len(digits) > 0 {
			break
		}
	}
	if len(digits) == 0 {
		return MaterialRef{}, false
	}
	var idx int
	if _, err := fmt.Sscanf(string(digits), "%d", &idx); err != nil {
		return MaterialRef{}, false
	}
	if idx < 1 || idx > len(materials) {
		return MaterialRef{}, false
	}
	return materials[idx-1], true
}

// normalizeMaterialName — ключ сравнения названий: регистр, ё/е, кавычки,
// дефисы и лишние пробелы не должны делать из одного материала два.
func normalizeMaterialName(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = strings.ReplaceAll(name, "ё", "е")
	name = strings.NewReplacer("«", "", "»", "", "\"", "", "'", "", "`", "").Replace(name)
	name = strings.NewReplacer("‐", "-", "‑", "-", "–", "-", "—", "-", "−", "-").Replace(name)
	name = strings.ReplaceAll(name, "×", "x")
	return strings.Join(strings.Fields(name), " ")
}

// normalizeEkn — ключ сравнения номеров: пробелы и дефисы не делают номер другим.
func normalizeEkn(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// --- Системы и материалы по тегам (Task 6 плана 2026-10-30) -------------
//
// Две сущности различаются ТЕГАМИ карточки Базы знаний, а не названием:
//   - commercial_propose + material — материал (то, что продаётся);
//   - commercial_propose без material — система (обоснование сочетания).
//
// Признак «есть ТН- в названии» не годится: он есть почти у любой системы, но
// полагаться на написание нельзя — технолог заведёт карточку без префикса, и
// она попадёт не в тот раздел. Тег — явное решение человека.

// splitCardsByTags делит карточки на системы и материалы.
func splitCardsByTags(notes []KBNoteSummary) (systems, materials []KBNoteSummary) {
	for _, n := range notes {
		if noteHasTag(n, materialTagName) {
			materials = append(materials, n)
			continue
		}
		systems = append(systems, n)
	}
	return systems, materials
}

// noteHasTag проверяет тег по списку тегов карточки.
func noteHasTag(n KBNoteSummary, tag string) bool {
	for _, t := range n.Tags {
		if strings.EqualFold(strings.TrimSpace(t.Name), tag) {
			return true
		}
	}
	return false
}

// reCompositionSku — SKU в составной строке. Код вида MAT-МЕМБ-ПВХ-0007 или
// РАБ-МОНТ-ОБЩ-0001: буквы, три группы и номер через дефис.
//
// Без \b: в Go кириллица не считается «словом» для границы слова, и шаблон
// с \b не находил бы код, начинающийся с РАБ-. Эта же грабля уже ловилась в
// skills (2026-10-29).
var reCompositionSku = regexp.MustCompile(`(?:MAT|РАБ)-[А-ЯЁA-Z0-9]{1,8}-[А-ЯЁA-Z0-9]{1,8}-\d{4}`)

// extractCompositionSku достаёт SKU материала из строки состава.
//
// Строки состава пишут по-разному: «- **1** Мембрана (MAT-МЕМБ-ПВХ-0007)»,
// «- MAT-МЕМБ-ПВХ-0007 — мембрана 1,5 мм». SKU ищется как код, а не по
// позиции в строке: иначе порядок слов ломал бы разбор.
func extractCompositionSku(line string) string {
	return reCompositionSku.FindString(strings.ToUpper(line))
}
