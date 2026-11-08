package skills

import (
	"fmt"
	"regexp"
	"strconv"
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
// tableDoc — правило для подсказки, которая просит ТАБЛИЦУ.
//
// Таблицу модель возвращает только в разметке Markdown: сервер узнаёт её и
// печатает настоящей таблицей Word (`layoutTable` в docx_layout.go).
// Свободным текстом таблицу не построить — именно это и показал живой прогон
// КП 003/2026, где вместо таблицы пришла проза с рассуждением.
const tableDoc = `
ТАБЛИЦА. Подсказка просит таблицу — верни её разметкой Markdown, и ничего
кроме неё:

| ЕКН | Наименование материала | Единица поставки | Цена за единицу, руб | Объем поставки | Стоимость, руб |
|---| --- | --- | --- | --- | --- |
| | Мембрана V-SL 2 мм | м2 | 769,56 | 5520 | 4 247 971,20 |

Правила таблицы:
- Колонки — ровно те, что просит подсказка, в том же порядке и с теми же
  подписями. Пустой столбец не выдумывай и не удаляй: ставь пустую ячейку.
- Строка-подзаголовок группы (элемент конструкции или место применения)
  идёт отдельной строкой, где название в первом столбце, остальные пусты.
- Числа бери из данных предложения: цена и стоимость — те, что в составе.
  Ничего не считай заново и не округляй по-своему.
- Нет значения (например, ЕКН или коэффициент запаса) — оставь ячейку
  пустой. Вопросов и пометок в таблице не пиши.
- После таблицы можно одну строку с итоговой стоимостью, если подсказка
  её просит.
- Никаких пояснений до и после таблицы: ни «ниже приведена», ни разбора
  того, как ты её собирал.
`

// RequestedWords — сколько слов просит подсказка, если просит.
//
// По умолчанию блок пишет около 150 слов, и это правильно для абзаца. Но
// администратор вправе попросить объём прямо: в живом макете КП 003/2026
// стоит «Объем текста не менее 4000 знаков на одну систему». Это примерно
// 600 слов, а бюджет ответа оставался 150 — модели не хватало места, и она
// отдавала план («1. Intro… 2. h2 "Состав решения"…») вместо текста.
//
// Возвращает 0, если объём не задан: тогда действует обычный.
// Понимает «не менее 4000 знаков» (знаки), «не меньше 500 слов» (слова) и
// «4000 символов». Знаки пересчитываются в слова по 6,5 знака на слово —
// средняя длина русского слова с пробелом.
func RequestedWords(instruction string) int {
	matches := reRequestedVolume.FindAllStringSubmatch(strings.ToLower(instruction), -1)
	best := 0
	for _, m := range matches {
		n := atoiSafe(m[1])
		if n <= 0 {
			continue
		}
		unit := m[2]
		words := n
		if strings.HasPrefix(unit, "зн") || strings.HasPrefix(unit, "симв") {
			words = n / 6
		}
		if words > best {
			best = words
		}
	}
	return best
}

// reRequestedVolume — «не менее 4000 знаков», «не меньше 500 слов»,
// «4000 символов»: число и единица рядом.
var reRequestedVolume = regexp.MustCompile(`(\d+)\s*(знак|знаков|символ|символов|слов|слова)`)

// atoiSafe — число из строки, 0 при мусоре.
func atoiSafe(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// asksForTable — просит ли подсказка таблицу.
//
// Признак берётся по словам, которые администратор реально пишет в макете:
// «вставь таблицу», «таблицей», «в таблицу». Ложное срабатывание безобидно:
// правило таблицы тогда просто расширит формат, а текст останется текстом.
func asksForTable(instruction string) bool {
	lower := strings.ToLower(instruction)
	for _, marker := range []string{"таблиц", "table"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// reForbidsPrices — явный запрет цен/стоимости в подсказке администратора.
var reForbidsPrices = regexp.MustCompile(`(?i)(без|не\s+привод\S*|не\s+указыва\S*|не\s+включа\S*|не\s+упомина\S*|не\s+пиш\S*|исключ\S*)\s+[^.!?\n]{0,40}?(стоимост|цен|расцен|бюджет|руб|сум)`)

// AsksForPrices — просит ли подсказка администратора привести цены или
// стоимость (2026-11-08, живая жалоба на КП 003/2026).
//
// По умолчанию текстовые блоки КП (введение, техническое описание системы)
// описывают инженерное решение, а цены и итоговая сумма печатаются в таблице
// коммерческого предложения. Если модель видит суммы в пользовательском промпте,
// она вставляет их в конец каждого блока даже вопреки запрету «без стоимостных
// показателей». Поэтому цены даются модели только тогда, когда подсказка прямо
// просит о стоимости и не содержит запрета на неё.
func AsksForPrices(instruction string) bool {
	lower := strings.ToLower(instruction)
	if reForbidsPrices.MatchString(lower) {
		return false
	}
	for _, marker := range []string{"стоимост", "цен", "расцен", "итогов", "рубл"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

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
	b.WriteString("Это текст печатного документа: обычные абзацы. Список — только там, где он ")
	b.WriteString("уместен по смыслу (перечень слоёв, преимущества), и тогда каждый пункт ")
	b.WriteString("с новой строки.\n")
	b.WriteString("Адресов сайтов и ссылок в тексте не пиши — кроме раздела ссылок, если подсказка ")
	b.WriteString("просит его отдельно: тогда каждый адрес идёт адресом, чтобы он стал кликабельным.\n")
	// Подсказка может прямо просить таблицу — тогда формат ответа другой.
	// Живой прогон КП 003/2026 на макете (2026-11-06): промт просил таблицу,
	// а общий запрет на списки говорил обратное; модель провела весь этот
	// конфликт рассуждением вслух («Проблема: ЕКН не дан… Хм…»), и клиент
	// увидел черновик вместо таблицы. Просьба администратора главнее общего
	// правила, и она должна быть в промпте явной.
	if asksForTable(instruction) {
		b.WriteString(tableDoc)
	}
	b.WriteString("\nЧЕГО НЕ ДЕЛАТЬ.\n")
	b.WriteString("1. Не пересказывай задание и не показывай, как ты думаешь. Строки «The task», ")
	b.WriteString("«Let me think», «I should», «hmm», перечисление того, что известно и чего нет, ")
	b.WriteString("черновик с последующей правкой — это черновик, а не блок. В ответе только ")
	b.WriteString("готовый текст для клиента. Ничего по-английски: ни слов, ни фраз.\n")
	b.WriteString("2. В ответе нет ни одного предложения о самом тексте: «ниже приведено», ")
	b.WriteString("«в этом разделе», «надеюсь, это подходит», «слов: 148».\n")
	b.WriteString("3. Не хватает факта — короткая пометка ровно в той фразе, где он нужен, ")
	b.WriteString("и больше ничего. Не превращай нехватку в рассуждение о том, чего писать не стоит.\n")
	// План вместо текста — самый частый провал на макете (2026-11-06).
	// Живой ответ 003/2026 приходил так: «1. Intro (задача объекта…).
	// 2. h2 "Состав решения" — перечень слоёв. 3. h2 "Мембрана…"». Это
	// оглавление будущего текста, а не текст: клиенту оно ничего не говорит,
	// а места занимает столько же, сколько настоящий раздел.
	b.WriteString("4. Не пиши ПЛАН. Нумерованный перечень разделов («1. Вступление, ")
	b.WriteString("2. Состав системы…»), строки вида «h2 …», «Heading …», «Intro …», ")
	b.WriteString("«Section …» — это оглавление, а не блок. Пиши сразу тот текст, ")
	b.WriteString("который прочитает заказчик, в том порядке, в каком он будет в документе. ")
	b.WriteString("Если подсказка просит несколько частей — напиши каждую часть целиком, ")
	b.WriteString("а не её название.\n")
	if !AsksForPrices(instruction) {
		b.WriteString("5. Не приводи цены, расценки и итоговую стоимость предложения (в рублях): ")
		b.WriteString("финансовый расчёт вынесен в отдельный раздел документа.\n")
	}
	// Структуру просить МОЖНО, потому что печать её теперь понимает
	// (2026-11-06). До этой правки здесь стоял запрет на разметку: просьба
	// макета «помечай заголовки» давала `##` в документе, и знаки уходили
	// заказчику. Запрет лечил симптом и отнимал у администратора главное —
	// возможность разделить текст на части. Теперь `##` и `- ` не печатаются
	// как текст (layoutLines в docx_layout.go), а становятся заголовком и
	// списком документа.
	b.WriteString("\nЗАГОЛОВКИ И СПИСКИ.\n")
	b.WriteString("Если подсказка просит разделить текст на части — начинай каждую часть ")
	b.WriteString("строкой «## Название части»: в документе это станет заголовком. ")
	b.WriteString("Списки оформляй пунктами с «- » в начале строки: в документе это станет ")
	b.WriteString("списком, а не перечислением в строку. Нумерованный список — «1. », «2. » ")
	b.WriteString("с начала строки. Никакой другой разметки не нужно: звёздочки жирного, ")
	b.WriteString("таблицы в тексте и блоки кода в документе не печатаются.\n")
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
