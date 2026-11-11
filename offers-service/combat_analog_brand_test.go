package main

import (
	"strings"
	"testing"
)

// Бренд конкурента и разнообразие подсказки (2026-11-12, живая жалоба).
//
// «По ПВХ мембранам находятся одни и те же мембраны пластфойл, хотя есть ещё
// мембраны Sika и Фахманн». Правило: в подсказке не больше
// `analogPerBrandLimit` продуктов одного бренда.

func TestCompetitorBrandOf(t *testing.T) {
	cases := []struct{ title, want string }{
		{"ПВХ мембрана PLASTFOIL® CLASSIC", "PLASTFOIL"},
		{"Кровельная ПВХ мембрана FACHMANN PVC-RP 2,0мм", "FACHMANN"},
		{"Sikaplan® G-12 RU", "Sikaplan"},
		{"Sika Roof® Cleaner 2000 RU", "Sika"},
		{"PIRRO®VentiDuct", "PIRRO"},
		{"Кровельная ПВХ-мембрана PVC-RP", ""},
	}
	for _, c := range cases {
		if got := competitorBrandOf(c.title); got != c.want {
			t.Fatalf("%q: бренд %q, ожидался %q", c.title, got, c.want)
		}
	}
}

// TestDiversifyByBrand — подсказка не состоит из одного бренда.
func TestDiversifyByBrand(t *testing.T) {
	var items []SystemCard
	for i := 0; i < 9; i++ {
		items = append(items, SystemCard{NoteID: int64(i + 1), Title: "ПВХ мембрана PLASTFOIL® " + itoa(i)})
	}
	for i := 0; i < 6; i++ {
		items = append(items, SystemCard{NoteID: int64(100 + i), Title: "Sikaplan® G-" + itoa(i)})
	}
	got := diversifyByBrand(items, analogPerBrandLimit)
	plast, sika := 0, 0
	for _, card := range got {
		switch competitorBrandOf(card.Title) {
		case "PLASTFOIL":
			plast++
		case "Sikaplan":
			sika++
		}
	}
	if plast > analogPerBrandLimit || sika > analogPerBrandLimit {
		t.Fatalf("бренд превысил предел: PLASTFOIL %d, Sikaplan %d (%d всего)", plast, sika, len(got))
	}
	if len(got) != 4 {
		t.Fatalf("ожидали 4 карточки (по две с бренда), получили %d", len(got))
	}
}

// TestAnalogCoreOf — ядро названия отбрасывает применения.
func TestAnalogCoreOf(t *testing.T) {
	cases := []struct{ title, want string }{
		{"Кровельная ПВХ-мембрана LOGICROOF® V-RP", "ПВХ мембрана"},
		{"Теплоизоляционные плиты LOGICPIR® PROF Ф/Ф", "плиты"},
		{"Плиты теплоизоляционные LOGICPIR® VENT", "Плиты"},
	}
	for _, c := range cases {
		if got := analogCoreOf(c.title); got != c.want {
			t.Fatalf("%q: ядро %q, ожидалось %q", c.title, got, c.want)
		}
	}
}

// TestAnalogStrongClassLive — класс товара из названий боевой базы.
func TestAnalogStrongClassLive(t *testing.T) {
	cases := []struct{ title, class string }{
		{"Кровельная ПВХ-мембрана LOGICROOF® V-RP", "мембран"},
		{"Кровельная ПВХ мембрана FACHMANN PVC-RP 2,0мм 2_15м, серая", "мембран"},
		{"Плиты теплоизоляционные LOGICPIR® VENT", "плит"},
	}
	for _, c := range cases {
		if got := analogStrongClassOf(c.title); got != c.class {
			t.Fatalf("%q: класс %q, ожидался %q", c.title, got, c.class)
		}
	}
}

// Карточка, у которой класс виден только в ОПИСАНИИ (Sikaplan), тоже должна
// попадать в подсказку: название «Sikaplan® G-12 RU» ни «мембраны», ни «плиты»
// не несёт, а продукт — мембрана.
func TestAnalogClassFromDescription(t *testing.T) {
	title := "Sikaplan® G-12 RU"
	desc := "армированный полиэстером многослойный полимерный рулонный материал " +
		"для кровельной гидроизоляции на основе поливинилхлорида (ПВХ)"
	if got := analogStrongClassIn(title + " " + desc); got != "мембран" && got != "гидроиз" {
		t.Fatalf("класс по описанию %q — ожидался мембранный", got)
	}
}

func TestAnalogSearchQueriesLive(t *testing.T) {
	membrane := analogSearchQueries(SystemCard{Title: "Кровельная ПВХ-мембрана LOGICROOF® V-RP"})
	t.Logf("мембрана: %q", membrane)
	// Общий классовый запрос идёт ПЕРВЫМ: по живому прогону именно он находит
	// ФАХМАНН и PVC-RP, тогда как «ПВХ мембрана» — девять PLASTFOIL подряд.
	if len(membrane) == 0 || !strings.Contains(strings.ToLower(membrane[0]), "мембран") {
		t.Fatalf("первый запрос не про класс мембраны: %q", membrane)
	}
	if !strings.Contains(strings.ToLower(membrane[0]), "кровел") {
		t.Fatalf("первый запрос без применения — не найдёт других брендов: %q", membrane)
	}

	plates := analogSearchQueries(SystemCard{Title: "Плиты теплоизоляционные LOGICPIR® VENT"})
	t.Logf("плиты: %q", plates)
	if len(plates) == 0 || !strings.Contains(strings.ToLower(plates[0]), "плит") {
		t.Fatalf("плиты: первый запрос не про класс: %q", plates)
	}
}
