package main

import (
	"strings"
	"testing"
)

// Макет 003/2026 — живой случай, из-за которого разбор меток переделан
// (2026-11-06).
//
// В загруженном макете инструкция модели была набрана ДВУМЯ парами фигурных
// скобок, 364 символами, и Word разрезал её на пять прогонов (запись правок,
// орфография). Прежний разбор не находил метку по двум причинам сразу:
// предел 400 считался в БАЙТАХ (русский текст вдвое длиннее по байтам), а
// `{{…}}` разбиралась только как имя поля. Текст промта уходил в документ
// литералом — с клиент-видимыми скобками в обе стороны.
const userLayoutParagraph = `<w:p><w:pPr><w:jc w:val="both"/></w:pPr>` +
	`<w:r><w:t>{</w:t></w:r>` +
	`<w:r><w:t>{</w:t></w:r>` +
	`<w:r><w:t>вставь последовательное описание всех систем, включенных в коммерческое предложение, приведи их состав, описание материалов, входящих в состав решения и преимущества применения данных материалов в составе стандартизированного технического решения, описание сопровождай изображениями составов и материалов, материалы не имеющие описания укажи только в перечне слоев</w:t></w:r>` +
	`<w:r><w:t xml:space="preserve">, структурные разделы помечай как заголовки второго уровня в разметке </w:t></w:r>` +
	`<w:proofErr w:type="spellStart"/>` +
	`<w:r><w:t>ворда</w:t></w:r>` +
	`<w:proofErr w:type="spellEnd"/>` +
	`<w:r><w:t>}</w:t></w:r>` +
	`<w:r><w:t>}</w:t></w:r></w:p>`

const userLayoutInstruction = "вставь последовательное описание всех систем, включенных в коммерческое " +
	"предложение, приведи их состав, описание материалов, входящих в состав решения и преимущества " +
	"применения данных материалов в составе стандартизированного технического решения, описание " +
	"сопровождай изображениями составов и материалов, материалы не имеющие описания укажи только " +
	"в перечне слоев, структурные разделы помечай как заголовки второго уровня в разметке ворда"

// Метка из живого макета — промт, и её текст доходит до модели целиком.
func TestUserLayoutPromptIsRecognized(t *testing.T) {
	doc := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		userLayoutParagraph + `</w:body></w:document>`
	files := map[string][]byte{
		"[Content_Types].xml": []byte(`<?xml version="1.0" encoding="UTF-8"?><Types/>`),
		"word/document.xml":   []byte(doc),
	}
	data, err := zipFiles(files, []string{"[Content_Types].xml", "word/document.xml"})
	if err != nil {
		t.Fatalf("сборка макета: %v", err)
	}
	var gotInstruction string
	out, err := renderLayoutDocx(data, layoutContext{
		values: map[string]string{"object_name": "Дом"},
		prompt: func(instruction string) string {
			gotInstruction = instruction
			return "Текст модели о системах."
		},
	})
	if err != nil {
		t.Fatalf("печать по макету: %v", err)
	}
	if gotInstruction != userLayoutInstruction {
		t.Fatalf("модель получила не тот промт:\n got %q\nwant %q", gotInstruction, userLayoutInstruction)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if !strings.Contains(body, "Текст модели о системах.") {
		t.Fatalf("текст модели не попал в документ:\n%s", body)
	}
	for _, bad := range []string{"вставь последовательное", "{{", "}}"} {
		if strings.Contains(body, bad) {
			t.Fatalf("в документе остался литерал промта %q:\n%s", bad, body)
		}
	}
}

// Описание макета для администратора находит ту же метку и показывает его
// словами — «что система поняла из файла» не должно отличаться от того, что
// она подставит.
func TestDescribeLayoutReportsUserPrompt(t *testing.T) {
	doc := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		userLayoutParagraph + `</w:body></w:document>`
	files := map[string][]byte{
		"[Content_Types].xml": []byte(`<?xml version="1.0" encoding="UTF-8"?><Types/>`),
		"word/document.xml":   []byte(doc),
	}
	data, err := zipFiles(files, []string{"[Content_Types].xml", "word/document.xml"})
	if err != nil {
		t.Fatalf("сборка макета: %v", err)
	}
	fields, prompts := describeLayout(data)
	if len(fields) != 0 {
		t.Fatalf("промт показан как поле: %v", fields)
	}
	if len(prompts) != 1 || prompts[0] != userLayoutInstruction {
		t.Fatalf("промт не найден в макете: %v", prompts)
	}
}

// Предел длины промта в рунах совпадает с тем, что зашито в выражение поля.
func TestLayoutFieldLimitMatchesRegex(t *testing.T) {
	long := strings.Repeat("я", layoutFieldMaxRunes)
	tokens := scanLayoutTokens("["+long+"]", map[string]string{})
	if len(tokens) != 1 || len([]rune(tokens[0].Inner)) != layoutFieldMaxRunes {
		t.Fatalf("поле из %d рун не разобрано: %+v", layoutFieldMaxRunes, tokens)
	}
	tooLong := strings.Repeat("я", layoutFieldMaxRunes+1)
	if got := scanLayoutTokens("["+tooLong+"]", map[string]string{}); len(got) != 0 {
		t.Fatalf("поле сверх предела разобрано целиком: %+v", got)
	}
}
