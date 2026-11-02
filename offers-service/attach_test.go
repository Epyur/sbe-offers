package main

import (
	"strings"
	"testing"

	"mailers/offers/model"
	"mailers/offers/skills"
)

// Приём файлов в КП (шаг 6 плана 2026-10-29).
//
// Сеть и БД снаружи: проверяются чистые части — что бот говорит человеку и что
// попадает в бриф/состав. Ручки проверяются интеграционным прогоном на стенде.

func TestBriefFromMarkdownFields(t *testing.T) {
	parsed := skills.ParseMarkdownAttachment(strings.Join([]string{
		"Объект: ЖК «Поэма», корпус 3",
		"Сроки: до 15 сентября",
		"Бюджет: до 12 млн",
		"Заказчик: ООО Строй-Инвест",
	}, "\n"))

	got := briefFromMarkdown(parsed)

	if got.Fields["object_name"][0] != "ЖК «Поэма», корпус 3" {
		t.Fatalf("объект в полях = %#v", got.Fields["object_name"])
	}
	if len(got.Summary) == 0 {
		t.Fatal("пустая сводка «что понял»")
	}
	// Заказчик — не поле брифа, но виден в заметках.
	if !strings.Contains(got.Notes, "ООО Строй-Инвест") {
		t.Fatalf("заказчик потерялся: %q", got.Notes)
	}
}

func TestAttachReplyAsksConfirmationForContext(t *testing.T) {
	// Контекст из файла (2026-11-01): таблица без состава, но с объектом.
	reply := attachReply(&attachedFile{
		ContextFields: &skills.FileContext{
			Fields:  map[string][]string{"object_name": {"Склад"}},
			Summary: []string{"объект: Склад"},
		},
	}, "описание.xlsx")
	if !strings.Contains(reply, "Склад") {
		t.Fatalf("в ответе нет объекта: %q", reply)
	}
	if !strings.Contains(reply, "✓") {
		t.Fatalf("нет вопроса-подтверждения: %q", reply)
	}
	if !strings.Contains(reply, "бриф") {
		t.Fatalf("не сказано, куда пойдёт разбор: %q", reply)
	}
}

func TestAttachReplyWithoutAnythingToApply(t *testing.T) {
	reply := attachReply(&attachedFile{Context: []string{"какая-то строка"}}, "файл.xlsx")
	if !strings.Contains(reply, "ничего применить не смог") {
		t.Fatalf("не сказано, что применять нечего: %q", reply)
	}
	if !strings.Contains(reply, "объект") {
		t.Fatalf("не предложено рассказать про объект: %q", reply)
	}
}

func TestAttachReplySpecMentionsPriceDiff(t *testing.T) {
	reply := attachReply(&attachedFile{
		Spec: &specFromFile{
			Rows:       []specRowView{{Name: "Плитка"}},
			Matched:    1,
			PriceDiffs: 1,
			Summary:    specSummary(1, 1, 0, 1),
		},
	}, "таблица.xlsx")
	if !strings.Contains(reply, "цен") {
		t.Fatalf("расхождение цен не упомянуто: %q", reply)
	}
	if !strings.Contains(reply, "каталог") {
		t.Fatalf("не сказано, по чьим ценам собирать: %q", reply)
	}
}

// Таблица состава И контекстом сразу: старый КП в Excel — это и позиции, и
// описание объекта. Ответ обязан назвать обе части (2026-11-01).
func TestAttachReplySpecAndContext(t *testing.T) {
	reply := attachReply(&attachedFile{
		Spec: &specFromFile{
			Rows:    []specRowView{{Name: "Мембрана"}},
			Summary: specSummary(41, 0, 41, 0),
		},
		ContextFields: &skills.FileContext{
			Fields:  map[string][]string{"object_name": {"Дом на Охотской"}},
			Summary: []string{"объект: Дом на Охотской"},
		},
	}, "КП.xlsx")
	if !strings.Contains(reply, "41") {
		t.Fatalf("состав не назван: %q", reply)
	}
	if !strings.Contains(reply, "Охотской") {
		t.Fatalf("контекст не назван: %q", reply)
	}
	if !strings.Contains(reply, "записать контекст в бриф") {
		t.Fatalf("не сказано, что запишется и контекст: %q", reply)
	}
}

func TestSpecSummary(t *testing.T) {
	got := specSummary(12, 9, 3, 2)
	for _, want := range []string{"12", "9", "3", "2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("в сводке %q нет %q", got, want)
		}
	}
}

func TestMergeBriefFieldsKeepsWhatManagerSaid(t *testing.T) {
	prev := model.EmptyBriefPayload()
	prev.ObjectName = "Старый склад"
	prev.CustomerPains = []string{"протечки"}

	// Файл дополняет бриф, а не переписывает: присланный черновик не должен
	// стирать то, что менеджер рассказал в чате.
	got := mergeBriefFields(prev, map[string][]string{
		"object_name":    {"Новый адрес"},
		"customer_pains": {"протечки", "срыв сроков"},
		"timeline":       {"до 15 сентября"},
	})

	if got.ObjectName != "Новый адрес" {
		t.Fatalf("объект = %q, ожидали значение из файла", got.ObjectName)
	}
	if len(got.CustomerPains) != 2 {
		t.Fatalf("боли объединены с повтором: %#v", got.CustomerPains)
	}
	if got.Timeline != "до 15 сентября" {
		t.Fatalf("сроки = %q", got.Timeline)
	}
}

func TestMergeBriefFieldsSkipsObjectionWithoutResponse(t *testing.T) {
	got := mergeBriefFields(model.EmptyBriefPayload(), map[string][]string{
		"objections": {"дорого", "дорого — дадим скидку"},
	})
	if len(got.Objections) != 1 {
		t.Fatalf("возражения = %#v, ожидали только полное", got.Objections)
	}
	if got.Objections[0].Response != "дадим скидку" {
		t.Fatalf("ответ = %q", got.Objections[0].Response)
	}
}

func TestMergeBriefFieldsKeepsNotesIntact(t *testing.T) {
	prev := model.EmptyBriefPayload()
	prev.Notes = "позвонить в четверг"
	got := mergeBriefFields(prev, map[string][]string{})
	if got.Notes != "позвонить в четверг" {
		t.Fatalf("notes = %q", got.Notes)
	}
}

func TestBriefProgress(t *testing.T) {
	got := briefProgress(model.EmptyBriefPayload())
	if !strings.Contains(got, "0 из 14") {
		t.Fatalf("прогресс = %q", got)
	}
}

// Дословный разбор и свободный контекст складываются в один (2026-11-01).
//
// Порядок важен: подпись «Объект:» из разбора файла главнее свободного текста,
// а заметки объединяются без повторов.
func TestMergeBriefFilesPrefersStrictLabels(t *testing.T) {
	base := &briefFromFile{
		Fields:  map[string][]string{"object_name": {"Дом на Охотской"}},
		Notes:   "строка из файла",
		Summary: []string{"объект: Дом на Охотской"},
	}
	extra := &briefFromFile{
		Fields:  map[string][]string{"object_name": {"ЖК «Поэма»"}, "object_area": {"9 200 м2"}},
		Notes:   "строка из файла\nвторая строка",
		Summary: []string{"площадь: 9 200 м2"},
	}

	got := mergeBriefFiles(base, extra)
	if len(got.Fields["object_name"]) != 1 || got.Fields["object_name"][0] != "Дом на Охотской" {
		t.Fatalf("объект = %#v, ожидали значение из строгого разбора", got.Fields["object_name"])
	}
	if got.Fields["object_area"][0] != "9 200 м2" {
		t.Fatalf("площадь контекста потеряна: %#v", got.Fields["object_area"])
	}
	if strings.Count(got.Notes, "строка из файла") != 1 {
		t.Fatalf("заметки повторены: %q", got.Notes)
	}
	if !strings.Contains(got.Notes, "вторая строка") {
		t.Fatalf("заметки контекста потеряны: %q", got.Notes)
	}
	if len(got.Summary) != 2 {
		t.Fatalf("сводка = %#v, ожидали обе строки", got.Summary)
	}
}

func TestMergeBriefFilesHandlesOneSide(t *testing.T) {
	base := &briefFromFile{Fields: map[string][]string{"object_name": {"Дом"}}}
	if got := mergeBriefFiles(nil, base); got != base {
		t.Fatal("единственный разбор должен вернуться как есть")
	}
	if got := mergeBriefFiles(base, nil); got != base {
		t.Fatal("единственный разбор должен вернуться как есть")
	}
	if got := mergeBriefFiles(nil, nil); got != nil {
		t.Fatal("двух пустых разборов быть не должно")
	}
}

func TestBriefFromContext(t *testing.T) {
	if got := briefFromContext(nil); got != nil {
		t.Fatal("пустой контекст стал разбором брифа")
	}
	empty := &skills.FileContext{Fields: map[string][]string{}}
	if got := briefFromContext(empty); got != nil {
		t.Fatal("контекст без полей стал разбором брифа")
	}
	got := briefFromContext(&skills.FileContext{
		Fields:  map[string][]string{"object_name": {"Склад"}},
		Notes:   "описание",
		Summary: []string{"объект: Склад"},
	})
	if got == nil || got.Fields["object_name"][0] != "Склад" || got.Notes != "описание" {
		t.Fatalf("контекст переведён неверно: %#v", got)
	}
}

func TestBriefFromFileHasFields(t *testing.T) {
	if (*briefFromFile)(nil).hasFields() {
		t.Fatal("nil-разбор должен считаться пустым")
	}
	if (&briefFromFile{}).hasFields() {
		t.Fatal("разбор без полей должен считаться пустым")
	}
	if !(&briefFromFile{Fields: map[string][]string{"object_name": {"Дом"}}}).hasFields() {
		t.Fatal("разбор с полем должен считаться непустым")
	}
}

func TestAttachmentTitle(t *testing.T) {
	if got := attachmentTitle("C:\\tmp\\бриф.md"); got != "бриф.md" {
		t.Fatalf("attachmentTitle = %q", got)
	}
	if got := attachmentTitle(""); got != "файл" {
		t.Fatalf("attachmentTitle() = %q", got)
	}
}

// Таблица состава — это позиции, а НЕ описание объекта (2026-11-01).
//
// Живой случай: старый КП в Excel разобрался в состав, а наименования позиций
// уехали в заметки брифа; бот обещал «записать контекст», которого не было.
// Проверяем правило на чистом решении, а не на собранной книге: строить xlsx в
// тестах сервиса — отдельная инфраструктура, а решение от формы файла не
// зависит.
func TestSpecFileDoesNotBecomeBriefContext(t *testing.T) {
	if contextFromParsed(true, []string{"Мембрана", "Геотекстиль"}) != nil {
		t.Fatal("контекст построен из позиций состава")
	}
	got := contextFromParsed(false, []string{"для объекта: Дом на Охотской"})
	if got == nil || len(got.Fields["object_name"]) == 0 {
		t.Fatalf("контекст не построен из таблицы-описания: %#v", got)
	}
}
