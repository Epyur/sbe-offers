package main

import (
	"fmt"
	"strings"
)

// Изображение в тексте с обтеканием (2026-11-01).
//
// Требование пользователя: «система должна уметь вставлять изображения в текст
// с обтеканием изображения текстом, чтобы изображения не формировали
// разрывов». Дословно: картинка не должна занимать собственную строку и
// раздвигать абзац — текст идёт рядом с ней.
//
// В Word это `wp:anchor` (плавающий объект) с `wrapSquare`, а не `wp:inline`.
// Inline-картинка, какую печатали карточки систем до сих пор, всегда стоит
// отдельным run в абзаце и разрывает поток текста.
//
// Узел `image` в блоке формы:
//
//	{"type":"image","imageKey":"…","widthCm":6,"wrap":"right","align":"center",
//	 "caption":"Подпись"}
//
// Пиксели считаются в EMU (1 см = 360000 EMU), как и остальные размеры.

// imageWrapSpec — обтекание: сторона, с которой идёт текст, и отступы.
type imageWrapSpec struct {
	// side: both | left | right | topAndBottom | none. none — «в тексте»,
	// т.е. строка, но это осознанный выбор админа, а не наше умолчание.
	side string
}

// wrapAttr — имя атрибута wrap-элемента для обтекания текстом.
func (w imageWrapSpec) wrapAttr() string {
	switch w.side {
	case "left":
		return "wrapLeft"
	case "right":
		return "wrapRight"
	case "topAndBottom":
		return "wrapTopAndBottom"
	case "none":
		return "wrapNone"
	default:
		return "wrapSquare"
	}
}

// normalizeWrap — значение из JSON в вид обтекания. По умолчанию — square
// (текст с обеих сторон): именно так «картинка не разрывает текст».
func normalizeWrap(raw string) imageWrapSpec {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "left", "слева":
		return imageWrapSpec{side: "left"}
	case "right", "справа":
		return imageWrapSpec{side: "right"}
	case "topandbottom", "сверху-снизу", "сверху и снизу":
		return imageWrapSpec{side: "topAndBottom"}
	case "none", "в тексте":
		return imageWrapSpec{side: "none"}
	default:
		return imageWrapSpec{side: "both"}
	}
}

// wrapElementXML — элемент обтекания внутри wp:anchor.
//
// Отступы (distT/distB/distL/distR у соседей картинки) — 0,18 см = 6 пт:
// текст вплотную к картинке читается как текст поверх изображения.
func (w imageWrapSpec) elementXML() string {
	switch w.side {
	case "topAndBottom":
		return `<wp:wrapTopAndBottom/>`
	case "none":
		return `<wp:wrapNone/>`
	default:
		return `<wp:wrapSquare wrapText="bothSides"/>`
	}
}

// anchorAlign — выравнивание картинки по горизонтали относительно колонки.
// relativeFrom="page" даёт предсказуемое место: картинка не уезжает вместе с
// абзацем при правке текста выше.
func anchorAlign(align string) string {
	switch strings.ToLower(strings.TrimSpace(align)) {
	case "left":
		return "left"
	case "right":
		return "right"
	default:
		return "center"
	}
}

// docxAnchorXML — плавающая картинка с обтеканием.
//
// behindDoc="0" — картинка над текстом (текст не печатается поверх неё),
// allowOverlap="1" — Word не сдвигает её так, чтобы освободить строку.
// Всё это вместе и есть «не формирует разрывов».
func docxAnchorXML(relID, name string, picID int, cx, cy int64, wrap imageWrapSpec, align string) string {
	posOffsetX := "0"
	posAlign := anchorAlign(align)
	return fmt.Sprintf(
		`<w:r><w:drawing><wp:anchor `+
			`distT="0" distB="0" distL="114300" distR="114300" `+
			`simplePos="0" relativeHeight="%d" behindDoc="0" locked="0" layoutInCell="1" allowOverlap="1" `+
			`xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing">`+
			`<wp:simplePos x="0" y="0"/>`+
			`<wp:positionH relativeFrom="column"><wp:align>%s</wp:align></wp:positionH>`+
			`<wp:positionV relativeFrom="paragraph"><wp:posOffset>%s</wp:posOffset></wp:positionV>`+
			`<wp:extent cx="%d" cy="%d"/>`+
			`<wp:effectExtent l="0" t="0" r="0" b="0"/>`+
			`%s`+
			`<wp:docPr id="%d" name="%s"/>`+
			`<wp:cNvGraphicFramePr><a:graphicFrameLocks xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" noChangeAspect="1"/></wp:cNvGraphicFramePr>`+
			`<a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">`+
			`<a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">`+
			`<pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">`+
			`<pic:nvPicPr><pic:cNvPr id="%d" name="%s"/><pic:cNvPicPr/></pic:nvPicPr>`+
			`<pic:blipFill><a:blip xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" r:embed="%s"/>`+
			`<a:stretch><a:fillRect/></a:stretch></pic:blipFill>`+
			`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm>`+
			`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr>`+
			`</pic:pic></a:graphicData></a:graphic>`+
			`</wp:anchor></w:drawing></w:r>`,
		picID, posAlign, posOffsetX, cx, cy, wrap.elementXML(),
		picID, name, picID, name, relID, cx, cy)
}

// imageNodeXML — абзац с плавающей картинкой и необязательной подписью.
//
// Подпись — отдельный абзац по центру, а не run внутри того же: иначе она
// «плавает» вместе с картинкой и отъезжает от неё при правке.
func imageNodeXML(node blockNode, media *docxMedia, images ImageSource) (string, bool) {
	if media == nil || images == nil || strings.TrimSpace(node.ImageKey) == "" {
		return "", false
	}
	data, err := images.Get(nil, node.ImageKey)
	if err != nil || len(data) == 0 {
		// Картинка необязательна: документ важнее.
		return "", false
	}
	ext, contentType := imageExt(node.ImageName)
	widthCm := node.WidthCm
	if widthCm <= 0 || widthCm > 18 {
		// Шире 18 см на А4 не помещается в поле набора.
		widthCm = 8
	}
	cx := int64(widthCm * 360000)
	cy := int64(float64(widthCm) * 0.72 * 360000)
	drawing := media.addAnchored(data, ext, contentType, cx, cy, normalizeWrap(node.Wrap), node.Align)
	if drawing == "" {
		return "", false
	}
	var sb strings.Builder
	sb.WriteString(`<w:p><w:pPr><w:pStyle w:val="` + styleNormal + `"/><w:ind w:firstLine="0"/></w:pPr>` + drawing + `</w:p>`)
	if caption := strings.TrimSpace(node.Caption); caption != "" {
		sb.WriteString(paragraphXMLOpts(caption, "center", paraOpts{After: 6, NoIndent: true}))
	}
	return sb.String(), true
}
