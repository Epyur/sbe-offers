package main

import (
	"testing"

	"mailers/offers/skills"
)

// Разрешение состава по ссылке (2026-10-31).
//
// Главное правило: строка со ссылкой на заметку материала — материал, а код
// в строке остаётся запасным способом. Проверяется на живом виде состава:
// «- **2** [[Техноэласт К ЭКП]]».

func materialFromNote(title string, id int64) Material {
	return Material{
		ID: id, SKU: "MAT-МЕМБ-ПВХ-0007", Kind: "material",
		Name: title, Unit: "м2", NoteTitle: title, NoteID: 100 + id,
	}
}

// Строка со ссылкой на материал находит материал по названию заметки.
func TestResolveCompositionByWikiLink(t *testing.T) {
	card := skills.SystemCard{
		Composition: []string{"**2** [[Техноэласт К ЭКП]]"},
		Links:       []skills.CompositionLink{{Raw: "**2** [[Техноэласт К ЭКП]]"}},
	}
	items, problems := resolveComposition(card, []Material{materialFromNote("Техноэласт К ЭКП", 1)})
	if len(items) != 1 || !items[0].Found {
		t.Fatalf("материал по ссылке не найден: %#v / %#v", items, problems)
	}
	if items[0].MaterialID != 1 {
		t.Errorf("не тот материал: %#v", items[0])
	}
	if len(problems) != 0 {
		t.Errorf("проблем быть не должно: %#v", problems)
	}
}

// Ссылка на заметку без материала в справочнике материалом не считается:
// это норматив или протокол, а не то, что продаётся.
func TestResolveCompositionWikiNonMaterial(t *testing.T) {
	card := skills.SystemCard{
		Composition: []string{"[[ГОСТ 30244]]"},
		Links:       []skills.CompositionLink{{Raw: "[[ГОСТ 30244]]"}},
	}
	items, problems := resolveComposition(card, []Material{materialFromNote("Техноэласт К ЭКП", 1)})
	if items[0].Found {
		t.Fatalf("норматив найден материалом: %#v", items[0])
	}
	if len(problems) != 1 {
		t.Fatalf("строка должна быть названа в проблемах: %#v", problems)
	}
}

// Код в строке без ссылки работает по-прежнему: запасной способ.
func TestResolveCompositionSkuStillWorks(t *testing.T) {
	card := skills.SystemCard{
		Composition: []string{"Мембрана (MAT-МЕМБ-ПВХ-0007)"},
		Links:       []skills.CompositionLink{{Raw: "Мембрана (MAT-МЕМБ-ПВХ-0007)", SKU: "MAT-МЕМБ-ПВХ-0007"}},
	}
	items, _ := resolveComposition(card, []Material{{ID: 5, SKU: "MAT-МЕМБ-ПВХ-0007", Name: "Мембрана"}})
	if !items[0].Found || items[0].MaterialID != 5 {
		t.Fatalf("код перестал работать: %#v", items[0])
	}
}

// Ссылка важнее кода: если в строке и ссылка, и код от разных материалов,
// верим ссылке — её человек ставил руками на нужную заметку.
func TestResolveCompositionWikiBeatsSku(t *testing.T) {
	card := skills.SystemCard{
		Composition: []string{"[[Техноэласт К ЭКП]] (MAT-ПРОЧ-ОБЩ-0001)"},
		Links: []skills.CompositionLink{{
			Raw: "[[Техноэласт К ЭКП]] (MAT-ПРОЧ-ОБЩ-0001)", SKU: "MAT-ПРОЧ-ОБЩ-0001",
		}},
	}
	materials := []Material{
		{ID: 1, SKU: "MAT-МЕМБ-ПВХ-0007", Name: "Техноэласт К ЭКП", NoteTitle: "Техноэласт К ЭКП"},
		{ID: 2, SKU: "MAT-ПРОЧ-ОБЩ-0001", Name: "Другое"},
	}
	items, _ := resolveComposition(card, materials)
	if !items[0].Found || items[0].MaterialID != 1 {
		t.Fatalf("ссылка не победила код: %#v", items[0])
	}
}

// Пустая ссылка ([[ ]]) — не совпадение ни с чем.
func TestResolveCompositionEmptyWikiLink(t *testing.T) {
	card := skills.SystemCard{
		Composition: []string{"[[   ]]"},
		Links:       []skills.CompositionLink{{Raw: "[[   ]]"}},
	}
	items, problems := resolveComposition(card, []Material{materialFromNote("Техноэласт К ЭКП", 1)})
	if items[0].Found {
		t.Fatalf("пустая ссылка нашла материал: %#v", items[0])
	}
	if len(problems) != 1 {
		t.Fatalf("пустая ссылка должна быть проблемой: %#v", problems)
	}
}

// Регистр и ё/е в имени заметки не важны: Obsidian ищет так же.
func TestResolveCompositionWikiNormalizesName(t *testing.T) {
	card := skills.SystemCard{
		Composition: []string{"[[техноэласт к экп]]"},
		Links:       []skills.CompositionLink{{Raw: "[[техноэласт к экп]]"}},
	}
	items, _ := resolveComposition(card, []Material{materialFromNote("Техноэласт К ЭКП", 3)})
	if !items[0].Found {
		t.Fatalf("регистр помешал найти материал: %#v", items[0])
	}
}

// Архивный материал по ссылке — отдельная понятная проблема, а не «не найден».
func TestResolveCompositionWikiArchived(t *testing.T) {
	m := materialFromNote("Техноэласт К ЭКП", 4)
	m.Archived = true
	card := skills.SystemCard{
		Composition: []string{"[[Техноэласт К ЭКП]]"},
		Links:       []skills.CompositionLink{{Raw: "[[Техноэласт К ЭКП]]"}},
	}
	items, problems := resolveComposition(card, []Material{m})
	if items[0].Found {
		t.Fatalf("архивный материал найденным быть не должен: %#v", items[0])
	}
	if len(problems) != 1 {
		t.Fatalf("должна быть одна проблема: %#v", problems)
	}
}
