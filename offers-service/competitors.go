package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

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

// competitorTagName — тег, которым менеджеры помечают карточки конкурентов
// (2026-11-10). Второй источник рядом с папкой, а не замена ей: тег надёжнее —
// карточку легко положить мимо раздела, а тег виден и в поиске Базы знаний.
// Обе выборки сливаются по id заметки, поэтому заметка, помеченная и тегом, и
// лежащая в папке, попадает в список один раз.
const competitorTagName = "competitor"

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
// боевом листке). Пустой кеш читается из Базы знаний один раз; источников нет
// (ни тега, ни папки) — `folderFound:false` без ошибки: боевой листок всё равно
// собирается.
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

// CompetitorsFromKB читает карточки конкурентов из ДВУХ источников и сливает
// их по идентификатору заметки (2026-11-10):
//
//  1. тег `competitor` — карточки, которые менеджеры пометили вручную;
//  2. папка «материалы и системы конкурентов» и ВСЕ её подпапки — исторический
//     источник, добавлен 2026-11-08/09.
//
// Второй источник оставлен, чтобы ничего не пропало: заметки, положенные в
// раздел до появления тега, продолжают работать. Заметка, помеченная и тегом,
// и лежащая в папке, попадает в список ровно один раз (слияние по id).
//
// Возвращает карточки и признак «источник найден»: тег без карточек — это
// найденный источник с пустым результатом, отсутствие и тега, и папки — не
// найден. Отсутствие источников — не ошибка, раздел может быть ещё не заведён.
func CompetitorsFromKB(ctx context.Context, kb *KBClient) ([]SystemCard, bool, error) {
	if kb == nil || !kb.configured() {
		return []SystemCard{}, false, nil
	}
	cards := make([]SystemCard, 0)
	seen := make(map[int64]bool)
	found := false

	// 1. По тегу. Ошибка «тега нет» — не авария: тег заводит менеджер, и до
	// первого проставления его в базе может не быть вовсе.
	if tagged, ok, err := competitorTaggedNotes(ctx, kb); err != nil {
		return []SystemCard{}, found, err
	} else if ok {
		found = true
		cards = append(cards, cardsForSummaries(ctx, kb, tagged, seen)...)
	}

	// 2. Из папки и её подпапок. Нет папки — «не найдено», прочие ошибки (сеть,
	// права) идут наверх: пустая папка и недоступная база знаний — разные вещи.
	rootID, err := kb.FolderIDByName(ctx, competitorsFolderName)
	if err == nil {
		found = true
		folderIDs, ferr := kb.SubtreeFolderIDs(ctx, rootID)
		if ferr != nil {
			return cards, true, ferr
		}
		for _, folderID := range folderIDs {
			notes, nerr := kb.FolderNotes(ctx, folderID)
			if nerr != nil {
				// Одна недоступная папка не скрывает остальные ветки раздела.
				continue
			}
			cards = append(cards, cardsForSummaries(ctx, kb, notes, seen)...)
		}
	}

	return attachImageKeys(ctx, kb, cards), found, nil
}

// competitorTaggedNotes — заметки с тегом `competitor`.
//
// Второе значение — «тег существует»: у kb-service отсутствие тега —
// ошибка (`TagID`), а пустой тег и отсутствующий — разные вещи. Ошибку
// отсутствующего тега гасим здесь, прочие (сеть, права) отдаём наверх.
func competitorTaggedNotes(ctx context.Context, kb *KBClient) ([]KBNoteSummary, bool, error) {
	tagID, err := kb.TagID(ctx, competitorTagName)
	if err != nil {
		return nil, false, nil
	}
	notes, err := kb.Notes(ctx, tagID)
	if err != nil {
		return nil, true, err
	}
	return notes, true, nil
}

// cardsForSummaries читает тела заметок и собирает карточки, пропуская уже
// виденные (слияние источников по id заметки).
func cardsForSummaries(ctx context.Context, kb *KBClient, notes []KBNoteSummary, seen map[int64]bool) []SystemCard {
	out := make([]SystemCard, 0, len(notes))
	for _, summary := range notes {
		if seen[summary.ID] {
			continue
		}
		seen[summary.ID] = true
		note, err := kb.Note(ctx, summary.ID)
		// Одна сломанная карточка не скрывает остальные.
		if err != nil {
			continue
		}
		parsed := skills.ParseSystemCard(note.ContentMD)
		out = append(out, competitorCardFromParsed(summary, note, parsed))
	}
	return out
}

// competitorCardFromParsed собирает карточку конкурента из заметки.
//
// Отличие от систем: НАЗВАНИЕ всегда берётся из заголовка заметки, а не из
// тела (2026-11-10). Карточки конкурентов — произвольные заметки, а не карточки
// по стандарту; разбор тела подменял название первой содержательной строкой и в
// списке появлялись чужие фразы вместо названия продукта. Тело по-прежнему даёт
// описание, состав и ссылки — только не название.
func competitorCardFromParsed(summary KBNoteSummary, note KBNote, parsed skills.SystemCard) SystemCard {
	card := systemCardFromParsed(summary, note, parsed)
	if title := strings.TrimSpace(summary.Title); title != "" {
		card.Title = title
	}
	return card
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
