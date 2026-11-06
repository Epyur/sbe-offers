package main

import (
	"strings"
	"testing"
)

// Промт макета, разбитый на НЕСКОЛЬКО абзацев Word (2026-11-06).
//
// Живой макет КП 003/2026 набран именно так: человек писал инструкцию
// списком и жал Enter — открывающие `{{` стоят в первом абзаце, а закрывающие
// `}}` в девятом. Разбор идёт ПО АБЗАЦАМ (`reWordParagraph`), поэтому метка,
// пересекающая границу, не находилась ни в одном из них и уходила клиенту
// литералом вместе со всеми девятью абзацами инструкции.
//
// Требование пользователя 2026-11-06: «промт должен быть написан одним
// блоком, без переносов строк». Дело не в том, что человек обязан набрать его
// одной строкой, — система обязана понимать многоабзацный промт так же, как
// одноабзацный.
func TestLayoutPromptAcrossParagraphs(t *testing.T) {
	const doc = `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p><w:pPr><w:jc w:val="both"/></w:pPr><w:r><w:t>{{Вставь описание систем: </w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:jc w:val="both"/></w:pPr><w:r><w:t>- приведи их состав, </w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:jc w:val="both"/></w:pPr><w:r><w:t>- опиши материалы решения; </w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:jc w:val="both"/></w:pPr><w:r><w:t>Объём не менее 4000 знаков}}</w:t></w:r></w:p>` +
		`</w:body></w:document>`
	files := map[string][]byte{
		"[Content_Types].xml": []byte(`<?xml version="1.0" encoding="UTF-8"?><Types/>`),
		"word/document.xml":   []byte(doc),
	}
	data, err := zipFiles(files, []string{"[Content_Types].xml", "word/document.xml"})
	if err != nil {
		t.Fatalf("сборка макета: %v", err)
	}

	var got []string
	out, err := renderLayoutDocx(data, layoutContext{
		values: map[string]string{},
		prompt: func(instruction string) string {
			got = append(got, instruction)
			return "ОПИСАНИЕ ОТ МОДЕЛИ."
		},
	})
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("промт не сработал: вызовов модели %d, ожидался 1", len(got))
	}
	for _, want := range []string{"Вставь описание систем", "приведи их состав", "опиши материалы решения", "4000 знаков"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("модель не получила часть промта %q:\n%s", want, got[0])
		}
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if !strings.Contains(body, "ОПИСАНИЕ ОТ МОДЕЛИ.") {
		t.Fatalf("текст модели не попал в документ:\n%s", body)
	}
	for _, bad := range []string{"{{", "}}"} {
		if strings.Contains(body, bad) {
			t.Fatalf("в документе остались скобки метки %q:\n%s", bad, body)
		}
	}
}

// Метка, целиком лежащая в одном абзаце, печатается как раньше: правка не
// должна сломать обычный случай.
func TestLayoutPromptInOneParagraphStillWorks(t *testing.T) {
	const doc = `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p><w:r><w:t>{Опиши систему кратко}</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>Обычный текст после</w:t></w:r></w:p>` +
		`</w:body></w:document>`
	files := map[string][]byte{
		"[Content_Types].xml": []byte(`<?xml version="1.0" encoding="UTF-8"?><Types/>`),
		"word/document.xml":   []byte(doc),
	}
	data, err := zipFiles(files, []string{"[Content_Types].xml", "word/document.xml"})
	if err != nil {
		t.Fatalf("сборка макета: %v", err)
	}
	var got string
	out, err := renderLayoutDocx(data, layoutContext{
		values: map[string]string{},
		prompt: func(instruction string) string {
			got = instruction
			return "Коротко о системе."
		},
	})
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	if got != "Опиши систему кратко" {
		t.Fatalf("модель получила: %q", got)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if !strings.Contains(body, "Коротко о системе.") || !strings.Contains(body, "Обычный текст после") {
		t.Fatalf("документ потерял текст:\n%s", body)
	}
}
