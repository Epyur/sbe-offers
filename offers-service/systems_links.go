package main

import (
	"regexp"
	"strings"
)

// Связь состава системы с материалами по ВИКИ-ССЫЛКЕ (2026-10-31).
//
// Решение пользователя: SKU в карточке материала вводится как признак
// материала, но состав системы ссылается на материал ССЫЛКОЙ на заметку,
// а не кодом. Замер на живых данных: кодов в составе нет ни одного (0 из
// 3387 строк), а вики-ссылок — 2103, и 2103 из них ведут на заметки с
// тегом `material`.
//
// Правило: материал — это заметка с тегом `material`, на которую указывает
// ссылка. Код (`MAT-…`) остаётся запасным способом: если человек вписал код,
// работаем по коду.
//
// Ссылка на заметку без тега (`[[ГОСТ 30244]]`) материалом НЕ считается:
// в составе систем ссылаются и на нормативы, и на протоколы. Угадывать тут
// значит подставлять в КП чужую строку.

// reCompositionWiki — `[[Название]]`, `[[Название|подпись]]`,
// `[[Название#якорь]]`. Не жадный: захватывает до первой `]`.
var reCompositionWiki = regexp.MustCompile(`\[\[([^\[\]]+?)\]\]`)

// wikiTarget — цель ссылки из строки состава; пусто, если ссылки нет.
//
// Подпись (`|`) и якорь (`#`) отбрасываются: они не часть имени заметки.
func wikiTarget(line string) string {
	m := reCompositionWiki.FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	name := m[1]
	if cut, _, ok := strings.Cut(name, "|"); ok {
		name = cut
	}
	if cut, _, ok := strings.Cut(name, "#"); ok {
		name = cut
	}
	return normalizeWikiTarget(name)
}

// normalizeWikiTarget — ключ сравнения имён заметок: без учёта регистра и
// без краевых пробелов. В Obsidian имя заметки регистронезависимо.
func normalizeWikiTarget(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// resolveWikiLink — цель ссылки и признак «ссылка была». Обёртка нужна
// тестам: сама wikiTarget возвращает пустую строку и в случае, когда ссылки
// нет, и в случае пустой ссылки `[[ ]]`.
func resolveWikiLink(line string) (string, bool) {
	target := wikiTarget(line)
	return target, target != ""
}
