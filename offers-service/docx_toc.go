package main

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
)

// Оглавление генерируемого DOCX (2026-11-14, шаг 5a).
//
// Дефект, который это чинит: кэш оглавления приходит из МАКЕТА. `ensureUpdateFields`
// просит Word пересчитать поля при открытии, но файл, прочитанный в Телеграме,
// Word не открывает — человек видит четыре пункта шаблона с номерами страниц
// 3/3/4/5, хотя документ другой (`_Toc245563567`… на живом temple.docx).
//
// Чего это НЕ делает: номера страниц. Word считает их раскладкой, а сервер
// страницы не раскладывает. Гарантия другая и честная: набор пунктов, их
// порядок и якоря — ВСЕГДА из самого документа.

// tocEntry — один заголовок, напечатанный в этом документе.
type tocEntry struct {
	Text  string
	Level int
	// Bookmark — имя закладки `_Toc…`: уникальное, начинается с `_Toc`.
	Bookmark string
}

// tocBookmarkPrefix — начало имени закладки. Word связывает пункт оглавления с
// закладкой по имени и, чтобы пункт пережил пересчёт поля, имя обязано
// начинаться с `_Toc`.
const tocBookmarkPrefix = "_Toc"

// tocHeadingXML — заголовок, который попадёт в оглавление.
//
// Помимо самого абзаца вокруг текста ставится закладка: оглавление ссылается
// на закладку, а не на абзац, и без неё пункт — нерабочая ссылка.
//
// Префикс `_Toc` присутствует всегда: макет мог завести своё оглавление
// (`TOC \o "1-3"`), и чужое имя закладки в нём не находится.
func (lc layoutContext) tocHeadingXML(text string, level int, opts paraOpts) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if level <= 0 {
		level = 2
	}
	if level > 4 {
		level = 4
	}
	bookmark, id := lc.addTOCHeading(text, level)
	if bookmark == "" {
		// Вне сборки пакета закладке негде жить — печатаем обычный заголовок.
		return headingXMLOpts(text, level, opts)
	}
	heading := headingXMLOpts(text, level, opts)
	return wrapHeadingBookmark(heading, bookmark, id)
}

// wrapHeadingBookmark ставит закладку вокруг текста заголовка.
//
// Закладка идёт ВНУТРИ абзаца и обнимает прогоны: `<w:bookmarkStart>` до
// первого `<w:r>` и `<w:bookmarkEnd>` после последнего `</w:r>`. Заголовок,
// собранный нашим `headingXMLOpts`, всегда один прогон — но ставим между
// `</w:pPr>` и концом абзаца, чтобы numPr и стиль остались нетронутыми.
func wrapHeadingBookmark(heading, bookmark string, id int) string {
	open := strings.Index(heading, "</w:pPr>")
	if open < 0 {
		return heading
	}
	open += len("</w:pPr>")
	close := strings.LastIndex(heading, "</w:p>")
	if close < open {
		return heading
	}
	start := `<w:bookmarkStart w:id="` + strconv.Itoa(id) + `" w:name="` + escapeXML(bookmark) + `"/>`
	end := `<w:bookmarkEnd w:id="` + strconv.Itoa(id) + `"/>`
	return heading[:open] + start + heading[open:close] + end + heading[close:]
}

// addTOCHeading регистрирует заголовок и отдаёт имя закладки и её id.
//
// Имя уникально по построению: к префиксу и порядковому номеру добавляется
// хеш текста — два одинаковых заголовка («Введение» в двух разделах) получают
// разные закладки, а само имя остаётся читаемым в Word.
//
// Без runtime (чистая печать абзаца в тесте) закладок нет: закладка живёт в
// документе, и её id занимает пространство id ВСЕГО файла.
func (lc layoutContext) addTOCHeading(text string, level int) (string, int) {
	if lc.layout == nil {
		return "", 0
	}
	lc.layout.tocSeq++
	seq := lc.layout.tocSeq
	bookmark := fmt.Sprintf("%s%d_%s", tocBookmarkPrefix, seq, hashShort(text))
	lc.layout.toc = append(lc.layout.toc, tocEntry{
		Text: text, Level: level, Bookmark: bookmark,
	})
	return bookmark, tocBookmarkID(seq)
}

// dropTOCHeading снимает последнюю зарегистрированную закладку.
//
// Нужен там, где заголовок зарегистрирован ДО того, как стало известно, что он
// не напечатается: рамка раздела заводит закладку заголовка первой (порядок
// документа), а «ни один подраздел не написался» означает «раздел молчит
// целиком» (§10.1). Закладка без абзаца в оглавлении дала бы пункт-ссылку в
// никуда.
func (lc layoutContext) dropTOCHeading() {
	if lc.layout == nil || len(lc.layout.toc) == 0 {
		return
	}
	lc.layout.toc = lc.layout.toc[:len(lc.layout.toc)-1]
	if lc.layout.tocSeq > 0 {
		lc.layout.tocSeq--
	}
}

// tocBookmarkID — id закладки: стартуем выше всего макета.
//
// В temple.docx заняты id 0..2; документы пользователя доходят до десятков.
// 70000 не встречается ни в одном живом макете, а диапазон Word принимает.
func tocBookmarkID(seq int) int {
	return 70000 + seq
}

func rebuildLayoutTOCWithStyles(documentXML string, files map[string][]byte, entries []tocEntry) string {
	styles := string(files["word/styles.xml"])
	return rebuildLayoutTOC(documentXML, styles, entries)
}

func rebuildLayoutTOC(documentXML, stylesXML string, entries []tocEntry) string {
	start := strings.Index(documentXML, "<w:sdt>")
	if start < 0 {
		return documentXML
	}
	end := strings.Index(documentXML[start:], "</w:sdt>")
	if end < 0 {
		return documentXML
	}
	end += start + len("</w:sdt>")
	sdt := documentXML[start:end]
	if !strings.Contains(sdt, "Table of Contents") {
		// Это чужой sdt — не оглавление: не трогаем.
		return documentXML
	}
	styleIDs, itemStyleIDs := tocEntryStyles(stylesXML)
	breakXML := tocHeadingBreakXML(documentXML)
	rebuilt := replaceTOCEntries(sdt, entries, styleIDs, itemStyleIDs, breakXML)
	return documentXML[:start] + rebuilt + documentXML[end:]
}

// tocEntryStyles — styleId уровней оглавления из styles.xml этого документа.
//
// В temple.docx пункт — styleId `12` (имя «toc 1»); у другого макета номер
// будет иным. Стиль берётся из styles.xml документа; не нашлось — известное
// значение бланка: пункт без стиля печатается обычным абзацем, и это читаемо,
// а не сломано.
//
// Второе значение — ВСЕ стили пунктов («toc 1»…«toc 9»): по ним узнаётся, где
// в sdtContent кончается заголовок оглавления и начинается снимок пунктов.
func tocEntryStyles(stylesXML string) (map[int]string, map[string]bool) {
	styles := map[int]string{1: "12"}
	itemIDs := map[string]bool{"12": true}
	for _, m := range reTOCStyle.FindAllStringSubmatch(stylesXML, -1) {
		id := strings.TrimSpace(m[1])
		if id == "" || len(m[2]) != 1 {
			continue
		}
		itemIDs[id] = true
		if level := int(m[2][0] - '0'); level >= 1 && level <= 3 {
			styles[level] = id
		}
	}
	return styles, itemIDs
}

// reHyperlinkAnchor — якорь гиперссылки (ссылка пункта оглавления).
var reHyperlinkAnchor = regexp.MustCompile(`<w:hyperlink w:anchor="([^"]+)"`)

// reParagraphStyle — styleId абзаца.
var reParagraphStyle = regexp.MustCompile(`<w:pStyle w:val="([^"]+)"`)

// reTOCStyle — стиль пункта оглавления в styles.xml: `w:styleId="12"` с
// `w:name w:val="toc 1"`.
var reTOCStyle = regexp.MustCompile(`w:styleId="([^"]+)"[^>]*>\s*<w:name w:val="toc ([0-9])"`)

// tocHeadingBreakXML — как макет отделяет заголовок «СОДЕРЖАНИЕ» от пунктов.
//
// В temple.docx после заголовка стоит пустой абзац `af9`. Если его не
// сохранить, пункты начинаются вплотную под словом «СОДЕРЖАНИЕ» — выглядит
// сломанным, хотя формально верно. Пустой абзац берётся из макета, а не
// выдумывается.
func tocHeadingBreakXML(documentXML string) string {
	const empty = `<w:p><w:pPr><w:pStyle w:val="af9"/></w:pPr></w:p>`
	if strings.Contains(documentXML, empty) {
		return empty
	}
	return ""
}

// replaceTOCEntries собирает sdt оглавления заново.
//
// Части макета, которые сохраняются: `<w:sdtPr>` и `<w:sdtEndPr>` (инструкция
// поля и оформление), заголовок оглавления и первый абзац с полем `TOC`.
// Остальные абзацы `sdtContent` — снимок пунктов — выбрасываются.
func replaceTOCEntries(sdt string, entries []tocEntry, styles map[int]string, itemStyles map[string]bool, breakXML string) string {
	contentStart := strings.Index(sdt, "<w:sdtContent>")
	contentEnd := strings.LastIndex(sdt, "</w:sdtContent>")
	if contentStart < 0 || contentEnd < contentStart {
		return sdt
	}
	contentStart += len("<w:sdtContent>")
	content := sdt[contentStart:contentEnd]
	keep := tocKeepingParagraphs(content, itemStyles)
	var sb strings.Builder
	sb.WriteString(sdt[:contentStart])
	sb.WriteString(keep)
	sb.WriteString(breakXML)
	for _, entry := range entries {
		if entry.Level > 3 {
			// `TOC \o "1-3"`: глубже трёх уровней оглавление не печатает.
			continue
		}
		sb.WriteString(tocEntryXML(entry, styles[entry.Level]))
	}
	sb.WriteString(sdt[contentEnd:])
	return sb.String()
}

// tocKeepingParagraphs — абзацы начала sdtContent до первого пункта.
//
// Это заголовок оглавления и абзац с полем `TOC`. Первый пункт — абзац со
// стилем пункта оглавления; всё от него и дальше и есть снимок.
func tocKeepingParagraphs(content string, itemStyles map[string]bool) string {
	paragraphs := reWordParagraph.FindAllStringIndex(content, -1)
	if len(paragraphs) == 0 {
		return ""
	}
	var keep strings.Builder
	for _, span := range paragraphs {
		paragraph := content[span[0]:span[1]]
		if sentenceHasTOCItemStyle(paragraph, itemStyles) {
			break
		}
		keep.WriteString(paragraph)
	}
	return keep.String()
}

// sentenceHasTOCItemStyle — абзац оформлен как пункт оглавления.
//
// Стиль пункта берётся ИЗ styles.xml этого документа (у temple.docx это 12,
// имя «toc 1»): у другого макета номер иной, а заголовок оглавления («TOC
// Heading») от пункта отличается.
func sentenceHasTOCItemStyle(paragraph string, itemStyles map[string]bool) bool {
	style := reParagraphStyle.FindStringSubmatch(paragraph)
	if style == nil {
		return false
	}
	return itemStyles[style[1]]
}

// tocEntryXML — один пункт оглавления: гиперссылка на закладку и PAGEREF.
//
// Номер страницы оставлен пустым полем PAGEREF: его считает Word при
// открытии, а сервер страниц не раскладывает — обещать число значило бы
// врать. Пункт без числа остаётся рабочим: он ведёт на правильную закладку.
func tocEntryXML(entry tocEntry, styleID string) string {
	if strings.TrimSpace(styleID) == "" {
		styleID = "12"
	}
	anchor := escapeXML(entry.Bookmark)
	var sb strings.Builder
	sb.WriteString(`<w:p><w:pPr><w:pStyle w:val="` + escapeXML(styleID) + `"/><w:tabs><w:tab w:val="right" w:leader="dot" w:pos="9911"/></w:tabs></w:pPr>`)
	sb.WriteString(`<w:hyperlink w:anchor="` + anchor + `" w:history="1">`)
	sb.WriteString(`<w:r><w:rPr><w:rStyle w:val="ab"/></w:rPr><w:t xml:space="preserve">` + escapeXML(entry.Text) + `</w:t></w:r>`)
	sb.WriteString(`<w:r><w:tab/></w:r>`)
	sb.WriteString(`<w:r><w:fldChar w:fldCharType="begin"/></w:r>`)
	sb.WriteString(`<w:r><w:instrText xml:space="preserve"> PAGEREF ` + anchor + ` \h </w:instrText></w:r>`)
	sb.WriteString(`<w:r><w:fldChar w:fldCharType="separate"/></w:r>`)
	sb.WriteString(`<w:r><w:fldChar w:fldCharType="end"/></w:r>`)
	sb.WriteString(`</w:hyperlink></w:p>`)
	return sb.String()
}

// logTOC говорит в журнал, сколько пунктов поставили и что номеров страниц
// может не быть.
func logTOC(count int) {
	if count == 0 {
		log.Printf("оглавление: заголовков нет — кэш макета оставлен")
		return
	}
	log.Printf("оглавление: пунктов %d, якоря из документа; номера страниц проставит Word", count)
}
