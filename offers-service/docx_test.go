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
	for _, want := range []string{`w:val="Heading1"`, "Заголовок", `w:jc w:val="center"`, `numId w:val="1"`, "<w:tr>"} {
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
