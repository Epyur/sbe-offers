package main

import (
	"strings"
	"testing"
)

// Заголовки и списки в ответе модели печатаются структурой документа
// (2026-11-06, живой КП 003/2026).
//
// Симптом: макет просил «структурные разделы помечай как заголовки второго
// уровня», модель прислала строки-заголовки обычным текстом, и в документе они
// выглядели как тело. Списки тоже печатались по одной строке без `w:numPr`:
// в живом КП списков не было вовсе, хотя инструкция просила их именно списком.
//
// Причина: у ответа модели в печати был ровно один вид — абзац. Здесь вводятся
// три вида строки, и вид решается ОДНИМ местом (layoutLineKind), чтобы печать и
// очистка не разошлись.

// TestLayoutAnswerKeepsHeadingStructure — строка-заголовок печатается заголовком
// второго уровня (outlineLvl="1"), а не основным текстом и не заголовком 1 уровня.
func TestLayoutAnswerKeepsHeadingStructure(t *testing.T) {
	body := renderLayoutAnswer(t, "## Горизонтальная часть\nМембрана укладывается на плиту.")
	if !strings.Contains(body, `<w:outlineLvl w:val="1"/>`) {
		t.Fatalf("строка-заголовок ## не получила второй уровень (outlineLvl=1):\n%s", body)
	}
	if strings.Contains(body, `<w:pStyle w:val="`+styleHeading1+`"`) {
		t.Fatalf("строка-заголовок ## стала заголовком 1 уровня:\n%s", body)
	}
	if strings.Contains(body, "##") {
		t.Fatal("знаки разметки дошли до документа")
	}
	if !strings.Contains(body, "Горизонтальная часть") {
		t.Fatal("текст заголовка пропал")
	}
	if !strings.Contains(body, "Мембрана укладывается на плиту.") {
		t.Fatal("текст под заголовком пропал")
	}
}

// TestLayoutAnswerKeepsLists — пункты списка печатаются списком, а не абзацами.
func TestLayoutAnswerKeepsLists(t *testing.T) {
	body := renderLayoutAnswer(t, "Преимущества:\n- прочный шов\n- ремонтопригодность\n- быстрый монтаж")
	if n := strings.Count(body, `<w:numPr>`); n != 3 {
		t.Fatalf("пунктов списка %d, ожидали 3", n)
	}
	for _, item := range []string{"прочный шов", "ремонтопригодность", "быстрый монтаж"} {
		if !strings.Contains(body, item) {
			t.Errorf("пункт %q пропал", item)
		}
	}
	if strings.Contains(body, "- прочный") {
		t.Fatal("маркер разметки напечатан текстом")
	}
}

// TestLayoutAnswerNumbersNumberedList — нумерованный список печатается номерами
// бланка, а не «1.» в тексте: иначе в документе две нумерации — своя и Word.
func TestLayoutAnswerNumbersNumberedList(t *testing.T) {
	body := renderLayoutAnswer(t, "Порядок работ:\n1. Подготовка основания\n2. Укладка мембраны")
	if n := strings.Count(body, `<w:numPr>`); n != 2 {
		t.Fatalf("пунктов нумерованного списка %d, ожидали 2", n)
	}
	if strings.Contains(body, ">1. Подготовка") || strings.Contains(body, ">2. Укладка") {
		t.Fatal("номер остался в тексте вместе с нумерацией Word")
	}
}

// TestLayoutAnswerNumberedSourceRefsStayText — «[1]» и «[7]» в тексте — это
// библиографические ссылки, а не списки: их трогать нельзя.
func TestLayoutAnswerNumberedSourceRefsStayText(t *testing.T) {
	body := renderLayoutAnswer(t, "Техническое описание приведено в листе [7], узлы — в альбоме [8].")
	if strings.Contains(body, "<w:numPr>") {
		t.Fatal("ссылка на источник превратилась в пункт списка")
	}
	if !strings.Contains(body, "[7]") || !strings.Contains(body, "[8]") {
		t.Fatal("ссылки на источники пропали из текста")
	}
}

// TestLayoutAnswerHeadingsNotForFirstLine — короткая строка без пустой строки
// перед ней не считается заголовком: в живом ответе первая строка описания —
// обычный текст в одну строку, и поднимать её в заголовок значит менять смысл.
func TestLayoutAnswerPlainStartStaysBody(t *testing.T) {
	body := renderLayoutAnswer(t, "Закрывает весь подземный контур одним комплектом.\n\nВторой абзац.")
	if strings.Contains(body, `<w:pStyle w:val="`+styleHeading1+`"`) {
		t.Fatal("первая строка ответа стала заголовком без просьбы модели")
	}
}

// TestLayoutAnswerEmptyMarkersDropped — строка из одних знаков разметки не
// становится пустым абзацем: в живом ответе был такой хвост («##.»).
func TestLayoutAnswerEmptyMarkersDropped(t *testing.T) {
	body := renderLayoutAnswer(t, "Текст.\n\n##\n\nЕщё текст.")
	if strings.Contains(body, "##") {
		t.Fatal("пустая строка разметки попала в документ")
	}
}
