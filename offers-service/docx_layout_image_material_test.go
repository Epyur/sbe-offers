package main

import (
	"strings"
	"testing"
)

// Метка картинки материала (2026-11-06).
//
// Картинка материала живёт не в карточке, а в позиции предложения (`PhotoKey`),
// поэтому её путь отдельный от системного и проверяется отдельно: без теста
// ветка не покрыта вовсе, и «картинка материала не вставляется» никто не
// заметит до жалобы.

// layoutMaterialDoc — макет с меткой картинки материала.
func layoutMaterialDoc(t *testing.T) []byte {
	t.Helper()
	return layoutDocxFromXML(t,
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:r><w:t>{{картинка: материал}}</w:t></w:r></w:p>
</w:body></w:document>`)
}

// TestLayoutImageOfMaterialPrintsItemPicture — картинка берётся из позиции.
func TestLayoutImageOfMaterialPrintsItemPicture(t *testing.T) {
	offer := Offer{Items: []OfferItem{
		{Name: "Мембрана V-SL", PhotoKey: "kb/mat.png", ImageName: "mat.png"},
		{Name: "Вторая позиция"},
	}}
	data, err := renderLayoutDocx(layoutMaterialDoc(t), layoutContext{
		values: map[string]string{},
		offer:  offer,
		images: &fakeImages{data: pngBytes()},
	})
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	body := string(docxEntry(t, data, "word/document.xml"))
	if !strings.Contains(body, "<w:drawing") {
		t.Fatal("картинка материала не вставлена")
	}
	if !hasMediaPart(t, data) {
		t.Fatal("файл картинки материала не в пакете")
	}
	if strings.Contains(body, "картинка: материал") {
		t.Fatal("метка напечатана текстом")
	}
}

// TestLayoutImageOfMaterialByNumber — «материал 2» берёт вторую позицию.
func TestLayoutImageOfMaterialByNumber(t *testing.T) {
	offer := Offer{Items: []OfferItem{
		{Name: "Первая"},
		{Name: "Вторая", PhotoKey: "kb/two.png", ImageName: "two.png"},
	}}
	data, err := renderLayoutDocx(layoutDocxFromXML(t,
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:r><w:t>{{картинка: материал 2}}</w:t></w:r></w:p>
</w:body></w:document>`), layoutContext{
		values: map[string]string{},
		offer:  offer,
		images: &fakeImages{data: pngBytes()},
	})
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	if !hasMediaPart(t, data) {
		t.Fatal("вторая позиция не дала картинку")
	}
}

// TestLayoutImageOfMaterialWithoutPositionsIsSilent — позиций нет: метка молчит.
func TestLayoutImageOfMaterialWithoutPositionsIsSilent(t *testing.T) {
	data, err := renderLayoutDocx(layoutMaterialDoc(t), layoutContext{
		values: map[string]string{},
		offer:  Offer{},
		images: &fakeImages{data: pngBytes()},
	})
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	body := string(docxEntry(t, data, "word/document.xml"))
	if strings.Contains(body, "<w:drawing") {
		t.Fatal("позиций нет, а картинка вставлена")
	}
	if strings.Contains(body, "картинка") {
		t.Fatal("метка напечатана заказчику")
	}
}

// TestLayoutImageOfMaterialWithoutPhotoKeyIsSilent — у позиции нет картинки:
// метка молчит, а не подставляет картинку соседней позиции.
func TestLayoutImageOfMaterialWithoutPhotoKeyIsSilent(t *testing.T) {
	data, err := renderLayoutDocx(layoutMaterialDoc(t), layoutContext{
		values: map[string]string{},
		offer:  Offer{Items: []OfferItem{{Name: "Без картинки"}}},
		images: &fakeImages{data: pngBytes()},
	})
	if err != nil {
		t.Fatalf("сборка упала: %v", err)
	}
	body := string(docxEntry(t, data, "word/document.xml"))
	if strings.Contains(body, "<w:drawing") {
		t.Fatal("картинка вставлена у позиции без ключа")
	}
}

// TestParseLayoutImageSpec — какие фразы человека понимаются как метка.
func TestParseLayoutImageSpec(t *testing.T) {
	cases := []struct {
		in   string
		of   string
		idx  int
		want bool
	}{
		{"картинка", imageOfSystem, 0, true},
		{"картинка: система", imageOfSystem, 0, true},
		{"картинка: система 2", imageOfSystem, 2, true},
		{"картинка: материал", imageOfMaterial, 0, true},
		{"картинка: материал 3", imageOfMaterial, 3, true},
		{"изображение системы", imageOfSystem, 0, true},
		{"рисунок 2", imageOfSystem, 2, true},
		{"image: material 2", imageOfMaterial, 2, true},
		{"опиши систему", "", 0, false},
		{"текст", "", 0, false},
		{"картинки нет", "", 0, false},
		{"не вставляй изображение", "", 0, false},
		{"картинный ряд", "", 0, false},
	}
	for _, c := range cases {
		spec, ok := parseLayoutImageSpec(c.in)
		if ok != c.want {
			t.Errorf("%q: распознано=%v, ожидали %v", c.in, ok, c.want)
			continue
		}
		if !ok {
			continue
		}
		if spec.of != c.of || spec.index != c.idx {
			t.Errorf("%q: получили of=%q idx=%d, ожидали of=%q idx=%d",
				c.in, spec.of, spec.index, c.of, c.idx)
		}
	}
}
