package main

// Проверки чистых частей агента (2026-11-14, шаг P6): разбор хода модели,
// аргументы инструментов, извлечение текста документа. Без сети и без БД —
// именно эти правила решают «выполнено или человеку показали разметку».

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestParseAgentTurnToolCall(t *testing.T) {
	raw := `{"type":"tool_call","tool":"get_our_products","arguments":{"query":"мембрана"}}`
	turn, ok := parseAgentTurn(raw)
	if !ok {
		t.Fatalf("ход не разобран: %q", raw)
	}
	if turn.Type != "tool_call" || turn.Tool != "get_our_products" {
		t.Fatalf("не тот ход: %+v", turn)
	}
	if got := turn.Arguments["query"]; got != "мембрана" {
		t.Fatalf("аргумент потерян: %v", turn.Arguments)
	}
}

func TestParseAgentTurnFinal(t *testing.T) {
	turn, ok := parseAgentTurn(`{"type":"final","text":"Система не найдена, есть похожие."}`)
	if !ok {
		t.Fatal("финальный ход не разобран")
	}
	if turn.Type != "final" || !strings.Contains(turn.Text, "не найдена") {
		t.Fatalf("не тот финальный ход: %+v", turn)
	}
}

// Модель часто оборачивает JSON в ```json … ``` и говорит вокруг.
func TestParseAgentTurnFenceAndProse(t *testing.T) {
	raw := "Вот вызов:\n```json\n{\"type\":\"tool_call\",\"tool\":\"read_card\",\"arguments\":{\"noteId\":42}}\n```\nДальше."
	turn, ok := parseAgentTurn(raw)
	if !ok || turn.Tool != "read_card" {
		t.Fatalf("ход в заборе не разобран: %+v ok=%v", turn, ok)
	}
	if turn.Arguments["noteId"] != float64(42) {
		t.Fatalf("noteId потерян: %v", turn.Arguments)
	}
}

// Провайдер обрывает конец генерации на одну скобку — достраиваем.
func TestParseAgentTurnTruncatedJSON(t *testing.T) {
	raw := `{"type":"tool_call","tool":"get_documents","arguments":{"query":"протокол"}`
	turn, ok := parseAgentTurn(raw)
	if !ok {
		t.Fatalf("оборванный JSON не достроен: %q", raw)
	}
	if turn.Tool != "get_documents" {
		t.Fatalf("не тот инструмент: %+v", turn)
	}
}

// Разметка вызова в чужом формате — НЕ показывать человеку: переспрос.
func TestParseAgentTurnProtocolError(t *testing.T) {
	for _, raw := range []string{
		`{"tool":"get_our_products","arguments":{"query":"x"}}`, // нет "type"
		`<tool_calls><invoke name="get_our_products"></invoke></tool_calls>`,
		`Просто текстовый ответ без JSON`,
		``,
	} {
		if _, ok := parseAgentTurn(raw); ok {
			t.Fatalf("разметка/текст без протокола приняты как ход: %q", raw)
		}
	}
}

// Ход с tool_call без аргументов — законен (пустая карта, а не nil).
func TestParseAgentTurnNoArguments(t *testing.T) {
	turn, ok := parseAgentTurn(`{"type":"tool_call","tool":"build_docx"}`)
	if !ok || turn.Tool != "build_docx" {
		t.Fatalf("не разобран ход без аргументов: %+v ok=%v", turn, ok)
	}
	if turn.Arguments == nil {
		t.Fatal("аргументы должны быть пустой картой, а не nil")
	}
}

func TestArgStringAndInt(t *testing.T) {
	args := map[string]any{"query": " мембрана ", "noteId": float64(77), "offerId": "12"}
	if got := argString(args, "query"); got != "мембрана" {
		t.Fatalf("argString: %q", got)
	}
	if got := argInt(args, "noteId"); got != 77 {
		t.Fatalf("argInt number: %d", got)
	}
	if got := argInt(args, "offerId"); got != 12 {
		t.Fatalf("argInt string: %d", got)
	}
	if got := argInt(args, "missing"); got != 0 {
		t.Fatalf("отсутствующий аргумент: %d", got)
	}
}

func TestAgentToolsWhitelist(t *testing.T) {
	specs := (&Server{}).agentToolDescriptors()
	if len(specs) == 0 {
		t.Fatal("список инструментов пуст")
	}
	for _, spec := range specs {
		name, _ := spec["name"].(string)
		if name == "" {
			t.Fatalf("у инструмента нет имя: %+v", spec)
		}
		schema, _ := spec["input_schema"].(map[string]any)
		if schema["type"] != "object" {
			t.Fatalf("схема %s не объект: %+v", name, schema)
		}
	}
	// v1 не содержит инструментов записи YouGile и писем (P5b ещё нет).
	for _, forbidden := range []string{"create_yougile_task", "save_rule", "add_skill", "get_emails"} {
		for _, spec := range specs {
			if spec["name"] == forbidden {
				t.Fatalf("инструмент %q не должен быть в v1", forbidden)
			}
		}
	}
}

// Инструменты P6-бис на месте и с теми аргументами, на которые опирается промт
// (2026-11-14).
func TestAgentToolsAfterP6bis(t *testing.T) {
	tools := (&Server{}).agentTools()
	for _, name := range []string{"read_card", "search_competitors", "create_combat_sheet", "set_combat_side", "create_offer"} {
		if _, ok := tools[name]; !ok {
			t.Fatalf("инструмента %q нет в карте", name)
		}
	}
	required := func(tool string) []string {
		schema := tools[tool].InputSchema
		list, _ := schema["required"].([]string)
		return list
	}
	for _, key := range []string{"sheetId", "noteId", "side"} {
		found := false
		for _, item := range required("set_combat_side") {
			if item == key {
				found = true
			}
		}
		if !found {
			t.Fatalf("set_combat_side: аргумент %q не обязателен: %v", key, required("set_combat_side"))
		}
	}
}

// Роль карточки решают теги, и только теги (согласованная модель 2026-11-14).
func TestCardRoleByTags(t *testing.T) {
	tag := func(names ...string) []KBTag {
		out := make([]KBTag, 0, len(names))
		for _, name := range names {
			out = append(out, KBTag{Name: name})
		}
		return out
	}
	cases := []struct {
		tags []KBTag
		want string
	}{
		{tag("commercial_propose"), "system"},
		{tag("commercial_propose", "material"), "material"},
		{tag("material"), "material"},
		{tag("competitor"), "competitor"},
		{tag("competitor", "commercial_propose"), "competitor"},
		{tag("journal"), "other"},
		{tag(), "other"},
	}
	for _, c := range cases {
		if got := cardRole(c.tags); got != c.want {
			t.Fatalf("роль %v: получили %q, ждали %q", c.tags, got, c.want)
		}
	}
}

// Граница стороны листка — в инструменте: материал/система не попадают на
// сторону конкурента, конкурент — на нашу (2026-11-14, «инструмент — граница»).
func TestCombatSideRoleBoundary(t *testing.T) {
	if msg := combatSideRoleError(combatSideOur, "system"); msg != "" {
		t.Fatalf("система должна пускаться на нашу сторону: %s", msg)
	}
	if msg := combatSideRoleError(combatSideOur, "material"); msg != "" {
		t.Fatalf("материал должен пускаться на нашу сторону: %s", msg)
	}
	if msg := combatSideRoleError(combatSideOur, "competitor"); msg == "" {
		t.Fatal("конкурент не должен попадать на нашу сторону")
	}
	if msg := combatSideRoleError(combatSideCompetitor, "competitor"); msg != "" {
		t.Fatalf("конкурент должен пускаться на сторону конкурента: %s", msg)
	}
	for _, role := range []string{"system", "material", "other"} {
		if msg := combatSideRoleError(combatSideCompetitor, role); msg == "" {
			t.Fatalf("роль %q не должна попадать на сторону конкурента", role)
		}
	}
}

// `httptest.NewRequest` не разбирает параметры пути, а ручки читают `{id}`
// через `r.PathValue`: без проставления `SetPathValue` сборка документа руками
// агента отвечала «invalid id» на живом предложении (стенд поймал 2026-11-14,
// unit-тест его не видел).
func TestAgentRequestCarriesPathValue(t *testing.T) {
	env := &agentEnv{Email: "owner@tn.ru"}
	req := env.requestWith(context.Background(), http.MethodPost, "/api/offers/offers/42/export",
		map[string]string{"id": "42"})
	if got := req.PathValue("id"); got != "42" {
		t.Fatalf("параметр пути потерян: %q", got)
	}
	if req.Context().Value(permEmailCtx{}) != "owner@tn.ru" {
		t.Fatal("идентичность не доехала до ручки")
	}
}

func TestArgFloatAndBool(t *testing.T) {
	args := map[string]any{"qty": 3.5, "qtyStr": "10,5", "flag": true, "flagStr": "true", "off": false}
	if got := argFloat(args, "qty"); got != 3.5 {
		t.Fatalf("argFloat number: %v", got)
	}
	if got := argFloat(args, "qtyStr"); got != 10.5 {
		t.Fatalf("argFloat comma string: %v", got)
	}
	if !argBool(args, "flag") || !argBool(args, "flagStr") {
		t.Fatal("argBool не прочитал true")
	}
	if argBool(args, "off") || argBool(args, "missing") {
		t.Fatal("argBool прочитал true там, где false")
	}
}

func TestDocumentTextOfDocx(t *testing.T) {
	docx := buildTestDocx(t, `<w:p><w:r><w:t>Протокол испытаний</w:t></w:r></w:p><w:p><w:r><w:t>кровельной мембраны</w:t></w:r></w:p>`)
	text := documentTextOf("documents/abc/main-protokol.docx", docx)
	if !strings.Contains(text, "Протокол испытаний") || !strings.Contains(text, "кровельной мембраны") {
		t.Fatalf("текст DOCX не извлечён: %q", text)
	}
}

func TestDocumentTextOfPdfIsEmpty(t *testing.T) {
	// PDF в v1 честно даёт пустой текст: агент откроет его ссылкой.
	if text := documentTextOf("documents/x/main-scan.pdf", []byte("%PDF-1.7 junk")); text != "" {
		t.Fatalf("PDF не должен давать текст: %q", text)
	}
}

func TestDocumentTextOfPlain(t *testing.T) {
	if text := documentTextOf("documents/x/readme.md", []byte("  слово  ")); text != "слово" {
		t.Fatalf("простой текст не прочитан: %q", text)
	}
}

// buildTestDocx собирает минимальный .docx с заданным телом document.xml.
func buildTestDocx(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	file, err := writer.Create("word/document.xml")
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	xml := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body + `</w:body></w:document>`
	if _, err := file.Write([]byte(xml)); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}
