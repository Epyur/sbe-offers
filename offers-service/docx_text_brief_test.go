package main

import (
	"context"
	"strings"
	"testing"
)

// Промпт блока по промту (2026-11-05).
//
// Живое КП 003/2026 показало: бриф у менеджера заполнен, а модель в ответ
// пишет «[НУЖНО: что за объект и какие грунтовые воды]». Причина не в модели и
// не в промпте — модель этих данных не получала: buildBlockUserPrompt читал
// четыре ключа из form_values без префикса `brief.`, а бриф живёт отдельно и
// под префиксом. Здесь зафиксирован контракт: модель видит весь бриф.

// briefValues — как бриф ложится в form_values: ключ с префиксом `brief.`.
func briefValues() map[string]string {
	return map[string]string{
		"brief.object_name":    "ЖК «Сахаров Парк»",
		"brief.object_purpose": "Жилые здания",
		"brief.object_area":    "7 883 м2",
		"brief.customer_goals": "сдать подвал без протечек",
		"brief.project_risks":  "высокий уровень грунтовых вод",
		"brief.timeline":       "сдача 3 квартал 2027",
		"brief.decision_maker": "технический директор",
		"brief.budget_signal":  "бюджет согласован",
		"brief.why_us":         "своя монтажная бригада",
	}
}

// Модель получает весь бриф, а не четыре поля.
func TestBlockPromptCarriesBrief(t *testing.T) {
	offer := testOfferForBlocks()
	offer.FormValues = briefValues()

	user := buildBlockUserPrompt(offer, nil, nil)
	for _, want := range []string{
		"ЖК «Сахаров Парк»",
		"Жилые здания",
		"7 883 м2",
		"высокий уровень грунтовых вод",
		"сдача 3 квартал 2027",
		"технический директор",
		"бюджет согласован",
	} {
		if !strings.Contains(user, want) {
			t.Fatalf("брифа нет в промпте, не нашли %q:\n%s", want, user)
		}
	}
}

// Старые ключи без префикса тоже понимаются: КП, собранные до перехода на
// `brief.`, не должны потерять объект.
func TestBlockPromptAcceptsBothKeyShapes(t *testing.T) {
	offer := testOfferForBlocks()
	offer.FormValues = map[string]string{"object_name": "Склад на Охотской"}

	user := buildBlockUserPrompt(offer, nil, nil)
	if !strings.Contains(user, "Склад на Охотской") {
		t.Fatalf("объект без префикса потерян:\n%s", user)
	}
}

// Пустые поля брифа не печатаются: модель не должна видеть «Риски: » и
// достраивать пустое догадкой.
func TestBlockPromptSkipsEmptyBriefFields(t *testing.T) {
	offer := testOfferForBlocks()
	offer.FormValues = map[string]string{
		"brief.object_name":   "ЖК «Сахаров Парк»",
		"brief.project_risks": "",
		"brief.timeline":      "",
	}

	user := buildBlockUserPrompt(offer, nil, nil)
	if strings.Contains(user, "Риски") {
		t.Fatalf("пустой риск напечатан как факт:\n%s", user)
	}
	if !strings.Contains(user, "ЖК «Сахаров Парк»") {
		t.Fatalf("объект потерян:\n%s", user)
	}
}

// Оценка бюджета (blockMaxTokens) осталась, но печать её больше не применяет:
// ответ обрывался на полуслове («…одним кон», КП 003/2026) именно из-за неё.
// Проверяем обе половины решения: оценка положительна, а запрос идёт без неё.
func TestBlockTokenBudgetCoversWords(t *testing.T) {
	const words = 200
	if got := blockMaxTokens(words); got < words*4 {
		t.Fatalf("оценка бюджета %d при %d словах", got, words)
	}
	if got := blockMaxTokens(0); got <= 0 {
		t.Fatalf("оценка по умолчанию не положительная: %d", got)
	}
	if got := documentTextMaxTokens(); got != 0 {
		t.Fatalf("текст документа просится с пределом %d — предел должен давать провайдер", got)
	}
}

// Явный маркер [НУЖНО: …] не должен попадать в клиентский документ: его
// ставит модель, а читает файл заказчик (живое КП 003/2026).
func TestDocumentTextDropsNeedMarkers(t *testing.T) {
	raw := "Система защищает подвал. [НУЖНО: что за объект и какие грунтовые воды]\n\n" +
		"Комплект закрывает подземную часть одним контуром."
	got := documentText(raw)
	if strings.Contains(got, "НУЖНО") {
		t.Fatalf("маркер остался в тексте документа: %q", got)
	}
	if !strings.Contains(got, "Система защищает подвал.") {
		t.Fatalf("полезный текст потерян: %q", got)
	}
	if !strings.Contains(got, "одним контуром") {
		t.Fatalf("второй абзац потерян: %q", got)
	}
}

// Маркер, занимающий весь абзац, убирает абзац целиком, а не оставляет
// пунктуацию и пустые скобки.
func TestDocumentTextDropsMarkerOnlyParagraph(t *testing.T) {
	raw := "Первое.\n\n«[НУЖНО: что за объект, какие грунтовые воды и какой срок службы требуется]»\n\nВторое."
	got := documentText(raw)
	if strings.Contains(got, "НУЖНО") || strings.Contains(got, "«»") {
		t.Fatalf("остались следы маркера: %q", got)
	}
	if !strings.Contains(got, "Первое.") || !strings.Contains(got, "Второе.") {
		t.Fatalf("потерян текст вокруг маркера: %q", got)
	}
}

// Кэш текста по промту хранит ЧИСТЫЙ текст: иначе заказчик увидит маркер при
// следующем скачивании того же предложения.
func TestGeneratedBlockTextIsClean(t *testing.T) {
	c := &fakeCompleter{answer: "Мембрана укладывается в два слоя. [НУЖНО: марка грунта]"}
	values, _, written, _ := generateBlockTexts(context.Background(), promptForm(),
		testOfferForBlocks(), c, nil, nil)
	if written != 1 {
		t.Fatalf("ожидали один текст, получили %d", written)
	}
	got := values[genKeyTextByID("desc", 0, "")]
	if strings.Contains(got, "НУЖНО") {
		t.Fatalf("маркер попал в кэш: %q", got)
	}
	if !strings.Contains(got, "два слоя") {
		t.Fatalf("текст потерян: %q", got)
	}
}

// Уже сохранённые КП (0.5.0) содержат маркер в form_values. Печать обязана
// убрать его и при чтении из кэша, а не только при генерации.
func TestRenderedPromptBlockDropsLegacyMarker(t *testing.T) {
	offer := testOfferForBlocks()
	offer.FormValues = map[string]string{
		genKeySection:                 hashOfferFacts(testOfferForBlocks()),
		genKeyTextByID("test", 0, ""): "Текст с маркером [НУЖНО: срок службы].",
	}
	docx, err := renderOfferDocxWithCards(formWithBlock(
		`[{"type":"prompt_text","instruction":"Опиши систему","maxWords":60}]`),
		offer, nil, nil)
	if err != nil {
		t.Fatalf("сборка: %v", err)
	}
	body := string(docxEntry(t, docx, "word/document.xml"))
	if strings.Contains(body, "НУЖНО") {
		t.Fatal("маркер из старого кэша попал в документ")
	}
	if !strings.Contains(body, "Текст с маркером") {
		t.Fatal("текст из кэша потерян")
	}
}

// Цены и итоговая сумма не передаются модели в текстовый блок, если подсказка
// администратора их не просит (2026-11-08, живая жалоба на КП 003/2026).
// Модель не может вставить в ответ «20 158 633,08 руб.», если она этого числа
// не видела.
func TestBlockPromptOmitsPricesUnlessAsked(t *testing.T) {
	offer := testOfferForBlocks()
	offer.Total = 20158633.08
	offer.Items = []OfferItem{
		{Name: "Мембрана V-SL 2 мм", Unit: "м2", Qty: 5520, Price: 769.56, Sum: 4247971.20},
	}

	userNoPrice := buildBlockUserPromptFor("Опиши систему без стоимостных показателей", offer, nil, nil)
	if strings.Contains(userNoPrice, "20 158 633,08") || strings.Contains(userNoPrice, "769,56") || strings.Contains(userNoPrice, "4 247 971,20") {
		t.Fatalf("в промпт блока без запроса стоимости попали цены:\n%s", userNoPrice)
	}
	if !strings.Contains(userNoPrice, "5520 м2") {
		t.Fatalf("количество материала потеряно:\n%s", userNoPrice)
	}

	userWithPrice := buildBlockUserPromptFor("Напиши вывод и укажи итоговую стоимость предложения", offer, nil, nil)
	if !strings.Contains(userWithPrice, "20 158 633,08") {
		t.Fatalf("при явном запросе стоимости итог не попал в промпт:\n%s", userWithPrice)
	}
}
