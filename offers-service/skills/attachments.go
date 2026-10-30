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
	Timeline      string
	BudgetSignal  string
	DecisionMaker string
	CustomerPains []string
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
	{pattern: []string{"задача", "цель", "задачи"}, key: "customer_goals"},
	{pattern: []string{"боли", "боли заказчика", "проблемы"}, key: "customer_pains"},
	{pattern: []string{"риски", "риски проекта"}, key: "project_risks"},
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
func classifyXlsxSheet(rows [][]string) XlsxAttachment {
	if idx, ok := findXlsxHeaderRow(rows); ok {
		spec, _ := buildXlsxSpec(rows[idx], rows[idx+1:])
		if spec.IsSpecification {
			return spec
		}
		// Шапка есть, но колонок состава нет — это контекст: отдаём текст
		// таблицы целиком, вместе с шапкой.
		return XlsxAttachment{Text: flushXlsxRows(rows)}
	}
	// Шапки не нашли — пробуем без неё (выгрузка без строки заголовков).
	if spec, _ := buildXlsxSpec(nil, rows); spec.IsSpecification {
		return spec
	}
	return XlsxAttachment{Text: flushXlsxRows(rows)}
}

// buildXlsxSpec собирает строки спецификации по колонкам шапки.
func buildXlsxSpec(header []string, rows [][]string) (XlsxAttachment, []string) {
	cols := matchSpecColumns(header)
	var out XlsxAttachment
	if cols.name < 0 || cols.qty < 0 {
		return out, nil
	}
	var warnings []string
	for _, row := range rows {
		name := xlsxCellAt(row, cols.name)
		if !model.NonEmpty(name) {
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
		if len(out.Rows) >= MaxSpecRows {
			warnings = append(warnings, fmt.Sprintf("взяты только первые %d строк", MaxSpecRows))
			break
		}
	}
	if len(out.Rows) == 0 {
		return XlsxAttachment{}, nil
	}
	out.IsSpecification = true
	out.Warnings = warnings
	return out, warnings
}

// specColumns — номера колонок спецификации (с нуля), -1 если колонки нет.
type specColumns struct {
	name, unit, qty, price, sum int
}

func matchSpecColumns(header []string) specColumns {
	cols := specColumns{name: -1, unit: -1, qty: -1, price: -1, sum: -1}
	for i, cell := range header {
		label := normalizeAttachmentLabel(cell)
		switch {
		case cols.name < 0 && isNameColumnLabel(label):
			cols.name = i
		case cols.unit < 0 && isUnitColumnLabel(label):
			cols.unit = i
		case cols.qty < 0 && strings.Contains(label, "кол-во") || cols.qty < 0 && strings.Contains(label, "количество"):
			cols.qty = i
		case cols.price < 0 && strings.Contains(label, "цена"):
			cols.price = i
		case cols.sum < 0 && strings.Contains(label, "сумма") || cols.sum < 0 && strings.Contains(label, "стоимость"):
			cols.sum = i
		}
	}
	return cols
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

func isUnitColumnLabel(label string) bool {
	switch {
	case label == "ед", strings.HasPrefix(label, "ед."), label == "ед изм":
		return true
	}
	return strings.Contains(label, "единиц")
}

// findXlsxHeaderRow — строка, похожая на шапку таблицы: в ней есть и
// наименование, и количество. Только по такой строке колонки и опознаются.
func findXlsxHeaderRow(rows [][]string) (int, bool) {
	limit := len(rows)
	if limit > 20 {
		// Шапку ниже двадцатой строки не ищем: это уже не шапка.
		limit = 20
	}
	for i := 0; i < limit; i++ {
		cols := matchSpecColumns(rows[i])
		if cols.name >= 0 && cols.qty >= 0 {
			return i, true
		}
	}
	return 0, false
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
	Rows []struct {
		Cells []struct {
			Type  string `xml:"t,attr"`
			Value string `xml:"v"`
			Text  string `xml:"is>t"`
		} `xml:"c"`
	} `xml:"sheetData>row"`
}

// parseXlsxSheetRows читает лист в таблицу строк. Ячейки со словарным типом
// «s» разворачиваются через общий словарь.
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
			cells = append(cells, strings.TrimSpace(value))
		}
		rows = append(rows, cells)
	}
	return rows
}
