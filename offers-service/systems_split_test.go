package main

import (
	"context"
	"strings"
	"testing"

	"mailers/offers/skills"
)

// Системы и материалы по тегам (Task 6 плана 2026-10-30).
//
// Одна лента карточек Базы знаний делится надвое тегом material: карточка с
// ним — материал (то, что продаётся, у него есть строка в offers_material), без
// него — система. Тег — явное решение человека, а не догадка по названию.
//
// Состав системы разрешается против справочника материалов: строка со ссылкой
// на код получает название и единицу, незнакомая строка остаётся видимой и
// попадает в список проблем. Молча терять строку состава нельзя — человек не
// узнает, что в системе чего-то не хватает.

// splitKB — база знаний для проверки разделения: список карточек и их тексты.
type splitKB struct {
	notes []KBNoteSummary
	byID  map[int64]KBNote
}

func (f *splitKB) TagID(_ context.Context, _ string) (int64, error) { return 7, nil }

func (f *splitKB) Notes(_ context.Context, _ int64) ([]KBNoteSummary, error) {
	return f.notes, nil
}

func (f *splitKB) Note(_ context.Context, id int64) (KBNote, error) {
	return f.byID[id], nil
}

func (f *splitKB) Attachments(_ context.Context, _ int64) ([]KBAttachment, error) {
	return nil, nil
}

func (f *splitKB) FileLink(_ context.Context, key string) (string, error) { return key, nil }

// noteWith делает карточку по стандарту: название строкой под служебным H1.
func noteWith(id int64, title string, tags []KBTag, composition ...string) KBNote {
	var b strings.Builder
	b.WriteString("---\nsystem_sku: SKU-" + title + "\n---\n# Карточка системы\n\n")
	b.WriteString(title + "\n\n")
	if len(composition) > 0 {
		b.WriteString("## Состав системы\n\n")
		for _, item := range composition {
			b.WriteString("- " + item + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Описание системы\n\nОписание.\n")
	return KBNote{
		KBNoteSummary: KBNoteSummary{ID: id, Title: title, Path: "База/" + title + ".md", Tags: tags},
		ContentMD:     b.String(),
	}
}

// Карточка с тегом material материалом и является, с commercial_propose без
// material — системой.
func TestSplitByTagsKeepsMaterialsOutOfSystems(t *testing.T) {
	materialNote := noteWith(1, "Мембрана ПВХ", []KBTag{{Name: "commercial_propose"}, {Name: "material"}})
	systemNote := noteWith(2, "ТН-БАЗА", []KBTag{{Name: "commercial_propose"}})

	systems, materials := splitCardsByTags([]KBNoteSummary{materialNote.KBNoteSummary, systemNote.KBNoteSummary})
	if len(systems) != 1 || systems[0].ID != 2 {
		t.Fatalf("система определена неверно: %#v", systems)
	}
	if len(materials) != 1 || materials[0].ID != 1 {
		t.Fatalf("материал определён неверно: %#v", materials)
	}
}

// Тег сверяется без учёта регистра и пробелов — база знаний пишет как хочет.
func TestSplitByTagsIgnoresCase(t *testing.T) {
	n := KBNoteSummary{ID: 1, Tags: []KBTag{{Name: " Material "}}}
	systems, materials := splitCardsByTags([]KBNoteSummary{n})
	if len(materials) != 1 || len(systems) != 0 {
		t.Fatalf("тег не распознан: %#v %#v", systems, materials)
	}
}

// Карточка без тегов вообще — система: в ленте commercial_propose её не будет,
// но если она попала — не теряем, а считаем системой.
func TestSplitByTagsNoTagsIsSystem(t *testing.T) {
	systems, materials := splitCardsByTags([]KBNoteSummary{{ID: 1}})
	if len(systems) != 1 || len(materials) != 0 {
		t.Fatalf("карточка без тегов: %#v %#v", systems, materials)
	}
}

// Состав, ссылающийся на существующий материал, показывает его название,
// единицу и ЕКН — так плагин может собрать позиции без второго запроса.
func TestResolveCompositionFindsMaterial(t *testing.T) {
	card := skills.SystemCard{
		Title: "ТН-ТЕСТ",
		Composition: []string{
			"Мембрана (MAT-МЕМБ-ПВХ-0007)",
			"Песок",
		},
		Links: []skills.CompositionLink{
			{Raw: "Мембрана (MAT-МЕМБ-ПВХ-0007)", SKU: "MAT-МЕМБ-ПВХ-0007"},
			{Raw: "Песок"},
		},
	}
	materials := []Material{
		{ID: 5, SKU: "MAT-МЕМБ-ПВХ-0007", Name: "Мембрана", Unit: "м2",
			Ekn: []MaterialEkn{{Ekn: "4600000000001"}}},
	}
	composition, problems := resolveComposition(card, materials)

	if len(composition) != 2 {
		t.Fatalf("строк состава должно быть две: %#v", composition)
	}
	if !composition[0].Found || composition[0].MaterialID != 5 {
		t.Errorf("строка с кодом не разрешилась: %#v", composition[0])
	}
	if composition[0].Name != "Мембрана" || composition[0].Unit != "м2" {
		t.Errorf("не подставлены название и единица: %#v", composition[0])
	}
	if len(composition[0].Ekn) != 1 || composition[0].Ekn[0] != "4600000000001" {
		t.Errorf("ЕКН не подставлены: %#v", composition[0].Ekn)
	}
	if composition[1].Found {
		t.Errorf("строка без кода не может быть найдена: %#v", composition[1])
	}
	if composition[1].Raw == "" {
		t.Error("строка без кода потеряла текст")
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "Песок") {
		t.Fatalf("ожидали одну проблему — строка без кода: %#v", problems)
	}
}

// Незнакомый код — не падение, а строка с Found=false и понятная проблема.
func TestResolveCompositionUnknownSku(t *testing.T) {
	card := skills.SystemCard{
		Composition: []string{"Мембрана (MAT-МЕМБ-ПВХ-9999)"},
		Links:       []skills.CompositionLink{{Raw: "Мембрана (MAT-МЕМБ-ПВХ-9999)", SKU: "MAT-МЕМБ-ПВХ-9999"}},
	}
	composition, problems := resolveComposition(card, nil)
	if len(composition) != 1 || composition[0].Found {
		t.Fatalf("незнакомый код найденным быть не может: %#v", composition)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "MAT-МЕМБ-ПВХ-9999") {
		t.Fatalf("проблема должна называть код: %#v", problems)
	}
}

// Код ищется без учёта регистра: в карточке могли написать строчными.
func TestResolveCompositionCaseInsensitiveSku(t *testing.T) {
	card := skills.SystemCard{
		Composition: []string{"Мембрана (mat-мемб-пвх-0007)"},
		Links:       []skills.CompositionLink{{Raw: "Мембрана", SKU: "MAT-МЕМБ-ПВХ-0007"}},
	}
	materials := []Material{{ID: 5, SKU: "MAT-МЕМБ-ПВХ-0007", Name: "Мембрана"}}
	composition, problems := resolveComposition(card, materials)
	if len(problems) != 0 || !composition[0].Found {
		t.Fatalf("код не найден без учёта регистра: %#v %#v", composition, problems)
	}
}

// Карточка без состава — ноль строк и одна общая проблема «состав не заполнен».
func TestResolveCompositionEmptyIsProblem(t *testing.T) {
	composition, problems := resolveComposition(skills.SystemCard{Title: "ТН-ПУСТО"}, nil)
	if len(composition) != 0 {
		t.Fatalf("состава быть не должно: %#v", composition)
	}
	if len(problems) != 1 {
		t.Fatalf("пустой состав — это проблема: %#v", problems)
	}
}

// Архивный материал не считается найденным: ссылка на снятый с продажи код —
// именно то, что человек должен увидеть.
func TestResolveCompositionSkipsArchived(t *testing.T) {
	card := skills.SystemCard{
		Composition: []string{"Мембрана (MAT-МЕМБ-ПВХ-0007)"},
		Links:       []skills.CompositionLink{{Raw: "Мембрана", SKU: "MAT-МЕМБ-ПВХ-0007"}},
	}
	materials := []Material{{ID: 5, SKU: "MAT-МЕМБ-ПВХ-0007", Name: "Мембрана", Archived: true}}
	composition, problems := resolveComposition(card, materials)
	if composition[0].Found {
		t.Fatalf("архивный материал найденным быть не должен: %#v", composition[0])
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "удалён") {
		t.Fatalf("проблема должна объяснять причину: %#v", problems)
	}
}

// SystemsFromKB читает ленту и отдаёт ТОЛЬКО системы, уже с составом.
func TestSystemsFromKBReturnsSystemsOnly(t *testing.T) {
	systemNote := noteWith(2, "ТН-БАЗА", []KBTag{{Name: "commercial_propose"}},
		"Мембрана (MAT-МЕМБ-ПВХ-0007)")
	materialNote := noteWith(1, "Мембрана", []KBTag{{Name: "commercial_propose"}, {Name: "material"}})
	kb := &splitKB{
		notes: []KBNoteSummary{systemNote.KBNoteSummary, materialNote.KBNoteSummary},
		byID:  map[int64]KBNote{1: materialNote, 2: systemNote},
	}
	materials := []Material{{ID: 5, SKU: "MAT-МЕМБ-ПВХ-0007", Name: "Мембрана", Unit: "м2"}}

	cards, err := SystemsFromKB(context.Background(), kb, materials)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if len(cards) != 1 || cards[0].Title != "ТН-БАЗА" {
		t.Fatalf("в список систем попал не тот набор: %#v", cards)
	}
	if len(cards[0].CompositionLinks) != 1 || !cards[0].CompositionLinks[0].Found {
		t.Fatalf("состав не разрешён: %#v", cards[0].CompositionLinks)
	}
}

// База знаний не настроена — пустой список без ошибки, как у карточек.
func TestSystemsFromKBNotConfigured(t *testing.T) {
	cards, err := SystemsFromKB(context.Background(), nil, nil)
	if err != nil || cards == nil {
		t.Fatalf("ждём пустой список и отсутствие ошибки: %#v %v", cards, err)
	}
}
