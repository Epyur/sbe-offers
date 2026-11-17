package main

// Устойчивые факты боевого листка (2026-11-17, шаг 3e плана
// `2026-11-16-sbe-offers-combat-sections-and-agent-tools-plan.md`, §9.1).
//
// Зачем. Агент читает ссылку заказчика (`fetch_url`) и приложенный файл
// (`parse_file`), но добытое жило только в разговоре: следующий ход начинался
// с чистого листа, а раздел листка переписывался по памяти модели. Это уже
// был живой дефект класса вступления 2026-11-08 — текст, верный на момент
// разговора, потом расходится с карточками.
//
// Правило (§9.1). Добытые факты запоминаются у САМОГО листка и входят в
// отпечаток содержания: правка факта переписывает раздел, повторная сборка без
// правки — берёт готовый текст. Источник запоминает адрес/имя и sha256, а не
// только «что-то прочитали»: тот же адрес, но другое содержимое — другой факт.
//
// Границы. Факт — это ТЕКСТ, а не файл и не картинка: у листка две стороны и
// спора по документам, а хранить десятки мегабайт в `form_values` нельзя.
// Обрезка через `truncateText` — та же, что у ответа модели.

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// combatFactLimit - сколько фактов держит листок.
//
// Двенадцать на две стороны — это шесть на сторону при разумном запасе. Предел
// есть намеренно: `form_values` — колонка JSONB, и «все ссылки заказчика» с
// сайта утянут в неё мегабайты, а модель всё равно не прочтёт их за один ход.
const combatFactLimit = 12

// combatFactTextChars - сколько символов одного факта попадает в отпечаток и
// в промт. Шесть тысяч — тот же предел, что у текстового вида `fetch_url`
// (`compactFetchTextChars`), чтобы «прочитано» и «запомнено» не расходились.
const combatFactTextChars = compactFetchTextChars

// combatFactKey - имя факта в `form_values` листка.
//
// `gen.combatfact.<n>` намеренно с точкой в имени: `sectionGen` пишет свои
// ключи `gen.sect.*`, и по этому префиксу их видно отдельно. Число — порядковый
// номер, а не хэш адреса: перенумерация при удалении — наша же операция, а
// позиционный ключ читается человеком в psql.
func combatFactKey(index int) string { return fmt.Sprintf("gen.combatfact.%02d", index) }

// combatFactPrefix - по нему отделяем факты листка от прочего в form_values.
const combatFactPrefix = "gen.combatfact."

// combatFact - один добытый факт: откуда и что.
type combatFact struct {
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Digest string `json:"digest"`
	Text   string `json:"text"`
}

// newCombatFact - факт из ОДНОГО добытого куска текста.
//
// Пустой текст не становится фактом: «прочитано, но ничего не понятно» — это
// не факт, а повод сказать человеку. Дата и время в факт не входят намеренно —
// иначе повторное чтение того же адреса меняло бы отпечаток и раздел
// переписывался бы без изменения содержания.
func newCombatFact(kind, source, text string) (combatFact, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || strings.TrimSpace(source) == "" {
		return combatFact{}, false
	}
	trimmed = truncateText(trimmed, combatFactTextChars)
	return combatFact{
		Kind:   strings.TrimSpace(kind),
		Source: strings.TrimSpace(source),
		Digest: factDigest(trimmed),
		Text:   trimmed,
	}, true
}

// factDigest - sha256 по содержимому факта (без описания и без времени).
func factDigest(text string) string {
	return hashShort("combatfact\x1f" + text)
}

// combatFactsFromValues - факты листка из его `form_values`.
//
// Порядок — по номеру в ключе, а не по порядку ключей карты: карта не
// сохраняет порядок, а «факт №2» в промте обязан оставаться вторым, иначе
// отпечаток будет плавать между сборками на одном и том же листке.
func combatFactsFromValues(values map[string]string) []combatFact {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		if strings.HasPrefix(key, combatFactPrefix) {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)
	out := make([]combatFact, 0, len(keys))
	for _, key := range keys {
		fact, ok := parseCombatFact(values[key])
		if !ok {
			continue
		}
		out = append(out, fact)
	}
	return out
}

// parseCombatFact - разбор строки факта.
//
// Формат держится простым: `kind | source | digest` в первой строке, текст —
// со второй. JSON был бы строже, но в `psql` его не прочесть глазами, а
// факт — короткая справка, а не протокол.
func parseCombatFact(raw string) (combatFact, bool) {
	header, text, _ := strings.Cut(raw, "\n")
	parts := strings.Split(header, "|")
	if len(parts) != 3 {
		return combatFact{}, false
	}
	fact := combatFact{
		Kind:   strings.TrimSpace(parts[0]),
		Source: strings.TrimSpace(parts[1]),
		Digest: strings.TrimSpace(parts[2]),
		Text:   strings.TrimSpace(text),
	}
	if fact.Source == "" || fact.Text == "" {
		return combatFact{}, false
	}
	if fact.Digest == "" {
		fact.Digest = factDigest(fact.Text)
	}
	return fact, true
}

// encode - строка факта для `form_values`.
func (f combatFact) encode() string {
	return fmt.Sprintf("%s | %s | %s\n%s", f.Kind, f.Source, f.Digest, f.Text)
}

// label - как факт называется в промте и в отпечатке.
func (f combatFact) label() string {
	if f.Kind == "" {
		return f.Source
	}
	return f.Kind + ": " + f.Source
}

// combatFactHandle - отпечаток набора фактов для ключа кэша.
//
// В отпечаток входят описание и sha256, но НЕ текст целиком: текст уже учтён
// sha256, а гнать его второй раз в хэш дороже и ничего не добавляет. Пустой
// набор — пустая строка, чтобы у листка без фактов ключ остался прежним и
// уже написанные тексты не переписывались при выкатке.
func combatFactHandle(facts []combatFact) string {
	if len(facts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, fact := range facts {
		b.WriteString(fact.label())
		b.WriteString("\x1f")
		b.WriteString(fact.Digest)
		b.WriteString("\x1e")
	}
	return hashShort(b.String())
}

// withCombatFacts - факты, которых в листке ещё нет, дописаны в конец.
//
// Один факт = один источник. Повторное чтение того же адреса ЗАМЕНЯЕТ текст
// (содержимое могло измениться — ради этого и хранится sha256), но не
// заводит вторую запись: две одинаковые ссылки в промте только путают модель.
// Найдено по стенду 2026-11-17: без замены повторное чтение плодило факты до
// предела за три хода.
func withCombatFacts(existing []combatFact, added combatFact) []combatFact {
	out := make([]combatFact, 0, len(existing)+1)
	replaced := false
	for _, fact := range existing {
		if sameFactSource(fact, added) {
			if !replaced {
				out = append(out, added)
				replaced = true
			}
			continue
		}
		out = append(out, fact)
	}
	if !replaced {
		out = append(out, added)
	}
	if len(out) > combatFactLimit {
		out = out[len(out)-combatFactLimit:]
	}
	return out
}

// sameFactSource - один ли это источник (вид + адрес/имя без регистра).
func sameFactSource(a, b combatFact) bool {
	return foldText(a.Kind) == foldText(b.Kind) && foldText(a.Source) == foldText(b.Source)
}

// combatFactsPrompt - как факты попадают в промт подраздела листка.
//
// Отдельным блоком после карточек: модель обязана видеть, что это добытые
// сведения, а не свойства продукта, и не выдавать их за характеристику.
// Пустой набор — пустая строка: у листка без фактов промт остаётся прежним.
func combatFactsPrompt(facts []combatFact) string {
	if len(facts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nДобытые факты (ссылки и файлы заказчика, прочитанные в разговоре):\n")
	for i, fact := range facts {
		b.WriteString(fmt.Sprintf("%d) %s\n%s\n", i+1, fact.label(), fact.Text))
	}
	b.WriteString("Это сведения из источников, а не свойства нашего продукта: ссылайся на них как на «по документам заказчика», числовые характеристики бери только отсюда.\n")
	return b.String()
}

// persistCombatFacts - дописать факт в `form_values` листка.
//
// Одно правило с кэшем текстов (`persistCombatSectionTexts`): только
// `form_values`, только изменившееся, стороны не трогаются. Ошибка записи не
// роняет ход агента — факт останется в разговоре и попадёт в листок на
// следующем; но человеку ход об этом не врёт, поэтому ошибка уходит в журнал.
func (s *Server) persistCombatFacts(ctx context.Context, sheetID int64, fact combatFact) (combatFact, bool, error) {
	if s == nil || s.pool == nil || sheetID <= 0 {
		return combatFact{}, false, nil
	}
	sheet, err := s.loadCombatSheet(ctx, sheetID)
	if err != nil {
		return combatFact{}, false, err
	}
	before := combatFactsFromValues(sheet.FormValues)
	after := withCombatFacts(before, fact)
	if len(after) == len(before) && combatFactHandle(before) == combatFactHandle(after) {
		return fact, false, nil
	}
	values := make(map[string]string, len(sheet.FormValues)+len(after))
	for key, value := range sheet.FormValues {
		if strings.HasPrefix(key, combatFactPrefix) {
			continue
		}
		values[key] = value
	}
	for i, item := range after {
		values[combatFactKey(i+1)] = item.encode()
	}
	sheet.FormValues = values
	cols, err := combatColumns(sheet)
	if err != nil {
		return combatFact{}, false, err
	}
	if err := s.updateCombat(ctx, sheetID, sheet, cols); err != nil {
		return combatFact{}, false, err
	}
	return fact, true, nil
}
