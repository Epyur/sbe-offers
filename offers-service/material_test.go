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

// --- Ручки материалов (Task 3) -----------------------------------------
//
// Правила проверяются отдельно от HTTP и БД: это чистые функции входа,
// и именно на них держится «в справочнике не бывает материала без имени
// или с чужим видом».

func TestValidateMaterialRequiresName(t *testing.T) {
	if msg := validateMaterial(Material{Name: "  ", Unit: "м2"}); msg == "" {
		t.Fatal("материал без названия должен отклоняться")
	}
	if msg := validateMaterial(Material{Name: "Мембрана", Unit: "м2"}); msg != "" {
		t.Fatalf("годный материал отклонён: %q", msg)
	}
}

// Система материалом не является: попытка завести kind='system' в справочнике
// материалов должна отклоняться. Иначе сущности снова смешаются.
func TestValidateMaterialRejectsSystemKind(t *testing.T) {
	if msg := validateMaterial(Material{Name: "ТН-Кровля", Kind: "system"}); msg == "" {
		t.Fatal("system не должен проходить в справочник материалов")
	}
	if msg := validateMaterial(Material{Name: "Монтаж", Kind: "work"}); msg != "" {
		t.Fatalf("работа должна проходить: %q", msg)
	}
}

// Пустая единица — это «шт», как и в каталоге: поле необязательное.
func TestNormalizeMaterialDefaults(t *testing.T) {
	m := normalizeMaterial(Material{Name: "  Мембрана  ", Unit: ""})
	if m.Name != "Мембрана" {
		t.Errorf("имя не обрезано: %q", m.Name)
	}
	if m.Unit != "шт" {
		t.Errorf("единица по умолчанию не подставлена: %q", m.Unit)
	}
	if m.Kind != "material" {
		t.Errorf("вид по умолчанию не подставлен: %q", m.Kind)
	}
}

// Вилка цен: расширяемся, но никогда не сужаемся — иначе одна дешёвая
// поставка затрёт историю дорогих.
func TestWidenPriceRange(t *testing.T) {
	low, high := 100.0, 200.0
	lo, hi, changed := widenPriceRange(&low, &high, 150)
	if changed || *lo != 100 || *hi != 200 {
		t.Fatalf("цена внутри вилки не должна её менять: %v %v %v", *lo, *hi, changed)
	}
	lo, hi, changed = widenPriceRange(&low, &high, 50)
	if !changed || *lo != 50 || *hi != 200 {
		t.Fatalf("нижняя граница не расширена: %v %v %v", *lo, *hi, changed)
	}
	lo, hi, changed = widenPriceRange(&low, &high, 500)
	if !changed || *lo != 100 || *hi != 500 {
		t.Fatalf("верхняя граница не расширена: %v %v %v", *lo, *hi, changed)
	}
}

// Первая увиденная цена задаёт обе границы, а не «0 до цены».
func TestWidenPriceRangeFirstValue(t *testing.T) {
	var lo, hi *float64
	lo, hi, changed := widenPriceRange(lo, hi, 320.5)
	if !changed || lo == nil || hi == nil || *lo != 320.5 || *hi != 320.5 {
		t.Fatalf("первая цена должна задать обе границы: %v %v %v", lo, hi, changed)
	}
}
