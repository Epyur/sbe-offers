package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
	// items / cards — данные для меток-ТАБЛИЦ (2026-11-06). Модель таблицу не
	// пишет: на просьбе о таблице она рассуждает о группировке, и таблицей это
	// не становится. Строки собираются из того, что уже есть у предложения:
	// позиции для «предложения по материалам», характеристики карточек для
	// «сравнения вариантов».
	items func() [][]string
	cards func() []SystemCard
	// images — источник байтов картинки для метки «{картинка}» (2026-11-06).
	// Без него (и без ключа у карточки) метка молчит: картинка необязательна,
	// как и узел `image` в блоках формы.
	images ImageSource
	// offer — предложение: из него берётся картинка материала (PhotoKey позиции).
	offer Offer
	// layout — сборка пакета на время печати: сборщик картинок и его id.
	// Указатель, потому что заполняется во время печати, а контекст передаётся
	// значением; иначе картинка добавлялась бы в копию и терялась.
	layout *layoutRuntime
	// form — блоки формы для служебной метки `{{block:N}}` (шаг C, 2026-11-06).
	// nil — блоков нет, метка молчит.
	form func() OfferForm
	// refs / service — ссылки карточек и служебные поля: нужны блокам формы,
	// напечатанным по метке, и метке `{{ссылки: …}}`.
	refs    []skills.CardRef
	service map[string]string
	// selection — признак НАБОРА выбранных систем, а не карточек для модели
	// (2026-11-08). Печать блока с повтором сужает `cards` до одной системы, и
	// промт потерял бы, что систем в предложении несколько: вступление КП
	// 003/2026 описывало первую и молчало о второй. Пусто — признак считается
	// по `cards` и `offer.Systems` (так живут тесты и старые вызовы).
	selection string
	// systemTexts — сохранённые тексты системы (шаг B): метка `{{текст: …}}`
	// печатает их и модель не зовёт.
	systemTexts func(card SystemCard) map[string]string
	// systemText — дописать недостающий текст и сохранить его (2026-11-07).
	// Нет сохранённого — метка зовёт это, а не молчит: пустая таблица
	// `offers_system_text` делала метку мёртвой для всех живых систем.
	// nil в тестах и в сборках без БД: метка тогда печатает только сохранённое.
	systemText func(card SystemCard, block string) string
}

// selectionHandle — набор выбранных систем для ключей кэша текстов.
//
// Заданный признак главнее расчётного: печать блока с повтором сужает `cards`
// до одной системы, а промт вступления обязан знать весь комплект (2026-11-08).
func (lc layoutContext) selectionHandle() string {
	if lc.selection != "" {
		return lc.selection
	}
	if lc.offer.Systems != nil {
		return offerSelectionHandle(lc.offer)
	}
	return cardSelectionHandle(lc.cardList())
}

// layoutRuntime — состояние одной сборки по макету.
type layoutRuntime struct {
	media *docxMedia
	// numbering — списки, заведённые на время сборки: у каждого состава свой
	// numId, чтобы нумерация слоёв начиналась заново у каждой системы
	// (2026-11-08). nil — состав печатается одним сквозным списком.
	numbering *numberingPlan
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
	// Метки картинок наполняют этот сборщик; картинки дописываются в пакет
	// после тела — тогда id визуальных элементов читаются по всему пакету.
	// Сборщик живёт в структуре печати, а не в копии контекста: fillLayoutDocument
	// работает с копией, и наполнение копии потерялось бы.
	if lc.layout == nil {
		// Сборщик читает занятые id ВИЗУАЛЬНЫХ ЭЛЕМЕНТОВ САМОГО МАКЕТА, а не
		// бланка: у пользовательского документа свои номера, и картинка с id от
		// другого шаблона может совпасть с уже занятым — Word тогда молча не
		// покажет её (2026-11-07).
		lc.layout = &layoutRuntime{media: newDocxMedia(files)}
	}
	// План нумерации списков: у каждого состава свой numId (2026-11-08).
	// abstractNum 0 — десятичный список бланка (на него ссылается numId 1).
	// Заводим план, только если в макете ЕСТЬ numbering.xml: без него нашим
	// numId некуда положить определение, и список напечатался бы без номеров.
	// Тогда состав идёт общим списком бланка (numId 1), как раньше, — это
	// честнее пустой нумерации.
	if lc.layout.numbering == nil {
		if _, hasNumbering := files["word/numbering.xml"]; hasNumbering {
			lc.layout.numbering = newNumberingPlan(numberingAbstractDecimal)
		}
	}
	rendered := fillLayoutDocument(string(docXML), lc)
	// Адреса в ответе модели делаются кликабельными (2026-11-06). Раньше этот
	// шаг жил только в блочном пути, и при загруженном макете ссылки в КП не
	// могли стать ссылками ни при каком ответе: читатель видел длинный адрес
	// простым текстом.
	rendered, links := linkLayoutDocument(rendered)
	files["word/document.xml"] = []byte(rendered)
	if len(links) > 0 {
		// Отношения дописываются к тем, что уже есть в макете: замена файла
		// целиком оставила бы документ без колонтитулов (та же ошибка, что со
		// styles.xml в 2026-10-29).
		rels, err := relsXMLWithLinks(files["word/_rels/document.xml.rels"], links)
		if err != nil {
			return nil, err
		}
		if _, existed := files["word/_rels/document.xml.rels"]; !existed {
			names = append(names, "word/_rels/document.xml.rels")
		}
		files["word/_rels/document.xml.rels"] = rels
	}
	// Картинки метки «{картинка}»: файлы, связи и расширения — как у блока
	// формы. Без единой найденной картинки applyMedia молчит и пакет не меняется.
	if err := applyMedia(files, lc.layout.media); err != nil {
		return nil, err
	}
	// Определения наших списков (перезапуск нумерации состава) дописываются к
	// numbering.xml макета: файл не заменяется, только получает новые <w:num>.
	injectNumberingDefinitions(files, lc.layout.numbering)
	return zipFiles(files, names)
}

// linkLayoutDocument делает адреса макета кликабельными.
//
// Правило одно: кликабельным становится каждый адрес, попавший в текст абзаца,
// — набранный в самом макете или пришедший из ответа модели. Это проще
// правила блочного пути (там ссылки собираются из карточек и печатаются
// отдельным узлом) и не может разойтись с печатью: превращается ровно то, что
// видит читатель.
func linkLayoutDocument(docXML string) (string, []renderedLink) {
	paragraphs := reWordParagraph.FindAllString(docXML, -1)
	if len(paragraphs) == 0 {
		return docXML, nil
	}
	rewritten := make(map[int]string)
	links := make([]renderedLink, 0, 8)
	seen := make(map[string]bool, 8)
	for i, paragraph := range paragraphs {
		text := strings.Join(paragraphTexts(paragraph), "")
		if !strings.Contains(text, "http") {
			continue
		}
		newParagraph, found := linkParagraph(paragraph, text, seen)
		if len(found) == 0 {
			continue
		}
		rewritten[i] = newParagraph
		links = append(links, found...)
	}
	if len(rewritten) == 0 {
		return docXML, nil
	}
	return replaceLinkedParagraphs(docXML, paragraphs, rewritten), links
}

// linkParagraph переписывает абзац так, чтобы в нём была настоящая ссылка.
//
// Прогоны абзаца не трогаются: подменяется только `<w:t>` того прогона, в
// котором начинается адрес, остальные чищатся — ровно как в rebuildParagraphText.
// Абзац был собран нами (layoutAsParagraphs), поэтому прогон там один; если
// макет напечатал адрес своими прогонами, оформление получает первый из них.
func linkParagraph(paragraph, text string, seen map[string]bool) (string, []renderedLink) {
	matches := skills.FindURLs(text)
	if len(matches) == 0 {
		return "", nil
	}
	var sb strings.Builder
	var links []renderedLink
	prev := 0
	for _, m := range matches {
		url := text[m[0]:m[1]]
		sb.WriteString(escapeXML(text[prev:m[0]]))
		relID := placeholderRelID(url)
		sb.WriteString(`<w:hyperlink r:id="` + relID + `" w:history="1">`)
		sb.WriteString(`<w:r><w:t xml:space="preserve">` + escapeXML(url) + `</w:t></w:r>`)
		sb.WriteString(`</w:hyperlink>`)
		prev = m[1]
		if !seen[url] {
			seen[url] = true
			links = append(links, renderedLink{RelID: relID, URL: url})
		}
	}
	sb.WriteString(escapeXML(text[prev:]))
	return replaceLinkedText(paragraph, sb.String()), links
}

// replaceLinkedText кладёт готовую разметку в первый `<w:t>` абзаца, остальные
// чистит. Оформление (стиль, шрифт) остаётся от абзаца — как у обычного текста.
func replaceLinkedText(paragraph, markup string) string {
	first := true
	return reWordText.ReplaceAllStringFunc(paragraph, func(tag string) string {
		if first {
			first = false
			return replaceWordRaw(tag, markup)
		}
		return replaceWordText(tag, "")
	})
}

// replaceWordRaw — как replaceWordText, но значение уже разметка, а не текст.
func replaceWordRaw(tag, markup string) string {
	open := strings.Index(tag, ">")
	close := strings.LastIndex(tag, "</w:t>")
	if open < 0 || close < 0 {
		return tag
	}
	head := tag[:open+1]
	if !strings.Contains(head, `xml:space=`) {
		head = strings.TrimSuffix(head, ">") + ` xml:space="preserve">`
	}
	return head + markup + tag[close:]
}

// replaceLinkedParagraphs подставляет переписанные абзацы в XML.
//
// Переписанные абзацы адресуются по НОМЕРУ исходного абзаца, а не по счётчику
// изменённых: нумерация двух списков разошлась бы на первом же пропуске, и
// ссылка встала бы не в тот абзац (та же ловушка, что у replaceParagraphsInDocument).
func replaceLinkedParagraphs(docXML string, paragraphs []string, rewritten map[int]string) string {
	var sb strings.Builder
	last := 0
	searchFrom := 0
	for i, paragraph := range paragraphs {
		at := strings.Index(docXML[searchFrom:], paragraph)
		if at < 0 {
			return docXML
		}
		at += searchFrom
		searchFrom = at + len(paragraph)
		if replacement, ok := rewritten[i]; ok {
			sb.WriteString(docXML[last:at])
			sb.WriteString(replacement)
			last = searchFrom
		}
	}
	if last == 0 {
		return docXML
	}
	sb.WriteString(docXML[last:])
	return sb.String()
}

// fillLayoutDocument подставляет метки в тело документа.
//
// Метки вне абзацев (колонтитулы, сноски) не трогаются: макет — образец
// вёрстки, а метки ставят в теле документа. Если человек поставил метку в
// колонтитул, она останется как есть — это лучше, чем молча её съесть.
//
// Метка может пересекать границу абзаца (2026-11-06). Живой макет 003/2026
// набран так: `{{` в первом абзаце инструкции, `}}` — в девятом, потому что
// человек писал промт списком и нажимал Enter. Разбор по одному абзацу
// (раньше так и было) не находил метку ни в одном из них, и девять абзацев
// инструкции уходили клиенту литералом. Поэтому теперь:
//
//  1. метки ищутся в СКЛЕЙКЕ текста всех абзацев — так же, как их ищет
//     описание макета для администратора (layoutPlainText, form.go);
//  2. подстановка идёт по абзацам, но с общим списком найденных меток: часть
//     метки, попавшая в абзац, заменяется текстом модели, а остатки скобок в
//     соседних абзацах вычищаются.
func fillLayoutDocument(docXML string, lc layoutContext) string {
	paragraphs := reWordParagraph.FindAllString(docXML, -1)
	if len(paragraphs) == 0 {
		return docXML
	}
	spans := paragraphSpans(paragraphs)
	// Метки ищутся ровно по ТОЙ ЖЕ склейке, по которой размечены спаны:
	// позиции меток и границы абзацев должны жить в одной системе координат.
	tokens := scanLayoutTokens(spans.joined, lc.values)
	if len(tokens) == 0 {
		return docXML
	}
	// Какой текст получить каждому абзацу: подстановка раскладывается по
	// абзацам, в которых лежала метка, остальные абзацы не меняются.
	replaced := splitReplacedByParagraph(spans, tokens, lc)
	return replaceParagraphsInDocument(docXML, paragraphs, replaced)
}

// paragraphSpan — абзац и место его текста в склейке.
type paragraphSpan struct {
	// start / end — границы текста абзаца в общей склейке.
	start, end int
	// texts — содержимое `<w:t>` абзаца по порядку.
	texts []string
}

// paragraphTexts — склейка текста абзаца из его `<w:t>`.
func paragraphTexts(paragraph string) []string {
	matches := reWordText.FindAllStringSubmatch(paragraph, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, unescapeXML(m[1]))
	}
	return out
}

// paragraphSpans собирает склейку и место каждого абзаца в ней.
//
// Абзацы склеиваются ЧЕРЕЗ ПЕРЕВОД СТРОКИ, и это существенно (2026-11-06).
//
// Метка обязана находиться, даже если человек писал промт в девяти абзацах:
// склейка без разделителей находила её, но ПОРТИЛА текст инструкции — на
// живом КП 003/2026 модель получила «…входящих в состав решения;- преимущества
// применения…» и «…технического решенияОписания найди в базе знаний…»: слова
// склеились вплотную, а пункты списка потеряли друг друга. Инструкция,
// написанная списком, доходила до модели как одна строка без разделителей.
//
// Перевод строки внутри метки для сканера безопасен: в промт он попадает как
// обычный символ (`[^{}]` его не исключает), а чтобы края не «съедались»,
// промт дополнительно чистит разметку строк — см. promptInner и
// normalizePromptText.
func paragraphSpans(paragraphs []string) struct {
	texts  []string
	spans  []paragraphSpan
	joined string
} {
	var out struct {
		texts  []string
		spans  []paragraphSpan
		joined string
	}
	for _, paragraph := range paragraphs {
		texts := paragraphTexts(paragraph)
		out.texts = append(out.texts, strings.Join(texts, ""))
	}
	out.joined = strings.Join(out.texts, "\n")
	pos := 0
	for _, text := range out.texts {
		out.spans = append(out.spans, paragraphSpan{start: pos, end: pos + len(text)})
		// +1 — сам разделитель: он принадлежит склейке, но не абзацу.
		pos += len(text) + 1
	}
	return out
}

// splitReplacedByParagraph раскладывает подстановку меток по абзацам.
//
// Пустой элемент — абзац не меняется. Непустой — новый текст абзаца целиком;
// многострочный ответ модели печатается абзацами внутри одного___абзаца метки
// (layoutAsParagraphs), поэтому возвращается уже готовый XML.
func splitReplacedByParagraph(spans struct {
	texts  []string
	spans  []paragraphSpan
	joined string
}, tokens []layoutToken, lc layoutContext) []paragraphChange {
	// Что подставить вместо каждой метки — один раз на метку, а не на абзац:
	// иначе одна инструкция вызовет модель столько раз, сколько абзацев она
	// занимает.
	replacement := make(map[int]string, len(tokens))
	// imageMarkup — разметка картинок, которые печатаются разметкой абзаца.
	// Отдельно от replacement: картинка — не текст абзаца, и склеивать одно с
	// другим в одном значении нельзя (порядок склейки потерялся бы).
	imageMarkup := make(map[int]string, len(tokens))
	// serviceMarkup — готовая разметка служебных меток (шаг C): таблица,
	// список, блок формы. Печатается как XML, а не как текст абзаца.
	serviceMarkup := make(map[int]string, len(tokens))
	for i, token := range tokens {
		switch token.Kind {
		case layoutTokenImage:
			spec, _ := parseLayoutImageSpec(token.Inner)
			// В тексте абзаца метка картинки заменяется ПУСТОЙ строкой: сама
			// картинка печатается разметкой абзаца (attachLayoutImages), а не
			// текстом. Так «Разрез системы: {картинка}» даёт подпись и
			// картинку, а не строку с разметкой картинки.
			replacement[i] = ""
			imageMarkup[i] = layoutImageMarkup(spec, lc)
		case layoutTokenPrompt:
			// Таблица проверяется ДО модели (2026-11-06): раньше `lc.prompt`
			// вызывался первым, ответ выбрасывался, и `layoutTableFor` подменял
			// его таблицей из данных. Один лишний вызов модели на каждую
			// сборку — на длинном промте это и есть те минуты ожидания.
			if table := layoutTableFor(token.Inner, lc); table != nil {
				replacement[i] = tableMarkdown(table)
				continue
			}
			if lc.prompt != nil {
				replacement[i] = lc.prompt(token.Inner)
			}
		case layoutTokenBlock, layoutTokenSystem, layoutTokenLinks,
			layoutTokenSpecs, layoutTokenText, layoutTokenMaterial:
			// Служебные метки (шаг C, 2026-11-06): печатают данные предложения
			// и карточки, модель не зовут вовсе. Их разметка — готовый XML
			// (таблица, список, картинка), поэтому она идёт полем `blockMarkup`,
			// а не текстом: в тексте `<w:tbl>` был бы экранирован и клиент
			// увидел бы теги. Метка в тексте абзаца заменяется пустой строкой.
			replacement[i] = ""
			if markup := renderLayoutService(token, lc, layoutServiceRow{}); markup != "" {
				serviceMarkup[i] = markup
			}
		default:
			replacement[i] = layoutFieldValue(token.Inner, lc.values)
		}
	}

	out := make([]paragraphChange, len(spans.spans))
	for idx, span := range spans.spans {
		newText, changed := replaceInSpan(spans.joined, span, tokens, replacement)
		out[idx] = paragraphChange{text: newText, changed: changed}
	}
	// Картинка занимает абзац целиком: в живом макете метка стоит отдельной
	// строкой в описании системы. Абзац, где метка картинки была единственным
	// содержимым, печатается картинкой, а не пустым абзацем с подстановкой.
	attachLayoutImages(out, spans.texts, spans.spans, tokens, imageMarkup)
	// Служебные метки печатаются разметкой абзаца: таблица, список состава,
	// блок формы — это XML, а не текст (в `<w:t>` он был бы экранирован).
	attachServiceMarkup(out, spans.spans, tokens, serviceMarkup)
	return out
}

// attachServiceMarkup кладёт готовую разметку служебной метки вместо абзаца.
//
// Правила те же, что у картинки (attachLayoutImages): метка отдельной строкой
// печатается вместо абзаца; метка посреди текста даёт разметку ПОСЛЕ текста
// абзаца, и текст сохраняется. Пустая разметка — метка молчит, абзац остаётся
// пустым (иначе на его месте остался бы литерал метки).
func attachServiceMarkup(changes []paragraphChange, spans []paragraphSpan,
	tokens []layoutToken, markup map[int]string) {
	for i, token := range tokens {
		value, ok := markup[i]
		if !ok || !isServiceKind(token.Kind) {
			continue
		}
		idx := spanIndexAt(spans, token.Start)
		if idx < 0 {
			continue
		}
		if strings.TrimSpace(changes[idx].text) == "" {
			changes[idx].image = value
			changes[idx].changed = true
			continue
		}
		changes[idx].imageAfter = value
		changes[idx].changed = true
	}
}

// attachLayoutImages переносит разметку картинки из текста абзаца в его поле.
//
// Метка картинки в макете стоит отдельной строкой, поэтому абзац, состоящий
// только из неё, печатается картинкой: пустой абзац рядом с подстановкой
// читался бы как сбой. Метка посреди текста даёт картинку в потоке — тогда
// картинка ставится ВСЛЕД за абзацем, и текст абзаца сохраняется.
func attachLayoutImages(changes []paragraphChange, texts []string,
	spans []paragraphSpan, tokens []layoutToken, imageMarkup map[int]string) {
	for i, token := range tokens {
		markup, ok := imageMarkup[i]
		if token.Kind != layoutTokenImage || !ok || markup == "" {
			continue
		}
		idx := spanIndexAt(spans, token.Start)
		if idx < 0 {
			continue
		}
		if strings.TrimSpace(changes[idx].text) == "" && strings.TrimSpace(texts[idx]) == token.Raw {
			// Абзац целиком был меткой: он и становится картинкой.
			changes[idx].image = markup
			changes[idx].changed = true
			continue
		}
		// Метка стояла в тексте: текст абзаца печатается, картинка идёт следом.
		changes[idx].imageAfter = markup
		changes[idx].changed = true
	}
}

// spanIndexAt — номер абзаца, в котором лежит позиция склейки.
func spanIndexAt(spans []paragraphSpan, pos int) int {
	for i, span := range spans {
		if pos >= span.start && pos < span.end {
			return i
		}
	}
	return -1
}

// layoutImageMarkup — разметка картинки для подстановки.
//
// Пустая строка — картинки нет: метка молчит, как и узел `image` без файла.
// Разметка добавляет файл в lc.media, и он уходит в пакет в renderLayoutDocx.
func layoutImageMarkup(spec layoutImageSpec, lc layoutContext) string {
	if lc.images == nil || lc.layout == nil || lc.layout.media == nil {
		return ""
	}
	img, ok := layoutImageCard(spec, lc)
	if !ok {
		return ""
	}
	data, err := lc.images.Get(context.Background(), img.ImageKey)
	if err != nil || len(data) == 0 {
		// Картинка необязательна: документ важнее.
		return ""
	}
	ext, contentType := imageExt(img.ImageName)
	widthCm := layoutCardImageWidthCm
	cx := int64(widthCm * 360000)
	cy := int64(float64(widthCm) * 0.72 * 360000)
	drawing := lc.layout.media.addAnchored(data, ext, contentType, cx, cy,
		imageWrapSpec{side: "right"}, "center")
	if drawing == "" {
		return ""
	}
	return imageParagraphXML(drawing)
}

// imageParagraphXML — картинка прогоном ВНУТРИ абзаца.
//
// Прогон `<w:r>` не может быть соседом абзаца: в OOXML он живёт только внутри
// `<w:p>`, и голый прогон Word молча не показывает. Раньше метка карточки
// склеивала пустой абзац и прогон рядом — картинка системы не вставлялась
// (живая жалоба 2026-11-07). Одна точка сборки на обе метки: иначе они
// разойдутся, и одну из них снова никто не заметит.
func imageParagraphXML(drawing string) string {
	return `<w:p><w:pPr><w:pStyle w:val="` + styleNormal + `"/><w:ind w:firstLine="0"/>` +
		`<w:jc w:val="center"/></w:pPr>` + drawing + `</w:p>`
}

// layoutCardImageWidthCm — ширина картинки в макете: 10 см.
//
// Уже, чем у блока формы (12 см): в макете рядом с картинкой идёт текст в два
// столбца или подпись автора оформления, и 12 см сдвигало бы набор.
const layoutCardImageWidthCm = 10

// layoutImageCard — карточка, чью картинку просит метка.
//
// Номер системы считается в порядке выбора (cardsForOffer даёт тот же порядок,
// что и остальная печать). Материал берётся из позиций предложения: картинка
// материала — это `PhotoKey` позиции.
func layoutImageCard(spec layoutImageSpec, lc layoutContext) (SystemCard, bool) {
	if spec.of == imageOfMaterial {
		// Картинка материала берётся из позиций: у материала в КП нет своего
		// списка карточек, а PhotoKey позиции заполняется при синхронизации.
		return layoutImageMaterial(spec, lc)
	}
	if lc.cards == nil {
		return SystemCard{}, false
	}
	cards := lc.cards()
	if len(cards) == 0 {
		return SystemCard{}, false
	}
	idx := spec.index
	if idx <= 0 || idx > len(cards) {
		idx = 1
	}
	card := cards[idx-1]
	return card, card.ImageKey != ""
}

// layoutImageMaterial — картинка материала из позиции предложения.
//
// Позиции — в самом предложении (lc.offer.Items), а не в карточках: `PhotoKey`
// позиции заполняется при синхронизации материалов с Базой знаний. Пустой
// список или позиция без ключа — картинки нет, метка молчит.
func layoutImageMaterial(spec layoutImageSpec, lc layoutContext) (SystemCard, bool) {
	items := lc.offer.Items
	if len(items) == 0 {
		return SystemCard{}, false
	}
	idx := spec.index
	if idx <= 0 || idx > len(items) {
		idx = 1
	}
	item := items[idx-1]
	if strings.TrimSpace(item.PhotoKey) == "" {
		return SystemCard{}, false
	}
	return SystemCard{ImageKey: item.PhotoKey, ImageName: item.ImageName, Title: item.Name}, true
}

// layoutTableKind — какую таблицу просит метка макета.
type layoutTableKind int

const (
	layoutTableNone    layoutTableKind = iota // таблица не просится
	layoutTableItems                          // предложение по материалам
	layoutTableCompare                        // сравнение вариантов решений
)

// layoutTableKindOf — вид таблицы по тексту инструкции (2026-11-06).
//
// Два смысла метки `{{Вставь таблицу …}}` требуют РАЗНЫХ данных:
//
//	«предложение по материалам, ЕКН, цена, объём, стоимость» — позиции КП;
//	«сравнение вариантов решений / сравни системы»          — характеристики
//	                                                          карточек.
//
// Сравнение проверяется ПЕРВЫМ: промт сравнения часто упоминает и материалы
// («сравни варианты по материалам»), и проверка по слову «материала» увела бы
// его к позициям.
func layoutTableKindOf(instruction string) layoutTableKind {
	lower := strings.ToLower(instruction)
	if !strings.Contains(lower, "таблиц") && !strings.Contains(lower, "table") {
		return layoutTableNone
	}
	for _, marker := range []string{"сравн", "вариант", "альтернатив", "compare"} {
		if strings.Contains(lower, marker) {
			return layoutTableCompare
		}
	}
	return layoutTableItems
}

// layoutTableFor — строки таблицы для метки: nil, если метка не про таблицу
// или данных для неё нет.
//
// nil возвращается и тогда, когда модель должна написать текст как обычно:
// «таблица сравнения» при одной системе — не сравнение, и печатать таблицу из
// одного столбца нельзя (см. compareSystemRows).
func layoutTableFor(instruction string, lc layoutContext) [][]string {
	switch layoutTableKindOf(instruction) {
	case layoutTableItems:
		if lc.items == nil {
			return nil
		}
		return lc.items()
	case layoutTableCompare:
		if lc.cards == nil {
			return nil
		}
		return layoutCompareTable(lc.cards())
	}
	return nil
}

// layoutOfferTable — таблица предложения по позициям.
//
// Колонки повторяют то, что просит живой макет 003/2026: наименование, ЕКН,
// единица, количество, цена, сумма. Пустые колонки не печатаются: колонка
// «ЕКН» без единого номера выглядит как потеря данных, а не как «нет номеров».
// Внизу — строка «Итого», если позиции есть.
func layoutOfferTable(offer Offer) [][]string {
	if len(offer.Items) == 0 {
		return nil
	}
	withEkn := false
	for _, item := range offer.Items {
		if strings.TrimSpace(item.EKN) != "" {
			withEkn = true
			break
		}
	}
	header := []string{"№", "Наименование материала"}
	if withEkn {
		header = append(header, "ЕКН")
	}
	header = append(header, "Единица поставки", "Объём поставки", "Цена за единицу, руб", "Стоимость, руб")

	rows := [][]string{header}
	for i, item := range offer.Items {
		row := []string{strconv.Itoa(i + 1), item.Name}
		if withEkn {
			row = append(row, item.EKN)
		}
		row = append(row,
			item.Unit,
			formatQtyValue(item.Qty),
			formatMoney(item.Price),
			formatMoney(item.Sum),
		)
		rows = append(rows, row)
	}
	// Итог — внизу, в колонке стоимости: читатель ищет сумму там же, где суммы
	// строк, а не в отдельной строке под таблицей.
	total := make([]string, len(header))
	total[0] = ""
	total[1] = "Итого"
	for i := 2; i < len(total); i++ {
		total[i] = ""
	}
	total[len(total)-1] = formatMoney(offer.Total)
	rows = append(rows, total)
	return rows
}

// layoutCompareTable — таблица сравнения систем по характеристикам карточек.
//
// Обёртка над compareSystemRows: та возвращает nil при одной системе, и это
// правильно — «сравнение» из одного столбца обманывает читателя.
func layoutCompareTable(cards []SystemCard) [][]string {
	return compareSystemRows(cards)
}

// tableCell готовит ячейку к markdown-таблице.
//
// Переводы строки внутри названия — не редкость: в живом КП 003/2026 так
// записаны утеплители («Утеплитель ХPS Carbon Prof толщиной 100 мм, на
// глубину промерзания»), и пришли они из таблицы менеджера. Markdown-строка
// таблицы не может содержать перевод строки: таблица рвалась на две, и вместо
// 41 позиции в документе было пять обрывков (проверено на живой выгрузке).
// Черта внутри названия — тоже ломает разбор, её заменяем на косую.
func tableCell(cell string) string {
	cleaned := strings.Join(strings.Fields(strings.ReplaceAll(cell, "|", "/")), " ")
	return cleaned
}

// tableMarkdown печатает строки таблицей Markdown.
//
// Не ради формата: `layoutAnswer` уже умеет превращать markdown-таблицу в
// настоящий `<w:tbl>` с вёрсткой документа — один разбор на две дороги.
// Здесь только сборка строки; печать остаётся в одном месте.
func tableMarkdown(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, row := range rows {
		sb.WriteString("|")
		for _, cell := range row {
			sb.WriteString(" ")
			sb.WriteString(tableCell(cell))
			sb.WriteString(" |")
		}
		sb.WriteString("\n")
		if i == 0 {
			// Разделитель шапки: столько же колонок, сколько в первой строке.
			sb.WriteString("|")
			for range row {
				sb.WriteString("---|")
			}
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// paragraphChange — новый текст абзаца и признак, что он менялся.
//
// Признак нужен отдельно от текста: абзац, который ЦЕЛИКОМ принадлежал
// продолжению метки, должен стать ПУСТЫМ, и пустая строка здесь означает
// «менялся», а не «не трогать».
//
// image — готовая разметка картинки (2026-11-06): метка «{картинка}» — это не
// текст. Пустой текст при непустой картинке значит «текста нет, картинка есть»,
// поэтому признак `changed` обязателен и здесь.
type paragraphChange struct {
	text string
	// image — картинка вместо всего абзаца (метка стояла отдельной строкой).
	image string
	// imageAfter — картинка после текста абзаца (метка стояла в тексте).
	imageAfter string
	changed    bool
}

// replaceInSpan собирает новый текст одного абзаца.
//
// Правило простое: идём по тексту абзаца слева направо и на каждой позиции
// смотрим, не начинается ли здесь метка.
//
//   - метка начинается в этом абзаце — пишем подстановку и перепрыгиваем на
//     её конец (или на конец абзаца, если она кончается дальше);
//   - позиция попала ВНУТРЬ метки, начавшейся раньше (хвост `}}` в последнем
//     абзаце многоабзацной инструкции) — эти символы выбрасываются: остаток
//     метки не текст документа;
//   - обычный символ — пишется как есть.
//
// Второе значение — менялся ли абзац: нетронутый абзац печатается исходным
// XML со всей его вёрсткой.
func replaceInSpan(joined string, span paragraphSpan, tokens []layoutToken, replacement map[int]string) (string, bool) {
	var sb strings.Builder
	changed := false
	pos := span.start
	for pos < span.end {
		i, token, inside := tokenAt(tokens, pos)
		if token == nil {
			// Выводим ЦЕЛЫЙ символ: кириллица в UTF-8 занимает два байта, и
			// побайтовый шаг порвал бы её на половинки. Через границу абзаца
			// символ не выходит: спаны режутся по её концам.
			r, size := utf8.DecodeRuneInString(joined[pos:])
			if pos+size > span.end {
				// Последний символ абзаца не влез целиком — этого не должно
				// быть (спаны строятся по байтам текста), но лучше его
				// пропустить, чем напечатать половину буквы.
				break
			}
			sb.WriteRune(r)
			pos += size
			continue
		}
		changed = true
		if inside {
			// Хвост метки, начавшейся в предыдущем абзаце: его символы —
			// остаток инструкции, а не текст документа.
			pos = minInt(token.End, span.end)
			continue
		}
		sb.WriteString(replacement[i])
		pos = minInt(token.End, span.end)
	}
	if !changed {
		return "", false
	}
	return sb.String(), true
}

// tokenAt — метка, которая начинается в pos или накрывает её.
//
// inside = true, если положение оказалось строго внутри метки: тогда её
// символы — остаток инструкции, а не текст для клиента.
func tokenAt(tokens []layoutToken, pos int) (int, *layoutToken, bool) {
	for i := range tokens {
		token := &tokens[i]
		if pos >= token.End {
			continue
		}
		if pos < token.Start {
			return 0, nil, false
		}
		return i, token, pos > token.Start
	}
	return 0, nil, false
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// replaceParagraphsInDocument подставляет новые тексты абзацев в XML.
//
// Абзац, у которого текст не менялся, остаётся байт-в-байт: вёрстка, прогоны
// и служебные пометки Word в нём не трогаются.
func replaceParagraphsInDocument(docXML string, paragraphs []string, replaced []paragraphChange) string {
	var sb strings.Builder
	last := 0
	searchFrom := 0
	for i, paragraph := range paragraphs {
		at := strings.Index(docXML[searchFrom:], paragraph)
		if at < 0 {
			// Абзац не находится байтово (не должно случаться) — оставляем
			// исходный документ, чем портим разметку.
			return docXML
		}
		at += searchFrom
		if i >= len(replaced) || !replaced[i].changed {
			searchFrom = at + len(paragraph)
			continue
		}
		sb.WriteString(docXML[last:at])
		change := replaced[i]
		if change.image != "" {
			sb.WriteString(change.image)
		} else {
			sb.WriteString(rebuildParagraph(paragraph, change.text))
			if change.imageAfter != "" {
				sb.WriteString(change.imageAfter)
			}
		}
		last = at + len(paragraph)
		searchFrom = last
	}
	if last == 0 {
		return docXML
	}
	sb.WriteString(docXML[last:])
	return sb.String()
}

// rebuildParagraph печатает абзац с новым текстом.
//
// Многострочный ответ модели печатается НЕСКОЛЬКИМИ абзацами (2026-11-06).
//
// Раньше весь текст писался в один `<w:t>`, а переводы строк внутри `<w:t>`
// Word не показывает: заголовки и абзацы слипались в один полотнище-абзац, и
// «оформления» в документе не было вовсе. Теперь каждая непустая строка
// ответа становится отдельным абзацем с вёрсткой метки  её `<w:pPr>` (стиль,
// выравнивание, отступ) копируется каждому.
//
// Строки модели-заголовки (`## Заголовок`) уходят без знаков разметки (чистка
// — `stripMarkdownHeadings`), но остаются отдельными абзацами: читаются как
// подзаголовки, даже если макет своим стилем их не выделит.
// rebuildParagraph печатает абзац с новым текстом.
//
// Многострочный ответ модели печатается НЕСКОЛЬКИМИ абзацами (2026-11-06).
//
// Раньше весь текст писался в один `<w:t>`, а переводы строк внутри `<w:t>`
// Word не показывает: заголовки и абзацы слипались в один полотнище-абзац, и
// «оформления» в документе не было вовсе. Теперь каждая непустая строка
// ответа становится отдельным абзацем с вёрсткой метки — её `<w:pPr>` (стиль,
// выравнивание, отступ) копируется каждому.
//
// Ответ может содержать markdown-таблицу: её печатает `layoutAnswer` настоящей
// таблицей Word, остальное — абзацами.
func rebuildParagraph(paragraph, text string) string {
	return layoutAnswer(paragraph, text)
}

// layoutAnswer печатает ответ модели с учётом таблицы.
//
// Модель по промту таблицы возвращает её разметкой Markdown (так просит
// tableDoc в skills/prompt.go). Разметку нельзя печатать текстом: в живом КП
// 003/2026 клиент увидел бы строки с вертикальными чертами. Здесь ответ
// раскладывается на куски: обычный текст — абзацами, таблица — настоящим
// `<w:tbl>` вёрстки документа (tableXML из docx.go).
//
// Пустая таблица (шапка без строк) печатается как текст: пустой `<w:tbl>` в
// Word выглядит сломанным, и лучше показать то, что пришло.
func layoutAnswer(paragraph, text string) string {
	blocks := splitAnswerTables(text)
	if len(blocks) == 1 && blocks[0].table == nil {
		// Таблицы нет — прежнее поведение, ни одного лишнего шага.
		if lines := layoutLines(text); len(lines) > 1 {
			return layoutAsParagraphs(paragraph, lines)
		}
		return rebuildParagraphText(paragraph, text)
	}
	var sb strings.Builder
	for _, block := range blocks {
		switch {
		case block.table != nil:
			sb.WriteString(tableXML(block.table))
		case strings.TrimSpace(block.text) != "":
			sb.WriteString(layoutAsParagraphs(paragraph, layoutLines(block.text)))
		}
	}
	if sb.Len() == 0 {
		return rebuildParagraphText(paragraph, text)
	}
	return sb.String()
}

// answerBlock — кусок ответа: либо текст, либо готовая таблица.
type answerBlock struct {
	text  string
	table [][]string
}

// rebuildParagraphText пишет текст в первый `<w:t>` абзаца, остальные чистит.
//
// Одна строка тоже идёт через общий конвейер (plainText + runsXML): без него
// значение поля формы и короткий ответ модели печатались бы без надстрочных
// единиц, а многострочный — с ними (2026-11-06).
func rebuildParagraphText(paragraph, text string) string {
	baseRPr := runPropertiesInner(firstRunProperties(paragraph))
	markup := runsXML(plainText(text), baseRPr)
	first := true
	return reWordText.ReplaceAllStringFunc(paragraph, func(tag string) string {
		if first {
			first = false
			return replaceWordRaw(tag, markup)
		}
		return replaceWordText(tag, "")
	})
}

// splitAnswerTables делит ответ модели на текст и markdown-таблицы.
func splitAnswerTables(text string) []answerBlock {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var out []answerBlock
	var textLines []string
	flush := func() {
		if len(textLines) > 0 {
			out = append(out, answerBlock{text: strings.Join(textLines, "\n")})
			textLines = nil
		}
	}
	for i := 0; i < len(lines); {
		if !isMarkdownRow(lines[i]) {
			textLines = append(textLines, lines[i])
			i++
			continue
		}
		rows, next := collectMarkdownTable(lines, i)
		flush()
		out = append(out, answerBlock{table: rows})
		i = next
	}
	flush()
	return out
}

// isMarkdownRow — строка таблицы Markdown: начинается и кончается чертой.
func isMarkdownRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	return len(trimmed) >= 2 && strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") &&
		strings.Count(trimmed, "|") >= 3
}

// markdownCells разбивает строку таблицы на ячейки.
func markdownCells(line string) []string {
	trimmed := strings.Trim(strings.TrimSpace(line), "|")
	parts := strings.Split(trimmed, "|")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, strings.TrimSpace(part))
	}
	return out
}

// isMarkdownSeparator — строка-разделитель `|---|---|`.
func isMarkdownSeparator(line string) bool {
	for _, cell := range markdownCells(line) {
		if strings.Trim(cell, "-: ") != "" {
			return false
		}
	}
	return true
}

// collectMarkdownTable читает строки таблицы начиная с i.
//
// Возвращает строки (первая — шапка, разделитель выброшен) и индекс, с
// которого продолжается текст.
func collectMarkdownTable(lines []string, i int) ([][]string, int) {
	var rows [][]string
	for ; i < len(lines) && isMarkdownRow(lines[i]); i++ {
		if isMarkdownSeparator(lines[i]) {
			continue
		}
		rows = append(rows, markdownCells(lines[i]))
	}
	return rows, i
}

// layoutLineKind — вид строки ответа модели.
//
// Модель отвечает одним текстом, но читать его нужно тремя способами: абзац
// тела, заголовок части и пункт списка. До 2026-11-06 у печати был один вид —
// абзац, и в живом КП 003/2026 заголовки, написанные моделью, читались как
// тело, а списки — как подряд идущие строки.
//
// Вид решается ЗДЕСЬ и только здесь: печать (layoutAsParagraphs) разбирает
// ответ через layoutLines, и второй разбор структуры не заводится.
type layoutLineKind int

const (
	layoutLineBody layoutLineKind = iota
	layoutLineHeading
	layoutLineBullet
	layoutLineNumbered
)

// layoutLine — строка ответа с видом.
type layoutLine struct {
	kind layoutLineKind
	text string
}

// layoutLines разбирает ответ модели на строки, убирая разметку Markdown.
//
// Заголовок — строка `## Текст` (по просьбе макета: «структурные разделы
// помечай как заголовки»). Нумерованный пункт — только «1. текст» или «1) текст»
// с точкой/скобкой сразу после цифры: «[7]» в тексте БИБЛИОГРАФИЧЕСКАЯ ссылка,
// а не пункт, и в список она попадать не должна (правило из живого КП: в тексте
// стоят [7], [8], [3], [5], [2], [4]).
//
// Строка из одних знаков разметки («##», «**») выбрасывается: в живом ответе
// был такой хвост, и пустой абзац в документе читается как сбой.
func layoutLines(text string) []layoutLine {
	var out []layoutLine
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if line, ok := layoutLineOf(raw); ok {
			out = append(out, line)
		}
	}
	return out
}

// layoutLineOf — вид и текст одной строки. Ложь — печатать нечего.
func layoutLineOf(raw string) (layoutLine, bool) {
	trimmed := strings.TrimSpace(raw)
	kind := layoutLineBody
	if m := reMarkdownHeading.FindStringSubmatch(trimmed); m != nil {
		trimmed = strings.TrimSpace(m[1])
		kind = layoutLineHeading
	} else if m := reMarkdownBullet.FindStringSubmatch(trimmed); m != nil {
		trimmed = strings.TrimSpace(m[1])
		kind = layoutLineBullet
	} else if m := reListNumber.FindStringSubmatch(trimmed); m != nil {
		trimmed = strings.TrimSpace(m[1])
		kind = layoutLineNumbered
	}
	trimmed = stripInlineMarkdown(trimmed)
	// От строки остались только знаки разметки — печатать нечего.
	if strings.TrimSpace(strings.Trim(trimmed, `#*_-–—.,;: `)) == "" {
		return layoutLine{}, false
	}
	return layoutLine{kind: kind, text: trimmed}, true
}

// reListNumber — начало нумерованного пункта: «1. » или «1) ».
//
// Требуется знак после цифры: «5520 м2» и «[7]» пунктами не становятся, а
// «20 158 633,08» не начинается с цифры, после которой стоит точка с пробелом.
var reListNumber = regexp.MustCompile(`^\d{1,3}[.)]\s+(.+)$`)

// layoutAsParagraphs печатает строки отдельными абзацами с вёрсткой исходного.
//
// Исходный абзац макета состоит из `<w:pPr>` (стиль и правила) и прогонов.
// Первая строка наследует `<w:pPr>` как есть, остальные получают его копию —
// так стиль абзаца макета действует на весь ответ модели, а не на первую
// строку. Пустая строка исходного абзаца (он мог быть пустым) не теряется:
// между строками абзацы не вставляются, потому что пустая строка в тексте
// модели — разделитель абзацев, а не пустой абзац документа.
//
// Текст строки идёт через ТОТ ЖЕ конвейер, что и у блоков формы (2026-11-06):
// plainText (вики-ссылки, разметка, слипшиеся слова) + runsXML (надстрочные
// единицы). До этой правки здесь стоял escapeXML, и живой КП 003/2026 печатал
// «5520 м2» обычной двойкой, а `[[…]]` из карточки дошли бы до заказчика со
// скобками — при том, что в блочном пути всё это работало. Правило 2026-11-05:
// конвейер текста один, иначе два пути печати расходятся молча.
func layoutAsParagraphs(paragraph string, lines []layoutLine) string {
	pPr, baseRPr := layoutParagraphStyle(paragraph)
	var sb strings.Builder
	for _, line := range lines {
		sb.WriteString(layoutLineXML(line, pPr, baseRPr))
	}
	return sb.String()
}

// layoutLineXML — разметка одной строки ответа по её виду.
//
// Заголовок получает стиль бланка «heading 1» и выравнивание по центру —
// оформление документов экосистемы (см. headingXML). Список — нумерацию
// бланка: маркер и номер ставит Word, в тексте их нет.
func layoutLineXML(line layoutLine, pPr, baseRPr string) string {
	switch line.kind {
	case layoutLineHeading:
		return headingXMLOpts(plainText(line.text), 2, paraOpts{})
	case layoutLineBullet:
		return layoutListItemXML(plainText(line.text), styleListBullet, baseRPr)
	case layoutLineNumbered:
		return layoutListItemXML(plainText(line.text), styleListNumbered, baseRPr)
	default:
		return `<w:p>` + pPr + runsXML(plainText(line.text), baseRPr) + `</w:p>`
	}
}

// layoutListItemXML — пункт списка с оформлением макета (шрифт строки).
//
// Отступ первой строки снят: он есть у пункта списка по определению, и красная
// строка рядом с маркером выглядит сдвинутой.
func layoutListItemXML(text, numID, baseRPr string) string {
	return `<w:p><w:pPr><w:pStyle w:val="` + styleNormal + `"/><w:ind w:firstLine="0"/>` +
		`<w:spacing w:line="360" w:lineRule="auto"/>` +
		`<w:numPr><w:ilvl w:val="0"/><w:numId w:val="` + numID + `"/></w:numPr></w:pPr>` +
		runsXML(text, baseRPr) + `</w:p>`
}

// layoutParagraphStyle — правила абзаца и оформление прогонов исходного абзаца.
//
// У абзаца нет правил — синтезируется минимум, чтобы Word не считал его
// отдельным «Normal по умолчанию».
func layoutParagraphStyle(paragraph string) (pPr, baseRPr string) {
	pPr = reParagraphPr.FindString(paragraph)
	if strings.TrimSpace(pPr) == "" {
		pPr = `<w:pPr><w:jc w:val="both"/></w:pPr>`
	}
	// Оформление прогонов макета (шрифт, размер) действует на все строки.
	baseRPr = runPropertiesInner(firstRunProperties(paragraph))
	return pPr, baseRPr
}

// runPropertiesInner — содержимое `<w:rPr>…</w:rPr>` без самого тега.
//
// runsXML оборачивает каждый прогон в свой `<w:rPr>`, поэтому внешние теги
// исходного прогона нужно снять: иначе получилось бы `<w:rPr><w:rPr>…` и Word
// отказался бы открывать файл. Пустая строка — у прогона макета своих правил
// нет, копировать нечего.
func runPropertiesInner(rPr string) string {
	if strings.TrimSpace(rPr) == "" {
		return ""
	}
	inner := strings.TrimSpace(rPr)
	inner = strings.TrimPrefix(inner, "<w:rPr>")
	inner = strings.TrimSuffix(inner, "</w:rPr>")
	return strings.TrimSpace(inner)
}

// reParagraphPr — свойства абзаца (`<w:pPr>…</w:pPr>`) исходного абзаца.
var reParagraphPr = regexp.MustCompile(`<w:pPr>[\s\S]*?</w:pPr>`)

// reRunProperties — свойства прогона (`<w:rPr>…</w:rPr>`).
var reRunProperties = regexp.MustCompile(`<w:rPr>[\s\S]*?</w:rPr>`)

// firstRunProperties — оформление первого прогона абзаца (шрифт, размер).
// Пустая строка — прогон макета без своих правил; тогда и копировать нечего.
func firstRunProperties(paragraph string) string {
	m := reRunProperties.FindString(paragraph)
	return m
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
	section string
	// selection — набор выбранных систем на момент сборки (offerSelectionHandle).
	// Промт без повтора описывает все системы, поэтому его ключ и текст зависят
	// от набора: добавили систему — вступление пересобирается (2026-11-08).
	selection string
	offer     Offer
	cards     []SystemCard
	refs      []skills.CardRef
	completed Completer
	// report — сводка сборки для журнала (2026-11-06). nil в тестах: печать
	// документа без журнала ведёт себя так же.
	report *buildReport
	// persist — куда записать дописанный текст системы (2026-11-07). nil —
	// сохранять некуда: текст печатается, но при следующей сборке будет
	// написан заново.
	persist func(noteID int64, block, text, hash string)
}

// systemText — текст системы для метки `{{текст: …}}`: сохранённый в этой
// сборке, из кэша прошлой или дописанный сейчас.
//
// Повод (2026-11-07): метка печатала только строку `offers_system_text`, а у
// всех живых систем она пуста — метка молчала и в макете, и в блоке формы.
// Порядок здесь повторяет выбор системы (шаг B): пишем один раз, сохраняем в
// `offers_system_text` и кэшируем в form_values предложения, чтобы повторная
// сборка не платила за тот же ответ дважды.
//
// key — по какому тексту кэш: у метки макета это инструкция блока, у повтора
// по системам та же инструкция, но заметка другая.
func (c *layoutPromptCache) systemText(noteID int64, block, instruction string) string {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return ""
	}
	// Карточку ищем ОДИН раз и до всякой записи: без неё нечего ни спросить,
	// ни сохранить. Раньше путь «заметки нет» доходил до сохранения и писал
	// строку с note_id = 0 (поймано на стенде 2026-11-07).
	card := cardByNote(c.cards, noteID)
	if card.NoteID == 0 {
		return ""
	}
	key := systemTextCacheKey(noteID, instruction)
	if v, ok := c.values[key]; ok {
		return v
	}
	if v := c.previousSystemText(key); v != "" {
		c.values[key] = v
		return v
	}
	text := c.writeSystemTextNow(card, instruction)
	if text == "" {
		return ""
	}
	if c.persist != nil {
		c.persist(card.NoteID, block, text, systemTextHash(card, block))
	}
	return text
}

// previousSystemText — текст из прошлой сборки, если данные предложения те же.
//
// Кэш завязан на хеш данных КП: поменялись цены — текст с прежними цифрами не
// годится, и модель напишет новый (правило 2026-11-01).
func (c *layoutPromptCache) previousSystemText(key string) string {
	if c.previous == nil || c.previous[genKeySection] != c.section {
		return ""
	}
	text := documentText(strings.TrimSpace(c.previous[key]))
	if text == "" {
		return ""
	}
	if c.report != nil {
		c.report.FromCache++
	}
	return text
}

// systemTextCacheKey — ключ кэша текста системы.
func systemTextCacheKey(noteID int64, instruction string) string {
	return genKeySystemText(noteID) + "." + hashShort(instruction)
}

// writeSystemTextNow — один вызов модели; ответ кладётся в кэш этой сборки.
//
// Карточка приходит готовой: вызывающий уже убедился, что она есть. Второй
// поиск по списку здесь означал бы, что «нет карточки» можно пропустить.
func (c *layoutPromptCache) writeSystemTextNow(card SystemCard, instruction string) string {
	if c.completed == nil {
		return ""
	}
	started := time.Now()
	text := layoutPromptText(context.Background(), instruction, c.offer, []SystemCard{card}, c.cards, c.refs, c.completed)
	reportFailed := strings.TrimSpace(text) == ""
	logModelCall("system:"+instruction, fmt.Sprintf("note=%d", card.NoteID), len(instruction), started, nil)
	if c.report != nil {
		if reportFailed {
			c.report.Failed++
		} else {
			c.report.Generated++
		}
	}
	if reportFailed {
		return ""
	}
	c.values[systemTextCacheKey(card.NoteID, instruction)] = text
	return text
}

// genKeySystemText — начало ключа кэша текста системы в form_values.
func genKeySystemText(noteID int64) string {
	return "gen.systext." + itoa(int(noteID))
}

// cardByNote — карточка по номеру заметки; пустая, если такой нет.
func cardByNote(cards []SystemCard, noteID int64) SystemCard {
	for _, card := range cards {
		if card.NoteID == noteID {
			return card
		}
	}
	return SystemCard{}
}

// offerSelectionHandle — признак набора выбранных систем в предложении.
//
// Зачем отдельный признак, если есть `hashOfferFacts` (2026-11-08). Хеш данных
// не видит выбора систем: карточки в него не входят, а живут они в
// `offers_context`. Живой случай КП 003/2026 — вторую систему добавили, хеш не
// изменился, и сборка взяла из кэша вступление, написанное про ОДНУ систему.
//
// Поэтому имя системы (номер заметки, иначе код) входит и в ключ текста, и в
// хеш данных: сменился набор — текст пересобирается, повторный выбор той же
// системы — нет.
func offerSelectionHandle(offer Offer) string {
	parts := make([]string, 0, len(offer.Systems))
	for _, s := range offer.Systems {
		if handle := systemChoiceHandle(s); handle != "" {
			parts = append(parts, handle)
		}
	}
	return strings.Join(parts, "+")
}

// systemChoiceHandle — устойчивый признак выбранной системы.
func systemChoiceHandle(s OfferSystem) string {
	if s.NoteID > 0 {
		return itoa(int(s.NoteID))
	}
	if sku := strings.TrimSpace(s.SKU); sku != "" {
		return "sku" + hashShort(sku)
	}
	return ""
}

// cardSelectionHandle — признак ОДНОЙ карточки в том же виде.
//
// Если у предложения выбор не записан (старый КП, сборка из тестов), признак
// набора берётся по карточкам, которые получила модель: без этого умолчания
// ключ текста вступления не совпал бы с ключом чтения.
func cardSelectionHandle(cards []SystemCard) string {
	if len(cards) == 0 {
		return ""
	}
	parts := make([]string, 0, len(cards))
	for _, card := range cards {
		if handle := systemCacheHandle(card); handle != "" && handle != "0" {
			parts = append(parts, handle)
		}
	}
	return strings.Join(parts, "+")
}

// layoutPromptKeyCache — ключ кэша одного промта макета.
//
// В ключ входит сам текст промта (хеш): администратор мог переписать фразу в
// макете, и старый текст к другой подсказке не годится. Второй вход — набор
// систем: промт без повтора описывает ВСЕ выбранные системы, и его текст
// обязан меняться вместе с выбором (2026-11-08).
func layoutPromptKeyCache(instruction, selection string) string {
	key := "gen.layout." + hashShort(instruction)
	if selection == "" {
		return key
	}
	return key + ".sel" + hashShort(selection)
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
	key := layoutPromptKeyCache(instruction, c.selection)
	if v, ok := c.values[key]; ok {
		return v
	}
	if c.previous != nil && c.previous[genKeySection] == c.section {
		if v := documentText(strings.TrimSpace(c.previous[key])); v != "" {
			c.values[key] = v
			if c.report != nil {
				c.report.FromCache++
			}
			return v
		}
	}
	started := time.Now()
	text := layoutPromptText(context.Background(), instruction, c.offer, c.cards, c.cards, c.refs, c.completed)
	reportFailed := text == ""
	logModelCall("layout", key, len(instruction), started, nil)
	if c.report != nil {
		if reportFailed {
			c.report.Failed++
		} else {
			c.report.Generated++
		}
	}
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

// offerSystemTitles — человеческие названия выбранных систем в порядке выбора.
//
// Название карточки Базы знаний главнее служебного номера заметки (2026-11-08,
// живая жалоба на КП 003/2026): ни одна из 375 живых систем не имеет `system_sku`,
// поэтому без карточек вместо «ТН-ФУНДАМЕНТ Дренаж Проф» в документ уходило
// «система №5874». Если карточка в кеше не нашлась — остаётся код или запасной
// номер заметки, чтобы метка не печатала пустоту.
func offerSystemTitles(offer Offer, cards []SystemCard) []string {
	out := make([]string, 0, len(offer.Systems))
	for _, s := range offer.Systems {
		title := ""
		if card, found := FindSystem(cards, SystemChoice{SKU: s.SKU, NoteID: s.NoteID}); found {
			title = strings.TrimSpace(card.Title)
		}
		if title == "" {
			title = strings.TrimSpace(s.SKU)
		}
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
	cards, allCards []SystemCard, refs []skills.CardRef, c Completer) string {
	if c == nil || strings.TrimSpace(instruction) == "" {
		return ""
	}
	maxWords := genDefaultMaxWords
	// Объём, заданный в самом промте, главнее умолчания (2026-11-06). Живой
	// макет 003/2026 просит «не менее 4000 знаков на одну систему» — около 600
	// слов. С бюджетом 150 модель в него не укладывалась и отдавала план
	// разделов вместо текста.
	if requested := skills.RequestedWords(instruction); requested > 0 {
		maxWords = requested
		// Верхняя граница — предел, которым живёт `prompt_text` в блоках: выше
		// 800 слов ответ режется провайдером и приходит обрывком.
		if maxWords > 800 {
			maxWords = 800
		}
	}
	// Тот же системный промпт, что у блока «по промту»: правила «пиши для
	// заказчика, не рассуждай» уже выведены там, и второй их набор разошёлся
	// бы с первым.
	system := skills.RenderBlockSystem(instruction, "", maxWords)
	user := buildBlockUserPromptSelection(instruction, offer, cards, allCards, "", refs)
	raw, err := c.Complete(ctx, system, user, documentTextMaxTokens())
	if err != nil {
		return ""
	}
	return cleanBlockText(raw)
}
