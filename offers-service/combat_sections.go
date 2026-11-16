package main

// Боевой листок на разделах (2026-11-16, шаг 2a плана
// docs/superpowers/plans/2026-11-16-sbe-offers-combat-sections-and-agent-tools-plan.md).
//
// Модель, прежде чем код. До этого шага листок печатал ДВЕ группы карточек и
// один общий текст сравнения (`renderCombatComparison`). Это не настраивалось и
// не покрывало того, что нужно менеджеру: таблица характеристик, честный ответ
// «где конкурент сильнее», вопросы по объекту.
//
// Теперь у листка те же содержательные РАЗДЕЛЫ, что у КП: администратор задаёт
// подразделы, условия, бриф и повторы. Печать переиспользует конвейер разделов
// (`sections.go` / `sections_print.go`), а не второй такой же.
//
// Что остаётся от прежнего вида:
//   - заголовок листка и перечень продуктов («кто с кем сравнивается»);
//   - если разделов нет или ВСЕ пусты — общий текст сравнения и карточки
//     (`writeCombatCards`): документ никогда не бывает пустым.
//
// Данные модели для подраздела — карточки ОБЕИХ сторон. В отличие от КП, где
// все карточки «наши», у листка это сравнение: сторона называется в брифе и
// стоит в самой карточке, чтобы модель не путала, чей продукт описывает.

import (
	"context"
	"log"
	"strings"

	"mailers/offers/skills"
)

// combatSectionOffer — листок в виде «предложения» для конвейера разделов.
//
// Отличие от `combatAsOffer` одно: в `Systems` лежат карточки ОБЕИХ сторон.
// Без этого `sectionCalls` с `repeat:"systems"` не нашёл бы ни одной системы —
// у листка `Systems` пуст, — и повторяемый подраздел молчал бы.
func combatSectionOffer(sheet CombatSheet, ourCards, competitorCards []SystemCard) Offer {
	offer := combatAsOffer(sheet)
	systems := make(OfferSystems, 0, len(ourCards)+len(competitorCards))
	for _, card := range combatSectionCards(ourCards, competitorCards) {
		systems = systems.Add(Ref{SKU: card.SKU, NoteID: card.NoteID})
	}
	offer.Systems = systems
	return offer
}

// combatSectionCards — карточки обеих сторон одним списком.
//
// Дублей по заметке нет: продукт может стоять на обеих сторонах (менеджер
// сравнивает систему сама с собой), но модели он нужен один раз.
func combatSectionCards(ourCards, competitorCards []SystemCard) []SystemCard {
	seen := make(map[int64]bool, len(ourCards)+len(competitorCards))
	out := make([]SystemCard, 0, len(ourCards)+len(competitorCards))
	for _, card := range append(append([]SystemCard{}, ourCards...), competitorCards...) {
		if card.NoteID > 0 {
			if seen[card.NoteID] {
				continue
			}
			seen[card.NoteID] = true
		}
		out = append(out, card)
	}
	return out
}

// combatSectionBrief — бриф подраздела листка: инструкция администратора, роли
// сторон и замороженный контракт ответа (тот же, что у КП).
//
// Роли сторон — в тексте брифа, а не отдельным полем схемы: у КП все карточки
// свои, у листка это сравнение, и модель обязана знать, чей продукт описывает.
// Люди это видят в готовом тексте («где конкурент сильнее»), поэтому путаница
// сторон — не стилистическая мелочь.
func combatSectionBrief(instruction string, ourCards, competitorCards []SystemCard) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(instruction))
	b.WriteString("\n\nЭто боевой листок: сравнение НАШЕГО продукта с продуктами конкурентов.")
	if names := cardNames(ourCards); names != "" {
		b.WriteString("\nНаш продукт: " + names + ".")
	}
	if names := cardNames(competitorCards); names != "" {
		b.WriteString("\nПродукты конкурентов: " + names + ".")
	}
	b.WriteString("\nНе путай стороны: наш продукт — тот, что предлагаем мы, конкуренты — чужие.")
	b.WriteString("\n\n")
	b.WriteString(sectionAnswerContract)
	return b.String()
}

// cardNames — названия карточек через запятую (строка брифа).
func cardNames(cards []SystemCard) string {
	names := make([]string, 0, len(cards))
	for _, card := range cards {
		if title := strings.TrimSpace(card.Title); title != "" {
			names = append(names, title)
		}
	}
	return strings.Join(names, ", ")
}

// combatSectionGen — генератор текстов разделов листка.
//
// Кэш, повтор, разбор ответа — те же функции, что у КП (`sectionCacheKey`,
// `parseSectionAnswer`), поэтому сторожа конвейера разделов действуют и здесь.
// Отличается только бриф (`brief`).
func combatSectionGen(sheet CombatSheet, ourCards, competitorCards []SystemCard,
	refs []skills.CardRef) (*sectionGen, Offer) {
	offer := combatSectionOffer(sheet, ourCards, competitorCards)
	cards := combatSectionCards(ourCards, competitorCards)
	return &sectionGen{
		values:    sectionValuesFor(offer),
		previous:  sheet.FormValues,
		section:   combatSectionHandle(sheet, ourCards, competitorCards),
		selection: combatSelectionHandle(sheet, ourCards, competitorCards),
		offer:     offer,
		cards:     cards,
		refs:      refs,
		brief: func(instruction string, call sectionCall) string {
			return combatSectionBrief(instruction, ourCards, competitorCards)
		},
		complete: llmCompleter{},
	}, offer
}

// combatSectionHandle — отпечаток СОДЕРЖИМОГО листка.
//
// У КП это `hashOfferFacts` (номер, заказчик, поля объекта). У листка заказчика
// нет, а содержание — стороны, уточнение и сами карточки: правка карточки
// конкурента ОБЯЗАНА переписать текст раздела (грабля 2026-11-08: кэш ключевался
// одним noteId, правка тела карточки не доходила до текста). Поэтому в
// отпечаток идут и названия, и текст карточек.
func combatSectionHandle(sheet CombatSheet, ourCards, competitorCards []SystemCard) string {
	var b strings.Builder
	b.WriteString("combat\x1f" + sheet.Number)
	b.WriteString("\x1fhint\x1f" + strings.TrimSpace(combatHint(sheet)))
	for _, side := range []struct {
		name  string
		cards []SystemCard
	}{{"our", ourCards}, {"competitor", competitorCards}} {
		b.WriteString("\x1f" + side.name + "\x1f")
		for _, card := range side.cards {
			b.WriteString(itoa(int(card.NoteID)) + ":" + card.Title + "\x1f")
			b.WriteString(card.PromptText() + "\x1f")
		}
	}
	return hashShort(b.String())
}

// combatSelectionHandle — признак набора продуктов для ключа кэша.
//
// Тот же смысл, что `offerSelectionHandle` у КП: текст без повтора видит ВЕСЬ
// набор и обязан пересобраться, когда набор поменялся (дефект вступления
// 2026-11-08). Текст карточек уже входит в отпечаток содержания; здесь — состав
// и порядок сторон.
func combatSelectionHandle(sheet CombatSheet, ourCards, competitorCards []SystemCard) string {
	var b strings.Builder
	for _, cards := range [][]SystemCard{ourCards, competitorCards} {
		for _, card := range cards {
			b.WriteString(itoa(int(card.NoteID)) + ":" + card.Title + "\x1f")
		}
		b.WriteString("\x1e")
	}
	b.WriteString(strings.TrimSpace(combatHint(sheet)))
	return hashShort(b.String())
}

// renderCombatSections печатает содержательные разделы листка.
//
// Возвращает текст и признак «хоть что-то напечатано»: пустой результат обязывает
// вызывающего напечатать прежний вид (карточки), иначе документ останется без
// сравнения вовсе.
//
// Ошибка чтения формы и ошибка модели не роняют печать: листок — справка, и
// пустой раздел хуже, чем честно карточный запасной вид. Ошибка в журнал.
func (s *Server) renderCombatSections(ctx context.Context, sheet CombatSheet,
	ourCards, competitorCards []SystemCard, refs []skills.CardRef,
	images ImageSource, media *docxMedia, links *layoutRuntime) (string, bool) {
	// Сервера без базы не бывает в жизни, но бывает в тестах: разделы читаются
	// из формы, а формы без базы нет — честный запасной вид, без паники.
	if s == nil || s.pool == nil {
		return "", false
	}
	form, err := s.loadForm(ctx, combatFormMode)
	if err != nil {
		log.Printf("боевой листок %s: форма листка не прочитана, разделы не печатаются: %v", sheet.Number, err)
		return "", false
	}
	if len(form.Sections) == 0 {
		return "", false
	}
	gen, offer := combatSectionGen(sheet, ourCards, competitorCards, refs)
	cards := combatSectionCards(ourCards, competitorCards)
	lc := combatLayoutFor(offer, cards, refs, images, links, form)

	var printed strings.Builder
	for _, section := range form.Sections {
		calls := sectionCalls(section, offer, cards)
		if len(calls) == 0 {
			continue
		}
		nums := sectionNumbering{plan: lc.numberingPlan()}
		var markup strings.Builder
		any := false
		for _, call := range calls {
			text := gen.text(section, call)
			if strings.TrimSpace(text) == "" {
				continue
			}
			markup.WriteString(renderSectionSubsection(call.sub, text, lc, nums))
			any = true
		}
		if !any {
			// Все подразделы пусты — раздел молчит целиком, включая заголовок
			// (§10.1: пустое не печатается).
			continue
		}
		printed.WriteString(combatSectionTitle(section))
		printed.WriteString(markup.String())
	}
	if printed.Len() == 0 {
		return "", false
	}
	return printed.String(), true
}

// combatSectionTitle — заголовок раздела листка.
//
// Регистрации в оглавлении нет намеренно: у листка нет оглавления, а его макет —
// вставка на месте метки `{{сравнение}}`. Закладка в никуда только засоряет
// пакет.
func combatSectionTitle(section ContentSection) string {
	title := strings.TrimSpace(section.Title)
	if title == "" {
		return ""
	}
	prefix := ""
	if section.Numbered {
		// Нумерация разделов — часть макета КП; у макета листка она есть не
		// всегда, поэтому номер ставим только когда администратор его включил.
		prefix = ""
	}
	return headingXML(prefix+title, 2)
}

// combatLayoutFor — layoutContext для печати подраздела листка.
//
// Тот же вид, что у печати КП (`renderContentSection`), но с данными листка:
// значения листка, ссылки и карточки обеих сторон, нумерация макета. Форма
// передаётся, чтобы `figure` разрешался по настройкам той же формы.
func combatLayoutFor(offer Offer, cards []SystemCard, refs []skills.CardRef,
	images ImageSource, links *layoutRuntime, form OfferForm) layoutContext {
	return layoutContext{
		values:    offer.FormValues,
		service:   serviceValuesWithCards(offer, cards),
		offer:     offer,
		refs:      refs,
		cards:     func() []SystemCard { return cards },
		images:    images,
		layout:    links,
		form:      func() OfferForm { return form },
		selection: offerSelectionHandle(offer),
	}
}
