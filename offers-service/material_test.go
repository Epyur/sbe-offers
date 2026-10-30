package main

import (
	"regexp"
	"strings"
	"testing"
)

// SKU материала — идентичность, а не подпись (2026-10-30). Тесты держат три
// свойства, от которых зависит вся модель: формат, разные значения на повторе
// и стабильность входа (тип/серия не влияют на форму).

func TestSkuFormat(t *testing.T) {
	got := buildSku("МЕМБ", "ПВХ", 7)
	if !regexp.MustCompile(`^MAT-МЕМБ-ПВХ-0007$`).MatchString(got) {
		t.Fatalf("SKU не по формату: %q", got)
	}
}

// Номер дополняется нулями до четырёх знаков, но пятизначный не обрезается:
// обрезка однажды дала бы два разных материала с одним SKU.
func TestSkuNumberPadding(t *testing.T) {
	cases := map[int64]string{1: "0001", 42: "0042", 9999: "9999", 12345: "12345"}
	for n, want := range cases {
		got := buildSku("УТЕПЛ", "ПИР", n)
		if !strings.HasSuffix(got, "-"+want) {
			t.Errorf("номер %d: получили %q, ожидали хвост %q", n, got, want)
		}
	}
}

// Пустой тип и пустая серия — законные значения, а не ошибка: карточку часто
// заводят раньше, чем поняли, к какому классу относится материал.
func TestSkuFallsBackOnEmptyParts(t *testing.T) {
	got := buildSku("", "", 3)
	if got != "MAT-ПРОЧ-ОБЩ-0003" {
		t.Fatalf("запасные части не подставились: %q", got)
	}
}

// Работа — не материал: у неё свой префикс, иначе в документах её нельзя
// отличить от поставки.
func TestSkuWorkPrefix(t *testing.T) {
	if got := buildSkuForKind("work", "", "", 5); !strings.HasPrefix(got, "РАБ-") {
		t.Fatalf("у работы должен быть префикс РАБ: %q", got)
	}
	if got := buildSkuForKind("material", "", "", 5); !strings.HasPrefix(got, "MAT-") {
		t.Fatalf("у материала должен быть префикс MAT: %q", got)
	}
}

// В SKU годится только безопасный набор: пробелы, точки и дефисы в серии
// сломали бы и разбор, и ключ файла.
func TestSkuSanitizesSeries(t *testing.T) {
	got := buildSku("МЕМБ", "ПВХ 2.1-мм", 1)
	if !regexp.MustCompile(`^MAT-МЕМБ-[А-ЯЁA-Z0-9]+-0001$`).MatchString(got) {
		t.Fatalf("серия не очищена: %q", got)
	}
}
