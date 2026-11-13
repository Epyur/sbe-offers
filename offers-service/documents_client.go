package main

// Клиент документов для агента КП (2026-11-14, шаг P6).
//
// Зовёт внутренние ручки `documents-service`, которые сделаны шагом P5a
// (2026-11-13): компактный срез «документы, привязанные к продуктам Базы
// знаний», без JWT человека и с отдельным секретом `DOCUMENTS_INTERNAL_SECRET`.
//
// Почему свой клиент, а не JWT documents: документы отдаются НЕ от имени
// человека (правило §4.4 дизайна — «инструмент есть граница»). «Общий доступ» —
// свойство данных, а не полномочие; параметра email в ручках нет вовсе. Значит
// offers-service ходит служебным секретом, а права человека проверяет сам на
// своих ручках.
//
// Пустой секрет = связь не настроена: агент честно скажет «документы
// недоступны», а не покажет пустой список (пустой список читается как «нет
// документов»).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// documentsAgentDefaultURL — адрес documents-service внутри docker-сети.
const documentsAgentDefaultURL = "http://documents:3000"

// documentsAgentTimeout — предел одного запроса. Файл документа может быть
// крупным (скан), поэтому запас больше, чем у карточек.
const documentsAgentTimeout = 60 * time.Second

// errDocumentsNotConfigured — связь с документами не настроена.
var errDocumentsNotConfigured = errors.New("documents not configured")

// DocumentsAgentClient — HTTP-клиент внутренних ручек документов.
type DocumentsAgentClient struct {
	baseURL string
	secret  string
	client  *http.Client
}

// NewDocumentsAgentClient собирает клиент из окружения.
func NewDocumentsAgentClient() *DocumentsAgentClient {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("DOCUMENTS_SERVICE_URL")), "/")
	return &DocumentsAgentClient{
		baseURL: base,
		secret:  os.Getenv("DOCUMENTS_INTERNAL_SECRET"),
		client:  &http.Client{Timeout: documentsAgentTimeout},
	}
}

func (c *DocumentsAgentClient) configured() bool {
	return c != nil && c.baseURL != "" && strings.TrimSpace(c.secret) != ""
}

// AgentDoc — документ в том виде, в каком его видит агент. Поля — snake_case,
// как у ручки (чужой контракт, не наш тип).
type AgentDoc struct {
	DocumentID  int64             `json:"document_id"`
	Title       string            `json:"title"`
	DocType     string            `json:"doc_type"`
	FileName    string            `json:"file_name"`
	FileKey     string            `json:"file_key"`
	LinkURL     string            `json:"link_url"`
	ProductRefs []AgentProductRef `json:"product_refs"`
	NoAgent     bool              `json:"no_agent"`
	UpdatedAt   string            `json:"updated_at"`
}

// AgentProductRef — привязка карточки к продукту Базы знаний. Поля — как у
// документов (`product_refs` JSONB): `note_id`, `title`, необязательный `sku`.
// Своего типа на каждый сервис не заводим: форма одна, привязка — одна.
type AgentProductRef struct {
	NoteID int64  `json:"note_id"`
	Title  string `json:"title"`
	SKU    string `json:"sku,omitempty"`
}

// AgentDocList — ответ списка. `Items` всегда массив (ручка гарантирует).
type AgentDocList struct {
	Source string     `json:"source"`
	Total  int        `json:"total"`
	Items  []AgentDoc `json:"items"`
}

// List — документы общего доступа по запросу. limit <= 0 — по умолчанию.
func (c *DocumentsAgentClient) List(ctx context.Context, query string, limit int) (AgentDocList, error) {
	var out AgentDocList
	values := url.Values{}
	values.Set("q", strings.TrimSpace(query))
	if limit > 0 {
		values.Set("limit", strconv.Itoa(limit))
	}
	if err := c.getJSON(ctx, "/internal/documents/agent/list", values, &out); err != nil {
		return AgentDocList{Items: []AgentDoc{}}, err
	}
	if out.Items == nil {
		out.Items = []AgentDoc{}
	}
	return out, nil
}

// FileLink — временная ссылка на файл документа.
func (c *DocumentsAgentClient) FileLink(ctx context.Context, key string) (string, error) {
	values := url.Values{}
	values.Set("key", key)
	var out struct {
		URL string `json:"url"`
	}
	if err := c.getJSON(ctx, "/internal/documents/agent/file-link", values, &out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.URL) == "" {
		return "", errors.New("сервис документов вернул пустую ссылку")
	}
	return out.URL, nil
}

// ReadText — текст файла документа.
//
// Байты скачиваются тем же внутренним путём. В текст их приводит
// `documentTextOf`: PDF/DOCX разбираются, остальное берётся как UTF-8-текст.
// Скан без текстового слоя даст пустую строку — это честный ответ, а не сбой.
func (c *DocumentsAgentClient) ReadText(ctx context.Context, key string) (string, error) {
	values := url.Values{}
	values.Set("key", key)
	data, err := c.getBytes(ctx, "/internal/documents/agent/file", values)
	if err != nil {
		return "", err
	}
	return documentTextOf(key, data), nil
}

// getJSON — GET с секретом и разбором JSON.
func (c *DocumentsAgentClient) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	data, err := c.getBytes(ctx, path, query)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("сервис документов ответил не JSON: %w", err)
	}
	return nil
}

// getBytes — GET с секретом, тело ответа целиком.
func (c *DocumentsAgentClient) getBytes(ctx context.Context, path string, query url.Values) ([]byte, error) {
	if !c.configured() {
		return nil, errDocumentsNotConfigured
	}
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, bytes.NewReader(nil))
	if err != nil {
		return nil, fmt.Errorf("собрать запрос к документам: %w", err)
	}
	req.Header.Set("X-Service-Secret", c.secret)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("сервис документов недоступен: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("прочитать ответ документов: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("сервис документов ответил %d: %s", resp.StatusCode, errorTextOfBody(body))
	}
	return body, nil
}

// errorTextOfBody — текст ошибки из тела ответа для журнала и модели.
func errorTextOfBody(body []byte) string {
	var parsed struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error != "" {
		return parsed.Error
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}
