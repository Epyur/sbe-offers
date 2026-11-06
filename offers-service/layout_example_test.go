package main

import (
	"strings"
	"testing"
)

// Образец макета (2026-11-06, шаг E) — не украшение справки, а рабочий файл.
//
// Тест держит главное обещание: образец собирается тем же кодом и печатается
// этой же системой. Если метка в образце разойдётся с разбором, здесь покраснеет
// — а не у администратора, который загрузил образец и получил скобки в КП.
func TestExampleLayoutPrintsEveryMarker(t *testing.T) {
	data, err := exampleLayoutDocx()
	if err != nil {
		t.Fatalf("сборка образца: %v", err)
	}
	lc := serviceLayoutContext(nil)
	lc.form = func() OfferForm {
		return OfferForm{Blocks: []Block{{ID: "1",
			Content: []byte(`[{"type":"paragraph","text":"Блок формы из образца"}]`)}}}
	}
	out, err := renderLayoutDocx(data, lc)
	if err != nil {
		t.Fatalf("печать по образцу: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	// Ни одной метки не осталось: всё распознано и подставлено/снято.
	for _, bad := range []string{"{{", "}}"} {
		if strings.Contains(body, bad) {
			t.Fatalf("в документе остались скобки метки %q — разбор разошёлся с образцом:\n%s", bad, body)
		}
	}
	// И подставляемые значения дошли.
	for _, want := range []string{
		"МКД на ул. Примерной", // [Наименование объекта]
		"Обоснование применения",
		"Преимущества",
		"Состав системы",
		"Блок формы из образца",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("в документе нет %q — метка образца не напечатала данные", want)
		}
	}
}

// Список образца и список разбора должны знать одни и те же виды меток: иначе
// администратор увидит в образце метку, которой печать не понимает.
func TestExampleMarkersAreAllRecognised(t *testing.T) {
	for _, item := range layoutExampleMarkers {
		tokens := scanLayoutTokens(item.marker, nil)
		if len(tokens) == 0 {
			t.Errorf("метка образца %q не распознана вовсе", item.marker)
			continue
		}
		if tokens[0].Kind == layoutTokenPrompt && strings.HasPrefix(strings.TrimSpace(item.marker), "{{") {
			// Двойная скобка, принятая за промт, — не ошибка, но у служебных
			// меток она обязана опознаваться как служебная.
			if isServiceKind(tokens[0].Kind) {
				t.Errorf("служебная метка образца %q ушла в модель промтом", item.marker)
			}
		}
	}
}
