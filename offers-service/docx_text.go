package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

// cardsForOffer — карточки тех систем, чьи материалы реально вошли в состав.
//
// Проверка по материалу, а не по SKU системы: в позиции попадает материал, а
// не система, и связь «система → состав» есть только в карточке. Отдаём
// карточки в порядке кэша; несвязанная система в документ не попадает, иначе
// КП получало бы описание того, чего в нём нет.
func cardsForOffer(cards []SystemCard, offer Offer) []SystemCard {
	if len(offer.Items) == 0 {
		return nil
	}
	inOffer := make(map[string]bool, len(offer.Items))
	for _, it := range offer.Items {
		inOffer[normalizeItemName(it.Name)] = true
	}
	out := make([]SystemCard, 0, len(cards))
	for _, card := range cards {
		if cardInOffer(card, inOffer) {
			out = append(out, card)
		}
	}
	return out
}

// cardInOffer — есть ли хоть один материал карточки в составе предложения.
//
// Поиск идёт по CompositionLinks, если они есть (кэш их хранит), иначе по
// названиям строк состава: название материала в позиции совпадает с ним.
func cardInOffer(card SystemCard, inOffer map[string]bool) bool {
	for _, link := range card.CompositionLinks {
		if !link.Found {
			continue
		}
		if inOffer[normalizeItemName(link.Name)] {
			return true
		}
	}
	for _, line := range card.Composition {
		if inOffer[normalizeItemName(line)] {
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
	return strings.TrimSpace(rc.values[genKeyTextByID(blockID, nodeIndex)])
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
	raw, err := c.Complete(ctx, system, user, maxWords*4)
	if err != nil {
		return "", err
	}
	return cleanBlockText(raw), nil
}

// cleanBlockText убирает то, что модель добавляет по привычке: маркеры кода и
// кавычки вокруг всего ответа.
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
	return text
}

// buildBlockUserPrompt — данные предложения текстом для модели.
//
// Ссылки идут как факты: модель может сказать «техдокументация приложена в
// конце», но адреса в текст не вписывает — иначе в документе будут адреса,
// которых нет в разделе ссылок.
func buildBlockUserPrompt(offer Offer, cards []SystemCard, refs []skills.CardRef) string {
	var b strings.Builder
	b.WriteString("ДАННЫЕ ПРЕДЛОЖЕНИЯ:\n")
	b.WriteString(offerFactsText(offer))
	if offer.FormValues != nil {
		line := func(label, key string) {
			if v := strings.TrimSpace(offer.FormValues[key]); v != "" {
				b.WriteString("- " + label + ": " + v + "\n")
			}
		}
		line("Объект", "object_name")
		line("Назначение", "object_purpose")
		line("Площадь", "object_area")
		line("Адресат", "customer_address")
	}
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
