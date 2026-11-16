package main

// Сторожа «бриф — не зона окна» (0.11.3, 2026-11-16).
//
// История: 0.11.1 добавил зону `brief`, 0.11.2 довёл её до редактора и
// приложения. Пользователь попросил другое — вызов брифа из карточки каждого
// раздела, — и зона стала настройкой, которая ничего не делает (дефект по
// AGENTS.md). Здесь держится её вывод: бриф остаётся функцией, зоны нет.
//
// Тесты красные на коде 0.11.2 (проверено откатом): там зона была в наборе,
// дописывалась в умолчание и вычищалась не была.

import (
	"strings"
	"testing"
)

// TestBriefIsNotWindowZone — зоны брифа нет ни в наборе, ни в умолчании.
func TestBriefIsNotWindowZone(t *testing.T) {
	if knownWindowZone(windowZoneBrief) {
		t.Fatal("зона brief не должна быть известной: её нет в наборе окна")
	}
	for _, item := range effectiveWindow(openTestForm()) {
		if item.Kind == windowKindZone && item.ID == windowZoneBrief {
			t.Fatalf("бриф не должен появляться в окне по умолчанию: %+v", item)
		}
	}
}

// TestEffectiveWindowDropsBriefZone — окно, сохранённое в 0.11.2, приходит с
// `zone:brief`. На чтении вычищается: иначе в редакторе останется строка,
// которой в приложении ничего не соответствует.
func TestEffectiveWindowDropsBriefZone(t *testing.T) {
	form := openTestForm()
	form.Window = []WindowItem{
		{Kind: windowKindZone, ID: windowZoneClient, Visible: true},
		{Kind: windowKindZone, ID: windowZoneBrief, Visible: true},
		{Kind: windowKindZone, ID: "systems", Visible: true},
		{Kind: windowKindField, ID: "object_name", Visible: true},
	}
	got := effectiveWindow(form)
	if indexOfZone(got, windowZoneBrief) >= 0 {
		t.Fatalf("устаревшая зона brief осталась в окне: %s", zoneOrderText(got))
	}
	// Порядок остальных элементов не изменился.
	if got[0].ID != windowZoneClient || got[1].ID != "systems" {
		t.Fatalf("вычистка сдвинула порядок: %s", zoneOrderText(got))
	}
}

// TestSaveDropsBriefZone — старый плагин присылает `zone:brief` на запись.
// Сохранение проходит, зона в базу не пишется.
func TestSaveDropsBriefZone(t *testing.T) {
	incoming := []WindowItem{
		{Kind: windowKindZone, ID: windowZoneClient, Visible: true},
		{Kind: windowKindZone, ID: windowZoneBrief, Visible: true},
		{Kind: windowKindZone, ID: "systems", Visible: true},
	}
	got := dropLegacyZones(incoming)
	if indexOfZone(got, windowZoneBrief) >= 0 {
		t.Fatalf("зона brief не вычищена перед записью: %s", zoneOrderText(got))
	}
	if len(got) != 2 {
		t.Fatalf("после вычистки %d элементов, ожидали 2", len(got))
	}
}

// TestBriefNotRequiredZone — обязательной бриф быть не может: его больше нет
// в окне, и подсветки ему некуда рисоваться.
func TestBriefNotRequiredZone(t *testing.T) {
	form := openTestForm()
	form.Window = []WindowItem{{Kind: windowKindZone, ID: windowZoneBrief, Visible: true, Required: true}}
	if msg := validateWindow(form); msg == "" {
		t.Fatal("зона brief должна отвергаться как неизвестная")
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
