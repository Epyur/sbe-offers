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
	// level — на каком уровне списка считается нумерация (0 — обычный список;
	// 1 — подраздел многоуровневого списка, «%1.%2.»).
	level int
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

// newListAt — numId списка, который считается на заданном уровне.
func (p *numberingPlan) newListAt(level int) string {
	if p == nil {
		return styleListNumbered
	}
	id := p.next
	p.next++
	p.defs = append(p.defs, numberingDef{numID: id, abstractID: p.abstractID, level: level})
	return itoa(id)
}

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

// sectionNumbering — списки нумерации заголовков содержательного раздела.
//
// Раздел получает свой список уровня 0 (номер «1.»), подразделы — отдельный
// список уровня 1 («%1.%2.»): у многоуровневого списка макета уровень 1 живёт
// в счётчике уровня 0, а порядок вызовов задаёт ссылки — подраздел ссылается на
// список, созданный после списка своего раздела.
//
// nil-план (макет без numbering.xml) даёт nil: печатать номер некуда, и
// заголовок идёт без него, а не с выдуманным «1.1».
type sectionNumbering struct {
	plan *numberingPlan
	// list — список подразделов (уровень 1), заведённый при первой просьбе.
	list *numberingList
}

// numberingList — numId, который печатает параграф (paraOpts.Numbering).
type numberingList struct {
	numID string
}

// section — список раздела (уровень 0); nil при плане без numbering.xml.
func (n sectionNumbering) section() *numberingList {
	if n.plan == nil {
		return nil
	}
	return &numberingList{numID: n.plan.newListAt(0)}
}

// subsection — список подразделов (уровень 1); nil при плане без numbering.xml.
func (n sectionNumbering) subsection() *numberingList {
	if n.plan == nil {
		return nil
	}
	return &numberingList{numID: n.plan.newListAt(1)}
}

// listsFor заводит списки заголовков ОДНОГО раздела: сначала раздел, затем
// подразделы.
//
// Порядок существенен: у многоуровневого списка подраздел ссылается на уровень
// 0, заведённый ДО него (список раздела), поэтому заголовок раздела и его
// подразделы ссылаются на разные numId одного abstractNum. Numbered выключен —
// ни одна строка не получает номера (списки даже не заводятся).
func (n sectionNumbering) listsFor(numbered bool) (*numberingList, sectionNumbering) {
	if !numbered {
		return nil, sectionNumbering{}
	}
	title := n.section()
	if title == nil {
		return nil, sectionNumbering{}
	}
	subs := sectionNumbering{plan: n.plan}
	// Список подразделов заводится сейчас, а не при печати: он должен стоять в
	// плане ПОСЛЕ списка раздела, иначе порядок numId зависел бы от того, есть
	// ли у раздела подразделы.
	subs.list = &numberingList{numID: n.plan.newListAt(1)}
	return title, subs
}

// subsectionList — список подразделов, заведённый для их заголовков.
func (n sectionNumbering) subsectionList() *numberingList {
	return n.list
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
		sb.WriteString(`<w:lvlOverride w:ilvl="` + itoa(d.level) + `"><w:startOverride w:val="1"/></w:lvlOverride>`)
		sb.WriteString(`</w:num>`)
	}
	return sb.String()
}
