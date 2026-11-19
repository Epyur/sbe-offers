package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// Placeholder — поле формы КП, которое администратор заводит в конфигураторе.
type Placeholder struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Block — блок документа формы (абзацы/заголовки/списки/таблицы, разрывы
// страниц, ссылки и текст по промту).
//
// Размещение на странице (2026-11-01): отступы в пунктах (Word меряет абзацный
// интервал в twips, 1 пт = 20; в JSON пишем пункты — админу понятнее),
// «не разрывать между страницами» и «начинать с новой страницы». Абсолютного
// позиционирования нет намеренно: текстовые поля и якоря не переживают правку
// абзаца человеком, а для письма нужны именно отступы, выравнивание и разрыв.
type Block struct {
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	Content        json.RawMessage `json:"content"`
	ShowInClient   bool            `json:"showInClient"`
	ShowInInternal bool            `json:"showInInternal"`
	// SpacingBefore / SpacingAfter — отступ до и после блока в пунктах.
	// 0 — как в стиле бланка; отрицательные не допускаются.
	SpacingBefore int `json:"spacingBefore,omitempty"`
	SpacingAfter  int `json:"spacingAfter,omitempty"`
	// KeepTogether — не разрывать абзацы блока между страницами.
	KeepTogether bool `json:"keepTogether,omitempty"`
	// PageBreakBefore — начинать блок с новой страницы.
	PageBreakBefore bool `json:"pageBreakBefore,omitempty"`
	// Repeat / Source — повтор и источник данных (2026-11-06, шаг D плана
	// «генерация без модели»). `{{block:N}}` для блока с repeat:"systems"
	// печатается по разу на выбранную систему, с repeat:"materials" — по
	// позициям предложения. Source:"card" говорит, что данные блока — карточка
	// Базы знаний, а не собственный текст администратора (сейчас используется
	// для подсказки в редакторе; печать его не требует).
	Repeat string `json:"repeat,omitempty"`
	Source string `json:"source,omitempty"`
}

// Виды повтора блока формы.
const (
	blockRepeatSystems   = "systems"
	blockRepeatMaterials = "materials"
)

// RepeatName — имя повтора: id блока.
//
// Метки макета ссылаются на повтор через `{{… name="S"}}`, и имя — это id
// блока: администратор пишет `{{система name="7"}}`, где 7 — id блока с
// repeat:"systems". Одно имя на два места, и держать его в двух полях незачем.
func (b Block) RepeatName() string {
	if strings.TrimSpace(b.Repeat) == "" {
		return ""
	}
	return strings.TrimSpace(b.ID)
}

// SubsectionCondition — условие, при котором подраздел печатается (2026-11-14).
//
// Условия нарочно маленькие и декларативные — не код и не язык формул. Их
// смысл один: подраздел, чьё условие не выполнено, не печатается ВООБЩЕ — ни
// заголовка, ни пустой таблицы. Это и есть разница с блоками `{{block:…}}`,
// из-за которой администратор получал заголовки без текста.
type SubsectionCondition string

const (
	// conditionAlways — подраздел печатается всегда.
	conditionAlways SubsectionCondition = "always"
	// conditionHasSystems — в предложении есть хотя бы одна выбранная система.
	conditionHasSystems SubsectionCondition = "has_systems"
	// conditionHasMaterials — есть хотя бы один материал, выбранный описанием.
	conditionHasMaterials SubsectionCondition = "has_materials"
	// conditionMultiSystem — систем больше одной: случай «почему эти».
	conditionMultiSystem SubsectionCondition = "multi_system"
	// conditionHasCritique — администратор включил у раздела переключатель
	// `critique` (единственное условие, читающее РАЗДЕЛ, а не предложение).
	conditionHasCritique SubsectionCondition = "has_critique"
	// conditionHasItems — в предложении есть позиции с ценой.
	conditionHasItems SubsectionCondition = "has_items"
)

// subsectionConditions — известные условия. Одно место и для проверки формы, и
// для проверки разбора: новое условие без правки здесь просто не сохранится.
var subsectionConditions = []SubsectionCondition{
	conditionAlways, conditionHasSystems, conditionHasMaterials,
	conditionMultiSystem, conditionHasCritique, conditionHasItems,
}

// knownSubsectionCondition — условие известно серверу.
func knownSubsectionCondition(c SubsectionCondition) bool {
	for _, known := range subsectionConditions {
		if c == known {
			return true
		}
	}
	return false
}

// ContentSubsection — подраздел содержательного раздела: ОДИН вызов модели.
//
// Модель пишет текст и структуру (заголовок/абзац/список/таблица/ссылка на
// рисунок); оформление берётся из макета Word. Здесь только бриф: что писать,
// при каком условии и когда звать модель по разу на систему.
type ContentSubsection struct {
	// ID — устойчивый идентификатор: он же префикс ключа кэша текста.
	ID string `json:"id"`
	// Title — заголовок подраздела. Его печатает рамка, а не модель.
	Title string `json:"title"`
	// Instruction — что написать: бриф для модели.
	Instruction string `json:"instruction"`
	// Condition — при каком условии печатать. Пусто — `always`.
	Condition SubsectionCondition `json:"condition,omitempty"`
	// Repeat — `systems`: один вызов на каждую выбранную систему.
	Repeat string `json:"repeat,omitempty"`
	// MaxWords — предел длины текста; 0 — серверное значение по умолчанию.
	MaxWords int `json:"maxWords,omitempty"`
	// Level — уровень заголовка рамки (1..3); 0 — значение по умолчанию.
	Level int `json:"level,omitempty"`
}

// ConditionOrAlways — условие с умолчанием: пусто значит «всегда».
func (s ContentSubsection) ConditionOrAlways() SubsectionCondition {
	if strings.TrimSpace(string(s.Condition)) == "" {
		return conditionAlways
	}
	return s.Condition
}

// ContentSection — содержательный раздел: настройка, на которую ссылается
// метка `{ content: <id> }` (2026-11-14).
type ContentSection struct {
	// ID — id метки: `{ content: <id> }`.
	ID string `json:"id"`
	// Title — человеческое название раздела; его печатает рамка.
	Title string `json:"title"`
	// Numbered — печатать нумерацию разделов (номера даёт нумерация макета).
	Numbered bool `json:"numbered,omitempty"`
	// Critique — переключатель, который читает условие `has_critique`.
	// Один рубильник: его включает администратор, его же проверяет условие.
	Critique bool `json:"critique,omitempty"`
	// Subsections — подразделы в порядке печати.
	Subsections []ContentSubsection `json:"subsections,omitempty"`
}

// OfferForm — конфигурация формы КП.
type OfferForm struct {
	ID           int64         `json:"id"`
	Mode         string        `json:"mode"`
	Title        string        `json:"title"`
	Placeholders []Placeholder `json:"placeholders"`
	Blocks       []Block       `json:"blocks"`
	// Sections — содержательные разделы (2026-11-14): настройки, на которые
	// ссылаются метки `{ content: <id> }`. Сосед блоков: блоки — вёрстка,
	// написанная администратором руками, разделы — бриф для модели.
	Sections       []ContentSection `json:"sections,omitempty"`
	RequiredFields []string         `json:"requiredFields"`
	// BasePrompt — БАЗОВЫЙ ПРОМТ документа (2026-11-17, просьба пользователя):
	// общая часть системного промта, которую видит модель при написании каждого
	// подраздела. Раньше этот текст жил только в коде (`sectionAnswerContract`),
	// и правка формулировок требовала выпуска плагина; теперь администратор
	// правит его в форме — в том числе требования к согласованности разделов.
	//
	// Пусто — встроенный текст (`sectionAnswerContract`), то есть прежнее
	// поведение: старая форма печатает документы как раньше.
	BasePrompt string `json:"basePrompt,omitempty"`
	// BasePromptDefault — встроенный базовый промт (только на чтение): редактор
	// показывает его, когда своё значение не задано, и по нему делает «вернуть
	// базовый текст». В базу не пишется.
	BasePromptDefault string `json:"basePromptDefault,omitempty"`
	// SectionsDefault — встроенная заготовка разделов (только на чтение,
	// 2026-11-18). До этой версии структура разделов листка жила ТОЛЬКО в коде
	// (`combatDefaultSections`), и вернуть её после правок было нечем: редактор
	// заготовки не знал. Теперь она приходит вместе с формой, а кнопка
	// «↩ Вернуть заготовку» ставит её в разделы.
	//
	// В базу поле не пишется и в отпечаток формы (`combatFormFingerprint`) не
	// входит: там только `sections` и `basePrompt`, иначе возврат заготовки
	// менял бы хэш файла дважды — от самих разделов и от копии заготовки.
	SectionsDefault []ContentSection `json:"sectionsDefault,omitempty"`
	// Window — окно КП в приложении (2026-11-15): порядок, видимость и
	// обязательность полей формы и зон экрана. Пусто — «как сейчас»
	// (`effectiveWindow` строит нынешнее поведение), и старая форма не
	// меняет ни экран, ни проверку обязательности.
	Window []WindowItem `json:"window,omitempty"`
	// WindowStored (2026-11-15) — окно БЫЛО записано в базе. Сервер отдаёт
	// окно всегда (`effectiveWindow`), а этому полю верить нельзя для отличия
	// «настройки нет» от «администратор всё скрыл» (форма с полями всегда даёт
	// окно). Поэтому флаг ставится на чтении из базы и в базу не пишется.
	WindowStored bool   `json:"windowStored,omitempty"`
	UpdatedAt    string `json:"updatedAt"`
	// HasLayout/LayoutName — загружен ли макет .docx (2026-11-05). Сами байты
	// в JSON формы не отдаются: клиенту нужно знать, есть ли макет, а не
	// тащить десятки килобайт в каждом ответе `/form`.
	HasLayout  bool   `json:"hasLayout"`
	LayoutName string `json:"layoutName,omitempty"`
	// LayoutFields/LayoutPrompts — какие метки найдены в макете: администратор
	// должен видеть, что система поняла из его документа, ещё до первой сборки.
	LayoutFields  []string `json:"layoutFields,omitempty"`
	LayoutPrompts []string `json:"layoutPrompts,omitempty"`
}

// maxBlockSpacing — предел отступа блока в пунктах. Свыше 200 пт блок
// уезжает с страницы целиком, и это ошибка админа, а не оформление.
const maxBlockSpacing = 200

// validateForm проверяет форму перед сохранением; пустая строка — форма корректна.
func validateForm(f OfferForm) string {
	if strings.TrimSpace(f.Title) == "" {
		return "не заполнен заголовок формы"
	}
	seenBlocks := make(map[string]bool, len(f.Blocks))
	for _, block := range f.Blocks {
		if strings.TrimSpace(block.ID) == "" {
			return "у блока пустой идентификатор"
		}
		if seenBlocks[block.ID] {
			return "повторяющийся идентификатор блока"
		}
		seenBlocks[block.ID] = true
		if block.SpacingBefore < 0 || block.SpacingAfter < 0 {
			return "отступ блока «" + block.Title + "» не может быть отрицательным"
		}
		if block.SpacingBefore > maxBlockSpacing || block.SpacingAfter > maxBlockSpacing {
			return "отступ блока <" + block.Title + "> больше " + strconv.Itoa(maxBlockSpacing) + " пт"
		}
		// Повтор блока (2026-11-06, шаг D): только известные значения. Молча
		// не повторить блок значило бы напечатать его один раз и не сказать.
		switch block.Repeat {
		case "", blockRepeatSystems, blockRepeatMaterials:
		default:
			return "неизвестный повтор блока <" + block.Title + ">: " + block.Repeat
		}
	}
	seenFields := make(map[string]bool, len(f.Placeholders))
	for _, p := range f.Placeholders {
		if strings.TrimSpace(p.ID) == "" {
			return "пустой идентификатор поля"
		}
		if seenFields[p.ID] {
			return "повторяющийся идентификатор поля"
		}
		seenFields[p.ID] = true
	}
	for _, req := range f.RequiredFields {
		if !seenFields[req] {
			return "обязательное поле «" + req + "» не заведено в списке полей"
		}
	}
	if msg := validateSections(f.Sections); msg != "" {
		return msg
	}
	// Боевой листок без разделов (2026-11-18). Пустой список не значит «печатать
	// только карточки»: `renderCombatSections` подставляет заготовку, то есть
	// документ всё равно печатает разделы — но уже не те, что администратор
	// видит в окне. «В окне пусто, в документе текст» — та же тихая потеря
	// настройки, что в 0.10.2 с разделами КП, поэтому сохранение отклоняется и
	// отказ называет выход.
	if f.Mode == combatFormMode && len(f.Sections) == 0 {
		return combatEmptySectionsProblem
	}
	// Окно КП (2026-11-15) проверяется после полей: оно ссылается на их id.
	if msg := validateWindow(f); msg != "" {
		return msg
	}
	// Базовый промт (2026-11-17): предел на всякий случай — это текст для
	// модели, и гигантское значение съело бы окно на каждом подразделе.
	if runes := len([]rune(f.BasePrompt)); runes > maxBasePromptRunes {
		return "Базовый промт слишком длинный: " + itoa(runes) + " знаков, предел " + itoa(maxBasePromptRunes)
	}
	return ""
}

// maxBasePromptRunes — предел длины базового промта формы.
const maxBasePromptRunes = 20000

// combatEmptySectionsProblem — отказ на пустое сохранение формы боевого листка.
//
// Тот же текст держит клиент (`COMBAT_SECTIONS_EMPTY` в
// `src/ui/sections-model.ts`): администратор видит отказ ещё до запроса, а не
// после круглого отказа сервера. Совпадение текстов сторожит
// `scripts/check_combat_sections_rules.mjs`.
const combatEmptySectionsProblem = "У боевого листка нет ни одного раздела: без разделов документ печатает только карточки. Нажмите «↩ Вернуть заготовку» или добавьте раздел."

// validateSections — проверка содержательных разделов (2026-11-14).
//
// Ошибка в разделе обязана называться ДО первой сборки: молча не выполненное
// условие человек увидит как пропавший кусок документа и будет искать причину в
// модели.
func validateSections(sections []ContentSection) string {
	seenSections := make(map[string]bool, len(sections))
	for _, section := range sections {
		id := strings.TrimSpace(section.ID)
		if id == "" {
			return "у содержательного раздела пустой идентификатор"
		}
		if seenSections[id] {
			return "повторяющийся идентификатор содержательного раздела: " + id
		}
		seenSections[id] = true
		if strings.TrimSpace(section.Title) == "" {
			return "у содержательного раздела «" + id + "» пустой заголовок"
		}
		if msg := validateSubsections(section); msg != "" {
			return msg
		}
	}
	return ""
}

// validateSubsections — подразделы одного раздела: уникальные id, известное
// условие, известный повтор.
func validateSubsections(section ContentSection) string {
	seen := make(map[string]bool, len(section.Subsections))
	for _, sub := range section.Subsections {
		id := strings.TrimSpace(sub.ID)
		if id == "" {
			return "у подраздела раздела «" + section.ID + "» пустой идентификатор"
		}
		if seen[id] {
			return "повторяющийся идентификатор подраздела «" + id + "» в разделе «" + section.ID + "»"
		}
		seen[id] = true
		if strings.TrimSpace(sub.Title) == "" {
			return "у подраздела «" + id + "» раздела «" + section.ID + "» пустой заголовок"
		}
		if strings.TrimSpace(sub.Instruction) == "" {
			return "у подраздела «" + id + "» раздела «" + section.ID + "» нет инструкции для модели"
		}
		if !knownSubsectionCondition(sub.ConditionOrAlways()) {
			return "у подраздела «" + id + "» неизвестное условие: " + string(sub.Condition)
		}
		// Повтор: пусто или «по системам». Неизвестный повтор молча не выполнился
		// бы — подраздел напечатался бы один раз, хотя администратор ждал по
		// системе на каждую.
		switch strings.TrimSpace(sub.Repeat) {
		case "", blockRepeatSystems:
		default:
			return "у подраздела «" + id + "» неизвестный повтор: " + sub.Repeat
		}
		if sub.MaxWords < 0 {
			return "у подраздела «" + id + "» отрицательный предел слов"
		}
		if sub.Level < 0 || sub.Level > 3 {
			return "у подраздела «" + id + "» уровень заголовка вне 1..3: " + itoa(sub.Level)
		}
	}
	return ""
}

// defaultForm — форма по умолчанию: пока администратор ничего не настроил,
// предложение всё равно собирается.
//
// Порядок — как просил пользователь (2026-11-01): титульный лист, разрыв
// страницы, затем поставщик / получатель / объект, свободный блок про систему и
// материалы (текст пишет модель по подсказке), позиции (их печатает сборщик
// документа сам) и ссылки в конце.
//
// Уже сохранённые формы эта функция не трогает: «строки нет в БД» — это и есть
// «форма не настроена», а сохранённая пустая форма остаётся пустой (правило
// 2026-10-27, живой дефект с подменой формы).
func defaultForm(mode string) OfferForm {
	if mode == combatFormMode {
		return combatDefaultForm()
	}
	return OfferForm{
		Mode:              mode,
		Title:             "Коммерческое предложение",
		BasePromptDefault: sectionAnswerContract,
		Placeholders: []Placeholder{
			{ID: "supplier_name", Label: "Поставщик"},
			{ID: "supplier_contacts", Label: "Контакты поставщика"},
			{ID: "client_name", Label: "Получатель (организация)"},
			{ID: "client_contact", Label: "Контактное лицо получателя"},
			{ID: "object_name", Label: "Объект"},
			{ID: "object_address", Label: "Адрес объекта"},
			{ID: "valid_until", Label: "Предложение действительно до"},
			{ID: "conditions", Label: "Условия"},
			// Поля брифа (2026-11-15). Бриф живёт отдельно (`offers_brief`), но
			// `briefFormValues` переносит его в значения формы под этими id — и
			// только форма знает, какие поля существуют. Без них модель не видит
			// ответов менеджера, а окно не может их показать: на живом КП
			// 003/2026 заполненный бриф дошёл до модели пустым, и она написала
			// «[НУЖНО: …]» (та же ловушка, что описана в `brief.go`).
			{ID: "brief.object_name", Label: "Объект (бриф)"},
			{ID: "brief.object_purpose", Label: "Назначение объекта (бриф)"},
			{ID: "brief.object_area", Label: "Площадь (бриф)"},
			{ID: "brief.customer_address", Label: "Адресат КП (бриф)"},
			{ID: "brief.planned_systems", Label: "Системы (бриф)"},
			{ID: "brief.customer_goals", Label: "Задачи клиента (бриф)"},
			{ID: "brief.customer_pains", Label: "Боли (бриф)"},
			{ID: "brief.project_risks", Label: "Риски (бриф)"},
			{ID: "brief.decision_maker", Label: "Кто решает (бриф)"},
			{ID: "brief.timeline", Label: "Сроки (бриф)"},
			{ID: "brief.budget_signal", Label: "Бюджет (бриф)"},
			{ID: "brief.why_us", Label: "Почему мы (бриф)"},
			{ID: "brief.notes", Label: "Прочее (бриф)"},
		},
		Blocks: []Block{
			{
				ID:           "cover",
				Title:        "Титульный лист",
				Content:      json.RawMessage(coverBlockJSON),
				ShowInClient: true,
			},
			{
				ID:              "parties",
				Title:           "Поставщик, получатель, объект",
				Content:         json.RawMessage(partiesBlockJSON),
				ShowInClient:    true,
				PageBreakBefore: true,
			},
			{
				ID:            "description",
				Title:         "Система и материалы",
				Content:       json.RawMessage(descriptionBlockJSON),
				ShowInClient:  true,
				SpacingBefore: 12,
			},
			{
				ID:           "links",
				Title:        "Ссылки",
				Content:      json.RawMessage(`[{"type":"links","level":2}]`),
				ShowInClient: true,
			},
		},
		RequiredFields: []string{"client_name"},
	}
}

// Содержимое блоков формы по умолчанию. Текстом, а не собранным в коде JSON:
// эти строки правятся при изменении шаблона, и держать их рядом проще.
const (
	// coverBlockJSON — титульный лист. Абзацы по центру, подписи полей — в
	// одном абзаце с {{полем}}: пустое поле даёт «Кому: » без пустых строк.
	//
	// Номер и дата — служебными полями (2026-11-02): у каждого КП они свои, и
	// набирать их в форме руками значило бы забыть обновить после копирования.
	coverBlockJSON = `[
		{"type":"paragraph","text":"Коммерческое предложение № {{offer_number}} от {{offer_date}}","align":"center"},
		{"type":"paragraph","text":"{{supplier_name}}","align":"center"},
		{"type":"paragraph","text":"Кому: {{client_name}}","align":"center"},
		{"type":"paragraph","text":"{{client_contact}}","align":"center"}
	]`

	// partiesBlockJSON — содержательная часть: кто предлагает, кому и для
	// какого объекта. Таблицей из двух колонок, как в письмах.
	partiesBlockJSON = `[
		{"type":"heading","text":"Об объекте","level":2},
		{"type":"table","rows":[
			["Поставщик","{{supplier_name}}"],
			["Контакты","{{supplier_contacts}}"],
			["Получатель","{{client_name}}"],
			["Контактное лицо","{{client_contact}}"],
			["Объект","{{object_name}}"],
			["Адрес объекта","{{object_address}}"]
		]}
	]`

	// descriptionBlockJSON — свободный блок: текст пишет модель по подсказке,
	// опираясь на объект, системы из карточек и состав предложения.
	descriptionBlockJSON = `[
		{"type":"heading","text":"Предлагаемое решение","level":2},
		{"type":"prompt_text","instruction":"Опиши предлагаемую систему и материалы, входящие в неё: как они работают вместе и что дают на этом объекте. Опирайся только на данные предложения и карточки систем.","maxWords":200}
	]`
)

// combatFormMode — режим формы боевого листка в таблице offers_form.
//
// Форма боевого листка — ТА ЖЕ форма КП (тот же редактор блоков), только под
// другим режимом. Отдельная таблица не заводится: блоки, поля и макет .docx у
// двух документов одинаковой природы, и вторая таблица разошлась бы с первой
// при первой же правке редактора.
const combatFormMode = "combat"

// clientFormMode — режим обычного коммерческого предложения.
const clientFormMode = "client"

// combatDefaultForm — форма боевого листка по умолчанию.
//
// Отличие от КП — в назначении: это справка-сравнение для менеджера, а не
// письмо заказчику. Поэтому в ней титул с названием и датой, блок нашего
// предложения и блок конкурентов, а также свободный блок аргументации. Редактор
// администратора может перестроить всё это, как и форму КП.
func combatDefaultForm() OfferForm {
	// У боевого листка формы как таковой НЕТ (2026-11-11, решение
	// пользователя): содержание всегда одно — перечень продуктов, затем их
	// сравнение. Настройка осталась одна: УТОЧНЕНИЕ менеджера, на чём сделать
	// акцент в разборе. Прежние поля (наша компания, объект, заказчик,
	// менеджер) были невалидны: заказчика у внутренней справки нет.
	//
	// 2026-11-16 (шаг 2a плана sbe-offers): содержание листка стало РАЗДЕЛАМИ —
	// та же модель `ContentSection`, что у КП, и печать идёт общим конвейером
	// (`combat_sections.go`). Заготовка ниже — то, что менеджер называл в
	// жалобе: где конкурент сильнее и что отвечать, сравнение таблицей, вопросы
	// по объекту. Администратор правит её в том же редакторе формы.
	//
	// Блоков по-прежнему нет: у листка нет вёрстки от администратора — только
	// макет .docx с меткой `{{сравнение}}`. Пустой список блоков честно говорит,
	// что конфигуратор блоков здесь ни на что не влияет.
	return OfferForm{
		Mode:  combatFormMode,
		Title: "Боевой листок",
		Placeholders: []Placeholder{
			{ID: combatHintField, Label: "Уточнение для разбора"},
		},
		Blocks:          []Block{},
		RequiredFields:  []string{},
		Sections:        combatDefaultSections(),
		SectionsDefault: combatDefaultSections(),
		// У листка свой встроенный контракт (2026-11-19): контракт КП говорит
		// «Рисунок — ссылка на картинку из КП», а КП в листке нет.
		BasePromptDefault: combatAnswerContract,
	}
}

// withCombatSectionsDefault прикладывает к форме листка встроенную заготовку
// разделов и заполняет пустой список разделов (2026-11-18).
//
// Два поля, две разные беды:
//
//  1. `sectionsDefault` — чтобы кнопке «↩ Вернуть заготовку» было что вернуть.
//     Отдельным полем, а не «пусто = заготовка»: редактор должен видеть разницу
//     между «у меня разделы вот такие» и «верни как в коде».
//  2. `sections` — пустой список у листка значит ЗАГОТОВКУ, а не пустоту:
//     `renderCombatSections` при пустом списке подставляет `combatDefaultSections`,
//     то есть документ печатает разделы всегда. Пока чтение отдавало пустоту,
//     окно настроек показывало «разделов нет», а собранный листок печатал четыре
//     раздела — расхождение «в окне одно, в документе другое» (ровно этот класс
//     и стоил 0.10.2). На стенде такой листок и нашёлся: сохранённая форма с
//     пустыми разделами печатала заготовку.
//
// Правило «пустые массивы в БД — это настроенная пустая форма» (0.10.3) здесь
// сознательно не действует: для разделов листка пустой список НЕ значит
// «печатать без разделов» — такого состояния у листка нет вовсе.
//
// Клиент присылает форму целиком, поэтому своё значение этих полей не
// принимается: `handleSaveForm` обнуляет заготовку сразу после разбора тела.
func withCombatSectionsDefault(form OfferForm) OfferForm {
	if form.Mode != combatFormMode {
		return form
	}
	if len(form.SectionsDefault) == 0 {
		form.SectionsDefault = combatDefaultSections()
	}
	if len(form.Sections) == 0 {
		form.Sections = combatDefaultSections()
	}
	return form
}

// combatDefaultSections — заготовка разделов боевого листка (2026-11-16).
//
// Это НЕ окончательная форма: администратор правит разделы в редакторе. Но
// листок без разделов печатал бы только карточки (запасной вид), а менеджеру
// нужен разбор — поэтому заготовка задаёт пять подразделов, покрывающие то, ради
// чего листок заводят:
//
//  1. общее сравнение — где мы сильнее и почему, только по карточкам;
//  2. сравнение характеристик — ТАБЛИЦА (контракт разрешает `table`), потому
//     что «повествование вместо таблицы» в сравнении читать неудобно;
//  3. схема сравнения — mermaid-ДИАГРАММА (шаг 5 плана 2026-11-16 дал вид
//     блока `diagram`): наглядная картинка «кто с кем и по каким осям», ради
//     которой пользователь и просил «схемы, если нужно для наглядности»;
//  4. где конкурент сильнее — честная строка, а не пустота (правило §9.4:
//     ключевой раздел не может молчать);
//  5. по каждому продукту — повтор по обеим сторонам (`repeat: systems`).
//
// Инструкции — рабочим русским и с запретом выдумывать: данные модели приходят
// только из карточек, и заготовка это повторяет (промт КП говорит то же, но
// администратор часто правит тексты инструкций, а не промт).
func combatDefaultSections() []ContentSection {
	return []ContentSection{
		{
			ID:       "general",
			Title:    "Общее сравнение",
			Numbered: true,
			Subsections: []ContentSubsection{
				{
					ID:          "overview",
					Title:       "Коротко",
					Instruction: "Сравни наш продукт с продуктами конкурентов кратко: в чём мы сильнее и за счёт чего. Опирайся только на карточки продуктов из данных, ничего не придумывай. Не более 150 слов.",
					Condition:   conditionAlways,
					MaxWords:    150,
				},
				{
					ID:          "specs",
					Title:       "Сравнение характеристик",
					Instruction: "Сравни технические характеристики нашего продукта и конкурентов ТАБЛИЦЕЙ: строки — характеристики, колонки — продукты. Бери только те характеристики, что есть в карточках; если у продукта значения нет, пиши «не указано». Не выдумывай цифры.",
					Condition:   conditionAlways,
					MaxWords:    300,
				},
			},
		},
		{
			ID:       "scheme",
			Title:    "Схема сравнения",
			Numbered: true,
			Subsections: []ContentSubsection{
				{
					ID:          "diagram",
					Title:       "Схема",
					Instruction: "Нарисуй КОМПАКТНУЮ вертикальную СХЕМУ (диаграмму mermaid, flowchart TD), которая наглядно показывает ГЛАВНОЕ отличие нашего продукта от конкурентов. Жёсткие рамки: не больше 5–6 узлов; у каждого узла короткая подпись (2–4 слова, не перечисляй в узле все характеристики и все продукты); не выстраивай по одному узлу на каждого конкурента — объединяй конкурентов в одну-две группы. Схема должна читаться сверху вниз, а не растягиваться вширь: если продуктов много, схема показывает оси сравнения и место нашего продукта на них, а не список всех. Блок kind = diagram, lang = mermaid, code — код mermaid. Подпись — короткая, без слова «Схема»: номер подписи поставит сервер. Только то, что есть в карточках.",
					Condition:   conditionAlways,
					MaxWords:    120,
				},
			},
		},
		{
			ID:       "weakness",
			Title:    "Где конкурент сильнее — и что отвечать",
			Numbered: true,
			Subsections: []ContentSubsection{
				{
					ID:          "risks",
					Title:       "Слабые места и ответ",
					Instruction: "Назови, в чём конкурент объективно сильнее нашего продукта по данным карточек, и предложи короткий ответ менеджеру: что сказать заказчику. Если по карточкам конкурент ни в чём не сильнее — так и напиши одной строкой, не выдумывай слабости.",
					Condition:   conditionAlways,
					MaxWords:    250,
				},
			},
		},
		{
			ID:       "per-product",
			Title:    "По каждому продукту",
			Numbered: true,
			Subsections: []ContentSubsection{
				{
					ID:          "about",
					Title:       "О продукте",
					Instruction: "Расскажи коротко о продукте, о котором идёт речь в этом вызове: что это, для чего применяется, в чём ключевое отличие. Только факты из его карточки.",
					Condition:   conditionAlways,
					Repeat:      blockRepeatSystems,
					MaxWords:    160,
				},
			},
		},
	}
}

func (s *Server) seedForms(ctx context.Context) error {
	// Форму КП сеем пустой строкой: она «настроена», админ правит её в
	// конфигураторе, а до первой правки `loadForm` отдаёт форму по умолчанию
	// только когда строки нет. Для боевого листка строку НЕ сеем намеренно:
	// пока админ не сохранил свою форму, `loadForm` отдаёт `combatDefaultForm`,
	// а пустая "настроенная" строка показала бы пустую форму вместо заготовки.
	_, err := s.pool.Exec(ctx,
		`INSERT INTO offers_form (mode) VALUES ('client') ON CONFLICT (mode) DO NOTHING`)
	return err
}

func (s *Server) loadForm(ctx context.Context, mode string) (OfferForm, error) {
	if strings.TrimSpace(mode) == "" {
		mode = clientFormMode
	}
	var form OfferForm
	var placeholders, blocks, required, sections, window []byte
	var layoutDocx []byte
	err := s.pool.QueryRow(ctx, `
SELECT id, mode, title, placeholders, blocks, required_fields, sections, window_config, base_prompt,
	to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'), layout_docx, layout_name
FROM offers_form WHERE mode = $1`, mode).
		Scan(&form.ID, &form.Mode, &form.Title, &placeholders, &blocks, &required, &sections, &window, &form.BasePrompt,
			&form.UpdatedAt, &layoutDocx, &form.LayoutName)
	if err != nil {
		// Строки нет вовсе — форма не настроена, показываем поля по умолчанию.
		// Пустые массивы в БД — это НАСТРОЕННАЯ пустая форма, и подменять её
		// нельзя: раньше проверка стояла до разбора JSON и сравнивала nil-слайс,
		// поэтому сохранённая форма не доходила ни до редактора, ни до DOCX.
		return withCombatSectionsDefault(defaultForm(mode)), nil
	}
	form.Placeholders = []Placeholder{}
	form.Blocks = []Block{}
	form.RequiredFields = []string{}
	form.Sections = []ContentSection{}
	if len(placeholders) > 0 {
		if err := json.Unmarshal(placeholders, &form.Placeholders); err != nil {
			return form, err
		}
	}
	if len(blocks) > 0 {
		if err := json.Unmarshal(blocks, &form.Blocks); err != nil {
			return form, err
		}
	}
	if len(required) > 0 {
		if err := json.Unmarshal(required, &form.RequiredFields); err != nil {
			return form, err
		}
	}
	if len(sections) > 0 {
		if err := json.Unmarshal(sections, &form.Sections); err != nil {
			return form, err
		}
	}
	// Пустые правила — законное состояние: выводим умолчание, а не пустой
	// срез. Иначе редактор показал бы «ни одной группы», и первое же
	// сохранение формы стёрло бы прежнее поведение печати ссылок.
	form.HasLayout = len(layoutDocx) > 0
	form.LayoutFields, form.LayoutPrompts = describeLayout(layoutDocx)
	// Окно КП (2026-11-15): пусто — «как сейчас», иначе сохранённый порядок
	// плюс недостающее в конец невидимым. Редактор плагина получает ту же
	// форму, что увидит приложение, — иначе он показал бы одно, а экран другое.
	form.Window = make([]WindowItem, 0, len(window))
	if len(window) > 0 {
		if err := json.Unmarshal(window, &form.Window); err != nil {
			return form, err
		}
	}
	// Пустое окно в базе — настройки нет, и клиенту это надо знать: зоны берутся
	// по умолчанию, а флаг говорит, что показ брифа никто не выбирал (2026-11-15).
	form.WindowStored = len(form.Window) > 0
	form.Window = effectiveWindow(form)
	// Базовый промт (2026-11-17): своё значение администратора или встроенное.
	// Встроенный текст отдаётся редактору отдельным полем: по нему кнопка
	// «вернуть базовый текст» заполняет поле, не заставляя искать код.
	form.BasePrompt = strings.TrimSpace(form.BasePrompt)
	form.BasePromptDefault = sectionAnswerContract
	// Заготовка разделов — только у листка (2026-11-18): у КП её нет, разделы
	// задаёт администратор.
	return withCombatSectionsDefault(form), nil
}

// describeLayout — какие метки есть в макете, для показа администратору.
//
// Разбор терпимый: испорченный макет не должен ломать загрузку формы — иначе
// один неудачный файл скрыл бы от человека весь конфигуратор.
// describeLayout — метки, которые система нашла в макете.
//
// Пустой срез, а не nil: `encoding/json` пишет nil-срез как `null`, а клиент
// читает `fields.length` — и падает «Cannot read properties of null» на первом
// же макете без полей или с текстом вместо меток (2026-11-11). Правило общее:
// пустой набор в JSON отдаём массивом.
func describeLayout(layout []byte) (fields, prompts []string) {
	if len(layout) == 0 {
		return []string{}, []string{}
	}
	files, _, err := readZipEntries(layout)
	if err != nil {
		return []string{}, []string{}
	}
	doc := string(files["word/document.xml"])
	seenField := map[string]bool{}
	seenPrompt := map[string]bool{}
	for _, token := range scanLayoutTokens(layoutPlainText(doc), nil) {
		if token.Kind == layoutTokenPrompt {
			if !seenPrompt[token.Inner] {
				seenPrompt[token.Inner] = true
				prompts = append(prompts, token.Inner)
			}
			continue
		}
		// `{{сравнение}}` — служебная метка боевого листка (2026-11-11). В списке
		// «полей» она была бы неправдой: полем её никто не заполняет. Называем её
		// отдельный списком промтов, чтобы плагин видел, что метка в макете есть.
		if token.Kind == layoutTokenComparison {
			if !seenPrompt[token.Inner] {
				seenPrompt[token.Inner] = true
				prompts = append(prompts, token.Inner)
			}
			continue
		}
		// Прочие служебные метки (`{{система}}` и т.п.) — не поля формы:
		// значением их не подставить, и в списке «Подставляемые поля» они
		// читались бы как незаполненные. Показываем их вместе с промтами.
		if isServiceKind(token.Kind) {
			continue
		}
		if !seenField[token.Inner] {
			seenField[token.Inner] = true
			fields = append(fields, token.Inner)
		}
	}
	return fields, prompts
}

// layoutPlainText — весь текст документа одной строкой: метки в макете
// считаются по нему, а не по отдельным абзацам, чтобы «что система увидела»
// совпадало с тем, что она будет подставлять.
//
// Абзацы склеиваются ВПЛОТНУЮ (`""`, а не `"\n"`): маркер конца строки —
// служебный символ, а не текст документа, и перенос, вставленный между
// абзацами, разорвал бы промт, набранный в двух соседних абзацах. Поля
// макета абзац не пересекают — это и ограничивает склейку безвредной.
func layoutPlainText(docXML string) string {
	var sb strings.Builder
	for _, m := range reWordText.FindAllStringSubmatch(docXML, -1) {
		sb.WriteString(unescapeXML(m[1]))
	}
	return sb.String()
}

func (s *Server) handleGetForm(w http.ResponseWriter, r *http.Request) {
	form, err := s.loadForm(r.Context(), r.URL.Query().Get("mode"))
	if err != nil {
		dbError(w, "get form", err)
		return
	}
	writeJSON(w, http.StatusOK, form)
}

func (s *Server) handleSaveForm(w http.ResponseWriter, r *http.Request) {
	var form OfferForm
	if err := decodeJSON(w, r, &form, 4<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	// Заготовка разделов — с сервера, а не от клиента (2026-11-18): клиент
	// присылает форму целиком, и подменённое поле вернулось бы ему как
	// «заготовка», которую он же и прислал.
	form.SectionsDefault = nil
	if strings.TrimSpace(form.Mode) == "" {
		form.Mode = clientFormMode
	}
	if form.Blocks == nil {
		form.Blocks = []Block{}
	}
	if form.Placeholders == nil {
		form.Placeholders = []Placeholder{}
	}
	if form.RequiredFields == nil {
		form.RequiredFields = []string{}
	}
	if form.Sections == nil {
		form.Sections = []ContentSection{}
	}
	if form.Window == nil {
		form.Window = []WindowItem{}
	}
	if msg := validateForm(form); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
		return
	}

	placeholders, err := json.Marshal(form.Placeholders)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}
	blocks, err := json.Marshal(form.Blocks)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}
	required, err := json.Marshal(form.RequiredFields)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}
	// Содержательные разделы (2026-11-14, дефект 0.10.2). Маршалится ВСЁ, что
	// клиент прислал: раньше `sections` молча выпадало, и раздел исчезал после
	// «Сохранить форму» — настройка терялась без ошибки (класс 0.1.18).
	sections, err := json.Marshal(form.Sections)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}
	// Окно КП в приложении (2026-11-15). Маршалится ВСЁ, что клиент прислал:
	// молча не записать настройку — ровно та потеря, которую поймал сторож
	// `sections` (класс 0.1.18). Пусто — пишем `[]`: «настройки нет» значит
	// умолчание, и оно должно пережить сохранение формы.
	window := form.Window
	if window == nil {
		window = []WindowItem{}
	}
	// Защита от старого клиента (0.11.2, после совещания 2026-11-16): плагин
	// 0.11.0 не знает зоны `brief` и при сохранении присылал окно без неё, а
	// `window_config = EXCLUDED.window_config` затирал включённую зону — и
	// следующее чтение снова дописывало её НЕВИДИМОЙ. Настройка молча
	// откатывалась (класс 0.1.18). Здесь: если в базе зона ВКЛЮЧЕНА, а клиент
	// её не прислал вовсе, сохраняем состояние из базы, а не теряем его.
	window = keepKnownZonesFromDB(r.Context(), s, form.Mode, window)
	// 0.11.3: бриф больше не зона окна. Старый плагин (0.11.1/0.11.2) может
	// прислать `zone:brief` — не сохраняем её и не отказываем в записи: отказ
	// заблокировал бы сохранение формы человеку, который ничего не сделал не так.
	window = dropLegacyZones(window)
	windowJSON, err := json.Marshal(window)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}

	_, err = s.pool.Exec(r.Context(), `
INSERT INTO offers_form (mode, title, placeholders, blocks, required_fields, sections, window_config, base_prompt, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
ON CONFLICT (mode) DO UPDATE SET
	title = EXCLUDED.title, placeholders = EXCLUDED.placeholders,
	blocks = EXCLUDED.blocks, required_fields = EXCLUDED.required_fields,
	sections = EXCLUDED.sections,
	window_config = EXCLUDED.window_config,
	base_prompt = EXCLUDED.base_prompt,
	updated_at = now()`,
		form.Mode, strings.TrimSpace(form.Title), placeholders, blocks, required, sections, windowJSON,
		strings.TrimSpace(form.BasePrompt))
	if err != nil {
		dbError(w, "save form", err)
		return
	}
	// Макет в JSON формы не приходит и здесь не трогается: его загружают и
	// удаляют отдельными ручками. Сохранение формы не должно его стирать —
	// человек правит блоки чаще, чем макет, и терять загруженный файл при
	// каждом «Сохранить» нельзя.
	var layoutDocx []byte
	var layoutName string
	_ = s.pool.QueryRow(r.Context(), `SELECT layout_docx, layout_name FROM offers_form WHERE mode = $1`,
		form.Mode).Scan(&layoutDocx, &layoutName)
	form.HasLayout = len(layoutDocx) > 0
	form.LayoutName = layoutName
	form.LayoutFields, form.LayoutPrompts = describeLayout(layoutDocx)
	// Отвечаем тем, что реально записали, а НЕ перечитанным значением: пустая
	// форма из БД трактуется как «не настроена» и подменяется формой по
	// умолчанию — админ сохранил бы новую форму и получил ответ со старой.
	saved := form
	saved.ID = 0
	var savedID int64
	var updatedAt string
	if err := s.pool.QueryRow(r.Context(),
		`SELECT id, to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM offers_form WHERE mode = $1`,
		form.Mode).Scan(&savedID, &updatedAt); err != nil {
		dbError(w, "reload form", err)
		return
	}
	saved.ID = savedID
	saved.UpdatedAt = updatedAt
	// Заготовку прикладываем к ответу: экран настроек продолжает работать после
	// сохранения, и кнопка «↩ Вернуть заготовку» не должна пропадать до
	// перезагрузки окна.
	writeJSON(w, http.StatusOK, withCombatSectionsDefault(saved))
}
