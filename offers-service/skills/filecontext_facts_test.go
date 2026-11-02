package skills

import (
	"strings"
	"testing"
)

// Дословный разбор контекста из файла (2026-11-01).
//
// Живой случай: менеджер прислал старый КП и сказал «информация в файле».
// Объект и площадь в файле есть — их нельзя терять.

func legacyFileLines() []string {
	return []string{
		"Корпорация ТЕХНОНИКОЛЬ",
		"129110, Россия, Москва, улица Гиляровского, дом 47, строение 5",
		"«_12_» октября 2026 г. | Региональный менеджер",
		"Предлагаем Вам, для объекта: Многоквартирный многоэтажный дом со встроенными помещениями обслуживания жилой застройки и подземной автостоянкой по ул. Охотская в Заельцовском районе города Новосибирска, расположенный на земельном участке с кадастровым номером 54:35:031350:134,",
		"использовать материалы «Корпорации ТехноНИКОЛЬ» по специальным ценам.",
		"ТН-ФУНДАМЕНТ Дренаж Проф | СОСТАВ СИСТЕМЫ",
		"Описание системы | Преимущества системы",
		"Ремонтопригодная система изоляции подземных конструкций c однослойной гидроизоляционной полимерной мембраной LOGICBASE, утеплением и дренажом.",
		"СИСТЕМА ТН-СТИЛОБАТ ЭКСПЕРТ ТРОТУАР | СОСТАВ СИСТЕМЫ",
		"Общая площадь: | м2 | 9200",
	}
}

func TestExtractBriefFactsFindsObject(t *testing.T) {
	got := ExtractBriefFacts(legacyFileLines())
	name := got.Fields["object_name"]
	if len(name) == 0 {
		t.Fatalf("объект не найден: %+v", got.Fields)
	}
	if !strings.Contains(name[0], "Охотская") {
		t.Fatalf("объект = %q, ожидали фразу про Охотскую", name[0])
	}
	if strings.HasSuffix(strings.TrimSpace(name[0]), ",") {
		t.Fatalf("объект с висячей запятой: %q", name[0])
	}
	if strings.Contains(name[0], "использовать материалы") {
		t.Fatalf("в объект попало продолжение фразы: %q", name[0])
	}
}

func TestExtractBriefFactsFindsPurpose(t *testing.T) {
	got := ExtractBriefFacts(legacyFileLines())
	if len(got.Fields["object_purpose"]) == 0 {
		t.Fatalf("назначение не выведено из наименования: %+v", got.Fields)
	}
	if !strings.Contains(strings.ToLower(got.Fields["object_purpose"][0]), "дом") {
		t.Fatalf("назначение = %q", got.Fields["object_purpose"][0])
	}
}

func TestExtractBriefFactsFindsArea(t *testing.T) {
	got := ExtractBriefFacts(legacyFileLines())
	area := got.Fields["object_area"]
	if len(area) == 0 {
		t.Fatalf("площадь не найдена: %+v", got.Fields)
	}
	if !strings.Contains(area[0], "9200") && !strings.Contains(area[0], "9 200") {
		t.Fatalf("площадь = %q", area[0])
	}
}

func TestExtractBriefFactsFindsSystems(t *testing.T) {
	got := ExtractBriefFacts(legacyFileLines())
	systems := got.Fields["planned_systems"]
	if len(systems) < 2 {
		t.Fatalf("систем найдено %d, ожидали две: %#v", len(systems), systems)
	}
	joined := strings.Join(systems, " | ")
	if !strings.Contains(joined, "ФУНДАМЕНТ") || !strings.Contains(joined, "СТИЛОБАТ") {
		t.Fatalf("системы = %#v", systems)
	}
	// «СОСТАВ СИСТЕМЫ» — подпись графы, а не система.
	for _, s := range systems {
		if strings.Contains(s, "|") || strings.Contains(strings.ToLower(s), "состав") {
			t.Fatalf("подпись графы попала в системы: %q", s)
		}
	}
}

func TestExtractBriefFactsKeepsDescriptionInNotes(t *testing.T) {
	got := ExtractBriefFacts(legacyFileLines())
	if !strings.Contains(got.Notes, "LOGICBASE") {
		t.Fatalf("описание системы потерялось: %q", got.Notes)
	}
	if !got.HasContent() {
		t.Fatal("HasContent() = false при непустом результате")
	}
}

func TestExtractBriefFactsEmptyFileGivesNothing(t *testing.T) {
	got := ExtractBriefFacts([]string{"", "  ", "\n"})
	if got.HasContent() {
		t.Fatalf("пустой файл дал содержимое: %+v", got.Fields)
	}
}

// Объект почти в каждой строке — не повод повторять его в каждом поле.
func TestExtractBriefFactsDeduplicates(t *testing.T) {
	lines := []string{
		"для объекта: Многоквартирный дом по улице Ленина, 5, расположенный в городе Твери",
		"для объекта: Многоквартирный дом по улице Ленина, 5, расположенный в городе Твери",
	}
	got := ExtractBriefFacts(lines)
	if len(got.Fields["object_name"]) != 1 {
		t.Fatalf("объект повторён: %#v", got.Fields["object_name"])
	}
}

// Живая форма файла менеджера: «Объект: …», «Площадь: … м2», «Системы: …».
// Здесь подпись «Объект», а не «для объекта», и карточка — не таблица.
func TestExtractBriefFactsPlainLabels(t *testing.T) {
	got := ExtractBriefFacts([]string{
		"Объект: Многоквартирный дом по ул. Охотская в Заельцовском районе Новосибирска",
		"Назначение: жилой дом",
		"Площадь: 9 200 м2",
		"Системы: ТН-ФУНДАМЕНТ Дренаж Проф",
	})
	if len(got.Fields["object_name"]) == 0 {
		t.Fatalf("объект по подписи «Объект:» не найден: %#v", got.Fields)
	}
	// Площадь — «9 200 м2», а не двойка из «м2».
	if area := got.Fields["object_area"]; len(area) == 0 || !strings.Contains(area[0], "200") {
		t.Fatalf("площадь = %#v, ожидали 9 200 м2", area)
	}
}
