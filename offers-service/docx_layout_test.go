package main

import (
	"strings"
	"testing"
)

// Печать по загруженному макету .docx (2026-11-05).
//
// Макет — собственный документ пользователя. Проверяем главное: метки
// подставляются, а его вёрстка, стили и колонтитулы остаются.

// sampleLayout — документ Word с метками обоих видов.
const sampleLayoutDoc = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>ТКП № [Номер]</w:t></w:r></w:p>
<w:p><w:r><w:t>Объект: [Наименование объекта]</w:t></w:r></w:p>
<w:p><w:r><w:t>{Опиши систему в одном абзаце}</w:t></w:r></w:p>
<w:p><w:r><w:t>Кому: {{client_name}}</w:t></w:r></w:p>
</w:body></w:document>`

func layoutDocxForTest(t *testing.T) []byte {
	t.Helper()
	// Минимальный пакет Word: содержимое и [Content_Types].xml достаточно —
	// сборка читает части по именам и пишет их обратно.
	files := map[string][]byte{
		"[Content_Types].xml": []byte(`<?xml version="1.0" encoding="UTF-8"?><Types/>`),
		"word/document.xml":   []byte(sampleLayoutDoc),
	}
	names := []string{"[Content_Types].xml", "word/document.xml"}
	data, err := zipFiles(files, names)
	if err != nil {
		t.Fatalf("сборка макета: %v", err)
	}
	return data
}

// Метки `[…]` получают значения, `{…}` — текст модели, `{{…}}` — тоже значение.
func TestRenderLayoutFillsTokens(t *testing.T) {
	layout := layoutDocxForTest(t)
	lc := layoutContext{
		values: map[string]string{
			"offer_number": "003/2026",
			"object_name":  "Дом по ул. Примерная",
			"client_name":  "ООО Поэма",
		},
		prompt: func(string) string { return "Текст модели о системе." },
	}
	data, err := renderLayoutDocx(layout, lc)
	if err != nil {
		t.Fatalf("печать по макету: %v", err)
	}
	body := string(docxEntry(t, data, "word/document.xml"))
	for _, want := range []string{
		"ТКП № 003/2026",
		"Объект: Дом по ул. Примерная",
		"Текст модели о системе.",
		"Кому: ООО Поэма",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("в документе нет %q:\n%s", want, body)
		}
	}
	for _, bad := range []string{"[Номер]", "[Наименование объекта]", "{Опиши", "{{client_name}}"} {
		if strings.Contains(body, bad) {
			t.Errorf("в документе осталась метка %q", bad)
		}
	}
}

// Вёрстка макета остаётся: выравнивание абзаца и жирный прогон не потеряны.
func TestRenderLayoutKeepsFormatting(t *testing.T) {
	data, err := renderLayoutDocx(layoutDocxForTest(t), layoutContext{
		values: map[string]string{"offer_number": "1"},
		prompt: func(string) string { return "" },
	})
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, data, "word/document.xml"))
	if !strings.Contains(body, `<w:jc w:val="center"/>`) {
		t.Fatal("выравнивание по центру из макета потеряно")
	}
	if !strings.Contains(body, `<w:b/>`) {
		t.Fatal("жирный шрифт из макета потерян")
	}
}

// Метка, разрезанная Word на несколько w:t, всё равно подставляется: Word режет
// текст по правкам, и «[Наимено» + «вание объекта]» — обычное дело.
func TestRenderLayoutHandlesSplitRuns(t *testing.T) {
	doc := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p><w:r><w:t>Объект: [Наимено</w:t></w:r><w:r><w:t>вание объекта]</w:t></w:r></w:p>` +
		`</w:body></w:document>`
	files := map[string][]byte{
		"[Content_Types].xml": []byte(`<?xml version="1.0" encoding="UTF-8"?><Types/>`),
		"word/document.xml":   []byte(doc),
	}
	data, err := zipFiles(files, []string{"[Content_Types].xml", "word/document.xml"})
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	out, err := renderLayoutDocx(data, layoutContext{
		values: map[string]string{"object_name": "Дом"},
		prompt: func(string) string { return "" },
	})
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if !strings.Contains(body, "Объект: Дом") {
		t.Fatalf("метка через два прогона не подставлена:\n%s", body)
	}
}

// Модель недоступна — промт молчит, остальные метки подставляются.
func TestRenderLayoutWithoutModel(t *testing.T) {
	data, err := renderLayoutDocx(layoutDocxForTest(t), layoutContext{
		values: map[string]string{"object_name": "Дом"},
		prompt: nil,
	})
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, data, "word/document.xml"))
	if strings.Contains(body, "{Опиши") {
		t.Fatal("промт модели напечатан литералом")
	}
	if !strings.Contains(body, "Объект: Дом") {
		t.Fatal("жёсткая подстановка потерялась без модели")
	}
}

// Не-docx отвергается с понятным текстом: старый .doc — не zip.
func TestValidateLayoutRejectsNotDocx(t *testing.T) {
	if err := validateLayoutDocx([]byte("это не zip")); err == nil {
		t.Fatal("не-docx принят как макет")
	}
}

// Найденные метки показываются администратору (что система поняла из файла).
func TestDescribeLayoutReportsTokens(t *testing.T) {
	fields, prompts := describeLayout(layoutDocxForTest(t))
	joined := strings.Join(fields, "|")
	for _, want := range []string{"Номер", "Наименование объекта", "client_name"} {
		if !strings.Contains(joined, want) {
			t.Errorf("поле %q не найдено в макете: %v", want, fields)
		}
	}
	if len(prompts) != 1 || prompts[0] != "Опиши систему в одном абзаце" {
		t.Fatalf("промт не найден: %v", prompts)
	}
}
