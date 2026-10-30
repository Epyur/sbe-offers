package main

import (
	"context"
	"encoding/json"
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
		card := SystemCard{
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
func (c SystemCard) PromptText() string {
	var b strings.Builder
	if c.Title != "" {
		b.WriteString("Система: " + c.Title + "\n")
	}
	if c.Description != "" {
		b.WriteString("\nОписание:\n" + c.Description + "\n")
	}
	if len(c.Composition) > 0 {
		b.WriteString("\nСостав:\n")
		for _, item := range c.Composition {
			b.WriteString("- " + item + "\n")
		}
	}
	return strings.TrimSpace(b.String())
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
