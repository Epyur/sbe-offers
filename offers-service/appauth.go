package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Вход в Mini App по initData (2026-11-13, шаг S1 плана миграции).
//
// Telegram подписывает initData токеном бота (HMAC-SHA256, ключ
// HMAC("WebAppData", bot_token)). Токен бота живёт РОВНО в одном месте —
// таблица offers_bot_config (правило 2026-10-29: второй источник однажды уже
// отправлял уведомления чужим ботом три месяца). Поэтому проверка подписи —
// здесь, в сервисе, который токен и так знает; отдельного секрета не заводим.
//
// Приложение после проверки получает обычный JWT: роли проверяет
// существующий requirePerm, новых гейтов не появляется.

const (
	// initDataMaxBytes — потолок тела initData. Живая строка Telegram —
	// 300-800 байт; 16 КиБ отсекает попытку прислать мегабайт мусора.
	initDataMaxBytes = 16 * 1024
	// initDataDefaultTTL — сколько считается свежей подпись.
	initDataDefaultTTL = 24 * time.Hour
	// appJWTTTL — срок жизни выдаваемого JWT приложения. Как у плагина:
	// приложение при истечении повторно обменивает initData (он ещё жив).
	appJWTTTL = time.Hour
)

// telegramUser — поля user из initData. Хватает id: сами имена приложению
// не нужны, а лишние поля — лишний повод их случайно залогировать.
type telegramUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

// initDataTTL — срок свежести из OFFERS_APP_INITDATA_TTL. Мусор и
// неположительное значение — сутки: «0» иначе отключал бы вход целиком.
func initDataTTL() time.Duration {
	raw := strings.TrimSpace(os.Getenv("OFFERS_APP_INITDATA_TTL"))
	if raw == "" {
		return initDataDefaultTTL
	}
	ttl, err := time.ParseDuration(raw)
	if err != nil || ttl <= 0 {
		return initDataDefaultTTL
	}
	return ttl
}

// appInitDataError — отказ разбора initData с причиной для журнала и кодом.
//
// Текст ошибки уходит клиенту только коротким видом (см. appSessionReason);
// внутренние подробности (какое поле) — в журнал, потому что initData
// содержит имя и username человека.
type appInitDataError struct {
	reason string
	detail string
}

func (e *appInitDataError) Error() string {
	if e.detail == "" {
		return e.reason
	}
	return e.reason + ": " + e.detail
}

func initDataError(reason, detail string) error {
	return &appInitDataError{reason: reason, detail: detail}
}

// splitInitData разбирает строку query-string в пары без URL-декодирования
// ключей и проверяет, что ключ не повторяется.
//
// Повтор ключа — не стиль, а атака: Telegram строит data-check-string из
// карты, поэтому «две разные auth_date» дали бы одну подпись на два разных
// смысла. Оставляем только уникальные ключи.
func splitInitData(raw string) (map[string]string, error) {
	if raw == "" {
		return nil, initDataError("empty", "")
	}
	if len(raw) > initDataMaxBytes {
		return nil, initDataError("oversize", strconv.Itoa(len(raw)))
	}

	fields := make(map[string]string)
	for _, chunk := range strings.Split(raw, "&") {
		if chunk == "" {
			continue
		}
		eq := strings.Index(chunk, "=")
		if eq <= 0 {
			return nil, initDataError("malformed", "no equals sign")
		}
		key, err := url.QueryUnescape(chunk[:eq])
		if err != nil || key == "" {
			return nil, initDataError("malformed", "bad key")
		}
		if _, dup := fields[key]; dup {
			return nil, initDataError("duplicate-key", key)
		}
		value, err := url.QueryUnescape(chunk[eq+1:])
		if err != nil {
			return nil, initDataError("malformed", "bad value")
		}
		fields[key] = value
	}
	return fields, nil
}

// initDataHash — ожидаемая подпись Telegram для набора полей.
//
// data-check-string: «k=v», ключи по алфавиту, через \n, без самого hash.
// secret_key = HMAC_SHA256("WebAppData", bot_token) — строка в роли ключа.
func initDataHash(fields map[string]string, botToken string) string {
	pairs := make([]string, 0, len(fields))
	for key, value := range fields {
		if key == "hash" {
			continue
		}
		pairs = append(pairs, key+"="+value)
	}
	sortStrings(pairs)

	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(botToken))

	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(pairs, "\n")))
	return hex.EncodeToString(mac.Sum(nil))
}

// sortStrings — своя сортировка, чтобы не тянуть sort ради двенадцати строк
// и не спутать её с сортировкой по локали.
func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

// verifyInitData проверяет подпись и свежесть и возвращает пользователя.
//
// Порядок проверок намеренно такой: форма → дубликаты (в splitInitData) →
// подпись → свежесть → user. Подпись ДО разбора user: неподписанный user
// не разбираем вовсе.
func verifyInitData(raw, botToken string, now time.Time, ttl time.Duration) (telegramUser, error) {
	var zero telegramUser

	if strings.TrimSpace(botToken) == "" {
		return zero, initDataError("no-bot-token", "")
	}

	fields, err := splitInitData(raw)
	if err != nil {
		return zero, err
	}

	provided, ok := fields["hash"]
	if !ok || provided == "" {
		return zero, initDataError("no-hash", "")
	}

	expected := initDataHash(fields, botToken)
	// Постоянное время сравнения: подпись — это ключ доступа.
	if !hmac.Equal([]byte(strings.ToLower(provided)), []byte(expected)) {
		return zero, initDataError("bad-hash", "")
	}

	authDateRaw, ok := fields["auth_date"]
	if !ok {
		return zero, initDataError("no-auth-date", "")
	}
	authUnix, err := strconv.ParseInt(authDateRaw, 10, 64)
	if err != nil {
		return zero, initDataError("bad-auth-date", authDateRaw)
	}
	age := now.Sub(time.Unix(authUnix, 0))
	// Подпись из будущего — тоже отказ: часы не должны давать бессрочный вход.
	if age > ttl || age < -time.Minute {
		return zero, initDataError("stale", age.String())
	}

	userRaw, ok := fields["user"]
	if !ok || userRaw == "" {
		return zero, initDataError("no-user", "")
	}
	var user telegramUser
	if err := json.Unmarshal([]byte(userRaw), &user); err != nil {
		return zero, initDataError("bad-user", err.Error())
	}
	if user.ID <= 0 {
		return zero, initDataError("bad-user", "id must be positive")
	}
	return user, nil
}

// signAppJWT — обычный токен приложения, такой же, как выдаёт auth-service
// плагину Obsidian. Один формат на оба клиента: роли и проверки не меняются.
func signAppJWT(email, deviceID string, now time.Time) (string, error) {
	if len(jwtSecret) == 0 {
		return "", errors.New("jwt secret is not loaded")
	}
	appID := appIDFromEnv()

	claims := &jwtClaims{Email: email, DeviceID: deviceID, AppID: appID}
	claims.Issuer = "auth-service"
	claims.Audience = jwt.ClaimStrings{appID}
	claims.IssuedAt = jwt.NewNumericDate(now)
	claims.ExpiresAt = jwt.NewNumericDate(now.Add(appJWTTTL))

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
}

// logAppAuth — след отказа входа приложения. Строки initData здесь НЕТ и
// быть не может: это действующий ключ доступа.
func logAppAuth(reason, chatID string) {
	_, _ = fmt.Fprintf(authLogWriter, "app auth reject: %s chat=%s\n", reason, chatID)
}

// appSessionRequest — тело POST /api/offers/app/session.
type appSessionRequest struct {
	InitData string `json:"initData"`
}

// handleAppSession — обмен initData на JWT приложения.
//
// Коды ответа:
//
//	200 {token, user:{email, role}, offers, sheets}
//	400 — initData не той формы (битый, пустой, слишком большой)
//	401 — подпись не сходится, устарела или нет токена бота в настройках
//	403 {error:"not_linked"} — подпись верна, но человек не входил в бота
//
// 403 not_linked — это НЕ ошибка приложения: экран говорит «нажмите /start
// в боте», после входа бот выдаст ссылку на приложение снова.
func (s *Server) handleAppSession(w http.ResponseWriter, r *http.Request) {
	var req appSessionRequest
	if err := decodeJSON(w, r, &req, initDataMaxBytes*2); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}

	botToken, _, err := s.loadBotConfig(r.Context())
	if err != nil {
		dbError(w, "app session bot config", err)
		return
	}

	now := time.Now()
	user, err := verifyInitData(req.InitData, botToken, now, initDataTTL())
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
		dbError(w, "app session link", err)
		return
	}
	if email == "" {
		logAppAuth("not-linked", chatID)
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": "not_linked",
			"hint":  "Сначала войдите в бота: нажмите /start и введите код из письма",
		})
		return
	}

	token, err := signAppJWT(email, "app:"+chatID, now)
	if err != nil {
		log.Printf("app session sign: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "не удалось выдать токен"})
		return
	}

	role, err := s.effectiveRole(r.Context(), appIDFromEnv(), email)
	if err != nil {
		dbError(w, "app session role", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token":  token,
		"user":   map[string]any{"email": email, "role": role},
		"offers": s.countOwned(r.Context(), "offers_offer", email),
		"sheets": s.countOwned(r.Context(), "offers_combat_sheet", email),
	})
}

// emailForChatID — привязка «чат → почта». Пустая строка значит «не привязан»:
// это нормальный ответ для человека, который ещё не входил в бота, а не сбой.
func (s *Server) emailForChatID(ctx context.Context, chatID string) (string, error) {
	var email string
	err := s.pool.QueryRow(ctx,
		`SELECT email FROM offers_bot_link WHERE chat_id = $1`, chatID).Scan(&email)
	if err != nil {
		if isNoRows(err) {
			return "", nil
		}
		return "", err
	}
	return email, nil
}

// countOwned — сколько записей принадлежит человеку. Таблица приходит из
// константы в коде (не из запроса): подставлять имя таблицы извне — путь к
// внедрению SQL, а пользу даёт нулевую.
func (s *Server) countOwned(ctx context.Context, table, email string) int {
	switch table {
	case "offers_offer", "offers_combat_sheet":
	default:
		return 0
	}
	var n int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM `+table+` WHERE lower(owner_email) = lower($1)`, email).Scan(&n); err != nil {
		log.Printf("app session count %s: %v", table, err)
		return 0
	}
	return n
}

// appSessionError переводит причину отказа в код и текст для клиента.
// Внутренняя причина уже ушла в журнал: клиенту достаточно кода.
func appSessionError(err error) (int, map[string]any) {
	var appErr *appInitDataError
	if !errors.As(err, &appErr) {
		return http.StatusUnauthorized, map[string]any{"error": "initData", "detail": "invalid"}
	}
	switch appErr.reason {
	case "no-bot-token":
		// Настройка сервера, а не вина человека: 503 честнее 401.
		return http.StatusServiceUnavailable, map[string]any{
			"error":  "initData",
			"detail": "бот не настроен: введите токен в разделе «Бот и доступ»",
		}
	case "empty", "oversize", "malformed", "no-hash", "no-auth-date", "no-user", "bad-user", "bad-auth-date", "duplicate-key":
		return http.StatusBadRequest, map[string]any{"error": "initData", "detail": appErr.reason}
	default:
		return http.StatusUnauthorized, map[string]any{"error": "initData", "detail": appErr.reason}
	}
}

// handleAppConfig — что приложению и боту нужно знать о себе, не входя в
// него: публичный адрес Mini App и имя бота. Ручка открыта намеренно (гейта
// нет): здесь нет ни данных человека, ни секретов — то же, что в /health.
//
// Бот берёт отсюда адрес для кнопки `web_app`, приложение — для ссылок в
// чат. Одна точка вместо трёх мест, где адрес мог бы разойтись (2026-11-13).
func (s *Server) handleAppConfig(w http.ResponseWriter, r *http.Request) {
	appURL := strings.TrimSpace(getEnv("OFFERS_APP_URL"))
	if appURL == "" {
		appURL = "https://epyur.fvds.ru/app-offers/"
	}
	if !strings.HasSuffix(appURL, "/") {
		appURL += "/"
	}

	botUsername := ""
	if _, username, err := s.loadBotConfig(r.Context()); err == nil {
		botUsername = username
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"appUrl":      appURL,
		"botUsername": botUsername,
		"initDataTtl": int(initDataTTL().Seconds()),
	})
}

// appBuildInfo — версия сервиса для отладочной строки приложения (S1).'
// Читается из сборки, поэтому правка вручную здесь не нужна.
func appBuildInfo() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Main.Version
	}
	return "(devel)"
}
