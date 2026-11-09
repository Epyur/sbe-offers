package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"mailers/offers/skills"
)

// Ссылки в документе (2026-11-01).
//
// В карточках Базы знаний есть страницы на корпоративном сайте и техлисты на
// навигаторе. Пользователь просил печатать их в конце КП. Печатаем списком:
// «Название — адрес», сначала системы, потом материалы.
//
// Ссылки в Word — внешние отношения (<w:hyperlink r:id="…">). Отношения
// собираются по всему документу и дописываются в word/_rels/document.xml.rels
// один раз: так одна и та же ссылка не заводит два отношения.

// maxLinkRefs — предел ссылок в разделе. У системы с богатой карточкой их
// бывает десяток, у десяти систем — сотня; печатать всё без предела нельзя,
// а обрезать молча — вдвойне.
const maxLinkRefs = 60

// linkSectionRendered — как ссылку напечатали: для отношений Word.
type renderedLink struct {
	RelID string
	URL   string
}

// collectOfferRefs собирает ссылки предложения из карточек систем.
//
// Порядок: сначала системы в том порядке, в котором они попали в документ,
// потом их ссылки по виду (страница раньше документа). Повторы по адресу
// схлопываются: один техлист часто приложен к нескольким системам.
//
// В список входят и ссылки карточки (раздел «Ссылки»), и ссылки материалов, на
// которые ведёт её состав (`LinkedRefs`, 2026-11-09): живой технолог просил,
// чтобы в КП попадали документы не только систем, но и продуктов в них.
func collectOfferRefs(cards []SystemCard) []skills.CardRef {
	if len(cards) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	out := make([]skills.CardRef, 0, len(cards)*4)
	for _, card := range cards {
		for _, ref := range cardAllRefs(card) {
			if ref.URL == "" || seen[ref.URL] {
				continue
			}
			seen[ref.URL] = true
			out = append(out, ref)
			if len(out) >= maxLinkRefs {
				return out
			}
		}
	}
	return out
}

// cardAllRefs — все ссылки карточки: её собственные и материалы состава.
// Отсортированы общим правилом (страницы раньше документов).
func cardAllRefs(card SystemCard) []skills.CardRef {
	if len(card.LinkedRefs) == 0 {
		return skills.SortCardRefs(card.Refs)
	}
	all := make([]skills.CardRef, 0, len(card.Refs)+len(card.LinkedRefs))
	all = append(all, card.Refs...)
	all = append(all, card.LinkedRefs...)
	return skills.SortCardRefs(all)
}

// renderLinkSection печатает узел `links`. Второе значение — были ли ссылки:
// пустой раздел не печатается вовсе.
func renderLinkSection(node blockNode, refs []skills.CardRef) (string, bool) {
	if len(refs) == 0 {
		return "", false
	}
	title := "Ссылки"
	opts := paraOpts{Before: 12}
	var sb strings.Builder
	sb.WriteString(headingXMLOpts(title, node.Level, opts))
	for _, ref := range refs {
		// Видимый текст пункта — подпись, а не адрес (2026-11-09). Раньше
		// печаталось «подпись — адрес»: адрес с параметрами занимал полстроки
		// и выглядел как сбой вёрстки. Адрес попадает только в отношение
		// (`applyHyperlinks`), то есть в саму ссылку, и остаётся кликабельным.
		// Подписи нет — показываем адрес: другого текста для пункта нет.
		sb.WriteString(listItemXML(refDisplayText(ref)))
	}
	if len(refs) >= maxLinkRefs {
		sb.WriteString(paragraphAligned(
			"Ссылок в карточках больше — остальные есть в Базе знаний.", ""))
	}
	return sb.String(), true
}

// refDisplayText — то, что читатель видит вместо ссылки: подпись, если она
// есть, иначе адрес. Одна функция на все места печати ссылок (раздел формы,
// метка макета), иначе они разойдутся: где-то останется «подпись — адрес».
func refDisplayText(ref skills.CardRef) string {
	label := strings.TrimSpace(ref.Label)
	if label != "" && label != ref.URL {
		return label
	}
	return ref.URL
}

// applyHyperlinks превращает адреса в разделе ссылок в кликабельные.
//
// Проход по готовой разметке, а не сборка при печати: адрес в тексте и адрес в
// отношении обязаны совпасть побайтно, иначе Word откроет не то. Берутся только
// абзацы, созданные разделом ссылок: в остальном документе адреса может
// набрать человек, и превращать их в ссылки — не его просьба.
func applyHyperlinks(bodyXML string, refs []skills.CardRef) (string, []renderedLink) {
	if len(refs) == 0 {
		return bodyXML, nil
	}
	out := bodyXML
	links := make([]renderedLink, 0, len(refs))
	for _, ref := range refs {
		if ref.URL == "" {
			continue
		}
		// Кавычка в адресе — обычное дело у техлистов навигатора
		// (`?response-content-disposition=filename="…"`). В атрибут отношения
		// она попадает как `&quot;` (escapeXML), а Word читает её обратно
		// кавычкой. Раньше такие адреса пропускались целиком, и документ
		// печатал их голым текстом без ссылки (2026-11-09).
		item := listItemXML(hyperlinkItemLabel(ref))
		replacement := hyperlinkXML(placeholderRelID(ref.URL), hyperlinkItemLabel(ref), ref.URL)
		if !strings.Contains(out, item) {
			continue
		}
		out = strings.Replace(out, item, replacement, 1)
		links = append(links, renderedLink{RelID: placeholderRelID(ref.URL), URL: ref.URL})
	}
	return out, links
}

// hyperlinkItemLabel — текст пункта раздела ссылок. Одна функция на печать и на
// замену: расхождение здесь значило бы «ссылку не нашли и не сделали».
func hyperlinkItemLabel(ref skills.CardRef) string {
	return refDisplayText(ref)
}

// hyperlinkListItemXML — пункт списка, в котором весь текст кликабелен.
//
// Нужен ровно там, где ссылку печатает метка макета: подпись видна читателю, а
// адрес живёт только в отношении. Отдельный от hyperlinkXML, потому что там
// абзац без маркера — у метки `{{ссылки}}` пункт был списком.
func hyperlinkListItemXML(relID, text string) string {
	if relID == "" {
		return listItemXML(text)
	}
	return `<w:p><w:pPr><w:pStyle w:val="` + styleNormal + `"/><w:ind w:firstLine="0"/>` +
		`<w:numPr><w:ilvl w:val="0"/><w:numId w:val="` + styleListBullet + `"/></w:numPr></w:pPr>` +
		`<w:hyperlink r:id="` + escapeXML(relID) + `" w:history="1">` +
		`<w:r><w:rPr><w:rStyle w:val="` + styleHyperlink + `"/></w:rPr>` +
		`<w:t xml:space="preserve">` + escapeXML(text) + `</w:t></w:r></w:hyperlink></w:p>`
}

// appendLinkOnce добавляет отношение, если адреса ещё нет. Один адрес — одно
// отношение: тот же техлист бывает приложен к нескольким системам.
func appendLinkOnce(links []renderedLink, link renderedLink) []renderedLink {
	for _, existing := range links {
		if existing.RelID == link.RelID || existing.URL == link.URL {
			return links
		}
	}
	return append(links, link)
}

// placeholderRelID — устойчивый id отношения. Индекс, а не адрес: id в Word
// должен быть простым (rIdN), а адрес бывает длинным и с параметрами.
func placeholderRelID(url string) string {
	// Хеш по адресу без импорта криптографии: номер строки не обязан быть
	// криптостойким, важно, чтобы один адрес дал один id.
	var h uint64 = 1469598103934665603
	for i := 0; i < len(url); i++ {
		h ^= uint64(url[i])
		h *= 1099511628211
	}
	return fmt.Sprintf("rIdLink%d", h%1000000)
}

// relsXMLWithLinks дописывает внешние отношения в word/_rels/document.xml.rels.
//
// Файла может не быть (документ без картинок) — тогда создаём корневой.
// Существующие отношения не трогаются: у бланка свои картинки и колонтитулы.
func relsXMLWithLinks(existing []byte, links []renderedLink) ([]byte, error) {
	if len(links) == 0 {
		return existing, nil
	}
	base := string(existing)
	if strings.TrimSpace(base) == "" {
		base = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`</Relationships>`
	}
	var sb strings.Builder
	for _, link := range links {
		if strings.Contains(base, `Id="`+link.RelID+`"`) {
			continue
		}
		sb.WriteString(`<Relationship Id="` + link.RelID + `" ` +
			`Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" ` +
			`Target="` + escapeXML(link.URL) + `" TargetMode="External"/>`)
	}
	if sb.Len() == 0 {
		return existing, nil
	}
	idx := strings.LastIndex(base, "</Relationships>")
	if idx < 0 {
		return nil, fmt.Errorf("в document.xml.rels нет </Relationships>")
	}
	return []byte(base[:idx] + sb.String() + base[idx:]), nil
}

// blockNodes — узлы блока как есть (для проверок и для редактора).
func blockNodes(block Block) []blockNode {
	var nodes []blockNode
	if len(block.Content) == 0 {
		return nil
	}
	if err := json.Unmarshal(block.Content, &nodes); err != nil {
		return nil
	}
	return nodes
}
