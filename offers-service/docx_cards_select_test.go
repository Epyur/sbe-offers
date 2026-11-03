package main

import "testing"

// Сброс кэша текстов (2026-11-01).
//
// Выбор карточек систем переехал в systems_compose_test.go (2026-11-03):
// карточка теперь выбирается по выбранной системе, а не по совпадению названий
// позиций.

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
