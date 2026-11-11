package main

import (
	"testing"
)

// Отсев кандидатов и сбор по запросам (2026-11-12).
//
// Живые прогоны на боевой базе показали, что одного поиска мало: по запросу
// «мембрана кровельная» честно находятся «Активатор для ПВХ мембран» и
// «Очиститель для ПВХ мембран», а по «плиты» — «SIKA BOOM» (монтажная пена) и
// «Система кровли PIR-Кровля Эксперт». Заменой нашему товару они не являются.

func TestIsAnalogCandidateRejectsSecondary(t *testing.T) {
	source := SystemCard{NoteID: 1, Title: "Кровельная ПВХ-мембрана LOGICROOF® V-RP"}
	membrane := analogStrongClassOf(source.Title)
	for _, title := range []string{
		"Активатор для ПВХ мембран, 1 л",
		"Очиститель для ПВХ мембран, 3 л",
		"Фартук из ПВХ мембраны для воронок",
		"Воронка кровельная V-PVC 110x720",
		"ШАЙБА КРОВЕЛЬНАЯ",
		"ДЮБЕЛЬ РАСПОРНЫЙ PROPLUG",
		"Кронштейн ограждения Fachmann PL_PVC_H57",
		"Система кровли PIR-Кровля Эксперт",
		"SikaRoof® Cleaner 2000 RU",
	} {
		card := SystemCard{NoteID: 999, Title: title}
		if isAnalogCandidate(source, card, membrane) {
			t.Fatalf("второстепенный товар принят аналогом: %q", title)
		}
	}
}

func TestIsAnalogCandidateAcceptsMembranes(t *testing.T) {
	source := SystemCard{NoteID: 1, Title: "Кровельная ПВХ-мембрана LOGICROOF® V-RP"}
	membrane := analogStrongClassOf(source.Title)
	for _, title := range []string{
		"Кровельная ПВХ мембрана FACHMANN PVC-RP 2,0мм 2_15м, серая",
		"Кровельная ПВХ-мембрана PVC-RP",
		"ПВХ мембрана PLASTFOIL® CLASSIC",
		"PIRRO®Membrane",
		// Sikaplan класс в названии не несёт — продукт описан мембраной в
		// карточке; такое пропускаем, иначе у Sika не будет аналогов вовсе.
		"Sikaplan® G-12 RU",
		"SikaProof®-754",
	} {
		card := SystemCard{NoteID: 999, Title: title}
		if !isAnalogCandidate(source, card, membrane) {
			t.Fatalf("настоящий аналог отвергнут: %q", title)
		}
	}
}

func TestIsAnalogCandidateSkipsOwnAndForeignClass(t *testing.T) {
	source := SystemCard{NoteID: 1, Title: "Кровельная ПВХ-мембрана LOGICROOF® V-RP"}
	membrane := analogStrongClassOf(source.Title)
	if isAnalogCandidate(source, source, membrane) {
		t.Fatal("наш продукт не может быть своим аналогом")
	}
	// Карточка сама себя называет герметиком — мембраной она не станет.
	if isAnalogCandidate(source, SystemCard{NoteID: 2, Title: "Герметик кровельный"}, membrane) {
		t.Fatal("чужая классовая карточка принята аналогом")
	}
}

func TestSameStrongClass(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"плит", "плит", true},
		{"плит", "утеплит", true},
		{"утеплит", "плит", true},
		{"мембран", "гидроиз", true},
		{"плит", "гермет", false},
		{"мембран", "плит", false},
	}
	for _, c := range cases {
		if got := sameStrongClass(c.a, c.b); got != c.want {
			t.Fatalf("%q/%q: %v, ожидалось %v", c.a, c.b, got, c.want)
		}
	}
}

func TestAnalogClassWordsCoversClasses(t *testing.T) {
	// Каждый класс из списка товарных должен иметь словоформу для запроса.
	for _, class := range analogStrongClasses {
		if analogStrongClassText(class) == "" {
			t.Fatalf("для класса %q нет словоформы", class)
		}
	}
	// Неизвестный класс уходит как есть: поиск по префиксам стерпит основу.
	if got := analogStrongClassText("нечто"); got != "нечто" {
		t.Fatalf("неизвестный класс подменён: %q", got)
	}
}

func TestAnalogCoreOfKeepsClassFirst(t *testing.T) {
	// «PIR-плита» — товар, PIR не марка: класс остаётся в ядре.
	if got := analogCoreOf("PIR-плита и теплоизоляция PIR"); got == "" {
		t.Fatal("ядро пустое")
	}
	// Совсем без товарных слов ядро собирается из того, что осталось — но не
	// выдумывает содержимое.
	if got := analogCoreOf("PROF"); got != "PROF" {
		t.Fatalf("ядро из одного слова: %q", got)
	}
	if got := analogCoreOf(""); got != "" {
		t.Fatalf("пустое название дало ядро %q", got)
	}
	if got := analogCoreOf("Кровельная"); got == "" {
		t.Fatal("название из одного применения дало пустое ядро")
	}
}

func TestAnalogPurposeWords(t *testing.T) {
	purposes := analogPurposeWords("Кровельная ПВХ-мембрана LOGICROOF® V-RP")
	if len(purposes) == 0 || purposes[0] != "кровельная" {
		t.Fatalf("применение не найдено: %q", purposes)
	}
	// Назначение из описания: у LOGICPIR VENT в названии его нет.
	vent := analogPurposeWords("Плиты теплоизоляционные LOGICPIR® VENT " +
		"предназначенные для изготовления коробов воздуховодов систем вентиляции")
	if len(vent) == 0 || vent[0] != "воздуховод" {
		t.Fatalf("назначение из описания потеряно: %q", vent)
	}
	if got := analogPurposeWords("Мембрана"); len(got) != 0 {
		t.Fatalf("без назначения ничего не ждём: %q", got)
	}
}

func TestAnalogDescriptionHead(t *testing.T) {
	long := make([]rune, 0, 600)
	for i := 0; i < 600; i++ {
		long = append(long, 'а')
	}
	head := analogDescriptionHead(string(long))
	if len([]rune(head)) != 400 {
		t.Fatalf("описание не обрезано до 400: %d", len([]rune(head)))
	}
	if got := analogDescriptionHead("  короткое  "); got != "короткое" {
		t.Fatalf("короткое описание испорчено: %q", got)
	}
}
