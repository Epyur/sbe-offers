package main

// Содержательные разделы боевого листка правятся в настройках листка
// (2026-11-18, просьба пользователя: вынести структуру разделов из кода в
// настройку, вариант A — «только разделы»).
//
// Здесь сторожатся два правила, которых не видно из окна редактора:
//   - пустое сохранение отклоняется. Без этого администратор удалил бы все
//     разделы, документ молча печатал бы только карточки, и человек считал бы,
//     что «так и настроил» (класс «тихая потеря настройки»);
//   - редактору отдаётся ЗАГОТОВКА (`sectionsDefault`), иначе кнопка
//     «↩ Вернуть заготовку» возвращала бы пустоту, то есть не работала.
//
// Зеркало клиентской проверки — `combatSectionsProblem` в
// `src/ui/sections-model.ts`; тексты обеих сторон сверяет
// `scripts/check_combat_sections_rules.mjs`.

import (
	"strings"
	"testing"
)

// Пустая форма листка не сохраняется, и отказ говорит, что делать.
func TestCombatFormRejectsEmptySections(t *testing.T) {
	form := combatDefaultForm()
	form.Sections = nil
	msg := validateForm(form)
	if msg == "" {
		t.Fatal("пустая форма листка прошла проверку: листок молча печатал бы только карточки")
	}
	// Отказ обязан называть выход: иначе человек видит тупик.
	if !strings.Contains(msg, "раздел") {
		t.Fatalf("отказ не говорит про разделы: %s", msg)
	}
	if !strings.Contains(msg, "заготовку") {
		t.Fatalf("отказ не называет кнопку «Вернуть заготовку»: %s", msg)
	}
}

// Пустая форма КП — законное состояние: у КП может не быть содержательных
// разделов вовсе, и печатать он от этого не перестаёт.
func TestClientFormAllowsEmptySections(t *testing.T) {
	form := defaultForm(clientFormMode)
	form.Sections = []ContentSection{}
	if msg := validateForm(form); msg != "" {
		t.Fatalf("форма КП без разделов не сохраняется: %s", msg)
	}
}

// Заготовка отдаётся редактору отдельным полем и НЕ попадает в разделы:
// «разделов нет» и «разделы по умолчанию» — разные состояния, и подменять
// одно другим на чтении значило бы потерять факт правки администратора.
func TestCombatFormCarriesSectionsDefault(t *testing.T) {
	form := defaultForm(combatFormMode)
	if len(form.SectionsDefault) == 0 {
		t.Fatal("форма листка не несёт заготовку разделов: кнопке «Вернуть заготовку» нечего вернуть")
	}
	if len(form.Sections) != len(form.SectionsDefault) {
		t.Fatalf("у формы по умолчанию разделов %d, в заготовке %d — ожидались одинаковые",
			len(form.Sections), len(form.SectionsDefault))
	}
	// Заготовка обязана проходить проверку формы: иначе «Вернуть заготовку»
	// давала бы состояние, которое нельзя сохранить.
	restored := combatDefaultForm()
	restored.Sections = restored.SectionsDefault
	if msg := validateForm(restored); msg != "" {
		t.Fatalf("заготовка не сохраняется: %s", msg)
	}
}

// Пустой список разделов у листка значит ЗАГОТОВКУ (2026-11-18): печать при
// пустом списке подставляет заготовку, и окно настроек обязано показывать то же,
// что печатается. На стенде нашлась сохранённая форма листка с пустыми
// разделами — она печатала четыре раздела, а окно показало бы «разделов нет».
func TestCombatFormEmptySectionsMeanDefault(t *testing.T) {
	form := withCombatSectionsDefault(OfferForm{Mode: combatFormMode, Sections: []ContentSection{}})
	if len(form.Sections) == 0 {
		t.Fatal("пустые разделы листка не заполнены заготовкой: окно и документ разойдутся")
	}
	if len(form.Sections) != len(combatDefaultSections()) {
		t.Fatalf("подставлено %d разделов, в заготовке %d", len(form.Sections), len(combatDefaultSections()))
	}
	// Заданные разделы не трогаются: подстановка — только для пустого списка.
	own := []ContentSection{{ID: "own", Title: "Свой"}}
	if got := withCombatSectionsDefault(OfferForm{Mode: combatFormMode, Sections: own}); len(got.Sections) != 1 || got.Sections[0].ID != "own" {
		t.Fatalf("заданные разделы листка подменены: %+v", got.Sections)
	}
	// У формы КП подстановки нет: там пустой список — законное состояние.
	if got := withCombatSectionsDefault(OfferForm{Mode: clientFormMode}); len(got.Sections) != 0 {
		t.Fatalf("форма КП получила разделы листка: %d", len(got.Sections))
	}
}

// У КП заготовки нет: её разделы задаёт администратор с нуля.
func TestSectionsDefaultOnlyForCombat(t *testing.T) {
	if form := defaultForm(clientFormMode); len(form.SectionsDefault) != 0 {
		t.Fatalf("форма КП несёт заготовку разделов листка: %d", len(form.SectionsDefault))
	}
}

// Заготовка не отдаётся клиенту как есть дважды: повторный вызов помощника не
// плодит копий и не затирает уже заданную заготовку.
func TestWithCombatSectionsDefaultKeepsGivenValue(t *testing.T) {
	custom := []ContentSection{{ID: "own", Title: "Свой", Subsections: []ContentSubsection{
		{ID: "one", Title: "Один", Instruction: "Напиши что-нибудь"},
	}}}
	form := withCombatSectionsDefault(OfferForm{Mode: combatFormMode, SectionsDefault: custom})
	if len(form.SectionsDefault) != 1 || form.SectionsDefault[0].ID != "own" {
		t.Fatalf("заданная заготовка затёрта: %+v", form.SectionsDefault)
	}
	filled := withCombatSectionsDefault(OfferForm{Mode: combatFormMode})
	if len(filled.SectionsDefault) == 0 {
		t.Fatal("пустая заготовка не заполнена встроенной")
	}
}
