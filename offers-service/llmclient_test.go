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

// Таймаут берётся из окружения, и такой запрос ПОВТОРЯЕТСЯ (2026-11-06).
//
// Прежняя версия утверждала «одна попытка и конец». Решение изменено: один
// медленный ответ оставлял блок модели пустым, поэтому теперь три попытки с
// паузой — как `retryWithBackoff` в `llm-center.ts` плагина 007. Тест проверяет
// обе половины: отказ всё равно возвращается ошибкой, и попыток было ровно
// столько, сколько объявлено.
func TestCompleteTimeout(t *testing.T) {
	os.Setenv("OFFERS_LLM_TIMEOUT_MS", "60")
	t.Cleanup(func() { os.Unsetenv("OFFERS_LLM_TIMEOUT_MS") })
	os.Setenv(llmRetryBackoffEnv, "0")
	t.Cleanup(func() { os.Unsetenv(llmRetryBackoffEnv) })
	var calls int
	stubLLM(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"content":"поздно"}`))
	})
	_, err := Complete(context.Background(), "s", "u", 100)
	if err == nil {
		t.Fatal("ожидали ошибку таймаута")
	}
	if calls != llmRetryAttempts {
		t.Fatalf("попыток %d, ожидалось %d", calls, llmRetryAttempts)
	}
}

// Один таймаут, второй заход успешен — текст возвращается.
//
// Это и есть живой сценарий 2026-11-06: длинный ответ КП не успевал за один
// заход, и человек получал пустой блок.
func TestCompleteRetriesAfterTimeout(t *testing.T) {
	os.Setenv("OFFERS_LLM_TIMEOUT_MS", "100")
	t.Cleanup(func() { os.Unsetenv("OFFERS_LLM_TIMEOUT_MS") })
	os.Setenv(llmRetryBackoffEnv, "0")
	t.Cleanup(func() { os.Unsetenv(llmRetryBackoffEnv) })
	var calls int
	stubLLM(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			time.Sleep(300 * time.Millisecond)
			_, _ = w.Write([]byte(`{"content":"поздно"}`))
			return
		}
		_, _ = w.Write([]byte(`{"content":"Готовый текст описания."}`))
	})
	got, err := Complete(context.Background(), "s", "u", 100)
	if err != nil {
		t.Fatalf("повтор не спас: %v", err)
	}
	if got != "Готовый текст описания." {
		t.Fatalf("получен не тот текст: %q", got)
	}
	if calls != 2 {
		t.Fatalf("попыток %d, ожидалось 2", calls)
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

// Повтор запроса к модели (2026-11-06).
//
// Живой случай: длинный текст КП модель пишет дольше одного захода, запрос
// обрывается — и блок молчит, потому что второго захода не было вовсе.
// Плагин 007 в этом месте повторяет (`retryWithBackoff` в `llm-center.ts`).
// Повторяем только то, что от нашего запроса не зависит; «нет ключа» и 400
// повторять бессмысленно — ответ на тот же запрос будет тем же.
func TestLLMRetryableOnTimeout(t *testing.T) {
	if !llmRetryable(errLLMTimeout) {
		t.Fatal("таймаут должен повторяться")
	}
}

func TestLLMRetryableOnGatewayAndLimit(t *testing.T) {
	for _, msg := range []string{
		"модель ответила 504: upstream timeout",
		"модель ответила 502: upstream request failed",
		"модель ответила 429: too many requests, try later",
		"модель недоступна: context deadline exceeded",
		"модель недоступна: connection reset by peer",
	} {
		if !llmRetryable(errors.New(msg)) {
			t.Errorf("авария %q не повторяется, хотя должна", msg)
		}
	}
}

// Нет ключа модели — не авария сети: второй такой же запрос даст тот же отказ,
// а человек будет ждать три паузы.
func TestLLMNotRetryableWhenNotConfigured(t *testing.T) {
	if llmRetryable(errLLMNotConfigured) {
		t.Fatal("отсутствие ключа повторяется, хотя повтор не поможет")
	}
	wrapped := fmt.Errorf("модель недоступна: %w", errLLMNotConfigured)
	if llmRetryable(wrapped) {
		t.Fatal("незаданный ключ в обёртке считается повторяемой аварией")
	}
}

func TestLLMNotRetryableOnBadRequest(t *testing.T) {
	if llmRetryable(errors.New("модель ответила 400: invalid json")) {
		t.Fatal("400 повторяется, хотя ответ на тот же запрос будет тот же")
	}
}

// Пауза уважает контекст: отменённый запрос не ждёт впустую.
func TestSleepCtxStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepCtx(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("ожидалась отмена контекста, получено %v", err)
	}
}

// Пауза перед повтором настраивается: тесты не ждут настоящих секунд.
func TestRetryBackoffOverride(t *testing.T) {
	if got := retryBackoff(); got != llmRetryBackoff {
		t.Fatalf("без переопределения пауза %v, ожидалась %v", got, llmRetryBackoff)
	}
	os.Setenv(llmRetryBackoffEnv, "7")
	t.Cleanup(func() { os.Unsetenv(llmRetryBackoffEnv) })
	if got := retryBackoff(); got != 7*time.Millisecond {
		t.Fatalf("переопределённая пауза %v, ожидалось 7ms", got)
	}
	os.Setenv(llmRetryBackoffEnv, "мусор")
	if got := retryBackoff(); got != llmRetryBackoff {
		t.Fatalf("мусор в переопределении дал %v, ожидалось умолчание", got)
	}
}
