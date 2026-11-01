package main

import (
	"strings"
	"testing"

	"mailers/offers/skills"
)

// Блоки документа формы (2026-11-01): разрыв страницы, размещение на странице,
// ссылки и текст по промту.
//
// Что проверяется и почему:
//   - разрыв страницы — то, что пользователь попросил прямо: «редактор должен
//     уметь вставлять разрывы страниц»;
//   - размещение блока (отступ до/после, «не разрывать») — «управлять
//     размещением блоков на странице» без текстовых полей, которые не переживут
//     правки документа человеком;
//   - ссылки — из карточек Базы знаний в конец КП;
//   - текст по промту — блок, который модель пишет по подсказке администратора.

func testOfferForBlocks() Offer {
	return Offer{
		Number:     "007/2026",
		CreatedAt:  "2026-11-01T10:00:00.000Z",
		OwnerEmail: "m@tn.ru",
		Client:     OfferClient{Name: "ООО Заказчик", Contact: "Иванов"},
		Items: []OfferItem{
			{Name: "Мембрана ПВХ", Unit: "м²", Qty: 100, Price: 850, Sum: 85000},
		},
		Total:      85000,
		FormValues: map[string]string{"client_name": "ООО Заказчик"},
	}
}

func formWithBlock(content string) OfferForm {
	return OfferForm{
		Mode:  "client",
		Title: "Коммерческое предложение",
		Blocks: []Block{{
			ID:           "test",
			Title:        "Проверочный блок",
			Content:      []byte(content),
			ShowInClient: true,
		}},
	}
}

// Разрыв страницы — узел `page_break`, а не флаг блока: человек вставляет его
// в нужное место, в том числе в середину блока.
func TestPageBreakNodeRendersBreak(t *testing.T) {
	form := formWithBlock(`[
		{"type":"paragraph","text":"Первая страница"},
		{"type":"page_break"},
		{"type":"paragraph","text":"Вторая страница"}
	]`)
	data, err := renderOfferDocx(form, testOfferForBlocks())
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	xml := docXMLOf(t, data)
	if !contains(xml, `<w:br w:type="page"/>`) {
		t.Fatal("разрыв страницы не попал в документ")
	}
	first := strings.Index(xml, "Первая страница")
	second := strings.Index(xml, "Вторая страница")
	brk := strings.Index(xml, `<w:br w:type="page"/>`)
	if first < 0 || second < 0 || brk < 0 {
		t.Fatalf("не все части на месте: first=%d second=%d brk=%d", first, second, brk)
	}
	if !(first < brk && brk < second) {
		t.Fatalf("разрыв стоит не между абзацами: first=%d brk=%d second=%d", first, brk, second)
	}
}

// Разрыв в начале документа не рисуется: Word на пустом первом абзаце перед
// разрывом даёт Пустую страницу, и это всегда ошибка оформления.
func TestPageBreakAtStartIgnored(t *testing.T) {
	form := formWithBlock(`[
		{"type":"page_break"},
		{"type":"paragraph","text":"Текст"}
	]`)
	data, err := renderOfferDocx(form, testOfferForBlocks())
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	if count := strings.Count(docXMLOf(t, data), `<w:br w:type="page"/>`); count != 0 {
		t.Fatalf("разрыв перед первым текстом должен быть проигнорирован, найдено %d", count)
	}
}

// Размещение блока: отступ до и после в пунктах, «не разрывать».
func TestBlockSpacingAndKeepTogether(t *testing.T) {
	form := OfferForm{
		Mode:  "client",
		Title: "КП",
		Blocks: []Block{{
			ID:            "intro",
			Title:         "Вступление",
			Content:       []byte(`[{"type":"paragraph","text":"Уважаемые коллеги"}]`),
			ShowInClient:  true,
			SpacingBefore: 24,
			SpacingAfter:  12,
			KeepTogether:  true,
		}},
	}
	data, err := renderOfferDocx(form, testOfferForBlocks())
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	xml := docXMLOf(t, data)
	for _, want := range []string{`w:before="480"`, `w:after="240"`, `<w:keepLines/>`} {
		if !contains(xml, want) {
			t.Fatalf("в документе нет %q:\n%s", want, excerptAround(xml, "Уважаемые коллеги"))
		}
	}
}

// Блок «начинать с новой страницы» — без отдельного узла разрыва: админу
// проще поставить галочку, чем искать, куда вставить разрыв.
func TestBlockPageBreakBefore(t *testing.T) {
	form := OfferForm{
		Mode:  "client",
		Title: "КП",
		Blocks: []Block{{
			ID:              "intro",
			Title:           "Вступление",
			Content:         []byte(`[{"type":"paragraph","text":"Содержательная часть"}]`),
			ShowInClient:    true,
			PageBreakBefore: true,
		}},
	}
	data, err := renderOfferDocx(form, testOfferForBlocks())
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	if xml := docXMLOf(t, data); !contains(xml, `<w:pageBreakBefore/>`) {
		t.Fatal("флаг «с новой страницы» не попал в документ")
	}
}

// Узел выравнивания: абзац по центру — как просили про титульную страницу.
func TestParagraphAlignReachesDocument(t *testing.T) {
	form := formWithBlock(`[{"type":"paragraph","text":"Титул","align":"center"}]`)
	data, err := renderOfferDocx(form, testOfferForBlocks())
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	if xml := docXMLOf(t, data); !contains(xml, `<w:jc w:val="center"/>`) {
		t.Fatal("выравнивание абзаца не попало в документ")
	}
}

// Блок с ссылками: заголовок, потом названия и адреса из карточек.
func TestLinksNodeRendersCollectedRefs(t *testing.T) {
	form := formWithBlock(`[{"type":"links"}]`)
	refs := []skills.CardRef{
		{Label: "Страница системы", URL: "https://nav.tn.ru/systems/tn-krovlya-avto/", Kind: skills.RefKindPage},
		{Label: "Техлист", URL: "https://nav.tn.ru/pim/x.pdf", Kind: skills.RefKindDoc},
	}
	data, err := renderOfferDocxFull(form, testOfferForBlocks(), nil, nil, refs)
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	xml := docXMLOf(t, data)
	if !contains(xml, "Ссылки") {
		t.Fatal("заголовок раздела ссылок не попал в документ")
	}
	if !contains(xml, "https://nav.tn.ru/systems/tn-krovlya-avto/") {
		t.Fatal("адрес страницы системы не попал в документ")
	}
	if !contains(xml, "https://nav.tn.ru/pim/x.pdf") {
		t.Fatal("адрес техлиста не попал в документ")
	}
	if !contains(xml, "Страница системы") || !contains(xml, "Техлист") {
		t.Fatal("подписи ссылок не попали в документ")
	}
}

// Без ссылок узел молчит: пустой заголовок «Ссылки» в документе — хуже, чем
// его отсутствие.
func TestLinksNodeEmptyRendersNothing(t *testing.T) {
	form := formWithBlock(`[{"type":"links"}]`)
	data, err := renderOfferDocxFull(form, testOfferForBlocks(), nil, nil, nil)
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	xml := docXMLOf(t, data)
	if strings.Contains(xml, "Ссылки") && !strings.Contains(xml, "Состав предложения") {
		t.Fatal("ожидали, что без ссылок раздела не будет")
	}
	if count := strings.Count(xml, "Ссылки"); count > 0 {
		// Допустимо, только если это часть другого текста — здесь такого нет.
		t.Fatalf("пустой раздел ссылок всё же напечатан (%d)", count)
	}
}

// Текст по промту: узел без модели печатается кэшем, если он есть, и молчит,
// если кэша нет. Модель недоступна — документ всё равно собирается.
func TestPromptNodeUsesCacheAndDegrades(t *testing.T) {
	form := formWithBlock(`[{"type":"prompt_text","instruction":"Опиши систему","maxWords":80}]`)
	offer := testOfferForBlocks()
	offer.FormValues[genKeySection] = hashOfferFacts(offer)
	offer.FormValues[genKeyTextByID("test", 0)] = "Готовый текст из кэша."

	data, err := renderOfferDocxFull(form, offer, nil, nil, nil)
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	if xml := docXMLOf(t, data); !contains(xml, "Готовый текст из кэша.") {
		t.Fatal("текст из кэша не попал в документ")
	}

	fresh := testOfferForBlocks()
	data, err = renderOfferDocxFull(form, fresh, nil, nil, nil)
	if err != nil {
		t.Fatalf("без кэша и без модели рендер не должен падать: %v", err)
	}
	if xml := docXMLOf(t, data); contains(xml, "Опиши систему") {
		t.Fatal("инструкция админа попала в документ вместо текста")
	}
}

// Устаревший кэш (предложение изменилось) не печатается: иначе в файле будет
// текст про старые суммы.
func TestPromptNodeStaleCacheIgnored(t *testing.T) {
	form := formWithBlock(`[{"type":"prompt_text","instruction":"Опиши систему"}]`)
	offer := testOfferForBlocks()
	offer.FormValues[genKeySection] = "старый-хеш"
	offer.FormValues[genKeyTextByID("test", 0)] = "Текст по старой версии."

	data, err := renderOfferDocxFull(form, offer, nil, nil, nil)
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	if xml := docXMLOf(t, data); contains(xml, "Текст по старой версии.") {
		t.Fatal("устаревший текст попал в документ")
	}
}

// excerptAround — кусок XML вокруг фразы, чтобы падение теста было читаемым.
func excerptAround(xml, needle string) string {
	i := strings.Index(xml, needle)
	if i < 0 {
		return xml[:min(len(xml), 400)]
	}
	start := i - 400
	if start < 0 {
		start = 0
	}
	end := i + 400
	if end > len(xml) {
		end = len(xml)
	}
	return xml[start:end]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
