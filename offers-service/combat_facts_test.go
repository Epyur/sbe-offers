package main

// Тесты шага 3e (2026-11-17): добытые факты листка (ссылки и файлы заказчика)
// становятся УСТОЙЧИВЫМИ — живут в `form_values` самого листка и входят в
// отпечаток содержания, поэтому правка факта переписывает раздел, а повторная
// сборка без правки берёт готовый текст.
//
// Живой дефект, ради которого это заведено: агент прочитал документ заказчика,
// ответил по нему, и на следующем ходу сведения пропали — раздел писался по
// памяти модели (§9.1 спеки).

import (
	"strings"
	"testing"
)

func TestCombatFactRejectsEmpty(t *testing.T) {
	if _, ok := newCombatFact("ссылка", "https://example.com", "   "); ok {
		t.Fatal("пустой текст стал фактом: «прочитано, но ничего не понятно» — не факт")
	}
	if _, ok := newCombatFact("ссылка", "  ", "текст"); ok {
		t.Fatal("факт без источника: потом не отличить, откуда сведения")
	}
	fact, ok := newCombatFact("ссылка", "https://example.com/a", "  Толщина 1,5 мм.  ")
	if !ok {
		t.Fatal("нормальный факт отвергнут")
	}
	if fact.Text != "Толщина 1,5 мм." {
		t.Fatalf("текст не обрезан по краям: %q", fact.Text)
	}
	if fact.Digest == "" || fact.Digest == factDigest("другой текст") {
		t.Fatal("отпечаток факта не считается по содержимому")
	}
}

// Факт обязан пройти через `form_values` без потерь: это единственное место,
// где он живёт между ходами. Формат — строка, чтобы читалась в psql.
func TestCombatFactRoundTrip(t *testing.T) {
	fact, _ := newCombatFact("файл", "смета.xlsx", "Строка 1\nСтрока 2")
	values := map[string]string{combatFactKey(1): fact.encode()}
	got := combatFactsFromValues(values)
	if len(got) != 1 {
		t.Fatalf("фактов после чтения %d, ожидался 1", len(got))
	}
	if got[0].Kind != "файл" || got[0].Source != "смета.xlsx" || got[0].Text != fact.Text {
		t.Fatalf("факт разобран неверно: %+v", got[0])
	}
	if got[0].Digest != fact.Digest {
		t.Fatal("отпечаток факта потерялся при сохранении")
	}
}

// Порядок фактов в промте и в отпечатке — по номеру ключа, а не по порядку
// карты: иначе отпечаток плавал бы между сборками на одном листке.
func TestCombatFactsKeepKeyOrder(t *testing.T) {
	values := map[string]string{
		combatFactKey(3): "c | третий | x\n3",
		combatFactKey(1): "a | первый | x\n1",
		combatFactKey(2): "b | второй | x\n2",
	}
	facts := combatFactsFromValues(values)
	if len(facts) != 3 {
		t.Fatalf("фактов %d, ожидалось 3", len(facts))
	}
	if facts[0].Source != "первый" || facts[1].Source != "второй" || facts[2].Source != "третий" {
		t.Fatalf("порядок фактов не по номеру ключа: %+v", facts)
	}
}

// Повторное чтение того же адреса ЗАМЕНЯЕТ факт, а не заводит второй: две
// одинаковые ссылки в промте только путают модель, а предел кончается за
// три хода (найдено на стенде 2026-11-17).
func TestCombatFactsReplaceSameSource(t *testing.T) {
	first, _ := newCombatFact("ссылка", "https://example.com/spec", "старое содержимое")
	second, _ := newCombatFact("ссылка", "https://example.com/spec", "новое содержимое")
	other, _ := newCombatFact("ссылка", "https://example.com/other", "другое")
	facts := withCombatFacts(withCombatFacts(withCombatFacts(nil, first), second), other)
	if len(facts) != 2 {
		t.Fatalf("фактов %d, ожидалось 2 (тот же адрес обновлён, а не добавлен)", len(facts))
	}
	if facts[0].Text != "новое содержимое" {
		t.Fatalf("факт по тому же адресу не обновился: %q", facts[0].Text)
	}
	if combatFactHandle([]combatFact{first}) == combatFactHandle([]combatFact{second}) {
		t.Fatal("другое содержимое того же адреса не изменило отпечаток набора")
	}
}

// Предел — не «все ссылки сайта»: `form_values` это колонка JSONB, и мегабайты
// туда не кладутся.
func TestCombatFactsRespectLimit(t *testing.T) {
	var facts []combatFact
	for i := 0; i < combatFactLimit+5; i++ {
		fact, _ := newCombatFact("ссылка", "https://example.com/"+itoa(i), "текст")
		facts = withCombatFacts(facts, fact)
	}
	if len(facts) != combatFactLimit {
		t.Fatalf("фактов %d, предел %d", len(facts), combatFactLimit)
	}
}

// Главное: правка факта меняет отпечаток содержания и ключ раздела, поэтому
// раздел переписывается. Без этого текст описывал бы документы, которые с тех
// пор изменились (класс дефекта 2026-11-08).
func TestCombatFactsChangeSectionKey(t *testing.T) {
	sheet, our, competitor := combatSectionFixture()
	without := combatSectionHandle(sheet, []SystemCard{our}, []SystemCard{competitor}, nil, "")

	fact, _ := newCombatFact("файл", "смета.xlsx", "Толщина 1,5 мм")
	withFacts := sheet
	withFacts.FormValues = map[string]string{combatHintField: combatHint(sheet), combatFactKey(1): fact.encode()}
	with := combatSectionHandle(withFacts, []SystemCard{our}, []SystemCard{competitor},
		combatFactsFromValues(withFacts.FormValues), "")
	if without == with {
		t.Fatal("добавление факта не изменило отпечаток содержания: раздел останется старым")
	}

	edited, _ := newCombatFact("файл", "смета.xlsx", "Толщина 1,2 мм")
	editedSheet := sheet
	editedSheet.FormValues = map[string]string{combatHintField: combatHint(sheet), combatFactKey(1): edited.encode()}
	afterEdit := combatSectionHandle(editedSheet, []SystemCard{our}, []SystemCard{competitor},
		combatFactsFromValues(editedSheet.FormValues), "")
	if afterEdit == with {
		t.Fatal("правка содержимого факта не изменила отпечаток содержания")
	}

	// Листку без фактов отпечаток не меняется: у выкатки старые тексты не
	// переписываются зря (пустой набор — пустая строка в отпечатке).
	if combatFactHandle(nil) != "" {
		t.Fatal("пустой набор фактов обязан давать пустой отпечаток")
	}
}

// Факты попадают в бриф подраздела отдельным блоком и названы сведениями
// заказчика, а не свойствами нашего продукта.
func TestCombatSectionBriefCarriesFacts(t *testing.T) {
	_, our, competitor := combatSectionFixture()
	fact, _ := newCombatFact("файл", "смета.xlsx", "Толщина у конкурента 1,2 мм")
	brief := combatSectionBrief("Сравни", []SystemCard{our}, []SystemCard{competitor}, "", []combatFact{fact}, "")
	for _, want := range []string{"Добытые факты", "смета.xlsx", "Толщина у конкурента 1,2 мм", "по документам заказчика"} {
		if !strings.Contains(brief, want) {
			t.Fatalf("в брифе нет %q:\n%s", want, brief)
		}
	}
	// Без фактов бриф прежний — иначе у листка без ссылок промт менялся бы зря.
	plain := combatSectionBrief("Сравни", []SystemCard{our}, []SystemCard{competitor}, "", nil, "")
	if strings.Contains(plain, "Добытые факты") {
		t.Fatal("блок фактов появился у листка без фактов")
	}
	if !strings.Contains(plain, `"blocks"`) {
		t.Fatal("контракт ответа потерялся")
	}
}

// Чужой ключ в form_values фактом не считается: иначе `gen.sect.*` попал бы в
// промт как «сведения заказчика».
func TestCombatFactsIgnoreForeignKeys(t *testing.T) {
	values := map[string]string{
		"gen.section":                   "abc",
		"gen.sect.weakness.risks.sel12": `{"blocks":[]}`,
		combatHintField:                 "срок",
	}
	if facts := combatFactsFromValues(values); len(facts) != 0 {
		t.Fatalf("чужие ключи стали фактами: %+v", facts)
	}
}
