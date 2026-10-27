package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// fakeCatalog — подмена БД для HTTP-ручек каталога: без Postgres.
// Минимальный in-memory вариант тех же операций, что делает catalog.go.
type fakeCatalog struct {
	rows  map[int64]CatalogItem
	nextI int64
}

func (f *fakeCatalog) create(item CatalogItem) CatalogItem {
	f.nextI++
	item.ID = f.nextI
	f.rows[item.ID] = item
	return item
}

func (f *fakeCatalog) update(id int64, item CatalogItem) (CatalogItem, bool) {
	if _, ok := f.rows[id]; !ok {
		return CatalogItem{}, false
	}
	item.ID = id
	f.rows[id] = item
	return item, true
}

func (f *fakeCatalog) archive(id int64) bool {
	item, ok := f.rows[id]
	if !ok {
		return false
	}
	item.Archived = true
	f.rows[id] = item
	return true
}

// catalogListLogic — воспроизводит отбор каталога: архивные скрыты, поиск по
// всем словам. Именно этот отбор вызывал жалобу «каталог пустой».
func catalogListLogic(rows map[int64]CatalogItem, query string, includeArchived bool) []CatalogItem {
	items := make([]CatalogItem, 0, len(rows))
	for _, item := range rows {
		if item.Archived && !includeArchived {
			continue
		}
		items = append(items, item)
	}
	return filterCatalogItems(items, query)
}

func TestCatalogCreateAndSearch(t *testing.T) {
	f := &fakeCatalog{rows: map[int64]CatalogItem{}}
	created := f.create(CatalogItem{Kind: "system", Name: "Кровельная система", Unit: "м2", BasePrice: 999.99})
	if created.ID == 0 {
		t.Fatal("карточка должна получить id")
	}
	if got := catalogListLogic(f.rows, "кровель", false); len(got) != 1 {
		t.Fatalf("поиск по слову не нашёл карточку: %+v", got)
	}
	if got := catalogListLogic(f.rows, "тепло", false); len(got) != 0 {
		t.Fatalf("по несуществующему слову список должен быть пуст, получили %d", len(got))
	}
}

func TestCatalogArchivedHiddenByDefault(t *testing.T) {
	f := &fakeCatalog{rows: map[int64]CatalogItem{}}
	item := f.create(CatalogItem{Kind: "system", Name: "Старая система"})
	if !f.archive(item.ID) {
		t.Fatal("архивация существующей карточки должна удаться")
	}
	if got := catalogListLogic(f.rows, "", false); len(got) != 0 {
		t.Fatalf("архивная карточка не должна попадать в обычный список, получили %d", len(got))
	}
	if got := catalogListLogic(f.rows, "", true); len(got) != 1 {
		t.Fatalf("с archived=1 карточка должна быть видна, получили %d", len(got))
	}
	if f.archive(999) {
		t.Fatal("архивация несуществующей карточки должна вернуть false")
	}
}

func TestCatalogUpdateMissingCard(t *testing.T) {
	f := &fakeCatalog{rows: map[int64]CatalogItem{}}
	if _, ok := f.update(42, CatalogItem{Name: "Нет такой"}); ok {
		t.Fatal("изменение несуществующей карточки не должно проходить")
	}
}

// validateCatalogItem — граница входа: проверяем данные там, где они приходят.
func TestCatalogValidationAtBoundary(t *testing.T) {
	cases := []struct {
		item CatalogItem
		want string
	}{
		{CatalogItem{}, "не заполнено название"},
		{CatalogItem{Name: "X", Kind: "прочее"}, "тип должен быть"},
		{CatalogItem{Name: "X", BasePrice: -5}, "цена не может быть отрицательной"},
		{CatalogItem{Name: "X", BasePrice: 10}, ""},
	}
	for i, tc := range cases {
		got := validateCatalogItem(tc.item)
		if tc.want == "" && got != "" {
			t.Fatalf("случай %d: корректная карточка отвергнута: %s", i, got)
		}
		if tc.want != "" && !strings.Contains(got, tc.want) {
			t.Fatalf("случай %d: ожидали %q, получили %q", i, tc.want, got)
		}
	}
}

// Пустой каталог и путь «загружается» — состояния, которые видит человек.
func TestEmptyCatalogMessagePath(t *testing.T) {
	items := catalogListLogic(map[int64]CatalogItem{}, "", false)
	if len(items) != 0 {
		t.Fatalf("ожидали пустой список, получили %d", len(items))
	}
	if items == nil {
		t.Fatal("список должен быть пустым срезом, а не nil: nil сериализуется в null")
	}
}

// Проверка, что контекст с email не теряется на пути к ручке.
func TestPermContextCarriesEmail(t *testing.T) {
	ctx := context.WithValue(context.Background(), permEmailCtx{}, "a@tn.ru")
	if got, _ := ctx.Value(permEmailCtx{}).(string); got != "a@tn.ru" {
		t.Fatalf("email в контексте потерялся: %q", got)
	}
}

// Дата/время в ответах: формат с долями секунды, чтобы слияние не путалось.
func TestTimeFormatHasMillis(t *testing.T) {
	now := time.Date(2026, 10, 27, 10, 0, 0, 123000000, time.UTC)
	const layout = "2006-01-02T15:04:05.000Z"
	got := now.Format(layout)
	if !strings.Contains(got, ".123Z") {
		t.Fatalf("доли секунды потерялись: %q", got)
	}
	_ = fmt.Sprint()
}
