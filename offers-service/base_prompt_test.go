package main

// Базовый промт документа (2026-11-17, просьба пользователя).
//
// Живой листок 017/2026 показал противоречивый текст: в одном подразделе «мы
// сильнее по прочности на сжатие», в другом заголовке — «плиты равноценны».
// Подразделы пишутся РАЗНЫМИ вызовами модели и друг друга не видят, поэтому
// требования к формулировкам нужны в общей части промта — и править их должен
// администратор в форме, а не выпуск плагина.

import (
	"strings"
	"testing"
)

// Базовый промт из формы заменяет встроенный контракт в брифе листка.
func TestFormBasePromptReplacesDefaultContract(t *testing.T) {
	_, our, competitor := combatSectionFixture()
	custom := "ПРАВИЛО АДМИНИСТРАТОРА: не называй продукты равноценными, если значения различаются."
	brief := combatSectionBrief("Сравни коротко", []SystemCard{our}, []SystemCard{competitor}, "", nil, custom)
	if !strings.Contains(brief, custom) {
		t.Fatalf("базовый промт формы не попал в бриф:\n%s", brief)
	}
	if strings.Contains(brief, "Разрешены только эти шесть видов блоков") {
		t.Fatalf("встроенный контракт остался при своём базовом промте:\n%s", brief)
	}
	// Пусто — прежнее поведение: встроенный контракт на месте.
	plain := combatSectionBrief("Сравни коротко", []SystemCard{our}, []SystemCard{competitor}, "", nil, "")
	if !strings.Contains(plain, "Разрешены только эти шесть видов блоков") {
		t.Fatalf("без своего промта пропал встроенный контракт:\n%s", plain)
	}
}

// Свой базовый промт входит в ключ кэша: правка текста переписывает раздел.
func TestFormBasePromptChangesSectionKey(t *testing.T) {
	call := sectionCall{sub: ContentSubsection{ID: "intro", Instruction: "Вступление"}}
	plain := sectionCacheKey("general", call, "1", "")
	if plain != sectionCacheKey("general", call, "1", sectionAnswerContract) {
		t.Fatal("встроенный базовый промт не должен менять ключ кэша")
	}
	custom := sectionCacheKey("general", call, "1", "ПРАВИЛО АДМИНИСТРАТОРА")
	if custom == plain {
		t.Fatal("свой базовый промт не вошёл в ключ кэша: правка промта не перепишет текст")
	}
}

// Правка формы (промт или разделы) обязана пересобрать файл листка.
func TestCombatDocxHashSeesForm(t *testing.T) {
	sheet, _, _ := combatSectionFixture()
	layout := []byte("layout-v1")
	base := combatDocxHash(sheet, layout, "v1", combatFormFingerprint(OfferForm{}))
	custom := OfferForm{BasePrompt: "ПРАВИЛО АДМИНИСТРАТОРА"}
	if combatDocxHash(sheet, layout, "v1", combatFormFingerprint(custom)) == base {
		t.Fatal("правка базового промта не меняет хэш файла: кэш отдаст старый листок")
	}
	sections := OfferForm{Sections: combatDefaultSections()}
	if combatDocxHash(sheet, layout, "v1", combatFormFingerprint(sections)) == base {
		t.Fatal("правка разделов не меняет хэш файла: кэш отдаст старый листок")
	}
}

// Форма по умолчанию отдаёт редактору встроенный текст промта.
func TestDefaultFormCarriesBasePromptDefault(t *testing.T) {
	for _, mode := range []string{clientFormMode, combatFormMode} {
		if form := defaultForm(mode); strings.TrimSpace(form.BasePromptDefault) == "" {
			t.Fatalf("форма %q не несёт встроенный базовый промт: редактору нечего показать", mode)
		}
	}
}
