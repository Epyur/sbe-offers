package main

import (
	"context"
	"fmt"
	"strings"

	"mailers/offers/model"
	"mailers/offers/skills"
)

// Текст секций КП (Task 6 плана 2026-10-29).
//
// Секции пишет модель, менеджер принимает или правит. Согласованный текст
// живёт ТОЛЬКО в form_values["copy.<ключ>"] — второй копии в сессии бота нет:
// после перезапуска она потерялась бы, и менеджер увидел бы старый текст.
//
// Деградация сквозная: если модель недоступна, секция просто пустая и КП
// собирается с текстом формы. Документ не должен падать из-за модели.

// SectionKey — ключ секции в form_values.
//
// Системы нумеруются: у двух систем один и тот же текст быть не может.
// Фиксированные секции без номера.
func SectionKey(kind skills.SectionKind, systemIndex int) string {
	if kind == skills.SectionSystem {
		return fmt.Sprintf("copy.system.%d", systemIndex)
	}
	return "copy." + string(kind)
}

// SectionSlot — одна секция, которую предстоит согласовать.
type SectionSlot struct {
	Kind        skills.SectionKind
	SystemIndex int // только для SectionSystem
	Key         string
	Label       string
}

// SectionSlots — порядок согласования: вводный текст, описания систем,
// преимущества, условия.
func SectionSlots(offer model.Offer, systems int) []SectionSlot {
	slots := make([]SectionSlot, 0, systems+3)
	slots = append(slots, SectionSlot{
		Kind: skills.SectionIntro, Key: SectionKey(skills.SectionIntro, 0), Label: "Вводный текст",
	})
	for i := 1; i <= systems; i++ {
		slots = append(slots, SectionSlot{
			Kind:        skills.SectionSystem,
			SystemIndex: i,
			Key:         SectionKey(skills.SectionSystem, i),
			Label:       fmt.Sprintf("Описание системы %d", i),
		})
	}
	slots = append(slots,
		SectionSlot{Kind: skills.SectionAdvantages, Key: SectionKey(skills.SectionAdvantages, 0), Label: "Преимущества"},
		SectionSlot{Kind: skills.SectionConditions, Key: SectionKey(skills.SectionConditions, 0), Label: "Условия"},
	)
	return slots
}

// SectionText — согласованный текст секции (пусто, если его нет).
func SectionText(offer model.Offer, kind skills.SectionKind, systemIndex int) string {
	if offer.FormValues == nil {
		return ""
	}
	return strings.TrimSpace(offer.FormValues[SectionKey(kind, systemIndex)])
}

// AcceptSection — менеджер принял текст. Пустой текст означает «пропустить»:
// ключ удаляется, чтобы сборка документа отличала «пропущено» от «пусто».
func AcceptSection(offer model.Offer, kind skills.SectionKind, systemIndex int, text string) model.Offer {
	if offer.FormValues == nil {
		offer.FormValues = map[string]string{}
	}
	key := SectionKey(kind, systemIndex)
	if strings.TrimSpace(text) == "" {
		delete(offer.FormValues, key)
		return offer
	}
	offer.FormValues[key] = strings.TrimSpace(text)
	return offer
}

// SectionResult — текст и замечания чек-листа.
type SectionResult struct {
	Text     string
	Warnings []string
}

// WriteSection — модель пишет секцию. Ошибка модели НЕ является ошибкой:
// возвращается пустой текст, и КП собирается без написанной секции.
func WriteSection(ctx context.Context, kind skills.SectionKind, brief model.BriefPayload,
	offer model.Offer, pc model.ProductContext, c Completer) (string, error) {
	res, err := WriteSectionChecked(ctx, kind, brief, offer, pc, c)
	return res.Text, err
}

// WriteSectionChecked — то же, но с замечаниями чек-листа.
//
// Чек-лист только СООБЩАЕТ о проблемах: текст не переписывается молча.
// Правку делает человек — иначе менеджер не будет понимать, почему КП
// говорит не то, что он подтверждал.
func WriteSectionChecked(ctx context.Context, kind skills.SectionKind, brief model.BriefPayload,
	offer model.Offer, pc model.ProductContext, c Completer) (SectionResult, error) {
	system := skills.RenderCopySystem(kind, brief, offer, pc)
	user := buildCopyUserPrompt(kind, brief, offer)

	raw, err := c.Complete(ctx, system, user, 1200)
	if err != nil {
		// Недоступность модели и авария провайдера — одинаково деградация:
		// КП важнее текста.
		return SectionResult{}, nil
	}
	text := strings.TrimSpace(raw)
	if text == "" {
		return SectionResult{}, nil
	}
	return SectionResult{Text: text, Warnings: copyWarnings(text, kind, offer)}, nil
}

// EditSection — менеджер прислал свою правку, модель её вычитывает.
//
// При любой ошибке возвращается ТЕКСТ МЕНЕДЖЕРА: его правку терять нельзя.
func EditSection(ctx context.Context, kind skills.SectionKind, managerText string,
	pc model.ProductContext, c Completer) (string, error) {
	original := strings.TrimSpace(managerText)
	if original == "" {
		return "", nil
	}
	system := skills.RenderEditSystem(kind, pc)
	raw, err := c.Complete(ctx, system, original, 1200)
	if err != nil {
		return original, nil
	}
	edited := strings.TrimSpace(raw)
	if edited == "" {
		return original, nil
	}
	return edited, nil
}

// copyWarnings — замечания чек-листов к готовому тексту.
func copyWarnings(text string, kind skills.SectionKind, offer model.Offer) []string {
	out := append([]string{}, skills.CheckAiTells(text)...)
	out = append(out, skills.CheckSectionStructure(text, kind)...)
	out = append(out, skills.CheckClaimNumbers(text, allowedNumbers(offer))...)
	return out
}

// allowedNumbers — числа предложения в формате документа. Только их модель
// имеет право использовать: всё остальное — выдумка, и её увидит менеджер.
func allowedNumbers(offer model.Offer) []string {
	out := []string{
		skills.FormatMoney(offer.Total),
		offer.Number,
	}
	for _, it := range offer.Items {
		out = append(out,
			skills.FormatMoney(it.Price),
			skills.FormatMoney(it.Sum),
			formatQtyValue(it.Qty),
		)
	}
	return out
}

// formatQtyValue — количество без хвостов плавающей точки.
func formatQtyValue(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", v), "0"), ".")
}

// buildCopyUserPrompt — что модель получает на входе. Числа и состав идут
// готовыми строками: считает сервер, а не модель.
func buildCopyUserPrompt(kind skills.SectionKind, brief model.BriefPayload, offer model.Offer) string {
	var b strings.Builder
	b.WriteString("СЕКЦИЯ: " + string(kind) + "\n\n")
	b.WriteString("ПРЕДЛОЖЕНИЕ:\n")
	if raw, err := marshalBriefPayload(brief); err == nil {
		b.WriteString("бриф (JSON): " + raw + "\n")
	}
	b.WriteString("номер: " + offer.Number + "\n")
	if offer.Client.Name != "" {
		b.WriteString("заказчик: " + offer.Client.Name + "\n")
	}
	b.WriteString("итого: " + skills.FormatMoney(offer.Total) + " руб.\n")
	if len(offer.Items) > 0 {
		b.WriteString("состав:\n")
		for _, it := range offer.Items {
			b.WriteString("  • " + it.Name + " — " + formatQtyValue(it.Qty) + " " + it.Unit +
				" × " + skills.FormatMoney(it.Price) + " = " + skills.FormatMoney(it.Sum) + " руб.\n")
		}
	}
	b.WriteString("\nНапиши только текст секции.")
	return b.String()
}
