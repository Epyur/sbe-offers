package main

import (
	"testing"
)

// Боевой листок (2026-11-08): проверки ввода и нормализации.
//
// Тесты поведения, а не реализации: они спрашивают то же, что сервер спросит у
// менеджера, — «этот листок можно сохранить?» и «что окажется в полях после
// сохранения?».

func TestValidateCombatSheetTitleRequired(t *testing.T) {
	msg := validateCombatSheet(CombatSheet{Title: "   "})
	if msg == "" {
		t.Fatal("ожидалась ошибка на пустое название боевого листка")
	}
}

func TestValidateCombatSheetAcceptsDraftWithoutProducts(t *testing.T) {
	// Пустой доходный листок — законный черновик: менеджер только начал.
	if msg := validateCombatSheet(CombatSheet{Title: "Боевой листок"}); msg != "" {
		t.Fatalf("пустой черновик должен сохраняться, получено: %q", msg)
	}
}

func TestValidateCombatSheetRejectsNoteWithoutID(t *testing.T) {
	if msg := validateCombatSheet(CombatSheet{
		Title:    "Боевой листок",
		OurNotes: []CombatNoteRef{{NoteID: 0, Title: "Без заметки"}},
	}); msg == "" {
		t.Fatal("ожидалась ошибка: у нашего продукта нет заметки")
	}
	if msg := validateCombatSheet(CombatSheet{
		Title:           "Боевой листок",
		CompetitorNotes: []CombatNoteRef{{NoteID: 0, Title: "Конкурент без заметки"}},
	}); msg == "" {
		t.Fatal("ожидалась ошибка: у продукта конкурента нет заметки")
	}
}

func TestValidateCombatSheetAcceptsFilled(t *testing.T) {
	msg := validateCombatSheet(CombatSheet{
		Title:           "Сравнение с конкурентом",
		OurNotes:        []CombatNoteRef{{NoteID: 10, Title: "Наша система"}},
		CompetitorNotes: []CombatNoteRef{{NoteID: 20, Title: "Конкурент"}},
	})
	if msg != "" {
		t.Fatalf("корректный листок должен сохраняться, получено: %q", msg)
	}
}

func TestNormalizeCombatFillsEmpty(t *testing.T) {
	c := CombatSheet{}
	normalizeCombat(&c)
	if c.OurNotes == nil || c.CompetitorNotes == nil {
		t.Fatal("списки заметок должны стать непустыми срезами")
	}
	if c.FormValues == nil {
		t.Fatal("карта значений формы должна стать непустой")
	}
}

func TestCanSeeCombatOwnerAndAdmin(t *testing.T) {
	c := CombatSheet{OwnerEmail: "manager@tn.ru"}
	if !canSeeCombat("editor", "manager@tn.ru", c) {
		t.Fatal("владелец должен видеть свой боевой листок")
	}
	if canSeeCombat("editor", "other@tn.ru", c) {
		t.Fatal("чужой editor не должен видеть чужой боевой листок")
	}
	if !canSeeCombat("admin", "someone@tn.ru", c) {
		t.Fatal("admin должен видеть любой боевой листок")
	}
}
