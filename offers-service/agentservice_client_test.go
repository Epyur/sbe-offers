package main

// Проверки клиента agent-service (2026-11-16, шаг 3 плана
// 2026-11-16-sbe-offers-combat-sections-and-agent-tools-plan.md).
//
// Проверяем ГРАНИЦУ, а не чужой сервис: заголовок секрета уходит (без него
// любая ручка ответит 403), `configured()` без окружения — отказ, а не пустой
// ответ, и ошибка сервиса превращается в текст ошибки, а не в панику.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newAgentStub поднимает подставной agent-service: ручки проверяют секрет и
// отдают записанные ответы.
func newAgentStub(t *testing.T, seen *http.Header, fetchBody, parseBody, renderBody string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	jsonStub := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if seen != nil {
				*seen = r.Header.Clone()
			}
			if r.Header.Get("X-Service-Secret") != "test-agent-secret" {
				http.Error(w, `{"error":"bad service secret"}`, http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, body)
		}
	}
	mux.HandleFunc("/internal/agent/fetch", jsonStub(fetchBody))
	mux.HandleFunc("/internal/agent/file/parse", jsonStub(parseBody))
	mux.HandleFunc("/internal/agent/render", jsonStub(renderBody))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// Заголовок секрета обязан уходить на ВСЕ три ручки; иначе agent-service
// ответит 403 (проверка «пустой секрет = связь не настроена» там своя).
func TestAgentServiceClientSendsSecret(t *testing.T) {
	var seen http.Header
	stub := newAgentStub(t, &seen,
		`{"status":200,"content_type":"text/html","text":"ok"}`,
		`{"kind":"json","data":{"ok":true}}`,
		`{"format":"png","base64":"AAAA","width":10,"height":20}`)
	t.Setenv("AGENT_SERVICE_URL", stub.URL)
	t.Setenv("AGENT_INTERNAL_SECRET", "test-agent-secret")
	client := NewAgentServiceClient()

	if _, err := client.Fetch(context.Background(), AgentFetchRequest{URL: "https://example.com"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got := seen.Get("X-Service-Secret"); got != "test-agent-secret" {
		t.Fatalf("fetch: секрет не ушёл, заголовок = %q", got)
	}
	if _, err := client.ParseFile(context.Background(), "data.json", []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if got := seen.Get("X-Service-Secret"); got != "test-agent-secret" {
		t.Fatalf("parse: секрет не ушёл, заголовок = %q", got)
	}
	if _, err := client.Render(context.Background(), "mermaid", "graph TD;A-->B"); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := seen.Get("X-Service-Secret"); got != "test-agent-secret" {
		t.Fatalf("render: секрет не ушёл, заголовок = %q", got)
	}
}

// Без окружения клиент отказывает, а не отдаёт пустой результат.
func TestAgentServiceClientNotConfigured(t *testing.T) {
	t.Setenv("AGENT_SERVICE_URL", "")
	t.Setenv("AGENT_INTERNAL_SECRET", "")
	client := NewAgentServiceClient()
	if client.configured() {
		t.Fatal("без секрета configured() должен быть false")
	}
	if _, err := client.ParseFile(context.Background(), "data.json", []byte("{}")); err != errAgentServiceNotConfigured {
		t.Fatalf("ParseFile без настройки: ждали errAgentServiceNotConfigured, получили %v", err)
	}
	if _, err := client.Fetch(context.Background(), AgentFetchRequest{URL: "https://example.com"}); err != errAgentServiceNotConfigured {
		t.Fatalf("Fetch без настройки: ждали errAgentServiceNotConfigured, получили %v", err)
	}
	if _, err := client.Render(context.Background(), "mermaid", "graph TD"); err != errAgentServiceNotConfigured {
		t.Fatalf("Render без настройки: ждали errAgentServiceNotConfigured, получили %v", err)
	}
}

// Ошибка сервиса — текст ошибки с его кодом и причиной, а не паника и не
// пустой результат (модель должна сказать менеджеру правду).
func TestAgentServiceClientErrorText(t *testing.T) {
	stub := newAgentStub(t, nil, ``, ``, ``)
	t.Setenv("AGENT_SERVICE_URL", stub.URL)
	t.Setenv("AGENT_INTERNAL_SECRET", "wrong-secret")
	client := NewAgentServiceClient()

	_, err := client.Render(context.Background(), "mermaid", "graph TD")
	if err == nil {
		t.Fatal("ждали ошибку при чужом секрете")
	}
	if !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "bad service secret") {
		t.Fatalf("в ошибке нет кода и причины сервиса: %v", err)
	}
}

// Разбор файла отвечает тем, что прислал сервис (и `data:null` не ломает).
func TestAgentServiceClientParseResult(t *testing.T) {
	stub := newAgentStub(t, nil, ``, `{"kind":"json","data":null}`, ``)
	t.Setenv("AGENT_SERVICE_URL", stub.URL)
	t.Setenv("AGENT_INTERNAL_SECRET", "test-agent-secret")
	client := NewAgentServiceClient()

	out, err := client.ParseFile(context.Background(), "data.json", []byte(`null`))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if out.Kind != "json" {
		t.Fatalf("kind = %q", out.Kind)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), `"data":null`) {
		t.Fatalf("ожидали пустое поле data, а не явный null: %s", raw)
	}
}
