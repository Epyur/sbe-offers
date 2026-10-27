package main

import (
	"strings"
	"testing"
)

func TestDocxFileNameReadable(t *testing.T) {
	offer := Offer{Number: "017/2026", Client: OfferClient{Name: "ООО Из Бота"}}
	got := docxFileName(offer)
	if !strings.Contains(got, "ООО Из Бота") {
		t.Fatalf("имя клиента должно остаться читаемым, получили %q", got)
	}
	if strings.Contains(got, "_") {
		t.Fatalf("подчёркивания вместо пробелов не нужны: %q", got)
	}
	if !strings.HasSuffix(got, ".docx") {
		t.Fatalf("расширение потерялось: %q", got)
	}

	tricky := Offer{Number: "001/2026", Client: OfferClient{Name: "ООО \"А/Б\" \nи К"}}
	got = docxFileName(tricky)
	for _, bad := range []string{"/", "\n", "\"А/Б\""} {
		if strings.Contains(got, bad) {
			t.Fatalf("запрещённый символ %q остался в имени: %q", bad, got)
		}
	}
}
