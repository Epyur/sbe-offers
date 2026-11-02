package skills

import (
	"strings"
	"testing"
)

// Модельный разбор контекста и слияние с дословным (2026-11-01).

func TestParseFileContextAnswer(t *testing.T) {
	raw := "```json\n" + `{
  "object_name": "По ул. Охотская, 20",
  "object_purpose": "Жилой дом",
  "object_area": "9 200 м2",
  "customer_address": "",
  "planned_systems": ["ТН-ФУНДАМЕНТ Дренаж Проф", "ТН-СТИЛОБАТ ЭКСПЕРТ Тротуар"],
  "customer_goals": ["Защита подземной части"],
  "project_risks": ["Зимние работы"],
  "notes": "Система с однослойной мембраной LOGICBASE."
}` + "\n```"
	got, err := ParseFileContextAnswer(raw)
	if err != nil {
		t.Fatalf("разбор упал: %v", err)
	}
	if got.Fields["object_name"][0] != "По ул. Охотская, 20" {
		t.Fatalf("объект = %#v", got.Fields["object_name"])
	}
	if len(got.Fields["planned_systems"]) != 2 {
		t.Fatalf("системы = %#v", got.Fields["planned_systems"])
	}
	if !strings.Contains(got.Notes, "LOGICBASE") {
		t.Fatalf("заметки = %q", got.Notes)
	}
	if len(got.Summary) == 0 {
		t.Fatal("сводка пуста")
	}
}

func TestParseFileContextAnswerRejectsGarbage(t *testing.T) {
	if _, err := ParseFileContextAnswer("не JSON вовсе"); err == nil {
		t.Fatal("мусор разобран без ошибки")
	}
}

func TestParseFileContextAnswerSkipsEmptyFields(t *testing.T) {
	got, err := ParseFileContextAnswer(`{"object_name": "  ", "object_area": "", "planned_systems": []}`)
	if err != nil {
		t.Fatalf("разбор упал: %v", err)
	}
	if got.HasContent() {
		t.Fatalf("пустые поля записаны: %+v", got.Fields)
	}
}

// Дословное главнее модельного: «Многоквартирный дом» из файла не должно
// превращаться в «жилой комплекс» от модели.
func TestMergeFileContextsPrefersLiteral(t *testing.T) {
	facts := emptyFileContext()
	facts.put("object_name", "Многоквартирный дом по ул. Охотская")
	facts.put("planned_systems", "ТН-ФУНДАМЕНТ Дренаж Проф")

	model := emptyFileContext()
	model.put("object_name", "Жилой комплекс «Поэма»")
	model.put("object_area", "9 200 м2")
	model.put("planned_systems", "ТН-СТИЛОБАТ Эксперт")
	model.Notes = "Описание системы."

	got := MergeFileContexts(facts, model)
	if len(got.Fields["object_name"]) != 1 || !strings.Contains(got.Fields["object_name"][0], "Охотская") {
		t.Fatalf("объект = %#v, ожидали дословный", got.Fields["object_name"])
	}
	if got.Fields["object_area"][0] != "9 200 м2" {
		t.Fatalf("площадь от модели потеряна: %#v", got.Fields["object_area"])
	}
	if !strings.Contains(got.Notes, "Описание системы") {
		t.Fatalf("заметки модели потеряны: %q", got.Notes)
	}
	// Системы объединяются: дословная заменила список модели, иначе в КП
	// попали бы две формулировки одной системы.
	if len(got.Fields["planned_systems"]) != 1 {
		t.Fatalf("системы = %#v", got.Fields["planned_systems"])
	}
}

func TestMergeFileContextsKeepsFactsWhenModelEmpty(t *testing.T) {
	facts := emptyFileContext()
	facts.put("object_name", "Склад на Левобережной, 3")
	facts.Notes = "Описание из файла."

	got := MergeFileContexts(facts, emptyFileContext())
	if got.Fields["object_name"][0] != "Склад на Левобережной, 3" {
		t.Fatalf("дословный объект потерян: %#v", got.Fields)
	}
	if got.Notes != "Описание из файла." {
		t.Fatalf("заметки = %q", got.Notes)
	}
}

func TestJoinNotesDeduplicates(t *testing.T) {
	got := joinNotes("Первая строка\nВторая", "вторая\nТретья")
	if strings.Count(strings.ToLower(got), "вторая") != 1 {
		t.Fatalf("повтор не убран: %q", got)
	}
}

func TestRenderFileContextUserIncludesText(t *testing.T) {
	got := RenderFileContextUser([]string{"для объекта: Дом на Охотской"})
	if !strings.Contains(got, "Дом на Охотской") {
		t.Fatalf("текст файла не попал в запрос: %q", got)
	}
	if !strings.Contains(got, "JSON") {
		t.Fatalf("формат ответа не запрошен: %q", got)
	}
}
