package main

import "strings"

// Фильтр списков КП и боевых листков (2026-11-13, шаг S2 миграции в Mini App).
//
// Решение §9.1 плана: поиск идет НА СеРВеРе, а не в приложении. Список в App
// постраничный (`LIMIT 200`), поэтому фильтрация на клиенте искала бы только
// видимую страницу и молча врала бы «ничего не найдено».
//
// Что ищется:
//   - КП — по имени заказчика (`client->>'name'`, подстрока, регистр и «е» не
//     важны): менеджер помнит заказчика, а не номер;
//   - боевой листок — по названию и по названиям сторон (`our_notes` /
//     `competitor_notes`): клиента у листка нет, продукт — единственное, что у
//     него есть.
//
// Ловушка, которую нашел живой прогон: имя заказчика почти никогда не начинается
// с запроса («СтройМонтажСервис»), а `ILIKE 'q%'` — единственная форма, которая
// ходит по индексу. Поэтому строка ищется в двух видах — «начинается с» и
// «слово начинается с» — а подстрока внутри слова проверяется по списку в
// памяти (строк не больше `LIMIT 200`, это дешевле, чем `pg_trgm`).
const listLimit = 200

// listQuery — разобранный поисковый запрос.
type listQuery struct {
	// Raw — запрос как его ввел человек (без лишних пробелов).
	Raw string
	// Tie — как записать в параметр для ILIKE: «е» заменена на «е», «%» и «_»
	// экранированы, чтобы «100%» не стал шаблоном.
	Tie string
}

// parseListQuery приводит строку поиска к виду, в котором ее можно отдать в
// SQL. Пустой запрос (или одни пробелы) — «поиска нет», прежнее поведение.
func parseListQuery(raw string) listQuery {
	normalized := joinFields(raw)
	return listQuery{
		Raw: normalized,
		Tie: likeEscape(yoTie(normalized)),
	}
}

// Empty — поиска нет.
func (q listQuery) Empty() bool {
	return q.Raw == ""
}

// PrefixPattern — шаблон для `ILIKE 'q%'`: именно эта форма ходит по
// text_pattern_ops-индексу.
func (q listQuery) PrefixPattern() string {
	return q.Tie + "%"
}

// WordPattern — шаблон для «слово начинается с запроса» (`ILIKE '% q%'`).
func (q listQuery) WordPattern() string {
	return "% " + q.Tie + "%"
}

// Contains — подстрока внутри слова: совпадение по этой форме возможно, но по
// индексу не ходит, поэтому проверяется в памяти на уже выбранных строках.
func (q listQuery) Contains() string {
	return q.Tie
}

// joinFields схлопывает пробелы и обрезает края — менеджер набирает с телефона.
func joinFields(raw string) string {
	var b []byte
	space := false
	for _, r := range raw {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\u00a0' {
			if len(b) > 0 {
				space = true
			}
			continue
		}
		if space {
			b = append(b, ' ')
			space = false
		}
		b = append(b, string(r)...)
	}
	return string(b)
}

// yoTie заменяет «е» на «е» в обоих регистрах: в базе и в запросе это одна
// буква («Федоров» находится на «Федор»).
func yoTie(s string) string {
	var out []rune
	for _, r := range s {
		switch {
		case r == 0x401:
			out = append(out, 0x435)
		case r == 0x451:
			out = append(out, 0x435)
		default:
			out = append(out, r)
		}
	}
	return string(out)
}

// likeEscape экранирует символы шаблона ILIKE. Обратная косая — умолчательный
// символ экранирования PostgreSQL; тогда ни `%`, ни `_` из запроса не станут
// шаблоном («50%» и «A_B» ищутся буквально).
func likeEscape(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\\' || r == '%' || r == '_' {
			out = append(out, '\\')
		}
		out = append(out, r)
	}
	return string(out)
}

// textHaystack — то же приведение для названия из базы: без регистра, «ё»→«е».
func textHaystack(parts ...string) string {
	joined := joinFields(yoTie(joinStrings(parts)))
	return lowerASCIIFree(joined)
}

// lowerASCIIFree приводит регистр для поиска при подстроке в памяти — это
// проверка результата SQL, а не замена SQL-условию: `lower()` в базе делает
// то же с учетом локали. Ряды символов заданы явно, как в кириллических
// регулярках сервиса: диапазоны вроде `А`…`Я` на уже испорченном файле
// незаметно перестают срабатывать, а перечень букв виден глазами.
func lowerASCIIFree(s string) string {
	var out []rune
	for _, r := range s {
		out = append(out, lowerRune(r))
	}
	return string(out)
}

// lowerRune — нижний регистр для одного символа.
//
// Явная таблица, а не `unicode.ToLower`: так правило одно для SQL и памяти и
// не зависит от версии Unicode. Граница 0x430 (заглавные ниже строчных) и
// отдельные буквы «Ё/ё» заданы числами: в прошлый раз «Ё» выпала из строки
// букв, и пары «Е/Ё» разъехались.
func lowerRune(r rune) rune {
	switch {
	case r >= 'A' && r <= 'Z':
		return r + 0x20
	case r >= 0x410 && r <= 0x44F:
		if r >= 0x430 {
			return r
		}
		lower := r + 0x20
		if lower > 0x44F {
			lower++
		}
		return lower
	case r == 0x401:
		return 0x451
	}
	return r
}

// joinStrings склеивает части непустыми через пробел.
func joinStrings(parts []string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += " "
		}
		out += p
	}
	return out
}

// containsTie — поиск по подстроке внутри слова с тем же приведением, что в SQL.
func containsTie(haystack, needle string) bool {
	q := parseListQuery(needle)
	if q.Empty() {
		return true
	}
	return hasSubstring(textHaystack(haystack), lowerASCIIFree(yoTie(q.Raw)))
}

// hasSubstring — поиск подстроки без регулярки (регистр уже приведён).
func hasSubstring(haystack, needle string) bool {
	return needle == "" || strings.Contains(haystack, needle)
}

// offerClientSearch — что сравнивается при подстрочном доборе: имя заказчика.
func offerClientSearch(offer Offer) string {
	return offer.Client.Name
}

// combatSearchText — по чему ищется листок: название и названия сторон.
func combatSearchText(sheet CombatSheet) string {
	parts := []string{sheet.Title}
	for _, ref := range sheet.OurNotes {
		parts = append(parts, ref.Title)
	}
	for _, ref := range sheet.CompetitorNotes {
		parts = append(parts, ref.Title)
	}
	return joinStrings(parts)
}
