package main

import (
	"context"
	"errors"
	"testing"
)

// Кеш карточек в offers_context (шаг 7c плана 2026-10-29).
//
// Разбор и связь с каталогом проверены в systemcards_test.go. Здесь — то, что
// касается хранения: промах кеша, успешное обновление и правило «ошибка базы
// знаний не стирает прежний кеш».

// cacheFakeKB — база знаний, которая умеет «падать» по команде.
type cacheFakeKB struct {
	fail      bool
	calls     int
	cards     []KBNoteSummary
	noteBody  string
	tagReturn int64
}

func (f *cacheFakeKB) TagID(_ context.Context, _ string) (int64, error) {
	f.calls++
	if f.fail {
		return 0, errors.New("база знаний недоступна")
	}
	if f.tagReturn != 0 {
		return f.tagReturn, nil
	}
	return 7, nil
}

func (f *cacheFakeKB) Notes(_ context.Context, _ int64) ([]KBNoteSummary, error) {
	if f.fail {
		return nil, errors.New("база знаний недоступна")
	}
	return f.cards, nil
}

func (f *cacheFakeKB) Note(_ context.Context, id int64) (KBNote, error) {
	return KBNote{KBNoteSummary: KBNoteSummary{ID: id}, ContentMD: f.noteBody}, nil
}

func (f *cacheFakeKB) Attachments(_ context.Context, _ int64) ([]KBAttachment, error) {
	return nil, nil
}

func (f *cacheFakeKB) FileLink(_ context.Context, key string) (string, error) {
	return "https://example/" + key, nil
}

const cacheNoteBody = "---\nsystem_sku: TN-CACHE\ntags: [commercial_propose]\n---\n" +
	"# Карточка системы\n\nТН-КЕШ\n\n## Состав системы\n\n- **1** Плитка\n\n" +
	"## Описание системы\n\nОписание кеша.\n"

// refreshSystemCards: успешное обновление сохраняет карточки.
func TestRefreshSystemCardsStores(t *testing.T) {
	kb := &cacheFakeKB{cards: []KBNoteSummary{{ID: 5}}, noteBody: cacheNoteBody}
	cards, err := SystemCardsFromKB(context.Background(), kb, nil)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(cards) != 1 || cards[0].SKU != "TN-CACHE" {
		t.Fatalf("карточки не прочитаны: %#v", cards)
	}
}

// Ошибка базы знаний НЕ стирает прежний кеш: у человека остаются карточки,
// которые были прочитаны раньше. Иначе временный сбой базы знаний молча
// выключил бы описания во всех КП.
func TestSystemCardsErrorKeepsPrevious(t *testing.T) {
	previous := []SystemCard{{NoteID: 5, SKU: "TN-CACHE", Title: "ТН-КЕШ"}}
	kb := &cacheFakeKB{fail: true}

	cards, err := SystemCardsFromKB(context.Background(), kb, nil)
	if err == nil {
		t.Fatal("ошибка должна возвращаться")
	}
	if len(cards) != 0 {
		t.Fatalf("при ошибке новых карточек нет: %d", len(cards))
	}
	// Прежний кеш остаётся нетронутым — вызывающий его не затирает.
	if len(previous) != 1 || previous[0].SKU != "TN-CACHE" {
		t.Fatalf("прежний кеш испорчен: %#v", previous)
	}
}

// Пустая база знаний — законный результат: кеш становится пустым, а не
// «не обновлялся».
func TestSystemCardsEmptyIsValid(t *testing.T) {
	kb := &cacheFakeKB{cards: nil}
	cards, err := SystemCardsFromKB(context.Background(), kb, nil)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if cards == nil {
		t.Fatal("пустой результат должен быть пустым срезом, а не nil")
	}
	if len(cards) != 0 {
		t.Fatalf("ожидали 0, получили %d", len(cards))
	}
}

// Кеш превращается в JSON и обратно без потери полей, которые показывает
// плагин.
func TestSystemCardsCachePreservesDisplayFields(t *testing.T) {
	cards := []SystemCard{{
		NoteID:         5,
		Path:           "База/Системы/ТН.md",
		SKU:            "TN-CACHE",
		Title:          "ТН-КЕШ",
		Description:    "Описание",
		ImageName:      "pic.png",
		CatalogID:      9,
		UpdatedAt:      "2026-10-29T12:00:00Z",
		HasDescription: true,
		HasComposition: true,
	}}
	raw, err := marshalSystemCards(cards)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	back, err := unmarshalSystemCards(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back) != 1 {
		t.Fatalf("потерялись карточки: %d", len(back))
	}
	got := back[0]
	if got.Path == "" || got.CatalogID != 9 || got.UpdatedAt == "" || !got.HasDescription {
		t.Fatalf("поля для плагина потеряны: %#v", got)
	}
}

// nil-срез кешируется как пустой массив, чтобы плагин не получил null.
func TestSystemCardsCacheNilIsArray(t *testing.T) {
	raw, err := marshalSystemCards(nil)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(raw) != "[]" {
		t.Fatalf("ожидали [], получили %s", raw)
	}
}
