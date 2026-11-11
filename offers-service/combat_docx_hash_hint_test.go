package main

import (
	"strings"
	"testing"
)

// Пустое уточнение и уточнение из старого листка: хэш обязан отличаться.
// Живой прогон стенда показал, что после смены `hint` файл отдавался прежний —
// проверяем сам хэш, а не догадки о кэше.
func TestCombatDocxHashHintVariants(t *testing.T) {
	base := CombatSheet{
		ID:         1,
		Number:     "001/2026",
		OurNotes:   []CombatNoteRef{{NoteID: 1, Title: "Наш"}},
		FormValues: map[string]string{},
	}
	none := combatDocxHash(base, nil, "v1")

	withHint := base
	withHint.FormValues = map[string]string{"hint": "срок службы"}
	hinted := combatDocxHash(withHint, nil, "v1")
	if hinted == none {
		t.Fatalf("уточнение не вошло в хэш: %q", hinted)
	}

	otherKey := base
	otherKey.FormValues = map[string]string{"object_name": "ЖК Поэма"}
	if combatDocxHash(otherKey, nil, "v1") != none {
		t.Fatal("чужое поле формы меняет хэш листка — кэш будет обновляться зря")
	}

	trimmed := base
	trimmed.FormValues = map[string]string{"hint": "  "}
	if combatDocxHash(trimmed, nil, "v1") != none {
		t.Fatal("пробельное уточнение — то же, что пустое")
	}
	_ = strings.TrimSpace("")
}
