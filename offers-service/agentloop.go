package main

// Цикл агента КП на сервере (2026-11-14, шаг P6).
//
// Канон цикла — `sbe-core/src/agent/engine.ts` (P4): те же правила (план →
// tool_call → observe → final, починка протокола, доведение хода до файла,
// анти-цикл, лимит шагов). Здесь его серверная часть для бота: цикл в боте
// вызывал бы этот же движок, но у бота нет модели — модель и инструменты живут
// в offers-service. Поэтому сервер исполняет ход сам, а бот только отдаёт текст
// человека и присылает ответ.
//
// Почему на Go, а не «бот позовёт движок и потом сервис за каждым инструментом»:
// так инструменты исполняются там, где данные и права (`offers-service`), без
// второй сессии и без гонки «кто держит диалог». Тот же результат, что у
// клиентского движка, достигается теми же шагами — сторож
// `plugins/scripts/check_agent_engine.mjs` держит совпадение правил, а этот файл
// им и руководствуется.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// Правила хода — те же числа, что в каноне движка. Изменение здесь без
// изменения канона расходится молча, поэтому они повторяют имена и значения
// `engine.ts`.
const (
	agentMaxIterations    = 15
	agentProtocolRetries  = 2
	agentConsecutiveTools = 5
	agentToolDataChars    = 30000
	agentToolHistory      = 15000
	agentHistoryCompact   = 50000
	agentKeepRecent       = 8
)

// agentTurn — один ход: текст человека и ответ агента (то, что бот вернёт в чат).
type agentTurn struct {
	Answer string      `json:"answer"`
	Files  []agentFile `json:"-"`
}

// agentRequest — вход хода.
type agentRequest struct {
	Email  string
	ChatID string
	Text   string
	// OfferID/SheetID — документ, о котором идёт речь (необязательно: агент
	// может работать и без привязанного документа, например «найди мембрану»).
	OfferID int64
	SheetID int64
}

// runAgentTurn исполняет один ход агента и пишет его в журнал.
//
// Всё, что пишется в журнал (запрос, инструменты, ответ, ручные правки), — это
// то, что человек и так видит, плюс состав до/после правки; содержимого, кроме
// этого, журнал не держит (дизайн §«The manager-analysis journal»).
func (s *Server) runAgentTurn(ctx context.Context, req agentRequest, envelope *agentEnvelope) (agentTurn, error) {
	started := time.Now()
	env := &agentEnv{Ctx: ctx, Server: s, Email: req.Email, ChatID: req.ChatID}
	tools := s.agentTools()
	logEntry := agentLogEntry{
		ChatID:   req.ChatID,
		Email:    req.Email,
		OfferID:  req.OfferID,
		SheetID:  req.SheetID,
		UserText: req.Text,
		Model:    agentModelName(),
	}

	transcript := s.agentTranscript(req)
	system := s.agentSystemPrompt()

	var toolsCalled []agentToolCall
	var answer string
	var reason = "final"

	for step := 0; step < agentMaxIterations; step++ {
		raw, err := Complete(ctx, system, transcript, 0)
		if err != nil {
			log.Printf("агент: модель не ответила: %v", err)
			answer = "Модель сейчас не ответила. Попробуйте ещё раз чуть позже."
			reason = "error"
			break
		}
		turn, ok := parseAgentTurn(raw)
		if !ok {
			// Разметка не разобрана — просим модель повторить в нашем формате,
			// пока не кончатся попытки. Человеку разметка не показывается.
			if step < agentProtocolRetries {
				transcript += "\n[Система] " + agentProtocolReminder + "\nТвой ход (только JSON):"
				continue
			}
			answer, _ = synthesizeAgentAnswer(ctx, system, transcript, toolsCalled)
			reason = "protocol"
			break
		}
		if turn.Type == "final" {
			answer = strings.TrimSpace(turn.Text)
			if answer == "" {
				answer = "Готово."
			}
			break
		}

		// tool_call
		tool, exists := tools[turn.Tool]
		if !exists {
			toolsCalled = append(toolsCalled, agentToolCall{Name: turn.Tool, OK: false, Summary: "неизвестный инструмент"})
			transcript += fmt.Sprintf("\n[Результат тула %s (ошибка)] Неизвестный инструмент «%s»\nТвой ход (только JSON):", turn.Tool, turn.Tool)
			continue
		}
		result, err := tool.Run(ctx, env, turn.Arguments)
		if err != nil {
			result = agentToolResult{OK: false, Error: err.Error()}
		}
		toolsCalled = append(toolsCalled, agentToolCall{
			Name: turn.Tool, Arguments: turn.Arguments, OK: result.OK, Summary: agentToolSummary(result),
		})
		// Слишком много одинаковых вызовов подряд — сворачиваем в ответ по
		// найденному (анти-цикл канона: не выбрасывать найденное).
		if tooManyToolCalls(toolsCalled, turn.Tool) {
			answer, _ = synthesizeAgentAnswer(ctx, system, transcript, toolsCalled)
			reason = "synthesis"
			break
		}
		transcript += fmt.Sprintf("\n%s\nТвой ход (только JSON):", agentToolForModel(turn.Tool, result))
	}

	if answer == "" {
		answer, _ = synthesizeAgentAnswer(ctx, system, transcript, toolsCalled)
		reason = "synthesis"
	}

	turn := agentTurn{Answer: answer, Files: env.Files}

	logEntry.Tools = toolsCalled
	logEntry.Answer = answer
	logEntry.ManualFix = env.Fixes
	logEntry.LatencyMs = int(time.Since(started).Milliseconds())
	logEntry.Reason = reason
	s.writeAgentLog(ctx, logEntry, envelope)

	return turn, nil
}

// agentToolCall — инструмент в журнале хода.
type agentToolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
	OK        bool           `json:"ok"`
	Summary   string         `json:"summary"`
}

// synthesizeAgentAnswer — последний ход без инструментов: модель собирает ответ
// по уже найденному. Тот же приём, что `finishWithSynthesis` в каноне.
func synthesizeAgentAnswer(ctx context.Context, system, transcript string, calls []agentToolCall) (string, bool) {
	note := "\n[Система] Дальнейшие вызовы инструментов недоступны. Сформулируй итоговый ответ пользователю " +
		"ТОЛЬКО на основе данных, уже полученных выше. Не выдумывай данные. Ответь обычным текстом, без JSON.\n"
	raw, err := Complete(ctx, system, transcript+note, 0)
	if err != nil {
		if len(calls) == 0 {
			return "Не удалось ничего найти. Уточните, пожалуйста, запрос.", false
		}
		return "Не удалось закончить ответ. Попробуйте ещё раз, пожалуйста.", false
	}
	if turn, ok := parseAgentTurn(raw); ok && turn.Type == "final" && strings.TrimSpace(turn.Text) != "" {
		return strings.TrimSpace(turn.Text), true
	}
	return strings.TrimSpace(raw), false
}

// tooManyToolCalls — больше N подряд одинаковых вызовов одного инструмента.
func tooManyToolCalls(calls []agentToolCall, name string) bool {
	count := 0
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].Name != name {
			break
		}
		count++
	}
	return count > agentConsecutiveTools
}

// agentToolForModel — строка результата для модели, с данными в JSON и
// ограничением размера (как `summaryForLlm` в каноне).
func agentToolForModel(name string, result agentToolResult) string {
	if !result.OK {
		return fmt.Sprintf("[Результат тула %s (ошибка)] %s", name, result.Error)
	}
	line := fmt.Sprintf("[Результат тула %s (ok)] %s", name, result.Summary)
	if result.Data == nil {
		return line
	}
	data, err := json.Marshal(result.Data)
	if err != nil {
		return line
	}
	if len(data) > agentToolDataChars {
		return fmt.Sprintf("%s\nДанные слишком большие (%d символов, лимит %d) — уточни запрос или лимит.",
			line, len(data), agentToolDataChars)
	}
	return line + "\nДанные: " + string(data)
}

// agentToolSummary — короткая сводка для журнала.
func agentToolSummary(result agentToolResult) string {
	text := result.Summary
	if !result.OK {
		text = result.Error
	}
	if len(text) > 500 {
		text = text[:500]
	}
	return text
}

// agentModelName — имя модели из окружения (для журнала). Пусто — «по умолчанию».
func agentModelName() string {
	return strings.TrimSpace(getEnv("OFFERS_AGENT_MODEL"))
}

// tooLong — обрезка длинной строки для журнала, чтобы одна запись не тащила
// мегабайты.
func clipText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "…"
}
