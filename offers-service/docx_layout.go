package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"

	"mailers/offers/skills"
)

// Печать КП по загруженному макету .docx (2026-11-05).
//
// Макет — это собственный документ пользователя. В нём свои стили, шрифты,
// колонтитулы, таблицы и картинки, и терять это нельзя: «пользователь мог бы
// загрузить макет ТКП» означает «КП должно выглядеть как мой документ».
//
// Поэтому пакет собирается из байтов макета, и меняется в нём только текст
// внутри меток. Стили бланка, раздел, колонтитулы и связи — как в макете.
//
// Что заменяется:
//
//	[Наименование объекта]  → значение поля формы или служебное значение;
//	{Опиши систему}         → текст модели (как узел `prompt_text`);
//	{{object_name}}         → то же, что и `[]`.
//
// Пустое значение оставляет пустую строку: служебные скобки в готовом КП не
// должны оставаться ни при каких условиях.

// reWordParagraph — абзац целиком: документ — плоский список абзацев, и
// метка в Word может быть разрезана на несколько `<w:r>` (Word режет текст по
// правкам, языкам и форматированию). Поэтому разбор идёт по абзацу, а не по
// одному `<w:t>`.
var reWordParagraph = regexp.MustCompile(`<w:p(?:\s[^>]*)?>[\s\S]*?</w:p>`)
var reWordText = regexp.MustCompile(`<w:t(?:\s[^>]*)?>([^<]*)</w:t>`)

// layoutContext — что нужно печати по макету.
type layoutContext struct {
	// values — значения формы и служебные поля предложения.
	values map[string]string
	// prompt — текст модели для промта макета. nil — модели нет, блок молчит.
	prompt func(instruction string) string
}

// renderLayoutDocx собирает КП по макету.
//
// Ошибка чтения макета возвращается наверх: без документа экспорт невозможен,
// и молчаливый пустой файл хуже ошибки.
func renderLayoutDocx(layout []byte, lc layoutContext) ([]byte, error) {
	files, names, err := readZipEntries(layout)
	if err != nil {
		return nil, err
	}
	docXML, ok := files["word/document.xml"]
	if !ok {
		return nil, composeError{status: 500, text: "в макете нет word/document.xml"}
	}
	rendered := fillLayoutDocument(string(docXML), lc)
	files["word/document.xml"] = []byte(rendered)
	return zipFiles(files, names)
}

// fillLayoutDocument подставляет метки во все абзацы документа.
//
// Метки вне абзацев (колонтитулы, сноски) не трогаются: макет — образец
// вёрстки, а метки ставят в теле документа. Если человек поставил метку в
// колонтитул, она останется как есть — это лучше, чем молча её съесть.
func fillLayoutDocument(docXML string, lc layoutContext) string {
	return reWordParagraph.ReplaceAllStringFunc(docXML, func(paragraph string) string {
		return fillLayoutParagraph(paragraph, lc)
	})
}

// fillLayoutParagraph — один абзац с метками.
//
// Текст абзаца собирается из всех `<w:t>` подряд, метки ищутся в собранном
// тексте, результат пишется в ПЕРВЫЙ `<w:t>` абзаца, остальные очищаются.
// Вёрстка абзаца (стиль, выравнивание, отступы) при этом не меняется — она
// живёт в `<w:pPr>`, который не трогается.
func fillLayoutParagraph(paragraph string, lc layoutContext) string {
	texts := reWordText.FindAllStringSubmatch(paragraph, -1)
	if len(texts) == 0 {
		return paragraph
	}
	var joined strings.Builder
	for _, m := range texts {
		joined.WriteString(unescapeXML(m[1]))
	}
	plain := joined.String()
	tokens := scanLayoutTokens(plain, lc.values)
	if len(tokens) == 0 {
		return paragraph
	}
	replaced := applyLayoutTokens(plain, tokens, lc)
	// Пишем результат в первый w:t и очищаем остальные. Содержимое первого
	// может быть разбито на несколько прогонов — тогда просто его заменяем.
	first := true
	out := reWordText.ReplaceAllStringFunc(paragraph, func(tag string) string {
		if first {
			first = false
			return replaceWordText(tag, replaced)
		}
		return replaceWordText(tag, "")
	})
	return out
}

// replaceWordText подменяет содержимое `<w:t>` с сохранением его атрибутов.
func replaceWordText(tag, value string) string {
	open := strings.Index(tag, ">")
	close := strings.LastIndex(tag, "</w:t>")
	if open < 0 || close < 0 {
		return tag
	}
	head := tag[:open+1]
	if !strings.Contains(head, `xml:space=`) {
		// `xml:space="preserve"` обязателен: без него Word съедает краевые
		// пробелы, и «Итого: 1 000,00» превратится в «Итого: 1 000,00» без
		// отступа от подписи.
		head = strings.TrimSuffix(head, ">") + ` xml:space="preserve">`
	}
	return head + escapeXML(value) + tag[close:]
}

// applyLayoutTokens собирает текст абзаца из кусков между метками.
func applyLayoutTokens(text string, tokens []layoutToken, lc layoutContext) string {
	var sb strings.Builder
	prev := 0
	for _, token := range tokens {
		if token.Start < prev {
			continue
		}
		sb.WriteString(text[prev:token.Start])
		switch token.Kind {
		case layoutTokenPrompt:
			if lc.prompt != nil {
				sb.WriteString(lc.prompt(token.Inner))
			}
		default:
			sb.WriteString(layoutFieldValue(token.Inner, lc.values))
		}
		prev = token.End
	}
	sb.WriteString(text[prev:])
	return sb.String()
}

// unescapeXML — обратный к escapeXML для текста из документа.
func unescapeXML(s string) string {
	r := strings.NewReplacer("&lt;", "<", "&gt;", ">", "&quot;", `"`, "&amp;", "&")
	return r.Replace(s)
}

// layoutPromptText — текст модели для одного промта макета.
//
// Кэш — как у блоков «по промту»: один и тот же макет не должен давать разный
// текст при каждом скачивании. Ключ — хеш инструкции и данных предложения,
// поэтому правка состава перепишет текст, а повторное скачивание — нет.

// layoutPromptCache собирает тексты промтов макета на время одной сборки.
//
// Живёт структурой, а не замыканием: у печати четыре входа (предложение,
// карточки, ссылки, модель), и каждый новый добавил бы пятый параметр в
// общую функцию сборки документа.
type layoutPromptCache struct {
	// values — собранные тексты: ключ `gen.layout.<хеш промта>`.
	values map[string]string
	// previous — form_values предложения до сборки (там же кэш прошлых текстов).
	previous map[string]string
	// section — хеш данных предложения: под ним кэш имеет силу.
	section   string
	offer     Offer
	cards     []SystemCard
	refs      []skills.CardRef
	completed Completer
}

// layoutPromptKeyCache — ключ кэша одного промта макета.
//
// В ключ входит сам текст промта (хеш): администратор мог переписать фразу в
// макете, и старый текст к другой подсказке не годится.
func layoutPromptKeyCache(instruction string) string {
	return "gen.layout." + hashShort(instruction)
}

// hashShort — первые байты sha256 шестнадцатерично.
func hashShort(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

// text отдаёт текст промта: из кэша, если он свежий, иначе пишет модель.
func (c *layoutPromptCache) text(instruction string) string {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return ""
	}
	key := layoutPromptKeyCache(instruction)
	if v, ok := c.values[key]; ok {
		return v
	}
	if c.previous != nil && c.previous[genKeySection] == c.section {
		if v := documentText(strings.TrimSpace(c.previous[key])); v != "" {
			c.values[key] = v
			return v
		}
	}
	text := layoutPromptText(context.Background(), instruction, c.offer, c.cards, c.refs, c.completed)
	if text == "" {
		return ""
	}
	c.values[key] = text
	return text
}

// changed — тексты, которых не было в form_values предложения.
func (c *layoutPromptCache) changed() map[string]string {
	out := map[string]string{}
	for k, v := range c.values {
		if c.previous == nil || c.previous[k] != v {
			out[k] = v
		}
	}
	return out
}

// mergeFormValues дописывает новые значения к старым, не теряя прежних.
func mergeFormValues(values, add map[string]string) map[string]string {
	out := make(map[string]string, len(values)+len(add)+1)
	for k, v := range values {
		out[k] = v
	}
	for k, v := range add {
		out[k] = v
	}
	// Хеш данных — тот же, по которому решается свежесть текстов блоков:
	// иначе следующая сборка усомнится в текстах, которые только что записали.
	out[genKeySection] = add[genKeySection]
	return out
}

// offerSystemTitles — названия выбранных систем в порядке выбора.
//
// Из самого предложения, а не из карточек: метка макета должна печатать то,
// что менеджер выбрал, даже если раздел «Системы» ещё не перечитывали.
func offerSystemTitles(offer Offer) []string {
	out := make([]string, 0, len(offer.Systems))
	for _, s := range offer.Systems {
		title := strings.TrimSpace(s.SKU)
		if title == "" && s.NoteID > 0 {
			title = "система №" + strconv.FormatInt(s.NoteID, 10)
		}
		if title != "" {
			out = append(out, title)
		}
	}
	return out
}
func layoutPromptText(ctx context.Context, instruction string, offer Offer,
	cards []SystemCard, refs []skills.CardRef, c Completer) string {
	if c == nil || strings.TrimSpace(instruction) == "" {
		return ""
	}
	maxWords := genDefaultMaxWords
	// Тот же системный промпт, что у блока «по промту»: правила «пиши для
	// заказчика, не рассуждай» уже выведены там, и второй их набор разошёлся
	// бы с первым.
	system := skills.RenderBlockSystem(instruction, "", maxWords)
	user := buildBlockUserPrompt(offer, cards, refs)
	raw, err := c.Complete(ctx, system, user, blockMaxTokens(maxWords))
	if err != nil {
		return ""
	}
	return cleanBlockText(raw)
}
