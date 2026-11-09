package skills

import (
	"strings"
	"testing"
)

// Ссылки карточки (2026-11-01). Карточки систем и материалов несут раздел
// «## Ссылки»: страница на корпоративном сайте и PDF техдокументации на
// навигаторе (nav.tn.ru). В КП эти ссылки нужны в конце документа, а модели —
// как контекст, чтобы она о них могла сказать словами, но не выдумывала URL.
//
// Проверено на живых данных: из 375 системных карточек раздел есть у 374, из
// 1166 материалов — у 1165.

const cardRefsSample = `---
sbe_kb_id: 67544c74
tags: [commercial_propose]
---

# ТН-КРОВЛЯ Авто

## Состав системы

- **1** Асфальтобетон

## Ссылки

- Страница системы: https://nav.tn.ru/systems/ploskaya-krysha/tn-krovlya-avto/
- [Техлист ПК_10000016 ТН_КРОВЛЯ Авто](https://nav.tn.ru/pim/d/7/e/e/d7ee.pdf?response-content-disposition=filename="Техлист.pdf")
- [Узлы ТН_КРОВЛЯ Авто PDF 10_2024](https://nav.tn.ru/pim/c/8/e/c/c8ec.pdf)
- https://nav.tn.ru/systems/ploskaya-krysha/tn-krovlya-avto/
`

func TestParseCardRefsFindsCompositionCardLinks(t *testing.T) {
	refs := ParseCardRefs(cardRefsSample)
	if len(refs) != 3 {
		t.Fatalf("ожидали три ссылки (повтор URL схлопывается), получили %d: %#v", len(refs), refs)
	}
	if refs[0].URL != "https://nav.tn.ru/systems/ploskaya-krysha/tn-krovlya-avto/" {
		t.Fatalf("первая ссылка: %#v", refs[0])
	}
	if refs[0].Kind != RefKindPage {
		t.Fatalf("страница системы должна иметь вид %q, а не %q", RefKindPage, refs[0].Kind)
	}
	if refs[0].Label != "Страница системы" {
		t.Fatalf("подпись страницы: %q", refs[0].Label)
	}
	if refs[1].Kind != RefKindDoc {
		t.Fatalf("PDF-техлист должен иметь вид %q, а не %q", RefKindDoc, refs[1].Kind)
	}
	if refs[1].Label != "Техлист ПК_10000016 ТН_КРОВЛЯ Авто" {
		t.Fatalf("подпись техлиста: %q", refs[1].Label)
	}
}

// Текст вне раздела «Ссылки» ссылкой карточки не считается: в описании и в
// составе бывают ссылки на материалы, и печатать их в конце КП — не то, что
// просили.
func TestParseCardRefsIgnoresLinksOutsideSection(t *testing.T) {
	md := "# Система\n\n## Описание\n\nСмотри https://example.com\n\n## Ссылки\n\n- Страница системы: https://nav.tn.ru/x/\n"
	refs := ParseCardRefs(md)
	if len(refs) != 1 || refs[0].URL != "https://nav.tn.ru/x/" {
		t.Fatalf("ждём только ссылку из раздела «Ссылки»: %#v", refs)
	}
}

// Карточка без раздела «Ссылки» — не ошибка и не пустая ссылка.
func TestParseCardRefsWithoutSection(t *testing.T) {
	if refs := ParseCardRefs("# Система\n\n## Описание\n\nтекст\n"); len(refs) != 0 {
		t.Fatalf("ожидали пустой список, получили %#v", refs)
	}
}

// Голый URL без подписи: подпись — сам адрес, иначе в документе получится
// строка « — https://…» с пустым именем.
func TestParseCardRefsBareURLGetsURLLabel(t *testing.T) {
	md := "## Ссылки\n\n- https://nav.tn.ru/pim/abc.pdf\n"
	refs := ParseCardRefs(md)
	if len(refs) != 1 {
		t.Fatalf("ожидали одну ссылку: %#v", refs)
	}
	if refs[0].Label != "https://nav.tn.ru/pim/abc.pdf" {
		t.Fatalf("подпись голого URL: %q", refs[0].Label)
	}
	if refs[0].Kind != RefKindDoc {
		t.Fatalf("pdf без подписи — документ, а не %q", refs[0].Kind)
	}
}

// Заголовок «Ссылки» может быть уровнем 2 или 3: технолог пишет по-разному.
func TestParseCardRefsAnyHeadingLevel(t *testing.T) {
	md := "### Ссылки\n\n- Страница материала: https://nav.tn.ru/catalog/a/\n"
	refs := ParseCardRefs(md)
	if len(refs) != 1 {
		t.Fatalf("раздел третьего уровня не разобран: %#v", refs)
	}
}

// Ссылки попадают в карточку через разбор и не ломают состав и описание.
// Адрес с пробелами и кавычками внутри имени файла разбирается целиком.
//
// Живой дефект КП 003/2026 (2026-11-09): техлисты навигатора приходят строкой
// `[название](https://…pdf?response-content-disposition=filename="название.pdf")`,
// и пробелы в имени файла ломали markdown-разбор. Ссылка проваливалась в
// «голый адрес», обрезанный по кавычке, а подпись терялась — в документ уходил
// сырой обрезок адреса вместо названия документа.
func TestParseCardRefsURLWithSpacesAndQuotes(t *testing.T) {
	md := "## Ссылки\n\n" +
		`- [Техлист ДНС от ФУНДАМЕНТ Дренаж Проф](https://nav.tn.ru/pim/abc.pdf?response-content-disposition=filename="Техлист ДНС от ФУНДАМЕНТ.pdf")` + "\n"
	refs := ParseCardRefs(md)
	if len(refs) != 1 {
		t.Fatalf("ожидали одну ссылку, получили %d: %#v", len(refs), refs)
	}
	want := `https://nav.tn.ru/pim/abc.pdf?response-content-disposition=filename="Техлист ДНС от ФУНДАМЕНТ.pdf"`
	if refs[0].URL != want {
		t.Fatalf("адрес разобран неверно:\n получили: %q\n ожидали: %q", refs[0].URL, want)
	}
	if refs[0].Label != "Техлист ДНС от ФУНДАМЕНТ Дренаж Проф" {
		t.Fatalf("подпись потеряна: %q", refs[0].Label)
	}
	if refs[0].Kind != RefKindDoc {
		t.Fatalf("техлист должен быть документом, а не %q", refs[0].Kind)
	}
}

func TestParseSystemCardKeepsRefs(t *testing.T) {
	card := ParseSystemCard(cardRefsSample)
	if len(card.Refs) != 3 {
		t.Fatalf("в карточке системы %d ссылок: %#v", len(card.Refs), card.Refs)
	}
	if len(card.Composition) != 1 {
		t.Fatalf("состав потерялся: %#v", card.Composition)
	}
}

// Для модели ссылки идут текстом: название и адрес. Модель не должна
// выдумывать URL — ей можно только назвать факт, что документация приложена.
func TestSystemCardPromptTextIncludesRefs(t *testing.T) {
	card := ParseSystemCard(cardRefsSample)
	text := card.PromptText()
	if !strings.Contains(text, "https://nav.tn.ru/systems/ploskaya-krysha/tn-krovlya-avto/") {
		t.Fatalf("адрес страницы системы не попал в текст для модели:\n%s", text)
	}
}
