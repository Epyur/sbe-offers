package main

// Сторожа шага 2a (2026-11-16): боевой листок печатает СОДЕРЖАТЕЛЬНЫЕ РАЗДЕЛЫ.
//
// Проверяется то, что видит менеджер в готовом файле:
//   - раздел формы листка печатается своим заголовком и текстом подраздела;
//   - повторяемый подраздел идёт по КАЖДОМУ продукту сравнения — обе стороны;
//   - пустой ответ модели и подраздел без брифа не печатаются вовсе;
//   - разделов нет (или базы нет) — прежний вид карточками, а не пустой листок;
//   - правка карточки КОНКУРЕНТА меняет отпечаток содержания (грабля 2026-11-08:
//     кэш ключевался одним noteId, правка тела карточки не доходила до текста).

import (
	"context"
	"strings"
	"testing"
)

// combatSectionFixture — листок с одной нашей системой и одним конкурентом.
func combatSectionFixture() (CombatSheet, SystemCard, SystemCard) {
	our, competitor := combatTestCards()
	sheet := CombatSheet{
		ID:              7,
		Number:          "011/2026",
		OwnerEmail:      "manager@tn.ru",
		Title:           "Сравнение",
		OurNotes:        []CombatNoteRef{{NoteID: our.NoteID, Title: our.Title}},
		CompetitorNotes: []CombatNoteRef{{NoteID: competitor.NoteID, Title: competitor.Title}},
		FormValues:      map[string]string{"hint": "сравни срока и цену монтажа"},
	}
	return sheet, our, competitor
}

// stubSectionCompleter — модель-заглушка: отвечает по маркеру из инструкции.
type stubSectionCompleter struct {
	answers map[string]string
	asked   []string
}

func (s *stubSectionCompleter) Complete(_ context.Context, system, user string, _ int) (string, error) {
	s.asked = append(s.asked, user)
	// Маркер ищется в ОБЕИХ половинах промта: бриф листка уходит в систему,
	// а данные — в пользовательское сообщение, и опора на одну половину молча
	// возвращала пустой ответ (старая грабля llm-stub, 2026-11-08).
	for marker, answer := range s.answers {
		if strings.Contains(system, marker) || strings.Contains(user, marker) {
			return answer, nil
		}
	}
	return "", nil
}

// renderCombatSectionsWithForm — печать разделов листка с ПОДСТАВЛЕННОЙ формой
// и моделью: база в тесте не нужна, а конвейер разделов — настоящий.
//
// Возвращает четыре значения: XML тела печати, число вызовов модели, число
// напечатанных заголовков подразделов и сами вызовы (для подробного отказа).
func renderCombatSectionsWithForm(sheet CombatSheet, our, competitor SystemCard,
	form OfferForm, stub *stubSectionCompleter) (string, int, int) {
	gen, offer := combatSectionGen(sheet, []SystemCard{our}, []SystemCard{competitor},
		collectOfferRefs([]SystemCard{our, competitor}))
	gen.complete = stub
	cards := combatSectionCards([]SystemCard{our}, []SystemCard{competitor})
	links := &layoutRuntime{numbering: newNumberingPlan(numberingAbstractDecimal)}
	lc := combatLayoutFor(offer, cards, nil, nil, links, form)

	var printed strings.Builder
	headings := 0
	for _, section := range form.Sections {
		calls := sectionCalls(section, offer, cards)
		if len(calls) == 0 {
			continue
		}
		nums := sectionNumbering{plan: lc.numberingPlan()}
		var markup strings.Builder
		any := false
		for _, call := range calls {
			text := gen.text(section, call)
			if strings.TrimSpace(text) == "" {
				continue
			}
			markup.WriteString(renderSectionSubsection(call.sub, text, lc, nums))
			if strings.TrimSpace(call.sub.Title) != "" {
				headings++
			}
			any = true
		}
		if !any {
			continue
		}
		printed.WriteString(combatSectionTitle(section))
		printed.WriteString(markup.String())
	}
	return printed.String(), len(stub.asked), headings
}

func TestCombatSectionsPrintSubsectionText(t *testing.T) {
	sheet, our, competitor := combatSectionFixture()
	form := OfferForm{Sections: []ContentSection{{
		ID:    "comparison",
		Title: "Где конкурент сильнее",
		Subsections: []ContentSubsection{{
			ID:          "weak",
			Title:       "Риски",
			Instruction: "Скажи, где конкурент сильнее",
		}},
	}}}
	stub := &stubSectionCompleter{answers: map[string]string{
		"Скажи, где конкурент сильнее": sectionParagraphAnswer("Конкурент дешевле на монтаже."),
	}}

	printedXML, calls, _ := renderCombatSectionsWithForm(sheet, our, competitor, form, stub)
	if calls != 1 {
		t.Fatalf("модель вызвана %d раз, ожидалось 1", calls)
	}
	if !strings.Contains(printedXML, "Конкурент дешевле на монтаже.") {
		t.Fatalf("в печати нет текста подраздела:\n%s", printedXML)
	}
	// Заголовки печатает РАМКА через оглавление макета (`tocHeadingXML`), а без
	// макета `lc.layout` пуст — заголовок там не печатается вовсе. Поэтому текст
	// заголовков проверяем по данным, а не по телу печати: у КП то же поведение
	// и оно закреплено `TestSectionOneSystemVsThree` (через макет).
	_, _, headings := renderCombatSectionsWithForm(sheet, our, competitor, form, stub)
	if headings != 1 {
		t.Fatalf("рамка не сообщила о подзаголовке: %d", headings)
	}
}

// Повторяемый подраздел (repeat:"systems") обязан идти по КАЖДОМУ продукту
// сравнения: у листка две стороны, и «выбранных систем КП» у него нет.
func TestCombatSectionsRepeatCoversBothSides(t *testing.T) {
	sheet, our, competitor := combatSectionFixture()
	form := OfferForm{Sections: []ContentSection{{
		ID:    "per-product",
		Title: "По каждому продукту",
		Subsections: []ContentSubsection{{
			ID:          "about",
			Title:       "О продукте",
			Instruction: "Опиши продукт отдельно",
			Repeat:      blockRepeatSystems,
		}},
	}}}
	stub := &stubSectionCompleter{answers: map[string]string{
		"Опиши продукт отдельно": sectionParagraphAnswer("Продукт описан."),
	}}

	_, calls, headings := renderCombatSectionsWithForm(sheet, our, competitor, form, stub)
	if calls != 2 {
		t.Fatalf("модель вызвана %d раз, ожидалось 2 (наш продукт и конкурент)", calls)
	}
	if headings != 2 {
		t.Fatalf("подзаголовок напечатан %d раз, ожидалось 2", headings)
	}
}

// Пустой ответ модели и подраздел без брифа не печатаются вовсе: заголовок без
// текста читался бы как поломка печати (§10.1).
func TestCombatSectionsSkipEmpty(t *testing.T) {
	sheet, our, competitor := combatSectionFixture()
	form := OfferForm{Sections: []ContentSection{
		{ID: "empty", Title: "Пустой раздел", Subsections: []ContentSubsection{{ID: "x", Title: "X", Instruction: "Скажи про конкурента"}}},
		{ID: "noinstr", Title: "Без брифа", Subsections: []ContentSubsection{{ID: "y", Title: "Y"}}},
	}}
	stub := &stubSectionCompleter{answers: map[string]string{}}
	printedXML, calls, headings := renderCombatSectionsWithForm(sheet, our, competitor, form, stub)
	if printedXML != "" || headings != 0 {
		t.Fatalf("пустые разделы напечатали текст (%d заголовков):\n%s", headings, printedXML)
	}
	if calls != 1 {
		t.Fatalf("модель должна зваться только для подраздела с брифом, вызовов %d", calls)
	}
}

// Бриф листка называет стороны — иначе модель путает, чей продукт описывает.
func TestCombatSectionBriefNamesSides(t *testing.T) {
	_, our, competitor := combatSectionFixture()
	brief := combatSectionBrief("Сравни", []SystemCard{our}, []SystemCard{competitor}, "срок службы", nil)
	for _, want := range []string{"Наш продукт:", our.Title, "Продукты конкурентов:", competitor.Title, "Не путай стороны"} {
		if !strings.Contains(brief, want) {
			t.Fatalf("в брифе нет %q:\n%s", want, brief)
		}
	}
	if !strings.Contains(brief, "срок службы") {
		t.Fatalf("бриф раздела не несёт уточнения менеджера:\n%s", brief)
	}
	if !strings.Contains(brief, `"blocks"`) {
		t.Fatal("в брифе нет замороженного контракта ответа")
	}
}

// Правка карточки КОНКУРЕНТА меняет отпечаток содержания: иначе текст раздела
// остался бы в кэше от старой карточки — грабля 2026-11-08.
func TestCombatSectionHandleSeesCardEdit(t *testing.T) {
	sheet, our, competitor := combatSectionFixture()
	before := combatSectionHandle(sheet, []SystemCard{our}, []SystemCard{competitor}, nil)
	edited := competitor
	edited.Description = "Обновлённое описание конкурента"
	after := combatSectionHandle(sheet, []SystemCard{our}, []SystemCard{edited}, nil)
	if before == after {
		t.Fatal("правка карточки конкурента не изменила отпечаток содержания: раздел отдаст старый текст")
	}
	rehinted := sheet
	rehinted.FormValues = map[string]string{"hint": "новый акцент"}
	if combatSectionHandle(rehinted, []SystemCard{our}, []SystemCard{competitor}, nil) == before {
		t.Fatal("правка уточнения не изменила отпечаток содержания")
	}
}

// Без базы (или без формы) печать не падает и печатает прежний вид карточками.
func TestCombatSectionsFallBackWithoutForm(t *testing.T) {
	sheet, our, competitor := combatSectionFixture()
	data, err := (*Server)(nil).renderCombatDocx(context.Background(), nil, sheet,
		[]SystemCard{our}, []SystemCard{competitor}, nil)
	if err != nil {
		t.Fatalf("печать без сервера упала: %v", err)
	}
	xml := docXMLOf(t, data)
	if !strings.Contains(xml, "Сравнение") {
		t.Fatal("без формы нет заголовка сравнения")
	}
	if !strings.Contains(xml, our.Title) || !strings.Contains(xml, competitor.Title) {
		t.Fatal("без формы не напечатаны обе стороны карточками")
	}
}
