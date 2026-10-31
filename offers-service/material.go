package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
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
//
// Пустой вид считается материалом: так его и трактует запись в базу
// (`normalizeMaterial` подставляет "material"), и так же ведёт себя поиск в
// каталоге бота. Иначе строка, сохранённая до появления вида, выпала бы оттуда
// молча — бот её не показал бы, а причина осталась бы невидимой.
func materialKindAllowed(kind string) bool {
	return kind == "" || kind == "material" || kind == "work"
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

// --- Правила входа -----------------------------------------------------

// normalizeMaterial приводит карточку к рабочему виду: обрезает поля и
// подставляет умолчания. Отдельной функцией, потому что эти же умолчания
// нужны и при создании, и при правке.
func normalizeMaterial(m Material) Material {
	m.Name = strings.TrimSpace(m.Name)
	m.Unit = strings.TrimSpace(m.Unit)
	m.Description = strings.TrimSpace(m.Description)
	m.Kind = strings.TrimSpace(m.Kind)
	if m.Unit == "" {
		m.Unit = "шт"
	}
	if m.Kind == "" {
		m.Kind = "material"
	}
	return m
}

// validateMaterial проверяет карточку; пустая строка — карточка годна.
func validateMaterial(m Material) string {
	if strings.TrimSpace(m.Name) == "" {
		return "не заполнено название материала"
	}
	kind := m.Kind
	if kind == "" {
		kind = "material"
	}
	if !materialKindAllowed(kind) {
		// Система — не материал: у неё нет SKU и ЕКН, её правда в Базе знаний.
		return "вид должен быть material или work; системы живут в карточках Базы знаний"
	}
	return ""
}

// widenPriceRange расширяет вилку цен новой ценой из таблицы менеджера.
//
// Расширяем, а не заменяем: вилка нужна для аналитики, и одна дешёвая
// поставка не должна затереть историю дорогих. Возвращает новые границы и
// признак «что-то изменилось» — по нему решается, двигать ли price_seen_at.
func widenPriceRange(low, high *float64, price float64) (*float64, *float64, bool) {
	changed := false
	if low == nil || price < *low {
		v := price
		low = &v
		changed = true
	}
	if high == nil || price > *high {
		v := price
		high = &v
		changed = true
	}
	return low, high, changed
}

// --- Ручки -------------------------------------------------------------

func (s *Server) handleListMaterials(w http.ResponseWriter, r *http.Request) {
	includeArchived := r.URL.Query().Get("archived") == "1"
	items, err := s.listMaterials(r.Context(), includeArchived)
	if err != nil {
		dbError(w, "list materials", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleSaveMaterial(w http.ResponseWriter, r *http.Request) {
	var req materialRequest
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	item := normalizeMaterial(req.Material)
	if msg := validateMaterial(item); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
		return
	}

	idFromPath, ok := pathID(w, r)
	if !ok {
		return
	}

	if idFromPath > 0 {
		item.ID = idFromPath
		saved, err := s.updateMaterial(r.Context(), item)
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "материал не найден"})
			return
		}
		if err != nil {
			dbError(w, "save material", err)
			return
		}
		writeJSON(w, http.StatusOK, saved)
		return
	}

	saved, err := s.insertMaterial(r.Context(), item, req.TypeCode, req.Series)
	if err != nil {
		dbError(w, "create material", err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// handleDeleteMaterial архивирует материал. Удалять нельзя: на карточку
// ссылаются старые предложения, и дырка в справочнике дороже архива.
func (s *Server) handleDeleteMaterial(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok || id <= 0 {
		return
	}
	row := s.pool.QueryRow(r.Context(), `
UPDATE offers_material SET archived = true, updated_at = now() WHERE id = $1
RETURNING `+materialColumns, id)
	saved, err := scanMaterial(row)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "материал не найден"})
		return
	}
	if err != nil {
		dbError(w, "archive material", err)
		return
	}
	saved.Ekn, _ = s.listMaterialEkn(r.Context(), saved.ID)
	writeJSON(w, http.StatusOK, saved)
}

// materialRequest — тело ручки: карточка и подсказки для SKU.
//
// TypeCode и Series — только для генерации SKU у нового материала; у правки
// они игнорируются, потому что SKU не меняется никогда.
type materialRequest struct {
	Material
	TypeCode string `json:"typeCode"`
	Series   string `json:"series"`
}

// pathID читает id из пути. Отвечает 400 и возвращает false при мусоре.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.PathValue("id")
	if raw == "" {
		return 0, true
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return 0, false
	}
	return parsed, true
}

// handleAddMaterialEkn добавляет вариацию ЕКН к материалу.
//
// Повторное добавление того же номера ничего не меняет и не падает: таблица
// менеджера может прийти дважды, и это нормальный ход работы, а не ошибка.
func (s *Server) handleAddMaterialEkn(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Ekn     string `json:"ekn"`
		Variant string `json:"variant"`
		Source  string `json:"source"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	ekn := strings.TrimSpace(req.Ekn)
	if ekn == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "не указан ЕКН"})
		return
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "manual"
	}
	if source != "manual" && source != "excel" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "source должен быть manual или excel"})
		return
	}

	// Материал должен существовать: иначе ЕКН повиснет в никуда.
	if _, err := s.loadMaterial(r.Context(), id); errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "материал не найден"})
		return
	} else if err != nil {
		dbError(w, "load material for ekn", err)
		return
	}

	_, err := s.pool.Exec(r.Context(), `
INSERT INTO offers_material_ekn (material_id, ekn, variant, source)
VALUES ($1, $2, $3, $4)
ON CONFLICT (material_id, ekn) DO UPDATE
	SET variant = CASE WHEN EXCLUDED.variant <> '' THEN EXCLUDED.variant ELSE offers_material_ekn.variant END,
	    last_seen_at = now()`,
		id, ekn, strings.TrimSpace(req.Variant), source)
	if err != nil {
		dbError(w, "add ekn", err)
		return
	}
	// updated_at материала двигаем: состав вариаций — часть карточки, и без
	// этого правка не выиграет слияние у того, кто откроет её позже.
	if _, err := s.pool.Exec(r.Context(),
		`UPDATE offers_material SET updated_at = now() WHERE id = $1`, id); err != nil {
		dbError(w, "touch material", err)
		return
	}
	saved, err := s.loadMaterial(r.Context(), id)
	if err != nil {
		dbError(w, "reload material", err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// handleDeleteMaterialEkn убирает вариацию.
func (s *Server) handleDeleteMaterialEkn(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	eknRaw := r.PathValue("eknId")
	eknID, err := strconv.ParseInt(eknRaw, 10, 64)
	if err != nil || eknID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid ekn id"})
		return
	}
	tag, err := s.pool.Exec(r.Context(),
		`DELETE FROM offers_material_ekn WHERE id = $1 AND material_id = $2`, eknID, id)
	if err != nil {
		dbError(w, "delete ekn", err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "вариация не найдена"})
		return
	}
	if _, err := s.pool.Exec(r.Context(),
		`UPDATE offers_material SET updated_at = now() WHERE id = $1`, id); err != nil {
		dbError(w, "touch material", err)
		return
	}
	saved, err := s.loadMaterial(r.Context(), id)
	if err != nil {
		dbError(w, "reload material", err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}
