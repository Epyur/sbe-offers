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

const (
	llmDefaultURL        = "http://llm:3000"
	llmDefaultTimeoutMS  = 45000
	llmDefaultMaxTokens  = 800
	llmBodyPreviewLength = 200
)

// Complete отправляет system+user в llm-service и возвращает текст ответа.
//
// Цепочка: offers-service → POST /internal/llm/complete (X-Service-Secret) →
// llm-service берёт ключ (общий или пользовательский) → провайдер.
func Complete(ctx context.Context, system, user string, maxTokens int) (string, error) {
	secret := strings.TrimSpace(os.Getenv("LLM_SERVICE_SECRET"))
	if secret == "" {
		return "", fmt.Errorf("%w: LLM_SERVICE_SECRET пуст", errLLMNotConfigured)
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("LLM_SERVICE_URL")), "/")
	if base == "" {
		base = llmDefaultURL
	}
	if maxTokens <= 0 {
		maxTokens = llmDefaultMaxTokens
	}

	payload, err := json.Marshal(map[string]any{
		"system":     system,
		"user":       user,
		"max_tokens": maxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("собрать запрос к модели: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/internal/llm/complete", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("собрать запрос к модели: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Secret", secret)

	client := &http.Client{Timeout: llmTimeout()}
	resp, err := client.Do(req)
	if err != nil {
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
