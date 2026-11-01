package main

import (
	"strings"
	"testing"
)

// Форма по умолчанию в новом порядке (2026-11-01).
//
// Пользователь описал содержание явно: титульный лист, разрыв страницы,
// поставщик/получатель/объект, свободный блок про систему и материалы, позиции,
// ссылки. Здесь проверяется именно ЭТОТ порядок, а не «форма собралась».

func TestDefaultFormOrder(t *testing.T) {
	form := defaultForm("client")
	want := []string{"cover", "parties", "description", "links"}
	var got []string
	for _, b := range form.Blocks {
		got = append(got, b.ID)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("порядок блоков: %v, ожидали %v", got, want)
	}
	// Разрыв страницы отделяет титул от содержательной части.
	parties := form.Blocks[1]
	if !parties.PageBreakBefore {
		t.Fatal("содержательная часть должна начинаться с новой страницы")
	}
	for _, b := range form.Blocks {
		if !b.ShowInClient {
			t.Fatalf("блок «%s» по умолчанию не попадёт в документ клиента", b.Title)
		}
	}
	// Свободный блок пишет модель — без подсказки он молчит.
	if !strings.Contains(string(form.Blocks[2].Content), "prompt_text") {
		t.Fatalf("блок описания должен быть текстом по промту: %s", form.Blocks[2].Content)
	}
	if !strings.Contains(string(form.Blocks[3].Content), `"links"`) {
		t.Fatalf("последний блок должен быть ссылками: %s", form.Blocks[3].Content)
	}
}

// Поля поставщика, получателя и объекта заведены: без них титул и таблица
// сторон напечатают пустоту.
func TestDefaultFormHasPartyFields(t *testing.T) {
	form := defaultForm("client")
	have := map[string]bool{}
	for _, p := range form.Placeholders {
		have[p.ID] = true
	}
	for _, id := range []string{"supplier_name", "supplier_contacts", "client_name", "client_contact", "object_name", "object_address"} {
		if !have[id] {
			t.Errorf("в форме по умолчанию нет поля %q", id)
		}
	}
}

// Форма по умолчанию действительно проходит валидацию: иначе её нельзя было бы
// сохранить из редактора.
func TestDefaultFormIsValid(t *testing.T) {
	if msg := validateForm(defaultForm("client")); msg != "" {
		t.Fatalf("форма по умолчанию не проходит проверку: %s", msg)
	}
}

// Документ по форме по умолчанию: титул, разрыв, стороны, ссылки — всё на месте.
func TestDefaultFormRendersInOrder(t *testing.T) {
	offer := testOfferForBlocks()
	offer.FormValues = map[string]string{
		"supplier_name":  "ООО Поставщик",
		"client_name":    "ООО Заказчик",
		"object_name":    "ЖК Поэма",
		"object_address": "Новосибирск, ул. Ленина, 1",
	}
	data, err := renderOfferDocx(defaultForm("client"), offer)
	if err != nil {
		t.Fatalf("рендер упал: %v", err)
	}
	xml := docXMLOf(t, data)

	if !contains(xml, `<w:br w:type="page"/>`) && !contains(xml, `<w:pageBreakBefore/>`) {
		t.Fatal("нет разрыва между титулом и содержательной частью")
	}
	order := []string{"ООО Поставщик", "ЖК Поэма", "Новосибирск, ул. Ленина, 1", "Состав предложения"}
	pos := make([]int, len(order))
	for i, needle := range order {
		pos[i] = strings.Index(xml, needle)
		if pos[i] < 0 {
			t.Fatalf("в документе нет %q", needle)
		}
	}
	for i := 1; i < len(pos); i++ {
		if pos[i] < pos[i-1] {
			t.Fatalf("порядок нарушен: %q идёт раньше %q", order[i], order[i-1])
		}
	}
	if contains(xml, "{{") {
		t.Fatal("плейсхолдеры остались незаполненными")
	}
}

// Отступ блока — часть формата, и он попадает в документ.
func TestDefaultFormPartiesBlockSpaced(t *testing.T) {
	form := defaultForm("client")
	if form.Blocks[2].SpacingBefore == 0 {
		t.Fatal("описанию нужен отступ до — иначе оно липнет к таблице сторон")
	}
}
