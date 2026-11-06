package main

import (
	"strings"
	"testing"
)

// Картинка системы в печати по макету .docx (2026-11-06).
//
// Симптом: в макете `temple.docx` инструкция просит «вставляй в текст
// изображения составов и материалов, приведённые в базе знаний», но у ответа
// модели в печати один выход — текст. Узел `image` с источником
// system/material существовал только в блочном пути формы, и картинка
// карточки в документ не попадала НИКОГДА, хотя файл в Базе знаний лежит и
// ключ вложения находится (замер: карточка 5874 → вложение 4489, 289797 байт).
//
// Метка картинки в макете: `{{картинка}}`, `{{картинка: система}}`,
// `{{картинка: материал: 2}}`, `{{картинка: 1}}`. Вид решает содержимое
// (правило 2026-11-06), поэтому метка не спорит с промтом.

// layoutImageContext — контекст печати с одной карточкой, у которой есть картинка.
func layoutImageContext(imageKey string) layoutContext {
	cards := []SystemCard{{
		SKU:       "TN-FND",
		Title:     "ТН-ФУНДАМЕНТ Дренаж Проф",
		ImageKey:  imageKey,
		ImageName: "pic.png",
	}}
	return layoutContext{
		values: map[string]string{},
		prompt: func(string) string { return "Текст." },
		cards:  func() []SystemCard { return cards },
	}
}

// imageLayoutDoc — макет с одной меткой.
func imageLayoutDoc(t *testing.T, marker string) []byte {
	t.Helper()
	return layoutDocxFromXML(t,
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:r><w:t>`+marker+`</w:t></w:r></w:p>
</w:body></w:document>`)
}

// TestLayoutImageTokenPrintsPicture — метка картинки печатает изображение.
func TestLayoutImageTokenPrintsPicture(t *testing.T) {
	lc := layoutImageContext("kb/pic.png")
	lc.images = &fakeImages{data: pngBytes()}
	data, err := renderLayoutDocx(imageLayoutDoc(t, "{{картинка: система}}"), lc)
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	body := string(docxEntry(t, data, "word/document.xml"))
	if !strings.Contains(body, "<w:drawing") {
		t.Fatal("картинка не вставлена")
	}
	if strings.Contains(body, "картинка: система") || strings.Contains(body, "{{") {
		t.Fatal("метка картинки напечатана текстом")
	}
	if !hasMediaPart(t, data) {
		t.Fatal("файл картинки не добавлен в пакет")
	}
}

// hasMediaPart — есть ли в пакете файл изображения и связь на него.
func hasMediaPart(t *testing.T, data []byte) bool {
	t.Helper()
	for _, part := range zipReaderOf(t, data) {
		if strings.HasPrefix(part.name, "word/media/") && len(part.data) > 0 {
			return true
		}
	}
	return false
}

// TestLayoutImageTokenWithoutImageIsSilent — нет картинки в карточке: метка
// молчит, а не печатает свои скобки заказчику.
func TestLayoutImageTokenWithoutImageIsSilent(t *testing.T) {
	lc := layoutImageContext("") // карточка без ключа картинки
	data, err := renderLayoutDocx(imageLayoutDoc(t, "{{картинка}}"), lc)
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	body := string(docxEntry(t, data, "word/document.xml"))
	if strings.Contains(body, "картинка") || strings.Contains(body, "{{") {
		t.Fatal("метка картинки без картинки напечатана заказчику")
	}
	if strings.Contains(body, "<w:drawing") {
		t.Fatal("картинки не было, а изображение вставлено")
	}
}

// TestLayoutImageTokenIsNotPrompt — «картинка» не уходит в модель как промт.
func TestLayoutImageTokenIsNotPrompt(t *testing.T) {
	lc := layoutImageContext("kb/pic.png")
	lc.images = &fakeImages{data: pngBytes()}
	called := false
	lc.prompt = func(string) string { called = true; return "модель" }
	if _, err := renderLayoutDocx(imageLayoutDoc(t, "{{картинка}}"), lc); err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	if called {
		t.Fatal("метку картинки отправили в модель как промт")
	}
}

// TestLayoutImageTokenWithOtherText — текст вокруг метки сохраняется: метка
// заменяет только саму себя.
func TestLayoutImageTokenWithOtherText(t *testing.T) {
	lc := layoutImageContext("kb/pic.png")
	lc.images = &fakeImages{data: pngBytes()}
	data, err := renderLayoutDocx(imageLayoutDoc(t, "Разрез системы: {{картинка}}."), lc)
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	body := string(docxEntry(t, data, "word/document.xml"))
	if !strings.Contains(body, "Разрез системы") {
		t.Fatal("текст вокруг метки пропал")
	}
	if !strings.Contains(body, "<w:drawing") {
		t.Fatal("картинка не вставлена")
	}
}

// TestLayoutWithoutMediaHasNoMediaPart — пакет без картинок не обрастает
// лишними частями: документ, в котором нет метки, собирается как раньше.
func TestLayoutWithoutMediaHasNoMediaPart(t *testing.T) {
	lc := layoutImageContext("kb/pic.png")
	lc.images = &fakeImages{data: pngBytes()}
	data, err := renderLayoutDocx(imageLayoutDoc(t, "Просто текст."), lc)
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	for _, part := range zipReaderOf(t, data) {
		if strings.Contains(part.name, "media/offerimg") {
			t.Fatalf("без метки в пакет добавлена картинка: %s", part.name)
		}
	}
}

// imageBytes — минимальные байты, которые shell картинки принимает как файл.
func pngBytes() []byte {
	return []byte("\x89PNG\r\n\x1a\n fake png bytes for layout test")
}
