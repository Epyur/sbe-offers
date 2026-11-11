package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
)

// Поиск систем и материалов для окна выбора в КП (2026-11-12).
//
// Живые жалобы: в КП083/КП013 человек ввёл в бриф «Гарант» или «Кровля», а бот
// не предложил ни одной системы; в плагине системы и материалы выбирались
// дропдауном на сотни строк. Причина одна: выбор шёл по подстроке в кэше, а не
// поиском самой Базы знаний.
//
// Здесь — тот же полнотекстовый поиск, что у конкурентов (2026-11-12,
// `GET /api/kb/search`: русская морфология, название весит выше текста), только
// по двум другим тегам: `commercial_propose` без `material` — системы,
// `material` — материалы. Правило одно на плагин и на бота; клиенты зовут эти
// ручки, а не фильтруют кэш сами.

// pickerSearchLimit — сколько карточек отдаёт поиск выбора.
//
// Окно выбора показывает найденное списком; сотни строк оно всё равно не
// нарисует, а лишние строки удлиняют ответ Базы знаний.
const pickerSearchLimit = 60

// pickerSystem — карточка системы в ответе поиска.
type pickerSystem struct {
	NoteID   int64    `json:"noteId"`
	SKU      string   `json:"sku,omitempty"`
	Title    string   `json:"title"`
	Problems []string `json:"problems,omitempty"`
}

// pickerMaterial — карточка материала в ответе поиска.
//
// Поля те же, что нужны позиции КП: имя, единица, код и цена; `materialId`
// связывает с справочником, `ekn` — номер, который печатает документ.
type pickerMaterial struct {
	NoteID     int64    `json:"noteId"`
	MaterialID int64    `json:"materialId"`
	SKU        string   `json:"sku,omitempty"`
	Name       string   `json:"name"`
	Unit       string   `json:"unit"`
	PriceMin   float64  `json:"priceMin"`
	PriceMax   float64  `json:"priceMax"`
	Ekn        []string `json:"ekn,omitempty"`
}

// handleSearchSystems — GET /systems/search?q=… (роль viewer).
func (s *Server) handleSearchSystems(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, http.StatusOK, map[string]any{"items": []pickerSystem{}, "total": 0})
		return
	}
	if s.kb == nil || !s.kb.configured() {
		writeJSON(w, http.StatusOK, map[string]any{
			"items": []pickerSystem{}, "total": 0, "kbEnabled": false,
			"error": "База знаний не настроена",
		})
		return
	}
	items, err := s.searchSystems(r.Context(), query)
	if err != nil {
		// Ошибка Базы знаний — не 500: окно выбора покажет предупреждение,
		// а уже выбранное в предложении останется на месте.
		writeJSON(w, http.StatusOK, map[string]any{
			"items": []pickerSystem{}, "total": 0, "kbEnabled": true,
			"error": errorText(err),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": len(items), "kbEnabled": true,
	})
}

// handleSearchMaterials — GET /materials/search?q=… (роль viewer).
func (s *Server) handleSearchMaterials(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, http.StatusOK, map[string]any{"items": []pickerMaterial{}, "total": 0})
		return
	}
	materials, err := s.listMaterials(r.Context(), false)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"items": []pickerMaterial{}, "total": 0, "error": errorText(err),
		})
		return
	}
	kbEnabled := s.kb != nil && s.kb.configured()
	if !kbEnabled {
		// Без Базы знаний остаётся справочник, заведённый руками: фильтр по
		// подстроке — не поиск, но другого источника нет. Так честнее, чем
		// пустое окно.
		writeJSON(w, http.StatusOK, map[string]any{
			"items": filterMaterialsByName(materials, query), "kbEnabled": false,
		})
		return
	}
	items, err := s.searchMaterials(r.Context(), query, materials)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"items": []pickerMaterial{}, "total": 0, "kbEnabled": true,
			"error": errorText(err),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": len(items), "kbEnabled": true,
	})
}

// searchSystems ищет системы по Базе знаний.
//
// Тег `commercial_propose` возвращает и системы, и материалы (материалы тоже
// помечены `commercial_propose`), поэтому найденное сверяется с тегом `material`
// по полной заметке — то же правило, что в `SystemsFromKB`.
func (s *Server) searchSystems(ctx context.Context, query string) ([]pickerSystem, error) {
	tagID, err := s.kb.TagID(ctx, commercialPropose)
	if err != nil {
		return nil, err
	}
	notes, err := s.kb.Search(ctx, query, tagID, pickerSearchLimit)
	if err != nil {
		return nil, err
	}
	items := make([]pickerSystem, 0, len(notes))
	seen := make(map[int64]bool, len(notes))
	for _, summary := range notes {
		if summary.ID <= 0 || seen[summary.ID] {
			continue
		}
		seen[summary.ID] = true
		note, nerr := s.kb.Note(ctx, summary.ID)
		if nerr != nil {
			// Одна сломанная карточка не скрывает остальные.
			continue
		}
		if noteHasTag(KBNoteSummary{Tags: note.Tags}, materialTagName) {
			continue
		}
		items = append(items, pickerSystem{
			NoteID: summary.ID,
			Title:  pickerTitle(summary),
		})
	}
	return items, nil
}

// searchMaterials ищет материалы по Базе знаний и связывает их со справочником.
//
// Карточка без материала в справочнике всё равно попадает в ответ: материал
// заведётся при следующем чтении списка (`syncMaterialsFromNotes`), а имя и
// единица известны уже сейчас.
func (s *Server) searchMaterials(ctx context.Context, query string, materials []Material) ([]pickerMaterial, error) {
	tagID, err := s.kb.TagID(ctx, materialTagName)
	if err != nil {
		return nil, err
	}
	notes, err := s.kb.Search(ctx, query, tagID, pickerSearchLimit)
	if err != nil {
		return nil, err
	}
	byNote := make(map[int64]Material, len(materials))
	for _, m := range materials {
		if m.NoteID > 0 {
			byNote[m.NoteID] = m
		}
	}
	items := make([]pickerMaterial, 0, len(notes))
	seen := make(map[int64]bool, len(notes))
	for _, summary := range notes {
		if summary.ID <= 0 || seen[summary.ID] {
			continue
		}
		seen[summary.ID] = true
		item := pickerMaterial{NoteID: summary.ID, Name: pickerTitle(summary), Unit: "шт"}
		if m, ok := byNote[summary.ID]; ok {
			item.MaterialID = m.ID
			item.SKU = m.SKU
			if strings.TrimSpace(m.Name) != "" {
				item.Name = m.Name
			}
			if strings.TrimSpace(m.Unit) != "" {
				item.Unit = m.Unit
			}
			if m.PriceMin != nil {
				item.PriceMin = *m.PriceMin
			}
			if m.PriceMax != nil {
				item.PriceMax = *m.PriceMax
			}
			item.Ekn = materialEknNumbers(m)
		}
		items = append(items, item)
	}
	return items, nil
}

// filterMaterialsByName — запасной путь без Базы знаний: слова запроса по
// названию и коду справочника.
func filterMaterialsByName(materials []Material, query string) []pickerMaterial {
	words := strings.Fields(normalizeCombatQuery(query))
	items := make([]pickerMaterial, 0)
	for _, m := range materials {
		if m.Archived {
			continue
		}
		hay := normalizeCombatQuery(m.Name + " " + m.SKU)
		matched := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		item := pickerMaterial{
			NoteID:     m.NoteID,
			MaterialID: m.ID,
			SKU:        m.SKU,
			Name:       m.Name,
			Unit:       m.Unit,
			Ekn:        materialEknNumbers(m),
		}
		if m.PriceMin != nil {
			item.PriceMin = *m.PriceMin
		}
		if m.PriceMax != nil {
			item.PriceMax = *m.PriceMax
		}
		items = append(items, item)
		if len(items) >= pickerSearchLimit {
			break
		}
	}
	return items
}

// pickerTitle — название карточки в списке выбора.
//
// Строка поиска несёт название заметки; если его нет — берём имя файла из
// пути. В отличие от карточек конкурентов (2026-11-11) здесь не бывает двух
// H1: системы и материалы заведены генератором, где H1 — название.
func pickerTitle(summary KBNoteSummary) string {
	if title := strings.TrimSpace(summary.Title); title != "" {
		return title
	}
	if notePath := strings.TrimSpace(summary.Path); notePath != "" {
		base := notePath
		if idx := strings.LastIndexAny(base, `/\`); idx >= 0 {
			base = base[idx+1:]
		}
		if base = strings.TrimSuffix(base, ".md"); strings.TrimSpace(base) != "" {
			return strings.TrimSpace(base)
		}
	}
	return "Без названия"
}

// errKBUnavailable — База знаний не настроена. Нужна боту, чтобы отличить
// «нечего искать» от «поиск недоступен».
var errKBUnavailable = errors.New("база знаний не настроена")

// suggestionsFor — то, что бот предлагает по свободной фразе (2026-11-12).
//
// Одна функция на все случаи: и подсказка в брифе, и команда «система …».
// Пустой результат — не ошибка: бот скажет, что не нашлось, и предложит
// команду с полным названием.
func (s *Server) suggestionsFor(ctx context.Context, query string) ([]pickerSystem, []pickerMaterial, error) {
	if s.kb == nil || !s.kb.configured() {
		return nil, nil, errKBUnavailable
	}
	systems, err := s.searchSystems(ctx, query)
	if err != nil {
		return nil, nil, err
	}
	materials, err := s.listMaterials(ctx, false)
	if err != nil {
		log.Printf("подсказки: справочник материалов недоступен: %v", err)
		return systems, nil, nil
	}
	matItems, err := s.searchMaterials(ctx, query, materials)
	if err != nil {
		log.Printf("подсказки: поиск материалов недоступен: %v", err)
		return systems, nil, nil
	}
	return systems, matItems, nil
}

// handleSearchSuggestions — GET /suggest?q=… (роль viewer).
//
// Отдаёт системы и материалы одним ответом: боту нужен один запрос на фразу,
// а не два подряд — сообщение и так идёт после каждого слова брифа.
func (s *Server) handleSearchSuggestions(w http.ResponseWriter, r *http.Request) {
	s.writeSuggestions(w, r)
}

// handleBotSuggest — та же подсказка для бота (X-Service-Secret).
//
// Отдельная ручка, а не роль JWT: бот ходит служебным секретом, а не ключом
// человека, и смешивать их нельзя (2026-11-12).
func (s *Server) handleBotSuggest(w http.ResponseWriter, r *http.Request) {
	s.writeSuggestions(w, r)
}

func (s *Server) writeSuggestions(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"systems":   []pickerSystem{},
			"materials": []pickerMaterial{},
		})
		return
	}
	systems, materials, err := s.suggestionsFor(r.Context(), query)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"systems":   []pickerSystem{},
			"materials": []pickerMaterial{},
			"error":     errorText(err),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"systems":   systems,
		"materials": materials,
	})
}
