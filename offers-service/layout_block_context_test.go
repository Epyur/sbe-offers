// Проверка поведения: печать по МАКЕТУ с блоком формы.
//
// Живой дефект стенда (2026-11-13): макет с блоком `{{block:97}}`, внутри
// которого стоят `{{система}}` или `{{текст: система: description}}`,
// печатал ПУСТОЙ абзац. Причина — путь «макет → блок» не передавал карточки
// систем в контекст блока: метка внутри блока оставалась без данных, хотя в
// самом макете та же метка печаталась.
//
// Тест идёт тем же путём, что и боевая сборка: renderLayoutDocx с
// layoutContext, где cards переданы функцией, и формой, у которой есть блок.
package main

import (
	"strings"
	"testing"
)

// layoutWithBlockOf — документ макета с одной меткой блока.
func layoutWithBlockOf(blockID string) []byte {
	doc := wordDoc(`<w:p><w:r><w:t>{{block:` + blockID + `}}</w:t></w:r></w:p>`)
	return layoutDocxFromXML(&testing.T{}, doc)
}

func TestLayoutBlockPrintsSystemCardFromLayoutPass(t *testing.T) {
	offer := Offer{Number: "001/2026", Systems: []OfferSystem{{NoteID: 100}}}
	card := SystemCard{
		NoteID:      100,
		Title:       "ТН-Кровля Классик",
		Description: "Классическая кровля.",
		Composition: []string{"Мембрана ПВХ"},
		Refs:        nil,
	}
	form := OfferForm{Blocks: []Block{{
		ID: "97", Title: "Система", ShowInClient: true,
		Content: []byte(`[{"type":"paragraph","text":"{{система}}"}]`),
	}}}
	lc := layoutContext{
		values: map[string]string{},
		offer:  offer,
		cards:  func() []SystemCard { return []SystemCard{card} },
		form:   func() OfferForm { return form },
		images: &fakeImages{data: pngBytes()},
	}
	out, err := renderLayoutDocx(layoutWithBlockOf("97"), lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	xml := string(docxEntry(t, out, "word/document.xml"))
	if !strings.Contains(xml, "Состав системы") {
		t.Fatalf("карточка системы внутри блока макета не напечатана:\n%s", xml)
	}
}

func TestLayoutBlockPrintsSystemTextFromLayoutPass(t *testing.T) {
	offer := Offer{Number: "002/2026", Systems: []OfferSystem{{NoteID: 100}}}
	card := SystemCard{NoteID: 100, Title: "ТН-Кровля Классик"}
	form := OfferForm{Blocks: []Block{{
		ID: "98", Title: "Текст", ShowInClient: true,
		Content: []byte(`[{"type":"paragraph","text":"{{текст: система: description}}"}]`),
	}}}
	lc := layoutContext{
		values: map[string]string{},
		offer:  offer,
		cards:  func() []SystemCard { return []SystemCard{card} },
		form:   func() OfferForm { return form },
		systemTexts: func(c SystemCard) map[string]string {
			return map[string]string{systemTextDescription: "Обоснование из хранилища."}
		},
		images: &fakeImages{data: pngBytes()},
	}
	out, err := renderLayoutDocx(layoutWithBlockOf("98"), lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	xml := string(docxEntry(t, out, "word/document.xml"))
	if !strings.Contains(xml, "Обоснование из хранилища.") {
		t.Fatalf("текст системы внутри блока макета не напечатан:\n%s", xml)
	}
}
