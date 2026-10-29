package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeBotTokenTrimsWhitespace(t *testing.T) {
	cases := map[string]string{
		"  123456:AAH-x  ": "123456:AAH-x",
		"123456:AAH-x\n":   "123456:AAH-x",
		"\t  ":             "",
		"":                 "",
	}
	for raw, want := range cases {
		if got := normalizeBotToken(raw); got != want {
			t.Errorf("normalizeBotToken(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestLooksLikeBotToken(t *testing.T) {
	valid := []string{"123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw", "1:x"}
	invalid := []string{"", "abc:def", "123456789", ":AAH", "123456789:", "12a456:AAH"}

	for _, token := range valid {
		if !looksLikeBotToken(token) {
			t.Errorf("looksLikeBotToken(%q) = false, want true", token)
		}
	}
	for _, token := range invalid {
		if looksLikeBotToken(token) {
			t.Errorf("looksLikeBotToken(%q) = true, want false", token)
		}
	}
}

// Ответ плагину не должен содержать ни поля token, ни его значения: ручка
// отдаёт только «задан/не задан», чей бот и кто менял.
func TestBotConfigResponseHasNoTokenField(t *testing.T) {
	secret := "123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"
	res := botConfigResponse{Configured: true, BotUsername: "lpi_pm_bot", UpdatedBy: "owner@tn.ru"}
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "token") {
		t.Errorf("ответ содержит поле с токеном: %s", data)
	}
	if strings.Contains(string(data), secret) {
		t.Errorf("ответ содержит само значение токена")
	}
}

func TestVerifyTokenViaBotAcceptsGoodToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/verify" {
			t.Errorf("неожиданный путь %s", r.URL.Path)
		}
		if r.Header.Get("X-Service-Secret") != "test-bot-secret" {
			t.Errorf("нет секрета в запросе")
		}
		var req struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Token != "123456:AAH" {
			t.Errorf("бот получил чужой токен %q", req.Token)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "username": "offers_bot"})
	}))
	defer srv.Close()

	t.Setenv("OFFERS_BOT_URL", srv.URL)
	t.Setenv("OFFERS_BOT_SECRET", "test-bot-secret")

	username, err := (&Server{}).verifyTokenViaBot(t.Context(), "123456:AAH")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if username != "offers_bot" {
		t.Errorf("username = %q, want offers_bot", username)
	}
}

func TestVerifyTokenViaBotReportsTelegramError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "Unauthorized"})
	}))
	defer srv.Close()

	t.Setenv("OFFERS_BOT_URL", srv.URL)
	t.Setenv("OFFERS_BOT_SECRET", "test-bot-secret")

	_, err := (&Server{}).verifyTokenViaBot(t.Context(), "123456:AAH")
	if err == nil || !strings.Contains(err.Error(), "Unauthorized") {
		t.Fatalf("ожидали ошибку Unauthorized, получили %v", err)
	}
}

func TestVerifyTokenViaBotWhenBotIsDown(t *testing.T) {
	// Порт никто не слушает — так выглядит остановленный контейнер бота.
	t.Setenv("OFFERS_BOT_URL", "http://127.0.0.1:1")
	t.Setenv("OFFERS_BOT_SECRET", "test-bot-secret")

	_, err := (&Server{}).verifyTokenViaBot(t.Context(), "123456:AAH")
	if err == nil || !strings.Contains(err.Error(), "бот недоступен") {
		t.Fatalf("ожидали «бот недоступен», получили %v", err)
	}
}
