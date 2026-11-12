package main

import (
	"context"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// Боевой листок в боте (2026-11-09).
//
// Сущность и печать уже есть в combat.go — боевой листок собирается в боте теми
// же функциями, что и в плагине: отдельного разбора данных и отдельного DOCX в
// боте нет. Здесь только то, что отличает чат от плагина:
//
//   - владелец листка — email привязанного chat_id (роли в боте нет);
//   - выбор стороны — не руками по `noteId`, а из готовых списков: у нас —
//     кеш карточек систем, у конкурента — кеш папки «материалы и системы
//     конкурентов». Бот показывает названия, а не заметки.
//
// Черновик создаётся пустым и получает номер сразу (как КП в боте): менеджер
// выбирает стороны по одной, а не заполняет форму целиком перед сохранением.

// botSystemItem — строка выбора стороны в боте: то, что увидит менеджер.
//
// Название и заметка: код системы живых карточек часто пуст, а выбирать надо
// по заметке — сервер наполняет `ourNotes`/`competitorNotes` именно ими.
type botSystemItem struct {
	NoteID   int64    `json:"noteId"`
	Title    string   `json:"title"`
	SKU      string   `json:"sku,omitempty"`
	Problems []string `json:"problems,omitempty"`
}

// combatSideMapper — как показать карточку и как положить её в листок: одна
// форма на две стороны (наш продукт и конкурент), чтобы списки не разошлись.
func combatItems(cards []SystemCard) []botSystemItem {
	items := make([]botSystemItem, 0, len(cards))
	for _, card := range cards {
		items = append(items, botSystemItem{
			NoteID:   card.NoteID,
			Title:    card.Title,
			SKU:      card.SKU,
			Problems: card.Problems,
		})
	}
	return items
}

// normalizeCombatQuery приводит запрос к нижнему регистру, «ё»→«е» и схлопывает
// пробелы: менеджер пишет с телефона, и «кров laatste»/«КРОВЛЯ» — один запрос.
func normalizeCombatQuery(raw string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(raw))), " ")
}

// filterCombatItems — поиск стороны по названию и коду.
//
// Все слова запроса обязаны найтись (в названии или коде) — тот же приём, что у
// каталога: «кровля мембрана» не ловится одной подстрокой. Пустой запрос
// возвращает весь список: без поиска сторона выбирается из первых, а полный
// перечень менеджеру не нужен.
func filterCombatItems(items []botSystemItem, query string) []botSystemItem {
	q := normalizeCombatQuery(query)
	if q == "" {
		return items
	}
	words := strings.Fields(q)
	out := make([]botSystemItem, 0, len(items))
	for _, item := range items {
		haystack := normalizeCombatQuery(item.Title + " " + item.SKU)
		matched := true
		for _, w := range words {
			if !strings.Contains(haystack, w) {
				matched = false
				break
			}
		}
		if matched {
			out = append(out, item)
		}
	}
	return out
}

// botCombatPage — что отдаётся боту для выбора стороны: страница списка и
// сколько всего совпадений. Пагинация обязательна (2026-11-10): каталог 1500+
// записей, а Telegram не любит длинные сообщения и большие клавиатуры.
type botCombatPage struct {
	Items   []botSystemItem
	Total   int
	Offset  int
	HasMore bool
}

// combatPageSizeDefault — сколько строк на страницу, если бот не попросил своё.
// Совпадает с размером страницы в боте документов: восемь строк читаются на
// телефоне без прокрутки, а номера кнопок помещаются в один ряд.
const combatPageSizeDefault = 8

// combatPageSizeMax — потолок: бот просит размер сам, но сервер не отдаёт
// больше этого, чтобы одна клавиатура не перестала отправляться.
const combatPageSizeMax = 50

// paginateCombatItems берёт запрошенную страницу из отфильтрованного списка.
//
// Значения `offset`/`limit` приходят от бота, поэтому проверяются здесь: малое
// или нечитаемое, оно молча становится умолчанием, а не ломает ответ. Потолок
// у `limit` и нижняя граница у `offset` — единственные ограничения.
func paginateCombatItems(items []botSystemItem, offset, limit int) botCombatPage {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = combatPageSizeDefault
	}
	if limit > combatPageSizeMax {
		limit = combatPageSizeMax
	}
	total := len(items)
	if offset > total {
		offset = total
	}
	end := offset + limit
	hasMore := end < total
	if end > total {
		end = total
	}
	page := append([]botSystemItem{}, items[offset:end]...)
	return botCombatPage{Items: page, Total: total, Offset: offset, HasMore: hasMore}
}

// parseCombatPage разбирает `offset` и `limit` из запроса.
func parseCombatPage(r *http.Request) (offset, limit int) {
	q := r.URL.Query()
	offset, _ = strconv.Atoi(q.Get("offset"))
	limit, _ = strconv.Atoi(q.Get("limit"))
	return offset, limit
}

// writeCombatPage отвечает страницей списка сторон: карточки, всего, смещение и
// признак «есть ещё». Флаг `found` здесь не используется — только конкурентам
// важно, найден ли источник; наш продукт всегда есть в кеше или в каталоге.
func writeCombatPage(w http.ResponseWriter, page botCombatPage, extra map[string]any) {
	body := map[string]any{
		"cards":   page.Items,
		"total":   page.Total,
		"offset":  page.Offset,
		"hasMore": page.HasMore,
	}
	for k, v := range extra {
		body[k] = v
	}
	writeJSON(w, http.StatusOK, body)
}

// handleBotCombatSystems — наш продукт для выбора стороны в боевом листке.
//
// Набор — материалы КАТАЛОГА и карточки систем Базы знаний вместе (2026-11-09):
// менеджер спорит с конкурентом конкретным товаром, а его код и цену он знает
// по каталогу. Каталог — 1500+ записей, перебирать их списком бессмысленно,
// поэтому `q` обязателен по смыслу: без него отдаём первый срез, а с ним —
// совпадения по названию.
//
// Кеш карточек (как и в плагине): экран не ходит в Базу знаний на каждый показ.
// Если кеша нет, но База знаний настроена, читаем один раз — иначе первый запуск
// бота давал пустой список без объяснения.
func (s *Server) handleBotCombatSystems(w http.ResponseWriter, r *http.Request) {
	items, err := s.combatOurItems(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"cards": []any{},
			"error": err.Error(),
		})
		return
	}
	query := r.URL.Query().Get("q")
	filtered := filterCombatItems(items, query)
	offset, limit := parseCombatPage(r)
	writeCombatPage(w, paginateCombatItems(filtered, offset, limit), nil)
}

// combatOurItems — список нашего продукта для боевого листка: материалы каталога
// и системы Базы знаний в одной форме выбора.
//
// Материал без заметки Базы знаний в листок попасть не может (у него нет
// карточки для документа), поэтому такие строки отбрасываются здесь, а не
// молча падают потом при сохранении стороны.
func (s *Server) combatOurItems(ctx context.Context) ([]botSystemItem, error) {
	materials, err := s.listMaterials(ctx, false)
	if err != nil {
		return nil, err
	}
	items := make([]botSystemItem, 0, len(materials))
	seen := make(map[int64]bool)
	// Материалы — основной набор: это то, что продаётся и что менеджер ищет
	// по каталогу.
	for _, m := range materials {
		if m.NoteID <= 0 || m.Archived || seen[m.NoteID] {
			continue
		}
		seen[m.NoteID] = true
		items = append(items, botSystemItem{NoteID: m.NoteID, Title: m.Name, SKU: m.SKU})
	}
	// Системы добавляются вторым проходом и вытесняются теми же заметками, если
	// материал уже занял эту карточку (одна и та же заметка не должна быть в
	// списке дважды — листок хранит сторону по заметке).
	for _, card := range s.systemCardsForCombat(ctx, materials) {
		if card.NoteID <= 0 || seen[card.NoteID] {
			continue
		}
		seen[card.NoteID] = true
		items = append(items, botSystemItem{
			NoteID:   card.NoteID,
			Title:    card.Title,
			SKU:      card.SKU,
			Problems: card.Problems,
		})
	}
	return items, nil
}

// systemCardsForCombat — системы для списка сторон: кеш карточек, а при пустом
// кеше — разовое чтение из Базы знаний с сохранением результата.
//
// Ошибку гасим намеренно: системы дополняют материалы, и их недоступность не
// должна гасить весь список нашего продукта (материалы уже прочитаны). Пустой
// кеш при живом сбое Базы знаний вернётся пустым списком — это лучше, чем
// отказ всей стороны.
func (s *Server) systemCardsForCombat(ctx context.Context, materials []Material) []SystemCard {
	cards, err := s.loadCachedSystemCards(ctx)
	if err != nil {
		return nil
	}
	if len(cards) == 0 && s.kb != nil && s.kb.configured() {
		fresh, ferr := SystemsFromKB(ctx, s.kb, materials)
		if ferr != nil {
			return nil
		}
		_ = s.saveCachedSystemCards(ctx, fresh, "")
		return fresh
	}
	return cards
}

// handleBotCombatCompetitors — продукты конкурентов для выбора стороны.
//
// Источник зависит от того, есть ли запрос (2026-11-12):
//
//   - **с запросом** — полнотекстовый поиск Базы знаний (`GET /api/kb/search`)
//     с тегом `competitor`, тот же механизм, что в веб-версии БЗ. Раньше поиск
//     шёл подстрокой по названию из кеша, и живая жалоба была честной:
//     «ПВХ мембрана» не находило «Кровельная ПВХ мембрана FACHMANN …» (другой
//     порядок слов) и не находило `Sikaplan®` (в названии нет слова «мембрана»),
//     а «PIR вент» — `PIRRO®VentiDuct`;
//   - **без запроса** — кеш конкурентов (competitors.go): тег `competitor` и всё
//     поддерево папки «материалы и системы конкурентов». Так «▼ Все продукты»
//     показывает тот же список, что и раньше, и не зависит от поиска.
//
// Если поиск недоступен (БЗ выключена или ответила ошибкой), отвечаем именем
// поля `error` и запасным путём по кешу — менеджер не остаётся без выбора.
func (s *Server) handleBotCombatCompetitors(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	offset, limit := parseCombatPage(r)
	if strings.TrimSpace(query) != "" {
		if items, err := s.searchCompetitors(r.Context(), query); err == nil {
			writeCombatPage(w, paginateCombatItems(items, offset, limit), map[string]any{
				"folderFound": true,
				"searched":    true,
			})
			return
		} else {
			log.Printf("боевой листок: поиск конкурентов в Базе знаний не удался (%v) — ищем по кешу", err)
		}
	}
	cards, found, err := s.loadCachedCompetitors(r.Context())
	if err == nil && len(cards) == 0 && s.kb != nil && s.kb.configured() {
		cards, found, err = s.refreshCompetitorsCache(r.Context(), "")
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"cards":       []any{},
			"folderFound": found,
			"error":       err.Error(),
		})
		return
	}
	items := combatItems(cards)
	filtered := filterCombatItems(items, query)
	writeCombatPage(w, paginateCombatItems(filtered, offset, limit), map[string]any{
		"folderFound": found,
	})
}

// searchCompetitors — поиск конкурентов по Базе знаний с тегом `competitor`.
//
// Ограничение выдачи — потолок страницы сервиса (`combatSearchLimit`): бот
// листает СВОИМИ страницами по уже собранному списку, поэтому за один запрос
// берём столько, сколько осмысленно показать поиском. Больше — через сужение
// запроса, а не через бесконечное листание.
func (s *Server) searchCompetitors(ctx context.Context, query string) ([]botSystemItem, error) {
	if s.kb == nil || !s.kb.configured() {
		return nil, errors.New("база знаний не настроена")
	}
	tagID, err := s.kb.TagID(ctx, competitorTagName)
	if err != nil {
		return nil, err
	}
	notes, err := s.kb.Search(ctx, query, tagID, combatSearchLimit)
	if err != nil {
		return nil, err
	}
	// Журнал индексации помечен тегом `journal`, а не `competitor`, поэтому в
	// выдачу не попадает. Заголовок карточки — из заметки: у конкурентов первая
	// строка H1 — предупреждение, а не название продукта.
	items := make([]botSystemItem, 0, len(notes))
	seen := make(map[int64]bool, len(notes))
	for _, summary := range notes {
		if summary.ID <= 0 || seen[summary.ID] {
			continue
		}
		seen[summary.ID] = true
		items = append(items, botSystemItem{NoteID: summary.ID, Title: competitorSearchTitle(summary)})
	}
	return items, nil
}

// competitorSearchTitle — название карточки по строке списка.
//
// kb/search отдаёт заголовок заметки. У конкурентов заголовок — название
// продукта (так их и заводит индексатор), поэтому обычно этого достаточно. Если
// сервис вернул дисклеймер вместо названия, берём имя из пути заметки: он
// всегда оканчивается именем файла, а имя файла у конкурентов — название.
func competitorSearchTitle(summary KBNoteSummary) string {
	title := strings.TrimSpace(summary.Title)
	if title != "" && !isCompetitorDisclaimer(title) {
		return title
	}
	if notePath := strings.TrimSpace(summary.Path); notePath != "" {
		if base := strings.TrimSuffix(path.Base(notePath), ".md"); strings.TrimSpace(base) != "" {
			return base
		}
	}
	return title
}

// combatSearchLimit — сколько конкурентов берёт поиск Базы знаний за раз.
// Полсотни — потолок страницы kb-service; больше одного запроса на показ списка
// не нужно, а бот листает уже собранное.
const combatSearchLimit = 50

// handleBotListCombat — «Боевые листки» в боте: видны ВСЕ (2026-11-12).
//
// Раньше список был «мой»: только листки привязанного email. Просьба
// пользователя — сделать листки доступными всем в боте: это внутренняя
// справка отдела, и менеджеру нужно видеть, что уже сравнивали коллеги. Правка
// при этом остаётся у автора (`botCanEditCombat`): чужой черновик не должен
// испортиться от нажатия «добавить конкурента».
func (s *Server) handleBotListCombat(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.emailForChat(w, r, r.URL.Query().Get("chat_id")); !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(),
		`SELECT id FROM offers_combat_sheet
		 ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		dbError(w, "bot list combat", err)
		return
	}
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			dbError(w, "bot list combat scan", err)
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
		// Подпись для списка считает сервер: в боте и в плагине она должна быть
		// одна, а не собираться в двух местах.
		if strings.TrimSpace(c.DisplayName) == "" {
			c.DisplayName = combatListLabel(c)
		}
		sheets = append(sheets, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"sheets": sheets})
}

// combatListLabel — как листок называется в списке: по продукту, а не служебным
// словом «Боевой листок» (2026-11-12, просьба пользователя).
//
// Порядок: наш продукт → первый конкурент → «пустой листок №». Наш продукт
// первым, потому что листок защищает именно его; конкурент — потому что в
// пустом сравнивать ещё нечего, а имя всё-таки нужно.
func combatListLabel(sheet CombatSheet) string {
	if title := firstNoteTitle(sheet.OurNotes); title != "" {
		return title
	}
	if title := firstNoteTitle(sheet.CompetitorNotes); title != "" {
		return title
	}
	if num := strings.TrimSpace(sheet.Number); num != "" {
		return "пустой листок № " + num
	}
	return "пустой боевой листок"
}

// firstNoteTitle — название первой стороны списка.
func firstNoteTitle(refs []CombatNoteRef) string {
	for _, ref := range refs {
		if title := strings.TrimSpace(ref.Title); title != "" {
			return title
		}
	}
	return ""
}

// botCanReadCombat — читать и печатать листок в боте может любой вошедший:
// доступ к боту даёт ключ (проверяется один раз в routes.ts).
func botCanReadCombat(_ CombatSheet, _ string) bool {
	return true
}

// botCanEditCombat — править листок может только его автор.
//
// Это не про секретность (читают все), а про сохранность: черновик коллеги
// меняется его кнопками, и чужое нажатие «добавить конкурента» переписало бы
// работу. Админ в боте роли не имеет — правило одно для всех.
func botCanEditCombat(sheet CombatSheet, email string) bool {
	return strings.EqualFold(strings.TrimSpace(sheet.OwnerEmail), strings.TrimSpace(email))
}

// botCombatForChat — листок по id для чтения в боте.
func (s *Server) botCombatForChat(w http.ResponseWriter, r *http.Request, chatID string, id int64) (CombatSheet, bool) {
	return s.botCombatWith(w, r, chatID, id, "нет доступа к боевому листку",
		func(sheet CombatSheet, email string) bool { return botCanReadCombat(sheet, email) })
}

// botCombatForEdit — листок по id для ПРАВКИ: только автор.
//
// Отдельная проверка, а не флаг у `botCombatForChat`: читать можно всем, править
// нельзя, и одна функция с двумя режимами рано или поздно перепутала бы их.
func (s *Server) botCombatForEdit(w http.ResponseWriter, r *http.Request, chatID string, id int64) (CombatSheet, bool) {
	return s.botCombatWith(w, r, chatID, id, "это боевой листок коллеги — править может только его автор",
		func(sheet CombatSheet, email string) bool { return botCanEditCombat(sheet, email) })
}

// botCombatWith — общая часть обеих проверок: кто это, есть ли листок, и пускает
// ли его данная проверка.
//
// Две функции выше отличаются ровно одной строкой — текстом отказа и самой
// проверкой; остальное (привязка чата, чтение листка, 404) одно и то же, и
// копия расходилась бы с оригиналом при первой же правке.
func (s *Server) botCombatWith(w http.ResponseWriter, r *http.Request, chatID string, id int64,
	denied string, allow func(CombatSheet, string) bool) (CombatSheet, bool) {
	email, ok := s.emailForChat(w, r, chatID)
	if !ok {
		return CombatSheet{}, false
	}
	sheet, err := s.loadCombatSheet(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "боевой листок не найден"})
		return CombatSheet{}, false
	}
	if !allow(sheet, email) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": denied})
		return CombatSheet{}, false
	}
	return sheet, true
}

// botCombatRequest — тело создания листка из бота. Стороны пустые: их выбирают
// по одной следующими кнопками.
type botCombatRequest struct {
	ChatID string `json:"chat_id"`
	Title  string `json:"title"`
}

// handleBotCreateCombat — создать пустой боевой листок с номером.
func (s *Server) handleBotCreateCombat(w http.ResponseWriter, r *http.Request) {
	var req botCombatRequest
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	email, ok := s.emailForChat(w, r, req.ChatID)
	if !ok {
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "Боевой листок"
	}
	c := CombatSheet{Title: title, Status: "draft"}
	normalizeCombat(&c)
	cols, err := combatColumns(c)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}
	id, err := s.createCombat(r.Context(), email, c, cols)
	if err != nil {
		dbError(w, "bot insert combat", err)
		return
	}
	saved, err := s.loadCombatSheet(r.Context(), id)
	if err != nil {
		dbError(w, "bot reload combat", err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// botCombatSideRequest — добавить/убрать сторону: заметка и её подпись.
type botCombatSideRequest struct {
	ChatID string `json:"chat_id"`
	NoteID int64  `json:"noteId"`
	Title  string `json:"title"`
}

// handleBotCombatSide добавляет сторону (`add=true`) или убирает её.
//
// Та же ручка на два вида стороны: `side` из пути говорит, куда писать — в
// `ourNotes` или `competitorNotes`. Один разбор одного запроса вместо двух
// почти одинаковых обработчиков.
func (s *Server) handleBotCombatSide(w http.ResponseWriter, r *http.Request, side string, add bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	var req botCombatSideRequest
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.NoteID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "у карточки нет заметки Базы знаний"})
		return
	}
	email, ok := s.emailForChat(w, r, req.ChatID)
	if !ok {
		return
	}
	sheet, err := s.loadCombatSheet(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "боевой листок не найден"})
		return
	}
	if !botCanEditCombat(sheet, email) {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": "это боевой листок коллеги — править может только его автор",
		})
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
		dbError(w, "bot update combat", err)
		return
	}
	saved, err := s.loadCombatSheet(r.Context(), id)
	if err != nil {
		dbError(w, "bot reload combat", err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// botCombatHintRequest — уточнение менеджера для разбора.
type botCombatHintRequest struct {
	ChatID string `json:"chat_id"`
	Hint   string `json:"hint"`
}

// handleBotCombatHint — сохранить уточнение боевого листка (2026-11-11).
//
// Уточнение — единственная настройка листка: на чём сравнение должно сделать
// акцент. Хранится в `form_values["hint"]`; поля формы у листка больше нет, так
// что ключ занят только этим.
func (s *Server) handleBotCombatHint(w http.ResponseWriter, r *http.Request) {
	s.botCombatHintHandler(w, r)
}

func (s *Server) botCombatHintHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	var req botCombatHintRequest
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	email, ok := s.emailForChat(w, r, req.ChatID)
	if !ok {
		return
	}
	sheet, err := s.loadCombatSheet(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "боевой листок не найден"})
		return
	}
	if !botCanEditCombat(sheet, email) {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": "это боевой листок коллеги — править может только его автор",
		})
		return
	}
	if sheet.FormValues == nil {
		sheet.FormValues = map[string]string{}
	}
	// Пустое уточнение не сохраняется пустой строкой, а снимается совсем: иначе
	// листкок носил бы «» и в промпте появлялась бы пустая строка акцента.
	if hint := strings.TrimSpace(req.Hint); hint != "" {
		sheet.FormValues[combatHintField] = hint
	} else {
		delete(sheet.FormValues, combatHintField)
	}
	cols, err := combatColumns(sheet)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}
	if err := s.updateCombat(r.Context(), id, sheet, cols); err != nil {
		dbError(w, "bot combat hint", err)
		return
	}
	saved, err := s.loadCombatSheet(r.Context(), id)
	if err != nil {
		dbError(w, "bot combat hint reload", err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// combatSideOur / combatSideCompetitor — две стороны листка. Строки, а не
// перечисление: то же значение serves и маршруту, и подписи в ответе.
const (
	combatSideOur        = "our"
	combatSideCompetitor = "competitor"
)

// defaultCombatTitle — название нового листка без своего заголовка. То же слово
// шлёт бот (`createCombat` в `callbacks.ts`): одна подпись на оба входа.
const defaultCombatTitle = "Боевой листок"

// appendNoteRef добавляет заметку, если её ещё нет: повторное нажатие кнопки не
// должно ставить одну и ту же систему в листок дважды.
func appendNoteRef(refs []CombatNoteRef, ref CombatNoteRef) []CombatNoteRef {
	for _, r := range refs {
		if r.NoteID == ref.NoteID {
			return refs
		}
	}
	return append(refs, ref)
}

// removeNoteRef убирает заметку по id. Отсутствующей нет — список не меняется.
func removeNoteRef(refs []CombatNoteRef, noteID int64) []CombatNoteRef {
	out := make([]CombatNoteRef, 0, len(refs))
	for _, r := range refs {
		if r.NoteID != noteID {
			out = append(out, r)
		}
	}
	return out
}

// handleBotExportCombat — DOCX боевого листка в чат (та же печать, что в плагине).
func (s *Server) handleBotExportCombat(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	sheet, ok := s.botCombatForChat(w, r, r.URL.Query().Get("chat_id"), id)
	if !ok {
		return
	}
	// Содержание листка фиксировано, от формы нужен только макет (2026-11-11).
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
	writeJSON(w, http.StatusOK, map[string]any{
		"docx_base64": base64.StdEncoding.EncodeToString(data),
		"file_name":   docxFileNameFor("Боевой листок", sheet.Number),
	})
}
