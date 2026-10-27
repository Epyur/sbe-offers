package main

import (
	"math"
	"testing"
)

func TestComputeTotal(t *testing.T) {
	if got := computeTotal([]OfferItem{{Qty: 3, Price: 999.99}}); got != 2999.97 {
		t.Fatalf("ожидали 2999.97 (без хвоста плавающей точки), получили %v", got)
	}
	if got := computeTotal([]OfferItem{{Qty: 2, Price: 10.005}, {Qty: 1, Price: 0}}); got != 20.01 {
		t.Fatalf("ожидали 20.01, получили %v", got)
	}
	if got := computeTotal(nil); got != 0 {
		t.Fatalf("пустой список должен дать 0, получили %v", got)
	}
	if got := computeTotal([]OfferItem{{Qty: 0.5, Price: 3.33}}); got != 1.67 {
		t.Fatalf("ожидали 1.67 (1.665 округляется до копеек), получили %v", got)
	}
}

func TestRoundMoney(t *testing.T) {
	cases := map[float64]float64{1.005: 1.01, 2.675: 2.68, 2.6744: 2.67, 0.1 + 0.2: 0.3}
	for in, want := range cases {
		if got := roundMoney(in); math.Abs(got-want) > 1e-9 {
			t.Fatalf("roundMoney(%v) = %v, ожидали %v", in, got, want)
		}
	}
}

func TestFormatNumber(t *testing.T) {
	if got := formatNumber(7, 2026); got != "007/2026" {
		t.Fatalf("ожидали 007/2026, получили %q", got)
	}
	if got := formatNumber(1234, 2027); got != "1234/2027" {
		t.Fatalf("ожидали 1234/2027, получили %q", got)
	}
}

func TestValidateOffer(t *testing.T) {
	if msg := validateOffer(Offer{}); msg != "в предложении нет позиций" {
		t.Fatalf("ожидали ошибку про пустые позиции, получили %q", msg)
	}
	offer := Offer{Items: []OfferItem{{Name: "Монтаж", Qty: 1, Price: 100, Sum: 100}}}
	if msg := validateOffer(offer); msg != "" {
		t.Fatalf("корректное предложение не должно ругаться, получили %q", msg)
	}
}

func TestRecalcOfferTotal(t *testing.T) {
	items := []OfferItem{
		{CatalogID: nil, Name: "Система", Unit: "шт", Qty: 3, Price: 999.99},
		{Name: "Монтаж", Unit: "шт", Qty: 1, Price: 1500},
	}
	total := recalcOfferTotal(items)
	if items[0].Sum != 2999.97 || items[1].Sum != 1500 {
		t.Fatalf("суммы строк не посчитаны: %+v", items)
	}
	if total != 4499.97 {
		t.Fatalf("итог должен быть 4499.97, получили %v", total)
	}
}
