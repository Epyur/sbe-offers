package main

// Заглушки чужих сервисов для тестов инструментов (без БД и без сети наружу).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestServerAgainstStubs поднимает два HTTP-заглушки: мейлер и документы,
// и отдаёт `Server` с клиентами, смотрящими на них через окружение.
//
// Заглушки проверяют заголовок секрета — без него ответ 403, как у живых ручек:
// тест, который забыл секрет, обязан покраснеть, а не пройти на «доброй»
// заглушке (правило P6: заглушка не должна быть добрее настоящей ручки).
func newTestServerAgainstStubs(t *testing.T, mailerBody, documentsBody string) *Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/mailer/agent/emails", stubJSON(mailerBody))
	mux.HandleFunc("/internal/mailer/agent/email", stubJSON(mailerBody))
	docs := http.NewServeMux()
	docs.HandleFunc("/internal/documents/agent/list", stubJSON(documentsBody))

	mailer := httptest.NewServer(mux)
	documents := httptest.NewServer(docs)
	t.Cleanup(mailer.Close)
	t.Cleanup(documents.Close)

	t.Setenv("MAILER_SERVICE_URL", mailer.URL)
	t.Setenv("MAILER_INTERNAL_SECRET", "test-mailer-secret")
	t.Setenv("DOCUMENTS_SERVICE_URL", documents.URL)
	t.Setenv("DOCUMENTS_INTERNAL_SECRET", "test-documents-secret")
	return &Server{}
}

// stubJSON — ответ заглушки: секрет обязателен, тело — записанное в тесте.
func stubJSON(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Service-Secret") == "" {
			http.Error(w, `{"error":"bad service secret"}`, http.StatusForbidden)
			return
		}
		var decoded any
		if err := json.Unmarshal([]byte(body), &decoded); err != nil {
			http.Error(w, `{"error":"bad fixture"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}
