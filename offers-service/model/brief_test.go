package model

import (
	"encoding/json"
	"strings"
	"testing"
)

// Поля брифа — контракт с плагином и с промптом модели. Пропавший ключ должен
// читаться пустым значением, а не ронять разбор: бриф живёт в JSONB и его
// правит и LLM, и человек.
func TestBriefPayloadKeepsAllKeys(t *testing.T) {
	raw, err := json.Marshal(EmptyBriefPayload())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{
		"object_name", "customer_pains", "customer_goals", "project_risks",
		"objections", "budget_signal", "decision_maker", "timeline", "why_us", "notes",
	} {
		if !strings.Contains(string(raw), `"`+key+`"`) {
			t.Errorf("в пустом брифе нет ключа %q: %s", key, raw)
		}
	}
	// Массивы — пустые срезы, а не null: иначе плагин при отрисовке получает
	// null и падает на .map().
	if strings.Contains(string(raw), "null") {
		t.Errorf("пустой бриф не должен содержать null: %s", raw)
	}
}

// Бриф, сохранённый прошлой версией (без нового ключа), читается без ошибки.
func TestBriefPayloadToleratesMissingKeys(t *testing.T) {
	var p BriefPayload
	if err := json.Unmarshal([]byte(`{"object_name":"ЖК Поэма"}`), &p); err != nil {
		t.Fatalf("старый бриф не разобрался: %v", err)
	}
	if p.ObjectName != "ЖК Поэма" {
		t.Fatalf("не прочиталось имя объекта: %q", p.ObjectName)
	}
	if p.CustomerPains == nil || len(p.CustomerPains) != 0 {
		t.Fatalf("отсутствующий массив должен стать пустым, а не nil: %#v", p.CustomerPains)
	}
}

// Пустые поля не считаются заполненными: по этому признаку бот решает, какую
// тему брифа спрашивать дальше.
func TestBriefPayloadFilledCount(t *testing.T) {
	p := EmptyBriefPayload()
	if n := p.FilledCount(); n != 0 {
		t.Fatalf("пустой бриф: ожидали 0 заполненных, получили %d", n)
	}
	p.ObjectName = "ЖК Поэма"
	p.BudgetSignal = "до 20 млн"
	if n := p.FilledCount(); n != 2 {
		t.Fatalf("ожидали 2 заполненных, получили %d", n)
	}
	p.CustomerPains = []string{"течёт подвал"}
	if n := p.FilledCount(); n != 3 {
		t.Fatalf("список из одного пункта — это заполненное поле, получили %d", n)
	}
	if total := p.TotalFields(); total != 10 {
		t.Fatalf("ожидали 10 полей брифа, получили %d", total)
	}
}

// Возражение без ответа — не возражение: бот должен его дозаполнить, а не
// считать тему закрытой.
func TestBriefPayloadObjectionNeedsBothSides(t *testing.T) {
	p := EmptyBriefPayload()
	p.Objections = []BriefObjection{{Claim: "дорого"}}
	if n := p.FilledCount(); n != 0 {
		t.Fatalf("возражение без ответа не должно считаться заполненным, получили %d", n)
	}
	p.Objections = []BriefObjection{{Claim: "дорого", Response: "сравниваем с ценой протечки"}}
	if n := p.FilledCount(); n != 1 {
		t.Fatalf("возражение с ответом должно считаться, получили %d", n)
	}
}

// Дефолтный контекст продукта — короткий набор из спеки, все ключи пустые
// строки, а не null: плагин рисует textarea, а не проверяет на undefined.
func TestDefaultContextHasShortSet(t *testing.T) {
	c := DefaultContext("client")
	if c.Mode != "client" {
		t.Fatalf("режим: %q", c.Mode)
	}
	for _, key := range []string{
		"positioning", "pains", "objections", "customer_language", "proof", "not_promised",
	} {
		v, ok := c.Sections[key]
		if !ok {
			t.Errorf("в контексте нет раздела %q", key)
			continue
		}
		if v != "" {
			t.Errorf("раздел %q должен быть пустым по умолчанию, а он %q", key, v)
		}
	}
	if c.Learnings == nil || len(c.Learnings) != 0 {
		t.Fatalf("Learnings должен быть пустым срезом: %#v", c.Learnings)
	}
}
