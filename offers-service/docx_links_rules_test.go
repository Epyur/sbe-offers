package main

import (
	"strings"
	"testing"

	"mailers/offers/skills"
)

// Правила группировки ссылок — настройка формы, а не код (2026-11-14, дефект
// 0.10.2, пункт 4).
//
// Пользователь: «сделать блок ссылок редактируемым… сейчас чтобы поменять
// способ группировки нужно менять код». Тест берёт форму с НЕСТАНДАРТНЫМИ
// правилами и проверяет готовый документ: группы и их порядок такие, какие
// задал администратор, а не те, что были вшиты.
//
// Это проверка поведения, а не структуры: ищется то, что увидит читатель
// (заголовки групп в теле документа и их порядок), поэтому тест переживёт
// любую внутреннюю переделку сортировки.
func TestLinkGroupsFromFormDrivePrintedDocument(t *testing.T) {
	form := formWithBlock(`[{"type":"links"}]`)
	// Правила с ног на голову: сертификаты первыми, «узлы» — своей группой,
	// страницы — последними. Ни одного id из прежнего набора в прежнем порядке.
	form.LinkGroups = skills.LinkGroupRules{
		{ID: "certs", Title: "Сертификаты сначала", LabelContains: []string{"сертификат"}},
		{ID: "nodes", Title: "Альбомы узлов", LabelContains: []string{"узл"}},
		{ID: "pages", Title: "Страницы продуктов", URLContains: []string{"/systems/", "/catalog/"}},
		{ID: "other", Title: "Прочее"},
	}
	refs := []skills.CardRef{
		{Label: "Страница системы", URL: "https://nav.tn.ru/systems/a/", Owner: skills.RefOwnerSystem},
		{Label: "Сертификат соответствия", URL: "https://nav.tn.ru/pim/cert.pdf", Owner: skills.RefOwnerMaterial},
		{Label: "Узлы PDF ТН_ОПОРА", URL: "https://nav.tn.ru/pim/uzly.pdf", Owner: skills.RefOwnerSystem},
	}
	data, err := renderOfferDocxFull(form, testOfferForBlocks(), nil, nil, refs)
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	xml := docXMLOf(t, data)

	want := []string{"Сертификаты сначала", "Альбомы узлов", "Страницы продуктов"}
	prev := -1
	for _, title := range want {
		idx := strings.Index(xml, title)
		if idx < 0 {
			t.Fatalf("нет заголовка группы из правил формы: %q", title)
		}
		if idx < prev {
			t.Fatalf("группа %q стоит не в порядке правил формы", title)
		}
		prev = idx
	}
	// Общая группа «Прочее» пуста — заголовка без ссылок в документе быть не
	// должно (правило: пустой заголовок читается как пропуск).
	if strings.Contains(xml, ">Прочее<") {
		t.Fatal("пустая группа напечатала заголовок")
	}
}

// Старая форма без правил печатает ссылки как раньше: умолчание не меняет
// существующие документы молча.
func TestLinkGroupsDefaultKeepsPreviousPrint(t *testing.T) {
	form := formWithBlock(`[{"type":"links"}]`)
	form.LinkGroups = nil
	data, err := renderOfferDocxFull(form, testOfferForBlocks(), nil, nil, linksTestRefs())
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	xml := docXMLOf(t, data)
	prev := -1
	for _, title := range []string{
		"Страницы продуктов", "Стандарты организации (СТО)", "Технические листы", "Узлы",
	} {
		idx := strings.Index(xml, title)
		if idx < 0 {
			t.Fatalf("форма без правил потеряла группу %q", title)
		}
		if idx < prev {
			t.Fatalf("форма без правил напечатала группу %q не по порядку", title)
		}
		prev = idx
	}
}
