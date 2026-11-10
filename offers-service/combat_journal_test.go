package main

import (
	"strings"
	"testing"
)

// Журнал конкурентов как источник аналогов (2026-11-11, идея пользователя).
//
// Журнал — служебная заметка индексатора: разделы каталога и строки со ссылкой
// на целевую карточку. Тесты проверяют разбор этой структуры и связывание строки
// с настоящей заметкой.

const journalSample = `# Журнал материалов конкурентов

## PLASTFOIL (plastfoil.ru/catalog/)

> Источник: https://plastfoil.ru/catalog/

## ПВХ-мембраны — 28 шт.

| Статус | Материал | URL | Карточка |
| --- | --- | --- | --- |
| ✅ | ПВХ мембрана PLASTFOIL CLASSIC | https://plastfoil.ru/catalog/pvkh-membrana/classic/ | [[ПВХ мембрана PLASTFOIL CLASSIC]] |
| ✅ | ШАЙБА КРОВЕЛЬНАЯ | https://plastfoil.ru/catalog/pvkh-membrana/shayba/ | [[ШАЙБА КРОВЕЛЬНАЯ]] |

## Комплектующие — 9 шт.

| Статус | Материал | URL | Карточка |
| --- | --- | --- | --- |
| ✅ | ГЕОТЕКСТИЛЬ TERRAISOL | https://plastfoil.ru/catalog/komplektuyushchie/geo/ | [[ГЕОТЕКСТИЛЬ TERRAISOL]] |
| ⬜ | Скотч | https://example.com/sk | |
`

func TestParseCompetitorJournalSections(t *testing.T) {
	sections := parseCompetitorJournal(journalSample)
	if len(sections) != 2 {
		t.Fatalf("ожидалось 2 раздела, получено %d: %+v", len(sections), sections)
	}
	if sections[0].Heading != "ПВХ-мембраны — 28 шт." {
		t.Fatalf("заголовок раздела %q", sections[0].Heading)
	}
	if sections[0].Owner != "PLASTFOIL (plastfoil.ru/catalog/)" {
		t.Fatalf("конкурент раздела %q", sections[0].Owner)
	}
	if len(sections[0].Items) != 2 {
		t.Fatalf("в первом разделе %d строк", len(sections[0].Items))
	}
	first := sections[0].Items[0]
	if first.Title != "ПВХ мембрана PLASTFOIL CLASSIC" ||
		first.Note != "ПВХ мембрана PLASTFOIL CLASSIC" ||
		!strings.Contains(first.URL, "pvkh-membrana") {
		t.Fatalf("строка разобрана неверно: %+v", first)
	}
	// Строка без ссылки на карточку (статус «индексировано») — допустима.
	last := sections[1].Items[1]
	if last.Note != "" || last.Title != "Скотч" {
		t.Fatalf("строка без карточки разобрана неверно: %+v", last)
	}
}

func TestParseCompetitorJournalEmpty(t *testing.T) {
	if got := parseCompetitorJournal(""); len(got) != 0 {
		t.Fatalf("пустой журнал дал разделы: %+v", got)
	}
	if got := parseCompetitorJournal("# Журнал материалов конкурентов\n\ntext"); len(got) != 0 {
		t.Fatalf("журнал без таблиц дал разделы: %+v", got)
	}
}

func TestJournalCandidatesNumbering(t *testing.T) {
	sections := parseCompetitorJournal(journalSample)
	shown, total := journalSectionCandidates(sections, 10)
	if total != 4 {
		t.Fatalf("всего строк %d, ожидалось 4", total)
	}
	// Сквозная нумерация: вторая строка второго раздела — номер 3.
	item, ok := journalProductAt(shown, 3)
	if !ok || item.Title != "ГЕОТЕКСТИЛЬ TERRAISOL" {
		t.Fatalf("номер 3 указывает не туда: %+v", item)
	}
	if _, ok := journalProductAt(shown, 0); ok {
		t.Fatal("ноль не должен находить строку")
	}
}

func TestJournalSectionCandidatesLimit(t *testing.T) {
	sections := parseCompetitorJournal(journalSample)
	shown, total := journalSectionCandidates(sections, 1)
	if total != 1 || len(shown) != 1 || len(shown[0].Items) != 1 {
		t.Fatalf("предел строк не применён: %d %+v", total, shown)
	}
}

func TestResolveJournalItemsByNoteName(t *testing.T) {
	catalog := []SystemCard{
		{NoteID: 10, Title: "ПВХ мембрана PLASTFOIL CLASSIC"},
		{NoteID: 11, Title: "ШАЙБА КРОВЕЛЬНАЯ"},
	}
	items := []journalItem{
		{Title: "ПВХ мембрана PLASTFOIL CLASSIC", Note: "ПВХ мембрана PLASTFOIL CLASSIC"},
		// Карточки в каталоге нет — пропускаем, а не выдумываем.
		{Title: "Неизвестный скотч", Note: "Неизвестный скотч"},
	}
	got := resolveJournalItems(items, catalog)
	if len(got) != 1 || got[0].NoteID != 10 {
		t.Fatalf("строка журнала должна дать настоящую карточку: %+v", got)
	}
}

func TestBuildJournalPromptCarriesSections(t *testing.T) {
	sections := parseCompetitorJournal(journalSample)
	prompt := buildJournalAnalogPrompt(SystemCard{Title: "Наш продукт"}, sections)
	for _, want := range []string{"Наш продукт", "ПВХ-мембраны", "Комплектующие", "PLASTFOIL"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("в запросе нет %q:\n%s", want, prompt)
		}
	}
	if !strings.Contains(prompt, "1. ПВХ мембрана PLASTFOIL CLASSIC") {
		t.Fatalf("нумерация строк потеряна:\n%s", prompt)
	}
}
