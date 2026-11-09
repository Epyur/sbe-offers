package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"mailers/offers/skills"
)

// Материалы и системы конкурентов (2026-11-08, «Боевой листок»).
//
// Источник правды о продукте конкурента — заметки Базы знаний в разделе
// «материалы и системы конкурентов». Отдельной таблицы у конкурентов НЕТ
// намеренно: это те же карточки, что и наши системы, и разбирать их второй раз
// своим разбором значило бы держать две правды об одном формате заметки.
// Поэтому карточка конкурента — это `SystemCard`, полученная тем же
// `skills.ParseSystemCard`.
//
// Кеш — как у систем: экран открывается часто, папка меняется редко.
// Перечитывание — явной кнопкой (admin), а не на каждый показ.

// competitorsFolderName — папка Базы знаний с материалами конкурентов.
//
// Одно место на всю систему: имя папки набирает человек в Базе знаний, и если
// оно разойдётся по коду, раздел молча покажет пустоту.
const competitorsFolderName = "материалы и системы конкурентов"

// competitorsCacheMode — ключ строки кеша конкурентов в offers_context.
// Отдельно от system_cards и systems: это третий, самостоятельный набор.
const competitorsCacheMode = "competitors"

// CompetitorCard — карточка продукта конкурента.
//
// Совпадает по форме с SystemCard: та же заметка, тот же разбор. Наследуется,
// а не копируется, чтобы новый признак карточки не появлялся в одном месте и
// отсутствовал в другом.
type CompetitorCard struct {
	SystemCard
}

// handleListCompetitors — карточки конкурентов из кеша (раздел и выбор в
// боевом листке). Пустой кеш читается из Базы знаний один раз; папки нет —
// `folderFound:false` без ошибки: боевой листок всё равно собирается.
func (s *Server) handleListCompetitors(w http.ResponseWriter, r *http.Request) {
	cards, found, err := s.loadCachedCompetitors(r.Context())
	if err == nil && len(cards) == 0 && s.kb != nil && s.kb.configured() {
		cards, found, err = s.refreshCompetitorsCache(r.Context(), "")
	}
	if err != nil {
		log.Printf("конкуренты: кеш не прочитан: %v", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cards":       cards,
		"kbEnabled":   s.kb != nil && s.kb.configured(),
		"folderFound": found,
		"error":       errorText(err),
	})
}

// handleRefreshCompetitors — перечитать папку конкурентов (admin).
//
// Ошибка Базы знаний возвращается полем `error`, но статус 200 и прежний кеш:
// раздел покажет предупреждение, а не станет пустым.
func (s *Server) handleRefreshCompetitors(w http.ResponseWriter, r *http.Request) {
	cards, found, err := s.refreshCompetitorsCache(r.Context(), actorEmailFromCtx(r))
	writeJSON(w, http.StatusOK, map[string]any{
		"cards":       cards,
		"kbEnabled":   s.kb != nil && s.kb.configured(),
		"folderFound": found,
		"count":       len(cards),
		"error":       errorText(err),
	})
}

// CompetitorsFromKB читает карточки конкурентов из папки. Возвращает карточки и
// признак «папка найдена»: отсутствие папки — не ошибка, её может ещё не быть.
func CompetitorsFromKB(ctx context.Context, kb *KBClient) ([]SystemCard, bool, error) {
	if kb == nil || !kb.configured() {
		return []SystemCard{}, false, nil
	}
	folderID, err := kb.FolderIDByName(ctx, competitorsFolderName)
	if err != nil {
		// Нет папки — «не найдено», а не авария. Прочие ошибки (сеть, права)
		// идут наверх: пустая папка и недоступная база знаний — разные вещи.
		return []SystemCard{}, false, nil
	}
	notes, err := kb.FolderNotes(ctx, folderID)
	if err != nil {
		return []SystemCard{}, true, err
	}
	cards := make([]SystemCard, 0, len(notes))
	for _, summary := range notes {
		note, err := kb.Note(ctx, summary.ID)
		// Одна сломанная карточка не скрывает остальные.
		if err != nil {
			continue
		}
		parsed := skills.ParseSystemCard(note.ContentMD)
		cards = append(cards, systemCardFromParsed(summary, note, parsed))
	}
	return attachImageKeys(ctx, kb, cards), true, nil
}

// loadCachedCompetitors читает карточки конкурентов из кеша и признак «папка
// была найдена». Нет строки — пустой список и found=false.
func (s *Server) loadCachedCompetitors(ctx context.Context) ([]SystemCard, bool, error) {
	var raw []byte
	if err := s.pool.QueryRow(ctx,
		`SELECT sections FROM offers_context WHERE mode = $1`, competitorsCacheMode).Scan(&raw); err != nil {
		return []SystemCard{}, false, nil
	}
	// Формат кеша: {found, cards}. Старый кеш (просто массив) тоже читаем —
	// иначе первое обновление после выпуска потеряло бы карточки.
	var payload struct {
		Found bool         `json:"found"`
		Cards []SystemCard `json:"cards"`
	}
	if err := json.Unmarshal(raw, &payload); err == nil && payload.Cards != nil {
		return payload.Cards, payload.Found, nil
	}
	var cards []SystemCard
	if err := json.Unmarshal(raw, &cards); err != nil {
		return nil, false, fmt.Errorf("кеш конкурентов повреждён: %w", err)
	}
	if cards == nil {
		cards = []SystemCard{}
	}
	return cards, len(cards) > 0, nil
}

// saveCachedCompetitors пишет карточки конкурентов и признак «папка найдена».
func (s *Server) saveCachedCompetitors(ctx context.Context, cards []SystemCard, found bool, updatedBy string) error {
	if cards == nil {
		cards = []SystemCard{}
	}
	raw, err := json.Marshal(struct {
		Found bool         `json:"found"`
		Cards []SystemCard `json:"cards"`
	}{Found: found, Cards: cards})
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO offers_context (mode, sections, learnings, updated_by, updated_at)
VALUES ($1, $2, '[]', $3, now())
ON CONFLICT (mode) DO UPDATE SET
	sections = EXCLUDED.sections, updated_by = EXCLUDED.updated_by, updated_at = now()`,
		competitorsCacheMode, raw, updatedBy)
	return err
}

// refreshCompetitorsCache читает папку конкурентов и обновляет кеш.
//
// Ошибка Базы знаний НЕ стирает прежний кеш: временный сбой не должен
// выключать раздел. Возвращаются прежний кеш, прежний признак и ошибка.
func (s *Server) refreshCompetitorsCache(ctx context.Context, updatedBy string) ([]SystemCard, bool, error) {
	fresh, found, err := CompetitorsFromKB(ctx, s.kb)
	if err != nil {
		cards, wasFound, _ := s.loadCachedCompetitors(ctx)
		return cards, wasFound, err
	}
	if err := s.saveCachedCompetitors(ctx, fresh, found, updatedBy); err != nil {
		return fresh, found, err
	}
	return fresh, found, nil
}
