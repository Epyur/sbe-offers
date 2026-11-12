package main

// Разбор хода модели в протокол агента (2026-11-14, шаг P6).
//
// Формат — тот же, что у канона (`sbe-core/src/agent/engine.ts`, `parseTurns`):
// ровно один JSON-объект одной из двух форм:
//   {"type":"tool_call","tool":"<имя>","arguments":{…}}
//   {"type":"final","text":"<ответ>"}
//
// Отличие от клиентского канона: там ещё разбирается DSML-разметка DeepSeek.
// Здесь модель зовётся через llm-service, а форма ответа задаётся системным
// промтом; если модель всё же ответила разметкой, разбор вернёт «не понял» и
// движок переспросит в нашем формате — разметка человеку не показывается.
// Терпимость к обёрткам (```json, текст до/после) нужна всегда.

import (
	"encoding/json"
	"strings"
)

// agentProtocolReminder — просьба повторить ход в нашем формате. Текст тот же,
// что `PROTOCOL_REMINDER` канона: модель обучена отвечать на него.
const agentProtocolReminder = "Твой предыдущий ответ не распознан: он похож на вызов инструмента в чужом формате. " +
	"Ответь ЗАНОВО ОДНИМ JSON-объектом ровно одной из двух форм (поле \"type\" обязательно): " +
	"{\"type\":\"tool_call\",\"tool\":\"<имя тула>\",\"arguments\":{…}} или {\"type\":\"final\",\"text\":\"<ответ пользователю>\"}. " +
	"Формы без \"type\" и любая разметка вызовов (DSML, XML-теги, function_call) системой не принимаются."

// agentTurnReply — разобранный ход.
type agentTurnReply struct {
	Type      string
	Tool      string
	Arguments map[string]any
	Text      string
	Raw       string
}

// parseAgentTurn разбирает ответ модели. Второе значение — удалось ли найти
// ровно один объект нужной формы; false значит «переспросить модель».
func parseAgentTurn(raw string) (agentTurnReply, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return agentTurnReply{Raw: text}, false
	}
	// ```json … ``` — снять забор.
	if fence := fenceBody(text); fence != "" {
		text = fence
	}
	// Первый JSON-объект в тексте (модель может добавить фразу до/после).
	objStr := firstJSONObject(text)
	if objStr == "" {
		return agentTurnReply{Raw: text}, false
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(objStr), &obj); err != nil {
		return agentTurnReply{Raw: text}, false
	}
	reply := agentTurnReply{Raw: text}
	if kind, _ := obj["type"].(string); kind != "" {
		reply.Type = kind
	}
	switch reply.Type {
	case "final":
		if textValue, ok := obj["text"].(string); ok {
			reply.Text = textValue
			return reply, true
		}
		return reply, false
	case "tool_call":
		name, _ := obj["tool"].(string)
		reply.Tool = strings.TrimSpace(name)
		if reply.Tool == "" {
			return reply, false
		}
		if args, ok := obj["arguments"].(map[string]any); ok {
			reply.Arguments = args
		} else {
			reply.Arguments = map[string]any{}
		}
		return reply, true
	}
	// `type` нет, но есть имя инструмента в «вольной» форме — не принимаем и
	// переспрашиваем: угадывание формы и есть источник «выполнилось не то».
	return reply, false
}

// fenceBody — содержимое ```…``` блока, если он есть.
func fenceBody(text string) string {
	start := strings.Index(text, "```")
	if start < 0 {
		return ""
	}
	rest := text[start+3:]
	// Пропустить ярлык языка (`json`) до перевода строки.
	if nl := strings.IndexAny(rest, "\r\n"); nl >= 0 && !strings.ContainsAny(rest[:nl], "{}") {
		rest = rest[nl+1:]
	}
	end := strings.Index(rest, "```")
	if end < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:end])
}

// firstJSONObject — подстрока первого сбалансированного `{…}` с учётом строк.
func firstJSONObject(text string) string {
	start := strings.Index(text, "{")
	if start < 0 {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(text); i++ {
		ch := text[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}
	// Обрыв на одну скобку — достраиваем (провайдер режет конец генерации).
	if depth > 0 {
		return text[start:] + strings.Repeat("}", depth)
	}
	return ""
}
