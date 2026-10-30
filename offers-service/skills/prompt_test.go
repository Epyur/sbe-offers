package skills

import (
	"strings"
	"testing"

	"mailers/offers/model"
)

// Промпт собирается из скилов и данных КП. Проверяется не точная формулировка
// (она меняется), а свойства, без которых письмо модели ломается:
// тема каждого вопроса, дословные числа и правило [НУЖНО: ...].
func TestRenderBriefSystemListsAllTopics(t *testing.T) {
	prompt := RenderBriefSystem(model.BriefPayload{}, model.ProductContext{})
	for _, topic := range BriefTopics() {
		if !strings.Contains(prompt, topic.Key) {
			t.Errorf("в промпте брифа нет темы %q", topic.Key)
		}
		if !strings.Contains(prompt, topic.Question) {
			t.Errorf("в промпте нет вопроса темы %q", topic.Key)
		}
	}
}

func TestRenderBriefSystemSaysNoInventing(t *testing.T) {
	prompt := RenderBriefSystem(model.BriefPayload{}, model.ProductContext{})
	for _, must := range []string{"не придумывай", "не знаю"} {
		if !strings.Contains(strings.ToLower(prompt), must) {
			t.Errorf("промпт брифа не запрещает выдумывать (%q)", must)
		}
	}
}

func TestRenderBriefSystemCarriesKnownFields(t *testing.T) {
	brief := model.BriefPayload{ObjectName: "ЖК Поэма", BudgetSignal: "до 20 млн"}
	prompt := RenderBriefSystem(brief, model.ProductContext{})
	if !strings.Contains(prompt, "ЖК Поэма") {
		t.Error("в промпте нет уже известного объекта")
	}
	if !strings.Contains(prompt, "до 20 млн") {
		t.Error("в промпте нет уже известного бюджета")
	}
}

// Числа КП попадают в промпт ДОСЛОВНО, как их напечатает документ: если модель
// получит 20158633.08, она перепишет их своим форматом и в КП будет
// расхождение с таблицей.
func TestRenderCopySystemKeepsTotalsVerbatim(t *testing.T) {
	offer := model.Offer{
		Number: "007/2026",
		Total:  20158633.08,
		Client: model.OfferClient{Name: "ООО Строй"},
		Items: []model.OfferItem{
			{Name: "Мембрана V-SL 2 мм", Unit: "м2", Qty: 5520, Price: 769.56, Sum: 4247971.20},
		},
	}
	prompt := RenderCopySystem(SectionIntro, model.BriefPayload{}, offer, model.ProductContext{})
	if !strings.Contains(prompt, "20 158 633,08") {
		t.Errorf("итог не дошёл дословно в формате документа:\n%s", prompt)
	}
	if strings.Contains(prompt, "20158633.08") {
		t.Error("в промпт ушёл машинный формат числа")
	}
}

func TestRenderCopySystemCarriesNeedRule(t *testing.T) {
	prompt := RenderCopySystem(SectionIntro, model.BriefPayload{}, model.Offer{}, model.ProductContext{})
	if !strings.Contains(prompt, "НУЖНО:") {
		t.Error("в промпте нет правила пометки нехватки факта")
	}
}

// Пустой контекст продукта — не повод получить пустой системный промпт:
// правила письма не зависят от того, заполнил ли админ разделы.
func TestRenderCopySystemWithoutContext(t *testing.T) {
	prompt := RenderCopySystem(SectionAdvantages, model.BriefPayload{}, model.Offer{}, model.ProductContext{})
	if strings.TrimSpace(prompt) == "" {
		t.Fatal("системный промпт пуст без контекста")
	}
	if len(prompt) < 500 {
		t.Fatalf("системный промпт подозрительно короткий (%d символов)", len(prompt))
	}
}

func TestRenderCopySystemCarriesContext(t *testing.T) {
	ctx := model.ProductContext{Sections: map[string]string{
		"positioning": "Производитель ПВХ-мембран для подземных сооружений",
		"pains":       "протечки подвала на сдаче объекта",
	}}
	prompt := RenderCopySystem(SectionIntro, model.BriefPayload{}, model.Offer{}, ctx)
	if !strings.Contains(prompt, "Производитель ПВХ-мембран") {
		t.Error("позиционирование не дошло до промпта")
	}
	if !strings.Contains(prompt, "протечки подвала") {
		t.Error("боли клиента не дошли до промпта")
	}
}

// Каждая секция объявляет свою задачу: модель не должна писать «в целом о КП».
func TestRenderCopySystemNamesSectionKind(t *testing.T) {
	for _, kind := range []SectionKind{SectionIntro, SectionSystem, SectionAdvantages, SectionConditions} {
		prompt := RenderCopySystem(kind, model.BriefPayload{}, model.Offer{}, model.ProductContext{})
		if !strings.Contains(prompt, string(kind)) {
			t.Errorf("секция %q не названа в промпте", kind)
		}
	}
}

func TestRenderEditSystemIsPointEdit(t *testing.T) {
	prompt := RenderEditSystem(SectionIntro, model.ProductContext{})
	if !strings.Contains(strings.ToLower(prompt), "прав") {
		t.Error("промпт редактуры не объясняет, что правится уже написанный текст")
	}
	if !strings.Contains(prompt, "НУЖНО:") {
		t.Error("редактура тоже обязана помечать нехватку факта")
	}
}

// Темы брифа — стабильный порядок: по нему строится «что спросить дальше».
func TestBriefTopicsStableOrder(t *testing.T) {
	topics := BriefTopics()
	if len(topics) < 8 {
		t.Fatalf("тем брифа слишком мало: %d", len(topics))
	}
	if topics[0].Key != "object_name" {
		t.Errorf("первая тема должна быть объектом, а не %q", topics[0].Key)
	}
	seen := map[string]bool{}
	for _, t2 := range topics {
		if seen[t2.Key] {
			t.Errorf("тема %q повторяется", t2.Key)
		}
		seen[t2.Key] = true
		if strings.TrimSpace(t2.Question) == "" {
			t.Errorf("у темы %q нет вопроса", t2.Key)
		}
	}
}

func TestNextOpenTopic(t *testing.T) {
	if _, ok := NextOpenTopic(model.BriefPayload{}); !ok {
		if BriefTopics()[0].Key != "object_name" {
			t.Fatal("у пустого брифа должна быть следующая тема")
		}
	}
	full := model.BriefPayload{
		ObjectName:    "ЖК Поэма",
		CustomerPains: []string{"протечки"},
	}
	topic, ok := NextOpenTopic(full)
	if !ok {
		t.Fatal("у частично заполненного брифа ещё есть темы")
	}
	if topic.Key == "object_name" || topic.Key == "customer_pains" {
		t.Fatalf("заполненная тема %q не должна спрашиваться", topic.Key)
	}
}
