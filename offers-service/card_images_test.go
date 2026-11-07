package main

import (
	"context"
	"testing"
)

// Картинки систем и материалов в блоке (2026-11-05).
//
// Требование пользователя: в конкретном блоке текста должна быть картинка
// системы и/или материала. Форма общая для всех предложений, поэтому в ней
// хранится ИСТОЧНИК картинки, а конкретную карточку даёт само предложение.

func sampleOfferWithItems() Offer {
	offer := sampleOffer()
	offer.Items = []OfferItem{
		{Name: "Мембрана", Unit: "м2", Qty: 1, Price: 100, Sum: 100,
			PhotoKey: "att/membrane.png", ImageName: "Мембрана.png"},
		{Name: "Геотекстиль", Unit: "м2", Qty: 1, Price: 50, Sum: 50},
	}
	return offer
}

// Картинка системы берётся из выбранной системы предложения.
func TestResolveNodeImageSystem(t *testing.T) {
	offer := sampleOfferWithItems()
	offer.Systems = []OfferSystem{{NoteID: 5}}
	card := sampleCard()
	card.ImageKey = "att/system.png"
	rc := renderCtx{offer: offer, cards: []SystemCard{card}}

	node := blockNode{Type: "image", ImageOf: imageOfSystem}
	img, ok := resolveNodeImage(node, rc)
	if !ok || img.Key != "att/system.png" {
		t.Fatalf("картинка системы не найдена: %+v %v", img, ok)
	}
	if img.Title != card.Title {
		t.Fatalf("название для подписи не взято из карточки: %q", img.Title)
	}
}

// Несколько систем: номер выбирает нужную в порядке выбора.
func TestResolveNodeImageSystemByIndex(t *testing.T) {
	offer := sampleOfferWithItems()
	offer.Systems = []OfferSystem{{NoteID: 5}, {NoteID: 6}}
	first := sampleCard()
	first.ImageKey = "att/first.png"
	first.NoteID = 5
	second := sampleCard()
	second.NoteID = 6
	second.ImageKey = "att/second.png"
	rc := renderCtx{offer: offer, cards: []SystemCard{first, second}}

	img, ok := resolveNodeImage(blockNode{ImageOf: imageOfSystem, SystemIndex: 2}, rc)
	if !ok || img.Key != "att/second.png" {
		t.Fatalf("вторая система: %+v %v", img, ok)
	}
}

// Картинка материала берётся из позиции предложения.
func TestResolveNodeImageMaterial(t *testing.T) {
	offer := sampleOfferWithItems()
	rc := renderCtx{offer: offer}
	img, ok := resolveNodeImage(blockNode{ImageOf: imageOfMaterial, ItemIndex: 1}, rc)
	if !ok || img.Key != "att/membrane.png" {
		t.Fatalf("картинка материала не найдена: %+v %v", img, ok)
	}
	if img.Title != "Мембрана" {
		t.Fatalf("подпись не из названия позиции: %q", img.Title)
	}
}

// Позиция без картинки — узел молчит, а не падает.
func TestResolveNodeImageMaterialWithoutPhoto(t *testing.T) {
	rc := renderCtx{offer: sampleOfferWithItems()}
	if _, ok := resolveNodeImage(blockNode{ImageOf: imageOfMaterial, ItemIndex: 2}, rc); ok {
		t.Fatal("картинка нашлась там, где её нет")
	}
}

// Статичный ключ работает как раньше: старые формы не ломаются.
func TestResolveNodeImageStatic(t *testing.T) {
	rc := renderCtx{offer: sampleOfferWithItems()}
	img, ok := resolveNodeImage(blockNode{ImageKey: "att/manual.png"}, rc)
	if !ok || img.Key != "att/manual.png" {
		t.Fatalf("статичная картинка не разрешилась: %+v %v", img, ok)
	}
}

// Нет выбранной системы — узел молчит.
func TestResolveNodeImageSystemWithoutChoice(t *testing.T) {
	rc := renderCtx{offer: sampleOfferWithItems()}
	if _, ok := resolveNodeImage(blockNode{ImageOf: imageOfSystem}, rc); ok {
		t.Fatal("картинка системы нашлась без выбранной системы")
	}
}

// Привязка ключа картинки к карточке по имени из `![[…]]`.
func TestAttachImageKeysPicksAttachment(t *testing.T) {
	kb := &fakeKB{attachments: []KBAttachment{
		{Filename: "Другая.png", Mime: "image/png", Key: "att/other.png"},
		{Filename: "ТН_Стилобат.png", Mime: "image/png", Key: "att/system.png"},
	}}
	cards := []SystemCard{{NoteID: 5, ImageName: "ТН_Стилобат.png"}}
	got := attachImageKeys(context.Background(), kb, cards)
	if got[0].ImageKey != "att/system.png" {
		t.Fatalf("взято не то вложение: %q", got[0].ImageKey)
	}
}

// Имя без расширения в заметке и с расширением во вложении — тоже совпадение.
func TestAttachImageKeysMatchesWithoutExtension(t *testing.T) {
	kb := &fakeKB{attachments: []KBAttachment{
		{Filename: "Схема.png", Mime: "image/png", Key: "att/scheme.png"},
	}}
	cards := []SystemCard{{NoteID: 5, ImageName: "Схема"}}
	got := attachImageKeys(context.Background(), kb, cards)
	if got[0].ImageKey != "att/scheme.png" {
		t.Fatalf("совпадение без расширения не найдено: %q", got[0].ImageKey)
	}
}

// Не-картинка не становится картинкой системы: PDF в карточке — не фото.
func TestAttachImageKeysSkipsNonImages(t *testing.T) {
	kb := &fakeKB{attachments: []KBAttachment{
		{Filename: "паспорт.pdf", Mime: "application/pdf", Key: "att/doc.pdf"},
	}}
	cards := []SystemCard{{NoteID: 5}}
	got := attachImageKeys(context.Background(), kb, cards)
	if got[0].ImageKey != "" {
		t.Fatalf("PDF принят за картинку: %q", got[0].ImageKey)
	}
}

// Ошибка вложений не теряет карточку: картинка необязательна.
func TestAttachImageKeysSurvivesError(t *testing.T) {
	kb := &failingAttachKB{}
	cards := []SystemCard{{NoteID: 5, Title: "Система"}}
	got := attachImageKeys(context.Background(), kb, cards)
	if len(got) != 1 || got[0].Title != "Система" {
		t.Fatalf("карточка потеряна при ошибке вложений: %+v", got)
	}
}

// failingAttachKB — База знаний, у которой падают вложения.
type failingAttachKB struct{}

func (failingAttachKB) TagID(context.Context, string) (int64, error) { return 0, nil }
func (failingAttachKB) Notes(context.Context, int64) ([]KBNoteSummary, error) {
	return nil, nil
}
func (failingAttachKB) Note(context.Context, int64) (KBNote, error) {
	return KBNote{}, nil
}
func (failingAttachKB) Attachments(context.Context, int64) ([]KBAttachment, error) {
	return nil, context.DeadlineExceeded
}
func (failingAttachKB) FileLink(context.Context, string) (string, error) { return "", nil }

// Материалу картинка ставится по вложению его заметки.
func TestAttachMaterialImageKeys(t *testing.T) {
	kb := &fakeKB{attachments: []KBAttachment{
		{Filename: "Мембрана.jpg", Mime: "image/jpeg", Key: "att/mem.jpg"},
	}}
	materials := []Material{{ID: 1, NoteID: 7, Name: "Мембрана"}}
	got := attachMaterialImageKeys(context.Background(), kb, materials)
	if got[0].PhotoKey != "att/mem.jpg" {
		t.Fatalf("картинка материала не привязана: %q", got[0].PhotoKey)
	}
}

// Имени картинки у карточки нет — оно берётся из вложения: без имени
// `imageExt` угадывал бы тип, и картинка уезжала бы в пакет с чужим расширением.
func TestAttachImageKeyFillsName(t *testing.T) {
	kb := &fakeKB{attachments: []KBAttachment{
		{Filename: "Схема.png", Mime: "image/png", Key: "att/scheme.png"},
	}}
	got := attachImageKeys(context.Background(), kb, []SystemCard{{NoteID: 5}})
	if got[0].ImageKey != "att/scheme.png" || got[0].ImageName != "Схема.png" {
		t.Fatalf("ключ или имя не заполнены: %+v", got[0])
	}
}

// Типизированный nil в интерфейсе не равен nil: без отдельной проверки
// `attachImageKeys` упал бы на первом вызове несуществующего клиента.
func TestKBReaderOrNil(t *testing.T) {
	if kbReaderOrNil(nil) != nil {
		t.Fatal("nil-клиент стал источником картинок")
	}
	if kbReaderOrNil(&KBClient{}) != nil {
		t.Fatal("ненастроенный клиент стал источником картинок")
	}
	if got := kbReaderOrNil(&KBClient{baseURL: "http://kb"}); got == nil {
		t.Fatal("настроенный клиент потерян")
	}
}
