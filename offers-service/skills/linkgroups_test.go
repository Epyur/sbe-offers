package skills

import "testing"

// Правила группировки ссылок как данные формы (2026-11-14, дефект 0.10.2).
//
// Сторожат ровно то, из-за чего блок ссылок нельзя было настроить без
// разработчика, и то, что легко сломать при переносе: умолчание обязано
// совпадать с прежним поведением (ни один печатный документ не меняется
// молча), а явные правила администратора обязаны обыгрывать старую пометку на
// ссылке — иначе настройка никогда не сработает на живом кеше карточек.

// Умолчание = прежнее поведение: те же группы в том же порядке.
func TestDefaultLinkGroupRulesMatchPreviousBehaviour(t *testing.T) {
	refs := []CardRef{
		zeroRef("Страница системы", "https://nav.tn.ru/systems/a/"),
		zeroRef("Страница материала", "https://nav.tn.ru/catalog/a/"),
		zeroRef("СТО 72746455-4.8.1-2023", "https://nav.tn.ru/pim/sto.pdf"),
		zeroRef("Техлист ТН_10000016", "https://nav.tn.ru/pim/tl.pdf"),
		zeroRef("Узлы PDF ТН_ОПОРА", "https://nav.tn.ru/pim/uzly.pdf"),
		zeroRef("Сертификат соответствия", "https://nav.tn.ru/pim/cert.pdf"),
	}
	// Сам сборщик ссылок предложения помечает их умолчанием (как живой кеш).
	refs = TagCardRefsOwner(refs, RefOwnerSystem)
	got := GroupCardRefsWith(SortCardRefsWith(refs, nil), nil)
	want := []string{RefGroupPage, RefGroupSTO, RefGroupTechList, RefGroupUzly, RefGroupCert}
	if len(got) != len(want) {
		t.Fatalf("групп %d, ожидали %d: %v", len(got), len(want), groupsOf(got))
	}
	for i := range want {
		if got[i].Group != want[i] {
			t.Fatalf("порядок групп: получили %v, ожидали %v", groupsOf(got), want)
		}
	}
	if title := got[0].Title; title != "Страницы продуктов" {
		t.Fatalf("заголовок группы страниц: %q", title)
	}
}

// Правила администратора главнее старой пометки на ссылке: иначе настройка не
// сработает на живом кеше, где ссылки уже помечены вшитым правилом.
func TestLinkGroupOfExplicitRulesBeatCachedMark(t *testing.T) {
	rules := LinkGroupRules{
		{ID: "docs", Title: "Документы", LabelContains: []string{"сертификат"}},
		{ID: "other", Title: "Прочее"},
	}
	ref := CardRef{Label: "Сертификат соответствия", URL: "https://nav.tn.ru/pim/c.pdf", Group: RefGroupCert}
	if got := LinkGroupOf(ref, rules); got != "docs" {
		t.Fatalf("явное правило не обыграло пометку: %q", got)
	}
}

// Порядок групп — порядок правил: администратор двигает группу и меняет печать.
//
// Ссылки берутся БЕЗ пометки: так они приходят из формы (`renderOfferDocxFull`
// с явными ссылками). Пометка — старое вшитое правило и правила формы не
// перебивает (см. TestLinkGroupOfExplicitRulesBeatCachedMark).
func TestLinkGroupRulesOrderDrivesPrintOrder(t *testing.T) {
	rules := LinkGroupRules{
		{ID: "cert", Title: "Сертификаты", LabelContains: []string{"сертификат"}},
		{ID: "page", Title: "Страницы продуктов", URLContains: []string{"/systems/"}},
	}
	refs := []CardRef{
		zeroRef("Страница системы", "https://nav.tn.ru/systems/a/"),
		zeroRef("Сертификат соответствия", "https://nav.tn.ru/pim/cert.pdf"),
	}
	sorted := SortCardRefsWith(refs, rules)
	got := GroupCardRefsWith(sorted, rules)
	if len(got) != 2 || got[0].Group != "cert" || got[1].Group != "page" {
		t.Fatalf("порядок групп не по правилам: %v", groupsOf(got))
	}
	if sorted[0].URL != "https://nav.tn.ru/pim/cert.pdf" {
		t.Fatalf("сортировка игнорирует правила: %v", urlsOf(sorted))
	}
}

// Заголовки групп берутся из правил, а неизвестная группа подписывается своим
// id, а не пустотой: пустой заголовок в документе выглядит сбоем вёрстки.
func TestLinkGroupTitleFromRules(t *testing.T) {
	rules := LinkGroupRules{{ID: "docs", Title: "Мои документы"}, {ID: "other", Title: "Прочее"}}
	if got := LinkGroupTitle("docs", rules); got != "Мои документы" {
		t.Fatalf("заголовок из правил: %q", got)
	}
	if got := LinkGroupTitle("ghost", rules); got != "ghost" {
		t.Fatalf("неизвестная группа: %q", got)
	}
}

// Проверка правил: пусто — годно (умолчание), общая группа обязана стоять
// последней, id и заголовки не пустые и не повторяются.
func TestValidateLinkGroupRules(t *testing.T) {
	cases := map[string]struct {
		rules LinkGroupRules
		ok    bool
	}{
		"пусто — умолчание": {nil, true},
		"умолчание годится": {DefaultLinkGroupRules(), true},
		"пустой id":         {LinkGroupRules{{ID: " ", Title: "X"}, {ID: "o", Title: "Y"}}, false},
		"пустой заголовок":  {LinkGroupRules{{ID: "a", Title: ""}, {ID: "o", Title: "Y"}}, false},
		"повтор id":         {LinkGroupRules{{ID: "a", Title: "A"}, {ID: "a", Title: "B"}, {ID: "o", Title: "Y"}}, false},
		"последняя не общая": {LinkGroupRules{
			{ID: "o", Title: "O"},
			{ID: "p", Title: "P", LabelContains: []string{"сто"}},
		}, false},
	}
	for name, c := range cases {
		got := ValidateLinkGroupRules(c.rules)
		if c.ok && got != "" {
			t.Errorf("%s: правило отвергнуто (%s)", name, got)
		}
		if !c.ok && got == "" {
			t.Errorf("%s: правило принято, хотя должно быть отвергнуто", name)
		}
	}
}

// zeroRef — ссылка без группы: так её читает разбор карточки (пометка ставится
// позже, сборщиком ссылок предложения).
func zeroRef(label, url string) CardRef {
	return CardRef{Label: label, URL: url}
}
