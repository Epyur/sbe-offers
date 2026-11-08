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
	values, reused, written, _ := generateBlockTexts(context.Background(), promptForm(), testOfferForBlocks(), c, nil, nil)
	if written != 1 || reused != 0 {
		t.Fatalf("ожидали 1 новый текст, получили reused=%d written=%d", reused, written)
	}
	if got := values[genKeyTextByID("desc", 0, "")]; got != "Система укладывается в два слоя." {
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
	values, _, _, _ := generateBlockTexts(context.Background(), promptForm(), offer, first, nil, nil)
	offer.FormValues = values

	second := &fakeCompleter{answer: "Другой текст."}
	got, reused, written, _ := generateBlockTexts(context.Background(), promptForm(), offer, second, nil, nil)
	if second.calls != 0 {
		t.Fatalf("модель вызвана повторно (%d раз) при свежем кэше", second.calls)
	}
	if reused != 1 || written != 0 {
		t.Fatalf("reused=%d written=%d", reused, written)
	}
	if got[genKeyTextByID("desc", 0, "")] != "Текст." {
		t.Fatalf("текст из кэша не сохранён: %q", got[genKeyTextByID("desc", 0, "")])
	}
}

// Правка состава стирает кэш: старый текст говорит про старые суммы.
func TestGenerateBlockTextsInvalidatesOnOfferChange(t *testing.T) {
	offer := testOfferForBlocks()
	values, _, _, _ := generateBlockTexts(context.Background(), promptForm(), offer, &fakeCompleter{answer: "Старый."}, nil, nil)
	offer.FormValues = values
	// Меняем цену — данные, которые видел прошлый текст.
	offer.Items[0].Price = 999
	offer.Items[0].Sum = 99900
	offer.Total = 99900

	c := &fakeCompleter{answer: "Новый."}
	got, reused, written, _ := generateBlockTexts(context.Background(), promptForm(), offer, c, nil, nil)
	if c.calls != 1 || written != 1 || reused != 0 {
		t.Fatalf("при изменившемся предложении текст должен быть переписан: calls=%d written=%d reused=%d", c.calls, written, reused)
	}
	if got[genKeyTextByID("desc", 0, "")] != "Новый." {
		t.Fatalf("текст не обновлён: %q", got[genKeyTextByID("desc", 0, "")])
	}
}

// Модель упала — блок молчит, остальные значения формы не теряются.
func TestGenerateBlockTextsDegradesOnModelError(t *testing.T) {
	offer := testOfferForBlocks()
	offer.FormValues["client_name"] = "ООО Заказчик"
	got, _, written, _ := generateBlockTexts(context.Background(), promptForm(), offer,
		&fakeCompleter{err: errors.New("502")}, nil, nil)
	if written != 0 {
		t.Fatalf("на ошибке модели текст не должен появляться: %d", written)
	}
	if got[genKeyTextByID("desc", 0, "")] != "" {
		t.Fatalf("ключ текста оставлен: %q", got[genKeyTextByID("desc", 0, "")])
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
	_, _, written, _ := generateBlockTexts(context.Background(), form, testOfferForBlocks(), c, nil, nil)
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
	values, _, _, _ := generateBlockTexts(context.Background(), form, testOfferForBlocks(), c, nil, nil)
	if values[genKeyTextByID("desc", 0, "")] == "" {
		t.Fatalf("текст блока не найден под своим id: %#v", values)
	}
}

// Размышление вслух выбрасывается (2026-11-03).
//
// Живой случай, КП 003/2026: модель вернула не текст, а свой черновик —
// английские строки о задании, перебор того, что известно, оценки собственных
// фраз («That's a real property», «Считаю допустимым»), причём вперемешку с
// русскими пересказами состава. Всё это ушло в документ клиенту. Промпт
// исправлен, но граница должна быть и в коде: ответ печатается без человека.
func TestCleanBlockTextDropsInterleavedReasoning(t *testing.T) {
	raw := `The task: write a block of a commercial proposal. Under 150 words.

Brief info: no explicit object description given. I need to write advantages.

- ПВХ мембрана — гидроизоляция подземной части; сварка швов даёт контур... hmm, "сварка" is not in the brief. Better to be careful.
- Гидрошпонки на деформационных швах — узлы остаются герметичными при подвижках.

Word limit 150. Need paragraphs, no bullets preferably.

Header: "заголовки должны утверждать" — but format says "без заголовка". So no heading.

Подземная часть защищена по всей высоте и по всем узлам.`
	got := cleanBlockText(raw)
	for _, bad := range []string{"The task", "Brief info", "Word limit", "Header:", "hmm", "Better to be", "So no heading"} {
		if strings.Contains(got, bad) {
			t.Fatalf("в тексте осталось %q:\n%s", bad, got)
		}
	}
	// Содержание при этом не теряется: это и есть смысл ответа.
	if !strings.Contains(got, "гидроизоляция подземной части") {
		t.Fatalf("пропал полезный текст:\n%s", got)
	}
	if !strings.Contains(got, "Гидрошпонки на деформационных швах") {
		t.Fatalf("пропала вторая строка ответа:\n%s", got)
	}
	if !strings.Contains(got, "Подземная часть защищена") {
		t.Fatalf("пропал последний абзац:\n%s", got)
	}
}

// Готовый текст модели не трогается: чистка ищет признаки рассуждения, а не
// «что-нибудь латинское». Иначе обычная марка материала съедала бы абзац.
func TestCleanBlockTextKeepsRealText(t *testing.T) {
	text := "Мембрана Logicbase V-SL держит воду под давлением. Заказчик получает сухой подвал на сдаче."
	if got := cleanBlockText(text); got != text {
		t.Fatalf("готовый текст изменился: %q", got)
	}
}

// Латинские слова в обычной фразе — не рассуждение: русских слов в строке
// всё равно больше.
func TestCleanBlockTextKeepsLineWithBrandNames(t *testing.T) {
	text := "Утеплитель XPS Carbon Prof укладывается на глубину промерзания, компенсатор 50х100 мм."
	if got := cleanBlockText(text); got != text {
		t.Fatalf("строка с марками материалов изменилась: %q", got)
	}
}

// Ответ целиком по-английски — это не блок для русского КП: блок молчит, как и
// при недоступной модели. Пустой блок честнее английского черновика.
func TestCleanBlockTextAllReasoningGivesEmpty(t *testing.T) {
	raw := "The task: write a block.\n\nLet me think about what's actually known.\n\nStill reasoning only."
	if got := cleanBlockText(raw); got != "" {
		t.Fatalf("ожидали пустой текст, получили %q", got)
	}
}

// Пометка о нехватке факта НЕ остаётся в тексте для клиента (переписано
// 2026-11-05).
//
// До этого тест закреплял обратное: «пометка — не рассуждение, оставляем, её
// увидит менеджер». Живое КП 003/2026 показало, что её читает заказчик.
// Решение изменено; рассуждение по-прежнему вырезается, полезный текст
// сохраняется.
func TestCleanBlockTextKeepsNeedMarker(t *testing.T) {
	raw := "Let me think first.\n\nМембрана укладывается в два слоя. [НУЖНО: марка грунта на объекте]"
	got := cleanBlockText(raw)
	if strings.Contains(got, "НУЖНО") {
		t.Fatalf("пометка осталась в тексте для клиента: %q", got)
	}
	if strings.Contains(got, "Let me") {
		t.Fatalf("рассуждение осталось: %q", got)
	}
	if !strings.Contains(got, "два слоя") {
		t.Fatalf("полезный текст потерян: %q", got)
	}
}
