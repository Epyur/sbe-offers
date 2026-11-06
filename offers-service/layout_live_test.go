package main

import (
	"os"
	"strings"
	"testing"
)

// Сквозная проверка на ЖИВОМ макете 003/2026 (2026-11-06).
//
// Файл в вашем рабочем каталоге, в репозиторий он не входит: это документ
// заказчика. Тест включается переменной OFFERS_LIVE_LAYOUT и сверяет, что
// инструкция модели из реального абзаца находится и текст модели печатается
// вместо неё.
func TestLiveLayoutRendersPrompt(t *testing.T) {
	path := os.Getenv("OFFERS_LIVE_LAYOUT")
	if path == "" {
		t.Skip("OFFERS_LIVE_LAYOUT не задан")
	}
	layout, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("чтение макета: %v", err)
	}
	var got string
	out, err := renderLayoutDocx(layout, layoutContext{
		values: map[string]string{"object_name": "Дом"},
		prompt: func(instruction string) string {
			got = instruction
			return "ОПИСАНИЕ ОТ МОДЕЛИ."
		},
	})
	if err != nil {
		t.Fatalf("печать по макету: %v", err)
	}
	if !strings.Contains(got, "вставь последовательное описание всех систем") {
		t.Fatalf("промт макета не найден, модель получила: %q", got)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if !strings.Contains(body, "ОПИСАНИЕ ОТ МОДЕЛИ.") {
		t.Fatal("текст модели не попал в документ")
	}
	if strings.Contains(body, "вставь последовательное описание") {
		t.Fatal("текст промта остался в документе литералом")
	}
}
