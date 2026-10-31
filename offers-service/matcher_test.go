package main

import (
	"context"
	"strings"
	"testing"
)

// Сопоставление строк таблицы с материалами (Task 4).
//
// Главное правило: догадка не равна факту. Точный ЕКН и единственное
// название — факты; ответ модели — предположение, которое человек подтверждает.

// countingCompleter считает вызовы: шаг модели должен работать ТОЛЬКО для
// строк, не разобранных первыми двумя шагами.
type countingCompleter struct {
	answer string
	calls  int
	err    error
}

func (c *countingCompleter) Complete(_ context.Context, _, _ string, _ int) (string, error) {
	c.calls++
	if c.err != nil {
		return "", c.err
	}
	return c.answer, nil
}

func testMaterials() []MaterialRef {
	return []MaterialRef{
		{ID: 1, SKU: "MAT-МЕМБ-ПВХ-0001", Name: "Мембрана ПВХ Эксперт", Ekn: []string{"068863", "068864"}},
		{ID: 2, SKU: "MAT-ГРУНТ-ОБЩ-0002", Name: "Грунтовка битумная", Ekn: []string{"111111"}},
		{ID: 3, SKU: "MAT-УТЕПЛ-ПИР-0003", Name: "Плита PIR 100", Ekn: nil},
	}
}

func TestMatchByExactEkn(t *testing.T) {
	rows := []SheetRow{
		{Name: "Что-то совсем другое", Ekn: "068863"},
	}
	res := matchRows(context.Background(), rows, testMaterials(), nil)
	if res[0].Method != matchByEkn || res[0].MaterialID != 1 {
		t.Fatalf("точный ЕКН должен победить название: %+v", res[0])
	}
}

// Номер в таблице может прийти с пробелами или дефисами — это тот же ЕКН.
func TestMatchByEknIgnoresSeparators(t *testing.T) {
	rows := []SheetRow{{Name: "X", Ekn: "068-863"}}
	res := matchRows(context.Background(), rows, testMaterials(), nil)
	if res[0].MaterialID != 1 {
		t.Fatalf("ЕКН с разделителями не узнан: %+v", res[0])
	}
}

func TestMatchByExactName(t *testing.T) {
	rows := []SheetRow{{Name: "Мембрана ПВХ Эксперт"}}
	res := matchRows(context.Background(), rows, testMaterials(), nil)
	if res[0].Method != matchByName || res[0].MaterialID != 1 {
		t.Fatalf("точное название не сработало: %+v", res[0])
	}
}

// Различия в регистре, ё/е и кавычках — не повод не узнать материал.
func TestMatchByNameNormalizes(t *testing.T) {
	materials := []MaterialRef{{ID: 9, SKU: "MAT-ПРОЧ-ОБЩ-0009", Name: "Мембрана «Ёлка»"}}
	rows := []SheetRow{{Name: "мембрана елка"}}
	res := matchRows(context.Background(), rows, materials, nil)
	if res[0].MaterialID != 9 {
		t.Fatalf("название с ё и кавычками не распознано: %+v", res[0])
	}
}

// Два материала с одним названием — не угадываем, а просим уточнить.
func TestMatchAmbiguousNameIsNotGuessed(t *testing.T) {
	materials := []MaterialRef{
		{ID: 1, SKU: "A", Name: "Мембрана"},
		{ID: 2, SKU: "B", Name: "Мембрана"},
	}
	rows := []SheetRow{{Name: "Мембрана"}}
	res := matchRows(context.Background(), rows, materials, nil)
	if res[0].Method != matchNone || res[0].MaterialID != 0 {
		t.Fatalf("неоднозначное название не должно угадываться: %+v", res[0])
	}
	if !strings.Contains(res[0].Reason, "несколько") {
		t.Fatalf("причина не объясняет неоднозначность: %q", res[0].Reason)
	}
}

// Модель спрашивают ТОЛЬКО про строки, которые не разобрали точные шаги.
func TestMatchAsksModelOnlyForUnmatched(t *testing.T) {
	c := &countingCompleter{answer: "3"}
	rows := []SheetRow{
		{Name: "Мембрана ПВХ Эксперт"},      // точное название
		{Name: "Плита пир 100 мм", Ekn: ""}, // не совпало ничем
		{Name: "Y", Ekn: "111111"},          // точный ЕКН
	}
	res := matchRows(context.Background(), rows, testMaterials(), c)
	if c.calls != 1 {
		t.Fatalf("модель спрошена %d раз, ожидали 1 (только для нераспознанного)", c.calls)
	}
	if res[1].Method != matchByAI || res[1].MaterialID != 3 {
		t.Fatalf("ответ модели не применён: %+v", res[1])
	}
	if !res[1].NeedsConfirm {
		t.Fatal("решение модели обязано требовать подтверждения")
	}
}

// Ответ модели «нет» и мусор в ответе — материал не выдумывается.
func TestMatchModelSaysNo(t *testing.T) {
	for _, answer := range []string{"нет", "", "не знаю", "999"} {
		c := &countingCompleter{answer: answer}
		rows := []SheetRow{{Name: "Неизвестная штука"}}
		res := matchRows(context.Background(), rows, testMaterials(), c)
		if res[0].MaterialID != 0 || res[0].Method != matchNone {
			t.Errorf("ответ %q дал материал %+v", answer, res[0])
		}
	}
}

// Модель недоступна — сопоставление не падает и не выдумывает.
func TestMatchModelFailureIsSoft(t *testing.T) {
	c := &countingCompleter{err: errLLMNotConfigured}
	rows := []SheetRow{{Name: "Неизвестная штука"}}
	res := matchRows(context.Background(), rows, testMaterials(), c)
	if res[0].MaterialID != 0 {
		t.Fatalf("при отказе модели материал не должен появляться: %+v", res[0])
	}
}

// Строка без названия и без ЕКН объясняется, а не пропадает молча.
func TestMatchEmptyRowHasReason(t *testing.T) {
	rows := []SheetRow{{MissingName: true}}
	res := matchRows(context.Background(), rows, testMaterials(), nil)
	if res[0].Reason == "" {
		t.Fatal("пустая строка должна получить причину")
	}
}

// pickMaterialByAnswer не выходит за пределы списка и понимает обрамление.
func TestPickMaterialByAnswer(t *testing.T) {
	materials := testMaterials()
	cases := map[string]int64{
		"2":           2,
		"Ответ: 3":    3,
		"материал 1.": 1,
		"0":           0,
		"4":           0,
		"нет":         0,
	}
	for answer, want := range cases {
		got, ok := pickMaterialByAnswer(answer, materials)
		if want == 0 && ok {
			t.Errorf("ответ %q должен дать «нет», получили %+v", answer, got)
		}
		if want != 0 && (!ok || got.ID != want) {
			t.Errorf("ответ %q дал %+v, ожидали id %d", answer, got, want)
		}
	}
}

// Разделение систем и материалов по тегам (Task 6).
//
// Признак «есть ТН- в названии» ненадёжен: технолог заведёт карточку без
// префикса, и она попадёт не в тот раздел. Тег — явное решение человека.

func TestSplitCardsByTags(t *testing.T) {
	notes := []KBNoteSummary{
		{ID: 1, Title: "ТН-Стилобат", Tags: []KBTag{{ID: 1, Name: "commercial_propose"}}},
		{ID: 2, Title: "Мембрана ПВХ", Tags: []KBTag{{ID: 1, Name: "commercial_propose"}, {ID: 2, Name: "material"}}},
		{ID: 3, Title: "Без тега", Tags: nil},
	}
	systems, materials := splitCardsByTags(notes)
	if len(systems) != 2 || len(materials) != 1 {
		t.Fatalf("разделено неверно: систем %d, материалов %d", len(systems), len(materials))
	}
	if materials[0].ID != 2 {
		t.Fatalf("материал определён неверно: %+v", materials[0])
	}
}

// Тег сравнивается без учёта регистра и пробелов: в заметках пишут по-разному.
func TestNoteHasTagIsCaseInsensitive(t *testing.T) {
	n := KBNoteSummary{Tags: []KBTag{{Name: " Material "}}}
	if !noteHasTag(n, materialTagName) {
		t.Fatal("тег с пробелами и другим регистром не узнан")
	}
	if noteHasTag(n, "commercial_propose") {
		t.Fatal("чужой тег опознан как свой")
	}
}

// SKU ищется в строке состава как код, а не по позиции.
func TestExtractCompositionSku(t *testing.T) {
	cases := map[string]string{
		"- **1** Мембрана ПВХ (MAT-МЕМБ-ПВХ-0007)": "MAT-МЕМБ-ПВХ-0007",
		"- MAT-МЕМБ-ПВХ-0007 — мембрана 1,5 мм":    "MAT-МЕМБ-ПВХ-0007",
		"1. Грунтовка [mat-грунт-общ-0012]":        "MAT-ГРУНТ-ОБЩ-0012",
		"- Песок, без кода":                        "",
		"- РАБ-МОНТ-ОБЩ-0001 монтаж":               "РАБ-МОНТ-ОБЩ-0001",
	}
	for line, want := range cases {
		if got := extractCompositionSku(line); got != want {
			t.Errorf("строка %q: получили %q, ожидали %q", line, got, want)
		}
	}
}

// Система без единого SKU в составе — законная: обоснование может быть
// текстом. Разбор не должен падать и не должен выдумывать коды.
func TestExtractCompositionSkuEmpty(t *testing.T) {
	if got := extractCompositionSku(""); got != "" {
		t.Fatalf("пустая строка дала %q", got)
	}
}
