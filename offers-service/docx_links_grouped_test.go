package main

import (
	"strings"
	"testing"

	"mailers/offers/skills"
)

// Раздел ссылок печатается группами и нумерованным списком (2026-11-10).
//
// Требование пользователя: ссылки в документе идут не сплошным списком, а
// группами в строгом порядке — страницы продуктов, СТО, техлисты, узлы,
// сертификаты и прочее; нумерация внутри каждой группы начинается с единицы.
// Проверяется ровно то, что увидит читатель: заголовки групп, пункты списка и
// отдельный numId у каждой группы (Word без него продолжит счёт).

func linksTestRefs() []skills.CardRef {
	return []skills.CardRef{
		{Label: "Страница материала", URL: "https://nav.tn.ru/catalog/a/", Owner: skills.RefOwnerMaterial},
		{Label: "Страница системы", URL: "https://nav.tn.ru/systems/a/", Owner: skills.RefOwnerSystem},
		{Label: "СТО 72746455-4.8.1-2023", URL: "https://nav.tn.ru/pim/sto.pdf", Owner: skills.RefOwnerSystem},
		{Label: "Техлист материала", URL: "https://nav.tn.ru/pim/tl-mat.pdf", Owner: skills.RefOwnerMaterial},
		{Label: "Техлист системы", URL: "https://nav.tn.ru/pim/tl-sys.pdf", Owner: skills.RefOwnerSystem},
		{Label: "Узлы системы", URL: "https://nav.tn.ru/pim/uzly.pdf", Owner: skills.RefOwnerSystem},
	}
}

func TestLinkSectionGroupedAndNumbered(t *testing.T) {
	form := formWithBlock(`[{"type":"links"}]`)
	data, err := renderOfferDocxFull(form, testOfferForBlocks(), nil, nil, linksTestRefs())
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	xml := docXMLOf(t, data)

	// Заголовки групп на месте и в правильном порядке.
	groupTitles := []string{
		"Страницы продуктов",
		"Стандарты организации (СТО)",
		"Технические листы",
		"Узлы",
	}
	prev := -1
	for _, title := range groupTitles {
		idx := strings.Index(xml, title)
		if idx < 0 {
			t.Fatalf("нет заголовка группы %q", title)
		}
		if idx < prev {
			t.Fatalf("группа %q стоит не по порядку", title)
		}
		prev = idx
	}

	// Каждый пункт — нумерованный (numPr в абзаце), а не абзац с маркером.
	if !strings.Contains(xml, "<w:numPr>") {
		t.Fatal("ссылки напечатаны без нумерации")
	}
	// У группы техлистов система идёт раньше материала.
	if sys, mat := strings.Index(xml, "Техлист системы"), strings.Index(xml, "Техлист материала"); sys > mat {
		t.Fatal("в группе техлистов материал стоит раньше системы")
	}
}

// Нумерация в каждой группе начинается с единицы: у групп РАЗНЫЕ numId, иначе
// Word продолжит сквозной счёт через группы. Проверяем по числу numId, которые
// завела сборка: их не меньше числа групп (страницы, СТО, техлисты, узлы).
func TestLinkSectionGroupNumberingRestarts(t *testing.T) {
	form := formWithBlock(`[{"type":"links"}]`)
	data, err := renderOfferDocxFull(form, testOfferForBlocks(), nil, nil, linksTestRefs())
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	numbering := string(partOf(t, data, "word/numbering.xml"))
	if n := strings.Count(numbering, `<w:startOverride w:val="1"/>`); n < 4 {
		t.Fatalf("у групп ссылок меньше четырёх собственных номеров: %d", n)
	}
}

// Ссылки внутри группы кликабельны: подпись видна, адрес живёт в отношении.
// С нумерованным пунктом замена должна сохранять номер (numPr) абзаца.
func TestLinkSectionNumberedItemsAreClickable(t *testing.T) {
	form := formWithBlock(`[{"type":"links"}]`)
	data, err := renderOfferDocxFull(form, testOfferForBlocks(), nil, nil, linksTestRefs())
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	xml := docXMLOf(t, data)
	if !strings.Contains(xml, `<w:hyperlink`) {
		t.Fatal("ссылки в документе не кликабельны")
	}
	// Кликабельный пункт остаётся пунктом: у абзаца-ссылки есть нумерация.
	idx := strings.Index(xml, "<w:hyperlink")
	paraStart := strings.LastIndex(xml[:idx], "<w:p>")
	if paraStart < 0 || !strings.Contains(xml[paraStart:idx], "<w:numPr>") {
		t.Fatal("кликабельный пункт потерял нумерацию")
	}
	rels := relsPartOf(t, data)
	if !strings.Contains(rels, `Target="https://nav.tn.ru/systems/a/"`) {
		t.Fatalf("отношение страницы системы не заведено:\n%s", rels)
	}
}
