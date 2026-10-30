package main

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestRenderOfferDocxRejectsEmptyOffer(t *testing.T) {
	_, err := renderOfferDocx(defaultForm("client"), Offer{Number: "001/2026"})
	if err == nil {
		t.Fatal("ожидали ошибку для предложения без позиций")
	}
	if !strings.Contains(err.Error(), "нет позиций") {
		t.Fatalf("текст ошибки должен быть понятным, получили %q", err.Error())
	}
}

func TestRenderOfferDocxEscapesClientName(t *testing.T) {
	form := defaultForm("client")
	offer := Offer{
		Number:     "001/2026",
		CreatedAt:  "2026-10-26T10:00:00.000Z",
		OwnerEmail: "manager@tn.ru",
		Client:     OfferClient{Name: `ООО "Ромашка" & <Ко>`},
		Items: []OfferItem{
			{Name: "Система <кровля>", Unit: "м²", Qty: 10, Price: 500, Sum: 5000},
		},
		FormValues: map[string]string{"conditions": "оплата & доставка", "valid_until": "01.12.2026"},
		Total:      5000,
	}

	data, err := renderOfferDocx(form, offer)
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("результат не является zip/docx: %v", err)
	}
	var docXML string
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("открыть document.xml: %v", err)
			}
			raw, _ := io.ReadAll(rc)
			rc.Close()
			docXML = string(raw)
		}
	}
	if docXML == "" {
		t.Fatal("в документе нет word/document.xml")
	}

	if strings.Contains(docXML, "<Ко>") {
		t.Fatal("«<» из названия клиента превратилось в тег — экранирование не работает")
	}
	if !strings.Contains(docXML, "&lt;Ко&gt;") {
		t.Fatal("не нашли экранированный фрагмент названия клиента")
	}
	if !strings.Contains(docXML, "&amp;") {
		t.Fatal("амперсанд должен быть экранирован")
	}
	if !strings.Contains(docXML, "Ромашка") {
		t.Fatal("кириллица потерялась в документе")
	}
	if !strings.Contains(docXML, "5 000,00") {
		t.Fatal("итог должен быть в русском формате «5 000,00»")
	}
}

func TestResolvePlaceholders(t *testing.T) {
	values := map[string]string{"client_name": "ООО Ромашка", "conditions": "оплата по счёту"}
	if got := resolvePlaceholders("Клиент: {{client_name}}", values); got != "Клиент: ООО Ромашка" {
		t.Fatalf("подстановка не сработала: %q", got)
	}
	if got := resolvePlaceholders("Поле {{unknown}} осталось", values); got != "Поле  осталось" {
		t.Fatalf("неизвестное поле должно выпадать: %q", got)
	}
	if got := resolvePlaceholders("без полей", values); got != "без полей" {
		t.Fatalf("текст без полей не должен меняться: %q", got)
	}
}

func TestFormatMoney(t *testing.T) {
	cases := map[float64]string{
		0:         "0,00",
		1500:      "1 500,00",
		1234567.5: "1 234 567,50",
		999.99:    "999,99",
	}
	for in, want := range cases {
		if got := formatMoney(in); got != want {
			t.Fatalf("formatMoney(%v) = %q, ожидали %q", in, got, want)
		}
	}
}

func TestRenderBlockContentAllNodeTypes(t *testing.T) {
	block := Block{
		ID:    "test",
		Title: "Тест",
		Content: []byte(`[
			{"type":"heading","level":2,"text":"Заголовок"},
			{"type":"paragraph","align":"center","text":"Абзац"},
			{"type":"bullet_list","items":["раз","два"]},
			{"type":"table","rows":[["A","B"],["1","2"]]}
		]`),
	}
	out, err := renderBlockContent(block, map[string]string{})
	if err != nil {
		t.Fatalf("разбор блока упал: %v", err)
	}
	// Стиль и нумерация — бланка: "1" это styleId «heading 1», а numId 6 —
	// маркированный список шаблона. До 2026-10-29 здесь стояли Heading1 и
	// numId=1: таких стилей в бланке нет, и Word показывал текст дефолтным
	// шрифтом (в этом и была «битая вёрстка»).
	for _, want := range []string{`w:val="1"`, "Заголовок", `w:jc w:val="center"`, `numId w:val="6"`, "<w:tr>"} {
		if !strings.Contains(out, want) {
			t.Fatalf("в выводе блока нет %q", want)
		}
	}
}

func TestRenderBlockContentRejectsBrokenJSON(t *testing.T) {
	block := Block{ID: "bad", Title: "Плохой", Content: []byte(`{не json`)}
	if _, err := renderBlockContent(block, map[string]string{}); err == nil {
		t.Fatal("ожидали ошибку разбора битого блока")
	} else if !strings.Contains(err.Error(), "Плохой") {
		t.Fatalf("в ошибке должно быть название блока, получили %q", err.Error())
	}
}

func TestRenderedDocxKeepsTemplateParts(t *testing.T) {
	form := defaultForm("client")
	offer := Offer{Number: "001/2026", CreatedAt: "2026-10-26T10:00:00.000Z", OwnerEmail: "m@tn.ru",
		Items:      []OfferItem{{Name: "X", Unit: "шт", Qty: 1, Price: 10, Sum: 10}},
		Total:      10,
		FormValues: map[string]string{}}
	data, err := renderOfferDocx(form, offer)
	if err != nil {
		t.Fatal(err)
	}
	zr, _ := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, want := range []string{"[Content_Types].xml", "word/document.xml", "word/header1.xml", "word/footer1.xml"} {
		if !names[want] {
			t.Fatalf("в результате нет части бланка %q", want)
		}
	}
	var docXML string
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, _ := f.Open()
			raw, _ := io.ReadAll(rc)
			rc.Close()
			docXML = string(raw)
		}
	}
	if strings.Contains(docXML, "{{Текст}}") {
		t.Fatal("заглушка бланка {{Текст}} осталась в документе")
	}
	if !strings.Contains(docXML, "sectPr") {
		t.Fatal("потерян sectPr — размеры страницы бланка не применились")
	}
}

func TestRenderOfferDocxWithoutCreatedAt(t *testing.T) {
	form := defaultForm("client")
	offer := Offer{Number: "001/2026", CreatedAt: "",
		Items: []OfferItem{{Name: "X", Unit: "шт", Qty: 1, Price: 10, Sum: 10}}, Total: 10}
	if _, err := renderOfferDocx(form, offer); err != nil {
		t.Fatalf("пустая дата создания не должна ронять рендер: %v", err)
	}
}

// Переключатели показа по умолчанию ВЫКЛЮЧЕНЫ: существующие предложения
// печатаются как раньше, без секций и сводки брифа (Task 11, 2026-10-30).
func TestDocxSectionCopyOffByDefault(t *testing.T) {
	offer := sampleOffer()
	offer.FormValues = map[string]string{"copy.intro": "Этот текст не должен печататься."}
	docx, err := renderOfferDocx(OfferForm{Title: "Коммерческое предложение"}, offer)
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	body := string(docxEntry(t, docx, "word/document.xml"))
	if strings.Contains(body, "Этот текст не должен печататься.") {
		t.Fatal("текст секции напечатан без включённого переключателя")
	}
}

// Сводка брифа включается своим ключом и печатает только заполненные поля.
func TestDocxBriefSummaryToggle(t *testing.T) {
	offer := sampleOffer()
	offer.FormValues = map[string]string{
		formFlagBriefSummary:   "1",
		"brief.object_name":    "ЖК «Поэма»",
		"brief.customer_pains": "протечки",
		"brief.timeline":       "",
	}
	docx, err := renderOfferDocx(OfferForm{Title: "Коммерческое предложение"}, offer)
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	body := string(docxEntry(t, docx, "word/document.xml"))
	if !strings.Contains(body, "ЖК «Поэма»") {
		t.Fatal("сводка брифа не попала в документ")
	}
	if strings.Contains(body, "Сроки:") {
		t.Fatal("пустое поле брифа напечатано")
	}
}

// Пустые значения формы — никакой паники: флаг читается из nil-карты.
func TestFormFlagOnNil(t *testing.T) {
	if formFlagOn(nil, formFlagBriefSummary) {
		t.Fatal("nil-карта должна давать «выключено»")
	}
	if !formFlagOn(map[string]string{formFlagBriefSummary: "true"}, formFlagBriefSummary) {
		t.Fatal("true не считается включённым")
	}
	if formFlagOn(map[string]string{formFlagBriefSummary: "0"}, formFlagBriefSummary) {
		t.Fatal("0 должен быть выключен")
	}
}
