package main

import "testing"

// Боевой листок в боте (2026-11-09): выбор стороны.
//
// Проверяется поведение, которое видит менеджер: нажал систему второй раз —
// в листке она одна; убрал — списка стало меньше. И то, что в список выбора
// попадает название и заметка, а не код (у живых карточек он пуст).

func TestAppendNoteRefDoesNotDuplicate(t *testing.T) {
	ref := CombatNoteRef{NoteID: 10, Title: "Наша система"}
	got := appendNoteRef(nil, ref)
	got = appendNoteRef(got, ref)
	if len(got) != 1 {
		t.Fatalf("повторный выбор не должен дублировать систему, получено %d", len(got))
	}
	if got[0].NoteID != 10 {
		t.Fatalf("заметка потеряна: %+v", got)
	}
}

func TestAppendNoteRefKeepsOrder(t *testing.T) {
	got := appendNoteRef(nil, CombatNoteRef{NoteID: 1})
	got = appendNoteRef(got, CombatNoteRef{NoteID: 2})
	if len(got) != 2 || got[0].NoteID != 1 || got[1].NoteID != 2 {
		t.Fatalf("порядок выбора должен сохраняться, получено %+v", got)
	}
}

func TestRemoveNoteRefRemovesOnlyTarget(t *testing.T) {
	refs := []CombatNoteRef{{NoteID: 1}, {NoteID: 2}, {NoteID: 3}}
	got := removeNoteRef(refs, 2)
	if len(got) != 2 {
		t.Fatalf("должно остаться две заметки, получено %d", len(got))
	}
	for _, r := range got {
		if r.NoteID == 2 {
			t.Fatal("удалённая заметка осталась в списке")
		}
	}
}

func TestRemoveNoteRefMissingIsNoop(t *testing.T) {
	got := removeNoteRef([]CombatNoteRef{{NoteID: 1}}, 99)
	if len(got) != 1 || got[0].NoteID != 1 {
		t.Fatalf("удаление отсутствующей заметки не должно менять список: %+v", got)
	}
}

// TestFilterCombatItems — поиск стороны по названию/коду (2026-11-09).
//
// Каталог 1500+ записей: перебирать списком невозможно, поэтому поиск обязан
// находить частичные совпадения по всем словам и по коду, и терпеть «ё»/регистр.
func TestFilterCombatItems(t *testing.T) {
	items := []botSystemItem{
		{NoteID: 1, Title: "Мембрана ПВХ Эксперт", SKU: "MAT-ПРОЧ-0001"},
		{NoteID: 2, Title: "Гидроизоляция фундамента", SKU: "TN-FUND"},
		{NoteID: 3, Title: "Кровельная мембрана", SKU: "MAT-КРОВ-0002"},
	}
	cases := []struct {
		query string
		want  []int64
	}{
		{"", []int64{1, 2, 3}},              // пусто — весь список
		{"мембрана", []int64{1, 3}},         // часть названия
		{"МЕМБРАНА кровельная", []int64{3}}, // все слова, регистр
		{"фундамент", []int64{2}},           // «фундамента» по «фундамент»
		{"пвх", []int64{1}},                 // короткое слово
		{"MAT-КРОВ", []int64{3}},            // по коду
		{"нет-такого", []int64{}},           // ничего
	}
	for _, tc := range cases {
		got := filterCombatItems(items, tc.query)
		if len(got) != len(tc.want) {
			t.Fatalf("query %q: ожидалось %d, получено %d (%+v)", tc.query, len(tc.want), len(got), got)
		}
		for i := range got {
			if got[i].NoteID != tc.want[i] {
				t.Fatalf("query %q: на позиции %d ожидался %d, получен %d", tc.query, i, tc.want[i], got[i].NoteID)
			}
		}
	}
}

func TestCombatItemsCarryNoteAndTitle(t *testing.T) {
	items := combatItems([]SystemCard{
		{NoteID: 100, Title: "Кровля", SKU: ""},
		{NoteID: 101, Title: "Фасад", SKU: "SYS-1", Problems: []string{"нет ссылки"}},
	})
	if len(items) != 2 {
		t.Fatalf("ожидалось две карточки, получено %d", len(items))
	}
	if items[0].NoteID != 100 || items[0].Title != "Кровля" {
		t.Fatalf("карточка без кода должна нести заметку и название: %+v", items[0])
	}
	if items[1].SKU != "SYS-1" || len(items[1].Problems) != 1 {
		t.Fatalf("код и замечания должны доехать до бота: %+v", items[1])
	}
}

// TestPaginateCombatItems — пагинация выдачи выбора стороны (2026-11-10).
//
// Менеджер листает список постранично, поэтому сервер обязан отдавать срез
// запрошенного размера, честный total и признак «есть ещё».
func TestPaginateCombatItems(t *testing.T) {
	items := make([]botSystemItem, 20)
	for i := range items {
		items[i] = botSystemItem{NoteID: int64(i + 1), Title: "Продукт"}
	}

	first := paginateCombatItems(items, 0, 8)
	if len(first.Items) != 8 || first.Total != 20 || !first.HasMore || first.Offset != 0 {
		t.Fatalf("первая страница неверна: %+v", first)
	}
	if first.Items[0].NoteID != 1 {
		t.Fatalf("первая страница должна начинаться с первого продукта, получено %d", first.Items[0].NoteID)
	}

	last := paginateCombatItems(items, 16, 8)
	if len(last.Items) != 4 || last.HasMore || last.Offset != 16 {
		t.Fatalf("последняя страница неверна: %+v", last)
	}
	if last.Items[0].NoteID != 17 {
		t.Fatalf("последняя страница должна начинаться с 17-го, получено %d", last.Items[0].NoteID)
	}

	// За краем списка — пустая страница без паники.
	beyond := paginateCombatItems(items, 100, 8)
	if len(beyond.Items) != 0 || beyond.HasMore || beyond.Total != 20 {
		t.Fatalf("за краем — пусто, получено: %+v", beyond)
	}
}

// TestPaginateCombatItemsDefaults — нечитаемое и нулевое «смещение/размер» не
// ломают ответ: молча становится умолчанием.
func TestPaginateCombatItemsDefaults(t *testing.T) {
	items := make([]botSystemItem, 30)
	got := paginateCombatItems(items, -5, 0)
	if got.Offset != 0 || len(got.Items) != combatPageSizeDefault {
		t.Fatalf("умолчание не применилось: %+v", got)
	}
}

// TestPaginateCombatItemsCapsLimit — бот не может попросить больше потолка:
// одна клавиатура с сотнями кнопок не должна отправиться.
func TestPaginateCombatItemsCapsLimit(t *testing.T) {
	items := make([]botSystemItem, 1000)
	got := paginateCombatItems(items, 0, 5000)
	if len(got.Items) != combatPageSizeMax {
		t.Fatalf("потолок размера страницы не сработал: %d", len(got.Items))
	}
}

// TestFilterThenPaginate — поиск сужает список ДО пагинации: то, что менеджер
// видит первую страницу «по запросу», не должно терять совпадения с других
// страниц (кнопка «Дальше» внутри поиска листает именно совпадения).
func TestFilterThenPaginate(t *testing.T) {
	items := make([]botSystemItem, 0, 20)
	for i := 0; i < 10; i++ {
		items = append(items, botSystemItem{NoteID: int64(i + 1), Title: "Мембрана"})
		items = append(items, botSystemItem{NoteID: int64(i + 101), Title: "Плитка"})
	}
	filtered := filterCombatItems(items, "мембрана")
	page := paginateCombatItems(filtered, 0, 8)
	if page.Total != 10 || !page.HasMore {
		t.Fatalf("фильтр должен оставить 10 совпадений с остатком: %+v", page)
	}
	for _, it := range page.Items {
		if it.Title != "Мембрана" {
			t.Fatalf("в выдаче по запросу чужой продукт: %+v", it)
		}
	}
}
