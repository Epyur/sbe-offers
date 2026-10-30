package main

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// Карточка системы в DOCX (шаг 7d плана 2026-10-29).
//
// Описание системы и фото продукта берутся из Базы знаний. Цены, единицы и
// артикулы — по-прежнему из каталога. Если карточки нет или она сломана,
// документ собирается как раньше: карточка не обязательна.

// fakeImages — источник картинок. nil означает «файла нет».
type fakeImages struct {
	data []byte
	err  error
	key  string
}

func (f *fakeImages) Get(_ context.Context, key string) ([]byte, error) {
	f.key = key
	if f.err != nil {
		return nil, f.err
	}
	return f.data, nil
}

func sampleCard() SystemCard {
	return SystemCard{
		NoteID:      5,
		SKU:         "TN-TEST",
		Title:       "ТН-ТЕСТ",
		Description: "Описание системы из карточки Базы знаний.",
		Composition: []string{"Плитка", "Песок"},
		ImageName:   "pic.png",
	}
}

// Описание из карточки попадает в документ вместо ничего.
func TestDocxUsesCardDescription(t *testing.T) {
	offer := sampleOffer()
	cards := []SystemCard{sampleCard()}
	docx, err := renderOfferDocxWithCards(OfferForm{Title: "Коммерческое предложение"},
		offer, cards, nil)
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	body := string(docxEntry(t, docx, "word/document.xml"))
	if !strings.Contains(body, "Описание системы из карточки Базы знаний.") {
		t.Fatal("описание карточки не попало в документ")
	}
}

// Согласованный менеджером текст важнее карточки: если секция принята, в
// документе она, а не текст из Базы знаний.
func TestDocxAcceptedCopyBeatsCard(t *testing.T) {
	offer := sampleOffer()
	offer.FormValues = map[string]string{"copy.intro": "Согласованный вводный текст."}
	docx, err := renderOfferDocxWithCards(OfferForm{Title: "Коммерческое предложение"},
		offer, []SystemCard{sampleCard()}, nil)
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	body := string(docxEntry(t, docx, "word/document.xml"))
	if !strings.Contains(body, "Согласованный вводный текст.") {
		t.Fatal("согласованный текст потерян")
	}
}

// Карточки нет — документ собирается как раньше.
func TestDocxWithoutCards(t *testing.T) {
	docx, err := renderOfferDocxWithCards(OfferForm{Title: "Коммерческое предложение"},
		sampleOffer(), nil, nil)
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	if _, err := readDocxPart(docx, "word/document.xml"); err != nil {
		t.Fatalf("документ без карточек не собран: %v", err)
	}
}

// Картинка из карточки встраивается: в пакете появляется media-файл, в тексте
// документа — <w:drawing>.
func TestDocxEmbedsCardImage(t *testing.T) {
	images := &fakeImages{data: []byte("\x89PNG\r\n\x1a\n fake png bytes for test")}
	card := sampleCard()
	card.ImageKey = "kb/att/pic.png"

	docx, err := renderOfferDocxWithCards(OfferForm{Title: "Коммерческое предложение"},
		sampleOffer(), []SystemCard{card}, images)
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	if images.key != "kb/att/pic.png" {
		t.Fatalf("картинка запрошена не по тому ключу: %q", images.key)
	}
	body := string(docxEntry(t, docx, "word/document.xml"))
	if !strings.Contains(body, "<w:drawing>") {
		t.Fatal("картинка не встроена")
	}
	if !hasDocxMedia(t, docx) {
		t.Fatal("в пакете нет media-файла картинки")
	}
}

// Картинка не нашлась — документ всё равно собирается, без картинки.
func TestDocxCardImageMissingIsNoop(t *testing.T) {
	images := &fakeImages{err: errors.New("нет файла")}
	card := sampleCard()
	card.ImageKey = "kb/att/missing.png"

	docx, err := renderOfferDocxWithCards(OfferForm{Title: "Коммерческое предложение"},
		sampleOffer(), []SystemCard{card}, images)
	if err != nil {
		t.Fatalf("отсутствие картинки не должно валить сборку: %v", err)
	}
	body := string(docxEntry(t, docx, "word/document.xml"))
	if strings.Contains(body, "<w:drawing>") {
		t.Fatal("картинки быть не должно")
	}
	if !strings.Contains(body, "Описание системы из карточки Базы знаний.") {
		t.Fatal("описание при этом должно остаться")
	}
}

// Стили и нумерация бланка не портятся встраиванием карточки.
func TestDocxCardKeepsTemplate(t *testing.T) {
	images := &fakeImages{data: []byte("png")}
	card := sampleCard()
	card.ImageKey = "kb/att/pic.png"

	docx, err := renderOfferDocxWithCards(OfferForm{Title: "Коммерческое предложение"},
		sampleOffer(), []SystemCard{card}, images)
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	if !bytes.Equal(docxEntry(t, docx, "word/styles.xml"), templateEntry(t, "word/styles.xml")) {
		t.Fatal("styles.xml изменился")
	}
	if !bytes.Equal(docxEntry(t, docx, "word/numbering.xml"), templateEntry(t, "word/numbering.xml")) {
		t.Fatal("numbering.xml изменился")
	}
}

// readDocxPart читает часть архива.
func readDocxPart(docx []byte, name string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, errors.New("часть не найдена")
}

// hasDocxMedia — есть ли в пакете хоть один файл в word/media.
func hasDocxMedia(t *testing.T, docx []byte) bool {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		return false
	}
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "word/media/offerimg") {
			return true
		}
	}
	return false
}
