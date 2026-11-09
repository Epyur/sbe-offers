package main

import (
	"context"
	"log"

	"mailers/offers/skills"
)

// Ссылки материалов, на которые ведёт состав системы (2026-11-09).
//
// Требование технолога: в КП должны попадать документы не только самих систем,
// но и продуктов в них. Правило — «на один уровень»:
//
//   - у заметки системы берём состав (`[[Название материала]]`);
//   - каждую ссылку состава разрешаем в материал справочника;
//   - у найденного материала читаем его раздел «Ссылки» и забираем ссылки.
//
// Глубже не ходим: материалы сами на другие заметки не ссылаются по смыслу
// состава, а «переход на два уровня» превратил бы раздел ссылок в свалку.
//
// Где это делается. При перечитывании карточек из Базы знаний (`refreshSystemCards`),
// а не при каждой сборке документа: печать читает кеш и в сеть не ходит.
// Ошибка чтения одной заметки не рушит остальные: у карточки просто не будет
// этих ссылок, а причина уходит в журнал.

// enrichLinkedRefs дозаполняет у карточек LinkedRefs по их составу.
//
// materials — справочник материалов, уже разрешённый по «названию заметки».
// kb — читалка Базы знаний; kb == nil — ничего не делаем (стенд без базы).
// Возвращает карточки с заполненным полем; вход не меняется.
func enrichLinkedRefs(ctx context.Context, kb KBReader, cards []SystemCard, materials []Material) []SystemCard {
	if kb == nil || len(cards) == 0 {
		return cards
	}
	byNote := materialIndexByNote(materials)
	// Кеш заметок материалов на заход: один материал часто входит в несколько
	// систем, и второй поход за ним означал бы повторный запрос.
	noteCache := make(map[int64][]skills.CardRef)
	out := make([]SystemCard, len(cards))
	copy(out, cards)
	for i := range out {
		out[i].LinkedRefs = linkedRefsForCard(ctx, kb, out[i], byNote, noteCache)
	}
	return out
}

// materialIndexByNote — материалы по ключу нормализованного названия заметки.
// Та же нормализация, что в resolveComposition: правка в одном месте правила
// сравнения имён не должна разойтись со вторым местом.
func materialIndexByNote(materials []Material) map[string]Material {
	byNote := make(map[string]Material, len(materials))
	for _, m := range materials {
		if key := normalizeWikiTarget(m.NoteTitle); key != "" {
			byNote[key] = m
		}
	}
	return byNote
}

// linkedRefsForCard — внешние ссылки материалов, на которые ведёт состав.
func linkedRefsForCard(ctx context.Context, kb KBReader, card SystemCard,
	byNote map[string]Material, noteCache map[int64][]skills.CardRef) []skills.CardRef {
	var out []skills.CardRef
	seenURL := make(map[string]bool)
	seenNote := make(map[int64]bool)
	for _, line := range card.Composition {
		target := wikiTarget(line)
		if target == "" {
			continue
		}
		m, ok := byNote[target]
		if !ok || m.NoteID <= 0 || seenNote[m.NoteID] {
			continue
		}
		seenNote[m.NoteID] = true
		for _, ref := range materialNoteRefs(ctx, kb, m.NoteID, noteCache) {
			if ref.URL == "" || seenURL[ref.URL] {
				continue
			}
			seenURL[ref.URL] = true
			out = append(out, ref)
		}
	}
	return out
}

// materialNoteRefs — раздел «Ссылки» заметки материала (с кешем на заход).
func materialNoteRefs(ctx context.Context, kb KBReader, noteID int64,
	cache map[int64][]skills.CardRef) []skills.CardRef {
	if refs, ok := cache[noteID]; ok {
		return refs
	}
	var refs []skills.CardRef
	note, err := kb.Note(ctx, noteID)
	if err != nil {
		log.Printf("ссылки материла %d: заметка не прочитана: %v", noteID, err)
	} else {
		refs = skills.ParseCardRefs(note.ContentMD)
	}
	cache[noteID] = refs
	return refs
}
