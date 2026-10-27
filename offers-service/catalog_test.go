package main

import "testing"

func TestFilterCatalogItems(t *testing.T) {
	items := []CatalogItem{
		{ID: 1, Name: "Кровельная система X", Kind: "system"},
		{ID: 2, Name: "Теплоизоляция PIR", Kind: "material"},
		{ID: 3, Name: "Монтаж кровли", Kind: "work", Description: "работа по укладке"},
	}

	// «кровель» ловит название системы, «кровли» — название работы.
	if got := filterCatalogItems(items, "кровель"); len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("ожидали совпадение по «кровель» (система), получили %+v", got)
	}
	if got := filterCatalogItems(items, "кровли"); len(got) != 1 || got[0].ID != 3 {
		t.Fatalf("ожидали совпадение по «кровли» (работа), получили %+v", got)
	}
	// Все слова запроса должны найтись, причём в разных полях.
	if got := filterCatalogItems(items, "монтаж укладке"); len(got) != 1 || got[0].ID != 3 {
		t.Fatalf("два слова из разных полей должны совпасть, получили %+v", got)
	}
	if got := filterCatalogItems(items, "тепло"); len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("ожидали единственное совпадение по «тепло» (id 2), получили %+v", got)
	}
	if got := filterCatalogItems(items, "несуществующее"); len(got) != 0 {
		t.Fatalf("ожидали пустой список, получили %d", len(got))
	}
	if got := filterCatalogItems(items, "  кровель "); len(got) != 1 {
		t.Fatalf("регистр и пробелы не должны мешать: получили %d", len(got))
	}
	if got := filterCatalogItems(items, ""); len(got) != 3 {
		t.Fatalf("пустой запрос возвращает всё, получили %d", len(got))
	}
}

func TestValidateCatalogItem(t *testing.T) {
	if msg := validateCatalogItem(CatalogItem{}); msg != "не заполнено название" {
		t.Fatalf("ожидали ошибку про название, получили %q", msg)
	}
	if msg := validateCatalogItem(CatalogItem{Name: "Система", BasePrice: -1}); msg != "цена не может быть отрицательной" {
		t.Fatalf("ожидали ошибку про цену, получили %q", msg)
	}
	if msg := validateCatalogItem(CatalogItem{Name: "Система", BasePrice: 100}); msg != "" {
		t.Fatalf("корректная карточка не должна давать ошибку, получили %q", msg)
	}
	if msg := validateCatalogItem(CatalogItem{Name: "Система", Kind: "прочее"}); msg == "" {
		t.Fatal("неизвестный тип должен отвергаться")
	}
}

func TestCatalogKindAllowed(t *testing.T) {
	if !catalogKindAllowed("system") || !catalogKindAllowed("material") || !catalogKindAllowed("work") {
		t.Fatal("все три типа должны быть допустимы")
	}
	if catalogKindAllowed("прочее") {
		t.Fatal("неизвестный тип должен быть отвергнут")
	}
}
