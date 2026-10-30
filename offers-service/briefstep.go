package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"mailers/offers/model"
	"mailers/offers/skills"
)

// Оркестрация бриф-диалога (Task 5 плана 2026-10-29).
//
// Чистая функция: сеть и БД снаружи, через Completer и вызывающего. Поэтому
// поведение «что записать и что спросить дальше» проверяется без HTTP, а ручка
// в botapi.go остаётся тонкой.

// Completer — модель в одном вызове. Совпадает по смыслу с Complete, но
// отдельным типом: тесты подставляют функцию, а не HTTP.
type Completer interface {
	Complete(ctx context.Context, system, user string, maxTokens int) (string, error)
}

// llmCompleter — боевая реализация поверх llmclient.go.
type llmCompleter struct{}

func (llmCompleter) Complete(ctx context.Context, system, user string, maxTokens int) (string, error) {
	return Complete(ctx, system, user, maxTokens)
}

// BriefStep — результат одного шага разговора.
type BriefStep struct {
	// Payload — бриф после шага. При любой ошибке модели — прежний.
	Payload model.BriefPayload
	// Reply — что сказать менеджеру.
	Reply string
	// Done — все темы закрыты, можно переходить к расчёту.
	Done bool
	// NextTopic — ключ темы, которую бот спросит следующей (пусто, когда Done).
	NextTopic string
	// Skipped — темы, закрытые пустым значением («не знаю»/«пропустить»).
	Skipped []string
	// Retry — модель ответила непонятно или упала; имеет смысл повторить.
	Retry bool
	// Unavailable — модель не настроена: работаем без текста, бриф не растёт.
	Unavailable bool
}

// ответ модели: {"payload": {…}, "reply": "…", "skip": ["budget_signal"]}.
// skip — темы, которые менеджер отказался отвечать («не знаю», «пропустим»).
// Без явного признака пропуск неотличим от «модель забыла поле»: значение
// пусто и до, и после шага.
type briefModelAnswer struct {
	Payload json.RawMessage `json:"payload"`
	Reply   string          `json:"reply"`
	Skip    []string        `json:"skip"`
}

// StepBrief выполняет один шаг: отправляет реплику менеджера модели, забирает
// обновлённый бриф и решает, что спросить дальше.
func StepBrief(ctx context.Context, brief model.BriefPayload, managerText string, c Completer) (BriefStep, error) {
	system := skills.RenderBriefSystem(brief, model.ProductContext{})
	user := buildBriefUserPrompt(brief, managerText)

	raw, err := c.Complete(ctx, system, user, 1200)
	if err != nil {
		return briefFailure(brief, err), nil
	}

	answer, parseErr := parseBriefAnswer(raw)
	if parseErr != nil {
		step := briefFailure(brief, nil)
		step.Reply = "Не разобрал ответ. Повторите, пожалуйста, последнюю мысль."
		return step, nil
	}

	next := mergeBrief(brief, answer.Payload)
	if len(answer.Skip) > 0 {
		next.Skipped = mergeSkipped(next.Skipped, answer.Skip)
	}
	step := finishStep(next)
	step.Skipped = next.Skipped

	// Реплику берём у модели, но если она её забыла — собираем сами:
	// данные брифа важнее фразы. Терять шаг из-за отсутствующего поля нельзя.
	// ВАЖНО: reply модели — «подтверждение и следующий вопрос», собранный
	// нами — тоже, формат один.
	if strings.TrimSpace(answer.Reply) != "" {
		step.Reply = strings.TrimSpace(answer.Reply)
	}
	return step, nil
}

// briefFailure — шаг без изменения брифа.
func briefFailure(brief model.BriefPayload, err error) BriefStep {
	step := finishStepFor(brief)
	step.Retry = true
	switch {
	case err == nil:
		// разбор не удался — caller уже выставил свой текст
	case errors.Is(err, errLLMNotConfigured):
		step.Retry = false
		step.Unavailable = true
		step.Reply = "Модель сейчас недоступна — бриф можно заполнить позже. " +
			"Предложение собирается и без него."
	default:
		step.Reply = "Модель не ответила. Нажмите «Дальше», чтобы повторить, " +
			"или «Пропустить бриф» и собирайте КП без текста."
	}
	return step
}

// parseBriefAnswer разбирает ответ модели терпимо (jsonrepair.go): лишний
// текст, ```json, обрезка — всё переживаем.
func parseBriefAnswer(raw string) (briefModelAnswer, error) {
	var answer briefModelAnswer
	value, _, err := extractJSON(raw)
	if err != nil {
		return answer, err
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return answer, fmt.Errorf("ожидали объект, получили %T", value)
	}
	// payload приводим обратно в JSON и разбираем в модель.BriefPayload —
	// так лишние ключи отбрасываются, а типы проверяются.
	if p, ok := obj["payload"]; ok && p != nil {
		rawPayload, err := json.Marshal(p)
		if err != nil {
			return answer, err
		}
		answer.Payload = rawPayload
	}
	if r, ok := obj["reply"].(string); ok {
		answer.Reply = r
	}
	// skip — темы, от которых менеджер отказался. Читаем и здесь: без этого
	// ключа «не знаю» ничем не отличается от «модель забыла поле».
	if raw, ok := obj["skip"].([]any); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok && s != "" {
				answer.Skip = append(answer.Skip, s)
			}
		}
	}
	return answer, nil
}

// mergeBrief накладывает ответ модели на прежний бриф. Пустое значение в
// ответе НЕ затирает уже известное: модель, забывшая поле, не должна стирать
// работу менеджера.
//
// Отдельно: строковые значения от модели проверяются на «похожесть на
// данные», пустые списки не перетирают непустые.
func mergeBrief(prev model.BriefPayload, rawPayload json.RawMessage) model.BriefPayload {
	if len(rawPayload) == 0 {
		return prev
	}
	var incoming model.BriefPayload
	if err := json.Unmarshal(rawPayload, &incoming); err != nil {
		return prev
	}
	out := prev
	keepStr := func(dst *string, src string) {
		if model.NonEmpty(src) {
			*dst = strings.TrimSpace(src)
		}
	}
	keepList := func(dst *[]string, src []string) {
		if len(src) > 0 {
			*dst = src
		}
	}
	keepStr(&out.ObjectName, incoming.ObjectName)
	keepList(&out.CustomerPains, incoming.CustomerPains)
	keepList(&out.CustomerGoals, incoming.CustomerGoals)
	keepList(&out.ProjectRisks, incoming.ProjectRisks)
	keepStr(&out.BudgetSignal, incoming.BudgetSignal)
	keepStr(&out.DecisionMaker, incoming.DecisionMaker)
	keepStr(&out.Timeline, incoming.Timeline)
	keepList(&out.WhyUs, incoming.WhyUs)
	keepStr(&out.Notes, incoming.Notes)
	if len(incoming.Objections) > 0 {
		out.Objections = incoming.Objections
	}
	// Пропущенные темы приходят двумя путями: отдельным ключом ответа (skip)
	// и внутри payload. Оба должны дойти до брифа, иначе тема «не знаю»
	// закроется только на текущем шаге и вернётся после перезапуска.
	if len(incoming.Skipped) > 0 {
		out.Skipped = mergeSkipped(out.Skipped, incoming.Skipped)
	}
	out.Normalize()
	return out
}

// finishStep заполняет «куда идём дальше» по итогам брифа.
func finishStep(p model.BriefPayload) BriefStep {
	next, ok := nextOpenTopic(p)
	step := BriefStep{Payload: p}
	if !ok {
		step.Done = true
		step.Reply = "Бриф заполнен. Переходим к расчёту предложения."
		return step
	}
	step.NextTopic = next.Key
	step.Reply = "Записал. " + next.Question
	return step
}

// finishStepFor — то же, но без реплики «Записал»: для неудачных шагов.
func finishStepFor(p model.BriefPayload) BriefStep {
	next, ok := nextOpenTopic(p)
	step := BriefStep{Payload: p}
	if ok {
		step.NextTopic = next.Key
	}
	return step
}

// buildBriefUserPrompt — реплика менеджера и то, что уже известно. История
// ответов модели сюда НЕ входит: бриф и есть сжатая история, а лишний текст
// тянет за собой её же ошибки.
func buildBriefUserPrompt(brief model.BriefPayload, managerText string) string {
	var b strings.Builder
	b.WriteString("ТЕКУЩИЙ БРИФ (JSON):\n")
	if raw, err := marshalBriefPayload(brief); err == nil {
		b.WriteString(raw)
	} else {
		b.WriteString("{}")
	}
	b.WriteString("\n\nРЕПЛИКА МЕНЕДЖЕРА:\n")
	b.WriteString(strings.TrimSpace(managerText))
	b.WriteString("\n\nВерни JSON с обновлённым payload и reply.")
	return b.String()
}

// marshalBriefPayload — бриф в JSON без лишних полей.
func marshalBriefPayload(p model.BriefPayload) (string, error) {
	p.Normalize()
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// nextOpenTopic — обёртка над манифестом, чтобы тесты и ручка говорили об одном.
func nextOpenTopic(p model.BriefPayload) (skills.Topic, bool) {
	return skills.NextOpenTopic(p)
}

// mergeSkipped объединяет пропущенные темы без повторов: модель может назвать
// тему повторно, и список не должен расти на каждом шаге.
func mergeSkipped(prev, add []string) []string {
	seen := make(map[string]bool, len(prev)+len(add))
	out := make([]string, 0, len(prev)+len(add))
	for _, v := range prev {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	for _, v := range add {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
