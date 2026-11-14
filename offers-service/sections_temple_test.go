package main

import (
	"strings"
	"testing"
)

// Шаг 5b (2026-11-14): содержательный раздел печатается в ЖИВОЙ образец —
// `testdata/temple.docx`, копию `C:\Obsidian\mailers\temple.docx`.
//
// Образец — не украшение: он решает, какие стили заголовков и оглавления
// существуют, где кончается портретная часть и начинается альбомная. Тест
// держит три обещания дизайна §11:
//
//  1. раздел ложится в нужное место — вслед за меткой `{ content: … }`,
//     которой в маке заменён `{{block:commercial_propose}}`;
//  2. альбомный «АЛЬБОМ ТИПОВЫХ РЕШЕНИЙ» не тронут: его `sectPr` остаётся
//     последним секционным свойством и по-прежнему альбомный;
//  3. оглавление подхватывает новые заголовки: у каждого пункта свежей сборки
//     есть якорь в теле, а старых пунктов образца в оглавлении нет.

// TestSectionPrintsIntoReferenceTemple — печать раздела в живой образец.
func TestSectionPrintsIntoReferenceTemple(t *testing.T) {
	rendered := renderSectionIntoReference(t)

	// (1) Раздел напечатан под своим заголовком.
	for _, want := range []string{"Описание предлагаемых систем", "Введение", "Ключевая мысль."} {
		if !strings.Contains(rendered.body, want) {
			t.Fatalf("в документе нет %q:\n%s", want, rendered.body)
		}
	}

	// (2) Альбомная часть — последний sectPr, и она альбомная.
	if !strings.Contains(rendered.body, `<w:pgSz w:w="16838" w:h="11906" w:orient="landscape"/>`) {
		t.Fatalf("альбомный sectPr потерян:\n%s", rendered.body)
	}
	if strings.Count(rendered.body, "<w:sectPr") != 2 {
		// Два sectPr — портретный и альбомный. Больше означало бы, что раздел
		// принёс свой.
		t.Fatalf("sectPr %d, ожидалось два (портрет + альбом):\n%s",
			strings.Count(rendered.body, "<w:sectPr"), rendered.body)
	}

	// (3) Оглавление — из собственных заголовков документа.
	toc := tocSDT(rendered.body)
	if toc == "" {
		t.Fatalf("оглавление не найдено:\n%s", rendered.body)
	}
	if !strings.Contains(toc, "Описание предлагаемых систем") {
		t.Fatalf("оглавление не подхватило заголовок раздела:\n%s", toc)
	}
	for _, anchor := range tocAnchors(toc) {
		if !strings.Contains(rendered.body, `w:name="`+anchor+`"`) {
			t.Fatalf("якорь оглавления %q не имеет закладки в теле", anchor)
		}
	}
}

// renderSectionIntoReference печатает раздел в живой temple.docx.
//
// Метка `{{block:commercial_propose}}` (абзац 22) заменяется на
// `{ content: proposal }`: именно там администратор поставит её в новом
// макете (дизайн §11). Абзац ищется по склейке текста, потому что Word режет
// метку на несколько `<w:t>`.
func renderSectionIntoReference(t *testing.T) renderedDocx {
	t.Helper()
	layout := withNamedSectionMarker(t, referenceTemple(t), "commercial_propose", "proposal")
	out, err := renderLayoutDocx(layout, tocContext(tocSection(), map[string]string{
		"intro": sectionParagraphAnswer("Ключевая мысль."),
	}))
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	files, _, err := readZipEntries(out)
	if err != nil {
		t.Fatalf("разбор пакета: %v", err)
	}
	return renderedDocx{body: string(files["word/document.xml"]), files: files}
}

// withNamedSectionMarker заменяет абзац с `{{block:<blockID>}}` на
// `{ content: <id> }`, ища метку по склейке текста абзаца.
func withNamedSectionMarker(t *testing.T, layout []byte, blockID, id string) []byte {
	t.Helper()
	files, names, err := readZipEntries(layout)
	if err != nil {
		t.Fatalf("разбор макета: %v", err)
	}
	doc, ok := files["word/document.xml"]
	if !ok {
		t.Fatalf("в макете нет word/document.xml")
	}
	needle := "{{block:" + blockID + "}}"
	marker := `<w:p><w:r><w:t>{ content: ` + id + ` }</w:t></w:r></w:p>`
	replaced := false
	for _, paragraph := range reWordParagraph.FindAllString(string(doc), -1) {
		if !strings.Contains(strings.Join(paragraphTexts(paragraph), ""), needle) {
			continue
		}
		files["word/document.xml"] = []byte(strings.Replace(string(doc), paragraph, marker, 1))
		replaced = true
		break
	}
	if !replaced {
		t.Fatalf("в макете нет абзаца с %q", needle)
	}
	out, err := zipFiles(files, names)
	if err != nil {
		t.Fatalf("сборка макета: %v", err)
	}
	return out
}
