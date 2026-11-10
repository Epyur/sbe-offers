package main

import "testing"

// «Подобрать аналог» (2026-11-11): подбор нашего продукта по карточке
// конкурента.
//
// Проверяем поведение, которое видит менеджер: по названию конкурента
// находится наш продукт того же класса, а не первая строка каталога; пустой
// подбор — законный ответ.

func TestFindAnaloguesByName(t *testing.T) {
	competitor := SystemCard{
		NoteID:      20,
		Title:       "PIRRO Мембрана ПВХ",
		Description: "Гидроизоляционная мембрана из пластифицированного ПВХ.",
	}
	catalog := []SystemCard{
		{NoteID: 1, Title: "Плита минеральная", Description: "Теплоизоляция из каменной ваты."},
		{NoteID: 2, Title: "Мембрана ПВХ LOGICROOF", Description: "Гидроизоляционная мембрана."},
		{NoteID: 3, Title: "Анкер-втулка", Description: "Крепёж для теплоизоляции."},
	}
	got := findAnalogues(competitor, catalog, 3)
	if len(got) == 0 {
		t.Fatal("аналог не найден")
	}
	if got[0].NoteID != 2 {
		t.Fatalf("первым должен быть продукт того же класса, получено: %+v", got)
	}
	// Совсем непохожие продукты в подбор не попадают.
	for _, card := range got {
		if card.NoteID == 1 || card.NoteID == 3 {
			t.Fatalf("непохожий продукт попал в аналоги: %+v", card)
		}
	}
}

func TestFindAnaloguesTitleOutweighsDescription(t *testing.T) {
	competitor := SystemCard{NoteID: 20, Title: "Мембрана ПВХ", Description: "Мембрана кровельная."}
	catalog := []SystemCard{
		// Название совпадает, описание — нет.
		{NoteID: 1, Title: "Мембрана ПВХ", Description: "Рулонный материал."},
		// Совпадение только в описании, и оно длиннее.
		{NoteID: 2, Title: "Профиль", Description: "Для кровли: мембрана, крепёж, мембрана."},
	}
	got := findAnalogues(competitor, catalog, 2)
	if len(got) < 2 || got[0].NoteID != 1 {
		t.Fatalf("название должно весить больше описания, получено: %+v", got)
	}
}

func TestFindAnaloguesLimit(t *testing.T) {
	competitor := SystemCard{NoteID: 99, Title: "Мембрана кровельная"}
	catalog := make([]SystemCard, 0, 10)
	for i := 1; i <= 10; i++ {
		catalog = append(catalog, SystemCard{NoteID: int64(i), Title: "Мембрана кровельная"})
	}
	if got := findAnalogues(competitor, catalog, 3); len(got) != 3 {
		t.Fatalf("ожидалось 3 аналога, получено %d", len(got))
	}
}

func TestFindAnaloguesNothingMatches(t *testing.T) {
	competitor := SystemCard{NoteID: 20, Title: "Асбестоцементный лист"}
	catalog := []SystemCard{{NoteID: 1, Title: "Мембрана ПВХ"}}
	if got := findAnalogues(competitor, catalog, 3); len(got) != 0 {
		t.Fatalf("непохожий продукт не должен попадать в аналоги: %+v", got)
	}
}

func TestFindAnaloguesSkipsItself(t *testing.T) {
	card := SystemCard{NoteID: 7, Title: "Мембрана ПВХ"}
	if got := findAnalogues(card, []SystemCard{card}, 3); len(got) != 0 {
		t.Fatalf("сам конкурент не может быть своим аналогом: %+v", got)
	}
}

func TestAnalogWordsDropsStopWords(t *testing.T) {
	got := analogWords("Для мембраны ПВХ и систем кровли")
	// «для», «и» и короткие обрывки отброшены; слова приведены к нижнему регистру.
	for _, w := range got {
		if w == "для" || w == "и" {
			t.Fatalf("служебное слово осталось: %+v", got)
		}
	}
	if len(got) == 0 {
		t.Fatal("значимые слова потеряны")
	}
}
