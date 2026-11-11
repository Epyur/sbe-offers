package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Поиск систем и материалов (2026-11-12): живая жалоба была на то, что человек
// ввёл «Гарант», а ни одна система не предложилась. Здесь стерегутся два
// правила: поиск идёт полнотекстовым поиском Базы знаний (те же ручки, что у
// конкурентов), и материалы отличаются от систем по тегу, а не по названию.

// pickerKBStub — kb-service с поиском и заметками по id.
type pickerKBStub struct {
	tagIDs  map[string]int64
	search  map[int64][]KBNoteSummary // результаты по id тега
	tags    map[int64][]KBTag         // теги полной заметки
	titles  map[int64]string
	lastQ   string
	lastTag string
	failAll bool
}

func (s *pickerKBStub) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/service-token", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"jwt":"kb-jwt"}`))
	})
	mux.HandleFunc("/api/kb/tags", func(w http.ResponseWriter, r *http.Request) {
		var sb strings.Builder
		sb.WriteString(`{"tags":[`)
		first := true
		for name, id := range s.tagIDs {
			if !first {
				sb.WriteString(",")
			}
			first = false
			sb.WriteString(`{"id":` + itoa(int(id)) + `,"name":"` + name + `"}`)
		}
		sb.WriteString(`]}`)
		_, _ = w.Write([]byte(sb.String()))
	})
	mux.HandleFunc("/api/kb/search", func(w http.ResponseWriter, r *http.Request) {
		if s.failAll {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"down"}`))
			return
		}
		s.lastQ = r.URL.Query().Get("q")
		tag := r.URL.Query().Get("tag_id")
		s.lastTag = tag
		list := s.search[atoi10(tag)]
		var sb strings.Builder
		sb.WriteString(`{"items":[`)
		for i, n := range list {
			if i > 0 {
				sb.WriteString(",")
			}
			title := n.Title
			if t, ok := s.titles[n.ID]; ok {
				title = t
			}
			sb.WriteString(`{"id":` + itoa(int(n.ID)) + `,"title":"` + title + `"}`)
		}
		sb.WriteString(`]}`)
		_, _ = w.Write([]byte(sb.String()))
	})
	mux.HandleFunc("/api/kb/notes/", func(w http.ResponseWriter, r *http.Request) {
		id := atoi10(strings.TrimPrefix(r.URL.Path, "/api/kb/notes/"))
		_, _ = w.Write([]byte(`{"id":` + itoa(int(id)) + `,"content_md":"","tags":` + tagsJSON(s.tags[id]) + `}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	return mux
}

func newPickerKB(t *testing.T, s *pickerKBStub) *KBClient {
	t.Helper()
	srv := httptest.NewServer(s.handler())
	t.Cleanup(srv.Close)
	return &KBClient{baseURL: srv.URL, authURL: srv.URL, serviceKey: "k", client: srv.Client()}
}

// Система помечена commercial_propose, материал — ещё и material: тег
// commercial_propose один на оба вида, поэтому «системы» без сверки тегов
// отдали бы и материалы.
func pickerStub() *pickerKBStub {
	return &pickerKBStub{
		tagIDs: map[string]int64{commercialPropose: 7, materialTagName: 9},
		search: map[int64][]KBNoteSummary{
			7: {
				{ID: 501, Title: "ТН-Кровля Гарант ПЛЮС"},
				{ID: 502, Title: "ТН-Кровля Гарант RE30"},
				{ID: 601, Title: "Мембрана ПВХ LOGICROOF"},
			},
			9: {
				{ID: 601, Title: "Мембрана ПВХ LOGICROOF"},
			},
		},
		tags: map[int64][]KBTag{
			501: {{ID: 7, Name: commercialPropose}},
			502: {{ID: 7, Name: commercialPropose}},
			601: {{ID: 7, Name: commercialPropose}, {ID: 9, Name: materialTagName}},
		},
	}
}

func TestSearchSystemsFindsCyrillicWord(t *testing.T) {
	stub := pickerStub()
	s := &Server{kb: newPickerKB(t, stub)}
	found, err := s.searchSystems(context.Background(), "Гарант")
	if err != nil {
		t.Fatalf("поиск систем: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("ожидали две системы «Гарант», получили %d: %+v", len(found), found)
	}
	if found[0].Title != "ТН-Кровля Гарант ПЛЮС" || found[1].NoteID != 502 {
		t.Fatalf("порядок или названия не те: %+v", found)
	}
	if stub.lastQ != "Гарант" {
		t.Errorf("в Базу знаний ушёл не тот запрос: %q", stub.lastQ)
	}
}

// Материал помечен и commercial_propose, и material — из систем он обязан
// выпасть, иначе окно систем предложит мембрану.
func TestSearchSystemsSkipsMaterials(t *testing.T) {
	s := &Server{kb: newPickerKB(t, pickerStub())}
	found, err := s.searchSystems(context.Background(), "Гарант")
	if err != nil {
		t.Fatalf("поиск систем: %v", err)
	}
	for _, item := range found {
		if item.NoteID == 601 {
			t.Fatalf("материал попал в список систем: %+v", found)
		}
	}
}

func TestSearchMaterialsUsesMaterialTag(t *testing.T) {
	stub := pickerStub()
	s := &Server{kb: newPickerKB(t, stub)}
	materials := []Material{{
		ID: 44, SKU: "MAT-КРВ-ПВХ-0007", Name: "Мембрана ПВХ LOGICROOF",
		Unit: "м2", NoteID: 601, NoteTitle: "Мембрана ПВХ LOGICROOF",
	}}
	found, err := s.searchMaterials(context.Background(), "мембрана", materials)
	if err != nil {
		t.Fatalf("поиск материалов: %v", err)
	}
	if stub.lastTag != "9" {
		t.Errorf("материалы искали не по тегу material: tag_id=%q", stub.lastTag)
	}
	if len(found) != 1 {
		t.Fatalf("ожидали один материал, получили %+v", found)
	}
	got := found[0]
	if got.MaterialID != 44 || got.SKU != "MAT-КРВ-ПВХ-0007" || got.Unit != "м2" {
		t.Fatalf("карточка не связана со справочником: %+v", got)
	}
}

// Материал, которого ещё нет в справочнике, всё равно предлагается: имя и
// единица известны из карточки, а запись заведётся при чтении списка.
func TestSearchMaterialsWorksWithoutCatalogueEntry(t *testing.T) {
	s := &Server{kb: newPickerKB(t, pickerStub())}
	found, err := s.searchMaterials(context.Background(), "мембрана", nil)
	if err != nil {
		t.Fatalf("поиск материалов: %v", err)
	}
	if len(found) != 1 || found[0].Name != "Мембрана ПВХ LOGICROOF" || found[0].Unit != "шт" {
		t.Fatalf("карточка вне справочника потеряна: %+v", found)
	}
}

// Ошибка Базы знаний — не 500 и не пустой экран: окно выбора получает 200 с
// полем error и уже выбранное в предложении остаётся на месте.
func TestSearchSystemsKBErrorIsSoft(t *testing.T) {
	stub := pickerStub()
	stub.failAll = true
	srv := &Server{kb: newPickerKB(t, stub)}
	req := httptest.NewRequest(http.MethodGet, "/api/offers/systems/search?q=Гарант", nil)
	rec := httptest.NewRecorder()
	srv.handleSearchSystems(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("статус %d, ожидали 200", rec.Code)
	}
	var body struct {
		Items []pickerSystem `json:"items"`
		Error string         `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("разбор ответа: %v (%s)", err, rec.Body.String())
	}
	if body.Error == "" {
		t.Fatalf("ошибка Базы знаний не названа: %s", rec.Body.String())
	}
	if body.Items == nil || len(body.Items) != 0 {
		t.Fatalf("items должен быть пустым массивом, а не null: %s", rec.Body.String())
	}
}

// Даже при ошибке ответ несёт `"items":[]`, а не `null`: клиент читает длину
// массива и падает на null (грабля 2026-11-11).
func TestSearchSystemsEmptyListIsArray(t *testing.T) {
	srv := &Server{kb: newPickerKB(t, pickerStub())}
	req := httptest.NewRequest(http.MethodGet, "/api/offers/systems/search", nil)
	rec := httptest.NewRecorder()
	srv.handleSearchSystems(rec, req)
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("пустой поиск вернул не массив: %s", rec.Body.String())
	}
}

// Без Базы знаний материалы ищутся по справочнику: другого источника нет, но
// молчание хуже честного «не настроено».
func TestFilterMaterialsByName(t *testing.T) {
	materials := []Material{
		{ID: 1, Name: "Мембрана ПВХ", SKU: "MAT-1", Unit: "м2"},
		{ID: 2, Name: "Утеплитель PIR", SKU: "MAT-2", Unit: "м3"},
		{ID: 3, Name: "Мембрана ТПО", SKU: "MAT-3", Unit: "м2", Archived: true},
	}
	found := filterMaterialsByName(materials, "мембрана")
	if len(found) != 1 || found[0].Name != "Мембрана ПВХ" {
		t.Fatalf("фильтр по справочнику сработал неверно: %+v", found)
	}
	if bySku := filterMaterialsByName(materials, "MAT-2"); len(bySku) != 1 || bySku[0].MaterialID != 2 {
		t.Fatalf("поиск по коду не работает: %+v", bySku)
	}
}

// Название карточки для списка: строка поиска, иначе имя файла, иначе честная
// подпись — пустая кнопка в окне выбора не нажимается.
func TestPickerTitleFallsBackToPath(t *testing.T) {
	cases := []struct {
		name string
		in   KBNoteSummary
		want string
	}{
		{"строка поиска", KBNoteSummary{Title: "ТН-Кровля Гарант"}, "ТН-Кровля Гарант"},
		{"имя файла", KBNoteSummary{Path: "Кровли/ТН-Кровля Гарант RE30.md"}, "ТН-Кровля Гарант RE30"},
		{"пусто", KBNoteSummary{}, "Без названия"},
	}
	for _, tc := range cases {
		if got := pickerTitle(tc.in); got != tc.want {
			t.Fatalf("%s: получили %q, ожидали %q", tc.name, got, tc.want)
		}
	}
}
