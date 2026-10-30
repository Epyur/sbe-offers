package skills

import (
	"os"
	"strings"
	"testing"
)

// Разбор карточки системы из Базы знаний (шаг 2 плана 2026-10-29).
//
// Карточка — обычная заметка с тегом commercial_propose. Формат взят с
// реального образца (testdata/system-card.md), а не придуман: менеджер и
// технолог пишут карточки в привычном виде, и разбор обязан их понимать.

func loadSampleCard(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/system-card.md")
	if err != nil {
		t.Fatalf("образец карточки не читается: %v", err)
	}
	return string(data)
}

// Заголовок карточки — первый заголовок заметки. Это НЕ название продукта:
// в образце он «Состав системы», а продукт определяется связью с каталогом.
func TestParseSystemCardTitle(t *testing.T) {
	card := ParseSystemCard(loadSampleCard(t))
	if strings.TrimSpace(card.Title) == "" {
		t.Fatal("заголовок не разобран")
	}
	if strings.Contains(card.Title, "#") {
		t.Errorf("в заголовке осталась разметка: %q", card.Title)
	}
	// В стандарте (docs/sbe-offers-system-card-standard.md) H1 — служебный
	// заголовок «Карточка системы», а название продукта идёт строкой под ним.
	if card.Title != "ТН-СТИЛОБАТ ЭКСПЕРТ ТРОТУАР" {
		t.Errorf("ожидали название строкой под H1, получили %q", card.Title)
	}
}

// Frontmatter не должен попадать в текст: это служебные метаданные Obsidian.
func TestParseSystemCardIgnoresFrontmatter(t *testing.T) {
	card := ParseSystemCard(loadSampleCard(t))
	if strings.Contains(card.Description, "sbe_kb_id") {
		t.Errorf("frontmatter попал в описание: %q", card.Description)
	}
	if strings.Contains(card.Description, "commercial_propose") {
		t.Errorf("тег попал в описание")
	}
}

// Описание — текст карточки до блока характеристик, очищенный от разметки
// ссылок и картинок.
func TestParseSystemCardDescription(t *testing.T) {
	card := ParseSystemCard(loadSampleCard(t))
	if len(card.Description) < 300 {
		t.Fatalf("описание подозрительно короткое (%d символов)", len(card.Description))
	}
	if strings.Contains(card.Description, "![[") {
		t.Error("ссылка на картинку осталась в описании")
	}
	if strings.Contains(card.Description, "](http") {
		t.Error("markdown-ссылка не очищена")
	}
	if strings.Contains(card.Description, "**") {
		t.Error("звёздочки разметки остались в описании")
	}
}

// Картинка продукта — из `![[имя]]`.
func TestParseSystemCardImage(t *testing.T) {
	card := ParseSystemCard(loadSampleCard(t))
	if card.ImageName == "" {
		t.Fatal("имя картинки не найдено")
	}
	if !strings.HasSuffix(strings.ToLower(card.ImageName), ".png") {
		t.Fatalf("ожидали png, получили %q", card.ImageName)
	}
	// Размер после | отбрасывается: это настройка отображения в Obsidian.
	if strings.Contains(card.ImageName, "|") {
		t.Errorf("в имени остался размер: %q", card.ImageName)
	}
}

// Состав: строки вида «- **1** материал» или «- 1 материал».
func TestParseSystemCardComposition(t *testing.T) {
	card := ParseSystemCard(loadSampleCard(t))
	if len(card.Composition) < 5 {
		t.Fatalf("состав разобран неверно: %d позиций", len(card.Composition))
	}
	// В образце первая позиция — «Тротуарная плитка»: это система покрытия, а
	// не гидроизоляция. Проверяем не текст, а что номер снят и разметка чистая.
	first := card.Composition[0]
	if strings.ContainsAny(first, "[]*") || strings.HasPrefix(first, "1") {
		t.Errorf("позиция не очищена: %q", first)
	}
	if !strings.Contains(first, "Тротуарная") {
		t.Errorf("первая позиция состава: %q", first)
	}
	for _, item := range card.Composition {
		if strings.HasPrefix(item, "-") || strings.Contains(item, "](http") {
			t.Errorf("позиция не очищена: %q", item)
		}
	}
}

// Характеристики — таблица после заголовка «Характеристики».
func TestParseSystemCardSpecifications(t *testing.T) {
	card := ParseSystemCard(loadSampleCard(t))
	if len(card.Specifications) == 0 {
		t.Fatal("таблица характеристик не разобрана")
	}
	row := card.Specifications[0]
	if len(row) < 2 {
		t.Fatalf("строка характеристик без колонок: %#v", row)
	}
}

// Минимальная карточка: H1 — само название (технолог написал проще, чем в
// стандарте). Читаем название из H1, а не из строки под ним.
func TestParseSystemCardMinimal(t *testing.T) {
	card := ParseSystemCard("# Простая система\n\nКороткое описание.")
	if card.Title != "Простая система" {
		t.Fatalf("заголовок: %q", card.Title)
	}
	if card.ImageName != "" || len(card.Composition) != 0 {
		t.Fatal("у минимальной карточки не должно быть картинки и состава")
	}
}

// Карточка по стандарту: H1 служебный, название — строкой под ним.
func TestParseSystemCardStandardLayout(t *testing.T) {
	md := "---\nsystem_sku: TN-TEST\ntags: [commercial_propose]\n---\n" +
		"# Карточка системы\n\nТН-ТЕСТ\n\n## Состав системы\n\n- **1** Плитка\n\n" +
		"## Описание системы\n\nОписание теста.\n\n## Свойства и преимущества\n\n| a | b |\n| - | - |\n| 1 | 2 |\n"
	card := ParseSystemCard(md)
	if card.SKU != "TN-TEST" {
		t.Errorf("system_sku не прочитан: %q", card.SKU)
	}
	if card.Title != "ТН-ТЕСТ" {
		t.Errorf("название должно быть строкой под H1: %q", card.Title)
	}
	if len(card.Composition) != 1 || card.Composition[0] != "Плитка" {
		t.Errorf("состав: %v", card.Composition)
	}
	if card.Description != "Описание теста." {
		t.Errorf("описание: %q", card.Description)
	}
	if len(card.Specifications) != 2 {
		t.Errorf("таблица: %#v", card.Specifications)
	}
}

// Пустая заметка — тоже не падение: пустая карточка.
func TestParseSystemCardEmpty(t *testing.T) {
	card := ParseSystemCard("")
	if card.Title != "" || card.Description != "" {
		t.Fatalf("ожидали пустую карточку: %#v", card)
	}
}

// Таблица характеристик отделена от описания: текст таблицы в описание не
// идёт. Проверяем по содержимому первой строки таблицы образца.
func TestParseSystemCardStopsDescriptionAtSpecs(t *testing.T) {
	card := ParseSystemCard(loadSampleCard(t))
	if strings.Contains(card.Description, "Класс пожарной опасности") {
		t.Error("таблица характеристик попала в описание")
	}
	if len(card.Specifications) == 0 {
		t.Fatal("таблица характеристик не разобрана")
	}
}

// Текст для промпта: описание + состав в читаемом виде, без служебных данных.
func TestSystemCardPromptText(t *testing.T) {
	card := ParseSystemCard(loadSampleCard(t))
	text := card.PromptText()
	if len(text) < 300 {
		t.Fatalf("текст для модели слишком короткий: %d", len(text))
	}
	if strings.Contains(text, "sbe_kb_id") {
		t.Error("служебные метаданные попали в промпт")
	}
	if !strings.Contains(text, "Состав") {
		t.Error("состав не попал в текст для модели")
	}
}
