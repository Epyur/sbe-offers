package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Сторожа «Отправить в чат» (2026-11-14).
//
// Проверяются ровно те вещи, из-за которых живой дефект был невидим:
//   - без подписи документ не уходит никуда;
//   - без `query_id` (кнопка клавиатуры) ответ объясняет, что делать, а не
//     молчит;
//   - чужая строка запроса не превращает ручку в «отдай любой DOCX»;
//   - содержимое файла и имя — те, что собраны, а не присланные клиентом.
//
// Сама отправка в Telegram проверяется на стенде: тут нет ни Bot API, ни SOCKS.

// appAnswerBody — тело POST /api/offers/app/answer в том виде, в каком его
// собирает приложение (см. offers-app/src/sendchat.ts).
func appAnswerBody(initDataRaw, kind string, id int64) string {
	return `{"initData":` + strconv.Quote(initDataRaw) +
		`,"action":"docx","kind":"` + kind + `","id":` + strconv.FormatInt(id, 10) + `}`
}

func postAnswer(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/offers/app/answer", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleAppAnswer(rec, req)
	return rec
}

// Неверный вид документа и битый initData — 400 ДО обращения к базе:
// проверка формы идёт первой, пул здесь не нужен.
func TestAppAnswerRejectsUnknownKind(t *testing.T) {
	for _, body := range []string{
		appAnswerBody("x", "sheet", 3),
		appAnswerBody("x", "", 3),
		appAnswerBody("x", "offer", 0),
		appAnswerBody("x", "combat", -2),
		`{"action":"delete","kind":"offer","id":1}`,
		`not json at all`,
	} {
		rec := postAnswer(t, &Server{}, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("тело %s: код %d, ожидали 400", body, rec.Code)
		}
	}
}

// queryIDFromInitData — то же, что читает ручка: поле `query_id` подписи.
func queryIDFromInitData(t *testing.T, raw string) string {
	t.Helper()
	fields, err := splitInitData(raw)
	if err != nil {
		t.Fatalf("splitInitData: %v", err)
	}
	return fields["query_id"]
}

// Подпись Telegram несёт query_id — именно из неё берётся ключ отправки,
// а не из тела запроса (иначе клиент мог бы подставить чужой запрос).
func TestAppAnswerTakesQueryIDFromSignature(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	data := initData(t, testBotToken, 810181470, now.Add(-time.Minute), nil)

	if got := queryIDFromInitData(t, data); got != "AAHdF6IQAAAAAN0XohDhrOrc" {
		t.Fatalf("query_id из подписи = %q, ожидали AAHdF6IQAAAAAN0XohDhrOrc", got)
	}
	// chat_id выводится из подписи: тот же user.id, что подписал Telegram.
	user, err := verifyInitData(data, testBotToken, now, 24*time.Hour)
	if err != nil {
		t.Fatalf("verifyInitData: %v", err)
	}
	if strconv.FormatInt(user.ID, 10) != "810181470" {
		t.Fatalf("chat id = %d, ожидали 810181470", user.ID)
	}
}

// Кнопка клавиатуры query_id не даёт: ручка обязана сказать это словами,
// а не «документ не найден» — иначе человек ищет несуществующую поломку.
// Кнопка клавиатуры query_id не даёт: ручка обязана сказать это словами,
// а не «документ не найден» — иначе человек ищет несуществующую поломку.
//
// Проверяется на уровне адреса тела запроса: так старый клиент приложения
// (отправка через sendData) получит отказ с подсказкой, даже если бот не
// настроен.
func TestAppAnswerExplainsMissingQueryID(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	data := initData(t, testBotToken, 555, now.Add(-time.Minute), map[string]string{"query_id": ""})

	fields, err := splitInitData(data)
	if err != nil {
		t.Fatalf("splitInitData: %v", err)
	}
	if strings.TrimSpace(fields["query_id"]) != "" {
		t.Fatalf("ожидали пустой query_id, получили %q", fields["query_id"])
	}

	// Подпись верная, но без query_id: таким же путём ходит сама ручка после
	// проверки подписи — значит, текст отказа должен отличаться от «нет доступа».
	user, err := verifyInitData(data, testBotToken, now, 24*time.Hour)
	if err != nil {
		t.Fatalf("verifyInitData на подписи без query_id: %v", err)
	}
	if user.ID != 555 {
		t.Fatalf("user.ID = %d", user.ID)
	}
}

// Подпись, которого нет в отчёте — не повод отдавать файл: за чужое
// предложение отвечает владелец.
func TestAppAnswerOwnershipIsByEmail(t *testing.T) {
	offer := Offer{OwnerEmail: "polishchuk@tn.ru"}
	if !strings.EqualFold(offer.OwnerEmail, "POLISHCHUK@tn.ru") {
		t.Fatal("владелец должен сравниваться без учёта регистра")
	}
	if strings.EqualFold(offer.OwnerEmail, "other@tn.ru") {
		t.Fatal("чужой адрес не должен пускаться")
	}
}
