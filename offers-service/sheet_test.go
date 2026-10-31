package main

import (
	"strings"
	"testing"
)

// Разбор таблицы менеджера (Task 4 плана 2026-10-30).
//
// Таблица — источник правды по цене: менеджер присылает ЕКН, названия и цены,
// и именно эти цены попадают в КП. Поэтому разбор проверяется отдельно от
// HTTP: колонки ищутся по заголовку, а не по номеру, и ошибка в них не должна
// превращаться в цену «0».

func TestSheetColumnsFoundByHeading(t *testing.T) {
	rows := [][]string{
		{"Наименование", "ЕКН", "Ед.", "Кол-во", "Цена"},
		{"Мембрана ПВХ", "068863", "м2", "10", "1500"},
	}
	sheet, err := parseSheetRows(rows)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(sheet.Rows) != 1 {
		t.Fatalf("строк %d, ожидали 1", len(sheet.Rows))
	}
	row := sheet.Rows[0]
	if row.Name != "Мембрана ПВХ" || row.Ekn != "068863" || row.Unit != "м2" {
		t.Fatalf("строка разобрана неверно: %+v", row)
	}
	if row.Qty != 10 || row.Price != 1500 {
		t.Fatalf("числа разобраны неверно: qty=%v price=%v", row.Qty, row.Price)
	}
}

// Колонки идут в любом порядке и в любом регистре: люди не сверяются со схемой.
func TestSheetColumnsAnyOrderAndCase(t *testing.T) {
	rows := [][]string{
		{"цена", "КОЛ-ВО", "ekn", "НАИМЕНОВАНИЕ"},
		{"99,50", "2", "123456", "Грунтовка"},
	}
	sheet, err := parseSheetRows(rows)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	row := sheet.Rows[0]
	if row.Name != "Грунтовка" || row.Ekn != "123456" || row.Price != 99.5 || row.Qty != 2 {
		t.Fatalf("строка разобрана неверно: %+v", row)
	}
}

// Без колонки цены таблица непригодна: цена — её смысл. Ошибка называет
// колонку, а не молчит.
func TestSheetWithoutPriceColumnFails(t *testing.T) {
	rows := [][]string{
		{"Наименование", "ЕКН"},
		{"Мембрана", "068863"},
	}
	_, err := parseSheetRows(rows)
	if err == nil {
		t.Fatal("таблица без цены должна отклоняться")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "цена") {
		t.Fatalf("ошибка не называет колонку цены: %v", err)
	}
}

// Без названия таблица тоже непригодна: сопоставлять нечего.
func TestSheetWithoutNameColumnFails(t *testing.T) {
	rows := [][]string{
		{"ЕКН", "Цена"},
		{"068863", "100"},
	}
	_, err := parseSheetRows(rows)
	if err == nil {
		t.Fatal("таблица без наименования должна отклоняться")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "наименован") {
		t.Fatalf("ошибка не называет колонку наименования: %v", err)
	}
}

// Цена с пробелами и запятой — обычное дело в русском Excel.
func TestSheetParsesRussianNumbers(t *testing.T) {
	rows := [][]string{
		{"Наименование", "Цена"},
		{"Мембрана", "1 500,50"},
		{"Грунтовка", "99,9"},
		{"Праймер", "1200"},
	}
	sheet, err := parseSheetRows(rows)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	want := []float64{1500.5, 99.9, 1200}
	for i, w := range want {
		if sheet.Rows[i].Price != w {
			t.Errorf("строка %d: цена %v, ожидали %v", i, sheet.Rows[i].Price, w)
		}
	}
}

// Строка без названия не выбрасывается молча: в ней может быть цена, и
// менеджер должен увидеть, что она не попала в КП.
func TestSheetKeepsRowWithoutName(t *testing.T) {
	rows := [][]string{
		{"Наименование", "Цена"},
		{"", "500"},
		{"Мембрана", "100"},
	}
	sheet, err := parseSheetRows(rows)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(sheet.Rows) != 2 {
		t.Fatalf("строк %d: строка без названия потеряна", len(sheet.Rows))
	}
	if !sheet.Rows[0].MissingName {
		t.Fatal("строка без названия не помечена")
	}
}

// Заголовок может быть не первой строкой: над ним бывает название документа.
func TestSheetFindsHeaderBelow(t *testing.T) {
	rows := [][]string{
		{"Коммерческое предложение № 5"},
		{},
		{"Наименование", "ЕКН", "Цена"},
		{"Мембрана", "068863", "1500"},
	}
	sheet, err := parseSheetRows(rows)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(sheet.Rows) != 1 || sheet.Rows[0].Name != "Мембрана" {
		t.Fatalf("заголовок не найден ниже первой строки: %+v", sheet.Rows)
	}
}

// Пустые строки между данными (частые в выгрузках) не становятся позициями.
func TestSheetSkipsBlankRows(t *testing.T) {
	rows := [][]string{
		{"Наименование", "Цена"},
		{},
		{"Мембрана", "100"},
		{},
		{"", ""},
		{"Грунтовка", "200"},
	}
	sheet, err := parseSheetRows(rows)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(sheet.Rows) != 2 {
		t.Fatalf("пустые строки попали в данные: %d", len(sheet.Rows))
	}
}
