package skills

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"mailers/offers/model"
)

// Разбор файлов, которые менеджер присылает в чат боту (шаг 5 пересмотренного
// плана 2026-10-29).
//
// Зачем: контекст объекта и таблицу спецификации менеджер получает от заказчика
// файлом. Перепечатывать её руками — та самая работа, ради которой бот и нужен.
//
// Принимается .md и .xlsx. .docx — вторым заходом (решение пользователя
// 2026-10-29): xlsx — основной сценарий, а docx без сторонних библиотек
// разбирать дольше всего. Старый .xls (бинарный) не принимается никогда:
// сообщаем «пересохраните как .xlsx».
//
// Тип определяется по СИГНАТУРЕ, а не по расширению: человек присылает файл
// как есть, имя бывает любым. zip с xl/workbook.xml — xlsx, zip с
// word/document.xml — docx, остальное, что не бинарное, — текст.

// AttachmentKind — что внутри файла.
type AttachmentKind string

const (
	AttachmentMarkdown AttachmentKind = "md"
	AttachmentXlsx     AttachmentKind = "xlsx"
	AttachmentDocx     AttachmentKind = "docx"
	AttachmentOldExcel AttachmentKind = "xls"
	AttachmentPDF      AttachmentKind = "pdf"
	AttachmentUnknown  AttachmentKind = ""
)

// DetectAttachmentType определяет тип файла.
//
// Пустые данные — пустой тип: разбирать нечего, и вызывающий скажет об этом
// человеку, а не модели.
func DetectAttachmentType(fileName string, data []byte) string {
	if len(data) == 0 {
		return ""
	}
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return zipKind(data)
	}
	if bytes.HasPrefix(data, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}) {
		return string(AttachmentOldExcel)
	}
	if bytes.HasPrefix(data, []byte("%PDF")) {
		return string(AttachmentPDF)
	}
	if isTextual(data) {
		return string(AttachmentMarkdown)
	}
	return string(AttachmentUnknown)
}

// zipKind различает ooxml-форматы по составу архива.
func zipKind(data []byte) string {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return string(AttachmentUnknown)
	}
	hasWorkbook, hasDocument := false, false
	for _, f := range zr.File {
		switch f.Name {
		case "xl/workbook.xml":
			hasWorkbook = true
		case "word/document.xml":
			hasDocument = true
		}
	}
	switch {
	case hasWorkbook:
		return string(AttachmentXlsx)
	case hasDocument:
		return string(AttachmentDocx)
	}
	return string(AttachmentUnknown)
}

// isTextual — похоже ли на текст: нет нулевых и управляющих байтов в начале.
func isTextual(data []byte) bool {
	limit := len(data)
	if limit > 4096 {
		limit = 4096
	}
	for _, b := range data[:limit] {
		if b == 0 {
			return false
		}
		if b < 0x20 && b != '\n' && b != '\r' && b != '\t' {
			return false
		}
	}
	return true
}

// MarkdownAttachment — что бот понял из текстового файла.
type MarkdownAttachment struct {
	// Known — разложенные по ключам темы значения. Ключи те же, что в
	// skills.BriefTopics: переводить их второй раз не нужно.
	Known map[string][]string
	// ObjectName, Timeline, BudgetSignal, DecisionMaker — поля, которые бот
	// показывают в «что я понял» отдельной строкой.
	ObjectName    string
	ObjectPurpose string
	ObjectArea    string
	// CustomerAddress — адресат КП (организация и лицо), а не заказчик
	// исследования из прежних файлов.
	CustomerAddress string
	// PlannedSystems — системы, которые менеджер собирается предложить.
	PlannedSystems []string
	Timeline       string
	BudgetSignal   string
	DecisionMaker  string
	CustomerPains  []string
	// Notes — всё, что не разложилось по ключам. Терять текст нельзя: в нём
	// бывают самые нужные подробности.
	Notes string
}

// HasContent — есть ли что раскладывать в бриф.
func (a MarkdownAttachment) HasContent() bool {
	return len(a.Known) > 0 || model.NonEmpty(a.Notes)
}

// attachmentFieldAliases — как поле называется в файле и в какой ключ темы оно
// превращается. Подписи сверяются целиком, без учёта регистра.
var attachmentFieldAliases = []struct {
	pattern []string
	key     string
}{
	{pattern: []string{"объект", "объект исследования", "наименование объекта"}, key: "object_name"},
	{pattern: []string{"назначение", "назначение объекта", "функциональное назначение"}, key: "object_purpose"},
	{pattern: []string{"площадь", "площадь объекта", "объём работ", "объем работ"}, key: "object_area"},
	{pattern: []string{"адресат", "кому", "адрес кп"}, key: "customer_address"},
	{pattern: []string{"задача", "цель", "задачи"}, key: "customer_goals"},
	{pattern: []string{"боли", "боли заказчика", "проблемы"}, key: "customer_pains"},
	{pattern: []string{"риски", "риски проекта"}, key: "project_risks"},
	{pattern: []string{"системы", "система", "предлагаемые системы"}, key: "planned_systems"},
	{pattern: []string{"возражения"}, key: "objections"},
	{pattern: []string{"бюджет"}, key: "budget_signal"},
	{pattern: []string{"решение", "лпр", "кто решает"}, key: "decision_maker"},
	{pattern: []string{"сроки", "срок поставки", "срок"}, key: "timeline"},
	{pattern: []string{"почему мы", "конкуренты"}, key: "why_us"},
	{pattern: []string{"примечания", "прочее", "комментарии"}, key: "notes"},
	{pattern: []string{"заказчик", "клиент", "адресат"}, key: "customer"},
}

var (
	// Маркер списка в начале строки — признак пункта, а не поля.
	reAttachmentListMarker = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s`)
	// Подпись поля — одно-три слова. Несколько слов подряд — уже предложение
	// текста («Он сказал: не успеем»), а не поле.
	reAttachmentFieldLabel = regexp.MustCompile(`^\S+(?:\s+\S+){0,2}$`)
	// reAttachmentSeparator находит двоеточие или тире, отделяющее подпись от
	// значения. Тире — только с пробелами вокруг, иначе «ТН-Стилобат» станет
	// полем «ТН». Двоеточие может стоять и в конце строки: «Боли:» с списком
	// под ней — так люди и пишут.
	reAttachmentSeparator = regexp.MustCompile(`: | — | – | - `)
)

// ParseMarkdownAttachment раскладывает текст по полям брифа.
//
// Правило простое и предсказуемое: строка «Ключ: значение» — поле; строка
// списка сразу под ней — пункт этого поля; всё остальное — заметки. Такое
// правило человек может объяснить себе сам, без инструкции.
func ParseMarkdownAttachment(text string) MarkdownAttachment {
	out := MarkdownAttachment{Known: map[string][]string{}}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	lines = stripAttachmentFrontmatter(lines)

	var notes []string
	// currentKey — поле брифа, к которому относится список ниже. Пусто — пункт
	// уйдёт в заметки: незнакомое поле («Адрес объекта») в бриф не кладём, но
	// и его пункты терять нельзя.
	currentKey := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			currentKey = ""
			continue
		}
		if key, value, ok := splitAttachmentField(trimmed); ok {
			currentKey = key
			switch {
			case key != "" && model.NonEmpty(value):
				out.Known[key] = append(out.Known[key], value)
			case key != "":
				// Поле без значения на этой строке («Боли:» и список под
				// ним) — нормальный вид файла: подпись не потерялась, список
				// соберётся ниже, в currentKey.
			default:
				// Незнакомое поле не теряется: «Адрес объекта», «Заказчик»
				// бывают самыми нужными подробностями, и человек должен
				// видеть строку целиком.
				notes = append(notes, trimmed)
			}
			continue
		}
		if item, ok := attachmentListItem(trimmed); ok {
			if currentKey != "" {
				out.Known[currentKey] = append(out.Known[currentKey], item)
			} else {
				notes = append(notes, item)
			}
			continue
		}
		// Заголовок markdown — не поле, но и не заметка: он повторяет смысл
		// файла («Бриф по объекту»), и в notes ему не место.
		if strings.HasPrefix(trimmed, "#") {
			currentKey = ""
			continue
		}
		currentKey = ""
		notes = append(notes, trimmed)
	}

	out.fillFields(strings.Join(notes, "\n"))
	return out
}

// fillFields раскладывает известные ключи по наглядным полям. В Known они
// остаются: бот показывает состав брифа, а не только названные поля.
//
// notes передаются сюда уже собранными: заказчик дописывается к ним, и порядок
// «сначала файл, потом разобранное» сохраняется.
func (a *MarkdownAttachment) fillFields(notes string) {
	first := func(key string) string {
		if list := a.Known[key]; len(list) > 0 {
			return list[0]
		}
		return ""
	}
	a.ObjectName = first("object_name")
	a.ObjectPurpose = first("object_purpose")
	a.ObjectArea = first("object_area")
	a.CustomerAddress = first("customer_address")
	a.PlannedSystems = append([]string{}, a.Known["planned_systems"]...)
	a.Timeline = first("timeline")
	a.BudgetSignal = first("budget_signal")
	a.DecisionMaker = first("decision_maker")
	a.CustomerPains = append([]string{}, a.Known["customer_pains"]...)

	// Заказчик не поле брифа (он в карточке КП), поэтому дописываем его в
	// заметки: бот покажет как «из файла».
	if customer := first("customer"); customer != "" {
		if model.NonEmpty(notes) {
			notes += "\nЗаказчик: " + customer
		} else {
			notes = "Заказчик: " + customer
		}
	}
	delete(a.Known, "customer")
	a.Notes = notes
}

// splitAttachmentField ищет «Ключ: значение» и возвращает ключ темы.
//
// Незнакомый ключ возвращает ok = true с пустой key: строка всё равно поле, и
// вызывающий отправит её в заметки, а не в список поля выше.
func splitAttachmentField(line string) (key, value string, ok bool) {
	loc := reAttachmentSeparator.FindStringIndex(line)
	if loc == nil {
		// «Боли:» — двоеточие в самом конце строки, значение — список под ней.
		// Без этого частый вид файла читался бы как заголовок, а пункты под
		// ним уходили бы в «прочее».
		if i := strings.LastIndex(line, ":"); i > 0 && strings.TrimSpace(line[i+1:]) == "" {
			loc = []int{i, i + 1}
		}
	}
	if loc == nil || loc[0] <= 0 {
		return "", "", false
	}
	sep := loc[0]
	value = strings.TrimSpace(line[loc[1]:])
	rawLabel := strings.TrimSpace(line[:sep])
	if reAttachmentListMarker.MatchString(rawLabel) {
		// «- Протечки по стилобату: видны на -1 этаже» — пункт списка с
		// двоеточием внутри, а не поле «- Протечки по стилобату».
		return "", "", false
	}
	if rawLabel == "" || !reAttachmentFieldLabel.MatchString(rawLabel) {
		return "", "", false
	}
	return matchAttachmentKey(rawLabel), value, true
}

// matchAttachmentKey сопоставляет подпись поля с ключом темы брифа.
//
// Только точное совпадение после нормализации: «Боли заказчика: протечки» —
// поле, «Протечки по стилобату: видны» — предложение, и подписью его делать
// нельзя. Терпимость к синонимам живёт в списке подписей, а не в подстрочном
// поиске.
func matchAttachmentKey(label string) string {
	normalized := normalizeAttachmentLabel(label)
	if normalized == "" {
		return ""
	}
	for _, alias := range attachmentFieldAliases {
		for _, p := range alias.pattern {
			if normalized == p {
				return alias.key
			}
		}
	}
	return ""
}

// normalizeAttachmentLabel приводит подпись к сравнимому виду: без разметки,
// регистра и лишних пробелов.
func normalizeAttachmentLabel(label string) string {
	s := strings.ToLower(strings.TrimSpace(label))
	s = strings.TrimPrefix(s, "-")
	s = strings.TrimPrefix(s, "*")
	s = strings.TrimSuffix(s, ":")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSuffix(s, ":")
}

// attachmentListItem — строка списка «- …», «* …», «1. …».
func attachmentListItem(line string) (string, bool) {
	for _, marker := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(line, marker) {
			item := strings.TrimSpace(line[len(marker):])
			if item == "" {
				return "", false
			}
			return item, true
		}
	}
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i > 0 && i+1 < len(line) && (line[i] == '.' || line[i] == ')') {
		item := strings.TrimSpace(line[i+1:])
		if item == "" {
			return "", false
		}
		return item, true
	}
	return "", false
}

// stripAttachmentFrontmatter убирает YAML-шапку заметки: её поля служебные, в
// бриф им нельзя (иначе тег commercial_propose уедет в «прочее»).
func stripAttachmentFrontmatter(lines []string) []string {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return lines
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return lines[i+1:]
		}
	}
	return nil
}

// XlsxAttachment — что бот понял из таблицы.
type XlsxAttachment struct {
	// IsSpecification — таблица похожа на спецификацию: есть наименование и
	// количество. Тогда бот предложит ею заменить состав КП.
	IsSpecification bool
	// Rows — позиции спецификации.
	Rows []SpecRow
	// Text — содержимое листа построчно, когда это контекст, а не
	// спецификация.
	Text []string
	// Warnings — что не сходится внутри самого файла (сумма не равна
	// количество × цена). Цены с каталогом сверяет отдельный шаг: здесь только
	// очевидная арифметика.
	Warnings []string
}

// SpecRow — строка спецификации из файла.
type SpecRow struct {
	Name  string
	Unit  string
	Qty   float64
	Price float64
	Sum   float64
}

// MaxSpecRows — предел: спецификации в КП — десятки строк. Огромный файл
// (выгрузка из 1С на тысячи строк) не должен ни раздувать ответ боту, ни
// превращаться в состав из тысячи позиций по ошибке.
const MaxSpecRows = 200

var errXlsxNoSheets = errors.New("в файле нет листов")

// ParseXlsxAttachment разбирает книгу .xlsx.
//
// Берётся первый лист с спецификацией, иначе первый непустой лист как
// контекст: в присланных файлах таблица почти всегда на первом листе, а второй
// — служебный.
func ParseXlsxAttachment(data []byte) (XlsxAttachment, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return XlsxAttachment{}, fmt.Errorf("открыть xlsx: %w", err)
	}
	parts := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, "xl/") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return XlsxAttachment{}, err
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return XlsxAttachment{}, err
		}
		parts[f.Name] = body
	}

	shared := parseXlsxSharedStrings(parts["xl/sharedStrings.xml"])
	var fallback XlsxAttachment
	for i := 1; ; i++ {
		raw, ok := parts[fmt.Sprintf("xl/worksheets/sheet%d.xml", i)]
		if !ok {
			break
		}
		rows := parseXlsxSheetRows(raw, shared)
		if len(rows) == 0 {
			continue
		}
		result := classifyXlsxSheet(rows)
		if result.IsSpecification {
			return result, nil
		}
		if len(fallback.Text) == 0 {
			fallback = result
		}
	}
	if len(fallback.Text) == 0 {
		return XlsxAttachment{}, errXlsxNoSheets
	}
	return fallback, nil
}

// classifyXlsxSheet решает, спецификация перед нами или контекст.
//
// Лист обходится ЦЕЛИКОМ, а не до первой шапки: живой пользовательский КП
// (2026-11-01) — это две системы на одном листе, у каждой своя шапка состава,
// между ними описание и преимущества. Разбор первой шапки терял вторую систему.
func classifyXlsxSheet(rows [][]string) XlsxAttachment {
	spec := collectXlsxSpec(rows)
	if spec.IsSpecification {
		return spec
	}
	return XlsxAttachment{Text: flushXlsxRows(rows)}
}

// collectXlsxSpec собирает состав по ВСЕМ шапкам листа.
//
// Если шапок нет вовсе — пробует разобрать лист как выгрузку без заголовков:
// люди присылают и так.
func collectXlsxSpec(rows [][]string) XlsxAttachment {
	cols, start := findSpecHeader(rows, 0)
	if !cols.ok() {
		if spec, _, _ := buildXlsxSpec(noSpecColumns(), rows); spec.IsSpecification {
			return spec
		}
		return XlsxAttachment{}
	}
	var out XlsxAttachment
	for {
		part, warnings, next := buildXlsxSpec(cols, rows[start:])
		out.Rows = append(out.Rows, part.Rows...)
		out.Warnings = append(out.Warnings, warnings...)
		if len(out.Rows) >= MaxSpecRows {
			out.Rows = out.Rows[:MaxSpecRows]
			out.Warnings = append(out.Warnings, fmt.Sprintf("взяты только первые %d строк", MaxSpecRows))
			break
		}
		nextCols, nextStart := findSpecHeader(rows, start+next)
		if !nextCols.ok() {
			break
		}
		cols, start = nextCols, nextStart
	}
	if len(out.Rows) == 0 {
		return XlsxAttachment{}
	}
	out.IsSpecification = true
	return out
}

// buildXlsxSpec собирает строки спецификации по колонкам шапки.
//
// Идёт до конца листа, но останавливается, встретив следующую шапку: её строки
// разберёт вызывающий своим набором колонок. Иначе вторая шапка («Наименование»,
// «Ед. изм.», …) сама стала бы позицией состава.
//
// Третий результат — сколько строк листа израсходовано (включая шапку и
// остановившую разбор строку). Без него поиск следующей шапки начинался бы
// там же, где предыдущая, и второй блок разбирался бы дважды (поймано тестом).
func buildXlsxSpec(cols specColumns, rows [][]string) (XlsxAttachment, []string, int) {
	var out XlsxAttachment
	if !cols.ok() {
		return out, nil, 0
	}
	var warnings []string
	used := 0
	for i, row := range rows {
		if i > 0 && cols.clashesWith(matchSpecColumns(row)) {
			break
		}
		used = i + 1
		name := xlsxCellAt(row, cols.name)
		if !model.NonEmpty(name) {
			continue
		}
		if isXlsxSectionLabel(name) {
			// «Устройство гидроизоляции фундамента (горизонталь)» — заголовок
			// раздела внутри таблицы: количество есть, материала нет.
			continue
		}
		item := SpecRow{
			Name:  name,
			Unit:  xlsxCellAt(row, cols.unit),
			Qty:   parseAttachmentNumber(xlsxCellAt(row, cols.qty)),
			Price: parseAttachmentNumber(xlsxCellAt(row, cols.price)),
			Sum:   parseAttachmentNumber(xlsxCellAt(row, cols.sum)),
		}
		if item.Qty <= 0 {
			// Строка без количества — подзаголовок или примечание внутри
			// таблицы, а не позиция. В состав им нельзя.
			continue
		}
		if item.Price <= 0 && item.Sum <= 0 {
			// Ни цены, ни суммы — итоговая или служебная строка. Позиция без
			// цены придёт из каталога; из файла её брать нечем.
			continue
		}
		if item.Sum == 0 && item.Price > 0 {
			item.Sum = roundAttachmentMoney(item.Qty * item.Price)
		}
		if item.Sum > 0 && item.Price > 0 && roundAttachmentMoney(item.Qty*item.Price) != item.Sum {
			warnings = append(warnings, fmt.Sprintf(
				"«%s»: количество × цена = %s, а в файле сумма %s",
				item.Name,
				formatAttachmentNumber(item.Qty*item.Price),
				formatAttachmentNumber(item.Sum)))
		}
		out.Rows = append(out.Rows, item)
	}
	if len(out.Rows) == 0 {
		return XlsxAttachment{}, nil, used
	}
	out.IsSpecification = true
	out.Warnings = warnings
	return out, warnings, used
}

// noSpecColumns — «колонок не нашли»: пустой набор для разбора без шапки.
func noSpecColumns() specColumns {
	return specColumns{name: -1, unit: -1, qty: -1, price: -1, sum: -1}
}

// specColumns — номера колонок спецификации (с нуля), -1 если колонки нет.
type specColumns struct {
	name, unit, qty, price, sum int
}

// ok — по шапке можно собирать состав: есть наименование и количество.
func (c specColumns) ok() bool {
	return c.name >= 0 && c.qty >= 0
}

// clashesWith — признак того, что строка является ШАПКОЙ, а не позицией:
// у неё своё наименование и своё количество, и они в других колонках.
func (c specColumns) clashesWith(other specColumns) bool {
	return other.ok() && (other.name != c.name || other.qty != c.qty)
}

// matchSpecColumns раскладывает шапку по колонкам.
//
// Порядок проверок важен, а не порядок колонок: живой файл ставит между
// количеством и ценой «Коэффициент расхода», и разбор по порядку сделал бы
// ценой коэффициент (1,15 вместо 769,56). Поэтому каждая колонка опознаётся по
// СВОЕЙ подписи, и «ЕКН» тоже: номер — не наименование.
func matchSpecColumns(header []string) specColumns {
	cols := noSpecColumns()
	for i, cell := range header {
		label := normalizeAttachmentLabel(cell)
		if label == "" || isEknColumnLabel(label) {
			continue
		}
		switch {
		case cols.qty < 0 && isQtyColumnLabel(label):
			cols.qty = i
		case cols.sum < 0 && isSumColumnLabel(label):
			cols.sum = i
		case cols.price < 0 && isPriceColumnLabel(label):
			cols.price = i
		case cols.unit < 0 && isUnitColumnLabel(label):
			cols.unit = i
		case cols.name < 0 && isNameColumnLabel(label):
			cols.name = i
		}
	}
	return cols
}

// isEknColumnLabel — колонка номера ЕКН. Проверяется раньше остальных:
// «ЕКН» без подписи не должна попасть ни в наименование, ни куда-либо ещё.
func isEknColumnLabel(label string) bool {
	return label == "екн" || label == "ekn" || strings.HasPrefix(label, "екн ")
}

func isQtyColumnLabel(label string) bool {
	return strings.Contains(label, "кол-во") || strings.Contains(label, "количество") ||
		strings.HasPrefix(label, "кол ")
}

// isPriceColumnLabel — «цена», «стоимость единицы», «цена за ед».
//
// «Сметная стоимость» — это СУММА, а не цена: у «стоимости» проверяется
// отдельно, есть ли в подписи единица.
func isPriceColumnLabel(label string) bool {
	if strings.Contains(label, "цена") {
		return true
	}
	return strings.Contains(label, "стоимость") &&
		(strings.Contains(label, "единиц") || strings.Contains(label, "ед.") || strings.Contains(label, "за "))
}

func isSumColumnLabel(label string) bool {
	return strings.Contains(label, "сумма") || strings.Contains(label, "итого") ||
		strings.Contains(label, "сметная стоимость")
}

func isNameColumnLabel(label string) bool {
	switch {
	case strings.Contains(label, "наименован"):
		return true
	case label == "материал", label == "позиция", label == "товар":
		return true
	}
	return strings.Contains(label, "продукц")
}

// isUnitColumnLabel — «Ед. изм.», «единица измерения», «ед.».
func isUnitColumnLabel(label string) bool {
	switch {
	case label == "ед", strings.HasPrefix(label, "ед."), label == "ед изм":
		return true
	}
	return strings.Contains(label, "единиц")
}

// isXlsxSectionLabel — заголовок раздела внутри таблицы, а не позиция.
//
// Живой файл: «Устройство гидроизоляции фундамента (горизонталь)» с площадью
// 4800 м2 и суммой по разделу. Это не материал, и в состав ему нельзя.
//
// Второй вид — строка номеров колонок под шапкой («2 | 3 | 4 | 5 | 6 | 7»):
// человек нумерует графы бланка. Для разбора это не позиция, а оглавление
// таблицы; без этой проверки в состав уезжала строка «2 — 3 × 6 = 7».
func isXlsxSectionLabel(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if strings.HasPrefix(lower, "устройство ") {
		return true
	}
	_, err := strconv.ParseFloat(strings.ReplaceAll(lower, ",", "."), 64)
	return err == nil
}

// findSpecHeader ищет ближайшую шапку состава, начиная со строки from.
// Возвращает колонки и номер строки ДАННЫХ (сразу под шапкой).
func findSpecHeader(rows [][]string, from int) (specColumns, int) {
	for i := from; i < len(rows); i++ {
		cols := matchSpecColumns(rows[i])
		if cols.ok() {
			return cols, i + 1
		}
	}
	return noSpecColumns(), from
}

// flushXlsxRows превращает строки в текст: пустые ячейки пропускаются, чтобы
// не получить «|  |  |  |» вместо содержания.
func flushXlsxRows(rows [][]string) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		cells := make([]string, 0, len(row))
		for _, cell := range row {
			if model.NonEmpty(cell) {
				cells = append(cells, strings.TrimSpace(cell))
			}
		}
		if len(cells) == 0 {
			continue
		}
		out = append(out, strings.Join(cells, " | "))
	}
	return out
}

func xlsxCellAt(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

// parseAttachmentNumber читает число из ячейки: неразрывные и обычные пробелы —
// разделители тысяч, запятая — десятичный знак, знак рубля отбрасывается.
func parseAttachmentNumber(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	s = strings.ReplaceAll(s, "\u00a0", "")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "руб.", "")
	s = strings.ReplaceAll(s, "₽", "")
	s = strings.ReplaceAll(s, ",", ".")
	// 1.234.567 — разделитель тысяч точками.
	if strings.Count(s, ".") > 1 {
		last := strings.LastIndex(s, ".")
		s = strings.ReplaceAll(s[:last], ".", "") + s[last:]
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// roundAttachmentMoney — деньги в файле округляются до копеек: 0.1+0.2 не
// должно выглядеть как расхождение.
func roundAttachmentMoney(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

// formatAttachmentNumber — число для сообщения человеку, без хвостов.
func formatAttachmentNumber(v float64) string {
	s := strings.TrimRight(strings.TrimRight(strconv.FormatFloat(v, 'f', 2, 64), "0"), ".")
	return strings.ReplaceAll(s, ".", ",")
}

// --- разбор xlsx: общий словарь строк и листы ---

type xlsxSharedStrings struct {
	Items []xlsxSharedItem `xml:"si"`
}

type xlsxSharedItem struct {
	Text string `xml:"t"`
	Runs []struct {
		Text string `xml:"t"`
	} `xml:"r"`
}

// parseXlsxSharedStrings читает общий словарь строк. В .xlsx текст ячеек лежит
// не в листе, а здесь, и в листе стоит его номер.
func parseXlsxSharedStrings(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var sst xlsxSharedStrings
	if err := xml.Unmarshal(raw, &sst); err != nil {
		return nil
	}
	out := make([]string, 0, len(sst.Items))
	for _, item := range sst.Items {
		text := item.Text
		if text == "" {
			var b strings.Builder
			for _, r := range item.Runs {
				b.WriteString(r.Text)
			}
			text = b.String()
		}
		out = append(out, text)
	}
	return out
}

type xlsxWorksheet struct {
	Rows []xlsxRow `xml:"sheetData>row"`
}

// xlsxRow — строка листа. Отдельный тип ради адреса ячейки: он лежит в
// атрибуте `r`, без него пропущенная ячейка сдвинула бы весь ряд влево.
type xlsxRow struct {
	Cells []xlsxCell `xml:"c"`
}

type xlsxCell struct {
	// Ref — адрес ячейки («B35»), а не её порядок в строке. В живых файлах
	// пропущенные ячейки просто отсутствуют, и счёт по порядку съезжает
	// (живой случай 2026-11-01: цена оказывалась в колонке коэффициента).
	Ref   string `xml:"r,attr"`
	Type  string `xml:"t,attr"`
	Value string `xml:"v"`
	Text  string `xml:"is>t"`
}

// parseXlsxSheetRows читает лист в таблицу строк. Ячейки со словарным типом
// «s» разворачиваются через общий словарь, адрес ячейки задаёт колонку.
func parseXlsxSheetRows(raw []byte, shared []string) [][]string {
	var ws xlsxWorksheet
	if err := xml.Unmarshal(raw, &ws); err != nil {
		return nil
	}
	rows := make([][]string, 0, len(ws.Rows))
	for _, row := range ws.Rows {
		cells := make([]string, 0, len(row.Cells))
		for _, c := range row.Cells {
			value := c.Value
			switch c.Type {
			case "s":
				idx, err := strconv.Atoi(strings.TrimSpace(c.Value))
				if err != nil || idx < 0 || idx >= len(shared) {
					value = ""
				} else {
					value = shared[idx]
				}
			case "inlineStr":
				value = c.Text
			}
			cells = xlsxPlaceCell(cells, c.Ref, strings.TrimSpace(value))
		}
		rows = append(rows, cells)
	}
	return rows
}

// xlsxPlaceCell ставит значение на колонку из адреса ячейки, добивая строку
// пустыми местами до неё. Адрес без номера колонки (мусор во входе) — значение
// дописывается в конец: потерять его хуже, чем поставить не туда.
func xlsxPlaceCell(cells []string, ref, value string) []string {
	col := xlsxColumnIndex(ref)
	switch {
	case col < 0:
		return append(cells, value)
	case col < len(cells):
		cells[col] = value
		return cells
	default:
		for len(cells) < col {
			cells = append(cells, "")
		}
		return append(cells, value)
	}
}

// xlsxColumnIndex переводит «B35» в номер колонки (с нуля): B → 1.
// Пусто и мусор — -1. Номер строки в адресе не нужен: строки уже разделены.
func xlsxColumnIndex(ref string) int {
	index, letters := 0, 0
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}
		index = index*26 + int(r-'A'+1)
		letters++
	}
	if letters == 0 {
		return -1
	}
	return index - 1
}

// XlsxSheetRows читает ПЕРВЫЙ непустой лист как есть — строка за строкой.
//
// Нужна загрузке таблицы менеджера (2026-10-30): там колонки ЕКН и Цена, а
// ParseXlsxAttachment ищет только спецификацию (наименование+количество) и
// остальное отдаёт текстом. Разбор колонок и сопоставление — в пакете main,
// здесь только чтение книги без догадок о смысле.
func XlsxSheetRows(data []byte) ([][]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("открыть xlsx: %w", err)
	}
	parts := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, "xl/") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		parts[f.Name] = body
	}
	shared := parseXlsxSharedStrings(parts["xl/sharedStrings.xml"])
	for i := 1; ; i++ {
		raw, ok := parts[fmt.Sprintf("xl/worksheets/sheet%d.xml", i)]
		if !ok {
			break
		}
		rows := parseXlsxSheetRows(raw, shared)
		if len(rows) > 0 {
			return rows, nil
		}
	}
	return nil, errXlsxNoSheets
}
