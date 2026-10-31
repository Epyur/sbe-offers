package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"mailers/offers/skills"
)

// Таблица менеджера (Task 4 плана 2026-10-30).
//
// Менеджер присылает Excel с ЕКН, названиями и ценами. Это ИСТОЧНИК ПРАВДЫ по
// цене для конкретного КП: один и тот же материал стоит по-разному в
// зависимости от условий поставки, и каталог такую цену знать не может.
//
// Колонки ищутся по заголовку, а не по номеру: люди не сверяются со схемой и
// ставят «Цена» куда удобно. Заголовок может быть не первой строкой — над ним
// часто пишут название документа.

// SheetRow — одна строка таблицы, как её поняли.
type SheetRow struct {
	Line        int     `json:"line"` // номер строки в файле, для показа человеку
	Name        string  `json:"name"`
	Ekn         string  `json:"ekn"`
	Unit        string  `json:"unit"`
	Qty         float64 `json:"qty"`
	Price       float64 `json:"price"`
	Sum         float64 `json:"sum"`
	HasQty      bool    `json:"hasQty"`
	HasPrice    bool    `json:"hasPrice"`
	HasSum      bool    `json:"hasSum"`
	MissingName bool    `json:"missingName"`
}

// Sheet — разобранная таблица.
type Sheet struct {
	Rows     []SheetRow `json:"rows"`
	Skipped  int        `json:"skipped"` // пустые строки — не ошибка, но пусть будет видно
	FileName string     `json:"fileName"`
}

// sheetColumn — как называется колонка в таблице и где она стоит.
type sheetColumn struct {
	name  string
	index int
}

// Признаки колонок. Сравнение свободное: заголовок «Кол-во (м2)» — это
// количество, а «Цена, руб.» — цена.
var sheetColumnAliases = []struct {
	kind    string
	aliases []string
}{
	{"name", []string{"наименование", "название", "материал", "позиция", "продукция"}},
	{"ekn", []string{"екн", "ekn"}},
	{"unit", []string{"ед", "единица", "ед.изм", "ед. изм"}},
	{"qty", []string{"кол-во", "количество", "кол"}},
	{"price", []string{"цена", "стоимость", "цена за ед"}},
	{"sum", []string{"сумма", "итого", "стоимость всего"}},
}

// parseSheetRows разбирает строки книги в таблицу.
//
// Возвращает ошибку, называющую недостающую колонку: без цены таблица
// бессмысленна (её и присылают), без наименования — нечего сопоставлять.
func parseSheetRows(rows [][]string) (Sheet, error) {
	headerIdx := findSheetHeader(rows)
	if headerIdx < 0 {
		return Sheet{}, fmt.Errorf("не нашёл строку заголовка: нужны колонки «Наименование» и «Цена»")
	}
	cols := mapSheetColumns(rows[headerIdx])
	if _, ok := cols["name"]; !ok {
		return Sheet{}, fmt.Errorf("в таблице нет колонки «Наименование» — нечего сопоставлять с материалами")
	}
	if _, ok := cols["price"]; !ok {
		return Sheet{}, fmt.Errorf("в таблице нет колонки «Цена» — без неё таблица не нужна")
	}

	sheet := Sheet{Rows: make([]SheetRow, 0, len(rows)-headerIdx)}
	for i := headerIdx + 1; i < len(rows); i++ {
		row := rows[i]
		if isBlankSheetRow(row) {
			sheet.Skipped++
			continue
		}
		parsed := SheetRow{Line: i + 1}
		parsed.Name = strings.TrimSpace(cellAt(row, cols["name"].index))
		parsed.Ekn = cleanSheetEkn(cellAt(row, cols["ekn"].index))
		parsed.Unit = strings.TrimSpace(cellAt(row, cols["unit"].index))
		if v, ok := parseSheetNumber(cellAt(row, cols["qty"].index)); ok {
			parsed.Qty, parsed.HasQty = v, true
		}
		if v, ok := parseSheetNumber(cellAt(row, cols["price"].index)); ok {
			parsed.Price, parsed.HasPrice = v, true
		}
		if v, ok := parseSheetNumber(cellAt(row, cols["sum"].index)); ok {
			parsed.Sum, parsed.HasSum = v, true
		}
		parsed.MissingName = parsed.Name == ""
		sheet.Rows = append(sheet.Rows, parsed)
	}
	return sheet, nil
}

// findSheetHeader ищет строку заголовка среди первых двадцати: выше бывает
// название документа, дата и подпись, а глубже начинаются данные.
//
// Достаточно ОДНОЙ из ключевых колонок: если в таблице нет только цены,
// сказать «нет колонки Цена» полезнее, чем «не нашёл заголовок». Поэтому
// заголовком считается строка, где есть наименование ИЛИ цена, а какая
// колонка отсутствует — выясняет вызывающий.
func findSheetHeader(rows [][]string) int {
	limit := len(rows)
	if limit > 20 {
		limit = 20
	}
	for i := 0; i < limit; i++ {
		cols := mapSheetColumns(rows[i])
		_, hasName := cols["name"]
		_, hasPrice := cols["price"]
		if hasName || hasPrice {
			return i
		}
	}
	return -1
}

// mapSheetColumns раскладывает заголовок по признакам.
func mapSheetColumns(header []string) map[string]sheetColumn {
	out := make(map[string]sheetColumn, len(header))
	for i, raw := range header {
		label := normalizeSheetLabel(raw)
		if label == "" {
			continue
		}
		for _, alias := range sheetColumnAliases {
			if _, taken := out[alias.kind]; taken {
				continue
			}
			for _, name := range alias.aliases {
				if strings.HasPrefix(label, name) {
					out[alias.kind] = sheetColumn{name: label, index: i}
					break
				}
			}
		}
	}
	return out
}

// normalizeSheetLabel приводит заголовок к виду для сравнения: нижний регистр,
// без точек в конце и без пояснений в скобках («Цена (руб.)» → «цена»).
func normalizeSheetLabel(raw string) string {
	label := strings.ToLower(strings.TrimSpace(raw))
	if idx := strings.Index(label, "("); idx > 0 {
		label = strings.TrimSpace(label[:idx])
	}
	label = strings.TrimSuffix(label, ".")
	return strings.Join(strings.Fields(label), " ")
}

// cellAt — значение ячейки или пустая строка, если колонки нет.
func cellAt(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return row[index]
}

// isBlankSheetRow — строка без единого непустого значения.
func isBlankSheetRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

// cleanSheetEkn оставляет только номер. В таблицах встречается «ЕКН 068863»
// или номер с пробелами по краям — это тот же ЕКН, а не другой.
func cleanSheetEkn(raw string) string {
	value := strings.TrimSpace(raw)
	if idx := strings.IndexFunc(value, func(r rune) bool { return unicode.IsLetter(r) }); idx >= 0 {
		prefix := value[:idx]
		if strings.TrimSpace(prefix) == "" {
			// «ЕКН» впереди: отбрасываем слово и берём номер.
			value = strings.TrimSpace(value[idx:])
			if sp := strings.IndexFunc(value, unicode.IsSpace); sp >= 0 {
				value = value[sp+1:]
			}
		}
	}
	return strings.TrimSpace(value)
}

// parseSheetNumber читает число из ячейки.
//
// Русский Excel пишет «1 500,50». Хрупкое место: Number("1 500,50") — не
// число, и без этой нормализации цена молча стала бы нулём.
func parseSheetNumber(raw string) (float64, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, false
	}
	// Пробелы (в том числе неразрывные) — разделители тысяч.
	value = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, value)
	value = strings.ReplaceAll(value, ",", ".")
	// «1.500.50» — вторую точку убираем, оставляя одну десятичную.
	if strings.Count(value, ".") > 1 {
		last := strings.LastIndex(value, ".")
		value = strings.ReplaceAll(value[:last], ".", "") + value[last:]
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

// ParseSheetXlsx читает книгу и разбирает её как таблицу менеджера.
func ParseSheetXlsx(data []byte, fileName string) (Sheet, error) {
	rows, err := skills.XlsxSheetRows(data)
	if err != nil {
		return Sheet{}, err
	}
	sheet, err := parseSheetRows(rows)
	if err != nil {
		return Sheet{}, err
	}
	sheet.FileName = fileName
	return sheet, nil
}
