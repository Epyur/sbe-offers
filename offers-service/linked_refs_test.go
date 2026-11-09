package main

import (
	"context"
	"testing"

	"mailers/offers/skills"
)

// Ссылки материалов состава (2026-11-09).
//
// Живой запрос технолога: в КП должны попадать документы не только систем, но и
// продуктов, из которых они состоят. Правило — один уровень: из заметки системы
// берём состав, ссылку состава разрешаем в материал справочника, у материала
// читаем раздел «Ссылки».

// fakeNoteKB — база знаний с разными заметками по id (для чтения материалов).
type fakeNoteKB struct {
	notes map[int64]KBNote
}

func (f *fakeNoteKB) TagID(context.Context, string) (int64, error) { return 0, nil }
func (f *fakeNoteKB) Notes(context.Context, int64) ([]KBNoteSummary, error) {
	return nil, nil
}
func (f *fakeNoteKB) Note(_ context.Context, id int64) (KBNote, error) {
	return f.notes[id], nil
}
func (f *fakeNoteKB) Attachments(context.Context, int64) ([]KBAttachment, error) {
	return nil, nil
}
func (f *fakeNoteKB) FileLink(context.Context, string) (string, error) { return "", nil }

// collectOfferRefs берёт и собственные ссылки системы, и ссылки её материалов,
// а повторы по адресу схлопывает.
func TestCollectOfferRefsIncludesLinkedMaterialRefs(t *testing.T) {
	cards := []SystemCard{
		{
			NoteID: 1,
			Refs: []skills.CardRef{
				{Label: "Страница системы", URL: "https://tn.ru/sys/1", Kind: skills.RefKindPage},
				{Label: "Техлист системы", URL: "https://tn.ru/doc/shared.pdf", Kind: skills.RefKindDoc},
			},
			LinkedRefs: []skills.CardRef{
				{Label: "Техлист материала", URL: "https://tn.ru/doc/shared.pdf", Kind: skills.RefKindDoc},
				{Label: "СТО материала", URL: "https://tn.ru/doc/sto.pdf", Kind: skills.RefKindDoc},
			},
		},
		{
			NoteID: 2,
			Refs: []skills.CardRef{
				{Label: "Страница второй системы", URL: "https://tn.ru/sys/2", Kind: skills.RefKindPage},
			},
		},
	}
	got := collectOfferRefs(cards)
	urls := make(map[string]bool, len(got))
	for _, ref := range got {
		if urls[ref.URL] {
			t.Fatalf("адрес повторился в сборке: %q", ref.URL)
		}
		urls[ref.URL] = true
	}
	for _, want := range []string{
		"https://tn.ru/sys/1",
		"https://tn.ru/sys/2",
		"https://tn.ru/doc/shared.pdf",
		"https://tn.ru/doc/sto.pdf",
	} {
		if !urls[want] {
			t.Fatalf("в сборке нет адреса %q: %#v", want, got)
		}
	}
	// shared.pdf пришёл из двух мест (система и материал) — один пункт.
	if n := countURL(got, "https://tn.ru/doc/shared.pdf"); n != 1 {
		t.Fatalf("повтор адреса схлопнулся в %d пунктов, ожидали 1", n)
	}
}

// enrichLinkedRefs читает раздел «Ссылки» материалов, на которые ведёт состав,
// и только на один уровень — ссылки соседей материалов не подтягиваются.
func TestEnrichLinkedRefsOneLevel(t *testing.T) {
	kb := &fakeNoteKB{notes: map[int64]KBNote{
		10: {KBNoteSummary: KBNoteSummary{ID: 10}, ContentMD: mdWithRefs(
			"- [Техлист мембраны](https://tn.ru/doc/memb.pdf)")},
		11: {KBNoteSummary: KBNoteSummary{ID: 11}, ContentMD: mdWithRefs(
			"- [Сертификат плитки](https://tn.ru/doc/tile.pdf)")},
		// 12 — «второй уровень»: на него материал ссылается, но подтягивать
		// его ссылки нельзя. Здесь этого и не видно, но пусть заметка будет.
		12: {KBNoteSummary: KBNoteSummary{ID: 12}, ContentMD: mdWithRefs(
			"- [Лишнее](https://tn.ru/doc/too-deep.pdf)")},
	}}
	materials := []Material{
		{ID: 1, NoteID: 10, NoteTitle: "Мембрана ПВХ"},
		{ID: 2, NoteID: 11, NoteTitle: "Плитка тротуарная"},
	}
	cards := []SystemCard{{
		NoteID: 1,
		Title:  "ТН-СИСТЕМА",
		Composition: []string{
			"- **1** [[Мембрана ПВХ]]",
			"- **2** [[Плитка тротуарная]]",
			"- **3** Устройство основания", // слой, ссылки нет
		},
	}}
	got := enrichLinkedRefs(context.Background(), kb, cards, materials)
	if len(got) != 1 {
		t.Fatalf("ждали одну карточку, получили %d", len(got))
	}
	urls := refURLSet(got[0].LinkedRefs)
	for _, want := range []string{"https://tn.ru/doc/memb.pdf", "https://tn.ru/doc/tile.pdf"} {
		if !urls[want] {
			t.Fatalf("ссылка материала %q не подтянулась: %#v", want, got[0].LinkedRefs)
		}
	}
	if urls["https://tn.ru/doc/too-deep.pdf"] {
		t.Fatal("подтянулась ссылка второго уровня — правило «один уровень» нарушено")
	}
}

// Без справочника (материал не сопоставлен) ссылок материалов нет.
func TestEnrichLinkedRefsUnknownMaterialSkipped(t *testing.T) {
	kb := &fakeNoteKB{notes: map[int64]KBNote{}}
	cards := []SystemCard{{NoteID: 1, Composition: []string{"- [[Неизвестный материал]]"}}}
	got := enrichLinkedRefs(context.Background(), kb, cards, nil)
	if len(got) != 1 || len(got[0].LinkedRefs) != 0 {
		t.Fatalf("ссылки завелись без материала справочника: %#v", got)
	}
}

// mdWithRefs — заметка с разделом «Ссылки».
func mdWithRefs(body string) string {
	return "# Материал\n\n## Описание\n\nтекст\n\n## Ссылки\n\n" + body + "\n"
}

func countURL(refs []skills.CardRef, url string) int {
	n := 0
	for _, ref := range refs {
		if ref.URL == url {
			n++
		}
	}
	return n
}

func refURLSet(refs []skills.CardRef) map[string]bool {
	out := make(map[string]bool, len(refs))
	for _, ref := range refs {
		out[ref.URL] = true
	}
	return out
}
