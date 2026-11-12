package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mailers/offers/model"
)

// OfferItem, OfferClient и Offer живут в пакете model: их видят и скилы, и
// чистые модули, а main импортировать нельзя.
type Offer = model.Offer
type OfferItem = model.OfferItem
type OfferClient = model.OfferClient
type OfferSystem = model.OfferSystem
type OfferSystems = model.OfferSystems
type OfferMaterial = model.Ref
type OfferMaterials = model.OfferMaterials

// Ref — выбор карточки (система или материал): код и/или номер заметки.
type Ref = model.Ref

// RefList — общий список выбора (Add/Remove, терпимость к старому виду).
type RefList = model.RefList

// roundMoney округляет до копеек без двоичных хвостов (0.1+0.2). Множитель
// 1000 снимает артефакт представления: 1.005 * 100 = 100.49999999999999,
// и math.Round по нему дал бы 1.00 вместо 1.01.
func roundMoney(v float64) float64 {
	scaled := math.Round(v*1000) / 10
	return math.Round(scaled) / 100
}

// computeTotal суммирует строки с округлением каждой строки до копеек, затем
// итог ещё раз — иначе 3 × 999.99 даёт 2999.9699999999998 в документе.
func computeTotal(items []OfferItem) float64 {
	total := 0.0
	for _, item := range items {
		total += roundMoney(item.Qty * item.Price)
	}
	return roundMoney(total)
}

// recalcOfferTotal пересчитывает суммы строк В СРЕЗЕ (по ссылке) и возвращает
// итог. Используется и сервером, и обработчиком бота.
func recalcOfferTotal(items []OfferItem) float64 {
	for i := range items {
		items[i].Sum = roundMoney(items[i].Qty * items[i].Price)
	}
	return computeTotal(items)
}

// formatNumber — номер предложения вида «007/2026» (сквозной по году).
func formatNumber(n, year int) string {
	return fmt.Sprintf("%03d/%d", n, year)
}

// validateOffer проверяет предложение перед сохранением.
func validateOffer(offer Offer) string {
	if len(offer.Items) == 0 {
		return "в предложении нет позиций"
	}
	for _, item := range offer.Items {
		if strings.TrimSpace(item.Name) == "" {
			return "у позиции не заполнено наименование"
		}
		if item.Qty <= 0 {
			return "количество должно быть больше нуля"
		}
		if item.Price < 0 {
			return "цена не может быть отрицательной"
		}
	}
	return ""
}

// nextNumber выдаёт следующий номер по году. Выполняется в транзакции с
// advisory-lock, чтобы параллельные запросы не получили один и тот же номер.
func (s *Server) nextNumber(ctx context.Context, year int) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	suffix := fmt.Sprintf("/%d", year)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(year)); err != nil {
		return "", err
	}
	var maxNum int
	err = tx.QueryRow(ctx, `
SELECT COALESCE(MAX(NULLIF(regexp_replace(split_part(number, '/', 1), '\D', '', 'g'), '')::int), 0)
FROM offers_offer WHERE number LIKE '%' || $1`, suffix).Scan(&maxNum)
	if err != nil {
		return "", err
	}
	number := formatNumber(maxNum+1, year)
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return number, nil
}

func (s *Server) loadOffer(ctx context.Context, id int64) (Offer, error) {
	var offer Offer
	var client, items, values, system, material []byte
	err := s.pool.QueryRow(ctx, `
SELECT id, number, owner_email, client, items, form_values, system_choice, material_choice, total, status, docx_key,
	to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
	to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
FROM offers_offer WHERE id = $1`, id).
		Scan(&offer.ID, &offer.Number, &offer.OwnerEmail, &client, &items, &values, &system, &material,
			&offer.Total, &offer.Status, &offer.DocxKey, &offer.CreatedAt, &offer.UpdatedAt)
	if err != nil {
		return offer, err
	}
	offer.Client = OfferClient{}
	offer.Items = []OfferItem{}
	offer.FormValues = map[string]string{}
	offer.Materials = OfferMaterials{}
	if len(client) > 0 {
		_ = json.Unmarshal(client, &offer.Client)
	}
	if len(items) > 0 {
		_ = json.Unmarshal(items, &offer.Items)
	}
	if len(values) > 0 {
		_ = json.Unmarshal(values, &offer.FormValues)
	}
	if len(system) > 0 {
		_ = json.Unmarshal(system, &offer.Systems)
	}
	if len(material) > 0 {
		_ = json.Unmarshal(material, &offer.Materials)
	}
	return offer, nil
}

// canSeeOffer — правила видимости: admin видит все, остальные — только свои.
func canSeeOffer(role, email string, offer Offer) bool {
	if isAdminRole(role) {
		return true
	}
	return strings.EqualFold(offer.OwnerEmail, email)
}

func (s *Server) handleListOffers(w http.ResponseWriter, r *http.Request) {
	role, _ := s.effectiveRole(r.Context(), appIDFromEnv(), actorEmailFromCtx(r))
	q := parseListQuery(r.URL.Query().Get("q"))

	// Поиск и видимость (2026-11-13, шаг S2; исправлено после стенда).
	//
	// Без поиска — прежний запрос: свежие сверху, видимость в SQL.
	//
	// С поиском — другой порядок, и это важно: СНАЧАЛА просматривается больше
	// строк (listScanLimit), ПОТОМ отсеивается совпадение и права, и только
	// затем берутся первые listLimit. Прежний вариант резал 200 строк ДО
	// поиска, и на живой базе «Ромаш» находилось 19 раз, а в ответе было ноль —
	// все совпадения оказались глубже свежих двухсот. Список без поиска этого
	// не видит, поэтому на стенде дефект выглядел как ошибка теста.
	//
	// Формы поиска: быстрый индексный срез («начинается с», «слово начинается
	// с») плюс встроенная `ILIKE '%q%'` — без неё имя заказчика в середине
	// слова не находится вовсе. На объёме до listScanLimit строк стоимость
	// предсказуема; когда база вырастет, следующий шаг — pg_trgm, а не
	// возврат к прежнему порядку.
	query := `SELECT id, lower(owner_email), client->>'name' FROM offers_offer`
	args := []any{}
	if !q.Empty() {
		query += ` WHERE (lower(client->>'name') LIKE $1` +
			` OR lower(client->>'name') LIKE $2` +
			` OR lower(replace(client->>'name', 'ё', 'е')) LIKE $3)`
		args = append(args, q.PrefixPattern(), q.WordPattern(), q.ContainsPattern())
	}
	// `WHERE` добавляется только поиском: без него условие видимости пришлось бы
	// начинать с `AND`, а это синтаксическая ошибка SQL (живой дефект 2026-11-13 —
	// «db error» у всех, у кого нет роли admin: список КП без поиска).
	if !isAdminRole(role) {
		if q.Empty() {
			query += ` WHERE lower(owner_email) = lower($` + itoa(len(args)+1) + `)`
		} else {
			query += ` AND lower(owner_email) = lower($` + itoa(len(args)+1) + `)`
		}
		args = append(args, actorEmailFromCtx(r))
	}
	scan := listLimit
	if !q.Empty() {
		scan = listScanLimit
	}
	rows, err := s.pool.Query(r.Context(),
		query+` ORDER BY created_at DESC LIMIT `+itoa(scan), args...)
	if err != nil {
		dbError(w, "list offers", err)
		return
	}
	defer rows.Close()
	offers := make([]Offer, 0, 32)
	for rows.Next() {
		var id int64
		var owner string
		var clientName *string
		if err := rows.Scan(&id, &owner, &clientName); err != nil {
			dbError(w, "list offers scan", err)
			return
		}
		// То же правило видимости, что в canSeeOffer, но по строке списка:
		// полную модель ради проверки владельца не загружаем.
		if !isAdminRole(role) && !strings.EqualFold(owner, actorEmailFromCtx(r)) {
			continue
		}
		if !q.Empty() {
			name := ""
			if clientName != nil {
				name = *clientName
			}
			if !containsTie(name, q.Contains()) {
				continue
			}
		}
		offer, err := s.loadOffer(r.Context(), id)
		if err != nil {
			continue
		}
		offers = append(offers, offer)
		if len(offers) >= listLimit {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"offers": offers})
}

func (s *Server) handleGetOffer(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, offer)
}

func (s *Server) handleSaveOffer(w http.ResponseWriter, r *http.Request) {
	var offer Offer
	if err := decodeJSON(w, r, &offer, 4<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if msg := validateOffer(offer); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
		return
	}
	offer.Total = recalcOfferTotal(offer.Items)
	if offer.Status != "final" {
		offer.Status = "draft"
	}
	if offer.FormValues == nil {
		offer.FormValues = map[string]string{}
	}

	clientJSON, err := json.Marshal(offer.Client)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}
	itemsJSON, err := json.Marshal(offer.Items)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}
	valuesJSON, err := json.Marshal(offer.FormValues)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}
	systemJSON, err := json.Marshal(offer.Systems)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}
	materialJSON, err := json.Marshal(offer.Materials)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}

	var id int64
	if raw := r.PathValue("id"); raw != "" {
		id, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
			return
		}
		var owner string
		if err := s.pool.QueryRow(r.Context(),
			`SELECT owner_email FROM offers_offer WHERE id = $1`, id).Scan(&owner); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "предложение не найдено"})
			return
		}
		role, _ := s.effectiveRole(r.Context(), appIDFromEnv(), actorEmailFromCtx(r))
		if !strings.EqualFold(owner, actorEmailFromCtx(r)) && !isAdminRole(role) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "править можно только своё предложение"})
			return
		}
		// Пустой выбор в запросе не стирает сохранённый (2026-11-03): плагин и
		// бот присылают предложение без этого поля, и полное сохранение
		// обнуляло бы системы, выбранные отдельной ручкой. С 2026-11-05 пустой
		// выбор — это `[]`, поэтому терпим оба вида. То же правило — материалы.
		_, err = s.pool.Exec(r.Context(), `
UPDATE offers_offer SET client = $2, items = $3, form_values = $4, total = $5,
	status = $6,
	system_choice = CASE WHEN $7::jsonb IN ('{}'::jsonb, '[]'::jsonb)
		THEN system_choice ELSE $7::jsonb END,
	material_choice = CASE WHEN $8::jsonb IN ('{}'::jsonb, '[]'::jsonb)
		THEN material_choice ELSE $8::jsonb END,
	updated_at = now()
WHERE id = $1`, id, clientJSON, itemsJSON, valuesJSON, offer.Total, offer.Status, systemJSON, materialJSON)
		if err != nil {
			dbError(w, "update offer", err)
			return
		}
	} else {
		number, err := s.nextNumber(r.Context(), time.Now().Year())
		if err != nil {
			dbError(w, "next number", err)
			return
		}
		err = s.pool.QueryRow(r.Context(), `
INSERT INTO offers_offer (number, owner_email, client, items, form_values, system_choice, material_choice, total, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
			number, strings.ToLower(actorEmailFromCtx(r)), clientJSON, itemsJSON,
			valuesJSON, systemJSON, materialJSON, offer.Total, offer.Status).Scan(&id)
		if err != nil {
			dbError(w, "insert offer", err)
			return
		}
	}

	saved, err := s.loadOffer(r.Context(), id)
	if err != nil {
		dbError(w, "запрос", err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteOffer(w http.ResponseWriter, r *http.Request) {
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
	if _, err := s.pool.Exec(r.Context(), `DELETE FROM offers_offer WHERE id = $1`, id); err != nil {
		dbError(w, "delete offer", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
