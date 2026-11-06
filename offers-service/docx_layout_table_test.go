package main

import (
	"strings"
	"testing"
)

// Модель по промту таблицы возвращает markdown-таблицу, а макет печатал
// её как обычный текст (2026-11-06).
//
// Живой прогон КП 003/2026: промт просил «вставь таблицу … столбцы ЕКН,
// Наименование материала, …»; модель отвечала прозой с рассуждением, потому
// что промпт запрещал таблицы. После правки промпта ответ приходит таблицей
// Markdown — и он обязан стать настоящей таблицей Word, а не строкой с
// вертикальными чертами.
func TestLayoutPrintsMarkdownTable(t *testing.T) {
	const answer = "| ЕКН | Наименование материала | Объем поставки | Стоимость, руб |\n" +
		"|---|---|---|---|\n" +
		"| | Мембрана V-SL 2 мм | 5 520 | 4 247 971,20 |\n" +
		"| | Геотекстиль 500 г/м² | 10 560 | 1 140 480,00 |\n" +
		"| | **Итого** | | **5 388 451,20** |"
	doc := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p><w:pPr><w:jc w:val="both"/></w:pPr><w:r><w:t>{{Вставь таблицу материалов}}</w:t></w:r></w:p>` +
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
		values: map[string]string{},
		prompt: func(string) string { return answer },
	})
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if !strings.Contains(body, "<w:tbl>") {
		t.Fatalf("таблица не напечатана как таблица Word:\n%s", body)
	}
	if strings.Contains(body, "|---") {
		t.Fatalf("в документе остался разделитель markdown-таблицы:\n%s", body)
	}
	// Три строки данных + шапка = четыре <w:tr>.
	if n := strings.Count(body, "<w:tr>"); n != 4 {
		t.Fatalf("строк таблицы %d, ожидалось 4:\n%s", n, body)
	}
	for _, want := range []string{"ЕКН", "Мембрана V-SL 2 мм", "4 247 971,20", "Геотекстиль"} {
		if !strings.Contains(body, want) {
			t.Errorf("в таблице нет %q", want)
		}
	}
	// Ячейка не должна остаться с markdown-звёздочками.
	if strings.Contains(body, "**") {
		t.Errorf("в ячейках осталась markdown-разметка:\n%s", body)
	}
}

// Обычный текст рядом с таблицей сохраняется: после таблицы печатаются
// абзацы, до неё — тоже.
func TestLayoutTableWithSurroundingText(t *testing.T) {
	const answer = "Кратко о предложении.\n\n" +
		"| Материал | Стоимость |\n|---|---|\n| Мембрана | 1 000,00 |\n\n" +
		"Стоимость указана с учётом доставки."
	doc := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p><w:r><w:t>{Опиши предложение}</w:t></w:r></w:p>` +
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
		values: map[string]string{},
		prompt: func(string) string { return answer },
	})
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if !strings.Contains(body, "<w:tbl>") {
		t.Fatalf("таблица не напечатана:\n%s", body)
	}
	for _, want := range []string{"Кратко о предложении.", "Стоимость указана с учётом доставки."} {
		if !strings.Contains(body, want) {
			t.Errorf("потерян текст вокруг таблицы: %q", want)
		}
	}
}
