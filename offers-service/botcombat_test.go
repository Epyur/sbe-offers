package main

import "testing"

// Боевой листок в боте (2026-11-09): выбор стороны.
//
// Проверяется поведение, которое видит менеджер: нажал систему второй раз —
// в листке она одна; убрал — списка стало меньше. И то, что в список выбора
// попадает название и заметка, а не код (у живых карточек он пуст).

func TestAppendNoteRefDoesNotDuplicate(t *testing.T) {
	ref := CombatNoteRef{NoteID: 10, Title: "Наша система"}
	got := appendNoteRef(nil, ref)
	got = appendNoteRef(got, ref)
	if len(got) != 1 {
		t.Fatalf("повторный выбор не должен дублировать систему, получено %d", len(got))
	}
	if got[0].NoteID != 10 {
		t.Fatalf("заметка потеряна: %+v", got)
	}
}

func TestAppendNoteRefKeepsOrder(t *testing.T) {
	got := appendNoteRef(nil, CombatNoteRef{NoteID: 1})
	got = appendNoteRef(got, CombatNoteRef{NoteID: 2})
	if len(got) != 2 || got[0].NoteID != 1 || got[1].NoteID != 2 {
		t.Fatalf("порядок выбора должен сохраняться, получено %+v", got)
	}
}

func TestRemoveNoteRefRemovesOnlyTarget(t *testing.T) {
	refs := []CombatNoteRef{{NoteID: 1}, {NoteID: 2}, {NoteID: 3}}
	got := removeNoteRef(refs, 2)
	if len(got) != 2 {
		t.Fatalf("должно остаться две заметки, получено %d", len(got))
	}
	for _, r := range got {
		if r.NoteID == 2 {
			t.Fatal("удалённая заметка осталась в списке")
		}
	}
}

func TestRemoveNoteRefMissingIsNoop(t *testing.T) {
	got := removeNoteRef([]CombatNoteRef{{NoteID: 1}}, 99)
	if len(got) != 1 || got[0].NoteID != 1 {
		t.Fatalf("удаление отсутствующей заметки не должно менять список: %+v", got)
	}
}

func TestCombatItemsCarryNoteAndTitle(t *testing.T) {
	items := combatItems([]SystemCard{
		{NoteID: 100, Title: "Кровля", SKU: ""},
		{NoteID: 101, Title: "Фасад", SKU: "SYS-1", Problems: []string{"нет ссылки"}},
	})
	if len(items) != 2 {
		t.Fatalf("ожидалось две карточки, получено %d", len(items))
	}
	if items[0].NoteID != 100 || items[0].Title != "Кровля" {
		t.Fatalf("карточка без кода должна нести заметку и название: %+v", items[0])
	}
	if items[1].SKU != "SYS-1" || len(items[1].Problems) != 1 {
		t.Fatalf("код и замечания должны доехать до бота: %+v", items[1])
	}
}
