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
func combatSectionBrief(instruction string, ourCards, competitorCards []SystemCard, hint string,
	facts []combatFact, basePrompt string) string {
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
	// Уточнение менеджера — акцент разбора (поле `hint` листка). Оно уже входит
	// в отпечаток содержания (`combatSectionHandle`) и в хэш DOCX, но в бриф его
	// раньше не клали: пока разделы на стенде не печатались, это было не видно,
	// а живой листок писал бы разбор БЕЗ учёта уточнения — против прямого
	// требования пользователя («акцент на…»). Модель обязана его видеть, иначе
	// смена уточнения пересобирает файл, но текст остаётся прежним.
	if trimmed := strings.TrimSpace(hint); trimmed != "" {
		b.WriteString("\nУточнение менеджера (чему уделить внимание): " + trimmed + ".")
	}
	// Добытые факты (шаг 3e) — после ролей и до контракта ответа: модель сперва
	// понимает, чей продукт, потом получает сведения, потом формат.
	b.WriteString(combatFactsPrompt(facts))
	b.WriteString("\n\n")
	// Базовый промт — формы листка (2026-11-17), а не константа: правила
	// формулировок и согласованности разделов администратор правит без выпуска.
	b.WriteString(effectiveBasePrompt(basePrompt))
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
	refs []skills.CardRef, basePrompt string) (*sectionGen, Offer) {
	offer := combatSectionOffer(sheet, ourCards, competitorCards)
	cards := combatSectionCards(ourCards, competitorCards)
	facts := combatFactsFromValues(sheet.FormValues)
	// content — ОДНА переменная на два поля: section (свежесть всей карты) и
	// selection (признак набора). Разные отпечатки в них развели бы два
	// условия свежести: карта хранится под section, а ключ подразумевает
	// selection, и правка карточки терялась бы в кэше (грабля 2026-11-08).
	content := combatSectionHandle(sheet, ourCards, competitorCards, facts, basePrompt)
	return &sectionGen{
		values:     sectionValuesFor(offer),
		previous:   sheet.FormValues,
		section:    content,
		selection:  combatSelectionHandle(sheet, ourCards, competitorCards, content, facts),
		offer:      offer,
		cards:      cards,
		refs:       refs,
		basePrompt: basePrompt,
		brief: func(instruction string, call sectionCall) string {
			return combatSectionBrief(instruction, ourCards, competitorCards, combatHint(sheet), facts, basePrompt)
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
//
// С шага 3e сюда же входит отпечаток добытых фактов (`combatFactHandle`): они
// тоже содержание листка, и правка факта обязана переписать раздел, иначе текст
// описывал бы документы, которые с тех пор изменились.
func combatSectionHandle(sheet CombatSheet, ourCards, competitorCards []SystemCard,
	facts []combatFact, basePrompt string) string {
	var b strings.Builder
	b.WriteString("combat\x1f" + sheet.Number)
	b.WriteString("\x1fhint\x1f" + strings.TrimSpace(combatHint(sheet)))
	// Базовый промт входит в отпечаток содержания (2026-11-17): правка текста в
	// форме меняет то, что модель напишет, и обязана переписать разделы.
	if trimmed := strings.TrimSpace(basePrompt); trimmed != "" && trimmed != sectionAnswerContract {
		b.WriteString("\x1fbase\x1f" + trimmed)
	}
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
	if handle := combatFactHandle(facts); handle != "" {
		b.WriteString("\x1ffacts\x1f" + handle)
	}
	return hashShort(b.String())
}

// combatSelectionHandle — признак набора продуктов для ключа кэша.
//
// Тот же смысл, что `offerSelectionHandle` у КП: текст без повтора видит ВЕСЬ
// набор и обязан пересобраться, когда набор поменялся (дефект вступления
// 2026-11-08).
//
// `content` входит в признак НАМЕРЕННО. `sectionGen` сверяет свежесть карты
// через `previous[genKeySection] == g.section`, то есть отпечатком содержания, а
// ключ строит по `selection`. Пока `selection` знал только состав, правка тела
// карточки оставляла ключ прежним — и листок отдавал текст по старой карточке
// ровно та грабля 2026-11-08. Поэтому здесь и состав, и отпечаток содержания.
//
// Факты (шаг 3e) — в признаке набора, а не только в отпечатке содержания:
// `combatSectionHandle` входит в `content` целиком, так что правка факта
// меняет и его. Явная строка ниже — следующая за `content`, чтобы человек,
// читающий ключ, видел факты отдельной частью, как и состав сторон.
func combatSelectionHandle(sheet CombatSheet, ourCards, competitorCards []SystemCard,
	content string, facts []combatFact) string {
	var b strings.Builder
	for _, cards := range [][]SystemCard{ourCards, competitorCards} {
		for _, card := range cards {
			b.WriteString(itoa(int(card.NoteID)) + ":" + card.Title + "\x1f")
		}
		b.WriteString("\x1e")
	}
	b.WriteString(strings.TrimSpace(combatHint(sheet)))
	b.WriteString("\x1fcontent\x1f" + content)
	if handle := combatFactHandle(facts); handle != "" {
		b.WriteString("\x1ffacts\x1f" + handle)
	}
	return hashShort(b.String())
}

// renderCombatSections печатает содержательные разделы листка.
//
// Возвращает текст и признак «хоть что-то напечатано»: пустой результат обязывает
// вызывающего напечатать прежний вид (карточки), иначе документ останется без
// сравнения вовсе.
//
// Кэш (шаг 2b плана 2026-11-16). Тексты пишутся в `form_values` САМОГО листка
// под ключами `gen.sect.*` — тем же механизмом, что у КП (`sectionGen`,
// `sectionCacheKey`). В ключе — отпечаток СОДЕРЖИМОГО (`combatSectionHandle`:
// стороны + уточнение + текст карточек) и признак набора
// (`combatSelectionHandle`), поэтому правка карточки конкурента переписывает
// текст, а повторное «⬇ DOCX» без изменений берёт его из базы и модель второй
// раз не зовёт.
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
		// Строка формы может существовать, но разделов в ней не быть (сохранена
		// до 2026-11-16 или очищена администратором). Пустой листок печатал бы
		// только карточки, а заготовка — это и есть «разделы листка по
		// умолчанию»; поэтому пусто — берём заготовку, а не молчим.
		form.Sections = combatDefaultSections()
	}
	gen, offer := combatSectionGen(sheet, ourCards, competitorCards, refs, form.BasePrompt)
	cards := combatSectionCards(ourCards, competitorCards)
	lc := combatLayoutFor(offer, cards, refs, images, links, form)
	// Подписи схем — сквозные по документу (2026-11-17, шаг 5): один счётчик на
	// всю печать листка, а не с единицы в каждом разделе.
	lc.sectionNumbering = &sectionDiagramCounter{}

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
	// Тексты, которых в листке ещё не было, сохраняются, чтобы следующая сборка
	// взяла их же. Только при живом листке: копия с чужим id (тесты,
	// предпросмотр) не должна порождать записи.
	if sheet.ID > 0 {
		s.persistCombatSectionTexts(ctx, sheet, gen)
	}
	if printed.Len() == 0 {
		return "", false
	}
	return printed.String(), true
}

// persistCombatSectionTexts сохраняет НОВЫЕ тексты разделов в form_values листка.
//
// Одно правило с КП (`persistLayoutTextCache`): только `form_values`, только
// изменившееся (`mergeSectionValues`), стороны и название не трогаются. Ошибка
// записи не мешает отдать документ: кэш — ускорение, а не повод отказать; текст
// будет написан заново на следующей сборке.
func (s *Server) persistCombatSectionTexts(ctx context.Context, sheet CombatSheet, gen *sectionGen) {
	if s == nil || s.pool == nil || gen == nil {
		return
	}
	gen.mu.Lock()
	generated := make(map[string]string, len(gen.values))
	for key, value := range gen.values {
		generated[key] = value
	}
	gen.mu.Unlock()

	merged := make(map[string]string, len(sheet.FormValues)+len(generated))
	for key, value := range sheet.FormValues {
		merged[key] = value
	}
	mergeSectionValues(merged, sheet.FormValues, generated)
	// Отпечаток содержания пишется вместе с текстами. Без него `sectionGen`
	// на следующей сборке не подтвердит свежесть (`previous[genKeySection] !=
	// section`) и МОЛЧА перепишет листок моделью заново — кэш был бы, но на
	// деле не работал бы.
	merged[genKeySection] = gen.section
	if len(merged) == len(sheet.FormValues) && sheet.FormValues[genKeySection] == gen.section {
		return
	}
	sheet.FormValues = merged
	cols, err := combatColumns(sheet)
	if err != nil {
		log.Printf("боевой листок %s: тексты разделов не закодированы: %v", sheet.Number, err)
		return
	}
	if err := s.updateCombat(ctx, sheet.ID, sheet, cols); err != nil {
		log.Printf("боевой листок %s: тексты разделов не сохранены: %v", sheet.Number, err)
	}
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
