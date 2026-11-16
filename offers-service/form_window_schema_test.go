package main

// Сторож схемы окна КП (2026-11-15, шаг 1 плана 0.11.0).
//
// Тот же класс дефекта, что у `sections` в 0.10.2: поле объявлено в модели и в
// `validateForm` — то есть выглядит готовым, — а колонки нет, и настройка
// теряется после «Сохранить форму» без единой ошибки. Здесь держатся все три
// места разом: объявлено в модели ↔ есть колонка ↔ участвует в SQL.

import (
	"regexp"
	"strings"
	"testing"
)

// TestOffersFormHasWindowColumn — `window_config` есть и в CREATE, и в ALTER.
func TestOffersFormHasWindowColumn(t *testing.T) {
	var creates, alters bool
	for _, stmt := range capturedMigrations(t) {
		flat := strings.Join(strings.Fields(stmt), " ")
		if strings.HasPrefix(flat, "CREATE TABLE IF NOT EXISTS offers_form") {
			if regexp.MustCompile(`(?i)\bwindow_config\s+JSONB NOT NULL DEFAULT '\[\]'`).MatchString(flat) {
				creates = true
			}
		}
		if regexp.MustCompile(`(?i)^ALTER TABLE offers_form ADD COLUMN IF NOT EXISTS window_config JSONB`).MatchString(flat) {
			alters = true
		}
	}
	if !creates {
		t.Error("в CREATE TABLE offers_form нет колонки window_config")
	}
	if !alters {
		t.Error("нет ALTER TABLE offers_form ADD COLUMN IF NOT EXISTS window_config: " +
			"на действующей базе колонка не появится, и окно КП снова будет теряться")
	}
}

// TestFormWindowInSaveAndLoad — `window_config` маршалится при записи и
// читается обратно. Обе половины обязательны: писать без чтения — та же потеря.
func TestFormWindowInSaveAndLoad(t *testing.T) {
	src := formSQL(t)
	save := functionBody(t, src, "func (s *Server) handleSaveForm(")
	load := functionBody(t, src, "func (s *Server) loadForm(")

	if !strings.Contains(save, "json.Marshal(window)") {
		t.Error("handleSaveForm не маршалит окно")
	}
	if !regexp.MustCompile(`(?s)INSERT INTO offers_form \([^)]*window_config`).MatchString(save) {
		t.Error("INSERT в handleSaveForm не перечисляет колонку window_config")
	}
	if !strings.Contains(save, "window_config = EXCLUDED.window_config") {
		t.Error("ON CONFLICT не обновляет window_config: правка окна не сохранится поверх старой строки")
	}
	if !strings.Contains(load, "&window") {
		t.Error("loadForm не разбирает window_config")
	}
	if !strings.Contains(load, "effectiveWindow(form)") {
		t.Error("loadForm не применяет effectiveWindow: неизвестное и недостающее не согласуется")
	}
	if !regexp.MustCompile(`(?s)SELECT [^` + "`" + `]*\bwindow_config\b`).MatchString(load) {
		t.Error("SELECT в loadForm не читает колонку window_config")
	}
	// WindowStored (2026-11-15): флаг «настройка была записана» ставится на
	// чтении. Без него клиент не отличает «настройки нет» от «всё скрыли», и
	// показывает бриф, которого никто не включал (класс 0.10.2).
	if !strings.Contains(load, "form.WindowStored = len(form.Window) > 0") {
		t.Error("loadForm не заполняет WindowStored: клиент не отличит умолчание от настройки")
	}
}
