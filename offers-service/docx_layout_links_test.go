package main

import (
	"strings"
	"testing"

	"mailers/offers/skills"
)

// Ссылки в печати по макету .docx (2026-11-06, живой КП 003/2026).
//
// Симптом: в готовом документе раздел «Ссылки» напечатан с адресами, но ни один
// адрес не кликабелен. При загруженном макете документ собирает renderLayoutDocx,
// и в этой ветке не было ни applyHyperlinks, ни relsXMLWithLinks: превращение
// адресов в отношения живёт только в блочном пути (renderOfferDocxFull). То есть
// при макете ссылки не могут стать кликабельными ни при каком ответе модели.

// layoutWithRefsDoc — макет, где адрес напечатан в ответе модели.
const layoutWithRefsDoc = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:pPr><w:ind w:firstLine="709"/><w:jc w:val="both"/></w:pPr><w:r><w:t>{перечисли ссылки}</w:t></w:r></w:p>
</w:body></w:document>`

// relsPartOf — содержимое word/_rels/document.xml.rels готового пакета; пустая
// строка, если части нет.
func relsPartOf(t *testing.T, data []byte) string {
	t.Helper()
	return string(partOf(t, data, "word/_rels/document.xml.rels"))
}

// partOf — часть пакета или nil, если её нет (в отличие от docxEntry не падает).
func partOf(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	for _, part := range zipReaderOf(t, data) {
		if part.name == name {
			return part.data
		}
	}
	return nil
}

// TestLayoutMakesLinksClickable — адрес из ответа модели становится ссылкой Word.
//
// Проверяется то, что увидит читатель: в теле есть w:hyperlink, в отношениях есть
// внешняя цель, и видимый текст адреса остался (ссылка, из-за которой пропал
// адрес, хуже непечатанной).
func TestLayoutMakesLinksClickable(t *testing.T) {
	layout := layoutDocxFromXML(t, layoutWithRefsDoc)
	url := "https://nav.tn.ru/systems/fundament/tn-fundament-drenazh-prof/"
	data, err := renderLayoutDocx(layout, layoutContext{
		values: map[string]string{},
		prompt: func(string) string { return "1. Страница системы — " + url },
	})
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	body := string(docxEntry(t, data, "word/document.xml"))
	if !strings.Contains(body, "<w:hyperlink") {
		t.Fatal("адрес в документе не стал ссылкой: нет w:hyperlink")
	}
	if !strings.Contains(body, url) {
		t.Fatal("видимый текст адреса пропал из документа")
	}
	rels := relsPartOf(t, data)
	if !strings.Contains(rels, `Target="`+url+`"`) {
		t.Fatalf("в отношениях нет внешней цели %q", url)
	}
	if !strings.Contains(rels, `TargetMode="External"`) {
		t.Fatal("отношение ссылки не внешнее: Word откроет файл внутри пакета")
	}
}

// TestLayoutLinksKeepExistingRels — отношение ссылки дописывается к прежним.
//
// У макета свои отношения (колонтитулы, стили, тема). Замена файла целиком
// оставила бы документ без колонтитулов — та же ошибка, что со styles.xml в
// 2026-10-29.
func TestLayoutLinksKeepExistingRels(t *testing.T) {
	const rels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId8" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/>` +
		`</Relationships>`
	layout := zipOf(t, map[string]string{
		"[Content_Types].xml":          `<?xml version="1.0" encoding="UTF-8"?><Types/>`,
		"word/document.xml":            layoutWithRefsDoc,
		"word/_rels/document.xml.rels": rels,
	})
	url := "https://nav.tn.ru/pim/x.pdf"
	data, err := renderLayoutDocx(layout, layoutContext{
		values: map[string]string{},
		prompt: func(string) string { return "Техлист — " + url },
	})
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	got := relsPartOf(t, data)
	if !strings.Contains(got, `Target="footer1.xml"`) {
		t.Fatal("прежнее отношение колонтитула потерялось")
	}
	if !strings.Contains(got, `Target="`+url+`"`) {
		t.Fatal("отношение ссылки не дописано")
	}
}

// TestLayoutWithoutUrlsKeepsPlainText — в обычном тексте ссылок не появляется:
// превращается только то, что модель вернула как ссылку, и ничего сверх.
func TestLayoutWithoutUrlsKeepsPlainText(t *testing.T) {
	layout := layoutDocxFromXML(t, layoutWithRefsDoc)
	data, err := renderLayoutDocx(layout, layoutContext{
		values: map[string]string{},
		prompt: func(string) string { return "Текст без адресов." },
	})
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	body := string(docxEntry(t, data, "word/document.xml"))
	if strings.Contains(body, "<w:hyperlink") {
		t.Fatal("ссылка появилась там, где адреса не было")
	}
	if rels := relsPartOf(t, data); strings.Contains(rels, "hyperlink") {
		t.Fatal("заведено отношение ссылки без ссылки")
	}
}

// TestLayoutLinkEachUrlOnce — адрес, повторённый в ответе дважды, заводит одно
// отношение: Word не любит двух ссылок с разными id на одну цель, а второй id
// бессмысленно увеличивает файл.
func TestLayoutLinkEachUrlOnce(t *testing.T) {
	url := "https://nav.tn.ru/pim/y.pdf"
	layout := layoutDocxFromXML(t, layoutWithRefsDoc)
	data, err := renderLayoutDocx(layout, layoutContext{
		values: map[string]string{},
		prompt: func(string) string { return "Техлист — " + url + "\nЕщё раз — " + url },
	})
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	rels := relsPartOf(t, data)
	if n := strings.Count(rels, `Target="`+url+`"`); n != 1 {
		t.Fatalf("на один адрес завели %d отношений", n)
	}
}

// TestLayoutLinksWithCardRefs — оба вида ссылок (страница и документ) печатаются
// кликабельными: вид влияет на подпись, но не на то, работает ссылка или нет.
func TestLayoutLinksWithCardRefs(t *testing.T) {
	refs := []skills.CardRef{
		{Label: "Страница системы", URL: "https://nav.tn.ru/systems/x/", Kind: skills.RefKindPage},
		{Label: "Техлист", URL: "https://nav.tn.ru/pim/y.pdf", Kind: skills.RefKindDoc},
	}
	layout := layoutDocxFromXML(t, layoutWithRefsDoc)
	data, err := renderLayoutDocx(layout, layoutContext{
		values: map[string]string{},
		prompt: func(string) string {
			return refs[0].Label + " — " + refs[0].URL + "\n" + refs[1].Label + " — " + refs[1].URL
		},
	})
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	body := string(docxEntry(t, data, "word/document.xml"))
	if n := strings.Count(body, "<w:hyperlink"); n != 2 {
		t.Fatalf("ожидали две ссылки, получили %d", n)
	}
}

// layoutDocxFromXML — минимальный .docx с заданным document.xml.
func layoutDocxFromXML(t *testing.T, docXML string) []byte {
	t.Helper()
	return zipOf(t, map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8"?><Types/>`,
		"word/document.xml":   docXML,
	})
}
