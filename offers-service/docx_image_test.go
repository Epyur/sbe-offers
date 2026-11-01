package main

import (
	"strings"
	"testing"
)

// Изображение в тексте с обтеканием (2026-11-01).
//
// Требование пользователя дословно: «система должна уметь вставлять изображения
// в текст с обтеканием изображения текстом, чтобы изображения не формировали
// разрывов». В Word это `wp:anchor` с `<wp:wrapSquare/>`, а не `wp:inline`:
// inline-картинка всегда занимает собственную строку и разрывает текст.

func TestImageNodeUsesAnchorWithWrap(t *testing.T) {
	form := formWithBlock(`[
		{"type":"paragraph","text":"Мембрана укладывается в два слоя."},
		{"type":"image","imageKey":"pic1","imageName":"m.png","widthCm":6,"wrap":"right"},
		{"type":"paragraph","text":"Второй слой идёт перпендикулярно."}
	]`)
	offer := testOfferForBlocks()
	data, err := renderOfferDocxFull(form, offer, nil, fakeImagesFor{"pic1"}, nil)
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	xml := docXMLOf(t, data)

	if !strings.Contains(xml, "<wp:anchor") {
		t.Fatal("картинка не плавающая: текст рядом с ней не потечёт")
	}
	if strings.Contains(xml, "<wp:inline") {
		t.Fatal("inline-картинка занимает свою строку и разрывает текст")
	}
	if !strings.Contains(xml, "<wp:wrapSquare") {
		t.Fatal("нет обтекания текстом (wrapSquare)")
	}
	if !strings.Contains(xml, `r:embed="rIdPhoto1"`) {
		t.Fatal("картинка не связана с файлом пакета")
	}
	if _, ok := imagePartOf(t, data); !ok {
		t.Fatal("файл картинки не попал в пакет")
	}
	// Текст до и после — на месте: картинка не съела окружение.
	for _, want := range []string{"Мембрана укладывается в два слоя.", "Второй слой идёт перпендикулярно."} {
		if !strings.Contains(xml, want) {
			t.Fatalf("текст вокруг картинки пропал: %q", want)
		}
	}
}

// Сторона обтекания пробрасывается: «слева» — текст справа от картинки.
func TestImageNodeWrapSideAndWidth(t *testing.T) {
	form := formWithBlock(`[{"type":"image","imageKey":"pic1","imageName":"m.png","widthCm":5,"wrap":"left","align":"left"}]`)
	data, err := renderOfferDocxFull(form, testOfferForBlocks(), nil, fakeImagesFor{"pic1"}, nil)
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	xml := docXMLOf(t, data)
	if !strings.Contains(xml, "<wp:positionH relativeFrom=\"column\"><wp:align>left</wp:align></wp:positionH>") {
		t.Fatalf("положение картинки не проброшено:\n%s", excerptAround(xml, "<wp:anchor"))
	}
	// 5 см = 1 800 000 EMU.
	if !strings.Contains(xml, `cx="1800000"`) {
		t.Fatalf("ширина картинки не 5 см:\n%s", excerptAround(xml, "<wp:anchor"))
	}
}

// «Сверху и снизу» — тоже допустимый выбор админа, и он тоже остаётся
// плавающим (не inline), иначе настройка ничего бы не меняла.
func TestImageNodeTopAndBottomWrap(t *testing.T) {
	form := formWithBlock(`[{"type":"image","imageKey":"pic1","imageName":"m.png","wrap":"topAndBottom"}]`)
	data, err := renderOfferDocxFull(form, testOfferForBlocks(), nil, fakeImagesFor{"pic1"}, nil)
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	if xml := docXMLOf(t, data); !strings.Contains(xml, "<wp:wrapTopAndBottom/>") {
		t.Fatal("обтекание «сверху и снизу» не попало в документ")
	}
}

// Нет картинки в хранилище — документа это не касается.
func TestImageNodeWithoutSourceIsSkipped(t *testing.T) {
	form := formWithBlock(`[{"type":"image","imageKey":"missing","imageName":"m.png"}]`)
	data, err := renderOfferDocxFull(form, testOfferForBlocks(), nil, fakeImagesFor{"другой"}, nil)
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	if xml := docXMLOf(t, data); strings.Contains(xml, "<wp:anchor") {
		t.Fatal("картинки нет, а плавающий объект в документе появился")
	}
}

// id визуальных элементов уникальны: у бланка свои, у формы и карточек — наши.
func TestImageIDsDoNotCollideWithTemplate(t *testing.T) {
	form := formWithBlock(`[
		{"type":"image","imageKey":"pic1","imageName":"a.png"},
		{"type":"image","imageKey":"pic2","imageName":"b.png"}
	]`)
	cards := []SystemCard{{SKU: "S1", Title: "Система", Description: "Описание", ImageKey: "pic1", ImageName: "a.png"}}
	data, err := renderOfferDocxFull(form, testOfferForBlocks(), cards, fakeImagesFor{"pic1", "pic2"}, nil)
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	ids := map[string]int{}
	for _, m := range reDocPrID.FindAllStringSubmatch(docXMLOf(t, data), -1) {
		ids[m[1]]++
	}
	for id, count := range ids {
		if count > 1 {
			t.Fatalf("повторный id визуального элемента %s (%d раз)", id, count)
		}
	}
}
