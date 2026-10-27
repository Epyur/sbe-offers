package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"
)

const (
	codeTTLSignal   = 15 * time.Minute
	codeTTLAdmin    = 7 * 24 * time.Hour
	maxCodeAttempts = 5
)

// botCode — строка таблицы bot_codes.
type botCode struct {
	ID        int64
	Email     string
	Code      string
	Kind      string
	ChatID    string
	ExpiresAt time.Time
	UsedAt    *time.Time
	Attempts  int
}

// generateCode — шестизначный код входа.
func generateCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// verifyCodeLogic проверяет код против записи; пустая строка — проверка
// пройдена. now передаётся параметром, чтобы проверка была тестируемой.
func verifyCodeLogic(c botCode, input string, now time.Time) string {
	if c.UsedAt != nil {
		return "код уже использован"
	}
	if !now.Before(c.ExpiresAt) {
		return "код истёк — запросите новый"
	}
	if c.Attempts >= maxCodeAttempts {
		return "слишком много попыток — запросите новый код"
	}
	if strings.TrimSpace(input) != c.Code {
		return "неверный код"
	}
	return ""
}

// normalizeEmail приводит адрес к нижнему регистру и отвергает строки без «@».
func normalizeEmail(raw string) string {
	email := strings.ToLower(strings.TrimSpace(raw))
	if !strings.Contains(email, "@") || strings.HasPrefix(email, "@") || strings.HasSuffix(email, "@") {
		return ""
	}
	return email
}

func defaultRoleForNewUser() string {
	return "viewer"
}

// recordCode создаёт код входа и сразу возвращает его значение.
func (s *Server) recordCode(ctx context.Context, email, kind string, ttl time.Duration) (string, time.Time, error) {
	code, err := generateCode()
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().Add(ttl)
	_, err = s.pool.Exec(ctx, `
INSERT INTO bot_codes (email, code, kind, expires_at) VALUES ($1, $2, $3, $4)`,
		email, code, kind, expires)
	if err != nil {
		return "", time.Time{}, err
	}
	return code, expires, nil
}

// latestCode ищет действующий код для пары email+chat_id.
func (s *Server) latestCode(ctx context.Context, email, chatID string) (botCode, error) {
	var c botCode
	err := s.pool.QueryRow(ctx, `
SELECT id, email, code, kind, chat_id, expires_at, used_at, attempts
FROM bot_codes
WHERE email = $1 AND (chat_id = $2 OR ($2 <> '' AND kind = 'admin' AND chat_id = '')) AND used_at IS NULL
ORDER BY created_at DESC LIMIT 1`, email, chatID).
		Scan(&c.ID, &c.Email, &c.Code, &c.Kind, &c.ChatID, &c.ExpiresAt, &c.UsedAt, &c.Attempts)
	return c, err
}

func (s *Server) handleIssueAdminCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	email := normalizeEmail(req.Email)
	if email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "укажите корректный e-mail"})
		return
	}
	code, expires, err := s.recordCode(r.Context(), email, "admin", codeTTLAdmin)
	if err != nil {
		log.Printf("issue admin code: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "db error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": code, "expiresAt": expires.Format(time.RFC3339)})
}

func (s *Server) handleListAccess(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
SELECT email, role,
	to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
	to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
FROM offers_access ORDER BY updated_at DESC`)
	if err != nil {
		log.Printf("list access: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "db error"})
		return
	}
	defer rows.Close()
	type entry struct {
		Email     string `json:"email"`
		Role      string `json:"role"`
		CreatedAt string `json:"createdAt"`
		UpdatedAt string `json:"updatedAt"`
	}
	entries := make([]entry, 0)
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.Email, &e.Role, &e.CreatedAt, &e.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "db error"})
			return
		}
		entries = append(entries, e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (s *Server) handleSetAccessRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	email := normalizeEmail(req.Email)
	if email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "укажите корректный e-mail"})
		return
	}
	role := strings.TrimSpace(req.Role)
	if role != "viewer" && role != "editor" && role != "admin" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "роль должна быть viewer, editor или admin"})
		return
	}
	var saved struct {
		Email     string `json:"email"`
		Role      string `json:"role"`
		CreatedAt string `json:"createdAt"`
		UpdatedAt string `json:"updatedAt"`
	}
	err := s.pool.QueryRow(r.Context(), `
INSERT INTO offers_access (email, role) VALUES ($1, $2)
ON CONFLICT (email) DO UPDATE SET role = EXCLUDED.role, updated_at = now()
RETURNING email, role,
	to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
	to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')`, email, role).
		Scan(&saved.Email, &saved.Role, &saved.CreatedAt, &saved.UpdatedAt)
	if err != nil {
		log.Printf("set access role: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "db error"})
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// handleBotRequestCode — самозапрос кода из бота (открыт всем). Код уходит
// письмом; в ответе его нет.
func (s *Server) handleBotRequestCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email  string `json:"email"`
		ChatID string `json:"chat_id"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	email := normalizeEmail(req.Email)
	if email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "укажите корректный e-mail"})
		return
	}
	code, expires, err := s.recordCode(r.Context(), email, "signal", codeTTLSignal)
	if err != nil {
		log.Printf("bot request code: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "db error"})
		return
	}
	// Привязка chat_id к коду — чтобы подтвердить можно было только из того чата.
	if strings.TrimSpace(req.ChatID) != "" {
		_, _ = s.pool.Exec(r.Context(),
			`UPDATE bot_codes SET chat_id = $1 WHERE email = $2 AND code = $3 AND used_at IS NULL`,
			req.ChatID, email, code)
	}
	body := fmt.Sprintf("Код для входа в бота «Коммерческие предложения»: %s\n\nКод действует 15 минут. "+
		"Если вы не запрашивали код, просто проигнорируйте письмо.", code)
	if err := sendEmail(email, "Код входа в бота Коммерческие предложения", body); err != nil {
		log.Printf("bot request code: письмо не отправлено: %v", err)
		writeJSON(w, http.StatusServiceUnavailable,
			map[string]any{"error": "не удалось отправить письмо с кодом, попробуйте позже"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "code_sent", "expiresAt": expires.Format(time.RFC3339)})
}

// handleBotVerifyCode — проверка кода из бота: привязывает chat_id к email,
// заводит запись доступа (viewer, если её ещё нет) и возвращает роль.
func (s *Server) handleBotVerifyCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ChatID string `json:"chat_id"`
		Email  string `json:"email"`
		Code   string `json:"code"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	chatID := strings.TrimSpace(req.ChatID)
	email := normalizeEmail(req.Email)
	code := strings.TrimSpace(req.Code)
	if chatID == "" || email == "" || code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "нужны chat_id, email и code"})
		return
	}

	c, err := s.latestCode(r.Context(), email, chatID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "код не найден — запросите новый"})
		return
	}
	if msg := verifyCodeLogic(c, code, time.Now()); msg != "" {
		if msg == "неверный код" || strings.Contains(msg, "попыток") {
			// Инкремент только пока лимит не выбран: иначе чужой перебор
			// безгранично накручивал бы счётчик.
			_, _ = s.pool.Exec(r.Context(),
				`UPDATE bot_codes SET attempts = attempts + 1 WHERE id = $1 AND attempts < $2`,
				c.ID, maxCodeAttempts)
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
		return
	}

	_, err = s.pool.Exec(r.Context(), `UPDATE bot_codes SET used_at = now(), attempts = attempts + 1 WHERE id = $1`, c.ID)
	if err != nil {
		log.Printf("verify code mark used: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "db error"})
		return
	}
	_, err = s.pool.Exec(r.Context(), `
INSERT INTO offers_bot_link (chat_id, email) VALUES ($1, $2)
ON CONFLICT (chat_id) DO UPDATE SET email = EXCLUDED.email, confirmed_at = now()`, chatID, email)
	if err != nil {
		log.Printf("verify code link: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "db error"})
		return
	}
	_, err = s.pool.Exec(r.Context(), `
INSERT INTO offers_access (email, role) VALUES ($1, $2) ON CONFLICT (email) DO UPDATE SET updated_at = now()`,
		email, defaultRoleForNewUser())
	if err != nil {
		log.Printf("verify code access: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "db error"})
		return
	}

	role := s.roleForEmail(r.Context(), email)
	writeJSON(w, http.StatusOK, map[string]any{"email": email, "role": role})
}

// roleForEmail — роль человека в приложении. Источник правды — central
// auth-service (та же карта, что у плагина через /permissions/me): иначе
// назначение роли в ЦУП не доходило бы до бота, а отзыв доступа не действовал.
// Локальная offers_access хранит только факт первого входа и роль по умолчанию
// (viewer) — используется как запасной вариант, если auth-service недоступен.
func (s *Server) roleForEmail(ctx context.Context, email string) string {
	if role, err := s.effectiveRole(ctx, appIDFromEnv(), email); err == nil && role != "" {
		return role
	}
	var role string
	if err := s.pool.QueryRow(ctx,
		`SELECT role FROM offers_access WHERE email = $1`, normalizeEmail(email)).Scan(&role); err != nil {
		return ""
	}
	return role
}

func (s *Server) handleBotIdentity(w http.ResponseWriter, r *http.Request) {
	chatID := strings.TrimSpace(r.URL.Query().Get("chat_id"))
	if chatID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "chat_id is required"})
		return
	}
	var email string
	err := s.pool.QueryRow(r.Context(),
		`SELECT email FROM offers_bot_link WHERE chat_id = $1`, chatID).Scan(&email)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "chat_id не привязан"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"email": email, "role": s.roleForEmail(r.Context(), email)})
}

// requireServiceSecret защищает ручки бота: их вызывает только offers-bot со
// своим секретом.
func (s *Server) requireServiceSecret(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		expected := strings.TrimSpace(getEnv("OFFERS_BOT_SECRET"))
		if expected == "" {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "bot secret is not configured"})
			return
		}
		provided := strings.TrimSpace(r.Header.Get("X-Service-Secret"))
		if provided == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "service secret required"})
			return
		}
		if provided != expected {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "invalid service secret"})
			return
		}
		next(w, r)
	}
}

// botEncodeJSON — общий помощник для ручек бота.
func botEncodeJSON(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}
