package main

import (
	"bytes"
	"regexp"
	"strconv"
	"testing"
)

// id изображений не должны пересекаться с фигурами бланка (2026-10-29).
// Грабля уже была в ЛИМС: там спасались константой 1000, но она не проверяет
// реальные id. Здесь id вычисляются по ключам шаблона.

func TestUsedDrawingIDsFindsHeaderFooter(t *testing.T) {
	files, _, err := readZipEntries(standardTemplateDocx)
	if err != nil {
		t.Fatalf("разбор шаблона: %v", err)
	}
	used := usedDrawingIDs(files)
	// Это фактический набор бланка: header1 (1,6), footer1 (2,3,5,10,25),
	// footer2 (4,13,15,16).
	for _, id := range []int{1, 2, 3, 4, 5, 6, 10, 13, 15, 16, 25} {
		if !used[id] {
			t.Errorf("id %d из бланка не найден", id)
		}
	}
}

func TestNextDrawingIDsStartsAfterMax(t *testing.T) {
	used := map[int]bool{1: true, 5: true, 25: true}
	got := nextDrawingIDs(used, 3)
	want := []int{26, 27, 28}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ожидали %v, получили %v", want, got)
		}
	}
}

// Дырки в занятых id не используются: Word не любит элементы с id меньше уже
// существующих.
func TestNextDrawingIDsSkipsGaps(t *testing.T) {
	used := map[int]bool{1: true, 2: true, 3: true, 10: true}
	got := nextDrawingIDs(used, 2)
	if got[0] != 11 || got[1] != 12 {
		t.Fatalf("ожидали [11 12], получили %v", got)
	}
}

// Повторный вызов не выдаёт тот же id: сборщик помечает выданные.
func TestNextDrawingIDsNoRepeat(t *testing.T) {
	used := map[int]bool{7: true}
	a := nextDrawingIDs(used, 1)[0]
	b := nextDrawingIDs(used, 1)[0]
	if a == b {
		t.Fatalf("выдан один id дважды: %d", a)
	}
}

var reDocPrAny = regexp.MustCompile(`<wp:docPr[^>]*\bid="(\d+)"`)

// Главная проверка: после сборки документа с картинкой ни один docPr id в
// document.xml не совпадает с id колонтитулов, и внутри документа нет дублей.
func TestDocxImageIDsDoNotClash(t *testing.T) {
	files, names, err := readZipEntries(standardTemplateDocx)
	if err != nil {
		t.Fatalf("разбор шаблона: %v", err)
	}
	media := newDocxMedia(files)
	// Два изображения — как система с фото и, например, схема.
	png := []byte("\x89PNG\r\n\x1a\n fake png for id check")
	d1 := media.add(png, "png", "image/png", 100, 100)
	d2 := media.add(png, "png", "image/png", 100, 100)
	if d1 == "" || d2 == "" {
		t.Fatal("изображения не зарегистрированы")
	}
	if err := applyMedia(files, media); err != nil {
		t.Fatalf("applyMedia: %v", err)
	}
	files["word/document.xml"] = append(files["word/document.xml"],
		[]byte(d1+d2)...)

	// Собираем архив обратно.
	docx := rebuildZip(t, files, names)

	// Занятые id в колонтитулах и шапке.
	reserved := map[int]bool{}
	for name, data := range files {
		if name == "word/document.xml" {
			continue
		}
		for _, m := range reDocPrAny.FindAllSubmatch(data, -1) {
			if n, err := strconv.Atoi(string(m[1])); err == nil {
				reserved[n] = true
			}
		}
	}

	docXML := string(docxEntry(t, docx, "word/document.xml"))
	seen := map[int]bool{}
	for _, m := range reDocPrAny.FindAllStringSubmatch(docXML, -1) {
		id, _ := strconv.Atoi(m[1])
		if reserved[id] {
			t.Errorf("id %d совпадает с колонтитулом бланка", id)
		}
		if seen[id] {
			t.Errorf("id %d повторяется в документе", id)
		}
		seen[id] = true
	}
	if len(seen) < 2 {
		t.Fatalf("ожидали 2 картинки, нашли %d id", len(seen))
	}
}

// Имена media-файлов не должны затирать логотип и знак подписи бланка
// (image1.png, image2.png).
func TestDocxImageNamesDoNotOverwriteTemplate(t *testing.T) {
	files, _, err := readZipEntries(standardTemplateDocx)
	if err != nil {
		t.Fatalf("разбор шаблона: %v", err)
	}
	logoBefore := append([]byte(nil), files["word/media/image1.png"]...)
	media := newDocxMedia(files)
	media.add([]byte("new png"), "png", "image/png", 10, 10)
	if err := applyMedia(files, media); err != nil {
		t.Fatalf("applyMedia: %v", err)
	}
	if !bytes.Equal(files["word/media/image1.png"], logoBefore) {
		t.Fatal("логотип бланка подменён новым изображением")
	}
	if _, ok := files["word/media/offerimg1.png"]; !ok {
		t.Fatal("изображение не добавлено под своим именем")
	}
}

// Связи и Content_Types дописываются, а не перезаписываются.
func TestDocxMediaKeepsExistingRelationships(t *testing.T) {
	files, _, err := readZipEntries(standardTemplateDocx)
	if err != nil {
		t.Fatalf("разбор шаблона: %v", err)
	}
	relsBefore := string(files["word/_rels/document.xml.rels"])
	media := newDocxMedia(files)
	media.add([]byte("png"), "png", "image/png", 10, 10)
	if err := applyMedia(files, media); err != nil {
		t.Fatalf("applyMedia: %v", err)
	}
	relsAfter := string(files["word/_rels/document.xml.rels"])
	if !bytes.Contains([]byte(relsAfter), []byte(relsBefore[:len(relsBefore)-len("</Relationships>")])) {
		t.Fatal("прежние связи потеряны")
	}
	if !bytes.Contains([]byte(relsAfter), []byte("rIdPhoto1")) {
		t.Fatal("новая связь не добавлена")
	}
	if bytes.Count([]byte(relsAfter), []byte("<Relationships")) != 1 {
		t.Fatal("корневой тег Relationships задвоен")
	}
}

// Повторный Default для png делает пакет невалидным.
func TestDocxMediaNoDuplicateContentType(t *testing.T) {
	files, _, err := readZipEntries(standardTemplateDocx)
	if err != nil {
		t.Fatalf("разбор шаблона: %v", err)
	}
	media := newDocxMedia(files)
	media.add([]byte("png"), "png", "image/png", 10, 10)
	media.add([]byte("jpg"), "jpeg", "image/jpeg", 10, 10)
	if err := applyMedia(files, media); err != nil {
		t.Fatalf("applyMedia: %v", err)
	}
	ct := string(files["[Content_Types].xml"])
	if n := bytes.Count([]byte(ct), []byte(`Extension="png"`)); n != 1 {
		t.Fatalf("Default для png объявлен %d раз", n)
	}
	if !bytes.Contains([]byte(ct), []byte(`Extension="jpeg"`)) {
		t.Fatal("расширение jpeg не добавлено")
	}
}

// Пустое изображение не должно ломать сборку: картинка необязательна.
func TestDocxMediaEmptyIsNoop(t *testing.T) {
	files, _, err := readZipEntries(standardTemplateDocx)
	if err != nil {
		t.Fatalf("разбор шаблона: %v", err)
	}
	relsBefore := string(files["word/_rels/document.xml.rels"])
	media := newDocxMedia(files)
	if got := media.add(nil, "png", "image/png", 10, 10); got != "" {
		t.Fatalf("пустое изображение должно давать пустую разметку, получили %q", got)
	}
	if err := applyMedia(files, media); err != nil {
		t.Fatalf("applyMedia: %v", err)
	}
	if string(files["word/_rels/document.xml.rels"]) != relsBefore {
		t.Fatal("связи изменились без изображений")
	}
}
