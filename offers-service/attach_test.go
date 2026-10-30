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

func TestAttachReplyBriefAsksConfirmation(t *testing.T) {
	reply := attachReplyBrief(&briefFromFile{
		Fields:  map[string][]string{"object_name": {"Склад"}},
		Summary: []string{"объект: Склад"},
	})
	if !strings.Contains(reply, "Склад") {
		t.Fatalf("в ответе нет объекта: %q", reply)
	}
	if !strings.Contains(reply, "✓") {
		t.Fatalf("нет вопроса-подтверждения: %q", reply)
	}
}

func TestAttachReplyBriefWithoutFields(t *testing.T) {
	reply := attachReplyBrief(&briefFromFile{})
	if !strings.Contains(reply, "прочее") {
		t.Fatalf("не предложено разложить заметки: %q", reply)
	}
}

func TestAttachReplySpecMentionsPriceDiff(t *testing.T) {
	reply := attachReplySpec(&specFromFile{
		Rows:       []specRowView{{Name: "Плитка"}},
		Matched:    1,
		PriceDiffs: 1,
		Summary:    specSummary(1, 1, 0, 1),
	})
	if !strings.Contains(reply, "цен") {
		t.Fatalf("расхождение цен не упомянуто: %q", reply)
	}
	if !strings.Contains(reply, "каталог") {
		t.Fatalf("не сказано, по чьим ценам собирать: %q", reply)
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
	if !strings.Contains(got, "0 из 10") {
		t.Fatalf("прогресс = %q", got)
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
