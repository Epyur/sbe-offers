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
// Повторы по адресу схлопываются: один техлист часто приложен к нескольким
// системам.
//
// В список входят и ссылки карточки (раздел «Ссылки»), и ссылки материалов, на
// которые ведёт её состав (`LinkedRefs`, 2026-11-09): живой технолог просил,
// чтобы в КП попадали документы не только систем, но и продуктов в них.
//
// Порядок — групповой (2026-11-10): страницы продуктов → СТО → техлисты → узлы
// → сертификаты и прочее. Сортировка идёт ПОСЛЕ схлопывания повторов: иначе
// первая встреченная копия адреса могла бы получить «хуже» владельца, и ссылка
// встала бы не на своё место внутри группы.
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
				return skills.SortCardRefsGrouped(out)
			}
		}
	}
	return skills.SortCardRefsGrouped(out)
}

// cardAllRefs — все ссылки карточки: её собственные и материалы состава.
//
// Порядок — групповой (2026-11-10): страницы продуктов, СТО, техлисты, узлы,
// сертификаты и прочее; внутри группы система раньше материала, у сертификатов
// материал раньше системы. Владелец ссылки проставляется здесь: разбор строки
// не знает, чья это карточка, а без владельца порядок внутри группы не
// выстроить. `Refs` принадлежат системе, `LinkedRefs` — материалам состава.
func cardAllRefs(card SystemCard) []skills.CardRef {
	all := make([]skills.CardRef, 0, len(card.Refs)+len(card.LinkedRefs))
	all = append(all, skills.TagCardRefsOwner(card.Refs, skills.RefOwnerSystem)...)
	all = append(all, skills.TagCardRefsOwner(card.LinkedRefs, skills.RefOwnerMaterial)...)
	return skills.SortCardRefsGrouped(all)
}

// renderLinkSection печатает узел `links`. Второе значение — были ли ссылки:
// пустой раздел не печатается вовсе.
//
// Раздел печатается ГРУППАМИ и НУМЕРОВАННЫМ списком (2026-11-10, требование
// пользователя). Группы и их порядок:
//
//  1. страницы продуктов на nav.tn.ru (сначала систем, потом материалов);
//  2. стандарты организации (СТО);
//  3. технические листы (сначала систем, потом материалов);
//  4. узлы;
//  5. сертификаты и декларации (сначала материалов, потом систем) — сюда же
//     всё остальное.
//
// Нумерация в каждой группе начинается с единицы: у каждого списка свой numId
// из плана сборки (plan.newList), и Word не продолжает счёт предыдущей группы.
// plan == nil (печать без плана) — сквозной номер бланка, как раньше.
func renderLinkSection(node blockNode, refs []skills.CardRef, plan *numberingPlan) (string, bool) {
	if len(refs) == 0 {
		return "", false
	}
	// Сортировка здесь, а не только в сборщике: ссылки могут прийти снаружи
	// (renderOfferDocxFull), и порядок групп не должен зависеть от того, кто
	// их передал. SortCardRefsGrouped не меняет вход.
	refs = skills.SortCardRefsGrouped(refs)
	opts := paraOpts{Before: 12}
	var sb strings.Builder
	sb.WriteString(headingXMLOpts("Ссылки", node.Level, opts))
	for _, group := range skills.GroupCardRefsByGroup(refs) {
		// Заголовок группы — четвёртый уровень: это подраздел внутри «Ссылок»,
		// и он не должен попадать в оглавление как раздел документа.
		sb.WriteString(headingXMLOpts(group.Title, linkGroupHeadingLevel, paraOpts{Before: 6}))
		numID := styleListNumbered
		if plan != nil {
			numID = plan.newList()
		}
		for _, ref := range group.Refs {
			// Видимый текст пункта — подпись, а не адрес (2026-11-09). Раньше
			// печаталось «подпись — адрес»: адрес с параметрами занимал полстроки
			// и выглядел как сбой вёрстки. Адрес попадает только в отношение
			// (`applyHyperlinks`), то есть в саму ссылку, и остаётся кликабельным.
			sb.WriteString(listItemNumXML(refDisplayText(ref), numID, paraOpts{}))
		}
	}
	if len(refs) >= maxLinkRefs {
		sb.WriteString(paragraphAligned(
			"Ссылок в карточках больше — остальные есть в Базе знаний.", ""))
	}
	return sb.String(), true
}

// linkGroupHeadingLevel — уровень заголовка группы ссылок в документе.
const linkGroupHeadingLevel = 4

// refDisplayText — то, что читатель видит вместо ссылки: подпись, если она
// есть, иначе адрес. Одна функция на все места печати ссылок (раздел формы,
// метка макета), иначе они разойдутся: где-то останется «подпись — адрес».
func refDisplayText(ref skills.CardRef) string {
	label := cleanRefLabel(ref.Label)
	if label != "" && label != ref.URL {
		return label
	}
	return ref.URL
}

// cleanRefLabel снимает с подписи остатки markdown, если ими оказалась сама
// подпись. Так выглядят ссылки в кеше карточек, прочитанном старым разбором
// (до 0.9.4): подпись = вся строка `[название](адрес…)`. Печатать это клиенту
// — тот же дефект, из-за которого выпуск и делался, поэтому чистим здесь, а не
// ждём перечитывания Базы знаний: старый кеш иначе испортит каждый документ.
func cleanRefLabel(label string) string {
	label = strings.TrimSpace(label)
	if !strings.HasPrefix(label, "[") {
		return label
	}
	if cut, _, ok := strings.Cut(label, "]("); ok {
		return strings.TrimSpace(strings.TrimPrefix(cut, "["))
	}
	return label
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
		//
		// Пункт теперь нумерованный (2026-11-10): у него свой numId у каждой
		// группы, поэтому точную разметку пункта не собрать наперёд. Ищем
		// абзац по тексту и делаем кликабельным его прогон — так пункт
		// остаётся пунктом своей группы, а подпись — ссылкой.
		updated, ok := hyperlinkParagraphByText(out, hyperlinkItemLabel(ref), ref.URL)
		if !ok {
			continue
		}
		out = updated
		links = append(links, renderedLink{RelID: placeholderRelID(ref.URL), URL: ref.URL})
	}
	return out, links
}

// hyperlinkParagraphByText находит в теле абзац-пункт с текстом text и делает
// его прогон кликабельной ссылкой на url. Второй результат — нашли ли: нет
// текста — нет и отношения (иначе в rels появится ссылка без тела).
//
// Ищется именно пункт (у него есть <w:numPr>), а не любой абзац: в документе
// тот же текст мог напечатать человек, и превращать его в ссылку — не просьба.
//
// Сохраняется ВЕСЬ абзац, включая `<w:pPr>` с номером: ссылка меняет только
// прогон с подписью, а нумерация группы остаётся на месте (2026-11-10). Раньше
// пункт был маркированным, и замену собирали целиком; с нумерацией номер ставит
// Word по `<w:numPr>`, и выбросить его значит оставить пункт без номера.
func hyperlinkParagraphByText(bodyXML, text, url string) (string, bool) {
	needle := `<w:t xml:space="preserve">` + escapeXML(text) + `</w:t>`
	idx := strings.Index(bodyXML, needle)
	if idx < 0 {
		return bodyXML, false
	}
	start := strings.LastIndex(bodyXML[:idx], "<w:p>")
	if start < 0 {
		return bodyXML, false
	}
	end := strings.Index(bodyXML[idx:], "</w:p>")
	if end < 0 {
		return bodyXML, false
	}
	paragraph := bodyXML[start : idx+end+len("</w:p>")]
	if !strings.Contains(paragraph, "<w:numPr>") {
		// Не пункт списка — раздел ссылок печатает только пункты. Не трогаем:
		// абзац мог напечатать человек тем же текстом.
		return bodyXML, false
	}
	replacement := hyperlinkRunsInParagraph(paragraph, text, url)
	if replacement == paragraph {
		return bodyXML, false
	}
	replaced := strings.Replace(bodyXML, paragraph, replacement, 1)
	return replaced, replaced != bodyXML
}

// hyperlinkRunsInParagraph оборачивает прогон с подписью в <w:hyperlink>,
// сохраняя <w:pPr> абзаца (стиль, отступ, номер пункта) как есть.
func hyperlinkRunsInParagraph(paragraph, text, url string) string {
	run := `<w:r><w:rPr><w:rStyle w:val="` + styleHyperlink + `"/></w:rPr>` +
		`<w:t xml:space="preserve">` + escapeXML(text) + `</w:t></w:r>`
	hyperlink := `<w:hyperlink r:id="` + placeholderRelID(url) + `" w:history="1">` + run + `</w:hyperlink>`
	// Прогон пункта собирается тем же runsXML, что и обычный текст (без
	// особого rPr). Заменяем первый такой прогон с нужным текстом.
	plainRun := `<w:r><w:t xml:space="preserve">` + escapeXML(text) + `</w:t></w:r>`
	if strings.Contains(paragraph, plainRun) {
		return strings.Replace(paragraph, plainRun, hyperlink, 1)
	}
	// runsXML мог добавить rPr (например, оформление) — тогда ищем <w:t> и
	// оборачиваем весь прогон вокруг него.
	return wrapRunAroundText(paragraph, text, hyperlink)
}

// wrapRunAroundText оборачивает в готовую разметку целиком тот <w:r>, внутри
// которого стоит <w:t> с нужным текстом. Узкий запасной путь: обычный пункт
// раздела ссылок проходит проверку выше.
func wrapRunAroundText(paragraph, text, markup string) string {
	needle := `<w:t xml:space="preserve">` + escapeXML(text) + `</w:t>`
	idx := strings.Index(paragraph, needle)
	if idx < 0 {
		return paragraph
	}
	runStart := strings.LastIndex(paragraph[:idx], "<w:r>")
	runEnd := strings.Index(paragraph[idx:], "</w:r>")
	if runStart < 0 || runEnd < 0 {
		return paragraph
	}
	runEnd += idx + len("</w:r>")
	return paragraph[:runStart] + markup + paragraph[runEnd:]
}

// hyperlinkItemLabel — текст пункта раздела ссылок. Одна функция на печать и на
// замену: расхождение здесь значило бы «ссылку не нашли и не сделали».
func hyperlinkItemLabel(ref skills.CardRef) string {
	// plainText — то же преобразование, что делает печать пункта
	// (`listItemNumXML` → `runsXML(plainText(...))`). Без него подпись с
	// разметкой («**жирный**», вики-ссылка) после печати выглядела бы иначе,
	// и «текст пункта» не нашёлся бы в теле при замене на ссылку.
	return plainText(refDisplayText(ref))
}

// hyperlinkListItemNumXML — нумерованный пункт списка, в котором весь текст
// кликабелен.
//
// Нужен там, где ссылку печатает метка макета: подпись видна читателю, а адрес
// живёт только в отношении. Номер ставит Word по `<w:numPr>` с numId группы; в
// тексте пункта числа нет (2026-11-10).
func hyperlinkListItemNumXML(relID, text, numID string) string {
	if relID == "" {
		return listItemNumXML(text, numID, paraOpts{})
	}
	return `<w:p><w:pPr><w:pStyle w:val="` + styleNormal + `"/><w:ind w:firstLine="0"/>` +
		`<w:numPr><w:ilvl w:val="0"/><w:numId w:val="` + escapeXML(numID) + `"/></w:numPr></w:pPr>` +
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
