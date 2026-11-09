package main

import (
	"regexp"
	"testing"

	"mailers/offers/skills"
)

// Живой сломанный КП 003/2026 (2026-11-09): документ не открывался в Word.
//
// Причина: метка `{{ссылки: система}}` стоит ВНУТРИ блока формы (блок систем,
// repeat:systems). Контекст блока строил СВОЙ `layoutRuntime`, и отношения,
// заведённые меткой, уходили в него и терялись. В теле оставалась гиперссылка
// на отношение, которого нет в `document.xml.rels`, — Word отказывается
// открывать такой файл.
//
// Проверка смотрит то, что видит Word: каждая `r:id` в теле обязана быть в
// отношениях. Работает для обоих путей — по макету и по блокам формы.

// linkRefs — карточка с двумя ссылками: страница (без кавычки в адресе) и
// документ (с пробелом и кавычкой внутри query, как у навигатора).
func linkRefsForProbe() []SystemCard {
	return []SystemCard{{
		NoteID: 1,
		Title:  "Система",
		Refs: []skills.CardRef{
			{Label: "Страница системы", URL: "https://nav.tn.ru/systems/x/", Kind: skills.RefKindPage},
			{Label: "Техлист", URL: `https://nav.tn.ru/pim/a.pdf?response-content-disposition=filename="Техлист ДНС.pdf"`, Kind: skills.RefKindDoc},
		},
	}}
}

// assertAllLinksDefined — каждая гиперссылка тела имеет отношение.
func assertAllLinksDefined(t *testing.T, data []byte) {
	t.Helper()
	body := string(docxEntry(t, data, "word/document.xml"))
	rels := relsPartOf(t, data)
	defined := make(map[string]bool)
	for _, m := range regexp.MustCompile(`Id="([^"]+)"`).FindAllStringSubmatch(rels, -1) {
		defined[m[1]] = true
	}
	used := regexp.MustCompile(`<w:hyperlink r:id="([^"]+)"`).FindAllStringSubmatch(body, -1)
	if len(used) == 0 {
		t.Fatal("ожидали гиперссылки в теле, их нет")
	}
	for _, m := range used {
		if !defined[m[1]] {
			t.Fatalf("тело ссылается на отношение %q, которого нет в rels — Word не откроет файл", m[1])
		}
	}
}

// Метка `{{ссылки}}` внутри блока формы в печати ПО МАКЕТУ: отношения не теряются.
func TestLinksMarkerInsideBlockLayoutDefinesRelations(t *testing.T) {
	// Блок с меткой, включён в форму под id, макет ссылается на него.
	form := OfferForm{
		Title: "КП",
		Blocks: []Block{{
			ID:      "blk",
			Title:   "Ссылки",
			Content: []byte(`[{"type":"paragraph","text":"{{ссылки: система}}"}]`),
		}},
	}
	layout := layoutDocxFromXML(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`+
		`<w:p><w:r><w:t>{{block:blk}}</w:t></w:r></w:p></w:body></w:document>`)
	lc := layoutContext{
		values: map[string]string{},
		offer:  Offer{Number: "1/2026", Systems: []OfferSystem{{NoteID: 1}}},
		cards:  func() []SystemCard { return linkRefsForProbe() },
		form:   func() OfferForm { return form },
	}
	data, err := renderLayoutDocx(layout, lc)
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	assertAllLinksDefined(t, data)
}

// Метка `{{ссылки}}` внутри блока в печати ПО БЛОКАМ ФОРМЫ: то же самое.
func TestLinksMarkerInsideBlockFormDefinesRelations(t *testing.T) {
	form := OfferForm{
		Title: "КП",
		Blocks: []Block{{
			ID:           "blk",
			Title:        "Ссылки",
			Content:      []byte(`[{"type":"paragraph","text":"{{ссылки: система}}"}]`),
			ShowInClient: true,
		}},
	}
	offer := Offer{Number: "1/2026", OwnerEmail: "m@tn.ru", Systems: []OfferSystem{{NoteID: 1}}}
	data, err := renderOfferDocxFull(form, offer, linkRefsForProbe(), nil, nil)
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	assertAllLinksDefined(t, data)
}

// Подпись, оставленная старым кешем как весь markdown `[название](адрес)`,
// печатается названием, а не сырой строкой.
func TestRefDisplayTextCleansMarkdownLabel(t *testing.T) {
	ref := skills.CardRef{
		Label: "[Техлист ФНД](https://nav.tn.ru/pim/x.pdf?response-content-disposition=filename=",
		URL:   "https://nav.tn.ru/pim/x.pdf?response-content-disposition=filename=",
	}
	if got := refDisplayText(ref); got != "Техлист ФНД" {
		t.Fatalf("подпись из старого кеша не очищена: %q", got)
	}
}
