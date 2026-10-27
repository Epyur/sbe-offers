package main

import "testing"

// Форма, у которой поля и блоки пришли из БД, НЕ должна подменяться формой по
// умолчанию: раньше проверка «пусто» стояла до разбора JSON и сравнивала
// nil-слайс, поэтому сохранённая форма не доходила ни до редактора, ни до DOCX
// (живой баг: админ настроил форму, а в файле — шаблон по умолчанию).
func TestResolvedFormKeepsSavedContent(t *testing.T) {
	saved := OfferForm{
		ID:    1,
		Mode:  "client",
		Title: "КОММЕРЧЕСКОЕ ПРЕДЛОЖЕНИЕ",
		Placeholders: []Placeholder{
			{ID: "client_name", Label: "Клиент"},
		},
		Blocks: []Block{
			{ID: "intro", Title: "Вступление", ShowInClient: true,
				Content: []byte(`[{"type":"paragraph","text":"Уважаемый(ая) {{client_name}}!"}]`)},
		},
	}
	if saved.Title == "" || len(saved.Blocks) == 0 {
		t.Fatal("форма должна оставаться с содержимым")
	}

	// Рендер должен использовать переданную форму, а не шаблон по умолчанию.
	offer := Offer{
		Number:     "001/2026",
		CreatedAt:  "2026-10-27T10:00:00.000Z",
		OwnerEmail: "m@tn.ru",
		Items:      []OfferItem{{Name: "Позиция", Unit: "шт", Qty: 1, Price: 10, Sum: 10}},
		Total:      10,
		FormValues: map[string]string{"client_name": "ООО Тест"},
	}
	data, err := renderOfferDocx(saved, offer)
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	docXML := docXMLOf(t, data)

	if !contains(docXML, "КОММЕРЧЕСКОЕ ПРЕДЛОЖЕНИЕ") {
		t.Fatal("заголовок сохранённой формы не попал в документ — отрисован шаблон по умолчанию")
	}
	if !contains(docXML, "Уважаемый(ая) ООО Тест!") {
		t.Fatal("блок формы с подставленным полем не попал в документ")
	}
	if contains(docXML, "{{client_name}}") {
		t.Fatal("плейсхолдер остался незаполненным")
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
