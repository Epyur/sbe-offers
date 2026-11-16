package main

// Окно КП в приложении — модель настройки (2026-11-15, шаг 1 плана 0.11.0).
//
// Проверяется ровно то, что обещано администратору: пусто — «как сейчас»;
// сохранённый порядок главный; недостающее дописывается в конец НЕВИДИМЫМ;
// настройка, которая ничего не делает, отвергается до сохранения.

import (
	"strings"
	"testing"
)

// openTestForm — форма с полями заказчика и объекта; окно не задано.
func openTestForm() OfferForm {
	return OfferForm{
		Mode:  clientFormMode,
		Title: "Коммерческое предложение",
		Placeholders: []Placeholder{
			{ID: "client_name", Label: "Получатель (организация)"},
			{ID: "client_contact", Label: "Контактное лицо получателя"},
			{ID: "object_name", Label: "Объект"},
		},
		RequiredFields: []string{"client_name"},
	}
}

// TestEffectiveWindowDefault — пустое окно даёт нынешнее поведение: зоны в
// порядке `defaultZoneOrder` (прежние пять, затем бриф), затем все поля формы.
// Бриф известен и присутствует, но НЕВИДИМ: раздел не появляется у всех сам
// (0.11.2: он должен быть хотя бы в списке редактора, иначе включить нечем).
func TestEffectiveWindowDefault(t *testing.T) {
	form := openTestForm()
	got := effectiveWindow(form)

	var zones, fields []string
	for _, item := range got {
		switch item.Kind {
		case windowKindZone:
			if item.ID != windowZoneBrief && !item.Visible {
				t.Errorf("зона %q по умолчанию должна быть видна", item.ID)
			}
			if item.ID == windowZoneBrief && item.Visible {
				t.Errorf("зона %q по умолчанию должна быть НЕВИДИМА: новый раздел не появляется сам", item.ID)
			}
			zones = append(zones, item.ID)
		case windowKindField:
			if !item.Visible {
				t.Errorf("поле %q по умолчанию должно быть видно", item.ID)
			}
			fields = append(fields, item.ID)
		}
	}
	if strings.Join(zones, ",") != strings.Join(defaultZoneOrder, ",") {
		t.Fatalf("порядок зон по умолчанию %v, ожидали %v", zones, defaultZoneOrder)
	}
	if strings.Join(fields, ",") != "client_name,client_contact,object_name" {
		t.Fatalf("поля по умолчанию %v", fields)
	}
	for _, item := range got {
		if item.Kind != windowKindField {
			continue
		}
		if wantRequired := item.ID == "client_name"; item.Required != wantRequired {
			t.Errorf("обязательность поля %q = %v, ожидали %v", item.ID, item.Required, wantRequired)
		}
	}
}

// TestEffectiveWindowKeepsOrder — записанный порядок главный; подпись поля
// берётся из `placeholders`, а не из сохранённой копии.
func TestEffectiveWindowKeepsOrder(t *testing.T) {
	form := openTestForm()
	form.Window = []WindowItem{
		{Kind: windowKindField, ID: "object_name", Label: "старая подпись", Visible: true},
		{Kind: windowKindZone, ID: "items", Visible: true},
		{Kind: windowKindZone, ID: "client", Visible: true, Required: true},
	}
	got := effectiveWindow(form)

	keys := make([]string, 0, len(got))
	for _, item := range got {
		keys = append(keys, item.Kind+":"+item.ID)
	}
	want := []string{
		"field:object_name", "zone:items", "zone:client",
	}
	if strings.Join(keys[:3], ",") != strings.Join(want, ",") {
		t.Fatalf("порядок окна %v, ожидали начало %v", keys, want)
	}
	if got[0].Label != "Объект" {
		t.Errorf("подпись поля должна приходить из placeholders, получено %q", got[0].Label)
	}
	// Зона без поля, но с подписью в данных: подпись зоны не хранится.
	for _, item := range got {
		if item.Kind == windowKindZone && item.Label != "" {
			t.Errorf("у зоны %q должна быть пустая подпись в данных, получено %q", item.ID, item.Label)
		}
	}
	if len(got) != len(knownWindowZones)+len(form.Placeholders) {
		t.Fatalf("в окне %d элементов, ожидали %d", len(got), len(knownWindowZones)+len(form.Placeholders))
	}
}

// TestEffectiveWindowAppendsMissingHidden — недостающая зона и недостающее поле
// дописываются в конец невидимыми: сохранённая до их появления настройка не
// покажет их случайно (решение пользователя §2.4 спеки).
func TestEffectiveWindowAppendsMissingHidden(t *testing.T) {
	form := openTestForm()
	form.Window = []WindowItem{
		{Kind: windowKindZone, ID: "client", Visible: true},
	}
	got := effectiveWindow(form)

	tail := got[1:]
	if len(tail) != len(knownWindowZones)-1+len(form.Placeholders) {
		t.Fatalf("хвост %d элементов, ожидали %d", len(tail), len(knownWindowZones)-1+len(form.Placeholders))
	}
	for _, item := range tail {
		if item.Visible {
			t.Errorf("дописанный элемент %s:%s должен быть невидимым", item.Kind, item.ID)
		}
	}
	// Порядок: бриф встаёт сразу за своим якорем («Заказчик»), остальное
	// дописывается после. До 0.11.2 бриф уходил в самый конец — под таблицей
	// полей формы, — и включённая зона оказывалась внизу (дефект, найденный
	// `claude-5-opus` 2026-11-16).
	if got[1].ID != windowZoneBrief {
		t.Errorf("бриф должен стоять сразу за Заказчиком, получено %q", got[1].ID)
	}
	if tail[1].ID != "systems" {
		t.Errorf("после брифа первым дописанным должен быть systems, получено %q", tail[1].ID)
	}
	last := tail[len(tail)-1]
	if last.Kind != windowKindField || last.ID != "object_name" {
		t.Errorf("последним дописанным должно быть поле object_name, получено %s:%s", last.Kind, last.ID)
	}
}

// TestEffectiveWindowDropsUnknown — неизвестные kind/id выкидываются: зону
// рисует код, и настройка без него ничего не делает.
func TestEffectiveWindowDropsUnknown(t *testing.T) {
	form := openTestForm()
	form.Window = []WindowItem{
		{Kind: windowKindZone, ID: "digest", Visible: true},
		{Kind: windowKindField, ID: "неизвестное", Visible: true},
		{Kind: "какой-то", ID: "client", Visible: true},
		{Kind: windowKindZone, ID: "client", Visible: true},
		{Kind: windowKindZone, ID: "client", Visible: false},
	}
	got := effectiveWindow(form)
	if len(got) == 0 || got[0].Kind != windowKindZone || got[0].ID != "client" {
		t.Fatalf("первым должен быть client, получено %+v", got[0])
	}
	if !got[0].Visible {
		t.Error("первый client виден (дубль не должен его перетереть)")
	}
	for _, item := range got {
		if item.ID == "digest" || item.ID == "неизвестное" {
			t.Errorf("неизвестный элемент %s:%s остался в окне", item.Kind, item.ID)
		}
	}
	// client встречается ровно один раз: дубль пары (kind, id) выкинут.
	count := 0
	for _, item := range got {
		if item.Kind == windowKindZone && item.ID == "client" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("зона client встречается %d раз, ожидали 1", count)
	}
}

// TestValidateWindowChecks — проверка сохранения: пустое окно законно,
// неизвестная зона/поле, пустой kind и дубли отвергаются.
func TestValidateWindowChecks(t *testing.T) {
	cases := []struct {
		name string
		win  []WindowItem
		want string
	}{
		{"пустое окно законно", nil, ""},
		{
			"известные элементы",
			[]WindowItem{
				{Kind: windowKindZone, ID: "client", Visible: true, Required: true},
				{Kind: windowKindField, ID: "object_name", Visible: true},
			},
			"",
		},
		{
			"неизвестная зона",
			[]WindowItem{{Kind: windowKindZone, ID: "digest", Visible: true}},
			"неизвестная зона",
		},
		{
			"поле не заведено",
			[]WindowItem{{Kind: windowKindField, ID: "ghost", Visible: true}},
			"не заведено в списке полей",
		},
		{
			"пустой вид",
			[]WindowItem{{Kind: "", ID: "client"}},
			"пустой вид элемента",
		},
		{
			"неизвестный вид",
			[]WindowItem{{Kind: "zone2", ID: "client"}},
			"неизвестный вид элемента",
		},
		{
			"дубль",
			[]WindowItem{
				{Kind: windowKindZone, ID: "client", Visible: true},
				{Kind: windowKindZone, ID: "client", Visible: false},
			},
			"повторяется элемент",
		},
		{
			"скрытое обязательное поле",
			[]WindowItem{{Kind: windowKindField, ID: "object_name", Visible: false, Required: true}},
			"обязательное поле не может быть скрытым",
		},
		{
			"скрытая обязательная зона",
			[]WindowItem{{Kind: windowKindZone, ID: "client", Visible: false, Required: true}},
			"обязательное поле не может быть скрытым",
		},
		{
			"скрытое необязательное поле законно",
			[]WindowItem{{Kind: windowKindField, ID: "object_name", Visible: false}},
			"",
		},
	}
	for _, tc := range cases {
		form := openTestForm()
		form.Window = tc.win
		got := validateWindow(form)
		if tc.want == "" {
			if got != "" {
				t.Errorf("%s: ожидали отсутствие ошибки, получено %q", tc.name, got)
			}
			continue
		}
		if !strings.Contains(got, tc.want) {
			t.Errorf("%s: ошибка %q, ожидали содержащую %q", tc.name, got, tc.want)
		}
	}
}

// TestValidateWindowRejectsRequiredOnSystems — обязательной может быть только
// зона «Заказчик»: у остальных зон обязательность никто не проверяет, и
// хранить её значит обещать то, чего нет.
func TestValidateWindowRejectsRequiredOnSystems(t *testing.T) {
	form := openTestForm()
	form.Window = []WindowItem{
		{Kind: windowKindZone, ID: "systems", Visible: true, Required: true},
	}
	if msg := validateWindow(form); msg == "" {
		t.Fatal("обязательная зона systems должна отвергаться")
	}
	form.Window = []WindowItem{
		{Kind: windowKindZone, ID: "client", Visible: true, Required: true},
	}
	got := validateWindow(form)
	if got != "" {
		t.Fatalf("обязательная зона client законна, получено %q", got)
	}
}

// TestFormWindowIsValid — форма с окном проходит общую валидацию: иначе
// настройку нельзя было бы сохранить из редактора.
func TestFormWindowIsValid(t *testing.T) {
	form := openTestForm()
	form.Window = []WindowItem{
		{Kind: windowKindZone, ID: "client", Visible: true, Required: true},
		{Kind: windowKindField, ID: "client_name", Visible: true, Required: true},
	}
	if msg := validateForm(form); msg != "" {
		t.Fatalf("форма с окном не проходит проверку: %s", msg)
	}
}

// TestHiddenWindowItemIsNotRequired — «пока скрыто, оно не обязательно»
// (решение пользователя 2026-11-15). Сохранить такую настройку нельзя
// (TestValidateWindowChecks), но прийти она может из старой формы или прямой
// правки базы — на чтении приводим к смыслу, а не верим данным.
func TestHiddenWindowItemIsNotRequired(t *testing.T) {
	form := openTestForm()
	form.Window = []WindowItem{
		{Kind: windowKindField, ID: "client_name", Visible: false, Required: true},
		{Kind: windowKindZone, ID: "client", Visible: false, Required: true},
	}
	// Настройка не проходит валидацию — сохранение такого состояния запрещено.
	if msg := validateWindow(form); msg == "" {
		t.Fatal("скрытое обязательное должно отвергаться валидацией")
	}

	for _, item := range effectiveWindow(form) {
		if item.Required {
			t.Errorf("скрытый элемент %s:%s остался обязательным", item.Kind, item.ID)
		}
	}
	// И пустые обязательные на нём не подсвечиваются.
	offer := Offer{FormValues: map[string]string{}}
	if got := missingWindowFields(form, offer); len(got) != 0 {
		t.Fatalf("скрытое обязательное не должно требовать заполнения, получено %+v", got)
	}
}

// TestBriefZoneIsOptIn — зона брифа известна серверу и ВХОДИТ в окно «как
// сейчас» (0.11.2), но НЕВИДИМОЙ: раздел не появляется у всех сам, это по-
// прежнему решение администратора. До 0.11.2 её не было в умолчании вовсе —
// у новой формы включить зону было нечем (живая жалоба 2026-11-16).
func TestBriefZoneIsOptIn(t *testing.T) {
	if !knownWindowZone(windowZoneBrief) {
		t.Fatal("зона brief должна быть известна серверу")
	}
	if title := (WindowItem{Kind: windowKindZone, ID: windowZoneBrief}).WindowTitle(); title != "Бриф" {
		t.Errorf("подпись зоны brief = %q, ожидали «Бриф»", title)
	}
	var found *WindowItem
	for _, item := range effectiveWindow(openTestForm()) {
		if item.Kind == windowKindZone && item.ID == windowZoneBrief {
			copied := item
			found = &copied
		}
	}
	if found == nil {
		t.Fatal("бриф обязан быть в окне по умолчанию, иначе его не включить")
	}
	if found.Visible {
		t.Fatal("бриф по умолчанию невидим: раздел не появляется у всех сам")
	}
}

// TestBriefZoneAppendedHidden — форма, сохранённая до появления брифа,
// получает его в конец НЕВИДИМЫМ: администратор включит сам (§2.4 спеки).
func TestBriefZoneAppendedHidden(t *testing.T) {
	form := openTestForm()
	form.Window = []WindowItem{
		{Kind: windowKindZone, ID: "client", Visible: true},
		{Kind: windowKindZone, ID: "systems", Visible: true},
	}
	var brief *WindowItem
	for _, item := range effectiveWindow(form) {
		if item.Kind == windowKindZone && item.ID == windowZoneBrief {
			copied := item
			brief = &copied
		}
	}
	if brief == nil {
		t.Fatal("новая зона должна дописываться в окно, иначе редактор её не увидит")
	}
	if brief.Visible {
		t.Fatal("дописанная зона должна быть невидимой до решения администратора")
	}
}

// TestBriefZoneSaves — включённый бриф проходит проверку сохранения, а
// обязательным его сделать нельзя: подсветки пустого брифа в окне нет.
func TestBriefZoneSaves(t *testing.T) {
	form := openTestForm()
	form.Window = []WindowItem{{Kind: windowKindZone, ID: windowZoneBrief, Visible: true}}
	if msg := validateWindow(form); msg != "" {
		t.Fatalf("бриф в окне не проходит проверку: %s", msg)
	}
	form.Window = []WindowItem{{Kind: windowKindZone, ID: windowZoneBrief, Visible: true, Required: true}}
	if msg := validateWindow(form); msg == "" {
		t.Fatal("обязательный бриф должен отвергаться: его никто не проверяет")
	}
}
