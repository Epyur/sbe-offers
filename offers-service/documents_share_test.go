package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mailers/offers/skills"
)

// Постоянные подписанные ссылки на файлы документов (2026-11-19).
//
// Живая жалоба: в листке 017/2026 две ссылки на документы не открывались —
// «unauthorized». База знаний пишет в карточку ПОСТОЯННУЮ ссылку
// `/api/documents/file?key=…`, а эта ручка закрыта JWT человека: из Word и
// Telegram её открывает аноним. Теперь в документ печатается наша подписанная
// ссылка `/api/offers/files?key=…&sig=…`: подпись детерминирована (хэш кэша
// DOCX не дрожит) и бессрочна, а видимость файла по-прежнему решает сервис
// документов — ручка лишь перенаправляет на свежую presigned-ссылку.

const testDocsFileURL = "https://epyur.fvds.ru/api/documents/file?key=documents%2F9458%2Fmain-%D0%94%D0%A1.pdf"

func withShareSecret(t *testing.T, secret string) {
	t.Helper()
	t.Setenv("OFFERS_APP_ID", "offers")
	t.Setenv("OFFERS_SERVICE_SECRET", secret)
}

// Ссылка документов переписывается в подписанную, прочие ссылки не трогаются.
func TestShareDocumentRefsSignsDocumentsLinks(t *testing.T) {
	withShareSecret(t, "test-share-secret")
	refs := []skills.CardRef{
		{Label: "ДС ФЗ 123", URL: testDocsFileURL},
		{Label: "Страница продукта", URL: "https://nav.tn.ru/catalog/x/"},
	}

	got := shareDocumentRefs(refs)
	if len(got) != 2 {
		t.Fatalf("ссылок стало %d", len(got))
	}
	if got[0].Label != "ДС ФЗ 123" {
		t.Fatalf("подпись ссылки потерялась: %q", got[0].Label)
	}
	u, err := url.Parse(got[0].URL)
	if err != nil {
		t.Fatalf("ссылка не разбирается: %v", err)
	}
	if u.Path != offersFilesPath {
		t.Fatalf("путь = %q, ждали %q", u.Path, offersFilesPath)
	}
	if u.Host != "epyur.fvds.ru" || u.Scheme != "https" {
		t.Fatalf("хост и схема должны остаться прежними: %q", got[0].URL)
	}
	if key := u.Query().Get("key"); key != "documents/9458/main-ДС.pdf" {
		t.Fatalf("ключ потерялся или перекодирован: %q", key)
	}
	if sig := u.Query().Get("sig"); sig == "" {
		t.Fatal("нет подписи в ссылке")
	}
	if got[1].URL != "https://nav.tn.ru/catalog/x/" {
		t.Fatalf("посторонняя ссылка переписана: %q", got[1].URL)
	}
}

// Без секрета подписывать нечем — ссылки остаются как есть (так работают тесты
// и стенд без окружения; на бою секрет есть всегда).
func TestShareDocumentRefsKeepsLinksWithoutSecret(t *testing.T) {
	withShareSecret(t, "")
	refs := []skills.CardRef{{Label: "ДС", URL: testDocsFileURL}}
	got := shareDocumentRefs(refs)
	if got[0].URL != testDocsFileURL {
		t.Fatalf("без секрета ссылка изменилась: %q", got[0].URL)
	}
}

// Подпись одна и та же для одного ключа и разная для разных — иначе кэш DOCX
// дрожал бы между сборками.
func TestDocumentShareSignatureIsStable(t *testing.T) {
	withShareSecret(t, "test-share-secret")
	first := documentShareSignature("documents/a.pdf", "test-share-secret")
	again := documentShareSignature("documents/a.pdf", "test-share-secret")
	other := documentShareSignature("documents/b.pdf", "test-share-secret")
	alien := documentShareSignature("documents/a.pdf", "other-secret")

	if first == "" {
		t.Fatal("пустая подпись")
	}
	if first != again {
		t.Fatalf("подпись нестабильна: %q / %q", first, again)
	}
	if first == other || first == alien {
		t.Fatal("подпись не различает ключ или секрет")
	}
}

// Сбор ссылок (единственный путь печати и хэша) отдаёт подписанные ссылки.
func TestCollectOfferRefsSignsDocumentsLinks(t *testing.T) {
	withShareSecret(t, "test-share-secret")
	card := SystemCard{
		NoteID: 6363,
		Title:  "LOGICPIR",
		Refs:   []skills.CardRef{{Label: "ДС", URL: testDocsFileURL}},
	}
	refs := collectOfferRefs([]SystemCard{card})
	if len(refs) != 1 {
		t.Fatalf("ссылок %d", len(refs))
	}
	if !strings.Contains(refs[0].URL, offersFilesPath) {
		t.Fatalf("в печать ушла неподписанная ссылка: %q", refs[0].URL)
	}
}

// --- ручка /api/offers/files -------------------------------------------

func docsStub(t *testing.T, link string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/documents/agent/file-link" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-Service-Secret") == "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"url":"` + link + `"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func withDocsService(t *testing.T, baseURL string) {
	t.Helper()
	if baseURL == "" {
		t.Setenv("DOCUMENTS_SERVICE_URL", "")
		t.Setenv("DOCUMENTS_INTERNAL_SECRET", "")
		return
	}
	t.Setenv("DOCUMENTS_SERVICE_URL", baseURL)
	t.Setenv("DOCUMENTS_INTERNAL_SECRET", "test-docs-secret")
}

func shareRequest(t *testing.T, key, sig string) *httptest.ResponseRecorder {
	t.Helper()
	q := url.Values{}
	if key != "" {
		q.Set("key", key)
	}
	if sig != "" {
		q.Set("sig", sig)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/offers/files?"+q.Encode(), nil)
	rec := httptest.NewRecorder()
	(&Server{}).handleSharedFile(rec, req)
	return rec
}

func TestHandleSharedFileRedirectsToPresigned(t *testing.T) {
	withShareSecret(t, "test-share-secret")
	link := "https://firsts3.ru/sbe-doc/documents/a.pdf?sign=abc"
	withDocsService(t, docsStub(t, link).URL)
	key := "documents/9458/main-ДС.pdf"

	rec := shareRequest(t, key, documentShareSignature(key, "test-share-secret"))

	if rec.Code != http.StatusFound {
		t.Fatalf("код %d, ждали 302: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != link {
		t.Fatalf("Location = %q, ждали %q", got, link)
	}
}

func TestHandleSharedFileRejectsBadSignature(t *testing.T) {
	withShareSecret(t, "test-share-secret")
	withDocsService(t, docsStub(t, "https://firsts3.ru/x").URL)

	rec := shareRequest(t, "documents/a.pdf", "подделка")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("код %d, ждали 403: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ссылка недействительна") {
		t.Fatalf("отказ не объясняет причину: %s", rec.Body.String())
	}
}

func TestHandleSharedFileWithoutKey(t *testing.T) {
	withShareSecret(t, "test-share-secret")
	withDocsService(t, docsStub(t, "https://firsts3.ru/x").URL)

	if rec := shareRequest(t, "", "x"); rec.Code != http.StatusBadRequest {
		t.Fatalf("без ключа код %d, ждали 400", rec.Code)
	}
}

// Сервис документов не подключён — честный 503, а не «файла нет».
func TestHandleSharedFileWithoutDocumentsService(t *testing.T) {
	withShareSecret(t, "test-share-secret")
	withDocsService(t, "")
	key := "documents/a.pdf"

	rec := shareRequest(t, key, documentShareSignature(key, "test-share-secret"))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("код %d, ждали 503: %s", rec.Code, rec.Body.String())
	}
}

// Документ не отдаётся (не общий, нет файла, ошибка S3) — 404 без подробностей.
func TestHandleSharedFileHidesRefusal(t *testing.T) {
	withShareSecret(t, "test-share-secret")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"file not found"}`))
	}))
	t.Cleanup(srv.Close)
	withDocsService(t, srv.URL)
	key := "documents/secret.pdf"

	rec := shareRequest(t, key, documentShareSignature(key, "test-share-secret"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("код %d, ждали 404: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "file not found") {
		t.Fatalf("чужой текст ошибки утёк наружу: %s", rec.Body.String())
	}
	_ = context.Background()
}
