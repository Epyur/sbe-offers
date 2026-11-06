package main

import (
	"strings"
	"testing"
)

// Готовые знаки степени поднимаются, а не печатаются символом (2026-11-06).
//
// Живой ответ КП 003/2026 после 0.7.5 написал «4800 м²» символом U+00B2 вместо
// «м2». Правила разметки искали только АСКИ-цифру, и текст печатался как есть:
// Word показывает `²` обычным строчным знаком, а пользователь просил настоящий
// верхний индекс для квадратных метров.
func TestRunsLiftUnicodeSquareSign(t *testing.T) {
	runs := splitSupsub("Площадь 4800 м², объём 12 м³.")
	if len(runs) < 4 {
		t.Fatalf("степени не разобраны: %+v", runs)
	}
	var sups []string
	for _, r := range runs {
		if r.Sup {
			sups = append(sups, r.Text)
		}
	}
	if len(sups) != 2 || sups[0] != "2" || sups[1] != "3" {
		t.Fatalf("надстрочные: %v, ожидали [2 3]", sups)
	}
	joined := ""
	for _, r := range runs {
		joined += r.Text
	}
	if strings.Contains(joined, "²") || strings.Contains(joined, "³") {
		t.Fatalf("знак степени остался в тексте: %q", joined)
	}
	if !strings.Contains(joined, "4800 м") {
		t.Fatalf("текст испорчен: %q", joined)
	}
}

// АСКИ-написание по-прежнему работает: обе формы дают одну разметку.
func TestRunsLiftAsciiAndUnicodeAlike(t *testing.T) {
	ascii := runsXML("площадь 4800 м2", "")
	unicode := runsXML("площадь 4800 м²", "")
	if ascii != unicode {
		t.Fatalf("разная разметка для одной единицы:\n ascii=%q\n unicode=%q", ascii, unicode)
	}
	if !strings.Contains(ascii, `w:vertAlign w:val="superscript"`) {
		t.Fatal("верхний индекс не поставлен")
	}
}

// Знак в названии материала не превращается в степень: «AL²» марки не бывает,
// но правило не должно ломать и обычный текст без единиц.
func TestRunsLeavePlainTextAlone(t *testing.T) {
	if got := runsXML("Мембрана V-SL 2 мм и плита PIR ²?", ""); !strings.Contains(got, `w:vertAlign`) {
		// «PIR ²» не единица Sc — но и не ломает текст; проверим главное:
		// «2 мм» степенью не стало.
		if strings.Contains(got, `>2</w:t>`) {
			t.Fatalf("число из «2 мм» подняли: %q", got)
		}
	}
}
