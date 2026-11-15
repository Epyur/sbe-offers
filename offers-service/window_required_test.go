package main

// Незаполненные обязательные поля окна КП (2026-11-15, шаг 1 плана 0.11.0).
//
// Обязательность МЯГКАЯ (решение пользователя): сервер возвращает список,
// клиенты подсвечивают, сохранение не блокируется. Проверяется ровно правило
// «элемент обязателен и пуст»: поле — по `formValues`, зона «Заказчик» — по
// паре имени и контакта.

import (
	"strings"
	"testing"
)

// windowTestOffer — документ с полями формы и заказчиком.
func windowTestOffer() Offer {
	return Offer{
		Client: OfferClient{Name: "ООО Заказчик", Contact: "Иван Иванов"},
		FormValues: map[string]string{
			"client_name": "ООО Заказчик",
			"object_name": "ЖК Поэма",
		},
	}
}

// TestMissingWindowFields — пустое обязательное поле видно, заполненное нет.
func TestMissingWindowFields(t *testing.T) {
	form := openTestForm()
	form.Window = []WindowItem{
		{Kind: windowKindField, ID: "client_name", Visible: true, Required: true},
		{Kind: windowKindField, ID: "object_name", Visible: true, Required: true},
		{Kind: windowKindField, ID: "client_contact", Visible: true},
	}
	if got := missingWindowFields(form, windowTestOffer()); len(got) != 0 {
		t.Fatalf("все обязательные заполнены, а получено %+v", got)
	}

	offer := windowTestOffer()
	offer.FormValues["object_name"] = "   "
	got := missingWindowFields(form, offer)
	if len(got) != 1 || got[0].ID != "object_name" {
		t.Fatalf("ожидали одно пустое object_name, получено %+v", got)
	}
	if got[0].Label != "Объект" {
		t.Errorf("подпись пустого поля %q, ожидали «Объект»", got[0].Label)
	}
}

// TestMissingWindowFieldsDefault — окно не задано: обязательность из
// `required_fields`, то есть прежнее поведение.
func TestMissingWindowFieldsDefault(t *testing.T) {
	form := openTestForm()
	offer := windowTestOffer()
	offer.FormValues = map[string]string{}
	if got := missingWindowFields(form, offer); len(got) != 1 || got[0].ID != "client_name" {
		t.Fatalf("ожидали пустое client_name, получено %+v", got)
	}
}

// TestMissingWindowFieldsClientZone — обязательная зона «Заказчик» проверяется
// по паре имени и контакта: заполнено хотя бы одно — зона не пуста.
func TestMissingWindowFieldsClientZone(t *testing.T) {
	form := openTestForm()
	form.Window = []WindowItem{
		{Kind: windowKindZone, ID: "client", Visible: true, Required: true},
	}
	empty := Offer{FormValues: map[string]string{}}
	got := missingWindowFields(form, empty)
	if len(got) != 1 || got[0].ID != windowZoneClient || got[0].Label != "Заказчик" {
		t.Fatalf("пустой заказчик должен попасть в список, получено %+v", got)
	}

	onlyContact := Offer{Client: OfferClient{Contact: "Иван Иванов"}, FormValues: map[string]string{}}
	if got := missingWindowFields(form, onlyContact); len(got) != 0 {
		t.Fatalf("контакт заполнен — зона не пуста, получено %+v", got)
	}
}

// TestMissingWindowFieldsIgnoresHidden — скрытое обязательное поле не
// подсвечивается: администратор убрал его с экрана, и требовать нечего.
func TestMissingWindowFieldsIgnoresHidden(t *testing.T) {
	form := openTestForm()
	form.Window = []WindowItem{
		{Kind: windowKindField, ID: "client_name", Visible: false, Required: true},
	}
	offer := Offer{FormValues: map[string]string{}}
	got := missingWindowFields(form, offer)
	if len(got) != 0 {
		t.Fatalf("скрытое поле не должно попадать в список, получено %+v", got)
	}
}

// TestMissingWindowFieldsEmptyIsArray — пустой список в JSON — массив, не
// `null`: клиент читает `.length` и падает на nil-срезе (живой краш 2026-11-11).
func TestMissingWindowFieldsEmptyIsArray(t *testing.T) {
	form := openTestForm()
	if got := missingWindowFields(form, windowTestOffer()); got == nil {
		t.Fatal("пустой список должен быть массивом, а не nil")
	}
}

// TestMissingWindowFieldsLabelNotRawID — подпись берётся из формы, в ответ
// человеку не уходит идентификатор поля.
func TestMissingWindowFieldsLabelNotRawID(t *testing.T) {
	form := openTestForm()
	offer := Offer{FormValues: map[string]string{}}
	got := missingWindowFields(form, offer)
	if len(got) == 0 {
		t.Fatal("ожидали пустое client_name")
	}
	if strings.TrimSpace(got[0].Label) == "" || got[0].Label == got[0].ID {
		t.Fatalf("у пустого поля должна быть человеческая подпись, получено %+v", got[0])
	}
}
