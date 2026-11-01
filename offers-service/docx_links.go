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
func collectOfferRefs(cards []SystemCard) []skills.CardRef {
	if len(cards) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	out := make([]skills.CardRef, 0, len(cards)*4)
	for _, card := range cards {
		for _, ref := range skills.SortCardRefs(card.Refs) {
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
		// Без отношения печатаем адрес текстом: ссылка без кликабельности
		// хуже, чем видимый адрес, который можно набрать руками.
		if ref.Label != "" && ref.Label != ref.URL {
			sb.WriteString(listItemXML(ref.Label + " — " + ref.URL))
		} else {
			sb.WriteString(listItemXML(ref.URL))
		}
	}
	if len(refs) >= maxLinkRefs {
		sb.WriteString(paragraphXML(
			fmt.Sprintf("Ссылок в карточках больше — остальные есть в Базе знаний."), ""))
	}
	return sb.String(), true
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
		if ref.URL == "" || strings.ContainsAny(ref.URL, `"<>`) {
			// Отношение с кавычкой в адресе сломает XML; такой адрес rare,
			// но печатать его текстом честнее, чем ломать документ.
			continue
		}
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
	if ref.Label != "" && ref.Label != ref.URL {
		return ref.Label + " — " + ref.URL
	}
	return ref.URL
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
