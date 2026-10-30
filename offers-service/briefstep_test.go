package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"mailers/offers/model"
)

// Completer — подмена модели в тестах: функция вместо HTTP.
type fakeCompleter struct {
	answer   string
	err      error
	calls    int
	lastSys  string
	lastUser string
}

func (f *fakeCompleter) Complete(_ context.Context, system, user string, _ int) (string, error) {
	f.calls++
	f.lastSys = system
	f.lastUser = user
	if f.err != nil {
		return "", f.err
	}
	return f.answer, nil
}

func step(t *testing.T, brief model.BriefPayload, text string, c Completer) BriefStep {
	t.Helper()
	res, err := StepBrief(context.Background(), brief, text, c)
	if err != nil {
		t.Fatalf("неожидали ошибку: %v", err)
	}
	return res
}

// Обычный шаг: модель вернула поле и реплику — поле записано, тема закрыта.
func TestStepBriefFillsTopic(t *testing.T) {
	c := &fakeCompleter{answer: `{"payload":{"object_name":"ЖК Поэма"},"reply":"Записал. Какая задача у объекта?"}`}
	res := step(t, model.EmptyBriefPayload(), "ЖК Поэма, Новосибирск", c)
	if res.Payload.ObjectName != "ЖК Поэма" {
		t.Fatalf("объект не записан: %#v", res.Payload)
	}
	if res.Done {
		t.Error("бриф не может быть готов после одной темы")
	}
	if !strings.Contains(res.Reply, "Записал") {
		t.Errorf("реплика модели потерялась: %q", res.Reply)
	}
}

// Модель вернула ТОЛЬКО payload без reply — данные важнее: reply собираем сами
// по манифесту. Модель часто забывает второй ключ; терять из-за этого бриф
// нельзя.
func TestStepBriefWithoutReply(t *testing.T) {
	c := &fakeCompleter{answer: `{"payload":{"object_name":"ЖК Поэма"}}`}
	res := step(t, model.EmptyBriefPayload(), "ЖК Поэма", c)
	if res.Payload.ObjectName != "ЖК Поэма" {
		t.Fatalf("данные потерялись: %#v", res.Payload)
	}
	if strings.TrimSpace(res.Reply) == "" {
		t.Fatal("реплика не собрана — менеджер останется без ответа")
	}
	// Реплика должна задавать СЛЕДУЮЩИЙ вопрос, а не повторять текущий.
	next, ok := nextOpenTopic(res.Payload)
	if !ok {
		t.Fatal("ожидали ещё открытую тему")
	}
	if !strings.Contains(res.Reply, next.Question) {
		t.Errorf("реплика не содержит следующего вопроса %q: %q", next.Key, res.Reply)
	}
}

// Модель ответила общим текстом без JSON — переспрашиваем по текущей теме,
// бриф не меняется.
func TestStepBriefNoJSONKeepsPayload(t *testing.T) {
	before := model.BriefPayload{ObjectName: "ЖК Поэма"}
	c := &fakeCompleter{answer: "Извините, я не понял. Повторите, пожалуйста."}
	res := step(t, before, "что-то", c)
	if res.Payload.ObjectName != "ЖК Поэма" {
		t.Fatalf("прежние данные потерялись: %#v", res.Payload)
	}
	if !res.Retry {
		t.Error("ожидали отметку «переспросить»")
	}
	if strings.TrimSpace(res.Reply) == "" {
		t.Error("переспрос без текста")
	}
}

// Модель придумала ключ, которого нет в брифе — он отбрасывается, а не
// попадает в JSONB и оттуда к плагину.
func TestStepBriefDropsUnknownKeys(t *testing.T) {
	c := &fakeCompleter{answer: `{"payload":{"object_name":"ЖК","competitor":"Acme","reply":"ок"}}`}
	res := step(t, model.EmptyBriefPayload(), "ЖК", c)
	raw, err := marshalBriefPayload(res.Payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(raw, "competitor") {
		t.Errorf("лишний ключ попал в бриф: %s", raw)
	}
}

// «Не знаю» / «пропустить» закрывает тему пустым значением, а не оставляет её
// висеть: иначе бот спросит то же самое второй раз.
func TestStepBriefSkipClosesTopic(t *testing.T) {
	// Все предыдущие темы закрыты, текущая — бюджет; модель вернула её пустой.
	before := model.BriefPayload{
		ObjectName:    "ЖК Поэма",
		CustomerPains: []string{"протечки"},
		CustomerGoals: []string{"сдать"},
		ProjectRisks:  []string{"воды"},
		Objections:    []model.BriefObjection{{Claim: "дорого", Response: "сравним"}},
	}
	c := &fakeCompleter{answer: `{"payload":{},"reply":"Хорошо, идём дальше.","skip":["budget_signal"]}`}
	res := step(t, before, "не знаю", c)
	if got, _ := nextOpenTopic(res.Payload); got.Key != "decision_maker" {
		t.Fatalf("тема «бюджет» должна была закрыться, следующая — %q", got.Key)
	}
	if !containsStr(res.Skipped, "budget_signal") {
		t.Errorf("тема не отмечена пропущенной: %v", res.Skipped)
	}
}

// LLM недоступен: бриф не портится, менеджеру понятное сообщение, работа
// продолжается (можно собирать КП без брифа).
func TestStepBriefLLMNotConfigured(t *testing.T) {
	c := &fakeCompleter{err: errLLMNotConfigured}
	before := model.BriefPayload{ObjectName: "ЖК Поэма"}
	res := step(t, before, "текст", c)
	if res.Payload.ObjectName != "ЖК Поэма" {
		t.Fatalf("бриф испорчен: %#v", res.Payload)
	}
	if !res.Unavailable {
		t.Error("ожидали признак «модель недоступна»")
	}
	if !strings.Contains(strings.ToLower(res.Reply), "недоступ") {
		t.Errorf("непонятное сообщение: %q", res.Reply)
	}
}

// Авария провайдера — тоже не портит бриф, но это НЕ «недоступно навсегда»:
// менеджер должен увидеть «попробуем ещё», а не «бриф отменён».
func TestStepBriefUpstreamError(t *testing.T) {
	c := &fakeCompleter{err: errors.New("модель ответила 502")}
	before := model.BriefPayload{ObjectName: "ЖК Поэма"}
	res := step(t, before, "текст", c)
	if res.Payload.ObjectName != "ЖК Поэма" {
		t.Fatalf("бриф испорчен: %#v", res.Payload)
	}
	if res.Unavailable {
		t.Error("авария провайдера — не «модель не настроена»")
	}
	if !res.Retry {
		t.Error("авария провайдера должна предлагать повтор")
	}
}

// Все темы закрыты — бриф готов, следующий шаг «к расчёту».
func TestStepBriefDoneWhenAllClosed(t *testing.T) {
	full := model.BriefPayload{
		ObjectName:    "ЖК Поэма",
		CustomerPains: []string{"протечки"},
		CustomerGoals: []string{"сдать без замечаний"},
		ProjectRisks:  []string{"грунтовые воды"},
		Objections:    []model.BriefObjection{{Claim: "дорого", Response: "сравним с ценой протечки"}},
		BudgetSignal:  "до 20 млн",
		DecisionMaker: "главный инженер",
		Timeline:      "к маю",
		WhyUs:         []string{"работали на соседнем объекте"},
		Notes:         "нужен монтаж",
	}
	c := &fakeCompleter{answer: `{"payload":{},"reply":"Готово."}`}
	res := step(t, full, "спасибо", c)
	if !res.Done {
		t.Fatal("бриф со всеми закрытыми темами должен быть готов")
	}
}

// Порядок манифеста главнее модели: если она заполнила третью тему, а первую
// нет, следующей спрашиваем ПЕРВУЮ незаполненную.
func TestStepBriefFollowsManifestOrder(t *testing.T) {
	c := &fakeCompleter{answer: `{"payload":{"timeline":"к маю"},"reply":"записал сроки"}`}
	res := step(t, model.EmptyBriefPayload(), "к маю", c)
	if res.Payload.Timeline != "к маю" {
		t.Fatalf("срок не записан: %#v", res.Payload)
	}
	if res.NextTopic != "object_name" {
		t.Fatalf("следующая тема должна быть первой незаполненной, получили %q", res.NextTopic)
	}
}

// Числа из ответа менеджера не должны попадать в промпт как «факты объекта»,
// если модель их выдумала: проверяется на уровне истории — в user-prompt
// идёт только реплика менеджера, без прежних ответов модели.
func TestStepBriefUserPromptCarriesManagerText(t *testing.T) {
	c := &fakeCompleter{answer: `{"payload":{},"reply":"ок"}`}
	step(t, model.EmptyBriefPayload(), "Площадь 4800 м2", c)
	if !strings.Contains(c.lastUser, "Площадь 4800 м2") {
		t.Errorf("реплика менеджера не дошла до модели: %q", c.lastUser)
	}
	if strings.Contains(c.lastUser, `"reply"`) {
		t.Errorf("в user-prompt попал служебный JSON: %q", c.lastUser)
	}
}

// containsStr — есть ли строка в срезе.
func containsStr(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// Одно поле неверного типа не должно обнулять весь бриф.
//
// Живой случай (2026-10-30): модель вернула "why_us": "" вместо [], разбор
// полезной части ответа падал, mergeBrief молча возвращал прежний (пустой)
// бриф — и менеджер видел «ответ не записан», хотя остальные поля пришли
// правильными. Терять весь бриф из-за одного ключа нельзя.
func TestStepBriefToleratesWrongTypedField(t *testing.T) {
	c := &fakeCompleter{answer: `{"payload":{"object_name":"ЖК Поэма","why_us":"","customer_goals":["сдать в срок"]},"reply":"Записал."}`}
	res := step(t, model.EmptyBriefPayload(), "ЖК Поэма", c)
	if res.Payload.ObjectName != "ЖК Поэма" {
		t.Fatalf("объект потерян из-за пустого why_us: %#v", res.Payload)
	}
	if len(res.Payload.CustomerGoals) != 1 {
		t.Fatalf("цели потеряны: %#v", res.Payload.CustomerGoals)
	}
}

// Разбор терпимого payload по отдельности: строка вместо списка становится
// списком из одного элемента, а мусор в одном поле не уносит остальные.
func TestTolerantBriefPayload(t *testing.T) {
	raw := json.RawMessage(`{"object_name":"ЖК Поэма","why_us":"надёжность","customer_goals":["сдать"],"objections":"не список","timeline":42}`)
	p := tolerantBriefPayload(raw)
	if p.ObjectName != "ЖК Поэма" {
		t.Errorf("объект: %q", p.ObjectName)
	}
	if len(p.WhyUs) != 1 || p.WhyUs[0] != "надёжность" {
		t.Errorf("строка вместо списка не стала списком: %#v", p.WhyUs)
	}
	if len(p.CustomerGoals) != 1 {
		t.Errorf("цели: %#v", p.CustomerGoals)
	}
	if p.Timeline != "" {
		t.Errorf("число вместо строки должно остаться пустым, получили %q", p.Timeline)
	}
	if p.Objections == nil {
		t.Error("objections должны быть пустым срезом, а не nil")
	}
}

// Совсем не JSON — пустой бриф, а не паника.
func TestTolerantBriefPayloadGarbage(t *testing.T) {
	p := tolerantBriefPayload(json.RawMessage(`не json`))
	if p.ObjectName != "" || p.Skipped == nil {
		t.Fatalf("ожидали пустой нормализованный бриф: %#v", p)
	}
}
