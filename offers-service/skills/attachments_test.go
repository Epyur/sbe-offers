package skills

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// Файлы, которые менеджер присылает в бота (шаг 5 пересмотренного плана).
//
// Разбор — чистые функции без сетей и БД: только байты на входе и разобранный
// результат на выходе. Поэтому и проверяется без стенда.

func TestDetectAttachmentType(t *testing.T) {
	xlsx := buildTestXlsx(t, xlsxSheet{
		shared: []string{"Наименование", "Ед.", "Кол-во", "Цена"},
		rows:   [][]string{{"Наименование", "Ед.", "Кол-во", "Цена"}},
	})
	docx := buildTestZip(t, map[string]string{
		"word/document.xml": `<w:document><w:body><w:p><w:r><w:t>Привет</w:t></w:r></w:p></w:body></w:document>`,
	})

	tests := []struct {
		name string
		file string
		data []byte
		want string
	}{
		{"markdown по расширению", "бриф.md", []byte("# Объект\nЖК Поэма"), "md"},
		{"markdown без расширения", "", []byte("Объект: ЖК Поэма"), "md"},
		{"xlsx по сигнатуре", "спецификация.bin", xlsx, "xlsx"},
		{"docx по сигнатуре", "письмо.bin", docx, "docx"},
		{"старый xls", "старый.xls", []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, "xls"},
		{"pdf", "документ.pdf", []byte("%PDF-1.7\n"), "pdf"},
		{"пустой файл", "пусто.md", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectAttachmentType(tt.file, tt.data)
			if got != tt.want {
				t.Fatalf("DetectAttachmentType(%q) = %q, ожидали %q", tt.file, got, tt.want)
			}
		})
	}
}

func TestParseMarkdownAttachment(t *testing.T) {
	source := strings.Join([]string{
		"# Бриф по объекту",
		"",
		"Объект: ЖК «Поэма», корпус 3",
		"Заказчик: ООО Строй-Инвест",
		"Сроки: до 15 сентября",
		"",
		"Боли:",
		"- Протечки по стилобату",
		"- Срыв сроков подрядчиком",
		"",
		"Свободный текст про объект, которого нет в ключах.",
	}, "\n")

	got := ParseMarkdownAttachment(source)

	if got.ObjectName != "ЖК «Поэма», корпус 3" {
		t.Fatalf("объект = %q", got.ObjectName)
	}
	if got.Timeline != "до 15 сентября" {
		t.Fatalf("сроки = %q", got.Timeline)
	}
	if len(got.CustomerPains) != 2 {
		t.Fatalf("боли = %#v, ожидали два пункта", got.CustomerPains)
	}
	if !strings.Contains(got.Notes, "Строй-Инвест") {
		t.Fatalf("незнакомый ключ потерялся: notes = %q", got.Notes)
	}
	if !strings.Contains(got.Notes, "Свободный текст") {
		t.Fatalf("свободный текст потерялся: notes = %q", got.Notes)
	}
	// «Заказчик» не поле брифа, но и не мусор: он дописывается в заметки
	// последним, чтобы менеджер увидел его в «что я понял».
	if !strings.Contains(got.Notes, "ООО Строй-Инвест") {
		t.Fatalf("заказчик потерялся: notes = %q", got.Notes)
	}
	if !got.HasContent() {
		t.Fatal("HasContent() = false при непустом содержимом")
	}
}

func TestParseMarkdownAttachmentIgnoresFrontmatter(t *testing.T) {
	source := "---\ntags: [commercial_propose]\n---\nОбъект: Склад\n"
	got := ParseMarkdownAttachment(source)
	if got.ObjectName != "Склад" {
		t.Fatalf("объект = %q", got.ObjectName)
	}
	if strings.Contains(got.Notes, "commercial_propose") {
		t.Fatalf("шапка заметки попала в notes: %q", got.Notes)
	}
}

func TestParseMarkdownAttachmentEmpty(t *testing.T) {
	got := ParseMarkdownAttachment("   \n\n")
	if got.HasContent() {
		t.Fatal("пустой файл не должен давать содержимое")
	}
	if len(got.Known) != 0 {
		t.Fatalf("Known = %#v, ожидали пусто", got.Known)
	}
}

func TestParseXlsxSpecification(t *testing.T) {
	sheet := xlsxSheet{
		shared: []string{
			"Наименование", "Ед.", "Кол-во", "Цена", "Сумма",
			"Стилобат Эксперт", "м2", "120", "1850", "222000",
			"Песок на подушку", "т", "18", "900", "16200",
		},
		rows: [][]string{
			{"Наименование", "Ед.", "Кол-во", "Цена", "Сумма"},
			{"Стилобат Эксперт", "м2", "120", "1850", "222000"},
			{"Песок на подушку", "т", "18", "900", "16200"},
		},
	}
	got, err := ParseXlsxAttachment(buildTestXlsx(t, sheet))
	if err != nil {
		t.Fatalf("разбор упал: %v", err)
	}
	if !got.IsSpecification {
		t.Fatalf("таблица не распознана как спецификация: %#v", got)
	}
	if len(got.Rows) != 2 {
		t.Fatalf("строк %d, ожидали 2", len(got.Rows))
	}
	first := got.Rows[0]
	if first.Name != "Стилобат Эксперт" || first.Unit != "м2" || first.Qty != 120 || first.Price != 1850 {
		t.Fatalf("первая строка разобрана неверно: %#v", first)
	}
	if first.Sum != 222000 {
		t.Fatalf("сумма первой строки = %v", first.Sum)
	}
}

func TestParseXlsxSpecificationDetectsMismatchedSum(t *testing.T) {
	sheet := xlsxSheet{
		shared: []string{
			"Наименование", "Кол-во", "Цена", "Сумма",
			"Плитка", "10", "100", "1500",
		},
		rows: [][]string{
			{"Наименование", "Кол-во", "Цена", "Сумма"},
			{"Плитка", "10", "100", "1500"},
		},
	}
	got, err := ParseXlsxAttachment(buildTestXlsx(t, sheet))
	if err != nil {
		t.Fatalf("разбор упал: %v", err)
	}
	if len(got.Warnings) == 0 {
		t.Fatal("расхождение суммы и количества×цены не замечено")
	}
}

func TestParseXlsxContext(t *testing.T) {
	// Таблица без колонок состава — это контекст, а не спецификация.
	sheet := xlsxSheet{
		shared: []string{"Параметр", "Значение", "Объект", "ЖК Поэма", "Площадь", "1200"},
		rows: [][]string{
			{"Параметр", "Значение"},
			{"Объект", "ЖК Поэма"},
			{"Площадь", "1200"},
		},
	}
	got, err := ParseXlsxAttachment(buildTestXlsx(t, sheet))
	if err != nil {
		t.Fatalf("разбор упал: %v", err)
	}
	if got.IsSpecification {
		t.Fatal("таблица без колонок состава не должна считаться спецификацией")
	}
	if len(got.Text) == 0 {
		t.Fatalf("текст контекста пуст: %#v", got)
	}
	if !strings.Contains(strings.Join(got.Text, "\n"), "ЖК Поэма") {
		t.Fatalf("содержимое потерялось: %#v", got.Text)
	}
}

func TestParseXlsxRejectsBrokenFile(t *testing.T) {
	if _, err := ParseXlsxAttachment([]byte("не zip")); err == nil {
		t.Fatal("битый файл разобран без ошибки")
	}
}

func TestParseXlsxPrefersFirstNonEmptySheet(t *testing.T) {
	sheet := xlsxSheet{
		shared: []string{"Наименование", "Кол-во", "Цена", "Труба", "5", "200"},
		rows: [][]string{
			{"Наименование", "Кол-во", "Цена"},
			{"Труба", "5", "200"},
		},
	}
	data := buildTestXlsxMulti(t, []xlsxSheet{{name: "Пусто"}, sheet})
	got, err := ParseXlsxAttachment(data)
	if err != nil {
		t.Fatalf("разбор упал: %v", err)
	}
	if len(got.Rows) != 1 || got.Rows[0].Name != "Труба" {
		t.Fatalf("взят не тот лист: %#v", got.Rows)
	}
}

// --- вспомогательное: сборка минимального xlsx ---

type xlsxSheet struct {
	name   string
	shared []string
	rows   [][]string
}

func buildTestZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip: %v", err)
		}
		if _, err := w.Write([]byte(files[name])); err != nil {
			t.Fatalf("zip: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip: %v", err)
	}
	return buf.Bytes()
}

func buildTestXlsx(t *testing.T, sheet xlsxSheet) []byte {
	t.Helper()
	return buildTestXlsxMulti(t, []xlsxSheet{sheet})
}

func buildTestXlsxMulti(t *testing.T, sheets []xlsxSheet) []byte {
	t.Helper()
	files := map[string]string{
		"xl/workbook.xml": `<workbook><sheets>` + workbookSheets(sheets) + `</sheets></workbook>`,
	}
	// Общий словарь один на книгу: строки всех листов складываются в него.
	shared := make([]string, 0)
	for _, s := range sheets {
		for _, v := range s.shared {
			if indexOf(shared, v) < 0 {
				shared = append(shared, v)
			}
		}
	}
	var sharedXML strings.Builder
	sharedXML.WriteString(`<sst>`)
	for _, s := range shared {
		sharedXML.WriteString(`<si><t>` + s + `</t></si>`)
	}
	sharedXML.WriteString(`</sst>`)
	files["xl/sharedStrings.xml"] = sharedXML.String()

	for i, s := range sheets {
		files["xl/worksheets/sheet"+itoa(i+1)+".xml"] = sheetXML(s, shared)
	}
	return buildTestZip(t, files)
}

func workbookSheets(sheets []xlsxSheet) string {
	var b strings.Builder
	for i, s := range sheets {
		b.WriteString(`<sheet name="` + s.name + `" sheetId="` + itoa(i+1) + `" r:id="rId` + itoa(i+1) + `"/>`)
	}
	return b.String()
}

// sheetXML пишет строки листа. Строки, которые встречаются в sharedStrings,
// отдаются как тип "s" (общий словарь), числа — как есть: так xlsx и устроен,
// и разбор обязан понимать оба вида ячеек.
func sheetXML(s xlsxSheet, shared []string) string {
	var b strings.Builder
	b.WriteString(`<worksheet><sheetData>`)
	for r, row := range s.rows {
		b.WriteString(`<row r="` + itoa(r+1) + `">`)
		for c, cell := range row {
			ref := string(rune('A'+c)) + itoa(r+1)
			b.WriteString(`<c r="` + ref + `"`)
			if isNumericCell(cell) {
				b.WriteString(`><v>` + cell + `</v></c>`)
				continue
			}
			b.WriteString(` t="s"><v>` + itoa(indexOf(shared, cell)) + `</v></c>`)
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData></worksheet>`)
	return b.String()
}

func isNumericCell(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && r != '.' && r != '-' {
			return false
		}
	}
	return true
}

func indexOf(list []string, value string) int {
	for i, s := range list {
		if s == value {
			return i
		}
	}
	return -1
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
