package main

import (
	"strings"
	"testing"
)

// Дефекты живого КП 003/2026 (2026-11-05), каждый — отдельный тест.
//
// Пользователь увидел в документе: вики-ссылки вокруг материалов состава,
// слипшиеся слова из заметки («подземныхсооружений») и степень в единицах
// измерения, напечатанную обычной цифрой («г/м2» вместо «г/м²»).

// Вики-ссылка из заметки печатается подписью, а не разметкой Obsidian.
func TestPlainTextDropsWikiLinks(t *testing.T) {
	got := plainText("[[Гидрошпонка ТЕХНОНИКОЛЬ ЕС-220-3]]")
	if got != "Гидрошпонка ТЕХНОНИКОЛЬ ЕС-220-3" {
		t.Fatalf("ожидалась подпись, получено %q", got)
	}
	if strings.Contains(got, "[[") || strings.Contains(got, "]]") {
		t.Fatalf("скобки Obsidian остались в тексте: %q", got)
	}
}

// Вложенная скобка «[[Цель|Подпись]]» отдаёт подпись.
func TestPlainTextWikiLinkUsesAlias(t *testing.T) {
	got := plainText("см. [[Мембрана V-SL|мембрану V-SL]]")
	if got != "см. мембрану V-SL" {
		t.Fatalf("получено %q", got)
	}
}

// Слипшиеся слова из живой карточки: пробел возвращается.
func TestPlainTextRepairsGluedWords(t *testing.T) {
	cases := map[string]string{
		"защиты подземныхсооружений с эксплуатируемыми": "защиты подземных сооружений с эксплуатируемыми",
		"грунтах с высоким уровнемподземных вод":        "грунтах с высоким уровнем подземных вод",
		"глинистых грунтах независимоот уровня":         "глинистых грунтах независимо от уровня",
		"помещениями вместных песчаных грунтах":         "помещениями в местных песчаных грунтах",
	}
	for in, want := range cases {
		if got := plainText(in); got != want {
			t.Errorf("plainText(%q)\nполучено: %q\nожидалось: %q", in, got, want)
		}
	}
}

// Обычные слова не должны терять буквы от «починки» пробелов.
func TestPlainTextDoesNotSplitNormalWords(t *testing.T) {
	cases := []string{
		"строительство и эксплуатация",
		"уровень воды в котловане",
		"мембрана LOGICBASE V-SL защищает",
		"Гидроизоляционная ПВХ мембрана",
	}
	for _, in := range cases {
		if got := plainText(in); got != in {
			t.Errorf("обычный текст изменён: %q → %q", in, got)
		}
	}
}

// Степень после единицы измерения становится надстрочной цифрой.
func TestSplitSupsubMarksUnitDegree(t *testing.T) {
	runs := splitSupsub("геотекстиль 500 г/м2")
	if len(runs) < 2 {
		t.Fatalf("ожидались отдельные прогоны, получено %d", len(runs))
	}
	last := runs[len(runs)-1]
	if !last.Sup || last.Text != "2" {
		t.Fatalf("последняя двойка должна быть надстрочной, получено %+v", last)
	}
	if got := joinRuns(runs); got != "геотекстиль 500 г/м2" {
		t.Fatalf("текст прогонов не совпал с исходным: %q", got)
	}
}

// «м2» и «см3» тоже степени, а «V-SL 2 мм» — нет: это толщина.
func TestSplitSupsubAreaAndThickness(t *testing.T) {
	runs := splitSupsub("площадью до 150 м2 при толщине 2 мм")
	sup := 0
	for _, r := range runs {
		if r.Sup {
			sup++
			if r.Text != "2" {
				t.Fatalf("надстрочной оказалась %q", r.Text)
			}
		}
	}
	if sup != 1 {
		t.Fatalf("ожидалась ровно одна надстрочная цифра, получено %d", sup)
	}
	if got := joinRuns(runs); got != "площадью до 150 м2 при толщине 2 мм" {
		t.Fatalf("текст изменился: %q", got)
	}
}

// Марка материала не должна терять цифру: «AL2» — часть названия.
func TestSplitSupsubLeavesBrandCodes(t *testing.T) {
	runs := splitSupsub("профиль AL2 и марка V-SL2")
	for _, r := range runs {
		if r.Sup || r.Sub {
			t.Fatalf("в марке материала поднята цифра: %+v", r)
		}
	}
}

func joinRuns(runs []textRun) string {
	var b strings.Builder
	for _, r := range runs {
		b.WriteString(r.Text)
	}
	return b.String()
}
