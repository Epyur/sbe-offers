package main

// Короткий список боевых листков (2026-11-18, задача №2 из
// `NEXT-SESSION-PROMPT.md`).
//
// Замер на стенде (`scripts/measure_combat_list.mjs`) показал: сам список
// считается быстро (200 строк — 83 мс), но ответ весит 8.2 КБ на листок, то есть
// до 1.6 МБ на пределе `listLimit`. Лишнее там — `formValues` (тексты разделов,
// написанные моделью) и `textCompetitors` (страницы, прочитанные из файла или
// ссылки): экрану списка нужно выбрать документ, а не показать его.
//
// Поэтому появился OPT-IN `?fields=preview`. Формат по умолчанию НЕ меняется:
// старый клиент (плагин Obsidian) читает полный листок из списка и сохраняет его
// целиком, и короткий ответ, ушедший ему по умолчанию, стёр бы `form_values` на
// первом же сохранении.

import (
	"encoding/json"
	"strings"
	"testing"
)

func previewFixture() CombatSheet {
	return CombatSheet{
		ID:              7,
		Number:          "012/2026",
		OwnerEmail:      "manager@tn.ru",
		Title:           "Сравнение с конкурентом",
		OfferID:         42,
		OurNotes:        []CombatNoteRef{{NoteID: 101, Title: "ТН-ФУНДАМЕНТ"}},
		CompetitorNotes: []CombatNoteRef{{NoteID: 202, Title: "PIRRO"}},
		TextCompetitors: []CombatTextRef{{
			Key: "k1", Title: "Конкурент из ссылки", Source: "url",
			Text: strings.Repeat("д", 500),
		}},
		FormValues: map[string]string{"gen.sect.general.overview": strings.Repeat("т", 5000)},
		Status:     "draft",
		CreatedAt:  "2026-11-18T10:00:00.000Z",
		UpdatedAt:  "2026-11-18T11:00:00.000Z",
	}
}

// Короткая строка несёт всё, что нужно списку, и не несёт содержимого.
func TestCombatSheetPreviewHasListFieldsOnly(t *testing.T) {
	raw, err := json.Marshal(combatSheetPreview(previewFixture()))
	if err != nil {
		t.Fatalf("строка списка не закодирована: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("строка списка не разбирается: %v", err)
	}
	// Содержимое документа — то, ради чего заведён короткий ответ.
	for _, forbidden := range []string{"formValues", "textCompetitors", "offerId"} {
		if _, ok := got[forbidden]; ok {
			t.Fatalf("в короткой строке осталось поле %q (вес ответа ради которого и делался preview)", forbidden)
		}
	}
	// Всё, чем список пользуется, обязано быть на месте: иначе экран потеряет
	// подпись, номер или автора и человек не выберет документ.
	for _, required := range []string{"id", "number", "title", "ownerEmail", "ourNotes", "competitorNotes", "status", "createdAt", "updatedAt", "displayName"} {
		if _, ok := got[required]; !ok {
			t.Fatalf("в короткой строке нет поля %q", required)
		}
	}
	if sides, ok := got["ourNotes"].([]any); !ok || len(sides) != 1 {
		t.Fatalf("наши продукты потерялись: %v", got["ourNotes"])
	}
	// Подпись считает сервер: конкурент из ссылки называется так же, хотя его
	// текста в коротком ответе нет.
	if got["displayName"] != "ТН-ФУНДАМЕНТ" {
		t.Fatalf("подпись списка не наша сторона: %v", got["displayName"])
	}
}

// Листок, где стороны только текстовые, тоже получает подпись по конкуренту —
// иначе короткий список показывал бы «пустой листок» вместо названия.
func TestCombatSheetPreviewLabelsTextCompetitorSheet(t *testing.T) {
	sheet := previewFixture()
	sheet.OurNotes = nil
	sheet.CompetitorNotes = nil
	if label := combatSheetPreview(sheet).DisplayName; label != "Конкурент из ссылки" {
		t.Fatalf("подпись листка с текстовым конкурентом — %q", label)
	}
}

// Preview — опция, а не режим: незнакомое или пустое значение отдаёт полный
// листок. Опечатка в адресе не должна отнимать данные у клиента.
func TestCombatListPreviewOptIn(t *testing.T) {
	cases := map[string]bool{
		"preview":   true,
		" Preview ": true,
		"PREVIEW":   true,
		"":          false,
		"full":      false,
		"previews":  false,
	}
	for raw, want := range cases {
		if got := combatListPreview(raw); got != want {
			t.Fatalf("?fields=%q → preview=%v, ждали %v", raw, got, want)
		}
	}
}
