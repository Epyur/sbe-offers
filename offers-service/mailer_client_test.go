package main

// Клиент писем агента КП (2026-11-14, шаг P5b). Без БД и без модели: только
// то, что определяет границу привязки и «отказ вместо пустого списка».

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Без секрета клиент не ходит вовсе: `configured() == false` — отказ, а не
// пустой список (пустой список агент пересказал бы как «писем нет»).
func TestMailerClientNotConfigured(t *testing.T) {
	t.Setenv("MAILER_SERVICE_URL", "http://backend:3000")
	t.Setenv("MAILER_INTERNAL_SECRET", "")
	client := NewMailerAgentClient()
	if client.configured() {
		t.Fatal("без секрета клиент считает себя настроенным")
	}
	list, err := client.Emails(context.Background(), "", 0)
	if err != errMailerNotConfigured {
		t.Fatalf("ждали отказ %v, получили %v", errMailerNotConfigured, err)
	}
	if list.Items == nil {
		t.Fatal("items должен быть пустым массивом, а не nil")
	}
}

// Заголовок секрета уходит на ручку, пустой `items: null` от ручки читается
// как пустой массив, а `attachments: null` — как пустой массив вложений.
func TestMailerClientListHeaderAndNullItems(t *testing.T) {
	seen := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/mailer/agent/emails", func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Get("X-Service-Secret")
		_, _ = w.Write([]byte(`{"source":"mailer","total":0,"items":null}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	t.Setenv("MAILER_SERVICE_URL", server.URL)
	t.Setenv("MAILER_INTERNAL_SECRET", "secret-42")

	client := NewMailerAgentClient()
	list, err := client.Emails(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("список не прочитан: %v", err)
	}
	if got := <-seen; got != "secret-42" {
		t.Fatalf("секрет не доехал: %q", got)
	}
	if list.Items == nil || len(list.Items) != 0 {
		t.Fatalf("items:null не стал пустым массивом: %+v", list.Items)
	}
}

// Ответ 403 несёт текст сервера — иначе агент не сможет сказать человеку,
// что связь настроена неверно.
func TestMailerClientForbiddenText(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/mailer/agent/emails", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"bad service secret"}`, http.StatusForbidden)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	t.Setenv("MAILER_SERVICE_URL", server.URL)
	t.Setenv("MAILER_INTERNAL_SECRET", "wrong")

	_, err := NewMailerAgentClient().Emails(context.Background(), "", 0)
	if err == nil {
		t.Fatal("403 должен быть ошибкой")
	}
	if !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "bad service secret") {
		t.Fatalf("ошибка потеряла код или текст сервера: %v", err)
	}
}
