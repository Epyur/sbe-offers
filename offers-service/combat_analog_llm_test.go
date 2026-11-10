package main

import (
	"strings"
	"testing"
)

// Подбор аналогов моделью (2026-11-11).
//
// Проверяем то, что МОЖНО проверить без сети: разбор ответа модели (это самое
// хрупкое место — модель по-разному оформляет JSON) и сборку запроса.

func TestParseAnalogAnswerPlain(t *testing.T) {
	if got := parseAnalogAnswer("[3, 7, 12]", 5); len(got) != 3 || got[0] != 3 {
		t.Fatalf("простой массив разобран неверно: %v", got)
	}
}

func TestParseAnalogAnswerFenced(t *testing.T) {
	raw := "```json\n[1, 2]\n```"
	if got := parseAnalogAnswer(raw, 5); len(got) != 2 {
		t.Fatalf("ответ в обрамлении не разобран: %v", got)
	}
}

func TestParseAnalogAnswerWithChatter(t *testing.T) {
	// Модель любит добавлять фразу перед массивом.
	raw := "Вот подходящие аналоги: [4, 9]. Больше ничего не подходит."
	got := parseAnalogAnswer(raw, 5)
	if len(got) != 2 || got[0] != 4 || got[1] != 9 {
		t.Fatalf("фраза вокруг массива сломала разбор: %v", got)
	}
}

func TestParseAnalogAnswerEmpty(t *testing.T) {
	if got := parseAnalogAnswer("[]", 5); len(got) != 0 {
		t.Fatalf("пустой массив должен дать пусто: %v", got)
	}
	// Не JSON — пусто, а не случайный выбор.
	if got := parseAnalogAnswer("аналогов нет", 5); len(got) != 0 {
		t.Fatalf("текстовый ответ принят за выбор: %v", got)
	}
}

func TestParseAnalogAnswerDropsDuplicatesAndBad(t *testing.T) {
	got := parseAnalogAnswer("[2, 2, -1, 0, 3]", 5)
	if len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("повторы и мусор не отфильтрованы: %v", got)
	}
}

func TestParseAnalogAnswerLimit(t *testing.T) {
	if got := parseAnalogAnswer("[1,2,3,4,5,6,7]", 5); len(got) != 5 {
		t.Fatalf("предел не применён: %v", got)
	}
}

func TestBuildAnalogPromptNumbersCandidates(t *testing.T) {
	source := SystemCard{Title: "Наш продукт", Description: "Описание нашего продукта."}
	candidates := []analogLLMItem{
		{Index: 1, Title: "Конкурент А", Info: "Описание А"},
		{Index: 2, Title: "Конкурент Б"},
	}
	prompt := buildAnalogPrompt(source, candidates)
	for _, want := range []string{"Наш продукт", "1. Конкурент А", "Описание А", "2. Конкурент Б"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("в запросе нет %q:\n%s", want, prompt)
		}
	}
}

func TestAnalogCandidatesKeepOriginCard(t *testing.T) {
	source := SystemCard{NoteID: 1, Title: "Мембрана ПВХ"}
	catalog := []SystemCard{{NoteID: 2, Title: "Мембрана PLASTFOIL"}}
	got := analogCandidatesForLLM(source, catalog)
	if len(got) != 1 || got[0].origin.NoteID != 2 {
		t.Fatalf("кандидат потерял карточку: %+v", got)
	}
}

// TestAnalogCandidatesFallbackOrder — когда класс не распознан, в модель идёт
// весь каталог по алфавиту: модель разберётся там, где словарь не уверен.
func TestAnalogCandidatesFallbackOrder(t *testing.T) {
	source := SystemCard{NoteID: 1, Title: "Нечто неопознанное"}
	catalog := []SystemCard{
		{NoteID: 3, Title: "Я"},
		{NoteID: 2, Title: "А"},
	}
	got := analogCandidatesForLLM(source, catalog)
	if len(got) != 2 || got[0].Title != "А" || got[1].Title != "Я" {
		t.Fatalf("запасной список не по алфавиту: %+v", got)
	}
}

func TestAnalogCandidatesCapped(t *testing.T) {
	source := SystemCard{NoteID: 1, Title: "Мембрана"}
	catalog := make([]SystemCard, 0, analogCandidateLimit+20)
	for i := 1; i <= analogCandidateLimit+20; i++ {
		catalog = append(catalog, SystemCard{NoteID: int64(i + 1), Title: "Мембрана " + string(rune('А'+i%20))})
	}
	if got := analogCandidatesForLLM(source, catalog); len(got) > analogCandidateLimit {
		t.Fatalf("список не ограничен: %d", len(got))
	}
}
