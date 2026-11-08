package main

import (
	"context"
	"strings"
	"testing"
)

// Повтор блока по системам: у каждой системы СВОЙ текст и СВОЯ нумерация
// состава (2026-11-08, живая жалоба на КП 003/2026).
//
// Две ошибки одной природы — «данные одной системы показывались для всех»:
//   - нумерация слоёв не перезапускалась (вторая система начиналась с 13);
//   - текст по промту второй системы повторял текст первой, потому что кэш
//     узла не знал про систему.
//
// Эти тесты стерегут именно повтор: блок с repeat:"systems" печатается по
// разу на систему, и каждый проход обязан брать данные СВОЕЙ системы.

// twoSystemCards — две разные системы в кеше и в выборе предложения.
func twoSystemCards() ([]SystemCard, Offer) {
	cards := []SystemCard{
		{NoteID: 1, Title: "ТН-ФУНДАМЕНТ", Composition: []string{"Слой фундамента A", "Слой фундамента B"}},
		{NoteID: 2, Title: "ТН-СТИЛОБАТ", Composition: []string{"Слой стилобата A", "Слой стилобата B"}},
	}
	offer := Offer{
		Number:  "003/2026",
		Systems: []OfferSystem{{NoteID: 1}, {NoteID: 2}},
		Items:   []OfferItem{{Name: "Мембрана", Unit: "м2", Qty: 1, Price: 1, Sum: 1}},
		Total:   1,
	}
	return cards, offer
}

// echoTitleCompleter возвращает ответ, называющий систему из подсказки, чтобы
// тест видел, какой карточке достался вызов.
type echoTitleCompleter struct {
	seen []string
}

func (c *echoTitleCompleter) Complete(_ context.Context, _ string, user string, _ int) (string, error) {
	title := "неизвестно"
	for _, t := range []string{"ТН-ФУНДАМЕНТ", "ТН-СТИЛОБАТ"} {
		if strings.Contains(user, t) {
			title = t
		}
	}
	c.seen = append(c.seen, title)
	return "Описание системы " + title + ".", nil
}

// Узел по промту в блоке с повтором пишется ОТДЕЛЬНО для каждой системы, и в
// промпт уходит ТОЛЬКО карточка этой системы.
func TestRepeatedBlockPromptTextIsPerSystem(t *testing.T) {
	cards, offer := twoSystemCards()
	form := OfferForm{
		Mode: "client",
		Blocks: []Block{{
			ID:           "main",
			Repeat:       blockRepeatSystems,
			ShowInClient: true,
			Content:      []byte(`[{"type":"prompt_text","instruction":"Опиши систему","maxWords":60}]`),
		}},
	}
	c := &echoTitleCompleter{}
	values, _, written, failed := generateBlockTexts(context.Background(), form, offer, c, cards, nil)
	if failed != 0 {
		t.Fatalf("модель «не справилась» %d раз", failed)
	}
	if written != 2 {
		t.Fatalf("написано текстов %d, ожидалось два (по одному на систему)", written)
	}
	// В ключе текста теперь есть и набор систем (2026-11-08): без него блок
	// продолжал бы печатать текст, написанный до добавления системы.
	selection := offerSelectionHandle(offer)
	first := values[genKeyTextForSystem("main", 0, "1", selection)]
	second := values[genKeyTextForSystem("main", 0, "2", selection)]
	if first == "" || second == "" {
		t.Fatalf("текст системы не записан: %q / %q", first, second)
	}
	if first == second {
		t.Fatalf("у двух систем один и тот же текст: %q", first)
	}
	if !strings.Contains(first, "ТН-ФУНДАМЕНТ") || !strings.Contains(second, "ТН-СТИЛОБАТ") {
		t.Fatalf("текст не относится к своей системе: %q / %q", first, second)
	}
	// В промпт каждой системы ушла ТОЛЬКО её карточка: иначе модель снова
	// описала бы «одну из двух» и подменила бы смысл. Один вызов — одна система.
	if len(c.seen) != 2 {
		t.Fatalf("модель звана %d раз, ожидалось два", len(c.seen))
	}
}

// Обычный (неповторяемый) блок по-прежнему пишет ОДИН текст под общим ключом.
func TestSingleBlockPromptTextStaysSingle(t *testing.T) {
	cards, offer := twoSystemCards()
	form := OfferForm{
		Mode: "client",
		Blocks: []Block{{
			ID:           "intro",
			ShowInClient: true,
			Content:      []byte(`[{"type":"prompt_text","instruction":"Напиши введение","maxWords":60}]`),
		}},
	}
	c := &fakeCompleter{answer: "Введение."}
	values, _, written, _ := generateBlockTexts(context.Background(), form, offer, c, cards, nil)
	if written != 1 {
		t.Fatalf("написано текстов %d, ожидался один", written)
	}
	// Один текст на всё предложение, но под набором систем: сменился набор —
	// вступление пишется заново (2026-11-08).
	introKey := genKeyTextByID("intro", 0, offerSelectionHandle(offer))
	if values[introKey] != "Введение." {
		t.Fatalf("текст введения не записан: %q", values[introKey])
	}
	if values[genKeyTextForSystem("intro", 0, "1", offerSelectionHandle(offer))] != "" {
		t.Fatal("обычный блок написал посистемный текст — ключ не должен зависеть от системы")
	}
	if values[genKeyTextByID("intro", 0, "")] != "" {
		t.Fatal("блок без выбора систем написал текст под общим ключом — кэш пережил бы смену набора")
	}
}

// Состав каждой системы получает СВОЙ numId: иначе нумерация второй системы
// продолжает первую (слои идут с 13, а не с 1).
func TestCompositionNumberingRestartsPerSystem(t *testing.T) {
	cards, offer := twoSystemCards()
	lc := layoutContext{
		offer:   offer,
		values:  map[string]string{},
		images:  &fakeImages{data: pngBytes()},
		cards:   func() []SystemCard { return cards },
		service: map[string]string{},
	}
	lc.form = func() OfferForm {
		return OfferForm{Blocks: []Block{{
			ID:           "main",
			Repeat:       blockRepeatSystems,
			ShowInClient: true,
			Content:      []byte(`[{"type":"paragraph","text":"{{система: таблица}}"}]`),
		}}}
	}
	doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>{{block:main}}</w:t></w:r></w:p>`))
	out, err := renderLayoutDocx(doc, lc)
	if err != nil {
		t.Fatalf("печать: %v", err)
	}
	body := string(docxEntry(t, out, "word/document.xml"))
	numIDs := compositionNumIDs(body)
	if len(numIDs) != 2 {
		t.Fatalf("у составов %d разных numId, ожидалось два:\n%s", len(numIDs), body)
	}
	for _, id := range numIDs {
		if id < numberingUserBase {
			t.Fatalf("состав ссылается на numId=%d — не наш список из плана", id)
		}
	}
	numbering := string(docxEntry(t, out, "word/numbering.xml"))
	for _, id := range numIDs {
		def := `w:numId="` + itoa(id) + `"`
		idx := strings.Index(numbering, def)
		if idx < 0 {
			t.Fatalf("в numbering.xml нет определения numId=%d", id)
		}
		// Перезапуск обязателен: без startOverride новый numId продолжил бы счёт.
		tail := numbering[idx:]
		if end := strings.Index(tail, "</w:num>"); end > 0 {
			tail = tail[:end]
		}
		if !strings.Contains(tail, `<w:startOverride w:val="1"/>`) {
			t.Fatalf("у numId=%d нет перезапуска нумерации: %s", id, tail)
		}
	}
}

// compositionNumIDs — уникальные numId в документе из диапазона нашего плана.
func compositionNumIDs(body string) []int {
	seen := map[int]bool{}
	var out []int
	for _, m := range reNumIDUse.FindAllStringSubmatch(body, -1) {
		id := atoiOr(m[1], 0)
		if id >= numberingUserBase && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// atoiOr — разбор строки в число с запасным значением (тестовая мелочь).
func atoiOr(s string, def int) int {
	n := 0
	if s == "" {
		return def
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
	}
	return n
}
