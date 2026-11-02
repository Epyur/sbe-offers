package skills

import (
	"strings"

	"mailers/offers/model"
)

// Контекст объекта из присланного файла (2026-11-01).
//
// Живой случай: менеджер прислал старый КП в Excel и написал «информация в
// файле». Бот разобрал таблицу как текст, отдал его в поле `context` — и оно
// никуда не пошло: бриф, секции и форма о файле не знали. Менеджеру пришлось
// перепечатать объект и площадь в чат, хотя всё это лежало в присланном файле.
//
// Здесь два пути:
//   - дословный (`extractBriefFacts`) — подписи «Объект:», «Площадь:» и
//     названия систем из самого файла. Работает без модели и никогда не врёт;
//   - модельный (промпт в prompt.go) — свободный текст КП: «Предлагаем Вам, для
//     объекта: …». Модель заполняет те же ключи брифа, бот показывает «что
//     понял» и спрашивает подтверждение.
//
// Оба пишут в ОДНИ ключи (model.BriefPayload), поэтому дальше разницы нет.

// FileContext — что удалось взять из файла для брифа.
type FileContext struct {
	// Fields — поля брифа по ключам model.BriefPayload: object_name,
	// object_purpose, object_area, planned_systems, customer_goals, …
	Fields map[string][]string `json:"fields"`
	// Summary — «что понял» человеческим языком, по пунктам.
	Summary []string `json:"summary"`
	// Notes — то, что не разложилось: описание и преимущества системы.
	Notes string `json:"notes,omitempty"`
}

// HasContent — есть ли что применять к брифу.
func (c FileContext) HasContent() bool {
	return len(c.Fields) > 0 || model.NonEmpty(c.Notes)
}

// emptyFileContext — пустой результат с готовой картой: вызывающий код не
// должен проверять nil перед записью.
func emptyFileContext() FileContext {
	return FileContext{Fields: map[string][]string{}}
}

// put добавляет значение к ключу, если его там ещё нет.
func (c FileContext) put(key, value string) {
	value = strings.TrimSpace(value)
	if value == "" || ContainsFold(c.Fields[key], value) {
		return
	}
	c.Fields[key] = append(c.Fields[key], value)
}

// ContainsFold - есть ли строка в списке без учёта регистра.
func ContainsFold(list []string, value string) bool {
	for _, v := range list {
		if strings.EqualFold(v, value) {
			return true
		}
	}
	return false
}

// maxContextLines — предел строк файла, уходящих в модель. Старый КП в
// Excel — это 130 строк с повторами; длиннее — уже не «описание объекта».
const maxContextLines = 200

// joinContextText склеивает строки файла в один текст для модели.
func joinContextText(lines []string) string {
	if len(lines) > maxContextLines {
		lines = lines[:maxContextLines]
	}
	return strings.Join(lines, "\n")
}
