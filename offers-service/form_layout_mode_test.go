package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// У макета КП и у макета боевого листка СВОЙ режим (2026-11-11, живой
// дефект: «вместо КП выгружался пустой макет боевого листка»).
//
// Проверяется контракт загрузки: загруженный файл адресуется режимом из
// запроса, и ответ говорит, куда он попал. Пока этого не было, макет одного
// документа мог оказаться в строке другого, и подмена была невидима снаружи.
func TestLayoutModeFromRequest(t *testing.T) {
	cases := map[string]string{
		"/api/offers/form/layout?mode=combat":       combatFormMode,
		"/api/offers/form/layout?mode=client":       clientFormMode,
		"/api/offers/form/layout":                   clientFormMode,
		"/api/offers/form/layout?mode=":             clientFormMode,
		"/api/offers/form/layout?mode=combatx":      clientFormMode,
		"/api/offers/form/layout?mode=%20combat%20": combatFormMode,
	}
	for url, want := range cases {
		req := httptest.NewRequest(http.MethodPost, url, nil)
		if got := layoutModeFrom(req); got != want {
			t.Fatalf("%s: режим %q, ожидается %q", url, got, want)
		}
	}
}

// Ответ загрузки называет режим, в который записан макет.
//
// Без этого поля человек, загрузивший макет в не тот документ, видел успех:
// «макет загружен» — и только печать показывала, что файл ушёл не туда.
func TestUploadLayoutResponseReportsMode(t *testing.T) {
	body, err := exampleLayoutDocx()
	if err != nil {
		t.Fatalf("образец макета не собрался: %v", err)
	}
	for _, mode := range []string{clientFormMode, combatFormMode} {
		payload := uploadLayoutResponse(mode, body, "макет.docx")
		if payload["mode"] != mode {
			t.Fatalf("ответ загрузки говорит о режиме %v, ожидается %q", payload["mode"], mode)
		}
		fields, _ := describeLayout(body)
		if len(fields) == 0 {
			t.Fatalf("метки образца не разобраны: %v", payload["layoutFields"])
		}
	}
}
