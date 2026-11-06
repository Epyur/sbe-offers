package main

import (
	"strings"
	"testing"
)

// Какую таблицу просит метка макета и откуда берутся её строки (2026-11-06).
//
// Метка вида `{{Вставь таблицу …}}` встречается в двух смыслах, и данные для
// них РАЗНЫЕ:
//
//   - предложение по материалам (ЕКН, единица поставки, цена, объём,
//     стоимость) — это ПОЗИЦИИ предложения;
//   - сравнение вариантов решений — это ХАРАКТЕРИСТИКИ карточек систем.
//
// Модель не пишет ни ту, ни другую: на просьбе о таблице она рассуждает о
// группировке, и таблицей это не становится. Строки собирает код из того, что
// уже есть у предложения.
func TestLayoutTableKindDetection(t *testing.T) {
	cases := []struct {
		instruction string
		want        layoutTableKind
	}{
		{"Вставь таблицу содержащую предложение по материалам, столбцы ЕКН, Наименование материала, Цена за единицу", layoutTableItems},
		{"Вставь таблицу сравнения вариантов решений", layoutTableCompare},
		{"Сравни системы таблицей: показатель и значение", layoutTableCompare},
		{"Таблица сравнения предлагаемых систем", layoutTableCompare},
		{"Вставь таблицу с итоговой стоимостью предложения", layoutTableItems},
		{"Опиши систему в двух абзацах", layoutTableNone},
	}
	for _, c := range cases {
		if got := layoutTableKindOf(c.instruction); got != c.want {
			t.Errorf("layoutTableKindOf(%q) = %v, ожидалось %v", c.instruction, got, c.want)
		}
	}
}

// «Сравнение» и «варианты» сильнее слов «материала» и «предложение»: промт
// сравнения часто упоминает и материалы тоже.
func TestLayoutTableCompareWinsOverItems(t *testing.T) {
	instruction := "Вставь таблицу сравнения вариантов решений по материалам: показатель, наша система, альтернатива"
	if got := layoutTableKindOf(instruction); got != layoutTableCompare {
		t.Fatalf("получено %v, ожидалось сравнение", got)
	}
}

// Строки таблицы сравнения берутся из характеристик карточек, а не из позиций.
func TestLayoutCompareTableRows(t *testing.T) {
	cards := []SystemCard{
		{
			Title: "ТН-ФУНДАМЕНТ Дренаж Проф",
			Specifications: [][]string{
				{"Показатель", "Значение"},
				{"Прочность на разрыв", "1,5 кН/м²"},
				{"Срок службы", "50 лет"},
			},
		},
		{
			Title: "ТН-ФУНДАМЕНТ Стандарт",
			Specifications: [][]string{
				{"Параметр", "Значение"},
				{"Срок службы", "25 лет"},
				{"Прочность на разрыв", "1,0 кН/м²"},
			},
		},
	}
	rows := layoutCompareTable(cards)
	if len(rows) < 3 {
		t.Fatalf("строк сравнения %d, ожидалось не меньше 3: %v", len(rows), rows)
	}
	header := strings.Join(rows[0], " | ")
	for _, want := range []string{"Показатель", "ТН-ФУНДАМЕНТ Дренаж Проф", "ТН-ФУНДАМЕНТ Стандарт"} {
		if !strings.Contains(header, want) {
			t.Errorf("в шапке нет %q: %s", want, header)
		}
	}
	// Порядок показателей задаёт первая карточка.
	body := strings.Join(rows[1], " | ")
	if !strings.Contains(body, "Прочность на разрыв") || !strings.Contains(body, "1,5 кН/м²") {
		t.Errorf("первая строка сравнения неверна: %s", body)
	}
}

// Одна система — не сравнение: таблица молчит и печатается текст модели.
func TestLayoutCompareTableSilentForOneSystem(t *testing.T) {
	if rows := layoutCompareTable([]SystemCard{{Title: "Одна"}}); rows != nil {
		t.Fatalf("для одной системы таблица не нужна: %v", rows)
	}
}

// Название позиции с ПЕРЕВОДОМ СТРОКИ не рвёт таблицу (2026-11-06).
//
// В живом КП 003/2026 так названы утеплители: «Утеплитель ХPS Carbon Prof
// толщиной 100 мм \n(на глубину промерзания)». Таблицу печатает markdown, а
// markdown-строка не может содержать перевод строки: таблица разрывалась на
// две, и в документе появлялись обрывки. Проверка на живом документе: 41
// позиция давала 5 таблиц вместо одной.
func TestLayoutOfferTableSingleLineCells(t *testing.T) {
	offer := Offer{
		Items: []OfferItem{
			{Name: "Утеплитель ХPS Carbon Prof 100 мм \n(на глубину промерзания)", Unit: "м.куб.", Qty: 113.3, Price: 8000, Sum: 906400},
			{Name: "Мембрана V-SL 1.5 мм\r\n(1 слой)", Unit: "м2", Qty: 1610, Price: 535.84, Sum: 862702.4},
		},
		Total: 1769102.4,
	}
	rows := layoutOfferTable(offer)
	if len(rows) != 4 {
		t.Fatalf("строк %d, ожидалось 4 (шапка + 2 позиции + итог): %v", len(rows), rows)
	}
	md := tableMarkdown(rows)
	// Ни в одной ячейке не должно остаться перевода строки.
	for _, line := range strings.Split(md, "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
			t.Fatalf("строка таблицы разорвана: %q", line)
		}
	}
	// И таблица целиком — один блок при печати.
	doc := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p><w:r><w:t>{{Вставь таблицу предложения по материалам}}</w:t></w:r></w:p>` +
		`</w:body></w:document>`
	files := map[string][]byte{
		"[Content_Types].xml": []byte(`<?xml version="1.0" encoding="UTF-8"?><Types/>`),
		"word/document.xml":   []byte(doc),
	}
	data, err := zipFiles(files, []string{"[Content_Types].xml", "word/document.xml"})
	if err != nil {
		t.Fatalf("сборка макета: %v", err)
	}
	out, err := renderLayoutDocx(data, layoutContext{
		values: layoutValues(offer),
		items:  func() [][]string { return layoutOfferTable(offer) },
		cards:  func() []SystemCard { return nil },
	})
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if n := strings.Count(body, "<w:tbl>"); n != 1 {
		t.Fatalf("таблиц %d, ожидалась 1 — перевод строки в ячейке рвёт таблицу", n)
	}
	if n := strings.Count(body, "<w:tr>"); n != 4 {
		t.Fatalf("строк %d, ожидалось 4", n)
	}
}
