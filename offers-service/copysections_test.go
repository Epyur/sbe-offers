package main

// Сторож шага S4 (2026-11-13): тексты разделов из приложения.
//
// Проверяется разбор адреса и правило «пропустить = удалить ключ»: именно на
// них держится совпадение с ботом. Модельная часть (написать/вычитать) общая с
// ботом и покрыта его тестами; живой путь проверяется на стенде.

import (
	"net/http/httptest"
	"testing"

	"mailers/offers/model"
	"mailers/offers/skills"
)

func TestSectionTargetSystemNeedsIndex(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/offers/offers/1/sections/system?systemIndex=2", nil)
	r.SetPathValue("key", "system")
	kind, index, ok := sectionTarget(httptest.NewRecorder(), r, 3)
	if !ok || kind != skills.SectionSystem || index != 2 {
		t.Fatalf("system с systemIndex=2: ok=%v kind=%q index=%d", ok, kind, index)
	}
}

func TestSectionTargetSystemRejectsOutOfRange(t *testing.T) {
	for _, raw := range []string{"", "0", "4", "-1", "abc"} {
		r := httptest.NewRequest("POST", "/x", nil)
		r.SetPathValue("key", "system")
		if raw != "" {
			r.URL.RawQuery = "systemIndex=" + raw
		}
		if _, _, ok := sectionTarget(httptest.NewRecorder(), r, 3); ok {
			t.Fatalf("system с systemIndex=%q принят, а систем 3", raw)
		}
	}
}

func TestSectionTargetFixedSlotIgnoresIndex(t *testing.T) {
	// У вступления номера нет: пришедший systemIndex не должен сделать его ошибкой.
	r := httptest.NewRequest("POST", "/x?systemIndex=2", nil)
	r.SetPathValue("key", "intro")
	kind, index, ok := sectionTarget(httptest.NewRecorder(), r, 3)
	if !ok || kind != skills.SectionIntro || index != 0 {
		t.Fatalf("intro: ok=%v kind=%q index=%d", ok, kind, index)
	}
}

func TestSectionTargetRejectsUnknownKind(t *testing.T) {
	r := httptest.NewRequest("POST", "/x", nil)
	r.SetPathValue("key", "испещрение")
	if _, _, ok := sectionTarget(httptest.NewRecorder(), r, 1); ok {
		t.Fatal("неизвестная секция принята")
	}
}

func TestAcceptSectionSkipDeletesKey(t *testing.T) {
	// «Пропустить» — пустой текст: ключ удаляется, чтобы сборка документа
	// отличала «пропущено» от «сохранён пустой».
	offer := model.Offer{FormValues: map[string]string{"copy.intro": "старый текст", "keep": "x"}}
	offer = AcceptSection(offer, skills.SectionIntro, 0, "   ")
	if _, exists := offer.FormValues["copy.intro"]; exists {
		t.Fatal("ключ секции не удалён при пропуске")
	}
	if offer.FormValues["keep"] != "x" {
		t.Fatal("пропуск секции затронул другое поле")
	}
}

func TestAcceptSectionStoresTrimmedText(t *testing.T) {
	offer := model.Offer{}
	offer = AcceptSection(offer, skills.SectionAdvantages, 0, "  Преимущества\n ")
	if offer.FormValues["copy.advantages"] != "Преимущества" {
		t.Fatalf("сохранено %q", offer.FormValues["copy.advantages"])
	}
}

func TestSectionKeyOfSystemIsNumbered(t *testing.T) {
	// Один и тот же текст у двух систем невозможен: ключ системы нумеруется.
	if SectionKey(skills.SectionSystem, 1) == SectionKey(skills.SectionSystem, 2) {
		t.Fatal("ключи описаний двух систем совпали")
	}
}

func TestNonNilStrings(t *testing.T) {
	if nonNilStrings(nil) == nil {
		t.Fatal("пустой список предупреждений остался nil — в JSON выйдет null")
	}
}
