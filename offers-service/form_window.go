package main

// Окно КП в приложении: модель настройки (2026-11-15, шаг 1 плана 0.11.0).
//
// Что настраивает администратор: порядок, видимость и обязательность полей
// формы И зон экрана приложения. Настройка живёт в `offers_form.window_config`
// (JSONB, `[]` = «как сейчас»), как `sections` и `link_groups`.
//
// Почему зоны объявлены в коде, а не в данных: зону рисует код приложения, и
// зона без рисующего её кода — это «настройка, которая ничего не делает»
// (класс дефекта «молча не выполненный повтор блока», form.go).
//
// Пусто = прежнее поведение. Правило то же, что у `linkRules`: старая форма
// не меняет ни экран приложения, ни проверку обязательности.

import (
	"strings"
)

// Виды элементов окна.
const (
	windowKindField = "field"
	windowKindZone  = "zone"
)

// windowZoneClient — зона заказчика: единственная зона, которую можно сделать
// обязательной (у неё есть поля, которые человек заполняет).
const windowZoneClient = "client"

// knownWindowZones — фиксированный набор зон экрана КП в приложении.
//
// Порядок здесь — порядок по умолчанию (прежнее поведение экрана):
// Заказчик → Системы → Позиции → Материалы → Тексты разделов, затем бриф.
//
// `brief` (2026-11-15) — зона брифа: что бот выяснил про объект. До 0.11.2 её
// не было в умолчании, и это оказалось дефектом: у формы с сохранённым окном
// зона дописывалась 12-м элементом — ПОСЛЕ полей формы, — а у формы без окна
// не появлялась вовсе (defaultWindow строился по `legacyWindowZones`). Теперь
// зона есть везде, но её ПОРЯДОК по умолчанию — сразу за «Заказчиком»
// (`defaultZoneOrder`): карточка брифа описывает объект и должна стоять рядом
// с заказчиком, а не под таблицей полей. Видимость по-прежнему решает
// администратор: новая зона в старом окне дописывается невидимой.
var knownWindowZones = []string{
	windowZoneClient,
	"systems",
	"items",
	"materials",
	"sections",
}

// windowZoneBrief — ЗОНА брифа, выведенная из окна (0.11.3).
//
// 0.11.1–0.11.2 держали бриф отдельным блоком экрана, и это оказалось
// неверным: менеджер стоит в разделе («Преимущества»), видит слабый текст и
// хочет дополнить контекст, не уходя наверх. Пользователь попросил перенести
// вызов брифа в карточку каждого раздела (2026-11-16), и зона стала настройкой,
// которая ничего не делает, — то есть дефектом по правилу проекта.
//
// Константа оставлена: по ней вычищаются старые окна (`dropLegacyZones`), и по
// ней на чтении понимается, что элемент устарел, а не испорчен.
const windowZoneBrief = "brief"

// defaultZoneOrder — порядок зон в окне «как сейчас»: прежние пять, затем
// бриф. Отличается от `knownWindowZones` ровно тем, что бриф идёт последним:
// окно, которое администратор никогда не трогал, не должно менять вид
// (бриф там всё равно невидим), а в редакторе новая строка видна в конце.
var defaultZoneOrder = []string{
	windowZoneClient,
	"systems",
	"items",
	"materials",
	"sections",
}

// legacyWindowZones — набор зон ДО появления брифа. Остался для чтения
// старого состояния и для сторожа: в умолчание бриф теперь входит.
var legacyWindowZones = []string{
	windowZoneClient,
	"systems",
	"items",
	"materials",
	"sections",
}

// windowZoneTitles — подпись зоны для человека. Язык экрана один, поэтому
// подпись живёт в коде, а не в данных: две правды разошлись бы.
var windowZoneTitles = map[string]string{
	windowZoneClient: "Заказчик",
	"systems":        "Системы",
	"items":          "Позиции с ценами",
	"materials":      "Материалы (описанием)",
	"sections":       "Тексты разделов",
}

// WindowItem — один элемент окна КП в приложении (2026-11-15).
type WindowItem struct {
	// Kind: "field" — поле формы (ID = id плейсхолдера),
	//       "zone"  — целая зона экрана (ID из knownWindowZones).
	Kind string `json:"kind"`
	ID   string `json:"id"`
	// Label — подпись для поля. У зон подпись берётся из кода
	// (windowZoneTitles), и хранить её в данных значило бы держать две правды.
	Label    string `json:"label,omitempty"`
	Visible  bool   `json:"visible"`
	Required bool   `json:"required,omitempty"`
}

// knownWindowZone — зона известна серверу.
func knownWindowZone(id string) bool {
	for _, known := range knownWindowZones {
		if id == known {
			return true
		}
	}
	return false
}

// WindowTitle — подпись элемента для человека: у зоны — из кода, у поля — своя.
func (w WindowItem) WindowTitle() string {
	if w.Kind == windowKindZone {
		if title, ok := windowZoneTitles[w.ID]; ok {
			return title
		}
		return w.ID
	}
	return w.Label
}

// effectiveWindow — окно, как его увидит клиент (2026-11-15).
//
// Пустая настройка — это нынешнее поведение: все пять зон в прежнем порядке,
// затем каждое поле формы (порядок `placeholders`), видимое, с обязательностью
// из `required_fields`. Старая форма не меняет ни экран, ни проверку.
//
// Иначе — `normalizeWindow`: сохранённый порядок главный, неизвестное
// выкидывается, недостающее дописывается в конец НЕВИДИМЫМ (решение
// пользователя §2.4 спеки: форма, сохранённая до появления зоны, не покажет
// её случайно).
func effectiveWindow(form OfferForm) []WindowItem {
	// 0.11.3: зона брифа выведена из окна. Окно, сохранённое в 0.11.2, несёт
	// `zone:brief` — вычищаем на чтении, чтобы в редакторе не осталось строки,
	// которой в приложении соответствует пустота.
	cleaned := dropLegacyZones(form.Window)
	saved := make([]WindowItem, 0, len(cleaned))
	for _, item := range cleaned {
		item.Kind = strings.TrimSpace(item.Kind)
		item.ID = strings.TrimSpace(item.ID)
		if item.Kind != windowKindField && item.Kind != windowKindZone {
			continue
		}
		if item.ID == "" {
			continue
		}
		saved = append(saved, item)
	}
	if len(saved) == 0 {
		return defaultWindow(form)
	}
	return normalizeWindow(saved, form)
}

// defaultWindow — «как сейчас»: зоны в прежнем порядке, затем поля формы.
//
// С 0.11.2 бриф ВХОДИТ в умолчание — но в конце (`defaultZoneOrder`), и
// видимым он от этого не становится: у формы без настройки окно вообще не
// было настройкой, и показывать в нём новый раздел никто не просил. Зато он
// появляется в редакторе плагина: до 0.11.2 его не было НИГДЕ у формы без
// сохранённого окна, и включить зону было нечем (живая жалоба 2026-11-16).
func defaultWindow(form OfferForm) []WindowItem {
	required := make(map[string]bool, len(form.RequiredFields))
	for _, id := range form.RequiredFields {
		required[id] = true
	}
	items := make([]WindowItem, 0, len(defaultZoneOrder)+len(form.Placeholders))
	for _, zone := range defaultZoneOrder {
		items = append(items, WindowItem{Kind: windowKindZone, ID: zone, Visible: true})
	}
	// Бриф в умолчании невидим: зона известна, но раздел не появляется у всех
	// сам (решение 0.11.0 §2.4 сохраняется — меняется только место в списке).
	for i := range items {
		if items[i].Kind == windowKindZone && items[i].ID == windowZoneBrief {
			items[i].Visible = false
		}
	}
	for _, ph := range form.Placeholders {
		if strings.TrimSpace(ph.ID) == "" {
			continue
		}
		items = append(items, WindowItem{
			Kind:     windowKindField,
			ID:       ph.ID,
			Label:    ph.Label,
			Visible:  true,
			Required: required[ph.ID],
		})
	}
	return items
}

// appendMissingZones дописывает недостающие зоны невидимыми — в конец.
//
// До 0.11.3 у зоны брифа был якорь («сразу за Заказчиком»). С 0.11.3 бриф не
// зона, а вызов из карточки раздела, поэтому якорь ушёл вместе с зоной;
// функция оставлена как единая точка дописывания (её зовёт `normalizeWindow`).
func appendMissingZones(items []WindowItem, seen map[string]bool) []WindowItem {
	for _, zone := range knownWindowZones {
		key := windowKindZone + "\x00" + zone
		if seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, WindowItem{Kind: windowKindZone, ID: zone, Visible: false})
	}
	return items
}

// dropLegacyZones вычищает из окна зоны, которых больше нет в наборе.
//
// Нужна ровно для одной: `zone:brief`. Окно, сохранённое в 0.11.2 (где бриф
// был зоной), иначе осталось бы с элементом, который никто не рисует, — а это
// и есть «настройка, которая ничего не делает». Вычищаем и на чтении, и перед
// записью: старый плагин может прислать бриф обратно.
//
// Отдельная функция, а не общий «выкинуть всё неизвестное»: неизвестную зону
// надо показывать в редакторе как ошибку (`validateWindow`), а устаревшую —
// молча убирать, потому что она когда-то была законной.
func dropLegacyZones(items []WindowItem) []WindowItem {
	if len(items) == 0 {
		return items
	}
	out := make([]WindowItem, 0, len(items))
	for _, item := range items {
		if item.Kind == windowKindZone && item.ID == windowZoneBrief {
			continue
		}
		out = append(out, item)
	}
	return out
}

// indexOfZone — позиция зоны в списке окна; -1, если её там нет.
func indexOfZone(items []WindowItem, id string) int {
	for i, item := range items {
		if item.Kind == windowKindZone && item.ID == id {
			return i
		}
	}
	return -1
}

// normalizeWindow — единственное место, где сходятся «что записано» и «что
// существует» (2026-11-15).
//
// Правила:
//   - порядок записанного сохраняется (он и есть настройка администратора);
//   - неизвестный kind/id выкидывается: хранить настройку, которая ничего не
//     делает, нельзя;
//   - дубль пары (kind, id) — второй экземпляр выкидывается;
//   - подпись поля обновляется из `placeholders` — один источник правды;
//   - недостающие зоны и недостающие поля формы дописываются в КОНЕЦ
//     невидимыми, обязательность берётся из `required_fields`;
//   - `Required` снимается у зон, кроме `client`: обязательность зоны, которую
//     никто не проверяет, — ложное обещание.
func normalizeWindow(saved []WindowItem, form OfferForm) []WindowItem {
	labels := make(map[string]string, len(form.Placeholders))
	for _, ph := range form.Placeholders {
		if id := strings.TrimSpace(ph.ID); id != "" {
			labels[id] = ph.Label
		}
	}
	required := make(map[string]bool, len(form.RequiredFields))
	for _, id := range form.RequiredFields {
		required[id] = true
	}

	seen := make(map[string]bool, len(saved)+len(knownWindowZones)+len(form.Placeholders))
	items := make([]WindowItem, 0, len(saved)+len(knownWindowZones)+len(form.Placeholders))
	for _, item := range saved {
		key := item.Kind + "\x00" + item.ID
		if seen[key] {
			continue
		}
		switch item.Kind {
		case windowKindZone:
			if !knownWindowZone(item.ID) {
				continue
			}
			item.Label = ""
			if item.ID != windowZoneClient {
				item.Required = false
			}
		case windowKindField:
			label, ok := labels[item.ID]
			if !ok {
				continue
			}
			item.Label = label
		default:
			continue
		}
		// Скрытое не обязательное (решение пользователя 2026-11-15): «пока оно
		// скрыто, оно не обязательно». Сохранить такое `validateWindow` не
		// даст, но старые формы и прямые правки в базе придут с обеими
		// галочками — приводим к смыслу на чтении, а не верим данным.
		if !item.Visible {
			item.Required = false
		}
		seen[key] = true
		items = append(items, item)
	}

	items = appendMissingZones(items, seen)
	for _, ph := range form.Placeholders {
		id := strings.TrimSpace(ph.ID)
		if id == "" {
			continue
		}
		key := windowKindField + "\x00" + id
		if seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, WindowItem{
			Kind:     windowKindField,
			ID:       id,
			Label:    ph.Label,
			Visible:  false,
			Required: required[id],
		})
	}
	return items
}

// validateWindow — проверка настройки окна перед сохранением (2026-11-15).
//
// Пустое окно законно: это «как сейчас». Всё остальное обязано называться до
// первой сборки — настройка, которая молча не выполнится, стоит дороже ошибки.
//
// Скрытое обязательное отвергается (решение пользователя 2026-11-15,
// буквальная формулировка): «обязательное не может быть скрытым, если мы
// скрываем обязательное, то пока оно скрыто, оно не обязательно». Иначе
// галочка «обязателен» пропадала бы без единого слова, как только поле убрали
// с экрана, — тихая потеря настройки (класс 0.10.2).
func validateWindow(form OfferForm) string {
	if len(form.Window) == 0 {
		return ""
	}
	fields := make(map[string]string, len(form.Placeholders))
	for _, ph := range form.Placeholders {
		if id := strings.TrimSpace(ph.ID); id != "" {
			fields[id] = ph.Label
		}
	}
	// windowTitle — подпись элемента ДЛЯ ОТКАЗА: у зоны она из кода, у поля —
	// из формы. Собрать `WindowItem` без `Label` значило бы ответить человеку
	// «поле ""», и по такому отказу нельзя понять, какую строку править.
	windowTitle := func(kind, id string) string {
		if kind == windowKindField {
			if label := strings.TrimSpace(fields[id]); label != "" {
				return label
			}
		}
		return WindowItem{Kind: kind, ID: id}.WindowTitle()
	}
	seen := make(map[string]bool, len(form.Window))
	for _, item := range form.Window {
		kind := strings.TrimSpace(item.Kind)
		id := strings.TrimSpace(item.ID)
		switch kind {
		case windowKindField:
			if _, ok := fields[id]; !ok {
				return "в окне КП поле «" + id + "» не заведено в списке полей"
			}
		case windowKindZone:
			if !knownWindowZone(id) {
				return "в окне КП неизвестная зона: " + id
			}
			if item.Required && id != windowZoneClient {
				return "в окне КП обязательной может быть только зона «Заказчик», а не «" +
					windowTitle(kind, id) + "»"
			}
		case "":
			return "в окне КП пустой вид элемента (kind)"
		default:
			return "в окне КП неизвестный вид элемента: " + kind
		}
		if item.Required && !item.Visible {
			return "в окне КП «" + windowTitle(kind, id) +
				"» помечено обязательным, но скрыто: обязательное поле не может быть скрытым"
		}
		key := kind + "\x00" + id
		if seen[key] {
			return "в окне КП повторяется элемент: " + id
		}
		seen[key] = true
	}
	return ""
}

// WindowFieldRef — пустое обязательное поле окна: что показать человеку.
type WindowFieldRef struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// missingWindowFields — незаполненные обязательные поля окна КП (2026-11-15).
//
// Обязательность МЯГКАЯ (решение пользователя): список уходит в ответ, клиенты
// подсвечивают, сохранение проходит. Менеджер в полевом приложении не должен
// «застрять» на КП, которое можно донести.
//
// Скрытое поле не подсвечивается: администратор убрал его с экрана, и
// требовать с менеджера то, чего он не видит, нельзя. Обязательность скрытого
// уже снята `effectiveWindow`, но проверка `Visible` оставлена вторым рубежом —
// список пустых полей не должен зависеть от порядка вызовов.
//
// Правило: элемент `Required`, `Visible` и пустой.
//   - поле — `strings.TrimSpace(offer.FormValues[id]) == ""`;
//   - зона `client` — пусто и имя, и контакт.
func missingWindowFields(form OfferForm, offer Offer) []WindowFieldRef {
	missing := []WindowFieldRef{}
	for _, item := range effectiveWindow(form) {
		if !item.Required || !item.Visible {
			continue
		}
		switch item.Kind {
		case windowKindField:
			if strings.TrimSpace(offer.FormValues[item.ID]) != "" {
				continue
			}
			missing = append(missing, WindowFieldRef{ID: item.ID, Label: item.WindowTitle()})
		case windowKindZone:
			if item.ID != windowZoneClient {
				continue
			}
			if strings.TrimSpace(offer.Client.Name) != "" || strings.TrimSpace(offer.Client.Contact) != "" {
				continue
			}
			missing = append(missing, WindowFieldRef{ID: windowZoneClient, Label: item.WindowTitle()})
		}
	}
	return missing
}

// windowMissingOf — ответ ручки правки документа: документ после записи плюс
// незаполненные обязательные поля окна (2026-11-15).
//
// Ответ перестаёт быть голым `Offer`, поэтому приложение читает `res.offer`.
// Плагин эти две ручки не вызывает вовсе (см. offerfields.go): он сохраняет
// документ целиком через `saveOffer`.
type windowMissing struct {
	Offer           Offer            `json:"offer"`
	MissingRequired []WindowFieldRef `json:"missingRequired"`
}
