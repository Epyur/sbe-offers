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
//
// Tags заполняются, когда сервис их прислал: по ним различаются системы и
// материалы (commercial_propose + material против одного commercial_propose).
// Список может их не содержать, поэтому признак проверяется терпимо.
type KBNoteSummary struct {
	ID        int64   `json:"id"`
	UUID      string  `json:"uuid"`
	Title     string  `json:"title"`
	Path      string  `json:"path"`
	UpdatedAt string  `json:"updated_at"`
	Tags      []KBTag `json:"tags"`
}

// KBNote — карточка с текстом.
type KBNote struct {
	KBNoteSummary
	ContentMD string `json:"content_md"`
	// Тегов здесь НЕТ намеренно: KBNoteSummary уже несёт Tags, а второе поле
	// перекрывало его. Go встраивает родителя, но `json.Unmarshal` писал в
	// ВНЕШНЕЕ поле, а встроенное оставалось пустым — и признак материала,
	// взятый из полной заметки, терялся (живой дефект 2026-10-31: материалы
	// попадали в раздел «Системы»).
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

// forgetToken забывает закешированный токен: следующий запрос возьмёт новый.
func (c *KBClient) forgetToken() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = ""
	c.tokenAt = time.Time{}
}

// get выполняет запрос к kb-service и разбирает JSON.
//
// Ошибки kb-service НЕ переводятся в пустые значения: «нет прав» и «карточек
// нет» — разные вещи, и плагин должен показывать правду.
//
// Первый 401 — «этот токен не подошёл», а не «нет доступа»: токен
// сбрасывается и запрос повторяется РОВНО один раз с новым. Живой случай
// 2026-11-01: заход по сотням карточек шёл минутами, пара запросов в середине
// получала 401 («токен не разобран»), и этот отказ кешировался на весь заход,
// хотя токен был годным.
func (c *KBClient) get(ctx context.Context, path string, query url.Values, out any) error {
	if !c.configured() {
		return errKBNotConfigured
	}
	err := c.getOnce(ctx, path, query, out)
	if !isUnauthorized(err) {
		return err
	}
	log.Printf("kb: токен отклонён (%v), беру новый и повторяю запрос", err)
	c.forgetToken()
	return c.getOnce(ctx, path, query, out)
}

// isUnauthorized — отказ по токену (401), а не «нет прав» (403).
func isUnauthorized(err error) bool {
	return err != nil && strings.Contains(err.Error(), "ответила 401")
}

// getOnce — один запрос с текущим токеном.
func (c *KBClient) getOnce(ctx context.Context, path string, query url.Values, out any) error {
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
	return c.notes(ctx, "tag_id", tagID)
}

// FolderNotes — карточки из папки базы знаний (2026-11-08, «Боевой листок»).
//
// Папка — не тег: карточки конкурентов лежат в разделе «материалы и системы
// конкурентов», у них нет собственного тега. Фильтр по `folder_id` есть у
// kb-service (`GET /api/kb/notes?folder_id=…`), и пагинация у него та же, что
// по тегу.
func (c *KBClient) FolderNotes(ctx context.Context, folderID int64) ([]KBNoteSummary, error) {
	return c.notes(ctx, "folder_id", folderID)
}

// FolderIDByName — id папки базы знаний по имени. Папки нет — ошибка: карточки
// искать негде, и вызывающий должен отличить «папки нет» от «папка пуста».
//
// Сравнение имён — без учёта регистра и лишних пробелов: имя папки набирает
// человек, и «Материалы и системы конкурентов» против «материалы и системы
// конкурентов» — одна и та же папка.
func (c *KBClient) FolderIDByName(ctx context.Context, name string) (int64, error) {
	var out struct {
		Folders []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"folders"`
	}
	if err := c.get(ctx, "/api/kb/folders", nil, &out); err != nil {
		return 0, err
	}
	want := strings.ToLower(strings.TrimSpace(name))
	for _, f := range out.Folders {
		if strings.ToLower(strings.TrimSpace(f.Name)) == want {
			return f.ID, nil
		}
	}
	return 0, fmt.Errorf("в базе знаний нет папки %q", name)
}

// notes — общий цикл по страницам для чтения заметок по любому фильтру:
// `tag_id` или `folder_id`. Одно место на пагинацию: второй такой цикл
// разошёлся бы с первым в пределе страниц, как уже было с `size`.
func (c *KBClient) notes(ctx context.Context, filterKey string, filterID int64) ([]KBNoteSummary, error) {
	out := make([]KBNoteSummary, 0, kbPageSize)
	for page := 1; page <= maxKBPages; page++ {
		items, total, err := c.notesPage(ctx, filterKey, filterID, page)
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
func (c *KBClient) notesPage(ctx context.Context, filterKey string, filterID int64, page int) ([]KBNoteSummary, int, error) {
	q := url.Values{}
	q.Set(filterKey, fmt.Sprintf("%d", filterID))
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
