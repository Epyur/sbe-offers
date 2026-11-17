package main

// Название продукта конкурента в боевом листке (2026-11-17).
//
// Живой дефект — листок 017/2026: в базе сохранено
// `{"title":"Материал конкурента, использовать только для критического сравнения","noteId":6654}`,
// а в листке 016/2026 та же заметка называется «PIRRO®Membrane». Причина: у
// заметок конкурентов ДВА H1 (первый — предупреждение, второй — продукт), и
// агентский путь (`readNoteCard` → `systemCardFromParsed`) брал первый.
//
// Проверяется то, что видит менеджер: карточка называется продуктом, дисклеймер
// не попадает в листок ни через одного клиента, а уже испорченный листок
// показывается правильно.

import (
	"context"
	"testing"
)

const competitorDisclaimerTitle = "Материал конкурента, использовать только для критического сравнения"

// competitorBody — так размечены живые заметки конкурентов.
const competitorBody = "# " + competitorDisclaimerTitle + "\n\n# PIRRO®Membrane\n\n## Описание\n\nPIR-плита.\n"

// competitorStubKB — База знаний с одной заметкой конкурента.
func competitorStubKB(t *testing.T, noteID int64, body string, notePath string) *KBClient {
	t.Helper()
	stub := &folderKBStub{
		tags:     map[string]int64{competitorTagName: 8},
		bodies:   map[int64]string{noteID: body},
		noteTags: map[int64][]KBTag{noteID: {{ID: 8, Name: competitorTagName}}},
	}
	if notePath != "" {
		stub.notePaths = map[int64]string{noteID: notePath}
	}
	return newFolderKB(t, stub)
}

// TestReadNoteCardNamesCompetitorByProduct — карточка конкурента, прочитанная
// агентом, называется продуктом, а не предупреждением.
func TestReadNoteCardNamesCompetitorByProduct(t *testing.T) {
	s := &Server{kb: competitorStubKB(t, 6654, competitorBody, "Материалы и системы конкурентов/Пиррогрупп/PIRRO®Membrane.md")}
	card, tags, err := s.readNoteCard(context.Background(), 6654)
	if err != nil {
		t.Fatalf("карточка не прочитана: %v", err)
	}
	if card.Title != "PIRRO®Membrane" {
		t.Fatalf("название конкурента должно быть продуктом, получено %q", card.Title)
	}
	if cardRole(tags) != "competitor" {
		t.Fatalf("роль карточки должна остаться competitor, получено %q", cardRole(tags))
	}
}

// TestReadNoteCardFallsBackToFileName — в заметке один дисклеймер и больше
// ничего: название берётся из имени файла, предупреждение не показывается.
func TestReadNoteCardFallsBackToFileName(t *testing.T) {
	body := "# " + competitorDisclaimerTitle + "\n\n## Описание\n\nБез второго заголовка.\n"
	s := &Server{kb: competitorStubKB(t, 7001, body, "Материалы и системы конкурентов/Пластфойл/ШАЙБА КРОВЕЛЬНАЯ.md")}
	card, _, err := s.readNoteCard(context.Background(), 7001)
	if err != nil {
		t.Fatalf("карточка не прочитана: %v", err)
	}
	if card.Title != "ШАЙБА КРОВЕЛЬНАЯ" {
		t.Fatalf("ожидалось имя файла вместо дисклеймера, получено %q", card.Title)
	}
}

// TestCompetitorRefTitleRules — подпись стороны: карточка главнее клиента,
// дисклеймер не название никогда.
func TestCompetitorRefTitleRules(t *testing.T) {
	cases := []struct {
		name   string
		card   string
		client string
		want   string
	}{
		{"карточка главнее клиента", "PIRRO®Membrane", "Старое имя", "PIRRO®Membrane"},
		{"клиент — запасной путь", "", "PIRRO®Membrane", "PIRRO®Membrane"},
		{"дисклеймер в карточке не название", competitorDisclaimerTitle, "PIRRO®Membrane", "PIRRO®Membrane"},
		{"дисклеймер у клиента не название", "PIRRO®Membrane", competitorDisclaimerTitle, "PIRRO®Membrane"},
		{"оба дисклеймеры — названия нет", competitorDisclaimerTitle, competitorDisclaimerTitle, ""},
		{"пробелы не название", "  ", "  ", ""},
	}
	for _, tc := range cases {
		if got := competitorRefTitle(tc.card, tc.client); got != tc.want {
			t.Errorf("%s: получено %q, ожидалось %q", tc.name, got, tc.want)
		}
	}
}

// TestCombatCompetitorTitleAsksTheCard — клиент прислал дисклеймер: сервер
// читает название из карточки сам.
func TestCombatCompetitorTitleAsksTheCard(t *testing.T) {
	s := &Server{kb: competitorStubKB(t, 6654, competitorBody, "")}
	if got := s.combatCompetitorTitle(context.Background(), 6654, competitorDisclaimerTitle); got != "PIRRO®Membrane" {
		t.Fatalf("дисклеймер клиента должен замениться названием карточки, получено %q", got)
	}
	// Правильная подпись клиента остаётся как есть — лишнего чтения нет.
	if got := s.combatCompetitorTitle(context.Background(), 6654, "ПВХ мембрана PLASTFOIL"); got != "ПВХ мембрана PLASTFOIL" {
		t.Fatalf("верная подпись не должна переписываться, получено %q", got)
	}
}

// TestWithRepairedCompetitorTitles — листок, испорченный до правки, читается
// правильно: название берётся из кеша конкурентов. Незнакомая заметка
// остаётся как есть, наши продукты не трогаются.
func TestWithRepairedCompetitorTitles(t *testing.T) {
	sheet := CombatSheet{
		OurNotes: []CombatNoteRef{{NoteID: 1, Title: "LOGICROOF V-RP"}},
		CompetitorNotes: []CombatNoteRef{
			{NoteID: 6654, Title: competitorDisclaimerTitle},
			{NoteID: 7001, Title: ""},
			{NoteID: 9001, Title: "Уже правильное имя"},
		},
	}
	cards := []SystemCard{
		{NoteID: 6654, Title: "PIRRO®Membrane"},
		{NoteID: 7001, Title: "ШАЙБА КРОВЕЛЬНАЯ"},
	}
	got := withRepairedCompetitorTitles(sheet, cards)
	want := []string{"PIRRO®Membrane", "ШАЙБА КРОВЕЛЬНАЯ", "Уже правильное имя"}
	for i, title := range want {
		if got.CompetitorNotes[i].Title != title {
			t.Errorf("конкурент %d: получено %q, ожидалось %q", i, got.CompetitorNotes[i].Title, title)
		}
	}
	if got.OurNotes[0].Title != "LOGICROOF V-RP" {
		t.Errorf("наша сторона не должна меняться, получено %q", got.OurNotes[0].Title)
	}
}

// TestWithRepairedCompetitorTitlesKeepsUnknown — заметки нет в кеше: название
// не выдумывается, листок остаётся как был.
func TestWithRepairedCompetitorTitlesKeepsUnknown(t *testing.T) {
	sheet := CombatSheet{CompetitorNotes: []CombatNoteRef{{NoteID: 42, Title: competitorDisclaimerTitle}}}
	got := withRepairedCompetitorTitles(sheet, []SystemCard{{NoteID: 7, Title: "Чужая карточка"}})
	if got.CompetitorNotes[0].Title != competitorDisclaimerTitle {
		t.Fatalf("без карточки название меняться не должно, получено %q", got.CompetitorNotes[0].Title)
	}
}

// TestWithRepairedCompetitorTitlesEmptyCache — пустой кеш конкурентов: листок
// возвращается без изменений (не паникуем и не чистим названия).
func TestWithRepairedCompetitorTitlesEmptyCache(t *testing.T) {
	sheet := CombatSheet{CompetitorNotes: []CombatNoteRef{{NoteID: 42, Title: competitorDisclaimerTitle}}}
	got := withRepairedCompetitorTitles(sheet, nil)
	if len(got.CompetitorNotes) != 1 || got.CompetitorNotes[0].Title != competitorDisclaimerTitle {
		t.Fatalf("пустой кеш не должен менять листок: %+v", got.CompetitorNotes)
	}
}
