package skills

import (
	"os"
	"strings"
	"testing"
)

// Шаблон карточки в вольте (`_Шаблон карточки системы.md`) должен читаться
// разбором так же, как настоящая карточка. Иначе шаблон тихо разойдётся с
// кодом, и администратор заполнит его по инструкции, а система не поймёт.

func loadTemplateCard(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/template-card.md")
	if err != nil {
		t.Fatalf("шаблон не читается: %v", err)
	}
	return string(data)
}

func TestTemplateCardParses(t *testing.T) {
	card := ParseSystemCard(loadTemplateCard(t))
	if card.SKU != "TN-XXXX-XXXX" {
		t.Errorf("system_sku шаблона: %q", card.SKU)
	}
	if !strings.Contains(card.Title, "НАЗВАНИЕ СИСТЕМЫ") {
		t.Errorf("название шаблона: %q", card.Title)
	}
	if len(card.Composition) == 0 {
		t.Error("состав шаблона не разобран")
	}
	if strings.TrimSpace(card.Description) == "" {
		t.Error("описание шаблона пустое")
	}
	if card.ImageName == "" {
		t.Error("картинка шаблона не найдена")
	}
	if len(card.Specifications) == 0 {
		t.Error("таблица характеристик шаблона не разобрана")
	}
}

// Все обязательные блоки стандарта в шаблоне присутствуют: администратор
// заполняет по образцу, а не догадывается о структуре.
func TestTemplateCardHasStandardBlocks(t *testing.T) {
	md := loadTemplateCard(t)
	for _, heading := range []string{"## Состав системы", "## Описание системы", "## Свойства и преимущества"} {
		if !strings.Contains(md, heading) {
			t.Errorf("в шаблоне нет блока %q", heading)
		}
	}
	if !strings.Contains(md, "system_sku:") {
		t.Error("в шаблоне нет поля system_sku")
	}
	if !strings.Contains(md, "tags: [commercial_propose]") {
		t.Error("в шаблоне нет тега commercial_propose")
	}
	if !strings.Contains(md, "![[") {
		t.Error("в шаблоне нет примера картинки")
	}
}
