package main

import (
	"testing"
)

// Каталог бота — только материалы (Task 8, 2026-10-31).
//
// В боте собирают КП на ходу, и система там не позиция: у неё нет цены, а
// состав берётся из Базы знаний, а не из справочника. Если система осталась бы
// в списке, менеджер нажал бы кнопку и получил строку с выдуманной ценой.
//
// Правило живёт в чистой функции: проверяется без базы, HTTP и бота.

// Система в списке бота не появляется, даже если бы попала в таблицу.
func TestMaterialsForBotDropsSystems(t *testing.T) {
	items := []Material{
		{ID: 1, Kind: "material", Name: "Мембрана"},
		{ID: 2, Kind: "work", Name: "Монтаж"},
		{ID: 3, Kind: "system", Name: "ТН-Кровля"},
	}
	got := materialsForBot(items)
	if len(got) != 2 {
		t.Fatalf("в списке бота должно остаться два вида: %#v", got)
	}
	for _, m := range got {
		if m.Kind == "system" {
			t.Fatalf("система попала в каталог бота: %s", m.Name)
		}
	}
}

// Пустой вид по умолчанию — материал: у старых записей kind может не быть.
func TestMaterialsForBotEmptyKindIsMaterial(t *testing.T) {
	got := materialsForBot([]Material{{ID: 1, Name: "Без вида"}, {ID: 2, Kind: "system"}})
	if len(got) != 1 || got[0].Name != "Без вида" {
		t.Fatalf("запись без вида должна остаться: %#v", got)
	}
}

// Поиск находит по названию, коду и номеру ЕКН.
func TestFilterMaterialsForBotSearchesEkn(t *testing.T) {
	items := []Material{
		{ID: 1, Kind: "material", Name: "Мембрана ПВХ", SKU: "MAT-МЕМБ-ПВХ-0007",
			Ekn: []MaterialEkn{{Ekn: "068863"}}},
		{ID: 2, Kind: "material", Name: "Песок", SKU: "MAT-ПРОЧ-ОБЩ-0001"},
	}
	for _, query := range []string{"068863", "mat-мемб", "мембрана"} {
		got := filterMaterialsForBot(items, query)
		if len(got) != 1 || got[0].ID != 1 {
			t.Errorf("запрос %q: %#v", query, got)
		}
	}
	if got := filterMaterialsForBot(items, ""); len(got) != 2 {
		t.Errorf("пустой запрос — весь список, получили %d", len(got))
	}
	if got := filterMaterialsForBot(items, "кровля"); len(got) != 0 {
		t.Errorf("несуществующее слово: %#v", got)
	}
}
