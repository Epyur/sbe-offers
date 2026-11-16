package main

// Проверки инструментов шага 4 (2026-11-16): `fetch_url` и `parse_file` через
// agent-service, плюс граница в КОДЕ — ссылку называет человек, файлы только
// из вложения хода (`agentEnv.Attachments`), а без настроенного сервиса —
// отказ, а не пустой ответ. Модель и настоящая сеть в эти тесты не входят.

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// agentToolsAfterStep4 — 17 имён, и схемы модели знают про новые.
func TestAgentToolsAfterStep4(t *testing.T) {
	server := &Server{}
	tools := server.agentTools()
	if len(tools) != 17 {
		t.Fatalf("по плану шаг 4 даёт 17 инструментов, а в карте %d", len(tools))
	}
	for _, name := range []string{"fetch_url", "parse_file"} {
		if _, ok := tools[name]; !ok {
			t.Fatalf("инструмента %q нет в карте", name)
		}
	}
	descriptors := server.agentToolDescriptors()
	names := map[string]bool{}
	for _, descriptor := range descriptors {
		if name, _ := descriptor["name"].(string); name != "" {
			names[name] = true
		}
	}
	if !names["fetch_url"] || !names["parse_file"] {
		t.Fatalf("новые инструменты не попали к модели: %v", names)
	}
	if strings.TrimSpace(tools["fetch_url"].Description) == "" || strings.TrimSpace(tools["parse_file"].Description) == "" {
		t.Fatal("у новых инструментов нет описания: модель не узнает границу")
	}
}

// Без настроенного сервиса агента обе ручки отвечают отказом, а не пустым
// результатом: «на сервере ничего нет» менеджер понял бы неверно.
func TestFetchAndParseRefuseWithoutAgentService(t *testing.T) {
	t.Setenv("AGENT_SERVICE_URL", "")
	t.Setenv("AGENT_INTERNAL_SECRET", "")
	server := &Server{}

	env := &agentEnv{Ctx: context.Background(), Server: server, Attachments: []agentAttachment{{Name: "a.docx", Data: []byte("x")}}}
	for _, name := range []string{"fetch_url", "parse_file"} {
		args := map[string]any{"url": "https://example.com"}
		if name == "parse_file" {
			args = map[string]any{}
		}
		result, err := server.agentTools()[name].Run(context.Background(), env, args)
		if err != nil {
			t.Fatalf("%s вернул ошибку вместо отказа: %v", name, err)
		}
		if result.OK || !strings.Contains(result.Error, "сервис агента не настроен") {
			t.Fatalf("%s: ждали отказ про ненастроенный сервис, получили %+v", name, result)
		}
	}
}

// Приложения нет — `parse_file` отказывает честно и не зовёт сервис.
func TestParseFileWithoutAttachment(t *testing.T) {
	t.Setenv("AGENT_INTERNAL_SECRET", "test-agent-secret")
	server := &Server{}
	env := &agentEnv{Ctx: context.Background(), Server: server}
	result, err := server.agentTools()["parse_file"].Run(context.Background(), env, map[string]any{})
	if err != nil {
		t.Fatalf("parse_file: %v", err)
	}
	if result.OK || !strings.Contains(result.Error, "файл не приложен") {
		t.Fatalf("ждали отказ про отсутствующее вложение: %+v", result)
	}
}

// Один файл в ходе разбирается целиком, без угадывания пути; HTML-ответ
// фетча приходит модели компактным (скрипты/ссылки/текст, без сырого HTML).
func TestFetchAndParseWithStub(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/agent/fetch", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Service-Secret") != "test-agent-secret" {
			http.Error(w, `{"error":"bad service secret"}`, http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":200,"content_type":"text/html; charset=utf-8","text":"<html><head><script src=\"/js/data.js\"></script></head><body><a href=\"/api/list\">Список</a><p>Таблица реестра</p></body></html>"}`))
	})
	mux.HandleFunc("/internal/agent/file/parse", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Service-Secret") != "test-agent-secret" {
			http.Error(w, `{"error":"bad service secret"}`, http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"docx","text":"Протокол испытаний"}`))
	})
	stub := httptest.NewServer(mux)
	defer stub.Close()
	t.Setenv("AGENT_SERVICE_URL", stub.URL)
	t.Setenv("AGENT_INTERNAL_SECRET", "test-agent-secret")
	server := &Server{}

	env := &agentEnv{
		Ctx:         context.Background(),
		Server:      server,
		Attachments: []agentAttachment{{Name: "protocol.docx", Data: []byte("PK\x03\x04bytes")}},
	}
	fetched, err := server.agentTools()["fetch_url"].Run(context.Background(), env, map[string]any{"url": "https://example.com/reestr"})
	if err != nil || !fetched.OK {
		t.Fatalf("fetch_url: %+v err=%v", fetched, err)
	}
	view, _ := fetched.Data.(map[string]any)["view"].(string)
	for _, want := range []string{"/js/data.js", "/api/list", "Таблица реестра"} {
		if !strings.Contains(view, want) {
			t.Fatalf("компактный вид не содержит %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "<script") || strings.Contains(view, "<p>") {
		t.Fatalf("сырой HTML утёк в контекст модели:\n%s", view)
	}

	parsed, err := server.agentTools()["parse_file"].Run(context.Background(), env, map[string]any{})
	if err != nil || !parsed.OK {
		t.Fatalf("parse_file: %+v err=%v", parsed, err)
	}
	if text, _ := parsed.Data.(map[string]any)["text"].(string); text != "Протокол испытаний" {
		t.Fatalf("текст файла не дошёл: %q", text)
	}
}

// Вложение хода кодируется из base64 с проверкой: пустой файл и битый base64
// — отказ с понятным текстом, имя файла — то, что назвал человек.
func TestDecodeAgentAttachments(t *testing.T) {
	data := []byte("содержимое")
	out, err := decodeAgentAttachments([]botAgentAttachment{{Name: " данные.xlsx ", Base64: base64.StdEncoding.EncodeToString(data)}})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 1 || out[0].Name != "данные.xlsx" || string(out[0].Data) != string(data) {
		t.Fatalf("вложение разобрано неверно: %+v", out)
	}
	if _, err := decodeAgentAttachments([]botAgentAttachment{{Name: "a.pdf", Base64: ""}}); err == nil {
		t.Fatal("пустой файл должен быть отказом")
	}
	if _, err := decodeAgentAttachments([]botAgentAttachment{{Name: "", Base64: "AAAA"}}); err == nil {
		t.Fatal("файл без имени должен быть отказом")
	}
	if _, err := decodeAgentAttachments([]botAgentAttachment{{Name: "a.pdf", Base64: "%%%"}}); err == nil {
		t.Fatal("битый base64 должен быть отказом")
	}
}
