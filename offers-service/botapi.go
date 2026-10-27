package main

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// botOfferRequest — тело создания предложения из бота.
type botOfferRequest struct {
	ChatID     string            `json:"chat_id"`
	Client     OfferClient       `json:"client"`
	Items      []OfferItem       `json:"items"`
	FormValues map[string]string `json:"formValues"`
}

// emailForChat возвращает email владельца chat_id (или пусто, если не привязан).
func (s *Server) emailForChat(w http.ResponseWriter, r *http.Request, chatID string) (string, bool) {
	if strings.TrimSpace(chatID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "chat_id is required"})
		return "", false
	}
	var email string
	if err := s.pool.QueryRow(r.Context(),
		`SELECT email FROM offers_bot_link WHERE chat_id = $1`, chatID).Scan(&email); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "chat_id не привязан"})
		return "", false
	}
	return email, true
}

func (s *Server) handleBotCatalog(w http.ResponseWriter, r *http.Request) {
	items, err := s.listCatalogItems(r.Context(), false)
	if err != nil {
		dbError(w, "bot catalog", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": filterCatalogItems(items, r.URL.Query().Get("q"))})
}

// handleBotListOffers — «Мои КП» в боте: только предложения привязанного email.
func (s *Server) handleBotListOffers(w http.ResponseWriter, r *http.Request) {
	email, ok := s.emailForChat(w, r, r.URL.Query().Get("chat_id"))
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(),
		`SELECT id FROM offers_offer WHERE lower(owner_email) = lower($1) ORDER BY created_at DESC LIMIT 50`, email)
	if err != nil {
		dbError(w, "bot list offers", err)
		return
	}
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			dbError(w, "запрос", err)
			return
		}
		ids = append(ids, id)
	}
	rows.Close()

	offers := make([]Offer, 0, len(ids))
	for _, id := range ids {
		offer, err := s.loadOffer(r.Context(), id)
		if err != nil {
			continue
		}
		offers = append(offers, offer)
	}
	writeJSON(w, http.StatusOK, map[string]any{"offers": offers})
}

func (s *Server) handleBotCreateOffer(w http.ResponseWriter, r *http.Request) {
	var req botOfferRequest
	if err := decodeJSON(w, r, &req, 4<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	email, ok := s.emailForChat(w, r, req.ChatID)
	if !ok {
		return
	}
	offer := Offer{
		Client:     req.Client,
		Items:      req.Items,
		FormValues: req.FormValues,
		Status:     "draft",
	}
	if msg := validateOffer(offer); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
		return
	}
	offer.Total = recalcOfferTotal(offer.Items)
	if offer.FormValues == nil {
		offer.FormValues = map[string]string{}
	}

	clientJSON, _ := json.Marshal(offer.Client)
	itemsJSON, _ := json.Marshal(offer.Items)
	valuesJSON, _ := json.Marshal(offer.FormValues)

	number, err := s.nextNumber(r.Context(), time.Now().Year())
	if err != nil {
		dbError(w, "bot create number", err)
		return
	}
	var id int64
	err = s.pool.QueryRow(r.Context(), `
INSERT INTO offers_offer (number, owner_email, client, items, form_values, total, status)
VALUES ($1, $2, $3, $4, $5, $6, 'draft') RETURNING id`,
		number, email, clientJSON, itemsJSON, valuesJSON, offer.Total).Scan(&id)
	if err != nil {
		dbError(w, "bot create offer", err)
		return
	}
	saved, err := s.loadOffer(r.Context(), id)
	if err != nil {
		dbError(w, "запрос", err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleBotPatchOffer(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	var req botOfferRequest
	if err := decodeJSON(w, r, &req, 4<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	email, ok := s.emailForChat(w, r, req.ChatID)
	if !ok {
		return
	}
	offer, err := s.loadOffer(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "предложение не найдено"})
		return
	}
	if !strings.EqualFold(offer.OwnerEmail, email) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "это не ваше предложение"})
		return
	}
	if len(req.Items) > 0 {
		offer.Items = req.Items
	}
	if req.Client.Name != "" || req.Client.Contact != "" {
		offer.Client = req.Client
	}
	if req.FormValues != nil {
		offer.FormValues = req.FormValues
	}
	if msg := validateOffer(offer); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
		return
	}
	offer.Total = recalcOfferTotal(offer.Items)

	clientJSON, _ := json.Marshal(offer.Client)
	itemsJSON, _ := json.Marshal(offer.Items)
	valuesJSON, _ := json.Marshal(offer.FormValues)
	_, err = s.pool.Exec(r.Context(), `
UPDATE offers_offer SET client = $2, items = $3, form_values = $4, total = $5, updated_at = now()
WHERE id = $1`, id, clientJSON, itemsJSON, valuesJSON, offer.Total)
	if err != nil {
		dbError(w, "bot patch offer", err)
		return
	}
	saved, _ := s.loadOffer(r.Context(), id)
	writeJSON(w, http.StatusOK, saved)
}

// docxFileName — имя файла для чата: пробелы и кириллица сохраняются (это
// имя видит человек в Telegram), запрещённые в файловых системах символы — нет.
func docxFileName(offer Offer) string {
	name := "Коммерческое предложение " + strings.ReplaceAll(offer.Number, "/", "-")
	if client := strings.TrimSpace(offer.Client.Name); client != "" {
		replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "\n", " ", "\r", " ")
		name += " " + strings.Join(strings.Fields(replacer.Replace(client)), " ")
	}
	return name + ".docx"
}

// buildOfferDocx собирает документ и кладёт его в S3, возвращая ключ.
// renderOffer — только рендер, без внешних сервисов. Экспорт байтами не должен
// зависеть от S3: раньше он ждал rclone до пяти минут, если хранилище лежало.
func (s *Server) renderOffer(r *http.Request, offer Offer) ([]byte, error) {
	form, err := s.loadForm(r.Context(), "client")
	if err != nil {
		return nil, err
	}
	return renderOfferDocx(form, offer)
}

func (s *Server) buildOfferDocx(r *http.Request, offer Offer) ([]byte, string, error) {
	data, err := s.renderOffer(r, offer)
	if err != nil {
		return nil, "", err
	}
	key := "offers/" + offer.Number + "/" + randomID() + ".docx"
	if s.s3 != nil {
		if err := s.s3.Put(r.Context(), key, data); err != nil {
			log.Printf("offer docx upload: %v", err)
			// Файл всё равно отдаём — боту важно выдать документ.
			return data, "", nil
		}
		_, _ = s.pool.Exec(r.Context(),
			`UPDATE offers_offer SET docx_key = $2, updated_at = now() WHERE id = $1`, offer.ID, key)
	}
	return data, key, nil
}

func (s *Server) handleExportDocx(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	offer, err := s.loadOffer(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "предложение не найдено"})
		return
	}
	role, _ := s.effectiveRole(r.Context(), appIDFromEnv(), actorEmailFromCtx(r))
	if !canSeeOffer(role, actorEmailFromCtx(r), offer) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "нет доступа к этому предложению"})
		return
	}
	data, err := s.renderOffer(r, offer)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+docxFileName(offer))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleBotDocx(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	// chat_id обязателен: без него нельзя проверить, чьё это предложение, и
	// ручка превратилась бы в выдачу любого DOCX любому держателю секрета.
	email, ok := s.emailForChat(w, r, r.URL.Query().Get("chat_id"))
	if !ok {
		return
	}
	offer, err := s.loadOffer(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "предложение не найдено"})
		return
	}
	if !strings.EqualFold(offer.OwnerEmail, email) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "это не ваше предложение"})
		return
	}
	data, _, err := s.buildOfferDocx(r, offer)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"docx_base64": base64.StdEncoding.EncodeToString(data),
		"file_name":   docxFileName(offer),
	})
}

// offerLink для пользователя: DOCX отдаётся presigned-ссылкой на 48 часов.
func (s *Server) offerLink(w http.ResponseWriter, r *http.Request, offer Offer) {
	if s.s3 == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "хранилище не настроено"})
		return
	}
	data, key, err := s.buildOfferDocx(r, offer)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if key == "" {
		tmpKey := "offers/" + offer.Number + "/" + randomID() + ".docx"
		if err := s.s3.Put(r.Context(), tmpKey, data); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": "не удалось сохранить файл"})
			return
		}
		key = tmpKey
	}
	url, err := s.s3.Link(r.Context(), key)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "не удалось получить ссылку"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"url":       url,
		"expiresAt": time.Now().Add(48 * time.Hour).Format(time.RFC3339),
	})
}

func (s *Server) handleOfferLink(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	offer, err := s.loadOffer(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "предложение не найдено"})
		return
	}
	role, _ := s.effectiveRole(r.Context(), appIDFromEnv(), actorEmailFromCtx(r))
	if !canSeeOffer(role, actorEmailFromCtx(r), offer) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "нет доступа к этому предложению"})
		return
	}
	s.offerLink(w, r, offer)
}

func (s *Server) handleBotOfferLink(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	// chat_id обязателен — см. handleBotDocx.
	email, ok := s.emailForChat(w, r, r.URL.Query().Get("chat_id"))
	if !ok {
		return
	}
	offer, err := s.loadOffer(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "предложение не найдено"})
		return
	}
	if !strings.EqualFold(offer.OwnerEmail, email) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "это не ваше предложение"})
		return
	}
	s.offerLink(w, r, offer)
}
