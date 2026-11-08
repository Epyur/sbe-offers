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

// offerSystemTitles: название карточки главнее кода и номера заметки (2026-11-08,
// живая жалоба на КП 003/2026): у живых систем нет `system_sku`, и вместо
// названия «ТН-ФУНДАМЕНТ Дренаж Проф» в документ уходило «система №5874».
func TestOfferSystemTitles(t *testing.T) {
	offer := Offer{Systems: []OfferSystem{{SKU: "TN-A"}, {NoteID: 5874}, {NoteID: 99}}}
	cards := []SystemCard{
		{NoteID: 5874, Title: "ТН-ФУНДАМЕНТ Дренаж Проф"},
	}
	got := offerSystemTitles(offer, cards)
	if len(got) != 3 {
		t.Fatalf("названия систем: %v", got)
	}
	if got[0] != "TN-A" {
		t.Fatalf("первая система: %q, ожидалось TN-A", got[0])
	}
	if got[1] != "ТН-ФУНДАМЕНТ Дренаж Проф" {
		t.Fatalf("вторая система должна печататься человеческим названием, получили %q", got[1])
	}
	if !strings.Contains(got[2], "99") {
		t.Fatalf("без карточки ожидался запасной номер заметки, получили %q", got[2])
	}
	if len(offerSystemTitles(Offer{}, nil)) != 0 {
		t.Fatal("без систем список не пуст")
	}
}

// Внутри повтора блока по системам `{{system_title}}` печатает название СВОЕЙ
// системы на каждом проходе, а не первой или служебный номер.
func TestLayoutBlockRepeatResolvesEachSystemTitle(t *testing.T) {
	offer := Offer{
		Systems: []OfferSystem{{NoteID: 101}, {NoteID: 102}},
	}
	cards := []SystemCard{
		{NoteID: 101, Title: "ТН-ФУНДАМЕНТ Дренаж Проф"},
		{NoteID: 102, Title: "ТН-КРОВЛЯ Классик"},
	}
	form := OfferForm{
		Blocks: []Block{{
			ID:           "sysblk",
			Repeat:       "systems",
			ShowInClient: true,
			Content:      []byte(`[{"type":"heading","level":2,"text":"{{system_title}}"}]`),
		}},
	}
	lc := layoutContext{
		values:  layoutValuesWithCards(offer, cards),
		service: serviceValuesWithCards(offer, cards),
		offer:   offer,
		cards:   func() []SystemCard { return cards },
		form:    func() OfferForm { return form },
	}
	doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>{{block:sysblk}}</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if !strings.Contains(body, "ТН-ФУНДАМЕНТ Дренаж Проф") || !strings.Contains(body, "ТН-КРОВЛЯ Классик") {
		t.Fatalf("в документе нет названий обеих систем:\n%s", body)
	}
	if strings.Contains(body, "система №") {
		t.Fatalf("в документе остался служебный идентификатор:\n%s", body)
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
