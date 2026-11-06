package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

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

// genKeyTextByID — тот же ключ по идентификатору блока: у формы блоки правятся
// и переезжают, и привязка к id переживает перестановку.
func genKeyTextByID(blockID string, nodeIndex int) string {
	return fmt.Sprintf("gen.%s.%d", blockID, nodeIndex)
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
	fmt.Fprintf(&b, "%.2f", offer.Total)
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
	// documentText и здесь: КП, собранные до 2026-11-05, держат в кэше пометку
	// [НУЖНО: …], и без чистки при чтении она уйдёт клиенту при следующем
	// скачивании того же файла.
	return documentText(strings.TrimSpace(rc.values[genKeyTextByID(blockID, nodeIndex)]))
}

// promptNodes — узлы `prompt_text` формы вместе с их местом.
type promptNode struct {
	BlockIndex int
	NodeIndex  int
	BlockID    string
	Node       blockNode
}

// collectPromptNodes — все узлы по промту в порядке документа.
func collectPromptNodes(form OfferForm) []promptNode {
	var out []promptNode
	for bi, block := range form.Blocks {
		if !block.ShowInClient {
			continue
		}
		for ni, node := range blockNodes(block) {
			if node.Type != "prompt_text" || strings.TrimSpace(node.Instruction) == "" {
				continue
			}
			out = append(out, promptNode{BlockIndex: bi, NodeIndex: ni, BlockID: block.ID, Node: node})
		}
	}
	return out
}

// generateBlockTexts пишет тексты узлов по промту и возвращает обновлённые
// form_values. Ошибка модели узла не является ошибкой всего документа.
//
// reused — сколько узлов взято из кэша: плагин показывает «тексты обновлены /
// взяты готовые», чтобы человек понимал, почему файл не изменился.
func generateBlockTexts(ctx context.Context, form OfferForm, offer Offer,
	c Completer, cards []SystemCard, refs []skills.CardRef) (map[string]string, int, int) {
	nodes := collectPromptNodes(form)
	values := map[string]string{}
	for k, v := range offer.FormValues {
		values[k] = v
	}
	if len(nodes) == 0 {
		return values, 0, 0
	}
	fresh := hashOfferFacts(offer)
	previous := offer.FormValues
	cacheValid := previous != nil && previous[genKeySection] == fresh

	reused, written := 0, 0
	for _, pn := range nodes {
		key := genKeyTextByID(pn.BlockID, pn.NodeIndex)
		if cacheValid {
			if strings.TrimSpace(previous[key]) != "" {
				values[key] = previous[key]
				reused++
				continue
			}
		}
		text, err := writePromptNode(ctx, pn.Node, offer, cards, refs, c)
		if err != nil || strings.TrimSpace(text) == "" {
			// Модель недоступна или ответила пустым — блок молчит.
			delete(values, key)
			continue
		}
		values[key] = text
		written++
	}
	values[genKeySection] = fresh
	return values, reused, written
}

// writePromptNode — один вызов модели для одного узла.
func writePromptNode(ctx context.Context, node blockNode, offer Offer,
	cards []SystemCard, refs []skills.CardRef, c Completer) (string, error) {
	if c == nil {
		return "", nil
	}
	maxWords := node.MaxWords
	if maxWords <= 0 || maxWords > 800 {
		maxWords = genDefaultMaxWords
	}
	system := skills.RenderBlockSystem(node.Instruction, node.Hint, maxWords)
	user := buildBlockUserPrompt(offer, cards, refs)
	raw, err := c.Complete(ctx, system, user, blockMaxTokens(maxWords))
	if err != nil {
		return "", err
	}
	return cleanBlockText(raw), nil
}

// blockMaxTokens — бюджет ответа модели на текст блока.
//
// «Слово ×4» не работает: у русского слова бывает 2–3 токена, а модель ещё
// и начинает с рассуждения, которое вырезается уже после. На живом КП
// 003/2026 ответ кончился на «…закрывает подземную часть одним кон» — это
// обрыв по токенам, а не по смыслу. Берём 6 токенов на слово (слово + запас)
// и добавляем 400 на служебные рассуждения и пометки.
//
// 2026-11-05: бюджет увеличен повторно. Живой текст показал, что 6×150+400 =
// 1300 всё ещё обрывается на 150-словном ответе с длинным рассуждением. Теперь
// 10 токенов на слово плюс 600 — ответ влезает с запасом, а лишние токены
// стоят дешевле повторного скачивания документа менеджером.
func blockMaxTokens(maxWords int) int {
	if maxWords <= 0 {
		maxWords = genDefaultMaxWords
	}
	return maxWords*10 + 600
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
	return trimTruncatedTail(documentText(stripReasoning(text)))
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
	// Последний непустой абзац — тот, что мог оборваться.
	idx := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return text
	}
	last := strings.TrimSpace(lines[idx])
	if endsSentence(last) {
		return text
	}
	if cut := lastSentenceEnd(last); cut >= 0 {
		finished := strings.TrimSpace(last[:cut+1])
		lines[idx] = finished
		return strings.TrimSpace(strings.Join(lines[:idx+1], "\n"))
	}
	// Законченных фраз в последнем абзаце нет. Обрыв приходится на последнее
	// слово: снимаем его и закрываем фразу точкой — текст остаётся целым по
	// смыслу, а «…одним кон» не превращается в «…одним кон.»
	words := strings.Fields(last)
	if len(words) < 2 {
		// Одно слово — это не абзац, а обрывок. Убираем абзац целиком, если
		// есть предыдущие.
		if idx == 0 {
			return ""
		}
		return strings.TrimSpace(strings.Join(lines[:idx], "\n"))
	}
	words = words[:len(words)-1]
	lines[idx] = strings.TrimRight(strings.Join(words, " "), ",;:—-–") + "."
	return strings.TrimSpace(strings.Join(lines[:idx+1], "\n"))
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
var reReasoningLine = regexp.MustCompile(`(?i)^(the task\b|task:|brief info\b|let me\b|let's\b|i need\b|i should\b|i will\b|i'll\b|i have\b|i am\b|i'm\b|hmm\b|okay\b|ok,|first,\s+i\b|so,?\s+i\b|word limit\b|advantages that are\b|is a fact\b|that's a real\b|better to be\b|is not in the brief|it's fair to say|header:|format says|this is fine|so no heading|need paragraphs|считаю допустимым|считаю,? что допустимо|осторожн)`)

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
		if trimmed == "" {
			kept = append(kept, "")
			continue
		}
		if reReasoningLine.MatchString(trimmed) || !mostlyCyrillic(trimmed) {
			dropped++
			continue
		}
		if cleaned := reReasoningTail.ReplaceAllString(trimmed, ""); cleaned != trimmed {
			dropped++
			trimmed = strings.TrimSpace(reasoningTailSeparators.ReplaceAllString(cleaned, ""))
			if trimmed == "" {
				continue
			}
			// Хвост отрезан — точку в конце ставим, иначе строка висит.
			if !strings.HasSuffix(trimmed, ".") {
				trimmed += "."
			}
		}
		kept = append(kept, trimmed)
	}
	if dropped == 0 {
		return text
	}
	return collapseBlankLines(strings.Join(kept, "\n"))
}

// reReasoningTail — рассуждение, начавшееся с середины строки.
//
// В живом ответе оно начиналось словами после многоточия: «… smug контур...
// hmm, "сварка" is not in the brief», «…после сдачи. That's a real property».
// Берём от этого места до конца строки: всё, что после рассуждения, — тоже
// рассуждение, а не ответ.
var reReasoningTail = regexp.MustCompile(`(?i)\s*(?:\.\.\.\s*)?(?:hmm\b|\bthat's a real\b|\bis a fact about\b|\bit's fair to say\b|\bbetter to be\b|\bis not in the brief\b|\bi should\b|\bi need\b|\blet me\b|\bword limit\b).*$`)

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
// Строка считается текстом ответа, если русских букв в ней не меньше, чем
// латинских. «Мембрана Logicbase V-SL защищает подвал» — ответ. «Let me think
// about what's known: мембрана, геотекстиль» — черновик: слов немного и они
// английские. В живом ответе КП 003/2026 ровно такие строки шли вперемешку с
// русскими, и это единственное, что их различает.
func mostlyCyrillic(s string) bool {
	var cyr, lat int
	for _, r := range s {
		switch {
		case (r >= 'А' && r <= 'я') || r == 'Ё' || r == 'ё':
			cyr++
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			lat++
		}
	}
	return cyr > 0 && cyr >= lat
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
	var b strings.Builder
	b.WriteString("ДАННЫЕ ПРЕДЛОЖЕНИЯ:\n")
	b.WriteString(offerFactsText(offer))
	b.WriteString(renderBriefForPrompt(offer.FormValues))
	if len(cards) > 0 {
		b.WriteString("\nСИСТЕМЫ И МАТЕРИАЛЫ В ПРЕДЛОЖЕНИИ:\n")
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

// offerFactsText — числа предложения в формате документа. Тот же вид, что у
// секций (skills.renderOfferFacts): модель получает одинаковые числа на обоих
// путях, иначе два текста про одну сумму разойдутся в написании.
func offerFactsText(offer Offer) string {
	var b strings.Builder
	if model.NonEmpty(offer.Number) {
		b.WriteString("- Номер предложения: " + offer.Number + "\n")
	}
	if model.NonEmpty(offer.Client.Name) {
		b.WriteString("- Заказчик: " + offer.Client.Name + "\n")
	}
	if model.NonEmpty(offer.Client.Contact) {
		b.WriteString("- Контактное лицо: " + offer.Client.Contact + "\n")
	}
	if offer.Total > 0 {
		b.WriteString("- Итого: " + skills.FormatMoney(offer.Total) + " руб.\n")
	}
	if len(offer.Items) > 0 {
		b.WriteString("- Состав:\n")
		for _, it := range offer.Items {
			b.WriteString(fmt.Sprintf("  • %s — %s %s × %s = %s руб.\n",
				it.Name, formatQtyValue(it.Qty), it.Unit,
				skills.FormatMoney(it.Price), skills.FormatMoney(it.Sum)))
		}
	}
	if b.Len() == 0 {
		return "предложение ещё не рассчитано\n"
	}
	return b.String()
}
