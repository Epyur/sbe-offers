package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Отказ «не ваш чат» приходит ДО чтения тела файла. Если не дочитать тело,
// клиент продолжает отправлять мегабайты в уже отвеченный запрос и виснет до
// таймаута (поймано живьём на стенде 2026-10-30: 1 МБ не проходил вовсе).
// Поэтому ранний отказ обязан вычитать тело — это часть контракта ручки.
func TestAttachEarlyRejectDrainsBody(t *testing.T) {
	payload := bytes.Repeat([]byte("A"), 4<<20)
	req := httptest.NewRequest(http.MethodPost, "/api/offers/bot/offers/1/attach?chat_id=", bytes.NewReader(payload))
	req.SetPathValue("id", "1")

	rec := httptest.NewRecorder()
	// Сервер без БД: emailForChat отвечает на пустой chat_id до обращения к базе.
	s := &Server{}
	s.handleBotAttach(rec, req)

	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
		t.Fatalf("ожидали ранний отказ, получили %d", rec.Code)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("ответ пустой: клиент не поймёт, что случилось")
	}
	// Ключевая проверка: после ответа тело дочитано, читать больше нечего.
	rest, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("тело не дочитано: %v", err)
	}
	if len(rest) != 0 {
		t.Fatalf("в теле осталось %d байт: клиент будет ждать, пока он не отправит их", len(rest))
	}
}
