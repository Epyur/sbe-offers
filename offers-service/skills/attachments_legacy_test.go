package skills

import (
	"os"
	"strings"
	"testing"
)

// Старый пользовательский КП в формате Excel (2026-11-01).
//
// Живой случай: менеджер прислал в бот КП, сделанный в прошлом году — шапка
// бланка, две системы, таблицы состава с колонкой «ЕКН», итоги и сноски.
// Разбор не нашёл в нём ни одной позиции и ответил «состава нет», хотя состав
// там есть: строки с наименованием, количеством и ценой.
//
// Файл лежит в testdata как настоящий xlsx (без картинок и внешних связей —
// они к разбору отношения не имеют): синтетическая книга не воспроизводит
// главного — ячеек без ссылки и разделов внутри таблицы.

func loadLegacyOffer(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/legacy-offer.xlsx")
	if err != nil {
		t.Fatalf("прочитать файл опыта: %v", err)
	}
	return data
}

// Заголовок старого КП: «Наименование материалов», «Ед. изм.», «Кол-во»,
// «Коэффициент расхода», «Стоимость единицы, руб.», «Сметная стоимость, руб.».
// Разделитель между наименованием и количеством — единица измерения, а не
// наименование+количество подряд, поэтому шапка обязана опознаваться по
// наименованию и количеству в ЛЮБОМ порядке колонок.
func TestParseLegacyOfferFindsComposition(t *testing.T) {
	got, err := ParseXlsxAttachment(loadLegacyOffer(t))
	if err != nil {
		t.Fatalf("разбор упал: %v", err)
	}
	if !got.IsSpecification {
		t.Fatalf("состав не опознан, текст вместо строк: %d строк текста", len(got.Text))
	}
	if len(got.Rows) < 15 {
		t.Fatalf("строк состава %d, ожидали не меньше 15: %#v", len(got.Rows), got.Rows)
	}
}

// Строка-раздел внутри таблицы («Устройство гидроизоляции фундамента
// (горизонталь)») — не позиция: у неё есть количество, но нет ни названия
// материала, ни цены. В состав она попасть не должна, а позиции после неё —
// должны.
func TestParseLegacyOfferSkipsSectionRows(t *testing.T) {
	got, err := ParseXlsxAttachment(loadLegacyOffer(t))
	if err != nil {
		t.Fatalf("разбор упал: %v", err)
	}
	for _, row := range got.Rows {
		if strings.HasPrefix(row.Name, "Устройство гидроизоляции") {
			t.Fatalf("строка-раздел попала в состав: %#v", row)
		}
	}
	// Позиция первой системы: мембрана V-SL 2 мм, 5520 м2 по 769,56.
	var found *SpecRow
	for i := range got.Rows {
		if strings.Contains(got.Rows[i].Name, "Logicbase V-SL 2 мм") {
			found = &got.Rows[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("мембрана 2 мм не найдена среди %d строк", len(got.Rows))
	}
	if found.Qty != 5520 {
		t.Fatalf("количество мембраны = %v, ожидали 5520", found.Qty)
	}
	if found.Price != 769.56 {
		t.Fatalf("цена мембраны = %v, ожидали 769,56 — колонка «Стоимость единицы»", found.Price)
	}
	if found.Unit != "м2" {
		t.Fatalf("единица мембраны = %q, ожидали м2", found.Unit)
	}
}

// «Коэффициент расхода» стоит между количеством и ценой. Если разбор считает
// колонки по порядку, а не по заголовку, ценой становится коэффициент (1,15),
// и в КП уезжает цена в 600 раз меньше.
func TestParseLegacyOfferIgnoresCoefficientColumn(t *testing.T) {
	got, err := ParseXlsxAttachment(loadLegacyOffer(t))
	if err != nil {
		t.Fatalf("разбор упал: %v", err)
	}
	for _, row := range got.Rows {
		if row.Price > 0 && row.Price < 10 {
			t.Fatalf("похоже, ценой стал коэффициент расхода: %#v", row)
		}
	}
}

// Второй блок файла («ТН-СТИЛОБАТ ЭКСПЕРТ ТРОТУАР») — та же таблица ниже по
// листу. Её позиции тоже должны попасть в состав: менеджер присылает КП с
// несколькими системами, и терять вторую нельзя.
func TestParseLegacyOfferReadsSecondBlock(t *testing.T) {
	got, err := ParseXlsxAttachment(loadLegacyOffer(t))
	if err != nil {
		t.Fatalf("разбор упал: %v", err)
	}
	for _, row := range got.Rows {
		if strings.Contains(row.Name, "V-SТ 1.6") || strings.Contains(row.Name, "V-ST 1.6") {
			if row.Qty != 3600 || row.Price != 625.21 {
				t.Fatalf("позиция второго блока разобрана неверно: %#v", row)
			}
			return
		}
	}
	t.Fatalf("позиции второй системы нет среди %d строк", len(got.Rows))
}

// Сноски и итоги («Итого:», «Цены указаны с учетом 22% НДС») в состав не
// попадают: у них нет ни количества, ни цены.
func TestParseLegacyOfferSkipsFootnotes(t *testing.T) {
	got, err := ParseXlsxAttachment(loadLegacyOffer(t))
	if err != nil {
		t.Fatalf("разбор упал: %v", err)
	}
	for _, row := range got.Rows {
		low := strings.ToLower(row.Name)
		if strings.Contains(low, "ндс") || strings.Contains(low, "минимальной партией") {
			t.Fatalf("сноска попала в состав: %#v", row)
		}
	}
}

// Ячейка xlsx без атрибута ref принадлежит позиции по порядку среди соседей,
// но пропущенная ячейка mid-row сдвигает всё вправо. Разбор обязан читать
// адрес ячейки (A1, B1, …), а не порядок в XML.
func TestParseXlsxUsesCellReferences(t *testing.T) {
	// Строка 2: B2 (наименование), D2 (количество), F2 (цена), G2 (сумма).
	// C2, E2 отсутствуют вовсе — это дырки, а не пустые строки.
	shared := []string{"Наименование материалов", "Ед. изм.", "Кол-во", "Стоимость единицы, руб.", "Мембрана", "м2"}
	sheet := xlsxSheet{
		shared: shared,
		rows:   nil,
		raw: `<worksheet><sheetData>
<row r="1">
<c r="B1" t="s"><v>0</v></c><c r="C1" t="s"><v>1</v></c><c r="D1" t="s"><v>2</v></c><c r="F1" t="s"><v>3</v></c>
</row>
<row r="2">
<c r="B2" t="s"><v>4</v></c><c r="C2" t="s"><v>5</v></c><c r="D2"><v>120</v></c><c r="F2"><v>769.56</v></c>
</row>
</sheetData></worksheet>`,
	}
	got, err := ParseXlsxAttachment(buildTestXlsxRaw(t, sheet))
	if err != nil {
		t.Fatalf("разбор упал: %v", err)
	}
	if !got.IsSpecification || len(got.Rows) != 1 {
		t.Fatalf("состав не опознан: %#v", got)
	}
	row := got.Rows[0]
	if row.Name != "Мембрана" || row.Qty != 120 || row.Price != 769.56 {
		t.Fatalf("строка с пропущенными ячейками разобрана неверно: %#v", row)
	}
}

// Два блока таблицы на одном листе: шапка встречается дважды, и второй блок
// обязан читаться своей шапкой, а не первой. Живой файл именно так и устроен.
func TestParseXlsxReadsRepeatedHeaderBlocks(t *testing.T) {
	shared := []string{"Наименование", "Кол-во", "Цена", "Мембрана", "Грунтовка", "Геотекстиль"}
	sheet := xlsxSheet{
		shared: shared,
		raw: `<worksheet><sheetData>
<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="s"><v>2</v></c></row>
<row r="2"><c r="A2" t="s"><v>3</v></c><c r="B2"><v>10</v></c><c r="C2"><v>100</v></c></row>
<row r="5"><c r="A5" t="s"><v>0</v></c><c r="B5" t="s"><v>1</v></c><c r="C5" t="s"><v>2</v></c></row>
<row r="6"><c r="A6" t="s"><v>5</v></c><c r="B6"><v>20</v></c><c r="C6"><v>108</v></c></row>
</sheetData></worksheet>`,
	}
	got, err := ParseXlsxAttachment(buildTestXlsxRaw(t, sheet))
	if err != nil {
		t.Fatalf("разбор упал: %v", err)
	}
	if len(got.Rows) != 2 {
		t.Fatalf("строк %d, ожидали 2 — оба блока: %#v", len(got.Rows), got.Rows)
	}
	if got.Rows[1].Name != "Геотекстиль" || got.Rows[1].Qty != 20 {
		t.Fatalf("второй блок разобран неверно: %#v", got.Rows[1])
	}
}

// Колонка «ЕКН» рядом с наименованием: она НЕ наименование. Если разбор
// возьмёт первую подходящую по подстроке колонку, в названии окажется номер.
func TestParseXlsxIgnoresEknColumn(t *testing.T) {
	shared := []string{"ЕКН", "Наименование материалов", "Кол-во", "Цена", "Мембрана", "66950"}
	sheet := xlsxSheet{
		shared: shared,
		raw: `<worksheet><sheetData>
<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="s"><v>2</v></c><c r="D1" t="s"><v>3</v></c></row>
<row r="2"><c r="A2" t="s"><v>5</v></c><c r="B2" t="s"><v>4</v></c><c r="C2"><v>5</v></c><c r="D2"><v>200</v></c></row>
</sheetData></worksheet>`,
	}
	got, err := ParseXlsxAttachment(buildTestXlsxRaw(t, sheet))
	if err != nil {
		t.Fatalf("разбор упал: %v", err)
	}
	if len(got.Rows) != 1 {
		t.Fatalf("строк %d, ожидали 1: %#v", len(got.Rows), got.Rows)
	}
	if got.Rows[0].Name != "Мембрана" {
		t.Fatalf("наименование = %q, ожидали «Мембрана» (номер ЕКН не имя)", got.Rows[0].Name)
	}
	// И номер из колонки ЕКН не теряется: по нему строка сводится с материалом
	// справочника (2026-11-02 — живая жалоба «ЕКН не привязались к продуктам»).
	if got.Rows[0].EKN != "66950" {
		t.Fatalf("номер ЕКН строки = %q, ожидали 66950", got.Rows[0].EKN)
	}
}

// Живой файл менеджера: колонка ЕКН стоит первой в строке состава, и номер
// обязан доехать до каждой позиции — а не только до первой.
func TestParseLegacyOfferKeepsEkn(t *testing.T) {
	got, err := ParseXlsxAttachment(loadLegacyOffer(t))
	if err != nil {
		t.Fatalf("разбор упал: %v", err)
	}
	withEkn := 0
	for _, row := range got.Rows {
		if strings.TrimSpace(row.EKN) != "" {
			withEkn++
		}
	}
	if withEkn == 0 {
		t.Fatalf("ни у одной из %d строк нет ЕКН — колонка потеряна", len(got.Rows))
	}
	for _, row := range got.Rows {
		if strings.Contains(row.Name, "Logicbase V-SL 2 мм") && row.EKN != "66950" {
			t.Fatalf("у мембраны 2 мм ЕКН = %q, ожидали 66950", row.EKN)
		}
	}
}
