package main

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"mailers/offers/model"
)

// Хранение брифа КП (2026-10-29). Типы — в пакете model (его видят и скилы,
// и чистые модули); здесь только SQL и границы.

// Brief — строка offers_brief: снимок состояния разговора по предложению.
type Brief struct {
	ID        int64              `json:"id"`
	OfferID   int64              `json:"offerId"`
	Payload   model.BriefPayload `json:"payload"`
	Status    string             `json:"status"` // open | ready | agreed
	ChatID    string             `json:"chatId"`
	LastTopic string             `json:"lastTopic"` // тема, которую спрашивали последней
	UpdatedAt string             `json:"updatedAt"`
}

// BriefTurn — одна реплика разговора. Журнал нужен, чтобы после перезапуска
// бота (или по жалобе «мне не то ответили») увидеть, что именно было сказано.
type BriefTurn struct {
	ID        int64  `json:"id"`
	BriefID   int64  `json:"briefId"`
	Role      string `json:"role"` // bot | manager
	Text      string `json:"text"`
	Topic     string `json:"topic"`
	CreatedAt string `json:"createdAt"`
}

// loadBrief читает бриф предложения. Нет строки — возвращает pgx.ErrNoRows:
// вызывающий решает, создавать ли (в отличие от формы КП, где «нет строки» —
// штатное состояние «не настроено», здесь это «брифа ещё нет»).
func (s *Server) loadBrief(ctx context.Context, offerID int64) (Brief, error) {
	var b Brief
	var payload []byte
	err := s.pool.QueryRow(ctx, `
SELECT id, offer_id, payload, status, chat_id, last_topic,
	to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.US' || 'Z')
FROM offers_brief WHERE offer_id = $1`, offerID).
		Scan(&b.ID, &b.OfferID, &payload, &b.Status, &b.ChatID, &b.LastTopic, &b.UpdatedAt)
	if err != nil {
		return b, err
	}
	b.Payload = model.EmptyBriefPayload()
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &b.Payload); err != nil {
			return b, err
		}
	}
	b.Payload.Normalize()
	return b, nil
}

// createBrief создаёт бриф предложения. Идемпотентно: повторный вызов
// возвращает уже существующий — «Начать КП» можно нажать дважды, и это не
// должно обнулять набранное.
func (s *Server) createBrief(ctx context.Context, offerID int64, chatID string) (Brief, error) {
	existing, err := s.loadBrief(ctx, offerID)
	if err == nil {
		return existing, nil
	}
	if err != pgx.ErrNoRows {
		return Brief{}, err
	}
	raw, err := json.Marshal(model.EmptyBriefPayload())
	if err != nil {
		return Brief{}, err
	}
	var id int64
	if err := s.pool.QueryRow(ctx, `
INSERT INTO offers_brief (offer_id, payload, chat_id) VALUES ($1, $2, $3)
ON CONFLICT (offer_id) DO UPDATE SET updated_at = now()
RETURNING id`, offerID, raw, chatID).Scan(&id); err != nil {
		return Brief{}, err
	}
	return s.loadBrief(ctx, offerID)
}

// saveBriefPayload записывает бриф и двигает updated_at. Правка данных обязана
// двигать метку времени (правило проекта): иначе правка не выиграет слияние.
func (s *Server) saveBriefPayload(ctx context.Context, b Brief) error {
	raw, err := json.Marshal(b.Payload)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
UPDATE offers_brief SET payload = $2, status = $3, last_topic = $4,
	updated_at = now()
WHERE id = $1`, b.ID, raw, b.Status, b.LastTopic)
	return err
}

// appendTurn пишет реплику разговора.
func (s *Server) appendTurn(ctx context.Context, briefID int64, role, text, topic string) error {
	_, err := s.pool.Exec(ctx, `
INSERT INTO offers_brief_turn (brief_id, role, text, topic) VALUES ($1, $2, $3, $4)`,
		briefID, role, text, topic)
	return err
}

// listTurns читает журнал разговора, старые сверху.
func (s *Server) listTurns(ctx context.Context, briefID int64, limit int) ([]BriefTurn, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, brief_id, role, text, topic,
	to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS.US' || 'Z')
FROM offers_brief_turn WHERE brief_id = $1 ORDER BY created_at ASC, id ASC LIMIT $2`,
		briefID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]BriefTurn, 0)
	for rows.Next() {
		var t BriefTurn
		if err := rows.Scan(&t.ID, &t.BriefID, &t.Role, &t.Text, &t.Topic, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
