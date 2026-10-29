package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// botTokenVerifyTimeout — сколько ждём проверку токена у бота. Тот ходит в
// Telegram через SOCKS-туннель, поэтому запас больше, чем у обычного запроса.
const botTokenVerifyTimeout = 20 * time.Second

// botConfigResponse — ответ плагину. Токена здесь нет и не будет: человеку
// нужно знать, задан ли он и чьим ботом работает, а не значение.
type botConfigResponse struct {
	Configured  bool   `json:"configured"`
	BotUsername string `json:"botUsername"`
	UpdatedBy   string `json:"updatedBy"`
	UpdatedAt   string `json:"updatedAt"`
}

// botStatusResponse — состояние бота для строки статуса в плагине.
type botStatusResponse struct {
	Configured  bool   `json:"configured"`
	Running     bool   `json:"running"`
	BotUsername string `json:"botUsername"`
	LastError   string `json:"lastError"`
	CheckedAt   string `json:"checkedAt"`
	UpdatedBy   string `json:"updatedBy"`
	UpdatedAt   string `json:"updatedAt"`
}

const botConfigTimeSQL = `to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')`

// normalizeBotToken — единая точка приведения токена к рабочему виду. Пробелы
// и переводы строк — самая частая беда при копировании из @BotFather.
func normalizeBotToken(raw string) string {
	return strings.TrimSpace(raw)
}

// looksLikeBotToken — дешёвая проверка формы до обращения к Telegram:
// «123456789:AAH...». Не заменяет проверку getMe, только отсекает явную ерунду.
func looksLikeBotToken(token string) bool {
	colon := strings.Index(token, ":")
	if colon <= 0 || colon >= len(token)-1 {
		return false
	}
	for _, r := range token[:colon] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// loadBotConfig читает строку настроек. Отсутствие строки — не ошибка:
// значит администратор ещё не вводил токен.
func (s *Server) loadBotConfig(ctx context.Context) (token, username string, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT bot_token, bot_username FROM offers_bot_config WHERE id = 1`).Scan(&token, &username)
	if err != nil {
		if isNoRows(err) {
			return "", "", nil
		}
		return "", "", err
	}
	return token, username, nil
}

func (s *Server) botConfigInfo(ctx context.Context) (botConfigResponse, error) {
	var res botConfigResponse
	var token string
	err := s.pool.QueryRow(ctx,
		`SELECT bot_token, bot_username, updated_by, `+botConfigTimeSQL+`
		 FROM offers_bot_config WHERE id = 1`).
		Scan(&token, &res.BotUsername, &res.UpdatedBy, &res.UpdatedAt)
	if err != nil {
		if isNoRows(err) {
			return botConfigResponse{Configured: false}, nil
		}
		return botConfigResponse{}, err
	}
	res.Configured = token != ""
	return res, nil
}

// botVerifyReply — ответ бота на проверку токена.
type botVerifyReply struct {
	Ok       bool   `json:"ok"`
	Username string `json:"username"`
	Error    string `json:"error"`
}

// askBotToVerify отправляет кандидата в токене боту и возвращает его ответ.
func askBotToVerify(ctx context.Context, token string) (*botVerifyReply, int, error) {
	base := strings.TrimRight(getEnv("OFFERS_BOT_URL"), "/")
	if base == "" {
		base = "http://offers-bot:3000"
	}
	payload, err := json.Marshal(map[string]string{"token": token})
	if err != nil {
		return nil, 0, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, botTokenVerifyTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, base+"/internal/verify", bytes.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Secret", getEnv("OFFERS_BOT_SECRET"))
	resp, err := (&http.Client{Timeout: botTokenVerifyTimeout}).Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("бот недоступен: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var parsed botVerifyReply
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, resp.StatusCode, errors.New("бот ответил не по формату")
	}
	return &parsed, resp.StatusCode, nil
}

// verifyTokenViaBot — единственный способ проверить токен: с сервера Telegram
// недоступен (диапазоны заблокированы хостингом), ходит в него только бот через
// SOCKS-сайдкар. Бот отвечает getMe через свою ручку /internal/verify.
func (s *Server) verifyTokenViaBot(ctx context.Context, token string) (string, error) {
	reply, status, err := askBotToVerify(ctx, token)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK || !reply.Ok {
		if reply.Error != "" {
			return "", errors.New(reply.Error)
		}
		return "", fmt.Errorf("бот ответил %d", status)
	}
	return reply.Username, nil
}

func (s *Server) saveBotConfig(ctx context.Context, token, username, actor string) error {
	_, err := s.pool.Exec(ctx, `
INSERT INTO offers_bot_config (id, bot_token, bot_username, updated_by, bot_last_error)
VALUES (1, $1, $2, $3, '')
ON CONFLICT (id) DO UPDATE SET
	bot_token = EXCLUDED.bot_token,
	bot_username = EXCLUDED.bot_username,
	updated_by = EXCLUDED.updated_by,
	updated_at = now(),
	bot_last_error = ''`, token, username, strings.TrimSpace(actor))
	return err
}

func (s *Server) handleGetBotConfig(w http.ResponseWriter, r *http.Request) {
	res, err := s.botConfigInfo(r.Context())
	if err != nil {
		dbError(w, "bot config", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleSetBotConfig — «Проверить и сохранить» из плагина. Порядок намеренный:
// сначала проверка токена через бота, и только потом запись. Иначе опечатка
// заменила бы рабочий токен неработающим, и бот замолчал бы отнюдь не очевидно.
func (s *Server) handleSetBotConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	token := normalizeBotToken(req.Token)
	if token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "сначала укажите токен бота"})
		return
	}
	if !looksLikeBotToken(token) {
		writeJSON(w, http.StatusBadRequest,
			map[string]any{"error": "токен не похож на тот, что выдаёт @BotFather (вида 123456789:AA…)"})
		return
	}
	username, err := s.verifyTokenViaBot(r.Context(), token)
	if err != nil {
		// Не сохраняем: незачем держать токен, который ни разу не проверен.
		writeJSON(w, http.StatusBadRequest,
			map[string]any{"error": "токен не подошёл: " + err.Error()})
		return
	}
	if err := s.saveBotConfig(r.Context(), token, username, actorEmailFromCtx(r)); err != nil {
		dbError(w, "save bot config", err)
		return
	}
	res, err := s.botConfigInfo(r.Context())
	if err != nil {
		dbError(w, "bot config", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleDeleteBotConfig — «Удалить токен»: бот перестаёт принимать сообщения в
// пределах кэша (15 минут на стороне бота нет — там 15 секунд).
func (s *Server) handleDeleteBotConfig(w http.ResponseWriter, r *http.Request) {
	_, err := s.pool.Exec(r.Context(), `
UPDATE offers_bot_config
SET bot_token = '', bot_username = '', updated_by = $1, updated_at = now()
WHERE id = 1`, strings.TrimSpace(actorEmailFromCtx(r)))
	if err != nil {
		dbError(w, "delete bot config", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"configured": false})
}

// handleBotStatus — живое состояние для строки статуса. running берём из
// отметки бота (heartbeat), а не из запроса к нему: страница не должна ждать
// сеть до контейнера бота.
func (s *Server) handleBotStatus(w http.ResponseWriter, r *http.Request) {
	var res botStatusResponse
	var token string
	var lastSeen *time.Time
	err := s.pool.QueryRow(r.Context(), `
SELECT bot_token, bot_username, updated_by, `+botConfigTimeSQL+`, bot_last_seen_at, bot_last_error
FROM offers_bot_config WHERE id = 1`).
		Scan(&token, &res.BotUsername, &res.UpdatedBy, &res.UpdatedAt, &lastSeen, &res.LastError)
	if err != nil && !isNoRows(err) {
		dbError(w, "bot status", err)
		return
	}
	res.Configured = token != ""
	// Живая отметка — не старше минуты. Бот пишет её на каждой итерации
	// поллинга (раз в 25 секунд), так что минута — это два подряд пропуска.
	res.Running = res.Configured && lastSeen != nil && time.Since(*lastSeen) < time.Minute
	res.CheckedAt = time.Now().UTC().Format(time.RFC3339)
	writeJSON(w, http.StatusOK, res)
}

// handleBotToken — внутренняя ручка для offers-bot: единственный источник
// токена. Пустая строка означает «не задан», это не ошибка: бот подождёт.
func (s *Server) handleBotToken(w http.ResponseWriter, r *http.Request) {
	token, username, err := s.loadBotConfig(r.Context())
	if err != nil {
		dbError(w, "bot token", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "username": username})
}

// handleBotHeartbeat — бот отмечается, что жив, и сообщает последнюю ошибку.
// Так состояние видно в плагине без обращения к контейнеру бота.
func (s *Server) handleBotHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LastError string `json:"last_error"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	lastError := strings.TrimSpace(req.LastError)
	if len(lastError) > 500 {
		lastError = lastError[:500]
	}
	// Строки может ещё не быть (никто не вводил токен), а бот уже жив и
	// отмечается. Поэтому INSERT ... ON CONFLICT, а не UPDATE: пустой UPDATE
	// молча не записывал ничего, и плагин не видел, что бот работает.
	_, err := s.pool.Exec(r.Context(), `
INSERT INTO offers_bot_config (id, bot_token, bot_last_seen_at, bot_last_error)
VALUES (1, '', now(), $1)
ON CONFLICT (id) DO UPDATE SET
	bot_last_seen_at = now(),
	bot_last_error = EXCLUDED.bot_last_error`, lastError)
	if err != nil {
		dbError(w, "bot heartbeat", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// isNoRows — отсутствие строки настроек. Отдельная функция, чтобы обе ветки
// («ещё не настраивали» и «ошибка базы») читались одинаково во всех ручках.
func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
