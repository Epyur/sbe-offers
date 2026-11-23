package main

// Картинки Технодайджеста (2026-11-23): подписанная ссылка, отдача байтов и
// отправка кадра в чат.
//
// Проверяются правила, ради которых ручка сделана именно так:
//   - адрес картинки не отдаётся без подписи (гейт — подпись, JWT у `<img>` нет);
//   - подпись связана И с хешем, И с именем файла: по ней нельзя попросить
//     другой объект хранилища;
//   - в ленту уходит путь только у новости, у которой картинка есть;
//   - подпись под кадром обрезается по числу СИМВОЛОВ, а не байтов.

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const digestTestSecret = "digest-test-secret"

// digestImageRequest собирает запрос к ручке картинки из подписанного пути.
func digestImageRequest(t *testing.T, path string) *http.Request {
	t.Helper()
	query := strings.TrimPrefix(path, "/digest/image?")
	return httptest.NewRequest(http.MethodGet, "/api/offers/digest/image?"+query, nil)
}

func TestDigestImagePathNeedsSecret(t *testing.T) {
	t.Setenv("OFFERS_SERVICE_SECRET", "")
	if got := digestImageSignedPath("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "кадр.png"); got != "" {
		t.Fatalf("без секрета ссылка не должна собираться: %q", got)
	}

	t.Setenv("OFFERS_SERVICE_SECRET", digestTestSecret)
	got := digestImageSignedPath("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "кадр.png")
	if got == "" {
		t.Fatal("с секретом ссылка не собралась")
	}
	if !strings.HasPrefix(got, "/digest/image?") {
		t.Errorf("ссылка не того вида: %q", got)
	}
	for _, want := range []string{"hash=", "name=", "sig="} {
		if !strings.Contains(got, want) {
			t.Errorf("в адресе нет %q: %s", want, got)
		}
	}
}

// Мусор в хеше и в имени не превращается в ссылку: имя — часть ключа хранилища.
func TestDigestImagePathRejectsJunk(t *testing.T) {
	t.Setenv("OFFERS_SERVICE_SECRET", digestTestSecret)
	for _, c := range []struct{ hash, name string }{
		{"", "кадр.png"},
		{"короткий", "кадр.png"},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ""},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "../секрет.png"},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "папка/кадр.png"},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", `папка\кадр.png`},
	} {
		if got := digestImageSignedPath(c.hash, c.name); got != "" {
			t.Errorf("мусор (%q, %q) дал ссылку: %s", c.hash, c.name, got)
		}
	}
}

// Подпись связана с именем: та же подпись для другого файла не годится.
func TestDigestImageSignatureBindsName(t *testing.T) {
	t.Setenv("OFFERS_SERVICE_SECRET", digestTestSecret)
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	path := digestImageSignedPath(hash, "кадр.png")
	req := digestImageRequest(t, path)

	sig := req.URL.Query().Get("sig")
	if !digestImageSignatureValid(hash, "кадр.png", sig) {
		t.Fatal("подпись своего файла не принята")
	}
	if digestImageSignatureValid(hash, "чужой.png", sig) {
		t.Fatal("подпись принята для другого имени файла — по ней можно просить чужой объект")
	}
	if digestImageSignatureValid("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "кадр.png", sig) {
		t.Fatal("подпись принята для другого хеша")
	}
}

// Без подписи и без Базы знаний — отказ, а не картинка из ниоткуда.
func TestDigestImageRefusals(t *testing.T) {
	t.Setenv("OFFERS_SERVICE_SECRET", digestTestSecret)
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	cases := []struct {
		name   string
		target string
		want   int
	}{
		{"нет подписи", "/api/offers/digest/image?hash=" + hash + "&name=kadr.png", http.StatusForbidden},
		{"чужая подпись", "/api/offers/digest/image?hash=" + hash + "&name=kadr.png&sig=deadbeef", http.StatusForbidden},
		{"мусор в хеше", "/api/offers/digest/image?hash=xx&name=kadr.png&sig=deadbeef", http.StatusBadRequest},
		{"путь в имени", "/api/offers/digest/image?hash=" + hash + "&name=..%2Fx.png&sig=deadbeef", http.StatusBadRequest},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		(&Server{}).handleAppDigestImage(rec, httptest.NewRequest(http.MethodGet, c.target, nil))
		if rec.Code != c.want {
			t.Errorf("%s: код %d, ждали %d (%s)", c.name, rec.Code, c.want, rec.Body.String())
		}
	}

	// Подпись верна, но База знаний не настроена — честная причина, а не 404.
	rec := httptest.NewRecorder()
	(&Server{}).handleAppDigestImage(rec, digestImageRequest(t, digestImageSignedPath(hash, "kadr.png")))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("без Базы знаний код %d, ждали 503: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "База знаний") {
		t.Errorf("отказ не называет причину: %s", rec.Body.String())
	}
}

// Ключ хранилища: «kb/<хеш>/<имя>» — правило Базы знаний.
func TestDigestImageObjectKey(t *testing.T) {
	got := digestImageObjectKey("abc", "кадр.png")
	if got != "kb/abc/кадр.png" {
		t.Fatalf("ключ собран неверно: %q", got)
	}
}

// В ленте путь картинки есть ТОЛЬКО у новости с картинкой и только с секретом.
func TestDigestFeedCarriesImagePath(t *testing.T) {
	body := `{"issue":{"id":2,"ym":"2026-03","title":"Март"},"items":[
	 {"id":7,"title":"С кадром","status":"published","image_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","image_name":"кадр.png"},
	 {"id":8,"title":"Без кадра","status":"published"}]}`

	t.Setenv("OFFERS_SERVICE_SECRET", digestTestSecret)
	c, _ := digestStub(t, map[string]string{
		"/api/kb/digest/issues":  `{"issues":[],"current":"2026-03"}`,
		"/api/kb/digest/2026-03": body,
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
		t.Fatalf("новостей %d, ждали 2", len(feed.Items))
	}
	if feed.Items[0].ImagePath == "" {
		t.Fatalf("у новости с картинкой нет пути: %#v", feed.Items[0])
	}
	if !strings.Contains(feed.Items[0].ImagePath, "sig=") {
		t.Errorf("путь картинки без подписи: %s", feed.Items[0].ImagePath)
	}
	if feed.Items[1].ImagePath != "" {
		t.Errorf("у новости без картинки появился путь: %q", feed.Items[1].ImagePath)
	}

	// Секрета нет — лента остаётся без картинок: неподписанная ссылка всегда 403.
	t.Setenv("OFFERS_SERVICE_SECRET", "")
	rec = digestGet(t, &Server{kb: c}, "/api/offers/digest")
	var noSecret digestFeed
	if err := json.Unmarshal(rec.Body.Bytes(), &noSecret); err != nil {
		t.Fatalf("ответ не разобран: %v", err)
	}
	if len(noSecret.Items) != 2 || noSecret.Items[0].ImagePath != "" {
		t.Errorf("без секрета путь картинки не должен появляться: %#v", noSecret.Items)
	}
}

// Новость уходит ОДНИМ сообщением: либо целиком в подписи под кадром, либо
// целым текстом — но никогда двумя сообщениями.
func TestDigestOneMessage(t *testing.T) {
	issue := KBDigestIssue{YM: "2026-11", Title: "Технические новости ноября 2026"}
	news := KBDigestNews{
		ID: 7, Title: "Новая мембрана", RubricName: "Материалы",
		Lead: "Коротко о главном", BodyMD: "Первый абзац.",
		Status: digestPublished,
	}
	full := digestNewsForChat(issue, news, "2026-11")
	// Живые новости (256–647 знаков) целиком влезают в подпись — тогда уходит
	// кадр с полным текстом, и это одно сообщение.
	if !digestFitsCaption(full) {
		t.Fatalf("короткая новость не влезла в подпись (%d знаков):\n%s", len([]rune(full)), full)
	}
	// Заголовок не повторяется: шапка — часть того же сообщения, а не второй.
	if strings.Count(full, "Новая мембрана") != 1 {
		t.Errorf("заголовок в сообщении не один раз:\n%s", full)
	}
	for _, want := range []string{"Технические новости ноября 2026", "Материалы", "Коротко о главном", "Первый абзац."} {
		if !strings.Contains(full, want) {
			t.Errorf("в сообщении нет %q:\n%s", want, full)
		}
	}

	// Длинная новость: одно сообщение, обрыв помечен многоточием.
	longNews := KBDigestNews{
		ID: 8, Title: "Длинная новость",
		BodyMD: strings.Repeat("Очень длинный абзац текста новости. ", 400),
		Status: digestPublished,
	}
	long := digestOneMessage(digestNewsForChat(issue, longNews, "2026-11"))
	if len([]rune(long)) > digestMessageLimit+1 {
		t.Fatalf("сообщение длиннее предела: %d знаков", len([]rune(long)))
	}
	if !strings.HasSuffix(long, "…") {
		t.Errorf("обрезанное сообщение не помечено многоточием")
	}
	if strings.Contains(long, "  ") || strings.HasSuffix(strings.TrimSuffix(long, "…"), " ") {
		t.Errorf("обрезка оставила висячий пробел")
	}
	// Пустой текст не превращается в «…».
	if got := digestOneMessage("   "); got != "" {
		t.Errorf("пустой текст дал %q", got)
	}
}

// Текст после картинки не повторяет шапку: она уже в подписи под кадром.
func TestDigestBodyForChatHasNoHeadline(t *testing.T) {
	news := KBDigestNews{
		Title: "Новая мембрана", RubricName: "Материалы",
		Lead: "Коротко о главном", BodyMD: "Первый абзац.",
		Status: digestPublished,
	}
	body := digestNewsBodyForChat(news)
	if strings.Contains(body, "Новая мембрана") || strings.Contains(body, "📰") {
		t.Errorf("в тексте повторилась шапка:\n%s", body)
	}
	for _, want := range []string{"Коротко о главном", "Первый абзац."} {
		if !strings.Contains(body, want) {
			t.Errorf("в тексте нет %q:\n%s", want, body)
		}
	}
}

// Транспорт «картинка ботом в чат»: секрет, адресат, подпись и содержимое.
func TestSendChatPhoto(t *testing.T) {
	var got struct {
		ChatID  string `json:"chat_id"`
		Name    string `json:"file_name"`
		Caption string `json:"caption"`
		Content string `json:"content"`
	}
	secret := ""
	bot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/photo" {
			t.Errorf("запрос ушёл не в /internal/photo: %s", r.URL.Path)
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

	photo := []byte{0xff, 0xd8, 0xff, 0xdb}
	if err := (&Server{}).sendChatPhoto(t.Context(), "42", "кадр.jpg", photo, "подпись"); err != nil {
		t.Fatalf("отправка не прошла: %v", err)
	}
	if got.ChatID != "42" || got.Name != "кадр.jpg" || got.Caption != "подпись" {
		t.Errorf("боту ушло не то: %#v", got)
	}
	decoded, err := base64.StdEncoding.DecodeString(got.Content)
	if err != nil || string(decoded) != string(photo) {
		t.Errorf("содержимое доехало неверно: %q", got.Content)
	}
	if secret != "bot-secret" {
		t.Errorf("секрет бота не передан: %q", secret)
	}
}

// Отказ бота (кадра не будет) — ошибка, а новость всё равно уходит текстом:
// это решает вызывающий, поэтому транспорт обязан сказать правду.
func TestSendChatPhotoRefusal(t *testing.T) {
	bot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"ok":false,"error":"Telegram sendPhoto не удался"}`))
	}))
	t.Cleanup(bot.Close)
	t.Setenv("OFFERS_BOT_URL", bot.URL)
	t.Setenv("OFFERS_BOT_SECRET", "bot-secret")

	err := (&Server{}).sendChatPhoto(t.Context(), "42", "кадр.jpg", []byte{0xff, 0xd8, 0xff}, "подпись")
	if err == nil {
		t.Fatal("отказ бота проглочен")
	}
	if !strings.Contains(err.Error(), "sendPhoto") {
		t.Errorf("ошибка не называет причину: %v", err)
	}
}
