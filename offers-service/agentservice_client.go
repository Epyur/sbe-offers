package main

// Клиент агента-сервиса LogicTEAM.007 (2026-11-16, шаг 3 плана
// docs/superpowers/plans/2026-11-16-sbe-offers-combat-sections-and-agent-tools-plan.md).
//
// Зачем. Агенту КП нужны инструменты sbe-agent: скачать страницу или API,
// разобрать приложенный файл (Word/Excel/PDF/JSON) и отрисовать mermaid-схему.
// SSRF-защита, лимиты и разбор уже живут в agent-service — второй точки в сеть
// здесь НЕТ. Правило дизайна §3.1: «неправильно делать fetch/разбор в
// offers-service — это уже написано и работает».
//
// Почему /internal/ и свой секрет. Ручки agent-service
// `/internal/agent/{fetch,file/parse,render}` живут только в docker-сети (Caddy
// публикует наружу лишь `/api/agent/*`) и отдают по X-Service-Secret со
// значением AGENT_INTERNAL_SECRET — отдельному от AGENT_SERVICE_SECRET (тем
// подписываются JWT людей). Тот же приём, что у документов и писем
// (documents_client.go / mailer_client.go).
//
// Отказ вместо пустоты: `configured() == false` — отказ, а не пустой ответ.
// Пустой список агент пересказал бы как «на сервере ничего нет», и менеджер
// искал бы обход.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"
)

// agentServiceDefaultURL — адрес agent-service в docker-сети.
const agentServiceDefaultURL = "http://agent:3000"

// agentServiceTimeout — предел одного запроса к agent-service.
//
// Отдельно от документов и писем: fetch по чужому сайту может идти до 120 с
// (это предел самого agent-service), рендер mermaid — до 90 с (семафор 2 слота,
// очередь), разбор большого файла — до 60 с. Берём верхнюю границу, чтобы
// таймаут клиента не отменял работу, которую сервер уже делает.
const agentServiceTimeout = 150 * time.Second

// errAgentServiceNotConfigured — сервис агента не настроен.
var errAgentServiceNotConfigured = errors.New("agent service not configured")

// AgentServiceClient — HTTP-клиент внутренних ручек agent-service.
type AgentServiceClient struct {
	baseURL string
	secret  string
	client  *http.Client
}

// NewAgentServiceClient собирает клиент из окружения.
func NewAgentServiceClient() *AgentServiceClient {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("AGENT_SERVICE_URL")), "/")
	if base == "" {
		base = agentServiceDefaultURL
	}
	return &AgentServiceClient{
		baseURL: base,
		secret:  os.Getenv("AGENT_INTERNAL_SECRET"),
		client:  &http.Client{Timeout: agentServiceTimeout},
	}
}

// configured — сервис настроен (есть адрес и секрет).
func (c *AgentServiceClient) configured() bool {
	return c != nil && c.baseURL != "" && strings.TrimSpace(c.secret) != ""
}

// AgentFetchRequest — тело `POST /internal/agent/fetch`. Поля совпадают с тем,
// что принимает agent-service (та же форма, что у `/api/agent/fetch`).
type AgentFetchRequest struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Body      string            `json:"body,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	TimeoutMs int               `json:"timeout_ms,omitempty"`
}

// AgentFetchResult — ответ фетча.
type AgentFetchResult struct {
	Status      int    `json:"status"`
	ContentType string `json:"content_type"`
	Text        string `json:"text"`
}

// Fetch — скачать страницу или API чужого сайта через agent-service.
func (c *AgentServiceClient) Fetch(ctx context.Context, req AgentFetchRequest) (AgentFetchResult, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return AgentFetchResult{}, fmt.Errorf("собрать запрос фетча: %w", err)
	}
	data, err := c.post(ctx, "/internal/agent/fetch", "application/json", bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return AgentFetchResult{}, err
	}
	var out AgentFetchResult
	if err := json.Unmarshal(data, &out); err != nil {
		return AgentFetchResult{}, fmt.Errorf("сервис агента ответил не JSON: %w", err)
	}
	return out, nil
}

// AgentParseResult — ответ разбора файла. Заполнено ровно одно из Text/Sheets/
// Data — в зависимости от того, что за файл (та же форма, что у
// `/api/agent/file/parse`).
type AgentParseResult struct {
	Kind   string           `json:"kind"`
	Text   string           `json:"text,omitempty"`
	Sheets []map[string]any `json:"sheets,omitempty"`
	Data   any              `json:"data,omitempty"`
}

// ParseFile — разобрать приложенный файл через agent-service (multipart).
//
// Имя не угадывается вызывающим: разбирается ровно тот файл, который человек
// приложил. Формат выбирает расширение имени (`doParse` в agent-service).
func (c *AgentServiceClient) ParseFile(ctx context.Context, name string, data []byte) (AgentParseResult, error) {
	if !c.configured() {
		return AgentParseResult{}, errAgentServiceNotConfigured
	}
	if strings.TrimSpace(name) == "" {
		return AgentParseResult{}, errors.New("нужно имя файла")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		return AgentParseResult{}, fmt.Errorf("собрать multipart: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return AgentParseResult{}, fmt.Errorf("записать файл в multipart: %w", err)
	}
	if err := writer.Close(); err != nil {
		return AgentParseResult{}, fmt.Errorf("закрыть multipart: %w", err)
	}
	raw, err := c.post(ctx, "/internal/agent/file/parse", writer.FormDataContentType(), &body, int64(body.Len()))
	if err != nil {
		return AgentParseResult{}, err
	}
	var out AgentParseResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return AgentParseResult{}, fmt.Errorf("сервис агента ответил не JSON: %w", err)
	}
	return out, nil
}

// AgentRenderRequest — тело `/internal/agent/render`. `Lang` принимается только
// `mermaid`: произвольный SVG сервис не растеризует (спека §7.2).
type AgentRenderRequest struct {
	Lang string `json:"lang"`
	Code string `json:"code"`
}

// AgentRenderResult — картинка: base64 PNG и размеры в пикселях.
type AgentRenderResult struct {
	Format string `json:"format"`
	Base64 string `json:"base64"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// Render — mermaid-код → PNG через agent-service.
func (c *AgentServiceClient) Render(ctx context.Context, lang, code string) (AgentRenderResult, error) {
	payload, err := json.Marshal(AgentRenderRequest{Lang: lang, Code: code})
	if err != nil {
		return AgentRenderResult{}, fmt.Errorf("собрать запрос рендера: %w", err)
	}
	data, err := c.post(ctx, "/internal/agent/render", "application/json", bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return AgentRenderResult{}, err
	}
	var out AgentRenderResult
	if err := json.Unmarshal(data, &out); err != nil {
		return AgentRenderResult{}, fmt.Errorf("сервис агента ответил не JSON: %w", err)
	}
	return out, nil
}

// post — POST с секретом; тело ответа целиком (ограничение — как у документов).
func (c *AgentServiceClient) post(ctx context.Context, path, contentType string, body io.Reader, size int64) ([]byte, error) {
	if !c.configured() {
		return nil, errAgentServiceNotConfigured
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("собрать запрос к сервису агента: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Service-Secret", c.secret)
	if size > 0 {
		req.ContentLength = size
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("сервис агента недоступен: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("прочитать ответ сервиса агента: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("сервис агента ответил %d: %s", resp.StatusCode, errorTextOfBody(raw))
	}
	return raw, nil
}
