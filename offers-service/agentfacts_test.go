package main

// Тесты границы инструмента `remember_sheet_fact` (шаг 3e, 2026-11-17).
//
// Инструмент записывает факт в чужой документ, поэтому проверяется то, что
// отказывает: источник, которого не читали; текст, которого нет; листок не
// автора. Разрешающая часть проверена косвенно — через `persistCombatFacts`
// (юнит-тесты `combat_facts_test.go`) и на стенде.
//
// Отказ инструмента приходит в РЕЗУЛЬТАТЕ (`OK:false`, `Error`), а не второй
// величиной: так устроен `errTool` во всём наборе, и это проверяется здесь же —
// иначе «отказал» легко спутать с «прошло».

import (
	"context"
	"strings"
	"testing"
)

// factServerWithoutDB — сервер без базы: `loadCombatSheet` падает, поэтому
// инструмент обязан честно ответить «листка нет», а не «факт записан».
func factServerWithoutDB() *Server { return &Server{} }

func factRefused(t *testing.T, result agentToolResult, err error, want string) {
	t.Helper()
	if err != nil {
		t.Fatalf("неожиданная ошибка исполнения: %v", err)
	}
	if result.OK {
		t.Fatalf("инструмент промолчал и принял: %+v", result)
	}
	if want != "" && !strings.Contains(result.Error, want) {
		t.Fatalf("отказ не называет причину %q: %q", want, result.Error)
	}
}

func TestRememberSheetFactRequiresKnownSource(t *testing.T) {
	server := factServerWithoutDB()
	env := &agentEnv{Ctx: context.Background(), Server: server, Email: "owner@tn.ru"}
	tool := server.toolRememberSheetFact()

	// Источник не читался вовсе.
	result, err := tool.Run(context.Background(), env, map[string]any{
		"sheetId": float64(1), "source": "https://example.com/spec",
	})
	factRefused(t, result, err, "не читался")

	// Прочитано другое — отказ по-прежнему.
	env.noteRead("ссылка", "https://example.com/other", "Что-то другое")
	result, err = tool.Run(context.Background(), env, map[string]any{
		"sheetId": float64(1), "source": "https://example.com/spec",
	})
	factRefused(t, result, err, "не читался")
}

// Источник находится по НОРМАЛИЗОВАННОМУ имени: модель может повторить адрес
// другим регистром, и это не должно превращаться в «не читали».
func TestRememberSheetFactFindsSourceCaseInsensitive(t *testing.T) {
	server := factServerWithoutDB()
	env := &agentEnv{Ctx: context.Background(), Server: server, Email: "owner@tn.ru"}
	env.noteRead("ссылка", "https://example.com/Spec", "Толщина 1,5 мм")
	tool := server.toolRememberSheetFact()

	result, err := tool.Run(context.Background(), env, map[string]any{
		"sheetId": float64(1), "source": "HTTPS://EXAMPLE.COM/spec",
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка исполнения: %v", err)
	}
	if strings.Contains(result.Error, "не читался") {
		t.Fatalf("источник не найден из-за регистра: %q", result.Error)
	}
	// Базы нет — значит до записи инструмент доходит и честно отвечает,
	// что записать некуда. Это и есть доказательство, что источник найден.
	if result.OK {
		t.Fatal("без базы факт «записан»")
	}
}

// Пустое прочитанное не становится источником: «прочитано, но текста нет» —
// не факт.
func TestRememberSheetFactIgnoresEmptyReads(t *testing.T) {
	server := factServerWithoutDB()
	env := &agentEnv{Ctx: context.Background(), Server: server, Email: "owner@tn.ru"}
	env.noteRead("ссылка", "https://example.com/empty", "   ")
	if _, ok := env.readSources[foldText("https://example.com/empty")]; ok {
		t.Fatal("пустой текст прочитанного запомнен источником")
	}
}

// Обязательные аргументы — sheetId и source: отказ обязан называть, чего нет.
func TestRememberSheetFactRequiresArguments(t *testing.T) {
	server := factServerWithoutDB()
	env := &agentEnv{Ctx: context.Background(), Server: server}
	tool := server.toolRememberSheetFact()

	result, err := tool.Run(context.Background(), env, map[string]any{"source": "x"})
	factRefused(t, result, err, "sheetId")
	result, err = tool.Run(context.Background(), env, map[string]any{"sheetId": float64(1)})
	factRefused(t, result, err, "source")
}

// Разбор файла в факт: у таблиц строки с разделителем, у текста — сам текст,
// а пустой разбор фактом не становится.
func TestParseFileTextForFacts(t *testing.T) {
	table := AgentParseResult{Kind: "xlsx", Sheets: []map[string]any{{
		"name": "Смета",
		"rows": []any{[]any{"1", "Мембрана"}, []any{"2", "Утеплитель"}},
	}}}
	text := parseFileText(table)
	for _, want := range []string{"Лист Смета:", "1 | Мембрана", "2 | Утеплитель"} {
		if !strings.Contains(text, want) {
			t.Fatalf("в тексте таблицы нет %q:\n%s", want, text)
		}
	}
	if got := parseFileText(AgentParseResult{Kind: "docx", Text: "  текст  "}); got != "  текст  " {
		t.Fatalf("текст файла подменён: %q", got)
	}
	if got := parseFileText(AgentParseResult{Kind: "pdf"}); got != "" {
		t.Fatalf("скан без текстового слоя стал текстом: %q", got)
	}
}
