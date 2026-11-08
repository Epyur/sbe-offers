package main

import (
	"strings"
	"testing"
)

// Живой случай 2026-11-07: администратор поставил `{{текст: система: description}}`
// словом в макете temple.docx — и в документе осталось пустое место.
//
// Причина измерена: составная метка содержит двоеточия, и первый проход
// `reLayoutBraceField` принимал её за ИМЯ ПОЛЯ — когда такого поля нет, метка
// проверялась правильно, но описание макета (values == nil) и блок формы шли
// другим путём. Здесь закреплено главное: составная метка ни при каких условиях
// не становится полем, а пустой сохранённый текст даёт ПУСТО, а не литерал.
func TestCompositionMarkersAreNeverFields(t *testing.T) {
	cases := []string{
		"текст: система: description",
		"текст: система: advantages",
		"ссылки: система",
		"характеристики: система",
		"система: таблица",
		"block:blk_mw6oonck8g8z",
	}
	for _, inner := range cases {
		if looksLikeFieldName(inner) {
			t.Errorf("%q выглядит именем поля — метка ушла бы в подстановку", inner)
		}
	}
}

// Метка текста системы сканируется как служебная, а не как имя поля, даже
// когда проверять значения не по чему (описание макета для администратора).
func TestSystemTextMarkerIsServiceInAnyContext(t *testing.T) {
	tokens := scanLayoutTokens(`{{текст: система: description}}`, nil)
	if len(tokens) != 1 {
		t.Fatalf("меток %d, ожидалась одна: %+v", len(tokens), tokens)
	}
	if tokens[0].Kind != layoutTokenText {
		t.Fatalf("вид %q, ожидался %q", tokens[0].Kind, layoutTokenText)
	}
	if tokens[0].Service.textBlock != systemTextDescription {
		t.Fatalf("блок текста %q", tokens[0].Service.textBlock)
	}
}

// В блоке формы метка текста системы печатает СОХРАНЁННЫЙ текст (2026-11-07).
//
// Прямая вставка в макет работала всегда; в блоке формы текст системы
// вырезался, потому что строка метки считалась «текстом, а не полотном», и
// вместо содержимого оставалась пустота. Проверяем обе ветки одной таблицей.
func TestSystemTextMarkerWorksInLayoutAndBlock(t *testing.T) {
	var prompts []string
	lc := serviceLayoutContext(&prompts)
	lc.form = func() OfferForm {
		return OfferForm{Blocks: []Block{
			{ID: "1", Title: "Система", Content: []byte(`[{"type":"paragraph","text":"{{текст: система: description}}"}]`)},
		}}
	}
	doc := layoutDocxFromXML(t, wordDoc(
		`<w:p><w:r><w:t>{{текст: система: description}}</w:t></w:r></w:p>`+
			`<w:p><w:r><w:t>{{block:1}}</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if n := strings.Count(body, "Обоснование применения в двух словах."); n != 2 {
		t.Fatalf("текст системы напечатан %d раз, ожидалось два (макет + блок):\n%s", n, body)
	}
	if strings.Contains(body, "{{") {
		t.Fatalf("метка ушла литералом:\n%s", body)
	}
	if len(prompts) != 0 {
		t.Fatalf("метка текста позвала модель: %v", prompts)
	}
}
