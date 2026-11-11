package main

import (
	"strings"
	"testing"
)

// Доступ к боевым листкам в боте и подпись в списке (2026-11-12).
//
// Просьба пользователя: «все боевые листки нужно сделать доступными всем в
// боте» и «при выводе доступных боевых листков выводить название продукта, по
// которому сделан листок, а не просто „Боевой листок“».
//
// Правила, которые здесь закреплены:
//   - список в боте — ВСЕ листки, а не только свои;
//   - открыть и распечатать чужой листок можно, править — только свой
//     (черновик коллеги не должен испортиться от чужого нажатия);
//   - в списке подпись — продукт листка, а не служебное слово.

func TestCombatListLabelUsesProduct(t *testing.T) {
	sheet := CombatSheet{
		Number: "005/2026",
		Title:  "Боевой листок",
		OurNotes: []CombatNoteRef{
			{NoteID: 1, Title: "Кровельная ПВХ-мембрана LOGICROOF® V-RP"},
		},
		CompetitorNotes: []CombatNoteRef{{NoteID: 2, Title: "ПВХ мембрана PLASTFOIL® LAY"}},
	}
	label := combatListLabel(sheet)
	if label == "Боевой листок" || label == "" {
		t.Fatalf("подпись осталась служебной: %q", label)
	}
	if !strings.Contains(label, "LOGICROOF") {
		t.Fatalf("в подписи нет нашего продукта: %q", label)
	}
}

// Без нашего продукта берём первого конкурента: пустого листка в списке быть
// может (его только что создали), и он тоже должен называться по делу.
func TestCombatListLabelFallsBackToCompetitor(t *testing.T) {
	sheet := CombatSheet{
		Number:          "001/2026",
		Title:           "Боевой листок",
		CompetitorNotes: []CombatNoteRef{{NoteID: 2, Title: "PIRRO®VentiDuct"}},
	}
	if label := combatListLabel(sheet); !strings.Contains(label, "VentiDuct") {
		t.Fatalf("подпись без продукта: %q", label)
	}
}

// Совсем пустой листок — номером называется: «Боевой листок» четыре раза подряд
// в списке бесполезен.
func TestCombatListLabelEmptySheet(t *testing.T) {
	sheet := CombatSheet{Number: "004/2026", Title: "Боевой листок"}
	label := combatListLabel(sheet)
	if !strings.Contains(label, "004/2026") && !strings.Contains(label, "пустой") {
		t.Fatalf("пустой листок подписан непонятно: %q", label)
	}
}

// Право правки: автор правит, все остальные — нет; читать и печатать могут все.
func TestCombatBotPermissions(t *testing.T) {
	sheet := CombatSheet{OwnerEmail: "ivanov@tn.ru"}
	if !botCanEditCombat(sheet, "ivanov@tn.ru") {
		t.Fatal("автор должен править свой листок")
	}
	if botCanEditCombat(sheet, "petrov@tn.ru") {
		t.Fatal("чужой листок править нельзя")
	}
	if !botCanReadCombat(sheet, "petrov@tn.ru") {
		t.Fatal("читать чужой листок в боте можно")
	}
	if !botCanReadCombat(sheet, "") {
		t.Fatal("проверка входа — в routes.ts; здесь пустой email не отказ")
	}
}
