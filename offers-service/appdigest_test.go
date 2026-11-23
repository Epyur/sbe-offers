package main

// Лента Технодайджеста в Mini App (2026-11-23).
//
// Проверяем не «ходит ли HTTP», а правила, ради которых сервис вообще
// появился: в ленту попадают только вышедшие новости, месяц по умолчанию
// берётся у Базы знаний, мусор в пути не доходит до сети, а пустые списки
// приходят массивами (клиент читает .length и падал бы на null).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// digestStub — подставные auth-service и kb-service: ответ по пути, пути
// запоминаются, чтобы проверить «куда пошли».
func digestStub(t *testing.T, bodies map[string]string) (*KBClient, *[]string) {
	t.Helper()
	paths := &[]string{}
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/service-token", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jwt":"kb-jwt"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		*paths = append(*paths, r.URL.Path)
		body, ok := bodies[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"такого выпуска нет"}`))
			return
		}
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &KBClient{
		baseURL:    srv.URL,
		authURL:    srv.URL,
		serviceKey: "offers-service-secret",
		client:     srv.Client(),
	}, paths
}

// Месяц в фикстуре ВЫМЫШЛЕННЫЙ и заведомо не текущий (2026-03): иначе проверка
// «месяц берётся у Базы знаний» зеленела бы и при подмене на календарь машины —
// правило «a check that cannot fail proves nothing».
const digestIssuesBody = `{"issues":[
 {"id":2,"ym":"2026-03","title":"Технические новости марта 2026","items":2},
 {"id":1,"ym":"2026-02","title":"Технические новости февраля 2026","items":5}],
 "current":"2026-03"}`

const digestIssueBody = `{"issue":{"id":2,"ym":"2026-03","title":"Технические новости марта 2026","items":2},
 "items":[
  {"id":7,"title":"Новая мембрана","lead":"Коротко о главном","body_md":"Первый абзац.\n\nВторой.","rubric_name":"Материалы","author_name":"Иван Петров","author_email":"petrov@tn.ru","link_url":"https://example.org/news","link_text":"Подробности","status":"published","is_lead":true,"created_at":"2026-03-05T09:00:00Z"},
  {"id":8,"title":"Черновик на модерации","body_md":"Ещё не вышло","status":"pending","created_at":"2026-03-06T09:00:00Z"},
  {"id":9,"title":"Вторая новость","author_email":"sidorov@tn.ru","status":"published","created_at":"2026-03-07T09:00:00Z"}]}`

func digestGet(t *testing.T, s *Server, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	s.handleAppDigest(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

// В ленту идут только вышедшие новости: редакторская очередь модерации — не
// газета, и сервисный токен вполне может оказаться редакторским.
func TestDigestFeedOnlyPublished(t *testing.T) {
	c, _ := digestStub(t, map[string]string{
		"/api/kb/digest/issues":  digestIssuesBody,
		"/api/kb/digest/2026-03": digestIssueBody,
	})
	rec := digestGet(t, &Server{kb: c}, "/api/offers/digest")

	if rec.Code != http.StatusOK {
		t.Fatalf("код %d: %s", rec.Code, rec.Body.String())
	}
	var feed digestFeed
	if err := json.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("ответ не разобран: %v", err)
	}
	if len(feed.Items) != 2 {
		t.Fatalf("в ленте %d новостей, ждали 2 (черновик не должен попасть): %#v", len(feed.Items), feed.Items)
	}
	for _, it := range feed.Items {
		if strings.Contains(it.Title, "Черновик") {
			t.Errorf("неопубликованная новость попала в ленту: %#v", it)
		}
	}
	first := feed.Items[0]
	if first.Title != "Новая мембрана" || first.Rubric != "Материалы" || first.Author != "Иван Петров" {
		t.Errorf("новость разобрана неверно: %#v", first)
	}
	if first.LinkURL != "https://example.org/news" || !first.IsLead {
		t.Errorf("ссылка или признак ведущей потеряны: %#v", first)
	}
	if feed.Issue.YM != "2026-03" || feed.Current != "2026-03" || len(feed.Issues) != 2 {
		t.Errorf("выпуски разобраны неверно: %#v", feed)
	}
}

// Без `ym` показываем месяц, который назвала База знаний, а не свой расчёт.
func TestDigestDefaultMonthFromKB(t *testing.T) {
	c, paths := digestStub(t, map[string]string{
		"/api/kb/digest/issues":  digestIssuesBody,
		"/api/kb/digest/2026-03": digestIssueBody,
	})
	rec := digestGet(t, &Server{kb: c}, "/api/offers/digest")
	if rec.Code != http.StatusOK {
		t.Fatalf("код %d: %s", rec.Code, rec.Body.String())
	}
	want := "/api/kb/digest/2026-03"
	found := false
	for _, p := range *paths {
		if p == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("выпуск не запрошен по месяцу сервиса: %v", *paths)
	}
}

// Мусор в месяце отсекается до сети: «2026-13» — не выпуск.
func TestDigestBadMonthIsRefusedBeforeKB(t *testing.T) {
	c, paths := digestStub(t, map[string]string{"/api/kb/digest/issues": digestIssuesBody})
	rec := digestGet(t, &Server{kb: c}, "/api/offers/digest?ym=2026-13")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("код %d, ждали 400", rec.Code)
	}
	if len(*paths) != 0 {
		t.Fatalf("мусорный месяц дошёл до сети: %v", *paths)
	}
}

// База знаний не назвала текущий выпуск — не запрос в пустоту, а отказ словами.
func TestDigestWithoutCurrentMonth(t *testing.T) {
	c, paths := digestStub(t, map[string]string{"/api/kb/digest/issues": `{"issues":[],"current":""}`})
	rec := digestGet(t, &Server{kb: c}, "/api/offers/digest")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("код %d, ждали 502: %s", rec.Code, rec.Body.String())
	}
	for _, p := range *paths {
		if p != "/api/kb/digest/issues" {
			t.Fatalf("пошли за выпуском без месяца: %v", *paths)
		}
	}
}

// Нет выпуска — честный 404, а не пустая газета.
func TestDigestMissingIssueIs404(t *testing.T) {
	c, _ := digestStub(t, map[string]string{"/api/kb/digest/issues": digestIssuesBody})
	rec := digestGet(t, &Server{kb: c}, "/api/offers/digest?ym=2025-01")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("код %d: %s", rec.Code, rec.Body.String())
	}
}

// База знаний не настроена — не пустая лента, а причина словами.
func TestDigestWithoutKBExplainsItself(t *testing.T) {
	rec := digestGet(t, &Server{}, "/api/offers/digest")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("код %d, ждали 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "База знаний") {
		t.Errorf("отказ не называет причину: %s", rec.Body.String())
	}
}

// Пустые списки — массивы, никогда null (клиент читает .length).
func TestDigestEmptyListsAreArrays(t *testing.T) {
	c, _ := digestStub(t, map[string]string{
		"/api/kb/digest/issues":  `{"issues":[],"current":"2026-03"}`,
		"/api/kb/digest/2026-03": `{"issue":{"id":1,"ym":"2026-03","title":""},"items":[]}`,
	})
	rec := digestGet(t, &Server{kb: c}, "/api/offers/digest")
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("код %d: %s", rec.Code, body)
	}
	if !strings.Contains(body, `"items":[]`) || strings.Contains(body, `"items":null`) {
		t.Fatalf("пустые новости пришли не массивом: %s", body)
	}
	// Строка переключения всё равно содержит показанный месяц — и это массив.
	if strings.Contains(body, `"issues":null`) || !strings.Contains(body, `"issues":[`) {
		t.Fatalf("пустой список выпусков пришёл не массивом: %s", body)
	}
}

// Показанный выпуск обязан быть в переключателе, даже если списка он ещё не
// успел попасть: у текущего месяца строку заводит само чтение выпуска.
func TestDigestAddsShownMonthToIssues(t *testing.T) {
	c, _ := digestStub(t, map[string]string{
		"/api/kb/digest/issues": `{"issues":[{"id":1,"ym":"2026-02","title":"Февраль","items":3}],
			"current":"2026-03"}`,
		"/api/kb/digest/2026-03": `{"issue":{"id":2,"ym":"2026-03","title":"Март"},"items":[]}`,
	})
	rec := digestGet(t, &Server{kb: c}, "/api/offers/digest")
	if rec.Code != http.StatusOK {
		t.Fatalf("код %d: %s", rec.Code, rec.Body.String())
	}
	var feed digestFeed
	if err := json.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("ответ не разобран: %v", err)
	}
	if len(feed.Issues) != 2 || feed.Issues[0].YM != "2026-03" || feed.Issues[1].YM != "2026-02" {
		t.Fatalf("переключатель месяцев собран неверно: %#v", feed.Issues)
	}
}

// Текст для чата: заголовок выпуска, рубрика, новость, автор, источник.
func TestDigestNewsForChat(t *testing.T) {
	issue := KBDigestIssue{YM: "2026-11", Title: "Технические новости ноября 2026"}
	news := KBDigestNews{
		ID: 7, Title: "Новая мембрана", Lead: "Коротко о главном",
		BodyMD: "Первый абзац.", RubricName: "Материалы",
		AuthorName: "Иван Петров", LinkURL: "https://example.org/news", LinkText: "Подробности",
		CreatedAt: time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC),
		Status:    digestPublished,
	}
	text := digestNewsForChat(issue, news, "2026-11")
	for _, want := range []string{
		"Технические новости ноября 2026", "Материалы", "Новая мембрана",
		"Коротко о главном", "Первый абзац.", "Иван Петров", "05.11.2026",
		"https://example.org/news", "Подробности",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("в тексте нет %q:\n%s", want, text)
		}
	}
}

// Без ссылки и без имени — без пустых строк «Источник:» и висячего «·».
func TestDigestNewsForChatWithoutLink(t *testing.T) {
	news := KBDigestNews{Title: "Без ссылки", AuthorEmail: "sidorov@tn.ru", Status: digestPublished}
	text := digestNewsForChat(KBDigestIssue{YM: "2026-11"}, news, "2026-11")
	if strings.Contains(text, "Источник") {
		t.Errorf("нет ссылки — нет строки источника:\n%s", text)
	}
	if !strings.Contains(text, "sidorov") {
		t.Errorf("имя не взято из почты:\n%s", text)
	}
	if !strings.Contains(text, "Технические новости 2026-11") {
		t.Errorf("пустое имя выпуска не заменено: %s", text)
	}
}

// Номер неопубликованной новости не отправляется в чат.
func TestDigestNewsByIDSkipsPending(t *testing.T) {
	news := []KBDigestNews{
		{ID: 8, Status: "pending"},
		{ID: 9, Status: digestPublished},
	}
	if _, ok := digestNewsByID(news, 8); ok {
		t.Error("новость на модерации найдена — она не должна уходить в чат")
	}
	if _, ok := digestNewsByID(news, 9); !ok {
		t.Error("вышедшая новость не найдена")
	}
	if _, ok := digestNewsByID(news, 10); ok {
		t.Error("несуществующий номер найден")
	}
}

// Мусор в теле «Отправить в чат» отсекается до базы и до бота.
func TestAppNewsRejectsBadRequest(t *testing.T) {
	for _, body := range []string{
		`{"initData":"x","ym":"2026-13","id":7}`,
		`{"initData":"x","ym":"2026-11","id":0}`,
		`не json`,
	} {
		rec := httptest.NewRecorder()
		(&Server{}).handleAppNews(rec, httptest.NewRequest(http.MethodPost, "/api/offers/app/news", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("тело %q дало код %d, ждали 400", body, rec.Code)
		}
	}
}

// Месяц выпуска берём из ответа Базы знаний, а не из запроса: kb-service мог
// завести текущий выпуск на лету и назвать его сам.
func TestDigestIssueMonthFromKB(t *testing.T) {
	c, _ := digestStub(t, map[string]string{
		"/api/kb/digest/issues":  `{"issues":[],"current":"2026-03"}`,
		"/api/kb/digest/2026-03": `{"issue":{"id":2,"ym":"2026-07","title":"Июль"},"items":[]}`,
	})
	rec := digestGet(t, &Server{kb: c}, "/api/offers/digest?ym=2026-03")
	if rec.Code != http.StatusOK {
		t.Fatalf("код %d: %s", rec.Code, rec.Body.String())
	}
	var feed digestFeed
	if err := json.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("ответ не разобран: %v", err)
	}
	if feed.Issue.YM != "2026-07" {
		t.Errorf("месяц взят не из ответа Базы знаний: %#v", feed.Issue)
	}
	if len(feed.Issues) != 1 || feed.Issues[0].YM != "2026-07" {
		t.Errorf("показанный выпуск не попал в переключатель: %#v", feed.Issues)
	}
}

// Текст новости уходит в чат без markdown: сообщение печатается как есть.
func TestDigestPlainText(t *testing.T) {
	body := "## Подзаголовок\n\n**Первый** абзац.\n\n- пункт\n- второй\n\n[Подробности](https://example.org/news)"
	got := digestPlainText(body)
	for _, unwanted := range []string{"**", "##", "- пункт", "](https"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("в тексте осталась разметка %q:\n%s", unwanted, got)
		}
	}
	for _, want := range []string{"Подзаголовок", "Первый абзац.", "пункт", "второй", "Подробности — https://example.org/news"} {
		if !strings.Contains(got, want) {
			t.Errorf("в тексте нет %q:\n%s", want, got)
		}
	}
}

// Текст для чата несёт уже очищенный текст, а не сырой markdown.
func TestDigestNewsForChatStripsMarkdown(t *testing.T) {
	news := KBDigestNews{
		Title: "Новость", BodyMD: "**Важно.** Ссылка: [сайт](https://example.org).",
		Status: digestPublished,
	}
	text := digestNewsForChat(KBDigestIssue{YM: "2026-03"}, news, "2026-03")
	if strings.Contains(text, "**") || strings.Contains(text, "](https") {
		t.Fatalf("разметка дошла до сообщения:\n%s", text)
	}
	if !strings.Contains(text, "Важно.") || !strings.Contains(text, "сайт — https://example.org") {
		t.Fatalf("текст потерял смысл:\n%s", text)
	}
}

// Нулевое время не превращается в «01.01.0001» на экране.
func TestDigestZeroTimeIsEmpty(t *testing.T) {
	views := digestNewsViews([]KBDigestNews{{ID: 1, Title: "Без даты", Status: digestPublished}})
	if len(views) != 1 || views[0].At != "" {
		t.Fatalf("нулевая дата ушла в ленту: %#v", views)
	}
}

// Транспорт «сообщение ботом в чат»: тот самый путь, которым уходит новость.
func TestSendChatText(t *testing.T) {
	var got struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}
	secret := ""
	bot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/notify" {
			t.Errorf("запрос ушёл не в notify: %s", r.URL.Path)
		}
		secret = r.Header.Get("X-Service-Secret")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("тело не разобрано: %v", err)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(bot.Close)
	t.Setenv("OFFERS_BOT_URL", bot.URL)
	t.Setenv("OFFERS_BOT_SECRET", "bot-secret")

	if err := (&Server{}).sendChatText(context.Background(), "42", "привет"); err != nil {
		t.Fatalf("отправка не прошла: %v", err)
	}
	if got.ChatID != "42" || got.Text != "привет" {
		t.Errorf("боту ушло не то: %#v", got)
	}
	if secret != "bot-secret" {
		t.Errorf("секрет бота не передан: %q", secret)
	}
}

// Отказ бота становится ошибкой (её видит нажавший), а не тишиной.
func TestSendChatTextRefusal(t *testing.T) {
	bot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"ok":false,"error":"бот ещё не готов"}`))
	}))
	t.Cleanup(bot.Close)
	t.Setenv("OFFERS_BOT_URL", bot.URL)
	t.Setenv("OFFERS_BOT_SECRET", "bot-secret")

	err := (&Server{}).sendChatText(context.Background(), "42", "привет")
	if err == nil {
		t.Fatal("отказ бота проглочен")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("ошибка не называет код: %v", err)
	}
}
