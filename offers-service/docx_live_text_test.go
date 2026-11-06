package main

import (
	"strings"
	"testing"
)

// Сквозная проверка по живому тексту карточки ТН-ФУНДАМЕНТ Дренаж Проф
// (2026-11-05). Именно этот текст попал в КП 003/2026, и именно его увидел
// заказчик: вики-ссылки, слипшиеся слова, «г/м2» обычной цифрой.

const liveCardDescription = `В качестве гидроизоляционного материала применяется неармированная мембрана LOGICBASE V-SL с сигнальным слоем.
Особенности системы:
Скрепление полотен осуществляется при помощи геотекстиля с поверхностной плотностью 500 г/м2.
**Область применения:** Система применяется для защиты подземныхсооружений с эксплуатируемыми помещениями вместных песчаных грунтах с высоким уровнемподземных вод, либо глинистых грунтах независимоот уровня подземных вод.`

const liveCardComposition = `[[Геотекстиль иглопробивной термофиксированный ТЕХНОНИКОЛЬ ПРОФ ФУНДАМЕНТ 500]]`

// В документе нет разметки Obsidian и нет слипшихся слов.
func TestLiveCardTextIsHumanReadable(t *testing.T) {
	got := plainText(liveCardComposition)
	if strings.Contains(got, "[[") || strings.Contains(got, "]]") {
		t.Fatalf("вики-ссылка ушла в документ: %q", got)
	}
	if got != "Геотекстиль иглопробивной термофиксированный ТЕХНОНИКОЛЬ ПРОФ ФУНДАМЕНТ 500" {
		t.Fatalf("состав напечатан неверно: %q", got)
	}
}

// Слипшиеся слова живого документа исправлены, а не перенесены в КП.
func TestLiveCardDescriptionRepaired(t *testing.T) {
	got := plainText(liveCardDescription)
	for _, bad := range []string{"подземныхсооружений", "уровнемподземных", "независимоот", "вместных", "**"} {
		if strings.Contains(got, bad) {
			t.Errorf("в тексте осталось %q:\n%s", bad, got)
		}
	}
	for _, want := range []string{"подземных сооружений", "уровнем подземных", "независимо от", "в местных"} {
		if !strings.Contains(got, want) {
			t.Errorf("не найдено %q", want)
		}
	}
}

// Плотность «500 г/м2» печатается надстрочной двойкой, текст при этом не теряется.
func TestLiveCardDensityIsSuperscript(t *testing.T) {
	runs := splitSupsub(plainText("плотностью 500 г/м2"))
	if len(runs) < 2 {
		t.Fatalf("ожидались отдельные прогоны: %+v", runs)
	}
	last := runs[len(runs)-1]
	if !last.Sup || last.Text != "2" {
		t.Fatalf("двойка не надстрочная: %+v", last)
	}
	if joinRuns(runs) != "плотностью 500 г/м2" {
		t.Fatalf("текст изменился: %q", joinRuns(runs))
	}
}

// В собранном документе описание карточки идёт абзацами, а не одним куском с \n.
func TestDocxCardDescriptionIsParagraphs(t *testing.T) {
	card := sampleCard()
	card.Description = "Первый абзац описания.\n\nВторой абзац описания."
	docx, err := renderOfferDocxWithCards(OfferForm{Title: "Коммерческое предложение"},
		sampleOffer(), []SystemCard{card}, nil)
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	body := string(docxEntry(t, docx, "word/document.xml"))
	if strings.Contains(body, "описания.\n") {
		t.Fatal("перенос строки остался внутри абзаца: Word его не покажет")
	}
	if !strings.Contains(body, "Первый абзац описания.") || !strings.Contains(body, "Второй абзац описания.") {
		t.Fatal("абзацы описания не напечатаны")
	}
}

// Вики-ссылка не попадает в собранный DOCX даже если пришла в составе.
func TestDocxCompositionHasNoWikiMarkup(t *testing.T) {
	card := sampleCard()
	card.Composition = []string{"[[Мембрана V-SL]]", "[[Лента ПВХ]]"}
	docx, err := renderOfferDocxWithCards(OfferForm{Title: "Коммерческое предложение"},
		sampleOffer(), []SystemCard{card}, nil)
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	body := string(docxEntry(t, docx, "word/document.xml"))
	if strings.Contains(body, "[[") || strings.Contains(body, "]]") {
		t.Fatal("разметка вики-ссылок попала в документ")
	}
	if !strings.Contains(body, "Мембрана V-SL") {
		t.Fatal("название материала потерялось вместе со скобками")
	}
}
