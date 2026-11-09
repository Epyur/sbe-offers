package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"mailers/offers/model"
	"mailers/offers/skills"
)

// Текст блока по промту (2026-11-01).
//
// Администратор в форме пишет подсказку («опиши систему и материалы, входящие в
// неё»), а текст в документе сочиняет модель по данным конкретного КП: объект,
// адресат, площадь, системы, состав, суммы.
//
// Три правила, ради которых это отдельный файл:
//  1. Модель недоступна — блок просто молчит. Документ важнее текста (та же
//     деградация, что у секций, 2026-10-29).
//  2. Написанное кэшируется в form_values и печатается при следующей сборке.
//     Без кэша один и тот же КП давал бы разный текст при каждом скачивании, а
//     заказчик видит разницу между двумя файлами.
//  3. Кэш привязан к хешу данных предложения: правка состава или суммы
//     стирает старый текст, иначе в документе останутся «старые» цифры.
//
// Ключи кэша: `gen.section` — хеш данных, `gen.<блок>.<узел>` — текст.

const (
	// genKeySection — хеш данных, под которые был написан весь текст блоков.
	genKeySection = "gen.section"
	// genKeySystems — набор выбранных систем на момент последней сборки
	// (2026-11-08). Хеш данных систем не видит: живут они в `offers_context`, а
	// не в предложении, поэтому добавленная система раньше не меняла ни один
	// ключ — и вступление оставалось написанным про одну (живой КП 003/2026).
	genKeySystems = "gen.systems"
	// genPrefixNode — начало ключа текста одного узла.
	genPrefixNode = "gen."
	// genDefaultMaxWords — длина текста блока по умолчанию: абзац в
	// коммерческом письме, а не сочинение.
	genDefaultMaxWords = 150
)

// genKeyText — ключ кэша для узла блока.
func genKeyText(blockIndex, nodeIndex int) string {
	return fmt.Sprintf("gen.block%d.%d", blockIndex, nodeIndex)
}

// genKeyTextByID — тот же ключ по идентификатору блока.
//
// selection — набор выбранных систем (2026-11-08). Блок без повтора пишет ОДИН
// текст про все системы, поэтому при смене набора он обязан пересобраться:
// живой КП 003/2026 держал вступление, написанное про одну систему, хотя
// выбраны были две. Пусто — ключ прежний (выбора нет, блок не зависит от него).
func genKeyTextByID(blockID string, nodeIndex int, selection string) string {
	key := fmt.Sprintf("gen.%s.%d", blockID, nodeIndex)
	if selection == "" {
		return key
	}
	return key + ".sel" + hashShort(selection)
}

// genKeyTextForSystem — ключ кэша текста узла для ОДНОЙ системы.
//
// Повод (2026-11-08, живая жалоба на КП 003/2026): у блока с repeat:"systems"
// узел по промту печатался один раз, и его текст повторялся для КАЖДОЙ системы.
// Причина — ключ кэша не содержал систему, и вторая система брала текст первой.
// Теперь у каждой системы свой ключ, и модель пишет её карточку.
//
// handle — по чему различать системы (обычно номер заметки; см. systemCacheHandle).
// selection — набор выбранных систем: весь блок пересобирается, когда набор
// сменился (2026-11-08), и текст ОДНОЙ системы живёт под тем же набором.
func genKeyTextForSystem(blockID string, nodeIndex int, handle, selection string) string {
	return fmt.Sprintf("%s.sys%s", genKeyTextByID(blockID, nodeIndex, selection), handle)
}

// systemCacheHandle — устойчивый признак системы для ключа кэша.
//
// Номер заметки главнее (он не меняется). Система без заметки, выбранная по
// коду, различается хешем кода — иначе все такие системы получили бы один
// текст. Нет ни того, ни другого — "0": общий ключ, как до правки (лучше он,
// чем ключ несуществующей системы).
func systemCacheHandle(card SystemCard) string {
	if card.NoteID > 0 {
		return itoa(int(card.NoteID))
	}
	if sku := strings.TrimSpace(card.SKU); sku != "" {
		return "sku" + hashShort(sku)
	}
	return "0"
}

// cardsForOffer — карточки систем, о которых говорится в предложении.
//
// Основание — сам факт выбора системы, а не совпадение названий позиций
// (2026-11-03). До этого карточка попадала в КП, только если какой-то её
// материал нашёлся среди позиций; система, выбранная ради обоснования, в
// документ не печаталась вовсе. Названия в карточке и в таблице менеджера
// совпадать не обязаны: «Гидроизоляционная ПВХ мембрана Logicbase V-SL 2 мм
// (1 слой)» и «Мембрана Logicbase V-SL» — одна мембрана, а дословно две
// разные строки.
//
// Пустой выбор — ни одной карточки: старые КП печатаются как раньше.
func cardsForOffer(cards []SystemCard, offer Offer) []SystemCard {
	if len(offer.Systems) == 0 {
		return nil
	}
	out := make([]SystemCard, 0, len(offer.Systems))
	for _, chosen := range offer.Systems {
		choice := SystemChoice{SKU: chosen.SKU, NoteID: chosen.NoteID}
		if card, found := FindSystem(cards, choice); found {
			// Одна и та же карточка не печатается дважды, даже если менеджер
			// выбрал её по коду и по заметке: это одна система.
			if !containsCard(out, card) {
				out = append(out, card)
			}
		}
	}
	return out
}

// containsCard — уже ли эта карточка в списке.
func containsCard(cards []SystemCard, card SystemCard) bool {
	for _, c := range cards {
		if c.NoteID > 0 && card.NoteID > 0 {
			if c.NoteID == card.NoteID {
				return true
			}
			continue
		}
		if strings.EqualFold(strings.TrimSpace(c.SKU), strings.TrimSpace(card.SKU)) {
			return true
		}
	}
	return false
}

// normalizeItemName — название для сравнения: регистр, ё и пробелы.
func normalizeItemName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(name, "ё", "е")), " "))
}

// hashOfferFacts — отпечаток данных, по которым модель пишет текст.
//
// Берётся только то, что модель реально видит: без created_at (иначе текст
// переписывался бы каждый день) и без form_values целиком (в них сам кэш).
func hashOfferFacts(offer Offer) string {
	var b strings.Builder
	b.WriteString(offer.Number + "\x1f")
	b.WriteString(offer.Client.Name + "\x1f")
	b.WriteString(offer.Client.Contact + "\x1f")
	for _, key := range []string{"object_name", "object_purpose", "object_area", "customer_address"} {
		if offer.FormValues != nil {
			b.WriteString(offer.FormValues[key] + "\x1f")
		}
	}
	for _, it := range offer.Items {
		fmt.Fprintf(&b, "%s|%s|%g|%g|%g\x1f", it.Name, it.Unit, it.Qty, it.Price, it.Sum)
	}
	// v3 (2026-11-09): в текстах повтора по системам имя «система №N» попадало
	// в промт вместо названия из карточки и застревало в кэше под неизменным
	// хешем. Смена версии выключает старый кэш у всех КП — тексты один раз
	// пересобираются уже с правильными именами систем.
	fmt.Fprintf(&b, "%.2f|v3", offer.Total)
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:8])
}

// generatedTextFor — текст узла: из кэша, если он свежий, иначе пусто.
//
// Генерация требует модели и записи в БД, а рендер — чистая сборка документа.
// Поэтому печать только читает, а пишет отдельный проход (generateBlockTexts)
// перед сборкой файла: так документ собирается одинаково в любом месте и не
// ходит в сеть сам.
func generatedTextFor(rc renderCtx, blockID string, nodeIndex int) string {
	if rc.values == nil {
		return ""
	}
	if rc.values[genKeySection] != hashOfferFacts(rc.offer) {
		return ""
	}
	// Внутри повтора по системам текст узла — свой у каждой системы
	// (2026-11-08): с общим ключом вторая система печатала текст первой.
	selection := rc.selectionHandle()
	key := genKeyTextByID(blockID, nodeIndex, selection)
	if rc.systemCard != nil {
		key = genKeyTextForSystem(blockID, nodeIndex, systemCacheHandle(*rc.systemCard), selection)
	}
	// documentText и здесь: КП, собранные до 2026-11-05, держат в кэше пометку
	// [НУЖНО: …], и без чистки при чтении она уйдёт клиенту при следующем
	// скачивании того же файла.
	return documentText(strings.TrimSpace(rc.values[key]))
}

// promptNodes — узлы `prompt_text` формы вместе с их местом.
type promptNode struct {
	BlockIndex int
	NodeIndex  int
	BlockID    string
	// Repeat — вид повтор блока: пусто — обычный блок, "systems" — блок
	// печатается по разу на систему (2026-11-08). От повтора зависит, писать
	// ли ОДИН текст или по тексту на каждую систему.
	Repeat string
	Node   blockNode
}

// collectPromptNodes — все узлы по промту в порядке документа.
func collectPromptNodes(form OfferForm) []promptNode {
	var out []promptNode
	for bi, block := range form.Blocks {
		if !block.ShowInClient {
			continue
		}
		repeat := strings.TrimSpace(block.Repeat)
		for ni, node := range blockNodes(block) {
			if node.Type != "prompt_text" || strings.TrimSpace(node.Instruction) == "" {
				continue
			}
			out = append(out, promptNode{
				BlockIndex: bi, NodeIndex: ni, BlockID: block.ID,
				Repeat: repeat, Node: node,
			})
		}
	}
	return out
}

// generateBlockTexts пишет тексты узлов по промту и возвращает обновлённые
// form_values. Ошибка модели узла не является ошибкой всего документа.
//
// reused — сколько узлов взято из кэша: плагин показывает «тексты обновлены /
// взяты готовые», чтобы человек понимал, почему файл не изменился. failed —
// сколько узлов модель не написала (для сводки сборки в журнале, 2026-11-06).
func generateBlockTexts(ctx context.Context, form OfferForm, offer Offer,
	c Completer, cards []SystemCard, refs []skills.CardRef) (map[string]string, int, int, int) {
	nodes := collectPromptNodes(form)
	values := map[string]string{}
	for k, v := range offer.FormValues {
		values[k] = v
	}
	if len(nodes) == 0 {
		return values, 0, 0, 0
	}
	fresh := hashOfferFacts(offer)
	sections := cardsForOffer(cards, offer)
	selection := offerSelectionHandle(offer)
	if selection == "" {
		selection = cardSelectionHandle(sections)
	}
	previous := offer.FormValues
	cacheValid := previous != nil && previous[genKeySection] == fresh

	var reused, written, failed int
	for _, pn := range nodes {
		// Блок с repeat:"systems" печатается по разу на систему — значит, и
		// текст узла внутри него пишется для КАЖДОЙ системы, под её карточку
		// (2026-11-08). Иначе все системы получали текст первой.
		if pn.Repeat == blockRepeatSystems {
			for _, card := range sections {
				// cards сужены до одной системы (её описываем), а allCards —
				// весь набор: имена остальных систем в разделе «выбрано»
				// должны браться из карточек, а не подменяться «система №N».
				r, w, f := generatePromptNodeText(ctx, pn, systemCacheHandle(card),
					[]SystemCard{card}, sections, offer, refs, c, values, previous, cacheValid, selection)
				reused, written, failed = reused+r, written+w, failed+f
			}
			continue
		}
		r, w, f := generatePromptNodeText(ctx, pn, "", cards, cards, offer, refs, c,
			values, previous, cacheValid, selection)
		reused, written, failed = reused+r, written+w, failed+f
	}
	values[genKeySection] = fresh
	// Признак набора — рядом с признаком систем: он ставится даже когда модель
	// не ответила ни на один узел, иначе следующая сборка повторила бы отказ.
	values[genKeySystems] = selection
	return values, reused, written, failed
}

// generatePromptNodeText пишет (или берёт из кэша) текст одного узла.
//
// handle != "" — узел в блоке с повтором по системам: ключ кэша и подсказка для
// модели относятся к ОДНОЙ системе (card), а не ко всему предложению.
// allCards — весь набор выбранных систем (для раздела «выбрано» в подсказке):
// у повтора cards сужены до одной, и без полного списка имена остальных систем
// подменялись «система №N» (2026-11-09, живое КП 003/2026). nil — все cards.
// selection — набор выбранных систем: подсказке он нужен всегда (вступление
// перечисляет все системы), ключу — чтобы текст пересобрался со сменой набора
// (2026-11-08).
// Возвращает (взято из кэша, написано, не удалось).
func generatePromptNodeText(ctx context.Context, pn promptNode, handle string,
	cards, allCards []SystemCard, offer Offer, refs []skills.CardRef, c Completer,
	values, previous map[string]string, cacheValid bool, selection string) (int, int, int) {
	key := genKeyTextByID(pn.BlockID, pn.NodeIndex, selection)
	if handle != "" {
		key = genKeyTextForSystem(pn.BlockID, pn.NodeIndex, handle, selection)
	}
	if cacheValid && strings.TrimSpace(previous[key]) != "" {
		values[key] = previous[key]
		return 1, 0, 0
	}
	started := time.Now()
	text, err := writePromptNode(ctx, pn.Node, offer, cards, allCards, selection, refs, c)
	logModelCall("block:"+pn.BlockID, key, len(pn.Node.Instruction), started, err)
	if err != nil || strings.TrimSpace(text) == "" {
		// Модель недоступна или ответила пустым — блок молчит.
		delete(values, key)
		return 0, 0, 1
	}
	values[key] = text
	return 0, 1, 0
}

// writePromptNode — один вызов модели для одного узла.
//
// allCards — весь набор выбранных систем: у блока с повтором `cards` сужены до
// одной системы, а разделу «выбрано» нужны названия всех (nil — все cards).
// selection — набор выбранных систем всего предложения: без этого признака
// модель не знала бы, что в КП их несколько.
func writePromptNode(ctx context.Context, node blockNode, offer Offer,
	cards, allCards []SystemCard, selection string, refs []skills.CardRef, c Completer) (string, error) {
	if c == nil {
		return "", nil
	}
	maxWords := node.MaxWords
	if maxWords <= 0 || maxWords > 800 {
		maxWords = genDefaultMaxWords
	}
	system := skills.RenderBlockSystem(node.Instruction, node.Hint, maxWords)
	user := buildBlockUserPromptSelection(node.Instruction+" "+node.Hint, offer, cards, allCards, selection, refs)
	raw, err := c.Complete(ctx, system, user, documentTextMaxTokens())
	if err != nil {
		return "", err
	}
	return cleanBlockText(raw), nil
}

// blockMaxTokens — бюджет ответа модели на текст блока.
//
// **Больше не применяется к тексту документа** (2026-11-06). Свой `max_tokens`
// у КП означал, что предел ответа назначаем мы, а не провайдер, — и на живом
// КП 003/2026 бюджет кончался раньше, чем модель дописывала описание: в
// документ уходило вступление-рассуждение, а дальше пусто. Плагин 007 этого
// не знает вовсе, потому что `max_tokens` не шлёт (`llm-center.ts`).
//
// Функция оставлена как оценка «хватит ли ответа» для тестов бюджета и как
// запасная цифра, если когда-нибудь понадобится ограничение по смыслу.
// Печать текста документа вызывает Complete с нулём — предел даёт провайдер.
func blockMaxTokens(maxWords int) int {
	if maxWords <= 0 {
		maxWords = genDefaultMaxWords
	}
	return maxWords*10 + 600
}

// documentTextMaxTokens — предел ответа для текста документа.
//
// Ноль значит «предел назначает провайдер»: `buildChatBody` в llm-service не
// кладёт `max_tokens` в тело при нуле, и ответ не режется нашим числом. Одно
// решение в одном месте — здесь, чтобы печать блока и печать макета не
// разошлись (2026-11-06).
func documentTextMaxTokens() int {
	return 0
}

// cleanBlockText убирает то, что модель добавляет по привычке: маркеры кода,
// кавычки вокруг всего ответа и размышление вслух перед ответом.
//
// Рассуждение появилось в живом КП 003/2026 (2026-11-03): модель вернула
// «The task: … / Let me think… / hmm, is not in the brief» и черновик, и всё
// это ушло клиенту. Запрет стоит и в промпте, но промпт — просьба, а не
// гарантия: у провайдера меняется модель, и однажды поведение вернётся. Ответ
// печатается без человека, поэтому граница должна быть в коде.
//
// Что делает: если ответ открывается признаком рассуждения (строка о задании,
// «Let me…», «I should…», «думаю», «итак», «hmm»), отбрасывается всё до
// первого абзаца русского текста. Пусто после этого — блок молчит, как и при
// недоступной модели: пустой блок в документе честнее черновика.
func cleanBlockText(raw string) string {
	text := strings.TrimSpace(raw)
	text = strings.TrimPrefix(text, "```markdown")
	text = strings.TrimPrefix(text, "```md")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)
	if len(text) > 1 && strings.HasPrefix(text, `"`) && strings.HasSuffix(text, `"`) {
		text = strings.TrimSpace(text[1 : len(text)-1])
	}
	text = stripMarkdownHeadings(text)
	cleaned := trimTruncatedTail(documentText(stripReasoning(text)))
	// План - не текст: пустой блок честнее оглавления (см. looksLikePlan).
	if looksLikePlan(cleaned) {
		return ""
	}
	return cleaned
}

// reMarkdownHeading — строка-заголовок в разметке Markdown: `## Заголовок`.
//
// Появилась в живом КП 003/2026 (2026-11-06): в макете подсказка просила
// «помечай заголовки второго уровня в разметке ворда», модель поняла это как
// Markdown и написала `## …` в тексте. Макет печатает текст как есть, поэтому
// `##` ушли клиенту. Знаки разметки — не текст документа: снимаем `#` и
// оставляем заголовок отдельной строкой, её печать оформляет как абзац.
var reMarkdownHeading = regexp.MustCompile(`^\s{0,3}#{1,6}\s+(.+?)\s*#*\s*$`)

// reMarkdownBullet — строка-пункт Markdown: `- пункт`, `* пункт`.
var reMarkdownBullet = regexp.MustCompile(`^\s{0,3}[-*+]\s+(.+)$`)

// stripMarkdownHeadings убирает знаки Markdown-разметки из ответа модели.
//
// Только знаки, текст не переписывается: «## Отмостка» → «Отмостка»,
// «**жирный**» → «жирный». Строка из одних знаков («##.») не становится
// пустым абзацем, а выбрасывается целиком.
func stripMarkdownHeadings(text string) string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			out = append(out, "")
			continue
		}
		if m := reMarkdownHeading.FindStringSubmatch(trimmed); m != nil {
			trimmed = strings.TrimSpace(m[1])
		} else if m := reMarkdownBullet.FindStringSubmatch(trimmed); m != nil {
			trimmed = strings.TrimSpace(m[1])
		}
		trimmed = stripInlineMarkdown(trimmed)
		// От строки остались только знаки разметки — печатать нечего.
		if strings.TrimSpace(strings.Trim(trimmed, `#*_-–—.,;: `)) == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return strings.Join(out, "\n")
}

// stripInlineMarkdown снимает `**жирный**` и `__жирный__`, сохраняя текст.
func stripInlineMarkdown(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	return strings.TrimSpace(s)
}

// reSentenceEnd — конец законченной фразы: знак, за которым идёт пробел, перевод
// строки или конец текста. «г/м2» и «V-SL» сюда не относятся: там цифра, а не
// знак конца.
var reSentenceEnd = regexp.MustCompile(`[.!?](?:\s|$)`)

// trimTruncatedTail снимает обрыв ответа модели (2026-11-05).
//
// Ответ, оборванный бюджетом токенов, кончается посреди слова: «…часть одним
// кон». Печатать его клиенту нельзя — это видно и читается как сбой. Правило:
//
//   - последний абзац кончается знаком конца фразы — не трогаем ничего;
//   - внутри последнего абзаца есть законченные фразы — оставляем их, дописав
//     точку, если эта фраза стояла посреди абзаца;
//   - законченных фраз в последнем абзаце нет — абзац снимается целиком
//     (лучше пусто, чем огрызок). Предыдущие абзацы остаются как были.
//
// Точку в конце дописываем, чтобы строка не висела: «…одним контуром» без
// знака конца читается как та же недописанная фраза.
func trimTruncatedTail(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	lines := strings.Split(text, "\n")
	idx := lastNonEmptyLine(lines)
	if idx < 0 {
		return text
	}
	last := strings.TrimSpace(lines[idx])
	if paragraphIsFinished(last) {
		return text
	}
	if cut := lastSentenceEnd(last); cut >= 0 {
		lines[idx] = strings.TrimSpace(last[:cut+1])
		return strings.TrimSpace(strings.Join(lines[:idx+1], "\n"))
	}
	return dropUnfinishedParagraph(lines, idx)
}

// paragraphIsFinished — последний абзац не оборван.
//
// Два признака конца. Первый — знак конца фразы. Второй — адрес (2026-11-06):
// обрыв режет ответ по последней точке, а в адресе точек сколько угодно
// («nav.tn.ru»), поэтому строка «Технический лист: https://nav.tn.ru/pim/x.pdf»
// считалась оборванной и превращалась в «Технический лист.» — адрес пропадал
// из документа целиком. Ссылка — готовый факт, обрывать в ней нечего.
func paragraphIsFinished(last string) bool {
	return endsSentence(last) || hasLayoutURL(last)
}

// dropUnfinishedParagraph убирает абзац, у которого не нашлось законченной
// фразы. Одно слово — обрывок, а не абзац: он снимается целиком. Из длинного
// абзаца снимается последнее слово и ставится точка, чтобы текст не висел
// («…одним кон» не превращается в «…одним кон.»).
func dropUnfinishedParagraph(lines []string, idx int) string {
	words := strings.Fields(strings.TrimSpace(lines[idx]))
	if len(words) < 2 {
		if idx == 0 {
			return ""
		}
		return strings.TrimSpace(strings.Join(lines[:idx], "\n"))
	}
	words = words[:len(words)-1]
	lines[idx] = strings.TrimRight(strings.Join(words, " "), ",;:—-–") + "."
	return strings.TrimSpace(strings.Join(lines[:idx+1], "\n"))
}

// lastNonEmptyLine — номер последнего непустого абзаца; -1 — пусто всё.
func lastNonEmptyLine(lines []string) int {
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return i
		}
	}
	return -1
}

// endsSentence — кончается ли текст знаком конца фразы, за которым допустимы
// закрывающие кавычки и скобки.
func endsSentence(s string) bool {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	for len(runes) > 0 {
		switch runes[len(runes)-1] {
		case '"', '»', ')', ']':
			runes = runes[:len(runes)-1]
			continue
		}
		break
	}
	if len(runes) == 0 {
		return false
	}
	switch runes[len(runes)-1] {
	case '.', '!', '?', '…':
		return true
	}
	return false
}

// lastSentenceEnd — индекс последнего знака конца фразы (байтовый), после
// которого идёт пробел или конец текста. -1 — законченных фраз нет.
func lastSentenceEnd(s string) int {
	all := reSentenceEnd.FindAllStringIndex(s, -1)
	if len(all) == 0 {
		return -1
	}
	return all[len(all)-1][0]
}

// reNeedMarker — пометка «здесь не хватает факта», которую ставит модель.
//
// Внутри может быть всё что угодно, включая точку и запятую: на живом КП
// 003/2026 модель написала «[НУЖНО: что за объект, какие грунтовые воды и
// какой срок службы требуется]». Жадный `[^\]]*` берёт пометку целиком и не
// оставляет обрывков.
var reNeedMarker = regexp.MustCompile(`\[НУЖНО:[^\]]*\]`)

// documentText — текст, готовый к печати клиенту.
//
// Убирает пометки [НУЖНО: …]. Пометку ставит модель, а читает документ
// заказчик: в живом КП 003/2026 он увидел вопрос, адресованный менеджеру.
// Пробел не должен оставаться: фраза, которая состояла только из пометки,
// уходит целиком, вместе с висящей кавычкой или тире.
//
// Применяется в двух местах — при записи текста в кэш и при печати из кэша:
// КП, собранные до этой правки, уже хранят пометку в form_values.
func documentText(raw string) string {
	if !strings.Contains(raw, "[НУЖНО:") {
		return raw
	}
	var kept []string
	for _, line := range strings.Split(raw, "\n") {
		if !strings.Contains(line, "[НУЖНО:") {
			kept = append(kept, line)
			continue
		}
		cleaned := reNeedMarker.ReplaceAllString(line, " ")
		cleaned = strings.Join(strings.Fields(cleaned), " ")
		// От строки остались только кавычки, тире и точки — это мусор от
		// убранной пометки, а не текст.
		if strings.TrimSpace(strings.Trim(cleaned, `«»"'()[]{}.,;:—-– `)) == "" {
			continue
		}
		kept = append(kept, strings.TrimRight(cleaned, " "))
	}
	return collapseBlankLines(strings.Join(kept, "\n"))
}

// reReasoningLine — строка-признак размышления модели, а не текста для клиента.
//
// Список собран по живому ответу КП 003/2026 (2026-11-03): модель пересказывала
// задание по-английски, планировала ответ («Let me draft»), считала слова
// («Word limit 150») и оценивала свои же фразы («That's a real property…»,
// «Считаю допустимым»). Именно эти строки увидел заказчик.
//
// Проверяется начало строки: «Мембрана защищает…» в абзаце КП не совпадёт ни
// с одним признаком.
// «решение:» из списка убрано 2026-11-06: это не признак рассуждения, а
// обычное начало заголовка («Решение: дренаж по периметру»), и живой ответ
// КП 003/2026 терял из-за него строку «Состав решения». Рассуждение модели
// распознаётся по «Задача:», «Считаю…», «думаю» — там оно и живёт.
//
// «Brief says:» и «Alternative grouping:» добавлены 2026-11-06 по второму
// живому ответу того же КП: английские вводные строки, а за ними русский текст
// в кавычках — счёт по языку их пропускал, потому что русских слов в строке
// больше, чем английских. В документ заказчику это попадать не должно.
var reReasoningLine = regexp.MustCompile(`(?i)^(the task\b|task:|brief info\b|brief says\b|alternative\b|option\b|let me\b|let's\b|i need\b|i should\b|i will\b|i'll\b|i have\b|i am\b|i'm\b|hmm\b|okay\b|ok,|first,\s+i\b|so,?\s+i\b|word limit\b|advantages that are\b|is a fact\b|that's a real\b|better to be\b|is not in the brief|it's fair to say|header:|format says|this is fine|so no heading|need paragraphs|считаю допустимым|считаю,? что допустимо|осторожн|задача:|попробуем|сделаю проще|думаю|может,?\s|пометка|прочерк|не выдумыват|гуппа)`)

// reReasoningHeadingMarker — строка, которой модель ОПИСЫВАЕТ вёрстку вместо
// того, чтобы её написать.
//
// Живой прогон КП 003/2026 на макете (2026-11-06) принёс и такое:
// «Heading 2: "Гидроизоляционный слой: …"», «Text: геотекстиль 500 г/м²…»,
// «Ссылки (Heading 1)», «Заголовок 2: …». Модель перечисляла, из каких блоков
// будет документ, вместо того чтобы написать сам текст — в документе это
// читается как черновик редактора, а не как абзац КП.
// Строка-описание вёрстки распознаётся в двух видах: с меткой в НАЧАЛЕ
// («Heading 2: …») и со ссылкой на неё в СКОБКАХ («Ссылки (Heading 1)»).
// Второй вид — тоже план, а не текст: в КП попадали строки «Ссылки (Heading 1)»
// и «Цена» без единого предложения для заказчика.
// Строка-описание вёрстки распознаётся с меткой в НАЧАЛЕ («Heading 2: …»)
// и со ссылкой на неё в СКОБКАХ в конце («Ссылки (Heading 1)»). Второй вид —
// тоже план: в КП попадали строки «Ссылки (Heading 1)» и «Цена» без единого
// предложения для заказчика.
//
// Якорь `^` стоит В КАЖДОЙ ветке: общий якорь перед группой альтернатив
// привязывал к началу только первую, и скобочный вид не находился вовсе.
//
// МАРКЕР И ТЕКСТ — РАЗНЫЕ ВЕЩИ (2026-11-06, разбор «куда делась описательная
// часть»). Первая версия считала весь ответ планом, если в нём была хоть одна
// строка «Заголовок: …», и cleanBlockText возвращал пустую строку: настоящий
// текст, стоявший следующими строками, в документ не попадал вовсе. Живой
// ответ КП 003/2026 на этом и пропал — в макете осталось десять пустых
// абзацев. Поэтому решается СУДЬБА ОДНОЙ СТРОКИ:
//
//   - метка с содержимым («Заголовок: "…"», «Text: геотекстиль…») — метку
//     снимаем, содержимое печатаем (stripLayoutMarker);
//   - метка без содержимого и ссылка на вёрстку в скобках («Ссылки
//     (Heading 1)», «Цена») — строка-черновик, выбрасывается целиком
//     (isLayoutMarkerLine).
//
// План как ответ целиком по-прежнему отсекается — но по нескольким маркерам
// вёстки сразу, см. looksLikePlan.
var reReasoningHeadingMarker = regexp.MustCompile(
	`(?i)^(heading|paragraph|section|bullets?|table|list|заголовок|абзац|раздел|список|таблица|текст|text|title|цена)\s*(\d+|h[1-6])?\s*(?:[:(]|[«"])` +
		`|^[^(]*\((?:heading|заголовок|paragraph|абзац|section|раздел|text|текст)\s*(?:\d+|h[1-6])?\)\s*$` +
		`|^Цена$`,
)

// reLayoutMarkerPrefix — метка вёрстки в начале строки: «Заголовок: …»,
// «Текст: …», «Heading 2: …», «Heading "…"». Снимается как префикс, а остаток
// строки — обычный текст для заказчика. Кавычка тоже годится как разделитель:
// модель пишет название и без двоеточия.
var reLayoutMarkerPrefix = regexp.MustCompile(
	`(?i)^(heading|paragraph|section|bullets?|table|list|text|title|заголовок|абзац|раздел|список|таблица|текст)\s*(\d+|h[1-6])?\s*(?:[:(]|[«"])\s*`,
)

// stripLayoutMarker снимает метку вёрстки с начала строки, оставляя текст.
//
// «Заголовок: "Горизонтальная часть: мембрана 2 мм"» → «Горизонтальная часть:
// мембрана 2 мм». Второй результат — была ли метка: по нему вызывающий отличает
// строку с текстом от сироты-черновика.
func stripLayoutMarker(line string) (string, bool) {
	if !reLayoutMarkerPrefix.MatchString(line) {
		return line, false
	}
	rest := reLayoutMarkerPrefix.ReplaceAllString(line, "")
	return strings.TrimSpace(strings.Trim(rest, `"«»`)), true
}

// isLayoutMarkerLine — строка целиком является черновиком вёрстки: ссылка на
// вёрстку вместо текста («Ссылки (Heading 1)», «Цена»).
//
// Метка с содержимым сюда НЕ относится — она снимается как префикс, а
// содержимое остаётся текстом (см. stripLayoutMarker).
func isLayoutMarkerLine(line string) bool {
	if !reReasoningHeadingMarker.MatchString(line) {
		return false
	}
	if _, ok := stripLayoutMarker(line); ok {
		return false
	}
	return line == "Цена" || strings.HasSuffix(line, ")")
}

// reDraftTitleLine — строка-название, которое модель написала вместо абзаца.
//
// Живой ответ КП 003/2026 открывался строкой `1. "Гидроизоляция фундамента:
// одна система на горизонталь, вертикаль и отмостку".` — это название будущего
// раздела в кавычках, а не фраза для заказчика; ниже модель пишет то же самое
// уже нормальным заголовком. Признак узкий: вся строка — пункт списка, целиком
// в кавычках, и в ней нет глагола-сказуемого, то есть это не предложение с
// цитатой внутри.
var reDraftTitleLine = regexp.MustCompile(`^(\d{1,2}[.)]\s*)?["«][^"»]{10,200}["»]\.?$`)

var reSelfQuestion = regexp.MustCompile(`^[^.!?]{0,80}\?$`)

// looksLikePlan - ответ модели это ОГЛАВЛЕНИЕ, а не текст.
//
// Живой прогон КП 003/2026 (2026-11-06): на просьбу описать систему модель
// отвечала "1. Intro (задача объекта, что предлагаем, цена). 2. h2 \"Состав
// решения\" - перечень слоёв. 3. h2 \"Мембрана LOGICBASE V-SL: ...\"". Это план
// будущего текста: заказчику он не говорит ничего.
//
// ЧТО ЗДЕСЬ НЕ ПРИЗНАК (решение пользователя 2026-11-06): сам по себе
// нумерованный список. Состав конструкции в КП - как раз честный нумерованный
// список слоёв ("1. Подготовка основания", "2. Грунтовка битумная",
// "3. Мембрана V-SL 2 мм"), и принимать его за план нельзя: получился бы пустой
// документ на самом нужном месте.
//
// Признак - СЛУЖЕБНЫЕ МАРКЕРЫ вёрстки, которых в тексте для заказчика не
// бывает: "h2 ...", "Heading 2:", "Section:", "Intro ...". Один такой маркер
// уже говорит, что модель перечисляет блоки документа, а не пишет документ.
// План — это ОГЛАВЛЕНИЕ, а не строка с заголовком (2026-11-06).
//
// Два разбора подряд упирались в одно и то же. Сначала «есть хоть одна
// метка — весь ответ план»: живой ответ КП 003/2026 начинался с «Заголовок:
// "…"» и пропадал целиком. Затем «служебных строк не меньше половины»: тогда
// ответ ИЗ ОДНОЙ строки «Heading 2: "Гидроизоляционный слой…"» — это одна
// служебная строка из одной, то есть снова план.
//
// Отличие плана от заголовка не в количестве, а в том, ЧТО в строке. Заголовок
// несёт текст для заказчика, план — только название будущего блока («h2
// "Состав решения" — перечень слоёв», «Section:», «Цена»). Поэтому план — это
// когда метка НЕ ОСТАВЛЯЕТ текста (cleanDraftLine её выбросил) в двух и более
// строках подряд.
func looksLikePlan(text string) bool {
	outline, prose := 0, 0
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Признак оглавления главнее признака текста: «2. h2 "Состав
		// решения" - перечень слоёв» несёт слова, но описывает БУДУЩИЙ
		// раздел, а не документ. Такую строку заказчику печатать нельзя.
		if rePlanMarker.MatchString(trimmed) || reHeadingWord.MatchString(trimmed) {
			outline++
			continue
		}
		if _, keep := cleanDraftLine(trimmed); keep {
			prose++
		}
	}
	// Оглавления в ответе БОЛЬШЕ, чем настоящих строк, — это план.
	// Одна строка-заголовок в ответе ничего не значит.
	return outline > prose
}

// rePlanMarker - маркер оглавления в начале строки: "h2 ...", "Intro:",
// "Section ...", "Глава 1". Это названия блоков документа, а не текст, и в
// готовом КП их быть не может.
// Маркер оглавления стоит в начале строки или в начале пункта списка:
// "h2 ...", "Intro:", "2. h2 \"Состав решения\"", "3. Section: ...".
var rePlanMarker = regexp.MustCompile(`(?i)^(\d{1,2}[.)]\s*)?(h[1-6]\b|intro\b|outro\b|section\b|chapter\b|part\b|глава\s|вступление\b)`)

// reHeadingWord - слово-метка вёрстки само по себе, без двоеточия:
// "Heading \"...\"", "Paragraph \"...\"". Модель пишет так, описывая блоки.
var reHeadingWord = regexp.MustCompile(`(?i)^(heading|paragraph|заголовок|абзац)\s*[:(\"]`)

// stripReasoning выбрасывает размышление модели из ответа.
//
// Живой ответ из КП 003/2026 (2026-11-03) показал, что рассуждение не идёт
// «куском до ответа»: строки модели о задании перемежаются с русскими
// пересказами состава, а внутри строк оно начинается с середины — «…даёт
// герметичный контур... hmm, "сварка" is not in the brief». Поэтому чистка
// идёт в два прохода: по строкам и внутри строки.
//
// Что выбрасывается:
//   - строка-признак рассуждения (план, «Let me…», «Word limit», «Header: …»);
//   - строка, где русского меньше, чем латинского: служебный текст модели;
//   - кусок строки от первой латинской скобки-рассуждения до конца строки
//     (см. reReasoningTail) и короткие вставки вида «hmm, …» внутри строки.
//
// Что НЕ делается: русский текст не переписывается и абзацы не склеиваются
// заново сверх уборки пустых строк. Текст КП для клиента терять нельзя.
func stripReasoning(text string) string {
	if text == "" || !hasReasoningLine(text) {
		return text
	}
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	dropped := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		clean, keep := cleanDraftLine(trimmed)
		if keep {
			kept = append(kept, clean)
			// Строка осталась, но изменилась (снята метка вёрстки) — это тоже
			// правка ответа, иначе возврат `text` вернул бы метку назад.
			if clean != trimmed {
				dropped++
			}
			continue
		}
		if trimmed == "" {
			// Пустая строка — разделитель абзацев, не черновик.
			kept = append(kept, "")
		}
		dropped++
	}
	if dropped == 0 {
		return text
	}
	return collapseBlankLines(strings.Join(kept, "\n"))
}

// cleanDraftLine — судьба ОДНОЙ строки ответа модели (2026-11-06).
//
// Второй результат — оставлять ли строку. Первый — что именно печатать: та же
// строка либо без метки вёрстки («Заголовок: "…"» → «…»), либо без хвоста
// рассуждения. Одно решение в одном месте: раньше судьбу строки решали три
// разных места в stripReasoning, и правка одного расходилась с другим.
func cleanDraftLine(trimmed string) (string, bool) {
	trimmed, ok := withoutLayoutMarker(trimmed)
	if !ok || isDraftSentence(trimmed) {
		return "", false
	}
	trimmed = cutReasoningTail(trimmed)
	if trimmed == "" {
		return "", false
	}
	// Строка со ссылкой — текст ответа, даже если в ней почти нет кириллицы
	// (2026-11-06). Живой прогон на стенде: строка «Технический лист:
	// https://nav.tn.ru/…» почти целиком латиница, проверка языка считала её
	// английским рассуждением и выбрасывала — в документе пропадал адрес,
	// который модель привела по просьбе макета. Адрес — факт, а не речь.
	if hasLayoutURL(trimmed) {
		return trimmed, true
	}
	if !mostlyCyrillic(trimmed) {
		return "", false
	}
	return trimmed, true
}

// hasLayoutURL — в строке есть адрес. Адрес не бывает рассуждением модели.
func hasLayoutURL(line string) bool {
	return len(skills.FindURLs(line)) > 0
}

// withoutLayoutMarker снимает метку вёрстки с начала строки.
//
// Второй результат — false, если строку печатать нельзя вовсе: пустая,
// строка-рассуждение, сирота-метка или метка, за которой осталась только
// вёрстка («Heading 2: "…"» — сам заголовок клиенту не нужен, структура
// документа его и так задаёт).
func withoutLayoutMarker(trimmed string) (string, bool) {
	if trimmed == "" || reReasoningLine.MatchString(trimmed) || isLayoutMarkerLine(trimmed) {
		return "", false
	}
	stripped, wasMarker := stripLayoutMarker(trimmed)
	if !wasMarker {
		return trimmed, true
	}
	if stripped == "" || reReasoningHeadingMarker.MatchString(stripped) || reHeadingWord.MatchString(stripped) {
		return "", false
	}
	return stripped, true
}

// isDraftSentence — строка, которая выглядит как текст, но им не является:
// название раздела в кавычках вместо абзаца или вопрос модели к себе.
func isDraftSentence(trimmed string) bool {
	return reDraftTitleLine.MatchString(trimmed) || reSelfQuestion.MatchString(trimmed)
}

// cutReasoningTail отрезает рассуждение, начавшееся с середины строки.
//
// Отрезать нужно ДО проверки языка: в строке «ПВХ мембрана — …; сварка швов…
// hmm, "сварка" is not in the brief» английские слова стоят только в хвосте, и
// проверка по всей строке ошибла бы её полезную часть.
func cutReasoningTail(trimmed string) string {
	cleaned := reReasoningTail.ReplaceAllString(trimmed, "")
	if cleaned == trimmed {
		return trimmed
	}
	trimmed = strings.TrimSpace(reasoningTailSeparators.ReplaceAllString(cleaned, ""))
	if trimmed == "" {
		return ""
	}
	// Хвост отрезан — точку в конце ставим, иначе строка висит.
	if !strings.HasSuffix(trimmed, ".") {
		trimmed += "."
	}
	return trimmed
}

// reReasoningTail — рассуждение, начавшееся с середины строки.
//
// В живом ответе оно начиналось словами после многоточия: «… smug контур...
// hmm, "сварка" is not in the brief», «…после сдачи. That's a real property».
// Берём от этого места до конца строки: всё, что после рассуждения, — тоже
// рассуждение, а не ответ.
// Короткое английское суждение после русского текста в кавычках («…"Секция
// 1 (по расчёту: 5 520 м² мембраны)..." Ugly.») — тоже хвост: модель оценивает
// свой же черновик. Слово узнаётся по короткому списку оценочных, а не по
// языку строки: русских слов в строке больше, и счёт по языку её пропускает.
var reReasoningTail = regexp.MustCompile(`(?i)\s*(?:\.\.\.\s*)?(?:hmm\b|\bthat's a real\b|\bis a fact about\b|\bit's fair to say\b|\bbetter to be\b|\bis not in the brief\b|\bi should\b|\bi need\b|\blet me\b|\bword limit\b|\bugly\b|\bnice\b|\btoo vague\b|\btoo long\b|\bnot bad\b).*$`)

// reasoningTailSeparators — то, что остаётся на месте отрезанного хвоста:
// лишние точки, запятые и пробелы.
var reasoningTailSeparators = regexp.MustCompile(`[\s.,;:—–-]+$`)

// hasReasoningLine — есть ли в тексте хоть один признак рассуждения. Дешёвая
// проверка до разбора: обычный ответ модели её не проходит.
func hasReasoningLine(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if reReasoningLine.MatchString(trimmed) || !mostlyCyrillic(trimmed) {
			return true
		}
		// Метка вёрстки (с содержимым или без) — тоже причина пройтись
		// по строкам: stripReasoning её снимает, а текст за ней оставляет
		// (2026-11-06). Без этой строки ответ, начинавшийся с «Заголовок: …»,
		// проходил мимо разбора целиком.
		if reReasoningHeadingMarker.MatchString(trimmed) || reSelfQuestion.MatchString(trimmed) {
			return true
		}
		if reReasoningTail.MatchString(trimmed) {
			return true
		}
	}
	return false
}

// collapseBlankLines убирает лишние пустые строки, оставшиеся после выброса
// кусков, и оставляет абзацы ровно с одной пустой строкой между ними.
func collapseBlankLines(text string) string {
	var out []string
	blank := false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
			}
			blank = true
			continue
		}
		blank = false
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// mostlyCyrillic — строка написана по-русски, а не латиницей с русскими
// вставками.
//
// Строка считается текстом ответа, если латинских СЛОВ в ней почти нет
// (2026-11-06). До этого считались буквы, и строка «Given the risk, I'll
// frame group 3 as "Конструкция отмостки и примыканий"» проходила: русских
// букв в кавычках было больше, чем латинских в английской части, и
// английское рассуждение печаталось заказчику.
//
// Марки и артикулы не мешают: «XPS Carbon Prof», «V-SL», «PIR-плиты».
// Английская речь узнаётся по СЛУЖЕБНЫМ словам, а не по длине: «the», «and»,
// «for», «with», «from», «this», «that» в русском техническом тексте не
// встречаются отдельными словами, а перечислять их можно исчерпывающе. Порог
// по длине («слово короче пяти букв — марка») не годится: «risk», «group»,
// «frame» — обычные английские слова той же длины, что и марки.
func mostlyCyrillic(s string) bool {
	if countCyrillicLetters(s) == 0 {
		return false
	}
	return countSpeechLatinWords(s) <= 1
}

// countCyrillicLetters — сколько русских букв в строке.
func countCyrillicLetters(s string) int {
	n := 0
	for _, r := range s {
		if (r >= 'А' && r <= 'я') || r == 'Ё' || r == 'ё' {
			n++
		}
	}
	return n
}

// countSpeechLatinWords — сколько в строке латинских СЛОВ английской речи
// (марки и артикулы не считаются).
func countSpeechLatinWords(s string) int {
	n := 0
	for _, word := range strings.Fields(s) {
		word = strings.TrimFunc(word, func(r rune) bool {
			return !isLatinOrCyrillic(r)
		})
		if word == "" || !isLatin(word) || isLatinBrandWord(word) {
			continue
		}
		n++
	}
	return n
}

// isLatinOrCyrillic — буква, годная для слова.
func isLatinOrCyrillic(r rune) bool {
	switch {
	case r >= 'А' && r <= 'я', r == 'Ё', r == 'ё':
		return true
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		return true
	}
	return false
}

// isLatin — слово целиком из латинских букв.
func isLatin(word string) bool {
	for _, r := range word {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return word != ""
}

// englishFunctionWords — служебные слова английской речи. Именно по ним
// узнаётся рассуждение модели посреди русского текста; в техническом описании
// на русском они отдельными словами не встречаются.
var englishFunctionWords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
	"of": true, "to": true, "in": true, "on": true, "at": true, "by": true,
	"for": true, "with": true, "from": true, "as": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "it": true, "this": true,
	"that": true, "these": true, "those": true, "i": true, "i'll": true,
	"i'm": true, "i've": true, "we": true, "you": true, "they": true,
	"will": true, "would": true, "should": true, "need": true, "can": true,
	"could": true, "not": true, "if": true, "so": true, "then": true,
	"given": true, "frame": true, "group": true, "risk": true, "better": true,
	"about": true, "into": true, "than": true, "there": true, "their": true,
	"has": true, "have": true, "do": true, "does": true, "just": true,
	"like": true, "more": true, "most": true, "some": true, "no": true,
	"yes": true, "let": true, "lets": true, "hmm": true, "okay": true,
	"here": true, "where": true, "which": true, "whose": true, "how": true,
}

// isLatinBrandWord — латинское слово — марка или артикул, а не английская речь.
//
// Порядок проверок важен (живой промах 2026-11-06): служебное слово ищется
// ПЕРВЫМ. «Given», «I'll», «The» — обычная английская речь, хотя и написаны с
// заглавной; правило «заглавная буква = марка» само по себе пропускало строку
// «Given the risk, I'll frame group 3 as …» как текст ответа.
func isLatinBrandWord(word string) bool {
	lower := strings.ToLower(word)
	lower = strings.TrimRight(lower, ".,;:!?)]'\"")
	if englishFunctionWords[lower] {
		return false
	}
	if containsDigitOrHyphen(word) {
		return true
	}
	runes := []rune(word)
	if len(runes) == 0 {
		return false
	}
	allUpper := true
	for _, r := range runes {
		if r >= 'a' && r <= 'z' {
			allUpper = false
		}
	}
	if allUpper {
		return true
	}
	first := runes[0]
	return first >= 'A' && first <= 'Z'
}

// containsDigitOrHyphen — в слове есть цифра или дефис: артикул, а не речь.
func containsDigitOrHyphen(word string) bool {
	for _, r := range word {
		if (r >= '0' && r <= '9') || r == '-' {
			return true
		}
	}
	return false
}

// buildBlockUserPrompt — данные предложения текстом для модели.
//
// Ссылки идут как факты: модель может сказать «техдокументация приложена в
// конце», но адреса в текст не вписывает — иначе в документе будут адреса,
// которых нет в разделе ссылок.
//
// Бриф передаётся ЦЕЛИКОМ (2026-11-05). До этого модель видела четыре поля
// (объект, назначение, площадь, адресат) и ключи без префикса, а бриф плагин
// кладёт под `brief.` — поэтому на живом КП 003/2026, где менеджер всё
// заполнил, модель писала «[НУЖНО: что за объект и какие грунтовые воды]».
// Обоснование преимуществ без целей, болей и рисков писать не из чего.
func buildBlockUserPrompt(offer Offer, cards []SystemCard, refs []skills.CardRef) string {
	return buildBlockUserPromptFor("", offer, cards, refs)
}

// buildBlockUserPromptFor — промпт для узла, где набор систем совпадает с
// карточками (так пишут и читают тесты, и так работает блок без повтора).
func buildBlockUserPromptFor(instruction string, offer Offer, cards []SystemCard, refs []skills.CardRef) string {
	return buildBlockUserPromptSelection(instruction, offer, cards, cards, "", refs)
}

// buildBlockUserPromptSelection — данные предложения текстом для модели.
//
// allCards — весь набор выбранных систем для раздела «выбрано»: у блока с
// повтором `cards` сужены до одной системы, а читателю нужен весь комплект
// (2026-11-08). Если nil — берутся cards (блок без повтора, тесты). Без
// полного списка имена остальных систем подменялись «система №N» и доезжали
// до клиента в тексте модели (2026-11-09, живое КП 003/2026).
//
// selection — набор выбранных систем всего предложения (offerSelectionHandle).
// Пусто — набор считается по карточкам.
func buildBlockUserPromptSelection(instruction string, offer Offer, cards, allCards []SystemCard,
	selection string, refs []skills.CardRef) string {
	if allCards == nil {
		allCards = cards
	}
	var b strings.Builder
	b.WriteString("ДАННЫЕ ПРЕДЛОЖЕНИЯ:\n")
	b.WriteString(offerFactsTextFor(offer, skills.AsksForPrices(instruction)))
	b.WriteString(renderBriefForPrompt(offer.FormValues))
	b.WriteString(renderSystemsForPrompt(offer, allCards, selection))
	if len(cards) > 0 {
		b.WriteString("\nКАРТОЧКИ СИСТЕМ (для обоснования, в подмножестве ниже):\n")
		for _, card := range cards {
			b.WriteString(card.PromptText() + "\n\n")
		}
	}
	if len(refs) > 0 {
		b.WriteString("\nССЫЛКИ, КОТОРЫЕ БУДУТ В КОНЦЕ ДОКУМЕНТА (в текст адреса не вписывай,\n")
		b.WriteString("можно сказать, что техническая документация приложена):\n")
		for _, ref := range refs {
			b.WriteString("- " + ref.Label + "\n")
		}
	}
	b.WriteString("\nНапиши только текст этого блока.")
	return b.String()
}

// renderSystemsForPrompt — какие системы выбраны в предложении, разделом.
//
// Один раздел вместо разрозненных фактов (2026-11-08). На живом КП 003/2026
// модель получила карточки обеих систем и всё равно написала вступление об
// одной: в промпте не было сказано, что систем несколько и что вступление —
// введение ко всему комплекту. Правило о комплектной поставке появляется
// только при двух и более системах: при одной системе оно бессмысленно и
// сбивает модель на выдумку «сравнения вариантов».
func renderSystemsForPrompt(offer Offer, cards []SystemCard, selection string) string {
	names := promptSystemNames(offer, cards, selection)
	if len(names) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nВЫБРАННЫЕ СИСТЕМЫ В ПРЕДЛОЖЕНИИ (" + itoa(len(names)) + " шт.):\n")
	for _, name := range names {
		b.WriteString("- " + name + "\n")
	}
	if len(names) < 2 {
		return b.String()
	}
	b.WriteString("\nЭти системы выбраны по отдельности, каждая — на свой участок объекта, и в\n")
	b.WriteString("документе идут в комплекте. Если подсказка просит общий текст (вступление,\n")
	b.WriteString("резюме), говори обо ВСЕХ системах как об одном комплектном решении: что\n")
	b.WriteString("закрывает каждая и что вместе дают. Выдумывать системы, которых нет в\n")
	b.WriteString("списке, нельзя; молча писать об одной системе, когда их несколько, тоже\n")
	b.WriteString("нельзя — читатель решит, что остальное в предложение не входит.\n")
	return b.String()
}

// promptSystemNames — названия систем в порядке выбора, без повторов.
//
// Название берётся из карточек (так же, как их печатает метка `[Название
// системы]`): ни у одной живой системы нет кода, и номер заметки читателю ни о
// чём не говорит. Система, чьей карточки нет в переданном списке, называется
// так же, как её печатает metка: код, иначе номер заметки. Выбрасывать её
// нельзя — вступление обязано упомянуть все системы, даже если карточка ещё не
// загрузилась в кэш (2026-11-08).
func promptSystemNames(offer Offer, cards []SystemCard, selection string) []string {
	out := make([]string, 0, len(offer.Systems)+len(cards))
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		for _, have := range out {
			if strings.EqualFold(have, name) {
				return
			}
		}
		out = append(out, name)
	}
	for _, title := range offerSystemTitles(offer, cards) {
		add(title)
	}
	// Старые КП и тесты: предложение без выбора, но с карточками.
	if selection == "" || selection == cardSelectionHandle(cards) {
		for _, card := range cards {
			add(card.Title)
		}
	}
	return out
}

// briefPromptFields — что из брифа попадает в промпт, в каком порядке и под
// каким ключом. Один список: печать сводки (renderBriefSummary) и промпт
// модели должны называть одни и те же вещи одинаково.
var briefPromptFields = []struct {
	Key   string // ключ брифа (model.BriefPayload)
	Label string
}{
	{"object_name", "Объект"},
	{"object_purpose", "Назначение объекта"},
	{"object_area", "Площадь"},
	{"customer_address", "Адресат КП"},
	{"customer_goals", "Задача заказчика"},
	{"customer_pains", "Боли"},
	{"project_risks", "Риски проекта"},
	{"planned_systems", "Планируемые системы"},
	{"timeline", "Сроки"},
	{"budget_signal", "Бюджет"},
	{"decision_maker", "Кто решает"},
	{"why_us", "Почему мы"},
	{"notes", "Прочее"},
}

// renderBriefForPrompt собирает раздел «БРИФ» для модели.
//
// Ключи понимаются в двух видах: `brief.<key>` (как их пишет плагин) и
// `<key>` (КП, собранные до перехода на префикс). Префикс главнее: если оба
// есть, берётся `brief.`.
//
// Пустые поля не печатаются: «Риски: » модель достроит догадкой, и это ровно
// та выдумка, которую запрещает noInventDoc.
func renderBriefForPrompt(values map[string]string) string {
	if values == nil {
		return ""
	}
	var b strings.Builder
	for _, f := range briefPromptFields {
		v := strings.TrimSpace(values["brief."+f.Key])
		if v == "" {
			v = strings.TrimSpace(values[f.Key])
		}
		if v == "" {
			continue
		}
		if b.Len() == 0 {
			b.WriteString("\nБРИФ (заполнил менеджер, это факты об объекте и сделке):\n")
		}
		b.WriteString("- " + f.Label + ": " + v + "\n")
	}
	if b.Len() == 0 {
		return "\nБРИФ: не заполнен — пиши только по данным предложения и карточкам.\n"
	}
	return b.String()
}

// offerFactsText — числа предложения в формате документа (с ценами).
func offerFactsText(offer Offer) string {
	return offerFactsTextFor(offer, true)
}

// offerFactsTextFor — данные предложения для модели; цены и итоговая сумма
// включаются только тогда, когда подсказка администратора прямо их просит
// (2026-11-08, живая жалоба на КП 003/2026). Иначе модель видит рублёвые суммы
// и вставляет их в конец каждого текстового блока даже при запрете в промпте.
func offerFactsTextFor(offer Offer, includePrices bool) string {
	var b strings.Builder
	writeOfferFactLine(&b, "Номер предложения", offer.Number)
	writeOfferFactLine(&b, "Заказчик", offer.Client.Name)
	writeOfferFactLine(&b, "Контактное лицо", offer.Client.Contact)
	if includePrices && offer.Total > 0 {
		b.WriteString("- Итого: " + skills.FormatMoney(offer.Total) + " руб.\n")
	}
	if len(offer.Items) > 0 {
		b.WriteString("- Состав:\n")
		for _, it := range offer.Items {
			b.WriteString(offerItemFactLine(it, includePrices))
		}
	}
	if b.Len() == 0 {
		return "предложение ещё не рассчитано\n"
	}
	return b.String()
}

// writeOfferFactLine дописывает непустую строку факта предложения.
func writeOfferFactLine(b *strings.Builder, label, value string) {
	if model.NonEmpty(value) {
		b.WriteString("- " + label + ": " + value + "\n")
	}
}

// offerItemFactLine — одна позиция состава: с ценой или только с количеством.
func offerItemFactLine(it OfferItem, includePrices bool) string {
	if includePrices {
		return fmt.Sprintf("  • %s — %s %s × %s = %s руб.\n",
			it.Name, formatQtyValue(it.Qty), it.Unit,
			skills.FormatMoney(it.Price), skills.FormatMoney(it.Sum))
	}
	return fmt.Sprintf("  • %s — %s %s\n",
		it.Name, formatQtyValue(it.Qty), it.Unit)
}
