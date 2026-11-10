package skills

import "testing"

// Группы ссылок в документе (2026-11-10).
//
// Требование пользователя: раздел ссылок печатается не сплошным списком, а
// группами в строгом порядке:
//
//  1. страницы продукта на nav.tn.ru — сначала страницы систем, потом продуктов;
//  2. стандарты организации (СТО);
//  3. технические листы — сначала систем, потом материалов;
//  4. узлы;
//  5. сертификаты и декларации — материала сначала, систем потом; всё, что не
//     попало в группы выше, идёт сюда же («остальное»).
//
// «Сначала системы, потом материалы» — это не алфавит, а роль владельца
// ссылки. Ссылки одного вида и одного владельца упорядочиваются по подписи.

// Внутри группы страниц системы стоят перед страницами материалов.
func TestSortCardRefsGroupedPageSystemBeforeMaterial(t *testing.T) {
	refs := []CardRef{
		newRef("Страница материала", "https://nav.tn.ru/catalog/a/", RefOwnerMaterial),
		newRef("Страница системы", "https://nav.tn.ru/systems/a/", RefOwnerSystem),
	}
	got := SortCardRefsGrouped(refs)
	want := []string{"https://nav.tn.ru/systems/a/", "https://nav.tn.ru/catalog/a/"}
	assertOrder(t, got, want)
}

// Полный приоритет групп: страницы → СТО → техлисты → узлы → остальное.
func TestSortCardRefsGroupedGroupOrder(t *testing.T) {
	refs := []CardRef{
		groupRef(RefGroupCert, "сертификат", RefOwnerMaterial),
		groupRef(RefGroupUzly, "Узлы", RefOwnerSystem),
		groupRef(RefGroupTechList, "Техлист", RefOwnerMaterial),
		groupRef(RefGroupSTO, "СТО 1", RefOwnerSystem),
		groupRef(RefGroupPage, "Страница системы", RefOwnerSystem),
	}
	got := SortCardRefsGrouped(refs)
	want := []string{
		"https://x/page-system-1",
		"https://x/sto-system-1",
		"https://x/techlist-material-1",
		"https://x/uzly-system-1",
		"https://x/cert-material-1",
	}
	assertOrder(t, got, want)
}

// Техлисты: системы перед материалами, внутри вида — по подписи.
func TestSortCardRefsGroupedTechListSystemBeforeMaterial(t *testing.T) {
	refs := []CardRef{
		groupRef(RefGroupTechList, "Техлист материала", RefOwnerMaterial),
		groupRef(RefGroupTechList, "Техлист системы", RefOwnerSystem),
	}
	got := SortCardRefsGrouped(refs)
	if got[0].Label != "Техлист системы" {
		t.Fatalf("в группе техлистов система должна идти раньше материала: %v", labelsOf(got))
	}
}

// Сертификаты: материалы перед системами (исключение из общего правила).
func TestSortCardRefsGroupedCertMaterialBeforeSystem(t *testing.T) {
	refs := []CardRef{
		groupRef(RefGroupCert, "Сертификат системы", RefOwnerSystem),
		groupRef(RefGroupCert, "Сертификат материала", RefOwnerMaterial),
	}
	got := SortCardRefsGrouped(refs)
	if got[0].Label != "Сертификат материала" {
		t.Fatalf("в группе сертификатов материал должен идти раньше системы: %v", labelsOf(got))
	}
}

// Ссылки одной группы и одного владельца упорядочиваются по подписи
// (регистр и «ё» не важны для читателя); исходный срез не меняется.
func TestSortCardRefsGroupedKeepsInput(t *testing.T) {
	refs := []CardRef{
		groupRef(RefGroupTechList, "б", RefOwnerSystem),
		groupRef(RefGroupTechList, "А", RefOwnerSystem),
	}
	_ = SortCardRefsGrouped(refs)
	if refs[0].Label != "б" {
		t.Fatal("исходный срез переставлен на месте")
	}
	got := SortCardRefsGrouped(refs)
	if got[0].Label != "А" {
		t.Fatalf("подписи должны идти по алфавиту: %v", labelsOf(got))
	}
}

// GroupTitle — заголовок группы для документа; неизвестная группа даёт
// «Прочие материалы».
func TestCardRefGroupTitle(t *testing.T) {
	cases := map[string]string{
		RefGroupPage:     "Страницы продуктов",
		RefGroupSTO:      "Стандарты организации (СТО)",
		RefGroupTechList: "Технические листы",
		RefGroupUzly:     "Узлы",
		RefGroupCert:     "Сертификаты и декларации",
	}
	for group, want := range cases {
		if got := CardRefGroupTitle(group); got != want {
			t.Errorf("заголовок группы %q: получили %q, ожидали %q", group, got, want)
		}
	}
}

// Альбом узлов узнаётся по корню «узл», а не по «узел»: в живых подписях
// пишут «Узлы PDF …», и в слове «узлы» нет буквы «е» (2026-11-10).
func TestCardRefGroupOfUzlyMatchesUzlyNotJustUzel(t *testing.T) {
	if g := cardRefGroupOf("Узлы PDF ТН_ОПОРА Маст 11_2023", "https://nav.tn.ru/pim/x.pdf"); g != RefGroupUzly {
		t.Fatalf("«Узлы …» распознаны как %q, а не %q", g, RefGroupUzly)
	}
	if g := cardRefGroupOf("Альбом узлов", "https://nav.tn.ru/pim/x.pdf"); g != RefGroupUzly {
		t.Fatalf("«Альбом узлов» распознан как %q, а не %q", g, RefGroupUzly)
	}
}

// GroupByKind раскладывает отсортированный список на группы в порядке печати;
// пустые группы не попадают в результат.
func TestGroupCardRefsByGroup(t *testing.T) {
	refs := SortCardRefsGrouped([]CardRef{
		groupRef(RefGroupPage, "страница", RefOwnerSystem),
		groupRef(RefGroupUzly, "узел", RefOwnerSystem),
		groupRef(RefGroupTechList, "техлист", RefOwnerSystem),
	})
	got := GroupCardRefsByGroup(refs)
	if len(got) != 3 {
		t.Fatalf("ожидали три группы, получили %d: %v", len(got), got)
	}
	want := []string{RefGroupPage, RefGroupTechList, RefGroupUzly}
	for i, g := range want {
		if got[i].Group != g {
			t.Fatalf("порядок групп: получили %v, ожидали %v", groupsOf(got), want)
		}
	}
}

// --- помощники тестов -------------------------------------------------

// newRef — ссылка без группы: используется сериализацией старого кеша,
// проверяет, что SortCardRefsGrouped не разваливается без Kind группы.
func newRef(label, url, owner string) CardRef {
	return CardRef{Label: label, URL: url, Owner: owner}
}

// groupRef — ссылка заданной группы и владельца.
func groupRef(group, label, owner string) CardRef {
	return CardRef{Group: group, Label: label, URL: "https://x/" + slugOf(group) + "-" + owner + "-1", Owner: owner}
}

func assertOrder(t *testing.T, got []CardRef, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("ссылок %d, ожидали %d: %v", len(got), len(want), urlsOf(got))
	}
	for i, u := range want {
		if got[i].URL != u {
			t.Fatalf("порядок ссылок: получили %v, ожидали %v", urlsOf(got), want)
		}
	}
}

func labelsOf(refs []CardRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.Label)
	}
	return out
}

func groupsOf(groups []CardRefGroup) []string {
	out := make([]string, 0, len(groups))
	for _, g := range groups {
		out = append(out, g.Group)
	}
	return out
}

func slugOf(group string) string {
	out := make([]rune, 0, len(group))
	for _, r := range group {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out = append(out, r)
		}
	}
	return string(out)
}
