package main

import (
	"strings"
	"testing"
)

// prepareOffer — граница входа для КП: проверки и нормализация В ОДНОМ месте,
// чтобы ручки (плагин и бот) не расходились в правилах.
func TestPrepareOfferNormalizes(t *testing.T) {
	offer, msg := prepareOffer(Offer{
		Items: []OfferItem{{Name: "Монтаж", Unit: "шт", Qty: 3, Price: 999.99}},
	})
	if msg != "" {
		t.Fatalf("корректное КП отвергнуто: %s", msg)
	}
	if offer.Total != 2999.97 {
		t.Fatalf("итог должен посчитаться: %v", offer.Total)
	}
	if offer.Items[0].Sum != 2999.97 {
		t.Fatalf("сумма строки должна посчитаться: %v", offer.Items[0].Sum)
	}
	if offer.Status != "draft" {
		t.Fatalf("неизвестный статус должен стать draft, получили %q", offer.Status)
	}
	if offer.FormValues == nil {
		t.Fatal("formValues должен стать пустой картой, а не nil")
	}
}

func TestPrepareOfferKeepsFinalStatus(t *testing.T) {
	offer, msg := prepareOffer(Offer{
		Items:  []OfferItem{{Name: "X", Qty: 1, Price: 10}},
		Status: "final",
	})
	if msg != "" {
		t.Fatalf("отвергнуто: %s", msg)
	}
	if offer.Status != "final" {
		t.Fatalf("final должен сохраниться, получили %q", offer.Status)
	}
}

func TestPrepareOfferRejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		in   Offer
		want string
	}{
		{"без позиций", Offer{}, "нет позиций"},
		{"пустое наименование", Offer{Items: []OfferItem{{Qty: 1, Price: 1}}}, "наименование"},
		{"нулевое количество", Offer{Items: []OfferItem{{Name: "X", Qty: 0, Price: 1}}}, "количество"},
		{"отрицательная цена", Offer{Items: []OfferItem{{Name: "X", Qty: 1, Price: -1}}}, "цена"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, msg := prepareOffer(tc.in)
			if !strings.Contains(msg, tc.want) {
				t.Fatalf("ожидали ошибку про %q, получили %q", tc.want, msg)
			}
		})
	}
}

// Права: владелец приложения (superadmin) может править чужое КП.
func TestCanManageOffer(t *testing.T) {
	offer := Offer{OwnerEmail: "ivanov@tn.ru"}

	cases := []struct {
		role, email string
		want        bool
	}{
		{"superadmin", "owner@tn.ru", true},
		{"admin", "owner@tn.ru", true},
		{"editor", "ivanov@tn.ru", true},
		{"editor", "petrov@tn.ru", false},
		{"viewer", "ivanov@tn.ru", true},
		{"viewer", "petrov@tn.ru", false},
		{"", "petrov@tn.ru", false},
	}
	for _, tc := range cases {
		if got := canManageOffer(tc.role, tc.email, offer); got != tc.want {
			t.Fatalf("роль %q, email %q: ожидали %v, получили %v", tc.role, tc.email, tc.want, got)
		}
	}
}

func TestCanSeeOfferIsAdminAware(t *testing.T) {
	offer := Offer{OwnerEmail: "ivanov@tn.ru"}
	if !canManageOffer("superadmin", "other@tn.ru", offer) {
		t.Fatal("superadmin должен видеть любое КП")
	}
	if canManageOffer("editor", "other@tn.ru", offer) {
		t.Fatal("editor не должен видеть чужое КП")
	}
	if !canManageOffer("editor", "IVANOV@tn.ru", offer) {
		t.Fatal("владелец видит своё КП без учёта регистра email")
	}
}
