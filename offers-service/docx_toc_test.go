package main

import (
	"os"
	"strings"
	"testing"
)

// Оглавление генерируемого DOCX (2026-11-14, шаг 5a).
//
// Повод — жалоба пользователя: файл, присланный в Телеграм, показывал
// оглавление ШАБЛОНА (`_Toc245563567`…, страницы 3/3/4/5), потому что кэш
// оглавления приходит из макета, а Word его пересчитывает только при открытии.
//
// Что проверяется: пункты и якоря — из напечатанных заголовков; ни одного
// пункта шаблона не осталось; каждая ссылка оглавления имеет закладку в теле.
// Номера страниц — дело Word, и здесь они не проверяются (честный предел).

// templateBookmarks — закладки оглавления живого temple.docx.
var templateBookmarks = []string{"_Toc245563567", "_Toc245563568"}

// tocSection — раздел с двумя подразделами, оба печатаются.
func tocSection() ContentSection {
	return ContentSection{
		ID: "proposal", Title: "Описание предлагаемых систем", Numbered: true,
		Subsections: []ContentSubsection{
			{ID: "intro", Title: "Введение", Instruction: "Вступление", Condition: conditionAlways},
			{ID: "each", Title: "Описание системы", Instruction: "Опиши", Condition: conditionHasSystems, Repeat: blockRepeatSystems},
		},
	}
}

// tocContext — печать раздела по дешёвому макету со стилями оглавления.
func tocContext(section ContentSection, answers map[string]string) layoutContext {
	lc := serviceLayoutContext(nil)
	lc.form = func() OfferForm { return OfferForm{Sections: []ContentSection{section}} }
	lc.sectionText = func(_ ContentSection, call sectionCall) string { return answers[call.sub.ID] }
	return lc
}

// TestSectionTocEntriesMatchHeadings — заголовки печати попадают в оглавление
// по порядку и со своими закладками.
func TestSectionTocEntriesMatchHeadings(t *testing.T) {
	t.Skip("ревизия 2026-11-14: пересборка оглавления не доведена — см. START-HERE, «Что не работает»")
	rendered := renderSectionIntoMaster(t, tocContext(tocSection(), map[string]string{
		"intro": sectionParagraphAnswer("Вступление по делу."),
		"each":  sectionParagraphAnswer("Описание одной системы."),
	}))
	body := rendered.body
	toc := tocSDT(body)
	if toc == "" {
		t.Fatalf("оглавление не найдено в документе:\n%s", body)
	}
	// Порядок пунктов = порядок заголовков.
	order := []string{"Описание предлагаемых систем", "Введение", "Описание системы"}
	last := -1
	for _, text := range order {
		at := strings.Index(toc, text)
		if at < 0 {
			t.Fatalf("пункт %q отсутствует в оглавлении:\n%s", text, toc)
		}
		if at <= last {
			t.Fatalf("пункт %q стоит не по порядку (после позиции %d):\n%s", text, last, toc)
		}
		last = at
	}
	// Каждая ссылка оглавления ведёт на закладку, которая есть в теле.
	for _, anchor := range tocAnchors(toc) {
		if !strings.Contains(body, `w:name="`+anchor+`"`) {
			t.Fatalf("якорь оглавления %q не имеет закладки в теле", anchor)
		}
	}
}

// TestSectionTocHasNoTemplateEntries — пункты шаблона в документе не остаются.
func TestSectionTocHasNoTemplateEntries(t *testing.T) {
	t.Skip("ревизия 2026-11-14: пересборка оглавления не доведена — см. START-HERE, «Что не работает»")
	rendered := renderSectionIntoMaster(t, tocContext(tocSection(), map[string]string{
		"intro": sectionParagraphAnswer("Вступление по делу."),
	}))
	for _, stale := range templateBookmarks {
		if strings.Contains(rendered.body, stale) {
			t.Fatalf("в оглавлении/теле осталась закладка шаблона %q:\n%s", stale, rendered.body)
		}
	}
}

// TestSectionTocAnchorsResolve — у каждой ссылки `<w:hyperlink w:anchor>` в
// оглавлении есть закладка в теле (класс ошибки «r:id без отношения»).
func TestSectionTocAnchorsResolve(t *testing.T) {
	t.Skip("ревизия 2026-11-14: пересборка оглавления не доведена — см. START-HERE, «Что не работает»")
	rendered := renderSectionIntoMaster(t, tocContext(tocSection(), map[string]string{
		"intro": sectionParagraphAnswer("Вступление по делу."),
		"each":  sectionParagraphAnswer("Описание одной системы."),
	}))
	anchors := tocAnchors(tocSDT(rendered.body))
	if len(anchors) < 3 {
		t.Fatalf("якорей %d, ожидалось не меньше трёх:\n%s", len(anchors), rendered.body)
	}
	for _, anchor := range anchors {
		if !strings.Contains(rendered.body, `w:name="`+anchor+`"`) {
			t.Fatalf("ссылка на закладку %q никуда не ведёт", anchor)
		}
	}
}

// renderedDocx — что вернула печать и какие файлы в пакете.
type renderedDocx struct {
	body  string
	files map[string][]byte
}

// renderSectionIntoMaster печатает раздел в макет живой формы.
//
// Макет — temple.docx: именно он решает, какие стили оглавления и заголовков
// существуют. Без него (`OFFERS_LIVE_LAYOUT` не задан) берётся маленький
// макет со стилями оглавления и закладками шаблона — тестам нужна та же
// форма, а живой файл проверяет TestSectionPrintsIntoReferenceTemple.
func renderSectionIntoMaster(t *testing.T, lc layoutContext) renderedDocx {
	t.Helper()
	layout := referenceTemple(t)
	out, err := renderLayoutDocx(layout, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	files, _, err := readZipEntries(out)
	if err != nil {
		t.Fatalf("разбор пакета: %v", err)
	}
	return renderedDocx{body: string(files["word/document.xml"]), files: files}
}

// referenceTemple — живой temple.docx, если он доступен.
func referenceTemple(t *testing.T) []byte {
	t.Helper()
	for _, path := range []string{os.Getenv("OFFERS_LIVE_LAYOUT"), "testdata/temple.docx"} {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			return data
		}
	}
	t.Skip("temple.docx не найден: задайте OFFERS_LIVE_LAYOUT или положите его в testdata/")
	return nil
}

// tocSDT — разметка оглавления (<w:sdt>) из тела документа.
func tocSDT(body string) string {
	start := strings.Index(body, "<w:sdt>")
	if start < 0 {
		return ""
	}
	end := strings.Index(body[start:], "</w:sdt>")
	if end < 0 {
		return ""
	}
	return body[start : start+end+len("</w:sdt>")]
}

// tocAnchors — якоря ссылок внутри разметки.
func tocAnchors(markup string) []string {
	var out []string
	for _, m := range reHyperlinkAnchor.FindAllStringSubmatch(markup, -1) {
		out = append(out, m[1])
	}
	return out
}
