package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"mailers/offers/skills"
)

// stylesXML — стандартное оформление Word (Times New Roman 14pt, полуторный
// интервал, абзацный отступ 1,25 см, выравнивание по ширине) — тот же набор,
// что у agent-service, чтобы документы экосистемы выглядели одинаково.
const stylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:docDefaults>
    <w:rPrDefault><w:rPr>
      <w:rFonts w:ascii="Times New Roman" w:eastAsia="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/>
      <w:sz w:val="28"/><w:szCs w:val="28"/>
    </w:rPr></w:rPrDefault>
    <w:pPrDefault><w:pPr/></w:pPrDefault>
  </w:docDefaults>
  <w:style w:type="paragraph" w:default="1" w:styleId="Normal">
    <w:name w:val="Normal"/><w:qFormat/>
    <w:pPr><w:spacing w:before="0" w:after="0" w:line="360" w:lineRule="auto"/><w:ind w:firstLine="709"/><w:jc w:val="both"/></w:pPr>
  </w:style>
  <w:style w:type="paragraph" w:styleId="Title">
    <w:name w:val="Title"/><w:basedOn w:val="Normal"/><w:qFormat/>
    <w:pPr><w:spacing w:before="0" w:after="240" w:line="360" w:lineRule="auto"/><w:jc w:val="center"/><w:ind w:firstLine="0"/></w:pPr>
    <w:rPr><w:b/><w:sz w:val="36"/><w:szCs w:val="36"/></w:rPr>
  </w:style>
  <w:style w:type="paragraph" w:styleId="Heading1">
    <w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/>
    <w:pPr><w:spacing w:before="240" w:after="120" w:line="360" w:lineRule="auto"/><w:jc w:val="center"/><w:ind w:firstLine="0"/><w:outlineLvl w:val="0"/></w:pPr>
    <w:rPr><w:b/><w:sz w:val="32"/><w:szCs w:val="32"/></w:rPr>
  </w:style>
  <w:style w:type="paragraph" w:styleId="Heading2">
    <w:name w:val="heading 2"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/>
    <w:pPr><w:spacing w:before="240" w:after="120" w:line="360" w:lineRule="auto"/><w:jc w:val="center"/><w:ind w:firstLine="0"/><w:outlineLvl w:val="1"/></w:pPr>
    <w:rPr><w:b/><w:sz w:val="30"/><w:szCs w:val="30"/></w:rPr>
  </w:style>
  <w:style w:type="paragraph" w:styleId="Heading3">
    <w:name w:val="heading 3"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/>
    <w:pPr><w:spacing w:before="240" w:after="120" w:line="360" w:lineRule="auto"/><w:jc w:val="center"/><w:ind w:firstLine="0"/><w:outlineLvl w:val="2"/></w:pPr>
    <w:rPr><w:b/><w:sz w:val="28"/><w:szCs w:val="28"/></w:rPr>
  </w:style>
</w:styles>`

// --- Стили и нумерация БЛАНКА (2026-10-29) -----------------------------
//
// Имена взяты из шаблона standard.docx, а не выдуманы: styles.xml и
// numbering.xml теперь берутся из бланка как есть, поэтому ссылаться можно
// только на то, что там есть. Раньше здесь стояли "Normal"/"Title"/"Heading1"
// и своя нумерация — таких стилей в бланке нет (Normal называется a0,
// заголовок — "1"), поэтому Word показывал текст дефолтным шрифтом.
//
// Если шаблон поменяют — упадёт TestDocxOnlyKnownStyles с точным именем
// недостающего стиля, а не «поехавшая вёрстка» у заказчика.

const (
	// styleNormal — «Normal» бланка (w:styleId="a0").
	styleNormal = "a0"
	// styleHeading1 — «heading 1» бланка (w:styleId="1"). Уровней глубже в
	// бланке нет: headingXML рисует размер шрифта сам.
	styleHeading1 = "1"
	// styleListBullet — маркированный список бланка. abstractNum 1 — bullet,
	// numId 6 на него ссылается (нумерации 1..12 заняты бланком).
	styleListBullet = "6"
	// styleListNumbered — нумерованный список бланка (abstractNum 0 — decimal,
	// numId 1). Заголовки/списки ответа модели печатаются им с 2026-11-06.
	styleListNumbered = "1"
	// styleHyperlink — стиль ссылки бланка (w:styleId="ab", имя "Hyperlink").
	// Своего стиля не заводим: имена в бланке — часть его оформления.
	styleHyperlink = "ab"
)
const numberingXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:abstractNum w:abstractNumId="0">
    <w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="bullet"/><w:lvlText w:val="•"/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="720" w:hanging="360"/></w:pPr></w:lvl>
  </w:abstractNum>
  <w:abstractNum w:abstractNumId="1">
    <w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1."/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="720" w:hanging="360"/></w:pPr></w:lvl>
  </w:abstractNum>
  <w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>
  <w:num w:numId="2"><w:abstractNumId w:val="1"/></w:num>
</w:numbering>`

func escapeXML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	return r.Replace(s)
}

func jcVal(align string) string {
	switch strings.ToLower(strings.TrimSpace(align)) {
	case "center":
		return "center"
	case "right":
		return "right"
	case "justify", "both":
		return "both"
	default:
		return ""
	}
}

// paraOpts — размещение абзаца на странице (2026-11-01). Всё в пунктах; в
// разметке Word 1 пт = 20 twips.
//
// Разрыв страницы живёт здесь, а не отдельным абзацем: `<w:br w:type="page"/>`
// внутри пустого абзаца даёт лишнюю пустую строку и часто — пустую страницу.
type paraOpts struct {
	// Before/After — отступ до и после в пунктах (0 — как в стиле бланка).
	Before int
	After  int
	// KeepLines — не разрывать абзац между страницами.
	KeepLines bool
	// PageBreakBefore — начать с новой страницы.
	PageBreakBefore bool
	// Align — выравнивание абзаца. Пусто — «по ширine» (оформление документа).
	Align string
	// NoIndent — без отступа первой строки (заголовки, картинки, подписи).
	NoIndent bool
}

// Оформление документа (2026-11-05).
//
// Живой КП 003/2026 вышел без выравнивания: стиль Normal бланка (`a0`) —
// Arial 11pt, прижат влево, без отступа первой строки. Пользователь ждал
// обычного печатного текста: по ширине, с красной строкой 1,25 см.
//
// Стиль бланка править нельзя — в нём колонтитулы, нумерация и остальное
// оформление фирменного листа (2026-10-29, грабля с потерей номеров страниц).
// Поэтому оформление задаётся в pPr КАЖДОГО абзаца; так же сформулировано
// правило документов агента (sbe-agent, «Стандартное оформление Word»).
const (
	// bodyFirstLine — отступ первой строки 1,25 см = 709 twips.
	bodyFirstLine = "709"
	// bodyLine — полуторный интервал (240 × 1,5).
	bodyLineRule = `<w:spacing w:line="360" w:lineRule="auto"/>`
	// bodyAlign — выравнивание основного текста.
	bodyAlign = "both"
)

func (o paraOpts) spacingXML() string {
	var sb strings.Builder
	sb.WriteString("<w:spacing")
	// Полуторный интервал — оформление документа; отступы блока его дополняют.
	sb.WriteString(` w:line="360" w:lineRule="auto"`)
	if o.Before > 0 {
		sb.WriteString(` w:before="` + strconv.Itoa(o.Before*20) + `"`)
	}
	if o.After > 0 {
		sb.WriteString(` w:after="` + strconv.Itoa(o.After*20) + `"`)
	}
	sb.WriteString("/>")
	return sb.String()
}

// alignXML — выравнивание абзаца: явное из блока формы или оформление
// документа (по ширине) для основного текста.
func (o paraOpts) alignXML() string {
	align := o.Align
	if jcVal(align) == "" {
		align = bodyAlign
	}
	return `<w:jc w:val="` + jcVal(align) + `"/>`
}

// indentXML — отступ первой строки. Красная строка нужна основному тексту и
// вредна заголовку, картинке и подписи под ней.
func (o paraOpts) indentXML() string {
	if o.NoIndent {
		return `<w:ind w:firstLine="0"/>`
	}
	return `<w:ind w:firstLine="` + bodyFirstLine + `"/>`
}

// flagXML — элемент разметки Word по флагу. Два одинаковых «если включено —
// вернуть разметку, иначе пусто» были бы парой, которую dryer ловит как дубль.
func flagXML(on bool, markup string) string {
	if on {
		return markup
	}
	return ""
}

func (o paraOpts) keepXML() string {
	return flagXML(o.KeepLines, "<w:keepLines/>")
}

func (o paraOpts) pageBreakXML() string {
	return flagXML(o.PageBreakBefore, "<w:pageBreakBefore/>")
}

func paragraphAligned(text, align string) string {
	return paragraphXMLOpts(text, align, paraOpts{})
}

// paragraphXMLOpts — абзац с размещением. Порядок элементов в w:pPr задан
// схемой: стиль, отступы/разрыв, выравнивание.
//
// Текст проходит через plainText и runsXML: вики-ссылки из заметок убираются,
// потерянные пробелы возвращаются, степени в единицах измерения становятся
// надстрочными (2026-11-05 — живые дефекты КП 003/2026).
func paragraphXMLOpts(text, align string, opts paraOpts) string {
	// Выравнивание — из аргумента или блока формы; пусто значит «оформление
	// документа» (по ширине), а не «как получится».
	opts.Align = align
	return paragraphXML(text, opts)
}

// paragraphXML — абзац по опциям. Одна точка сборки pPr: порядок элементов
// задан схемой (стиль, отступы/разрыв, интервал, отступ строки, выравнивание).
func paragraphXML(text string, opts paraOpts) string {
	var pPr strings.Builder
	pPr.WriteString(`<w:pStyle w:val="` + styleNormal + `"/>`)
	pPr.WriteString(opts.pageBreakXML())
	pPr.WriteString(opts.spacingXML())
	pPr.WriteString(opts.indentXML())
	pPr.WriteString(opts.alignXML())
	pPr.WriteString(opts.keepXML())
	return `<w:p><w:pPr>` + pPr.String() + `</w:pPr>` + runsXML(plainText(text), "") + `</w:p>`
}

// paragraphsXML — текст с переносами строк как НЕСКОЛЬКО абзацев.
//
// До 2026-11-05 описание карточки с переносами печаталось одним абзацем с
// символами \n внутри: Word их не показывает, и целые строки («Особенности
// системы:») слипались с соседними. Пустые строки пропускаются — абзацев-
// пустышек в документе быть не должно.
func paragraphsXML(text, align string, opts paraOpts) string {
	var sb strings.Builder
	first := true
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		o := paraOpts{}
		if first {
			o = opts
			first = false
		}
		sb.WriteString(paragraphXMLOpts(strings.TrimSpace(line), align, o))
	}
	return sb.String()
}

// pageBreakRunXML — разрыв страницы как узел содержимого: отдельный пустой
// абзац с разрывом внутри. Так его можно поставить между любыми абзацами, в
// том числе в середине блока.
func pageBreakRunXML() string {
	return `<w:p><w:r><w:br w:type="page"/></w:r></w:p>`
}

// hyperlinkXML — внешняя ссылка. Отношение создаётся в docx_links.go; здесь
// только разметка, чтобы печать ссылок не разъезжалась с печатью текста.
func hyperlinkXML(relID, text, url string) string {
	if relID == "" {
		return paragraphAligned(url, "")
	}
	return `<w:p><w:pPr><w:pStyle w:val="` + styleNormal + `"/><w:ind w:firstLine="0"/></w:pPr>` +
		`<w:hyperlink r:id="` + escapeXML(relID) + `" w:history="1">` +
		`<w:r><w:rPr><w:rStyle w:val="` + styleHyperlink + `"/></w:rPr>` +
		`<w:t xml:space="preserve">` + escapeXML(text) + `</w:t></w:r></w:hyperlink></w:p>`
}

func listItemXML(text string) string {
	return listItemXMLOpts(text, paraOpts{})
}

// listItemXMLOpts — пункт списка с размещением: отступы блока нужны и пункту.
func listItemXMLOpts(text string, opts paraOpts) string {
	return listItemNumXML(text, styleListBullet, opts)
}

// listItemNumXML — пункт списка с заданной нумерацией бланка.
func listItemNumXML(text, numID string, opts paraOpts) string {
	return `<w:p><w:pPr><w:pStyle w:val="` + styleNormal + `"/><w:ind w:firstLine="0"/>` +
		opts.spacingXML() +
		`<w:numPr><w:ilvl w:val="0"/><w:numId w:val="` + numID + `"/></w:numPr></w:pPr>` +
		runsXML(plainText(text), "") + `</w:p>`
}

func headingXML(text string, level int) string {
	return headingXMLOpts(text, level, paraOpts{})
}

func headingXMLOpts(text string, level int, opts paraOpts) string {
	// В бланке есть только «heading 1» (styleId "1", outlineLvl 0). Назначать
	// его заголовкам 2–4 уровней нельзя (2026-11-08, живая жалоба на КП
	// 003/2026): любой абзац со стилем "1" Word и оглавление `TOC \o "1-3" \u`
	// считают заголовком 1-го уровня, хотя в настройках блока стоял уровень 2.
	// Поэтому styleHeading1 получает только уровень 1, а уровни 2–4 идут на
	// основе styleNormal со своим `<w:outlineLvl w:val="level-1"/>` и
	// `<w:keepNext/>`.
	if level <= 0 {
		level = 2
	}
	if level > 4 {
		level = 4
	}
	style := styleNormal
	size := "28" // 14pt (уровень 2)
	switch level {
	case 1:
		style = styleHeading1
		size = "32" // 16pt
	case 3:
		size = "26" // 13pt
	case 4:
		size = "24" // 12pt
	}
	outline := strconv.Itoa(level - 1)
	const headingRPr = `<w:b/>`
	// Заголовок — по центру, без красной строки, не отрывается от следующего
	// абзаца: так оформлены документы экосистемы (sbe-agent) и так их читают в КП.
	opts.NoIndent = true
	opts.Align = "center"
	return `<w:p><w:pPr><w:pStyle w:val="` + style + `"/>` +
		`<w:keepNext/><w:keepLines/>` +
		opts.pageBreakXML() + opts.spacingXML() + opts.indentXML() + opts.alignXML() +
		`<w:outlineLvl w:val="` + outline + `"/></w:pPr>` +
		runsXML(plainText(text), headingRPr+`<w:sz w:val="`+size+`"/><w:szCs w:val="`+size+`"/>`) +
		`</w:p>`
}

func tableCellXML(text string, header bool) string {
	pPr := `<w:spacing w:line="240" w:lineRule="auto"/><w:ind w:firstLine="0"/>`
	if header {
		pPr += `<w:jc w:val="center"/>`
	}
	rPr := `<w:rFonts w:ascii="Times New Roman" w:eastAsia="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/><w:sz w:val="20"/><w:szCs w:val="20"/>`
	if header {
		rPr += `<w:b/><w:color w:val="1F3864"/>`
	}
	shading := ""
	if header {
		shading = `<w:shd w:val="clear" w:color="auto" w:fill="DCE6F1"/>`
	}
	return `<w:tc><w:tcPr><w:tcW w:w="0" w:type="auto"/>` + shading + `</w:tcPr>` +
		`<w:p><w:pPr>` + pPr + `</w:pPr>` + runsXML(plainText(text), rPr) + `</w:p></w:tc>`
}

func tableXML(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/><w:tblBorders>`)
	for _, e := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
		sb.WriteString(`<w:` + e + ` w:val="single" w:sz="4" w:space="0" w:color="808080"/>`)
	}
	sb.WriteString(`</w:tblBorders></w:tblPr>`)
	for i, row := range rows {
		sb.WriteString("<w:tr>")
		for _, cell := range row {
			sb.WriteString(tableCellXML(cell, i == 0))
		}
		sb.WriteString("</w:tr>")
	}
	sb.WriteString("</w:tbl>")
	return sb.String()
}

// resolvePlaceholders подставляет {{id}} значениями формы.
//
// Неизвестные поля заменяются пустой строкой — в документе не должно остаться
// служебных скобок. Служебные поля (`offer_number`, `offer_date`, `manager`)
// подставляются всегда: номер, дата и подпись есть у каждого предложения, и
// держать их в форме руками значит забыть обновить после копирования КП.
func resolvePlaceholders(text string, values map[string]string) string {
	for key, value := range values {
		text = strings.ReplaceAll(text, "{{"+key+"}}", value)
	}
	for {
		start := strings.Index(text, "{{")
		if start < 0 {
			break
		}
		end := strings.Index(text[start:], "}}")
		if end < 0 {
			break
		}
		text = text[:start] + text[start+end+2:]
	}
	return text
}

// formatMoney переехал в skills.FormatMoney: один экземпляр на весь сервис.
// Раньше копии были здесь и в боте, и они разошлись (1.005 печаталось как 1,00).
func formatMoney(v float64) string { return skills.FormatMoney(v) }

// blockNode — один узел содержимого блока формы.
//
// Новые виды узлов (2026-11-01): `page_break` — разрыв страницы; `links` —
// собранные ссылки предложения; `prompt_text` — текст, который модель пишет по
// подсказке администратора.
type blockNode struct {
	Type  string     `json:"type"`
	Level int        `json:"level"`
	Align string     `json:"align"`
	Text  string     `json:"text"`
	Items []string   `json:"items"`
	Rows  [][]string `json:"rows"`
	// Instruction — подсказка администратора для `prompt_text`: о чём писать.
	Instruction string `json:"instruction"`
	// Hint — необязательное уточнение к подсказке.
	Hint string `json:"hint"`
	// MaxWords — предел длины текста по промту (0 — значение по умолчанию).
	MaxWords int `json:"maxWords"`
	// Поля узла `image` (2026-11-01): картинка вставляется в текст с
	// обтеканием, а не отдельной строкой.
	ImageKey  string `json:"imageKey"`
	ImageName string `json:"imageName"`
	WidthCm   int    `json:"widthCm"`
	Wrap      string `json:"wrap"`
	Caption   string `json:"caption"`
	// ImageOf — откуда брать картинку (2026-11-05): пусто/static — по ключу
	// ImageKey; system — картинка выбранной системы КП; material — картинка
	// материала из позиций. Форма общая для всех предложений, и «какая
	// система» знает только само предложение.
	ImageOf string `json:"imageOf,omitempty"`
	// SystemIndex — номер выбранной системы (1 = первая в порядке выбора).
	SystemIndex int `json:"systemIndex,omitempty"`
	// ItemIndex — номер позиции предложения (1 = первая).
	ItemIndex int `json:"itemIndex,omitempty"`
}

// renderCtx — что нужно содержимому блока, кроме значений формы.
//
// Собрано структурой, а не списком аргументов: у рендера уже семь входов без
// неё, и каждый новый вид узла добавлял бы восьмой.
type renderCtx struct {
	values map[string]string
	// service — служебные поля шаблона (номер, дата, подпись, клиент, объект):
	// считаются из самого предложения и в форме не хранятся.
	service map[string]string
	offer   Offer
	refs    []skills.CardRef
	// cards — карточки систем предложения: нужны узлу сравнения.
	cards  []SystemCard
	images ImageSource
	media  *docxMedia
	// layoutRuntime — состояние сборки по макету (сборщик картинок, нумерация
	// и собранные ссылки). Общий указатель с внешним контекстом печати: иначе
	// ссылки, напечатанные меткой внутри блока формы, попадали бы в тело, а их
	// отношения терялись — Word не открывает файл со ссылкой на неизвестное
	// отношение (2026-11-09, живой сломанный КП 003/2026).
	layoutRuntime *layoutRuntime
	// numbering — план нумерации списков этой сборки: каждый состав получает
	// свой numId, чтобы нумерация слоёв начиналась заново у каждой системы
	// (2026-11-08). nil — состав печатается одним сквозным списком.
	numbering *numberingPlan
	// systemCard — выбранная система повтора (шаг D): блок формы с repeat
	// печатается по разу на систему, и служебные метки внутри блока обязаны
	// брать ЕЁ карточку, а не первую. nil — обычная печать.
	systemCard *SystemCard
	// materialIndex — номер позиции повтора по материалам (1-based); 0 — нет.
	materialIndex int
	// materialSource — полный список позиций предложения для повтора: узлу
	// отдаётся одна позиция, а метка `{{материал}}` ищет свою по номеру.
	materialSource []OfferItem
	// systemTexts — сохранённые тексты системы (шаг B): узлы блока читают их
	// так же, как метки макета, и модель не зовут.
	systemTexts func(card SystemCard) map[string]string
	// systemText — дописать недостающий текст системы (2026-11-07). nil —
	// дописывать некуда, блок печатает только сохранённое.
	systemText func(card SystemCard, block string) string
	// selection - набор выбранных систем всего предложения (2026-11-08). В
	// повторе `cards` сужены до одной системы, а промт вступления обязан знать
	// весь комплект. Пусто - признак считается по карточкам и `offer.Systems`.
	selection string
}

// selectionHandle - признак набора систем для ключей кэша текстов.
func (rc renderCtx) selectionHandle() string {
	if rc.selection != "" {
		return rc.selection
	}
	if handle := offerSelectionHandle(rc.offer); handle != "" {
		return handle
	}
	if rc.systemCard != nil {
		return cardSelectionHandle([]SystemCard{*rc.systemCard})
	}
	return cardSelectionHandle(rc.cards)
}

// text подставляет поля формы и служебные значения предложения в один кусок
// текста. Одна точка на все виды узлов: иначе служебные поля доезжали бы до
// таблицы и не доезжали до абзаца (или наоборот), и расхождение искали бы глазами.
func (rc renderCtx) text(raw string) string {
	// Сначала служебные метки, потом поля формы: resolvePlaceholders вычищает
	// любые оставшиеся `{{…}}`, и служебная метка, не раскрытая до него,
	// просто исчезла бы (2026-11-06).
	return resolvePlaceholders(expandBlockServiceMarkers(raw, rc), rc.allValues())
}

// expandBlockServiceMarkers печатает служебные метки внутри блока формы.
//
// Правило 2026-11-06 (шаг C): метка макета и узел блока печатают одни и те же
// данные одним кодом. Иначе `{{система}}` в блоке печатался бы скобками, а в
// макете — таблицей, и заказчик получил бы разное от одной формы.
//
// Метки, которые печатают НЕ текст (таблица состава, таблица характеристик),
// здесь остаются пустыми: это текстовый конвейер (заголовок, пункт списка,
// ячейка таблицы), и полотно Word в него не встаёт. Абзац обрабатывает
// renderParagraphNode — он умеет и текст, и полотно.
func expandBlockServiceMarkers(text string, rc renderCtx) string {
	if !strings.Contains(text, "{{") && !strings.Contains(text, "{") {
		return text
	}
	tokens := scanLayoutTokens(text, rc.allValues())
	if len(tokens) == 0 {
		return text
	}
	lc := rc.layoutContext()
	var sb strings.Builder
	last := 0
	for _, token := range tokens {
		if !isServiceKind(token.Kind) {
			continue
		}
		row := rc.serviceRow()
		row.viaBlock = true
		markup := renderLayoutService(token, lc, row)
		switch token.Kind {
		case layoutTokenText, layoutTokenMaterial:
			// Текст системы и позиции — строки: узел абзаца печатает их как есть.
		default:
			// Таблица/картинка внутрь текста не встаёт: оставляем пусто.
			markup = ""
		}
		sb.WriteString(text[last:token.Start])
		sb.WriteString(markup)
		last = token.End
	}
	sb.WriteString(text[last:])
	return sb.String()
}

// blockServiceKind — метка печатает ПОЛОТНО Word (таблицу, картинку), а не
// строку текста.
//
// `{{block:N}}` здесь намеренно нет: блок внутри самого себя ушёл бы в
// бесконечную печать, а выгоды у такой ссылки нет — блоки ставят в макет.
func blockServiceKind(kind string) bool {
	switch kind {
	case layoutTokenSystem, layoutTokenLinks, layoutTokenSpecs, layoutTokenImage:
		return true
	}
	return false
}

// renderBlockNodeMarker — разметка полотна для узла абзаца блока формы.
//
// Картинка разбирается отдельно: `renderLayoutService` печатает данные
// предложения, а метка `{{картинка}}` — изображение, и её вид (система или
// материал) живёт в `layoutImageSpec`. Одна ветка, потому что иначе номер
// системы пришлось бы разбирать второй раз и в другом месте.
func renderBlockNodeMarker(token layoutToken, lc layoutContext, row layoutServiceRow) string {
	if token.Kind != layoutTokenImage {
		return renderLayoutService(token, lc, row)
	}
	spec, ok := parseLayoutImageSpec(token.Inner)
	if !ok {
		return ""
	}
	return layoutImageMarkup(spec, lc)
}

// renderParagraphNode — абзац блока формы со служебными метками.
//
// Метка, печатающая полотно (`{{система}}`, `{{ссылки: система}}`,
// `{{характеристики: система}}`, `{{картинка}}`), НЕ встаёт внутрь `<w:p>`:
// таблица или картинка там либо экранируется, либо даёт невалидный OOXML.
// Поэтому текст абзаца печатается абзацем, а полотно — следом, как `imageAfter`
// в attachServiceMarkup. Абзац, состоявший из одной метки, не даёт пустого
// `<w:p>` — от него оставалось пустое место в готовом КП (2026-11-07).
func renderParagraphNode(text, align string, opts paraOpts, rc renderCtx) string {
	tokens := scanLayoutTokens(text, rc.allValues())
	if !hasBlockServiceMarker(tokens) {
		return paragraphXMLOpts(rc.text(text), align, opts)
	}
	lc := rc.layoutContext()
	row := rc.serviceRow()
	var rest, block strings.Builder
	last := 0
	for _, token := range tokens {
		if !blockServiceKind(token.Kind) {
			continue
		}
		rest.WriteString(text[last:token.Start])
		last = token.End
		block.WriteString(renderBlockNodeMarker(token, lc, row))
	}
	rest.WriteString(text[last:])
	var sb strings.Builder
	if strings.TrimSpace(rest.String()) != "" {
		sb.WriteString(paragraphXMLOpts(rc.text(rest.String()), align, opts))
	}
	sb.WriteString(block.String())
	return sb.String()
}

// hasBlockServiceMarker — есть ли в абзаце метка, печатающая полотно.
func hasBlockServiceMarker(tokens []layoutToken) bool {
	for _, token := range tokens {
		if blockServiceKind(token.Kind) {
			return true
		}
	}
	return false
}

// layoutContext — контекст печати по макету из контекста блоков: один перевод,
// чтобы служебные метки в блоке и в макете печатались одним кодом.
func (rc renderCtx) layoutContext() layoutContext {
	offer := rc.offer
	// Внутри повтора по материалам `rc.offer.Items` сужен до текущей позиции,
	// а номер позиции — исходный: метка `{{материал}}` обязана попасть в свою
	// строку полного списка, иначе со второго прохода она молчит (2026-11-06).
	if rc.materialIndex > 0 && rc.materialSource != nil {
		offer = Offer{Number: rc.offer.Number, Items: rc.materialSource}
	}
	ctx := layoutContext{
		values:      rc.allValues(),
		offer:       offer,
		refs:        rc.refs,
		service:     rc.service,
		systemTexts: rc.systemTexts,
		// Текста системы нет — дописать (2026-11-07): в блоке формы метка
		// `{{текст: система: description}}` вела себя так же, как в макете, и
		// так же молчала. Одна функция на оба пути.
		systemText: rc.systemText,
		cards:      func() []SystemCard { return rc.cards },
		images:     rc.images,
	}
	// Сборщик картинок обязан доехать до метки системы: без него
	// `layoutCardImageMarkup` видит `nil` и молчит, и `{{система}}` в блоке
	// печаталась без картинки (живая жалоба 2026-11-07). Указатель общий с
	// блоком формы: добавленные файлы уходят в пакет тем же сборщиком.
	// Тот же общий указатель несуёт и собранные ссылки: иначе отношения,
	// заведённые меткой `{{ссылки}}` внутри блока, терялись бы, и документ со
	// ссылкой на неизвестное отношение Word не открывает (2026-11-09).
	if rc.layoutRuntime != nil {
		ctx.layout = rc.layoutRuntime
	} else if rc.media != nil {
		ctx.layout = &layoutRuntime{media: rc.media, numbering: rc.numbering}
	}
	return ctx
}

// serviceRow — проход повтора: своя система или своя позиция.
func (rc renderCtx) serviceRow() layoutServiceRow {
	row := layoutServiceRow{material: rc.materialIndex}
	if rc.systemCard != nil {
		row.system = rc.systemCard
	}
	return row
}

// allValues — значения формы плюс служебные, служебные не затирают форму.
func (rc renderCtx) allValues() map[string]string {
	out := make(map[string]string, len(rc.values)+len(ServiceFieldIDs))
	for _, key := range ServiceFieldIDs {
		if v, ok := rc.service[key]; ok && v != "" {
			out[key] = v
		}
	}
	for k, v := range rc.values {
		out[k] = v
	}
	return out
}

func renderBlockContent(block Block, values map[string]string) (string, error) {
	return renderBlockContentCtx(block, renderCtx{values: values})
}

func renderBlockContentCtx(block Block, rc renderCtx) (string, error) {
	var nodes []blockNode
	if len(block.Content) > 0 {
		if err := json.Unmarshal(block.Content, &nodes); err != nil {
			return "", fmt.Errorf("разбор блока «%s»: %w", block.Title, err)
		}
	}
	var sb strings.Builder
	// Разрыв страницы перед первым текстом не печатается: Word на нём даёт
	// пустую страницу, и это всегда ошибка оформления, а не замысел.
	started := false
	for i, node := range nodes {
		opts := nodeOpts(block, i, started)
		switch node.Type {
		case "page_break":
			if started {
				sb.WriteString(pageBreakRunXML())
				// После разрыва блок снова «начинается»: флаг «с новой
				// страницы» ко второму абзацу того же блока не относится.
				started = false
			}
			continue
		case "heading":
			sb.WriteString(headingXMLOpts(rc.text(node.Text), node.Level, opts))
		case "bullet_list":
			for _, item := range node.Items {
				sb.WriteString(listItemXMLOpts(rc.text(item), opts))
				started = true
			}
			continue
		case "table":
			rows := make([][]string, 0, len(node.Rows))
			for _, row := range node.Rows {
				cells := make([]string, 0, len(row))
				for _, cell := range row {
					cells = append(cells, rc.text(cell))
				}
				rows = append(rows, cells)
			}
			sb.WriteString(tableXML(rows))
			started = true
			continue
		case "links":
			links, ok := renderLinkSection(node, rc.refs)
			if ok {
				sb.WriteString(links)
				started = true
			}
			continue
		case "compare_table":
			// Сравниваются только ВЫБРАННЫЕ системы: карточки в rc — весь кеш
			// раздела, и без отбора в таблицу попали бы все 375 систем.
			if rows := compareSystemRows(cardsForOffer(rc.cards, rc.offer)); len(rows) > 0 {
				if title := strings.TrimSpace(node.Text); title != "" {
					sb.WriteString(headingXMLOpts(rc.text(title), node.Level, opts))
					started = true
				}
				sb.WriteString(tableXML(rows))
				started = true
			}
			continue
		case "prompt_text":
			text := generatedTextFor(rc, block.ID, i)
			if text != "" {
				sb.WriteString(paragraphXMLOpts(text, node.Align, opts))
				started = true
			}
			continue
		case "image":
			// Плавающая картинка с обтеканием: текст идёт рядом, строка не
			// разрывается (2026-11-01). Источник — ключ узла или карточка
			// системы/материала (2026-11-05).
			if markup, ok := showCardImages(context.Background(), node, rc.media, rc.images, rc); ok {
				sb.WriteString(markup)
				started = true
			}
			continue
		default:
			sb.WriteString(renderParagraphNode(node.Text, node.Align, opts, rc))
		}
		started = true
	}
	return sb.String(), nil
}

// nodeOpts — размещение абзаца для узла.
//
// Флаги блока («с новой страницы», отступы, не разрывать) относятся к ПЕРВОМУ
// напечатанному абзацу блока, а не к каждому: иначе «с новой страницы» давало
// бы страницу на каждый абзац.
func nodeOpts(block Block, index int, started bool) paraOpts {
	if started {
		return paraOpts{}
	}
	opts := paraOpts{
		Before:    block.SpacingBefore,
		After:     block.SpacingAfter,
		KeepLines: block.KeepTogether,
	}
	if index == firstPrintedNode(block) {
		opts.PageBreakBefore = block.PageBreakBefore
	}
	return opts
}

// firstPrintedNode — индекс первого узла, который что-то напечатает. Разрыв
// страницы и «молчащие» узлы флаг не забирают.
func firstPrintedNode(block Block) int {
	var nodes []blockNode
	if len(block.Content) > 0 {
		if err := json.Unmarshal(block.Content, &nodes); err != nil {
			return 0
		}
	}
	for i, node := range nodes {
		if node.Type == "page_break" {
			continue
		}
		if node.Type == "links" || node.Type == "prompt_text" {
			// Могут смолчать — но если напечатаются, флаг их. Индекс отдаём
			// первый такой: админ ставит «с новой страницы» на блок целиком.
			return i
		}
		return i
	}
	return 0
}

// renderOfferDocx собирает предложение в DOCX. Порядок: заголовок формы, блоки
// формы (только клиентские), таблица позиций, итог, подпись менеджера. Файл
// строится на корпоративном бланке (шапка/футер/тема остаются как есть).
// renderOfferDocx — сборка без карточек систем: тот же путь, что и раньше.
func renderOfferDocx(form OfferForm, offer Offer) ([]byte, error) {
	return renderOfferDocxFull(form, offer, nil, nil, nil)
}

// renderOfferDocxWithCards — сборка с описаниями и фото из Базы знаний.
//
// Порядок источников: согласованный текст секции (form_values["copy.*"])
// главнее карточки; карточка главнее пустоты. Из карточки берётся только
// описание: состав и цены — из каталога, чтобы в КП не было двух правд.
//
// images может быть nil, карточки и ссылки могут отсутствовать — документ
// собирается в любом случае. Картинка необязательна: нет файла — нет картинки.
// refs — ссылки из карточек (2026-11-01), печатаются узлом `links`.
func renderOfferDocxWithCards(form OfferForm, offer Offer, cards []SystemCard, images ImageSource) ([]byte, error) {
	return renderOfferDocxFull(form, offer, cards, images, nil)
}

// renderOfferDocxFull — то же плюс ссылки из карточек (2026-11-01).
//
// Пустой состав — НЕ ошибка (2026-11-01). Менеджеры на первых шагах берут из
// системы только описательную часть: объект, систему, преимущества, условия, а
// цены вписывают сами. Раньше документ в этом случае не собирался вовсе, и
// «Собрать КП» без позиций упиралось в отказ. Теперь печатается всё, кроме
// таблицы состава, а вместо неё — честная строка о том, что состав передаётся
// отдельно. Врать про «итого 0,00» нельзя: это выглядит как бесплатно.
func renderOfferDocxFull(form OfferForm, offer Offer, cards []SystemCard, images ImageSource, refs []skills.CardRef) ([]byte, error) {
	return renderOfferDocxWithTexts(form, offer, cards, images, refs, nil, nil)
}

// renderOfferDocxWithTexts — блочный путь с текстами систем (2026-11-07).
//
// Без systemTexts/systemText метка `{{текст: система: …}}` в блоке формы
// печатала пустое место: блоки просто не знали о текстах, которые для макета
// уже читались. Здесь оба пути получают одно и то же, и модель дописывает
// недостающее одинаково (живая жалоба «метка не работает и в блоке формы»).
func renderOfferDocxWithTexts(form OfferForm, offer Offer, cards []SystemCard, images ImageSource,
	refs []skills.CardRef, systemTexts func(SystemCard) map[string]string,
	systemText func(SystemCard, string) string) ([]byte, error) {

	// Все ссылки предложения: из карточек систем, если их не передали.
	if len(refs) == 0 {
		refs = collectOfferRefs(cards)
	}
	// Картинки из формы (узлы `image`) собираются тем же сборщиком, что и
	// картинки карточек: у бланка свои id, и второй сборщик дал бы дубли.
	media := newTemplateMedia()
	// Сборщик ссылок тоже общий: метка `{{ссылки}}` внутри блока печатает
	// настоящую гиперссылку, и её отношение нельзя потерять — документ со
	// ссылкой на неизвестное отношение Word не открывает (2026-11-09).
	links := &layoutRuntime{media: media, numbering: newNumberingPlan(numberingAbstractDecimal)}
	rc := renderCtx{
		values:        offer.FormValues,
		service:       serviceValuesWithCards(offer, cards),
		offer:         offer,
		refs:          refs,
		cards:         cards,
		images:        images,
		media:         media,
		layoutRuntime: links,
		numbering:     links.numbering,
		systemTexts:   systemTexts,
		systemText:    systemText,
	}

	var sb strings.Builder
	// Заголовок, номер и клиента печатает ПЕРВЫЙ блок формы (обычно титульный
	// лист). Расшаблонивать их здесь нельзя: подставленные шаблонные фразы
	// попадали в готовый файл (2026-11-02 — «Текст с полем», английский текст
	// задания модели в экспортированном КП 003/2026). Когда блоков нет вовсе,
	// шапку рисуем сами — документ без названия читать невозможно.
	if len(form.Blocks) == 0 {
		writeOfferHead(&sb, form, offer)
	}

	blocks := visibleBlocks(form)
	for _, block := range blocks {
		content, err := renderBlockContentCtx(block, rc)
		if err != nil {
			return nil, err
		}
		sb.WriteString(content)
	}
	// Название формы — подзаголовок, если шапку не дал ни один блок: форму
	// могли сохранить пустой (админ удалил титульный лист), а документ без
	// названия читать невозможно. Не повторяем, если такой заголовок уже есть.
	if !documentHasTitle(blocks, form.Title) {
		sb.WriteString(paragraphAligned(titleOr(form.Title), ""))
	}

	// Согласованный менеджером текст секций (2026-10-29). Идёт после блоков
	// формы и перед составом: это текст «о предложении», таблица цен — факты.
	// Пустой текст секции не рисуется вовсе: «пропущено» и «пусто» — одно.
	// С 2026-10-30 показ управляется флагом КП и по умолчанию выключен —
	// существующие предложения печатаются как раньше (Task 11).
	if formFlagOn(offer.FormValues, formFlagSectionCopy) {
		renderAcceptedSections(&sb, offer)
	}
	// Сводка брифа: что известно про объект. Выключена по умолчанию: это
	// рабочая информация менеджера, а не всегда уместный текст для клиента.
	if formFlagOn(offer.FormValues, formFlagBriefSummary) {
		renderBriefSummary(&sb, offer.FormValues)
	}

	// Описания систем из карточок Базы знаний: только там, где менеджер не
	// согласовал свой текст (согласованное главнее).
	// Карточки получают тот же сборщик: у формы могли уже быть картинки, и
	// второй сборщик выдал бы повторные id — Word молча ломает вёрстку.
	renderCardSectionsWithMedia(&sb, offer, cards, images, media)

	sb.WriteString(headingXML("Состав предложения", 2))
	if len(offer.Items) == 0 {
		// Состав передаётся отдельно — так и говорим, без нулевой таблицы и
		// без «итого 0,00»: ноль в счёте читается как «бесплатно».
		sb.WriteString(paragraphAligned(
			"Состав и стоимость материалов передаются отдельным документом.", ""))
	} else {
		sb.WriteString(tableXML(offerItemsTable(offer.Items)))
		// Итог — отдельной строкой справа, без красной строки: отступ первой
		// строки на короткой сумме выглядит как сдвинутая вправо колонка.
		sb.WriteString(paragraphXMLOpts("Итого: "+formatMoney(offer.Total)+" руб.", "right",
			paraOpts{NoIndent: true, Before: 6}))
	}

	// Подпись — только если её нет в блоке формы: форму легко испортить
	// шаблонной подписью, и вторая строка «Менеджер» выглядела бы как сбой.
	if !formBlocksMention(form, "{{manager}}") {
		sb.WriteString(paragraphAligned("", ""))
		sb.WriteString(paragraphXMLOpts("Менеджер: "+offer.OwnerEmail, "", paraOpts{NoIndent: true}))
	}

	return buildBodyWithLinks(sb.String(), refs, links, media)
}

// buildBodyWithLinks собирает тело и отношения ссылок и упаковывает документ.
//
// Одна точка на оба пути печати (форма и боевой листок): ссылки раздела
// `links` делаются кликабельными, к ним добавляются отношения, собранные
// меткой `{{ссылки}}` внутри блока (иначе документ сослался бы на неизвестное
// отношение и Word его не открыл бы — 2026-11-09), и всё это уходит в пакет.
func buildBodyWithLinks(body string, refs []skills.CardRef, rt *layoutRuntime, media *docxMedia) ([]byte, error) {
	linked, linkRels := applyHyperlinks(body, refs)
	for _, link := range rt.links {
		linkRels = appendLinkOnce(linkRels, link)
	}
	// Ссылки без отношений печатаются текстом: документ важнее.
	if _, relErr := relsXMLWithLinks(nil, linkRels); relErr != nil {
		linked = body
		linkRels = nil
	}
	return buildDocxWithNumbering(linked, media, linkRels, rt.numbering)
}

// Служебные поля шаблона: то, что служба знает про предложение сама, без
// заполнения админом.
const (
	fieldOfferNumber = "offer_number"
	fieldOfferDate   = "offer_date"
	fieldManager     = "manager"
	fieldClientName  = "offer_client"
	fieldObjectName  = "offer_object"
	fieldSystemTitle = "system_title"
	fieldSystemsList = "systems_list"
)

// ServiceFieldIDs — служебные поля, которые конфигуратор показывает администратору
// справочно: они есть всегда и заполнять их в форме не нужно.
//
// Один список на сервер и плагин: в плагине он продублирован для чипов, и
// расхождение видно сразу (поле-чип, которое сервер не знает).
var ServiceFieldIDs = []string{
	fieldOfferNumber, fieldOfferDate, fieldManager, fieldClientName, fieldObjectName,
	fieldSystemTitle, fieldSystemsList,
}

// serviceValues — значения служебных полей шаблона без карточек.
func serviceValues(offer Offer) map[string]string {
	return serviceValuesWithCards(offer, nil)
}

// serviceValuesWithCards — значения служебных полей шаблона с карточками систем.
//
// Считаются при печати, а не хранятся в form_values: номер, дата, подпись и
// названия выбранных систем есть у каждого предложения, и заполнять их руками
// в форме значит забыть обновить после копирования КП.
func serviceValuesWithCards(offer Offer, cards []SystemCard) map[string]string {
	out := map[string]string{
		fieldOfferNumber: offer.Number,
		fieldOfferDate:   offerDateText(offer.CreatedAt),
		fieldManager:     offer.OwnerEmail,
		fieldClientName:  offer.Client.Name,
		fieldObjectName:  formValue(offer.FormValues, "object_name"),
	}
	if titles := offerSystemTitles(offer, cards); len(titles) > 0 {
		out[fieldSystemTitle] = titles[0]
		out[fieldSystemsList] = strings.Join(titles, "; ")
	}
	return out
}

// titleOr — заголовок документа: сохранённый или заводской.
func titleOr(title string) string {
	if t := strings.TrimSpace(title); t != "" {
		return t
	}
	return "Коммерческое предложение"
}

// visibleBlocks — блоки, которые печатаются клиенту, в порядке формы.
func visibleBlocks(form OfferForm) []Block {
	out := make([]Block, 0, len(form.Blocks))
	for _, block := range form.Blocks {
		if block.ShowInClient {
			out = append(out, block)
		}
	}
	return out
}

// documentHasTitle — есть ли уже название документа среди блоков. Сравнение по
// подстроке: название может стоять внутри строки титульного листа.
func documentHasTitle(blocks []Block, title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return false
	}
	for _, block := range blocks {
		for _, node := range blockNodes(block) {
			if strings.Contains(node.Text, title) {
				return true
			}
			for _, row := range node.Rows {
				for _, cell := range row {
					if strings.Contains(cell, title) {
						return true
					}
				}
			}
		}
	}
	return false
}

// formBlocksMention ищет метку во всех блоках формы, включая таблицы и списки.
func formBlocksMention(form OfferForm, marker string) bool {
	for _, block := range form.Blocks {
		for _, node := range blockNodes(block) {
			if strings.Contains(node.Text, marker) {
				return true
			}
			for _, item := range node.Items {
				if strings.Contains(item, marker) {
					return true
				}
			}
			for _, row := range node.Rows {
				for _, cell := range row {
					if strings.Contains(cell, marker) {
						return true
					}
				}
			}
		}
	}
	return false
}

// formValue — значение из карты формы или пустая строка.
func formValue(values map[string]string, key string) string {
	if values == nil {
		return ""
	}
	return strings.TrimSpace(values[key])
}

// offerDateText — дата предложения в виде «01.11.2026»; пусто, если её нет.
func offerDateText(createdAt string) string {
	if len(createdAt) < 10 {
		return ""
	}
	day, month, year, ok := parseISODate(createdAt[:10])
	if !ok {
		return createdAt[:10]
	}
	return day + "." + month + "." + year
}

// parseISODate разбирает «2026-11-01» на части. Сделано вручную: time.Parse здесь
// тянет часовые пояса, а дата в шапке — просто текст.
func parseISODate(raw string) (day, month, year string, ok bool) {
	parts := strings.Split(raw, "-")
	if len(parts) != 3 || len(parts[0]) != 4 {
		return "", "", "", false
	}
	return parts[2], parts[1], parts[0], true
}

// offerItemsTable — таблица состава для документа.
//
// Колонка ЕКН появилась в 2026-11-02: менеджер и заказчик сверяют позиции по
// номеру. Если ЕКН не известен ни у одной позиции, колонки нет вовсе — пустая
// колонка на всю таблицу выглядит как сбой, а не как «номера нет».
func offerItemsTable(items []OfferItem) [][]string {
	withEkn := false
	for _, item := range items {
		if strings.TrimSpace(item.EKN) != "" {
			withEkn = true
			break
		}
	}
	rows := [][]string{{"№", "Наименование"}}
	if withEkn {
		rows[0] = append(rows[0], "Код ЕКН")
	}
	rows[0] = append(rows[0], "Ед.", "Кол-во", "Цена", "Сумма")
	for i, item := range items {
		row := []string{strconv.Itoa(i + 1), item.Name}
		if withEkn {
			row = append(row, item.EKN)
		}
		row = append(row,
			item.Unit,
			strconv.FormatFloat(item.Qty, 'f', -1, 64),
			formatMoney(item.Price),
			formatMoney(item.Sum),
		)
		rows = append(rows, row)
	}
	return rows
}

// writeOfferHead — название документа, номер с датой и клиент.
//
// Печатается только у формы без блоков: при нормальной форме шапку собирает
// первый блок, и здесь она дала бы второй заголовок и вторую дату.
func writeOfferHead(sb *strings.Builder, form OfferForm, offer Offer) {
	title := strings.TrimSpace(form.Title)
	if title == "" {
		title = "Коммерческое предложение"
	}
	sb.WriteString(`<w:p><w:pPr><w:pStyle w:val="` + styleHeading1 + `"/></w:pPr><w:r><w:rPr><w:b/><w:sz w:val="36"/><w:szCs w:val="36"/></w:rPr><w:t xml:space="preserve">`)
	sb.WriteString(escapeXML(title))
	sb.WriteString(`</w:t></w:r></w:p>`)

	dateLine := "№ " + offer.Number
	// Дата — только если она есть: у предложения, собранного в коде без БД,
	// createdAt пустой, и срез [:10] уронил бы весь экспорт.
	if len(offer.CreatedAt) >= 10 {
		dateLine += " от " + offer.CreatedAt[:10]
	}
	sb.WriteString(paragraphAligned(dateLine, "right"))
	if offer.Client.Name != "" {
		sb.WriteString(paragraphAligned("Клиент: "+offer.Client.Name, ""))
	}
	if offer.Client.Contact != "" {
		sb.WriteString(paragraphAligned("Контактное лицо: "+offer.Client.Contact, ""))
	}
}

// buildDocxFull — сборка с картинками и внешними ссылками.
func buildDocxFull(bodyXML string, media *docxMedia, links []renderedLink, _ []byte) ([]byte, error) {
	return buildDocxWithNumbering(bodyXML, media, links, nil)
}

// buildDocxWithNumbering — сборка с планом нумерации списков (2026-11-08).
//
// plan != nil — в numbering.xml бланка дописываются наши numId, чтобы состав
// каждой системы нумеровался с единицы. nil — прежнее поведение (сквозной
// список), как у документов без составов.
func buildDocxWithNumbering(bodyXML string, media *docxMedia, links []renderedLink, plan *numberingPlan) ([]byte, error) {
	return buildDocxWithMediaAndLinksAndNumbering(bodyXML, media, links, plan)
}

// buildDocx вставляет тело в корпоративный бланк; при его отсутствии собирает
// минимальный документ с тем же оформлением текста.
func buildDocx(bodyXML string) ([]byte, error) {
	return buildDocxWithMedia(bodyXML, nil)
}

// buildDocxWithMedia — сборка с изображениями (2026-10-29).
//
// media может быть nil: документ без картинок. id визуальных элементов
// вычисляются по ключам шаблона (docx_media.go), а не по константе: у бланка
// свои фигуры в колонтитулах, и дубль id ломает вёрстку молча.
func buildDocxWithMedia(bodyXML string, media *docxMedia) ([]byte, error) {
	return buildDocxWithMediaAndLinks(bodyXML, media, nil)
}

// buildDocxWithMediaAndLinks — сборка с картинками и внешними ссылками.
//
// Ссылки — обычные отношения пакета (TargetMode="External"), как картинки:
// дописываются в word/_rels/document.xml.rels после того, как тело уже
// вставлено, чтобы одна и та же ссылка не завела два отношения.
func buildDocxWithMediaAndLinks(bodyXML string, media *docxMedia, links []renderedLink) ([]byte, error) {
	return buildDocxWithMediaAndLinksAndNumbering(bodyXML, media, links, nil)
}

// buildDocxWithMediaAndLinksAndNumbering — то же плюс определения нумерации.
func buildDocxWithMediaAndLinksAndNumbering(bodyXML string, media *docxMedia, links []renderedLink, plan *numberingPlan) ([]byte, error) {
	tr, err := zip.NewReader(bytes.NewReader(standardTemplateDocx), int64(len(standardTemplateDocx)))
	if err != nil {
		return nil, fmt.Errorf("открыть шаблон: %w", err)
	}
	files := map[string][]byte{}
	var names []string
	for _, f := range tr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		files[f.Name] = data
		names = append(names, f.Name)
	}

	docXML, ok := files["word/document.xml"]
	if !ok {
		return nil, fmt.Errorf("в шаблоне нет word/document.xml")
	}
	prefix, suffix, err := splitTemplateBody(string(docXML))
	if err != nil {
		return nil, err
	}
	files["word/document.xml"] = []byte(prefix + bodyXML + suffix)
	// styles.xml и numbering.xml из шаблона НЕ трогаем (2026-10-29).
	//
	// Раньше они подменялись своими короткими версиями — 5 стилей вместо 58 и
	// 2 нумерации вместо 12. Из-за этого в собранном файле пропадала нумерация
	// страниц в нижнем колонтитуле: поля PAGE/SECTIONPAGES живут в текстовых
	// блоках и опираются на стили бланка, а без них Word пересчитывает
	// разметку и вытесняет нумерацию. Таблицы теряли TableParagraph, шапка и
	// футер — свои стили. У ЛИМС та же грабля была и починена так же: менять
	// только word/document.xml.
	//
	// Следствие: разметка обязана пользоваться ТОЛЬКО стилями и нумерациями
	// бланка. За этим следят TestDocxOnlyKnownStyles и TestDocxOnlyKnownNumIds.

	// Изображения и связи дописываем ПОСЛЕ тела: сборщик читает занятые id
	// визуальных элементов по всем частям пакета, включая только что
	// вставленное тело.
	if media != nil {
		if err := applyMedia(files, media); err != nil {
			return nil, err
		}
	}
	// Внешние ссылки: в пакете шаблона document.xml.rels уже есть (в нём
	// колонтитулы), но без него файл тоже соберётся — relsXMLWithLinks
	// создаёт корневой.
	if len(links) > 0 {
		rels, err := relsXMLWithLinks(files["word/_rels/document.xml.rels"], links)
		if err != nil {
			return nil, err
		}
		if _, existed := files["word/_rels/document.xml.rels"]; !existed {
			names = append(names, "word/_rels/document.xml.rels")
		}
		files["word/_rels/document.xml.rels"] = rels
	}
	// Перезапуск нумерации составов: определения наших numId дописываются к
	// numbering.xml бланка (2026-11-08). Без плана файл не меняется.
	injectNumberingDefinitions(files, plan)
	return zipFiles(files, names)
}

// zipFiles складывает пакет обратно в .docx: сначала части в исходном порядке,
// потом добавленные (картинки, новые связи).
//
// Вынесено из сборки по бланку, чтобы печать по макету пользователя не
// заводила второй такой же цикл: порядок частей в zip — часть контракта Word,
// и две копии этого правила разошлись бы молча.
func zipFiles(files map[string][]byte, names []string) ([]byte, error) {
	ensureUpdateFields(files)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// Новые части (word/media/*) в names отсутствуют — пишем их следом.
	written := make(map[string]bool, len(names))
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		written[name] = true
		if _, err := w.Write(files[name]); err != nil {
			return nil, err
		}
	}
	for name, data := range files {
		if written[name] {
			continue
		}
		w, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ensureUpdateFields заставляет Word пересчитать поля (оглавление TOC, PAGEREF)
// при открытии документа (2026-11-08, живая жалоба на КП 003/2026).
//
// Сервер не раскладывает документ по страницам — номера страниц знает только
// сам Word. Без `<w:updateFields w:val="true"/>` в word/settings.xml Word
// показывает кэш оглавления из шаблона, где новых заголовков и актуальных
// страниц ещё нет.
func ensureUpdateFields(files map[string][]byte) {
	settings, ok := files["word/settings.xml"]
	if !ok {
		return
	}
	s := string(settings)
	if strings.Contains(s, "<w:updateFields") {
		return
	}
	const closeTag = "</w:settings>"
	if idx := strings.LastIndex(s, closeTag); idx >= 0 {
		files["word/settings.xml"] = []byte(s[:idx] + `<w:updateFields w:val="true"/>` + s[idx:])
		return
	}
	if idx := strings.LastIndex(s, "/>"); idx >= 0 && strings.Contains(s, "<w:settings") {
		files["word/settings.xml"] = []byte(s[:idx] + `><w:updateFields w:val="true"/></w:settings>` + s[idx+2:])
	}
}

func splitTemplateBody(docXML string) (prefix, suffix string, err error) {
	bodyOpen := strings.Index(docXML, "<w:body>")
	if bodyOpen < 0 {
		return "", "", fmt.Errorf("в шаблоне не найден <w:body>")
	}
	bodyContentStart := bodyOpen + len("<w:body>")
	sectPr := strings.Index(docXML, "<w:sectPr")
	if sectPr < bodyContentStart {
		return "", "", fmt.Errorf("в шаблоне не найден <w:sectPr>")
	}
	return docXML[:bodyContentStart], docXML[sectPr:], nil
}

// renderAcceptedSections рисует согласованный текст секций КП (2026-10-29).
//
// Ключи — copy.<kind> и copy.system.<N> (copystep.go). Порядок: вводный текст,
// описания систем, преимущества, условия. Пустой текст = секция пропущена:
// менеджер мог её не писать, и «пустой абзац» в документе не нужен.
//
// Преимущества — список бланка (styleListBullet), а не абзац с переносами:
// иначе в документе получается текст, а не список.
func renderAcceptedSections(sb *strings.Builder, offer Offer) {
	if offer.FormValues == nil {
		return
	}
	section := func(key string) string {
		return strings.TrimSpace(offer.FormValues[key])
	}
	if text := section(SectionKey(skills.SectionIntro, 0)); text != "" {
		sb.WriteString(paragraphAligned(text, ""))
	}
	// Системы: идём по порядку, пока ключи есть. Количество систем сервер не
	// хранит отдельно — оно живёт в составе предложения и в тексте формы.
	for i := 1; i <= maxSystemSections; i++ {
		text := section(SectionKey(skills.SectionSystem, i))
		if text == "" {
			continue
		}
		sb.WriteString(headingXML("Система "+strconv.Itoa(i), 3))
		sb.WriteString(paragraphAligned(text, ""))
	}
	if text := section(SectionKey(skills.SectionAdvantages, 0)); text != "" {
		for _, line := range strings.Split(text, "\n") {
			item := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "•"))
			if item == "" {
				continue
			}
			sb.WriteString(listItemXML(item))
		}
	}
	if text := section(SectionKey(skills.SectionConditions, 0)); text != "" {
		sb.WriteString(paragraphAligned(text, ""))
	}
}

// Флаги показа в документе (Task 11, 2026-10-30). Живут в form_values, а не в
// схеме формы: это свойство конкретного КП, а не настройка бланка. По
// умолчанию ВЫКЛЮЧЕНЫ — существующие предложения собираются как раньше.
const (
	formFlagBriefSummary = "doc.brief_summary"
	formFlagSectionCopy  = "doc.section_copy"
)

// formFlagOn читает булев флаг из form_values. Нет ключа — выключено.
func formFlagOn(values map[string]string, key string) bool {
	if values == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(values[key])) {
	case "1", "true", "yes", "да":
		return true
	}
	return false
}

// renderBriefSummary печатает сводку брифа: что известно про объект.
//
// Источник — значения формы с префиксом `brief.` (их пишет плагин из брифа).
// Пустые поля пропускаются: сводка не должна печатать пустые строки под
// заголовками, о которых нечего сказать.
func renderBriefSummary(sb *strings.Builder, values map[string]string) {
	type line struct {
		key   string
		label string
	}
	order := []line{
		{"brief.object_name", "Объект"},
		{"brief.customer_goals", "Задачи клиента"},
		{"brief.customer_pains", "Боли"},
		{"brief.project_risks", "Риски"},
		{"brief.decision_maker", "Кто решает"},
		{"brief.timeline", "Сроки"},
		{"brief.budget_signal", "Бюджет"},
		{"brief.why_us", "Почему мы"},
		{"brief.notes", "Прочее"},
	}
	printed := false
	for _, item := range order {
		text := strings.TrimSpace(values[item.key])
		if text == "" {
			continue
		}
		if !printed {
			sb.WriteString(headingXML("Бриф", 2))
			printed = true
		}
		sb.WriteString(paragraphAligned(item.label+": "+text, ""))
	}
}

// maxSystemSections — предел обхода ключей copy.system.N. Систем в КП
// единицы; без предела документ с подменённым form_values мог бы крутиться
// бесконечно долго.
const maxSystemSections = 50

// ImageSource — откуда берутся байты картинки. Интерфейс, а не *S3Store:
// тесты подставляют свою реализацию и не ходят в хранилище.
type ImageSource interface {
	Get(ctx context.Context, key string) ([]byte, error)
}

// renderCardSections рисует описания систем из карточек Базы знаний.
//
// Что попадает в документ:
//   - описание системы — если менеджер не согласовал свой текст для этой
//     системы (согласованное главнее);
//   - картинка продукта — если есть ключ вложения и хранилище отдало файл.
//
// Состав и цены из карточки НЕ берутся: они в каталоге, и второй копии в КП
// быть не должно. Возвращает сборщик изображений, чтобы картинки доехали до
// пакета.
func renderCardSections(sb *strings.Builder, offer Offer, cards []SystemCard, images ImageSource) *docxMedia {
	media := newTemplateMedia()
	renderCardSectionsWithMedia(sb, offer, cards, images, media)
	return media
}

// renderCardSectionsWithMedia — та же печать, но с уже существующим сборщиком
// изображений (2026-11-01): у формы могут быть свои картинки, и второй
// сборщик выдал бы повторные wp:docPr id.
// Фото карточки тоже ставится с обтеканием: текстовое описание системы рядом
// с фото читается ровнее, чем фото отдельной строкой во всю ширину.
func renderCardSectionsWithMedia(sb *strings.Builder, offer Offer, cards []SystemCard, images ImageSource, media *docxMedia) {
	if len(cards) == 0 || media == nil {
		return
	}
	for i, c := range cards {
		if strings.TrimSpace(c.Description) == "" {
			continue
		}
		// Согласованный текст системы главнее карточки.
		if strings.TrimSpace(offer.FormValues[SectionKey(skills.SectionSystem, i+1)]) != "" {
			continue
		}
		sb.WriteString(headingXML("Система: "+c.Title, 3))
		// Описание карточки — несколько абзацев, а не один с \n: живая заметка
		// содержит строки («Особенности системы:») между пустыми строками, и
		// Word символ \n не показывает — строки слипались в кашу (2026-11-05).
		sb.WriteString(paragraphsXML(c.Description, "", paraOpts{}))

		// Разрез системы — ДО состава (образец конкурента, 2026-11-05): сначала
		// пирог, потом его расшифровка. Раньше картинка печаталась после текста
		// и никак не была связана с составом.
		writeCardImage(sb, c, images, media)

		// Состав и характеристики — факты из карточки, а не текст модели.
		// Клиент должен видеть, из чего состоит система; до 2026-11-05 состав
		// уходил только в промпт и в документ не попадал.
		if len(c.Composition) > 0 {
			sb.WriteString(headingXML("Состав системы", 4))
			for _, item := range c.Composition {
				sb.WriteString(listItemXML(item))
			}
		}
		if len(c.Specifications) > 1 {
			sb.WriteString(headingXML("Свойства и преимущества", 4))
			sb.WriteString(tableXML(c.Specifications))
		}
	}
}

// writeCardImage встраивает картинку карточки с обтеканием справа.
//
// Картинка необязательна: нет ключа, нет хранилища или файл не отдался —
// просто ничего не печатаем. Документ важнее картинки.
func writeCardImage(sb *strings.Builder, c SystemCard, images ImageSource, media *docxMedia) {
	if c.ImageKey == "" || images == nil || media == nil {
		return
	}
	data, err := images.Get(context.Background(), c.ImageKey)
	if err != nil || len(data) == 0 {
		return
	}
	ext, contentType := imageExt(c.ImageName)
	drawing := media.addAnchored(data, ext, contentType, cardImageWidth, cardImageHeight,
		imageWrapSpec{side: "right"}, "right")
	if drawing != "" {
		sb.WriteString(`<w:p><w:pPr><w:pStyle w:val="` + styleNormal + `"/><w:ind w:firstLine="0"/></w:pPr>` + drawing + `</w:p>`)
	}
}

// hasCardDescriptions — есть ли в карточках хоть одно описание.
func hasCardDescriptions(cards []SystemCard) bool {
	for _, c := range cards {
		if strings.TrimSpace(c.Description) != "" {
			return true
		}
	}
	return false
}

// Размер картинки продукта в документе: 12 см по ширине (в EMU, 1 см =
// 360000). Высота пропорционально не считается: у нас нет декодера картинок,
// а фиксированный размер не ломает вёрстку.
const (
	cardImageWidth  = 12 * 360000
	cardImageHeight = 9 * 360000
)

// imageExt — расширение и тип по имени файла. По умолчанию jpeg: большинство
// фото продуктов в базе знаний — jpg.
func imageExt(name string) (string, string) {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".png"):
		return "png", "image/png"
	case strings.HasSuffix(lower, ".gif"):
		return "gif", "image/gif"
	default:
		return "jpeg", "image/jpeg"
	}
}
