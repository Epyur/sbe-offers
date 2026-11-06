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
