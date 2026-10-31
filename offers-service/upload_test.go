package main

import (
	"strings"
	"testing"
)

// Превью таблицы (Task 5): что человек увидит ДО применения.
//
// Главное правило — молчаливых решений нет. Несопоставленные строки,
// догадки модели и расхождения в суммах обязаны быть названы.

func previewResults() []MatchResult {
	return []MatchResult{
		{Row: SheetRow{Line: 2, Name: "Мембрана", HasPrice: true, Price: 1500}, MaterialID: 1, Method: matchByEkn},
		{Row: SheetRow{Line: 3, Name: "Грунтовка", HasPrice: true, Price: 99}, MaterialID: 2, Method: matchByName},
		{Row: SheetRow{Line: 4, Name: "Плита", HasPrice: true, Price: 300}, MaterialID: 3, Method: matchByAI, NeedsConfirm: true},
		{Row: SheetRow{Line: 5, Name: "Непонятное"}, Method: matchNone, Reason: "нет такого названия"},
	}
}

func TestPreviewCountsEachMethod(t *testing.T) {
	p := buildPreview("таблица.xlsx", previewResults())
	if p.Counts.Total != 4 {
		t.Fatalf("всего %d, ожидали 4", p.Counts.Total)
	}
	if p.Counts.ByEkn != 1 || p.Counts.ByName != 1 || p.Counts.ByAI != 1 || p.Counts.None != 1 {
		t.Fatalf("счёт по способам неверен: %+v", p.Counts)
	}
	if p.FileName != "таблица.xlsx" {
		t.Errorf("имя файла потеряно: %q", p.FileName)
	}
}

// Догадка модели и несопоставленные строки обязаны попасть в замечания.
func TestPreviewNamesGuessesAndMisses(t *testing.T) {
	p := buildPreview("t.xlsx", previewResults())
	joined := strings.Join(p.Problems, " ")
	if !strings.Contains(joined, "Модель предложила") {
		t.Errorf("нет предупреждения о догадке модели: %v", p.Problems)
	}
	if !strings.Contains(joined, "Не сопоставлено") {
		t.Errorf("нет предупреждения о несопоставленных строках: %v", p.Problems)
	}
}

// Расхождение суммы с количеством и ценой — не молчаливая правка, а замечание.
func TestPreviewReportsSumMismatch(t *testing.T) {
	results := []MatchResult{{
		Row: SheetRow{Line: 7, Name: "Мембрана", Qty: 2, HasQty: true,
			Price: 100, HasPrice: true, Sum: 999, HasSum: true},
		MaterialID: 1, Method: matchByName,
	}}
	p := buildPreview("t.xlsx", results)
	joined := strings.Join(p.Problems, " ")
	if !strings.Contains(joined, "не сходится") {
		t.Fatalf("расхождение суммы не замечено: %v", p.Problems)
	}
	if !strings.Contains(joined, "7") {
		t.Errorf("в замечании нет номера строки: %v", p.Problems)
	}
}

// Сумма сходится — лишнего замечания нет.
func TestPreviewNoProblemWhenSumMatches(t *testing.T) {
	results := []MatchResult{{
		Row: SheetRow{Line: 2, Name: "Мембрана", Qty: 2, HasQty: true,
			Price: 100, HasPrice: true, Sum: 200, HasSum: true},
		MaterialID: 1, Method: matchByName,
	}}
	p := buildPreview("t.xlsx", results)
	if len(p.Problems) != 0 {
		t.Fatalf("замечаний быть не должно: %v", p.Problems)
	}
}

// Пустая и нулевая цена — почти всегда опечатка; говорим об этом заранее.
func TestPreviewFlagsZeroPrice(t *testing.T) {
	results := []MatchResult{{
		Row:        SheetRow{Line: 3, Name: "Мембрана", HasPrice: true, Price: 0},
		MaterialID: 1, Method: matchByName,
	}}
	p := buildPreview("t.xlsx", results)
	joined := strings.Join(p.Problems, " ")
	if !strings.Contains(joined, "опечатк") {
		t.Fatalf("нулевая цена не замечена: %v", p.Problems)
	}
}

// Пустая таблица — пустое превью без паники.
func TestPreviewEmpty(t *testing.T) {
	p := buildPreview("empty.xlsx", nil)
	if p.Counts.Total != 0 || len(p.Problems) != 0 {
		t.Fatalf("пустая таблица дала %+v", p)
	}
}
