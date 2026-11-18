package main

// Конкуренты из файла и ссылки (2026-11-17) — то, где ошибка видна не сразу.
//
// Проверяется: адрес не читается, если это не страница http/https; название и
// текст страницы очищаются от скриптов; один адрес — одна запись (повторное
// чтение обновляет текст, а не плодит строки); текстовые конкуренты получают
// РАЗНЫЕ коды (иначе все они склеятся в ключе кэша `.sys0` и перезапишут
// тексты друг друга); правка текста расходится по хэшу готового файла;
// сохранение из плагина не стирает прочитанное.

import (
	"errors"
	"strings"
	"testing"
)

func TestFetchURLProblemRules(t *testing.T) {
	cases := []struct {
		name string
		url  string
		ok   bool
	}{
		{"полный адрес http", "http://example.com/", true},
		{"полный адрес https", "https://plastfoil.ru/catalog/", true},
		{"пустой", "   ", false},
		{"без схемы", "example.com", false},
		{"файл на диске", "file:///etc/passwd", false},
		{"javascript", "javascript:alert(1)", false},
		{"ftp", "ftp://example.com/", false},
		{"слишком длинный", "https://example.com/" + strings.Repeat("a", combatURLMaxLen), false},
	}
	for _, tc := range cases {
		problem := fetchURLProblem(tc.url)
		if tc.ok && problem != "" {
			t.Errorf("%s: ожидалось «можно читать», получено %q", tc.name, problem)
		}
		if !tc.ok && problem == "" {
			t.Errorf("%s: ожидался отказ, получено «можно читать»", tc.name)
		}
	}
}

func TestCombatURLKeyOneAddressOneRecord(t *testing.T) {
	base := combatURLKey("https://plastfoil.ru/catalog/")
	for _, same := range []string{
		"https://plastfoil.ru/catalog",
		"https://plastfoil.ru/catalog/#top",
		"HTTPS://PLASTFOIL.RU/catalog/",
		" https://plastfoil.ru/catalog/ ",
	} {
		if got := combatURLKey(same); got != base {
			t.Errorf("адрес %q должен дать тот же ключ %q, получено %q", same, base, got)
		}
	}
	if combatURLKey("https://plastfoil.ru/catalog/logicroof") == base {
		t.Fatal("разные страницы не должны считаться одной записью")
	}
}

func TestCombatPageTitleAndText(t *testing.T) {
	page := "<!DOCTYPE html><html><head><title>ПВХ мембрана PLASTFOIL</title>" +
		"<style>body{color:red}</style><script>var x = 1;</script></head>" +
		"<body><h1>Кровельная мембрана</h1><p>Толщина 1,5 мм &mdash; серая.</p></body></html>"
	title, text := combatPageTitleAndText("text/html; charset=UTF-8", page)
	if title != "ПВХ мембрана PLASTFOIL" {
		t.Errorf("название страницы разобрано неверно: %q", title)
	}
	if strings.Contains(text, "var x") || strings.Contains(text, "color:red") {
		t.Errorf("в текст попали скрипты или стили: %q", text)
	}
	if !strings.Contains(text, "Толщина 1,5 мм — серая.") {
		t.Errorf("текст страницы потерялся: %q", text)
	}
}

func TestCombatPageTitleAndTextPlain(t *testing.T) {
	title, text := combatPageTitleAndText("text/plain", "  Просто   текст  без  разметки ")
	if title != "" {
		t.Errorf("у обычного текста названия нет, получено %q", title)
	}
	if text != "Просто текст без разметки" {
		t.Errorf("текст должен схлопнуться по пробелам, получено %q", text)
	}
}

func TestTextCompetitorCardsGetDifferentCodes(t *testing.T) {
	refs := []CombatTextRef{
		{Key: "url:https://a.ru/", Title: "Первый", Text: "текст один", Source: combatSourceURL},
		{Key: "url:https://b.ru/", Title: "Второй", Text: "текст два", Source: combatSourceURL},
		{Key: "file:скан:1", Title: "Третий", Text: "текст три", Source: combatSourceFile},
	}
	cards := textCompetitorCards(refs)
	if len(cards) != 3 {
		t.Fatalf("ожидалось 3 карточки, получено %d", len(cards))
	}
	seen := map[string]bool{}
	for _, card := range cards {
		if strings.TrimSpace(card.SKU) == "" {
			t.Fatalf("у карточки %q нет кода — ключ кэша подраздела станет общим", card.Title)
		}
		if seen[card.SKU] {
			t.Fatalf("код %q повторяется — тексты конкурентов перезапишут друг друга", card.SKU)
		}
		seen[card.SKU] = true
		if !card.HasDescription || card.Description == "" {
			t.Fatalf("у карточки %q нет описания, она не напечатается", card.Title)
		}
		if card.NoteID != 0 {
			t.Fatalf("у текстового конкурента не должно быть заметки, получено %d", card.NoteID)
		}
	}
}

func TestTextCompetitorCardsSkipEmpty(t *testing.T) {
	cards := textCompetitorCards([]CombatTextRef{
		{Key: "url:x", Title: "", Text: "текст"},
		{Key: "url:y", Title: "Без текста", Text: "   "},
		{Key: "url:z", Title: "Нормальный", Text: "текст"},
	})
	if len(cards) != 1 || cards[0].Title != "Нормальный" {
		t.Fatalf("пустые записи печататься не должны, получено %+v", cards)
	}
}

func TestAddCombatTextRefReplacesSameKey(t *testing.T) {
	refs := addCombatTextRef(nil, newCombatTextRef("url:https://a.ru/", "Первый", "старый текст", combatSourceURL))
	refs = addCombatTextRef(refs, newCombatTextRef("url:https://a.ru/", "Первый", "новый текст", combatSourceURL))
	if len(refs) != 1 {
		t.Fatalf("повторный тот же адрес должен обновлять запись, получено %d записей", len(refs))
	}
	if refs[0].Text != "новый текст" {
		t.Fatalf("текст не обновился: %q", refs[0].Text)
	}
}

func TestAddCombatTextRefKeepsLimit(t *testing.T) {
	var refs []CombatTextRef
	for i := 0; i < combatTextLimit+3; i++ {
		refs = addCombatTextRef(refs, newCombatTextRef("url:https://site"+itoa(i)+".ru/", "Конкурент "+itoa(i), "текст", combatSourceURL))
	}
	if len(refs) != combatTextLimit {
		t.Fatalf("предел не держится: %d записей", len(refs))
	}
	if refs[len(refs)-1].Title != "Конкурент "+itoa(combatTextLimit+2) {
		t.Fatalf("должна остаться последняя добавленная запись, получено %q", refs[len(refs)-1].Title)
	}
}

func TestRemoveCombatTextRef(t *testing.T) {
	refs := []CombatTextRef{
		{Key: "url:a", Title: "A", Text: "1"},
		{Key: "url:b", Title: "B", Text: "2"},
	}
	got := removeCombatTextRef(refs, "url:a")
	if len(got) != 1 || got[0].Key != "url:b" {
		t.Fatalf("удаление по ключу не сработало: %+v", got)
	}
	if len(removeCombatTextRef(refs, "url:нет")) != 2 {
		t.Fatal("отсутствующий ключ не должен менять список")
	}
}

func TestKeepStoredTextCompetitorsOnSave(t *testing.T) {
	stored := []CombatTextRef{{Key: "url:a", Title: "Первый", Text: "текст"}}
	if got := keepStoredTextCompetitors(nil, stored); len(got) != 1 {
		t.Fatal("пустой список от старого клиента не должен стирать прочитанное")
	}
	fresh := []CombatTextRef{{Key: "url:b", Title: "Второй", Text: "текст"}}
	if got := keepStoredTextCompetitors(fresh, stored); len(got) != 1 || got[0].Key != "url:b" {
		t.Fatal("присланный список должен побеждать")
	}
}

func TestValidateCombatSheetAcceptsTextCompetitor(t *testing.T) {
	sheet := CombatSheet{
		Title: "Боевой листок",
		TextCompetitors: []CombatTextRef{
			{Key: "url:https://a.ru/", Title: "ПВХ мембрана", Text: "текст", Source: combatSourceURL},
		},
	}
	if msg := validateCombatSheet(sheet); msg != "" {
		t.Fatalf("текстовый конкурент — законный: %q", msg)
	}
	for _, broken := range []CombatTextRef{
		{Key: "", Title: "Без ключа", Text: "текст"},
		{Key: "url:x", Title: " ", Text: "текст"},
		{Key: "url:x", Title: "Без текста", Text: "  "},
	} {
		bad := CombatSheet{Title: "Боевой листок", TextCompetitors: []CombatTextRef{broken}}
		if msg := validateCombatSheet(bad); msg == "" {
			t.Fatalf("запись %+v должна быть отвергнута", broken)
		}
	}
	tooMany := CombatSheet{Title: "Боевой листок"}
	for i := 0; i <= combatTextLimit; i++ {
		tooMany.TextCompetitors = append(tooMany.TextCompetitors, CombatTextRef{
			Key: "url:" + itoa(i), Title: "Конкурент " + itoa(i), Text: "текст",
		})
	}
	if msg := validateCombatSheet(tooMany); msg == "" {
		t.Fatal("превышение предела текстовых конкурентов должно быть отвергнуто")
	}
}

// TestCombatDocxHashSeesTextChanges — правка текста конкурента обязана
// разойтись по хэшу, иначе «⬇ DOCX» отдаст старый файл как свежий.
func TestCombatDocxHashSeesTextChanges(t *testing.T) {
	sheet := CombatSheet{
		Number: "017/2026",
		TextCompetitors: []CombatTextRef{
			{Key: "url:https://a.ru/", Title: "ПВХ мембрана", Text: "старый текст", Source: combatSourceURL},
		},
	}
	first := combatDocxHash(sheet, nil, combatDocxVersion, "form", nil)
	edited := sheet
	edited.TextCompetitors = []CombatTextRef{
		{Key: "url:https://a.ru/", Title: "ПВХ мембрана", Text: "новый текст", Source: combatSourceURL},
	}
	second := combatDocxHash(edited, nil, combatDocxVersion, "form", nil)
	if first == second {
		t.Fatal("правка текста конкурента не меняет хэш файла — кэш отдаст старый DOCX")
	}
	// Добавление записи тоже меняет файл.
	edited.TextCompetitors = append(edited.TextCompetitors, CombatTextRef{
		Key: "url:https://b.ru/", Title: "Второй", Text: "текст", Source: combatSourceURL,
	})
	if combatDocxHash(edited, nil, combatDocxVersion, "form", nil) == second {
		t.Fatal("добавление конкурента не меняет хэш файла")
	}
}

// TestTextCompetitorsSurviveSectionOffer — самая тихая ловушка шага C:
// `Ref{SKU:"", NoteID:0}` в `combatSectionOffer` отбрасывается `RefList.Add`,
// и конкурент ИСЧЕЗАЕТ из разделов с `repeat: "systems"` без единой ошибки.
func TestTextCompetitorsSurviveSectionOffer(t *testing.T) {
	cards := textCompetitorCards([]CombatTextRef{
		{Key: "url:https://a.ru/", Title: "Первый", Text: "текст один", Source: combatSourceURL},
		{Key: "url:https://b.ru/", Title: "Второй", Text: "текст два", Source: combatSourceURL},
	})
	if len(cards) != 2 {
		t.Fatalf("нужны две карточки, получено %d", len(cards))
	}
	offer := combatSectionOffer(CombatSheet{Number: "017/2026"}, nil, cards)
	if len(offer.Systems) != 2 {
		t.Fatalf("в наборе разделов должно быть два конкурента, получено %d", len(offer.Systems))
	}
	printed := cardsForOffer(cards, offer)
	if len(printed) != 2 {
		t.Fatalf("печать по разделу должна вернуть обоих конкурентов, получено %d", len(printed))
	}
	for i, card := range printed {
		if card.Title != cards[i].Title {
			t.Fatalf("порядок конкурентов сбит: %q вместо %q", card.Title, cards[i].Title)
		}
	}
	// Ключи кэша подразделов обязаны различаться, иначе тексты перезапишутся.
	if systemCacheHandle(cards[0]) == systemCacheHandle(cards[1]) {
		t.Fatal("у двух текстовых конкурентов один ключ кэша подраздела")
	}
}

// TestCombatFileParseProblem — техническое «unsupported format: txt» человеку
// ничего не говорит; отказ обязан называть читаемые форматы (найдено живой
// проверкой 2026-11-17: agent-service не разбирает .txt).
func TestCombatFileParseProblem(t *testing.T) {
	msg := combatFileParseProblem(errors.New("сервис агента ответил 400: unsupported format: txt"))
	if !strings.Contains(msg, "PDF, DOCX или XLSX") {
		t.Fatalf("отказ должен называть форматы, получено %q", msg)
	}
	other := combatFileParseProblem(errors.New("сервис агента ответил 500: boom"))
	if !strings.Contains(other, "boom") {
		t.Fatalf("прочие ошибки отдаются как есть, получено %q", other)
	}
}

func TestCombatFileTitle(t *testing.T) {
	cases := map[string]string{
		"Конкурент ПВХ.pdf":           "Конкурент ПВХ",
		"C:\\Users\\me\\Смета.xlsx":   "Смета",
		"/tmp/Описание продукта.docx": "Описание продукта",
		"без расширения":              "без расширения",
		".скрытый":                    ".скрытый",
	}
	for name, want := range cases {
		if got := combatFileTitle(name); got != want {
			t.Errorf("%q: получено %q, ожидалось %q", name, got, want)
		}
	}
}
