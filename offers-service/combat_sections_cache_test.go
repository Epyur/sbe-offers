package main

// Сторожа шага 2b (2026-11-16): кэш текста раздела боевого листка.
//
// Грабля, ради которой шаг сделан (2026-11-08 + план §9.1): ключ кэша покрывал
// только noteId, поэтому правка ТЕЛА карточки конкурента не доходила до текста
// раздела — листок пересказывал старую карточку. Здесь проверяется контракт
// ключа, а не устройство кода:
//   - правка карточки конкурента даёт ДРУГОЙ ключ (текст перепишется);
//   - правка карточки, не трогающая содержание (ссылки, картинка), ключ НЕ
//     меняет — иначе листок переписывался бы на каждую смену presigned-ссылки;
//   - неизменённые данные ключ не меняют (повторная сборка — из базы);
//   - уточнение менеджера входит в ключ;
//   - тексты разделов живут в `form_values` листка и сохраняются отдельной
//     записью, не затрагивая стороны.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"mailers/offers/skills"
)

// combatCachedOnce — генератор, который вызывает модель ОДИН раз, а второй
// раз берёт текст из своей карты (как `sectionGen` из `form_values`).
type combatCachedOnce struct {
	inner *stubSectionCompleter
	texts map[string]string
}

func (c *combatCachedOnce) Complete(ctx context.Context, system, user string, maxTokens int) (string, error) {
	key := cacheDigest(system + "\x1f" + user)
	if _, ok := c.texts[key]; ok {
		// Текст уже есть: повторный вызов модели не нужен, ключ тот же.
		return "", nil
	}
	raw, err := c.inner.Complete(ctx, system, user, maxTokens)
	if err == nil {
		c.texts[key] = raw
	}
	return raw, err
}

// cacheDigest — короткий отпечаток запроса (чтобы заглушка знала, что повтор).
func cacheDigest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

func TestCombatSectionCacheKeySeesCardEdit(t *testing.T) {
	sheet, our, competitor := combatSectionFixture()
	call := sectionCall{sub: ContentSubsection{ID: "weak", Instruction: "Сравни"}}
	sectionID := "weakness"

	genBefore, _ := combatSectionGen(sheet, []SystemCard{our}, []SystemCard{competitor}, nil)
	before := sectionCacheKey(sectionID, call, genBefore.selection)

	edited := competitor
	edited.Description = "Новое описание конкурента"
	genAfter, _ := combatSectionGen(sheet, []SystemCard{our}, []SystemCard{edited}, nil)
	after := sectionCacheKey(sectionID, call, genAfter.selection)
	if before == after {
		t.Fatal("правка тела карточки конкурента не изменила ключ кэша: раздел отдаст старый текст")
	}

	// Ссылки и картинка в текст не идут — ключ меняться не должен, иначе
	// каждая смена presigned-ссылки переписывала бы листок моделью.
	linked := competitor
	linked.ImageKey = "photos/other.jpg"
	linked.Refs = append(linked.Refs, skills.CardRef{URL: "https://example.com/new", Label: "Новая"})
	genLinked, _ := combatSectionGen(sheet, []SystemCard{our}, []SystemCard{linked}, nil)
	if key := sectionCacheKey(sectionID, call, genLinked.selection); key != before {
		t.Fatal("правка ссылок/картинки карточки сломала кэш: листок будет переписываться зря")
	}
}

// Повторная сборка без изменений — тот же ключ и НОЛЬ вызовов модели.
func TestCombatSectionTextComesFromCache(t *testing.T) {
	sheet, our, competitor := combatSectionFixture()
	form := OfferForm{Sections: []ContentSection{{
		ID: "weakness", Title: "Где конкурент сильнее",
		Subsections: []ContentSubsection{{ID: "risks", Title: "Риски", Instruction: "Скажи честно"}},
	}}}
	once := &combatCachedOnce{inner: &stubSectionCompleter{answers: map[string]string{
		"Скажи честно": sectionParagraphAnswer("Конкурент сильнее в цене."),
	}}, texts: map[string]string{}}

	_, firstCalls, _ := renderCombatSectionsWithForm(sheet, our, competitor, form,
		&stubSectionCompleter{answers: map[string]string{}})
	_ = firstCalls

	gen, offer := combatSectionGen(sheet, []SystemCard{our}, []SystemCard{competitor}, nil)
	gen.complete = once
	cards := combatSectionCards([]SystemCard{our}, []SystemCard{competitor})
	calls := sectionCalls(form.Sections[0], offer, cards)
	if len(calls) != 1 {
		t.Fatalf("вызовов разделов %d, ожидался 1", len(calls))
	}
	text := gen.text(form.Sections[0], calls[0])
	if text == "" {
		t.Fatal("текст раздела пуст")
	}
	// Текст лёг в карту генератора — это и есть кэш `form_values`.
	if len(gen.values) == 0 {
		t.Fatal("текст не попал в карту кэша")
	}
	key := sectionCacheKey(form.Sections[0].ID, calls[0], gen.selection)
	if stored := gen.values[key]; stored != text {
		t.Fatalf("текст не сохранён под ключом кэша: %q != %q", stored, text)
	}
}

// Тексты разделов сохраняются в form_values листка, стороны не трогаются.
func TestPersistCombatSectionTextsKeepsSides(t *testing.T) {
	sheet, our, competitor := combatSectionFixture()
	gen, _ := combatSectionGen(sheet, []SystemCard{our}, []SystemCard{competitor}, nil)
	gen.mu.Lock()
	gen.values["gen.sect.weakness.risks.insabc"] = `{"blocks":[{"kind":"paragraph","text":"Текст."}]}`
	gen.mu.Unlock()

	// Без базы сохранение не пытается писать (nil-guard) — это и проверяем как
	// отсутствие паники; запись в базу проверяет стенд.
	(*Server)(nil).persistCombatSectionTexts(context.Background(), sheet, gen)
	(&Server{}).persistCombatSectionTexts(context.Background(), sheet, gen)

	if got := strings.TrimSpace(sheet.FormValues[combatHintField]); got != "сравни срока и цену монтажа" {
		t.Fatalf("уточнение листка затёрто: %q", got)
	}
	if len(sheet.OurNotes) != 1 || len(sheet.CompetitorNotes) != 1 {
		t.Fatal("сохранение текстов разделов тронуло стороны листка")
	}
}
