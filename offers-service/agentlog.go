package main

// Журнал ходов агента (2026-11-14, шаг P6).
//
// Дизайн §«The manager-analysis journal»: одна строка на ход. Поля — только то,
// что менеджер и так видит (его текст, ответ), плюс какие инструменты звали и
// что они вернули сводкой, и ручные правки строки (состав до/после). Это
// материал «раз в неделю прочитать переписку менеджеров и понять, где агент не
// понял» — и заодно регрессионный корпус для стенда: ход, кончившийся ручной
// правкой, сам себе тест.
//
// Хранение 180 дней, обрезка при записи (та же политика, что у chat_history
// agent-service). Ручка чтения — только admin.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// agentLogRetentionDays — сколько дней хранится журнал.
const agentLogRetentionDays = 180

// agentLogEntry — одна строка журнала хода.
type agentLogEntry struct {
	ChatID    string
	Email     string
	OfferID   int64
	SheetID   int64
	UserText  string
	Tools     []agentToolCall
	Answer    string
	ManualFix []agentManualFix
	Model     string
	LatencyMs int
	Reason    string
}

// agentEnvelope — транспортные сведения для записи в журнал (необязательно).
// Оставлено ради явного имени: журнал пишется и без него.
type agentEnvelope struct{}

// writeAgentLog пишет ход в offers_agent_log. Ошибка записи НЕ ломает ход:
// человеку важен ответ, а не строка журнала (сбой журнала — только лог).
func (s *Server) writeAgentLog(ctx context.Context, entry agentLogEntry, _ *agentEnvelope) {
	toolsJSON, err := json.Marshal(entry.Tools)
	if err != nil {
		toolsJSON = []byte("[]")
	}
	fixJSON := []byte("null")
	if len(entry.ManualFix) > 0 {
		if data, merr := json.Marshal(entry.ManualFix); merr == nil {
			fixJSON = data
		}
	}
	var offerID, sheetID any
	if entry.OfferID > 0 {
		offerID = entry.OfferID
	}
	if entry.SheetID > 0 {
		sheetID = entry.SheetID
	}
	// Контекст записи может быть уже отменён (ход отработал) — пишем отдельным
	// коротким контекстом, иначе последняя строка каждого медленного хода
	// терялась бы.
	writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.pool.Exec(writeCtx, `
INSERT INTO offers_agent_log
	(chat_id, email, offer_id, sheet_id, user_text, tools, answer_text, manual_fix, model, latency_ms, reason)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		entry.ChatID, entry.Email, offerID, sheetID,
		clipText(entry.UserText, 4000), toolsJSON, clipText(entry.Answer, 8000),
		fixJSON, entry.Model, entry.LatencyMs, entry.Reason,
	); err != nil {
		log.Printf("агент: журнал хода не записан: %v", err)
		return
	}
	s.trimAgentLog(writeCtx)
}

// trimAgentLog удаляет строки старше срока хранения.
func (s *Server) trimAgentLog(ctx context.Context) {
	if _, err := s.pool.Exec(ctx,
		fmt.Sprintf(`DELETE FROM offers_agent_log WHERE created_at < now() - interval '%d days'`, agentLogRetentionDays)); err != nil {
		log.Printf("агент: обрезка журнала не удалась: %v", err)
	}
}

// agentLogRow — строка для ручки чтения (то, что видит администратор).
type agentLogRow struct {
	ID         int64           `json:"id"`
	ChatID     string          `json:"chatId,omitempty"`
	Email      string          `json:"email"`
	OfferID    *int64          `json:"offerId,omitempty"`
	SheetID    *int64          `json:"sheetId,omitempty"`
	UserText   string          `json:"userText"`
	Tools      json.RawMessage `json:"tools"`
	AnswerText string          `json:"answerText"`
	ManualFix  json.RawMessage `json:"manualFix,omitempty"`
	Model      string          `json:"model,omitempty"`
	LatencyMs  int             `json:"latencyMs"`
	Reason     string          `json:"reason"`
	CreatedAt  string          `json:"createdAt"`
}

// handleAgentLog — GET /api/offers/admin/agent-log?from=&to=&email=&limit=
// (роль admin). `from`/`to` — даты `YYYY-MM-DD` (МСК не делаем: время сервера,
// для разбора переписки этого хватает).
func (s *Server) handleAgentLog(w http.ResponseWriter, r *http.Request) {
	query := `SELECT id, chat_id, email, offer_id, sheet_id, user_text, tools, answer_text,
		manual_fix, model, latency_ms, reason,
		to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
	FROM offers_agent_log WHERE 1=1`
	args := []any{}
	add := func(cond string, value any) {
		args = append(args, value)
		query += fmt.Sprintf(cond, len(args))
	}
	if from := strings.TrimSpace(r.URL.Query().Get("from")); from != "" {
		add(` AND created_at >= $%d`, from)
	}
	if to := strings.TrimSpace(r.URL.Query().Get("to")); to != "" {
		add(` AND created_at <= $%d`, to+" 23:59:59")
	}
	if email := strings.TrimSpace(r.URL.Query().Get("email")); email != "" {
		add(` AND lower(email) = lower($%d)`, email)
	}
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT %d`, limit)

	rows, err := s.pool.Query(r.Context(), query, args...)
	if err != nil {
		dbError(w, "agent log", err)
		return
	}
	defer rows.Close()
	out := make([]agentLogRow, 0, 32)
	for rows.Next() {
		var row agentLogRow
		var tools, fix []byte
		if err := rows.Scan(&row.ID, &row.ChatID, &row.Email, &row.OfferID, &row.SheetID, &row.UserText,
			&tools, &row.AnswerText, &fix, &row.Model, &row.LatencyMs, &row.Reason, &row.CreatedAt); err != nil {
			dbError(w, "agent log scan", err)
			return
		}
		row.Tools = rawOrEmptyArray(tools)
		if len(fix) > 0 {
			row.ManualFix = json.RawMessage(fix)
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": len(out)})
}

// rawOrEmptyArray — JSONB в массив; nil в JSON клиента не годится.
func rawOrEmptyArray(raw []byte) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("[]")
	}
	return json.RawMessage(raw)
}
