package main

import (
	"archive/zip"
	"bytes"
	"io"
	"regexp"
	"strings"
	"testing"
)

// Приёмка бланка (2026-10-29, живая жалоба).
//
// Симптом: в собранном файле пропадала нумерация страниц в нижнем колонтитуле.
// Причина не в шаблоне — он побайтово тот же, что в ЛИМС. Причина в том, что
// offers ПОДМЕНЯЛ word/styles.xml и word/numbering.xml своими короткими
// версиями (5 стилей вместо 58, 2 нумерации вместо 12). Поля PAGE и
// SECTIONPAGES в колонтитуле живут в текстовых блоках и зависят от стилей
// бланка: без них Word пересчитывает разметку и вытесняет нумерацию.

// docxEntry читает одну часть готового .docx.
func docxEntry(t *testing.T, docx []byte, name string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		t.Fatalf("открыть архив: %v", err)
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("открыть %s: %v", name, err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("прочитать %s: %v", name, err)
		}
		return data
	}
	t.Fatalf("в документе нет части %s", name)
	return nil
}

// templateEntry читает часть шаблона бланка.
func templateEntry(t *testing.T, name string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(standardTemplateDocx), int64(len(standardTemplateDocx)))
	if err != nil {
		t.Fatalf("открыть шаблон: %v", err)
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("открыть %s шаблона: %v", name, err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("прочитать %s шаблона: %v", name, err)
		}
		return data
	}
	t.Fatalf("в шаблоне нет части %s", name)
	return nil
}

func sampleOffer() Offer {
	return Offer{
		ID:     1,
		Number: "019/2026",
		Client: OfferClient{Name: "ООО Строй", Contact: "Иван"},
		Items: []OfferItem{
			{Name: "Мембрана V-SL 2 мм", Unit: "м2", Qty: 100, Price: 500, Sum: 50000},
		},
		Total: 50000,
	}
}

// Стили бланка не подменяются: 58 стилей шаблона должны доехать до файла.
func TestDocxKeepsTemplateStyles(t *testing.T) {
	docx, err := renderOfferDocx(OfferForm{Title: "Коммерческое предложение"}, sampleOffer())
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	got := docxEntry(t, docx, "word/styles.xml")
	want := templateEntry(t, "word/styles.xml")
	if !bytes.Equal(got, want) {
		t.Fatalf("styles.xml подменён: %d байт вместо %d из шаблона", len(got), len(want))
	}
}

// Нумерация бланка не подменяется: колонтитулы и таблицы на неё ссылаются.
func TestDocxKeepsTemplateNumbering(t *testing.T) {
	docx, err := renderOfferDocx(OfferForm{Title: "Коммерческое предложение"}, sampleOffer())
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	got := docxEntry(t, docx, "word/numbering.xml")
	want := templateEntry(t, "word/numbering.xml")
	if !bytes.Equal(got, want) {
		t.Fatalf("numbering.xml подменён: %d байт вместо %d из шаблона", len(got), len(want))
	}
}

// Поля нумерации страниц в колонтитулах доезжают до файла нетронутыми.
func TestDocxKeepsFooterPageFields(t *testing.T) {
	docx, err := renderOfferDocx(OfferForm{Title: "Коммерческое предложение"}, sampleOffer())
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	for _, part := range []string{"word/footer1.xml", "word/footer2.xml"} {
		got := docxEntry(t, docx, part)
		for _, field := range []string{"PAGE", "SECTIONPAGES"} {
			if !strings.Contains(string(got), field) {
				t.Errorf("в %s нет поля %s — нумерация страниц пропадёт", part, field)
			}
		}
		// И побайтово тоже: любая правка колонтитула ломает положения текстовых блоков.
		if want := templateEntry(t, part); !bytes.Equal(got, want) {
			t.Errorf("%s изменён, а не должен", part)
		}
	}
}

var rePStyle = regexp.MustCompile(`<w:pStyle w:val="([^"]+)"`)
var reStyleID = regexp.MustCompile(`w:styleId="([^"]+)"`)

// В document.xml не должно быть ссылок на стили, которых нет в styles.xml:
// иначе Word показывает текст дефолтным шрифтом и разъезжается вёрстка.
func TestDocxOnlyKnownStyles(t *testing.T) {
	docx, err := renderOfferDocx(OfferForm{Title: "Коммерческое предложение"}, sampleOffer())
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	known := map[string]bool{}
	for _, m := range reStyleID.FindAllStringSubmatch(string(docxEntry(t, docx, "word/styles.xml")), -1) {
		known[m[1]] = true
	}
	for _, m := range rePStyle.FindAllStringSubmatch(string(docxEntry(t, docx, "word/document.xml")), -1) {
		if !known[m[1]] {
			t.Errorf("document.xml ссылается на стиль %q, которого нет в styles.xml", m[1])
		}
	}
	if len(known) < 30 {
		t.Fatalf("в styles.xml подозрительно мало стилей: %d", len(known))
	}
}

var reNumIDUse = regexp.MustCompile(`<w:numId w:val="(\d+)"/>`)
var reNumDef = regexp.MustCompile(`<w:num w:numId="(\d+)"`)

// Списки ссылаются только на нумерации, которые есть в numbering.xml бланка.
func TestDocxOnlyKnownNumIds(t *testing.T) {
	docx, err := renderOfferDocx(OfferForm{Title: "Коммерческое предложение"}, sampleOffer())
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	known := map[string]bool{}
	for _, m := range reNumDef.FindAllStringSubmatch(string(docxEntry(t, docx, "word/numbering.xml")), -1) {
		known[m[1]] = true
	}
	for _, m := range reNumIDUse.FindAllStringSubmatch(string(docxEntry(t, docx, "word/document.xml")), -1) {
		if !known[m[1]] {
			t.Errorf("document.xml ссылается на numId=%s, которого нет в numbering.xml", m[1])
		}
	}
}

// Бриф и текст секций из формы тоже доезжают до документа.
func TestDocxKeepsBody(t *testing.T) {
	docx, err := renderOfferDocx(OfferForm{Title: "Коммерческое предложение"}, sampleOffer())
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	body := string(docxEntry(t, docx, "word/document.xml"))
	for _, want := range []string{"019/2026", "ООО Строй", "Мембрана V-SL 2 мм", "50 000,00"} {
		if !strings.Contains(body, want) {
			t.Errorf("в документе нет %q", want)
		}
	}
}

// rebuildZip собирает .docx обратно из карты частей в исходном порядке.
func rebuildZip(t *testing.T, files map[string][]byte, names []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("создать %s: %v", name, err)
		}
		if _, err := w.Write(files[name]); err != nil {
			t.Fatalf("записать %s: %v", name, err)
		}
	}
	// Новые части (media) в names нет — дописываем отдельно.
	for name, data := range files {
		found := false
		for _, n := range names {
			if n == name {
				found = true
				break
			}
		}
		if found {
			continue
		}
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("создать %s: %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("записать %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("закрыть архив: %v", err)
	}
	return buf.Bytes()
}
