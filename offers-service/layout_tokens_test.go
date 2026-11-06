package main

import (
	"strings"
	"testing"
)

// Плейсхолдеры макета .docx (2026-11-05, расширены 2026-11-06).
//
// Пользователь загружает свой документ Word и ставит в нём два вида меток:
// `[…]` — жёсткая подстановка значения, `{…}` — промт модели. Промт можно
// писать одним, двумя и тремя фигурными скобками: человек набирает так, как
// ему удобно, и система обязана понять любой из трёх видов.

// Обе метки находятся в порядке появления и с верным видом.
func TestScanLayoutTokensFindsBothKinds(t *testing.T) {
	text := "Объект: [Наименование объекта]. {Опиши систему кратко}."
	tokens := scanLayoutTokens(text, nil)
	if len(tokens) != 2 {
		t.Fatalf("найдено %d меток: %+v", len(tokens), tokens)
	}
	if tokens[0].Kind != layoutTokenField || tokens[0].Inner != "Наименование объекта" {
		t.Fatalf("первая метка не поле: %+v", tokens[0])
	}
	if tokens[1].Kind != layoutTokenPrompt || tokens[1].Inner != "Опиши систему кратко" {
		t.Fatalf("вторая метка не промт: %+v", tokens[1])
	}
	if tokens[0].Start > tokens[1].Start {
		t.Fatal("метки не упорядочены по месту в тексте")
	}
}

// `{{object_name}}` при известном поле — жёсткая подстановка, а не промт.
func TestScanLayoutTokensBraceField(t *testing.T) {
	values := map[string]string{"client_name": "ООО Поэма"}
	tokens := scanLayoutTokens("Кому: {{client_name}}", values)
	if len(tokens) != 1 || tokens[0].Kind != layoutTokenField || tokens[0].Inner != "client_name" {
		t.Fatalf("двойная скобка разобрана неверно: %+v", tokens)
	}
}

// Русское имя поля в двойных скобках (`{{Объект}}`) — тоже подстановка.
func TestScanLayoutTokensBraceFieldByAlias(t *testing.T) {
	tokens := scanLayoutTokens("{{Объект}}", map[string]string{})
	if len(tokens) != 1 || tokens[0].Kind != layoutTokenField {
		t.Fatalf("русское имя поля в двойных скобках не узнано: %+v", tokens)
	}
}

// Двойные скобки с длинной инструкцией — промт, хотя скобок две. Ровно этот
// случай ушёл в макет 003/2026: 364 символа текста, и метка не находилась.
func TestScanLayoutTokensDoubleBracePrompt(t *testing.T) {
	instruction := "вставь последовательное описание всех систем, включенных в коммерческое " +
		"предложение, приведи их состав, описание материалов, входящих в состав решения и " +
		"преимущества применения данных материалов в составе стандартизированного технического " +
		"решения, описание сопровождай изображениями составов и материалов, материалы не имеющие " +
		"описания укажи только в перечне слоев, структурные разделы помечай как заголовки " +
		"второго уровня в разметке ворда"
	tokens := scanLayoutTokens("{{"+instruction+"}}", map[string]string{"object_name": "Дом"})
	if len(tokens) != 1 {
		t.Fatalf("ожидалась одна метка, получено %d: %+v", len(tokens), tokens)
	}
	if tokens[0].Kind != layoutTokenPrompt {
		t.Fatalf("длинная инструкция в двойных скобках не промт: %+v", tokens[0])
	}
	if tokens[0].Inner != instruction {
		t.Fatalf("текст промта искажён:\n got %q\nwant %q", tokens[0].Inner, instruction)
	}
	if tokens[0].End != len("{{"+instruction+"}}") {
		t.Fatalf("метка съела не весь текст: end=%d", tokens[0].End)
	}
}

// Три скобки — тот же промт без лишних скобок в тексте.
func TestScanLayoutTokensTripleBracePrompt(t *testing.T) {
	tokens := scanLayoutTokens("{{{Опиши систему кратко}}}", map[string]string{})
	if len(tokens) != 1 || tokens[0].Kind != layoutTokenPrompt {
		t.Fatalf("тройная скобка не промт: %+v", tokens)
	}
	if tokens[0].Inner != "Опиши систему кратко" {
		t.Fatalf("в промт попали скобки метки: %q", tokens[0].Inner)
	}
}

// Вложенные фигурные скобки внутри промта не рвут метку.
func TestScanLayoutTokensNestedBracesInPrompt(t *testing.T) {
	tokens := scanLayoutTokens("{{Опиши модуль {параметр}}}", map[string]string{})
	if len(tokens) != 1 || tokens[0].Kind != layoutTokenPrompt {
		t.Fatalf("вложенные скобки сломали метку: %+v", tokens)
	}
	if tokens[0].Inner != "Опиши модуль {параметр}" {
		t.Fatalf("текст промта искажён: %q", tokens[0].Inner)
	}
}

// Предел длины считается в рунах: русский текст вдвое короче по символам,
// чем по байтам, и старый предел в 400 «символов» резал 364-символьный промт.
func TestScanLayoutTokensPromptLengthInRunes(t *testing.T) {
	instruction := strings.Repeat("я", 800)
	tokens := scanLayoutTokens("{{"+instruction+"}}", nil)
	if len(tokens) != 1 || len([]rune(tokens[0].Inner)) != 800 {
		t.Fatalf("промт из 800 рун не разобран: %+v", tokens)
	}
}

// Markdown-ссылка в макете не считается плейсхолдером: внутри адреса метки нет.
func TestScanLayoutTokensIgnoresMarkdownLink(t *testing.T) {
	tokens := scanLayoutTokens("См. [сайт](https://example.ru)", map[string]string{})
	// Квадратная скобка здесь — метка, но подпись ссылки не должна тянуть
	// за собой адрес: метка кончается на скобке.
	if len(tokens) != 1 {
		t.Fatalf("ожидалась одна метка, получено %d: %+v", len(tokens), tokens)
	}
	if strings.Contains(tokens[0].Inner, "https") {
		t.Fatalf("в метку попал адрес ссылки: %+v", tokens[0])
	}
}

// Перевод строки закрывает метку: плейсхолдер — всегда одна строка.
func TestScanLayoutTokensStopsAtNewline(t *testing.T) {
	tokens := scanLayoutTokens("[начало\nсередина]", map[string]string{})
	if len(tokens) != 0 {
		t.Fatalf("метка через перевод строки разобрана: %+v", tokens)
	}
}

// Жёсткая подстановка понимает и русское имя, и идентификатор поля.
func TestLayoutFieldValueByHumanName(t *testing.T) {
	values := map[string]string{
		"object_name":  "Дом по ул. Примерная",
		"offer_number": "003/2026",
	}
	if got := layoutFieldValue("Наименование объекта", values); got != "Дом по ул. Примерная" {
		t.Fatalf("русское имя поля не найдено: %q", got)
	}
	if got := layoutFieldValue("object_name", values); got != "Дом по ул. Примерная" {
		t.Fatalf("идентификатор поля не найден: %q", got)
	}
	if got := layoutFieldValue("Номер", values); got != "003/2026" {
		t.Fatalf("«Номер» не сопоставлен с номером предложения: %q", got)
	}
}

// Неизвестное имя переходит в пустую строку: служебных скобок в документе нет.
func TestLayoutFieldValueUnknownIsEmpty(t *testing.T) {
	if got := layoutFieldValue("Нет такого поля", map[string]string{}); got != "" {
		t.Fatalf("неизвестное поле не пустое: %q", got)
	}
}
