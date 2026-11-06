package main

import (
	"regexp"
	"strings"
)

// Макет КП (.docx) с плейсхолдерами (2026-11-05).
//
// Пользователь загружает свой документ Word и расставляет в нём два вида
// меток:
//
//	[Наименование объекта]  — ЖЁСТКАЯ подстановка: значение поля формы или
//	                          служебное значение предложения. Пишется как
//	                          есть, ничего не сочиняется.
//	{Опиши систему в двух    — ПРОМТ для модели: текст пишет модель по данным
//	 абзацах}                 конкретного КП, как узел `prompt_text`.
//
// Макет — образец вёрстки: в самом документе остаются его стили, шрифты,
// колонтитулы, таблицы и картинки. Меняется только текст внутри меток.
//
// Почему квадратные и фигурные скобки, а не `{{…}}`: пользователь так и
// сформулировал. `{{…}}` в макете тоже понимается — это имя поля формы
// (`{{object_name}}`) для совместимости с уже существующими формами.

// Виды меток макета.
const (
	layoutTokenField  = "field"  // [имя] или {{имя}} — жёсткая подстановка
	layoutTokenPrompt = "prompt" // {текст} — промт модели
)

// layoutToken — метка вместе с её местом в тексте.
type layoutToken struct {
	Kind  string
	Raw   string // как написано в макете, вместе со скобками
	Inner string // что внутри скобок
	Start int
	End   int
}

var (
	// reLayoutField — квадратные скобки: `[Наименование объекта]`.
	//
	// Внутри не должно быть квадратных скобок и переводов строк: цель ссылки
	// Markdown `[текст](адрес)` в макете тоже квадратная, и без ограничения
	// в метку попала бы половина ссылки. Перевод строки ограничивает метку
	// абзацем: плейсхолдер в макете — всегда одна строка.
	reLayoutField = regexp.MustCompile(`\[([^\[\]\n]{1,120})\]`)
	// reLayoutPrompt — фигурные скобки: `{Опиши систему}`.
	//
	// Скобка не одиночная: `{{object_name}}` — это имя поля формы, и она
	// должна остаться меткой поля, а не превратиться в промт «object_name».
	reLayoutPrompt = regexp.MustCompile(`\{([^{}\n]{1,400})\}`)
	// reLayoutBraceField — `{{object_name}}`: имя поля формы, как в блоках.
	reLayoutBraceField = regexp.MustCompile(`\{\{([^{}\n]{1,80})\}\}`)
)

// scanLayoutTokens находит все метки макета в порядке появления.
//
// Возвращает метки ПО ВОЗРАСТАНИЮ Start; пересечений нет — `{{…}}` разбирается
// первым и «съедает» свои скобки, чтобы `{object_name}` внутри не стало
// промтом.
func scanLayoutTokens(text string) []layoutToken {
	var tokens []layoutToken
	masked := text
	for _, m := range reLayoutBraceField.FindAllStringSubmatchIndex(text, -1) {
		tokens = append(tokens, layoutToken{
			Kind: layoutTokenField, Raw: text[m[0]:m[1]],
			Inner: strings.TrimSpace(text[m[2]:m[3]]), Start: m[0], End: m[1],
		})
		// Затираем найденное одинаковым по длине мусором: индексы остальных
		// совпадений не сдвинутся, а фигурная скобка внутри уже не найдётся.
		masked = replaceRange(masked, m[0], m[1])
	}
	for _, m := range reLayoutField.FindAllStringSubmatchIndex(masked, -1) {
		tokens = append(tokens, layoutToken{
			Kind: layoutTokenField, Raw: text[m[0]:m[1]],
			Inner: strings.TrimSpace(text[m[2]:m[3]]), Start: m[0], End: m[1],
		})
	}
	for _, m := range reLayoutPrompt.FindAllStringSubmatchIndex(masked, -1) {
		tokens = append(tokens, layoutToken{
			Kind: layoutTokenPrompt, Raw: text[m[0]:m[1]],
			Inner: strings.TrimSpace(text[m[2]:m[3]]), Start: m[0], End: m[1],
		})
	}
	sortLayoutTokens(tokens)
	return tokens
}

// replaceRange заполняет диапазон символами-заглушками.
func replaceRange(s string, start, end int) string {
	if start < 0 || end > len(s) || start >= end {
		return s
	}
	return s[:start] + strings.Repeat("\x00", end-start) + s[end:]
}

// sortLayoutTokens — порядок по месту в тексте (сортировка вставками: меток в
// абзаце единицы, и лишний пакет ради неё не нужен).
func sortLayoutTokens(t []layoutToken) {
	for i := 1; i < len(t); i++ {
		for j := i; j > 0 && t[j-1].Start > t[j].Start; j-- {
			t[j-1], t[j] = t[j], t[j-1]
		}
	}
}

// layoutFieldValue — значение жёсткой подстановки.
//
// Имя в макете пишется по-человечески («Наименование объекта»), поэтому
// сравнивается без учёта регистра, «ё/е» и пробелов. Неизвестное имя
// подменяется пустой строкой, как и `{{…}}` в блоках: служебные скобки в
// готовом документе недопустимы.
func layoutFieldValue(inner string, values map[string]string) string {
	if v := layoutAliasValue(inner, values); v != "" {
		return v
	}
	if alias, ok := builtinFieldAliases[normalizeFieldKey(inner)]; ok {
		// Служебное поле может быть заполнено из формы под другим ключом:
		// объект в форме КП — `object_name`, а служебное — `offer_object`.
		// Оба описывают одно и то же, и держать это в двух местах — забыть
		// об одном.
		if v := firstNonEmpty(values, alias, fallbackAlias[alias]); v != "" {
			return v
		}
	}
	return ""
}

// fallbackAlias — второй ключ для служебного поля.
var fallbackAlias = map[string]string{
	fieldObjectName: "object_name",
	fieldClientName: "client_name",
}

// layoutAliasValue — значение по человеческому имени или идентификатору поля.
func layoutAliasValue(inner string, values map[string]string) string {
	key := normalizeFieldKey(inner)
	// Сначала точное имя поля формы (как в `{{…}}`), потом человеческое.
	if v, ok := values[strings.TrimSpace(inner)]; ok {
		return v
	}
	for id, label := range fieldAliases {
		if normalizeFieldKey(label) == key {
			return firstNonEmpty(values, id)
		}
	}
	// Ещё одна попытка: имя поля без префикса `brief.`. Бриф кладёт туда же.
	if v, ok := values["brief."+strings.TrimSpace(inner)]; ok {
		return v
	}
	return ""
}

// firstNonEmpty — первое непустое значение из ключей.
func firstNonEmpty(values map[string]string, keys ...string) string {
	for _, k := range keys {
		if k == "" {
			continue
		}
		if v := strings.TrimSpace(values[k]); v != "" {
			return v
		}
	}
	return ""
}

// normalizeFieldKey — «Наименование объекта» → «наименованиеобъекта».
func normalizeFieldKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "ё", "е")
	return strings.Join(strings.Fields(s), "")
}

// fieldAliases — как поле называется в макете по-человечески.
//
// Пользователь пишет в документе `[Наименование объекта]`, а не
// `[object_name]`: макет — документ для людей, и набирать служебные имена в
// Word никто не станет. Идентификаторы совпадают с полями формы, чтобы одно и
// то же значение можно было взять и `[]`, и `{{…}}`.
var fieldAliases = map[string]string{
	// Служебные — есть у каждого предложения.
	fieldOfferNumber: "Номер предложения",
	fieldOfferDate:   "Дата предложения",
	fieldManager:     "Имя менеджера",
	fieldClientName:  "Получатель из КП",
	fieldObjectName:  "Объект из формы КП",
	"system_title":   "Название системы",
	"systems_list":   "Выбранные системы",

	// Поля формы по умолчанию.
	"supplier_name":     "Поставщик",
	"supplier_contacts": "Контакты поставщика",
	"client_name":       "Получатель (организация)",
	"client_contact":    "Контактное лицо получателя",
	"object_name":       "Объект",
	"object_address":    "Адрес объекта",
	"valid_until":       "Предложение действительно до",
	"conditions":        "Условия",
}

// builtinFieldAliases — дополнительные имена для служебных полей: в макете
// человек пишет «Название объекта» и «Название системы», а не «Объект».
var builtinFieldAliases = map[string]string{
	"названиеобъекта":     fieldObjectName,
	"наименованиеобъекта": fieldObjectName,
	"объект":              fieldObjectName,
	"названиесистемы":     "system_title",
	"система":             "system_title",
	"менеджер":            fieldManager,
	"номер":               fieldOfferNumber,
	"дата":                fieldOfferDate,
	"получатель":          fieldClientName,
	"поставщик":           "supplier_name",
}
