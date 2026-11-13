package main

import (
	"net/http/httptest"
	"testing"
)

// Сколько времени отведено ручке (2026-11-14, живое КП 031/2026).
//
// Сборка документа по макету и тексты моделей длятся минуты. Ручка
// «Отправить в чат» собирает ТОТ ЖЕ документ, что и «⬇ DOCX», и обязана
// получать тот же длинный предел — иначе макет не успевает (уходим в блочный
// путь), а тексты обрываются на второй системе.
func TestAnswerRouteGetsLayoutTimeout(t *testing.T) {
	cases := map[string]bool{
		"/api/offers/app/answer":      true,
		"/api/offers/offers/8/docx":   true,
		"/api/offers/offers/8/link":   true,
		"/api/offers/offers/8/export": true,
		"/api/offers/offers":          false,
		"/api/offers/app/session":     false,
		"/api/offers/app/config":      false,
	}
	for path, wantLong := range cases {
		req := httptest.NewRequest("POST", path, nil)
		got := writeTimeoutFor(req.URL.Path)
		if wantLong && got != layoutBuildTimeout {
			t.Errorf("%s: предел %v, ожидали длинный %v", path, got, layoutBuildTimeout)
		}
		if !wantLong && got == layoutBuildTimeout {
			t.Errorf("%s: неожиданно длинный предел %v", path, got)
		}
	}
}

// У ручки «выбрать систему» свой предел: тексты пишутся там, но макет не
// собирается.
func TestSystemRouteGetsSystemTextTimeout(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/offers/bot/offers/8/system", nil)
	if got := writeTimeoutFor(req.URL.Path); got != systemTextTimeout {
		t.Fatalf("выбор системы: предел %v, ожидали %v", got, systemTextTimeout)
	}
}
