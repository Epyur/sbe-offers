package main

import (
	"testing"
)

// Условия подразделов (2026-11-14).
//
// Смысл условия один: подраздел с невыполненным условием не печатается ВООБЩЕ
// — включая заголовок. Поэтому проверка идёт и в «да», и в «нет»: условие,
// которое всегда истинно, пропускает заголовки без текста, а условие, которое
// всегда ложно, теряет нужный текст.

func TestSectionConditions(t *testing.T) {
	empty := sectionFacts{}
	oneSystem := sectionFacts{systems: 1}
	twoSystems := sectionFacts{systems: 2}
	oneMaterial := sectionFacts{systems: 1, materials: 1}
	withItems := sectionFacts{systems: 1, items: 3}
	zeroItems := sectionFacts{
		// Позиция с нулевой ценой — всё равно позиция: таблица состава есть.
		items: 1,
	}

	cases := []struct {
		name      string
		condition SubsectionCondition
		facts     sectionFacts
		section   ContentSection
		want      bool
	}{
		{"always без данных", conditionAlways, empty, ContentSection{}, true},
		{"пустое условие — всегда", "", empty, ContentSection{}, true},
		{"has_systems без систем", conditionHasSystems, empty, ContentSection{}, false},
		{"has_systems с одной", conditionHasSystems, oneSystem, ContentSection{}, true},
		{"has_materials без материалов", conditionHasMaterials, oneSystem, ContentSection{}, false},
		{"has_materials с материалом", conditionHasMaterials, oneMaterial, ContentSection{}, true},
		{"multi_system с одной", conditionMultiSystem, oneSystem, ContentSection{}, false},
		{"multi_system с двумя", conditionMultiSystem, twoSystems, ContentSection{}, true},
		{"has_items без позиций", conditionHasItems, empty, ContentSection{}, false},
		{"has_items с нулевой ценой", conditionHasItems, zeroItems, ContentSection{}, true},
		{"has_items с позициями", conditionHasItems, withItems, ContentSection{}, true},
		// Единственное условие, читающее РАЗДЕЛ: переключатель critique.
		{"has_critique выключен", conditionHasCritique, oneSystem, ContentSection{}, false},
		{"has_critique включён", conditionHasCritique, oneSystem, ContentSection{Critique: true}, true},
		// Неизвестное условие молчит: валидация такое не сохраняет, но данные
		// могли прийти из старой базы.
		{"неизвестное условие", SubsectionCondition("sometimes"), oneSystem, ContentSection{Critique: true}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := conditionHolds(c.condition, c.facts, c.section); got != c.want {
				t.Fatalf("conditionHolds(%q) = %v, ожидалось %v", c.condition, got, c.want)
			}
		})
	}
}

// TestOfferFactsCountsChoices — факты считают ВЫБОР, а не пустые записи.
func TestOfferFactsCountsChoices(t *testing.T) {
	offer := Offer{
		Systems:   RefList{{}, {NoteID: 1}, {SKU: "TN-1"}, {NoteID: 1}},
		Materials: RefList{{}, {NoteID: 7547}},
		Items:     []OfferItem{{Name: "A", Qty: 1}},
	}
	facts := offerFacts(offer)
	// Повтор «{NoteID: 1}» — реальный выбор: дедуп здесь не нужен, условию
	// важен сам факт «выбрано что-то» и «больше одной».
	if facts.systems != 3 {
		t.Fatalf("систем %d, ожидалось 3", facts.systems)
	}
	if facts.materials != 1 {
		t.Fatalf("материалов %d, ожидался 1", facts.materials)
	}
	if facts.items != 1 {
		t.Fatalf("позиций %d, ожидалась 1", facts.items)
	}
}
