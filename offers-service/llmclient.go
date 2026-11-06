package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Клиент общего слоя моделей (llm-service). Своих ключей провайдера у
// offers-service нет и быть не должно: ключ живёт в llm-service (общий или
// пользовательский), а мы ходим во внутреннюю ручку под X-Service-Secret.
// Caddy наружу её не пускает — она доступна только на docker-сети.
//
// Почему не через прокси "/api/llm/chat/completions": та ручка требует JWT
// человека, а текст КП пишет сервис от своего имени, без сессии менеджера.

// errLLMNotConfigured — модель недоступна по причине конфигурации (нет ключа
// или не задан секрет сервиса). Это НЕ авария: на ней держится деградация —
// КП собирается с текстом формы, без написанных секций.
var errLLMNotConfigured = errors.New("llm not configured")

// Таймаут ответа модели (2026-11-06).
//
// Было 45 секунд, и это резало именно длинные тексты КП. Замер на живом
// КП 003/2026: промт «опиши систему, не менее 4000 знаков» модель писала
// 58 секунд — сначала обдумывание, потом текст. Клиент обрывал запрос на
// 45-й секунде, и в документ шло то, что модель успела сказать ДО ответа:
// «The user wants a ready-to-deliver text… Let me write…». Отсюда и
// «модель отвечает рассуждением» — ответа просто не было.
//
// Провайдер держит запрос до 190 секунд (llm-service, doProviderRequest),
// так что 180 с здесь — это верхняя граница разумного: место для длинного
// ответа есть, а зависший запрос всё равно отпустится.
const (
	llmDefaultURL        = "http://llm:3000"
	llmDefaultTimeoutMS  = 180000
	llmBodyPreviewLength = 200
	// llmRetryAttempts — сколько раз пробуем ОДИН И ТОТ ЖЕ запрос. Аварии,
	// которые повтор не лечит (нет ключа, 400), сюда не попадают.
	llmRetryAttempts = 3
	// llmRetryBackoff — пауза перед повтором; растёт линейно.
	llmRetryBackoff = 2 * time.Second
	// llmRetryBackoffEnv — переопределение паузы в миллисекундах. Нужно тестам:
	// ждать настоящие секунды незачем, а поведение повтора проверять надо.
	llmRetryBackoffEnv = "OFFERS_LLM_RETRY_BACKOFF_MS"
)

// retryBackoff — пауза перед повтором с учётом переопределения из окружения.
func retryBackoff() time.Duration {
	raw := strings.TrimSpace(os.Getenv(llmRetryBackoffEnv))
	if raw == "" {
		return llmRetryBackoff
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms < 0 {
		return llmRetryBackoff
	}
	return time.Duration(ms) * time.Millisecond
}

// Complete отправляет system+user в llm-service и возвращает текст ответа.
//
// Цепочка: offers-service → POST /internal/llm/complete (X-Service-Secret) →
// llm-service берёт ключ (общий или пользовательский) → провайдер.
//
// maxTokens НЕ передаётся, когда он нулевой или отрицательный: тогда предел
// ответа назначает провайдер (2026-11-06). Своё число резать ответ не должно —
// ровно на этом КП 003/2026 теряла описательную часть: бюджет кончался раньше,
// чем модель дописывала текст, и в `content` оставалось вступление-рассуждение
// («Заголовок: …», «1. "…"»). Так же работает плагин 007 — он `max_tokens` не
// шлёт вовсе (`sbe-llm/src/services/llm-center.ts`).
func Complete(ctx context.Context, system, user string, maxTokens int) (string, error) {
	secret := strings.TrimSpace(os.Getenv("LLM_SERVICE_SECRET"))
	if secret == "" {
		return "", fmt.Errorf("%w: LLM_SERVICE_SECRET пуст", errLLMNotConfigured)
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("LLM_SERVICE_URL")), "/")
	if base == "" {
		base = llmDefaultURL
	}

	payload, err := json.Marshal(map[string]any{
		"system":     system,
		"user":       user,
		"max_tokens": maxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("собрать запрос к модели: %w", err)
	}

	client := &http.Client{Timeout: llmTimeout()}
	var lastErr error
	for attempt := 1; attempt <= llmRetryAttempts; attempt++ {
		if attempt > 1 {
			// Пауза до повтора, отменяемая вместе с запросом.
			if err := sleepCtx(ctx, time.Duration(attempt-1)*retryBackoff()); err != nil {
				return "", err
			}
		}
		content, err := llmOnce(ctx, client, base, secret, payload)
		if err == nil {
			return content, nil
		}
		if !llmRetryable(err) {
			return "", err
		}
		lastErr = err
	}
	return "", lastErr
}

// llmOnce — один запрос к llm-service.
func llmOnce(ctx context.Context, client *http.Client, base, secret string, payload []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/internal/llm/complete", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("собрать запрос к модели: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Secret", secret)

	resp, err := client.Do(req)
	if err != nil {
		// Таймаут и обрыв связи — та же авария, что 504: ответа нет, запрос
		// можно повторить. Живой случай 2026-11-06 — длинный текст КП,
		// который модель пишет дольше, чем длится один заход.
		return "", fmt.Errorf("модель недоступна: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("прочитать ответ модели: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", llmStatusError(resp.StatusCode, body)
	}

	var parsed struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("модель ответила не JSON: %s", previewText(body, llmBodyPreviewLength))
	}
	content := strings.TrimSpace(parsed.Content)
	if content == "" {
		return "", errors.New("модель вернула пустой ответ")
	}
	return content, nil
}

// errLLMTimeout — ответа не дождались. Своя метка, потому что текст ошибки
// клиента http содержит подробности транспорта, а не ответ сервиса.
var errLLMTimeout = errors.New("модель не ответила вовремя")

// llmRetryable — стоит ли повторять запрос.
//
// Повторяем то, что не зависит от нашего запроса: нет ответа (таймаут,
// обрыв), 502/504 (шлюз и провайдер не успели), 429 (лимит частоты, как в
// `llm-center.ts` плагина 007). Не повторяем «ключа нет» и 400/403: второй
// такой же запрос даст тот же ответ, а ждать придётся человеку.
func llmRetryable(err error) bool {
	if errors.Is(err, errLLMTimeout) {
		return true
	}
	if errors.Is(err, errLLMNotConfigured) {
		return false
	}
	msg := err.Error()
	for _, marker := range []string{"504", "502", "429", "timeout", "deadline exceeded", "connection reset", "EOF"} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// sleepCtx ждёт паузу, прерываясь вместе с контекстом запроса.
func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// llmStatusError различает «ключа нет» (деградация) и остальные аварии
// (повтор, пауза). Раньше обе ситуации были одной ошибкой — и бот не мог
// решить, писать ли секцию вообще.
func llmStatusError(status int, body []byte) error {
	detail := previewText(body, llmBodyPreviewLength)
	if status == http.StatusBadRequest && strings.Contains(string(body), "llm key not configured") {
		return fmt.Errorf("%w: ключ модели не настроен", errLLMNotConfigured)
	}
	return fmt.Errorf("модель ответила %d: %s", status, detail)
}

func llmTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("OFFERS_LLM_TIMEOUT_MS"))
	if raw == "" {
		return llmDefaultTimeoutMS * time.Millisecond
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return llmDefaultTimeoutMS * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}

// previewText обрезает тело ответа для журнала: в нём бывает эхо промпта с
// данными заказчика, поэтому в логи и ошибки идёт только начало.
func previewText(body []byte, n int) string {
	s := strings.TrimSpace(string(body))
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
