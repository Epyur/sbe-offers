package main

import "testing"

// Пустое предложение в боте — законное начало разговора (2026-11-01).
//
// Менеджер сначала рассказывает про объект и присылает файлы, а файл
// прикладывается к строке предложения. Поэтому строка создаётся ДО позиций,
// и валидатор состава на этом пути не должен мешать. На REST-пути правило
// прежнее: там клиент присылает готовое КП, и пустое отклоняется.

// Пустой состав проверку не проходит — это и есть причина отдельного пути.
func TestValidateOfferRejectsEmptyItems(t *testing.T) {
	if msg := validateOffer(Offer{Items: []OfferItem{}}); msg == "" {
		t.Fatal("validateOffer должна отклонять пустое предложение — на этом держится REST-правило")
	}
}

// Непустой состав проверяется как раньше: имя и количество обязательны.
func TestValidateOfferChecksProvidedItems(t *testing.T) {
	bad := []OfferItem{{Name: "", Qty: 1, Price: 10}}
	if msg := validateOffer(Offer{Items: bad}); msg == "" {
		t.Fatal("позиция без наименования должна отклоняться")
	}
	good := []OfferItem{{Name: "Мембрана", Qty: 1, Price: 10}}
	if msg := validateOffer(Offer{Items: good}); msg != "" {
		t.Fatalf("корректная позиция отклонена: %q", msg)
	}
}

// Итог пустого состава — ноль, а не ошибка: черновик начала разговора.
func TestEmptyOfferTotalIsZero(t *testing.T) {
	if got := recalcOfferTotal([]OfferItem{}); got != 0 {
		t.Fatalf("итог пустого состава: %v", got)
	}
	if got := recalcOfferTotal(nil); got != 0 {
		t.Fatalf("итог nil-состава: %v", got)
	}
}
