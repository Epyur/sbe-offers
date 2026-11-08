package main

import (
	"fmt"
	"strings"
)

// Перезапуск нумерации состава для каждой системы (2026-11-08, живая жалоба
// на КП 003/2026).
//
// Симптом: во второй системе слои получали номера 13, 14, 15… вместо 1, 2, 3.
// Причина в OOXML: у всех пунктов стоял ОДИН numId ("1"), а Word считает
// нумерацию по numId сквозной — один список на весь документ. Без отдельного
// numId на каждый состав второй список просто продолжает первый.
//
// Как чиним: для каждого состава заводим свой numId, ссылающийся на ту же
// десятичную нумерацию бланка, но с `<w:startOverride w:val="1"/>`. Это и есть
// «начать этот список заново с единицы» в терминах Word.
//
// Почему не «взять numId 2, 3, … из бланка»: у бланка их конечное число и разный
// вид, а систем в КП может быть сколько угодно. Поэтому определения создаются на
// время одной сборки и дописываются к numbering.xml макета — файл НЕ
// заменяется, только получает новые `<w:num>` (правило 2026-10-29: подмена
// styles.xml/numbering.xml целиком ломает страницы и колонтитулы).

// numberingUserBase — с какого numId начинаем выдавать наши списки.
//
// В бланке и в живом макете заняты numId 1..12. Отрыв от 12 оставляет место,
// если в макет добавят нумерации; 9000 не может совпасть с осмысленными 1..99.
const numberingUserBase = 9000

// numberingDef — определение одного нашего numId в numbering.xml.
type numberingDef struct {
	numID      int
	abstractID string
}

// numberingPlan — нумерации, заведённые на время одной сборки документа.
//
// Создаётся пустой, выдаёт numId по требованию, а в конце сборки отдаёт свои
// определения для дописывания в numbering.xml. nil-план — «плана нет»: тогда
// состав печатается одним сквозным списком, как раньше.
type numberingPlan struct {
	next       int
	abstractID string
	defs       []numberingDef
}

// newNumberingPlan — план, ссылающийся на десятичную нумерацию бланка.
//
// abstractID — какое оформление списка брать (в живом бланке и макете это
// abstractNumId "0", на который ссылается numId "1": десятичный список).
func newNumberingPlan(abstractID string) *numberingPlan {
	if strings.TrimSpace(abstractID) == "" {
		abstractID = numberingAbstractDecimal
	}
	return &numberingPlan{next: numberingUserBase, abstractID: abstractID}
}

// numberingAbstractDecimal — оформление десятичного списка бланка (abstractNum 0).
const numberingAbstractDecimal = "0"

// newList выдаёт numId нового списка, который Word начнёт с единицы.
//
// Каждый вызов — отдельный список: именно разные numId и дают перезапуск.
// Один numId на два состава вернул бы ровно ту ошибку, из-за которой этот файл
// и появился.
func (p *numberingPlan) newList() string {
	if p == nil {
		return styleListNumbered
	}
	id := p.next
	p.next++
	p.defs = append(p.defs, numberingDef{numID: id, abstractID: p.abstractID})
	return itoa(id)
}

// numberingXMLInject дописывает определения наших списков в numbering.xml.
//
// Не заменяет файл: только вставляет `<w:num>` перед закрывающим тегом. Если
// планировщик пуст или файла нет — ничего не меняет и возвращает false, чтобы
// вызывающий знал, что правки не было (например, тестам это не нужно).
func injectNumberingDefinitions(files map[string][]byte, plan *numberingPlan) bool {
	if plan == nil || len(plan.defs) == 0 {
		return false
	}
	raw, ok := files["word/numbering.xml"]
	if !ok {
		return false
	}
	s := string(raw)
	const closeTag = "</w:numbering>"
	idx := strings.LastIndex(s, closeTag)
	if idx < 0 {
		return false
	}
	insert := numberingDefinitionsXML(plan.defs)
	files["word/numbering.xml"] = []byte(s[:idx] + insert + s[idx:])
	return true
}

// numberingDefinitionsXML — `<w:num>` на каждый созданный список.
//
// `<w:startOverride w:val="1"/>` и есть перезапуск: без него новый numId,
// ссылающийся на ту же abstractNum, продолжил бы общий счётчик, потому что
// abstractNum хранит счётчик сам.
func numberingDefinitionsXML(defs []numberingDef) string {
	var sb strings.Builder
	for _, d := range defs {
		fmt.Fprintf(&sb, `<w:num w:numId="%d"><w:abstractNumId w:val="%s"/>`,
			d.numID, escapeXML(d.abstractID))
		sb.WriteString(`<w:lvlOverride w:ilvl="0"><w:startOverride w:val="1"/></w:lvlOverride>`)
		sb.WriteString(`</w:num>`)
	}
	return sb.String()
}
