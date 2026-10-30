package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// Материалы и ЕКН (2026-10-30).
//
// Материал — то, что продаётся: у него свой SKU и вариации (ЕКН). Система
// материалом не является: это карточка Базы знаний с обоснованием сочетания,
// своей цены у неё нет. Поэтому здесь только материалы; системы живут в
// syscards.go и читаются из базы знаний.
//
// Цены у материала НЕТ намеренно: цена одного и того же материала зависит от
// условий поставки и приходит из таблицы менеджера для конкретного КП. Здесь
// хранится только вилка из загруженных таблиц — для аналитики.

// Material — материал или работа в справочнике КП.
type Material struct {
	ID          int64  `json:"id"`
	SKU         string `json:"sku"`
	Kind        string `json:"kind"` // material | work
	Name        string `json:"name"`
	Unit        string `json:"unit"`
	Description string `json:"description"`
	// PriceMin/PriceMax — вилка из таблиц менеджера. nil значит «не видели
	// ни одной цены»; ноль значил бы бесплатный материал, а это разные вещи.
	PriceMin    *float64 `json:"priceMin,omitempty"`
	PriceMax    *float64 `json:"priceMax,omitempty"`
	PriceSeenAt string   `json:"priceSeenAt,omitempty"`
	PhotoKey    string   `json:"photoKey"`
	SortOrder   int      `json:"sortOrder"`
	Archived    bool     `json:"archived"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
	// Ekn — вариации материала. Заполняется при чтении карточки.
	Ekn []MaterialEkn `json:"ekn"`
}

// MaterialEkn — одна вариация материала: номер ЕКН и как она называется.
//
// Вариативность живёт здесь, а не в материале: толщина, цвет и размер — это
// свойства КОНКРЕТНОГО исполнения, и один материал может иметь их десяток.
type MaterialEkn struct {
	ID          int64  `json:"id"`
	MaterialID  int64  `json:"materialId"`
	Ekn         string `json:"ekn"`
	Variant     string `json:"variant"`
	Source      string `json:"source"` // excel | manual
	FirstSeenAt string `json:"firstSeenAt"`
	LastSeenAt  string `json:"lastSeenAt"`
}

// Материал и работа — законные виды карточки; система — нет.
func materialKindAllowed(kind string) bool {
	return kind == "material" || kind == "work"
}

// Префиксы SKU. Работа отделена от материала: в документах это разные строки,
// и «РАБ-» сразу говорит, что ЕКН у такой позиции не бывает.
const (
	skuPrefixMaterial = "MAT"
	skuPrefixWork     = "РАБ"
	skuFallbackType   = "ПРОЧ"
	skuFallbackSeries = "ОБЩ"
)

// reSkuPart — что допустимо в частях SKU: буквы (кириллица и латиница),
// цифры. Пробелы, точки и дефисы в серии развалили бы и разбор, и ключ файла
// в хранилище.
var reSkuPart = regexp.MustCompile(`[^А-ЯЁA-Z0-9]+`)

// skuPart приводит часть SKU к безопасному виду и обрезает до max знаков.
//
// Длины разные не случайно: код типа (`МЕМБ`, `УТЕПЛ`) в ходу четырёхзначный —
// трёх знаков не хватало, `МЕМБ` превращался в `МЕМ` и путал мембрану с
// мемориалом. Серия (`ПВХ`, `ПИР`, `ЭПП`) трёхзначная, и длиннее её читать
// в переписке уже неудобно.
func skuPart(raw, fallback string, max int) string {
	cleaned := reSkuPart.ReplaceAllString(strings.ToUpper(strings.TrimSpace(raw)), "")
	if cleaned == "" {
		return fallback
	}
	runes := []rune(cleaned)
	if len(runes) > max {
		runes = runes[:max]
	}
	return string(runes)
}

// buildSku — SKU материала: MAT-{ТИП}-{СЕРИЯ}-{НОМЕР}.
func buildSku(typeCode, series string, number int64) string {
	return buildSkuForKind("material", typeCode, series, number)
}

// buildSkuForKind — то же с префиксом по виду карточки.
func buildSkuForKind(kind, typeCode, series string, number int64) string {
	prefix := skuPrefixMaterial
	if kind == "work" {
		prefix = skuPrefixWork
	}
	return fmt.Sprintf("%s-%s-%s-%04d",
		prefix,
		skuPart(typeCode, skuFallbackType, 4),
		skuPart(series, skuFallbackSeries, 3),
		number)
}

// --- Хранилище ---------------------------------------------------------

const materialColumns = `id, sku, kind, name, unit, description,
	price_min, price_max,
	to_char(price_seen_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
	photo_key, sort_order, archived,
	to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
	to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')`

// scanMaterial читает строку материала. Вилка цен — указатели: «не видели
// цены» и «цена ноль» — разные состояния, и путать их нельзя.
func scanMaterial(row interface{ Scan(...any) error }) (Material, error) {
	var m Material
	var minPrice, maxPrice *float64
	var seenAt *string
	err := row.Scan(&m.ID, &m.SKU, &m.Kind, &m.Name, &m.Unit, &m.Description,
		&minPrice, &maxPrice, &seenAt,
		&m.PhotoKey, &m.SortOrder, &m.Archived, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return Material{}, err
	}
	m.PriceMin, m.PriceMax = minPrice, maxPrice
	if seenAt != nil {
		m.PriceSeenAt = *seenAt
	}
	if m.Ekn == nil {
		m.Ekn = []MaterialEkn{}
	}
	return m, nil
}

// listMaterials — справочник материалов. Системы сюда не попадают: их больше
// нет в таблице материалов по построению.
func (s *Server) listMaterials(ctx context.Context, includeArchived bool) ([]Material, error) {
	query := `SELECT ` + materialColumns + ` FROM offers_material`
	if !includeArchived {
		query += ` WHERE NOT archived`
	}
	query += ` ORDER BY sort_order, name`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Material, 0)
	for rows.Next() {
		m, err := scanMaterial(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range items {
		ekns, err := s.listMaterialEkn(ctx, items[i].ID)
		if err != nil {
			return nil, err
		}
		items[i].Ekn = ekns
	}
	return items, nil
}

// loadMaterial — один материал с вариациями.
func (s *Server) loadMaterial(ctx context.Context, id int64) (Material, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+materialColumns+` FROM offers_material WHERE id = $1`, id)
	m, err := scanMaterial(row)
	if err != nil {
		return Material{}, err
	}
	m.Ekn, err = s.listMaterialEkn(ctx, id)
	return m, err
}

// listMaterialEkn — вариации материала, старые сверху.
func (s *Server) listMaterialEkn(ctx context.Context, materialID int64) ([]MaterialEkn, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, material_id, ekn, variant, source,
	to_char(first_seen_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
	to_char(last_seen_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
FROM offers_material_ekn WHERE material_id = $1 ORDER BY first_seen_at, id`, materialID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MaterialEkn, 0)
	for rows.Next() {
		var e MaterialEkn
		if err := rows.Scan(&e.ID, &e.MaterialID, &e.Ekn, &e.Variant, &e.Source, &e.FirstSeenAt, &e.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// nextSkuNumber — следующий номер счётчика для пары {ТИП}-{СЕРИЯ}.
//
// Считаем по уже выданным SKU, а не отдельным счётчиком: удалённый материал
// не должен освобождать номер — иначе новый материал получил бы SKU, который
// ещё живёт в старых предложениях.
func (s *Server) nextSkuNumber(ctx context.Context, prefix string) (int64, error) {
	var maxNo *int64
	err := s.pool.QueryRow(ctx, `
SELECT MAX(NULLIF(regexp_replace(sku, '^.*-', ''), '')::bigint)
FROM offers_material WHERE sku LIKE $1 || '-%'`, prefix).Scan(&maxNo)
	if err != nil {
		return 0, err
	}
	if maxNo == nil {
		return 1, nil
	}
	return *maxNo + 1, nil
}

// insertMaterial создаёт материал и назначает ему SKU.
//
// SKU назначает ТОЛЬКО сервер: клиентское значение не принимается, иначе в
// справочнике появятся два материала с одним кодом (правило модели).
func (s *Server) insertMaterial(ctx context.Context, m Material, typeCode, series string) (Material, error) {
	kind := m.Kind
	if kind == "" {
		kind = "material"
	}
	prefix := skuPrefixMaterial
	if kind == "work" {
		prefix = skuPrefixWork
	}
	prefix = fmt.Sprintf("%s-%s-%s",
		prefix, skuPart(typeCode, skuFallbackType, 4), skuPart(series, skuFallbackSeries, 3))

	number, err := s.nextSkuNumber(ctx, prefix)
	if err != nil {
		return Material{}, err
	}
	sku := buildSkuForKind(kind, typeCode, series, number)

	row := s.pool.QueryRow(ctx, `
INSERT INTO offers_material (sku, kind, name, unit, description, photo_key, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING `+materialColumns,
		sku, kind, m.Name, m.Unit, m.Description, m.PhotoKey, m.SortOrder)
	saved, err := scanMaterial(row)
	if err != nil {
		return Material{}, err
	}
	saved.Ekn = []MaterialEkn{}
	return saved, nil
}

// updateMaterial правит карточку. SKU не меняется никогда: это идентичность.
func (s *Server) updateMaterial(ctx context.Context, m Material) (Material, error) {
	kind := m.Kind
	if kind == "" {
		kind = "material"
	}
	row := s.pool.QueryRow(ctx, `
UPDATE offers_material SET kind = $2, name = $3, unit = $4, description = $5,
	photo_key = $6, sort_order = $7, archived = $8, updated_at = now()
WHERE id = $1
RETURNING `+materialColumns,
		m.ID, kind, m.Name, m.Unit, m.Description, m.PhotoKey, m.SortOrder, m.Archived)
	saved, err := scanMaterial(row)
	if err != nil {
		return Material{}, err
	}
	saved.Ekn, err = s.listMaterialEkn(ctx, saved.ID)
	return saved, err
}
