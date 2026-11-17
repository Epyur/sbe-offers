package main

// Ссылки в документе: ОДИН список в порядке упоминания (2026-11-17).
//
// Требование пользователя: «убрать деление ссылок на типы, все ссылки (в том
// числе на иные документы) приводим в порядке их упоминания в заметке. Ссылки
// на ресурсы конкурента не приводим». До правки печать резала список на группы
// («Страницы продуктов», «Стандарты организации (СТО)», «Технические листы»,
// «Узлы», «Сертификаты и декларации») со своей нумерацией в каждой.

import (
	"context"
	"strings"
	"testing"

	"mailers/offers/skills"
)

// testLinkNode — узел `links` формы.
func testLinkNode() blockNode {
	return blockNode{Type: "links", Level: 2}
}

// Список печатается в порядке входа, без заголовков групп.
func TestLinksPrintedInMentionOrder(t *testing.T) {
	refs := []skills.CardRef{
		{Label: "Сертификат соответствия", URL: "https://nav.tn.ru/pim/cert.pdf"},
		{Label: "Страница системы", URL: "https://nav.tn.ru/systems/x/"},
		{Label: "Техлист 8_01", URL: "https://nav.tn.ru/pim/tl.pdf"},
	}
	markup, ok := renderLinkSection(testLinkNode(), refs, nil)
	if !ok {
		t.Fatal("раздел ссылок не напечатан")
	}
	// Порядок — как в заметке: сертификат, страница, техлист.
	last := -1
	for _, label := range []string{"Сертификат соответствия", "Страница системы", "Техлист 8_01"} {
		pos := strings.Index(markup, label)
		if pos < 0 {
			t.Fatalf("ссылка %q не напечатана:\n%s", label, markup)
		}
		if pos < last {
			t.Fatalf("порядок ссылок не совпал с порядком упоминания (%q встал раньше):\n%s", label, markup)
		}
		last = pos
	}
	// Ни одного заголовка группы: типов ссылок в документе больше нет.
	for _, title := range []string{
		"Страницы продуктов", "Стандарты организации", "Технические листы",
		"Узлы", "Сертификаты и декларации",
	} {
		if strings.Contains(markup, title) {
			t.Fatalf("в документе остался заголовок группы %q:\n%s", title, markup)
		}
	}
}

// Ссылки конкурента в боевой листок не попадают: печатаются только наши.
//
// Печать идёт по макету с меткой `{{ссылки}}` — это путь, на котором ссылки
// вообще доходят до документа (встроенный запасной вид печатает карточки и
// ссылок не содержит).
func TestCombatSheetSkipsCompetitorLinks(t *testing.T) {
	layout := layoutDocxFromXML(t, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:r><w:t>{{ссылки}}</w:t></w:r></w:p>
</w:body></w:document>`)
	sheet, our, competitor := combatSectionFixture()
	ourRef := "https://nav.tn.ru/systems/our/"
	competitorRef := "https://pirrogroup.ru/katalog-professionalnoe"
	our.Refs = []skills.CardRef{{Label: "Страница нашей системы", URL: ourRef}}
	competitor.Refs = []skills.CardRef{{Label: "Сайт конкурента", URL: competitorRef}}

	data, err := (*Server)(nil).renderCombatDocx(context.Background(), layout, sheet,
		[]SystemCard{our}, []SystemCard{competitor}, nil)
	if err != nil {
		t.Fatalf("листок не собран: %v", err)
	}
	body := string(docxEntry(t, data, "word/document.xml"))
	rels := relsPartOf(t, data)
	if !strings.Contains(body, "Страница нашей системы") || !strings.Contains(rels, ourRef) {
		t.Fatalf("наша ссылка не попала в документ:\n%s", body)
	}
	if strings.Contains(body, "Сайт конкурента") || strings.Contains(rels, competitorRef) {
		t.Fatalf("ссылка конкурента попала в документ:\n%s", body)
	}
}
