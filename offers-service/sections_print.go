package main

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"mailers/offers/model"
	"mailers/offers/skills"
)

// Печать подраздела содержательного раздела (2026-11-14, шаг 5).
//
// Контракт с моделью заморожен (START-HERE, «The contract to freeze»): ответ
// подраздела — маленький JSON `{blocks:[…]}`, и только пять видов блоков:
// heading, paragraph, list, table, figure. Незнакомый вид выбрасывается и
// НИКОГДА не печатается разметкой: литерал `{"kind":"chart"}` в готовом КП —
// это утечка разметки, а не «почти получилось».
//
// С 2026-11-17 (шаг 5 плана) добавлен шестой вид — `diagram`: mermaid-код,
// который сервер растеризует в PNG (спека §7.2: `lang` только `"mermaid"`,
// SVG не поддерживается). Ошибка рендера печатается ВИДИМОЙ строкой, а не
// молчанием (§9.2): пустое место менеджер прочитает как недоделанный документ.
//
// Оформление — из макета: заголовки идут стилем заголовка макета с outlineLvl,
// абзацы — стилем Normal с выравниванием по обоим краям, списки — нумерацией
// макета, таблицы — настоящим `<w:tbl>` того же вида, что печатают позиции,
// рисунок — ссылка на картинку КП (не байты: модель картинок не видит).

// sectionBlock — один блок ответа модели.
//
// Поля — ровно те, что в замороженном контракте. Всё прочее `encoding/json`
// молча игнорирует, и это правильно: расширение ответа не должно ломать печать.
type sectionBlock struct {
	Kind    string     `json:"kind"`
	Level   int        `json:"level,omitempty"`
	Text    string     `json:"text,omitempty"`
	Ordered bool       `json:"ordered,omitempty"`
	Items   []string   `json:"items,omitempty"`
	Caption string     `json:"caption,omitempty"`
	Rows    [][]string `json:"rows,omitempty"`
	Of      string     `json:"of,omitempty"`
	Index   int        `json:"index,omitempty"`
	// Lang / Code — только у вида `diagram` (2026-11-17, шаг 5). Lang моделей
	// обязан быть `mermaid`; Code — mermaid-текст, который сервер растеризует
	// в PNG. Схема приходит КОДОМ, а не байтами: модель картинок не видит.
	Lang string `json:"lang,omitempty"`
	Code string `json:"code,omitempty"`
}

// sectionAnswer — ответ модели на один подраздел.
type sectionAnswer struct {
	Blocks []sectionBlock `json:"blocks"`
}

// sectionBlockKinds — разрешённые виды. Всё прочее выбрасывается.
var sectionBlockKinds = map[string]bool{
	"heading": true, "paragraph": true, "list": true, "table": true, "figure": true,
	"diagram": true,
}

// parseSectionAnswer разбирает ответ модели терпимо, как layoutAnswer: модель
// отвечает по-разному (ограждение ```json, пояснением впереди, обрывом), и
// строгий разбор бросил бы весь подраздел.
//
// Незнакомый вид блока выбрасывается: он не ломает соседей и не печатается.
func parseSectionAnswer(raw string) (sectionAnswer, bool) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return sectionAnswer{}, false
	}
	candidate := raw[start : end+1]
	var answer sectionAnswer
	if err := json.Unmarshal([]byte(candidate), &answer); err != nil {
		// Ремонт — общей функцией из model (обрывы и лишние запятые встречаются
		// в живых ответах). Второй ремонт здесь означал бы вторую копию правила.
		value, repaired := model.RepairJSON(candidate)
		if !repaired {
			log.Printf("раздел: ответ модели не разобран: %v", err)
			return sectionAnswer{}, false
		}
		return normalizeSectionAnswer(encodeAnswerValue(value))
	}
	return normalizeSectionAnswer(&answer)
}

// encodeAnswerValue приводит восстановленное значение к ответу через JSON.
func encodeAnswerValue(value any) *sectionAnswer {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var answer sectionAnswer
	if err := json.Unmarshal(raw, &answer); err != nil {
		return nil
	}
	return &answer
}

// normalizeSectionAnswer выбрасывает неизвестные и пустые блоки.
//
// Заодно убирает из текста пометки-заглушки модели вида `[НУЖНО: …]` (2026-11-17):
// промт просит модель помечать нехватку факта, но печатать такую пометку в
// готовом документе (хоть КП, хоть боевом листке) нельзя — она видна читателю
// как недоделка. Убираем В ОДНОМ месте, на разборе ответа: второй такой проход
// в печати разошёлся бы с первым (правило «одно решение в одном месте»).
func normalizeSectionAnswer(answer *sectionAnswer) (sectionAnswer, bool) {
	if answer == nil {
		return sectionAnswer{}, false
	}
	kept := make([]sectionBlock, 0, len(answer.Blocks))
	for _, block := range answer.Blocks {
		kind := strings.ToLower(strings.TrimSpace(block.Kind))
		if !sectionBlockKinds[kind] {
			log.Printf("раздел: вид блока %q не печатается", block.Kind)
			continue
		}
		block.Kind = kind
		stripFactNotesFromBlock(&block)
		if blockIsEmpty(block) {
			continue
		}
		kept = append(kept, block)
	}
	return sectionAnswer{Blocks: kept}, len(kept) > 0
}

// stripFactNotesFromBlock выкидывает `[НУЖНО: …]` из всех текстовых полей блока.
//
// Пустые после чистки список/строки таблицы отбрасываются тут же, чтобы блок не
// остался с пустым пунктом или пустой строкой.
func stripFactNotesFromBlock(block *sectionBlock) {
	block.Text = stripFactNotes(block.Text)
	block.Caption = stripFactNotes(block.Caption)
	for i := range block.Items {
		block.Items[i] = stripFactNotes(block.Items[i])
	}
	for r := range block.Rows {
		for c := range block.Rows[r] {
			block.Rows[r][c] = stripFactNotes(block.Rows[r][c])
		}
	}
	if block.Kind == "list" {
		block.Items = nonEmptyStrings(block.Items)
	}
	if block.Kind == "table" {
		rows := make([][]string, 0, len(block.Rows))
		for _, row := range block.Rows {
			if len(nonEmptyStrings(row)) == 0 {
				continue
			}
			rows = append(rows, row)
		}
		block.Rows = rows
	}
}

// blockIsEmpty — блок, который нечего печатать.
func blockIsEmpty(block sectionBlock) bool {
	switch block.Kind {
	case "heading", "paragraph":
		return strings.TrimSpace(block.Text) == ""
	case "list":
		return len(nonEmptyStrings(block.Items)) == 0
	case "table":
		return len(block.Rows) == 0
	case "figure":
		return block.Index <= 0
	case "diagram":
		return strings.TrimSpace(block.Code) == ""
	}
	return true
}

// printSectionAnswer печатает разобранный ответ через конструкции макета.
//
// Контекст печати берёт счётчик схем документа; его нет — свой (см.
// sectionDiagramCounter). Так подпись «Схема N» сквозная по документу, а не
// внутри подраздела (спека §9.2: «нумерация без двойного «Схема N»»).
func printSectionAnswer(answer sectionAnswer, lc layoutContext) string {
	counter := lc.sectionNumbering
	if counter == nil {
		counter = &sectionDiagramCounter{}
	}
	lc.sectionNumbering = counter
	var sb strings.Builder
	for _, block := range answer.Blocks {
		sb.WriteString(printSectionBlock(block, lc))
	}
	return sb.String()
}

// printSectionBlock — один блок ответа.
func printSectionBlock(block sectionBlock, lc layoutContext) string {
	switch block.Kind {
	case "heading":
		return sectionHeadingXML(block.Text, block.Level, lc)
	case "paragraph":
		return sectionParagraphXML(block.Text)
	case "list":
		return sectionListXML(nonEmptyStrings(block.Items), block.Ordered, lc)
	case "table":
		return sectionTableXML(block)
	case "figure":
		return sectionFigureXML(block, lc)
	case "diagram":
		return sectionDiagramXML(block, lc)
	}
	return ""
}

// stripFactNotes удаляет из строки пометки модели «[НУЖНО: …]».
//
// Пометка может стоять посреди предложения («… по вашему объекту [НУЖНО: …].»).
// После удаления чистим два очевидных следа: лишний пробел перед знаком и
// двойные пробелы. Пустой результат — пустая строка (блок/пункт отбросит
// вызывающий). Разбор ручной, а не регуляркой по `\b` — кириллица (правило
// AGENTS.md): ищем «[НУЖНО» и «]».
func stripFactNotes(text string) string {
	if !strings.Contains(text, "[НУЖНО") {
		return text
	}
	var b strings.Builder
	for {
		start := strings.Index(text, "[НУЖНО")
		if start < 0 {
			b.WriteString(text)
			break
		}
		b.WriteString(text[:start])
		rest := text[start:]
		end := strings.Index(rest, "]")
		if end < 0 {
			// Незакрытая пометка: режем до конца — текст после неё всё равно
			// часть пометки, которую модель не дописала.
			break
		}
		rest = rest[end+1:]
		// Пометка стояла отдельным предложением, а модель продолжила его тире
		// («… . [НУЖНО: …] — тогда …»): после вырезания остаётся висячий
		// обрывок. Убираем его до конца фразы (2026-11-19).
		if noteOwnSentence(b.String()) {
			rest = reSeamDanglingDash.ReplaceAllString(rest, "")
		}
		text = rest
	}
	out := b.String()
	// Следы удаления: пробел перед пунктуацией, двойные пробелы и ШОВ из двух
	// знаков. Шов виден в готовом документе: «камеры, [НУЖНО: …], иначе» после
	// чистки давало «камеры,, иначе», а «по техлистам: [НУЖНО: цифры].» —
	// «по техлистам:.» (живой листок 017/2026). Чистим только знаки, а не слова:
	// выдумывать за модель текст нельзя.
	out = reSpaceBeforePunct.ReplaceAllString(out, "$1")
	out = reSeamDoubled.ReplaceAllString(out, "$1")
	out = reSeamBeforeDot.ReplaceAllString(out, ".")
	out = reSeamDoubleDot.ReplaceAllString(out, ".")
	out = reMultiSpace.ReplaceAllString(out, " ")
	out = strings.TrimLeft(out, ",;: ")
	return strings.TrimSpace(out)
}

// noteOwnSentence — пометка модели стояла отдельным предложением.
//
// Перед ней конец фразы или начало текста. Только в этом случае тире после
// пометки — продолжение, которого больше нет: «…документами. [НУЖНО: срок] —
// тогда …». Пометка посреди фразы («камеры, [НУЖНО: …], иначе») под правило не
// попадает, иначе чистка съедала бы живой текст.
func noteOwnSentence(prefix string) bool {
	prefix = strings.TrimRight(prefix, " \t\n")
	if prefix == "" {
		return true
	}
	for _, term := range []string{".", "!", "?", "…"} {
		if strings.HasSuffix(prefix, term) {
			return true
		}
	}
	return false
}

// sectionHeadingXML — заголовок подраздела: стиль заголовка макета.
//
// Макет ведёт оглавление `TOC \o "1-3"`, поэтому заголовки 1–3 получают стиль
// «heading 1» макета (styleId "1"), более глубокие — Normal с outlineLvl:
// один и тот же стиль на уровнях 2–4 превратил бы все заголовки в «главные»
// (живой дефект КП 003/2026, 2026-11-08).
func sectionHeadingXML(text string, level int, lc layoutContext) string {
	return lc.tocHeadingXML(strings.TrimSpace(text), level, paraOpts{NoIndent: true})
}

// sectionParagraphXML — абзац тела: стиль Normal макета, по ширине.
func sectionParagraphXML(text string) string {
	return paragraphXMLOpts(strings.TrimSpace(text), "both", paraOpts{})
}

// sectionListXML — список: маркер или номер даёт нумерация макета.
//
// Номер/маркер в текст не пишется: он принадлежит нумерации, иначе при правке
// документа в Word получилось бы «1. 1.».
func sectionListXML(items []string, ordered bool, lc layoutContext) string {
	if len(items) == 0 {
		return ""
	}
	numID := styleListBullet
	if ordered {
		numID = styleListNumbered
		// Свой numId на каждый список: иначе второй нумерованный список
		// продолжал бы первый счётчик (та же причина, что у состава, 2026-11-08).
		if plan := lc.numberingPlan(); plan != nil {
			numID = plan.newList()
		}
	}
	var sb strings.Builder
	for _, item := range items {
		sb.WriteString(listItemNumXML(strings.TrimSpace(item), numID, paraOpts{}))
	}
	return sb.String()
}

// sectionTableXML — таблица ответа: первая строка — шапка.
func sectionTableXML(block sectionBlock) string {
	rows := make([][]string, 0, len(block.Rows))
	for _, row := range block.Rows {
		if len(row) == 0 {
			continue
		}
		rows = append(rows, row)
	}
	return tableXML(rows)
}

// sectionFigureXML — рисунок: ссылка на картинку КП, сама картинка из данных.
//
// Модель называет систему и номер; байты берутся из карточки предложения — как
// у `{картинка: система 1}`. Модель картинок не видит и придумать не может.
func sectionFigureXML(block sectionBlock, lc layoutContext) string {
	of := strings.TrimSpace(block.Of)
	if of == "" {
		of = imageOfSystem
	}
	image := layoutImageMarkup(layoutImageSpec{of: of, index: block.Index}, lc)
	if image == "" {
		// Картинки нет — блок молчит: подпись под пустым местом была бы
		// вёрсткой без содержания.
		return ""
	}
	if caption := strings.TrimSpace(block.Caption); caption != "" {
		return image + paragraphXMLOpts(caption, "center", paraOpts{NoIndent: true})
	}
	return image
}

// renderSectionSubsection печатает подраздел: заголовок рамки и блоки ответа.
//
// Заголовок подраздела печатает РАМКА, а не модель: названия подразделов —
// настройка администратора, и модель не должна их придумывать. Заголовки в
// ответе модели — её собственные подзаголовки ВНУТРИ подраздела.
//
// nums — нумерация подразделов раздела (anonymous на раздел): подраздел без
// числа не печатается, а число — `<w:numPr>`, не символ в тексте.
func renderSectionSubsection(sub ContentSubsection, text string, lc layoutContext, nums sectionNumbering) string {
	answer, ok := parseSectionAnswer(text)
	if !ok {
		return ""
	}
	var sb strings.Builder
	if title := strings.TrimSpace(sub.Title); title != "" {
		var opts paraOpts
		if list := nums.subsectionList(); list != nil {
			opts.Numbering = list
			opts.NumberingLevel = 1
		}
		opts.NoIndent = true
		sb.WriteString(lc.tocHeadingXML(title, subsectionTitleLevel(sub), opts))
	}
	sb.WriteString(printSectionAnswer(answer, lc))
	return sb.String()
}

// subsectionTitleLevel — уровень заголовка подраздела: 1..3, по умолчанию 2.
func subsectionTitleLevel(sub ContentSubsection) int {
	if sub.Level >= 1 && sub.Level <= 3 {
		return sub.Level
	}
	return 2
}

// sectionMaxWords — предел длины текста подраздела с учётом брифа.
//
// Одна функция на оба пути (форма КП и боевой листок): правило «явный предел в
// инструкции важнее настройки» не должно зависеть от того, кто пишет бриф.
func sectionMaxWords(call sectionCall) int {
	maxWords := call.sub.MaxWords
	if maxWords <= 0 {
		maxWords = genDefaultMaxWords
	}
	if requested := skills.RequestedWords(call.sub.Instruction); requested > 0 {
		maxWords = requested
		if maxWords > 800 {
			maxWords = 800
		}
	}
	return maxWords
}

// sectionPromptText — промт одного подраздела: бриф администратора и данные.
//
// Тот же конвейер, что у блоков формы (skills.RenderBlockSystem +
// buildBlockUserPromptSelection): правила «пиши для заказчика» уже выведены там,
// и второй их набор разошёлся бы с первым. Меняется только ЗАДАНИЕ: модель
// обязана ответить JSON виду `{"blocks":[…]}`, а не текстом.
//
// Карточка системы — только у вызова повторяемого подраздела: остальные видят
// весь набор и не придумывают, что описывают.
func sectionPromptText(instruction string, call sectionCall, maxWords int, offer Offer,
	cards, allCards []SystemCard, refs []skills.CardRef, basePrompt string) (string, string) {
	if strings.TrimSpace(instruction) == "" {
		return "", ""
	}
	// Явный предел в брифе важнее настройки; `maxWords` оставлен аргументом
	// ради совместимости вызовов (его значение — умолчание `call.sub.MaxWords`).
	_ = maxWords
	maxWords = sectionMaxWords(call)
	if call.system != nil {
		cards = []SystemCard{*call.system}
	}
	system := skills.RenderBlockSystem(sectionBriefWithContract(instruction, basePrompt), "", maxWords)
	user := buildBlockUserPromptSelection(instruction, offer, cards, allCards, "", refs)
	return system, user
}

// sectionBriefWithContract — бриф администратора плюс БАЗОВЫЙ ПРОМТ формы.
//
// Базовый промт — общая часть на все подразделы (2026-11-17): контракт ответа,
// правила сторон и формулировок. Он правится в форме документа, а не в коде:
// живой листок 017/2026 показал, что требования к тексту (согласованность
// разделов, запрет называть продукты равноценными при разных значениях) меняются
// чаще, чем печать. Пусто — встроенный текст `sectionAnswerContract`.
func sectionBriefWithContract(instruction, basePrompt string) string {
	return strings.TrimSpace(instruction) + "\n\n" + effectiveBasePrompt(basePrompt)
}

// effectiveBasePrompt — базовый промт формы или встроенный текст.
func effectiveBasePrompt(basePrompt string) string {
	if trimmed := strings.TrimSpace(basePrompt); trimmed != "" {
		return trimmed
	}
	return sectionAnswerContract
}

// sectionAnswerContract — что именно должна вернуть модель.
//
// Про `diagram` (2026-11-17, шаг 5): схему модель просит рисовать ТОЛЬКО когда
// бриф прямо говорит «схема» или «диаграмма»; для сравнения характеристик —
// таблица. `lang` — только `mermaid`, SVG и прочие языки блок выбрасывает.
const sectionAnswerContract = `Ответь ТОЛЬКО JSON такого вида, без пояснений вокруг:
{"blocks":[
 {"kind":"heading","level":2,"text":"…"},
 {"kind":"paragraph","text":"…"},
 {"kind":"list","ordered":false,"items":["…","…"]},
 {"kind":"table","caption":"Таблица 1 — …","rows":[["шапка","шапка"],["…","…"]]},
 {"kind":"figure","of":"system","index":1,"caption":"Рисунок 1 — …"},
 {"kind":"diagram","lang":"mermaid","code":"flowchart TD; A-->B","caption":"Схема 1 — …"}
]}
Разрешены только эти шесть видов блоков. Первая строка таблицы — её шапка.
Рисунок — ссылка на картинку из КП (of: system или material, index — номер),
адреса и картинки не придумывай.
Схему (diagram) рисуй ТОЛЬКО если в брифе прямо сказано «схема» или «диаграмма»;
для сравнения характеристик предпочитай таблицу. В diagram поле lang — только
"mermaid", а code — код mermaid (flowchart, graph и т.п.), без Markdown.
Никакого Markdown и никакой разметки — только JSON.`

// sectionCacheKey — ключ кэша текста одного вызова.
//
// Три входа, по правилу «кэш ключуется всем, что увидела модель» (AGENTS.md):
//   - отпечаток данных предложения (`gen.section` — внешний слой свежести);
//   - хеш СОБСТВЕННОЙ инструкции подраздела: правка брифа одного подраздела не
//     выбрасывает тексты соседних;
//   - признак набора систем: текст без повтора видит весь набор, и после
//     добавления системы он обязан пересобраться (дефект вступления 2026-11-08).
func sectionCacheKey(sectionID string, call sectionCall, selection, basePrompt string) string {
	key := "gen.sect." + sectionID + "." + call.sub.ID
	if instruction := strings.TrimSpace(call.sub.Instruction); instruction != "" {
		key += ".ins" + hashShort(instruction)
	}
	if selection != "" {
		key += ".sel" + hashShort(selection)
	}
	if call.system != nil {
		key += ".sys" + systemCacheHandle(*call.system)
	}
	// Свой базовый промт (2026-11-17) входит в ключ: правка текста в форме
	// обязана переписать раздел. Встроенный текст не входит — иначе первая же
	// выкладка выбросила бы кэш всех документов ради ничего.
	if strings.TrimSpace(basePrompt) != "" && strings.TrimSpace(basePrompt) != sectionAnswerContract {
		key += ".bp" + hashShort(basePrompt)
	}
	return key
}

// sectionGen — генерация текстов разделов: один вызов на подраздел, из кэша
// или моделью, с сохранением в form_values предложения.
type sectionGen struct {
	values    map[string]string
	previous  map[string]string
	section   string
	selection string
	offer     Offer
	cards     []SystemCard
	refs      []skills.CardRef
	complete  Completer
	report    *buildReport
	// brief — подмена брифа подраздела (2026-11-16). nil — бриф из формы
	// (`sectionBriefWithContract`); боевой листок подставляет свой, потому что
	// у него ДВЕ стороны сравнения и контракт ответа обязан назвать роли.
	// Правило и разбор ответа при этом те же: вторая копия разошлась бы.
	brief func(instruction string, call sectionCall) string
	// userPrompt — подмена USER-промта (2026-11-19). nil — данные предложения
	// (`buildBlockUserPromptSelection`). Боевой листок подставляет свои стороны:
	// данные КП («ДАННЫЕ ПРЕДЛОЖЕНИЯ», «ВЫБРАННЫЕ СИСТЕМЫ В ПРЕДЛОЖЕНИИ» и
	// правило комплекта «каждая на свой участок объекта») описывают не то, что
	// видит читатель листка, — модель честно повторяла это в тексте.
	userPrompt func() string
	// basePrompt — базовый промт формы (2026-11-17): общая часть на все
	// подразделы. Пусто — встроенный `sectionAnswerContract`.
	basePrompt string
	// mu — запись в values: вызовы идут параллельно (sections.go), и обычная
	// запись в карту из двух горутин роняет процесс («concurrent map writes»).
	mu sync.Mutex
}

// text отдаёт текст одного вызова: кэш, если свежий, иначе модель.
func (g *sectionGen) text(section ContentSection, call sectionCall) string {
	key := sectionCacheKey(section.ID, call, g.selection, g.basePrompt)
	if v, ok := g.cached(key); ok {
		return v
	}
	if g.previous != nil && g.previous[genKeySection] == g.section {
		if v := strings.TrimSpace(g.previous[key]); v != "" {
			g.set(key, v)
			if g.report != nil {
				g.report.addFromCache()
			}
			return v
		}
	}
	if g.complete == nil {
		return ""
	}
	started := time.Now()
	system, user := sectionPromptText(call.sub.Instruction, call, call.sub.MaxWords,
		g.offer, g.cards, g.cards, g.refs, g.basePrompt)
	if g.brief != nil {
		system = skills.RenderBlockSystem(g.brief(call.sub.Instruction, call), "", sectionMaxWords(call))
	}
	if g.userPrompt != nil {
		user = g.userPrompt()
	}
	raw, err := g.complete.Complete(context.Background(), system, user, documentTextMaxTokens())
	logModelCall("sect:"+key, "", len(call.sub.Instruction), started, err)
	if err != nil || strings.TrimSpace(raw) == "" {
		return ""
	}
	answer, ok := parseSectionAnswer(raw)
	if !ok {
		return ""
	}
	// В кэш — нормализованный ответ, а не сырой текст модели: следующий
	// читатель не должен разбираться с ограждением и пояснениями.
	encoded, err := json.Marshal(answer)
	if err != nil {
		return ""
	}
	g.set(key, string(encoded))
	return string(encoded)
}

// cached — текст из карты под замком (вызовы параллельны).
func (g *sectionGen) cached(key string) (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	v, ok := g.values[key]
	return v, ok
}

// set — запись текста под ключ под замком (вызовы параллельны).
func (g *sectionGen) set(key, value string) {
	g.mu.Lock()
	g.values[key] = value
	g.mu.Unlock()
}

// sectionValuesFor — стартовые значения генератора: копия form_values.
//
// Копия, а не карта предложения: веер вызовов пишется в свою карту под
// уникальными ключами, и общая карта с предложением потребовала бы ещё одного
// правила синхронизации. Слияние с form_values — одна точка, в botapi.go.
func sectionValuesFor(offer Offer) map[string]string {
	out := make(map[string]string, len(offer.FormValues)+1)
	for k, v := range offer.FormValues {
		out[k] = v
	}
	return out
}

// mergeSectionValues дописывает новые тексты подразделов в карту кэша.
//
// Только изменившееся: значение, равное сохранённому, не считается новым, иначе
// каждая печать файла писала бы form_values без причины.
func mergeSectionValues(into, previous, generated map[string]string) {
	for key, value := range generated {
		if previous != nil && previous[key] == value {
			continue
		}
		into[key] = value
	}
}
