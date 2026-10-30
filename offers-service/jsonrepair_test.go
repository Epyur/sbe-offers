package main

import (
	"errors"
	"testing"
)

// Каждый случай взят из истории sbe-llm / sbe-photobank: это те ответы
// провайдера, на которых «не разобрал» доходило до 90% жалоб.

func TestExtractJSONPlain(t *testing.T) {
	v, repaired, err := extractJSON(`{"payload":{"object_name":"ЖК Поэма"},"reply":"ок"}`)
	if err != nil {
		t.Fatalf("чистый JSON не разобрался: %v", err)
	}
	if repaired {
		t.Error("чистый JSON не должен считаться отремонтированным")
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("ожидали объект, получили %T", v)
	}
	if _, ok := m["payload"]; !ok {
		t.Errorf("потерялся payload: %#v", m)
	}
}

// Модель написала текст до и после — самый частый случай.
func TestExtractJSONWithChatterAround(t *testing.T) {
	text := "Записал, что вы сказали. Вот ответ:\n{\"payload\":{},\"reply\":\"дальше\"}\nНадеюсь, помог."
	v, _, err := extractJSON(text)
	if err != nil {
		t.Fatalf("текст вокруг JSON должен отбрасываться: %v", err)
	}
	if _, ok := v.(map[string]any); !ok {
		t.Fatalf("не объект: %T", v)
	}
}

// Ответ обёрнут в markdown-блок.
func TestExtractJSONFenced(t *testing.T) {
	text := "```json\n{\"payload\":{\"timeline\":\"до мая\"}}\n```"
	v, _, err := extractJSON(text)
	if err != nil {
		t.Fatalf("fenced-блок должен разбираться: %v", err)
	}
	m := v.(map[string]any)
	if m["payload"] == nil {
		t.Fatalf("потерялся payload: %#v", m)
	}
}

// Массив объектов: резка «от { до последней }» дала бы
// «Unexpected non-whitespace character after JSON».
func TestExtractJSONTopLevelArray(t *testing.T) {
	text := `[{"a":1},{"b":2}]`
	v, _, err := extractJSON(text)
	if err != nil {
		t.Fatalf("массив не разобрался: %v", err)
	}
	arr, ok := v.([]any)
	if !ok {
		t.Fatalf("ожидали массив, получили %T", v)
	}
	if len(arr) != 2 {
		t.Fatalf("ожидали 2 элемента, получили %d", len(arr))
	}
}

// Обрезка по лимиту токенов: последняя строка не закрыта, скобок нет.
func TestExtractJSONTruncated(t *testing.T) {
	text := `{"payload":{"object_name":"ЖК Поэма","customer_pains":["протечки подвала`
	v, repaired, err := extractJSON(text)
	if err != nil {
		t.Fatalf("обрезанный ответ должен достраиваться: %v", err)
	}
	if !repaired {
		t.Error("ожидали отметку «отремонтировано»")
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("не объект: %T", v)
	}
	payload, ok := m["payload"].(map[string]any)
	if !ok {
		t.Fatalf("потерялся payload: %#v", m)
	}
	if payload["object_name"] != "ЖК Поэма" {
		t.Errorf("частичные данные потерялись: %#v", payload)
	}
}

// Обрезка посреди объекга внутри массива — скобки разных типов.
func TestExtractJSONTruncatedNested(t *testing.T) {
	text := `{"payload":{"objections":[{"claim":"дорого","response":"сравним с ценой протечки`
	v, repaired, err := extractJSON(text)
	if err != nil {
		t.Fatalf("вложенная обрезка должна достраиваться: %v", err)
	}
	if !repaired {
		t.Error("ожидали отметку «отремонтировано»")
	}
	if _, ok := v.(map[string]any); !ok {
		t.Fatalf("не объект: %T", v)
	}
}

// Хвостовая запятая — модель так часто закрывает списки.
func TestExtractJSONTrailingComma(t *testing.T) {
	text := `{"payload":{"customer_pains":["протечки","сроки",]},"reply":"ок"}`
	if _, _, err := extractJSON(text); err != nil {
		t.Fatalf("хвостовая запятая должна чиниться: %v", err)
	}
}

// Вложенные кавычки в строке не должны сбивать поиск скобок.
func TestExtractJSONQuotesInsideStrings(t *testing.T) {
	text := `{"reply":"Он сказал \"дорого\", а мы ответили: {так не пойдёт}"}`
	v, _, err := extractJSON(text)
	if err != nil {
		t.Fatalf("кавычки и скобки внутри строк ломают поиск: %v", err)
	}
	m := v.(map[string]any)
	if _, ok := m["reply"]; !ok {
		t.Fatalf("потерялся reply: %#v", m)
	}
}

// JSON нет вовсе — единственный случай настоящего отказа.
func TestExtractJSONNoJSON(t *testing.T) {
	_, _, err := extractJSON("Простите, я не понял вопрос. Уточните, пожалуйста.")
	if !errors.Is(err, errNoJSON) {
		t.Fatalf("ожидали errNoJSON, получили %v", err)
	}
}

// Пустой ответ — тоже «нет JSON», но с понятным текстом для журнала.
func TestExtractJSONEmpty(t *testing.T) {
	_, _, err := extractJSON("   ")
	if !errors.Is(err, errNoJSON) {
		t.Fatalf("ожидали errNoJSON, получили %v", err)
	}
}

// Незакрытый fenced-блок — обрезка посреди блока кода.
func TestExtractJSONUnclosedFence(t *testing.T) {
	text := "```json\n{\"payload\":{\"budget_signal\":\"до 20 млн\"}"
	v, _, err := extractJSON(text)
	if err != nil {
		t.Fatalf("незакрытый блок должен разбираться: %v", err)
	}
	if _, ok := v.(map[string]any); !ok {
		t.Fatalf("не объект: %T", v)
	}
}
