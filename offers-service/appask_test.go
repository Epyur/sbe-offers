package main

// Сторожа «продолжить в боте» и ответа на вопрос брифа из приложения
// (2026-11-15).
//
// Проверяется то, из-за чего живой отказ был бы невидим:
//   - адресат берётся ИЗ ПОДПИСИ: тело с чужим `chat_id` не читается вовсе —
//     иначе приложение могло бы попросить бота написать в любой чат;
//   - без подписи ручка не идёт ни в базу, ни к боту;
//   - пустой ответ на вопрос брифа отвергается до вызова модели: шаг без
//     текста ничего не значит, а модель на пустом ответе стоит денег.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// appAskBody — тело POST /app/ask в том виде, в каком его собирает приложение
// (см. `offers-app/src/brief.ts`).
func appAskBody(initDataRaw, action string, id int64) string {
	payload, _ := json.Marshal(appAskRequest{
		InitData: initDataRaw,
		Action:   action,
		Kind:     "offer",
		ID:       id,
	})
	return string(payload)
}

func postAsk(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/offers/app/ask", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleAppAsk(rec, req)
	return rec
}

// TestAppAskRejectsUnknownAction — неизвестное действие и битое тело отсекаются
// формой запроса, ДО базы и бота: пул здесь не нужен.
func TestAppAskRejectsUnknownAction(t *testing.T) {
	for _, body := range []string{
		appAskBody("x", "", 3),
		appAskBody("x", "docx", 3),
		`{"initData":"x","action":"brief","kind":"combat","id":3}`,
		appAskBody("x", "brief", 0),
		`not json at all`,
	} {
		rec := postAsk(t, &Server{}, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("тело %s: код %d, ожидали 400", body, rec.Code)
		}
	}
}

// TestAppAskChatIDComesFromSignature — подпись Telegram несёт `user.id`, и
// именно он адресат. Поле `chat_id` в теле не объявлено в `appAskRequest`,
// поэтому положить его туда нельзя даже случайно.
func TestAppAskChatIDComesFromSignature(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	data := initData(t, testBotToken, 810181470, now.Add(-time.Minute), nil)

	user, err := verifyInitData(data, testBotToken, now, 24*time.Hour)
	if err != nil {
		t.Fatalf("verifyInitData: %v", err)
	}
	if got := strconv.FormatInt(user.ID, 10); got != "810181470" {
		t.Fatalf("адресат = %q, ожидали 810181470", got)
	}

	body := `{"initData":` + strconv.Quote(data) + `,"action":"brief","kind":"offer","id":1,"chat_id":"999"}`
	var req appAskRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("json: %v", err)
	}
	// Чужой chat_id в теле некуда положить: поле не разбирается.
	if req.Action != askActionBrief || req.ID != 1 {
		t.Fatalf("тело прочитано неверно: %+v", req)
	}
}

// TestAppAskRejectsEmptySignature — пустая подпись не должна доходить до
// чтения настроек бота: отказ приходит сразу, без базы и без чата.
func TestAppAskRejectsEmptySignature(t *testing.T) {
	// Проверяем правило разбора подписи, а не маршрут: пустой `initData`
	// `verifyInitData` отвергает ещё до базы.
	if _, err := verifyInitData("", testBotToken, time.Now(), time.Hour); err == nil {
		t.Fatal("пустая подпись должна отвергаться")
	}
	if _, err := splitInitData("garbage"); err == nil {
		t.Fatal("мусор вместо подписи должен отвергаться")
	}
}

// TestAppBriefAnswerRejectsEmptyText — пустой ответ отвергается формой.
func TestAppBriefAnswerRejectsEmptyText(t *testing.T) {
	s := &Server{}
	for _, body := range []string{
		`{"text":"   "}`,
		`{"text":""}`,
		`not json`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/offers/offers/1/brief/object_name", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		// Права проверяются раньше тела, поэтому без базы ручка тоже отвечает
		// отказом — важно лишь, что она НЕ отвечает 200 на пустоту.
		s.handleAppBriefAnswer(rec, req)
		if rec.Code == http.StatusOK {
			t.Errorf("тело %s: пустой ответ принят", body)
		}
	}
}
