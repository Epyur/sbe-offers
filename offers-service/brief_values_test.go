package main

import (
	"strings"
	"testing"

	"mailers/offers/model"
)

// Бриф → значения формы (2026-11-05).
//
// До этого бриф жил только в таблице offers_brief, а печать, сводка и промпт
// модели читали form_values. Никто их не соединял, поэтому на живом КП
// 003/2026 модель получала пустой объект и писала «[НУЖНО: …]», хотя менеджер
// всё заполнил. Функция — единственное место, где бриф попадает в form_values.

func filledBrief() model.BriefPayload {
	p := model.EmptyBriefPayload()
	p.ObjectName = "ЖК «Сахаров Парк»"
	p.ObjectPurpose = "Жилые здания"
	p.ObjectArea = "7 883 м2"
	p.CustomerAddress = "ООО «Алтайгражданпроект»"
	p.CustomerGoals = []string{"сдать подвал без протечек", "уложиться в срок"}
	p.CustomerPains = []string{"протечки на сдаче"}
	p.ProjectRisks = []string{"высокий уровень грунтовых вод"}
	p.PlannedSystems = []string{"ТН-ФУНДАМЕНТ Протект Эксперт"}
	p.BudgetSignal = "бюджет согласован"
	p.DecisionMaker = "технический директор"
	p.Timeline = "3 квартал 2027"
	p.WhyUs = []string{"своя монтажная бригада"}
	p.Notes = "объект сдаётся двумя очередями"
	return p
}

// Поля брифа ложатся под ключами brief.* и печатаются списками через «; ».
func TestBriefFormValuesFillsFields(t *testing.T) {
	got := briefFormValues(emptyFormValues(), filledBrief())

	want := map[string]string{
		"brief.object_name":      "ЖК «Сахаров Парк»",
		"brief.object_purpose":   "Жилые здания",
		"brief.object_area":      "7 883 м2",
		"brief.customer_address": "ООО «Алтайгражданпроект»",
		"brief.customer_goals":   "сдать подвал без протечек; уложиться в срок",
		"brief.project_risks":    "высокий уровень грунтовых вод",
		"brief.timeline":         "3 квартал 2027",
		"brief.why_us":           "своя монтажная бригада",
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("%s = %q, ожидали %q", key, got[key], value)
		}
	}
}

// Пустые поля брифа НЕ затирают то, что уже есть в форме: форму заполняет
// человек, и бриф не имеет права обнулить его правку.
func TestBriefFormValuesKeepsFormValues(t *testing.T) {
	base := map[string]string{
		"brief.object_name": "Объект из формы",
		"client_name":       "ООО Заказчик",
	}
	got := briefFormValues(base, model.EmptyBriefPayload())
	if got["brief.object_name"] != "Объект из формы" {
		t.Fatalf("пустой бриф затёр значение формы: %q", got["brief.object_name"])
	}
	if got["client_name"] != "ООО Заказчик" {
		t.Fatalf("чужое поле формы пропало: %#v", got)
	}
}

// Менеджер правил объект руками в форме — бриф не перебивает правку.
func TestBriefFormValuesFormWinsOverBrief(t *testing.T) {
	base := map[string]string{"brief.object_name": "Как поправил менеджер"}
	got := briefFormValues(base, filledBrief())
	if got["brief.object_name"] != "Как поправил менеджер" {
		t.Fatalf("бриф перебил правку формы: %q", got["brief.object_name"])
	}
}

// nil-карта на входе — не паника, а новая карта.
func TestBriefFormValuesHandlesNil(t *testing.T) {
	got := briefFormValues(nil, filledBrief())
	if got == nil {
		t.Fatal("вернулась nil-карта")
	}
	if !strings.Contains(got["brief.object_name"], "Сахаров") {
		t.Fatalf("объект не записан: %#v", got)
	}
}

// Сводка брифа печатает те же значения, что получила модель: один источник,
// вторая копия не должна разойтись с промптом.
func TestBriefSummaryUsesMergedValues(t *testing.T) {
	values := briefFormValues(emptyFormValues(), filledBrief())
	var b strings.Builder
	renderBriefSummary(&b, values)
	text := b.String()
	if !strings.Contains(text, "ЖК «Сахаров Парк»") {
		t.Fatalf("сводка без объекта:\n%s", text)
	}
}

// emptyFormValues — форма без значений: карта не nil, иначе запись упадёт.
func emptyFormValues() map[string]string {
	return map[string]string{}
}
