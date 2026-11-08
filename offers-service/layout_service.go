package main

import (
	"context"
	"log"
	"strings"
)

// Печать служебных меток макета (2026-11-06, шаг C плана «генерация без
// модели»).
//
// Повод — живая жалоба: администратор поменял структуру промтов в макете, и
// система «ушла думать и не вернулась». Причина измерена: модель звалась при
// каждой сборке документа, ей разрешено 180 с на попытку, а сервер резал ответ
// на 90-й секунде. Лечится это не скоростью модели, а тем, что данные уже есть
// — в предложении, в карточке Базы знаний, в сохранённых текстах системы.
//
// Поэтому здесь НЕТ вызовов модели. Совсем: метка, попавшая в этот файл,
// печатает то, что уже лежит, иначе молчит. Молчание — тоже ответ: пустое место
// честнее черновика.

// layoutMaterialRepeatLimit — сколько позиций печатать повтором по материалам.
//
// Решение пользователя 2026-11-06: не больше десяти. В живом КП бывает 41
// позиция, и «41 × абзац описания» — это не КП, а книга; выше предела печатается
// строка о том, что остальное — в приложении, и полная таблица состава.
const layoutMaterialRepeatLimit = 10

// layoutServiceRow — какая система/материал конкретного прохода.
//
// Пустой указатель — обычная (неповторяемая) печать: метка берёт первую
// выбранную систему. Это состояние передаётся значением, чтобы повтор не
// оставлял след в контексте сборки.
type layoutServiceRow struct {
	// system — карточка текущей системы повтора; nil — первая выбранная.
	system *SystemCard
	// material — номер позиции повтора (1-based); 0 — не повтор по материалам.
	material int
	// viaBlock — печать идёт внутри блока формы: метка отдаёт ТЕКСТ, а не
	// разметку пункта (узел абзаца сам её не примет).
	viaBlock bool
}

// renderLayoutService печатает одну служебную метку.
func renderLayoutService(token layoutToken, lc layoutContext, row layoutServiceRow) string {
	spec := token.Service
	switch spec.kind {
	case layoutTokenBlock:
		return renderLayoutBlock(spec.blockID, lc)
	case layoutTokenSystem:
		return renderLayoutSystemCard(lc, row, spec.variant == "table")
	case layoutTokenLinks:
		return renderLayoutCardLinks(lc, row)
	case layoutTokenSpecs:
		return renderLayoutCardSpecs(lc, row)
	case layoutTokenText:
		return renderLayoutCardText(lc, row, spec.textBlock)
	case layoutTokenMaterial:
		return renderLayoutMaterial(lc, row)
	}
	return ""
}

// renderLayoutMaterial печатает строку состава по позиции предложения.
//
// Смысл метки — в повторе по материалам (шаг D): блок с repeat:"materials"
// даёт строку для каждой позиции. Без повтора печатается первая позиция —
// как и у системы.
func renderLayoutMaterial(lc layoutContext, row layoutServiceRow) string {
	idx := row.material
	if idx <= 0 {
		idx = 1
	}
	if idx > len(lc.offer.Items) {
		return ""
	}
	item := lc.offer.Items[idx-1]
	name := strings.TrimSpace(item.Name)
	if name == "" {
		return ""
	}
	// В макете — пункт списка; внутри блока формы (expandBlockServiceMarkers)
	// метка печатается как текст, поэтому разметку пункта решает вызывающий.
	if row.viaBlock {
		return name
	}
	return listItemXMLOpts(name, paraOpts{})
}

// renderLayoutBlock печатает блок формы целиком по его id.
//
// Именно по ID, а не по позиции в списке (решение пользователя 2026-11-06):
// блоков в форме может быть десять, и «третий» после добавления нового
// означает уже другой блок. Неизвестный id печатает пусто и пишет в журнал:
// литерал `{{block:9}}` в готовом документе недопустим.
//
// Блоки печатаются с раскрытыми служебными метками в своём тексте: у блока
// может быть ребёнок-сам-себе: `{{block:1}}` внутри блока 1 не раскрывается,
// поэтому глубина ограничена.
func renderLayoutBlock(blockID string, lc layoutContext) string {
	if lc.form == nil {
		return ""
	}
	block, ok := findFormBlock(lc.form().Blocks, blockID)
	if !ok {
		log.Printf("макет: блок %q не найден в форме — метка молчит", blockID)
		return ""
	}
	// Повтор блока (шаг D): печатается по разу на систему/позицию. Это
	// ПЕЧАТЬ, а не генерация — модель не зовётся ни разу, сколько бы систем
	// ни было выбрано.
	rows, limited := layoutRepeatRows(lc, block.RepeatName())
	if len(rows) > 0 {
		var sb strings.Builder
		for _, row := range rows {
			sb.WriteString(renderLayoutBlockWith(block, lc, row))
		}
		if limited > 0 {
			// Предел повтора по материалам: выше него печатается честная
			// строка о приложении, а не документ-книга (шаг D).
			sb.WriteString(paragraphXMLOpts(
				"Остальные "+itoa(limited-layoutMaterialRepeatLimit)+
					" позиций — в приложении к предложению.", "", paraOpts{Before: 6}))
		}
		return sb.String()
	}
	return renderLayoutBlockWith(block, lc, layoutServiceRow{})
}

// renderLayoutBlockWith печатает блок один раз, с данными прохода.
func renderLayoutBlockWith(block Block, lc layoutContext, row layoutServiceRow) string {
	rc := lc.renderCtx()
	if row.system != nil {
		// Внутри повтора метка системы и {{system_title}} в блоке указывают на
		// СВОЮ систему, а не на первую.
		rc = withRowSystem(rc, *row.system)
	}
	if row.material > 0 && row.material <= len(lc.offer.Items) {
		rc.materialSource = lc.offer.Items
		rc.offer.Items = []OfferItem{lc.offer.Items[row.material-1]}
		rc.materialIndex = row.material
	}
	content, err := renderBlockContentCtx(block, rc)
	if err != nil {
		log.Printf("макет: блок %q не напечатан: %v", block.ID, err)
		return ""
	}
	return content
}

// withRowSystem привязывает контекст блока к системе текущего прохода повтора.
func withRowSystem(rc renderCtx, card SystemCard) renderCtx {
	rc.cards = []SystemCard{card}
	rc.offer.Systems = nil
	rc.systemCard = &card
	title := strings.TrimSpace(card.Title)
	if title == "" {
		return rc
	}
	rc.service = copyMapWith(rc.service, fieldSystemTitle, title)
	if rc.values != nil && rc.values[fieldSystemTitle] != "" {
		rc.values = copyMapWith(rc.values, fieldSystemTitle, title)
	}
	return rc
}

// copyMapWith возвращает копию карты с подставленным ключом.
func copyMapWith(src map[string]string, key, value string) map[string]string {
	out := make(map[string]string, len(src)+1)
	for k, v := range src {
		out[k] = v
	}
	out[key] = value
	return out
}

// findFormBlock ищет блок по id.
func findFormBlock(blocks []Block, id string) (Block, bool) {
	id = strings.TrimSpace(id)
	for _, block := range blocks {
		if strings.TrimSpace(block.ID) == id {
			return block, true
		}
	}
	return Block{}, false
}

// renderCtx собирает контекст блоков формы из контекста макета.
//
// Одна точка перевода: у печати по макету и у блоков формы разные структуры
// контекста, и второй ручной сборки не должно быть — иначе служебные поля
// (номер, дата, менеджер) доедут до блока или не доедут в зависимости от
// причины.
func (lc layoutContext) renderCtx() renderCtx {
	return renderCtx{
		values:      lc.values,
		service:     lc.service,
		offer:       lc.offer,
		refs:        lc.refs,
		cards:       lc.cardList(),
		images:      lc.images,
		media:       lc.media(),
		numbering:   lc.numberingPlan(),
		systemTexts: lc.systemTexts,
		selection:   lc.selectionHandle(),
	}
}

// cardList — карточки, если они переданы.
func (lc layoutContext) cardList() []SystemCard {
	if lc.cards == nil {
		return nil
	}
	return lc.cards()
}

// media — сборщик картинок текущей сборки.
func (lc layoutContext) media() *docxMedia {
	if lc.layout == nil {
		return nil
	}
	return lc.layout.media
}

// numberingPlan — план нумерации текущей сборки (nil — сквозной список).
func (lc layoutContext) numberingPlan() *numberingPlan {
	if lc.layout == nil {
		return nil
	}
	return lc.layout.numbering
}

// layoutRowCard — карточка, которую печатает метка.
//
// Повтор передаёт свою систему явно (шаг D); без повтора берётся первая
// выбранная — та же, что печаталась до появления повторов.
func layoutRowCard(lc layoutContext, row layoutServiceRow) (SystemCard, bool) {
	if row.system != nil {
		return *row.system, true
	}
	cards := cardsForOffer(lc.cardList(), lc.offer)
	if len(cards) == 0 {
		return SystemCard{}, false
	}
	return cards[0], true
}

// renderLayoutSystemCard печатает карточку системы: картинку и состав.
//
// Решение пользователя 2026-11-06: картинка и состав — ОДНА таблица без границ.
// Ячейка с картинкой и ячейка со списком состава стоят рядом, Word держит их
// вместе и не разрывает страницей. Три ручные метки («картинка», «состав»,
// «отступ») этим заменяются одной.
//
// plain — `{{система: таблица}}`: картинка и состав друг под другом без
// таблицы, для макетов, где двухколоночная врезка не встаёт.
func renderLayoutSystemCard(lc layoutContext, row layoutServiceRow, plain bool) string {
	card, ok := layoutRowCard(lc, row)
	if !ok {
		return ""
	}
	image := layoutCardImageMarkup(card, lc)
	composition := layoutCompositionMarkup(card, lc.numberingPlan())
	if plain {
		return image + composition
	}
	if image == "" && composition == "" {
		// Пустая карточка — метка молчит: пустая таблица в документе выглядит
		// сломанной.
		return ""
	}
	// Одна из частей пуста — печатается одна ячейка того же вида: читатель
	// видит ровно то, что есть у карточки, без пустого столбца.
	return layoutBorderlessTable(image, composition)
}

// layoutCardImageMarkup — картинка карточки в ячейке таблицы.
//
// Ширина фиксированная (layoutCardImageWidthCm = 10 см): пользователь просил
// одну таблицу без границ, а не размер, и лишний параметр в метке только
// запутал бы разметку.
func layoutCardImageMarkup(card SystemCard, lc layoutContext) string {
	if strings.TrimSpace(card.ImageKey) == "" || lc.images == nil || lc.media() == nil {
		return ""
	}
	data, err := lc.images.Get(context.Background(), card.ImageKey)
	if err != nil || len(data) == 0 {
		// Картинка необязательна: документ важнее.
		return ""
	}
	ext, contentType := imageExt(card.ImageName)
	widthCm := layoutCardImageWidthCm
	cx := int64(widthCm * 360000)
	cy := int64(float64(widthCm) * 0.72 * 360000)
	drawing := lc.media().add(data, ext, contentType, cx, cy)
	if drawing == "" {
		return ""
	}
	return imageParagraphXML(drawing)
}

// compositionHeading — подпись списка состава.
const compositionHeading = "Состав системы"

// layoutCompositionMarkup — список состава карточки.
//
// Пункты идут тем же конвейером текста, что и остальной документ: вики-ссылки
// печатаются подписью, степени в единицах — надстрочными.
//
// Список НУМЕРОВАННЫЙ (2026-11-07, живая жалоба): карточка Базы знаний нумерует
// слои, и на разрезе системы маркеры слоёв стоят с номерами. Маркированный
// список сопоставить с разрезом было нельзя. Номер в карточке разбор уже снял
// (reCardNumber), так что его ставит Word: он идёт подряд и не задваивается.
//
// Перезапуск нумерации у каждой системы (2026-11-08, живая жалоба КП 003/2026):
// у каждого состава теперь свой numId из плана сборки. С общим numId второй
// список продолжал первый, и слои второй системы шли с 13, а не с 1.
func layoutCompositionMarkup(card SystemCard, plan *numberingPlan) string {
	items := compositionLines(card)
	if len(items) == 0 {
		return ""
	}
	numID := styleListNumbered
	if plan != nil {
		numID = plan.newList()
	}
	var sb strings.Builder
	sb.WriteString(headingXMLOpts(compositionHeading, 4, paraOpts{NoIndent: true}))
	for _, item := range items {
		sb.WriteString(listItemNumXML(item, numID, paraOpts{}))
	}
	return sb.String()
}

// compositionLines — строки состава без служебных пометок.
func compositionLines(card SystemCard) []string {
	out := make([]string, 0, len(card.Composition))
	for _, line := range card.Composition {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// layoutBorderlessTable — две ячейки без единой границы.
//
// Без границ намеренно: это не таблица данных, а врезка «картинка + список».
// Строится настоящим `<w:tbl>`, а не абзацами: только таблица держит две
// ячейки рядом и не даёт разорвать их страницей.
func layoutBorderlessTable(left, right string) string {
	var sb strings.Builder
	sb.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="5000" w:type="pct"/><w:tblBorders>`)
	for _, edge := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
		sb.WriteString(`<w:` + edge + ` w:val="none" w:sz="0" w:space="0" w:color="auto"/>`)
	}
	sb.WriteString(`</w:tblBorders></w:tblPr><w:tr>`)
	// Пустая часть не даёт пустого столбца: ячейка создаётся, только если в
	// ней что-то есть.
	for _, part := range []string{left, right} {
		if strings.TrimSpace(part) == "" {
			continue
		}
		sb.WriteString(`<w:tc><w:tcPr><w:tcW w:w="4526" w:type="dxa"/></w:tcPr>` + cellBody(part) + `</w:tc>`)
	}
	sb.WriteString(`</w:tr></w:tbl>`)
	return sb.String()
}

// cellBody — содержимое ячейки; пустая ячейка остаётся пустым абзацем, иначе
// Word считает таблицу битой.
func cellBody(markup string) string {
	if strings.TrimSpace(markup) == "" {
		return `<w:p><w:pPr><w:ind w:firstLine="0"/></w:pPr></w:p>`
	}
	return markup
}

// renderLayoutCardLinks — ссылки карточки кликабельными.
//
// Адреса печатаются через общий конвейер ссылок макета (linkLayoutDocument):
// он превращает любой адрес в абзаце в настоящую гиперссылку. Второго
// механизма не заводим — он разошёлся бы с первым.
func renderLayoutCardLinks(lc layoutContext, row layoutServiceRow) string {
	card, ok := layoutRowCard(lc, row)
	if !ok {
		return ""
	}
	refs := card.Refs
	if len(refs) == 0 {
		refs = lc.refs
	}
	if len(refs) > maxLinkRefs {
		refs = refs[:maxLinkRefs]
	}
	var sb strings.Builder
	for _, ref := range refs {
		if ref.URL == "" {
			continue
		}
		label := strings.TrimSpace(ref.Label)
		if label != "" && label != ref.URL {
			sb.WriteString(listItemXML(label + " — " + ref.URL))
			continue
		}
		sb.WriteString(listItemXML(ref.URL))
	}
	return sb.String()
}

// renderLayoutCardSpecs — характеристики карточки таблицей.
func renderLayoutCardSpecs(lc layoutContext, row layoutServiceRow) string {
	card, ok := layoutRowCard(lc, row)
	if !ok {
		return ""
	}
	if len(card.Specifications) == 0 {
		return ""
	}
	return tableXML(card.Specifications)
}

// renderLayoutCardText — текст системы: сохранённый, а при его отсутствии —
// дописанный.
//
// Текст пишется при выборе системы (шаг B) и лежит в `offers_system_text`. До
// 2026-11-07 метка печатала ТОЛЬКО сохранённое и при пустом месте молчала — из-за
// этого `{{текст: система: description}}` «не работала» ни в макете, ни в блоке
// формы: у живой системы текстов не было вовсе (замер: таблица пуста), и человек
// видел пустое место, а не текст.
//
// Правило теперь симметрично выбору системы: пустых систем не бывает. Сборка
// дописывает недостающее тем же вызовом, сохраняет и печатает — второй раз эта
// работа не делается, дальше текст берётся из таблицы. Ошибка модели по-прежнему
// не роняет документ: блок молчит, а причина уходит в журнал генерации.
func renderLayoutCardText(lc layoutContext, row layoutServiceRow, block string) string {
	card, ok := layoutRowCard(lc, row)
	if !ok {
		return ""
	}
	text := strings.TrimSpace(storedSystemText(lc, card, block))
	if text == "" && lc.systemText != nil {
		text = strings.TrimSpace(lc.systemText(card, block))
	}
	if text == "" {
		// Нет ни сохранённого, ни написанного — метка молчит: пустое место
		// честнее выдумки, а причина записана журналом генерации.
		log.Printf("макет: текст %q системы %d не написан — метка молчит", block, card.NoteID)
		return ""
	}
	if row.viaBlock {
		// Внутри узла блока текст печатает сам узел: отдаём строки как есть.
		return text
	}
	return layoutAsParagraphs(`<w:p><w:pPr><w:jc w:val="both"/></w:pPr></w:p>`, layoutLines(text))
}

// storedSystemText — уже сохранённый текст блока.
func storedSystemText(lc layoutContext, card SystemCard, block string) string {
	if lc.systemTexts == nil {
		return ""
	}
	return lc.systemTexts(card)[block]
}

// layoutRepeatRows — сколько проходов делает повтор и по каким данным.
//
// Повтор задаётся атрибутом `name` у метки и типом повтора у блока формы
// (шаг D): блок с repeat:"systems" печатается по разу на выбранную систему,
// repeat:"materials" — по позициям предложения (не больше десяти).
//
// Второе значение — сколько позиций было всего, если предел превышен: по нему
// печатается строка о приложении.
func layoutRepeatRows(lc layoutContext, name string) ([]layoutServiceRow, int) {
	repeat, ok := layoutRepeatKind(lc, name)
	if !ok {
		return nil, 0
	}
	var rows []layoutServiceRow
	switch repeat {
	case blockRepeatSystems:
		cards := cardsForOffer(lc.cardList(), lc.offer)
		for i := range cards {
			rows = append(rows, layoutServiceRow{system: &cards[i]})
		}
		return rows, 0
	case blockRepeatMaterials:
		total := len(lc.offer.Items)
		count := total
		if count > layoutMaterialRepeatLimit {
			count = layoutMaterialRepeatLimit
		}
		for i := 1; i <= count; i++ {
			rows = append(rows, layoutServiceRow{material: i})
		}
		if total > count {
			return rows, total
		}
		return rows, 0
	}
	return nil, 0
}

// layoutRepeatKind — каким повтором помечен блок с этим именем.
func layoutRepeatKind(lc layoutContext, name string) (string, bool) {
	if lc.form == nil || strings.TrimSpace(name) == "" {
		return "", false
	}
	for _, block := range lc.form().Blocks {
		if strings.TrimSpace(block.RepeatName()) != strings.TrimSpace(name) {
			continue
		}
		if block.Repeat != "" {
			return block.Repeat, true
		}
	}
	return "", false
}
