package main

import (
	"strings"
	"testing"
)

// Сравнение боевого листка (2026-11-11).
//
// Проверяем то, что увидит менеджер: в документе есть перечень продуктов и
// сравнение; уточнение менеджера доходит до модели; макет с меткой
// `{{сравнение}}` вставляет листок целиком, а без метки — ничего не ломается.

func TestCombatComparisonMarkupHeaders(t *testing.T) {
	text := "## Общее сравнение\nПервый абзац.\n### Продукт конкурента\nВторой абзац."
	xml := combatComparisonMarkup(text)
	if !strings.Contains(xml, "Общее сравнение") || !strings.Contains(xml, "Продукт конкурента") {
		t.Fatalf("заголовки сравнения потеряны: %s", xml)
	}
	// «##» и «###» — уровни 3 и 4, а не текст с решётками.
	if strings.Contains(xml, "##") {
		t.Fatalf("решётки ушли в текст документа: %s", xml)
	}
	if !strings.Contains(xml, "Первый абзац.") || !strings.Contains(xml, "Второй абзац.") {
		t.Fatalf("абзацы сравнения потеряны: %s", xml)
	}
}

func TestCombatComparePromptCarriesHintAndFacts(t *testing.T) {
	our := SystemCard{Title: "Наша система", Description: "Наше описание."}
	comp := SystemCard{
		Title:       "Продукт конкурента",
		Description: "Описание конкурента.",
		Specifications: [][]string{
			{"Показатель", "Значение"},
			{"Класс горючести", "Г1"},
		},
	}
	prompt := buildCombatComparePrompt("огонестойкость", []SystemCard{our}, []SystemCard{comp})
	for _, want := range []string{"огонестойкость", "Наша система", "Продукт конкурента", "Класс горючести"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("в запросе модели нет %q:\n%s", want, prompt)
		}
	}
	// Шапка таблицы модели не нужна: она не факт о продукте.
	if strings.Contains(prompt, "Показатель — Значение") {
		t.Fatalf("шапка таблицы ушла в запрос: %s", prompt)
	}
}

func TestCombatHintReadsFormValues(t *testing.T) {
	sheet := CombatSheet{FormValues: map[string]string{combatHintField: "  срок службы  "}}
	if got := combatHint(sheet); got != "срок службы" {
		t.Fatalf("уточнение прочитано как %q", got)
	}
	if got := combatHint(CombatSheet{}); got != "" {
		t.Fatalf("пустой листок дал уточнение %q", got)
	}
}

// TestCombatDocxUsesLayoutMarker — макет с `{{сравнение}}` вставляет весь
// листок: перечень продуктов и сравнение печатает метка, а не встроенный вид.
func TestCombatDocxUsesLayoutMarker(t *testing.T) {
	our, competitor := combatTestCards()
	sheet := CombatSheet{Number: "009/2026", Title: "Макет"}
	layout := layoutDocxFromXML(t, wordDoc(
		`<w:p><w:r><w:t>Фирменный бланк</w:t></w:r></w:p>`+
			`<w:p><w:r><w:t>{{сравнение}}</w:t></w:r></w:p>`))
	data, err := renderCombatDocx(layout, sheet, []SystemCard{our}, []SystemCard{competitor}, nil)
	if err != nil {
		t.Fatalf("сборка по макету упала: %v", err)
	}
	xml := docXMLOf(t, data)
	if !strings.Contains(xml, "Фирменный бланк") {
		t.Fatal("текст макета потерян")
	}
	if !strings.Contains(xml, "Продукты сравнения") {
		t.Fatal("метка не напечатала перечень продуктов")
	}
	if strings.Contains(xml, "{{") {
		t.Fatal("метка осталась в документе незаполненной")
	}
}

// TestCombatDocxLayoutWithoutMarker — макет без метки печатает только свой
// текст: листок в него не подставляется, и это осознанно — предупредить
// администратора. Сборка при этом не падает.
func TestCombatDocxLayoutWithoutMarker(t *testing.T) {
	our, competitor := combatTestCards()
	sheet := CombatSheet{Number: "010/2026", Title: "Макет без метки"}
	layout := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>Только бланк</w:t></w:r></w:p>`))
	data, err := renderCombatDocx(layout, sheet, []SystemCard{our}, []SystemCard{competitor}, nil)
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	xml := docXMLOf(t, data)
	if !strings.Contains(xml, "Только бланк") {
		t.Fatal("текст макета потерян")
	}
}
