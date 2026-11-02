package skills

import (
	"encoding/json"
	"fmt"
	"strings"

	"mailers/offers/model"
)

// Контекст объекта из свободного текста — через модель (2026-11-01).
//
// Дословный разбор (filecontext_facts.go) берёт «Объект: …» и «Площадь: …», но
// живой КП написан прозой: «Предлагаем Вам, для объекта: … , использовать
// материалы … по специальным ценам», плюс описание и преимущества системы
// абзацами. Из этого списком ничего не достать — понимать приходится смысл.
//
// Правила те же, что у всего проекта: модель не выдумывает, чего нет в файле.
// Пустое остаётся пустым, а нехватка помечается, если она важна для текста.

// fileContextRules — что модель делает с присланным файлом.
const fileContextRules = `ТЫ ЧИТАЕШЬ ДОКУМЕНТ, КОТОРЫЙ МЕНЕДЖЕР ПРИСЛАЛ ПО ОБЪЕКТУ.

Документ может быть чем угодно: старым коммерческим предложением, техническим
описанием, письмом заказчика, спецификацией. Твоя задача — вытащить из него
КОНТЕКСТ ОБЪЕКТА для нового коммерческого предложения.

ЧЕГО НЕ ДЕЛАТЬ:
- не придумывать. Чего в документе нет — оставь пустым;
- не пересказывать состав и цены: позиции приходят отдельной таблицей;
- не переписывать текст документа — только разложи по полям.

ЧТО ИЗВЛЕКАТЬ:
- object_name: полное наименование объекта, как в документе. Адрес, кадастровый
  номер — часть наименования, если они там есть;
- object_purpose: тип объекта по назначению (жилой дом, паркинг, школа, склад).
  Только если назван или прямо следует из наименования;
- object_area: площадь или объём работ с единицей («9 200 м2», «горизонталь
  4 800 м2, вертикаль 1 400 м2»);
- customer_address: кому адресован документ — организация, город. Часто это
  адресат, а не заказчик;
- planned_systems: названия систем, которые в документе предлагаются, по одной
  в элементе списка. Коротко, как называют сами: «ТН-ФУНДАМЕНТ Дренаж Проф»;
- customer_goals: зачем объекту эта система, если сказано;
- project_risks: риски и особые условия монтажа, если названы (зимние работы,
  грунтовые воды, стеснённые условия);
- notes: всё остальное полезное для текста — описание системы, преимущества,
  особые требования. Один абзац на мысль.

ОТВЕТ ФОРМАТОМ JSON, без маркеров кода и текста вокруг:
{"object_name": "", "object_purpose": "", "object_area": "",
 "customer_address": "", "planned_systems": [], "customer_goals": [],
 "project_risks": [], "notes": ""}
`

// contextAnswer — ответ модели на разбор файла.
type contextAnswer struct {
	ObjectName      string   `json:"object_name"`
	ObjectPurpose   string   `json:"object_purpose"`
	ObjectArea      string   `json:"object_area"`
	CustomerAddress string   `json:"customer_address"`
	PlannedSystems  []string `json:"planned_systems"`
	CustomerGoals   []string `json:"customer_goals"`
	ProjectRisks    []string `json:"project_risks"`
	Notes           string   `json:"notes"`
}

// RenderFileContextSystem — системный промпт разбора файла.
func RenderFileContextSystem() string {
	return fileContextRules + "\n" + noInventDoc
}

// RenderFileContextUser — сам текст файла и просьба вернуть JSON.
func RenderFileContextUser(lines []string) string {
	text := joinContextText(lines)
	return "ДОКУМЕНТ:\n" + text + "\n\nВерни JSON с контекстом объекта."
}

// ParseFileContextAnswer разбирает ответ модели терпимо: лишний текст и
// маркеры кода вокруг JSON переживаем (jsonrepair общий для проекта).
func ParseFileContextAnswer(raw string) (FileContext, error) {
	value, _, err := model.ExtractJSON(raw)
	if err != nil {
		return emptyFileContext(), err
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return emptyFileContext(), fmt.Errorf("ожидали объект, получили %T", value)
	}
	rawJSON, err := json.Marshal(obj)
	if err != nil {
		return emptyFileContext(), err
	}
	var answer contextAnswer
	if err := json.Unmarshal(rawJSON, &answer); err != nil {
		return emptyFileContext(), err
	}
	return fileContextFromAnswer(answer), nil
}

// fileContextFromAnswer переводит ответ модели в поля брифа без догадок:
// пустая строка не превращается в «не знаю», а просто не записывается.
func fileContextFromAnswer(a contextAnswer) FileContext {
	out := emptyFileContext()
	out.put("object_name", a.ObjectName)
	out.put("object_purpose", a.ObjectPurpose)
	out.put("object_area", a.ObjectArea)
	out.put("customer_address", a.CustomerAddress)
	for _, v := range a.PlannedSystems {
		out.put("planned_systems", v)
	}
	for _, v := range a.CustomerGoals {
		out.put("customer_goals", v)
	}
	for _, v := range a.ProjectRisks {
		out.put("project_risks", v)
	}
	out.Notes = strings.TrimSpace(a.Notes)
	out.Summary = fileContextSummary(out)
	if model.NonEmpty(out.Notes) {
		out.Summary = append(out.Summary, "описание и преимущества — в заметках")
	}
	return out
}

// MergeFileContexts накладывает модельный разбор на дословный.
//
// Дословный главнее: он берёт текст как написан, а модель иногда «уточняет»
// («жилой комплекс» вместо «многоквартирный дом»). Модель дополняет тем, чего
// дословный не нашёл вовсе. Заметки объединяются.
func MergeFileContexts(facts, fromModel FileContext) FileContext {
	out := emptyFileContext()
	for key, values := range fromModel.Fields {
		for _, v := range values {
			out.put(key, v)
		}
	}
	for key, values := range facts.Fields {
		// Дословное значение заменяет модельное по этому ключу целиком:
		// смешивать две формулировки одного поля — хуже, чем выбрать одну.
		out.Fields[key] = append([]string{}, values...)
	}
	out.Notes = joinNotes(facts.Notes, fromModel.Notes)
	out.Summary = fileContextSummary(out)
	if model.NonEmpty(out.Notes) {
		out.Summary = append(out.Summary, "описание и преимущества — в заметках")
	}
	return out
}

// joinNotes склеивает заметки без повторов строк.
func joinNotes(parts ...string) string {
	var out []string
	for _, part := range parts {
		for _, line := range strings.Split(part, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || ContainsFold(out, line) {
				continue
			}
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
