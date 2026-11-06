package main

import (
	"strings"
	"testing"
)

// Обрыв ответа модели (живой КП 003/2026, 2026-11-05).
//
// В документе остался хвост «…закрывает подземную часть одним кон» — ответ
// вышел за бюджет токенов, и печать отдала клиенту недописанное слово.
// Два лечения: бюджет вырос (blockMaxTokens), а то, что всё же оборвалось,
// печать отрезает по последней законченной фразе.

// Бюджет токенов покрывает ответ на максимум слов с запасом на рассуждение.
func TestBlockTokenBudgetIsGenerous(t *testing.T) {
	// maxWords 800 — верхняя граница узла; ответ должен влезать с запасом.
	if got := blockMaxTokens(800); got < 4000 {
		t.Fatalf("бюджет %d мал для 800 слов", got)
	}
	if got := blockMaxTokens(150); got < 1200 {
		t.Fatalf("бюджет %d мал для 150 слов: ответ снова оборвётся", got)
	}
}

// Оборванный по слову текст: незаконченное слово снимается целиком.
func TestTrimTruncatedTailDropsBrokenWord(t *testing.T) {
	raw := "Комплект закрывает подземную часть одним кон"
	got := trimTruncatedTail(raw)
	if strings.HasSuffix(got, "кон") {
		t.Fatalf("оборванное слово осталось: %q", got)
	}
	if !strings.HasSuffix(got, ".") {
		t.Fatalf("текст не закончен точкой: %q", got)
	}
}

// Полный текст не трогается: точка в конце — признак законченной мысли.
func TestTrimTruncatedTailKeepsFinishedText(t *testing.T) {
	raw := "Мембрана защищает подвал от грунтовых вод."
	if got := trimTruncatedTail(raw); got != raw {
		t.Fatalf("готовый текст изменён: %q", got)
	}
}

// Несколько абзацев: режется только последний, предыдущие остаются как были.
func TestTrimTruncatedTailKeepsPreviousParagraphs(t *testing.T) {
	raw := "Первый абзац закончен полностью.\n\nВторой абзац оборван на слов"
	got := trimTruncatedTail(raw)
	if !strings.Contains(got, "Первый абзац закончен полностью.") {
		t.Fatalf("первый абзац потерян: %q", got)
	}
	if strings.Contains(got, "на слов") {
		t.Fatalf("обрыв остался: %q", got)
	}
	if !strings.HasSuffix(strings.TrimSpace(got), ".") {
		t.Fatalf("последний абзац не закрыт точкой: %q", got)
	}
}

// Ответ обрывается посреди фразы без точки — печатается до последней
// законченной фразы, а не до последнего пробела (иначе остаётся «огрызок»).
func TestTrimTruncatedTailCutsToLastSentence(t *testing.T) {
	raw := "Мембрана защищает подвал. Геотекстиль выполняет разделительную функцию и защищает"
	got := trimTruncatedTail(raw)
	if !strings.HasSuffix(strings.TrimSpace(got), "функцию.") && !strings.HasSuffix(strings.TrimSpace(got), "подвал.") {
		t.Fatalf("обрезано не по границе фразы: %q", got)
	}
	if strings.HasSuffix(strings.TrimSpace(got), "защищает") {
		t.Fatalf("оборванное предложение осталось: %q", got)
	}
}
