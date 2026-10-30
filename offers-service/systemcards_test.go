package main

import (
	"context"
	"encoding/json"
	"testing"

	"mailers/offers/model"
	"mailers/offers/skills"
)

// Кеш карточек систем из Базы знаний (шаг 7c плана 2026-10-29).
//
// Карточки читаются из базы знаний, а не вводятся в плагине. В BД они лежат
// как кеш: это даёт работу без базы знаний (раз прочитали — есть) и быструю
// выдачу списка плагину.
//
// Главное правило: кеш НЕ является источником правды. Сброс — всегда
// возможен, а промах базы знаний не должен валить КП.

// fakeKB — подставная база знаний.
type fakeKB struct {
	tagID       int64
	tagErr      error
	notes       []KBNoteSummary
	note        KBNote
	attachments []KBAttachment
	link        string
	calls       int
}

func (f *fakeKB) TagID(_ context.Context, name string) (int64, error) {
	f.calls++
	if f.tagErr != nil {
		return 0, f.tagErr
	}
	return f.tagID, nil
}

func (f *fakeKB) Notes(_ context.Context, tagID int64) ([]KBNoteSummary, error) {
	f.calls++
	return f.notes, nil
}

func (f *fakeKB) Note(_ context.Context, id int64) (KBNote, error) {
	return f.note, nil
}

func (f *fakeKB) Attachments(_ context.Context, id int64) ([]KBAttachment, error) {
	return f.attachments, nil
}

func (f *fakeKB) FileLink(_ context.Context, key string) (string, error) {
	return f.link, nil
}

func sampleCardNote() KBNote {
	return KBNote{
		KBNoteSummary: KBNoteSummary{ID: 5, Title: "Карточка", Path: "База/Системы/ТН.md"},
		ContentMD: "---\nsystem_sku: TN-TEST\ntags: [commercial_propose]\n---\n" +
			"# Карточка системы\n\nТН-ТЕСТ\n\n## Состав системы\n\n- **1** Плитка\n\n" +
			"## Описание системы\n\nОписание.\n\n![[pic.png|335]]\n",
	}
}

// Разбор карточек из базы знаний: список с кодом системы и состоянием связи.
func TestSystemCardsFromKB(t *testing.T) {
	kb := &fakeKB{
		tagID: 7,
		notes: []KBNoteSummary{{ID: 5, Title: "Карточка", Path: "База/ТН.md"}},
		note:  sampleCardNote(),
	}
	cards, err := SystemCardsFromKB(context.Background(), kb, nil)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("ожидали одну карточку, получили %d", len(cards))
	}
	c := cards[0]
	if c.SKU != "TN-TEST" {
		t.Errorf("system_sku не прочитан: %q", c.SKU)
	}
	if c.Title != "ТН-ТЕСТ" {
		t.Errorf("название: %q", c.Title)
	}
	if c.NoteID != 5 {
		t.Errorf("id заметки: %d", c.NoteID)
	}
	if !c.HasDescription || !c.HasComposition {
		t.Errorf("признаки блоков не выставлены: %+v", c)
	}
}

// Связь с каталогом: карточка связывается по system_sku. Нет совпадения —
// помечено «не связана», но карточка в списке остаётся.
func TestSystemCardsLinkState(t *testing.T) {
	kb := &fakeKB{tagID: 7, notes: []KBNoteSummary{{ID: 5}}, note: sampleCardNote()}
	catalog := []CatalogItem{{ID: 1, Name: "ТН-ТЕСТ", SKU: "TN-TEST"}}
	cards, err := SystemCardsFromKB(context.Background(), kb, catalog)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if cards[0].CatalogID != 1 {
		t.Fatalf("карточка не связалась с каталогом: %+v", cards[0])
	}

	// Код другой — связи нет.
	catalog[0].SKU = "OTHER"
	cards, _ = SystemCardsFromKB(context.Background(), kb, catalog)
	if cards[0].CatalogID != 0 {
		t.Fatalf("связь не должна появляться при другом коде: %+v", cards[0])
	}
}

// База знаний не настроена — это не ошибка сборки КП: карточек просто нет.
func TestSystemCardsNotConfigured(t *testing.T) {
	cards, err := SystemCardsFromKB(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("отсутствие базы знаний не должно быть ошибкой: %v", err)
	}
	if len(cards) != 0 {
		t.Fatalf("ожидали пустой список, получили %d", len(cards))
	}
}

// Ошибка базы знаний не должна валить КП: список пустой, ошибка наверх.
func TestSystemCardsKBError(t *testing.T) {
	kb := &fakeKB{tagErr: errKBNotConfigured}
	cards, err := SystemCardsFromKB(context.Background(), kb, nil)
	if err == nil {
		t.Fatal("ошибка базы знаний должна возвращаться")
	}
	if len(cards) != 0 {
		t.Fatalf("при ошибке карточек нет: %d", len(cards))
	}
}

// Карточка без названия не связывается ни с чем и помечается как незаполненная.
func TestSystemCardsEmptyCard(t *testing.T) {
	kb := &fakeKB{
		tagID: 7,
		notes: []KBNoteSummary{{ID: 9}},
		note:  KBNote{ContentMD: "---\nsystem_sku: X\n---\n"},
	}
	cards, err := SystemCardsFromKB(context.Background(), kb, nil)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("пустая карточка всё равно в списке: %d", len(cards))
	}
	if cards[0].Empty != true {
		t.Errorf("пустая карточка не помечена: %+v", cards[0])
	}
}

// Карточка по коду системы — для сборки документа.
func TestFindSystemCard(t *testing.T) {
	cards := []SystemCard{
		{NoteID: 1, SKU: "A", Title: "Система А"},
		{NoteID: 2, SKU: "B", Title: "Система Б"},
	}
	got, ok := FindSystemCard(cards, "B")
	if !ok || got.NoteID != 2 {
		t.Fatalf("карточка по коду не найдена: %+v %v", got, ok)
	}
	if _, ok := FindSystemCard(cards, "C"); ok {
		t.Fatal("для несуществующего кода карточки быть не должно")
	}
}

// Кеш в offers_context переживает перезапуск: читается из БД, а не только
// из памяти.
func TestContextCacheRoundTrip(t *testing.T) {
	cards := []SystemCard{
		{NoteID: 5, SKU: "TN-TEST", Title: "ТН-ТЕСТ", Description: "Описание", ImageName: "pic.png"},
	}
	raw, err := marshalSystemCards(cards)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	back, err := unmarshalSystemCards(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back) != 1 || back[0].SKU != "TN-TEST" {
		t.Fatalf("кеш потерял данные: %#v", back)
	}
}

// Старый формат кеша (разделы текстом, ProductContext) должен читаться без
// падения: у кого-то в БД остался прежний offers_context.
func TestContextCacheToleratesOldFormat(t *testing.T) {
	old := map[string]any{"sections": map[string]string{"positioning": "старое"}}
	raw, _ := json.Marshal(old)
	cards, err := unmarshalSystemCards(raw)
	if err != nil {
		t.Fatalf("старый кеш не должен ломать чтение: %v", err)
	}
	if len(cards) != 0 {
		t.Fatalf("в старом кеше карточек нет: %d", len(cards))
	}
}

// SystemCardPromptText — то, что уходит в промпт модели.
func TestSystemCardPrompt(t *testing.T) {
	card := SystemCard{
		Title:       "ТН-ТЕСТ",
		Description: "Описание системы.",
		Composition: []string{"Плитка", "Песок"},
	}
	text := card.PromptText()
	if text == "" {
		t.Fatal("пустой текст для модели")
	}
}

// Внутренний разбор карточки в модель скилов доступен и наружу.
func TestSystemCardSkillsBridge(t *testing.T) {
	parsed := skills.ParseSystemCard("---\nsystem_sku: Z\n---\n# x\n")
	if parsed.SKU != "Z" {
		t.Fatalf("system_sku: %q", parsed.SKU)
	}
	var _ = model.DefaultContext
}
