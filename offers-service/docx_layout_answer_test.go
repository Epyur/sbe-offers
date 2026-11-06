package main

import (
	"strings"
	"testing"
)

// Живой ответ модели в макете КП 003/2026 (2026-11-06, сборка 11:21).
//
// Три дефекта одного выпуска, все видны в одном документе:
//
//  1. `##` Markdown-заголовки ушли в документ литералом — промт просил
//     «помечай заголовки второго уровня в разметке ворда», модель поняла это
//     как Markdown, а макет печатает текст как есть.
//  2. Русская строка с одним английским словом («Given the risk, I'll frame
//     group 3 as …») уцелела: `mostlyCyrillic` считает её русской, если
//     русских букв не меньше латинских.
//  3. Весь ответ — один абзац: переводы строк внутри `<w:t>` Word не
//     показывает, поэтому абзацы слиплись.
const liveLayoutAnswer = `## Гидроизоляция плиты фундамента (горизонтальная часть)
## Гидроизоляция стен фундамента (вертикальная часть)
## Утеплённая отмостка

Given the risk, I'll frame group 3 as "Конструкция отмостки и примыканий".

## Гидроизоляционная мембрана: двойной сварной шов с контролем герметичности
## Теплоизоляция XPS Carbon Prof

## Мембрана LOGICBASE V-SL: сварной шов проверяется воздухом, а не на глаз
## Геотекстиль 500 г/м² держит мембрану целой при обратной засыпке
##.`

// Markdown-заголовок модели уходит в документ знаком `##` — метка разметки
// это не текст для заказчика.
func TestCleanBlockTextDropsMarkdownHeadings(t *testing.T) {
	out := cleanBlockText(liveLayoutAnswer)
	if strings.Contains(out, "##") {
		t.Fatalf("в тексте остались Markdown-заголовки:\n%s", out)
	}
	for _, want := range []string{
		"Гидроизоляция плиты фундамента",
		"Теплоизоляция XPS Carbon Prof",
		"Геотекстиль 500 г/м²",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("потерян текст заголовка %q:\n%s", want, out)
		}
	}
}

// Строка, где английского не меньше русского, — рассуждение, а не текст.
func TestCleanBlockTextDropsMixedReasoningLine(t *testing.T) {
	out := cleanBlockText(liveLayoutAnswer)
	for _, bad := range []string{"Given the risk", "I'll frame", "group 3"} {
		if strings.Contains(out, bad) {
			t.Fatalf("рассуждение модели осталось в тексте (%q):\n%s", bad, out)
		}
	}
}

// Висящая строка из одних знаков («##.») не должна печататься.
func TestCleanBlockTextDropsPunctuationOnlyLine(t *testing.T) {
	out := cleanBlockText(liveLayoutAnswer)
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.TrimSpace(strings.Trim(trimmed, " .,;:—-–«»\"'()[]{}#*")) == "" {
			t.Fatalf("в тексте строка без слов: %q\n%s", line, out)
		}
	}
}

// Абзацы ответа остаются отдельными строками: в документе их разведёт печать.
func TestCleanBlockTextKeepsParagraphs(t *testing.T) {
	out := cleanBlockText(liveLayoutAnswer)
	if lines := strings.Count(out, "\n"); lines < 3 {
		t.Fatalf("абзацы слиплись (%d переводов строк):\n%s", lines, out)
	}
}

// Многострочный ответ модели печатается НЕСКОЛЬКИМИ абзацами Word.
//
// Живой документ 003/2026 (2026-11-06, 11:21): весь ответ лёг в один `<w:p>`
// с переводами строк внутри `<w:t>`, Word их не показывает — и в документе
// была одна сплошная строка. Оформление задаётся вёрсткой абзаца, значит
// абзацев должно быть столько, сколько строк ответа.
func TestLayoutPromptPrintsParagraphs(t *testing.T) {
	doc := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p><w:pPr><w:jc w:val="both"/><w:ind w:firstLine="709"/></w:pPr>` +
		`<w:r><w:rPr><w:rFonts w:ascii="Arial"/></w:rPr><w:t>{Опиши системы}</w:t></w:r></w:p>` +
		`</w:body></w:document>`
	files := map[string][]byte{
		"[Content_Types].xml": []byte(`<?xml version="1.0" encoding="UTF-8"?><Types/>`),
		"word/document.xml":   []byte(doc),
	}
	data, err := zipFiles(files, []string{"[Content_Types].xml", "word/document.xml"})
	if err != nil {
		t.Fatalf("сборка макета: %v", err)
	}
	answer := "Гидроизоляция плиты фундамента\n\nВертикальная часть\n\nОтмостка"
	out, err := renderLayoutDocx(data, layoutContext{
		values: map[string]string{},
		prompt: func(string) string { return answer },
	})
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if got := strings.Count(body, "<w:p>") + strings.Count(body, "<w:p "); got < 3 {
		t.Fatalf("абзацев %d, ожидалось 3:\n%s", got, body)
	}
	for _, want := range []string{"Гидроизоляция плиты фундамента", "Вертикальная часть", "Отмостка"} {
		if !strings.Contains(body, want) {
			t.Fatalf("потерян абзац %q:\n%s", want, body)
		}
	}
	// Вёрстка метки (по ширине и красная строка) — у КАЖДОГО абзаца, иначе
	// второй и следующие выйдут в стиле по умолчанию.
	if n := strings.Count(body, `w:jc w:val="both"`); n < 3 {
		t.Fatalf("выравнивание скопировано не всем абзацам (%d):\n%s", n, body)
	}
	if n := strings.Count(body, `w:firstLine="709"`); n < 3 {
		t.Fatalf("красная строка потеряна у части абзацев (%d):\n%s", n, body)
	}
}
