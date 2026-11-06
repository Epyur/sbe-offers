package main

import (
	"strings"
	"testing"
)

// Добор покрытия функций, появившихся в 2026-11-05 (порог CRAP < 10 по
// навыку ub-mode). Покрыто поведение, а не реализация: значения для макета,
// слияние кэша и выборка названий систем — то, что видит человек.

// layoutValues отдаёт служебные поля и название системы.
func TestLayoutValuesHasServiceFields(t *testing.T) {
	offer := sampleOfferWithItems()
	offer.Number = "003/2026"
	offer.Client.Name = "ООО Поэма"
	offer.Systems = []OfferSystem{{SKU: "TN-ROOF"}, {SKU: "TN-WALL"}}
	offer.FormValues = map[string]string{"object_name": "Дом по ул. Примерная"}

	values := layoutValues(offer)
	cases := map[string]string{
		fieldOfferNumber: "003/2026",
		fieldClientName:  "ООО Поэма",
		fieldObjectName:  "Дом по ул. Примерная",
		"system_title":   "TN-ROOF",
		"systems_list":   "TN-ROOF; TN-WALL",
	}
	for key, want := range cases {
		if values[key] != want {
			t.Errorf("%s = %q, ожидалось %q", key, values[key], want)
		}
	}
}

// Без выбранных систем макет не получает названия — и не врёт «система №0».
func TestLayoutValuesWithoutSystems(t *testing.T) {
	values := layoutValues(sampleOfferWithItems())
	if values["system_title"] != "" {
		t.Fatalf("название системы без выбора: %q", values["system_title"])
	}
}

// Первое значение главнее: форма заполнена — берётся она.
func TestLayoutValuesFormWins(t *testing.T) {
	offer := sampleOfferWithItems()
	offer.FormValues = map[string]string{fieldOfferNumber: "из формы"}
	offer.Number = "из предложения"
	if got := layoutValues(offer)[fieldOfferNumber]; got != "из формы" {
		t.Fatalf("значение формы не главнее: %q", got)
	}
}

// offerSystemTitles: код, иначе «система №N» — макет не должен печатать пусто.
func TestOfferSystemTitles(t *testing.T) {
	offer := Offer{Systems: []OfferSystem{{SKU: "TN-A"}, {NoteID: 42}}}
	got := offerSystemTitles(offer)
	if len(got) != 2 || got[0] != "TN-A" || !strings.Contains(got[1], "42") {
		t.Fatalf("названия систем: %v", got)
	}
	if len(offerSystemTitles(Offer{})) != 0 {
		t.Fatal("без систем список не пуст")
	}
}

// mergeFormValues сохраняет старые ключи и дописывает хеш свежести.
func TestMergeFormValuesKeepsOldKeys(t *testing.T) {
	merged := mergeFormValues(
		map[string]string{"object_name": "Дом", genKeySection: "old"},
		map[string]string{"gen.layout.x": "текст", genKeySection: "new"},
	)
	if merged["object_name"] != "Дом" {
		t.Fatal("старое значение потеряно")
	}
	if merged["gen.layout.x"] != "текст" || merged[genKeySection] != "new" {
		t.Fatalf("новые значения не дописаны: %v", merged)
	}
}

// Тексты промтов макета кэшируются: второй вызов не спрашивает модель.
func TestLayoutPromptCacheRidesCache(t *testing.T) {
	counter := &countingCompleter{answer: "Готовый текст."}
	cache := &layoutPromptCache{
		values:    map[string]string{},
		previous:  map[string]string{genKeySection: hashOfferFacts(sampleOfferWithItems())},
		section:   hashOfferFacts(sampleOfferWithItems()),
		offer:     sampleOfferWithItems(),
		completed: counter,
	}
	if got := cache.text("опиши систему"); got != "Готовый текст." {
		t.Fatalf("первый вызов: %q", got)
	}
	if got := cache.text("опиши систему"); got != "Готовый текст." {
		t.Fatalf("второй вызов: %q", got)
	}
	if counter.calls != 1 {
		t.Fatalf("модель спрошена %d раз, ожидался один", counter.calls)
	}
	if len(cache.changed()) == 0 {
		t.Fatal("новый текст не попал в список изменений")
	}
}

// Пустой промт не зовёт модель: узел молчит.
func TestLayoutPromptCacheEmptyInstruction(t *testing.T) {
	counter := &countingCompleter{}
	cache := &layoutPromptCache{values: map[string]string{}, completed: counter}
	if got := cache.text("   "); got != "" {
		t.Fatalf("пустой промт дал текст: %q", got)
	}
	if counter.calls != 0 {
		t.Fatal("модель вызвана на пустом промте")
	}
}
