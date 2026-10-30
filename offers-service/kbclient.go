package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// Клиент Базы знаний (шаг 1 пересмотренного плана 2026-10-29).
//
// Карточки систем живут в базе знаний как обычные заметки с тегом
// commercial_propose. offers-service их ЧИТАЕТ: текст описания, состав,
// картинку. Цены и артикулы по-прежнему берутся из каталога — каждый источник
// отвечает за своё.
//
// Авторизация: kb-service требует JWT с app_id=kb, а не служебный секрет.
// Токен выдаёт auth-service (/internal/service-token), вызывающий
// представляется своим OFFERS_SERVICE_SECRET — ровно так же, как
// lab-service ходит в ekn-service (ekn_client.go).
//
// Токен кешируется: на один заход по карточкам auth-service дёргать не надо.

// errKBNotConfigured — база знаний не настроена. Это НЕ авария: КП собирается
// без карточек, как раньше.
var errKBNotConfigured = errors.New("база знаний не настроена")

const (
	kbDefaultURL      = "http://kb:3000"
	authDefaultURL    = "http://auth-service:3000"
	kbTokenTTL        = 50 * time.Minute // JWT живёт час, берём с запасом
	kbRequestTimeout  = 20 * time.Second
	commercialPropose = "commercial_propose"

	// kbPageSize — сколько заметок просим за раз. Сервис отдаёт не больше 50
	// на запрос при любом size, поэтому просить 200 бессмысленно: получим те же
	// 50, и если не листать дальше — потеряем остальные (живой дефект
	// 2026-10-30: из 588 карточек видели 50).
	kbPageSize = 50

	// maxKBPages — предел страниц. Сервис, который всегда отдаёт полную
	// страницу, не должен крутить цикл вечно.
	maxKBPages = 40
)

// KBTag — тег заметки.
type KBTag struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// KBNoteSummary — карточка в списке.
type KBNoteSummary struct {
	ID        int64  `json:"id"`
	UUID      string `json:"uuid"`
	Title     string `json:"title"`
	Path      string `json:"path"`
	UpdatedAt string `json:"updated_at"`
}

// KBNote — карточка с текстом.
type KBNote struct {
	KBNoteSummary
	ContentMD string  `json:"content_md"`
	Tags      []KBTag `json:"tags"`
}

// KBAttachment — вложение карточки (картинка продукта и прочее).
//
// Структура нарочно совпадает с той, что отдаёт kb-service: filename,
// mime, key. По mime выбирается image/png или image/jpeg для DOCX.
type KBAttachment struct {
	Filename    string `json:"filename"`
	Mime        string `json:"mime"`
	Key         string `json:"key"`
	ContentHash string `json:"content_hash"`
}

// KBClient — HTTP-клиент базы знаний.
type KBClient struct {
	baseURL    string
	authURL    string
	serviceKey string
	client     *http.Client

	mu      sync.Mutex
	token   string
	tokenAt time.Time
	// tokenFactory подменяется в тестах, чтобы не ходить в auth-service.
	tokenFactory func(ctx context.Context) (string, error)
}

// NewKBClient собирает клиент из окружения. Пустой адрес — клиент выключен:
// все вызовы вернут errKBNotConfigured, и КП соберётся без карточек.
func NewKBClient() *KBClient {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("KB_SERVICE_URL")), "/")
	auth := strings.TrimRight(strings.TrimSpace(os.Getenv("AUTH_SERVICE_URL")), "/")
	if auth == "" {
		auth = authDefaultURL
	}
	return &KBClient{
		baseURL:    base,
		authURL:    auth,
		serviceKey: os.Getenv("OFFERS_SERVICE_SECRET"),
		client:     &http.Client{Timeout: kbRequestTimeout},
	}
}

func (c *KBClient) configured() bool {
	return c != nil && c.baseURL != ""
}

// authToken — действующий межсервисный токен, при необходимости запрошенный
// заново.
func (c *KBClient) authToken(ctx context.Context) (string, error) {
	if c.tokenFactory != nil {
		return c.tokenFactory(ctx)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Since(c.tokenAt) < kbTokenTTL {
		return c.token, nil
	}
	token, err := fetchServiceToken(ctx, c.authURL, c.serviceKey, "kb", c.client)
	if err != nil {
		return "", err
	}
	c.token = token
	c.tokenAt = time.Now()
	return token, nil
}

// fetchServiceToken — межсервисный JWT у auth-service (образец — lab-service).
func fetchServiceToken(ctx context.Context, authURL, serviceSecret, targetAppID string, hc *http.Client) (string, error) {
	if strings.TrimSpace(serviceSecret) == "" {
		return "", fmt.Errorf("%w: OFFERS_SERVICE_SECRET пуст", errKBNotConfigured)
	}
	body, err := json.Marshal(map[string]string{"target_app_id": targetAppID})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(authURL, "/")+"/internal/service-token", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Secret", serviceSecret)
	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("auth-service недоступен: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth-service отказал в токене: %d", resp.StatusCode)
	}
	var out struct {
		JWT string `json:"jwt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("auth-service ответил не JSON: %w", err)
	}
	if strings.TrimSpace(out.JWT) == "" {
		return "", errors.New("auth-service вернул пустой токен")
	}
	return out.JWT, nil
}

// get выполняет запрос к kb-service и разбирает JSON.
//
// Ошибки kb-service НЕ переводятся в пустые значения: «нет прав» и «карточек
// нет» — разные вещи, и плагин должен показывать правду.
func (c *KBClient) get(ctx context.Context, path string, query url.Values, out any) error {
	if !c.configured() {
		return errKBNotConfigured
	}
	token, err := c.authToken(ctx)
	if err != nil {
		return err
	}
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("база знаний недоступна: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("прочитать ответ базы знаний: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("база знаний ответила %d: %s", resp.StatusCode, previewKB(body))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("база знаний ответила не JSON: %s", previewKB(body))
	}
	return nil
}

// previewKB — начало тела ответа для журнала.
func previewKB(body []byte) string {
	s := strings.Join(strings.Fields(string(body)), " ")
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

// TagID — id тега по имени. Нет такого тега — ошибка: карточки искать негде.
func (c *KBClient) TagID(ctx context.Context, name string) (int64, error) {
	var out struct {
		Tags []KBTag `json:"tags"`
	}
	if err := c.get(ctx, "/api/kb/tags", nil, &out); err != nil {
		return 0, err
	}
	for _, t := range out.Tags {
		if strings.EqualFold(t.Name, name) {
			return t.ID, nil
		}
	}
	return 0, fmt.Errorf("в базе знаний нет тега %q", name)
}

// Notes — карточки с тегом.
//
// Поле списка у kb-service называется `items`; `notes` понимаем для
// совместимости со старыми ответами. Живой случай 2026-10-30: клиент читал
// только `notes`, сервис отдавал `items` — соединение было, kbEnabled=true, а
// карточки всегда приходили пустым списком.
//
// Пагинация обязательна: сервис отдаёт не больше kbPageSize заметок на
// запрос при ЛЮБОМ size, но сообщает total. Раньше клиент просил size=200,
// получал 50 и считал, что это всё, — из 588 карточек плагин видел 50.
func (c *KBClient) Notes(ctx context.Context, tagID int64) ([]KBNoteSummary, error) {
	out := make([]KBNoteSummary, 0, kbPageSize)
	for page := 1; page <= maxKBPages; page++ {
		items, total, err := c.notesPage(ctx, tagID, page)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		// Дальше идти некуда: страница неполная или собрали всё, что обещали.
		if len(items) < kbPageSize || (total > 0 && len(out) >= total) {
			return out, nil
		}
	}
	// Предел страниц достигнут, а сервис всё ещё отдаёт полные страницы.
	// Молча обрезать нельзя — это значит «показали не всё», и лучше сказать
	// это в журнал, чем притвориться, что карточек хватило.
	log.Printf("kb: достигнут предел %d страниц, карточек собрано %d", maxKBPages, len(out))
	return out, nil
}

// notesPage — одна страница списка заметок и заявленное всего.
func (c *KBClient) notesPage(ctx context.Context, tagID int64, page int) ([]KBNoteSummary, int, error) {
	q := url.Values{}
	q.Set("tag_id", fmt.Sprintf("%d", tagID))
	q.Set("size", fmt.Sprintf("%d", kbPageSize))
	q.Set("page", fmt.Sprintf("%d", page))
	var out struct {
		Items []KBNoteSummary `json:"items"`
		Notes []KBNoteSummary `json:"notes"`
		Total int             `json:"total"`
	}
	if err := c.get(ctx, "/api/kb/notes", q, &out); err != nil {
		return nil, 0, err
	}
	if len(out.Items) > 0 {
		return out.Items, out.Total, nil
	}
	return out.Notes, out.Total, nil
}

// Note — карточка с текстом.
func (c *KBClient) Note(ctx context.Context, id int64) (KBNote, error) {
	var out KBNote
	err := c.get(ctx, fmt.Sprintf("/api/kb/notes/%d", id), nil, &out)
	return out, err
}

// Attachments — вложения карточки.
func (c *KBClient) Attachments(ctx context.Context, noteID int64) ([]KBAttachment, error) {
	var out struct {
		Attachments []KBAttachment `json:"attachments"`
	}
	err := c.get(ctx, fmt.Sprintf("/api/kb/notes/%d/attachments", noteID), nil, &out)
	return out.Attachments, err
}

// FileLink — ссылка на файл вложения.
func (c *KBClient) FileLink(ctx context.Context, key string) (string, error) {
	q := url.Values{}
	q.Set("key", key)
	var out struct {
		URL string `json:"url"`
	}
	if err := c.get(ctx, "/api/kb/file-link", q, &out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.URL) == "" {
		return "", errors.New("база знаний не дала ссылку на файл")
	}
	return out.URL, nil
}
