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
	list("Задача заказчика", p.CustomerGoals)
	list("Боли", p.CustomerPains)
	list("Риски проекта", p.ProjectRisks)
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
