package main

import (
	"context"
	"strings"
	"testing"
)

// «Опиши блок словами» (2026-11-05).
//
// Администратор пишет, что должно быть в блоке, модель отдаёт готовые узлы.
// Проверяем: ответ модели разбирается терпимо, чужие виды узлов приводятся к
// печатаемым, пустой ответ — ошибка, а не молчаливый пустой блок.

// fakeCompleter живёт в briefstep_test.go: там же проверяется разговор брифа.
// Своя копия здесь разъехалась бы с ним при первой же правке сигнатуры.

// Ответ модели в обёртке разбирается и отдаёт готовый блок.
func TestDescribeBlockParsesReply(t *testing.T) {
	c := &fakeCompleter{answer: "Вот блок:\n```json\n" +
		`{"title":"Условия","nodes":[` +
		`{"type":"heading","text":"Условия поставки","level":3},` +
		`{"type":"bullet_list","items":["Доставка включена","Оплата по счёту"]}` +
		`]}` + "\n```"}
	resp, err := describeBlock(context.Background(), "заголовок и два пункта", []string{"object_name"}, c)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if resp.Title != "Условия" {
		t.Fatalf("заголовок блока: %q", resp.Title)
	}
	if len(resp.Nodes) != 2 {
		t.Fatalf("узлов %d: %+v", len(resp.Nodes), resp.Nodes)
	}
	if resp.Nodes[1].Type != "bullet_list" || len(resp.Nodes[1].Items) != 2 {
		t.Fatalf("список разобран неверно: %+v", resp.Nodes[1])
	}
}

// Чужой вид узла приводится к абзацу: иначе блок молча не напечатается.
func TestDescribeBlockNormalizesUnknownNodeType(t *testing.T) {
	c := &fakeCompleter{answer: `{"title":"Блок","nodes":[{"type":"text","text":"Обычный текст"}]}`}
	resp, err := describeBlock(context.Background(), "абзац", nil, c)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(resp.Nodes) != 1 || resp.Nodes[0].Type != "paragraph" {
		t.Fatalf("чужой вид узла не приведён: %+v", resp.Nodes)
	}
	if resp.Nodes[0].Text != "Обычный текст" {
		t.Fatalf("текст потерян: %+v", resp.Nodes[0])
	}
}

// Синоним списка `list` становится `bullet_list`.
func TestDescribeBlockMapsListSynonym(t *testing.T) {
	c := &fakeCompleter{answer: `{"nodes":[{"type":"list","items":["раз","два"]}]}`}
	resp, err := describeBlock(context.Background(), "список", nil, c)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if resp.Nodes[0].Type != "bullet_list" {
		t.Fatalf("синоним списка не понят: %+v", resp.Nodes[0])
	}
}

// Пустые узлы выбрасываются: редактор показывает только то, что напечатается.
func TestDescribeBlockDropsEmptyNodes(t *testing.T) {
	c := &fakeCompleter{answer: `{"nodes":[{"type":"paragraph","text":"  "},{"type":"paragraph","text":"Текст"}]}`}
	resp, err := describeBlock(context.Background(), "абзац", nil, c)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(resp.Nodes) != 1 {
		t.Fatalf("пустой узел не убран: %+v", resp.Nodes)
	}
}

// Ни одного годного узла — ошибка, а не пустой блок.
func TestDescribeBlockEmptyReplyIsError(t *testing.T) {
	c := &fakeCompleter{answer: `{"nodes":[]}`}
	if _, err := describeBlock(context.Background(), "блок", nil, c); err == nil {
		t.Fatal("пустой ответ модели принят за блок")
	}
}

// Модель недоступна — ошибка наверх, человек настроит блок руками.
func TestDescribeBlockModelError(t *testing.T) {
	c := &fakeCompleter{err: errLLMNotConfigured}
	if _, err := describeBlock(context.Background(), "блок", nil, c); err == nil {
		t.Fatal("недоступная модель не дала ошибки")
	}
}

// Текст администратора уходит модели как есть.
func TestDescribeBlockPassesDescription(t *testing.T) {
	c := &fakeCompleter{answer: `{"nodes":[{"type":"paragraph","text":"x"}]}`}
	_, err := describeBlock(context.Background(), "таблица с составом и два абзаца", nil, c)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if !strings.Contains(c.lastUser, "таблица с составом") {
		t.Fatalf("описание администратора не ушло модели: %q", c.lastUser)
	}
}
