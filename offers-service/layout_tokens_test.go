package main

import (
	"strings"
	"testing"
)

// Плейсхолдеры макета .docx (2026-11-05).
//
// Пользователь загружает свой документ Word и ставит в нём два вида меток:
// `[…]` — жёсткая подстановка значения, `{…}` — промт модели.

// Обе метки находятся в порядке появления и с верным видом.
func TestScanLayoutTokensFindsBothKinds(t *testing.T) {
	text := "Объект: [Наименование объекта]. {Опиши систему кратко}."
	tokens := scanLayoutTokens(text)
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

// `{{object_name}}` — тоже жёсткая подстановка, но не промт.
func TestScanLayoutTokensBraceField(t *testing.T) {
	tokens := scanLayoutTokens("Кому: {{client_name}}")
	if len(tokens) != 1 || tokens[0].Kind != layoutTokenField || tokens[0].Inner != "client_name" {
		t.Fatalf("двойная скобка разобрана неверно: %+v", tokens)
	}
}

// Markdown-ссылка в макете не считается плейсхолдером: внутри адреса метки нет.
func TestScanLayoutTokensIgnoresMarkdownLink(t *testing.T) {
	tokens := scanLayoutTokens("См. [сайт](https://example.ru)")
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
	tokens := scanLayoutTokens("[начало\nсередина]")
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
