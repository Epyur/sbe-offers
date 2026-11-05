package main

import (
	"testing"
)

// Выбор системы в предложение (переработан 2026-11-03).
//
// Позиций выбор системы больше НЕ добавляет: в КП от него попадают обоснование
// применения, преимущества и описание основных продуктов, а состав с ценами
// менеджер ведёт сам — файлом или руками. Причина — живое КП 003/2026: состав
// дописывался к позициям из таблицы, и одни и те же продукты стояли в таблице
// дважды, с ценой и с нулём.

// Система даёт число основных продуктов — только найденных в справочнике.
// Слои и работы материалом не являются, и обещать больше продуктов, чем есть,
// значит врать в документе.
func TestCountCompositionMaterialsCountsFoundOnly(t *testing.T) {
	links := []CompositionItem{
		{Raw: "Мембрана", MaterialID: 1, Found: true},
		{Raw: "Песок", MaterialID: 2, Found: true},
		{Raw: "Подготовка основания", Layer: true},
		{Raw: "Забытый материал"},
	}
	if got := countCompositionMaterials(links); got != 2 {
		t.Fatalf("ожидали двух продуктов, получили %d", got)
	}
}

// Пустой состав — ноль, а не ошибка: карточку без состава выбирать можно.
func TestCountCompositionMaterialsEmpty(t *testing.T) {
	if got := countCompositionMaterials(nil); got != 0 {
		t.Fatalf("у пустого состава ожидали 0, получили %d", got)
	}
}

// Попытка дописать позиции по системе должна исчезнуть вместе с кодом —
// проверяем, что в ответе ручки нет поля позиций, а состав предложения не
// меняется. До 2026-11-03 здесь было `added` и дописанные строки.
func TestComposeSystemResponseHasNoPositions(t *testing.T) {
	resp := ComposeSystemResponse{
		Offer:       Offer{Items: []OfferItem{{Name: "Своя позиция", Qty: 1, Price: 10, Sum: 10}}},
		System:      SystemCard{Title: "Кровля", Description: "Описание"},
		Composition: 3,
	}
	if len(resp.Offer.Items) != 1 || resp.Offer.Items[0].Name != "Своя позиция" {
		t.Fatalf("состав предложения изменился: %#v", resp.Offer.Items)
	}
	if resp.System.Description == "" || resp.Composition != 3 {
		t.Fatalf("карточка и число продуктов не заполнены: %#v", resp)
	}
}

// Пустой выбор системы в предложении — ни одной карточки: старые КП печатаются
// как раньше, без описаний систем.
func TestCardsForOfferWithoutChoice(t *testing.T) {
	offer := Offer{Items: []OfferItem{{Name: "Мембрана ПВХ", Qty: 1, Price: 10, Sum: 10}}}
	cards := []SystemCard{{SKU: "A", Title: "Кровля", Description: "Описание"}}
	if got := cardsForOffer(cards, offer); len(got) != 0 {
		t.Fatalf("без выбранной системы карточки не нужны: %#v", got)
	}
}

// Выбранная система печатается даже в пустом КП: описание и обоснование нужны
// менеджеру ДО того, как он завёл позиции (это и есть весь смысл выбора).
func TestCardsForOfferByChoiceWithoutItems(t *testing.T) {
	offer := Offer{Systems: OfferSystems{OfferSystem{NoteID: 11}}}
	cards := []SystemCard{
		{NoteID: 11, Title: "Кровля", Description: "Описание кровли"},
		{NoteID: 12, Title: "Стена", Description: "Описание стены"},
	}
	got := cardsForOffer(cards, offer)
	if len(got) != 1 || got[0].Title != "Кровля" {
		t.Fatalf("ожидали только выбранную систему: %#v", got)
	}
}

// Система выбирается и по коду: код главнее заметки, правило одно с FindSystem.
func TestCardsForOfferBySku(t *testing.T) {
	offer := Offer{Systems: OfferSystems{OfferSystem{SKU: " tn-roof "}}}
	cards := []SystemCard{{SKU: "TN-ROOF", Title: "Кровля", Description: "Описание"}}
	if got := cardsForOffer(cards, offer); len(got) != 1 || got[0].SKU != "TN-ROOF" {
		t.Fatalf("система по коду не выбрана: %#v", got)
	}
}

// Выбранная система исчезла из Базы знаний — карточки нет, и это не падение:
// документ собирается без описания, как раньше.
func TestCardsForOfferUnknownChoice(t *testing.T) {
	offer := Offer{Systems: OfferSystems{OfferSystem{NoteID: 999}}}
	cards := []SystemCard{{NoteID: 11, Title: "Кровля", Description: "Описание"}}
	if got := cardsForOffer(cards, offer); len(got) != 0 {
		t.Fatalf("чужой номер заметки ничего не выбирает: %#v", got)
	}
}

// Несколько систем в одном КП (2026-11-05): печатаются все выбранные, в
// порядке выбора. Именно этого не хватало для сравнения двух решений —
// образца конкурента с СТИЛОБАТ V1.0 и V2.0.
func TestCardsForOfferSeveralSystems(t *testing.T) {
	offer := Offer{Systems: OfferSystems{{NoteID: 12}, {NoteID: 11}}}
	cards := []SystemCard{
		{NoteID: 11, Title: "Кровля", Description: "Описание кровли"},
		{NoteID: 12, Title: "Стена", Description: "Описание стены"},
	}
	got := cardsForOffer(cards, offer)
	if len(got) != 2 {
		t.Fatalf("ожидали две карточки, получили %d: %#v", len(got), got)
	}
	if got[0].Title != "Стена" || got[1].Title != "Кровля" {
		t.Fatalf("порядок выбора не сохранён: %#v", got)
	}
}

// Одна и та же система, выбранная и по заметке, и по коду, печатается один
// раз: сопоставление в списке не должно давать дубль раздела.
func TestCardsForOfferDeduplicatesCards(t *testing.T) {
	offer := Offer{Systems: OfferSystems{{NoteID: 11}, {SKU: "TN-ROOF"}}}
	cards := []SystemCard{{NoteID: 11, SKU: "TN-ROOF", Title: "Кровля", Description: "Описание"}}
	if got := cardsForOffer(cards, offer); len(got) != 1 {
		t.Fatalf("дубль не отсечён: %#v", got)
	}
}

// Исчезнувшая из Базы знаний система не мешает печатать остальные.
func TestCardsForOfferSkipsUnknownOne(t *testing.T) {
	offer := Offer{Systems: OfferSystems{{NoteID: 999}, {NoteID: 11}}}
	cards := []SystemCard{{NoteID: 11, Title: "Кровля", Description: "Описание"}}
	got := cardsForOffer(cards, offer)
	if len(got) != 1 || got[0].Title != "Кровля" {
		t.Fatalf("живая система потерялась: %#v", got)
	}
}

// Поиск системы по коду: регистр и краевые пробелы не важны, пустой код ничего
// не находит.
func TestFindSystemNormalizesSku(t *testing.T) {
	systems := []SystemCard{
		{NoteID: 1, SKU: "TN-ROOF-CLASSIC", Title: "Кровля"},
		{NoteID: 2, SKU: " TN-STILOBAT ", Title: "Стилобат"},
	}
	if got, ok := FindSystem(systems, ParseSystemChoice(" tn-roof-classic  ")); !ok || got.NoteID != 1 {
		t.Fatalf("код не найден без учёта регистра и пробелов: %+v %v", got, ok)
	}
	if got, ok := FindSystem(systems, ParseSystemChoice("tn-stilobat")); !ok || got.NoteID != 2 {
		t.Fatalf("код с пробелами в карточке не найден: %+v %v", got, ok)
	}
	if _, ok := FindSystem(systems, ParseSystemChoice("   ")); ok {
		t.Fatal("пустой код не должен ничего находить")
	}
	if _, ok := FindSystem(systems, ParseSystemChoice("TN-OTHER")); ok {
		t.Fatal("незнакомый код не должен ничего находить")
	}
}

// Выбор системы из списка не теряется (2026-11-02, живая жалоба по КП 003).
//
// В живых карточках Базы знаний кода системы нет вовсе: ни одна из 375 заметок
// с тегом commercial_propose не содержит `system_sku`. Плагин при этом
// предлагал выбрать систему из списка, которого нет — единственный вариант
// списка имел пустое значение, и «Добавить» отвечало «Выберите систему».
// Поэтому выбор несёт идентификатор заметки, а поиск идёт сначала по коду,
// затем по заметке.
func TestFindSystemByNoteID(t *testing.T) {
	systems := []SystemCard{
		{NoteID: 11, Title: "Кровля"},
		{NoteID: 12, SKU: "TN-STILOBAT", Title: "Стилобат"},
	}
	if got, ok := FindSystem(systems, SystemChoice{NoteID: 11}); !ok || got.NoteID != 11 {
		t.Fatalf("система без кода не найдена по заметке: %+v %v", got, ok)
	}
	if got, ok := FindSystem(systems, SystemChoice{NoteID: 12}); !ok || got.SKU != "TN-STILOBAT" {
		t.Fatalf("система с кодом не найдена по заметке: %+v %v", got, ok)
	}
	if _, ok := FindSystem(systems, SystemChoice{NoteID: 99}); ok {
		t.Fatal("чужой номер заметки не должен ничего находить")
	}
}

// Пустой выбор — отказ с понятным текстом, а не молчаливое «не найдено».
func TestFindSystemRejectsEmptyChoice(t *testing.T) {
	systems := []SystemCard{{NoteID: 11, Title: "Кровля"}}
	for _, choice := range []SystemChoice{{}, {NoteID: -1}} {
		if _, ok := FindSystem(systems, choice); ok {
			t.Fatalf("пустой выбор %+v не должен ничего находить", choice)
		}
	}
}

// Код главнее номера заметки: система с кодом ищется и по коду, по которому
// её печатает бот и подставляет старый плагин.
func TestFindSystemPrefersSku(t *testing.T) {
	systems := []SystemCard{{NoteID: 11, SKU: "TN-ROOF", Title: "Кровля"}}
	if got, ok := FindSystem(systems, SystemChoice{SKU: " tn-roof ", NoteID: 99}); !ok || got.NoteID != 11 {
		t.Fatalf("поиск по коду сломан: %+v %v", got, ok)
	}
}

// Значение варианта списка никогда не пустое, и заметка не путается с кодом:
// по нему плагин отправляет выбор назад, а сервис разбирает его.
//
// Ровно здесь и был живой дефект: у 375 карточек нет кода, вариант списка
// получался пустым, и «Добавить» отвечало «Выберите систему».
func TestSystemChoiceHandleRoundTrip(t *testing.T) {
	cases := []struct {
		card   SystemCard
		handle string
		parsed SystemChoice
	}{
		{SystemCard{NoteID: 42}, "sys-42", SystemChoice{NoteID: 42}},
		{SystemCard{NoteID: 42, SKU: "TN-ROOF"}, "sys-42", SystemChoice{NoteID: 42}},
		{SystemCard{SKU: "TN-ROOF"}, "TN-ROOF", SystemChoice{SKU: "TN-ROOF"}},
		// Код, похожий на число, остаётся кодом: префикс его отличает.
		{SystemCard{SKU: "12"}, "12", SystemChoice{SKU: "12"}},
	}
	for _, tc := range cases {
		handle := tc.card.Choice().Handle()
		if handle != tc.handle {
			t.Fatalf("значение варианта %+v = %q, ожидали %q", tc.card, handle, tc.handle)
		}
		if handle == "" {
			t.Fatalf("у карточки %+v пустое значение варианта — выбрать её нельзя", tc.card)
		}
		if got := ParseSystemChoice(handle); got != tc.parsed {
			t.Fatalf("разбор %q = %+v, ожидали %+v", handle, got, tc.parsed)
		}
	}
	// Мусор в значении варианта — пустой выбор, а не поиск по коду "sys-x".
	for _, bad := range []string{"", "   ", "sys-", "sys-abc", "sys-0", "sys--3"} {
		if got := ParseSystemChoice(bad); got != (SystemChoice{}) {
			t.Fatalf("разбор мусора %q = %+v, ожидали пустой выбор", bad, got)
		}
	}
}

// Номер ЕКН из справочника: первый непустой. После 2026-11-03 он нужен не
// выбору системы, а загрузке таблицы менеджера (upload.go), поэтому функция
// осталась и проверяется здесь же.
func TestPrimaryEknTakesFirstNonEmpty(t *testing.T) {
	m := Material{ID: 9, Name: "Мембрана", Unit: "м2",
		Ekn: []MaterialEkn{{Ekn: "  "}, {Ekn: "66950"}, {Ekn: "66951"}}}
	if got := primaryEkn(m); got != "66950" {
		t.Fatalf("ЕКН = %q, ожидали первый непустой номер 66950", got)
	}
}

// Номеров нет вовсе — пустая строка, а не выдуманный код.
func TestPrimaryEknWithoutNumbers(t *testing.T) {
	if got := primaryEkn(Material{ID: 10, Name: "Песок", Unit: "м3"}); got != "" {
		t.Fatalf("у материала без ЕКН номер = %q, ожидали пусто", got)
	}
}
