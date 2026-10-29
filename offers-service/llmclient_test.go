package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// stubLLM — подставной llm-service: проверяет, что клиент ходит ровно туда,
// куда сказано, и правильно разбирает три возможных ответа (успех, ошибка
// ключа, авария провайдера).
func stubLLM(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *string, *string) {
	t.Helper()
	var gotSecret, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSecret = r.Header.Get("X-Service-Secret")
		buf := new(strings.Builder)
		if _, err := io.Copy(buf, r.Body); err != nil {
			t.Errorf("чтение тела: %v", err)
		}
		gotBody = buf.String()
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	os.Setenv("LLM_SERVICE_URL", srv.URL)
	os.Setenv("LLM_SERVICE_SECRET", "test-secret")
	t.Cleanup(func() {
		os.Unsetenv("LLM_SERVICE_URL")
		os.Unsetenv("LLM_SERVICE_SECRET")
	})
	return srv, &gotSecret, &gotBody
}

func TestCompleteOK(t *testing.T) {
	_, secret, body := stubLLM(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":"готовый текст"}`))
	})
	got, err := Complete(context.Background(), "система", "вопрос", 500)
	if err != nil {
		t.Fatalf("ожидали успех, получили ошибку: %v", err)
	}
	if got != "готовый текст" {
		t.Fatalf("ожидали «готовый текст», получили %q", got)
	}
	if *secret != "test-secret" {
		t.Fatalf("секрет не передан: %q", *secret)
	}
	if !strings.Contains(*body, `"system":"система"`) || !strings.Contains(*body, `"max_tokens":500`) {
		t.Fatalf("тело запроса не то: %s", *body)
	}
}

// Ключ не настроен — не авария, а состояние, про которое сервис обязан знать:
// на нём держится деградация (КП собирается без текста).
func TestCompleteNoKey(t *testing.T) {
	stubLLM(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"llm key not configured"}`))
	})
	_, err := Complete(context.Background(), "s", "u", 100)
	if !errors.Is(err, errLLMNotConfigured) {
		t.Fatalf("ожидали errLLMNotConfigured, получили %v", err)
	}
}

// Авария провайдера — это НЕ «ключа нет»: на неё бот отвечает иначе (пауза и
// повтор), а не деградацией текста.
func TestCompleteUpstreamDown(t *testing.T) {
	stubLLM(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream error"}`))
	})
	_, err := Complete(context.Background(), "s", "u", 100)
	if err == nil {
		t.Fatal("ожидали ошибку")
	}
	if errors.Is(err, errLLMNotConfigured) {
		t.Fatal("авария провайдера не должна выглядеть как «ключ не настроен»")
	}
}

// Пустой секрет сервиса — значит клиент не настроен вовсе; запрос не идёт.
func TestCompleteNoSecret(t *testing.T) {
	os.Unsetenv("LLM_SERVICE_SECRET")
	os.Setenv("LLM_SERVICE_URL", "http://127.0.0.1:1")
	t.Cleanup(func() {
		os.Unsetenv("LLM_SERVICE_URL")
	})
	_, err := Complete(context.Background(), "s", "u", 100)
	if !errors.Is(err, errLLMNotConfigured) {
		t.Fatalf("ожидали errLLMNotConfigured, получили %v", err)
	}
}

// Таймаут берётся из окружения: провайдер лагает 60 с, шлюз обрывает раньше.
func TestCompleteTimeout(t *testing.T) {
	os.Setenv("OFFERS_LLM_TIMEOUT_MS", "80")
	t.Cleanup(func() { os.Unsetenv("OFFERS_LLM_TIMEOUT_MS") })
	stubLLM(t, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte(`{"content":"поздно"}`))
	})
	start := time.Now()
	_, err := Complete(context.Background(), "s", "u", 100)
	if err == nil {
		t.Fatal("ожидали ошибку таймаута")
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("клиент ждал %v — таймаут не сработал", elapsed)
	}
}

// Битый JSON от провайдера — ошибка, но не «ключа нет».
func TestCompleteBadJSON(t *testing.T) {
	stubLLM(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html>oops</html>`))
	})
	_, err := Complete(context.Background(), "s", "u", 100)
	if err == nil || errors.Is(err, errLLMNotConfigured) {
		t.Fatalf("ожидали обычную ошибку, получили %v", err)
	}
}

// Пустой ответ модели — ошибка с понятным текстом, чтобы бот не отправил
// пустую секцию в КП.
func TestCompleteEmptyContent(t *testing.T) {
	stubLLM(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"content":""}`))
	})
	_, err := Complete(context.Background(), "s", "u", 100)
	if err == nil {
		t.Fatal("пустой текст должен быть ошибкой")
	}
	if !strings.Contains(err.Error(), "пуст") && !strings.Contains(fmt.Sprint(err), "empty") {
		t.Fatalf("непонятная ошибка для пустого ответа: %v", err)
	}
}
