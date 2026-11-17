package main

import (
	"context"
	"strings"
	"testing"

	"mailers/offers/skills"
)

// Боевой листок: сборка DOCX (2026-11-08, вторая партия 0.8.3).
//
// Тесты поведения: собираем документ по заданной форме и проверяем то, что
// увидит человек в файле, — наши системы, продукты конкурентов, подставленные
// поля и отсутствие служебных скобок. Внутреннее устройство рендера теста не
// касается.

// combatTestCards — наш продукт и продукт конкурента для сборки.
func combatTestCards() (our, competitor SystemCard) {
	our = SystemCard{
		NoteID:      10,
		Title:       "Наша система",
		Description: "Наше описание системы.",
		Composition: []string{"Мембрана наша"},
	}
	competitor = SystemCard{
		NoteID:      20,
		Title:       "Продукт конкурента",
		Description: "Описание конкурента.",
		Composition: []string{"Мембрана конкурента"},
	}
	return our, competitor
}

func TestRenderCombatDocxBasics(t *testing.T) {
	our, competitor := combatTestCards()
	sheet := CombatSheet{
		ID:              1,
		Number:          "003/2026",
		OwnerEmail:      "manager@tn.ru",
		Title:           "Боевой листок",
		OurNotes:        []CombatNoteRef{{NoteID: 10, Title: "Наша система"}},
		CompetitorNotes: []CombatNoteRef{{NoteID: 20, Title: "Продукт конкурента"}},
		FormValues:      map[string]string{"object_name": "ЖК Поэма", "conclusion": "Берём нашу систему"},
	}
	data, err := combatNoForm((*Server).renderCombatDocx)(context.Background(), nil, sheet, []SystemCard{our}, []SystemCard{competitor}, nil)
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	xml := docXMLOf(t, data)

	if strings.Contains(xml, "{{") || strings.Contains(xml, "}}") {
		t.Fatal("в документе остались незаполненные плейсхолдеры")
	}
	if !strings.Contains(xml, "Боевой листок") {
		t.Fatal("в документе нет названия боевого листка")
	}
	// Полей формы у боевого листка больше нет (2026-11-11): объекта, заказчика
	// и вывода в документе быть не должно, а перечень продуктов — должен.
	for _, gone := range []string{"ЖК Поэма", "Берём нашу систему"} {
		if strings.Contains(xml, gone) {
			t.Fatalf("в документе осталось поле формы %q", gone)
		}
	}
	if !strings.Contains(xml, "Продукты сравнения") {
		t.Fatal("нет перечня продуктов")
	}
	if !strings.Contains(xml, "Сравнение") {
		t.Fatal("нет раздела сравнения")
	}
	// Наши системы и конкуренты — обе стороны сравнения в документе.
	if !strings.Contains(xml, "Наша система") {
		t.Fatal("нет нашего продукта в документе")
	}
	if !strings.Contains(xml, "Продукт конкурента") {
		t.Fatal("нет продукта конкурента в документе")
	}
	if !strings.Contains(xml, "Описание конкурента") {
		t.Fatal("нет описания конкурента в документе")
	}
}

// Встроенный вид листка печатает ссылки наших продуктов (2026-11-17).
//
// Требование пользователя: «в боевой листок включаем все ссылки по нашим
// продуктам, в том числе на документы в S3, на которые есть ссылка в заметке
// Базы знаний». Без макета метку `{{ссылки}}` ставить некуда, поэтому встроенный
// вид дописывает раздел сам — иначе настройка «ссылки включены» ничего бы не
// значила для листка без макета.
func TestCombatSheetBuiltInPrintsOurLinks(t *testing.T) {
	our, competitor := combatTestCards()
	our.Refs = []skills.CardRef{
		{Label: "Страница нашего продукта", URL: "https://nav.tn.ru/systems/our/"},
		{Label: "Декларация соответствия", URL: "https://epyur.fvds.ru/api/documents/file?key=documents%2Fd.pdf"},
	}
	competitor.Refs = []skills.CardRef{{Label: "Сайт конкурента", URL: "https://pirrogroup.ru/katalog"}}
	sheet := CombatSheet{Number: "004/2026", Title: "Боевой листок"}

	data, err := combatNoForm((*Server).renderCombatDocx)(context.Background(), nil, sheet,
		[]SystemCard{our}, []SystemCard{competitor}, nil)
	if err != nil {
		t.Fatalf("листок не собран: %v", err)
	}
	body := string(docxEntry(t, data, "word/document.xml"))
	rels := relsPartOf(t, data)
	for _, want := range []string{"Ссылки", "Страница нашего продукта", "Декларация соответствия"} {
		if !strings.Contains(body, want) {
			t.Fatalf("во встроенном виде нет %q:\n%s", want, body)
		}
	}
	for _, url := range []string{"https://nav.tn.ru/systems/our/", "https://epyur.fvds.ru/api/documents/file?key=documents%2Fd.pdf"} {
		if !strings.Contains(rels, url) {
			t.Fatalf("адрес %q не попал в отношения (ссылка не кликабельна):\n%s", url, rels)
		}
	}
	if strings.Contains(body, "Сайт конкурента") || strings.Contains(rels, "pirrogroup.ru") {
		t.Fatalf("ссылка конкурента попала в листок:\n%s", body)
	}
}

// Макет со своей меткой `{{ссылки}}` не должен получить второй список из
// встроенного вида: документ с двумя разделами «Ссылки» читается как ошибка.
func TestCombatSheetLayoutDoesNotDuplicateLinks(t *testing.T) {
	layout := layoutDocxFromXML(t, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:r><w:t>{{сравнение}}</w:t></w:r></w:p>
</w:body></w:document>`)
	our, competitor := combatTestCards()
	our.Refs = []skills.CardRef{{Label: "Страница нашего продукта", URL: "https://nav.tn.ru/systems/our/"}}
	sheet := CombatSheet{Number: "005/2026", Title: "Боевой листок"}

	data, err := combatNoForm((*Server).renderCombatDocx)(context.Background(), layout, sheet,
		[]SystemCard{our}, []SystemCard{competitor}, nil)
	if err != nil {
		t.Fatalf("листок по макету не собран: %v", err)
	}
	body := string(docxEntry(t, data, "word/document.xml"))
	if strings.Contains(body, "Страница нашего продукта") {
		t.Fatalf("встроенный раздел ссылок продублирован в макете:\n%s", body)
	}
	if !strings.Contains(body, "Продукты сравнения") {
		t.Fatalf("макет не получил тело листка:\n%s", body)
	}
}

func TestRenderCombatDocxNoPlaceholdersFromEmptyFields(t *testing.T) {
	// Пустые поля формы не должны ронять сборку — их просто нечем подставить.
	our, competitor := combatTestCards()
	sheet := CombatSheet{Number: "001/2026", Title: "Сравнение", FormValues: map[string]string{}}
	data, err := combatNoForm((*Server).renderCombatDocx)(context.Background(), nil, sheet, []SystemCard{our}, []SystemCard{competitor}, nil)
	if err != nil {
		t.Fatalf("сборка упала на пустых полях: %v", err)
	}
	xml := docXMLOf(t, data)
	if strings.Contains(xml, "{{") {
		t.Fatal("плейсхолдер без значения остался в документе")
	}
}

func TestRenderCombatDocxEmptyCompetitors(t *testing.T) {
	// Конкурентов не выбрали — документ всё равно собирается.
	our, _ := combatTestCards()
	sheet := CombatSheet{Number: "002/2026", Title: "Без конкурентов"}
	data, err := combatNoForm((*Server).renderCombatDocx)(context.Background(), nil, sheet, []SystemCard{our}, nil, nil)
	if err != nil {
		t.Fatalf("сборка без конкурентов упала: %v", err)
	}
	xml := docXMLOf(t, data)
	if !strings.Contains(xml, "Наша система") {
		t.Fatal("наш продукт обязан остаться в документе")
	}
}
