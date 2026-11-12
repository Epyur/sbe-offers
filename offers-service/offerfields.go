package main

// Точечные правки документа (2026-11-13, шаг S3 миграции в Mini App).
//
// Модель, согласованная с пользователем (вариант «В», дословно):
//   - `PUT /offers/{id}/client` — имя и контакт заказчика;
//   - `PUT /offers/{id}/items`  — позиции целиком (список строк);
//   - системы правятся существующими `POST/DELETE /offers/{id}/system`.
//
// Почему не один `PUT /offers/{id}` на весь документ: приложение сохраняет
// карточку по частям, и полное сохранение затирало бы то, что в другой вкладке
// (или в боте) поменяли параллельно — «устаревшая вкладка побеждает» это
// худший вид потери данных. Точечная ручка касается РОВНО одного поля: если
// человек правил заказчика, позиции в запрос не входят вовсе.
//
// Общие правила, которые соблюдают обе ручки: пустая правка отвергается
// (иначе сохранение «без изменений» молча стирало бы поле), каждая запись
// двигает `updated_at` (иначе правка не выиграет слияние при синхронизации),
// ответ — тот же `Offer`, что отдаёт `GET /offers/{id}`, чтобы клиенту не
// приходилось гадать, что теперь на сервере.

import (
	"encoding/json"
	"net/http"
	"strings"
)

// handlePatchOfferClient — заказчик: имя и контакт.
func (s *Server) handlePatchOfferClient(w http.ResponseWriter, r *http.Request) {
	offer, ok := s.offerForEditor(w, r)
	if !ok {
		return
	}
	var req struct {
		Name    string `json:"name"`
		Contact string `json:"contact"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if msg := validateOfferClient(OfferClient{Name: req.Name, Contact: req.Contact}); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
		return
	}
	clientJSON, err := json.Marshal(OfferClient{Name: strings.TrimSpace(req.Name), Contact: strings.TrimSpace(req.Contact)})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}
	if _, err := s.pool.Exec(r.Context(),
		`UPDATE offers_offer SET client = $2, updated_at = now() WHERE id = $1`,
		offer.ID, clientJSON); err != nil {
		dbError(w, "update offer client", err)
		return
	}
	s.respondWithOffer(w, r, offer.ID)
}

// handlePatchOfferItems — позиции целиком.
//
// Приходящий список ПЕРЕЗАПИСЫВАЕТ позиции, а не дополняет их: редактор шлёт
// то, что человек видит на экране. Сумма и итог пересчитываются на сервере —
// присланные `sum`/`total` не читаются вовсе, и приложение может считать их
// как хочет (только для показа).
func (s *Server) handlePatchOfferItems(w http.ResponseWriter, r *http.Request) {
	offer, ok := s.offerForEditor(w, r)
	if !ok {
		return
	}
	var req struct {
		Items []OfferItem `json:"items"`
	}
	if err := decodeJSON(w, r, &req, 4<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if msg := validateOfferItems(req.Items); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
		return
	}
	total := recalcOfferTotal(req.Items)
	itemsJSON, err := json.Marshal(req.Items)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}
	if _, err := s.pool.Exec(r.Context(),
		`UPDATE offers_offer SET items = $2, total = $3, updated_at = now() WHERE id = $1`,
		offer.ID, itemsJSON, total); err != nil {
		dbError(w, "update offer items", err)
		return
	}
	s.respondWithOffer(w, r, offer.ID)
}

// validateOfferClient — заказчик может быть пустым (черновик заполняют по
// частям), но имя не может состоять из пробелов: это не имя.
func validateOfferClient(client OfferClient) string {
	name := strings.TrimSpace(client.Name)
	contact := strings.TrimSpace(client.Contact)
	if name == "" && contact != "" {
		return "укажите заказчика, а не только контакт"
	}
	if len([]rune(name)) > 300 || len([]rune(contact)) > 300 {
		return "слишком длинное поле: не больше 300 символов"
	}
	return ""
}

// validateOfferItems отличается от validateOffer ровно одним: пустой список
// здесь законен. Позиции вводятся по одной, и «сначала сохранил заказчика,
// потом доберу строки» — нормальный порядок работы, а не ошибка. Полное
// сохранение (`PUT /offers/{id}`) по-прежнему требует позиции.
func validateOfferItems(items []OfferItem) string {
	if len(items) > 500 {
		return "слишком много строк: не больше 500"
	}
	for _, item := range items {
		if strings.TrimSpace(item.Name) == "" {
			return "в позиции не заполнено наименование"
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

// respondWithOffer — ответ = документ после записи, как у `GET`.
func (s *Server) respondWithOffer(w http.ResponseWriter, r *http.Request, id int64) {
	saved, err := s.loadOffer(r.Context(), id)
	if err != nil {
		dbError(w, "запрос", err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}
