package main

import (
	"strings"
	"testing"
)

// Типографика ответа модели (2026-11-10).
//
// Живая жалоба: в тексте КП, написанном моделью, много пропусков точек, двойные
// пробелы и слипшиеся слова. Модель не машинистка: её ответ надо приводить к
// обычному печатному виду, не меняя смысла. Здесь проверяется исправление:
//   - двойных и более пробелов;
//   - пробела перед знаком препинания;
//   - пропущенного пробела после знака конца фразы и запятой;
//   - «cлипшихся» слов вида «текст,который».

func TestNormalizeBodyTypographyCollapsesDoubleSpaces(t *testing.T) {
	got := normalizeBodyTypography("Мембрана   защищает    фундамент.")
	if got != "Мембрана защищает фундамент." {
		t.Fatalf("двойные пробелы не схлопнуты: %q", got)
	}
}

func TestNormalizeBodyTypographyRemovesSpaceBeforePunctuation(t *testing.T) {
	got := normalizeBodyTypography("Мембрана защищает фундамент . Это важно .")
	if got != "Мембрана защищает фундамент. Это важно." {
		t.Fatalf("пробел перед точкой не убран: %q", got)
	}
}

func TestNormalizeBodyTypographyAddsSpaceAfterPunctuation(t *testing.T) {
	got := normalizeBodyTypography("Первое предложение.Второе предложение,и ещё одно.")
	want := "Первое предложение. Второе предложение, и ещё одно."
	if got != want {
		t.Fatalf("пробел после знака препинания не добавлен:\n получили: %q\n ожидали: %q", got, want)
	}
}

// Числа и аббревиатуры не трогаем: «1,5», «г/м2», «т.е.» — не конец фразы.
func TestNormalizeBodyTypographyKeepsNumbersAndUnits(t *testing.T) {
	for _, s := range []string{
		"Площадь 1,5 га и толщина 2,0 мм.",
		"Плотность 150 г/м2 при нагрузке 5 кН.",
		"Марка V-SL 2 мм.",
	} {
		if got := normalizeBodyTypography(s); got != s {
			t.Errorf("исправный текст изменён:\n было:  %q\n стало: %q", s, got)
		}
	}
}

// Не трогаем адреса и марки с точками внутри.
func TestNormalizeBodyTypographyKeepsURLs(t *testing.T) {
	s := "Подробнее: https://nav.tn.ru/systems/a/ и дальше."
	if got := normalizeBodyTypography(s); got != s {
		t.Fatalf("адрес изменён: %q", got)
	}
}

// Разбиение ответа на абзацы (2026-11-10) проверяется на общем пути печати:
// paragraphsXML печатает абзацем каждую непустую строку, пустые строки
// разделяют и не создают пустых абзацев.

// Сборка документа: текст «по промту» с двумя абзацами из кеша печатается
// ДВУМЯ абзацами, а не одним полотном (2026-11-10). Попутно проверяется, что
// типографика исправлена при печати из кеша (двойной пробел и слипшаяся фраза
// чистятся `generatedTextFor` → `normalizeBodyTypography`).
func TestPromptTextNodePrintsParagraphs(t *testing.T) {
	offer := testOfferForBlocks()
	offer.FormValues = map[string]string{
		genKeySection:                 hashOfferFacts(offer),
		genKeySystems:                 "",
		genKeyTextByID("desc", 0, ""): "Первый  абзац.Второй абзац описания.\n\nТретий абзац описания.",
	}
	form := promptForm()
	data, err := renderOfferDocx(form, offer)
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	xml := docXMLOf(t, data)
	// Типографика исправлена: двойной пробел схлопнут, после точки пробел есть.
	if !strings.Contains(xml, "Первый абзац. Второй абзац описания.") {
		t.Fatal("типографика текста из кеша не исправлена")
	}
	if got := strings.Count(xml, "абзац описания."); got < 2 {
		t.Fatalf("ожидали минимум два абзаца описания, насчитали %d", got)
	}
	// Два отдельных <w:p> между абзацами: закрывающий тег предыдущего стоит
	// до начала следующего.
	first := strings.Index(xml, "Первый абзац. Второй абзац описания.")
	second := strings.Index(xml, "Третий абзац описания.")
	if first < 0 || second < 0 {
		t.Fatalf("абзацы не найдены в документе: first=%d second=%d", first, second)
	}
	between := xml[first:second]
	if !strings.Contains(between, "</w:p>") {
		t.Fatal("абзацы напечатаны одним <w:p>, а не двумя")
	}
}
