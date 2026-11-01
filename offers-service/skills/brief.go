package skills

import "mailers/offers/model"

// Темы брифа — манифест разговора (2026-10-29, выбор пользователя: LLM решает
// порядок внутри манифеста, но не выходит за него). Порядок стабилен: по нему
// бот строит «что спросить дальше», и на нём же держится восстановление после
// перезапуска — тема хранится ключом, а не номером.
//
// Тема считается закрытой, когда соответствующее поле брифа заполнено. «Не
// знаю» / «пропустить» тоже закрывает тему — пустым значением.

// Topic — одна тема разговора.
type Topic struct {
	// Key — имя поля в model.BriefPayload (или псевдоключ для списков).
	Key string
	// Label — как тема называется в подписи «заполнено N из M».
	Label string
	// Question — что бот спрашивает. Один вопрос за шаг.
	Question string
	// Stage — этап разговора. Позиции собираются только после последнего
	// этапа: раньше бот не знает, для чего и кому предлагает (2026-11-01).
	Stage string
}

// Этапы разговора. Порядок совпадает с порядком тем: сначала объект и
// адресат, потом сделка, потом что предлагаем, потом условия. Позиции — не
// тема, а следующий шаг после последнего этапа.
const (
	StageObject = "object"
	StageDeal   = "deal"
	StageOffer  = "offer"
	StageTerms  = "terms"
)

// stageOrder — порядок этапов: по нему считается «какой этап сейчас».
var stageOrder = []struct {
	Key   string
	Label string
}{
	{Key: StageObject, Label: "Объект и адресат"},
	{Key: StageDeal, Label: "Сделка"},
	{Key: StageOffer, Label: "Что предлагаем"},
	{Key: StageTerms, Label: "Условия"},
}

// briefTopics — контекст объекта и адресата идёт первым (2026-11-01):
// «сразу с корабля на бал» — это когда бот просит позиции, не зная объекта.
var briefTopics = []Topic{
	{
		Key:      "object_name",
		Stage:    StageObject,
		Label:    "Объект",
		Question: "Для какого объекта делаем предложение? Назовите полное наименование — адрес, кадастровый номер, если есть.",
	},
	{
		Key:      "object_purpose",
		Stage:    StageObject,
		Label:    "Назначение объекта",
		Question: "Что это за объект по назначению? Жилой дом, склад, паркинг, производство, торговый центр?",
	},
	{
		Key:      "object_area",
		Stage:    StageObject,
		Label:    "Площадь",
		Question: "Какая площадь или объём работ? Квадратные метры кровли, фасада, стилобата — что известно.",
	},
	{
		Key:      "customer_address",
		Stage:    StageObject,
		Label:    "Адресат КП",
		Question: "Кому адресуем предложение? Организация, контактное лицо, город — так, как должно стоять в шапке.",
	},
	{
		Key:      "customer_goals",
		Stage:    StageDeal,
		Label:    "Задача заказчика",
		Question: "Какую задачу решаете на этом объекте? Что должно получиться в итоге?",
	},
	{
		Key:      "customer_pains",
		Stage:    StageDeal,
		Label:    "Боли и проблемы",
		Question: "Что сейчас болит у заказчика? Протечки, срыв сроков, переплата, претензии от надзора — что он сам называет проблемой?",
	},
	{
		Key:      "project_risks",
		Stage:    StageDeal,
		Label:    "Риски проекта",
		Question: "Какие риски у проекта? Грунтовые воды, высокая нагрузка, жёсткие сроки, зимние работы?",
	},
	{
		Key:      "planned_systems",
		Stage:    StageOffer,
		Label:    "Системы",
		Question: "Какие системы планируем предложить? Назовите по названию — например «ТН-КРОВЛЯ Авто» или «кровля», я найду в Базе знаний.",
	},
	{
		Key:      "objections",
		Stage:    StageTerms,
		Label:    "Возражения",
		Question: "Какие возражения ждём от заказчика? Например «дорого» или «у нас уже есть подрядчик» — и что мы на это ответим.",
	},
	{
		Key:      "budget_signal",
		Stage:    StageTerms,
		Label:    "Бюджет",
		Question: "Что известно о бюджете? Есть ли ограничение по сумме или ориентир по цене за квадратный метр?",
	},
	{
		Key:      "decision_maker",
		Stage:    StageTerms,
		Label:    "Кто решает",
		Question: "Кто принимает решение по закупке и кто ещё влияет? Прораб, технадзор, снабжение?",
	},
	{
		Key:      "timeline",
		Stage:    StageTerms,
		Label:    "Сроки",
		Question: "Когда материалы нужны на объекте? Есть ли дата, к которой надо поставить?",
	},
	{
		Key:      "why_us",
		Stage:    StageTerms,
		Label:    "Почему мы",
		Question: "Почему заказчик должен выбрать нас? Может быть, у вас уже были с ним проекты или он слышал о нас от кого-то?",
	},
	{
		Key:      "notes",
		Stage:    StageTerms,
		Label:    "Прочее",
		Question: "Что ещё важно сказать, о чём я не спросил?",
	},
}

// BriefTopics возвращает копию манифеста: вызывающий не должен менять порядок
// случайно, поэтому срез копируется.
func BriefTopics() []Topic {
	out := make([]Topic, len(briefTopics))
	copy(out, briefTopics)
	return out
}

// topicFilled — закрыта ли тема по текущему брифу.
func topicFilled(p model.BriefPayload, key string) bool {
	// Пропущенная тема закрыта, даже если значение пустое: иначе бот спросит
	// её заново после ответа «не знаю».
	for _, s := range p.Skipped {
		if s == key {
			return true
		}
	}
	switch key {
	case "object_name":
		return model.NonEmpty(p.ObjectName)
	case "object_purpose":
		return model.NonEmpty(p.ObjectPurpose)
	case "object_area":
		return model.NonEmpty(p.ObjectArea)
	case "customer_address":
		return model.NonEmpty(p.CustomerAddress)
	case "customer_pains":
		return len(p.CustomerPains) > 0
	case "customer_goals":
		return len(p.CustomerGoals) > 0
	case "project_risks":
		return len(p.ProjectRisks) > 0
	case "planned_systems":
		return len(p.PlannedSystems) > 0
	case "objections":
		for _, o := range p.Objections {
			if model.NonEmpty(o.Claim) && model.NonEmpty(o.Response) {
				return true
			}
		}
		return false
	case "budget_signal":
		return model.NonEmpty(p.BudgetSignal)
	case "decision_maker":
		return model.NonEmpty(p.DecisionMaker)
	case "timeline":
		return model.NonEmpty(p.Timeline)
	case "why_us":
		return len(p.WhyUs) > 0
	case "notes":
		return model.NonEmpty(p.Notes)
	}
	return false
}

// NextOpenTopic — первая незакрытая тема в порядке манифеста. false, когда
// все темы закрыты (бриф готов к согласованию).
func NextOpenTopic(p model.BriefPayload) (Topic, bool) {
	for _, t := range briefTopics {
		if !topicFilled(p, t.Key) {
			return t, true
		}
	}
	return Topic{}, false
}

// IsTopicFilled — экспорт для бота и плагина: «заполнено N из M» и подсветка
// незаполненных тем.
func IsTopicFilled(p model.BriefPayload, key string) bool {
	return topicFilled(p, key)
}

// Stages — этапы разговора по порядку с подписями. Бот показывает этап, чтобы
// менеджер понимал, что до позиций ещё далеко (2026-11-01).
func Stages() []struct{ Key, Label string } {
	out := make([]struct{ Key, Label string }, len(stageOrder))
	copy(out, stageOrder)
	return out
}

// StageLabel — подпись этапа по ключу; пусто для незнакомого ключа.
func StageLabel(key string) string {
	for _, s := range stageOrder {
		if s.Key == key {
			return s.Label
		}
	}
	return ""
}

// StageOf — этап темы; пусто для незнакомого ключа.
//
// Тот же обход, что у StageLabel, и это осознанно: список этапов и манифест
// тем — разные срезы с разными полями; generic ради двух строк читался бы хуже.
func StageOf(key string) string {
	for _, t := range briefTopics {
		if key == t.Key {
			return t.Stage
		}
	}
	return ""
} // CurrentStage — этап, на котором стоит разговор: этап первой незакрытой
// темы. Все темы закрыты — пусто: контекст собран, дальше позиции.
func CurrentStage(p model.BriefPayload) string {
	if t, ok := NextOpenTopic(p); ok {
		return t.Stage
	}
	return ""
}

// briefRules — что модель делает в разговоре.
const briefRules = `ТЫ ВЕДЁШЬ РАЗГОВОР С МЕНЕДЖЕРОМ, КОТОРЫЙ ГОТОВИТ КОММЕРЧЕСКОЕ
ПРЕДЛОЖЕНИЕ ЗАКАЗЧИКУ.

Это не анкета. Это спокойный разговор: менеджер рассказывает про объект, ты
уточняешь и записываешь. Можешь переспросить, если ответ неполный или
двусмысленный. Можешь заметить, что менеджер сам себе противоречит.

Как вести:
- ОДИН вопрос за раз. Не задавай сразу три вопроса в одном сообщении.
- Сначала подтверди, что услышал: коротко, одной фразой, его словами.
- Потом задай следующий вопрос по теме.
- Если ответ уже содержит несколько тем — не переспрашивай то, что уже
  сказано, переходи к первой незаполненной.
- Если менеджер говорит «не знаю», «пропустим», «потом» — прими это и иди
  дальше. Не давить и не додумывать за него.
- Не больше двух уточнений подряд по одной теме: менеджер занят.

ЧЕГО НЕ ДЕЛАТЬ:
- не придумывай факты об объекте, заказчике, конкурентах и цифрах;
- не предлагай материалы и системы — это менеджер выбирает отдельно;
- не пиши текст самого предложения — это следующий шаг;
- не задавай вопрос, ответ на который уже есть в известных полях ниже.

ПОРЯДОК РАЗГОВОРА. Сначала контекст, потом позиции:
1. Объект и адресат: что за объект, его назначение, площадь, кому адресуем.
2. Сделка: задача, боли, риски.
3. Что предлагаем: какие системы.
4. Условия: бюджет, сроки, кто решает, возражения, прочее.
Позиции и цены — только ПОСЛЕ этого, это отдельный шаг, не твой.
Не перескакивай через темы: если контекст объекта не собран, не спрашивай
про сроки и бюджет.

ОТВЕТ ФОРМАТОМ JSON.
В первом блоке — только JSON без пояснений, маркеров кода и текста вокруг:
{"payload": {…полный бриф с учётом последнего ответа…},
 "reply": "что сказать менеджеру: подтверждение и следующий вопрос",
 "skip": ["ключ_темы"]}

В payload верни ВЕСЬ бриф целиком, включая ранее заполненные поля. Не
придумывай значения: если факт не назван — оставь пустую строку или пустой
список.

Про skip: если менеджер отказался отвечать («не знаю», «пропустим», «потом»),
добавь ключ этой темы в список skip и оставь поле пустым. Без этого тема
останется открытой, и бот спросит её снова.
`
