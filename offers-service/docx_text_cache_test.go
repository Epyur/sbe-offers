package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Кэш текстов блоков при ОБРЫВЕ генерации (2026-11-14, живое КП 031/2026).
//
// Что случилось: сборка документа оборвалась по таймауту — тексты первой
// системы успели написаться, вторая упала с `context deadline exceeded`.
// В базе остались ТЕКСТЫ без признака версии фактов (`gen.section` пуст), и
// каждая следующая сборка считала кэш недействительным: тексты писались
// заново, «Отправить» идёт минутами.
//
// Правило, которое здесь закреплено: признак версии ставится ПОСЛЕ обхода
// узлов, но и при обрыве части блоков — иначе готовые тексты не находятся.

// formTwoSystems — форма с блоком, который печатается по каждой системе.
func formTwoSystems() OfferForm {
	return OfferForm{
		Mode:  "client",
		Title: "КП",
		Blocks: []Block{{
			ID:           "blk_desc",
			Title:        "Описание системы",
			ShowInClient: true,
			Repeat:       blockRepeatSystems,
			Content:      []byte(`[{"type":"prompt_text","instruction":"Опиши систему для заказчика"}]`),
		}},
	}
}

// offerTwoSystems — КП с двумя системами (как 031/2026: 6885 и 6760).
func offerTwoSystems() Offer {
	o := testOfferForBlocks()
	o.Systems = []OfferSystem{{NoteID: 6885}, {NoteID: 6760}}
	return o
}

// sysCardsOf — карточки систем этого КП (не путать с сервисной cardsForOffer).
func sysCardsOf(o Offer) []SystemCard {
	cards := make([]SystemCard, 0, len(o.Systems))
	for _, s := range o.Systems {
		cards = append(cards, SystemCard{NoteID: s.NoteID, Title: "Система " + itoa(int(s.NoteID))})
	}
	return cards
}

// Первый блок написан, второй упал по отмене контекста — так и было на бою.
// Признак версии ОБЯЗАН остаться: иначе следующий сбор перепишет готовое.
func TestBlockTextsKeepSectionHashAfterPartialFailure(t *testing.T) {
	offer := offerTwoSystems()
	calls := 0
	c := &fakeCompleter{}
	// fakeCompleter всегда отдаёт один ответ; для «второй падает» нужен свой
	// проход, поэтому подменяем ошибку по счётчику.
	counting := &sequenceCompleter{
		answers: []seqAnswer{{text: "Текст первой системы"}, {err: errors.New("context deadline exceeded")}},
		onCall:  func() { calls++ },
	}

	values, reused, written, failed := generateBlockTexts(
		context.Background(), formTwoSystems(), offer, counting, sysCardsOf(offer), nil)

	if written == 0 {
		t.Fatalf("ожидали написанный текст первой системы: reused=%d written=%d failed=%d", reused, written, failed)
	}
	if failed == 0 {
		t.Fatalf("вторая система должна была упасть: reused=%d written=%d failed=%d", reused, written, failed)
	}
	if strings.TrimSpace(values[genKeySection]) == "" {
		t.Fatal("gen.section пуст после обрыва: следующий сбор не узнает, что тексты уже написаны")
	}
	_ = c
	_ = calls
}

// Второй сбор по тем же фактам не зовёт модель за текстом, который уже есть.
func TestBlockTextsDoNotRegenerateAfterFailure(t *testing.T) {
	offer := offerTwoSystems()
	counting := &sequenceCompleter{
		answers: []seqAnswer{{text: "Текст первой системы"}, {err: errors.New("context deadline exceeded")}},
	}
	values, _, _, _ := generateBlockTexts(
		context.Background(), formTwoSystems(), offer, counting, sysCardsOf(offer), nil)

	// Второй сбор: модель отвечает всем (первой системы текст уже в кэше).
	second := &sequenceCompleter{answers: []seqAnswer{{text: "Новый текст"}}}
	offer.FormValues = values
	got, reused, written, _ := generateBlockTexts(
		context.Background(), formTwoSystems(), offer, second, sysCardsOf(offer), nil)

	if reused == 0 {
		t.Fatalf("текст первой системы не взят из кэша: reused=%d written=%d", reused, written)
	}
	// И виновата не пустая карта: текст первой системы на месте.
	if strings.TrimSpace(got[genKeyTextForSystem("blk_desc", 0, "6885", "6885+6760")]) == "" {
		t.Fatal("текст первой системы потерян")
	}
}

// seqAnswer — ответ модели в очереди: текст или ошибка.
type seqAnswer struct {
	text string
	err  error
}

// sequenceCompleter отдаёт ответы по очереди; когда очередь кончилась —
// повторяет последний.
type sequenceCompleter struct {
	answers []seqAnswer
	calls   int
	onCall  func()
}

func (s *sequenceCompleter) Complete(_ context.Context, _, _ string, _ int) (string, error) {
	if s.onCall != nil {
		s.onCall()
	}
	idx := s.calls
	s.calls++
	if idx >= len(s.answers) {
		idx = len(s.answers) - 1
	}
	if idx < 0 {
		return "", nil
	}
	a := s.answers[idx]
	return a.text, a.err
}
