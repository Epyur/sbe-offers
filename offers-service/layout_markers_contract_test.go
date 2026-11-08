package main

import (
	"strings"
	"testing"
)

// Все метки, которые плагин предлагает кликом, обязаны печатать что-то по делу
// (2026-11-07).
//
// Повод — живая жалоба «метка не работает»: `{{текст: система: description}}`
// была заявлена в справке, но держалась на текстах, которых никто не писал, и
// молчала. Этот тест печатает КАЖДУЮ метку из списка плагина
// (`src/ui/markers.ts`) и требует, чтобы она либо дала содержимое, либо была
// честно пустой по известной причине — но не осталась литералом.
//
// Список меток здесь нарочно не дублируется: он лежит одним куском в
// `layoutMarkerContracts`, а сверка с плагином — `check_marker_registry.mjs`.
// Так проверка переживает добавление метки в плагин и краснеет только тогда,
// когда сервер не умеет то, что заявляет интерфейс.
type markerContract struct {
	marker string
	// want — подстрочный требуемый текст; пусто — метка допустимо пуста.
	want string
}

var layoutMarkerContracts = []markerContract{
	{"{{offer_number}}", "003/2026"},
	{"{{offer_date}}", "01.11.2026"},
	{"{{manager}}", "ivanov@example.ru"},
	{"{{offer_client}}", "ООО Пример"},
	{"{{offer_object}}", "МКД на ул. Примерной"},
	{"{{system_title}}", "ТН-ФУНДАМЕНТ Дренаж Проф"},
	{"{{systems_list}}", "ТН-ФУНДАМЕНТ Дренаж Проф"},
	{"{{система}}", "Мембрана Logicbase V-SL"},
	{"{{система: таблица}}", "Мембрана Logicbase V-SL"},
	{"{{текст: система: description}}", "Обоснование применения в двух словах."},
	{"{{текст: система: advantages}}", "Преимущества коротко."},
	{"{{ссылки: система}}", ""},
	{"{{характеристики: система}}", "Срок службы"},
	{"{картинка}", ""},
	{"{картинка: система 2}", ""},
	{"{картинка: материал}", ""},
}

// Почти каждая метка печатается напрямую из данных карточки и предложения.
func TestEveryDeclaredMarkerPrints(t *testing.T) {
	for _, c := range layoutMarkerContracts {
		lc := serviceLayoutContext(nil)
		// Служебные поля живут отдельной картой (serviceValues), а не в
		// form_values: так их считает сервер при печати.
		lc.service = map[string]string{
			fieldOfferNumber: "003/2026",
			fieldOfferDate:   "01.11.2026",
			fieldManager:     "ivanov@example.ru",
			fieldClientName:  "ООО Пример",
			fieldObjectName:  "МКД на ул. Примерной",
		}
		lc.values["system_title"] = "ТН-ФУНДАМЕНТ Дренаж Проф"
		lc.values["systems_list"] = "ТН-ФУНДАМЕНТ Дренаж Проф"
		// Служебные поля в форме тоже: одноимённое значение формы в макете
		// главнее расчётного — так их добавляет layoutValues при печати.
		for key, value := range lc.service {
			lc.values[key] = value
		}
		doc := layoutDocxFromXML(t, wordDoc(`<w:p><w:r><w:t>`+c.marker+`</w:t></w:r></w:p>`))
		out, err := renderLayoutDocx(doc, lc)
		if err != nil {
			t.Fatalf("%s: печать: %v", c.marker, err)
		}
		body := string(docxEntry(t, out, "word/document.xml"))
		if strings.Contains(body, c.marker) || strings.Contains(body, "{{") {
			t.Errorf("%s: метка ушла литералом:\n%s", c.marker, body)
			continue
		}
		if c.want != "" && !strings.Contains(body, c.want) {
			t.Errorf("%s: нет ожидаемого %q:\n%s", c.marker, c.want, body)
		}
		if c.want == "" && strings.TrimSpace(stripXMLTags(body)) == "" && markerMustPrint(c.marker) {
			t.Errorf("%s: метка напечатала пустое место", c.marker)
		}
	}
}

// markerMustPrint — метки, которые обязаны дать текст даже без картинки/ссылок:
// у системы в стенде они всегда есть.
func markerMustPrint(marker string) bool {
	return strings.Contains(marker, "offer_") || strings.Contains(marker, "manager")
}

// stripXMLTags оставляет только текст документа — для проверки «пусто или нет».
func stripXMLTags(xml string) string {
	var b strings.Builder
	in := false
	for _, r := range xml {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Поля формы и промт не должны становиться служебными метками: иначе
// администратор получит вместо своего поля чужой текст.
func TestDeclaredFieldsAreNotServiceMarkers(t *testing.T) {
	for _, marker := range []string{"{{offer_number}}", "{{system_title}}", "{{systems_list}}"} {
		inner := strings.TrimSuffix(strings.TrimPrefix(marker, "{{"), "}}")
		if _, ok := parseLayoutServiceSpec(inner); ok {
			t.Errorf("%s разобрана служебной меткой", marker)
		}
	}
}
