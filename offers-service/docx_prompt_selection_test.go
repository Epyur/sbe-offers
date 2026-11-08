package main

import (
	"context"
	"strings"
	"testing"
)

// Вступление КП описывает ВСЕ выбранные системы (2026-11-08).
//
// Живая жалоба на КП 003/2026: выбраны две системы, а введение написано про
// одну. Две причины, обе закреплены ниже тестами:
//   - текст кэшировался под ключом, где набора систем не было, поэтому при
//     добавлении второй системы продолжал печататься прежний текст;
//   - в промпте нет раздела «выбраны такие-то системы», и блок с повтором
//     сужал карточки до одной — модель физически не знала о второй.

// Цель теста — не описание карточки, а проверка раздела со списком систем.
func TestPromptListsEveryChosenSystem(t *testing.T) {
	cards, offer := twoSystemCards()
	// Блок с повтором: модель получает карточку ОДНОЙ системы, но о наборе
	// обязана знать — иначе вступление снова напишется про первую.
	prompt := buildBlockUserPromptSelection("Напиши введение", offer, cards,
		offerSelectionHandle(offer), nil)
	for _, title := range []string{"ТН-ФУНДАМЕНТ", "ТН-СТИЛОБАТ"} {
		if !strings.Contains(prompt, title) {
			t.Fatalf("в промпте нет выбранной системы %q:\n%s", title, prompt)
		}
	}
	if !strings.Contains(prompt, "ВЫБРАННЫЕ СИСТЕМЫ В ПРЕДЛОЖЕНИИ (2 шт.)") {
		t.Fatalf("в промпте нет раздела со всеми системами:\n%s", prompt)
	}
	// При двух системах промпт объясняет, что это комплект, а не альтернативы.
	if !strings.Contains(prompt, "комплектном решении") {
		t.Fatalf("в промпте нет правила о комплектной поставке:\n%s", prompt)
	}
}

// При одной системе правила о комплекте нет: лишний абзац толкает модель к
// выдуманному сравнению вариантов.
func TestPromptKeepsSingleSystemSimple(t *testing.T) {
	offer := Offer{Number: "001/2026", Systems: []OfferSystem{{NoteID: 1}}}
	card := SystemCard{NoteID: 1, Title: "ТН-ФУНДАМЕНТ"}
	prompt := buildBlockUserPromptSelection("Напиши введение", offer,
		[]SystemCard{card}, offerSelectionHandle(offer), nil)
	if !strings.Contains(prompt, "ВЫБРАННЫЕ СИСТЕМЫ В ПРЕДЛОЖЕНИИ (1 шт.)") {
		t.Fatalf("нет раздела с системой:\n%s", prompt)
	}
	if strings.Contains(prompt, "комплектном решении") {
		t.Fatalf("при одной системе промпт говорит о комплекте:\n%s", prompt)
	}
}

// Добавление системы меняет ключ текста вступления — кэш старых карточек не
// переживает смену набора. Ключ текста по системам меняется вместе с набором.
func TestSelectionChangesPromptAndTextKeys(t *testing.T) {
	one := offerSelectionHandle(Offer{Systems: []OfferSystem{{NoteID: 1}}})
	two := offerSelectionHandle(Offer{Systems: []OfferSystem{{NoteID: 1}, {NoteID: 2}}})
	if one == "" || two == "" || one == two {
		t.Fatalf("признак набора не различает состав: %q / %q", one, two)
	}
	if layoutPromptKeyCache("Введение", one) == layoutPromptKeyCache("Введение", two) {
		t.Fatal("ключ текста макета не зависит от набора систем — вступление останется про одну систему")
	}
	if genKeyTextByID("intro", 0, one) == genKeyTextByID("intro", 0, two) {
		t.Fatal("ключ текста блока не зависит от набора систем")
	}
	// Повторный выбор того же состава не должен стирать кэш.
	twoAgain := offerSelectionHandle(Offer{Systems: []OfferSystem{{NoteID: 1}, {NoteID: 2}}})
	if two != twoAgain {
		t.Fatalf("один и тот же набор даёт разные признаки: %q / %q", two, twoAgain)
	}
}

// Вступление, написанное при одной системе, не печатается после добавления
// второй: печать читает ключ, записанный для текущего набора.
func TestIntroIsRewrittenWhenSecondSystemAdded(t *testing.T) {
	cards, offer := twoSystemCards()
	oneOffer := offer
	oneOffer.Systems = []OfferSystem{{NoteID: 1}}

	form := OfferForm{
		Mode: "client",
		Blocks: []Block{{
			ID:           "intro",
			ShowInClient: true,
			Content:      []byte(`[{"type":"prompt_text","instruction":"Напиши введение","maxWords":60}]`),
		}},
	}
	c := &fakeCompleter{answer: "Введение про одну систему."}
	before, _, _, _ := generateBlockTexts(context.Background(), form, oneOffer, c, cards, nil)

	// Менеджер добавил вторую систему. Данные предложения (позиции, суммы) те
	// же — хеш `gen.section` не меняется, и старый код взял бы кэш.
	oneOffer.Systems = []OfferSystem{{NoteID: 1}, {NoteID: 2}}
	oneOffer.FormValues = before
	c2 := &fakeCompleter{answer: "Введение про комплект из двух систем."}
	after, reused, written, _ := generateBlockTexts(context.Background(), form, oneOffer, c2, cards, nil)

	if written != 1 {
		t.Fatalf("вступление не переписано при добавлении системы (reused=%d, written=%d)", reused, written)
	}
	key := genKeyTextByID("intro", 0, offerSelectionHandle(oneOffer))
	if got := after[key]; !strings.Contains(got, "комплект") {
		t.Fatalf("напечатался текст прежнего набора: %q", got)
	}
	// Старый ключ остаётся в values (история не стирается), но новый текст —
	// под ключом нового набора.
	if after[key] == before[genKeyTextByID("intro", 0, offerSelectionHandle(oneOffer))] {
		t.Fatal("ключи набора совпали — кэш всё ещё переживает смену состава")
	}
}

// Печать блока внутри повтора знает о наборе: промт несёт перечень всех
// систем, даже когда модель описывает одну карточку.
func TestRepeatedBlockPromptSeesWholeSelection(t *testing.T) {
	cards, offer := twoSystemCards()
	form := OfferForm{
		Mode: "client",
		Blocks: []Block{{
			ID:           "main",
			Repeat:       blockRepeatSystems,
			ShowInClient: true,
			Content:      []byte(`[{"type":"prompt_text","instruction":"Опиши систему","maxWords":60}]`),
		}},
	}
	c := &fakeCompleter{answer: "Описание."}
	generateBlockTexts(context.Background(), form, offer, c, cards, nil)
	if !strings.Contains(c.lastUser, "ВЫБРАННЫЕ СИСТЕМЫ В ПРЕДЛОЖЕНИИ (2 шт.)") {
		t.Fatalf("промт повтора не знает про весь набор систем:\n%s", c.lastUser)
	}
}
