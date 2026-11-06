package main

import (
	"os"
	"strings"
	"testing"
)

// Прогон живого ответа через ВСЮ цепочку: cleanBlockText → печать по макету.
//
// Проверяет итоговый документ, а не промежуточную строку: именно итог видел
// заказчик в КП 003/2026 (2026-11-06) — с `##`, с английским рассуждением и
// одним сплошным абзацем.
func TestLayoutEndToEndWithLiveAnswer(t *testing.T) {
	layoutPath := os.Getenv("OFFERS_LIVE_LAYOUT")
	if layoutPath == "" {
		t.Skip("OFFERS_LIVE_LAYOUT не задан")
	}
	layout, err := os.ReadFile(layoutPath)
	if err != nil {
		t.Fatalf("чтение макета: %v", err)
	}
	if !hasLivePrompt(string(mustLayoutDocXML(t, layout))) {
		t.Skip("подан готовый документ — нужен макет с метками")
	}
	cleaned := cleanBlockText(liveLayoutAnswer)
	if cleaned == "" {
		t.Fatal("ответ модели очистился в пустоту")
	}
	out, err := renderLayoutDocx(layout, layoutContext{
		values: map[string]string{"object_name": "Дом"},
		prompt: func(string) string { return cleaned },
	})
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	for _, bad := range []string{"##", "Given the risk", "I'll frame"} {
		if strings.Contains(body, bad) {
			t.Errorf("в готовом документе осталось %q", bad)
		}
	}
	// Абзацев должно стать много: заголовки и абзацы ответа не слипаются.
	if paragraphs := len(reWordParagraph.FindAllString(body, -1)); paragraphs < 5 {
		t.Errorf("текст лёг %d абзацами, ожидалось не меньше 5", paragraphs)
	}
}

func mustLayoutDocXML(t *testing.T, layout []byte) []byte {
	t.Helper()
	files, _, err := readZipEntries(layout)
	if err != nil {
		t.Fatalf("разбор макета: %v", err)
	}
	return files["word/document.xml"]
}
