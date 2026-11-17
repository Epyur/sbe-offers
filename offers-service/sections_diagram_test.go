package main

// Сторожа вида блока `diagram` (2026-11-17, шаг 5 плана
// 2026-11-16-sbe-offers-combat-sections-and-agent-tools-plan.md).
//
// Проверяется то, что увидит менеджер в готовом файле:
//   - mermaid-схема печатается как настоящий `<w:drawing>` (inline) и подпись
//     «Схема N — …» стоит под ней отдельным абзацем;
//   - `lang:"svg"`/чужой язык и пустой `code` — блок ВЫБРОШЕН (§7.2: схемы
//     только mermaid, произвольный SVG не поддерживается);
//   - битый ответ рендера — ВИДИМАЯ строка «[Схема не отрисована: …]», а не
//     пустое место (§9.2), и остальные блоки подраздела на месте;
//   - размеры схемы в EMU: ширина ≤16 см, высота по пропорции PNG, слишком
//     высокая масштабируется по высоте листа;
//   - кириллический mermaid-код проходит тот же путь (golden: PNG без тофу
//     доказать нечем без настоящего рендера, но размеры и подпись — да).

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// diagramPNG — настоящий PNG заданного размера (рендер-заглушка отдаёт его же).
func diagramPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, color.RGBA{R: 200, G: 220, B: 240, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("PNG: %v", err)
	}
	return buf.Bytes()
}

// stubDiagramRender подменяет рендер на время теста.
func stubDiagramRender(t *testing.T, fn func(code string) sectionDiagramRendered) {
	t.Helper()
	prev := sectionDiagramRender
	sectionDiagramRender = fn
	t.Cleanup(func() { sectionDiagramRender = prev })
}

// diagramAnswer собирает ответ модели из одного блока diagram.
func diagramAnswer(lang, code, caption string) string {
	return `{"blocks":[{"kind":"diagram","lang":` + quoteJSON(lang) +
		`,"code":` + quoteJSON(code) + `,"caption":` + quoteJSON(caption) + `}]}`
}

func TestSectionDiagramPrintsDrawingAndCaption(t *testing.T) {
	pngData := diagramPNG(t, 320, 180)
	stubDiagramRender(t, func(code string) sectionDiagramRendered {
		if code != "flowchart TD; A-->B" {
			t.Fatalf("рендер вызван с чужим кодом: %q", code)
		}
		return sectionDiagramRendered{PNG: pngData, Width: 320, Height: 180}
	})
	section := ContentSection{
		ID: "proposal", Title: "Схема", Numbered: true,
		Subsections: []ContentSubsection{{ID: "d", Title: "Схема", Instruction: "Нарисуй схему"}},
	}
	lc := sectionFrameContext(section, map[string]string{
		"d": diagramAnswer("mermaid", "flowchart TD; A-->B", "Сравнение продуктов"),
	})
	lc.layout = &layoutRuntime{media: newTemplateMedia()}
	out, err := renderLayoutDocx(sectionMaster(t), lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if !strings.Contains(body, "<w:drawing>") {
		t.Fatalf("схема не напечатана как картинка:\n%s", body)
	}
	// wp:inline (не anchor): спека §9.2 требует inline для схем.
	if !strings.Contains(body, "<wp:inline") {
		t.Fatal("схема не inline: Word 2007 вокруг anchor «плывёт»")
	}
	if strings.Contains(body, "<wp:anchor") {
		t.Fatal("схема напечатана плавающей картинкой (anchor), нужна inline")
	}
	if !strings.Contains(body, "Схема 1 — Сравнение продуктов") {
		t.Fatalf("нет подписи «Схема 1 — …»:\n%s", body)
	}
	// Медиа ушла в пакет: картинка есть в word/media.
	if !strings.Contains(strings.Join(zipNames(t, out), " "), "word/media/offerimg") {
		t.Fatal("картинка схемы не добавлена в пакет")
	}
}

func TestSectionDiagramRejectsNonMermaid(t *testing.T) {
	stubDiagramRender(t, func(code string) sectionDiagramRendered {
		t.Fatal("рендер не должен вызываться для не-mermaid блока")
		return sectionDiagramRendered{}
	})
	for _, lang := range []string{"svg", "graphviz", "dot"} {
		lc := sectionFrameContext(ContentSection{ID: "c"}, nil)
		lc.layout = &layoutRuntime{media: newTemplateMedia()}
		block := sectionBlock{Kind: "diagram", Lang: lang, Code: "<svg></svg>"}
		if out := printSectionBlock(block, lc); out != "" {
			t.Fatalf("lang %q напечатал что-то: %q", lang, out)
		}
	}
}

func TestSectionDiagramEmptyCodeDropped(t *testing.T) {
	answer, ok := parseSectionAnswer(`{"blocks":[{"kind":"diagram","lang":"mermaid","code":"  "}]}`)
	if ok {
		t.Fatalf("схема с пустым кодом не должна выживать нормализацию: %+v", answer)
	}
}

// Битый рендер: видимая строка вместо пустого места, остальные блоки целы.
func TestSectionDiagramBrokenRenderVisiblyFallsBack(t *testing.T) {
	stubDiagramRender(t, func(string) sectionDiagramRendered {
		return sectionDiagramRendered{Reason: "сервис рендера ответил 500"}
	})
	section := ContentSection{
		ID: "proposal", Title: "Раздел", Numbered: true,
		Subsections: []ContentSubsection{{ID: "d", Title: "Схема и текст", Instruction: "Нарисуй схему и опиши"}},
	}
	answer := `{"blocks":[` +
		`{"kind":"paragraph","text":"Текст до схемы."},` +
		`{"kind":"diagram","lang":"mermaid","code":"flowchart TD; X-->Y","caption":"Что-то"},` +
		`{"kind":"paragraph","text":"Текст после схемы."}]}`
	lc := sectionFrameContext(section, map[string]string{"d": answer})
	lc.layout = &layoutRuntime{media: newTemplateMedia()}
	out, err := renderLayoutDocx(sectionMaster(t), lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if !strings.Contains(body, "[Схема не отрисована: сервис рендера ответил 500]") {
		t.Fatalf("нет видимой строки отказа схемы:\n%s", body)
	}
	if strings.Contains(body, "<w:drawing>") {
		t.Fatal("при битом рендере нарисовалась картинка")
	}
	// Соседи схемы на месте — один отказ не теряет подраздел.
	if !strings.Contains(body, "Текст до схемы.") || !strings.Contains(body, "Текст после схемы.") {
		t.Fatal("битая схема потеряла соседние блоки")
	}
}

// Размеры: ширина ≤16 см, высота по пропорции; слишком высокая — по высоте.
func TestSectionDiagramExtent(t *testing.T) {
	// Широкая схема 1600×900: ширина 16 см, высота ~9 см.
	cx, cy := sectionDiagramExtent(1600, 900)
	if cx != 16*360000 {
		t.Fatalf("ширина %d EMU, ожидалось 16 см", cx)
	}
	if got := float64(cy) / float64(cx); got < 0.55 || got > 0.58 {
		t.Fatalf("пропорция высоты %.3f, ожидалось ~0.5625", got)
	}
	// Очень высокая 400×4000: ужимается по высоте листа (20 см), ширина по
	// пропорции и остаётся ≤16 см.
	cx, cy = sectionDiagramExtent(400, 4000)
	if cy != 20*360000 {
		t.Fatalf("высота %d EMU, ожидалось 20 см при превышении", cy)
	}
	if cx > 16*360000 {
		t.Fatalf("после масштаба по высоте ширина %d больше 16 см", cx)
	}
	if got := float64(cx) / float64(cy); got < 0.09 || got > 0.11 {
		t.Fatalf("пропорция после масштаба %.3f, ожидалось 0.1", got)
	}
}

// Подпись: номер ставит сервер, двойного «Схема N» нет.
func TestSectionDiagramCaptionNoDoubleNumber(t *testing.T) {
	if got := sectionDiagramCaption(3, "Схема 3 — Сравнение"); got != "Схема 3 — Сравнение" {
		t.Fatalf("двойная нумерация не снята: %q", got)
	}
	if got := sectionDiagramCaption(2, "Схема 1: моё"); got != "Схема 2 — моё" {
		t.Fatalf("номер модели не заменён серверным: %q", got)
	}
	if got := sectionDiagramCaption(1, ""); got != "Схема 1" {
		t.Fatalf("подпись без текста: %q", got)
	}
	if got := sectionDiagramCaption(4, "Сравнение"); got != "Схема 4 — Сравнение" {
		t.Fatalf("простая подпись: %q", got)
	}
}

// Кириллический mermaid-код проходит тот же путь (golden: размеры и подпись —
// то, что можно доказать без настоящего рендера; читаемость шрифта проверяется
// на бою, стенд отдаёт PNG 1×1).
func TestSectionDiagramCyrillicCodeGolden(t *testing.T) {
	pngData := diagramPNG(t, 640, 480)
	var askedCode string
	stubDiagramRender(t, func(code string) sectionDiagramRendered {
		askedCode = code
		return sectionDiagramRendered{PNG: pngData, Width: 640, Height: 480}
	})
	cyrillic := "flowchart TD\n  A[\"Наш продукт\"] --> B[\"Конкурент\"]"
	lc := sectionFrameContext(ContentSection{ID: "c"}, nil)
	lc.layout = &layoutRuntime{media: newTemplateMedia()}
	block := sectionBlock{Kind: "diagram", Lang: "mermaid", Code: cyrillic, Caption: "Схема сравнения"}
	out := printSectionBlock(block, lc)
	if askedCode != cyrillic {
		t.Fatalf("в рендер ушёл не тот код: %q", askedCode)
	}
	if !strings.Contains(out, "<w:drawing>") || !strings.Contains(out, "Схема 1 — Схема сравнения") {
		t.Fatalf("кириллическая схема не напечаталась:\n%s", out)
	}
	// Размер 640×480 (4:3) → ширина 16 см, высота 12 см.
	if !strings.Contains(out, `cx="5760000"`) || !strings.Contains(out, `cy="4320000"`) {
		t.Fatalf("размеры схемы 640×480 не превратились в 16×12 см:\n%s", out)
	}
}

// zipNames — имена частей пакета (для проверки медиа).
func zipNames(t *testing.T, data []byte) []string {
	t.Helper()
	files, names, err := readZipEntries(data)
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	_ = files
	return names
}
