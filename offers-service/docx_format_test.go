package main

import (
	"strings"
	"testing"
)

// Оформление текста (2026-11-05).
//
// Живой КП 003/2026 вышел «без выравнивания»: стиль Normal бланка прижат
// влево, без отступа первой строки, шрифт Arial 11. Пользователь ждал обычного
// печатного текста. Правило документов экосистемы (sbe-agent, «Стандартное
// оформление Word»): основной текст — по ширине, первая строка 1,25 см,
// полуторный интервал. Стиль бланка при этом НЕ меняется (2026-10-29: правка
// styles.xml уносила нумерацию страниц в колонтитуле), поэтому оформление
// задаётся в pPr каждого абзаца.

// Основной текст: по ширине, красная строка 709 twips, полуторный интервал.
func TestParagraphHasBodyFormatting(t *testing.T) {
	xml := paragraphAligned("Обычный текст предложения.", "")
	for _, want := range []string{
		`<w:jc w:val="both"/>`,
		`<w:ind w:firstLine="709"/>`,
		`w:line="360"`,
	} {
		if !strings.Contains(xml, want) {
			t.Errorf("в абзаце нет %s:\n%s", want, xml)
		}
	}
}

// Явное выравнивание блока главнее оформления документа.
func TestParagraphExplicitAlignmentWins(t *testing.T) {
	xml := paragraphAligned("Строка по центру", "center")
	if !strings.Contains(xml, `<w:jc w:val="center"/>`) {
		t.Fatalf("явное выравнивание потеряно:\n%s", xml)
	}
}

// Заголовок — по центру и без красной строки.
func TestHeadingIsCenteredWithoutIndent(t *testing.T) {
	xml := headingXML("Состав системы", 3)
	if !strings.Contains(xml, `<w:jc w:val="center"/>`) {
		t.Fatalf("заголовок не по центру:\n%s", xml)
	}
	if !strings.Contains(xml, `<w:ind w:firstLine="0"/>`) {
		t.Fatalf("у заголовка осталась красная строка:\n%s", xml)
	}
}

// Уровни заголовков (2026-11-08, живая жалоба на КП 003/2026):
// в настройках блоков указан заголовок 2 уровня, а в документ он вставлялся
// стилем heading 1 (outlineLvl 0) и попадал в оглавление как раздел 1-го уровня.
// Только level=1 получает styleHeading1; уровни 2, 3 и 4 получают свой
// outlineLvl="1", "2", "3", чтобы Word и оглавление `TOC \o "1-3" \u` строили
// правильную иерархию.
func TestHeadingLevelsProduceCorrectOutlineLvl(t *testing.T) {
	h1 := headingXML("Введение", 1)
	if !strings.Contains(h1, `<w:pStyle w:val="`+styleHeading1+`"/>`) {
		t.Fatalf("заголовок 1 уровня не получил styleHeading1:\n%s", h1)
	}
	if !strings.Contains(h1, `<w:outlineLvl w:val="0"/>`) {
		t.Fatalf("заголовок 1 уровня не получил outlineLvl=0:\n%s", h1)
	}

	h2 := headingXML("Перечень систем", 2)
	if strings.Contains(h2, `<w:pStyle w:val="`+styleHeading1+`"/>`) {
		t.Fatalf("заголовок 2 уровня получил styleHeading1 (станет 1 уровнем в Word):\n%s", h2)
	}
	if !strings.Contains(h2, `<w:outlineLvl w:val="1"/>`) {
		t.Fatalf("заголовок 2 уровня не получил outlineLvl=1:\n%s", h2)
	}

	h0 := headingXML("Без явного уровня (по умолчанию 2)", 0)
	if strings.Contains(h0, `<w:pStyle w:val="`+styleHeading1+`"/>`) || !strings.Contains(h0, `<w:outlineLvl w:val="1"/>`) {
		t.Fatalf("заголовок с level=0 должен быть 2 уровня (outlineLvl=1):\n%s", h0)
	}

	h3 := headingXML("Подзаголовок", 3)
	if !strings.Contains(h3, `<w:outlineLvl w:val="2"/>`) {
		t.Fatalf("заголовок 3 уровня не получил outlineLvl=2:\n%s", h3)
	}

	h4 := headingXML("Состав системы", 4)
	if !strings.Contains(h4, `<w:outlineLvl w:val="3"/>`) {
		t.Fatalf("заголовок 4 уровня не получил outlineLvl=3:\n%s", h4)
	}
}

// Пункт списка — без красной строки: отступ даёт нумерация бланка.
func TestListItemHasNoFirstLineIndent(t *testing.T) {
	xml := listItemXML("Пункт списка")
	if !strings.Contains(xml, `<w:ind w:firstLine="0"/>`) {
		t.Fatalf("у пункта списка красная строка:\n%s", xml)
	}
}

// Таблица остаётся лишённой красной строки: ячейка с отступом читается как сбой.
func TestTableCellHasNoIndent(t *testing.T) {
	xml := tableCellXML("Ячейка", false)
	if !strings.Contains(xml, `<w:ind w:firstLine="0"/>`) {
		t.Fatalf("в ячейке появилась красная строка:\n%s", xml)
	}
}
