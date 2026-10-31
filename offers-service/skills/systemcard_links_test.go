package skills

import (
	"strings"
	"testing"
)

// Ссылки состава на материалы по SKU (Task 6 плана 2026-10-30).
//
// Система — не строка каталога, а сочетание материалов. Чтобы показать состав
// не словами, а позициями, каждая строка состава несёт код материала. Код
// может стоять где угодно в строке: «Мембрана (MAT-МЕМБ-ПВХ-0007)» и
// «MAT-МЕМБ-ПВХ-0007 — мембрана» означают одно и то же.

// Код в скобках после названия находится.
func TestCompositionLinksSkuInParentheses(t *testing.T) {
	md := "---\nsystem_sku: TN-S\n---\n# Карточка системы\n\nТН-С\n\n" +
		"## Состав системы\n\n- **1** Мембрана ([MAT-МЕМБ-ПВХ-0007](MAT-МЕМБ-ПВХ-0007))\n"
	card := ParseSystemCard(md)
	if len(card.Links) != 1 {
		t.Fatalf("ожидали одну ссылку состава: %#v", card.Links)
	}
	if card.Links[0].SKU != "MAT-МЕМБ-ПВХ-0007" {
		t.Errorf("SKU не выделен: %q (строка %q)", card.Links[0].SKU, card.Links[0].Raw)
	}
	if !strings.Contains(card.Links[0].Raw, "Мембрана") {
		t.Errorf("строка состава потеряна: %q", card.Links[0].Raw)
	}
}

// Код в начале строки тоже находится — порядок слов её не ломает.
func TestCompositionLinksSkuAtLineStart(t *testing.T) {
	md := "## Состав системы\n\n- MAT-МЕМБ-ПВХ-0007 — мембрана 1,5 мм\n"
	card := ParseSystemCard(md)
	if len(card.Links) != 1 || card.Links[0].SKU != "MAT-МЕМБ-ПВХ-0007" {
		t.Fatalf("SKU в начале строки не найден: %#v", card.Links)
	}
}

// Строка без кода остаётся в составе, но без ссылки: это не ошибка карточки,
// а незаполненная ссылка, и человек должен её видеть.
func TestCompositionLinksWithoutSku(t *testing.T) {
	md := "## Состав системы\n\n- Песок строительный\n"
	card := ParseSystemCard(md)
	if len(card.Links) != 1 {
		t.Fatalf("строка состава потеряна: %#v", card.Links)
	}
	if card.Links[0].SKU != "" {
		t.Errorf("SKU не должно быть: %q", card.Links[0].SKU)
	}
	if len(card.Composition) != 1 || card.Composition[0] != "Песок строительный" {
		t.Errorf("текстовый состав сломался: %#v", card.Composition)
	}
}

// Незнакомый код сохраняется как ссылка (проверка существования — в сервисе),
// и разбор не падает.
func TestCompositionLinksUnknownSkuKept(t *testing.T) {
	md := "## Состав системы\n\n- Мембрана MAT-МЕМБ-ПВХ-9999\n"
	card := ParseSystemCard(md)
	if len(card.Links) != 1 || card.Links[0].SKU != "MAT-МЕМБ-ПВХ-9999" {
		t.Fatalf("незнакомый код потерян: %#v", card.Links)
	}
}

// Кириллический префикс РАБ- находится: граница слова в регулярке задана
// явно, а не через \b (грабля 2026-10-29 и 2026-10-31).
func TestCompositionLinksWorkSku(t *testing.T) {
	md := "## Состав системы\n\n- Укладка (РАБ-МОНТ-ОБЩ-0001)\n"
	card := ParseSystemCard(md)
	if len(card.Links) != 1 || card.Links[0].SKU != "РАБ-МОНТ-ОБЩ-0001" {
		t.Fatalf("кириллический SKU не найден: %#v", card.Links)
	}
}

// Пустой состав не ломает карточку.
func TestCompositionLinksEmptyCard(t *testing.T) {
	card := ParseSystemCard("# Простая система\n\nОписание.")
	if len(card.Links) != 0 {
		t.Fatalf("у карточки без состава ссылок быть не должно: %#v", card.Links)
	}
}

// Вариант материала («(альтернатива: …)») — не отдельная ссылка.
func TestCompositionLinksAlternativeIsNotSeparate(t *testing.T) {
	md := "## Состав системы\n\n- Мембрана (MAT-МЕМБ-ПВХ-0007)\n  (альтернатива: MAT-МЕМБ-ПВХ-0008)\n"
	card := ParseSystemCard(md)
	if len(card.Links) != 1 {
		t.Fatalf("вариант стал отдельной позицией: %#v", card.Links)
	}
}
