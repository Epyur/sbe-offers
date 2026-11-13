package main

// Инструменты агента КП (2026-11-14, шаг P6 плана sbe-offers).
//
// Модель, прежде чем код (правило 2026-11-11): агент НЕ четвёртая сущность рядом
// с КП и боевым листком — он СБОРЩИК. У него есть только те действия, которые
// менеджер делает кнопками; каждое действие вызывает готовую функцию сервиса, а
// не повторяет её. Дизайн — docs/superpowers/specs/2026-11-13-sbe-offers-single-agent-design.md
// (§«The agent's tools are the service's own code paths»).
//
// Почему whitelist, а не «любая ручка по имени». Инструмент — это ГРАНИЦА, а не
// подсказка в промте (ub-mode: «проверяйте данные там, где они входят»). Модель
// не может назвать путь, которого нет в карте ниже; неизвестное имя получает
// честный отказ и уходит обратно в диалог. Отдельно от прав: гейт человека —
// в транспорте (JWT или сервисный секрет бота), права инструмента — в тех
// функциях, которые он зовёт.
//
// Чистая часть (схемы, разбор аргументов, сводки) отделена от сети: её проверяют
// тесты без БД и без модели. Исполнение получает `agentEnv` — то, что даёт
// транспорт (контекст, автор, chat_id, запрос), и сервис.

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// agentToolSpec — один инструмент: имя, описание для модели, JSON Schema
// аргументов и исполнитель. `Schema` — в той же форме, что ждёт движок
// (`AgentToolSchema` в sbe-core/src/agent/engine.ts): name/description/input_schema.
type agentToolSpec struct {
	Name        string
	Description string
	InputSchema map[string]any
	Run         func(ctx context.Context, env *agentEnv, args map[string]any) (agentToolResult, error)
}

// agentToolResult — результат вызова в терминах движка: ok/summary/data/link и
// текст ошибки для случая ok=false. `Data` уезжает в модель как JSON (движок
// сам ограничивает его размер), `Summary` — то, что человек видит строкой.
type agentToolResult struct {
	OK      bool       `json:"ok"`
	Summary string     `json:"summary"`
	Data    any        `json:"data,omitempty"`
	Error   string     `json:"error,omitempty"`
	Link    *agentLink `json:"link,omitempty"`
}

// agentLink — ссылка, которую движок печатает кнопкой (файл DOCX, карточка).
type agentLink struct {
	URL   string `json:"url"`
	Label string `json:"label"`
}

// agentEnv — что транспорт даёт инструментам. `Email` — кто спрашивает (его
// правами действуют записи), `ChatID` — для журнала. `Files` собирает готовые
// документы: бот пересылает их в чат, а движок видит только подпись.
type agentEnv struct {
	Ctx    context.Context
	Server *Server
	Email  string
	ChatID string
	// Files — готовые документы за ход (`имя` → байты). В модель не идут.
	Files []agentFile
	// Fixes — ручные правки строки за ход (`manual_fix` журнала): что было и
	// что стало. В модель не идут; это материал разбора переписки менеджеров.
	Fixes []agentManualFix
	// visibleDocs — видимый набор документов на ход (письма проверяют по нему
	// привязку к продукту). Кэш на ОДИН ход, а не на сервер: привязку могли
	// поставить секунду назад, и «письмо не появилось» в тот же миг — задержка
	// кэша документов, а не ошибка. `ready` отличает «не спрашивали» от
	// «спросили, пришёл пустой набор».
	visibleDocs      []AgentDoc
	visibleDocsReady bool
}

// agentFile — готовый документ за ход.
type agentFile struct {
	Name  string `json:"name"`
	Bytes []byte `json:"-"`
}

// addFile запоминает готовый документ за ход.
func (env *agentEnv) addFile(name string, data []byte) {
	env.Files = append(env.Files, agentFile{Name: name, Bytes: data})
}

// agentManualFix — правка строки документом агента, записанная в журнал. Это не
// индекс и не diff: имя колонки и состав до/после — ровно то, что человек потом
// правил руками (дизайн §«The manager-analysis journal»).
type agentManualFix struct {
	Column string `json:"column"`
	RowID  int64  `json:"rowId"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

// noteFix записывает правку строки в журнал хода. Пустая разница не пишется:
// повторный выбор той же системы правкой не является.
func (env *agentEnv) noteFix(column string, rowID int64, before, after any) {
	if fmt.Sprint(before) == fmt.Sprint(after) {
		return
	}
	env.Fixes = append(env.Fixes, agentManualFix{Column: column, RowID: rowID, Before: before, After: after})
}

// agentTools — карта инструментов. Порядок не важен: движок ищет по имени, а
// промт строит список из этой же карты.
//
// Чего здесь НЕТ и почему:
//   - YouGile, правила, навыки, презентации — не входят в scope;
//   - `list_offers`/`get_offer` — менеджер и так видит их в приложении;
//     агенту нужен результат, а не второй список.
//
// P6-бис (2026-11-14): `read_card` читает ЛЮБУЮ заметку (дефект 0.9.22 —
// карточки материалов «не открывались»), добавлены `search_competitors`,
// `create_combat_sheet`, `set_combat_side` и `create_offer` (агент больше не
// упирается в «предложение не найдено» без id).
//
// P5b (2026-11-14): `get_emails` и `read_email` — письма, но видимые ТОЛЬКО
// через привязку карточки скана к продукту. Видимость письма — не «есть PDF»
// и не совпадение названия, а `product_refs` карточки (agentmail.go).
func (s *Server) agentTools() map[string]agentToolSpec {
	tools := []agentToolSpec{
		s.toolGetOurProducts(),
		s.toolReadCard(),
		s.toolSearchCompetitors(),
		s.toolGetDocuments(),
		s.toolGetDocumentLink(),
		s.toolReadDocumentFile(),
		s.toolGetEmails(),
		s.toolReadEmail(),
		s.toolCreateOffer(),
		s.toolSetSystem(),
		s.toolSetMaterial(),
		s.toolCreateCombatSheet(),
		s.toolSetCombatSide(),
		s.toolBuildDocx(),
		s.toolBuildCombatDocx(),
	}
	out := make(map[string]agentToolSpec, len(tools))
	for _, tool := range tools {
		out[tool.Name] = tool
	}
	return out
}

// agentToolDescriptors — схемы инструментов для движка (в порядке карты, чтобы
// ответ был стабилен между прогонами). Движок просит именно этот вид.
func (s *Server) agentToolDescriptors() []map[string]any {
	order := []string{
		"get_our_products", "read_card", "search_competitors",
		"get_documents", "get_document_link", "read_document_file",
		"get_emails", "read_email",
		"create_offer", "set_system", "set_material",
		"create_combat_sheet", "set_combat_side",
		"build_docx", "build_combat_docx",
	}
	tools := s.agentTools()
	out := make([]map[string]any, 0, len(order))
	for _, name := range order {
		tool, ok := tools[name]
		if !ok {
			continue
		}
		out = append(out, map[string]any{
			"name":         tool.Name,
			"description":  tool.Description,
			"input_schema": tool.InputSchema,
		})
	}
	return out
}

// ============================ helpers ============================

// argString вынимает строку; отсутствующее и не-строку даёт "".
func argString(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}

// argInt вынимает целое; принимает число, строку с числом, пустое — 0.
func argInt(args map[string]any, key string) int64 {
	v, ok := args[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int:
		return int64(t)
	case int64:
		return t
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		if err != nil {
			return 0
		}
		return n
	}
	return 0
}

// argFloat вынимает дробное; принимает число и строку с числом, иначе 0.
func argFloat(args map[string]any, key string) float64 {
	v, ok := args[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(strings.ReplaceAll(t, ",", ".")), 64)
		if err != nil {
			return 0
		}
		return f
	}
	return 0
}

// argBool вынимает флаг; отсутствующее — false. Строка «true»/"1" читается как
// true: модель иногда присылает флаг строкой.
func argBool(args map[string]any, key string) bool {
	v, ok := args[key]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "1", "да":
			return true
		}
	}
	return false
}

// objectSchema — JSON Schema объекта с перечисленными полями. Одна функция на
// все схемы: у инструментов одна форма (объект с обязательными и
// необязательными полями), и повторять её руками значило бы держать три копии
// правила.
func objectSchema(required []string, props map[string]any) map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}

// stringProp — свойство-строка.
func stringProp(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

// intProp — свойство-целое.
func intProp(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

// errTool — отказ инструмента: текстом для модели и человека. Не паника и не
// 500: движок должен получить «не вышло и почему» и продолжить ход.
func errTool(format string, a ...any) (agentToolResult, error) {
	return agentToolResult{OK: false, Error: fmt.Sprintf(format, a...)}, nil
}
