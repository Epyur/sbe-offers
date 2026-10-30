package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"mailers/offers/model"
	"mailers/offers/skills"
)

// Текст секций КП (Task 6, 2026-10-29).
//
// Секции пишет модель, менеджер принимает или правит. Согласованный текст
// живёт ТОЛЬКО в form_values["copy.<ключ>"] предложения — вторая копия в
// сессии недопустима: после перезапуска бота она потеряется, и менеджер увидит
// старое.

// fakeCopyCompleter — модель вместо HTTP.
type fakeCopyCompleter struct {
	answer   string
	err      error
	calls    int
	lastSys  string
	lastUser string
}

func (f *fakeCopyCompleter) Complete(_ context.Context, system, user string, _ int) (string, error) {
	f.calls++
	f.lastSys = system
	f.lastUser = user
	if f.err != nil {
		return "", f.err
	}
	return f.answer, nil
}

func sampleSectionOffer() model.Offer {
	return model.Offer{
		ID:     1,
		Number: "007/2026",
		Client: model.OfferClient{Name: "ООО Строй"},
		Items: []model.OfferItem{
			{Name: "Мембрана V-SL 2 мм", Unit: "м2", Qty: 5520, Price: 769.56, Sum: 4247971.20},
		},
		Total:      4247971.20,
		FormValues: map[string]string{},
	}
}

// WriteSection — модель написала текст, его нужно вернуть как есть.
func TestWriteSectionReturnsText(t *testing.T) {
	c := &fakeCopyCompleter{answer: "Предлагаем материалы для гидроизоляции фундамента."}
	text, err := WriteSection(context.Background(), skills.SectionIntro,
		model.BriefPayload{ObjectName: "ЖК Поэма"}, sampleSectionOffer(), model.DefaultContext("client"), c)
	if err != nil {
		t.Fatalf("неожидали ошибку: %v", err)
	}
	if !strings.Contains(text, "гидроизоляции") {
		t.Fatalf("текст потерялся: %q", text)
	}
}

// Числа предложения доходят до промпта дословно — иначе модель перепишет их
// в своём формате, и в КП будет расхождение с таблицей.
func TestWriteSectionKeepsNumbersVerbatim(t *testing.T) {
	c := &fakeCopyCompleter{answer: "текст"}
	_, err := WriteSection(context.Background(), skills.SectionSystem,
		model.BriefPayload{}, sampleSectionOffer(), model.DefaultContext("client"), c)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if !strings.Contains(c.lastUser, "4 247 971,20") {
		t.Errorf("итог не дошёл в формате документа: %q", c.lastUser)
	}
	if strings.Contains(c.lastUser, "4247971.2") {
		t.Error("в промпт ушёл машинный формат")
	}
}

// Предупреждения чек-листа прикладываются, но текст НЕ меняется молча:
// правку делает человек, а не сервис.
func TestWriteSectionWarnsButKeepsText(t *testing.T) {
	c := &fakeCopyCompleter{answer: "Это не дорого, это выгодно."}
	res, err := WriteSectionChecked(context.Background(), skills.SectionIntro,
		model.BriefPayload{}, sampleSectionOffer(), model.DefaultContext("client"), c)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if res.Text != "Это не дорого, это выгодно." {
		t.Errorf("текст изменён без согласия: %q", res.Text)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("AI-признак не помечен")
	}
}

// Выдуманное число — предупреждение.
func TestWriteSectionFlagsInventedNumber(t *testing.T) {
	c := &fakeCopyCompleter{answer: "Экономия до 40% на каждом квадратном метре."}
	res, err := WriteSectionChecked(context.Background(), skills.SectionIntro,
		model.BriefPayload{}, sampleSectionOffer(), model.DefaultContext("client"), c)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "40") {
			found = true
		}
	}
	if !found {
		t.Fatalf("выдуманное число не помечено: %v", res.Warnings)
	}
}

// Модель недоступна — секция пустая, но ошибки НЕТ: КП соберётся без текста.
func TestWriteSectionLLMNotConfigured(t *testing.T) {
	c := &fakeCopyCompleter{err: errLLMNotConfigured}
	text, err := WriteSection(context.Background(), skills.SectionIntro,
		model.BriefPayload{}, sampleSectionOffer(), model.DefaultContext("client"), c)
	if err != nil {
		t.Fatalf("недоступность модели не должна быть ошибкой: %v", err)
	}
	if text != "" {
		t.Fatalf("ожидали пустой текст, получили %q", text)
	}
}

// Авария провайдера — тоже не ошибка: деградация, а не падение КП.
func TestWriteSectionUpstreamError(t *testing.T) {
	c := &fakeCopyCompleter{err: errors.New("502")}
	text, err := WriteSection(context.Background(), skills.SectionAdvantages,
		model.BriefPayload{}, sampleSectionOffer(), model.DefaultContext("client"), c)
	if err != nil {
		t.Fatalf("авария провайдера не должна валить сборку: %v", err)
	}
	if text != "" {
		t.Fatalf("ожидали пустой текст, получили %q", text)
	}
}

// Пустой ответ модели — пустая секция, а не «пустая строка в документе».
func TestWriteSectionEmptyAnswer(t *testing.T) {
	c := &fakeCopyCompleter{answer: "   \n  "}
	text, err := WriteSection(context.Background(), skills.SectionIntro,
		model.BriefPayload{}, sampleSectionOffer(), model.DefaultContext("client"), c)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if text != "" {
		t.Fatalf("ожидали пустой текст, получили %q", text)
	}
}

// --- Правка менеджера ---

func TestEditSectionPassesManagerText(t *testing.T) {
	c := &fakeCopyCompleter{answer: "Предлагаем материалы для гидроизоляции."}
	text, err := EditSection(context.Background(), skills.SectionIntro,
		"предлагаем материалы для гидроизоляции", model.DefaultContext("client"), c)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if !strings.Contains(text, "гидроизоляции") {
		t.Fatalf("текст потерялся: %q", text)
	}
	if !strings.Contains(c.lastUser, "предлагаем материалы") {
		t.Errorf("правка менеджера не дошла до модели: %q", c.lastUser)
	}
}

// Редактура тоже деградирует: не смогли — отдаём то, что написал менеджер.
// Терять его правку нельзя.
func TestEditSectionDegradesToManagerText(t *testing.T) {
	c := &fakeCopyCompleter{err: errors.New("502")}
	original := "предлагаем материалы для гидроизоляции"
	text, err := EditSection(context.Background(), skills.SectionIntro,
		original, model.DefaultContext("client"), c)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if text != original {
		t.Fatalf("правка менеджера потеряна: %q", text)
	}
}

// --- Секции в form_values ---

func TestSectionKeyNames(t *testing.T) {
	if got := SectionKey(skills.SectionIntro, 0); got != "copy.intro" {
		t.Errorf("intro: %q", got)
	}
	if got := SectionKey(skills.SectionSystem, 2); got != "copy.system.2" {
		t.Errorf("system: %q", got)
	}
	if got := SectionKey(skills.SectionAdvantages, 0); got != "copy.advantages" {
		t.Errorf("advantages: %q", got)
	}
	if got := SectionKey(skills.SectionConditions, 0); got != "copy.conditions" {
		t.Errorf("conditions: %q", got)
	}
}

// Секции систем нужны по одной на каждую систему предложения: у двух систем
// не может быть одного текста.
func TestSectionSlotsPerSystem(t *testing.T) {
	offer := sampleSectionOffer()
	offer.Items = nil // системы считаются по form_values — здесь проверяем только фиксированные
	slots := SectionSlots(offer, 3)
	var systems int
	for _, s := range slots {
		if s.Kind == skills.SectionSystem {
			systems++
		}
	}
	if systems != 3 {
		t.Fatalf("ожидали 3 секции систем, получили %d", systems)
	}
}

// Принятый текст кладётся в form_values и НЕ затирает остальные значения
// формы.
func TestAcceptSectionKeepsOtherFormValues(t *testing.T) {
	offer := sampleSectionOffer()
	offer.FormValues = map[string]string{"valid_until": "2026-12-31"}
	offer = AcceptSection(offer, skills.SectionIntro, 0, "Предлагаем материалы.")
	if offer.FormValues["valid_until"] != "2026-12-31" {
		t.Fatal("остальные поля формы потеряны")
	}
	if offer.FormValues["copy.intro"] != "Предлагаем материалы." {
		t.Fatalf("текст не записан: %q", offer.FormValues["copy.intro"])
	}
}

// Принятие пустой секции («Пропустить») удаляет ключ, а не пишет пустоту:
// иначе сборка документа не отличит «менеджер пропустил» от «текста нет».
func TestAcceptEmptySectionRemovesKey(t *testing.T) {
	offer := sampleSectionOffer()
	offer.FormValues = map[string]string{"copy.intro": "старый текст"}
	offer = AcceptSection(offer, skills.SectionIntro, 0, "   ")
	if _, ok := offer.FormValues["copy.intro"]; ok {
		t.Fatal("пустая секция должна убирать ключ, а не писать пустую строку")
	}
}

// Секции читаются обратно из form_values — второй копии нигде нет.
func TestSectionTextReadsFromFormValues(t *testing.T) {
	offer := sampleSectionOffer()
	offer.FormValues = map[string]string{"copy.system.1": "Система для фундамента."}
	if got := SectionText(offer, skills.SectionSystem, 1); got != "Система для фундамента." {
		t.Fatalf("текст не прочитан: %q", got)
	}
	if got := SectionText(offer, skills.SectionSystem, 2); got != "" {
		t.Fatalf("у второй системы текста быть не должно: %q", got)
	}
}

// --- Текст секций в документе ---

// Принятый текст секции заменяет блок формы в документе.
func TestDocxUsesAcceptedSection(t *testing.T) {
	offer := sampleOffer()
	offer.FormValues = map[string]string{"copy.intro": "Предлагаем материалы для фундамента."}
	docx, err := renderOfferDocx(OfferForm{Title: "Коммерческое предложение"}, offer)
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	body := string(docxEntry(t, docx, "word/document.xml"))
	if !strings.Contains(body, "Предлагаем материалы для фундамента.") {
		t.Fatal("принятый текст секции не попал в документ")
	}
}

// Секция «преимущества» рисуется списком, а не абзацем.
func TestDocxAdvantagesAsList(t *testing.T) {
	offer := sampleOffer()
	offer.FormValues = map[string]string{
		"copy.advantages": "• Служит 50 лет\n• Выдерживает давление грунтовых вод",
	}
	docx, err := renderOfferDocx(OfferForm{Title: "Коммерческое предложение"}, offer)
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	body := string(docxEntry(t, docx, "word/document.xml"))
	if !strings.Contains(body, `numId w:val="`+styleListBullet+`"`) {
		t.Fatal("преимущества не оформлены списком бланка")
	}
	if !strings.Contains(body, "Служит 50 лет") {
		t.Fatal("первый пункт потерян")
	}
}

// Пустой текст секции — документ собирается как раньше, без ошибки.
func TestDocxEmptySectionIsNoop(t *testing.T) {
	offer := sampleOffer()
	offer.FormValues = map[string]string{"copy.intro": ""}
	if _, err := renderOfferDocx(OfferForm{Title: "Коммерческое предложение"}, offer); err != nil {
		t.Fatalf("пустая секция не должна ломать сборку: %v", err)
	}
}
