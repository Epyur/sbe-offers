package main

import (
	"strings"
	"testing"

	"mailers/offers/skills"
)

// Ссылки карточек — часть содержимого листка (2026-11-18).
//
// Живой дефект: боевой листок 017/2026 печатал две ссылки из «## Ссылки», хотя
// в заметке был и «## Иные документы» (файлы приложения «Документы»). Файл
// отдавался из `offers_combat_docx` по хэшу, в который ссылки не входили,
// поэтому даже после правильного перечитывания карточек выгрузка возвращала
// прежний список. Сторож держит обе стороны правила: новый набор ссылок —
// новый хэш, тот же набор — прежний (иначе кэш перестанет экономить сборку).
func TestCombatDocxHashChangesWithRefs(t *testing.T) {
	sheet := CombatSheet{
		ID:         7,
		Number:     "017/2026",
		OurNotes:   []CombatNoteRef{{NoteID: 6363, Title: "LOGICPIR PROF Ф/Ф"}},
		FormValues: map[string]string{},
	}
	layout := []byte("layout-v1")
	two := []skills.CardRef{
		{Label: "Страница продукта", URL: "https://nav.tn.ru/catalog/pir/"},
		{Label: "Техлист 8_01", URL: "https://nav.tn.ru/pim/8.01.pdf"},
	}
	withDocs := append(append([]skills.CardRef{}, two...),
		skills.CardRef{Label: "ДС ФЗ 123", URL: "https://epyur.fvds.ru/api/documents/file?key=documents%2Fa.pdf"},
		skills.CardRef{Label: "Подписанное письмо", URL: "https://epyur.fvds.ru/api/documents/file?key=documents%2Fb.pdf"},
	)

	base := combatDocxHash(sheet, layout, "v8", "form", two)
	same := combatDocxHash(sheet, layout, "v8", "form", two)
	if base != same {
		t.Fatalf("тот же набор ссылок дал разные хэши: %q / %q", base, same)
	}
	if combatDocxHash(sheet, layout, "v8", "form", withDocs) == base {
		t.Fatal("ссылки из «Иные документы» не вошли в хэш — файл останется прежним")
	}
	// Пустой список не должен менять хэш у листков, где ссылок нет вовсе:
	// иначе любой листок без карточек пересобирался бы после выпуска.
	if combatDocxHash(sheet, layout, "v8", "form", nil) != combatDocxHash(sheet, layout, "v8", "form", []skills.CardRef{}) {
		t.Fatal("пустой набор ссылок меняет хэш")
	}
	// Порядок ссылок — порядок заметки, и он тоже содержание документа.
	reordered := []skills.CardRef{two[1], two[0]}
	if combatDocxHash(sheet, layout, "v8", "form", reordered) == base {
		t.Fatal("перестановка ссылок не изменила хэш")
	}
}

// combatOurRefs — один набор для печати и для хэша: повторы по адресу
// схлопываются, порядок упоминания сохраняется.
func TestCombatOurRefsDedupesKeepingOrder(t *testing.T) {
	cards := []SystemCard{
		{NoteID: 1, Refs: []skills.CardRef{
			{Label: "Страница", URL: "https://a"},
			{Label: "Техлист", URL: "https://b"},
		}},
		{NoteID: 2, Refs: []skills.CardRef{
			{Label: "Тот же техлист", URL: "https://b"},
			{Label: "ДС", URL: "https://c"},
		}},
	}
	refs := combatOurRefs(cards)
	if len(refs) != 3 {
		t.Fatalf("ожидали 3 ссылки после дедупа, получили %d: %#v", len(refs), refs)
	}
	if refs[0].URL != "https://a" || refs[1].URL != "https://b" || refs[2].URL != "https://c" {
		t.Fatalf("порядок ссылок сбит: %#v", refs)
	}
}

// Пустой список ссылок виден в документе, а не молчит: иначе человек считает,
// что документов у продукта нет.
func TestCombatRefsWarning(t *testing.T) {
	cards := []SystemCard{{NoteID: 6363, Title: "LOGICPIR"}}
	warning := combatRefsWarning(cards, nil)
	if !strings.Contains(warning, "Контекст КП") {
		t.Fatalf("предупреждение должно называть раздел с кнопкой перечитывания: %q", warning)
	}
	if !strings.Contains(warning, "Перечитать") {
		t.Fatalf("предупреждение должно называть кнопку: %q", warning)
	}
	// Ссылки есть — предупреждения нет.
	if got := combatRefsWarning(cards, []skills.CardRef{{URL: "https://a"}}); got != "" {
		t.Fatalf("при непустом списке ссылок предупреждение не нужно: %q", got)
	}
	// Карточка без заметки (текстовый конкурент на нашей стороне) — не повод
	// показывать предупреждение: перечитывать нечего.
	if got := combatRefsWarning([]SystemCard{{Title: "текстовый"}}, nil); got != "" {
		t.Fatalf("у карточки без noteId предупреждение ложное: %q", got)
	}
	if got := combatRefsWarning(nil, nil); got != "" {
		t.Fatalf("без наших карточек предупреждение не печатается: %q", got)
	}
}
