package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// Содержательные разделы КП (2026-11-14).
//
// Метка `{ content: <id> }` называет настройку раздела: какие подразделы
// писать, при каком условии и о чём. Текст подраздел пишет модель — по общему
// конвейеру макета, — а оформление берётся из Word-макета: шрифты, заголовки,
// номера и подписи задаёт `temple.docx`, модель отдаёт только текст и разметку.
//
// Шаг 1: метка разбирается и печатает ПУСТО, если настройки нет.
// Шаг 3: условия (conditionHolds, offerFacts).
// Шаг 4 (этот файл): веер вызовов. Подразделы пишутся ОДНИМ вызовом модели
// каждый, параллельно и с ограничением, а не одним вызовом на весь раздел.
// Причина измерена (0.9.25): один вызов на кусок текста — это десятки секунд
// тишины, а обрыв одного вызова теряет всю работу. Сборка рамочной печати —
// шаг 5.

// sectionGenConcurrency — сколько вызовов модели делает раздел одновременно.
//
// Именованная константа, а не литерал: значение измеряется на живом
// провайдере и подстраивается здесь. Четыре — стартовое значение: оно даёт
// выигрыш независимых вызовов (~4×, как у writeBlocksInParallel) и не
// заливает провайдера, когда у раздела три системы по четыре подраздела.
const sectionGenConcurrency = 4

// sectionCall — один вызов модели: подраздел и (для repeat:"systems") система.
type sectionCall struct {
	// sub — подраздел, чью инструкцию получит модель.
	sub ContentSubsection
	// system — карточка прохода; nil — повтор не по системам.
	system *SystemCard
}

// cacheKey — ключ кэша ТЕКСТА этого вызова (шаг 5).
//
// Один подраздел — один ключ: правка инструкции соседнего подраздела не
// выбрасывает текст этого. Номер системы входит в ключ, иначе вторая система
// брала бы текст первой — дефект, уже измеренный у текстов блоков (2026-11-08).
func (c sectionCall) cacheKey(sectionID string) string {
	key := "gen.sect." + sectionID + "." + c.sub.ID
	if c.system != nil {
		key += ".sys" + systemCacheHandle(*c.system)
	}
	return key
}

// sectionCalls — какие вызовы делает раздел, в порядке печати.
//
// Пропуск делается ДО планирования: подраздел с невыполненным условием не
// попадает в список вовсе, поэтому для него не пишется ни текст, ни заголовок
// (правило §10.1: пустое не печатается). Пустой список — раздел не печатается
// целиком, включая свой заголовок.
func sectionCalls(section ContentSection, offer Offer, cards []SystemCard) []sectionCall {
	facts := offerFacts(offer)
	var calls []sectionCall
	for _, sub := range section.Subsections {
		if strings.TrimSpace(sub.ID) == "" || strings.TrimSpace(sub.Instruction) == "" {
			continue
		}
		if !conditionHolds(sub.ConditionOrAlways(), facts, section) {
			continue
		}
		if strings.TrimSpace(sub.Repeat) == blockRepeatSystems {
			systems := cardsForOffer(cards, offer)
			for i := range systems {
				calls = append(calls, sectionCall{sub: sub, system: &systems[i]})
			}
			continue
		}
		calls = append(calls, sectionCall{sub: sub})
	}
	return calls
}

// runSectionCalls выполняет вызовы ограниченным пулом и возвращает ответы по
// тем же индексам, что вход.
//
// Ошибка одного вызова не отменяет остальные: пустая строка на его месте, и
// рамка печатает остальное. Это не снисходительность — модель отвечает не
// всегда, а терять из-за этого весь раздел документ не должен (та же деградация,
// что у текстов блоков).
func runSectionCalls(ctx context.Context, calls []sectionCall,
	write func(ctx context.Context, call sectionCall) string) []string {
	out := make([]string, len(calls))
	if len(calls) == 0 {
		return out
	}
	limit := sectionGenConcurrency
	if limit <= 0 {
		limit = 1
	}
	slots := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(i int, call sectionCall) {
			defer wg.Done()
			// Пул, а не пул воркеров: горутина на вызов дешёвая, а порядок
			// ответов задаёт индекс, и он не зависит от того, кто пришёл первым.
			slots <- struct{}{}
			defer func() { <-slots }()
			out[i] = write(ctx, call)
		}(i, call)
	}
	wg.Wait()
	return out
}

// renderContentSection печатает содержательный раздел по его настройке.
//
// Настройки раздела берутся из формы КП (`OfferForm.Sections`). Неизвестный id
// печатает ПУСТО и пишет в журнал: литерал `{ content: … }` в готовом документе
// недопустим, а молчаливая подмена чужого раздела хуже пустого места.
func renderContentSection(sectionID string, lc layoutContext, row layoutServiceRow) string {
	_ = row
	id := strings.TrimSpace(sectionID)
	if id == "" {
		log.Printf("макет: у содержательного раздела нет имени — метка молчит")
		return ""
	}
	if lc.form == nil {
		log.Printf("макет: раздел %q не найден — формы нет, метка молчит", id)
		return ""
	}
	section, ok := findContentSection(lc.form().Sections, id)
	if !ok {
		log.Printf("макет: содержательный раздел %q не найден в форме — метка молчит", id)
		return ""
	}
	calls := sectionCalls(section, lc.offer, lc.cardList())
	if len(calls) == 0 {
		// Все подразделы пропущены — раздел молчит целиком, без своего
		// заголовка: заголовок без содержимого — та же поломка на уровень выше.
		log.Printf("макет: раздел %q не печатается — все подразделы пропущены", id)
		return ""
	}
	if lc.sectionText == nil {
		// Печати без модели пока нет (шаг 5): тексты подразделов не написаны.
		return ""
	}
	started := time.Now()
	texts := runSectionCalls(context.Background(), calls, func(_ context.Context, call sectionCall) string {
		return lc.sectionText(section, call)
	})
	// Рамка: заголовок раздела (номер даёт нумерация макета) и, по порядку,
	// только те подразделы, у которых есть текст. Заголовок подраздела печатает
	// рамка, а не модель.
	nums := sectionNumbering{plan: lc.numberingPlan()}
	numberTitles := section.Numbered
	// Списки заводятся ДО печати: подраздел обязан ссылаться на список своего
	// раздела, а порядок ссылок не зависит от того, какой подраздел что печатал.
	titleList, subsectionLists := nums.listsFor(numberTitles)
	// Порядок регистрации заголовков в оглавлении — порядок ДОКУМЕНТА
	// (2026-11-14). Заголовок раздела печатается первым, поэтому и закладка
	// заводится первой: иначе пересборка оглавления шла бы в порядке вызовов,
	// и «1. Раздел» стоял бы после «1.1 Введение» — пункты в неверном порядке.
	titleMarkup := sectionTitleMarkup(section, lc, titleList)
	var sb strings.Builder
	var printed []string
	for i, call := range calls {
		if i >= len(texts) || strings.TrimSpace(texts[i]) == "" {
			continue
		}
		printed = append(printed, renderSectionSubsection(call.sub, texts[i], lc, subsectionLists))
	}
	// renderSectionSubsection сама отбрасывает пустое, поэтому проверка идёт
	// по собранной разметке: подраздел без блоков не печатает ни заголовка, ни
	// пробела.
	printed = nonEmptyStrings(printed)
	if len(printed) == 0 {
		// Ни один подраздел не написался — раздел молчит целиком: заголовок
		// без содержимого — та же поломка на уровень выше. Заголовок в этом
		// случае НЕ печатается, поэтому и закладку для него не оставляем.
		if titleMarkup != "" {
			lc.dropTOCHeading()
		}
		log.Printf("макет: раздел %q не напечатан — текстов подразделов нет", id)
		return ""
	}
	sb.WriteString(titleMarkup)
	sb.WriteString(strings.Join(printed, ""))
	if lc.report != nil {
		lc.report.addGenerated()
	}
	logModelCall(fmt.Sprintf("sect:%s", id), fmt.Sprintf("calls=%d", len(calls)), 0, started, nil)
	return sb.String()
}

// sectionTitleMarkup — заголовок самого раздела.
//
// Номер раздел не пишет в текст: его даёт нумерация макета (пункт «Numbers come
// from the master's multilevel list»). Заголовок печатается всегда, когда
// напечатано хоть что-то: раздел без названия в документе не читается.
func sectionTitleMarkup(section ContentSection, lc layoutContext, list *numberingList) string {
	title := strings.TrimSpace(section.Title)
	if title == "" {
		return ""
	}
	return lc.tocHeadingXML(title, 1, paraOpts{Numbering: list, NoIndent: true})
}

// nonEmptyStrings — непустые ответы в порядке вызовов.
func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}

// findContentSection ищет раздел по id.
func findContentSection(sections []ContentSection, id string) (ContentSection, bool) {
	id = strings.TrimSpace(id)
	for _, section := range sections {
		if strings.TrimSpace(section.ID) == id {
			return section, true
		}
	}
	return ContentSection{}, false
}

// sectionOfferFacts — что предложение знает о себе для условий подразделов.
//
// Материалы берутся из выбора, а не из карточек: печать может пройти без
// сервера карточек (тесты, повтор), а «материал выбран» — факт самого
// предложения. Система с пустым выбором, но заполненным кодом — тоже выбор:
// Ref.Empty игнорирует строки, которые считают код.
type sectionFacts struct {
	systems   int
	materials int
	items     int
}

// offerFacts — факты предложения для условий.
func offerFacts(offer Offer) sectionFacts {
	facts := sectionFacts{items: len(offer.Items)}
	for _, ref := range offer.Systems {
		if !ref.Empty() {
			facts.systems++
		}
	}
	for _, ref := range offer.Materials {
		if !ref.Empty() {
			facts.materials++
		}
	}
	return facts
}

// conditionHolds решает, печатается ли подраздел (2026-11-14).
//
// Функция чистая: ни сети, ни БД, ни модели. Единственное условие, которое
// читает РАЗДЕЛ, а не предложение, — `has_critique` (переключатель `critique`
// на разделе); остальные смотрят только на предложение. Так подпись
// `(facts, section)` не разъезжается: где кончается предложение, видно по типу.
func conditionHolds(condition SubsectionCondition, facts sectionFacts, section ContentSection) bool {
	switch condition {
	case "", conditionAlways:
		return true
	case conditionHasSystems:
		return facts.systems >= 1
	case conditionHasMaterials:
		return facts.materials >= 1
	case conditionMultiSystem:
		return facts.systems > 1
	case conditionHasCritique:
		return section.Critique
	case conditionHasItems:
		return facts.items >= 1
	}
	// Неизвестное условие не печатает: validateForm такое не сохраняет, но
	// старые данные могли прийти без проверки, и напечатать «на всякий случай»
	// значило бы напечатать раздел, который администратор выключил.
	return false
}
