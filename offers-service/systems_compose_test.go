package main

import (
	"testing"
)

// Состав системы в предложение (Task 7, 2026-10-31).
//
// Правило денег проверяется отдельно от HTTP: цена позиции — нижняя граница
// вилки, а если вилки нет, цена остаётся нулём, и материал называется
// человеку. Подставить «похожее» число здесь означало бы отправить заказчику
// выдуманную цену.

// Вилка есть — позиция получает нижнюю границу и помечена как «с ценой».
func TestItemForMaterialUsesPriceFloor(t *testing.T) {
	low, high := 1200.0, 1500.0
	item, hasPrice := itemForMaterial(Material{ID: 5, Name: "Мембрана", Unit: "м2", PriceMin: &low, PriceMax: &high})
	if !hasPrice {
		t.Fatal("вилка есть — цена должна считаться известной")
	}
	if item.Price != 1200 {
		t.Errorf("цена: %v, ожидали нижнюю границу вилки 1200", item.Price)
	}
	if item.Sum != 1200 || item.Qty != 1 {
		t.Errorf("сумма и количество: %+v", item)
	}
	if item.Name != "Мембрана" || item.Unit != "м2" {
		t.Errorf("поля позиции: %+v", item)
	}
}

// Вилки нет — цена ноль, и признак «не знаем» выставлен: молча класть ноль как
// настоящую цену нельзя.
func TestItemForMaterialWithoutPrice(t *testing.T) {
	item, hasPrice := itemForMaterial(Material{ID: 7, Name: "Песок", Unit: "м3"})
	if hasPrice {
		t.Fatal("вилки нет — известной цены быть не может")
	}
	if item.Price != 0 {
		t.Errorf("цена без вилки: %v", item.Price)
	}
}

// Ноль в вилке — это настоящая цена (бесплатная работа), а не «не знаем».
func TestItemForMaterialZeroPriceIsKnown(t *testing.T) {
	zero := 0.0
	_, hasPrice := itemForMaterial(Material{ID: 8, Name: "Работа", PriceMin: &zero})
	if !hasPrice {
		t.Fatal("ноль в вилке — известная цена, а не её отсутствие")
	}
}

// Дописывание состава: связанные строки становятся позициями, несвязанные
// уходят в проблемы и не теряются, «без цены» называется отдельно.
func TestAppendCompositionKeepsUnlinkedLines(t *testing.T) {
	low := 900.0
	materials := map[int64]Material{
		1: {ID: 1, Name: "Мембрана", Unit: "м2", PriceMin: &low},
		2: {ID: 2, Name: "Песок", Unit: "м3"},
	}
	links := []CompositionItem{
		{Raw: "Мембрана (MAT-МЕМБ-ПВХ-0001)", MaterialID: 1, Found: true},
		{Raw: "Песок", MaterialID: 2, Found: true},
		{Raw: "Непонятная строка"},
	}
	resp := ComposeSystemResponse{Problems: []string{}}
	items, added := appendComposition(nil, links, materials, &resp)

	if added != 2 || len(items) != 2 {
		t.Fatalf("должны дописаться две строки из трёх: added=%d items=%d", added, len(items))
	}
	if len(resp.Problems) != 1 || resp.Problems[0] != "Непонятная строка" {
		t.Fatalf("несвязанная строка должна быть в проблемах: %#v", resp.Problems)
	}
	if len(resp.MissingPrice) != 1 || resp.MissingPrice[0] != "Песок" {
		t.Fatalf("материал без вилки должен называться: %#v", resp.MissingPrice)
	}
	if items[0].Price != 900 || items[1].Price != 0 {
		t.Errorf("цены: %v / %v", items[0].Price, items[1].Price)
	}
}

// Дописывание не стирает уже собранный состав: позиции прибавляются в конец.
func TestAppendCompositionKeepsExistingItems(t *testing.T) {
	existing := []OfferItem{{Name: "Старая позиция", Qty: 1, Price: 10, Sum: 10}}
	links := []CompositionItem{{Raw: "Мембрана", MaterialID: 1, Found: true}}
	low := 5.0
	resp := ComposeSystemResponse{Problems: []string{}}
	items, added := appendComposition(existing, links, map[int64]Material{1: {ID: 1, Name: "Мембрана", PriceMin: &low}}, &resp)

	if added != 1 || len(items) != 2 || items[0].Name != "Старая позиция" {
		t.Fatalf("прежняя позиция потеряна: %#v", items)
	}
}

// Поиск системы по коду: регистр и краевые пробелы не важны, пустой код ничего
// не находит.
func TestFindSystemNormalizesSku(t *testing.T) {
	systems := []SystemCard{
		{NoteID: 1, SKU: "TN-ROOF-CLASSIC", Title: "Кровля"},
		{NoteID: 2, SKU: " TN-STILOBAT ", Title: "Стилобат"},
	}
	if got, ok := FindSystem(systems, " tn-roof-classic  "); !ok || got.NoteID != 1 {
		t.Fatalf("код не найден без учёта регистра и пробелов: %+v %v", got, ok)
	}
	if got, ok := FindSystem(systems, "tn-stilobat"); !ok || got.NoteID != 2 {
		t.Fatalf("код с пробелами в карточке не найден: %+v %v", got, ok)
	}
	if _, ok := FindSystem(systems, "   "); ok {
		t.Fatal("пустой код не должен ничего находить")
	}
	if _, ok := FindSystem(systems, "TN-OTHER"); ok {
		t.Fatal("незнакомый код не должен ничего находить")
	}
}
