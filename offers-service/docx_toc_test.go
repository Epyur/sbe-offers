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

// templateBookmarks — закладки, которыми МАКЕТ связал свои заголовки с
// СВОИМ оглавлением.
//
// В temple.docx это закладки заголовков `КОММЕРЧЕСКОЕ ПРЕДЛОЖЕНИЕ` и `АЛЬБОМ
// ТИПОВЫХ РЕШЕНИЙ` — не кэш пунктов, а сами заголовки. Since the master is
// the document's own layout, its heading paragraphs remain, and their bookmarks
// with them: the point of §12 is that the CACHED TOC ENTRIES never describe the
// template any more, not that the master's headings disappear.
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

// TestSectionTocHasNoTemplateEntries — ни одного пункта шаблона в самом
// ОГЛАВЛЕНИИ не остаётся.
//
// Проверяется sdt оглавления, а НЕ весь документ: закладки `_Toc245563567/568`
// принадлежат заголовкам самого макета (`КОММЕРЧЕСКОЕ ПРЕДЛОЖЕНИЕ`,
// `АЛЬБОМ ТИПОВЫХ РЕШЕНИЙ`) — они часть документа и остаются законно.
// Проверка по всему телу краснела бы на правильном документе: конструкция
// «поиск по всему документу» не является проверкой (правило AGENTS.md
// «A check that searches the WHOLE document … is not a check»).
func TestSectionTocHasNoTemplateEntries(t *testing.T) {
	rendered := renderSectionIntoMaster(t, tocContext(tocSection(), map[string]string{
		"intro": sectionParagraphAnswer("Вступление по делу."),
	}))
	toc := tocSDT(rendered.body)
	if toc == "" {
		t.Fatalf("оглавление не найдено в документе:\n%s", rendered.body)
	}
	for _, stale := range templateBookmarks {
		if strings.Contains(toc, stale) {
			t.Fatalf("в оглавлении остался пункт шаблона %q:\n%s", stale, toc)
		}
	}
}

// TestSectionTocAnchorsResolve — у каждой ссылки `<w:hyperlink w:anchor>` в
// оглавлении есть закладка в теле (класс ошибки «r:id без отношения»).
func TestSectionTocAnchorsResolve(t *testing.T) {
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
//
// Метки в temple.docx стоят как `{{block:…}}` — разбитые Word на прогоны.
// Печать раздела требует `{ content: … }`, поэтому тест ПОДМЕНЯЕТ такую метку:
// иначе раздел просто не печатается, и оглавление винит не тот код
// (ревизия 2026-11-14, START-HERE — «доходит ли метка вообще»).
func renderSectionIntoMaster(t *testing.T, lc layoutContext) renderedDocx {
	t.Helper()
	layout := withSectionMarker(t, referenceTemple(t), "proposal")
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

// withSectionMarker заменяет первую метку `{{block:…}}` на `{ content: id }`.
//
// Замена идёт по СКЛЕЙКЕ абзаца, а не по байтам: Word режет метку на несколько
// `<w:t>` (`{{`, `block:`, `marteh`, `}}`), и побайтовая замена не находит её
// вовсе (проверено на живом temple.docx).
func withSectionMarker(t *testing.T, layout []byte, id string) []byte {
	t.Helper()
	files, names, err := readZipEntries(layout)
	if err != nil {
		t.Fatalf("разбор макета: %v", err)
	}
	doc, ok := files["word/document.xml"]
	if !ok {
		t.Fatalf("в макете нет word/document.xml")
	}
	marker := `<w:p><w:r><w:t>{ content: ` + id + ` }</w:t></w:r></w:p>`
	rewritten, replaced := replaceBlockMarkerParagraph(string(doc), marker)
	if !replaced {
		// Метки `{{block:…}}` в макете нет (другой макет) — используется как есть.
		return layout
	}
	files["word/document.xml"] = []byte(rewritten)
	out, err := zipFiles(files, names)
	if err != nil {
		t.Fatalf("сборка макета: %v", err)
	}
	return out
}

// replaceBlockMarkerParagraph ставит marker вместо первого абзаца с
// `{{block:…}}`. Ищет по склейке текста абзаца — границы прогонов не важны.
func replaceBlockMarkerParagraph(doc, marker string) (string, bool) {
	paragraphs := reWordParagraph.FindAllString(doc, -1)
	for _, paragraph := range paragraphs {
		if !strings.Contains(strings.Join(paragraphTexts(paragraph), ""), "{{block:") {
			continue
		}
		return strings.Replace(doc, paragraph, marker, 1), true
	}
	return doc, false
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
