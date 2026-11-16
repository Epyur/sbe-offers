package main

// Точечная правка полей формы из приложения (0.11.3).
//
// Зачем своя ручка. Карточка КП в приложении показывает элементы окна
// `kind:"field"` — это поля формы (объект, срок действия, условия, менеджер…).
// До 0.11.3 они не рисовались вовсе, и сохранять их было некуда: `PUT
// /offers/{id}/client` пишет заказчика, `PUT /offers/{id}/items` — позиции.
//
// Почему PATCH, а не PUT. `offer.form_values` — ОДИН склад на три слоя данных:
//   - поля формы (`object_name`, `manager_name`, …);
//   - тексты разделов (`copy.<kind>`, их пишет модель);
//   - бриф (`brief.<поле>`, его заполняет менеджер).
// Полная замена стёрла бы два последних слоя: человек правит «Условия», а у
// него пропадают принятые тексты разделов и весь бриф. Поэтому:
//   - ключи в теле СЛИВАЮТСЯ с тем, что лежит в базе (JSONB `||`), а не
//     заменяют объект;
//   - ключи `copy.*` и `brief.*` ручка не принимает — они пишутся своими
//     путями, и молчаливая запись в них была бы вторым способом правды;
//   - пустая строка — законное значение (поле очистили); отсутствие ключа —
//     «не менять».
//
// Слияние — в SQL, а не в приложении: чтение-изменение-запись в коде
// проиграла бы гонку с приёмом текста раздела. Это отметили все модели
// совещания 2026-11-16.

import (
	"encoding/json"
	"net/http"
	"strings"
)

// reservedFieldPrefixes — ключи, которые пишутся не этой ручкой.
var reservedFieldPrefixes = []string{"copy.", "brief."}

// handlePatchOfferFormValues — PATCH /api/offers/offers/{id}/form-values.
//
// Тело: `{"values": {"object_name": "ТЦ Лето", "manager_name": "Иванов"}}`.
// Ответ — тот же `{offer, missingRequired}`, что у прочих ручек правки, чтобы
// приложение не гадало, что теперь на сервере.
func (s *Server) handlePatchOfferFormValues(w http.ResponseWriter, r *http.Request) {
	offer, ok := s.offerForEditor(w, r)
	if !ok {
		return
	}
	var req struct {
		Values map[string]string `json:"values"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if len(req.Values) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "нечего сохранять: нет полей"})
		return
	}

	// Белый список берётся из формы: записанное поле обязано быть заведено.
	// Иначе ручка стала бы произвольной записью в form_values.
	form, err := s.loadForm(r.Context(), clientFormMode)
	if err != nil {
		dbError(w, "form values load form", err)
		return
	}
	known := make(map[string]bool, len(form.Placeholders))
	for _, ph := range form.Placeholders {
		if id := strings.TrimSpace(ph.ID); id != "" {
			known[id] = true
		}
	}

	patch := make(map[string]string, len(req.Values))
	for key, value := range req.Values {
		key = strings.TrimSpace(key)
		if key == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "пустое имя поля"})
			return
		}
		for _, prefix := range reservedFieldPrefixes {
			if strings.HasPrefix(key, prefix) {
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"error": "поле «" + key + "» пишется своим путём: тексты разделов и бриф этой ручкой не правятся",
				})
				return
			}
		}
		if !known[key] {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "поле «" + key + "» не заведено в форме КП",
			})
			return
		}
		if len([]rune(value)) > 2000 {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "слишком длинное значение поля «" + key + "»: не больше 2000 символов",
			})
			return
		}
		patch[key] = value
	}

	patchJSON, err := json.Marshal(patch)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}
	// Слияние в SQL: `||` кладёт присланные ключи поверх старых, остальные
	// (copy.*, brief.*, скрытые поля) остаются нетронутыми.
	if _, err := s.pool.Exec(r.Context(), `
UPDATE offers_offer
SET form_values = COALESCE(form_values, '{}'::jsonb) || $2::jsonb,
	updated_at = now()
WHERE id = $1`, offer.ID, patchJSON); err != nil {
		dbError(w, "update offer form values", err)
		return
	}
	s.respondWithOffer(w, r, offer.ID)
}
