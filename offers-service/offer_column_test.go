package main

import (
	"strings"
	"testing"
)

// persistOfferColumn — одна точка записи «секции» предложения (2026-11-13).
//
// Ошибка в имени колонки — это запрос к несуществующей колонке, то есть 500 у
// человека. Список допустимых имён живёт в одном месте; тест сторожит, что
// свободная строка в него не проскочит, а известные секции — проходят.
func TestPersistOfferColumnRejectsUnknownColumn(t *testing.T) {
	s := &Server{} // pool не нужен: отказ происходит ДО запроса
	err := s.persistOfferColumn(t.Context(), 1, "password", []byte(`"x"`))
	if err == nil {
		t.Fatal("неизвестная колонка принята — имя уходит в SQL как есть")
	}
	if !strings.Contains(err.Error(), "неизвестная колонка") {
		t.Fatalf("отказ не объясняет причину: %v", err)
	}
}

// TestOfferColumnsCoverEveryWriter — каждая секция, которую пишут отдельными
// ручками, обязана быть в списке разрешённых.
func TestOfferColumnsCoverEveryWriter(t *testing.T) {
	for _, column := range []string{"system_choice", "material_choice", "form_values"} {
		if !offerColumnNames[column] {
			t.Fatalf("колонка %q пишется ручкой, но не разрешена в offerColumnNames", column)
		}
	}
}
