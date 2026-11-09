package main

import (
	"strings"
	"testing"

	"mailers/offers/skills"
)

// Метки макета, которые читают ДАННЫЕ, а не модель (2026-11-06, шаг C плана
// «генерация без модели»).
//
// Повод — живая жалоба: администратор поменял структуру промтов в макете, и
// система «ушла думать и не вернулась». Причина была не в скорости модели, а в
// том, что модель звалась там, где данные уже есть. Эти тесты фиксируют
// границу: служебная метка печатается без единого вызова модели.

// serviceLayoutContext — контекст печати с карточкой, текстами и позициями.
func serviceLayoutContext(prompts *[]string) layoutContext {
	return layoutContext{
		values: map[string]string{"object_name": "МКД на ул. Примерной"},
		prompt: func(instruction string) string {
			if prompts != nil {
				*prompts = append(*prompts, instruction)
			}
			return "ТЕКСТ ОТ МОДЕЛИ"
		},
		offer: Offer{
			Number:  "003/2026",
			Systems: []OfferSystem{{NoteID: 5874}},
			Items: []OfferItem{
				{Name: "Мембрана V-SL 2 мм", Unit: "м2", Qty: 100, Price: 500, Sum: 50000},
				{Name: "Геотекстиль", Unit: "м2", Qty: 100, Price: 50, Sum: 5000},
			},
			Total: 55000,
		},
		images: &fakeImages{data: pngBytes()},
		cards: func() []SystemCard {
			return []SystemCard{{
				NoteID:         5874,
				Title:          "ТН-ФУНДАМЕНТ Дренаж Проф",
				Description:    "Система для подземной части.",
				Composition:    []string{"Мембрана Logicbase V-SL", "Геотекстиль", "Дренажная мембрана"},
				ImageKey:       "k/1",
				ImageName:      "schema.png",
				Specifications: [][]string{{"Показатель", "Значение"}, {"Срок службы", "50 лет"}},
			}}
		},
		systemTexts: func(card SystemCard) map[string]string {
			return map[string]string{
				systemTextDescription: "Обоснование применения в двух словах.",
				systemTextAdvantages:  "Преимущества коротко.",
			}
		},
	}
}

// Служебные метки не должны уходить в модель ни при каком написании.
func TestServiceMarkersScanAsService(t *testing.T) {
	cases := map[string]string{
		"{{block:1}}":                     layoutTokenBlock,
		"{{система}}":                     layoutTokenSystem,
		"{{система: таблица}}":            layoutTokenSystem,
		"{{ссылки}}":                      layoutTokenLinks,
		"{{ссылки: система}}":             layoutTokenLinks,
		"{{ссылки системы}}":              layoutTokenLinks,
		"{{характеристики: система}}":     layoutTokenSpecs,
		"{{характеристики системы}}":      layoutTokenSpecs,
		"{{текст: система: description}}": layoutTokenText,
		"{{текст: система: advantages}}":  layoutTokenText,
		"{{текст системы: обоснование}}":  layoutTokenText,
	}
	for raw, want := range cases {
		tokens := scanLayoutTokens(raw, map[string]string{})
		if len(tokens) != 1 {
			t.Errorf("%s: меток %d, ожидалась одна", raw, len(tokens))
			continue
		}
		if tokens[0].Kind != want {
			t.Errorf("%s: вид %q, ожидался %q", raw, tokens[0].Kind, want)
		}
	}
}

// Обычный промт служебной меткой не становится: слова-ключи проверяются по
// всему содержимому метки, а не по началу.
func TestPromptWithServiceWordIsStillPrompt(t *testing.T) {
	tokens := scanLayoutTokens("{{Опиши систему: состав, преимущества, ссылки.}}", nil)
	if len(tokens) != 1 || tokens[0].Kind != layoutTokenPrompt {
		t.Fatalf("инструкция принята за служебную метку: %+v", tokens)
	}
}

// Блок формы печатается по id, со всей вёрсткой, и не зовёт модель.
func TestLayoutBlockMarkerPrintsBlock(t *testing.T) {
	var prompts []string
	lc := serviceLayoutContext(&prompts)
	lc.form = func() OfferForm {
		return OfferForm{Blocks: []Block{
			{ID: "7", Title: "Объект", Content: []byte(`[{"type":"heading","text":"Объект исследования","level":2},{"type":"paragraph","text":"{{object_name}}"}]`)},
		}}
	}
	doc := layoutDocxFromXML(t, wordDoc(
		`<w:p><w:r><w:t>{{block:7}}</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if !strings.Contains(body, "Объект исследования") {
		t.Fatalf("блок не напечатан:\n%s", body)
	}
	if !strings.Contains(body, "МКД на ул. Примерной") {
		t.Fatalf("значение поля формы в блоке не подставлено:\n%s", body)
	}
	if len(prompts) != 0 {
		t.Fatalf("служебная метка позвала модель: %v", prompts)
	}
	if strings.Contains(body, "{{block") {
		t.Fatalf("в документе осталась служебная метка:\n%s", body)
	}
}

// Неизвестный id печатает ПУСТО: литерал `{{block:9}}` у клиента — дефект.
func TestLayoutBlockMarkerUnknownIdIsSilent(t *testing.T) {
	lc := serviceLayoutContext(nil)
	lc.form = func() OfferForm { return OfferForm{} }
	doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>До {{block:9}} после</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if strings.Contains(body, "block:9") || strings.Contains(body, "{{") {
		t.Fatalf("метка ушла клиенту литералом:\n%s", body)
	}
	if !strings.Contains(body, "До") || !strings.Contains(body, "после") {
		t.Fatalf("текст вокруг метки потерян:\n%s", body)
	}
}

// {{система}} — картинка и состав ОДНОЙ таблицей без границ.
func TestLayoutSystemMarkerIsBorderlessTable(t *testing.T) {
	var prompts []string
	lc := serviceLayoutContext(&prompts)
	doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>{{система}}</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if n := strings.Count(body, "<w:tbl>"); n != 1 {
		t.Fatalf("таблиц %d, ожидалась одна:\n%s", n, body)
	}
	for _, e := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
		if !strings.Contains(body, `<w:`+e+` w:val="none"`) {
			t.Errorf("у таблицы есть граница %q — по решению пользователя их нет вовсе", e)
		}
	}
	if !strings.Contains(body, "Мембрана Logicbase V-SL") {
		t.Fatalf("состав не напечатан:\n%s", body)
	}
	if !strings.Contains(body, "<w:drawing>") {
		t.Fatalf("картинка системы не напечатана:\n%s", body)
	}
	if len(prompts) != 0 {
		t.Fatalf("метка системы позвала модель: %v", prompts)
	}
}

// Карточка без картинки и карточка без состава вырождаются в одну ячейку —
// и это не ошибка: печатается то, что есть.
func TestLayoutSystemMarkerDegrades(t *testing.T) {
	cases := map[string]SystemCard{
		"без картинки": {NoteID: 1, Title: "Система", Composition: []string{"Слой A"}, ImageKey: ""},
		"без состава":  {NoteID: 2, Title: "Система", Composition: nil, ImageKey: "k/1", ImageName: "s.png"},
	}
	for name, card := range cases {
		lc := serviceLayoutContext(nil)
		lc.cards = func() []SystemCard { return []SystemCard{card} }
		lc.offer.Systems = []OfferSystem{{NoteID: card.NoteID}}
		doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>{{система}}</w:t></w:r></w:p>`))
		out, err := renderLayoutDocx(doc, lc)
		if err != nil {
			t.Fatalf("%s: печать: %v", name, err)
		}
		body := string(docxEntry(t, out, "word/document.xml"))
		if strings.Contains(body, "{{") {
			t.Fatalf("%s: метка ушла клиенту:\n%s", name, body)
		}
		if n := strings.Count(body, "<w:tbl>"); n != 1 {
			t.Fatalf("%s: таблиц %d, ожидалась одна", name, n)
		}
	}
}

// {{система: таблица}} — картинка и состав друг под другом, без таблицы.
func TestLayoutSystemTableVariant(t *testing.T) {
	lc := serviceLayoutContext(nil)
	doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>{{система: таблица}}</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if strings.Contains(body, "<w:tbl>") {
		t.Fatalf("вариант «таблица» нарисовал таблицу:\n%s", body)
	}
	if !strings.Contains(body, "Мембрана Logicbase V-SL") || !strings.Contains(body, "<w:drawing>") {
		t.Fatalf("картинка или состав потеряны:\n%s", body)
	}
}

// Состав системы — НУМЕРОВАННЫЙ список (2026-11-07).
//
// Живая жалоба: карточка Базы знаний нумерует слои («1 Мембрана», «2 Песок»), а
// на разрезе системы маркеры слоёв — с НОМЕРАМИ. В документе же состав шёл
// маркированным списком, и сопоставить разрез со строками было невозможно.
// Номер из карточки снимается при разборе, поэтому его ставит Word — так он идёт
// подряд и не задваивается в тексте.
//
// С 2026-11-08 состав получает СВОЙ numId из плана сборки (9000+), чтобы
// нумерация каждой системы шла с единицы; определение дописывается в
// numbering.xml со `<w:startOverride w:val="1"/>`.
func TestLayoutCompositionIsNumberedList(t *testing.T) {
	lc := serviceLayoutContext(nil)
	doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>{{система}}</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if !strings.Contains(body, `numId w:val="`+itoa(numberingUserBase)+`"`) {
		t.Fatalf("состав напечатан не нумерованным списком:\n%s", body)
	}
	if strings.Contains(body, `numId w:val="`+styleListBullet+`"`) {
		t.Fatalf("в составе остался маркированный список:\n%s", body)
	}
	numbering := string(docxEntry(t, out, "word/numbering.xml"))
	if !strings.Contains(numbering, `w:numId="`+itoa(numberingUserBase)+`"`) {
		t.Fatalf("в numbering.xml нет определения нашего списка:\n%s", numbering)
	}
	if !strings.Contains(numbering, `<w:startOverride w:val="1"/>`) {
		t.Fatalf("у нашего списка нет перезапуска нумерации:\n%s", numbering)
	}
}

// Тексты системы берутся из сохранённых, а не пишутся моделью.
func TestLayoutSystemTextMarkers(t *testing.T) {
	var prompts []string
	lc := serviceLayoutContext(&prompts)
	doc := layoutDocxFromXML(t, wordDoc(
		`<w:p><w:r><w:t>{{текст: система: description}}</w:t></w:r></w:p>`+
			`<w:p><w:r><w:t>{{текст системы: преимущества}}</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if !strings.Contains(body, "Обоснование применения в двух словах.") {
		t.Fatalf("обоснование не напечатано:\n%s", body)
	}
	if !strings.Contains(body, "Преимущества коротко.") {
		t.Fatalf("преимущества не напечатаны:\n%s", body)
	}
	if len(prompts) != 0 {
		t.Fatalf("метка текста позвала модель: %v", prompts)
	}
}

// Сохранённого текста нет — блок молчит. Пустая строка честнее черновика.
func TestLayoutSystemTextMissingIsSilent(t *testing.T) {
	lc := serviceLayoutContext(nil)
	lc.systemTexts = func(SystemCard) map[string]string { return nil }
	doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>{{текст: система: description}}</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if strings.Contains(body, "{{") || strings.Contains(body, "ТЕКСТ ОТ МОДЕЛИ") {
		t.Fatalf("метка с пустым текстом не молчит:\n%s", body)
	}
}

// Ссылки и характеристики — из карточки, без модели.
func TestLayoutCardDataMarkers(t *testing.T) {
	var prompts []string
	lc := serviceLayoutContext(&prompts)
	lc.cards = func() []SystemCard {
		return []SystemCard{{
			NoteID:         5874,
			Title:          "ТН-ФУНДАМЕНТ",
			Refs:           []skills.CardRef{{Label: "Страница системы", URL: "https://tn.ru/sys/1"}},
			Specifications: [][]string{{"Показатель", "Значение"}, {"Срок службы", "50 лет"}},
		}}
	}
	doc := layoutDocxFromXML(t, wordDoc(
		`<w:p><w:r><w:t>{{ссылки: система}}</w:t></w:r></w:p>`+
			`<w:p><w:r><w:t>{{характеристики: система}}</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	// Ссылка печатается подписью, адрес — только в отношении (2026-11-09).
	if !strings.Contains(body, "Страница системы") {
		t.Fatalf("подпись ссылки карточки не напечатана:\n%s", body)
	}
	if strings.Contains(body, "https://tn.ru/sys/1") {
		t.Fatalf("адрес попал в видимый текст вместо подписи:\n%s", body)
	}
	if rels := relsPartOf(t, out); !strings.Contains(rels, `Target="https://tn.ru/sys/1"`) {
		t.Fatalf("адрес не попал в отношения (ссылка нерабочая):\n%s", rels)
	}
	if !strings.Contains(body, "Срок службы") {
		t.Fatalf("характеристики карточки не напечатаны:\n%s", body)
	}
	if len(prompts) != 0 {
		t.Fatalf("метка данных позвала модель: %v", prompts)
	}
}

// Повтор по системам (шаг D): блок с repeat:"systems" печатается по разу на
// систему, и метки внутри берут данные СВОЕЙ системы.
func TestLayoutRepeatOverSystems(t *testing.T) {
	lc := serviceLayoutContext(nil)
	lc.cards = func() []SystemCard {
		return []SystemCard{
			{NoteID: 1, Title: "Первая", Composition: []string{"Слой P"}},
			{NoteID: 2, Title: "Вторая", Composition: []string{"Слой V"}},
		}
	}
	lc.offer.Systems = []OfferSystem{{NoteID: 1}, {NoteID: 2}}
	lc.systemTexts = func(card SystemCard) map[string]string {
		return map[string]string{systemTextDescription: "Описание: " + card.Title}
	}
	// Блок с повтором по системам: в нём метка сохранённого текста системы.
	lc.form = func() OfferForm {
		block := Block{ID: "1", Repeat: blockRepeatSystems,
			Content: []byte(`[{"type":"paragraph","text":"{{текст: система: description}}"}]`)}
		return OfferForm{Blocks: []Block{block}}
	}
	doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>{{block:1}}</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	// Каждая система получает СВОЙ текст — иначе вторая система повторила бы
	// первую, и читатель не заметил бы подмены.
	for _, want := range []string{"Описание: Первая", "Описание: Вторая"} {
		if !strings.Contains(body, want) {
			t.Fatalf("текст системы не напечатан: %q\n%s", want, body)
		}
	}
	if n := strings.Count(body, "Описание: "); n != 2 {
		t.Fatalf("блок напечатан %d раз, ожидалось два", n)
	}
}

// Повтор по материалам ограничен: выше предела печатается честная строка.
func TestLayoutRepeatOverMaterialsIsLimited(t *testing.T) {
	lc := serviceLayoutContext(nil)
	lc.offer.Items = nil
	for i := 0; i < 25; i++ {
		lc.offer.Items = append(lc.offer.Items, OfferItem{Name: "Позиция " + itoa(i)})
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
	if n := strings.Count(body, "Позиция "); n != layoutMaterialRepeatLimit {
		t.Fatalf("позиций напечатано %d, ожидалось %d", n, layoutMaterialRepeatLimit)
	}
	if !strings.Contains(body, "в приложении") {
		t.Fatalf("нет строки о том, что остальное — в приложении:\n%s", body)
	}
}

// wordDoc — минимальный document.xml вокруг списка абзацев.
func wordDoc(paragraphs string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		paragraphs + `</w:body></w:document>`
}
