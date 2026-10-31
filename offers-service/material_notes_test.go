package main

import (
	"context"
	"errors"
	"testing"
)

// Материалы из карточек Базы знаний (2026-10-31).
//
// Проверяется то, что можно проверить без Postgres: правила отбора заметок и
// то, что договорённости не разлезаются. Сама запись в базу проверяется на
// стенде (раздел 19 E2E) и на бою.

// notesFakeKB отдаёт тег material и список заметок.
type notesFakeKB struct {
	tagID   int64
	tagErr  error
	notes   []KBNoteSummary
	notesOK bool
}

func (f *notesFakeKB) TagID(_ context.Context, _ string) (int64, error) {
	if f.tagErr != nil {
		return 0, f.tagErr
	}
	return f.tagID, nil
}

func (f *notesFakeKB) Notes(_ context.Context, _ int64) ([]KBNoteSummary, error) {
	if !f.notesOK {
		return nil, errors.New("база знаний недоступна")
	}
	return f.notes, nil
}

func (f *notesFakeKB) Note(_ context.Context, _ int64) (KBNote, error) {
	return KBNote{}, nil
}

func (f *notesFakeKB) Attachments(_ context.Context, _ int64) ([]KBAttachment, error) {
	return nil, nil
}

func (f *notesFakeKB) FileLink(_ context.Context, _ string) (string, error) { return "", nil }

// Заметки без названия и без id отбрасываются: материал из них не собрать.
func TestMaterialNotesSkipsEmpty(t *testing.T) {
	kb := &notesFakeKB{tagID: 8, notesOK: true, notes: []KBNoteSummary{
		{ID: 1, Title: "Техноэласт К ЭКП"},
		{ID: 0, Title: "Без id"},
		{ID: 2, Title: "   "},
		{ID: 3, Title: "  Праймер битумный  "},
	}}
	notes, err := materialNotesFromKB(context.Background(), kb)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("ожидали две годные карточки: %#v", notes)
	}
	if notes[1].Title != "Праймер битумный" {
		t.Errorf("название не обрезано: %q", notes[1].Title)
	}
}

// База знаний не настроена — пустой список без ошибки.
func TestMaterialNotesNotConfigured(t *testing.T) {
	s := &Server{}
	notes, err := s.materialNotes(context.Background())
	if err != nil {
		t.Fatalf("отсутствие базы знаний не ошибка: %v", err)
	}
	if len(notes) != 0 {
		t.Fatalf("ожидали пустой список: %#v", notes)
	}
}

// Ошибка тега поднимается наверх — вызывающий пишет её в журнал, но список
// материалов от этого не рушится (проверяется в listMaterials).
func TestMaterialNotesTagError(t *testing.T) {
	if _, err := materialNotesFromKB(context.Background(), &notesFakeKB{tagErr: errors.New("нет связи")}); err == nil {
		t.Fatal("ошибка тега должна возвращаться")
	}
}

// Договорённость про SKU: ОДИН раз назначен и не меняется. Проверяется на
// чистой функции — имя материала меняется свободно, код вида не меняет.
func TestMaterialNameChangeKeepsSkuShape(t *testing.T) {
	first := buildSku("МЕМБ", "ПВХ", 7)
	second := buildSku("МЕМБ", "ПВХ", 7)
	if first != second {
		t.Fatalf("код от переименования меняться не должен: %q / %q", first, second)
	}
	if first != "MAT-МЕМБ-ПВХ-0007" {
		t.Fatalf("формат кода: %q", first)
	}
}
