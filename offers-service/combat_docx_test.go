package main

import (
	"strings"
	"testing"
)

// Боевой листок: сборка DOCX (2026-11-08, вторая партия 0.8.3).
//
// Тесты поведения: собираем документ по заданной форме и проверяем то, что
// увидит человек в файле, — наши системы, продукты конкурентов, подставленные
// поля и отсутствие служебных скобок. Внутреннее устройство рендера теста не
// касается.

// combatTestCards — наш продукт и продукт конкурента для сборки.
func combatTestCards() (our, competitor SystemCard) {
	our = SystemCard{
		NoteID:      10,
		Title:       "Наша система",
		Description: "Наше описание системы.",
		Composition: []string{"Мембрана наша"},
	}
	competitor = SystemCard{
		NoteID:      20,
		Title:       "Продукт конкурента",
		Description: "Описание конкурента.",
		Composition: []string{"Мембрана конкурента"},
	}
	return our, competitor
}

func TestRenderCombatDocxBasics(t *testing.T) {
	our, competitor := combatTestCards()
	sheet := CombatSheet{
		ID:              1,
		Number:          "003/2026",
		OwnerEmail:      "manager@tn.ru",
		Title:           "Боевой листок",
		OurNotes:        []CombatNoteRef{{NoteID: 10, Title: "Наша система"}},
		CompetitorNotes: []CombatNoteRef{{NoteID: 20, Title: "Продукт конкурента"}},
		FormValues:      map[string]string{"object_name": "ЖК Поэма", "conclusion": "Берём нашу систему"},
	}
	data, err := renderCombatDocx(nil, sheet, []SystemCard{our}, []SystemCard{competitor}, nil)
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	xml := docXMLOf(t, data)

	if strings.Contains(xml, "{{") || strings.Contains(xml, "}}") {
		t.Fatal("в документе остались незаполненные плейсхолдеры")
	}
	if !strings.Contains(xml, "Боевой листок") {
		t.Fatal("в документе нет названия боевого листка")
	}
	// Полей формы у боевого листка больше нет (2026-11-11): объекта, заказчика
	// и вывода в документе быть не должно, а перечень продуктов — должен.
	for _, gone := range []string{"ЖК Поэма", "Берём нашу систему"} {
		if strings.Contains(xml, gone) {
			t.Fatalf("в документе осталось поле формы %q", gone)
		}
	}
	if !strings.Contains(xml, "Продукты сравнения") {
		t.Fatal("нет перечня продуктов")
	}
	if !strings.Contains(xml, "Сравнение") {
		t.Fatal("нет раздела сравнения")
	}
	// Наши системы и конкуренты — обе стороны сравнения в документе.
	if !strings.Contains(xml, "Наша система") {
		t.Fatal("нет нашего продукта в документе")
	}
	if !strings.Contains(xml, "Продукт конкурента") {
		t.Fatal("нет продукта конкурента в документе")
	}
	if !strings.Contains(xml, "Описание конкурента") {
		t.Fatal("нет описания конкурента в документе")
	}
}

func TestRenderCombatDocxNoPlaceholdersFromEmptyFields(t *testing.T) {
	// Пустые поля формы не должны ронять сборку — их просто нечем подставить.
	our, competitor := combatTestCards()
	sheet := CombatSheet{Number: "001/2026", Title: "Сравнение", FormValues: map[string]string{}}
	data, err := renderCombatDocx(nil, sheet, []SystemCard{our}, []SystemCard{competitor}, nil)
	if err != nil {
		t.Fatalf("сборка упала на пустых полях: %v", err)
	}
	xml := docXMLOf(t, data)
	if strings.Contains(xml, "{{") {
		t.Fatal("плейсхолдер без значения остался в документе")
	}
}

func TestRenderCombatDocxEmptyCompetitors(t *testing.T) {
	// Конкурентов не выбрали — документ всё равно собирается.
	our, _ := combatTestCards()
	sheet := CombatSheet{Number: "002/2026", Title: "Без конкурентов"}
	data, err := renderCombatDocx(nil, sheet, []SystemCard{our}, nil, nil)
	if err != nil {
		t.Fatalf("сборка без конкурентов упала: %v", err)
	}
	xml := docXMLOf(t, data)
	if !strings.Contains(xml, "Наша система") {
		t.Fatal("наш продукт обязан остаться в документе")
	}
}
