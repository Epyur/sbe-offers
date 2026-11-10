package main

import "testing"

// «Найти аналоги и сравнить» (2026-11-11) — поиск КОНКУРЕНТОВ к нашему
// продукту.
//
// Живые жалобы, которые эти тесты закрывают:
//  1. листок 5: наш продукт выбран, конкурентов нет — кнопка ничего не находила;
//  2. листок 1: к PIR-плите подбирался «МОСТОФЛЕКС» — общее слово в описании.

func TestFindAnaloguesCompetitorByClass(t *testing.T) {
	// Наш продукт — PIR-плита; аналоги — продукты конкурентов того же класса.
	our := SystemCard{
		NoteID:      6363,
		Title:       "Теплоизоляционные плиты LOGICPIR PROF",
		Description: "Плиты из пенополиизоцианурата для кровли.",
	}
	competitors := []SystemCard{
		{NoteID: 7847, Title: "PIRRO Термо", Description: "Теплоизоляционная PIR-плита."},
		{NoteID: 6118, Title: "МОСТОФЛЕКС 250", Description: "Вяжущее для холодного применения."},
		{NoteID: 7184, Title: "Теплоизоляционная плита СмартПанель", Description: "Плита для фасада."},
	}
	got := findAnalogues(our, competitors, 3)
	if len(got) == 0 {
		t.Fatal("аналоги-конкуренты не найдены")
	}
	for _, card := range got {
		if card.NoteID == 6118 {
			t.Fatalf("нерелевантный продукт попал в аналоги: %+v", card)
		}
	}
}

// TestFindAnaloguesRequiresTitleClassHit — совпадения только в описании мало.
//
// Прежний подбор считал любое пересечение слов совпадением, и к PIR-плите
// попадал МОСТОФЛЕКС (общее слово «применения»).
func TestFindAnaloguesRequiresTitleClassHit(t *testing.T) {
	our := SystemCard{NoteID: 1, Title: "Мембрана ПВХ", Description: "Для кровли."}
	catalog := []SystemCard{
		// Слово «мембран» только в описании — не аналог по классу.
		{NoteID: 2, Title: "Профиль крепёжный", Description: "Мембрана крепится к профилю."},
	}
	if got := findAnalogues(our, catalog, 3); len(got) != 0 {
		t.Fatalf("совпадения в описании не должны давать аналог: %+v", got)
	}
}

func TestFindAnaloguesTitleMatchWins(t *testing.T) {
	our := SystemCard{NoteID: 1, Title: "Плита теплоизоляционная"}
	catalog := []SystemCard{
		{NoteID: 2, Title: "Плита теплоизоляционная PIR", Description: ""},
		{NoteID: 3, Title: "Клей", Description: "Для плит теплоизоляционных."},
	}
	got := findAnalogues(our, catalog, 2)
	if len(got) == 0 || got[0].NoteID != 2 {
		t.Fatalf("совпадение по названию должно идти первым: %+v", got)
	}
}

func TestFindAnaloguesEmptyWhenNoMatch(t *testing.T) {
	our := SystemCard{NoteID: 1, Title: "Асбестоцементный лист"}
	catalog := []SystemCard{{NoteID: 2, Title: "Мембрана ПВХ"}}
	if got := findAnalogues(our, catalog, 3); len(got) != 0 {
		t.Fatalf("нерелевантные продукты не должны попадать: %+v", got)
	}
}

func TestFindAnaloguesSkipsItself(t *testing.T) {
	card := SystemCard{NoteID: 7, Title: "Мембрана ПВХ"}
	if got := findAnalogues(card, []SystemCard{card}, 3); len(got) != 0 {
		t.Fatalf("продукт не может быть своим аналогом: %+v", got)
	}
}

func TestFindAnaloguesLimit(t *testing.T) {
	our := SystemCard{NoteID: 99, Title: "Мембрана кровельная"}
	catalog := make([]SystemCard, 0, 10)
	for i := 1; i <= 10; i++ {
		catalog = append(catalog, SystemCard{NoteID: int64(i), Title: "Мембрана кровельная"})
	}
	if got := findAnalogues(our, catalog, 3); len(got) != 3 {
		t.Fatalf("ожидалось 3 аналога, получено %d", len(got))
	}
}

// TestAnalogWordsStems — словоформы одного слова дают одну основу.
func TestAnalogWordsStems(t *testing.T) {
	pairs := [][2]string{
		{"плиты", "плита"},
		{"мембрана", "мембраны"},
		{"теплоизоляция", "теплоизоляционные"},
	}
	for _, pair := range pairs {
		a := analogWords(pair[0])
		b := analogWords(pair[1])
		if len(a) == 0 || len(b) == 0 {
			t.Fatalf("%q/%q: слова потеряны", pair[0], pair[1])
		}
		if a[0] != b[0] {
			t.Fatalf("%q и %q дали разные основы: %q / %q", pair[0], pair[1], a[0], b[0])
		}
	}
}

func TestAnalogScoreSplitsTitleAndBody(t *testing.T) {
	words := analogWords("Мембрана кровельная")
	score, classHit := analogScore(words, "Мембрана ПВХ", "")
	if !classHit || score < 3 {
		t.Fatalf("совпадение названия должно давать класс: %d %v", score, classHit)
	}
	score, classHit = analogScore(words, "Профиль", "Для мембрана кровельная")
	if classHit || score != 2 {
		t.Fatalf("совпадение только в описании не даёт класс: %d %v", score, classHit)
	}
}
