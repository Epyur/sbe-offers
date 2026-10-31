package main

import "strings"

// Каталог бота (Task 8, 2026-10-31).

// materialsForBot оставляет в списке только материалы и работы.
func materialsForBot(items []Material) []Material {
	out := make([]Material, 0, len(items))
	for _, m := range items {
		if materialKindAllowed(m.Kind) {
			out = append(out, m)
		}
	}
	return out
}

// materialMatches — тот же поиск, что у каталога, но по названию, SKU и ЕКН:
// в чат пишут то, что видят на бумаге, а там чаще номер ЕКН, чем полное имя.
func materialMatches(m Material, query string) bool {
	words := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	if len(words) == 0 {
		return true
	}
	haystack := strings.ToLower(m.Name + " " + m.SKU)
	for _, e := range m.Ekn {
		haystack += " " + strings.ToLower(e.Ekn)
	}
	for _, word := range words {
		if !strings.Contains(haystack, word) {
			return false
		}
	}
	return true
}

// filterMaterialsForBot — поиск по отобранным материалам.
func filterMaterialsForBot(items []Material, query string) []Material {
	kept := materialsForBot(items)
	out := make([]Material, 0, len(kept))
	for _, m := range kept {
		if materialMatches(m, query) {
			out = append(out, m)
		}
	}
	return out
}
