package main

// Технодайджест в Mini App (2026-11-23).
//
// Четвёртая кнопка приложения («Технодайджест») была заглушкой: раздел
// проектировался. Новости для неё уже есть — это раздел «Дайджест»
// Базы знаний (kb-service, таблицы kb_digest_*). Читать его напрямую из
// Telegram-приложения нельзя: ключ Mini App выдан приложению `offers`, а
// kb-service отвергает чужой `app_id` («ключ выдан другому приложению»,
// kb-service/jwt.go, requirePerm). Поэтому карточки читает ЭТОТ сервис — тем же
// межсервисным токеном, что и карточки систем, — и отдаёт приложению короткий
// вид в camelCase.
//
// Границы, которые здесь держатся:
//   - в ленту попадают только ВЫШЕДШИЕ новости (`published`). Сервисный токен
//     может оказаться редакторским, а редактору kb-service отдаёт и очередь
//     модерации: показывать её в приложении значило бы раскрыть неопубликованное;
//   - месяц по умолчанию берётся у kb-service (`current`), а не считается здесь:
//     месяц он определяет по Москве, и вторая такая арифметика разошлась бы;
//   - «Отправить в чат» отправляет ИМЕННО ту новость, которая лежит в выпуске:
//     приложение называет месяц и номер, сервис перечитывает выпуск и берёт
//     текст из Базы знаний, а не из тела запроса;
//   - адресат — из подписи `initData` (как у «Отправить документ в чат»),
//     попросить чужой чат нельзя.

import (
	"context"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// digestYMRe — тот же вид месяца, что у kb-service: `2026-11`.
// Мусор отсекается на входе, до похода в сеть.
var digestYMRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// digestPublished — статус вышедшей новости (kb-service, digest.go).
const digestPublished = "published"

// digestIssueView — выпуск для ленты Mini App.
type digestIssueView struct {
	YM    string `json:"ym"`
	Title string `json:"title"`
	Items int    `json:"items"`
}

// digestNewsView — новость для ленты Mini App.
//
// Время уходит в RFC3339 (time.Time), а подпись «5 ноября» и сокращённую дату
// собирает клиент: формат показа — решение экрана, а не сервиса.
//
// `ImagePath` — наш подписанный ПУТЬ картинки (см. `appdigest_image.go`), а не
// адрес: приложение склеивает его со своим адресом API. Пустая строка означает
// «картинки нет» — и это тот случай, когда показывать нечего, а не ошибка.
type digestNewsView struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Lead      string `json:"lead"`
	Body      string `json:"body"`
	Rubric    string `json:"rubric"`
	Author    string `json:"author"`
	At        string `json:"at"`
	IsLead    bool   `json:"isLead"`
	LinkURL   string `json:"linkUrl"`
	LinkText  string `json:"linkText"`
	ImagePath string `json:"imagePath"`
}

// digestFeed — ответ GET /api/offers/digest.
//
// `issues` — навигация по месяцам, `issue` — то, что показано сейчас.
// Пустые списки отдаются массивами, никогда null: клиент читает `.length`.
type digestFeed struct {
	Current string            `json:"current"`
	Issues  []digestIssueView `json:"issues"`
	Issue   digestIssueView   `json:"issue"`
	Items   []digestNewsView  `json:"items"`
}

// handleAppDigest — лента Технодайджеста для Mini App.
func (s *Server) handleAppDigest(w http.ResponseWriter, r *http.Request) {
	// `configured()` сам понимает nil (kbclient.go), поэтому отдельная проверка
	// на nil была бы мёртвой веткой.
	if !s.kb.configured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "новости технодайджеста недоступны: База знаний не настроена",
		})
		return
	}
	ctx := r.Context()
	// Мусор в месяце отсекается ДО сети: свой параметр проверяем сразу, а месяц
	// по умолчанию приходит от Базы знаний и проверяется ниже.
	ym := strings.TrimSpace(r.URL.Query().Get("ym"))
	if ym != "" && !digestYMRe.MatchString(ym) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "месяц выпуска задан неверно — нужен вид 2026-11",
		})
		return
	}

	issues, current, err := s.kb.DigestIssues(ctx)
	if err != nil {
		log.Printf("digest: список выпусков не прочитан: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": "База знаний не ответила — новости не пришли",
		})
		return
	}
	if ym == "" {
		ym = current
	}
	if !digestYMRe.MatchString(ym) {
		log.Printf("digest: база знаний не назвала текущий выпуск (%q)", current)
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": "База знаний не назвала текущий выпуск",
		})
		return
	}

	issue, news, err := s.kb.DigestIssue(ctx, ym)
	if err != nil {
		if kbStatusOf(err) == http.StatusNotFound {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "такого выпуска нет"})
			return
		}
		log.Printf("digest %s: выпуск не прочитан: %v", ym, err)
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": "База знаний не ответила — новости не пришли",
		})
		return
	}

	shown := digestIssueViewOf(issue, ym)
	writeJSON(w, http.StatusOK, digestFeed{
		Current: current,
		Issues:  withShownIssue(digestIssueViews(issues), shown),
		Issue:   shown,
		Items:   digestNewsViews(news),
	})
}

// withShownIssue — показанный выпуск обязан быть в строке переключения.
//
// У текущего месяца строки в списке может ещё не быть: kb-service заводит её на
// лету при первом чтении выпуска, а список читается ДО этого. Без этой строки
// экран показывал бы новости месяца, которого нет среди кнопок.
func withShownIssue(views []digestIssueView, shown digestIssueView) []digestIssueView {
	for _, v := range views {
		if v.YM == shown.YM {
			return views
		}
	}
	return append([]digestIssueView{shown}, views...)
}

// digestIssueViews — выпуски для навигации. Пусто — пустой массив.
func digestIssueViews(issues []KBDigestIssue) []digestIssueView {
	out := make([]digestIssueView, 0, len(issues))
	for _, it := range issues {
		out = append(out, digestIssueViewOf(it, it.YM))
	}
	return out
}

// digestIssueViewOf — выпуск в виде ленты; пустое имя не остаётся пустым.
//
// Месяц берём у самого выпуска, если он его назвал: запрошенный `ym` — это то,
// что просили, а не то, что пришло (kb-service заводит текущий выпуск на лету и
// отвечает уже своей строкой).
func digestIssueViewOf(issue KBDigestIssue, ym string) digestIssueView {
	month := strings.TrimSpace(issue.YM)
	if month == "" {
		month = ym
	}
	title := strings.TrimSpace(issue.Title)
	if title == "" {
		title = "Технические новости " + month
	}
	return digestIssueView{YM: month, Title: title, Items: issue.Items}
}

// digestNewsViews — только вышедшие новости, в порядке полосы (его задал
// kb-service: ведущая первой, дальше по дате подачи).
func digestNewsViews(news []KBDigestNews) []digestNewsView {
	out := make([]digestNewsView, 0, len(news))
	for _, n := range news {
		if n.Status != digestPublished {
			continue
		}
		// Нулевое время — пустая дата, а не «01.01.0001» на экране.
		at := ""
		if !n.CreatedAt.IsZero() {
			at = n.CreatedAt.Format(time.RFC3339)
		}
		out = append(out, digestNewsView{
			ID:        n.ID,
			Title:     strings.TrimSpace(n.Title),
			Lead:      strings.TrimSpace(n.Lead),
			Body:      strings.TrimSpace(n.BodyMD),
			Rubric:    strings.TrimSpace(n.RubricName),
			Author:    digestAuthorOf(n),
			At:        at,
			IsLead:    n.IsLead,
			LinkURL:   strings.TrimSpace(n.LinkURL),
			LinkText:  strings.TrimSpace(n.LinkText),
			ImagePath: digestImageSignedPath(n.ImageHash, n.ImageName),
		})
	}
	return out
}

// digestAuthorOf — подпись автора. Имя не пришло — часть почты до собачки:
// так же подписывает новость и полоса в Базе знаний.
func digestAuthorOf(n KBDigestNews) string {
	if name := strings.TrimSpace(n.AuthorName); name != "" {
		return name
	}
	email := strings.TrimSpace(n.AuthorEmail)
	if at := strings.Index(email, "@"); at > 0 {
		return email[:at]
	}
	return email
}

// appNewsRequest — тело POST /api/offers/app/news.
type appNewsRequest struct {
	InitData string `json:"initData"`
	YM       string `json:"ym"`
	ID       int64  `json:"id"`
}

// handleAppNews — «Отправить в чат» у новости Технодайджеста.
//
// Текст новости сервис перечитывает из выпуска, а не берёт из запроса: иначе
// кнопка отправляла бы в чат то, что прислал клиент, а не то, что написано в
// Базе знаний. Номера в пути нет намеренно — новость не документ, у неё нет
// своей ручки чтения (в kb-service новости читаются только выпуском).
func (s *Server) handleAppNews(w http.ResponseWriter, r *http.Request) {
	var req appNewsRequest
	if err := decodeJSON(w, r, &req, answerInitDataMaxBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	ym := strings.TrimSpace(req.YM)
	if !digestYMRe.MatchString(ym) || req.ID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "новость не указана"})
		return
	}
	if !s.kb.configured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "новости технодайджеста недоступны: База знаний не настроена",
		})
		return
	}

	botToken, _, err := s.loadBotConfig(r.Context())
	if err != nil {
		dbError(w, "app news bot config", err)
		return
	}
	user, err := verifyInitData(req.InitData, botToken, time.Now(), initDataTTL())
	if err != nil {
		status, body := appSessionError(err)
		var appErr *appInitDataError
		if errors.As(err, &appErr) {
			logAppAuth(appErr.reason, "")
		}
		writeJSON(w, status, body)
		return
	}
	chatID := strconv.FormatInt(user.ID, 10)
	email, err := s.emailForChatID(r.Context(), chatID)
	if err != nil {
		dbError(w, "app news link", err)
		return
	}
	if email == "" {
		logAppAuth("not-linked", chatID)
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "not_linked"})
		return
	}

	issue, news, err := s.kb.DigestIssue(r.Context(), ym)
	if err != nil {
		log.Printf("digest %s: новость %d не прочитана: %v", ym, req.ID, err)
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": "База знаний не ответила — новость не отправлена",
		})
		return
	}
	newsItem, ok := digestNewsByID(news, req.ID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "новость не найдена"})
		return
	}

	ctx := context.WithoutCancel(r.Context())
	// ОДНО сообщение на новость (2026-11-23, жалоба пользователя: «новость не
	// должна резаться на несколько сообщений»). Форму выбираем до отправки:
	// кадр с подписью — только если ВЕСЬ текст новости влезает в подпись
	// Telegram (1000 знаков); иначе уходит один текстовый месседж. Второго
	// сообщения не бывает никогда: ни «шапка + текст», ни «кадр + текст».
	text := digestNewsForChat(issue, newsItem, ym)
	sent := false
	if digestFitsCaption(text) {
		if photo, fileName, ok := s.digestNewsPhoto(ctx, newsItem); ok {
			if err := s.sendChatPhoto(ctx, chatID, fileName, photo, text); err != nil {
				// Кадр не ушёл — новость всё равно должна дойти: текстом.
				log.Printf("digest: картинка новости %d в чат %s не ушла: %v", req.ID, chatID, err)
			} else {
				sent = true
			}
		}
	}
	if !sent {
		// `digestOneMessage` держит текст в пределах одного сообщения Telegram:
		// длинную новость бот не разрежет на части, а закончит многоточием.
		if err := s.sendChatText(ctx, chatID, digestOneMessage(text)); err != nil {
			// Причина — в журнал, человеку — свой текст: ответ бота («бот ответил
			// 403 forbidden») ничего не говорит менеджеру и раскрывает внутренности.
			log.Printf("digest: новость %d в чат %s не ушла: %v", req.ID, chatID, err)
			writeJSON(w, http.StatusBadGateway, map[string]any{
				"error": "Бот не смог отправить новость в чат — попробуйте ещё раз",
			})
			return
		}
	}
	log.Printf("digest: новость %d отправлена в чат %s (%s)", req.ID, chatID, email)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// digestNewsPhoto — картинка новости для чата.
//
// Три отказа вместо половины работы: нет хеша/имени, хранилище не ответило,
// файл тяжелее предела Telegram. Все три означают «шлём текстом» — новость
// важнее кадра, и человек не должен остаться без неё из-за картинки.
func (s *Server) digestNewsPhoto(ctx context.Context, n KBDigestNews) ([]byte, string, bool) {
	hash := strings.TrimSpace(n.ImageHash)
	name := strings.TrimSpace(n.ImageName)
	if !digestImageHashRe.MatchString(hash) || !digestImageNameOK(name) {
		return nil, "", false
	}
	if s.kb == nil || !s.kb.configured() {
		return nil, "", false
	}
	src := NewKBImageSource(s.kb)
	if src == nil {
		return nil, "", false
	}
	data, err := src.Get(ctx, digestImageObjectKey(hash, name))
	if err != nil {
		log.Printf("digest: картинка %s для чата не прочитана: %v", hash, err)
		return nil, "", false
	}
	if len(data) == 0 || len(data) > digestPhotoMaxBytes {
		log.Printf("digest: картинка %s не годится для чата (%d байт)", hash, len(data))
		return nil, "", false
	}
	if !strings.HasPrefix(http.DetectContentType(data), "image/") {
		log.Printf("digest: по хешу %s пришёл не образ", hash)
		return nil, "", false
	}
	return data, name, true
}

// digestNewsByID — вышедшая новость выпуска по номеру.
func digestNewsByID(news []KBDigestNews, id int64) (KBDigestNews, bool) {
	for _, n := range news {
		if n.ID == id && n.Status == digestPublished {
			return n, true
		}
	}
	return KBDigestNews{}, false
}

var (
	// Ссылка новости: только http(s) — ровно те адреса, что пускает
	// kb-service при подаче (digest_items.go, validLinkURL).
	digestLinkMarkdown = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^)\s]+)\)`)
	// Начало строки с заголовком, маркером списка или номером пункта.
	digestLineMarkdown = regexp.MustCompile(`(?m)^[ \t]*(?:#{1,6}[ \t]+|[-*•][ \t]+|[0-9]{1,2}[.)][ \t]+)`)
)

// digestPlainText — текст новости без markdown-разметки.
//
// Сообщение бота уходит простым текстом (`parse_mode` не задаём), поэтому
// «**жирный**» и «[подпись](адрес)» человек увидел бы звёздочками и скобками.
// Правило одно на все новости — и оно же проверяется тестом, а не глазами в
// чате. В приложении та же чистка живёт в `offers-app/src/digest.ts`: сервисы
// общие функции не делят, но правило обязано совпадать.
func digestPlainText(body string) string {
	out := strings.ReplaceAll(body, "\r\n", "\n")
	out = digestLinkMarkdown.ReplaceAllString(out, "$1 — $2")
	out = strings.ReplaceAll(out, "**", "")
	out = strings.ReplaceAll(out, "__", "")
	out = digestLineMarkdown.ReplaceAllString(out, "")
	return strings.TrimSpace(out)
}

// digestHeadlineForChat — шапка новости: выпуск, рубрика, заголовок.
//
// Одна и та же последовательность идёт и в подпись под картинкой, и в начало
// текстового сообщения — поэтому она собрана здесь, а не двумя похожими
// кусками кода (разошлись бы на первой же правке).
func digestHeadlineForChat(issue KBDigestIssue, n KBDigestNews, ym string) []string {
	parts := []string{"📰 " + digestIssueViewOf(issue, ym).Title}
	if rubric := strings.TrimSpace(n.RubricName); rubric != "" {
		parts = append(parts, rubric)
	}
	if title := strings.TrimSpace(n.Title); title != "" {
		parts = append(parts, title)
	}
	return parts
}

// digestNewsBodyForChat — новость БЕЗ шапки: анонс, текст, автор, источник.
//
// Отдельно от шапки, потому что она уже стоит в подписи под картинкой: без
// этого разделения одно и то же названо дважды подряд.
func digestNewsBodyForChat(n KBDigestNews) string {
	parts := []string{}
	if lead := strings.TrimSpace(n.Lead); lead != "" {
		parts = append(parts, lead)
	}
	if body := strings.TrimSpace(n.BodyMD); body != "" {
		parts = append(parts, digestPlainText(body))
	}
	byline := []string{}
	if author := digestAuthorOf(n); author != "" {
		byline = append(byline, author)
	}
	if !n.CreatedAt.IsZero() {
		byline = append(byline, n.CreatedAt.Format("02.01.2006"))
	}
	if len(byline) > 0 {
		parts = append(parts, strings.Join(byline, " · "))
	}
	if link := strings.TrimSpace(n.LinkURL); link != "" {
		text := strings.TrimSpace(n.LinkText)
		if text == "" {
			text = link
		}
		parts = append(parts, "Источник: "+text+" — "+link)
	}
	return strings.Join(parts, "\n\n")
}

// digestNewsForChat — текст сообщения целиком: шапка и новость.
//
// Формат один на все новости, поэтому он проверяется тестом, а не глазами в
// чате. Этим текстом новость уходит и тогда, когда картинка не отправилась.
func digestNewsForChat(issue KBDigestIssue, n KBDigestNews, ym string) string {
	parts := digestHeadlineForChat(issue, n, ym)
	if body := digestNewsBodyForChat(n); body != "" {
		parts = append(parts, body)
	}
	return strings.Join(parts, "\n\n")
}

// digestCaptionForChat — подпись под картинкой новости.
//
// Telegram пускает в подпись не больше 1024 знаков и считает СИМВОЛЫ, а не
// байты, поэтому предел сравнивается по рунам. Подпись — это ВСЯ новость
// (`digestNewsForChat`): своей короткой «шапки» больше нет, иначе новость уходила
// бы двумя сообщениями (2026-11-23, жалоба «не резать на несколько сообщений»).
func digestFitsCaption(text string) bool {
	return len([]rune(text)) <= digestCaptionLimit
}

// digestOneMessage — текст в пределах ОДНОГО сообщения Telegram.
//
// Telegram не примет больше 4096 знаков и не даст «продолжить» в следующем
// сообщении так, чтобы это выглядело одной новостью. Поэтому длинную новость
// бот заканчивает многоточием на границе слова: одно сообщение, честный обрыв
// виден. Реальные новости выпуска — 256–647 знаков, так что обрезка — запасной
// путь, а не обычный.
func digestOneMessage(text string) string {
	trimmed := strings.TrimSpace(text)
	runes := []rune(trimmed)
	if len(runes) <= digestMessageLimit {
		return trimmed
	}
	cut := runes[:digestMessageLimit]
	// Режем по последнему пробелу или переводу строки, чтобы не рвать слово.
	for i := len(cut) - 1; i > len(cut)/2; i-- {
		if cut[i] == ' ' || cut[i] == '\n' {
			cut = cut[:i]
			break
		}
	}
	return strings.TrimRight(string(cut), " \n") + "…"
}
