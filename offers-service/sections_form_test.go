package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// Настройка содержательного раздела (2026-11-14): форма хранит её и отдаёт
// обратно без потерь.
//
// Почему это отдельный тест от проверки формы: человек настроил раздел один
// раз, а печатается он каждую сборку. Пропавшее по дороге поле (условие,
// повтор) не видно в интерфейсе — видно только в готовом документе, и искать
// там причину дорого.
//
// Поле `sections` проверяется на уровне JSON, а не только на уровне структур:
// сервер его сохраняет как есть, и расхождение тегов (`sections` против
// `sections`) потеряло бы настройку целиком.

func sampleSections() []ContentSection {
	return []ContentSection{{
		ID:       "proposal_systems",
		Title:    "Описание предлагаемых систем",
		Numbered: true,
		Critique: true,
		Subsections: []ContentSubsection{
			{ID: "intro", Title: "Введение", Instruction: "Напиши вступление", Condition: conditionAlways},
			{ID: "each", Title: "Описание системы", Instruction: "Опиши систему", Condition: conditionHasSystems, Repeat: blockRepeatSystems, MaxWords: 250, Level: 2},
			{ID: "why", Title: "Почему эти", Instruction: "Обоснуй выбор", Condition: conditionMultiSystem},
			{ID: "rebut", Title: "Критика альтернатив", Instruction: "Опровергни альтернативы", Condition: conditionHasCritique},
			{ID: "price", Title: "Коммерческое предложение", Instruction: "Опиши условия", Condition: conditionHasItems},
		},
	}}
}

// TestSectionFormRoundTrip — разделы переживают сохранение и чтение формы.
func TestSectionFormRoundTrip(t *testing.T) {
	form := defaultForm(clientFormMode)
	form.Sections = sampleSections()
	raw, err := json.Marshal(form)
	if err != nil {
		t.Fatalf("сохранение: %v", err)
	}
	if !strings.Contains(string(raw), `"sections"`) {
		t.Fatalf("поле sections не записано: %s", raw)
	}
	var back OfferForm
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("чтение: %v", err)
	}
	if msg := validateForm(back); msg != "" {
		t.Fatalf("сохранённая форма не проходит проверку: %s", msg)
	}
	if len(back.Sections) != 1 {
		t.Fatalf("разделов %d, ожидался один", len(back.Sections))
	}
	got := back.Sections[0]
	want := sampleSections()[0]
	if got.ID != want.ID || got.Title != want.Title || got.Numbered != want.Numbered || got.Critique != want.Critique {
		t.Fatalf("раздел изменился: %+v", got)
	}
	if len(got.Subsections) != len(want.Subsections) {
		t.Fatalf("подразделов %d, ожидалось %d", len(got.Subsections), len(want.Subsections))
	}
	for i := range want.Subsections {
		if got.Subsections[i] != want.Subsections[i] {
			t.Errorf("подраздел %d изменился:\n есть %+v\n надо %+v", i, got.Subsections[i], want.Subsections[i])
		}
	}
}

// TestSectionFormWithoutSectionsIsUnchanged — старая форма без разделов
// читается и действительна: у большинства форм разделов ещё нет.
func TestSectionFormWithoutSectionsIsUnchanged(t *testing.T) {
	form := defaultForm(clientFormMode)
	raw, err := json.Marshal(form)
	if err != nil {
		t.Fatalf("сохранение: %v", err)
	}
	if strings.Contains(string(raw), "sections") {
		t.Fatalf("пустые разделы попали в JSON: %s", raw)
	}
	var back OfferForm
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("чтение: %v", err)
	}
	if msg := validateForm(back); msg != "" {
		t.Fatalf("форма без разделов недействительна: %s", msg)
	}
}

// TestSectionFormValidation — ошибки называются до сборки.
func TestSectionFormValidation(t *testing.T) {
	cases := map[string]func(*[]ContentSection){
		"пустой id раздела": func(s *[]ContentSection) { (*s)[0].ID = " " },
		"повтор id раздела": func(s *[]ContentSection) {
			dup := (*s)[0]
			dup.Subsections = nil
			*s = append(*s, dup)
		},
		"пустой заголовок раздела": func(s *[]ContentSection) { (*s)[0].Title = "" },
		"пустой id подраздела":     func(s *[]ContentSection) { (*s)[0].Subsections[0].ID = "" },
		"повтор id подраздела": func(s *[]ContentSection) {
			dup := (*s)[0].Subsections[0]
			(*s)[0].Subsections = append((*s)[0].Subsections, dup)
		},
		"пустой заголовок подраздела": func(s *[]ContentSection) { (*s)[0].Subsections[0].Title = " " },
		"пустая инструкция":           func(s *[]ContentSection) { (*s)[0].Subsections[0].Instruction = "" },
		"неизвестное условие":         func(s *[]ContentSection) { (*s)[0].Subsections[0].Condition = "sometimes" },
		"неизвестный повтор":          func(s *[]ContentSection) { (*s)[0].Subsections[0].Repeat = "materials" },
		"отрицательный предел слов":   func(s *[]ContentSection) { (*s)[0].Subsections[0].MaxWords = -1 },
		"уровень заголовка вне 1..3":  func(s *[]ContentSection) { (*s)[0].Subsections[0].Level = 7 },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			sections := sampleSections()
			change(&sections)
			form := defaultForm(clientFormMode)
			form.Sections = sections
			if msg := validateForm(form); msg == "" {
				t.Fatalf("%s: форма признана действительной", name)
			}
		})
	}
}

// TestSectionConditionDefaultsToAlways — пустое условие значит «всегда».
func TestSectionConditionDefaultsToAlways(t *testing.T) {
	sub := ContentSubsection{ID: "x", Title: "X", Instruction: "Y"}
	if got := sub.ConditionOrAlways(); got != conditionAlways {
		t.Fatalf("пустое условие дало %q, ожидалось %q", got, conditionAlways)
	}
}
