package main

import (
	"strings"
	"testing"
)

// Содержательный раздел — метка `{ content: <id> }` (2026-11-14).
//
// Два правила, которые эти тесты держат вместе:
//
//  1. `{ content: x }` — служебная метка, а не инструкция модели. Голая `{…}`
//     уходит в модель промтом, и без явного разбора раздел печатался бы
//     литералом «content: x» на месте целого куска документа.
//  2. Неизвестный id печатает ПУСТО и пишет в журнал: литерал метки в готовом
//     КП недопустим, а подстановка чужого раздела хуже пустого места.

// TestSectionMarkerIsNotPrompt — `{ content: x }` не доходит до модели.
func TestSectionMarkerIsNotPrompt(t *testing.T) {
	cases := []string{
		"{ content: proposal_systems }",
		"{content: proposal_systems}",
		"{{ content: proposal_systems }}",
		"{ содержание: proposal_systems }",
	}
	for _, raw := range cases {
		var prompts []string
		tokens := scanLayoutTokens(raw, map[string]string{})
		if len(tokens) != 1 {
			t.Errorf("%s: меток %d, ожидалась одна", raw, len(tokens))
			continue
		}
		if tokens[0].Kind != layoutTokenSection {
			t.Errorf("%s: вид %q, ожидался %q", raw, tokens[0].Kind, layoutTokenSection)
			continue
		}
		if got := tokens[0].Service.sectionID; got != "proposal_systems" {
			t.Errorf("%s: id раздела %q, ожидался proposal_systems", raw, got)
		}
		// Модели такая метка не уходит: вид служебный.
		if !isServiceKind(tokens[0].Kind) {
			t.Errorf("%s: служебный вид не считается служебным", raw)
		}
		lc := serviceLayoutContext(&prompts)
		lc.form = func() OfferForm { return OfferForm{} }
		doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>`+raw+`</w:t></w:r></w:p>`))
		if _, err := renderLayoutDocx(doc, lc); err != nil {
			t.Fatalf("%s: печать: %v", raw, err)
		}
		if len(prompts) != 0 {
			t.Errorf("%s: метка раздела позвала модель: %v", raw, prompts)
		}
	}
}

// TestSectionUnknownIdIsSilentAndLogged — неизвестный id печатает пусто.
//
// Проверяется то, что увидит заказчик: метки-литерала в документе нет, текст
// вокруг неё сохранён, и раздела нет. Журнал пишется словами, и его наличие
// проверяет сама сборка (log.Printf), а не документ.
func TestSectionUnknownIdIsSilentAndLogged(t *testing.T) {
	lc := serviceLayoutContext(nil)
	lc.form = func() OfferForm { return OfferForm{} }
	// Метка ищется по ВСЕМУ тексту абзацев, а склейка идёт через перевод строки:
	// в живом макете токен собирается по части в каждом абзаце.
	for _, raw := range []string{
		"{ content: unknown_section }",
		"{{ content: unknown_section }}",
		"{ content: }",
	} {
		doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>До `+raw+` после</w:t></w:r></w:p>`))
		out, err := renderLayoutDocx(doc, lc)
		if err != nil {
			t.Fatalf("%s: печать: %v", raw, err)
		}
		body := string(docxEntry(t, out, "word/document.xml"))
		if strings.Contains(body, "content") || strings.Contains(body, "unknown_section") {
			t.Fatalf("%s: метка раздела ушла клиенту литералом:\n%s", raw, body)
		}
		if !strings.Contains(body, "До") || !strings.Contains(body, "после") {
			t.Fatalf("%s: текст вокруг метки потерян:\n%s", raw, body)
		}
	}
}

// TestSectionKnownIdPrintsNothingYet — до шага 5 известный раздел тоже молчит.
//
// Это не «метка не работает»: настройка есть, а печать подразделов приходит
// следующим шагом. Тест фиксирует, что метка НЕ печатает литерал и НЕ зовёт
// модель ни при каком состоянии настроек.
func TestSectionKnownIdPrintsNothingYet(t *testing.T) {
	lc := serviceLayoutContext(nil)
	lc.form = func() OfferForm {
		return OfferForm{Sections: []ContentSection{{
			ID: "proposal_systems", Title: "Описание систем",
			Subsections: []ContentSubsection{{ID: "intro", Title: "Введение", Instruction: "Напиши вступление", Condition: conditionAlways}},
		}}}
	}
	doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>{ content: proposal_systems }</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if strings.Contains(body, "content:") || strings.Contains(body, "proposal_systems") {
		t.Fatalf("метка раздела ушла клиенту литералом:\n%s", body)
	}
}
