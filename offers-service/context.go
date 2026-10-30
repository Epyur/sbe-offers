package main

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"mailers/offers/model"
)

// Контекст продукта для скилов (product-marketing-context, 2026-10-29).
// Типы — в пакете model; здесь SQL и посев.
//
// Короткий набор из спеки: без него промпты секций пишут «общими словами» —
// модель не знает, что компания продаёт, какие у клиентов боли и что мы НЕ
// обещаем. Длинный набор из исходного скила (13 разделов) сознательно не взят:
// его никто не заполнит, а пустой раздел хуже отсутствующего.

// validateContext — что можно записать. Возвращает текст ошибки или пусто.
func validateContext(c model.ProductContext) string {
	for key := range c.Sections {
		if !model.IsContextKey(key) {
			return "неизвестный раздел контекста: " + key
		}
	}
	for i, l := range c.Learnings {
		num := itoa(i + 1)
		if !model.NonEmpty(l.Learning) {
			return "в выводе №" + num + " не заполнено, что именно узнали"
		}
		if !model.NonEmpty(l.Date) {
			return "в выводе №" + num + " нет даты"
		}
		if l.Status != "" && !model.ValidLearningStatus(l.Status) {
			return "в выводе №" + num + " неизвестный статус: " + l.Status
		}
	}
	return ""
}

// itoa без strconv: используется только в текстах ошибок.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// loadContext читает контекст режима. Нет строки — отдаёт дефолтный: контекст
// не обязателен, без него работа продолжается.
func (s *Server) loadContext(ctx context.Context, mode string) (model.ProductContext, error) {
	if mode == "" {
		mode = "client"
	}
	var c model.ProductContext
	var sections, learnings []byte
	err := s.pool.QueryRow(ctx, `
SELECT id, mode, sections, learnings, updated_by,
	to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.US' || 'Z')
FROM offers_context WHERE mode = $1`, mode).
		Scan(&c.ID, &c.Mode, &sections, &learnings, &c.UpdatedBy, &c.UpdatedAt)
	if err == pgx.ErrNoRows {
		return model.DefaultContext(mode), nil
	}
	if err != nil {
		return c, err
	}
	c.Sections = map[string]string{}
	c.Learnings = []model.Learning{}
	if len(sections) > 0 {
		if err := json.Unmarshal(sections, &c.Sections); err != nil {
			return c, err
		}
	}
	if len(learnings) > 0 {
		if err := json.Unmarshal(learnings, &c.Learnings); err != nil {
			return c, err
		}
	}
	model.NormalizeContext(&c)
	return c, nil
}

// saveContext пишет контекст и двигает updated_at.
func (s *Server) saveContext(ctx context.Context, c model.ProductContext, updatedBy string) error {
	sections, err := json.Marshal(c.Sections)
	if err != nil {
		return err
	}
	learnings, err := json.Marshal(c.Learnings)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO offers_context (mode, sections, learnings, updated_by, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (mode) DO UPDATE SET
	sections = EXCLUDED.sections, learnings = EXCLUDED.learnings,
	updated_by = EXCLUDED.updated_by, updated_at = now()`,
		c.Mode, sections, learnings, updatedBy)
	return err
}

// seedContext создаёт пустую строку контекста при первом старте. Пустая строка
// в БД — это НАСТРОЕННЫЙ пустой контекст; администратор видит разделы, а не
// ошибку «контекст не найден». Уже существующую строку не трогает.
func (s *Server) seedContext(ctx context.Context) error {
	sections, err := json.Marshal(model.DefaultContext("client").Sections)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO offers_context (mode, sections, learnings) VALUES ('client', $1, '[]')
ON CONFLICT (mode) DO NOTHING`, sections)
	return err
}
