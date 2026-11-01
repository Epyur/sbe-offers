package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// Placeholder — поле формы КП, которое администратор заводит в конфигураторе.
type Placeholder struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Block — блок документа формы (абзацы/заголовки/списки/таблицы, разрывы
// страниц, ссылки и текст по промту).
//
// Размещение на странице (2026-11-01): отступы в пунктах (Word меряет абзацный
// интервал в twips, 1 пт = 20; в JSON пишем пункты — админу понятнее),
// «не разрывать между страницами» и «начинать с новой страницы». Абсолютного
// позиционирования нет намеренно: текстовые поля и якоря не переживают правку
// абзаца человеком, а для письма нужны именно отступы, выравнивание и разрыв.
type Block struct {
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	Content        json.RawMessage `json:"content"`
	ShowInClient   bool            `json:"showInClient"`
	ShowInInternal bool            `json:"showInInternal"`
	// SpacingBefore / SpacingAfter — отступ до и после блока в пунктах.
	// 0 — как в стиле бланка; отрицательные не допускаются.
	SpacingBefore int `json:"spacingBefore,omitempty"`
	SpacingAfter  int `json:"spacingAfter,omitempty"`
	// KeepTogether — не разрывать абзацы блока между страницами.
	KeepTogether bool `json:"keepTogether,omitempty"`
	// PageBreakBefore — начинать блок с новой страницы.
	PageBreakBefore bool `json:"pageBreakBefore,omitempty"`
}

// OfferForm — конфигурация формы КП.
type OfferForm struct {
	ID             int64         `json:"id"`
	Mode           string        `json:"mode"`
	Title          string        `json:"title"`
	Placeholders   []Placeholder `json:"placeholders"`
	Blocks         []Block       `json:"blocks"`
	RequiredFields []string      `json:"requiredFields"`
	UpdatedAt      string        `json:"updatedAt"`
}

// maxBlockSpacing — предел отступа блока в пунктах. Свыше 200 пт блок
// уезжает с страницы целиком, и это ошибка админа, а не оформление.
const maxBlockSpacing = 200

// validateForm проверяет форму перед сохранением; пустая строка — форма корректна.
func validateForm(f OfferForm) string {
	if strings.TrimSpace(f.Title) == "" {
		return "не заполнен заголовок формы"
	}
	seenBlocks := make(map[string]bool, len(f.Blocks))
	for _, block := range f.Blocks {
		if strings.TrimSpace(block.ID) == "" {
			return "у блока пустой идентификатор"
		}
		if seenBlocks[block.ID] {
			return "повторяющийся идентификатор блока"
		}
		seenBlocks[block.ID] = true
		if block.SpacingBefore < 0 || block.SpacingAfter < 0 {
			return "отступ блока «" + block.Title + "» не может быть отрицательным"
		}
		if block.SpacingBefore > maxBlockSpacing || block.SpacingAfter > maxBlockSpacing {
			return "отступ блока «" + block.Title + "» больше " + strconv.Itoa(maxBlockSpacing) + " пт"
		}
	}
	seenFields := make(map[string]bool, len(f.Placeholders))
	for _, p := range f.Placeholders {
		if strings.TrimSpace(p.ID) == "" {
			return "пустой идентификатор поля"
		}
		if seenFields[p.ID] {
			return "повторяющийся идентификатор поля"
		}
		seenFields[p.ID] = true
	}
	for _, req := range f.RequiredFields {
		if !seenFields[req] {
			return "обязательное поле «" + req + "» не заведено в списке полей"
		}
	}
	return ""
}

// defaultForm — форма по умолчанию: пока администратор ничего не настроил,
// предложение всё равно собирается.
//
// Порядок — как просил пользователь (2026-11-01): титульный лист, разрыв
// страницы, затем поставщик / получатель / объект, свободный блок про систему и
// материалы (текст пишет модель по подсказке), позиции (их печатает сборщик
// документа сам) и ссылки в конце.
//
// Уже сохранённые формы эта функция не трогает: «строки нет в БД» — это и есть
// «форма не настроена», а сохранённая пустая форма остаётся пустой (правило
// 2026-10-27, живой дефект с подменой формы).
func defaultForm(mode string) OfferForm {
	return OfferForm{
		Mode:  mode,
		Title: "Коммерческое предложение",
		Placeholders: []Placeholder{
			{ID: "supplier_name", Label: "Поставщик"},
			{ID: "supplier_contacts", Label: "Контакты поставщика"},
			{ID: "client_name", Label: "Получатель (организация)"},
			{ID: "client_contact", Label: "Контактное лицо получателя"},
			{ID: "object_name", Label: "Объект"},
			{ID: "object_address", Label: "Адрес объекта"},
			{ID: "valid_until", Label: "Предложение действительно до"},
			{ID: "conditions", Label: "Условия"},
		},
		Blocks: []Block{
			{
				ID:           "cover",
				Title:        "Титульный лист",
				Content:      json.RawMessage(coverBlockJSON),
				ShowInClient: true,
			},
			{
				ID:              "parties",
				Title:           "Поставщик, получатель, объект",
				Content:         json.RawMessage(partiesBlockJSON),
				ShowInClient:    true,
				PageBreakBefore: true,
			},
			{
				ID:            "description",
				Title:         "Система и материалы",
				Content:       json.RawMessage(descriptionBlockJSON),
				ShowInClient:  true,
				SpacingBefore: 12,
			},
			{
				ID:           "links",
				Title:        "Ссылки",
				Content:      json.RawMessage(`[{"type":"links","level":2}]`),
				ShowInClient: true,
			},
		},
		RequiredFields: []string{"client_name"},
	}
}

// Содержимое блоков формы по умолчанию. Текстом, а не собранным в коде JSON:
// эти строки правятся при изменении шаблона, и держать их рядом проще.
const (
	// coverBlockJSON — титульный лист. Абзацы по центру, подписи полей — в
	// одном абзаце с {{полем}}: пустое поле даёт «Кому: » без пустых строк.
	coverBlockJSON = `[
		{"type":"paragraph","text":"{{supplier_name}}","align":"center"},
		{"type":"paragraph","text":"КОММЕРЧЕСКОЕ ПРЕДЛОЖЕНИЕ","align":"center","level":2},
		{"type":"paragraph","text":"Кому: {{client_name}}","align":"center"},
		{"type":"paragraph","text":"{{client_contact}}","align":"center"}
	]`

	// partiesBlockJSON — содержательная часть: кто предлагает, кому и для
	// какого объекта. Таблицей из двух колонок, как в письмах.
	partiesBlockJSON = `[
		{"type":"heading","text":"Об объекте","level":2},
		{"type":"table","rows":[
			["Поставщик","{{supplier_name}}"],
			["Контакты","{{supplier_contacts}}"],
			["Получатель","{{client_name}}"],
			["Контактное лицо","{{client_contact}}"],
			["Объект","{{object_name}}"],
			["Адрес объекта","{{object_address}}"]
		]}
	]`

	// descriptionBlockJSON — свободный блок: текст пишет модель по подсказке,
	// опираясь на объект, системы из карточек и состав предложения.
	descriptionBlockJSON = `[
		{"type":"heading","text":"Предлагаемое решение","level":2},
		{"type":"prompt_text","instruction":"Опиши предлагаемую систему и материалы, входящие в неё: как они работают вместе и что дают на этом объекте. Опирайся только на данные предложения и карточки систем.","maxWords":200}
	]`
)

func (s *Server) seedForms(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO offers_form (mode) VALUES ('client') ON CONFLICT (mode) DO NOTHING`)
	return err
}

func (s *Server) loadForm(ctx context.Context, mode string) (OfferForm, error) {
	if strings.TrimSpace(mode) == "" {
		mode = "client"
	}
	var form OfferForm
	var placeholders, blocks, required []byte
	err := s.pool.QueryRow(ctx, `
SELECT id, mode, title, placeholders, blocks, required_fields,
	to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
FROM offers_form WHERE mode = $1`, mode).
		Scan(&form.ID, &form.Mode, &form.Title, &placeholders, &blocks, &required, &form.UpdatedAt)
	if err != nil {
		// Строки нет вовсе — форма не настроена, показываем поля по умолчанию.
		// Пустые массивы в БД — это НАСТРОЕННАЯ пустая форма, и подменять её
		// нельзя: раньше проверка стояла до разбора JSON и сравнивала nil-слайс,
		// поэтому сохранённая форма не доходила ни до редактора, ни до DOCX.
		return defaultForm(mode), nil
	}
	form.Placeholders = []Placeholder{}
	form.Blocks = []Block{}
	form.RequiredFields = []string{}
	if len(placeholders) > 0 {
		if err := json.Unmarshal(placeholders, &form.Placeholders); err != nil {
			return form, err
		}
	}
	if len(blocks) > 0 {
		if err := json.Unmarshal(blocks, &form.Blocks); err != nil {
			return form, err
		}
	}
	if len(required) > 0 {
		if err := json.Unmarshal(required, &form.RequiredFields); err != nil {
			return form, err
		}
	}
	return form, nil
}

func (s *Server) handleGetForm(w http.ResponseWriter, r *http.Request) {
	form, err := s.loadForm(r.Context(), r.URL.Query().Get("mode"))
	if err != nil {
		dbError(w, "get form", err)
		return
	}
	writeJSON(w, http.StatusOK, form)
}

func (s *Server) handleSaveForm(w http.ResponseWriter, r *http.Request) {
	var form OfferForm
	if err := decodeJSON(w, r, &form, 4<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if strings.TrimSpace(form.Mode) == "" {
		form.Mode = "client"
	}
	if form.Blocks == nil {
		form.Blocks = []Block{}
	}
	if form.Placeholders == nil {
		form.Placeholders = []Placeholder{}
	}
	if form.RequiredFields == nil {
		form.RequiredFields = []string{}
	}
	if msg := validateForm(form); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
		return
	}

	placeholders, err := json.Marshal(form.Placeholders)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}
	blocks, err := json.Marshal(form.Blocks)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}
	required, err := json.Marshal(form.RequiredFields)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}

	_, err = s.pool.Exec(r.Context(), `
INSERT INTO offers_form (mode, title, placeholders, blocks, required_fields, updated_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (mode) DO UPDATE SET
	title = EXCLUDED.title, placeholders = EXCLUDED.placeholders,
	blocks = EXCLUDED.blocks, required_fields = EXCLUDED.required_fields,
	updated_at = now()`, form.Mode, strings.TrimSpace(form.Title), placeholders, blocks, required)
	if err != nil {
		dbError(w, "save form", err)
		return
	}
	// Отвечаем тем, что реально записали, а НЕ перечитанным значением: пустая
	// форма из БД трактуется как «не настроена» и подменяется формой по
	// умолчанию — админ сохранил бы новую форму и получил ответ со старой.
	saved := form
	saved.ID = 0
	var savedID int64
	var updatedAt string
	if err := s.pool.QueryRow(r.Context(),
		`SELECT id, to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM offers_form WHERE mode = $1`,
		form.Mode).Scan(&savedID, &updatedAt); err != nil {
		dbError(w, "reload form", err)
		return
	}
	saved.ID = savedID
	saved.UpdatedAt = updatedAt
	writeJSON(w, http.StatusOK, saved)
}
