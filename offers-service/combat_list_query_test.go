package main

// Чтение листка по номеру и порядок списка бота (2026-11-20).
//
// Живая жалоба: «нажимаю DOCX — приходит файл самого первого листка». Причина
// была в том, что кнопка не знала своего листка, а бот брал последний элемент
// списка — при `ORDER BY created_at DESC` это самый СТАРЫЙ листок базы. Здесь
// сторожатся два решения сервера, на которых держится лечение:
//
//  1. у списка есть тай-брейкер `id DESC` — иначе «самый свежий» не определён;
//  2. один листок читается по номеру, без сканирования «50 самых новых».

import (
	"strings"
	"testing"
)

func TestCombatListQueryOrdersNewestFirst(t *testing.T) {
	if !strings.Contains(combatListSelect, "ORDER BY created_at DESC, id DESC") {
		t.Fatalf("список листков должен идти от новых к старым и иметь тай-брейкер по id, получено: %q", combatListSelect)
	}
	if !strings.Contains(combatListSelect, "LIMIT 50") {
		t.Fatalf("у списка должен остаться предел 50, получено: %q", combatListSelect)
	}
}

func TestCombatOneQueryReadsOnlyThatSheet(t *testing.T) {
	if !strings.Contains(combatOneSelect, "WHERE id = $1") {
		t.Fatalf("чтение одного листка обязано фильтровать по id, получено: %q", combatOneSelect)
	}
	if strings.Contains(combatOneSelect, "LIMIT") {
		t.Fatalf("одному листку предел не нужен: %q", combatOneSelect)
	}
}

// `?id=` — опция, а не режим: мусор и неположительные числа означают «весь
// список», а не ошибку. Тот же приём, что `?fields=preview`.
func TestCombatListID(t *testing.T) {
	cases := map[string]int64{
		"":     0,
		"   ":  0,
		"0":    0,
		"-3":   0,
		"abc":  0,
		"7":    7,
		" 12 ": 12,
	}
	for raw, want := range cases {
		if got := combatListID(raw); got != want {
			t.Fatalf("?id=%q → %d, ждали %d", raw, got, want)
		}
	}
}
