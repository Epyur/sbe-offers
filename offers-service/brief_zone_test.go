package main

// Сторожа дефекта «зоны брифа не видно» (2026-11-16, после совещания
// gemini-4-argon + claude-5-opus).
//
// Живая жалоба: «не вижу зоны бриф ни в настройках формы ни в миниап».
// Разбор нашёл три самостоятельные причины, и каждая проверяется здесь
// отдельно — так сторож называет виновника, а не «что-то сломалось».
//
// Все три теста НА СТАРОМ КОДЕ КРАСНЫЕ (прогнано откатом):
//   1. у формы без сохранённого окна зоны не было вовсе (defaultWindow по
//      legacyWindowZones) — теперь есть, невидимой;
//   2. дописанная зона вставала в самый конец, после полей формы — теперь
//      сразу за «Заказчиком» (якорь);
//   3. сохранение из старого клиента затирало включённую зону — теперь
//      состояние из базы сохраняется.

import (
	"strings"
	"testing"
)

// TestBriefZonePresentInDefaultWindow — дефект 1 (нашёл `gemini-4-argon`:
// ветка пустого окна не проходит нормализацию, но причина сформулирована им
// неточно — теряет не normalizeWindow, а сам defaultWindow).
func TestBriefZonePresentInDefaultWindow(t *testing.T) {
	got := effectiveWindow(openTestForm())
	if indexOfZone(got, windowZoneBrief) < 0 {
		t.Fatal("в окне по умолчанию нет зоны brief: включить её нечем (дефект 0.11.1)")
	}
}

// TestBriefZonePlacedAfterClient — дефект 2 (нашёл `claude-5-opus`: зона
// дописывалась в конец, то есть под таблицей полей формы).
func TestBriefZonePlacedAfterClient(t *testing.T) {
	form := openTestForm()
	form.Window = []WindowItem{
		{Kind: windowKindZone, ID: windowZoneClient, Visible: true},
		{Kind: windowKindField, ID: "client_name", Visible: true},
		{Kind: windowKindField, ID: "object_name", Visible: true},
	}
	got := effectiveWindow(form)

	briefAt := indexOfZone(got, windowZoneBrief)
	clientAt := indexOfZone(got, windowZoneClient)
	if briefAt < 0 {
		t.Fatal("бриф не дописан в сохранённое окно")
	}
	if briefAt != clientAt+1 {
		t.Fatalf("бриф должен стоять сразу за Заказчиком (позиция %d), получено %d: %s",
			clientAt+1, briefAt, zoneOrderText(got))
	}
}

// TestSaveKeepsZoneMissingFromOldClient — дефект 3 (нашёл `gemini-4-argon`:
// `window_config = EXCLUDED.window_config` затирал зону).
//
// Клиент 0.11.0 не знает `brief` и присылает окно без неё; в базе зона
// включена. Функция должна вернуть её из базы, а не потерять.
func TestSaveKeepsZoneMissingFromOldClient(t *testing.T) {
	stored := []WindowItem{
		{Kind: windowKindZone, ID: windowZoneClient, Visible: true},
		{Kind: windowKindZone, ID: windowZoneBrief, Visible: true},
		{Kind: windowKindZone, ID: "systems", Visible: true},
	}
	incoming := []WindowItem{
		{Kind: windowKindZone, ID: windowZoneClient, Visible: true},
		{Kind: windowKindZone, ID: "systems", Visible: true},
	}
	got := mergeStoredZones(incoming, stored)

	if indexOfZone(got, windowZoneBrief) < 0 {
		t.Fatal("сохранение из старого клиента потеряло включённую зону brief")
	}
	item := got[indexOfZone(got, windowZoneBrief)]
	if !item.Visible {
		t.Fatal("восстановленная зона должна остаться видимой, как в базе")
	}
	if at := indexOfZone(got, windowZoneBrief); at != indexOfZone(got, windowZoneClient)+1 {
		t.Fatalf("восстановленная зона должна встать на своё место (за Заказчиком), получено %s",
			zoneOrderText(got))
	}
}

// TestSaveKeepsExplicitHide — явное «скрыть» от нового клиента сохраняется:
// защита не должна возвращать зону, которую администратор сам снял.
func TestSaveKeepsExplicitHide(t *testing.T) {
	stored := []WindowItem{
		{Kind: windowKindZone, ID: windowZoneClient, Visible: true},
		{Kind: windowKindZone, ID: windowZoneBrief, Visible: true},
	}
	incoming := []WindowItem{
		{Kind: windowKindZone, ID: windowZoneClient, Visible: true},
		{Kind: windowKindZone, ID: windowZoneBrief, Visible: false},
	}
	got := mergeStoredZones(incoming, stored)
	at := indexOfZone(got, windowZoneBrief)
	if at < 0 {
		t.Fatal("зона пропала из сохранения")
	}
	if got[at].Visible {
		t.Fatal("явное «скрыть» проигнорировано: защита вернула зону из базы")
	}
}

// TestSaveEmptyWindowStaysEmpty — пустое окно законно и не подмешивает
// состояние из базы: администратор сознательно сбросил настройку.
func TestSaveEmptyWindowStaysEmpty(t *testing.T) {
	stored := []WindowItem{{Kind: windowKindZone, ID: windowZoneBrief, Visible: true}}
	if got := mergeStoredZones([]WindowItem{}, stored); len(got) != 0 {
		t.Fatalf("пустое окно должно остаться пустым, получено %d элементов", len(got))
	}
}

// zoneOrderText — порядок окна строкой: по отказу видно, что сломалось.
func zoneOrderText(items []WindowItem) string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.Kind+":"+item.ID)
	}
	return strings.Join(ids, ",")
}
