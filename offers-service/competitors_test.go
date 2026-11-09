package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Конкуренты (2026-11-08, «Боевой листок»): чтение папки Базы знаний.
//
// Проверяется поведение на уровне клиента: имя папки → её id → заметки →
// карточки. Заглушка отвечает как kb-service (folders, notes, note), поэтому
// тест ловит и разбор ответа, и склейку пагинации.

// folderKBStub — подставной kb-service с папками и заметками.
type folderKBStub struct {
	folders map[string]int64 // имя → id
	notes   map[int64][]KBNoteSummary
	bodies  map[int64]string
	paths   []string
	// forbidFolders — отказ на /api/kb/folders (проверка «папка не найдена»).
	failFolders bool
}

func (s *folderKBStub) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/service-token", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"jwt":"kb-jwt"}`))
	})
	mux.HandleFunc("/api/kb/folders", func(w http.ResponseWriter, r *http.Request) {
		if s.failFolders {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"down"}`))
			return
		}
		var sb strings.Builder
		sb.WriteString(`{"folders":[`)
		first := true
		for name, id := range s.folders {
			if !first {
				sb.WriteString(",")
			}
			first = false
			sb.WriteString(`{"id":`)
			sb.WriteString(itoa(int(id)))
			sb.WriteString(`,"name":"`)
			sb.WriteString(name)
			sb.WriteString(`"}`)
		}
		sb.WriteString(`]}`)
		_, _ = w.Write([]byte(sb.String()))
	})
	mux.HandleFunc("/api/kb/notes", func(w http.ResponseWriter, r *http.Request) {
		s.paths = append(s.paths, r.URL.RawQuery)
		folder := r.URL.Query().Get("folder_id")
		var id int64
		for _, ch := range folder {
			id = id*10 + int64(ch-'0')
		}
		list := s.notes[id]
		var sb strings.Builder
		sb.WriteString(`{"items":[`)
		for i, n := range list {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(`{"id":`)
			sb.WriteString(itoa(int(n.ID)))
			sb.WriteString(`,"title":"`)
			sb.WriteString(n.Title)
			sb.WriteString(`"}`)
		}
		sb.WriteString(`],"total":`)
		sb.WriteString(itoa(len(list)))
		sb.WriteString(`}`)
		_, _ = w.Write([]byte(sb.String()))
	})
	mux.HandleFunc("/api/kb/notes/", func(w http.ResponseWriter, r *http.Request) {
		// /api/kb/notes/{id}
		tail := strings.TrimPrefix(r.URL.Path, "/api/kb/notes/")
		var id int64
		for _, ch := range tail {
			id = id*10 + int64(ch-'0')
		}
		body := s.bodies[id]
		_, _ = w.Write([]byte(`{"id":` + itoa(int(id)) + `,"content_md":` + jsonString(body) + `}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Вложения и прочее — пусто.
		_, _ = w.Write([]byte(`{}`))
	})
	return mux
}

func newFolderKB(t *testing.T, s *folderKBStub) *KBClient {
	t.Helper()
	srv := httptest.NewServer(s.handler())
	t.Cleanup(srv.Close)
	return &KBClient{baseURL: srv.URL, authURL: srv.URL, serviceKey: "k", client: srv.Client()}
}

// jsonString — строка в кавычках для подстановки в JSON заглушки.
func jsonString(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			sb.WriteByte('\\')
			sb.WriteRune(r)
		case '\n':
			sb.WriteString(`\n`)
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

func TestCompetitorsFromKBFolderCaseInsensitive(t *testing.T) {
	stub := &folderKBStub{
		folders: map[string]int64{"Материалы и системы конкурентов": 42},
		notes: map[int64][]KBNoteSummary{
			42: {{ID: 7, Title: "Пеноплэкс конкурент"}},
		},
		bodies: map[int64]string{
			7: "# Пеноплэкс конкурент\n\n## Описание системы\n\nСравниваемый продукт.\n",
		},
	}
	kb := newFolderKB(t, stub)
	cards, found, err := CompetitorsFromKB(context.Background(), kb)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if !found {
		t.Fatal("папка должна считаться найденной")
	}
	if len(cards) != 1 || cards[0].Title != "Пеноплэкс конкурент" {
		t.Fatalf("ожидалась одна карточка конкурента, получено: %+v", cards)
	}
}

func TestCompetitorsFromKBMissingFolderIsNotError(t *testing.T) {
	stub := &folderKBStub{folders: map[string]int64{}, notes: map[int64][]KBNoteSummary{}}
	kb := newFolderKB(t, stub)
	cards, found, err := CompetitorsFromKB(context.Background(), kb)
	if err != nil {
		t.Fatalf("отсутствие папки — не ошибка, получено: %v", err)
	}
	if found {
		t.Fatal("папка не должна считаться найденной")
	}
	if len(cards) != 0 {
		t.Fatalf("ожидался пустой список, получено: %+v", cards)
	}
}

func TestCompetitorsFromKBNotConfigured(t *testing.T) {
	cards, found, err := CompetitorsFromKB(context.Background(), nil)
	if err != nil || found || len(cards) != 0 {
		t.Fatalf("без Базы знаний — пусто без ошибки, получено: %+v,%v,%v", cards, found, err)
	}
}

func TestFolderNotesUsesFolderFilter(t *testing.T) {
	stub := &folderKBStub{
		folders: map[string]int64{competitorsFolderName: 5},
		notes:   map[int64][]KBNoteSummary{5: {{ID: 1, Title: "A"}}},
	}
	kb := newFolderKB(t, stub)
	if _, err := kb.FolderNotes(context.Background(), 5); err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(stub.paths) == 0 || !strings.Contains(stub.paths[len(stub.paths)-1], "folder_id=5") {
		t.Fatalf("ожидался фильтр folder_id=5, запросы: %v", stub.paths)
	}
}
