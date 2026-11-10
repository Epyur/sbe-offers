package main

import (
	"encoding/json"
	"testing"
)

// Метки макета (2026-11-11): пустой набор отдаётся МАССИВОМ, не null.
//
// Живая ошибка: сразу после загрузки макета плагин падал «Cannot read
// properties of null (reading 'length')». `describeLayout` возвращал nil-срезы,
// `encoding/json` писал их как `null`, а карточка читала `.length`. Тест смотрит
// СЫРОЙ JSON: декодированный nil и пустой срез неразличимы.

type layoutDescribeBody struct {
	LayoutFields  json.RawMessage `json:"layoutFields"`
	LayoutPrompts json.RawMessage `json:"layoutPrompts"`
}

func TestDescribeLayoutEmptyIsArrays(t *testing.T) {
	fields, prompts := describeLayout(nil)
	if fields == nil || prompts == nil {
		t.Fatalf("пустые метки должны быть пустыми срезами, получено: %v %v", fields, prompts)
	}
	body, err := json.Marshal(map[string]any{
		"layoutFields":  fields,
		"layoutPrompts": prompts,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded layoutDescribeBody
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(decoded.LayoutFields) != "[]" {
		t.Fatalf("layoutFields = %s, ожидался []", decoded.LayoutFields)
	}
	if string(decoded.LayoutPrompts) != "[]" {
		t.Fatalf("layoutPrompts = %s, ожидался []", decoded.LayoutPrompts)
	}
}

// TestDescribeLayoutFindsComparisonMarker — метка {{сравнение}} видна в списке
// промтов: по ней плагин говорит администратору, что метка на месте.
func TestDescribeLayoutFindsComparisonMarker(t *testing.T) {
	doc := wordDoc(`<w:p><w:r><w:t>{{сравнение}}</w:t></w:r></w:p>`)
	layout := layoutDocxFromXML(t, doc)
	_, prompts := describeLayout(layout)
	found := false
	for _, p := range prompts {
		if p == "сравнение" {
			found = true
		}
	}
	if !found {
		t.Fatalf("метка {{сравнение}} не найдена в промтах: %v", prompts)
	}
}
