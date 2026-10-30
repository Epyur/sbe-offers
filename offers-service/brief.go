package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

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

// --- Бриф в плагине (Task 10, 2026-10-30) -------------------------------

// briefResponse — бриф вместе с журналом реплик: плагин показывает историю
// разговора, а не только поля. Журнал отдаём только при чтении одного брифа.
type briefResponse struct {
	Brief Brief       `json:"brief"`
	Turns []BriefTurn `json:"turns"`
}

// handleGetBrief читает бриф предложения. Брифа нет — пустая заготовка со
// статусом not_found: для плагина «брифа ещё нет» это обычное состояние, а не
// ошибка (КП могло родиться из плагина, без разговора в боте).
func (s *Server) handleGetBrief(w http.ResponseWriter, r *http.Request) {
	offer, ok := s.offerForReader(w, r)
	if !ok {
		return
	}
	brief, err := s.loadBrief(r.Context(), offer.ID)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusOK, briefResponse{
			Brief: Brief{
				OfferID: offer.ID,
				Payload: model.EmptyBriefPayload(),
				Status:  "absent",
			},
			Turns: []BriefTurn{},
		})
		return
	}
	if err != nil {
		dbError(w, "load brief", err)
		return
	}
	turns, err := s.listTurns(r.Context(), brief.ID, 200)
	if err != nil {
		dbError(w, "list brief turns", err)
		return
	}
	writeJSON(w, http.StatusOK, briefResponse{Brief: brief, Turns: turns})
}

// handleSaveBrief правит ответы брифа руками (owner или admin).
//
// Правка идёт тем же путём, что и шаг диалога: payload.replace целиком, а
// updated_at двигается обязательно — иначе правка не выиграет слияние у того,
// кто откроет КП позже.
func (s *Server) handleSaveBrief(w http.ResponseWriter, r *http.Request) {
	offer, ok := s.offerForEditor(w, r)
	if !ok {
		return
	}
	var req struct {
		Payload model.BriefPayload `json:"payload"`
		Status  string             `json:"status"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	req.Payload.Normalize()

	brief, err := s.createBrief(r.Context(), offer.ID, "")
	if err != nil {
		dbError(w, "save brief", err)
		return
	}
	brief.Payload = req.Payload
	if req.Status == "ready" || req.Status == "agreed" || req.Status == "open" {
		brief.Status = req.Status
	}
	if err := s.saveBriefPayload(r.Context(), brief); err != nil {
		dbError(w, "save brief payload", err)
		return
	}
	writeJSON(w, http.StatusOK, briefResponse{Brief: brief, Turns: []BriefTurn{}})
}

// offerForReader — предложение по id с правом чтения (viewer+ и видимость).
func (s *Server) offerForReader(w http.ResponseWriter, r *http.Request) (Offer, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return Offer{}, false
	}
	offer, err := s.loadOffer(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "предложение не найдено"})
		return Offer{}, false
	}
	role, _ := s.effectiveRole(r.Context(), appIDFromEnv(), actorEmailFromCtx(r))
	if !canSeeOffer(role, actorEmailFromCtx(r), offer) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "нет доступа к этому предложению"})
		return Offer{}, false
	}
	return offer, true
}

// offerForEditor — то же плюс право правки: владелец предложения или admin.
//
// Роль editor сама по себе не даёт правки чужого КП: бриф — это разговор
// конкретного менеджера с заказчиком, и править его может только он или
// администратор.
func (s *Server) offerForEditor(w http.ResponseWriter, r *http.Request) (Offer, bool) {
	offer, ok := s.offerForReader(w, r)
	if !ok {
		return Offer{}, false
	}
	role, _ := s.effectiveRole(r.Context(), appIDFromEnv(), actorEmailFromCtx(r))
	if !isAdminRole(role) && !strings.EqualFold(offer.OwnerEmail, actorEmailFromCtx(r)) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "бриф правит владелец предложения или администратор"})
		return Offer{}, false
	}
	return offer, true
}
