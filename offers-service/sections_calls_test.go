package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Веер вызовов раздела (2026-11-14, шаг 4).
//
// Модель не зовётся в этих тестах: `write` — подмена, как у
// writeBlocksInParallel. Проверяется форма работы, а не ответ модели:
//
//   - подразделы идут ПАРАЛЛЕЛЬНО, а не по очереди (иначе раздел из пяти
//     подразделов — это пять ожиданий подряд, по десяткам секунд каждое);
//   - одновременно в провайдере не больше sectionGenConcurrency вызовов
//     (иначе три системы × четыре подраздела заливают его);
//   - ответ одного вызова не отменяет остальные;
//   - подраздел с невыполненным условием не попадает даже в список вызовов.

// sectionUnderTest — раздел с несколькими подразделами.
func sectionUnderTest(count int) ContentSection {
	section := ContentSection{ID: "proposal", Title: "Описание систем"}
	for i := 0; i < count; i++ {
		section.Subsections = append(section.Subsections, ContentSubsection{
			ID:          fmt.Sprintf("sub%d", i),
			Title:       fmt.Sprintf("Подраздел %d", i),
			Instruction: fmt.Sprintf("Напиши часть %d", i),
			Condition:   conditionAlways,
		})
	}
	return section
}

// TestSectionSubsectionsRunConcurrently — N вызовов идут одновременно.
//
// Каждый вызов ждёт, пока в пуле соберётся не меньше двух: при последовательной
// работе такой тест повис бы, поэтому сверху стоит общий предел времени.
func TestSectionSubsectionsRunConcurrently(t *testing.T) {
	calls := sectionCalls(sectionUnderTest(4), Offer{}, nil)
	if len(calls) != 4 {
		t.Fatalf("вызовов %d, ожидалось 4", len(calls))
	}
	var started int32
	meet := make(chan struct{})
	finished := make(chan struct{})
	var texts []string
	go func() {
		defer close(finished)
		texts = runSectionCalls(context.Background(), calls, func(_ context.Context, call sectionCall) string {
			if atomic.AddInt32(&started, 1) == 2 {
				close(meet)
			}
			select {
			case <-meet:
			case <-time.After(3 * time.Second):
				return "один — не параллельно"
			}
			return "текст " + call.sub.ID
		})
	}()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("вызовы не выполнились: веер ждёт второго вызова, а его нет")
	}
	for i, text := range texts {
		if text != "текст sub"+itoa(i) {
			t.Fatalf("ответ %d = %q: веер не параллелен или потерян порядок подразделов", i, text)
		}
	}
}

// TestSectionConcurrencyIsBounded — одновременно не больше
// sectionGenConcurrency вызовов.
func TestSectionConcurrencyIsBounded(t *testing.T) {
	// Двенадцать подразделов: при пределе 4 пик должен быть ровно 4.
	calls := sectionCalls(sectionUnderTest(12), Offer{}, nil)
	var inFlight, peak int32
	var mu sync.Mutex
	runSectionCalls(context.Background(), calls, func(_ context.Context, _ sectionCall) string {
		now := atomic.AddInt32(&inFlight, 1)
		mu.Lock()
		if now > peak {
			peak = now
		}
		mu.Unlock()
		// Даём остальным шанс стартовать: без паузы пул успевает прокрутить
		// вызовы почти по одному, и пик был бы 1 при любом пределе.
		time.Sleep(30 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return "ок"
	})
	if peak > sectionGenConcurrency {
		t.Fatalf("одновременных вызовов %d, предел %d", peak, sectionGenConcurrency)
	}
	if peak < 2 {
		t.Fatalf("одновременных вызовов %d: вызовы не параллельны вовсе", peak)
	}
}

// TestSectionPartialFailureKeepsFrame — отказ одного вызова не теряет остальные.
func TestSectionPartialFailureKeepsFrame(t *testing.T) {
	lc := serviceLayoutContext(nil)
	lc.form = func() OfferForm {
		return OfferForm{Sections: []ContentSection{sectionUnderTest(3)}}
	}
	lc.sectionText = func(_ ContentSection, call sectionCall) string {
		if call.sub.ID == "sub1" {
			return "" // модель не ответила
		}
		// Ответ модели — JSON замороженного вида: он печатается как есть.
		return sectionParagraphAnswer("текст " + call.sub.ID)
	}
	doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>{ content: proposal }</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	for _, want := range []string{"текст sub0", "текст sub2"} {
		if !strings.Contains(body, want) {
			t.Fatalf("ответ уцелевшего вызова потерян: %q\n%s", want, body)
		}
	}
	if strings.Contains(body, "текст sub1") {
		t.Fatalf("отказ модели напечатан текстом:\n%s", body)
	}
}

// TestSectionSkippedBeforeScheduling — пропуск делается ДО веера.
//
// Пропущенный подраздел не должен ни звать модель, ни занимать место в пуле:
// иначе раздел с выключенной критикой всё равно тратит вызов на провайдере.
func TestSectionSkippedBeforeScheduling(t *testing.T) {
	section := ContentSection{
		ID: "proposal",
		Subsections: []ContentSubsection{
			{ID: "intro", Title: "Введение", Instruction: "Вступление", Condition: conditionAlways},
			{ID: "rebut", Title: "Критика", Instruction: "Опровергни", Condition: conditionHasCritique},
			{ID: "why", Title: "Почему эти", Instruction: "Обоснуй", Condition: conditionMultiSystem},
		},
	}
	oneSystem := Offer{Systems: RefList{{NoteID: 1}}}
	calls := sectionCalls(section, oneSystem, nil)
	if len(calls) != 1 || calls[0].sub.ID != "intro" {
		t.Fatalf("вызовы: %+v, ожидался только intro", calls)
	}
	// Ни один подраздел не проходит — раздела нет целиком, без заголовка.
	none := ContentSection{ID: "none", Subsections: []ContentSubsection{
		{ID: "why", Title: "Почему", Instruction: "Обоснуй", Condition: conditionMultiSystem},
	}}
	if got := sectionCalls(none, oneSystem, nil); len(got) != 0 {
		t.Fatalf("пропущенные подразделы попали в веер: %+v", got)
	}
}

// TestSectionRepeatSystemsPerCall — repeat:"systems" даёт вызов на каждую систему.
func TestSectionRepeatSystemsPerCall(t *testing.T) {
	section := ContentSection{ID: "proposal", Subsections: []ContentSubsection{
		{ID: "each", Title: "Описание системы", Instruction: "Опиши", Condition: conditionHasSystems, Repeat: blockRepeatSystems},
	}}
	offer := Offer{Systems: RefList{{NoteID: 1}, {NoteID: 2}}}
	cards := []SystemCard{{NoteID: 1, Title: "Первая"}, {NoteID: 2, Title: "Вторая"}}
	calls := sectionCalls(section, offer, cards)
	if len(calls) != 2 {
		t.Fatalf("вызовов %d, ожидалось 2 (по системе)", len(calls))
	}
	for i, call := range calls {
		if call.system == nil {
			t.Fatalf("вызов %d без карточки системы", i)
		}
		if want := []int64{1, 2}[i]; call.system.NoteID != want {
			t.Fatalf("вызов %d: система %d, ожидалась %d", i, call.system.NoteID, want)
		}
	}
	// Без выбранных систем вызовов нет: подраздел про системы не печатается.
	if got := sectionCalls(section, Offer{}, nil); len(got) != 0 {
		t.Fatalf("при пустом выборе вызовов %d", len(got))
	}
}
