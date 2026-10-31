package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Загрузка таблицы менеджера двумя шагами (Task 5 плана 2026-10-30).
//
// Шаг 1 — POST .../sheet: разобрать и сопоставить, НИЧЕГО НЕ ЗАПИСАТЬ, вернуть
// «что я понял». Шаг 2 — POST .../sheet/apply: записать только подтверждённое.
//
// Разделение не формальность: файл от заказчика бывает черновиком со старыми
// ценами и опечатками. Молчаливое применение уронит КП в глазах заказчика,
// а сверка занимает у менеджера полминуты.

// SheetPreview — что сервис понял из таблицы, без изменения данных.
type SheetPreview struct {
	FileName string        `json:"fileName"`
	Rows     []MatchResult `json:"rows"`
	// Matched — сколько строк сопоставлено каждым способом.
	Counts SheetCounts `json:"counts"`
	// Problems — то, что человек должен увидеть до применения.
	Problems []string `json:"problems"`
}

// SheetCounts — сводка по способам сопоставления.
type SheetCounts struct {
	Total  int `json:"total"`
	ByEkn  int `json:"byEkn"`
	ByName int `json:"byName"`
	ByAI   int `json:"byAi"`
	None   int `json:"none"`
}

// SheetApplyRow — строка, которую менеджер подтвердил.
//
// Материал приходит из подтверждённого превью: что человек видел, то и
// применяется. Пересопоставление на втором шаге запрещено — иначе он
// подтвердил бы одно, а записалось другое.
type SheetApplyRow struct {
	Line       int     `json:"line"`
	MaterialID int64   `json:"materialId"`
	Ekn        string  `json:"ekn"`
	Variant    string  `json:"variant"`
	Qty        float64 `json:"qty"`
	Price      float64 `json:"price"`
}

// SheetApplyRequest — тело второго шага.
type SheetApplyRequest struct {
	Rows []SheetApplyRow `json:"rows"`
	// AddMissingEkn — дописывать ли в справочник номера, которых там нет.
	AddMissingEkn bool `json:"addMissingEkn"`
}

// buildPreview собирает превью из сопоставления и замечаний.
func buildPreview(fileName string, results []MatchResult) SheetPreview {
	p := SheetPreview{FileName: fileName, Rows: results, Counts: SheetCounts{Total: len(results)}}
	for _, r := range results {
		switch r.Method {
		case matchByEkn:
			p.Counts.ByEkn++
		case matchByName:
			p.Counts.ByName++
		case matchByAI:
			p.Counts.ByAI++
		default:
			p.Counts.None++
		}
	}

	if p.Counts.ByAI > 0 {
		p.Problems = append(p.Problems, fmt.Sprintf(
			"Модель предложила материалы для %d строк(и) — проверьте их перед применением.", p.Counts.ByAI))
	}
	if p.Counts.None > 0 {
		p.Problems = append(p.Problems, fmt.Sprintf(
			"Не сопоставлено строк: %d. Они не попадут в предложение, но останутся в списке ниже.", p.Counts.None))
	}
	// Цены ниже единицы почти всегда опечатка или «тысячи без нулей».
	for _, r := range results {
		if r.MaterialID > 0 && r.Row.HasPrice && r.Row.Price <= 0 {
			p.Problems = append(p.Problems, fmt.Sprintf(
				"Строка %d: цена %v — похоже на опечатку.", r.Row.Line, r.Row.Price))
		}
	}
	// Сумма должна сходиться с количеством и ценой.
	for _, r := range results {
		if r.Row.HasSum && r.Row.HasQty && r.Row.HasPrice {
			want := roundMoney(r.Row.Qty * r.Row.Price)
			if roundMoney(r.Row.Sum) != want {
				p.Problems = append(p.Problems, fmt.Sprintf(
					"Строка %d: сумма %v не сходится с %v × %v = %v. Применим количество и цену, сумму пересчитаем.",
					r.Row.Line, r.Row.Sum, r.Row.Qty, r.Row.Price, want))
			}
		}
	}
	return p
}

// handleSheetPreview разбирает таблицу и сопоставляет её, ничего не записывая.
func (s *Server) handleSheetPreview(w http.ResponseWriter, r *http.Request) {
	offer, ok := s.offerForEditor(w, r)
	if !ok {
		return
	}
	data, name, ok := readSheetUpload(w, r)
	if !ok {
		return
	}
	sheet, err := ParseSheetXlsx(data, name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	materials, err := s.materialRefs(r.Context())
	if err != nil {
		dbError(w, "load material refs", err)
		return
	}
	results := matchRows(r.Context(), sheet.Rows, materials, llmCompleter{})
	preview := buildPreview(sheet.FileName, results)
	preview.FileName = name
	_ = offer
	writeJSON(w, http.StatusOK, preview)
}

// readSheetUpload читает файл из тела. Имя — из заголовка, как у attach.
func readSheetUpload(w http.ResponseWriter, r *http.Request) ([]byte, string, bool) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxAttachmentBytes))
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "файл больше 10 МБ"})
		return nil, "", false
	}
	name := strings.TrimSpace(r.URL.Query().Get("file_name"))
	if name == "" {
		name = strings.TrimSpace(r.Header.Get("X-File-Name"))
	}
	if len(data) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "пустой файл"})
		return nil, "", false
	}
	return data, name, true
}

// materialRefs — материалы с их ЕКН для сопоставления.
func (s *Server) materialRefs(ctx context.Context) ([]MaterialRef, error) {
	items, err := s.listMaterials(ctx, false)
	if err != nil {
		return nil, err
	}
	refs := make([]MaterialRef, 0, len(items))
	for _, m := range items {
		ekns := make([]string, 0, len(m.Ekn))
		for _, e := range m.Ekn {
			ekns = append(ekns, e.Ekn)
		}
		refs = append(refs, MaterialRef{ID: m.ID, SKU: m.SKU, Name: m.Name, Ekn: ekns})
	}
	return refs, nil
}

// handleSheetApply записывает подтверждённое: позиции, новые ЕКН, вилку цен.
func (s *Server) handleSheetApply(w http.ResponseWriter, r *http.Request) {
	offer, ok := s.offerForEditor(w, r)
	if !ok {
		return
	}
	var req SheetApplyRequest
	if err := decodeJSON(w, r, &req, 4<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if len(req.Rows) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "нечего применять: нет строк"})
		return
	}

	items := make([]OfferItem, 0, len(req.Rows))
	applied := 0
	for _, row := range req.Rows {
		if row.MaterialID <= 0 {
			continue
		}
		material, err := s.loadMaterial(r.Context(), row.MaterialID)
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": fmt.Sprintf("строка %d: материал %d не найден", row.Line, row.MaterialID),
			})
			return
		}
		if err != nil {
			dbError(w, "apply sheet load material", err)
			return
		}

		qty := row.Qty
		if qty <= 0 {
			qty = 1
		}
		// Цена — ИЗ ТАБЛИЦЫ. Каталог цены не знает вовсе; подставлять здесь
		// что-то из справочника значило бы выдумать число.
		price := row.Price
		catalogID := material.ID
		items = append(items, OfferItem{
			CatalogID: &catalogID,
			Name:      material.Name,
			Unit:      material.Unit,
			Qty:       qty,
			Price:     price,
			Sum:       roundMoney(qty * price),
		})
		applied++

		if req.AddMissingEkn {
			if err := s.rememberSheetPrice(r.Context(), material, row); err != nil {
				dbError(w, "apply sheet remember price", err)
				return
			}
		}
	}
	if applied == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ни одна строка не сопоставлена с материалом"})
		return
	}

	// Позиции получают цену менеджера и НЕ трогают форму: таблица — про
	// состав и цены, а не про текст предложения.
	offer.Items = items
	if err := s.persistOfferItems(r.Context(), offer); err != nil {
		dbError(w, "apply sheet save offer", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"applied": applied,
		"offer":   offer,
	})
}

// rememberSheetPrice дописывает ЕКН и расширяет вилку цен материала.
//
// Вилка расширяется, но не сужается: одна дешёвая поставка не должна затереть
// историю дорогих. price_seen_at двигается только когда вилка изменилась —
// иначе метка «когда видели цену» врала бы при каждой загрузке.
func (s *Server) rememberSheetPrice(ctx context.Context, material Material, row SheetApplyRow) error {
	ekn := strings.TrimSpace(row.Ekn)
	if ekn != "" {
		if _, err := s.pool.Exec(ctx, `
INSERT INTO offers_material_ekn (material_id, ekn, variant, source)
VALUES ($1, $2, $3, 'excel')
ON CONFLICT (material_id, ekn) DO UPDATE
	SET variant = CASE WHEN EXCLUDED.variant <> '' THEN EXCLUDED.variant ELSE offers_material_ekn.variant END,
	    last_seen_at = now()`,
			material.ID, ekn, strings.TrimSpace(row.Variant)); err != nil {
			return err
		}
	}
	if !row.HasPriceLike() {
		return nil
	}
	low, high, changed := widenPriceRange(material.PriceMin, material.PriceMax, row.Price)
	if !changed {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
UPDATE offers_material
SET price_min = $2, price_max = $3, price_seen_at = now(), updated_at = now()
WHERE id = $1`, material.ID, low, high)
	return err
}

// HasPriceLike — есть ли у строки цена, которую можно запомнить.
func (r SheetApplyRow) HasPriceLike() bool {
	return r.Price > 0
}
