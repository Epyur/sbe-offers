package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// Клиент Базы знаний (шаг 1 пересмотренного плана 2026-10-29).
//
// offers-service ходит в kb-service за карточками систем. kb-service требует
// JWT с app_id=kb, а не служебный секрет: поэтому клиент сначала берёт
// межсервисный токен у auth-service (тем же путём, что lab-service для EKN).
//
// Сеть подменяется httptest-ом: проверяем не «ходит ли HTTP», а что клиент
// собирает правильные адреса, кеширует токен и не молчит при отказе.

// kbStub — подставные auth-service и kb-service в одном сервере.
type kbStub struct {
	t          *testing.T
	tokenCalls int32
	kbCalls    int32
	tokenValue string
	tokenFail  bool
	kbFail     int // HTTP-код отказа kb (0 — успех)
	lastPath   string
	lastAuth   string
	lastQuery  string
	body       string
}

func (s *kbStub) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/service-token", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&s.tokenCalls, 1)
		if s.tokenFail {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		var req map[string]string
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["target_app_id"] != "kb" {
			s.t.Errorf("запрошен токен не для kb: %v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"jwt": s.tokenValue})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&s.kbCalls, 1)
		s.lastPath = r.URL.Path
		s.lastAuth = r.Header.Get("Authorization")
		s.lastQuery = r.URL.RawQuery
		if s.kbFail != 0 {
			w.WriteHeader(s.kbFail)
			_, _ = w.Write([]byte(`{"error":"denied"}`))
			return
		}
		_, _ = w.Write([]byte(s.body))
	})
	return mux
}

func newKBStub(t *testing.T) (*kbStub, *KBClient) {
	t.Helper()
	s := &kbStub{t: t, tokenValue: "kb-jwt"}
	srv := httptest.NewServer(s.handler())
	t.Cleanup(srv.Close)
	c := &KBClient{
		baseURL:      srv.URL,
		authURL:      srv.URL,
		serviceKey:   "offers-service-secret",
		client:       srv.Client(),
		tokenFactory: nil, // будет через authURL
	}
	return s, c
}

func TestKBClientTagID(t *testing.T) {
	s, c := newKBStub(t)
	s.body = `{"tags":[{"id":1,"name":"прочее"},{"id":7,"name":"commercial_propose"}]}`
	id, err := c.TagID(context.Background(), "commercial_propose")
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if id != 7 {
		t.Fatalf("ожидали тег 7, получили %d", id)
	}
	if !strings.Contains(s.lastAuth, "Bearer kb-jwt") {
		t.Errorf("токен не передан: %q", s.lastAuth)
	}
}

// Тег не найден — понятная ошибка, а не 0 и не пустой список карточек.
func TestKBClientTagMissing(t *testing.T) {
	s, c := newKBStub(t)
	s.body = `{"tags":[{"id":1,"name":"прочее"}]}`
	if _, err := c.TagID(context.Background(), "commercial_propose"); err == nil {
		t.Fatal("отсутствие тега должно быть ошибкой")
	}
}

// Токен кешируется: два вызова — один запрос к auth-service. Иначе на каждый
// лист карточек мы бы дёргали auth-service.
func TestKBClientCachesToken(t *testing.T) {
	s, c := newKBStub(t)
	s.body = `{"notes":[]}`
	for i := 0; i < 3; i++ {
		if _, err := c.Notes(context.Background(), 7); err != nil {
			t.Fatalf("ошибка: %v", err)
		}
	}
	if got := atomic.LoadInt32(&s.tokenCalls); got != 1 {
		t.Fatalf("токен запрашивался %d раз, ожидали 1", got)
	}
}

// Отказ kb-service (403) — ошибка с кодом, а не пустой список: иначе плагин
// покажет «карточек нет» вместо «нет прав».
func TestKBClientForbidden(t *testing.T) {
	s, c := newKBStub(t)
	s.kbFail = http.StatusForbidden
	_, err := c.Notes(context.Background(), 7)
	if err == nil {
		t.Fatal("403 должен быть ошибкой")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("в ошибке нет кода: %v", err)
	}
}

// Отказ auth-service — тоже ошибка, и текст не должен врать «нет карточек».
func TestKBClientNoToken(t *testing.T) {
	s, c := newKBStub(t)
	s.tokenFail = true
	_, err := c.Notes(context.Background(), 7)
	if err == nil {
		t.Fatal("отказ auth-service должен быть ошибкой")
	}
}

// Нет настроенного адреса kb — честный отказ «не настроено», на нём держится
// отказ от карточек без падения всего КП.
func TestKBClientNotConfigured(t *testing.T) {
	c := &KBClient{}
	if _, err := c.Notes(context.Background(), 7); err == nil {
		t.Fatal("без адреса должна быть ошибка")
	}
}

// Список карточек: путь и фильтр по тегу.
func TestKBClientNotesQuery(t *testing.T) {
	s, c := newKBStub(t)
	s.body = `{"notes":[{"id":5,"title":"ТН-Стилобат","path":"База/Системы/ТН-s.md"}]}`
	notes, err := c.Notes(context.Background(), 7)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(notes) != 1 || notes[0].Title != "ТН-Стилобат" {
		t.Fatalf("список разобран неверно: %#v", notes)
	}
	if !strings.Contains(s.lastQuery, "tag_id=7") {
		t.Errorf("в запросе нет tag_id: %q", s.lastQuery)
	}
}

// Полная карточка: Markdown и теги.
func TestKBClientNote(t *testing.T) {
	s, c := newKBStub(t)
	s.body = `{"id":5,"title":"Система","content_md":"# Система\nтекст","tags":[{"id":7,"name":"commercial_propose"}]}`
	n, err := c.Note(context.Background(), 5)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if !strings.Contains(n.ContentMD, "текст") {
		t.Fatalf("текст карточки потерян: %q", n.ContentMD)
	}
	if len(n.Tags) != 1 || n.Tags[0].Name != "commercial_propose" {
		t.Fatalf("теги потеряны: %#v", n.Tags)
	}
}

// Вложения карточки: имя, mime и ключ — из них выбирается картинка продукта.
func TestKBClientAttachments(t *testing.T) {
	s, c := newKBStub(t)
	s.body = `{"attachments":[{"filename":"ТН_Стилобат.png","mime":"image/png","key":"att/abc.png"}]}`
	atts, err := c.Attachments(context.Background(), 5)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(atts) != 1 || atts[0].Key != "att/abc.png" {
		t.Fatalf("вложения разобраны неверно: %#v", atts)
	}
}

// Ссылка на файл — отдельная ручка со своим query.
func TestKBClientFileLink(t *testing.T) {
	s, c := newKBStub(t)
	s.body = `{"url":"https://s3.example/att/abc.png"}`
	url, err := c.FileLink(context.Background(), "att/abc.png")
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if url == "" {
		t.Fatal("ссылка пустая")
	}
	if !strings.Contains(s.lastQuery, "key=att") {
		t.Errorf("в запросе нет ключа: %q", s.lastQuery)
	}
}

// Настоящий kb-service отдаёт список в поле items, а не notes (2026-10-30).
// Тест подставлял выдуманное поле и не ловил, что карточки на бою не читаются:
// соединение есть, kbEnabled=true, а список всегда пуст.
func TestKBClientNotesReadsItemsField(t *testing.T) {
	s, c := newKBStub(t)
	s.body = `{"items":[{"id":5605,"title":"ТН-СТИЛОБАТ ЭКСПЕРТ ТРОТУАР","path":"Инженерная гидроизоляция/Системы/ТН.md"}],"total":2,"page":1,"size":200}`
	notes, err := c.Notes(context.Background(), 11181)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(notes) != 1 || notes[0].Title != "ТН-СТИЛОБАТ ЭКСПЕРТ ТРОТУАР" {
		t.Fatalf("поле items не разобрано: %#v", notes)
	}
}

// Старое поле notes тоже понимаем: ответ сервиса мог отличаться в прошлых версиях.
func TestKBClientNotesReadsLegacyNotesField(t *testing.T) {
	s, c := newKBStub(t)
	s.body = `{"notes":[{"id":1,"title":"Старая карточка"}]}`
	notes, err := c.Notes(context.Background(), 7)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(notes) != 1 || notes[0].Title != "Старая карточка" {
		t.Fatalf("поле notes перестало читаться: %#v", notes)
	}
}

// Пагинация карточек (2026-10-30). Живой дефект: kb-service отдаёт максимум
// 50 заметок на запрос при любом size, но сообщает total. Клиент просил
// size=200, получал 50 и считал, что это всё — из 588 карточек плагин видел 50.
//
// Заглушка ниже ведёт себя как настоящий сервис: страница не больше лимита,
// и всегда есть total.
type pagedKBStub struct {
	t     *testing.T
	total int
	limit int
	calls int
	sizes []string
	pages []string
}

func (s *pagedKBStub) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/service-token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"jwt": "kb-jwt"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/kb/notes") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		s.calls++
		q := r.URL.Query()
		page := q.Get("page")
		if page == "" {
			page = "1"
		}
		s.sizes = append(s.sizes, q.Get("size"))
		s.pages = append(s.pages, page)

		pageNo := 0
		_, _ = fmt.Sscanf(page, "%d", &pageNo)
		if pageNo < 1 {
			pageNo = 1
		}
		from := (pageNo - 1) * s.limit
		to := from + s.limit
		if from > s.total {
			from = s.total
		}
		if to > s.total {
			to = s.total
		}
		items := make([]map[string]any, 0, s.limit)
		for i := from; i < to; i++ {
			items = append(items, map[string]any{
				"id": i + 1, "title": fmt.Sprintf("Карточка %d", i+1),
				"path": fmt.Sprintf("Папка/Карточка %d.md", i+1),
				"uuid": fmt.Sprintf("uuid-%d", i+1),
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": items, "total": s.total, "page": pageNo, "size": s.limit,
		})
	})
	return mux
}

func newPagedKBStub(t *testing.T, total, limit int) (*pagedKBStub, *KBClient) {
	t.Helper()
	s := &pagedKBStub{t: t, total: total, limit: limit}
	srv := httptest.NewServer(s.handler())
	t.Cleanup(srv.Close)
	return s, &KBClient{baseURL: srv.URL, authURL: srv.URL, serviceKey: "k", client: srv.Client()}
}

// Сервис режет выдачу до 50 при total=130 — клиент обязан собрать все 130.
func TestKBClientNotesPaginatesAllPages(t *testing.T) {
	s, c := newPagedKBStub(t, 130, 50)
	notes, err := c.Notes(context.Background(), 7)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(notes) != 130 {
		t.Fatalf("собрано %d карточек из 130 — пагинация не работает", len(notes))
	}
	if s.calls != 3 {
		t.Fatalf("ожидали 3 запроса (50+50+30), сделали %d: %v", s.calls, s.pages)
	}
	if notes[0].ID != 1 || notes[129].ID != 130 {
		t.Fatalf("порядок или содержимое страниц нарушены: первый %d, последний %d", notes[0].ID, notes[129].ID)
	}
}

// Ровно одна полная страница: лишнего запроса быть не должно.
func TestKBClientNotesStopsWhenPageNotFull(t *testing.T) {
	s, c := newPagedKBStub(t, 50, 50)
	notes, err := c.Notes(context.Background(), 7)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(notes) != 50 || s.calls != 1 {
		t.Fatalf("ожидали 50 карточек за один запрос, получили %d за %d", len(notes), s.calls)
	}
}

// Сервис, который всегда отдаёт полную страницу, не должен крутить цикл вечно.
func TestKBClientNotesHasPageCap(t *testing.T) {
	// total заведомо больше, чем можно получить при лимите maxKBPages.
	s, c := newPagedKBStub(t, 1000000, 50)
	notes, err := c.Notes(context.Background(), 7)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if s.calls > maxKBPages {
		t.Fatalf("запросов %d, предел %d — цикл не ограничен", s.calls, maxKBPages)
	}
	if len(notes) != s.calls*50 {
		t.Fatalf("карточек %d, а страниц %d по 50", len(notes), s.calls)
	}
}

// Карточек меньше одной страницы — один запрос, без хвостов.
func TestKBClientNotesSinglePartialPage(t *testing.T) {
	s, c := newPagedKBStub(t, 12, 50)
	notes, err := c.Notes(context.Background(), 7)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(notes) != 12 || s.calls != 1 {
		t.Fatalf("ожидали 12 карточек за один запрос, получили %d за %d", len(notes), s.calls)
	}
}
