package main

import (
	"strings"
	"testing"
)

// Текст ответа модели в печати по макету проходит тот же конвейер, что и блоки
// (2026-11-06, живой КП 003/2026).
//
// Симптом: в описательной части документа площади напечатаны как «5520 м2»,
// вики-ссылки через загруженный макет дошли бы до читателя со скобками, а
// слипшиеся слова из карточки («подземныхсооружений») так и остались бы
// слипшимися. Причина: layoutAsParagraphs писал `<w:t>` через escapeXML
// напрямую, а plainText + runsXML (docx_runs.go) — единственный конвейер
// текста — вызывался только в блочном пути. Правило 2026-11-05: конвейер один,
// иначе два пути печати расходятся молча.

// paragraphsOfLayout — текст модели, напечатанный по макету: строки ответа.
func renderLayoutAnswer(t *testing.T, answer string) string {
	t.Helper()
	layout := layoutDocxFromXML(t, layoutWithRefsDoc)
	data, err := renderLayoutDocx(layout, layoutContext{
		values: map[string]string{},
		prompt: func(string) string { return answer },
	})
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	return string(docxEntry(t, data, "word/document.xml"))
}

// TestLayoutAnswerMakesSuperscripts — «5520 м2» печатается с надстрочной двойкой.
//
// Читатель видит площадь в квадратных метрах, а не «м2» обычными символами:
// это требование пользователя 2026-11-05 («индексы в единицах измерения должны
// отображаться правильно»).
func TestLayoutAnswerMakesSuperscripts(t *testing.T) {
	body := renderLayoutAnswer(t, "Площадь горизонтали 5520 м2, вертикали 1400 м2.")
	if n := strings.Count(body, `<w:vertAlign w:val="superscript"/>`); n != 2 {
		t.Fatalf("надстрочных индексов %d, ожидали 2", n)
	}
	if !strings.Contains(body, `>2</w:t>`) {
		t.Fatal("сама цифра степени не напечатана")
	}
}

// TestLayoutAnswerKeepsBrandsIntact — «V-SL 2 мм» и «AL2» двойку не поднимают:
// это часть названия материала, а не степень.
func TestLayoutAnswerKeepsBrandsIntact(t *testing.T) {
	body := renderLayoutAnswer(t, "Мембрана LOGICBASE V-SL 2 мм и марка AL2.")
	if strings.Contains(body, `w:vertAlign`) {
		t.Fatal("двойку из названия материала подняли в надстрочную")
	}
}

// TestLayoutAnswerStripsWikiLinks — из карточки в текст не попадают квадратные
// скобки: читателю нужна подпись, а не разметка Obsidian.
func TestLayoutAnswerStripsWikiLinks(t *testing.T) {
	body := renderLayoutAnswer(t, "Слой [[Гидрошпонка ТЕХНОНИКОЛЬ ЕС-220-3]] ставится по шву.")
	if strings.Contains(body, "[[") || strings.Contains(body, "]]") {
		t.Fatal("вики-разметка дошла до документа")
	}
	if !strings.Contains(body, "Гидрошпонка ТЕХНОНИКОЛЬ ЕС-220-3") {
		t.Fatal("подпись вики-ссылки пропала")
	}
}

// TestLayoutAnswerRepairsGluedWords — слипшееся слово из карточки печатается
// с пробелом: в документе это ошибка, а не стиль заметки.
func TestLayoutAnswerRepairsGluedWords(t *testing.T) {
	body := renderLayoutAnswer(t, "Система для защиты подземныхсооружений в котловане.")
	if strings.Contains(body, "подземныхсооружений") {
		t.Fatal("слипшееся слово так и напечатано слипшимся")
	}
	if !strings.Contains(body, "подземных сооружений") {
		t.Fatal("пробел не восстановлен")
	}
}

// TestLayoutAnswerStripsBold — разметка жирного не печатается звёздочками.
func TestLayoutAnswerStripsBold(t *testing.T) {
	body := renderLayoutAnswer(t, "**Область применения:** подземные сооружения.")
	if strings.Contains(body, "**") {
		t.Fatal("звёздочки разметки дошли до документа")
	}
	if !strings.Contains(body, "Область применения:") {
		t.Fatal("текст выделенного фрагмента пропал")
	}
}

// TestLayoutFormattedTextKeepsSuperscripts — то же для подставимого значения
// формы: «Площадь: 4800 м2» из брифа тоже печатается с индексом.
func TestLayoutFormattedTextKeepsSuperscripts(t *testing.T) {
	const layoutWithField = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:r><w:t>Объект: [object_name]</w:t></w:r></w:p>
</w:body></w:document>`
	data, err := renderLayoutDocx(layoutDocxFromXML(t, layoutWithField), layoutContext{
		values: map[string]string{"object_name": "Дом, площадь 4800 м2"},
	})
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	body := string(docxEntry(t, data, "word/document.xml"))
	if !strings.Contains(body, `<w:vertAlign w:val="superscript"/>`) {
		t.Fatal("в значении поля формы степень не поднялась")
	}
}
