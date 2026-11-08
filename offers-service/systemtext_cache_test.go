package main

import (
	"context"
	"strings"
	"testing"
)

// Кэш дописанного текста системы (2026-11-07).
//
// Живая жалоба: метка `{{текст: …}}` печатала только сохранённое, а таблица
// `offers_system_text` была пуста — метка молчала у каждой системы. Теперь
// сборка дописывает недостающее, и здесь закреплено главное для счёта за
// модель: второй раз за тот же текст платить нечем.
//
// Проверки идут тем же путём, что печать: сборка → ключ кэша → ответ модели.

// systemTextCacheCard — карточка системы для кэша.
func systemTextCacheCard() SystemCard {
	return SystemCard{
		NoteID:      5874,
		Title:       "ТН-ФУНДАМЕНТ Дренаж Проф",
		Description: "Система гидроизоляции фундамента.",
		Composition: []string{"Мембрана Logicbase V-SL"},
	}
}

// cache с системами и счётчиком вызовов модели.
func systemTextCache(fake *sysTextCompleter) *layoutPromptCache {
	return &layoutPromptCache{
		values:    map[string]string{},
		offer:     Offer{Number: "003/2026"},
		cards:     []SystemCard{systemTextCacheCard()},
		completed: fake,
	}
}

// sysTextCompleter считает вызовы и отвечает готовым текстом.
type sysTextCompleter struct {
	calls int
}

func (c *sysTextCompleter) Complete(context.Context, string, string, int) (string, error) {
	c.calls++
	return "## Обоснование\nСистема закрывает подземный контур дома.", nil
}

// Первый вызов пишет модель, второй берёт тот же текст из кэша сборки.
func TestSystemTextCacheWritesOnceThenReuses(t *testing.T) {
	fake := &sysTextCompleter{}
	cache := systemTextCache(fake)
	instruction := systemTextInstruction[systemTextDescription]

	first := cache.systemText(5874, systemTextDescription, instruction)
	if strings.TrimSpace(first) == "" {
		t.Fatal("текст не написан")
	}
	if strings.Contains(first, "##") {
		t.Fatalf("в текст попала разметка: %q", first)
	}
	second := cache.systemText(5874, systemTextDescription, instruction)
	if fake.calls != 1 {
		t.Fatalf("модель вызвана %d раз, ожидался один", fake.calls)
	}
	if first != second {
		t.Fatalf("второй вызов отдал другой текст:\n%q\n%q", first, second)
	}
	if got := cache.values[genKeySystemText(5874)+"."+hashShort(instruction)]; got == "" {
		t.Fatal("текст не попал в кэш сборки: следующая сборка снова заплатит моделью")
	}
}

// Кэш прошлой сборки принимается, только если данные предложения не менялись:
// иначе в документе остались бы старые цифры.
func TestSystemTextCacheUsesPreviousBuildWhenDataUnchanged(t *testing.T) {
	instruction := systemTextInstruction[systemTextDescription]
	offer := Offer{Number: "003/2026", Items: []OfferItem{{Name: "Мембрана", Qty: 1, Price: 500, Sum: 500}}}
	key := genKeySystemText(5874) + "." + hashShort(instruction)

	cache := &layoutPromptCache{
		values:   map[string]string{},
		offer:    offer,
		cards:    []SystemCard{systemTextCacheCard()},
		section:  hashOfferFacts(offer),
		previous: map[string]string{genKeySection: hashOfferFacts(offer), key: "Текст прошлой сборки."},
	}
	if got := cache.systemText(5874, systemTextDescription, instruction); got != "Текст прошлой сборки." {
		t.Fatalf("текст прошлой сборки не принят: %q", got)
	}

	// Данные изменились — прежний текст не годится.
	changed := offer
	changed.Items = []OfferItem{{Name: "Мембрана", Qty: 2, Price: 500, Sum: 1000}}
	cache2 := &layoutPromptCache{
		values:    map[string]string{},
		offer:     changed,
		cards:     []SystemCard{systemTextCacheCard()},
		section:   hashOfferFacts(changed),
		previous:  map[string]string{genKeySection: hashOfferFacts(offer), key: "Текст прошлой сборки."},
		completed: &sysTextCompleter{},
	}
	if got := cache2.systemText(5874, systemTextDescription, instruction); got == "Текст прошлой сборки." {
		t.Fatal("кэш прошлой сборки принят при изменившихся данных: в документе были бы старые цифры")
	}
}

// Неизвестная заметка и пустая инструкция — пусто, а не вызов модели.
func TestSystemTextCacheIsSilentWithoutCard(t *testing.T) {
	fake := &sysTextCompleter{}
	cache := systemTextCache(fake)
	if got := cache.systemText(999, systemTextDescription, systemTextInstruction[systemTextDescription]); got != "" {
		t.Fatalf("текст напечатан для неизвестной заметки: %q", got)
	}
	if got := cache.systemText(5874, systemTextDescription, "  "); got != "" {
		t.Fatalf("текст написан по пустой инструкции: %q", got)
	}
	if fake.calls != 0 {
		t.Fatalf("модель вызвана %d раз", fake.calls)
	}
}

// Авария модели — пусто, без паники: сборка документа важнее текста.
func TestSystemTextCacheSurvivesModelFailure(t *testing.T) {
	cache := &layoutPromptCache{
		values:    map[string]string{},
		offer:     Offer{},
		cards:     []SystemCard{systemTextCacheCard()},
		completed: failingCompleter{},
	}
	if got := cache.systemText(5874, systemTextDescription, systemTextInstruction[systemTextDescription]); got != "" {
		t.Fatalf("при аварии модели вернулся текст %q", got)
	}
}
