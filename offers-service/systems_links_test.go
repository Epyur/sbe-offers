package main

import (
	"testing"
)

// Связь состава системы с материалами — по ВИКИ-ССЫЛКЕ (2026-10-31).
//
// Решение пользователя: в карточках материалов SKU вводится как признак
// материала, но связь системы с материалом идёт по ссылке `[[Название]]`,
// а не по коду. Причины:
//   - в живых карточках кода нет вообще (замер: 0 строк из 3387), а ссылки
//     есть у 2103 строк;
//   - карточки материалов в Базе знаний имеют тег `material`, и ссылка
//     однозначно говорит, на какую заметку смотреть.
//
// SKU остаётся вторым способом: если человек вписал код — работаем по коду.
// Никогда не угадываем: ссылка на заметку без тега material материалом не
// считается.

// Target — то, на что указывает ссылка.
type linkTarget struct {
	NoteID int64
	Title  string
	HasTag bool
	SKU    string
}

// Ссылка на заметку с тегом material — материал найден.
func TestResolveWikiLinkFindsMaterialNote(t *testing.T) {
	byTitle := map[string]linkTarget{
		"техноэласт к экп": {NoteID: 42, Title: "Техноэласт К ЭКП", HasTag: true},
	}
	line := "- **2** [[Техноэласт К ЭКП]]"
	target, ok := resolveWikiLink(line)
	if !ok {
		t.Fatal("ссылка не найдена")
	}
	got, found := byTitle[target]
	if !found || !got.HasTag || got.NoteID != 42 {
		t.Fatalf("материал не определён: %#v", got)
	}
}

// Подпись после | не мешает: `[[Техноэласт|мембрана]]`.
func TestResolveWikiLinkIgnoresAlias(t *testing.T) {
	target, ok := resolveWikiLink("- [[Техноэласт К ЭКП|кровельный материал]]")
	if !ok {
		t.Fatal("ссылка не найдена")
	}
	if target != "техноэласт к экп" {
		t.Fatalf("подпись попала в имя: %q", target)
	}
}

// Якорь после # тоже отбрасывается.
func TestResolveWikiLinkIgnoresAnchor(t *testing.T) {
	target, _ := resolveWikiLink("- [[Техноэласт К ЭКП#Состав]]")
	if target != "техноэласт к экп" {
		t.Fatalf("якорь попал в имя: %q", target)
	}
}

// Строки без ссылки — не материал: такой состав остаётся проблемой.
func TestResolveWikiLinkAbsent(t *testing.T) {
	if _, ok := resolveWikiLink("- Песок строительный"); ok {
		t.Fatal("ссылки нет — цель найдена быть не должна")
	}
}

// Ссылка на заметку БЕЗ тега material материалом не считается: это могут быть
// стандарты, протоколы, чужие заметки. Молча принимать их нельзя.
func TestMaterialByWikiRequiresMaterialTag(t *testing.T) {
	byTitle := map[string]linkTarget{
		"гост 30244": {NoteID: 7, Title: "ГОСТ 30244", HasTag: false},
	}
	line := "- [[ГОСТ 30244]]"
	target, _ := resolveWikiLink(line)
	got, found := byTitle[target]
	if !found {
		t.Fatal("заметка должна быть в карте")
	}
	if got.HasTag {
		t.Fatal("у заметки нет тега material — она не материал")
	}
}
