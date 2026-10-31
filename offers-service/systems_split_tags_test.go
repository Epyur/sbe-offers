package main

import (
	"context"
	"testing"
)

// Материалы не должны попадать в раздел «Системы» (2026-10-31, проверка
// пользователя).
//
// Живой дефект, который здесь воспроизводится: kb-service в ленте
// `/api/kb/notes` тегов НЕ отдаёт (проверено на бою: в ответе нет поля `tags`),
// поэтому деление «материал или система» видело пустой список тегов и
// записывало в системы все 1541 карточку, включая 1165 материалов. Признак
// надо брать из ПОЛНОЙ заметки, где `tags` есть.

type tagsKB struct{ notes []KBNote }

func (f *tagsKB) TagID(_ context.Context, _ string) (int64, error) { return 7, nil }

// Notes отдаёт ленту БЕЗ тегов — как настоящий сервис.
func (f *tagsKB) Notes(_ context.Context, _ int64) ([]KBNoteSummary, error) {
	out := make([]KBNoteSummary, 0, len(f.notes))
	for _, n := range f.notes {
		out = append(out, KBNoteSummary{ID: n.ID, Title: n.Title, Path: n.Path, UpdatedAt: n.UpdatedAt})
	}
	return out, nil
}

func (f *tagsKB) Note(_ context.Context, id int64) (KBNote, error) {
	for _, n := range f.notes {
		if n.ID == id {
			return n, nil
		}
	}
	return KBNote{}, nil
}

func (f *tagsKB) Attachments(_ context.Context, _ int64) ([]KBAttachment, error) {
	return nil, nil
}

func (f *tagsKB) FileLink(_ context.Context, _ string) (string, error) { return "", nil }

func cardWithTags(id int64, title string, tags ...KBTag) KBNote {
	return KBNote{
		KBNoteSummary: KBNoteSummary{ID: id, Title: title, Tags: tags},
		ContentMD:     "# " + title + "\n\n" + title + "\n",
	}
}

// Карточка с тегом material в раздел «Системы» не попадает, хотя в ленте тегов нет.
func TestSystemsFromKBExcludesMaterials(t *testing.T) {
	kb := &tagsKB{notes: []KBNote{
		cardWithTags(1, "Мембрана ПВХ", KBTag{ID: 7, Name: "commercial_propose"}, KBTag{ID: 8, Name: "material"}),
		cardWithTags(2, "ТН-Кровля Классик", KBTag{ID: 7, Name: "commercial_propose"}),
	}}
	systems, err := SystemsFromKB(context.Background(), kb, nil)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(systems) != 1 {
		t.Fatalf("в системах должна остаться одна карточка: %#v", systemTitles(systems))
	}
	if systems[0].Title != "ТН-Кровля Классик" {
		t.Fatalf("в системах оказалась не та карточка: %q", systems[0].Title)
	}
}

// Делитель работает в обе стороны на карточках с тегами.
func TestSplitCardsByTagsSplitsBothWays(t *testing.T) {
	notes := []KBNoteSummary{
		{ID: 1, Tags: []KBTag{{Name: "commercial_propose"}, {Name: "material"}}},
		{ID: 2, Tags: []KBTag{{Name: "commercial_propose"}}},
	}
	systems, materials := splitCardsByTags(notes)
	if len(systems) != 1 || systems[0].ID != 2 {
		t.Fatalf("системы: %#v", systems)
	}
	if len(materials) != 1 || materials[0].ID != 1 {
		t.Fatalf("материалы: %#v", materials)
	}
}

// Тег из полной заметки доходит до признака материала: встраивание
// KBNoteSummary и отдельное поле Tags в KBNote раньше перекрывали друг друга.
func TestKBNoteCarriesTags(t *testing.T) {
	note := KBNote{
		KBNoteSummary: KBNoteSummary{Tags: []KBTag{{ID: 8, Name: "material"}}},
	}
	if !noteHasTag(KBNoteSummary{Tags: note.Tags}, materialTagName) {
		t.Fatal("тег полной заметки не виден признаку материала")
	}
}

func systemTitles(cards []SystemCard) []string {
	out := make([]string, 0, len(cards))
	for _, c := range cards {
		out = append(out, c.Title)
	}
	return out
}
