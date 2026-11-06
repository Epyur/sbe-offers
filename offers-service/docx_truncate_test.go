package main

import (
	"strings"
	"testing"
)

// Обрыв ответа модели (живой КП 003/2026, 2026-11-05).
//
// В документе остался хвост «…закрывает подземную часть одним кон» — ответ
// вышел за бюджет токенов, и печать отдала клиенту недописанное слово.
//
// 2026-11-06: лечение бюджетом ОТМЕНЕНО. Свой `max_tokens` у КП и был причиной
// обрыва: предел назначали мы, а не провайдер. Текст документа больше не
// ограничивается (Complete с нулём), поэтому осталось одно лечение — печать
// отрезает то, что всё же оборвалось. Тест бюджета переписан под это решение.

// Текст документа запрашивается БЕЗ нашего предела токенов.
//
// Проверяется поведение, а не число: запрос к модели на описание блока уходит
// с maxTokens = 0, и llm-service не кладёт `max_tokens` в тело вовсе
// (buildChatBody). Живой КП 003/2026 терял описание именно потому, что бюджет
// `blockMaxTokens` кончался раньше ответа.
func TestDocumentTextAsksWithoutTokenCap(t *testing.T) {
	if got := documentTextMaxTokens(); got != 0 {
		t.Fatalf("текст документа запрашивается с пределом %d, ожидался 0 (предел даёт провайдер)", got)
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
