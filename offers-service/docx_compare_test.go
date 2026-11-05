package main

import (
	"strings"
	"testing"
)

// Таблица сравнения систем в документе (2026-11-05).
//
// Образец конкурента показывает две системы одной таблицей «показатель —
// значение»: так заказчик видит разницу, не листая четыре страницы. Данные
// берутся из таблицы «Свойства и преимущества» карточек, а не от модели:
// сравнение характеристик — это факты, их нельзя сочинять.

// compareCards — две карточки с общим показателем и разными значениями.
func compareCards() []SystemCard {
	return []SystemCard{
		{
			NoteID: 1,
			Title:  "СТИЛОБАТ V2.0",
			Specifications: [][]string{
				{"Показатель", "Ед. изм.", "Значение"},
				{"Контроль на этапе монтажа", "-", "сварные швы + вакуумный тест"},
				{"Стоимость", "-", "высокая"},
			},
		},
		{
			NoteID: 2,
			Title:  "СТИЛОБАТ V1.0",
			Specifications: [][]string{
				{"Показатель", "Ед. изм.", "Значение"},
				{"Контроль на этапе монтажа", "-", "сварные швы"},
				{"Стоимость", "-", "средняя"},
			},
		},
	}
}

// Строки сравнения: первая — заголовок с названиями систем, дальше по
// показателю на строку в порядке первой карточки.
func TestCompareSystemRows(t *testing.T) {
	rows := compareSystemRows(compareCards())
	if len(rows) != 3 {
		t.Fatalf("ожидали заголовок и две строки, получили %d: %#v", len(rows), rows)
	}
	if rows[0][0] != "Показатель" || rows[0][1] != "СТИЛОБАТ V2.0" || rows[0][2] != "СТИЛОБАТ V1.0" {
		t.Fatalf("заголовок неверный: %#v", rows[0])
	}
	if rows[1][0] != "Контроль на этапе монтажа" {
		t.Fatalf("первый показатель неверный: %#v", rows[1])
	}
	if rows[1][1] != "сварные швы + вакуумный тест" || rows[1][2] != "сварные швы" {
		t.Fatalf("значения разъехались: %#v", rows[1])
	}
}

// Показатель, которого нет у второй системы, остаётся пустым — а не
// заполняется «нет данных»: пустая ячейка честнее выдумки.
func TestCompareSystemRowsMissingValueIsEmpty(t *testing.T) {
	cards := compareCards()
	cards[1].Specifications = [][]string{
		{"Показатель", "Ед. изм.", "Значение"},
		{"Стоимость", "-", "средняя"},
	}
	rows := compareSystemRows(cards)
	var control []string
	for _, row := range rows {
		if row[0] == "Контроль на этапе монтажа" {
			control = row
		}
	}
	if control == nil {
		t.Fatal("показатель первой системы потерян")
	}
	if control[2] != "" {
		t.Fatalf("недостающее значение должно быть пустым, получили %q", control[2])
	}
}

// Одна система — сравнивать нечего, таблицы нет.
func TestCompareSystemRowsSingleSystem(t *testing.T) {
	if rows := compareSystemRows(compareCards()[:1]); rows != nil {
		t.Fatalf("для одной системы таблица не нужна: %#v", rows)
	}
}

// Заголовок карточки («Показатель | Ед. изм. | Значение») не становится
// строкой сравнения: иначе в таблице появится строка «Показатель».
func TestCompareSystemRowsSkipsHeaderRow(t *testing.T) {
	rows := compareSystemRows(compareCards())
	for _, row := range rows[1:] {
		if row[0] == "Показатель" {
			t.Fatalf("шапка карточки попала в сравнение: %#v", rows)
		}
	}
}

// «Параметр | Значение» — тоже шапка, а не показатель: так подписана таблица в
// живых карточках (и в стендовой 101). На стенде отсутствие этого слова в
// списке шапок выглядело как «таблица сравнения не печатается».
func TestCompareSystemRowsSkipsParametrHeader(t *testing.T) {
	cards := []SystemCard{
		{
			NoteID:         1,
			Title:          "А",
			Specifications: [][]string{{"Параметр", "Значение"}, {"Класс нагрузки", "1,5 кН/м²"}},
		},
		{
			NoteID:         2,
			Title:          "Б",
			Specifications: [][]string{{"Параметр", "Значение"}, {"Класс нагрузки", "2,0 кН/м²"}},
		},
	}
	rows := compareSystemRows(cards)
	for _, row := range rows {
		if row[0] == "Параметр" {
			t.Fatalf("шапка «Параметр» попала в сравнение: %#v", rows)
		}
	}
	if len(rows) != 2 || rows[1][0] != "Класс нагрузки" {
		t.Fatalf("таблица сравнения не построилась: %#v", rows)
	}
}

// Узел формы `compare_table` печатает таблицу из выбранных систем.
func TestDocumentPrintsCompareTable(t *testing.T) {
	offer := Offer{Systems: OfferSystems{{NoteID: 1}, {NoteID: 2}}}
	form := formWithBlock(`[{"type":"compare_table"}]`)
	docx, err := renderOfferDocxWithCards(form, offer, compareCards(), nil)
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	body := string(docxEntry(t, docx, "word/document.xml"))
	if !strings.Contains(body, "сварные швы + вакуумный тест") {
		t.Fatal("значения сравнения не попали в документ")
	}
	if !strings.Contains(body, "СТИЛОБАТ V1.0") || !strings.Contains(body, "СТИЛОБАТ V2.0") {
		t.Fatal("названия систем в сравнении потеряны")
	}
	if !strings.Contains(body, "<w:tbl>") {
		t.Fatal("сравнение должно печататься таблицей")
	}
}

// Одна система — узел молчит, а не печатает таблицу из одной колонки.
func TestDocumentSkipsCompareTableForOneSystem(t *testing.T) {
	offer := Offer{Systems: OfferSystems{{NoteID: 1}}}
	form := formWithBlock(`[{"type":"compare_table"}]`)
	docx, err := renderOfferDocxWithCards(form, offer, compareCards(), nil)
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	body := string(docxEntry(t, docx, "word/document.xml"))
	if strings.Contains(body, "сварные швы + вакуумный тест") {
		t.Fatal("таблица сравнения не должна печататься для одной системы")
	}
}
