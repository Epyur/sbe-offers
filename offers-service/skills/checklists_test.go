package skills

import (
	"strings"
	"testing"
)

// Чек-лист «нет AI-признаков» адаптирован из coreyhaines31/marketingskills
// (copywriting 2.3.2, references/ai-tells.md, MIT). Здесь проверяется сам
// механизм: какие шаблоны ловятся и что чистый текст не помечается.
func TestCheckAiTellsContrast(t *testing.T) {
	issues := CheckAiTells("Это не дорого, это выгодно.")
	if len(issues) == 0 {
		t.Fatal("контрастный оборот «не X, а Y» должен быть помечен")
	}
	if !strings.Contains(issues[0], "контраст") {
		t.Fatalf("непонятная причина: %q", issues[0])
	}
}

func TestCheckAiTellsNegationList(t *testing.T) {
	issues := CheckAiTells("Без выезда, без шаблонов, без ожидания.")
	if len(issues) == 0 {
		t.Fatal("перечисление «без X, без Y, без Z» должно быть помечено")
	}
}

func TestCheckAiTellsFakeQuestion(t *testing.T) {
	issues := CheckAiTells("Результат? Втрое быстрее.")
	if len(issues) == 0 {
		t.Fatal("вопрос с самому себе ответом («Результат?») должен быть помечен")
	}
}

func TestCheckAiTellsStockPhrase(t *testing.T) {
	issues := CheckAiTells("В современном быстро меняющемся мире это важно.")
	if len(issues) == 0 {
		t.Fatal("запасная заставка должна быть помечена")
	}
}

func TestCheckAiTellsCleanText(t *testing.T) {
	clean := "Гидроизоляция фундамента: площадь 4800 м², мембрана V-SL 2 мм. " +
		"Стоимость материалов 7 571 941,62 руб."
	if issues := CheckAiTells(clean); len(issues) != 0 {
		t.Fatalf("чистый текст не должен давать замечаний: %v", issues)
	}
}

func TestCheckAiTellsEmDashInShortCopy(t *testing.T) {
	issues := CheckAiTells("Гидроизоляция — это просто")
	if len(issues) == 0 {
		t.Fatal("длинное тире в короткой фразе должно быть помечено")
	}
}

// Числа в тексте разрешены только те, что уже есть в предложении: модель не
// имеет права посчитать новое число «примерно». Это защита от выдуманной
// экономии, которая потом не сойдётся со сметой.
func TestCheckClaimNumbersAllowsKnown(t *testing.T) {
	text := "Итого 20 158 633,08 руб. за 9 200 м²."
	issues := CheckClaimNumbers(text, []string{"20 158 633,08", "9 200"})
	if len(issues) != 0 {
		t.Fatalf("разрешённые числа не должны помечаться: %v", issues)
	}
}

func TestCheckClaimNumbersFlagsInvented(t *testing.T) {
	issues := CheckClaimNumbers("Экономия до 40% за счёт долговечности.", nil)
	if len(issues) == 0 {
		t.Fatal("выдуманный процент должен быть помечен")
	}
	if !strings.Contains(issues[0], "40") {
		t.Fatalf("не названо само число: %q", issues[0])
	}
}

func TestCheckClaimNumbersIgnoresUnits(t *testing.T) {
	// Единицы и простые ссылки не должны выглядеть как выдуманные числа.
	issues := CheckClaimNumbers("Мембрана 2 мм, ПВХ, класс B1.", []string{"2"})
	if len(issues) != 0 {
		t.Fatalf("единицы измерения не должны быть замечанием: %v", issues)
	}
}

// Структура секции: вводный текст — 2–4 предложения, ничего не должно быть
// пустым или длиннее разумного.
func TestCheckSectionStructureIntro(t *testing.T) {
	if issues := CheckSectionStructure("Предлагаем материалы для гидроизоляции фундамента.", SectionIntro); len(issues) != 0 {
		t.Fatalf("короткий вводный текст должен проходить: %v", issues)
	}
	if issues := CheckSectionStructure("", SectionIntro); len(issues) == 0 {
		t.Fatal("пустая секция — замечание")
	}
	long := strings.Repeat("Слово ", 400)
	if issues := CheckSectionStructure(long, SectionIntro); len(issues) == 0 {
		t.Fatal("вводный текст в 400 слов — замечание")
	}
}

func TestCheckSectionStructureAdvantages(t *testing.T) {
	issues := CheckSectionStructure("Долговечность, надёжность", SectionAdvantages)
	if len(issues) == 0 {
		t.Fatal("преимущества должны быть списком, а не одной строкой через запятую")
	}
	ok := "• Служит 50 лет по расчёту\n• Выдерживает давление грунтовых вод\n• Ремонтопригодна"
	if issues := CheckSectionStructure(ok, SectionAdvantages); len(issues) != 0 {
		t.Fatalf("список из трёх пунктов проходит: %v", issues)
	}
}
