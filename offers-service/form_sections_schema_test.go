package main

// Сторож схемы формы (2026-11-14, дефект 0.10.2 «раздел не сохраняется»).
//
// Живой симптом: администратор создавал «Раздел», нажимал «Сохранить форму» —
// и раздел исчезал. Причина была не в одной забытой строке, а в трёх местах
// сразу: у `offers_form` не было колонки `sections`, `handleSaveForm` её не
// писал, `loadForm` не читал. Ошибки при этом не было ни одной — это тихая
// потеря данных (тот же класс, что `input.value=''` в 0.1.18).
//
// Проверки ниже держат все три места разом. Миграции прогоняются в память, без
// PostgreSQL: тесту нужен не сервер, а ответ на вопрос «обещает ли схема
// сохранить то, что сервис пишет и читает».
//
// Почему это вообще тест, а не ревью: поле было объявлено в модели и в
// `validateForm` — то есть выглядело готовым. Только сверка «объявлено в
// модели» ↔ «есть колонка» ↔ «участвует в SQL» ловит такой разрыв.

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// capturedMigrations — что реально выполнил бы `migrate`.
func capturedMigrations(t *testing.T) []string {
	t.Helper()
	var got []string
	exec := func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
		got = append(got, sql)
		return pgconn.CommandTag{}, nil
	}
	if err := (&Server{}).runMigrations(context.Background(), exec); err != nil {
		t.Fatalf("миграции: %v", err)
	}
	return got
}

// TestOffersFormHasSectionsColumn — колонка `sections` есть и в CREATE, и в
// ALTER: первое — для новой базы, второе — для давно работающей (на боевой
// `offers_form` создана ещё до разделов).
func TestOffersFormHasSectionsColumn(t *testing.T) {
	var creates, alters bool
	for _, stmt := range capturedMigrations(t) {
		flat := strings.Join(strings.Fields(stmt), " ")
		if strings.HasPrefix(flat, "CREATE TABLE IF NOT EXISTS offers_form") {
			if regexp.MustCompile(`(?i)\bsections\s+JSONB NOT NULL DEFAULT '\[\]'`).MatchString(flat) {
				creates = true
			}
		}
		if regexp.MustCompile(`(?i)^ALTER TABLE offers_form ADD COLUMN IF NOT EXISTS sections JSONB`).MatchString(flat) {
			alters = true
		}
	}
	if !creates {
		t.Error("в CREATE TABLE offers_form нет колонки sections")
	}
	if !alters {
		t.Error("нет ALTER TABLE offers_form ADD COLUMN IF NOT EXISTS sections: " +
			"на действующей базе колонка не появится, и раздел снова будет теряться")
	}
}

// formSQL — тела `loadForm` и `handleSaveForm` из исходника.
//
// Берётся исходник, а не поведение: держать эту проверку на живой БД значило бы
// требовать Postgres в каждом `go test`. Зато проверка читает ровно тот текст,
// который уходит в базу, — включая список колонок, а не только аргументы.
func formSQL(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("form.go")
	if err != nil {
		t.Fatalf("form.go: %v", err)
	}
	return string(raw)
}

// TestFormSectionsInSaveAndLoad — `sections` маршалится при записи и читается
// обратно. Обе половины обязательны: писать без чтения — та же потеря.
func TestFormSectionsInSaveAndLoad(t *testing.T) {
	src := formSQL(t)
	save := functionBody(t, src, "func (s *Server) handleSaveForm(")
	load := functionBody(t, src, "func (s *Server) loadForm(")

	if !strings.Contains(save, "json.Marshal(form.Sections)") {
		t.Error("handleSaveForm не маршалит form.Sections")
	}
	if !regexp.MustCompile(`(?s)INSERT INTO offers_form \([^)]*sections`).MatchString(save) {
		t.Error("INSERT в handleSaveForm не перечисляет колонку sections")
	}
	if !strings.Contains(save, "sections = EXCLUDED.sections") {
		t.Error("ON CONFLICT не обновляет sections: правка раздела не сохранится поверх старой строки")
	}
	if !strings.Contains(load, "&form.Sections") {
		t.Error("loadForm не разбирает sections в form.Sections")
	}
	if !regexp.MustCompile(`(?s)SELECT [^` + "`" + `]*\bsections\b`).MatchString(load) {
		t.Error("SELECT в loadForm не читает колонку sections")
	}
}

// functionBody — текст функции до её закрывающей скобки на нулевом отступе.
// Простая склейка по «\n}» достаточна: файл выровнен gofmt.
func functionBody(t *testing.T, src, signature string) string {
	t.Helper()
	start := strings.Index(src, signature)
	if start < 0 {
		t.Fatalf("не найдена функция %q", signature)
	}
	rest := src[start:]
	if end := strings.Index(rest, "\n}\n"); end >= 0 {
		return rest[:end]
	}
	return rest
}
