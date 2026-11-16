package main

// Сторож шага ответа на вопрос брифа из приложения (2026-11-15).
//
// Логика шага живёт в `handleAppBriefAnswer` и требует базы и модели, поэтому
// здесь проверяется ПРАВИЛО, которое от неё отделено и которое чаще всего
// ошибается: когда ответ относится к текущему вопросу.
//
// Живой дефект, найденный стендом: бриф, созданный подтверждением файла или
// правкой из плагина, имеет пустой `last_topic` — приложение показывает
// «Объект» первым вопросом, а сервер отвечал 409 «вопрос сменился». Первый
// ответ на бриф должен приниматься.

import "testing"

func TestBriefTopicConflict(t *testing.T) {
	cases := []struct {
		name      string
		requested string
		current   string
		want      bool
	}{
		{"первый ответ на пустой бриф", "object_name", "", false},
		{"вопрос совпадает", "object_name", "object_name", false},
		{"вопрос сменился", "customer_goals", "object_name", true},
		{"пустая тема в адресе", "", "object_name", false},
		{"оба пусты", "", "", false},
	}
	for _, tc := range cases {
		if got := briefTopicConflict(tc.requested, tc.current); got != tc.want {
			t.Errorf("%s: конфликт %v, ожидали %v", tc.name, got, tc.want)
		}
	}
}
