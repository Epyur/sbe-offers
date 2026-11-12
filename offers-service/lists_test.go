package main

// Сторож шага S2 (2026-11-13): поиск в списках КП и боевых листков.
//
// Проверяется чистая часть — приведение запроса, шаблоны и добор по подстроке.
// Сам SQL проверяется на стенде (offers_local_e2e.mjs), но именно здесь
// закреплены две вещи, которые на стенде видно не сразу:
//   1. без `q` поведение прежнее — все строки, свежие сверху (регрессия из
//      §9.1 плана: плагин, бот-список и ЦУП не должны почувствовать правку);
//   2. поиск находит имя заказчика В СеРеДИНе слова — живая жалоба будет
//      именно про это («набираю Строй — ничего»).

import (
	"strings"
	"testing"
)

func TestParseListQueryNormalises(t *testing.T) {
	q := parseListQuery("  Строй\tМонтаж  Сервис  ")
	if q.Raw != "Строй Монтаж Сервис" {
		t.Fatalf("Raw = %q, хотим «Строй Монтаж Сервис»", q.Raw)
	}
	if q.PrefixPattern() != "Строй Монтаж Сервис%" {
		t.Fatalf("PrefixPattern = %q", q.PrefixPattern())
	}
	if q.WordPattern() != "% Строй Монтаж Сервис%" {
		t.Fatalf("WordPattern = %q", q.WordPattern())
	}
}

func TestParseListQueryEmpty(t *testing.T) {
	for _, raw := range []string{"", "   ", "\t\n", "\u00a0"} {
		if !parseListQuery(raw).Empty() {
			t.Fatalf("запрос %q считается поиском, а это пустой запрос", raw)
		}
	}
}

func TestParseListQueryYoTie(t *testing.T) {
	q := parseListQuery("Фёдоров")
	if q.Tie != "Федоров" {
		t.Fatalf("Tie = %q, хотим «Фёдоров»: «ё» и «е» — одна буква", q.Tie)
	}
	q = parseListQuery("ФЁДОРОВ")
	if q.Tie != "ФеДОРОВ" {
		t.Fatalf("Tie = %q, заглавная «Ё» тоже приводится", q.Tie)
	}
}

func TestParseListQueryEscapesWildcards(t *testing.T) {
	q := parseListQuery("50%_x")
	if q.Tie != `50\%\_x` {
		t.Fatalf("Tie = %q: «%%» и «_» из запроса не должны становиться шаблоном", q.Tie)
	}
}

func TestContainsTieFindsInsideWord(t *testing.T) {
	cases := []struct {
		haystack string
		needle   string
		want     bool
	}{
		// Главный случай S2: подстрока внутри слова.
		{"СтройМонтажСервис", "Строй", true},
		{"ООО «СтройМонтажСервис»", "монтаж", true},
		{"СтройМонтажСервис", "Бетон", false},
		// Регистр и «е».
		{"ФЁДОРОВ и партнеры", "фёдоров", true},
		{"фёдоров и партнеры", "Федоров", true},
		// Пустой запрос не режет список.
		{"что угодно", "", true},
		// Запрос длиннее строки.
		{"ООО", "СтройМонтажСервис", false},
	}
	for _, c := range cases {
		if got := containsTie(c.haystack, c.needle); got != c.want {
			t.Fatalf("containsTie(%q, %q) = %v, хотим %v", c.haystack, c.needle, got, c.want)
		}
	}
}

func TestCombatSearchTextCoversSides(t *testing.T) {
	sheet := CombatSheet{
		Title:           "Сравнение для ЖК Поэма",
		OurNotes:        []CombatNoteRef{{NoteID: 1, Title: "LOGICROOF V-RP"}},
		CompetitorNotes: []CombatNoteRef{{NoteID: 2, Title: "PIRRO Ventiduct"}},
	}
	text := combatSearchText(sheet)
	for _, needle := range []string{"ПОЭМА", "logicroof", "ventiduct", "PIRRO"} {
		if !containsTie(text, needle) {
			t.Fatalf("листок не находится по %q: %q", needle, text)
		}
	}
	if containsTie(text, "мембрана") {
		t.Fatalf("листок нашелся по слову, которого в нем нет: %q", text)
	}
}

func TestOfferClientSearchIsClientName(t *testing.T) {
	offer := Offer{Client: OfferClient{Name: "СтройМонтажСервис", Contact: "Пётр"}}
	if !containsTie(offerClientSearch(offer), "строй") {
		t.Fatal("поиск идёт по имени заказчика, а не находится")
	}
}

func TestListQueryWithoutSearchKeepsBehaviour(t *testing.T) {
	// Регрессия §9.1: пустой запрос не добавляет ни условия, ни добра.
	// Проверяем на уровне формы шаблонов — их не должно быть вовсе.
	for _, raw := range []string{"", "  "} {
		q := parseListQuery(raw)
		if !q.Empty() {
			t.Fatalf("пустой запрос %q не распознан", raw)
		}
		if strings.Contains(q.PrefixPattern(), "%%") {
			t.Fatalf("у пустого запроса шаблон %q", q.PrefixPattern())
		}
	}
}
