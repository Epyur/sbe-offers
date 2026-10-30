package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strconv"
)

// Вставка изображений в DOCX (2026-10-29).
//
// Грабля, уже пройденная в ЛИМС: у бланка СВОИ визуальные элементы, и каждый
// занимает свой wp:docPr id. В нашем шаблоне заняты:
//
//	word/header1.xml  — 1, 6
//	word/footer1.xml  — 2, 3, 5, 10, 25
//	word/footer2.xml  — 4, 13, 15, 16
//
// ЛИМС спасалась константой `docxDrawingIDBase = 1000` — она не проверяет
// реальные id и сломается на любом другом шаблоне (например, если в колонтитул
// добавят фигуру с id 1001). Word на дубле id молча ломает вёрстку или не
// показывает картинку, и это ищут глазами.
//
// Поэтому здесь id НЕ константа: перед сборкой читаем WP:docPr из всех XML
// пакета и выдаём номера начиная с максимального занятого + 1.

// reDocPrID ловит id визуальных элементов: wp:docPr (картинки, фигуры,
// текстовые блоки) и pic:cNvPr — они должны быть уникальны в пакете.
var reDocPrID = regexp.MustCompile(`<wp:docPr[^>]*\bid="(\d+)"`)

// usedDrawingIDs читает все части пакета и возвращает занятые id.
//
// Читаются именно те части, где Word хранит визуальные элементы: текст
// документа, колонтитулы и сноски. Остальные (.rels, styles) id не содержат.
func usedDrawingIDs(files map[string][]byte) map[int]bool {
	used := make(map[int]bool)
	for name, data := range files {
		if !isVisualPart(name) {
			continue
		}
		for _, m := range reDocPrID.FindAllSubmatch(data, -1) {
			if n, err := strconv.Atoi(string(m[1])); err == nil {
				used[n] = true
			}
		}
	}
	return used
}

// isVisualPart — части пакета, где бывают визуальные элементы.
func isVisualPart(name string) bool {
	switch {
	case name == "word/document.xml":
		return true
	case len(name) > 5 && name[:5] == "word/" && hasAnySuffix(name, "footnotes.xml", "endnotes.xml"):
		return true
	case len(name) > 5 && name[:5] == "word/" && hasAnySuffix(name, ".xml"):
		base := name[len("word/"):]
		return hasPrefixAny(base, "header", "footer")
	}
	return false
}

func hasAnySuffix(s string, suffixes ...string) bool {
	for _, suf := range suffixes {
		if len(s) >= len(suf) && s[len(s)-len(suf):] == suf {
			return true
		}
	}
	return false
}

func hasPrefixAny(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if len(s) >= len(p) && s[:len(p)] == p {
			return true
		}
	}
	return false
}

// nextDrawingIDs выдаёт последовательные свободные id, начиная с первого
// свободного после максимального занятого.
//
// Именно «после максимального», а не «первое свободное от 1»: Word не любит,
// когда новый элемент получает id меньше уже существующих, и в некоторых
// случаях перерисовывает документ целиком.
func nextDrawingIDs(used map[int]bool, count int) []int {
	max := 0
	for id := range used {
		if id > max {
			max = id
		}
	}
	out := make([]int, count)
	for i := 0; i < count; i++ {
		max++
		for used[max] {
			max++
		}
		out[i] = max
		used[max] = true
	}
	return out
}

// docxImagePart — изображение, добавленное в пакет.
type docxImagePart struct {
	relID       string
	mediaName   string
	contentType string
	data        []byte
	docPrID     int
}

// docxMedia собирает изображения и свободные id на время сборки одного файла.
type docxMedia struct {
	used    map[int]bool
	images  []docxImagePart
	nextRel int
}

// newDocxMedia создаёт сборщик по текущему набору частей шаблона.
func newDocxMedia(files map[string][]byte) *docxMedia {
	return &docxMedia{used: usedDrawingIDs(files)}
}

// add регистрирует изображение и возвращает <w:drawing> для вставки в run.
// Пустой data — пустая строка: картинка необязательна, документ не должен
// падать из-за отсутствующего файла.
func (m *docxMedia) add(data []byte, ext, contentType string, cx, cy int64) string {
	if len(data) == 0 {
		return ""
	}
	m.nextRel++
	ids := nextDrawingIDs(m.used, 1)
	img := docxImagePart{
		relID:       fmt.Sprintf("rIdPhoto%d", m.nextRel),
		mediaName:   fmt.Sprintf("offerimg%d.%s", m.nextRel, ext),
		contentType: contentType,
		data:        data,
		docPrID:     ids[0],
	}
	m.images = append(m.images, img)
	return docxDrawingXML(img.relID, img.docPrID, cx, cy)
}

// docxDrawingXML — блок картинки. Имена media-файлов начинаются с "offerimg",
// а не "image": в бланке уже заняты image1.png/image2.png (логотип и знак
// подписи), и совпадение имён подменило бы их (та же грабля была в ЛИМС).
func docxDrawingXML(relID string, picID int, cx, cy int64) string {
	return fmt.Sprintf(
		`<w:r><w:drawing>`+
			`<wp:inline distT="0" distB="0" distL="0" distR="0" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing">`+
			`<wp:extent cx="%d" cy="%d"/>`+
			`<wp:docPr id="%d" name="OfferImage%d"/>`+
			`<a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">`+
			`<a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">`+
			`<pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">`+
			`<pic:nvPicPr><pic:cNvPr id="%d" name="OfferImage%d"/><pic:cNvPicPr/></pic:nvPicPr>`+
			`<pic:blipFill><a:blip xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" r:embed="%s"/>`+
			`<a:stretch><a:fillRect/></a:stretch></pic:blipFill>`+
			`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm>`+
			`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr>`+
			`</pic:pic></a:graphicData></a:graphic>`+
			`</wp:inline></w:drawing></w:r>`,
		cx, cy, picID, picID, picID, picID, relID, cx, cy)
}

// applyMedia дописывает изображения в пакет: файлы word/media/*, связи в
// word/_rels/document.xml.rels и расширения в [Content_Types].xml.
//
// Расширения добавляются только те, которых нет в шаблоне: повторный Default
// для png делает файл невалидным.
func applyMedia(files map[string][]byte, m *docxMedia) error {
	if len(m.images) == 0 {
		return nil
	}
	var rels bytes.Buffer
	for _, img := range m.images {
		files["word/media/"+img.mediaName] = img.data
		fmt.Fprintf(&rels, `<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/%s"/>`,
			img.relID, img.mediaName)
	}
	relsXML, ok := files["word/_rels/document.xml.rels"]
	if !ok {
		return fmt.Errorf("в шаблоне нет word/_rels/document.xml.rels")
	}
	files["word/_rels/document.xml.rels"] = bytes.Replace(relsXML,
		[]byte("</Relationships>"),
		append(rels.Bytes(), []byte("</Relationships>")...), 1)

	ct, ok := files["[Content_Types].xml"]
	if !ok {
		return fmt.Errorf("в шаблоне нет [Content_Types].xml")
	}
	declared := declaredExtensions(string(ct))
	var extra bytes.Buffer
	for _, img := range m.images {
		ext := extOf(img.mediaName)
		if declared[ext] {
			continue
		}
		declared[ext] = true
		fmt.Fprintf(&extra, `<Default Extension="%s" ContentType="%s"/>`, ext, img.contentType)
	}
	if extra.Len() > 0 {
		files["[Content_Types].xml"] = bytes.Replace(ct,
			[]byte("</Types>"), append(extra.Bytes(), []byte("</Types>")...), 1)
	}
	return nil
}

var reDefaultExt = regexp.MustCompile(`<Default[^>]*Extension="([^"]+)"`)

// declaredExtensions — расширения, уже объявленные в [Content_Types].xml.
func declaredExtensions(ct string) map[string]bool {
	out := map[string]bool{}
	for _, m := range reDefaultExt.FindAllStringSubmatch(ct, -1) {
		out[m[1]] = true
	}
	return out
}

// extOf — расширение из имени файла.
func extOf(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			return name[i+1:]
		}
	}
	return ""
}

// readZipEntries читает все части .docx в карту: имя → байты.
func readZipEntries(data []byte) (map[string][]byte, []string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, err
	}
	files := make(map[string][]byte, len(zr.File))
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, nil, err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, nil, err
		}
		files[f.Name] = b
		names = append(names, f.Name)
	}
	return files, names, nil
}
