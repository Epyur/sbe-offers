package main

import (
	"os"
	"strings"
	"testing"
)

// Сквозная проверка на ЖИВОМ макете 003/2026 (2026-11-06).
//
// Файл в вашем рабочем каталоге, в репозиторий он не входит: это документ
// заказчика. Тест включается переменной OFFERS_LIVE_LAYOUT.
//
// Переменная может указывать и на МАКЕТ (документ с метками `{{…}}`), и на
// ГОТОВОЕ КП (документ, собранный из него): в каталоге выгрузок лежит второе.
// Для макета проверяется, что инструкция модели находится и её заменяет
// напечатанный текст; для готового КП — что в нём нет литерала метки и нет
// маркдаун-разметки от модели.
func TestLiveLayoutRendersPrompt(t *testing.T) {
	path := os.Getenv("OFFERS_LIVE_LAYOUT")
	if path == "" {
		t.Skip("OFFERS_LIVE_LAYOUT не задан")
	}
	layout, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("чтение документа: %v", err)
	}
	files, _, err := readZipEntries(layout)
	if err != nil {
		t.Fatalf("разбор документа: %v", err)
	}
	if !hasLivePrompt(string(files["word/document.xml"])) {
		t.Skip("подан готовый документ (текст промта уже заменён) — проверять нечего")
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

// Живая выгрузка: собранный документ не должен нести ни литерала метки, ни
// маркдаун-разметки модели, ни строки-рассуждения по-английски.
func TestLiveExportIsClean(t *testing.T) {
	path := os.Getenv("OFFERS_LIVE_EXPORT")
	if path == "" {
		t.Skip("OFFERS_LIVE_EXPORT не задан")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("чтение выгрузки: %v", err)
	}
	body := string(docxEntry(t, data, "word/document.xml"))
	for _, bad := range []string{"{{", "}}", "##", "Given the risk", "Let me", "The task"} {
		if strings.Contains(body, bad) {
			t.Errorf("в документе осталось %q", bad)
		}
	}
}

// hasLivePrompt — есть ли в тексте документа живой промт макета. Word режет
// текст абзаца на прогоны, поэтому проверяется склейка, а не один `<w:t>`.
func hasLivePrompt(docXML string) bool {
	return strings.Contains(layoutPlainText(docXML), "вставь последовательное описание")
}
