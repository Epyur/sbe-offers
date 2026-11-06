package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
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
	for i, token := range tokens {
		switch token.Kind {
		case layoutTokenPrompt:
			if table := layoutTableFor(token.Inner, lc); table != nil {
				replacement[i] = tableMarkdown(table)
				continue
			}
			if lc.prompt != nil {
				replacement[i] = lc.prompt(token.Inner)
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
	return out
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
type paragraphChange struct {
	text    string
	changed bool
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
		sb.WriteString(rebuildParagraph(paragraph, replaced[i].text))
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
		if lines := splitLayoutLines(text); len(lines) > 1 {
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
			sb.WriteString(layoutAsParagraphs(paragraph, splitLayoutLines(block.text)))
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
func rebuildParagraphText(paragraph, text string) string {
	first := true
	return reWordText.ReplaceAllStringFunc(paragraph, func(tag string) string {
		if first {
			first = false
			return replaceWordText(tag, text)
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

// splitLayoutLines режет текст на непустые строки, сохраняя краевые пробелы
// абзаца в начале и в конце.
func splitLayoutLines(text string) []string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

// layoutAsParagraphs печатает строки отдельными абзацами с вёрсткой исходного.
//
// Исходный абзац макета состоит из `<w:pPr>` (стиль и правила) и прогонов.
// Первая строка наследует `<w:pPr>` как есть, остальные получают его копию —
// так стиль абзаца макета действует на весь ответ модели, а не на первую
// строку. Пустая строка исходного абзаца (он мог быть пустым) не теряется:
// между строками абзацы не вставляются, потому что пустая строка в тексте
// модели — разделитель абзацев, а не пустой абзац документа.
func layoutAsParagraphs(paragraph string, lines []string) string {
	pPr := reParagraphPr.FindString(paragraph)
	if strings.TrimSpace(pPr) == "" {
		// У абзаца нет правил — синтезируем минимум, чтобы Word не считал
		// его отдельным «Normal по умолчанию».
		pPr = `<w:pPr><w:jc w:val="both"/></w:pPr>`
	}
	rPr := firstRunProperties(paragraph)
	var sb strings.Builder
	for _, line := range lines {
		sb.WriteString(`<w:p>`)
		sb.WriteString(pPr)
		sb.WriteString(`<w:r>`)
		sb.WriteString(rPr)
		sb.WriteString(`<w:t xml:space="preserve">`)
		sb.WriteString(escapeXML(line))
		sb.WriteString(`</w:t></w:r></w:p>`)
	}
	return sb.String()
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
	user := buildBlockUserPrompt(offer, cards, refs)
	raw, err := c.Complete(ctx, system, user, blockMaxTokens(maxWords))
	if err != nil {
		return ""
	}
	return cleanBlockText(raw)
}
