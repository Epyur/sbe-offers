package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// Placeholder — поле формы КП, которое администратор заводит в конфигураторе.
type Placeholder struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Block — блок документа формы (абзацы/заголовки/списки/таблицы).
type Block struct {
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	Content        json.RawMessage `json:"content"`
	ShowInClient   bool            `json:"showInClient"`
	ShowInInternal bool            `json:"showInInternal"`
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
// предложение всё равно собирается (клиент, контакт, условия).
func defaultForm(mode string) OfferForm {
	return OfferForm{
		Mode:  mode,
		Title: "Коммерческое предложение",
		Placeholders: []Placeholder{
			{ID: "client_name", Label: "Наименование клиента"},
			{ID: "client_contact", Label: "Контактное лицо"},
			{ID: "valid_until", Label: "Предложение действительно до"},
			{ID: "conditions", Label: "Условия"},
		},
		Blocks: []Block{
			{
				ID:           "intro",
				Title:        "Вступление",
				Content:      json.RawMessage(`[{"type":"paragraph","text":"Предлагаем Вам поставку материалов и выполнение работ по следующим позициям."}]`),
				ShowInClient: true,
			},
			{
				ID:           "conditions",
				Title:        "Условия",
				Content:      json.RawMessage(`[{"type":"paragraph","text":"Условия: {{conditions}}"},{"type":"paragraph","text":"Предложение действительно до {{valid_until}}."}]`),
				ShowInClient: true,
			},
		},
		RequiredFields: []string{"client_name"},
	}
}

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
		log.Printf("get form: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "db error"})
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
		log.Printf("save form: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "db error"})
		return
	}
	saved, err := s.loadForm(r.Context(), form.Mode)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "db error"})
		return
	}
	writeJSON(w, http.StatusOK, saved)
}
