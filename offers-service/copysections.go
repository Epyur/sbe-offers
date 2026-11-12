package main

// Тексты разделов КП из приложения (2026-11-13, шаг S4 миграции в Mini App).
//
// Модель, прежде чем код:
//   - текстов ровно столько же, сколько слотов у бота (`SectionSlots`):
//     вступление, описание каждой выбранной системы, преимущества, условия;
//   - живут они ТАМ ЖЕ, где их держит бот, — в `form_values["copy.<ключ>"]`
//     (одно правило `SectionKey`). Второго хранилища не появляется;
//   - три действия те же, что у бота: написать заново, выправить присланный
//     текст, принять (сохранить) или пропустить;
//   - ручка для бота закрыта сервисным секретом (`/bot/*`), а приложение —
//     JWT-клиент, поэтому здесь ТОЛЬКО транспорт и права. Логика генерации
//     не копируется: она вызывается из тех же `WriteSectionChecked`,
//     `EditSection`, `AcceptSection`, что и в боте.
//
// Права: смотреть и менять тексты может автор предложения или admin — то же,
// что у точечных правок карточки (`offerForEditor`).

import (
	"net/http"
	"strconv"
	"strings"

	"mailers/offers/model"
	"mailers/offers/skills"
)

// sectionResponse — то, что видит экран: текст, замечания и подпись слота.
//
// `warnings` — чек-лист: он только СООБЩАЕТ, текст не переписывается молча
// (то же правило, что у бота).
type sectionResponse struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Text     string   `json:"text"`
	Warnings []string `json:"warnings"`
}

// handleListSections — GET /offers/{id}/sections: все слоты и их тексты.
//
// Всегда массив (правило 2026-11-02: пустой набор — `[]`, не `null`).
func (s *Server) handleListSections(w http.ResponseWriter, r *http.Request) {
	offer, ok := s.offerForEditor(w, r)
	if !ok {
		return
	}
	slots := SectionSlots(offer, len(offer.Systems))
	items := make([]sectionResponse, 0, len(slots))
	for _, slot := range slots {
		items = append(items, sectionResponse{
			Key:      slot.Key,
			Label:    slot.Label,
			Text:     SectionText(offer, slot.Kind, slot.SystemIndex),
			Warnings: []string{},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sections": items})
}

// handleWriteSection — POST /offers/{id}/sections/{kind}: модель пишет текст.
//
// Как и в боте, недоступность модели НЕ ошибка: приходит пустой текст и
// `warnings` пустой — экран честно покажет «не написалось», а документ
// соберётся без этой секции.
func (s *Server) handleWriteSection(w http.ResponseWriter, r *http.Request) {
	offer, ok := s.offerForEditor(w, r)
	if !ok {
		return
	}
	kind, index, ok := sectionTarget(w, r, len(offer.Systems))
	if !ok {
		return
	}
	brief := model.EmptyBriefPayload()
	if b, err := s.loadBrief(r.Context(), offer.ID); err == nil {
		brief = b.Payload
	}
	pc, _ := s.loadContext(r.Context(), "client")
	res, err := WriteSectionChecked(r.Context(), kind, brief, offer, pc, llmCompleter{})
	if err != nil {
		dbError(w, "copy write", err)
		return
	}
	writeJSON(w, http.StatusOK, sectionResponse{
		Key:      SectionKey(kind, index),
		Label:    sectionLabel(kind, index),
		Text:     res.Text,
		Warnings: nonNilStrings(res.Warnings),
	})
}

// handleEditSection — POST /offers/{id}/sections/{kind}/edit: вычитать правку.
//
// Возвращает выправленный текст, но НЕ сохраняет его: человек сначала видит,
// что получилось, и только потом нажимает «Принять» — как в боте.
func (s *Server) handleEditSection(w http.ResponseWriter, r *http.Request) {
	offer, ok := s.offerForEditor(w, r)
	if !ok {
		return
	}
	kind, index, ok := sectionTarget(w, r, len(offer.Systems))
	if !ok {
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "пустой текст: чтобы убрать раздел, нажмите «Пропустить»"})
		return
	}
	pc, _ := s.loadContext(r.Context(), "client")
	text, err := EditSection(r.Context(), kind, req.Text, pc, llmCompleter{})
	if err != nil {
		dbError(w, "copy edit", err)
		return
	}
	writeJSON(w, http.StatusOK, sectionResponse{
		Key:      SectionKey(kind, index),
		Label:    sectionLabel(kind, index),
		Text:     text,
		Warnings: []string{},
	})
}

// handleAcceptSection — PUT /offers/{id}/sections/{kind}: принять или пропустить.
//
// Пустой текст = «пропустить»: ключ удаляется, чтобы сборка документа отличала
// «пропущено» от «сохранён пустой» (`AcceptSection`). Запись двигает
// `updated_at` — через общий `persistOfferFormValues`.
func (s *Server) handleAcceptSection(w http.ResponseWriter, r *http.Request) {
	offer, ok := s.offerForEditor(w, r)
	if !ok {
		return
	}
	kind, index, ok := sectionTarget(w, r, len(offer.Systems))
	if !ok {
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	offer = AcceptSection(offer, kind, index, req.Text)
	if err := s.persistOfferFormValues(r.Context(), offer); err != nil {
		dbError(w, "copy accept", err)
		return
	}
	s.respondWithOffer(w, r, offer.ID)
}

// sectionTarget — разбор `{kind}` из адреса и номера системы.
//
// Номер системы обязателен только для `system`: у остальных слотов его нет,
// и пришедший с ним номер игнорируется — молча (это не ошибка ввода).
func sectionTarget(w http.ResponseWriter, r *http.Request, systems int) (skills.SectionKind, int, bool) {
	kind := sectionKindFromPath(r)
	if kind == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "неизвестная секция"})
		return "", 0, false
	}
	index := 0
	if raw := r.URL.Query().Get("systemIndex"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "systemIndex: нужен номер системы"})
			return "", 0, false
		}
		index = parsed
	}
	// У остальных слотов номера нет: пришедший `systemIndex` игнорируется,
	// иначе номер попал бы в ключ и текст перестал бы находиться.
	if kind != skills.SectionSystem {
		return kind, 0, true
	}
	if index < 1 || index > systems {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "описание системы: нужен systemIndex от 1 до числа выбранных систем",
		})
		return "", 0, false
	}
	return kind, index, true
}

// nonNilStrings — пустой набор в JSON только массивом.
func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
