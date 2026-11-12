package main

// Сторож порядка поиска (2026-11-13, живой дефект на стенде).
//
// Было: SQL брал свежие `listLimit` строк и только потом их фильтровал. На
// живой базе «Ромаш» встречается в 19 строках из 142, но все они глубже
// свежих двухсот — поиск честно отвечал «ничего не найдено». Теперь поиск
// просматривает `listScanLimit`, а отдает не больше `listLimit`.
//
// Здесь закреплено ровно это отношение: их нельзя «упростить» до одного числа.

import "testing"

func TestSearchScansMoreThanItReturns(t *testing.T) {
	if listScanLimit <= listLimit {
		t.Fatalf("listScanLimit (%d) должен быть заметно больше listLimit (%d): иначе "+
			"совпадения глубже свежих строк снова потеряются", listScanLimit, listLimit)
	}
}

func TestContainsPatternIsSubstringTie(t *testing.T) {
	q := parseListQuery("Ромаш")
	if got := q.ContainsPattern(); got != "%ромаш%" {
		t.Fatalf("ContainsPattern = %q, хотим «%%ромаш%%»", got)
	}
	// «е» приводится так же, как в значение из базы.
	q = parseListQuery("Федоров")
	if got := q.ContainsPattern(); got != "%федоров%" {
		t.Fatalf("ContainsPattern с «е» = %q", got)
	}
	// Символы шаблона экранируются: «100%» ищется буквально.
	q = parseListQuery("100%")
	if got := q.ContainsPattern(); got != `%100\%%` {
		t.Fatalf("ContainsPattern с «%%» = %q", got)
	}
}
