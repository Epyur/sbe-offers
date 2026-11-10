package skills

import "testing"

// Порядок ссылок в документе (2026-11-01, обновлён 2026-11-10 под группы):
// сначала страницы продуктов, потом документы. Внутри страниц — сначала
// система, потом материал; внутри документов — тоже система раньше материала.
// Читать раздел ссылок сверху вниз — единственный способ, каким этот раздел
// вообще читают.
func TestSortCardRefsPagesFirst(t *testing.T) {
	refs := []CardRef{
		{Label: "Узлы", URL: "https://a/uzly.pdf", Owner: RefOwnerSystem},
		{Label: "Страница материала", URL: "https://nav.tn.ru/catalog/mat/", Owner: RefOwnerMaterial},
		{Label: "Техлист", URL: "https://a/techlist.pdf", Owner: RefOwnerSystem},
		{Label: "Страница системы", URL: "https://nav.tn.ru/systems/sys/", Owner: RefOwnerSystem},
	}
	got := SortCardRefs(refs)
	want := []string{
		"https://nav.tn.ru/systems/sys/",
		"https://nav.tn.ru/catalog/mat/",
		"https://a/techlist.pdf",
		"https://a/uzly.pdf",
	}
	for i, url := range want {
		if got[i].URL != url {
			t.Fatalf("порядок ссылок: %v, ожидали %v", urlsOf(got), want)
		}
	}
}

// Исходный срез не меняется: вызывающий держит его как кэш карточки.
func TestSortCardRefsDoesNotMutateInput(t *testing.T) {
	refs := []CardRef{
		{Label: "Б", URL: "https://b", Kind: RefKindPage},
		{Label: "А", URL: "https://a", Kind: RefKindPage},
	}
	_ = SortCardRefs(refs)
	if refs[0].URL != "https://b" {
		t.Fatal("исходный срез переставлен на месте")
	}
}

// Пустой список — пустой ответ, без паники.
func TestSortCardRefsEmpty(t *testing.T) {
	if got := SortCardRefs(nil); len(got) != 0 {
		t.Fatalf("ожидали пустой список, получили %v", urlsOf(got))
	}
}

func urlsOf(refs []CardRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.URL)
	}
	return out
}
