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

func paragraphXML(text, align string) string {
	var pPr strings.Builder
	pPr.WriteString(`<w:pStyle w:val="` + styleNormal + `"/>`)
	if j := jcVal(align); j != "" {
		pPr.WriteString(`<w:jc w:val="` + j + `"/>`)
	}
	return `<w:p><w:pPr>` + pPr.String() + `</w:pPr><w:r><w:t xml:space="preserve">` +
		escapeXML(text) + `</w:t></w:r></w:p>`
}

func listItemXML(text string) string {
	return `<w:p><w:pPr><w:pStyle w:val="` + styleNormal + `"/><w:ind w:firstLine="0"/>` +
		`<w:numPr><w:ilvl w:val="0"/><w:numId w:val="` + styleListBullet + `"/></w:numPr></w:pPr>` +
		`<w:r><w:t xml:space="preserve">` + escapeXML(text) + `</w:t></w:r></w:p>`
}

func headingXML(text string, level int) string {
	// В бланке есть только «heading 1» (styleId "1"). Уровни 3 и 4 отличаются
	// размером шрифта, а не стилем: своих Heading2/Heading3 в бланке нет, и
	// ссылаться на несуществующий стиль нельзя — Word покажет дефолтный шрифт.
	size := "28" // 14pt
	switch level {
	case 3:
		size = "26" // 13pt
	case 4:
		size = "24" // 12pt
	}
	return `<w:p><w:pPr><w:pStyle w:val="` + styleHeading1 + `"/></w:pPr>` +
		`<w:r><w:rPr><w:b/><w:sz w:val="` + size + `"/><w:szCs w:val="` + size + `"/></w:rPr>` +
		`<w:t xml:space="preserve">` + escapeXML(text) + `</w:t></w:r></w:p>`
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
		`<w:p><w:pPr>` + pPr + `</w:pPr><w:r><w:rPr>` + rPr + `</w:rPr><w:t xml:space="preserve">` +
		escapeXML(text) + `</w:t></w:r></w:p></w:tc>`
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

// resolvePlaceholders подставляет {{id}} значениями формы. Неизвестные поля
// заменяются пустой строкой — в документе не должно остаться служебных скобок.
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
type blockNode struct {
	Type  string     `json:"type"`
	Level int        `json:"level"`
	Align string     `json:"align"`
	Text  string     `json:"text"`
	Items []string   `json:"items"`
	Rows  [][]string `json:"rows"`
}

func renderBlockContent(block Block, values map[string]string) (string, error) {
	var nodes []blockNode
	if len(block.Content) > 0 {
		if err := json.Unmarshal(block.Content, &nodes); err != nil {
			return "", fmt.Errorf("разбор блока «%s»: %w", block.Title, err)
		}
	}
	var sb strings.Builder
	for _, node := range nodes {
		switch node.Type {
		case "heading":
			sb.WriteString(headingXML(resolvePlaceholders(node.Text, values), node.Level))
		case "bullet_list":
			for _, item := range node.Items {
				sb.WriteString(listItemXML(resolvePlaceholders(item, values)))
			}
		case "table":
			rows := make([][]string, 0, len(node.Rows))
			for _, row := range node.Rows {
				cells := make([]string, 0, len(row))
				for _, cell := range row {
					cells = append(cells, resolvePlaceholders(cell, values))
				}
				rows = append(rows, cells)
			}
			sb.WriteString(tableXML(rows))
		default:
			sb.WriteString(paragraphXML(resolvePlaceholders(node.Text, values), node.Align))
		}
	}
	return sb.String(), nil
}

// renderOfferDocx собирает предложение в DOCX. Порядок: заголовок формы, блоки
// формы (только клиентские), таблица позиций, итог, подпись менеджера. Файл
// строится на корпоративном бланке (шапка/футер/тема остаются как есть).
// renderOfferDocx — сборка без карточек систем: тот же путь, что и раньше.
func renderOfferDocx(form OfferForm, offer Offer) ([]byte, error) {
	return renderOfferDocxWithCards(form, offer, nil, nil)
}

// renderOfferDocxWithCards — сборка с описаниями и фото из Базы знаний.
//
// Порядок источников: согласованный текст секции (form_values["copy.*"])
// главнее карточки; карточка главнее пустоты. Из карточки берётся только
// описание: состав и цены — из каталога, чтобы в КП не было двух правд.
//
// images может быть nil, карточки могут отсутствовать — документ собирается в
// любом случае. Картинка необязательна: нет файла — нет картинки.
func renderOfferDocxWithCards(form OfferForm, offer Offer, cards []SystemCard, images ImageSource) ([]byte, error) {
	if len(offer.Items) == 0 {
		return nil, fmt.Errorf("в предложении нет позиций")
	}

	var sb strings.Builder
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
	sb.WriteString(paragraphXML(dateLine, "right"))
	if offer.Client.Name != "" {
		sb.WriteString(paragraphXML("Клиент: "+offer.Client.Name, ""))
	}
	if offer.Client.Contact != "" {
		sb.WriteString(paragraphXML("Контактное лицо: "+offer.Client.Contact, ""))
	}

	for _, block := range form.Blocks {
		if !block.ShowInClient {
			continue
		}
		content, err := renderBlockContent(block, offer.FormValues)
		if err != nil {
			return nil, err
		}
		sb.WriteString(content)
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

	// Описания систем из карточек Базы знаний: только там, где менеджер не
	// согласовал свой текст (согласованное главнее).
	cardMedia := renderCardSections(&sb, offer, cards, images)

	sb.WriteString(headingXML("Состав предложения", 2))
	rows := [][]string{{"№", "Наименование", "Ед.", "Кол-во", "Цена", "Сумма"}}
	for i, item := range offer.Items {
		rows = append(rows, []string{
			strconv.Itoa(i + 1),
			item.Name,
			item.Unit,
			strconv.FormatFloat(item.Qty, 'f', -1, 64),
			formatMoney(item.Price),
			formatMoney(item.Sum),
		})
	}
	sb.WriteString(tableXML(rows))
	sb.WriteString(paragraphXML("Итого: "+formatMoney(offer.Total)+" руб.", "right"))

	manager := offer.OwnerEmail
	sb.WriteString(paragraphXML("", ""))
	sb.WriteString(paragraphXML("Менеджер: "+manager, ""))

	return buildDocxWithMedia(sb.String(), cardMedia)
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
		sb.WriteString(paragraphXML(text, ""))
	}
	// Системы: идём по порядку, пока ключи есть. Количество систем сервер не
	// хранит отдельно — оно живёт в составе предложения и в тексте формы.
	for i := 1; i <= maxSystemSections; i++ {
		text := section(SectionKey(skills.SectionSystem, i))
		if text == "" {
			continue
		}
		sb.WriteString(headingXML("Система "+strconv.Itoa(i), 3))
		sb.WriteString(paragraphXML(text, ""))
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
		sb.WriteString(paragraphXML(text, ""))
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
		sb.WriteString(paragraphXML(item.label+": "+text, ""))
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
	if len(cards) == 0 {
		return nil
	}
	bySKU := make(map[string]SystemCard, len(cards))
	for _, c := range cards {
		if c.SKU != "" {
			bySKU[strings.ToUpper(c.SKU)] = c
		}
	}
	if len(bySKU) == 0 {
		return nil
	}

	// Список систем предложения в порядке строк состава: catalogId → SKU.
	// Без каталога под рукой его нет, поэтому идём по карточкам, у которых есть
	// описание, и не дублируем те, что уже согласованы менеджером.
	// Занятые id визуальных элементов берём из бланка: в колонтитулах свои
	// фигуры (1..25), и сборщик без карты падает на первой же картинке.
	media := newTemplateMedia()
	for i, c := range cards {
		if strings.TrimSpace(c.Description) == "" {
			continue
		}
		// Согласованный текст системы главнее карточки.
		if strings.TrimSpace(offer.FormValues[SectionKey(skills.SectionSystem, i+1)]) != "" {
			continue
		}
		sb.WriteString(headingXML("Система: "+c.Title, 3))
		sb.WriteString(paragraphXML(c.Description, ""))

		if c.ImageKey == "" || images == nil {
			continue
		}
		data, err := images.Get(context.Background(), c.ImageKey)
		if err != nil || len(data) == 0 {
			// Картинка необязательна: документ важнее.
			continue
		}
		ext, contentType := imageExt(c.ImageName)
		drawing := media.add(data, ext, contentType, cardImageWidth, cardImageHeight)
		if drawing != "" {
			sb.WriteString(`<w:p><w:pPr><w:jc w:val="center"/></w:pPr>` + drawing + `</w:p>`)
		}
	}
	if len(media.images) == 0 && !hasCardDescriptions(cards) {
		return nil
	}
	return media
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
