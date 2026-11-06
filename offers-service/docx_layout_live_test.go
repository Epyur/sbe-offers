package main

import (
	"os"
	"strings"
	"testing"

	"mailers/offers/skills"
)

// Живая проверка на настоящем макете, настоящем ответе модели и КАРТИНКЕ
// (2026-11-06).
//
//	OFFERS_LIVE_LAYOUT — макет temple.docx;
//	OFFERS_LIVE_ANSWER — ответ модели из form_values КП;
//	OFFERS_LIVE_IMAGE  — файл картинки карточки (необязателен).
//
// Файлы не входят в репозиторий: это документ заказчика и его данные.
func TestLiveLayoutRendersLiveAnswer(t *testing.T) {
	master := os.Getenv("OFFERS_LIVE_LAYOUT")
	answerPath := os.Getenv("OFFERS_LIVE_ANSWER")
	if master == "" || answerPath == "" {
		t.Skip("OFFERS_LIVE_LAYOUT / OFFERS_LIVE_ANSWER не заданы")
	}
	layout, err := os.ReadFile(master)
	if err != nil {
		t.Fatalf("чтение макета: %v", err)
	}
	raw, err := os.ReadFile(answerPath)
	if err != nil {
		t.Fatalf("чтение ответа модели: %v", err)
	}
	answer := string(raw)

	imagePath := os.Getenv("OFFERS_LIVE_IMAGE")
	var imageData []byte
	if imagePath != "" {
		if imageData, err = os.ReadFile(imagePath); err != nil {
			t.Fatalf("чтение картинки: %v", err)
		}
	}

	// В живой макет добавляется метка картинки: в самом temple.docx её нет,
	// и без неё проверка картинки ничего бы не проверяла.
	withImageMarker := insertImageMarker(t, layout)
	cards := []SystemCard{{Title: "ТН-ФУНДАМЕНТ Дренаж Проф", ImageKey: "kb/pic.png", ImageName: "pic.png"}}

	out, err := renderLayoutDocx(withImageMarker, layoutContext{
		values: map[string]string{"object_name": "Дом"},
		prompt: func(string) string { return answer },
		cards:  func() []SystemCard { return cards },
		images: &fakeImages{data: imageData},
	})
	if err != nil {
		t.Fatalf("печать по макету: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	rels := string(partOf(t, out, "word/_rels/document.xml.rels"))

	// 1. Ссылки: каждый адрес ответа стал ссылкой Word с внешним отношением.
	urls := countURLsIn(answer)
	if urls == 0 {
		t.Fatal("в живом ответе нет адресов — проверка ссылок ничего не значит")
	}
	if got := strings.Count(body, "<w:hyperlink"); got < urls {
		t.Fatalf("ссылок в документе %d, адресов в ответе %d", got, urls)
	}
	if !strings.Contains(rels, `TargetMode="External"`) {
		t.Fatal("отношения ссылок не внешние")
	}

	// 2. Верхние индексы: «м2» в ответе печатается с надстрочной двойкой.
	if strings.Count(answer, "м2") == 0 {
		t.Fatal("в живом ответе нет «м2» — проверка индексов ничего не значит")
	}
	if !strings.Contains(body, `<w:vertAlign w:val="superscript"/>`) {
		t.Fatal("ни одной надстрочной степени: конвейер не применился")
	}

	// 3. Картинка: метка превратилась в изображение пакета.
	if len(imageData) > 0 {
		if !strings.Contains(body, "<w:drawing") {
			t.Fatal("метка картинки не вставила изображение")
		}
		if !hasMediaPart(t, out) {
			t.Fatal("файл картинки не в пакете")
		}
	}

	// 4. Остатки разметки не доходят до читателя.
	for _, bad := range []string{"[[", "]]", "**", "{{", "}}"} {
		if strings.Contains(body, bad) {
			t.Errorf("в документе осталась разметка %q", bad)
		}
	}
}

// insertImageMarker ставит метку картинки в первый пустой абзац макета.
//
// Проверка нужна на настоящем файле, а метки картинки в нём нет: администратор
// её поставит сам. Вставляется байтово, чтобы не переписывать пакет.
func insertImageMarker(t *testing.T, layout []byte) []byte {
	t.Helper()
	files, names, err := readZipEntries(layout)
	if err != nil {
		t.Fatalf("разбор макета: %v", err)
	}
	doc := string(files["word/document.xml"])
	// Первый абзац после «ОПИСАНИЕ ПРЕДЛАГАЕМЫХ СИСТЕМ» — описание; метку
	// картинки ставим сразу после заголовка, как это сделал бы человек.
	const marker = `{картинка}`
	at := strings.Index(doc, marker)
	if at >= 0 {
		return layout
	}
	para := `<w:p><w:r><w:t>` + marker + `</w:t></w:r></w:p>`
	pos := strings.Index(doc, "</w:body>")
	if pos < 0 {
		t.Fatal("в макете нет </w:body>")
	}
	doc = doc[:pos] + para + doc[pos:]
	files["word/document.xml"] = []byte(doc)
	out, err := zipFiles(files, names)
	if err != nil {
		t.Fatalf("сборка макета: %v", err)
	}
	return out
}

// countURLsIn — сколько адресов в тексте, без повторов.
func countURLsIn(text string) int {
	seen := map[string]bool{}
	for _, m := range skills.FindURLs(text) {
		seen[text[m[0]:m[1]]] = true
	}
	return len(seen)
}
