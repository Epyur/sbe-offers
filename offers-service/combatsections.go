package main

// Боевой листок в приложении (2026-11-13, шаг S6 миграции в Mini App).
//
// Модель, прежде чем код:
//   - листок — тот же документ, что у бота (`offers_combat_sheet`): номер,
//     наши продукты (`our_notes`), продукты конкурентов (`competitor_notes`),
//     уточнение (`form_values["hint"]`), автор;
//   - читают и печатают ВСЕ вошедшие (правило 2026-11-12), правит только
//     автор или admin — то же, что `botCanEditCombat`/`handleSaveCombat`;
//   - стороны правятся ПОЗИЦИОННО (`POST/DELETE .../our|competitor`), а не
//     полным сохранением листка: приложение добавляет и убирает по одному
//     продукту, и `PUT /combat/{id}` затёр бы чужую правку;
//   - подбор аналогов и поиск сторон — те же функции, что у бота
//     (`findAnaloguesForSheet`, `searchCompetitors`, `combatOurItems`),
//     скопирован только транспорт и гейт (JWT вместо сервисного секрета).
//
// Почему новые ручки, а не `/bot/*`: ручка, которую зовут с `X-Service-Secret`,
// обязана жить в `/bot/*` (правило 2026-11-17 → 2026-11-13, живой 403 0.9.17).
// Приложение — JWT-клиент, значит ему нужны парные маршруты без секрета.

import (
	"net/http"
	"strconv"
	"strings"
)

// combatSideOur/combatSideCompetitor уже объявлены в botcombat.go.

// combatSideRequest — что добавить или убрать: карточка Базы знаний.
type combatSideRequest struct {
	NoteID int64  `json:"noteId"`
	Title  string `json:"title"`
}

// handleCombatOurList — GET /combat/our?q=&offset=&limit=: наш продукт.
//
// Наш продукт — материал каталога ИЛИ система Базы знаний (`combatOurItems`):
// снаружи это один список, человек не думает, откуда пришла карточка.
func (s *Server) handleCombatOurList(w http.ResponseWriter, r *http.Request) {
	items, err := s.combatOurItems(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, pageJSON(paginateCombatItems(nil, 0, 0), map[string]any{
			"error": err.Error(),
		}))
		return
	}
	filtered := filterCombatItems(items, r.URL.Query().Get("q"))
	offset, limit := parseCombatPage(r)
	writeJSON(w, http.StatusOK, pageJSON(paginateCombatItems(filtered, offset, limit), nil))
}

// handleCombatCompetitors — GET /combat/competitors-src?q=&offset=&limit=.
//
// Конкуренты — тег `competitor` и поддерево папки (как у бота). Поиск идёт в
// Базу знаний, кеш — запасной путь, когда БЗ недоступна.
//
// Адрес назван `competitors-src`, а не `combat/competitors`, потому что
// `GET /competitors` уже занят кешем для плагина (раздел «Конкуренты»), и две
// разные выдачи под одним путём — источник путаницы при разборе жалоб.
func (s *Server) handleCombatCompetitorsSrc(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	offset, limit := parseCombatPage(r)
	if strings.TrimSpace(query) != "" {
		if items, err := s.searchCompetitors(r.Context(), query); err == nil {
			writeJSON(w, http.StatusOK, pageJSON(paginateCombatItems(items, offset, limit), map[string]any{
				"folderFound": true,
				"searched":    true,
			}))
			return
		}
	}
	cards, found, err := s.loadCachedCompetitors(r.Context())
	if err == nil && len(cards) == 0 && s.kb != nil && s.kb.configured() {
		cards, found, err = s.refreshCompetitorsCache(r.Context(), "")
	}
	if err != nil {
		writeJSON(w, http.StatusOK, pageJSON(paginateCombatItems(nil, offset, limit), map[string]any{
			"folderFound": found,
			"error":       err.Error(),
		}))
		return
	}
	filtered := filterCombatItems(combatItems(cards), query)
	writeJSON(w, http.StatusOK, pageJSON(paginateCombatItems(filtered, offset, limit), map[string]any{
		"folderFound": found,
	}))
}

// pageJSON — форма страницы выбора, одна на все ручки приложения.
func pageJSON(page botCombatPage, extra map[string]any) map[string]any {
	out := map[string]any{
		"items":   page.Items,
		"total":   page.Total,
		"offset":  page.Offset,
		"hasMore": page.HasMore,
	}
	for key, value := range extra {
		out[key] = value
	}
	return out
}

// handleCombatAddSide / handleCombatRemoveSide — POST/DELETE sides.
func (s *Server) handleCombatOurSide(w http.ResponseWriter, r *http.Request, add bool) {
	s.combatSide(w, r, combatSideOur, add)
}

func (s *Server) handleCombatCompetitorSide(w http.ResponseWriter, r *http.Request, add bool) {
	s.combatSide(w, r, combatSideCompetitor, add)
}

// combatSide — общий путь добавления и удаления стороны.
//
// Гейт: `requirePerm("viewer")` на маршруте + проверка автора здесь. Читать и
// печатать листок может любой (правило 2026-11-12), а менять — только автор:
// иначе черновик коллеги переписывался бы чужими кнопками.
func (s *Server) combatSide(w http.ResponseWriter, r *http.Request, side string, add bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	var req combatSideRequest
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.NoteID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "у карточки нет заметки Базы знаний"})
		return
	}
	sheet, ok := s.combatSheetForEditor(w, r, id)
	if !ok {
		return
	}
	ref := CombatNoteRef{NoteID: req.NoteID, Title: strings.TrimSpace(req.Title)}
	target := &sheet.OurNotes
	if side == combatSideCompetitor {
		target = &sheet.CompetitorNotes
	}
	if add {
		*target = appendNoteRef(*target, ref)
	} else {
		*target = removeNoteRef(*target, ref.NoteID)
	}
	normalizeCombat(&sheet)
	cols, err := combatColumns(sheet)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}
	if err := s.updateCombat(r.Context(), id, sheet, cols); err != nil {
		dbError(w, "update combat side", err)
		return
	}
	saved, err := s.loadCombatSheet(r.Context(), id)
	if err != nil {
		dbError(w, "reload combat", err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// combatSheetForEditor — листок с проверкой права правки (автор или admin).
func (s *Server) combatSheetForEditor(w http.ResponseWriter, r *http.Request, id int64) (CombatSheet, bool) {
	sheet, err := s.loadCombatSheet(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "боевой листок не найден"})
		return CombatSheet{}, false
	}
	role, _ := s.effectiveRole(r.Context(), appIDFromEnv(), actorEmailFromCtx(r))
	if !botCanEditCombat(sheet, actorEmailFromCtx(r)) && !isAdminRole(role) {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": "Править можно только свой листок — читать и печатать может любой.",
		})
		return CombatSheet{}, false
	}
	return sheet, true
}

// handleCombatHintUpdate — PUT /combat/{id}/hint: уточнение для разбора.
//
// Уточнение — единственная настройка листка (`form_values["hint"]`): на чём
// сравнение должно сделать акцент. Отдельная ручка, а не полное сохранение:
// она же двигает `updated_at` через `updateCombat`.
func (s *Server) handleCombatHintUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	sheet, ok := s.combatSheetForEditor(w, r, id)
	if !ok {
		return
	}
	var req struct {
		Hint string `json:"hint"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if sheet.FormValues == nil {
		sheet.FormValues = map[string]string{}
	}
	sheet.FormValues["hint"] = strings.TrimSpace(req.Hint)
	cols, err := combatColumns(sheet)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}
	if err := s.updateCombat(r.Context(), id, sheet, cols); err != nil {
		dbError(w, "update combat hint", err)
		return
	}
	saved, err := s.loadCombatSheet(r.Context(), id)
	if err != nil {
		dbError(w, "reload combat", err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// handleCombatAnaloguesJWT — GET /combat/analogues?noteId=: JWT-двойник
// ботовой ручки. Логика та же (`findAnaloguesForSheet`): поиск конкурентов
// того же класса по нашему продукту.
func (s *Server) handleCombatAnaloguesJWT(w http.ResponseWriter, r *http.Request) {
	noteID := parseIDParam(r.URL.Query().Get("noteId"))
	if noteID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "не задан продукт"})
		return
	}
	source, ok := s.analogSourceCard(r, noteID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error": "продукт не найден в кеше — администратору нужно обновить раздел «Конкуренты»",
		})
		return
	}
	found := s.findAnaloguesForSheet(r.Context(), source)
	writeJSON(w, http.StatusOK, map[string]any{
		"source": map[string]any{"noteId": source.NoteID, "title": source.Title},
		"cards":  combatItems(found),
	})
}
