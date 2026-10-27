package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
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
	pPr.WriteString(`<w:pStyle w:val="Normal"/>`)
	if j := jcVal(align); j != "" {
		pPr.WriteString(`<w:jc w:val="` + j + `"/>`)
	}
	return `<w:p><w:pPr>` + pPr.String() + `</w:pPr><w:r><w:t xml:space="preserve">` +
		escapeXML(text) + `</w:t></w:r></w:p>`
}

func listItemXML(text string) string {
	return `<w:p><w:pPr><w:pStyle w:val="Normal"/><w:ind w:firstLine="0"/>` +
		`<w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr>` +
		`<w:r><w:t xml:space="preserve">` + escapeXML(text) + `</w:t></w:r></w:p>`
}

func headingXML(text string, level int) string {
	// level 2 → Heading1, 3 → Heading2, 4 → Heading3 (в styles.xml заведены
	// Heading1..3).
	styleNum := 1
	switch level {
	case 3:
		styleNum = 2
	case 4:
		styleNum = 3
	}
	return `<w:p><w:pPr><w:pStyle w:val="Heading` + strconv.Itoa(styleNum) + `"/></w:pPr>` +
		`<w:r><w:t xml:space="preserve">` + escapeXML(text) + `</w:t></w:r></w:p>`
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

// formatMoney — сумма в русском формате: «1 234,56».
func formatMoney(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	cents := int64(roundMoney(v)*100 + 0.5)
	whole := cents / 100
	frac := cents % 100
	digits := strconv.FormatInt(whole, 10)
	var parts []string
	for len(digits) > 3 {
		parts = append([]string{digits[len(digits)-3:]}, parts...)
		digits = digits[:len(digits)-3]
	}
	parts = append([]string{digits}, parts...)
	out := strings.Join(parts, " ") + "," + fmt.Sprintf("%02d", frac)
	if neg {
		return "-" + out
	}
	return out
}

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
func renderOfferDocx(form OfferForm, offer Offer) ([]byte, error) {
	if len(offer.Items) == 0 {
		return nil, fmt.Errorf("в предложении нет позиций")
	}

	var sb strings.Builder
	title := strings.TrimSpace(form.Title)
	if title == "" {
		title = "Коммерческое предложение"
	}
	sb.WriteString(`<w:p><w:pPr><w:pStyle w:val="Title"/></w:pPr><w:r><w:t xml:space="preserve">`)
	sb.WriteString(escapeXML(title))
	sb.WriteString(`</w:t></w:r></w:p>`)

	number := offer.Number
	dateLine := "№ " + number + " от " + offer.CreatedAt[:10]
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

	return buildDocx(sb.String())
}

// buildDocx вставляет тело в корпоративный бланк; при его отсутствии собирает
// минимальный документ с тем же оформлением текста.
func buildDocx(bodyXML string) ([]byte, error) {
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
	files["word/styles.xml"] = []byte(stylesXML)
	files["word/numbering.xml"] = []byte(numberingXML)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		content := files[name]
		if name == "word/document.xml" || name == "word/styles.xml" || name == "word/numbering.xml" {
			content = files[name]
		}
		if _, err := w.Write(content); err != nil {
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
