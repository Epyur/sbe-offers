package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Текст блока по промту (2026-11-01): генерация, кэш и деградация.
//
// Правила, которые здесь зафиксированы:
//   - написанное ложится в form_values и печатается следующей сборкой;
//   - правка состава стирает кэш — иначе в документе останутся старые цифры;
//   - модель недоступна — блок молчит, документ собирается.

func promptForm() OfferForm {
	return OfferForm{
		Mode:  "client",
		Title: "КП",
		Blocks: []Block{{
			ID:           "desc",
			Title:        "Описание системы",
			ShowInClient: true,
			Content:      []byte(`[{"type":"prompt_text","instruction":"Опиши систему и материалы","maxWords":60}]`),
		}},
	}
}

func TestGenerateBlockTextsWritesText(t *testing.T) {
	c := &fakeCompleter{answer: "Система укладывается в два слоя."}
	values, reused, written := generateBlockTexts(context.Background(), promptForm(), testOfferForBlocks(), c, nil, nil)
	if written != 1 || reused != 0 {
		t.Fatalf("ожидали 1 новый текст, получили reused=%d written=%d", reused, written)
	}
	if got := values[genKeyTextByID("desc", 0)]; got != "Система укладывается в два слоя." {
		t.Fatalf("текст не записан: %q", got)
	}
	if values[genKeySection] == "" {
		t.Fatal("хеш данных предложения не записан — кэш будет считаться устаревшим всегда")
	}
	if !strings.Contains(c.lastSys, "Опиши систему и материалы") {
		t.Fatalf("подсказка администратора не попала в промпт: %q", c.lastSys)
	}
}

// Второй прогон не зовёт модель: текст берётся из кэша.
func TestGenerateBlockTextsReusesCache(t *testing.T) {
	offer := testOfferForBlocks()
	first := &fakeCompleter{answer: "Текст."}
	values, _, _ := generateBlockTexts(context.Background(), promptForm(), offer, first, nil, nil)
	offer.FormValues = values

	second := &fakeCompleter{answer: "Другой текст."}
	got, reused, written := generateBlockTexts(context.Background(), promptForm(), offer, second, nil, nil)
	if second.calls != 0 {
		t.Fatalf("модель вызвана повторно (%d раз) при свежем кэше", second.calls)
	}
	if reused != 1 || written != 0 {
		t.Fatalf("reused=%d written=%d", reused, written)
	}
	if got[genKeyTextByID("desc", 0)] != "Текст." {
		t.Fatalf("текст из кэша не сохранён: %q", got[genKeyTextByID("desc", 0)])
	}
}

// Правка состава стирает кэш: старый текст говорит про старые суммы.
func TestGenerateBlockTextsInvalidatesOnOfferChange(t *testing.T) {
	offer := testOfferForBlocks()
	values, _, _ := generateBlockTexts(context.Background(), promptForm(), offer, &fakeCompleter{answer: "Старый."}, nil, nil)
	offer.FormValues = values
	// Меняем цену — данные, которые видел прошлый текст.
	offer.Items[0].Price = 999
	offer.Items[0].Sum = 99900
	offer.Total = 99900

	c := &fakeCompleter{answer: "Новый."}
	got, reused, written := generateBlockTexts(context.Background(), promptForm(), offer, c, nil, nil)
	if c.calls != 1 || written != 1 || reused != 0 {
		t.Fatalf("при изменившемся предложении текст должен быть переписан: calls=%d written=%d reused=%d", c.calls, written, reused)
	}
	if got[genKeyTextByID("desc", 0)] != "Новый." {
		t.Fatalf("текст не обновлён: %q", got[genKeyTextByID("desc", 0)])
	}
}

// Модель упала — блок молчит, остальные значения формы не теряются.
func TestGenerateBlockTextsDegradesOnModelError(t *testing.T) {
	offer := testOfferForBlocks()
	offer.FormValues["client_name"] = "ООО Заказчик"
	got, _, written := generateBlockTexts(context.Background(), promptForm(), offer,
		&fakeCompleter{err: errors.New("502")}, nil, nil)
	if written != 0 {
		t.Fatalf("на ошибке модели текст не должен появляться: %d", written)
	}
	if got[genKeyTextByID("desc", 0)] != "" {
		t.Fatalf("ключ текста оставлен: %q", got[genKeyTextByID("desc", 0)])
	}
	if got["client_name"] != "ООО Заказчик" {
		t.Fatal("прочие значения формы потерялись")
	}
}

// Блоки, которые клиенту не показывают, модель не пишет: платить за текст,
// которого не будет в файле, не нужно.
func TestGenerateBlockTextsSkipsInternalBlocks(t *testing.T) {
	form := promptForm()
	form.Blocks[0].ShowInClient = false
	c := &fakeCompleter{answer: "Не нужно."}
	_, _, written := generateBlockTexts(context.Background(), form, testOfferForBlocks(), c, nil, nil)
	if c.calls != 0 || written != 0 {
		t.Fatalf("внутренний блок не должен генерироваться: calls=%d written=%d", c.calls, written)
	}
}

// Кэш живёт по идентификатору блока, а не по индексу: перестановка блоков в
// редакторе не должна стирать тексты.
func TestGenerateBlockTextsKeyedByBlockID(t *testing.T) {
	form := promptForm()
	form.Blocks = append([]Block{{
		ID:           "cover",
		Title:        "Титул",
		ShowInClient: true,
		Content:      []byte(`[{"type":"paragraph","text":"Титул"}]`),
	}}, form.Blocks...)
	c := &fakeCompleter{answer: "Текст."}
	values, _, _ := generateBlockTexts(context.Background(), form, testOfferForBlocks(), c, nil, nil)
	if values[genKeyTextByID("desc", 0)] == "" {
		t.Fatalf("текст блока не найден под своим id: %#v", values)
	}
}
