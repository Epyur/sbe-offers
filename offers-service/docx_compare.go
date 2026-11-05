package main

import "strings"

// Таблица сравнения систем (2026-11-05).
//
// Менеджер может предложить и сравнить несколько систем в одном КП — так
// заказчик видит разницу, не листая разделы. Образец, по которому это делается,
// — ПЕНОПЛЭКС «СТИЛОБАТ»: одна таблица «показатель — значение» на две системы.
//
// Источник — таблица «Свойства и преимущества» карточек. Сравнение
// характеристик — факты, и модель их сочинять не имеет права: строки берутся
// дословно из карточки.

// compareSystemRows собирает строки таблицы сравнения.
//
// Порядок показателей задаёт ПЕРВАЯ карточка: у второй эти же показатели ищутся
// по названию. Так таблица читается сверху вниз по логике первого решения, а не
// по случайному порядку второй карточки.
//
// Возвращает nil, когда сравнивать нечего (одна система или нет карточек):
// таблица из одной колонки — это не сравнение, а обман читателя.
func compareSystemRows(cards []SystemCard) [][]string {
	if len(cards) < 2 {
		return nil
	}
	first := specsByName(cards[0].Specifications)
	if len(first.order) == 0 {
		return nil
	}

	header := []string{"Показатель"}
	for _, card := range cards {
		header = append(header, card.Title)
	}
	rows := [][]string{header}

	for _, name := range first.order {
		row := []string{name, first.values[name]}
		for _, card := range cards[1:] {
			row = append(row, specsByName(card.Specifications).values[name])
		}
		rows = append(rows, row)
	}
	return rows
}

// specTable — показатели карточки в порядке строк и по названию.
type specTable struct {
	order  []string
	values map[string]string
}

// specsByName разбирает таблицу характеристик карточки.
//
// Колонка «показатель» — первая, значение — последняя непустая. Между ними
// бывают «Ед. изм.» и «Метод испытаний», и брать фиксированный третий столбец
// значило бы показывать метод вместо значения. Шапка карточки («Показатель |
// Ед. изм. | Значение») пропускается: она не показатель.
func specsByName(rows [][]string) specTable {
	out := specTable{values: map[string]string{}}
	for _, row := range rows {
		if len(row) < 2 {
			continue
		}
		name := strings.TrimSpace(row[0])
		if name == "" || isSpecHeader(name) {
			continue
		}
		if _, seen := out.values[name]; !seen {
			out.order = append(out.order, name)
		}
		out.values[name] = lastNonEmpty(row[1:])
	}
	return out
}

// isSpecHeader — строка-шапка таблицы характеристик, а не показатель.
//
// «Параметр» — самая частая подпись в живых карточках (стендовая карточка 101
// пишет именно так), и без неё шапка становилась строкой сравнения: в таблице
// появлялось «Параметр | 1,5 кН/м² | ».
func isSpecHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "показатель", "показатели", "параметр", "параметры",
		"характеристика", "наименование":
		return true
	}
	return false
}

// lastNonEmpty — последняя непустая ячейка строки.
func lastNonEmpty(cells []string) string {
	for i := len(cells) - 1; i >= 0; i-- {
		if v := strings.TrimSpace(cells[i]); v != "" {
			return v
		}
	}
	return ""
}
