package main

import (
	"context"
	"strings"
	"testing"
)

// Поиск по Базе знаний тем же механизмом, что в веб-версии (2026-11-12).
//
// Живая жалоба: «по ПВХ мембранам находятся одни и те же PLASTFOIL, хотя есть
// Sika и ФАХМАНН; по плитам VENT нашёлся нерелевантный продукт, а VentiDuct не
// нашёлся». Причина — поиск по подстроке названия: у ФАХМАНН название
// «Кровельная ПВХ мембрана FACHMANN …» не содержит подстроку «пвх мембрана»
// в том порядке, а у VentiDuct в названии нет ни «плита», ни «вентил».
//
// kb-service умеет полнотекстовый поиск с русской морфологией
// (`GET /api/kb/search?q=…`): он есть в веб-версии и находит все бренды сразу.
// Здесь проверяется клиент этого поиска и фильтр по тегу.

func TestKBSearchSendsQueryAndTag(t *testing.T) {
	s, c := newKBStub(t)
	s.body = `{"items":[{"id":18419,"title":"Кровельная ПВХ мембрана FACHMANN PVC-RP 2,0мм"}]}`
	found, err := c.Search(context.Background(), "ПВХ мембрана", 23345, 50)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(found) != 1 || !strings.Contains(found[0].Title, "FACHMANN") {
		t.Fatalf("поиск не разобран: %#v", found)
	}
	if !strings.Contains(s.lastPath, "/api/kb/search") {
		t.Errorf("запрос пошёл не в поиск: %q", s.lastPath)
	}
	if !strings.Contains(s.lastQuery, "q=") {
		t.Errorf("в запросе нет q: %q", s.lastQuery)
	}
	if !strings.Contains(s.lastQuery, "tag_id=23345") {
		t.Errorf("в запросе нет tag_id: %q", s.lastQuery)
	}
}

// Пустой запрос — не ходим в сеть: «пустой поиск» вернул бы весь каталог.
func TestKBSearchEmptyQuery(t *testing.T) {
	s, c := newKBStub(t)
	found, err := c.Search(context.Background(), "   ", 0, 20)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("пустой запрос дал результаты: %#v", found)
	}
	if s.lastPath != "" {
		t.Fatalf("пустой запрос не должен ходить в сеть, путь %q", s.lastPath)
	}
}

// Поиск без тега — тоже законен: плагин ищет по всей Базе знаний.
func TestKBSearchWithoutTag(t *testing.T) {
	s, c := newKBStub(t)
	s.body = `{"items":[{"id":1,"title":"Что-то"}]}`
	if _, err := c.Search(context.Background(), "мембрана", 0, 20); err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if strings.Contains(s.lastQuery, "tag_id") {
		t.Errorf("tag_id не должен передаваться: %q", s.lastQuery)
	}
}

// `notes` вместо `items` тоже понимаем: старые сборки сервиса отвечают так.
func TestKBSearchAcceptsNotesField(t *testing.T) {
	s, c := newKBStub(t)
	s.body = `{"notes":[{"id":2,"title":"Старый формат"}]}`
	found, err := c.Search(context.Background(), "старый", 0, 20)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(found) != 1 || found[0].ID != 2 {
		t.Fatalf("ответ старого формата не разобран: %#v", found)
	}
}
