package main

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// CatalogItem — карточка системы/материала/работы в каталоге.
type CatalogItem struct {
	ID          int64   `json:"id"`
	Kind        string  `json:"kind"`
	Name        string  `json:"name"`
	Unit        string  `json:"unit"`
	BasePrice   float64 `json:"basePrice"`
	Description string  `json:"description"`
	SortOrder   int     `json:"sortOrder"`
	Archived    bool    `json:"archived"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

func catalogKindAllowed(kind string) bool {
	switch kind {
	case "system", "material", "work":
		return true
	default:
		return false
	}
}

// filterCatalogItems — поиск по названию и описанию, без учёта регистра и
// лишних пробелов. Все слова запроса должны найтись (в любых полях) — тот же
// приём, что у агентского поиска: «кровля X» не ловится одной подстрокой.
func filterCatalogItems(items []CatalogItem, q string) []CatalogItem {
	words := strings.Fields(strings.ToLower(strings.TrimSpace(q)))
	if len(words) == 0 {
		return items
	}
	result := make([]CatalogItem, 0, len(items))
	for _, item := range items {
		haystack := strings.ToLower(item.Name + " " + item.Description)
		matched := true
		for _, word := range words {
			if !strings.Contains(haystack, word) {
				matched = false
				break
			}
		}
		if matched {
			result = append(result, item)
		}
	}
	return result
}

// validateCatalogItem проверяет карточку перед сохранением; пустая строка —
// карточка корректна.
func validateCatalogItem(item CatalogItem) string {
	if strings.TrimSpace(item.Name) == "" {
		return "не заполнено название"
	}
	if item.BasePrice < 0 {
		return "цена не может быть отрицательной"
	}
	kind := item.Kind
	if kind == "" {
		kind = "system"
	}
	if !catalogKindAllowed(kind) {
		return "тип должен быть system, material или work"
	}
	return ""
}

func (s *Server) listCatalogItems(ctx context.Context, includeArchived bool) ([]CatalogItem, error) {
	query := `SELECT id, kind, name, unit, base_price, description, sort_order, archived,
		to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
		FROM offers_catalog`
	if !includeArchived {
		query += ` WHERE NOT archived`
	}
	query += ` ORDER BY sort_order, name`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]CatalogItem, 0)
	for rows.Next() {
		var it CatalogItem
		if err := rows.Scan(&it.ID, &it.Kind, &it.Name, &it.Unit, &it.BasePrice,
			&it.Description, &it.SortOrder, &it.Archived, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (s *Server) handleListCatalog(w http.ResponseWriter, r *http.Request) {
	includeArchived := r.URL.Query().Get("archived") == "1"
	items, err := s.listCatalogItems(r.Context(), includeArchived)
	if err != nil {
		log.Printf("list catalog: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "db error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": filterCatalogItems(items, r.URL.Query().Get("q"))})
}

func (s *Server) handleSaveCatalog(w http.ResponseWriter, r *http.Request) {
	var item CatalogItem
	if err := decodeJSON(w, r, &item, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	item.Name = strings.TrimSpace(item.Name)
	item.Unit = strings.TrimSpace(item.Unit)
	if item.Unit == "" {
		item.Unit = "шт"
	}
	if item.Kind == "" {
		item.Kind = "system"
	}
	if msg := validateCatalogItem(item); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
		return
	}

	idFromPath := int64(0)
	if raw := r.PathValue("id"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
			return
		}
		idFromPath = parsed
	}

	var saved CatalogItem
	var err error
	if idFromPath > 0 {
		err = s.pool.QueryRow(r.Context(), `
UPDATE offers_catalog SET kind = $2, name = $3, unit = $4, base_price = $5,
	description = $6, sort_order = $7, archived = $8, updated_at = now()
WHERE id = $1
RETURNING id, kind, name, unit, base_price, description, sort_order, archived,
	to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
	to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')`,
			idFromPath, item.Kind, item.Name, item.Unit, item.BasePrice,
			item.Description, item.SortOrder, item.Archived).
			Scan(&saved.ID, &saved.Kind, &saved.Name, &saved.Unit, &saved.BasePrice,
				&saved.Description, &saved.SortOrder, &saved.Archived, &saved.CreatedAt, &saved.UpdatedAt)
	} else {
		err = s.pool.QueryRow(r.Context(), `
INSERT INTO offers_catalog (kind, name, unit, base_price, description, sort_order, archived)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, kind, name, unit, base_price, description, sort_order, archived,
	to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
	to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')`,
			item.Kind, item.Name, item.Unit, item.BasePrice,
			item.Description, item.SortOrder, item.Archived).
			Scan(&saved.ID, &saved.Kind, &saved.Name, &saved.Unit, &saved.BasePrice,
				&saved.Description, &saved.SortOrder, &saved.Archived, &saved.CreatedAt, &saved.UpdatedAt)
	}
	if err != nil {
		log.Printf("save catalog: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "db error"})
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteCatalog(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	// Карточку не удаляем физически: на неё могут ссылаться старые предложения.
	if _, err := s.pool.Exec(r.Context(),
		`UPDATE offers_catalog SET archived = true, updated_at = now() WHERE id = $1`, id); err != nil {
		log.Printf("archive catalog: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "db error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
