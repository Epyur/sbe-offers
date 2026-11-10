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
	// parents — id папки → id родителя, для обхода поддерева (2026-11-09).
	parents map[int64]int64
	notes   map[int64][]KBNoteSummary
	bodies  map[int64]string
	paths   []string
	// forbidFolders — отказ на /api/kb/folders (проверка «папка не найдена»).
	failFolders bool
	// tags — имя тега → id; tagged — id тега → заметки с ним (2026-11-10:
	// конкуренты читаются по тегу competitor, а не только из папки).
	tags   map[string]int64
	tagged map[int64][]KBNoteSummary
	// tagLists — сколько раз просили /api/kb/tags (лишних запросов быть не должно).
	tagLists int
	// noteTags — теги ПОЛНОЙ заметки (json-теги в /api/kb/notes/{id}).
	noteTags map[int64][]KBTag
}

// tagsJSON — массив тегов для подстановки в JSON заглушки.
func tagsJSON(tags []KBTag) string {
	var sb strings.Builder
	sb.WriteString("[")
	for i, t := range tags {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"id":` + itoa(int(t.ID)) + `,"name":"` + t.Name + `"}`)
	}
	sb.WriteString("]")
	return sb.String()
}

func (s *folderKBStub) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/service-token", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"jwt":"kb-jwt"}`))
	})
	mux.HandleFunc("/api/kb/tags", func(w http.ResponseWriter, r *http.Request) {
		s.tagLists++
		var sb strings.Builder
		sb.WriteString(`{"tags":[`)
		first := true
		for name, id := range s.tags {
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
			sb.WriteString(`"`)
			if parent, ok := s.parents[id]; ok {
				sb.WriteString(`,"parent_id":`)
				sb.WriteString(itoa(int(parent)))
			}
			sb.WriteString(`}`)
		}
		sb.WriteString(`]}`)
		_, _ = w.Write([]byte(sb.String()))
	})
	mux.HandleFunc("/api/kb/notes", func(w http.ResponseWriter, r *http.Request) {
		s.paths = append(s.paths, r.URL.RawQuery)
		var list []KBNoteSummary
		if tag := r.URL.Query().Get("tag_id"); tag != "" {
			list = s.tagged[atoi10(tag)]
		} else {
			list = s.notes[atoi10(r.URL.Query().Get("folder_id"))]
		}
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
			sb.WriteString(`"`)
			// Теги важны: по ним отсеивается журнал индексации (2026-11-11).
			if len(n.Tags) > 0 {
				sb.WriteString(`,"tags":[`)
				for j, tg := range n.Tags {
					if j > 0 {
						sb.WriteString(",")
					}
					sb.WriteString(`{"id":` + itoa(int(tg.ID)) + `,"name":"` + tg.Name + `"}`)
				}
				sb.WriteString(`]`)
			}
			sb.WriteString(`}`)
		}
		sb.WriteString(`],"total":`)
		sb.WriteString(itoa(len(list)))
		sb.WriteString(`}`)
		_, _ = w.Write([]byte(sb.String()))
	})
	mux.HandleFunc("/api/kb/notes/", func(w http.ResponseWriter, r *http.Request) {
		// /api/kb/notes/{id}
		tail := strings.TrimPrefix(r.URL.Path, "/api/kb/notes/")
		id := atoi10(tail)
		body := s.bodies[id]
		// Настоящий kb-service отдаёт теги только в ПОЛНОЙ заметке, а не в
		// строке списка (2026-11-11) — заглушка повторяет это поведение.
		_, _ = w.Write([]byte(`{"id":` + itoa(int(id)) + `,"content_md":` + jsonString(body) + `,"tags":` + tagsJSON(s.noteTags[id]) + `}`))
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

// atoi10 — десятичное число из строки. Раньше этот разбор повторялся в двух
// местах заглушки; вынесен, чтобы первая же ошибка в нём не разошлась.
func atoi10(s string) int64 {
	var n int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			break
		}
		n = n*10 + int64(ch-'0')
	}
	return n
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

// TestCompetitorsFromKBWalksSubfolders — 2026-11-09: карточки берутся из всего
// поддерева раздела, а не только из верхней папки. Менеджеры раскладывают
// продукты конкурентов по подпапкам, и раньше была видна лишь «разводящая».
func TestCompetitorsFromKBWalksSubfolders(t *testing.T) {
	stub := &folderKBStub{
		folders: map[string]int64{
			competitorsFolderName: 1,
			"ТехноНИКОЛЬ":         2,
			"Пеноплэкс":           3,
			"Пеноплэкс/Кровля":    4,
		},
		parents: map[int64]int64{2: 1, 3: 1, 4: 3},
		notes: map[int64][]KBNoteSummary{
			1: {{ID: 10, Title: "Разводящая"}},
			2: {{ID: 20, Title: "ТехноНИКОЛЬ Кровля"}},
			3: {{ID: 30, Title: "Пеноплэкс Фундамент"}},
			4: {{ID: 40, Title: "Пеноплэкс Кровля"}},
		},
		bodies: map[int64]string{
			10: "# Разводящая\n",
			20: "# ТехноНИКОЛЬ Кровля\n\n## Описание системы\n\nАналог.\n",
			30: "# Пеноплэкс Фундамент\n",
			40: "# Пеноплэкс Кровля\n",
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
	titles := map[string]bool{}
	for _, c := range cards {
		titles[c.Title] = true
	}
	for _, want := range []string{"Разводящая", "ТехноНИКОЛЬ Кровля", "Пеноплэкс Фундамент", "Пеноплэкс Кровля"} {
		if !titles[want] {
			t.Fatalf("в поддереве должна быть карточка %q, получено: %v", want, titles)
		}
	}
	if len(cards) != 4 {
		t.Fatalf("ожидалось 4 карточки, получено %d", len(cards))
	}
}

// TestSubtreeFolderIDs — обход поддерева: папка + все потомки на любую глубину,
// без повторов и без чужих веток (2026-11-09).
func TestSubtreeFolderIDs(t *testing.T) {
	stub := &folderKBStub{
		folders: map[string]int64{
			"раздел": 1, "A": 2, "B": 3, "A1": 4, "A1x": 5, "чужое": 6,
		},
		parents: map[int64]int64{2: 1, 3: 1, 4: 2, 5: 4},
	}
	kb := newFolderKB(t, stub)
	got, err := kb.SubtreeFolderIDs(context.Background(), 1)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	want := map[int64]bool{1: true, 2: true, 3: true, 4: true, 5: true}
	if len(got) != len(want) {
		t.Fatalf("ожидалось %d папок, получено %d: %v", len(want), len(got), got)
	}
	for _, id := range got {
		if !want[id] {
			t.Fatalf("в поддерево попала чужая папка %d: %v", id, got)
		}
	}
	if _, err := kb.SubtreeFolderIDs(context.Background(), 999); err == nil {
		t.Fatal("несуществующая папка должна быть ошибкой")
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

// TestCompetitorsIncludeTaggedNotes — 2026-11-10: продукты конкурентов грузятся
// ещё и по тегу `competitor`, не только из папки. Менеджеры помечают карточки
// тегом, и раньше такие заметки не попадали в систему вовсе.
func TestCompetitorsIncludeTaggedNotes(t *testing.T) {
	stub := &folderKBStub{
		folders: map[string]int64{competitorsFolderName: 1},
		notes: map[int64][]KBNoteSummary{
			1: {{ID: 10, Title: "Разводящая"}},
		},
		tags: map[string]int64{competitorTagName: 8},
		tagged: map[int64][]KBNoteSummary{
			8: {
				{ID: 30, Title: "Конкурент из тега"},
				{ID: 10, Title: "Разводящая"}, // в папке и с тегом — не дубль
			},
		},
		bodies: map[int64]string{
			10: "# Разводящая\n",
			30: "# Конкурент из тега\n\n## Описание системы\n\nАналог.\n",
		},
	}
	kb := newFolderKB(t, stub)
	cards, found, err := CompetitorsFromKB(context.Background(), kb)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if !found {
		t.Fatal("папка найдена — признак должен быть true")
	}
	titles := map[string]int{}
	for _, c := range cards {
		titles[c.Title]++
	}
	if titles["Конкурент из тега"] != 1 {
		t.Fatalf("карточка по тегу должна попасть ровно раз, получено: %v", titles)
	}
	if titles["Разводящая"] != 1 {
		t.Fatalf("заметка в папке и с тегом не должна задваиваться: %v", titles)
	}
	if len(cards) != 2 {
		t.Fatalf("ожидалось 2 карточки, получено %d: %+v", len(cards), cards)
	}
}

// TestCompetitorsByTagWithoutFolderIsFound — тег есть, папки нет: карточки всё
// равно грузятся, `found` истинно (источник есть).
func TestCompetitorsByTagWithoutFolderIsFound(t *testing.T) {
	stub := &folderKBStub{
		folders: map[string]int64{},
		tags:    map[string]int64{competitorTagName: 8},
		tagged:  map[int64][]KBNoteSummary{8: {{ID: 30, Title: "Только по тегу"}}},
		bodies:  map[int64]string{30: "# Только по тегу\n"},
	}
	kb := newFolderKB(t, stub)
	cards, found, err := CompetitorsFromKB(context.Background(), kb)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if !found {
		t.Fatal("тег — источник, found должен быть true")
	}
	if len(cards) != 1 || cards[0].Title != "Только по тегу" {
		t.Fatalf("ожидалась карточка по тегу, получено: %+v", cards)
	}
}

// TestCompetitorTitleSkipsDisclaimerHeading — 2026-11-11, живая жалоба: в
// заметках конкурентов ДВА H1, первый — предупреждение, второй — название.
// Раньше весь список назывался предупреждением.
func TestCompetitorTitleSkipsDisclaimerHeading(t *testing.T) {
	body := "# Материал конкурента, использовать только для критического сравнения\n\n" +
		"# АНКЕР-ВТУЛКА PROPLUG\n\n## Описание\n\nКрепёж.\n"
	stub := &folderKBStub{
		folders: map[string]int64{},
		tags:    map[string]int64{competitorTagName: 8},
		tagged:  map[int64][]KBNoteSummary{8: {{ID: 30, Title: "Материал конкурента, использовать только для критического сравнения"}}},
		bodies:  map[int64]string{30: body},
	}
	kb := newFolderKB(t, stub)
	cards, _, err := CompetitorsFromKB(context.Background(), kb)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(cards) != 1 || cards[0].Title != "АНКЕР-ВТУЛКА PROPLUG" {
		t.Fatalf("название должно браться из второго H1, получено: %+v", cards)
	}
}

// TestCompetitorJournalExcluded — 2026-11-11: «Журнал материалов конкурентов»
// (тег journal) — служебная сводка, в выборе сторон её быть не должно.
func TestCompetitorJournalExcluded(t *testing.T) {
	stub := &folderKBStub{
		folders: map[string]int64{},
		tags:    map[string]int64{competitorTagName: 8},
		tagged: map[int64][]KBNoteSummary{8: {
			{ID: 30, Title: "Журнал материалов конкурентов"},
			{ID: 31, Title: "АНКЕР-ВТУЛКА PROPLUG"},
		}},
		// Теги — у полной заметки: список их не несёт.
		noteTags: map[int64][]KBTag{30: {{ID: 9, Name: "journal"}}},
		bodies: map[int64]string{
			30: "# Журнал материалов конкурентов\n\n## PLASTFOIL\n",
			31: "# АНКЕР-ВТУЛКА PROPLUG\n",
		},
	}
	kb := newFolderKB(t, stub)
	cards, _, err := CompetitorsFromKB(context.Background(), kb)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(cards) != 1 || cards[0].Title != "АНКЕР-ВТУЛКА PROPLUG" {
		t.Fatalf("журнал не должен попадать в список: %+v", cards)
	}
}

// TestCompetitorTitlesSkipsServiceHeadings — чистые случаи разбора названия.
func TestCompetitorTitlesSkipsServiceHeadings(t *testing.T) {
	md := "# Материал конкурента\n# Мембрана ПВХ\n## Описание\n# Ещё заголовок\n"
	got := competitorTitles(md)
	if len(got) != 2 || got[0] != "Мембрана ПВХ" || got[1] != "Ещё заголовок" {
		t.Fatalf("ожидались два содержательных H1, получено: %+v", got)
	}
	if isCompetitorDisclaimer("Материал конкурента, использовать только для критического сравнения ") != true {
		t.Fatal("дисклеймер должен распознаваться и с хвостовым пробелом")
	}
	if isCompetitorDisclaimer("АНКЕР-ВТУЛКА PROPLUG") {
		t.Fatal("название продукта не должно считаться дисклеймером")
	}
}

// TestCompetitorTitleFromNoteHeading — 2026-11-10: название карточки — это
// ЗАГОЛОВОК заметки, а не строка из тела. Раньше разбор тела подменял название
// произвольной заметки, и в списке появлялись чужие фразы.
func TestCompetitorTitleFromNoteHeading(t *testing.T) {
	stub := &folderKBStub{
		folders: map[string]int64{},
		tags:    map[string]int64{competitorTagName: 8},
		tagged:  map[int64][]KBNoteSummary{8: {{ID: 30, Title: "Логистик-Строй"}}},
		bodies: map[int64]string{
			30: "# Логистик-Строй\n\nКарточка системы\n\nТекст сравнения.\n",
		},
	}
	kb := newFolderKB(t, stub)
	cards, _, err := CompetitorsFromKB(context.Background(), kb)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(cards) != 1 || cards[0].Title != "Логистик-Строй" {
		t.Fatalf("название карточки — заголовок заметки, получено: %+v", cards)
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
