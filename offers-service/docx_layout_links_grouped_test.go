package main

import (
	"strings"
	"testing"

	"mailers/offers/skills"
)

// Метка `{{ссылки}}` в макете печатает тот же раздел, что узел `links`:
// группами и нумерованным списком (2026-11-10). Раньше метка печатала
// маркированный список без деления на группы — заказчик видел сплошную ленту
// из техлистов, СТО и узлов.
func TestLayoutLinksMarkerGroupedAndNumbered(t *testing.T) {
	const doc = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:pPr><w:jc w:val="both"/></w:pPr><w:r><w:t>{{ссылки}}</w:t></w:r></w:p>
</w:body></w:document>`
	layout := layoutDocxFromXML(t, doc)
	refs := []skills.CardRef{
		{Label: "Техлист системы", URL: "https://nav.tn.ru/pim/tl-sys.pdf", Owner: skills.RefOwnerSystem},
		{Label: "Страница системы", URL: "https://nav.tn.ru/systems/a/", Owner: skills.RefOwnerSystem},
		{Label: "СТО 1", URL: "https://nav.tn.ru/pim/sto.pdf", Owner: skills.RefOwnerSystem},
	}
	data, err := renderLayoutDocx(layout, layoutContext{
		values: map[string]string{},
		refs:   refs,
		cards:  func() []SystemCard { return nil },
	})
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	body := string(docxEntry(t, data, "word/document.xml"))
	for _, title := range []string{"Страницы продуктов", "Стандарты организации (СТО)", "Технические листы"} {
		if !strings.Contains(body, title) {
			t.Fatalf("метка не напечатала заголовок группы %q:\n%s", title, body)
		}
	}
	if !strings.Contains(body, "<w:numPr>") {
		t.Fatal("метка напечатала ссылки без нумерации")
	}
	if !strings.Contains(body, "<w:hyperlink") {
		t.Fatal("ссылки в макете не кликабельны")
	}
	rels := relsPartOf(t, data)
	if !strings.Contains(rels, `Target="https://nav.tn.ru/systems/a/"`) {
		t.Fatalf("отношение ссылки не заведено:\n%s", rels)
	}
}
