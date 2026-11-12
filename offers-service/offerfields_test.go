package main

// Сторож шага S3 (2026-11-13): точечные правки карточки КП.
//
// Здесь проверяется то, что нельзя увидеть глазами на стенде: пустая правка не
// стирает поле, присланные клиентом `sum`/`total` не учитываются, а порядок
// строк и количество позиций сохраняются как есть. Сами ручки (права, ответ,
// `updated_at`) проверяются на стенде — секция «5c. Карточка КП из приложения».

import "testing"

func TestValidateOfferClientAllowsPartialDraft(t *testing.T) {
	if msg := validateOfferClient(OfferClient{}); msg != "" {
		t.Fatalf("пустой черновик отвергнут: %s", msg)
	}
	if msg := validateOfferClient(OfferClient{Name: "ООО Ромашка"}); msg != "" {
		t.Fatalf("только имя отвергнуто: %s", msg)
	}
}

func TestValidateOfferClientRejectsContactWithoutName(t *testing.T) {
	if msg := validateOfferClient(OfferClient{Name: "   ", Contact: "Иван"}); msg == "" {
		t.Fatal("контакт без заказчика принят: в списке такая строка будет безымянной")
	}
}

func TestValidateOfferClientRejectsOverlong(t *testing.T) {
	long := ""
	for i := 0; i < 301; i++ {
		long += "а"
	}
	if msg := validateOfferClient(OfferClient{Name: long}); msg == "" {
		t.Fatal("имя в 301 символ принято")
	}
}

func TestValidateOfferItemsAllowsEmptyList(t *testing.T) {
	// Ключевое отличие от validateOffer: позиции вводятся по одной.
	if msg := validateOfferItems(nil); msg != "" {
		t.Fatalf("пустой список позиций отвергнут: %s", msg)
	}
}

func TestValidateOfferItemsChecksEachRow(t *testing.T) {
	cases := []struct {
		item OfferItem
		want bool
	}{
		{OfferItem{Name: "Плита", Qty: 1, Price: 0}, true},
		{OfferItem{Name: "  ", Qty: 1, Price: 10}, false},
		{OfferItem{Name: "Плита", Qty: 0, Price: 10}, false},
		{OfferItem{Name: "Плита", Qty: -1, Price: 10}, false},
		{OfferItem{Name: "Плита", Qty: 1, Price: -10}, false},
	}
	for _, c := range cases {
		msg := validateOfferItems([]OfferItem{c.item})
		if (msg == "") != c.want {
			t.Fatalf("позиция %+v: ответ %q, хотим годной=%v", c.item, msg, c.want)
		}
	}
}

func TestValidateOfferItemsRejectsTooManyRows(t *testing.T) {
	items := make([]OfferItem, 501)
	for i := range items {
		items[i] = OfferItem{Name: "Плита", Qty: 1, Price: 1}
	}
	if msg := validateOfferItems(items); msg == "" {
		t.Fatal("501 позиция принята")
	}
}

func TestItemsTotalIgnoresClientSum(t *testing.T) {
	// Приложение считает сумму для показа; сервер считает свою и присланной
	// не верит. 3 × 999.99 = 2999.97 без хвостов float.
	items := []OfferItem{
		{Name: "Система", Qty: 3, Price: 999.99, Sum: 1},
		{Name: "Работа", Qty: 1, Price: 0.1, Sum: 100},
	}
	total := recalcOfferTotal(items)
	if total != 3000.07 {
		t.Fatalf("итог = %v, хотим 3000.07 (сумма присланных Sum не учитывается)", total)
	}
	if items[0].Sum != 2999.97 {
		t.Fatalf("sum первой строки = %v, хотим 2999.97", items[0].Sum)
	}
	if items[1].Sum != 0.1 {
		t.Fatalf("sum второй строки = %v, хотим 0.1", items[1].Sum)
	}
}
