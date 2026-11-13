package main

// Клиент писем для агента КП (2026-11-14, шаг P5b, половина offers-service).
//
// Зовёт внутренние ручки `mailer-service` (sbe-mailer, 0.1.21):
//
//	GET /internal/mailer/agent/emails?q=&limit=
//	GET /internal/mailer/agent/email?id=
//	X-Service-Secret: $MAILER_INTERNAL_SECRET
//
// Чем это НЕ documents_client и почему форма та же:
//
//   - у мейлера нет ручки файла вовсе; сканы открываются через Документы по
//     `document_id` — так же, как в P5a: ключ файла разрешает Документы;
//   - ручки мейлера видимость писем НЕ проверяют: они отдают любое письмо с
//     карточкой скана, а пересечение с `visible` набором (документы с
//     `product_refs`) делает `get_emails` в agenttools_impl.go — решение
//     пользователя 2026-11-14, вариант «б»: мейлер и документы друг о друге
//     не знают.
//
// Пустой секрет = связь не настроена: агент честно скажет «письма недоступны»,
// а не покажет пустой список (пустой список читается как «писем нет»).
//
// Контракт заморожен и подтверждён живым ответом — фикстура
// `sbe-offers/scripts/mailer-fixture.json` (записана с боевой ручки
// 2026-11-13). Заглушка `mailer-stub.mjs` отдаёт ровно её.

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

// mailerDefaultURL — адрес mailer-service внутри docker-сети. Имя контейнера
// сверено с боевым compose (`backend`, стек /opt/mailers).
const mailerDefaultURL = "http://backend:3000"

// mailerTimeout — предел одного запроса. Письмо может нести длинный текст;
// 60 с — та же цифра, что у documents_client (сканы бывают крупными).
const mailerTimeout = 60 * time.Second

// errMailerNotConfigured — связь с письмами не настроена.
var errMailerNotConfigured = errors.New("mailer not configured")

// MailerAgentClient — HTTP-клиент внутренних ручек писем.
type MailerAgentClient struct {
	baseURL string
	secret  string
	client  *http.Client
}

// NewMailerAgentClient собирает клиент из окружения.
func NewMailerAgentClient() *MailerAgentClient {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("MAILER_SERVICE_URL")), "/")
	if base == "" {
		base = mailerDefaultURL
	}
	return &MailerAgentClient{
		baseURL: base,
		secret:  os.Getenv("MAILER_INTERNAL_SECRET"),
		client:  &http.Client{Timeout: mailerTimeout},
	}
}

// configured — связь настроена. Без секрета отвечать нечем, и пустой список
// был бы враньём: агент пересказал бы его как «писем нет».
func (c *MailerAgentClient) configured() bool {
	return c != nil && c.baseURL != "" && strings.TrimSpace(c.secret) != ""
}

// AgentMailAttachment — вложение письма в том виде, в каком его видит агент.
// Поля — snake_case, как у ручки (чужой контракт, не наш тип). `file_key`
// здесь НЕТ и быть не должно: ручка его не отдаёт, ключ разрешается по
// `document_id` через документы.
type AgentMailAttachment struct {
	DocumentID int64  `json:"document_id"`
	FileName   string `json:"file_name"`
	Kind       string `json:"kind"`
}

// AgentEmail — письмо в выдаче агента. `Text` в списке обрезан ручкой
// (≤ 600 рун), в `email?id=` — полный.
type AgentEmail struct {
	ID            int64                 `json:"id"`
	Number        string                `json:"number"`
	Subject       string                `json:"subject"`
	Author        string                `json:"author"`
	Date          string                `json:"date"`
	DirectionName string                `json:"direction_name"`
	Text          string                `json:"text"`
	Attachments   []AgentMailAttachment `json:"attachments"`
}

// AgentEmailList — ответ списка. `Items` всегда массив, даже если ручка
// ответила `null` (её это не должно случиться, но клиент не должен падать).
type AgentEmailList struct {
	Source string       `json:"source"`
	Total  int          `json:"total"`
	Items  []AgentEmail `json:"items"`
}

// Emails — список писем по запросу. `limit` <= 0 — значение по умолчанию
// ручки (20, максимум 200).
//
// Пустой `items` в ответе — законный результат: ручкой он отдаётся как `[]`,
// и клиент обязан сохранить это, а не подставить `nil`.
func (c *MailerAgentClient) Emails(ctx context.Context, query string, limit int) (AgentEmailList, error) {
	var out AgentEmailList
	values := url.Values{}
	values.Set("q", strings.TrimSpace(query))
	if limit > 0 {
		values.Set("limit", strconv.Itoa(limit))
	}
	if err := c.getJSON(ctx, "/internal/mailer/agent/emails", values, &out); err != nil {
		return AgentEmailList{Items: []AgentEmail{}}, err
	}
	if out.Items == nil {
		out.Items = []AgentEmail{}
	}
	for i := range out.Items {
		if out.Items[i].Attachments == nil {
			out.Items[i].Attachments = []AgentMailAttachment{}
		}
	}
	return out, nil
}

// Letter — одно письмо с ПОЛНЫМ текстом. Ни `404` для письма без скана, ни
// `404` для несуществующего id не различаются (ручка отвечает одинаково), так
// что здесь нет отдельного типа ошибки: обе причины — «письмо недоступно».
func (c *MailerAgentClient) Letter(ctx context.Context, id int64) (AgentEmail, error) {
	if id <= 0 {
		return AgentEmail{}, fmt.Errorf("нужен id письма")
	}
	values := url.Values{}
	values.Set("id", strconv.FormatInt(id, 10))
	var out AgentEmail
	if err := c.getJSON(ctx, "/internal/mailer/agent/email", values, &out); err != nil {
		return AgentEmail{}, err
	}
	if out.Attachments == nil {
		out.Attachments = []AgentMailAttachment{}
	}
	return out, nil
}

// getJSON — GET с секретом и разбором JSON.
func (c *MailerAgentClient) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	data, err := c.getBytes(ctx, path, query)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("сервис писем ответил не JSON: %w", err)
	}
	return nil
}

// getBytes — GET с секретом, тело ответа целиком.
func (c *MailerAgentClient) getBytes(ctx context.Context, path string, query url.Values) ([]byte, error) {
	if !c.configured() {
		return nil, errMailerNotConfigured
	}
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, bytes.NewReader(nil))
	if err != nil {
		return nil, fmt.Errorf("собрать запрос к письмам: %w", err)
	}
	req.Header.Set("X-Service-Secret", c.secret)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("сервис писем недоступен: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("прочитать ответ писем: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("сервис писем ответил %d: %s", resp.StatusCode, errorTextOfBody(body))
	}
	return body, nil
}
