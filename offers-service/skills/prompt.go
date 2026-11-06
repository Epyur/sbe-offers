package skills

import (
	"fmt"
	"strings"

	"mailers/offers/model"
)

// Сборка системных промптов. Правила живут текстом в других файлах пакета;
// здесь только подстановка данных предложения и брифа.

// RenderBriefSystem собирает системный промпт разговора. Меняется от вызова к
// вызову ровно тем, что уже известно и какая тема сейчас открыта.
func RenderBriefSystem(brief model.BriefPayload, ctx model.ProductContext) string {
	var b strings.Builder
	b.WriteString(briefRules)
	b.WriteString("\n")
	b.WriteString(noInventDoc)
	b.WriteString("\n")

	b.WriteString("ТЕМЫ РАЗГОВОРА (по порядку):\n")
	for _, t := range BriefTopics() {
		mark := "[ ]"
		if IsTopicFilled(brief, t.Key) {
			mark = "[x]"
		}
		b.WriteString(fmt.Sprintf("%s %s (%s) — %s\n", mark, t.Label, t.Key, t.Question))
	}
	b.WriteString("\n")
	b.WriteString("УЖЕ ИЗВЕСТНО:\n")
	if s := renderBriefKnown(brief); s != "" {
		b.WriteString(s)
	} else {
		b.WriteString("пока ничего\n")
	}
	b.WriteString("\n")

	if next, ok := NextOpenTopic(brief); ok {
		b.WriteString("СЕЙЧАС СПРАШИВАЙ ПРО: " + next.Label + "\n")
	} else {
		b.WriteString("ВСЕ ТЕМЫ ЗАКРЫТЫ. Подведи итог в двух-трёх строках и предложи перейти к расчёту.\n")
	}
	b.WriteString("\n")
	b.WriteString(renderContextBlock(ctx))
	return b.String()
}

// renderBriefKnown — поля брифа в читаемом виде. Пустые не выводятся: модель
// не должна видеть «object_name: ""» и заполнять догадкой.
func renderBriefKnown(p model.BriefPayload) string {
	var b strings.Builder
	line := func(label, value string) {
		if model.NonEmpty(value) {
			b.WriteString("- " + label + ": " + value + "\n")
		}
	}
	list := func(label string, values []string) {
		if len(values) > 0 {
			b.WriteString("- " + label + ": " + strings.Join(values, "; ") + "\n")
		}
	}
	line("Объект", p.ObjectName)
	line("Назначение объекта", p.ObjectPurpose)
	line("Площадь", p.ObjectArea)
	line("Адресат КП", p.CustomerAddress)
	list("Задача заказчика", p.CustomerGoals)
	list("Боли", p.CustomerPains)
	list("Риски проекта", p.ProjectRisks)
	list("Планируемые системы", p.PlannedSystems)
	for _, o := range p.Objections {
		if model.NonEmpty(o.Claim) || model.NonEmpty(o.Response) {
			b.WriteString("- Возражение: " + o.Claim + " → ответ: " + o.Response + "\n")
		}
	}
	line("Бюджет", p.BudgetSignal)
	line("Кто решает", p.DecisionMaker)
	line("Сроки", p.Timeline)
	list("Почему мы", p.WhyUs)
	line("Прочее", p.Notes)
	return b.String()
}

// RenderCopySystem собирает промпт для написания одной секции.
func RenderCopySystem(kind SectionKind, brief model.BriefPayload, offer model.Offer, ctx model.ProductContext) string {
	var b strings.Builder
	b.WriteString("ТЫ ПИШЕШЬ ОДНУ СЕКЦИЮ КОММЕРЧЕСКОГО ПРЕДЛОЖЕНИЯ.\n")
	b.WriteString("Секция: " + string(kind) + "\n")
	b.WriteString(taskFor(kind) + "\n\n")

	b.WriteString(salesDoc)
	b.WriteString("\n")
	b.WriteString(copyDoc)
	b.WriteString("\n")
	b.WriteString(noInventDoc)
	b.WriteString("\n")

	b.WriteString("ДАННЫЕ ПРЕДЛОЖЕНИЯ (числа брать отсюда дословно):\n")
	b.WriteString(renderOfferFacts(offer))
	b.WriteString("\n")

	b.WriteString("БРИФ:\n")
	if s := renderBriefKnown(brief); s != "" {
		b.WriteString(s)
	} else {
		b.WriteString("брифа нет — пиши только по данным предложения\n")
	}
	b.WriteString("\n")
	b.WriteString(renderContextBlock(ctx))
	b.WriteString("\nФОРМАТ ОТВЕТА: только текст секции. Без заголовка, без пояснений, ")
	b.WriteString("без маркеров кода. Нехватка факта — [НУЖНО: что именно].\n")
	return b.String()
}

// RenderEditSystem собирает промпт редактуры уже написанного менеджером текста.
func RenderEditSystem(kind SectionKind, ctx model.ProductContext) string {
	var b strings.Builder
	b.WriteString("ТЫ РЕДАКТИРУЕШЬ УЖЕ НАПИСАННЫЙ ТЕКСТ СЕКЦИИ КОММЕРЧЕСКОГО ПРЕДЛОЖЕНИЯ.\n")
	b.WriteString("Секция: " + string(kind) + "\n")
	b.WriteString("Менеджер правил текст сам — это его аргумент и его слова.\n\n")
	b.WriteString(editDoc)
	b.WriteString("\n")
	b.WriteString(noInventDoc)
	b.WriteString("\n")
	b.WriteString(renderContextBlock(ctx))
	b.WriteString("\nФОРМАТ ОТВЕТА: только исправленный текст. Без пояснений и списка правок.\n")
	return b.String()
}

// RenderBlockSystem собирает промпт для свободного блока формы, который
// администратор описал подсказкой (2026-11-01).
//
// instruction — его слова: «опиши систему и материалы, входящие в неё». hint —
// необязательное уточнение. Правила те же, что у секций (те же скилы), но
// задача другая: секции согласуются с менеджером в чате, а блок печатается в
// документе без согласования — поэтому границы «не выдумывать» здесь жёстче.
//
// Отдельный раздел «ЧЕГО НЕ ДЕЛАТЬ» появился 2026-11-03 по живому КП 003/2026:
// модель ответила не текстом, а размышлением вслух («The task: …», «Let me
// think…», «hmm, is not in the brief») и по-английски, и это ушло в документ
// клиенту. Ответ печатается как есть, человека между моделью и файлом нет,
// поэтому запрет должен стоять в промпте, а не в чистке постфактум.
func RenderBlockSystem(instruction, hint string, maxWords int) string {
	var b strings.Builder
	b.WriteString("ТЫ ПИШЕШЬ ОДИН БЛОК КОММЕРЧЕСКОГО ПРЕДЛОЖЕНИЯ ПО ПОДСКАЗКЕ АДМИНИСТРАТОРА.\n")
	b.WriteString("Подсказка: " + strings.TrimSpace(instruction) + "\n")
	if h := strings.TrimSpace(hint); h != "" {
		b.WriteString("Уточнение: " + h + "\n")
	}
	b.WriteString("\n")
	b.WriteString(salesDoc)
	b.WriteString("\n")
	b.WriteString(copyDoc)
	b.WriteString("\n")
	b.WriteString(noInventDoc)
	b.WriteString("\n")
	b.WriteString("Пиши по-русски: текст читает заказчик, а не администратор.\n")
	b.WriteString(fmt.Sprintf("Объём: не больше %d слов, если подсказка не просит иного. ", maxWords))
	b.WriteString("Это абзацы печатного документа, не список с маркерами — если без списка не обойтись, ")
	b.WriteString("не больше трёх пунктов по одной строке.\n")
	b.WriteString("Адресов сайтов и ссылок в тексте не пиши: они печатаются отдельным разделом в конце.\n")
	b.WriteString("\nЧЕГО НЕ ДЕЛАТЬ.\n")
	b.WriteString("1. Не пересказывай задание и не показывай, как ты думаешь. Строки «The task», ")
	b.WriteString("«Let me think», «I should», «hmm», перечисление того, что известно и чего нет, ")
	b.WriteString("черновик с последующей правкой — это черновик, а не блок. В ответе только ")
	b.WriteString("готовый текст для клиента. Ничего по-английски: ни слов, ни фраз.\n")
	b.WriteString("2. В ответе нет ни одного предложения о самом тексте: «ниже приведено», ")
	b.WriteString("«в этом разделе», «надеюсь, это подходит», «слов: 148».\n")
	b.WriteString("3. Не хватает факта — короткая пометка ровно в той фразе, где он нужен, ")
	b.WriteString("и больше ничего. Не превращай нехватку в рассуждение о том, чего писать не стоит.\n")
	// Просьба «помечай заголовки» читается моделью как Markdown — живой ответ
	// 003/2026 (2026-11-06) принёс в документ `## …`. Разметку в ответе
	// просить нельзя: документ печатается как есть.
	b.WriteString("\nЗАГОЛОВКИ И РАЗМЕТКА.\n")
	b.WriteString("Если подсказка просит разделить текст на части, пиши каждую часть отдельной ")
	b.WriteString("строкой обычным текстом — без звёздочек, решёток и другой разметки: «##», ")
	b.WriteString("«**», «- » в документе напечатаются как есть и увидят их заказчик. Пустая ")
	b.WriteString("строка между частями допустима.\n")
	b.WriteString("\nФОРМАТ ОТВЕТА: только текст блока. Без заголовка, без пояснений, ")
	b.WriteString("без маркеров кода. Нехватка факта — [НУЖНО: что именно].\n")
	return b.String()
}

// taskFor — что надо написать в секции.
func taskFor(kind SectionKind) string {
	if t, ok := sectionBrief[kind]; ok {
		return t
	}
	return "Напиши секцию по данным предложения."
}

// renderOfferFacts — числа предложения в ТОМ ЖЕ формате, в котором их напечатает
// документ. Машинный формат (20158633.08) модель перепишет по-своему, и в КП
// появится расхождение с таблицей.
func renderOfferFacts(o model.Offer) string {
	var b strings.Builder
	if model.NonEmpty(o.Number) {
		b.WriteString("- Номер предложения: " + o.Number + "\n")
	}
	if model.NonEmpty(o.Client.Name) {
		b.WriteString("- Заказчик: " + o.Client.Name + "\n")
	}
	if model.NonEmpty(o.Client.Contact) {
		b.WriteString("- Контактное лицо: " + o.Client.Contact + "\n")
	}
	if o.Total > 0 {
		b.WriteString("- Итого: " + FormatMoney(o.Total) + " руб.\n")
	}
	if len(o.Items) > 0 {
		b.WriteString("- Состав:\n")
		for _, it := range o.Items {
			b.WriteString(fmt.Sprintf("  • %s — %s %s × %s = %s руб.\n",
				it.Name, formatQty(it.Qty), it.Unit, FormatMoney(it.Price), FormatMoney(it.Sum)))
		}
	}
	if b.Len() == 0 {
		return "предложение ещё не рассчитано\n"
	}
	return b.String()
}

// renderContextBlock — контекст компании; пустой набор не добавляет блок,
// чтобы промпт не заканчивался пустым заголовком.
func renderContextBlock(ctx model.ProductContext) string {
	labels := map[string]string{
		"positioning":       "Что продаём",
		"pains":             "Боли клиентов (их словами)",
		"objections":        "Возражения и наши ответы",
		"customer_language": "Язык клиента: употреблять / не употреблять",
		"proof":             "Доказательства, на которые можно опираться",
		"not_promised":      "Чего мы НЕ обещаем",
	}
	var b strings.Builder
	for _, key := range model.ContextSectionKeys {
		v := strings.TrimSpace(ctx.Sections[key])
		if v == "" {
			continue
		}
		b.WriteString("- " + labels[key] + ": " + v + "\n")
	}
	if b.Len() == 0 {
		return ""
	}
	return contextDoc + "\nКОНТЕКСТ КОМПАНИИ:\n" + b.String()
}
