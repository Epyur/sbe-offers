package main

// Заготовка разделов боевого листка (2026-11-16, шаг 2a плана sbe-offers).
//
// Проверяется то, от чего зависит первый же листок менеджера:
//   - заготовка ВАЛИДНА по `validateForm` (иначе форму нельзя сохранить, и
//     печать молча откатилась бы в карточный вид);
//   - подраздел сравнения просит ТАБЛИЦУ: «повествование вместо таблицы» в
//     сравнении характеристик — та самая жалоба, из-за которой разделы и
//     заводились;
//   - ключевой раздел «где конкурент сильнее» есть и печатается всегда —
//     пустота в нём читалась бы как недоделанный листок (§9.4);
//   - повтор по продуктам идёт по обеим сторонам.

import (
	"strings"
	"testing"
)

func TestCombatDefaultSectionsValid(t *testing.T) {
	form := combatDefaultForm()
	if msg := validateForm(form); msg != "" {
		t.Fatalf("заготовка формы листка не проходит проверку: %s", msg)
	}
	if len(form.Sections) < 3 {
		t.Fatalf("в заготовке %d разделов, ожидалось не меньше 3", len(form.Sections))
	}
	if len(form.Blocks) != 0 {
		t.Fatalf("у листка не должно быть блоков вёрстки, а их %d", len(form.Blocks))
	}
}

// Сравнение характеристик должно просить TABЛИЦУ: это контракт ответа
// (`table` в `sectionAnswerContract`) и то, ради чего раздел заведён.
func TestCombatDefaultSpecsAskForTable(t *testing.T) {
	var found bool
	for _, section := range combatDefaultForm().Sections {
		for _, sub := range section.Subsections {
			if sub.ID != "specs" {
				continue
			}
			found = true
			if !strings.Contains(strings.ToLower(sub.Instruction), "таблиц") {
				t.Fatalf("инструкция сравнения характеристик не просит таблицу: %q", sub.Instruction)
			}
			// И запрещает выдумывать данные: сравнение по карточкам, а не по
			// «типовым значениям» модели.
			if !strings.Contains(sub.Instruction, "выдумывай") && !strings.Contains(sub.Instruction, "не указано") {
				t.Fatalf("инструкция не защищает от выдуманных цифр: %q", sub.Instruction)
			}
		}
	}
	if !found {
		t.Fatal("в заготовке нет подраздела сравнения характеристик (id=specs)")
	}
}

// Раздел честности есть и печатается ВСЕГДА: пустой ответ на вопрос «где
// конкурент сильнее» — это не «сравнение не нужно», а недоделанный листок.
func TestCombatDefaultHasWeaknessSection(t *testing.T) {
	var weakness *ContentSection
	for i := range combatDefaultForm().Sections {
		section := combatDefaultForm().Sections[i]
		if section.ID == "weakness" {
			weakness = &section
		}
	}
	if weakness == nil {
		t.Fatal("в заготовке нет раздела «где конкурент сильнее»")
	}
	if len(weakness.Subsections) == 0 {
		t.Fatal("в разделе честности нет подразделов")
	}
	for _, sub := range weakness.Subsections {
		if sub.ConditionOrAlways() != conditionAlways {
			t.Fatalf("подраздел честности печатается не всегда: %q", sub.Condition)
		}
	}
}

// Повтор по продуктам обязан идти по системам (`repeat: systems`): иначе текст
// напишется один раз и «каждому продукту» будет посвящено одно общее место.
func TestCombatDefaultPerProductRepeat(t *testing.T) {
	var found bool
	for _, section := range combatDefaultForm().Sections {
		for _, sub := range section.Subsections {
			if sub.Repeat == blockRepeatSystems {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("в заготовке нет подраздела с повтором по продуктам (repeat: systems)")
	}
}

// Заготовка выдерживает основной путь печати: на реальной паре карточек
// подразделы планово вызываются и повторяемый идёт по обеим сторонам.
func TestCombatDefaultSectionsPlanCalls(t *testing.T) {
	sheet, our, competitor := combatSectionFixture()
	form := combatDefaultForm()
	gen, offer := combatSectionGen(sheet, []SystemCard{our}, []SystemCard{competitor}, nil, "")
	_ = gen
	cards := combatSectionCards([]SystemCard{our}, []SystemCard{competitor})
	total := 0
	repeated := 0
	for _, section := range form.Sections {
		calls := sectionCalls(section, offer, cards)
		total += len(calls)
		for _, call := range calls {
			if call.system != nil {
				repeated++
			}
		}
	}
	// overview + specs + diagram + risks = 4, плюс повтор по двум продуктам = 2.
	if total != 6 {
		t.Fatalf("план вызовов заготовки — %d, ожидалось 6", total)
	}
	if repeated != 2 {
		t.Fatalf("повтор по продуктам дал %d вызовов, ожидалось 2", repeated)
	}
}

// Схема сравнения есть в заготовке (2026-11-17, шаг 6 плана): вид блока
// `diagram` появился шагом 5, и заготовка обязана его использовать — иначе
// администратору неоткуда узнать, что схемы в листке поддерживаются.
func TestCombatDefaultHasDiagramSection(t *testing.T) {
	var found *ContentSubsection
	for _, section := range combatDefaultForm().Sections {
		for i := range section.Subsections {
			if section.ID == "scheme" || strings.Contains(strings.ToLower(section.Subsections[i].Instruction), "схему") {
				found = &section.Subsections[i]
			}
		}
	}
	if found == nil {
		t.Fatal("в заготовке нет подраздела со схемой (diagram)")
	}
	lower := strings.ToLower(found.Instruction)
	if !strings.Contains(lower, "схем") {
		t.Fatalf("инструкция схемы не просит схему: %q", found.Instruction)
	}
	if !strings.Contains(lower, "mermaid") {
		t.Fatalf("инструкция схемы не называет mermaid (единственный поддерживаемый язык): %q", found.Instruction)
	}
}
