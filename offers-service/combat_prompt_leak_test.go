package main

import (
	"strings"
	"testing"
)

// Боевой листок не должен получать данные КП (2026-11-19, живой листок
// 017/2026).
//
// Жалоба пользователя: в готовом листке модель писала «В предложении три
// системы … идут комплектом, каждая на свой участок объекта», хотя в листке нет
// ни предложения, ни комплекта — есть наш продукт и два конкурента. Причина не в
// разделе и не в макете: подраздел листка получал USER-промт КП
// (`buildBlockUserPromptSelection`), а в нём — «ДАННЫЕ ПРЕДЛОЖЕНИЯ»,
// «ВЫБРАННЫЕ СИСТЕМЫ В ПРЕДЛОЖЕНИИ (3 шт.)» и правило комплекта из
// `renderSystemsForPrompt` (оно включается при двух и более системах, а
// `combatSectionOffer` кладёт в `Systems` карточки ОБЕИХ сторон).
//
// Сторожа держат три вещи: у листка свой текст данных, у КП он прежний, а
// встроенный базовый контракт листка не говорит про КП.

func leakOurCard() SystemCard {
	return SystemCard{
		NoteID:      6363,
		Title:       "Теплоизоляционные плиты LOGICPIR® PROF Ф/Ф",
		Description: "Плиты из жёсткого пенополиизоцианурата для плоских кровель.",
	}
}

func leakCompetitorCards() []SystemCard {
	return []SystemCard{
		{NoteID: 6654, Title: "PIRRO®Membrane", Description: "PIR-плита с фольгой."},
		{NoteID: 6655, Title: "PIRRO®Bitum", Description: "PIR-плита под наплавляемую гидроизоляцию."},
	}
}

func TestCombatSectionPromptDropsOfferFacts(t *testing.T) {
	sheet := CombatSheet{Number: "017/2026"}
	prompt := combatSectionUserPrompt(sheet, []SystemCard{leakOurCard()}, leakCompetitorCards(), nil)

	lower := strings.ToLower(prompt)
	// Слова, которых в листке быть не должно: это лексикон КП, а не сравнения.
	for _, bad := range []string{"предложени", "комплект", "участок объекта", "данные предложения"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("в промте листка остался текст КП (%q):\n%s", bad, prompt)
		}
	}
	// Стороны обязаны называться, иначе модель снова перепутает, чей продукт.
	for _, want := range []string{
		"БОЕВОГО ЛИСТКА",
		"LOGICPIR",
		"PIRRO®Membrane",
		"PIRRO®Bitum",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("в промте листка нет %q:\n%s", want, prompt)
		}
	}
	if !strings.Contains(prompt, "017/2026") {
		t.Fatalf("номер листка потерялся:\n%s", prompt)
	}
}

// Обратная сторона: у КП правило комплекта осталось. Оно там верное — менеджер
// действительно предлагает несколько систем на разные участки, — и «починить»
// его заодно значило бы сломать КП.
func TestOfferSectionPromptKeepsItsWording(t *testing.T) {
	offer := Offer{Number: "001/2026", Systems: OfferSystems{{NoteID: 1}, {NoteID: 2}}}
	cards := []SystemCard{{NoteID: 1, Title: "Система А"}, {NoteID: 2, Title: "Система Б"}}

	prompt := buildBlockUserPromptSelection("Напиши вступление", offer, cards, cards, "", nil)
	if !strings.Contains(prompt, "ВЫБРАННЫЕ СИСТЕМЫ В ПРЕДЛОЖЕНИИ") {
		t.Fatalf("у КП пропал раздел о выбранных системах:\n%s", prompt)
	}
	if !strings.Contains(prompt, "свой участок объекта") {
		t.Fatalf("у КП пропало правило комплекта:\n%s", prompt)
	}
}

// Встроенный контракт листка не говорит про КП: фигуры ссылаются на документ, а
// не на коммерческое предложение. У КП текст остался прежним.
func TestCombatBasePromptHasNoOfferWording(t *testing.T) {
	if got := effectiveCombatBasePrompt(""); strings.TrimSpace(got) == "" {
		t.Fatal("у листка нет встроенного базового контракта")
	}
	if got := effectiveCombatBasePrompt(""); strings.Contains(got, "КП") {
		t.Fatalf("встроенный контракт листка говорит про КП:\n%s", got)
	}
	if got := effectiveCombatBasePrompt("свой текст"); got != "свой текст" {
		t.Fatalf("свой промт листка подменён встроенным: %q", got)
	}
	if got := defaultForm(clientFormMode).BasePromptDefault; !strings.Contains(got, "из КП") {
		t.Fatalf("у КП изменился встроенный контракт: %q", got)
	}
	if got := defaultForm(combatFormMode).BasePromptDefault; strings.Contains(got, "КП") {
		t.Fatalf("заготовка формы листка отдаёт редактору контракт КП:\n%s", got)
	}
}

// Данные листка для модели: две стороны, без цен и без брифа КП.
func TestCombatSectionFactsNameBothSides(t *testing.T) {
	sheet := CombatSheet{Number: "017/2026", Title: "Сравнение для кровли"}
	facts := combatSectionFacts(sheet, []SystemCard{leakOurCard()}, leakCompetitorCards())

	if !strings.Contains(facts, "НАШ ПРОДУКТ") || !strings.Contains(facts, "КОНКУРЕНТ") {
		t.Fatalf("стороны не названы:\n%s", facts)
	}
	if strings.Contains(facts, "Цена") || strings.Contains(facts, "Сумма") {
		t.Fatalf("в данные листка попали цены КП:\n%s", facts)
	}
	if strings.Contains(facts, "комплект") || strings.Contains(facts, "участок") {
		t.Fatalf("в данные листка попало правило комплекта:\n%s", facts)
	}
	if !strings.Contains(facts, "Сравнение для кровли") {
		t.Fatalf("название листка потерялось:\n%s", facts)
	}
}
