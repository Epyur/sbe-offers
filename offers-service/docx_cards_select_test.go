package main

import "testing"

// Выбор карточек и сброс кэша текстов (2026-11-01).
//
// Обе функции решают, ЧТО попадёт в документ, и обе были без тестов: карточки
// чужой системы дали бы в КП описание того, чего в нём нет, а старый текст —
// цифры прошлой версии.

// В документ идёт описание только той системы, чей материал есть в составе.
func TestCardsForOfferKeepsOnlyComposedSystems(t *testing.T) {
	offer := testOfferForBlocks()
	offer.Items = []OfferItem{{Name: "Мембрана ПВХ", Unit: "м²", Qty: 1, Price: 10, Sum: 10}}
	cards := []SystemCard{
		{SKU: "A", Title: "Кровля", CompositionLinks: []CompositionItem{{Name: "Мембрана ПВХ", Found: true}}},
		{SKU: "B", Title: "Стена", CompositionLinks: []CompositionItem{{Name: "Утеплитель", Found: true}}},
	}
	got := cardsForOffer(cards, offer)
	if len(got) != 1 || got[0].SKU != "A" {
		t.Fatalf("ожидали только систему кровли: %#v", got)
	}
}

// Строка состава без ссылки тоже считается: в старых карточках состав — текст.
func TestCardsForOfferMatchesByCompositionText(t *testing.T) {
	offer := testOfferForBlocks()
	offer.Items = []OfferItem{{Name: "Песок", Unit: "т", Qty: 1, Price: 10, Sum: 10}}
	cards := []SystemCard{{SKU: "A", Title: "Тротуар", Composition: []string{"Песок"}}}
	if got := cardsForOffer(cards, offer); len(got) != 1 {
		t.Fatalf("состав текстом не сопоставлен: %#v", got)
	}
}

// Сравнение терпимо к регистру, ё и пробелам: карточку пишут руками.
func TestCardsForOfferNormalisesNames(t *testing.T) {
	offer := testOfferForBlocks()
	offer.Items = []OfferItem{{Name: "  ПЕНОПОЛИСТИРОЛ  XPS ", Unit: "м²", Qty: 1, Price: 10, Sum: 10}}
	cards := []SystemCard{{SKU: "A", Title: "Пол", CompositionLinks: []CompositionItem{{Name: "пенополистирол XPS", Found: true}}}}
	if got := cardsForOffer(cards, offer); len(got) != 1 {
		t.Fatalf("название не нормализовано: %#v", got)
	}
}

// Пустое предложение — ни одной карточки: описания без состава не нужны.
func TestCardsForOfferEmptyOffer(t *testing.T) {
	if got := cardsForOffer([]SystemCard{{SKU: "A", Title: "X"}}, Offer{}); len(got) != 0 {
		t.Fatalf("для пустого состава карточки не нужны: %#v", got)
	}
}

// Сброс кэша текстов стирает ТОЛЬКО хеш данных; тексты остаются.
func TestDropStaleBlockTextsRemovesHashOnly(t *testing.T) {
	values := map[string]string{
		genKeySection:          "hash",
		genKeyTextByID("d", 0): "Текст блока",
		"client_name":          "ООО Заказчик",
	}
	out := dropStaleBlockTexts(values)
	if _, ok := out[genKeySection]; ok {
		t.Fatal("хеш остался — текст считался бы свежим")
	}
	if out[genKeyTextByID("d", 0)] != "Текст блока" {
		t.Fatalf("текст блока пропал: %#v", out)
	}
	if out["client_name"] != "ООО Заказчик" {
		t.Fatalf("прочие поля пропали: %#v", out)
	}
}

// nil-карта остаётся nil: вызов на КП без формы не должен падать.
func TestDropStaleBlockTextsNil(t *testing.T) {
	if got := dropStaleBlockTexts(nil); got != nil {
		t.Fatalf("ожидали nil, получили %#v", got)
	}
}
