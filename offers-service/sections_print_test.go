package main

import (
	"strings"
	"testing"
)

// sectionParagraphAnswer — ответ модели с одним абзацем.
func sectionParagraphAnswer(text string) string {
	return `{"blocks":[{"kind":"paragraph","text":` + quoteJSON(text) + `}]}`
}

// quoteJSON — строка в JSON-кавычках (без лишних зависимостей в тесте).
func quoteJSON(text string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(text) + `"`
}

// sectionMaster — макет настоящей формы: стиль «heading 1», нумерация
// многоуровневого списка и TOC.
//
// Тесты ниже печатают в него, а не в минимальный wordDoc: именно макет
// решает, какие styleId существуют, и «заголовок печатается стилем макета»
// проверить на пустом документе нельзя.
func sectionMaster(t *testing.T) []byte {
	t.Helper()
	return layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>{ content: proposal }</w:t></w:r></w:p>`))
}

// sectionFrameContext — контекст печати раздела с ответами модели.
func sectionFrameContext(section ContentSection, answers map[string]string) layoutContext {
	lc := serviceLayoutContext(nil)
	lc.form = func() OfferForm { return OfferForm{Sections: []ContentSection{section}} }
	lc.sectionText = func(_ ContentSection, call sectionCall) string {
		return answers[call.sub.ID]
	}
	lc.report = newBuildReport("003/2026")
	return lc
}

// TestSectionConditionSkipsEmptySubsection — пропущенный подраздел не печатает
// даже своего заголовка.
//
// Проверяется ИМЕННО отсутствие строки заголовка: пустой абзац под заголовком —
// ровно тот дефект, из-за которого разделы появились (§1 дизайна).
func TestSectionConditionSkipsEmptySubsection(t *testing.T) {
	section := ContentSection{
		ID: "proposal", Title: "Описание предлагаемых систем",
		Subsections: []ContentSubsection{
			{ID: "intro", Title: "Введение", Instruction: "Вступление", Condition: conditionAlways},
			{ID: "rebut", Title: "Критика альтернатив", Instruction: "Опровергни", Condition: conditionHasCritique},
		},
	}
	// critique выключен: подраздел критики пропускается (и модель для него не
	// зовётся — ответа в карте нет вовсе).
	lc := sectionFrameContext(section, map[string]string{"intro": sectionParagraphAnswer("Вступление по делу.")})
	out, err := renderLayoutDocx(sectionMaster(t), lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if strings.Contains(body, "Критика альтернатив") {
		t.Fatalf("заголовок пропущенного подраздела напечатан:\n%s", body)
	}
	if !strings.Contains(body, "Вступление по делу.") {
		t.Fatalf("текст уцелевшего подраздела потерян:\n%s", body)
	}
}

// TestSectionTitleAbsentWhenAllSkipped — раздел, у которого всё пропущено, не
// печатает ничего, включая свой заголовок.
func TestSectionTitleAbsentWhenAllSkipped(t *testing.T) {
	section := ContentSection{
		ID: "proposal", Title: "Описание предлагаемых систем",
		Subsections: []ContentSubsection{
			{ID: "rebut", Title: "Критика альтернатив", Instruction: "Опровергни", Condition: conditionHasCritique},
			{ID: "why", Title: "Почему эти", Instruction: "Обоснуй", Condition: conditionMultiSystem},
		},
	}
	lc := sectionFrameContext(section, nil)
	lc.offer.Systems = RefList{{NoteID: 1}} // система одна: multi_system ложь
	out, err := renderLayoutDocx(sectionMaster(t), lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if strings.Contains(body, "Описание предлагаемых систем") || strings.Contains(body, "Критика") {
		t.Fatalf("раздел без подразделов напечатал заголовок:\n%s", body)
	}
}

// TestSectionOneSystemVsThree — та же настройка, разное число систем:
// подраздел с repeat:"systems" даёт текст на каждую систему, без правки формы.
func TestSectionOneSystemVsThree(t *testing.T) {
	section := ContentSection{
		ID: "proposal", Title: "Описание систем",
		Subsections: []ContentSubsection{
			{ID: "each", Title: "Описание системы", Instruction: "Опиши", Condition: conditionHasSystems, Repeat: blockRepeatSystems},
			{ID: "why", Title: "Почему эти", Instruction: "Обоснуй", Condition: conditionMultiSystem},
		},
	}
	cards := []SystemCard{{NoteID: 1, Title: "Первая"}, {NoteID: 2, Title: "Вторая"}, {NoteID: 3, Title: "Третья"}}
	three := Offer{Systems: RefList{{NoteID: 1}, {NoteID: 2}, {NoteID: 3}}}
	one := Offer{Systems: RefList{{NoteID: 1}}}

	// Три системы: три текста «по системе» и критическое обоснование выбора.
	lc := serviceLayoutContext(nil)
	lc.form = func() OfferForm { return OfferForm{Sections: []ContentSection{section}} }
	lc.offer = three
	lc.cards = func() []SystemCard { return cards }
	lc.sectionText = func(_ ContentSection, call sectionCall) string {
		if call.system != nil {
			return sectionParagraphAnswer("Система " + call.system.Title)
		}
		return sectionParagraphAnswer("Выбор из трёх.")
	}
	out, err := renderLayoutDocx(sectionMaster(t), lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	for _, want := range []string{"Система Первая", "Система Вторая", "Система Третья", "Выбор из трёх."} {
		if !strings.Contains(body, want) {
			t.Fatalf("нет %q при трёх системах:\n%s", want, body)
		}
	}

	// Одна система: «Почему эти» пропускается, остаётся описание одной.
	lcOne := serviceLayoutContext(nil)
	lcOne.form = func() OfferForm { return OfferForm{Sections: []ContentSection{section}} }
	lcOne.offer = one
	lcOne.cards = func() []SystemCard { return cards }
	lcOne.sectionText = func(_ ContentSection, call sectionCall) string {
		if call.system != nil {
			return sectionParagraphAnswer("Система " + call.system.Title)
		}
		return sectionParagraphAnswer("Выбор из трёх.")
	}
	outOne, err := renderLayoutDocx(sectionMaster(t), lcOne)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	bodyOne := string(docxEntry(t, outOne, "word/document.xml"))
	if !strings.Contains(bodyOne, "Система Первая") || strings.Contains(bodyOne, "Система Вторая") {
		t.Fatalf("при одной системе напечатано не то:\n%s", bodyOne)
	}
	if strings.Contains(bodyOne, "Почему эти") {
		t.Fatalf("подраздел для нескольких систем напечатан при одной:\n%s", bodyOne)
	}
}

// TestSectionNumberIsNumPrNotText — номер заголовка живёт в `<w:numPr>`, а не
// в тексте.
func TestSectionNumberIsNumPrNotText(t *testing.T) {
	section := ContentSection{
		ID: "proposal", Title: "Описание систем", Numbered: true,
		Subsections: []ContentSubsection{
			{ID: "intro", Title: "Введение", Instruction: "Вступление", Condition: conditionAlways},
		},
	}
	lc := sectionFrameContext(section, map[string]string{"intro": sectionParagraphAnswer("Текст.")})
	out, err := renderLayoutDocx(sectionMaster(t), lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	// Заголовок раздела — абзац с numPr и нашим numId.
	sectionHeading := paragraphTextOf(body, "Описание систем")
	if !strings.Contains(sectionHeading, "<w:numPr>") {
		t.Fatalf("у заголовка раздела нет нумерации:\n%s", sectionHeading)
	}
	if !strings.Contains(sectionHeading, `numId w:val="`+itoa(numberingUserBase)) {
		t.Fatalf("номер не из плана нумерации:\n%s", sectionHeading)
	}
	// В тексте номера нет: ни «1.», ни «1.1».
	text := plainParagraphText(sectionHeading)
	if strings.ContainsAny(text, "0123456789") {
		t.Fatalf("номер попал в текст заголовка: %q", text)
	}
	// Нумерация дописана в numbering.xml макета.
	numbering := string(docxEntry(t, out, "word/numbering.xml"))
	if !strings.Contains(numbering, `w:numId="`+itoa(numberingUserBase)+`"`) {
		t.Fatalf("в numbering.xml нет определения списка раздела")
	}
}

// TestSectionTablePrintsAsMasterTable — таблица ответа становится настоящим
// `<w:tbl>`, первая строка — шапка, и это проверяется В ГРАНИЦАХ таблицы, а не
// по всему документу.
func TestSectionTablePrintsAsMasterTable(t *testing.T) {
	section := ContentSection{
		ID: "proposal", Title: "Сравнение",
		Subsections: []ContentSubsection{{ID: "table", Title: "Таблица", Instruction: "Составь", Condition: conditionAlways}},
	}
	answer := `{"blocks":[{"kind":"table","caption":"Таблица 1","rows":[["Показатель","Значение"],["Срок","50 лет"]]}]}`
	lc := sectionFrameContext(section, map[string]string{"table": answer})
	out, err := renderLayoutDocx(sectionMaster(t), lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	table := firstTableXML(body)
	if table == "" {
		t.Fatalf("таблица не напечатана:\n%s", body)
	}
	if !strings.Contains(table, "Показатель") || !strings.Contains(table, "50 лет") {
		t.Fatalf("содержимое таблицы потеряно:\n%s", table)
	}
	// Шапка — первая строка таблицы: ячейка с заливкой.
	firstRow := firstTableRowXML(table)
	if !strings.Contains(firstRow, "DCE6F1") {
		t.Fatalf("первая строка не шапка таблицы:\n%s", firstRow)
	}
}

// TestSectionFigureIsReference — рисунок печатается картинкой карточки, а в
// промт модели байты картинки не попадают.
func TestSectionFigureIsReference(t *testing.T) {
	section := ContentSection{
		ID: "proposal", Title: "Разрез",
		Subsections: []ContentSubsection{{ID: "fig", Title: "Рисунок", Instruction: "Добавь рисунок", Condition: conditionAlways}},
	}
	answer := `{"blocks":[{"kind":"figure","of":"system","index":1,"caption":"Рисунок 1 — Разрез"}]}`
	lc := sectionFrameContext(section, map[string]string{"fig": answer})
	out, err := renderLayoutDocx(sectionMaster(t), lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	if !strings.Contains(body, "<w:drawing>") {
		t.Fatalf("картинка карточки не напечатана:\n%s", body)
	}
	if !strings.Contains(body, "Рисунок 1 — Разрез") {
		t.Fatalf("подпись рисунка потеряна:\n%s", body)
	}
	// В промте — только инструкция и контракт: ни base64, ни байтов картинки.
	system, _ := sectionPromptText("Добавь рисунок", sectionCall{sub: section.Subsections[0]}, 0,
		lc.offer, lc.cards(), lc.cards(), nil)
	if strings.Contains(system, "base64") || strings.Contains(system, "iVBOR") {
		t.Fatalf("в промт модели попали байты картинки")
	}
}

// TestSectionCacheKeyPerSubsection — правка инструкции одного подраздела
// выбрасывает кэш ТОЛЬКО его.
func TestSectionCacheKeyPerSubsection(t *testing.T) {
	intro := sectionCall{sub: ContentSubsection{ID: "intro", Instruction: "Вступление"}}
	rebut := sectionCall{sub: ContentSubsection{ID: "rebut", Instruction: "Опровергни"}}
	before := sectionCacheKey("proposal", intro, "1")
	// Соседний подраздел изменил инструкцию (обе инструкции сменились): ключ
	// вступления обязан остаться прежним.
	introAgain := sectionCall{sub: ContentSubsection{ID: "intro", Instruction: "Вступление"}}
	rebutChanged := sectionCall{sub: ContentSubsection{ID: "rebut", Instruction: "Опровергни подробнее"}}
	if after := sectionCacheKey("proposal", introAgain, "1"); after != before {
		t.Fatalf("правка соседнего подраздела сбросила вступление: %s → %s", before, after)
	}
	if sectionCacheKey("proposal", rebut, "1") == sectionCacheKey("proposal", rebutChanged, "1") {
		t.Fatal("правка собственной инструкции не сбросила кэш подраздела")
	}
	// Набор систем входит в ключ: добавили систему — текст пересобирается.
	if sectionCacheKey("proposal", intro, "1") == sectionCacheKey("proposal", intro, "1+2") {
		t.Fatal("смена набора систем не сбросила кэш")
	}
}

// TestSectionUnknownBlockKindIsDropped — незнакомый вид блока не печатается
// разметкой.
func TestSectionUnknownBlockKindIsDropped(t *testing.T) {
	answer, ok := parseSectionAnswer(`{"blocks":[{"kind":"chart","text":"x"},{"kind":"paragraph","text":"Живой текст."}]}`)
	if !ok || len(answer.Blocks) != 1 || answer.Blocks[0].Kind != "paragraph" {
		t.Fatalf("разбор: %+v", answer)
	}
	lc := serviceLayoutContext(nil)
	markup := printSectionAnswer(answer, lc)
	if strings.Contains(markup, "chart") || !strings.Contains(markup, "Живой текст.") {
		t.Fatalf("разметка незнакомого блока попала в документ:\n%s", markup)
	}
}

// TestSectionAnswerSurvivesFence — ответ в ограждении ```json разбирается.
func TestSectionAnswerSurvivesFence(t *testing.T) {
	raw := "Вот результат:\n```json\n{\"blocks\":[{\"kind\":\"paragraph\",\"text\":\"Текст.\"}],\n```\n"
	answer, ok := parseSectionAnswer(raw)
	if !ok || len(answer.Blocks) != 1 {
		t.Fatalf("ответ в ограждении не разобран: %+v %v", answer, ok)
	}
}

// paragraphTextOf — XML первого абзаца, содержащего строку.
func paragraphTextOf(body, needle string) string {
	paragraphs := reWordParagraph.FindAllString(body, -1)
	for _, p := range paragraphs {
		if strings.Contains(plainParagraphText(p), needle) {
			return p
		}
	}
	return ""
}

// plainParagraphText — видимый текст абзаца.
func plainParagraphText(paragraph string) string {
	return strings.Join(paragraphTexts(paragraph), "")
}

// firstTableXML — первая таблица документа целиком.
func firstTableXML(body string) string {
	start := strings.Index(body, "<w:tbl>")
	if start < 0 {
		return ""
	}
	end := strings.Index(body[start:], "</w:tbl>")
	if end < 0 {
		return ""
	}
	return body[start : start+end+len("</w:tbl>")]
}

// firstTableRowXML — первая строка таблицы.
func firstTableRowXML(table string) string {
	start := strings.Index(table, "<w:tr>")
	if start < 0 {
		return ""
	}
	end := strings.Index(table[start:], "</w:tr>")
	if end < 0 {
		return ""
	}
	return table[start : start+end+len("</w:tr>")]
}
