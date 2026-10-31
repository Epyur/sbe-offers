package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"mailers/offers/skills"
)

// Карточки систем из Базы знаний (шаг 7c плана 2026-10-29).
//
// Источник правды о тексте системы — заметка в базе знаний. Здесь:
//   - чтение карточек через kbclient.go,
//   - связь с каталогом по system_sku,
//   - кеш в offers_context, чтобы плагин получал список без похода в базу
//     знаний на каждый экран.
//
// Кеш НЕ источник правды: его всегда можно сбросить, и без базы знаний КП
// собирается как раньше — каталог и форма работают.

// --- Ручки для плагина -------------------------------------------------

// handleGetSystemCards — список карточек для раздела «Контекст КП».
//
// Отдаёт КЕШ, а не живой поход в базу знаний: экран плагина открывается часто,
// а карточки меняются редко. Обновление — отдельной ручкой.
func (s *Server) handleGetSystemCards(w http.ResponseWriter, r *http.Request) {
	cards, err := s.loadCachedSystemCards(r.Context())
	if err != nil {
		dbError(w, "get system cards", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cards":     cards,
		"kbEnabled": s.kb != nil && s.kb.configured(),
	})
}

// handleRefreshSystemCards — перечитать карточки из базы знаний (admin).
//
// Ошибка базы знаний возвращается полем `error`, но статус 200 и прежние
// карточки: плагин покажет предупреждение, а раздел не станет пустым.
func (s *Server) handleRefreshSystemCards(w http.ResponseWriter, r *http.Request) {
	email := actorEmailFromCtx(r)
	cards, err := s.refreshSystemCards(r.Context(), email)
	resp := map[string]any{
		"cards":     cards,
		"kbEnabled": s.kb != nil && s.kb.configured(),
		"count":     len(cards),
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleListSystems — системы из Базы знаний с составом (раздел «Системы»).
//
// Читает БАЗУ ЗНАНИЙ напрямую, а не кеш карточек: состав нужно разрешать против
// ТЕКУЩЕГО справочника материалов, а кеш держит имена карточек для «Контекста
// КП». Ошибка базы знаний — не пустой экран: отдаём пустой список и `error`,
// чтобы плагин написал, что именно не так.
func (s *Server) handleListSystems(w http.ResponseWriter, r *http.Request) {
	materials, err := s.listMaterials(r.Context(), false)
	if err != nil {
		dbError(w, "list systems: materials", err)
		return
	}
	cards, err := SystemsFromKB(r.Context(), s.kb, materials)
	resp := map[string]any{
		"systems":   cards,
		"kbEnabled": s.kb != nil && s.kb.configured(),
		"count":     len(cards),
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

// SystemCard — карточка системы, как её видит offers-service.
type SystemCard struct {
	// NoteID — заметка в базе знаний. По ней открывается карточка в плагине.
	NoteID int64 `json:"noteId"`
	// Path — путь заметки в вольте: плагин показывает его человеку.
	Path string `json:"path"`
	// SKU — код системы из frontmatter; по нему связь с каталогом.
	SKU string `json:"sku"`
	// Title — название системы.
	Title string `json:"title"`
	// Description — описание системы.
	Description string `json:"description"`
	// Composition — состав (позиции без цен: цены в каталоге).
	Composition []string `json:"composition"`
	// CompositionLinks — тот же состав, разрешённый против справочника
	// материалов: строка, код из неё, найденный материал. Строка, которой нет
	// в справочнике, остаётся с Found=false и попадает в Problems.
	CompositionLinks []CompositionItem `json:"compositionLinks,omitempty"`
	// Problems — что не так с составом. Показывается человеку явно: молча
	// терять строку состава нельзя.
	Problems []string `json:"problems,omitempty"`
	// Specifications — таблица характеристик.
	Specifications [][]string `json:"specifications,omitempty"`
	// ImageName — имя файла картинки в заметке.
	ImageName string `json:"imageName,omitempty"`
	// ImageKey — ключ вложения в хранилище Базы знаний. По нему картинка
	// читается для вставки в DOCX (FileLink отдаёт ссылку, а нам нужны байты).
	ImageKey string `json:"imageKey,omitempty"`
	// CatalogID — система каталога, с которой связана карточка. 0 — не связана.
	CatalogID int64 `json:"catalogId"`
	// UpdatedAt — когда заметку правили.
	UpdatedAt string `json:"updatedAt"`
	// HasDescription / HasComposition — есть ли обязательные блоки стандарта.
	HasDescription bool `json:"hasDescription"`
	HasComposition bool `json:"hasComposition"`
	// Empty — карточка без содержания.
	Empty bool `json:"empty"`
}

// CompositionItem — строка состава с разрешённым материалом.
//
// Raw — строка, как её написал человек. SKU — код из строки (пусто, если
// ссылку не проставили). Found — материал нашёлся в справочнике; тогда
// заполнены MaterialID, Name, Unit и Ekn. Ekn нужен, чтобы плагин мог собрать
// позицию по конкретному варианту, не делая ещё один запрос.
type CompositionItem struct {
	Raw        string   `json:"raw"`
	SKU        string   `json:"sku,omitempty"`
	MaterialID int64    `json:"materialId"`
	Name       string   `json:"name,omitempty"`
	Unit       string   `json:"unit,omitempty"`
	Ekn        []string `json:"ekn,omitempty"`
	Found      bool     `json:"found"`
}

// resolveComposition разрешает строки состава против справочника материалов.
//
// Проблема — не ошибка: система без единого материала тоже законна (работы
// без кода), но человек должен знать, что именно в ней не связано. Поэтому
// каждая несвязанная строка называется отдельно, а пустой состав — одно
// общее замечание.
func resolveComposition(card skills.SystemCard, materials []Material) ([]CompositionItem, []string) {
	links := card.Links
	// Старый кеш карточки мог не нести Links: тогда разбираем строки сами.
	if len(links) == 0 && len(card.Composition) > 0 {
		links = make([]skills.CompositionLink, 0, len(card.Composition))
		for _, raw := range card.Composition {
			links = append(links, skills.CompositionLink{Raw: raw, SKU: skills.ExtractSku(raw)})
		}
	}
	if len(links) == 0 {
		return []CompositionItem{}, []string{"состав системы не заполнен"}
	}

	// Архивные материалы тоже в карте: ссылка на снятый с продажи код —
	// отдельная причина, и она понятнее, чем «не найден».
	bySku := make(map[string]Material, len(materials))
	for _, m := range materials {
		bySku[strings.ToUpper(strings.TrimSpace(m.SKU))] = m
	}

	items := make([]CompositionItem, 0, len(links))
	problems := make([]string, 0)
	for _, link := range links {
		item := CompositionItem{Raw: link.Raw, SKU: link.SKU}
		sku := strings.ToUpper(strings.TrimSpace(link.SKU))
		if sku == "" {
			items = append(items, item)
			problems = append(problems, fmt.Sprintf("%q: не указан код материала", shortenRaw(link.Raw)))
			continue
		}
		m, ok := bySku[sku]
		if !ok {
			items = append(items, item)
			problems = append(problems, fmt.Sprintf("%s: материал не найден в справочнике", sku))
			continue
		}
		if m.Archived {
			items = append(items, item)
			problems = append(problems, fmt.Sprintf("%s: материал удалён из справочника", sku))
			continue
		}
		item.MaterialID, item.Name, item.Unit, item.Found = m.ID, m.Name, m.Unit, true
		item.Ekn = materialEknNumbers(m)
		items = append(items, item)
	}
	return items, problems
}

// materialEknNumbers — номера ЕКН материала в порядке справочника.
func materialEknNumbers(m Material) []string {
	if len(m.Ekn) == 0 {
		return nil
	}
	out := make([]string, 0, len(m.Ekn))
	for _, e := range m.Ekn {
		out = append(out, e.Ekn)
	}
	return out
}

// shortenRaw обрезает длинную строку состава для текста проблемы: человеку
// нужен узнаваемый кусок, а не вся строка в списке замечаний.
func shortenRaw(raw string) string {
	runes := []rune(strings.TrimSpace(raw))
	if len(runes) <= 60 {
		return string(runes)
	}
	return string(runes[:60]) + "…"
}

// SystemsFromKB — системы из Базы знаний с составом, разрешённым против
// материалов. Материальные карточки в список не попадают: у них свой раздел.
//
// kb == nil — «база знаний не настроена»: пустой список без ошибки.
func SystemsFromKB(ctx context.Context, kb KBReader, materials []Material) ([]SystemCard, error) {
	if kb == nil {
		return []SystemCard{}, nil
	}
	tagID, err := kb.TagID(ctx, commercialPropose)
	if err != nil {
		return []SystemCard{}, err
	}
	notes, err := kb.Notes(ctx, tagID)
	if err != nil {
		return []SystemCard{}, err
	}
	systems, _ := splitCardsByTags(notes)
	cards := make([]SystemCard, 0, len(systems))
	for _, summary := range systems {
		note, err := kb.Note(ctx, summary.ID)
		// Одна сломанная карточка не скрывает остальные.
		if err != nil {
			continue
		}
		parsed := skills.ParseSystemCard(note.ContentMD)
		card := systemCardFromParsed(summary, note, parsed)
		card.CompositionLinks, card.Problems = resolveComposition(parsed, materials)
		cards = append(cards, card)
	}
	return cards, nil
}

// systemCardFromParsed собирает карточку для плагина из разобранной заметки.
func systemCardFromParsed(summary KBNoteSummary, note KBNote, parsed skills.SystemCard) SystemCard {
	return SystemCard{
		NoteID:         summary.ID,
		Path:           note.Path,
		SKU:            parsed.SKU,
		Title:          parsed.Title,
		Description:    parsed.Description,
		Composition:    parsed.Composition,
		Specifications: parsed.Specifications,
		ImageName:      parsed.ImageName,
		UpdatedAt:      note.UpdatedAt,
		HasDescription: strings.TrimSpace(parsed.Description) != "",
		HasComposition: len(parsed.Composition) > 0,
		Empty:          parsed.Empty(),
	}
}

// KBReader — то, что нужно от базы знаний. Интерфейс, а не *KBClient: тесты
// подставляют свою реализацию и не ходят по сети.
type KBReader interface {
	TagID(ctx context.Context, name string) (int64, error)
	Notes(ctx context.Context, tagID int64) ([]KBNoteSummary, error)
	Note(ctx context.Context, id int64) (KBNote, error)
	Attachments(ctx context.Context, noteID int64) ([]KBAttachment, error)
	FileLink(ctx context.Context, key string) (string, error)
}

// SystemCardsFromKB читает карточки и связывает их с каталогом по system_sku.
//
// kb == nil означает «база знаний не настроена»: пустой список без ошибки.
// Ошибка базы знаний возвращается наверх вместе с пустым списком — вызывающий
// решает, показать предупреждение или работать без карточек.
func SystemCardsFromKB(ctx context.Context, kb KBReader, catalog []CatalogItem) ([]SystemCard, error) {
	if kb == nil {
		return []SystemCard{}, nil
	}
	tagID, err := kb.TagID(ctx, commercialPropose)
	if err != nil {
		return []SystemCard{}, err
	}
	notes, err := kb.Notes(ctx, tagID)
	if err != nil {
		return []SystemCard{}, err
	}
	cards := make([]SystemCard, 0, len(notes))
	for _, summary := range notes {
		note, err := kb.Note(ctx, summary.ID)
		// Одна сломанная карточка не должна скрывать остальные.
		if err != nil {
			continue
		}
		parsed := skills.ParseSystemCard(note.ContentMD)
		card := systemCardFromParsed(summary, note, parsed)
		card.CatalogID = linkCatalogID(card.SKU, catalog)
		cards = append(cards, card)
	}
	return cards, nil
}

// linkCatalogID ищет систему каталога по коду. Только по коду, не по имени:
// имя правится свободно, а код заводится один раз и не меняется.
func linkCatalogID(sku string, catalog []CatalogItem) int64 {
	sku = strings.TrimSpace(sku)
	if sku == "" {
		return 0
	}
	for _, item := range catalog {
		if strings.EqualFold(strings.TrimSpace(item.SKU), sku) {
			return item.ID
		}
	}
	return 0
}

// FindSystemCard — карточка по коду системы (для сборки документа).
func FindSystemCard(cards []SystemCard, sku string) (SystemCard, bool) {
	sku = strings.TrimSpace(sku)
	if sku == "" {
		return SystemCard{}, false
	}
	for _, c := range cards {
		if strings.EqualFold(strings.TrimSpace(c.SKU), sku) {
			return c, true
		}
	}
	return SystemCard{}, false
}

// --- Кеш карточек в offers_context -------------------------------------

// loadCachedSystemCards читает карточки из кеша. Нет строки — пустой список.
func (s *Server) loadCachedSystemCards(ctx context.Context) ([]SystemCard, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx,
		`SELECT sections FROM offers_context WHERE mode = 'system_cards'`).Scan(&raw)
	if err != nil {
		return []SystemCard{}, nil
	}
	return unmarshalSystemCards(raw)
}

// saveCachedSystemCards пишет карточки в кеш. Строка одна (mode =
// 'system_cards'): это кеш, а не набор записей.
func (s *Server) saveCachedSystemCards(ctx context.Context, cards []SystemCard, updatedBy string) error {
	raw, err := marshalSystemCards(cards)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO offers_context (mode, sections, learnings, updated_by, updated_at)
VALUES ('system_cards', $1, '[]', $2, now())
ON CONFLICT (mode) DO UPDATE SET
	sections = EXCLUDED.sections, updated_by = EXCLUDED.updated_by, updated_at = now()`,
		raw, updatedBy)
	return err
}

// refreshSystemCards читает карточки из базы знаний и обновляет кеш.
//
// Ошибка базы знаний НЕ стирает прежний кеш: временный сбой не должен
// выключать описания во всех КП. Возвращается прежний кеш и ошибка — вызывающий
// сам решает, показать предупреждение или молчать.
func (s *Server) refreshSystemCards(ctx context.Context, updatedBy string) ([]SystemCard, error) {
	catalog, _ := s.listCatalogItems(ctx, true)
	fresh, err := SystemCardsFromKB(ctx, s.kb, catalog)
	if err != nil {
		cached, _ := s.loadCachedSystemCards(ctx)
		return cached, err
	}
	if err := s.saveCachedSystemCards(ctx, fresh, updatedBy); err != nil {
		return fresh, err
	}
	return fresh, nil
}

// PromptText — текст карточки для промпта модели: название, описание, состав.
// Таблица характеристик сюда не идёт: модели нужно то, о чём писать.
//
// Формат текста живёт в skills — там же, где разбор карточки. Сервисная копия
// дублировала его символ в символ: правка в одном месте молча оставляла бы
// вторую старую, и промпт собирался бы из двух разных шаблонов.
func (c SystemCard) PromptText() string {
	return skills.SystemCard{
		Title:       c.Title,
		Description: c.Description,
		Composition: c.Composition,
	}.PromptText()
}

// marshalSystemCards — кеш в JSONB.
func marshalSystemCards(cards []SystemCard) ([]byte, error) {
	if cards == nil {
		cards = []SystemCard{}
	}
	return json.Marshal(cards)
}

// unmarshalSystemCards читает кеш. Старый формат (ProductContext с разделами)
// не ломает чтение: карточек в нём нет, и это нормально.
func unmarshalSystemCards(raw []byte) ([]SystemCard, error) {
	if len(raw) == 0 {
		return []SystemCard{}, nil
	}
	var cards []SystemCard
	if err := json.Unmarshal(raw, &cards); err == nil && cards != nil {
		return cards, nil
	}
	return []SystemCard{}, nil
}
