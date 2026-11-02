package main

import (
	"strings"
	"testing"
)

// Шапка документа (2026-11-02).
//
// Живой дефект: в выгруженном КП 003/2026 над титульным листом стоял заголовок,
// которого никто не заказывал, а внутри документа — служебные фразы шаблона
// редактора («Текст с полем») и английский текст задания модели. Причина:
// сервер печатал свою шапку ПЕРЕД блоками формы, а блок формы её повторял.
// Шапку теперь печатает первый блок; эти тесты держат правило.

func testHeadOffer() Offer {
	return Offer{
		Number:     "003/2026",
		CreatedAt:  "2026-11-01T10:00:00.000Z",
		OwnerEmail: "manager@tn.ru",
		Client:     OfferClient{Name: "ООО «Заказчик»"},
		Items:      []OfferItem{{Name: "Мембрана", Unit: "м2", Qty: 10, Price: 100, Sum: 1000}},
		Total:      1000,
		FormValues: map[string]string{"object_name": "ЖК «Поэма»"},
	}
}

// Заголовок формы Не печатается дважды: он приходит из блока формы.
func TestOfferTitlePrintedOnce(t *testing.T) {
	form := OfferForm{
		Title: "Коммерческое предложение",
		Blocks: []Block{
			{ID: "cover", ShowInClient: true, Content: []byte(
				`[{"type":"heading","text":"Коммерческое предложение","level":2}]`)},
		},
	}
	doc := docXMLOf(t, mustRender(t, form, testHeadOffer()))
	if got := strings.Count(doc, "Коммерческое предложение"); got != 1 {
		t.Fatalf("заголовок в документе встречается %d раз, ожидался 1", got)
	}
}

// Номер и дата предложения есть в документе один раз и в русском формате даты.
func TestOfferNumberAndDatePrintedOnce(t *testing.T) {
	form := OfferForm{
		Title: "Коммерческое предложение",
		Blocks: []Block{
			{ID: "cover", ShowInClient: true, Content: []byte(
				`[{"type":"paragraph","text":"№ {{offer_number}} от {{offer_date}}"}]`)},
		},
	}
	doc := docXMLOf(t, mustRender(t, form, testHeadOffer()))
	if strings.Count(doc, "003/2026") != 1 {
		t.Fatalf("номер предложения должен стоять один раз, в документе: %d", strings.Count(doc, "003/2026"))
	}
	if !strings.Contains(doc, "01.11.2026") {
		t.Fatal("дата предложения должна печататься как 01.11.2026")
	}
	if strings.Contains(doc, "{{") {
		t.Fatal("в документе остались служебные скобки {{…}}")
	}
}

// Служебное поле формы главнее вычисленного: человек мог задать другую подпись.
func TestFormValueOverridesServiceField(t *testing.T) {
	offer := testHeadOffer()
	offer.FormValues["manager"] = "Менеджер: Полищук Е. В."
	form := OfferForm{
		Title: "КП",
		Blocks: []Block{
			{ID: "sign", ShowInClient: true, Content: []byte(`[{"type":"paragraph","text":"{{manager}}"}]`)},
		},
	}
	doc := docXMLOf(t, mustRender(t, form, offer))
	if !strings.Contains(doc, "Полищук Е. В.") {
		t.Fatal("подпись из формы должна печататься")
	}
	if strings.Contains(doc, "manager@tn.ru") {
		t.Fatal("вычисленная подпись не должна дублировать строку из формы")
	}
}

// Подпись менеджера не печатается дважды: если она есть в блоках — своей не добавляем.
func TestManagerLineNotDuplicated(t *testing.T) {
	form := OfferForm{
		Title: "КП",
		Blocks: []Block{
			{ID: "sign", ShowInClient: true, Content: []byte(`[{"type":"paragraph","text":"Менеджер: {{manager}}"}]`)},
		},
	}
	doc := docXMLOf(t, mustRender(t, form, testHeadOffer()))
	if got := strings.Count(doc, "Менеджер:"); got != 1 {
		t.Fatalf("строка «Менеджер:» встречается %d раз, ожидалась 1", got)
	}
}

// Форма без блоков всё равно даёт документ с названием и номером.
func TestEmptyFormStillHasTitle(t *testing.T) {
	form := OfferForm{Title: "Коммерческое предложение"}
	doc := docXMLOf(t, mustRender(t, form, testHeadOffer()))
	if !strings.Contains(doc, "Коммерческое предложение") {
		t.Fatal("документ без блоков должен печатать название формы")
	}
	if !strings.Contains(doc, "003/2026") {
		t.Fatal("документ без блоков должен печатать номер предложения")
	}
}

func mustRender(t *testing.T, form OfferForm, offer Offer) []byte {
	t.Helper()
	data, err := renderOfferDocx(form, offer)
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	return data
}

// Колонка ЕКН в таблице состава (2026-11-02).
//
// Живая жалоба: «в таблице КП отсутствует столбец ЕКН, ЕКН не привязались к
// продуктам». Номер теперь доезжает до позиции при сборке состава, а печать
// показывает его отдельной колонкой.
func TestOfferItemsTableHasEknColumn(t *testing.T) {
	items := []OfferItem{
		{EKN: "66950", Name: "Мембрана", Unit: "м2", Qty: 10, Price: 100, Sum: 1000},
		{Name: "Геотекстиль", Unit: "м2", Qty: 5, Price: 50, Sum: 250},
	}
	rows := offerItemsTable(items)
	wantHead := []string{"№", "Наименование", "Код ЕКН", "Ед.", "Кол-во", "Цена", "Сумма"}
	if strings.Join(rows[0], "|") != strings.Join(wantHead, "|") {
		t.Fatalf("шапка таблицы = %v, ожидалась %v", rows[0], wantHead)
	}
	if rows[1][2] != "66950" {
		t.Fatalf("номер ЕКН не попал в позицию: %v", rows[1])
	}
	if rows[2][2] != "" {
		t.Fatalf("у позиции без номера ячейка ЕКН должна быть пустой: %v", rows[2])
	}
}

// Нет ни одного номера — нет и пустой колонки: она выглядит как сбой.
func TestOfferItemsTableNoEknColumnWhenUnknown(t *testing.T) {
	items := []OfferItem{{Name: "Мембрана", Unit: "м2", Qty: 10, Price: 100, Sum: 1000}}
	rows := offerItemsTable(items)
	if strings.Join(rows[0], "|") != "№|Наименование|Ед.|Кол-во|Цена|Сумма" {
		t.Fatalf("без номеров шапка не должна менять вид: %v", rows[0])
	}
}
