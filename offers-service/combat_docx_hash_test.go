package main

import "testing"

// Готовый DOCX боевого листка не пересобирается, пока не менялся (2026-11-12).
//
// Просьба пользователя: «нужно сделать механизм (как в КП), чтобы листок не
// перегенерировался каждый раз (это долго)». Сборка листка зовёт модель на
// сравнение — это десятки секунд, и повторное нажатие «⬇ DOCX» на том же
// листке не должно платить за это второй раз.
//
// Правило: хэш считается по всему, что видит сборка — стороны листка, макет,
// уточнение менеджера. Изменилось хоть что-то — хэш другой, и листок
// пересобирается. Обратное тоже обязанó быть верным: сравнение снимков карточек
// не входит в хэш намеренно, иначе любое обновление Базы знаний заставляло бы
// пересобирать все листки.

func TestCombatDocxHashStable(t *testing.T) {
	sheet := CombatSheet{
		ID:              1,
		OurNotes:        []CombatNoteRef{{NoteID: 1, Title: "ПВХ-мембрана"}},
		CompetitorNotes: []CombatNoteRef{{NoteID: 2, Title: "PLASTFOIL"}},
		FormValues:      map[string]string{"hint": "огнестойкость"},
	}
	a := combatDocxHash(sheet, []byte("layout-v1"), "v1")
	b := combatDocxHash(sheet, []byte("layout-v1"), "v1")
	if a != b {
		t.Fatalf("одинаковый листок дал разные хэши: %q / %q", a, b)
	}
}

func TestCombatDocxHashChanges(t *testing.T) {
	base := CombatSheet{
		ID:              1,
		OurNotes:        []CombatNoteRef{{NoteID: 1, Title: "ПВХ-мембрана"}},
		CompetitorNotes: []CombatNoteRef{{NoteID: 2, Title: "PLASTFOIL"}},
		FormValues:      map[string]string{"hint": "огнестойкость"},
	}
	layout := []byte("layout-v1")
	ref := combatDocxHash(base, layout, "v1")

	addComp := base
	addComp.CompetitorNotes = append(addComp.CompetitorNotes[:1:1],
		CombatNoteRef{NoteID: 3, Title: "Sikaplan"})
	if combatDocxHash(addComp, layout, "v1") == ref {
		t.Fatal("добавленная сторона не изменила хэш")
	}

	hint := base
	hint.FormValues = map[string]string{"hint": "срок службы"}
	if combatDocxHash(hint, layout, "v1") == ref {
		t.Fatal("смена уточнения не изменила хэш")
	}

	if combatDocxHash(base, []byte("layout-v2"), "v1") == ref {
		t.Fatal("новый макет не изменил хэш")
	}

	if combatDocxHash(base, layout, "v2") == ref {
		t.Fatal("смена версии пометки не изменила хэш")
	}
}

// Порядок сторон — часть содержания листка: поменяли порядок — другой хэш.
func TestCombatDocxHashOrderMatters(t *testing.T) {
	a := CombatSheet{
		ID:              1,
		CompetitorNotes: []CombatNoteRef{{NoteID: 2, Title: "A"}, {NoteID: 3, Title: "B"}},
		FormValues:      map[string]string{},
	}
	b := CombatSheet{
		ID:              1,
		CompetitorNotes: []CombatNoteRef{{NoteID: 3, Title: "B"}, {NoteID: 2, Title: "A"}},
		FormValues:      map[string]string{},
	}
	if combatDocxHash(a, nil, "v1") == combatDocxHash(b, nil, "v1") {
		t.Fatal("порядок сторон не входит в хэш")
	}
}
