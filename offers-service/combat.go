package main

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

// Боевой листок (2026-11-08) — быстрая справка для менеджера коммерческой
// службы: критическое сравнение нашего продукта (систем, что идут в КП) с
// продуктами конкурентов.
//
// Это СНАПШОТ, как и коммерческое предложение: номер и состав фиксируются в
// момент сохранения. Правка КП или карточки Базы знаний не переписывает уже
// выпущенный боевой листок молча — иначе ссылка в споре с заказчиком меняла бы
// смысл задним числом.
//
// Наш продукт — `our_notes`, продукт конкурента — `competitor_notes`. Оба —
// списки `{noteId, title}`: заметка Базы знаний и её название на момент выбора.
// Текущий текст карточки подтягивается по `noteId` из кеша конкурентов/систем.

// CombatNoteRef — ссылка на заметку Базы знаний внутри боевого листка.
type CombatNoteRef struct {
	NoteID int64  `json:"noteId"`
	Title  string `json:"title"`
}

// CombatSheet — боевой листок.
type CombatSheet struct {
	ID         int64  `json:"id"`
	Number     string `json:"number"`
	OwnerEmail string `json:"ownerEmail"`
	Title      string `json:"title"`
	// OfferID — КП, чей продукт защищаем. 0 — не привязан, наш продукт выбран
	// руками из систем.
	OfferID         int64             `json:"offerId"`
	OurNotes        []CombatNoteRef   `json:"ourNotes"`
	CompetitorNotes []CombatNoteRef   `json:"competitorNotes"`
	FormValues      map[string]string `json:"formValues"`
	Status          string            `json:"status"`
	CreatedAt       string            `json:"createdAt"`
	UpdatedAt       string            `json:"updatedAt"`
	// DisplayName — подпись листка в списке: продукт, а не «Боевой листок»
	// (2026-11-12). Поле вычисляет сервер у списка; в таблице его нет, потому
	// что оно зависит от сторон и меняется вместе с ними.
	DisplayName string `json:"displayName,omitempty"`
}

// validateCombatSheet проверяет боевой листок перед сохранением; пустая
// строка — корректно.
func validateCombatSheet(c CombatSheet) string {
	if strings.TrimSpace(c.Title) == "" {
		return "не заполнено название боевого листка"
	}
	// Ни наш продукт, ни продукт конкурента не обязателен: черновик нужен сразу,
	// как только менеджер начал собирать сравнение. Пустой итоговый документ
	// печатать не даст сборка — это её дело, а не проверки на входе.
	for _, n := range c.OurNotes {
		if n.NoteID <= 0 {
			return "у нашего продукта нет заметки Базы знаний"
		}
	}
	for _, n := range c.CompetitorNotes {
		if n.NoteID <= 0 {
			return "у продукта конкурента нет заметки Базы знаний"
		}
	}
	return ""
}

// nextCombatNumber выдаёт следующий номер по году. Счётчик свой, отдельный от
// КП: боевой листок — другой документ, и общая нумерация путала бы менеджера.
// advisory-lock тот же, что у предложений: параллельные сохранения не получат
// один номер.
func (s *Server) nextCombatNumber(ctx context.Context, year int) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	suffix := fmt.Sprintf("/%d", year)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(year)+1_000_000); err != nil {
		return "", err
	}
	var maxNum int
	err = tx.QueryRow(ctx, `
SELECT COALESCE(MAX(NULLIF(regexp_replace(split_part(number, '/', 1), '\D', '', 'g'), '')::int), 0)
FROM offers_combat_sheet WHERE number LIKE '%' || $1`, suffix).Scan(&maxNum)
	if err != nil {
		return "", err
	}
	number := formatNumber(maxNum+1, year)
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return number, nil
}

// loadCombatSheet читает боевой листок по id.
func (s *Server) loadCombatSheet(ctx context.Context, id int64) (CombatSheet, error) {
	var c CombatSheet
	var our, comp, values []byte
	err := s.pool.QueryRow(ctx, `
SELECT id, number, owner_email, title, COALESCE(offer_id, 0), our_notes, competitor_notes,
	form_values, status,
	to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
	to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
FROM offers_combat_sheet WHERE id = $1`, id).
		Scan(&c.ID, &c.Number, &c.OwnerEmail, &c.Title, &c.OfferID, &our, &comp,
			&values, &c.Status, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return c, err
	}
	c.OurNotes = []CombatNoteRef{}
	c.CompetitorNotes = []CombatNoteRef{}
	c.FormValues = map[string]string{}
	if len(our) > 0 {
		_ = json.Unmarshal(our, &c.OurNotes)
	}
	if len(comp) > 0 {
		_ = json.Unmarshal(comp, &c.CompetitorNotes)
	}
	if len(values) > 0 {
		_ = json.Unmarshal(values, &c.FormValues)
	}
	return c, nil
}

// canSeeCombat — видимость боевого листка (правило 2026-11-12).
//
// **Листки открыты всем, кто вошёл в приложение.** Раньше было как у
// предложений — admin видит все, остальные только свои, — но листок это
// внутренняя справка для спора с заказчиком: менеджер берёт готовое сравнение
// коллеги и печатает, не пересобирая. Закрывать чтение значило бы заставлять
// каждого заново собирать то же самое.
//
// Правит листок по-прежнему только автор (`botCanEditCombat` и проверка
// владельца при сохранении): читать чужое можно, менять — нет.
// Сейчас чтение открыто всем, кто вошёл в приложение (роль уже проверена requirePerm("viewer")
// на маршруте). Правка и удаление остаются у автора.
func canSeeCombat(_ string, _ string, _ CombatSheet) bool {
	return true
}

func (s *Server) handleListCombat(w http.ResponseWriter, r *http.Request) {
	// Список показывает ВСЕ листки (правило 2026-11-12): листок — общая
	// справка, а не личный документ. Правка остаётся у автора.
	//
	// Поиск (2026-11-13, шаг S2; порядок исправлен после стенда): по названию
	// и по названиям сторон. Названия сторон лежат в jsonb, поэтому SQL ищет
	// только по названию листка (индексный срез + встроенная подстрока), а
	// окончательное решение принимает `combatSearchText` — он видит и стороны.
	// Строк просматривается `listScanLimit`, отдаётся не больше `listLimit`:
	// иначе совпадения глубже свежих двухсот не находились бы (тот же дефект,
	// что был у КП).
	q := parseListQuery(r.URL.Query().Get("q"))
	query := `SELECT id, title FROM offers_combat_sheet`
	args := []any{}
	if !q.Empty() {
		query += ` WHERE (lower(title) LIKE $1` +
			` OR lower(title) LIKE $2` +
			` OR lower(replace(title, 'ё', 'е')) LIKE $3)`
		args = append(args, q.PrefixPattern(), q.WordPattern(), q.ContainsPattern())
	}
	scan := listLimit
	if !q.Empty() {
		scan = listScanLimit
	}
	rows, err := s.pool.Query(r.Context(),
		query+` ORDER BY created_at DESC LIMIT `+itoa(scan), args...)
	if err != nil {
		dbError(w, "list combat", err)
		return
	}
	ids := make([]int64, 0, 32)
	for rows.Next() {
		var id int64
		var title string
		if err := rows.Scan(&id, &title); err != nil {
			rows.Close()
			dbError(w, "list combat scan", err)
			return
		}
		ids = append(ids, id)
	}
	rows.Close()

	sheets := make([]CombatSheet, 0, len(ids))
	for _, id := range ids {
		c, err := s.loadCombatSheet(r.Context(), id)
		if err != nil {
			continue
		}
		if !q.Empty() && !containsTie(combatSearchText(c), q.Contains()) {
			continue
		}
		sheets = append(sheets, c)
		if len(sheets) >= listLimit {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sheets": sheets})
}

func (s *Server) handleGetCombat(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	c, err := s.loadCombatSheet(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "боевой листок не найден"})
		return
	}
	role, _ := s.effectiveRole(r.Context(), appIDFromEnv(), actorEmailFromCtx(r))
	if !canSeeCombat(role, actorEmailFromCtx(r), c) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "нет доступа к этому боевому листку"})
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleSaveCombat(w http.ResponseWriter, r *http.Request) {
	var c CombatSheet
	if err := decodeJSON(w, r, &c, 4<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if msg := validateCombatSheet(c); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
		return
	}
	if c.Status != "final" {
		c.Status = "draft"
	}
	normalizeCombat(&c)

	cols, err := combatColumns(c)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}

	raw := r.PathValue("id")
	var id int64
	if raw == "" {
		id, err = s.createCombat(r.Context(), actorEmailFromCtx(r), c, cols)
		if err != nil {
			dbError(w, "insert combat", err)
			return
		}
	} else {
		id, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
			return
		}
		if status, msg := s.combatEditAllowed(r, id); status != http.StatusOK {
			writeJSON(w, status, map[string]any{"error": msg})
			return
		}
		if err := s.updateCombat(r.Context(), id, c, cols); err != nil {
			dbError(w, "update combat", err)
			return
		}
	}

	saved, err := s.loadCombatSheet(r.Context(), id)
	if err != nil {
		dbError(w, "reload combat", err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// combatColumns — JSON-представление полей листка для записи в БД. Три
// маршала в одном месте: раньше три одинаковых блока ошибок стояли подряд.
type combatCols struct {
	our    []byte
	comp   []byte
	values []byte
	offer  any
}

func combatColumns(c CombatSheet) (combatCols, error) {
	var cols combatCols
	var err error
	if cols.our, err = json.Marshal(c.OurNotes); err != nil {
		return cols, err
	}
	if cols.comp, err = json.Marshal(c.CompetitorNotes); err != nil {
		return cols, err
	}
	if cols.values, err = json.Marshal(c.FormValues); err != nil {
		return cols, err
	}
	if c.OfferID > 0 {
		cols.offer = c.OfferID
	}
	return cols, nil
}

// combatEditAllowed проверяет право правки существующего листка. Отдаёт HTTP-код
// и текст отказа; 200 — можно править. Вынесено из обработчика: та же проверка
// нужна и сохранению, и в будущем удалению по владельцу.
func (s *Server) combatEditAllowed(r *http.Request, id int64) (int, string) {
	var owner string
	if err := s.pool.QueryRow(r.Context(),
		`SELECT owner_email FROM offers_combat_sheet WHERE id = $1`, id).Scan(&owner); err != nil {
		return http.StatusNotFound, "боевой листок не найден"
	}
	role, _ := s.effectiveRole(r.Context(), appIDFromEnv(), actorEmailFromCtx(r))
	if !strings.EqualFold(owner, actorEmailFromCtx(r)) && !isAdminRole(role) {
		return http.StatusForbidden, "править можно только свой боевой листок"
	}
	return http.StatusOK, ""
}

// createCombat вставляет новый листок, выдаёт ему номер года и возвращает id.
func (s *Server) createCombat(ctx context.Context, owner string, c CombatSheet, cols combatCols) (int64, error) {
	number, err := s.nextCombatNumber(ctx, time.Now().Year())
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.pool.QueryRow(ctx, `
INSERT INTO offers_combat_sheet (number, owner_email, title, offer_id, our_notes, competitor_notes, form_values, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		number, strings.ToLower(owner), c.Title, cols.offer,
		cols.our, cols.comp, cols.values, c.Status).Scan(&id)
	return id, err
}

// updateCombat пишет поля существующего листка и двигает updated_at.
func (s *Server) updateCombat(ctx context.Context, id int64, c CombatSheet, cols combatCols) error {
	_, err := s.pool.Exec(ctx, `
UPDATE offers_combat_sheet SET title = $2, offer_id = $3, our_notes = $4, competitor_notes = $5,
	form_values = $6, status = $7, updated_at = now()
WHERE id = $1`, id, c.Title, cols.offer, cols.our, cols.comp, cols.values, c.Status)
	return err
}

func (s *Server) handleDeleteCombat(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	c, err := s.loadCombatSheet(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "боевой листок не найден"})
		return
	}
	role, _ := s.effectiveRole(r.Context(), appIDFromEnv(), actorEmailFromCtx(r))
	if !canSeeCombat(role, actorEmailFromCtx(r), c) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "нет доступа к этому боевому листку"})
		return
	}
	if _, err := s.pool.Exec(r.Context(), `DELETE FROM offers_combat_sheet WHERE id = $1`, id); err != nil {
		dbError(w, "delete combat", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// normalizeCombat приводит списки и карту к непустому виду: nil-срезы и nil-карта
// в JSON превратились бы в `null`, а клиент ждёт массивы и объект.
func normalizeCombat(c *CombatSheet) {
	if c.OurNotes == nil {
		c.OurNotes = []CombatNoteRef{}
	}
	if c.CompetitorNotes == nil {
		c.CompetitorNotes = []CombatNoteRef{}
	}
	if c.FormValues == nil {
		c.FormValues = map[string]string{}
	}
}

// handleExportCombat собирает DOCX боевого листка (2026-11-08).
//
// Документ печатается по форме боевого листка (mode=combat), а карточки сторон
// берутся из кешей: наш продукт — из кеша карточек систем, конкурент — из кеша
// конкурентов. Карточки, которых в кеше нет (заметку удалили), просто не
// печатаются: листок собирается по тому, что известно, а не падает.
func (s *Server) handleExportCombat(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	sheet, err := s.loadCombatSheet(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "боевой листок не найден"})
		return
	}
	role, _ := s.effectiveRole(r.Context(), appIDFromEnv(), actorEmailFromCtx(r))
	if !canSeeCombat(role, actorEmailFromCtx(r), sheet) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "нет доступа к этому боевому листку"})
		return
	}

	// Содержание листка фиксировано, поэтому от формы нужен только макет
	// (2026-11-11). Ошибка чтения макета не гасит печать: без макета листок
	// печатается встроенным оформлением.
	layout, err := s.loadLayoutForMode(r.Context(), combatFormMode)
	if err != nil {
		log.Printf("боевой листок: макет не прочитан: %v", err)
		layout = nil
	}
	ourCards := s.resolveCombatCards(r.Context(), sheet.OurNotes, s.loadCachedSystemCards)
	compCards := s.resolveCombatCards(r.Context(), sheet.CompetitorNotes,
		func(ctx context.Context) ([]SystemCard, error) {
			cards, _, err := s.loadCachedCompetitors(ctx)
			return cards, err
		})
	data, err := s.renderCombatDocxCached(r.Context(), layout, sheet, ourCards, compCards, NewKBImageSource(s.kb))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+docxFileNameFor("Боевой листок", sheet.Number))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// resolveCombatCards сопоставляет ссылки на заметки с карточками из кеша.
//
// `load` — загрузчик кеша: у нас и конкурентов он разный (разные наборы), но
// правило одно — по `noteId` из готового кеша. Порядок карточек — порядок
// выбора в листке, а не порядок кеша: так документ повторяет то, что видел
// менеджер.
func (s *Server) resolveCombatCards(ctx context.Context, refs []CombatNoteRef,
	load func(context.Context) ([]SystemCard, error)) []SystemCard {
	if len(refs) == 0 {
		return nil
	}
	cached, err := load(ctx)
	if err != nil {
		return nil
	}
	byNote := make(map[int64]SystemCard, len(cached))
	for _, card := range cached {
		byNote[card.NoteID] = card
	}
	out := make([]SystemCard, 0, len(refs))
	for _, ref := range refs {
		if card, ok := byNote[ref.NoteID]; ok {
			out = append(out, card)
		}
	}
	return out
}
