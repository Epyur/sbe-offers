package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mailers/offers/model"
	"mailers/offers/skills"
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

// handleBotCatalog — «Добавить из каталога» в боте: ТОЛЬКО материалы.
//
// Система в боте позицией не является (2026-10-31): у неё нет цены, а состав
// берётся из Базы знаний. Раньше список приходил из offers_catalog вместе с
// базовой ценой, и менеджер в чате видел систему как строку с ценой.
//
// Форма ответа прежняя (id, name, unit, basePrice): контракт с ботом не ломаем,
// в basePrice кладём нижнюю границу вилки, а `hasPrice` говорит, была ли она
// вообще — без этого ноль выглядел бы как «бесплатно».
func (s *Server) handleBotCatalog(w http.ResponseWriter, r *http.Request) {
	materials, err := s.listMaterials(r.Context(), false)
	if err != nil {
		dbError(w, "bot catalog", err)
		return
	}
	query := r.URL.Query().Get("q")
	kept := filterMaterialsForBot(materials, query)
	items := make([]map[string]any, 0, len(kept))
	for _, m := range kept {
		price := 0.0
		hasPrice := m.PriceMin != nil
		if hasPrice {
			price = *m.PriceMin
		}
		items = append(items, map[string]any{
			"id":        m.ID,
			"kind":      "material",
			"name":      m.Name,
			"unit":      m.Unit,
			"sku":       m.SKU,
			"basePrice": price,
			"hasPrice":  hasPrice,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleBotSystems — системы для команды «система <название>» в боте.
//
// Отдаёт только то, что нужно чату: код, название и проблемы состава. Полного
// текста карточки и картинок боту не требуется, а лишние поля тянули бы
// байты в каждом ответе.
func (s *Server) handleBotSystems(w http.ResponseWriter, r *http.Request) {
	materials, err := s.listMaterials(r.Context(), true)
	if err != nil {
		dbError(w, "bot systems: materials", err)
		return
	}
	if s.kb == nil || !s.kb.configured() {
		writeJSON(w, http.StatusOK, map[string]any{
			"systems":   []any{},
			"kbEnabled": false,
			"error":     "База знаний не настроена",
		})
		return
	}
	cards, err := SystemsFromKB(r.Context(), s.kb, materials)
	resp := map[string]any{"kbEnabled": true, "systems": []any{}}
	if err != nil {
		resp["error"] = err.Error()
		writeJSON(w, http.StatusOK, resp)
		return
	}
	items := make([]map[string]any, 0, len(cards))
	for _, c := range cards {
		items = append(items, map[string]any{
			"sku":      c.SKU,
			"title":    c.Title,
			"problems": c.Problems,
		})
	}
	resp["systems"] = items
	writeJSON(w, http.StatusOK, resp)
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

// --- Бриф в боте (Task 5, 2026-10-29) ---------------------------------

// botBriefRequest — тело шага брифа.
type botBriefRequest struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

// botOfferForChat — общая проверка «это КП этого чата». Повторяет то же
// правило, что DOCX и ссылка: без chat_id нельзя понять, чьё это, и ручка
// стала бы выдачей любого предложения держателю секрета.
func (s *Server) botOfferForChat(w http.ResponseWriter, r *http.Request, id int64) (Offer, string, bool) {
	email, ok := s.emailForChat(w, r, r.URL.Query().Get("chat_id"))
	if !ok {
		return Offer{}, "", false
	}
	offer, err := s.loadOffer(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "предложение не найдено"})
		return Offer{}, "", false
	}
	if !strings.EqualFold(offer.OwnerEmail, email) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "это не ваше предложение"})
		return Offer{}, "", false
	}
	return offer, email, true
}

func (s *Server) handleBotBriefStart(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	_, email, ok := s.botOfferForChat(w, r, id)
	if !ok {
		return
	}
	brief, err := s.createBrief(r.Context(), id, r.URL.Query().Get("chat_id"))
	if err != nil {
		dbError(w, "brief start", err)
		return
	}
	topic, open := nextOpenTopic(brief.Payload)
	reply := "Расскажите про объект: для какого объекта делаем предложение?"
	topicKey := ""
	if open {
		reply = topic.Question
		topicKey = topic.Key
	}
	_ = s.appendTurn(r.Context(), brief.ID, "bot", reply, topicKey)
	writeJSON(w, http.StatusOK, map[string]any{
		"brief": brief,
		"reply": reply,
		"done":  !open,
		"owner": email,
	})
}

// handleBotBriefGet отдаёт бриф предложения.
//
// Нужен боту для сводки: сессия хранит только «на какой теме стоим», а сами
// ответы лежат в брифе. Второй копии в боте нет.
func (s *Server) handleBotBriefGet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	if _, _, ok := s.botOfferForChat(w, r, id); !ok {
		return
	}
	brief, err := s.loadBrief(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "брифа ещё нет"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"brief": brief})
}

func (s *Server) handleBotBriefStep(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	var req botBriefRequest
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	_, _, ok := s.botOfferForChatQ(w, r, id, req.ChatID)
	if !ok {
		return
	}
	brief, err := s.createBrief(r.Context(), id, req.ChatID)
	if err != nil {
		dbError(w, "brief step", err)
		return
	}
	_ = s.appendTurn(r.Context(), brief.ID, "manager", req.Text, brief.LastTopic)

	result, err := StepBrief(r.Context(), brief.Payload, req.Text, llmCompleter{})
	if err != nil {
		dbError(w, "brief step", err)
		return
	}
	// В бриф пишем только при удачном шаге: ошибка модели не должна ни
	// стирать набранное, ни сдвигать тему.
	if !result.Retry && !result.Unavailable {
		brief.Payload = result.Payload
		if result.Done {
			brief.Status = "ready"
		}
		brief.LastTopic = result.NextTopic
		if err := s.saveBriefPayload(r.Context(), brief); err != nil {
			dbError(w, "brief save", err)
			return
		}
	}
	_ = s.appendTurn(r.Context(), brief.ID, "bot", result.Reply, result.NextTopic)
	writeJSON(w, http.StatusOK, map[string]any{
		"brief":       brief,
		"reply":       result.Reply,
		"done":        result.Done,
		"nextTopic":   result.NextTopic,
		"retry":       result.Retry,
		"unavailable": result.Unavailable,
		"skipped":     result.Skipped,
	})
}

// --- Текст секций в боте (Task 6, 2026-10-29) -------------------------

// botCopyRequest — тело запроса секции.
type botCopyRequest struct {
	ChatID      string `json:"chat_id"`
	Action      string `json:"action"`      // write | edit | accept
	Text        string `json:"text"`        // правка менеджера или принятый текст
	SystemIndex int    `json:"systemIndex"` // для секций систем, с 1
}

// sectionKindFromPath — kind из пути; неизвестный отдаёт пусто.
func sectionKindFromPath(r *http.Request) skills.SectionKind {
	kind := skills.SectionKind(r.PathValue("key"))
	for _, k := range skills.AllSections() {
		if k == kind {
			return kind
		}
	}
	return ""
}

// handleBotCopySection — написать, поправить или принять секцию.
//
// Ручка одна на три действия: у них общий путь проверки «это ваше КП» и общий
// формат ответа. Разводить три ручки значило бы трижды повторить контроль
// доступа.
func (s *Server) handleBotCopySection(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	kind := sectionKindFromPath(r)
	if kind == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "неизвестная секция"})
		return
	}
	var req botCopyRequest
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	offer, _, ok := s.botOfferForChatQ(w, r, id, req.ChatID)
	if !ok {
		return
	}

	switch req.Action {
	case "write":
		brief := model.EmptyBriefPayload()
		if b, err := s.loadBrief(r.Context(), id); err == nil {
			brief = b.Payload
		}
		pc, _ := s.loadContext(r.Context(), "client")
		res, err := WriteSectionChecked(r.Context(), kind, brief, offer, pc, llmCompleter{})
		if err != nil {
			dbError(w, "copy write", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"key":      SectionKey(kind, req.SystemIndex),
			"text":     res.Text,
			"warnings": res.Warnings,
			"label":    sectionLabel(kind, req.SystemIndex),
		})
	case "edit":
		pc, _ := s.loadContext(r.Context(), "client")
		text, err := EditSection(r.Context(), kind, req.Text, pc, llmCompleter{})
		if err != nil {
			dbError(w, "copy edit", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"key":  SectionKey(kind, req.SystemIndex),
			"text": text,
		})
	case "accept":
		offer = AcceptSection(offer, kind, req.SystemIndex, req.Text)
		if err := s.persistOfferFormValues(r.Context(), offer); err != nil {
			dbError(w, "copy accept", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"key":      SectionKey(kind, req.SystemIndex),
			"accepted": strings.TrimSpace(req.Text) != "",
		})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "action: write|edit|accept"})
	}
}

// sectionLabel — как секция называется в чате.
func sectionLabel(kind skills.SectionKind, systemIndex int) string {
	switch kind {
	case skills.SectionIntro:
		return "Вводный текст"
	case skills.SectionSystem:
		return fmt.Sprintf("Описание системы %d", systemIndex)
	case skills.SectionAdvantages:
		return "Преимущества"
	case skills.SectionConditions:
		return "Условия"
	}
	return string(kind)
}

// botOfferForChatQ — как botOfferForChat, но chat_id берётся из тела запроса,
// а не из строки запроса: секции приходят POST-ом с JSON.
func (s *Server) botOfferForChatQ(w http.ResponseWriter, r *http.Request, id int64, chatID string) (Offer, string, bool) {
	email, ok := s.emailForChat(w, r, chatID)
	if !ok {
		return Offer{}, "", false
	}
	offer, err := s.loadOffer(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "предложение не найдено"})
		return Offer{}, "", false
	}
	if !strings.EqualFold(offer.OwnerEmail, email) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "это не ваше предложение"})
		return Offer{}, "", false
	}
	return offer, email, true
}

// handleBotAddSystem — «система <название>» в боте: состав дописывается в КП.
//
// Повторяет ручку плагина намеренно: у бота другой способ входа (секрет
// сервиса и chat_id), и сводить их в одну значило бы либо отдать боту
// пользовательский JWT, либо пустить плагин по сервисному секрету. Логика
// состава при этом ОДНА — appendComposition и SystemsFromKB.
func (s *Server) handleBotAddSystem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		ChatID string `json:"chat_id"`
		SKU    string `json:"sku"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	offer, _, ok := s.botOfferForChatQ(w, r, id, req.ChatID)
	if !ok {
		return
	}

	materials, err := s.listMaterials(r.Context(), false)
	if err != nil {
		dbError(w, "bot add system: materials", err)
		return
	}
	systems, err := SystemsFromKB(r.Context(), s.kb, materials)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": "База знаний недоступна: " + err.Error(),
		})
		return
	}
	card, found := FindSystem(systems, req.SKU)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "система не найдена"})
		return
	}

	byID := make(map[int64]Material, len(materials))
	for _, m := range materials {
		byID[m.ID] = m
	}
	resp := ComposeSystemResponse{Problems: []string{}}
	offer.Items, resp.Added = appendComposition(offer.Items, card.CompositionLinks, byID, &resp)
	if resp.Added == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "в составе системы нет ни одного материала из справочника",
		})
		return
	}
	if err := s.persistOfferItems(r.Context(), offer); err != nil {
		dbError(w, "bot add system: save offer", err)
		return
	}
	resp.Offer = offer
	writeJSON(w, http.StatusOK, resp)
}

// persistOfferFormValues пишет только form_values и двигает updated_at.
//
// Отдельно от handleSaveOffer: там правится всё предложение, а здесь — одна
// секция. Гонять полный validateOffer на каждое «принять секцию» значило бы
// отказать в приёме текста, если в составе что-то не так по мнению валидатора,
// хотя состав менеджер не трогал.
func (s *Server) persistOfferFormValues(ctx context.Context, offer Offer) error {
	values, err := json.Marshal(offer.FormValues)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
UPDATE offers_offer SET form_values = $2, updated_at = now() WHERE id = $1`,
		offer.ID, values)
	return err
}

// persistOfferItems пишет состав и итог предложения, двигая updated_at.
//
// Отдельно от handleSaveOffer: таблица менеджера меняет СОСТАВ и цены, а не
// клиента и не текст формы. Гонять полную валидацию здесь значило бы отказать
// в применении таблицы из-за поля, которого таблица не касается.
func (s *Server) persistOfferItems(ctx context.Context, offer Offer) error {
	items, err := json.Marshal(offer.Items)
	if err != nil {
		return err
	}
	offer.Total = recalcOfferTotal(offer.Items)
	_, err = s.pool.Exec(ctx, `
UPDATE offers_offer SET items = $2, total = $3, updated_at = now() WHERE id = $1`,
		offer.ID, items, offer.Total)
	return err
}
