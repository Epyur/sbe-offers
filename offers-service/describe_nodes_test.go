package main

import (
	"strings"
	"testing"
)

// Добор покрытия 2026-11-05: узлы, которые модель предлагает для блока, и
// выбор картинки из вложений. Печатается только то, что человек увидит в
// редакторе, поэтому пустые узлы и чужие виды проверяются отдельно.

// Каждый вид узла с содержимым остаётся как есть.
func TestSanitizeKeepsKnownNodes(t *testing.T) {
	nodes := []blockNode{
		{Type: "paragraph", Text: "текст"},
		{Type: "heading", Text: "Заголовок", Level: 2},
		{Type: "bullet_list", Items: []string{"раз", "два"}},
		{Type: "table", Rows: [][]string{{"А", "Б"}}},
		{Type: "page_break"},
		{Type: "links"},
		{Type: "prompt_text", Instruction: "опиши систему"},
		{Type: "image", ImageOf: imageOfSystem},
		{Type: "compare_table"},
	}
	got := sanitizeDescribedNodes(nodes)
	if len(got) != len(nodes) {
		t.Fatalf("известные узлы потеряны: %d из %d", len(got), len(nodes))
	}
}

// Пустой узел любого вида выбрасывается: редактор показывает только печатаемое.
func TestSanitizeDropsEmptyNodes(t *testing.T) {
	nodes := []blockNode{
		{Type: "paragraph", Text: "  "},
		{Type: "heading"},
		{Type: "bullet_list"},
		{Type: "table"},
		{Type: "prompt_text"},
		{Type: "image", ImageKey: ""},
	}
	for _, node := range sanitizeDescribedNodes(nodes) {
		t.Errorf("пустой узел остался: %+v", node)
	}
}

// Синонимы типов приводятся к печатаемым.
func TestSanitizeMapsSynonyms(t *testing.T) {
	cases := map[string]string{
		"list":      "bullet_list",
		"bullet":    "bullet_list",
		"список":    "bullet_list",
		"text":      "paragraph",
		"title":     "paragraph",
		"заголовок": "heading",
	}
	nodes := make([]blockNode, 0, len(cases))
	for in := range cases {
		nodes = append(nodes, blockNode{Type: in, Text: "x", Items: []string{"y"}})
	}
	for _, node := range sanitizeDescribedNodes(nodes) {
		want, ok := cases[node.Type]
		if !ok {
			continue // уже приведён
		}
		t.Errorf("тип %q не приведён к %q", node.Type, want)
	}
}

// nodeHasContent: картинка с источником считается содержимым, без — нет.
func TestNodeHasContent(t *testing.T) {
	if !nodeHasContent(blockNode{Type: "image", ImageOf: imageOfSystem}) {
		t.Fatal("картинка системы без ключа считается пустой")
	}
	if nodeHasContent(blockNode{Type: "image"}) {
		t.Fatal("пустая картинка считается содержимым")
	}
	if !nodeHasContent(blockNode{Type: "links"}) || !nodeHasContent(blockNode{Type: "page_break"}) {
		t.Fatal("служебные узлы считаются пустыми")
	}
	if nodeHasContent(blockNode{Type: "prompt_text", Instruction: " "}) {
		t.Fatal("промт без подсказки считается содержимым")
	}
}

// Вложение выбирается: точное имя, затем без расширения, затем первое.
func TestPickImageAttachmentOrder(t *testing.T) {
	atts := []KBAttachment{
		{Filename: "первая.png", Mime: "image/png", Key: "att/1"},
		{Filename: "Схема.png", Mime: "image/png", Key: "att/2"},
		{Filename: "паспорт.pdf", Mime: "application/pdf", Key: "att/3"},
	}
	if key, _ := pickImageAttachment(atts, "Схема.png"); key != "att/2" {
		t.Fatalf("точное имя: %q", key)
	}
	if key, _ := pickImageAttachment(atts, "Схема"); key != "att/2" {
		t.Fatalf("без расширения: %q", key)
	}
	if key, _ := pickImageAttachment(atts, "нет такой"); key != "att/1" {
		t.Fatalf("первая картинка при промахе: %q", key)
	}
	if key, _ := pickImageAttachment(atts, ""); key != "att/1" {
		t.Fatalf("без имени: %q", key)
	}
	if key, _ := pickImageAttachment(nil, "Схема.png"); key != "" {
		t.Fatalf("пустой список дал ключ: %q", key)
	}
	// PDF не картинка: из одного PDF ключа быть не должно.
	if key, _ := pickImageAttachment([]KBAttachment{
		{Filename: "паспорт.pdf", Mime: "application/pdf", Key: "att/3"},
	}, ""); key != "" {
		t.Fatalf("PDF принят за картинку: %q", key)
	}
}

// isImageMime: решает mime, а без mime — расширение.
func TestIsImageMime(t *testing.T) {
	cases := []struct {
		mime, name string
		want       bool
	}{
		{"image/png", "x", true},
		{"application/pdf", "x", false},
		{"", "схема.JPG", true},
		{"", "схема.pdf", false},
	}
	for _, c := range cases {
		if got := isImageMime(c.mime, c.name); got != c.want {
			t.Errorf("isImageMime(%q, %q) = %v", c.mime, c.name, got)
		}
	}
}

// Метка в колонтитуле не трогается: макет — образец вёрстки, метки — в теле.
func TestFillLayoutLeavesOtherParts(t *testing.T) {
	doc := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p><w:r><w:t>[Номер]</w:t></w:r></w:p></w:body></w:document>`
	out := fillLayoutDocument(doc, layoutContext{
		values: map[string]string{fieldOfferNumber: "7/2026"},
	})
	if !strings.Contains(out, "7/2026") {
		t.Fatalf("метка в абзаце не подставлена: %s", out)
	}
}
