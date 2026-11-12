package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// Материал как объект предложения (2026-11-13, R3).
//
// Повод — решение пользователя: материал можно предложить не только позицией
// с ценой, но и ОПИСАНИЕМ, как систему. Блок `repeat:"materials"` печатает
// выбранные материалы, и у блока ОДИН плейсхолдер — второй сломал бы вёрстку
// (форма одного файла-образца).

// TestMaterialChoiceRoundTrip — `material_choice` переживает запись и чтение.
func TestMaterialChoiceRoundTrip(t *testing.T) {
	offer := Offer{Materials: OfferMaterials{{NoteID: 7547}, {SKU: "MAT-КРОВ-ПВХ-0001"}}}
	raw, err := json.Marshal(offer)
	if err != nil {
		t.Fatalf("кодирование: %v", err)
	}
	if !strings.Contains(string(raw), `"materials":[`) {
		t.Fatalf("материалы не попали в JSON: %s", raw)
	}
	var back Offer
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if len(back.Materials) != 2 || back.Materials[0].NoteID != 7547 {
		t.Fatalf("материалы прочитались не так: %+v", back.Materials)
	}
}

// TestOfferWithoutMaterialsReadsAsEmpty — старая КП без поля material_choice.
func TestOfferWithoutMaterialsReadsAsEmpty(t *testing.T) {
	var offer Offer
	if err := json.Unmarshal([]byte(`{"id":1,"systems":[{"noteId":5}]}`), &offer); err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if len(offer.Materials) != 0 {
		t.Fatalf("пустой список материалов должен быть пустым, получили %#v", offer.Materials)
	}
	raw, err := json.Marshal(offer)
	if err != nil {
		t.Fatalf("кодирование: %v", err)
	}
	if !strings.Contains(string(raw), `"materials":[]`) {
		t.Fatalf("пустой выбор должен печататься как [], а не null: %s", raw)
	}
}

// TestMaterialChoiceAddRemoveDoesNotTouchSystems — два списка независимы.
func TestMaterialChoiceAddRemoveDoesNotTouchSystems(t *testing.T) {
	offer := Offer{Systems: OfferSystems{OfferSystem{NoteID: 1}}}
	offer.Materials = offer.Materials.Add(Ref{NoteID: 7547})
	offer.Materials = offer.Materials.Add(Ref{NoteID: 7547})
	if len(offer.Materials) != 1 {
		t.Fatalf("повторный выбор добавил второй материал: %+v", offer.Materials)
	}
	if len(offer.Systems) != 1 {
		t.Fatalf("выбор материала изменил системы: %+v", offer.Systems)
	}
	offer.Materials = offer.Materials.Remove(Ref{NoteID: 7547})
	if len(offer.Materials) != 0 || len(offer.Systems) != 1 {
		t.Fatalf("снятие материала задело системы: %+v / %+v", offer.Materials, offer.Systems)
	}
}

// TestMaterialChoiceRemoveByNoteAndSKU — снятие идёт и по заметке, и по коду:
// выбор в предложении помнит оба значения, и снимающий может прислать любое.
func TestMaterialChoiceRemoveByNoteAndSKU(t *testing.T) {
	chosen := OfferMaterials{{SKU: "MAT-ПРОЧ-ОБЩ-0001", NoteID: 103}, {NoteID: 6363}}
	if got := removeChoiceFromList(chosen, Ref{NoteID: 103}); len(got) != 1 || got[0].NoteID != 6363 {
		t.Fatalf("снятие по заметке не нашло материал: %+v", got)
	}
	if got := removeChoiceFromList(chosen, Ref{SKU: "mat-проч-общ-0001"}); len(got) != 1 || got[0].NoteID != 6363 {
		t.Fatalf("снятие по коду не нашло материал: %+v", got)
	}
}

// removeChoiceFromList — то же правило, что у сервера: выбор дополняется
// известными значениями записи перед снятием.
func removeChoiceFromList(list OfferMaterials, choice Ref) OfferMaterials {
	for _, existing := range list {
		if existing.Same(choice) || sameRefCard(choice, SystemCard{NoteID: existing.NoteID, SKU: existing.SKU}) {
			return list.Remove(existing)
		}
	}
	return list
}

// TestMaterialsForOfferMatchesByNoteAndSKU — карточка находится и по заметке,
// и по коду: у живых материалов заполнено то одно, то другое.
func TestMaterialsForOfferMatchesByNoteAndSKU(t *testing.T) {
	cards := []SystemCard{
		{NoteID: 7547, SKU: "MAT-КРОВ-ПВХ-0001", Title: "Мембрана LOGICROOF"},
		{NoteID: 6363, SKU: "MAT-УТЕП-ПИР-0002", Title: "Плита PIR"},
	}
	byNote := materialsForOffer(cards, RefList{{NoteID: 6363}})
	if len(byNote) != 1 || byNote[0].Title != "Плита PIR" {
		t.Fatalf("материал по заметке не найден: %+v", byNote)
	}
	bySKU := materialsForOffer(cards, RefList{{SKU: "mat-кров-пвх-0001"}})
	if len(bySKU) != 1 || bySKU[0].Title != "Мембрана LOGICROOF" {
		t.Fatalf("материал по коду не найден (регистр не важен): %+v", bySKU)
	}
	if got := materialsForOffer(cards, RefList{{NoteID: 999}}); len(got) != 0 {
		t.Fatalf("несуществующий материал попал в печать: %+v", got)
	}
}

// TestLayoutRepeatOverChosenMaterials — повтор печатает карточки ВЫБРАННЫХ
// материалов: в блоке `repeat:"materials"` метка `{{материал}}` берёт материал
// прохода, а не позицию предложения.
func TestLayoutRepeatOverChosenMaterials(t *testing.T) {
	lc := serviceLayoutContext(nil)
	lc.offer.Materials = OfferMaterials{{NoteID: 7547}, {NoteID: 6363}}
	lc.offer.Items = []OfferItem{{Name: "Мембрана V-SL 2 мм"}, {Name: "Геотекстиль"}}
	lc.materialCards = func() []SystemCard {
		return []SystemCard{
			{NoteID: 7547, Title: "Мембрана LOGICROOF", Description: "ПВХ-мембрана для кровли."},
			{NoteID: 6363, Title: "Плита PIR", Description: "Жёсткий утеплитель."},
		}
	}
	lc.form = func() OfferForm {
		block := Block{ID: "1", Repeat: blockRepeatMaterials,
			Content: []byte(`[{"type":"paragraph","text":"{{материал}}"}]`)}
		return OfferForm{Blocks: []Block{block}}
	}
	doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>{{block:1}}</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	for _, want := range []string{"Мембрана LOGICROOF", "Плита PIR"} {
		if !strings.Contains(body, want) {
			t.Fatalf("материал не напечатан: %q\n%s", want, body)
		}
	}
	if strings.Contains(body, "Геотекстиль") || strings.Contains(body, "Мембрана V-SL") {
		t.Fatalf("повтор по материалам напечатал ПОЗИЦИИ вместо материалов:\n%s", body)
	}
}

// TestLayoutRepeatOverChosenMaterialsPrintsCard — метка карточки печатает
// выбранный материал, а не первую систему предложения.
func TestLayoutRepeatOverChosenMaterialsPrintsCard(t *testing.T) {
	lc := serviceLayoutContext(nil)
	lc.offer.Materials = OfferMaterials{{NoteID: 7547}}
	lc.materialCards = func() []SystemCard {
		return []SystemCard{{NoteID: 7547, Title: "Мембрана LOGICROOF", Description: "ПВХ-мембрана."}}
	}
	lc.form = func() OfferForm {
		block := Block{ID: "1", Repeat: blockRepeatMaterials,
			Content: []byte(`[{"type":"paragraph","text":"{{система: таблица}}"}]`)}
		return OfferForm{Blocks: []Block{block}}
	}
	doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>{{block:1}}</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if strings.Contains(body, "Слой P") || strings.Contains(body, "Слой V") {
		t.Fatalf("в повторе по материалам напечатана система:\n%s", body)
	}
}

// TestLayoutRepeatOverMaterialsWithoutChoicePrintsPositions — старые КП, где
// материалов-объектов нет вовсе, печатаются по позициям, как раньше.
func TestLayoutRepeatOverMaterialsWithoutChoicePrintsPositions(t *testing.T) {
	lc := serviceLayoutContext(nil)
	lc.offer.Materials = nil
	lc.materialCards = func() []SystemCard { return nil }
	lc.form = func() OfferForm {
		block := Block{ID: "1", Repeat: blockRepeatMaterials,
			Content: []byte(`[{"type":"paragraph","text":"{{материал}}"}]`)}
		return OfferForm{Blocks: []Block{block}}
	}
	doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>{{block:1}}</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if !strings.Contains(body, "Геотекстиль") {
		t.Fatalf("позиция не напечатана без выбранных материалов:\n%s", body)
	}
}

// TestLayoutRepeatMaterialsZeroPrintsNothing — ноль материалов не даёт пустого
// заголовка и пустой страницы (требование пользователя к одному плейсхолдеру).
func TestLayoutRepeatMaterialsZeroPrintsNothing(t *testing.T) {
	lc := serviceLayoutContext(nil)
	lc.offer.Materials = nil
	lc.offer.Items = nil
	lc.materialCards = func() []SystemCard { return nil }
	// Образец формы — чистая форма без посторонних блоков: проверяем ровно
	// блок повтора, а не соседние разделы.
	lc.form = func() OfferForm {
		block := Block{ID: "1", Repeat: blockRepeatMaterials,
			Content: []byte(`[{"type":"paragraph","text":"{{материал}}"}]`)}
		return OfferForm{Blocks: []Block{block}}
	}
	rows, _ := layoutRepeatRows(lc, "1")
	if len(rows) != 0 {
		t.Fatalf("блок без материалов сделал %d проходов", len(rows))
	}
	// Блок вообще не печатается: пустой заголовок и пустая страница в документе
	// выглядят сломанными.
	doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>{{block:1}}</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if strings.Contains(body, "<w:t") {
		t.Fatalf("блок без материалов напечатал текст:\n%s", body)
	}
}
