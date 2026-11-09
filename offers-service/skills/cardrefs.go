package skills

import (
	"regexp"
	"strings"
)

// Ссылки карточки Базы знаний (2026-11-01).
//
// Карточки систем и материалов несут раздел «## Ссылки», и в нём — ровно то,
// что нужно коммерческому предложению:
//
//	- Страница системы: https://nav.tn.ru/systems/ploskaya-krysha/tn-krovlya-avto/
//	- [Техлист ПК_10000016](https://nav.tn.ru/pim/…pdf?response-content-disposition=…)
//
// Первое — корпоративный сайт, второе — техдокументация на навигаторе. Замер по
// живым данным: раздел есть у 374 из 375 системных карточек и у 1165 из 1166
// материалов.
//
// Правило «что считается ссылкой» одно: разбор нужен и для печати в конце КП, и
// для промпта модели, и второй копии быть не должно.

// Виды ссылок карточки.
const (
	// RefKindPage — страница материала или системы на корпоративном сайте.
	RefKindPage = "page"
	// RefKindDoc — технический документ: техлист, узлы, инструкция, СТО.
	RefKindDoc = "doc"
)

// CardRef — ссылка из раздела «Ссылки» карточки.
//
// Label — то, как её назвал человек; пустая подпись заменяется адресом, иначе
// в документе получилась бы строка « — https://…». Kind нужен, чтобы модель
// знала, о чём речь (страница или документ), и чтобы в документе ссылки
// группировались.
type CardRef struct {
	Label string `json:"label"`
	URL   string `json:"url"`
	Kind  string `json:"kind"`
}

var (
	// reCardRefURL — адрес в строке. Хвостовые знаки пунктуации отбрасываются:
	// в тексте люди пишут «см. https://…» и «(https://…)».
	reCardRefURL = regexp.MustCompile(`https?://[^\s<>()\[\]"']+[^\s<>()\[\]"'.,;:!?]`)
	// reCardRefMarkdown — ссылка markdown: [подпись](адрес). Адрес берём из
	// скобок целиком, а не регэкспом выше: в нём бывают query-параметры с
	// кавычками и пробелами, и обрезка по кавычке съела бы половину.
	//
	// Пробел в адресе — не редкий случай, а обычный (живой дефект 2026-11-09):
	// техдокументация навигатора приходит с `?response-content-disposition=
	// filename="Техлист ДНС от ФУНДАМЕНТ.pdf"`, и пробелы стоят внутри имени
	// файла. Прежнее `[^)\s]+` на таком адресе не совпадало вовсе, разбор
	// уходил в «голый адрес», где обрезка по кавычке оставляла от адреса
	// `…?response-content-disposition=filename=`, а подпись терялась. Отсюда
	// в документе появлялась гиперссылка с сырым обрезанным адресом вместо
	// названия документа.
	//
	// Захват — до первой `)` после адреса: скобка в адресе markdown-ссылки
	// обязана быть экранирована, а в живых ссылках её нет. Пробелы и кавычки
	// (`filename="…"`) при этом спокойно входят в адрес. Строка должна
	// кончаться скобкой — иначе это не markdown-ссылка, и решает запасной
	// разбор.
	reCardRefMarkdown = regexp.MustCompile(`\[([^\]]*)\]\((https?://[^)]+)\)\s*$`)
)

// FindURLs — адреса в произвольном тексте: индексы начал и концов.
//
// Правило «что считается адресом» одно на весь сервис (2026-11-06): по этому
// же выражению разбираются ссылки карточки и делаются кликабельными адреса в
// ответе модели. Вторая копия разошлась бы — одна признала бы адресом
// `https://x/a.`, другая нет, и один документ печатал бы ссылку, а другой нет.
func FindURLs(text string) [][]int {
	return reCardRefURL.FindAllStringIndex(text, -1)
}

// ParseCardRefs вытаскивает ссылки из раздела «Ссылки» карточки.
//
// Берётся ТОЛЬКО этот раздел: в описании и в составе бывают ссылки на
// материалы и стандарты, и печатать их в конце КП — не то, что просили.
// Повторы по адресу схлопываются: живые карточки содержат один и тот же
// адрес и подписанным, и голым.
func ParseCardRefs(markdown string) []CardRef {
	lines := cardRefSectionLines(markdown)
	if len(lines) == 0 {
		return []CardRef{}
	}
	out := make([]CardRef, 0, len(lines))
	seen := make(map[string]bool, len(lines))
	for _, line := range lines {
		ref, ok := cardRefFromLine(line)
		if !ok || seen[ref.URL] {
			continue
		}
		seen[ref.URL] = true
		out = append(out, ref)
	}
	return out
}

// cardRefSectionLines собирает строки раздела «Ссылки» (любого уровня,
// регистр не важен). Раздел кончается на следующем заголовке.
func cardRefSectionLines(markdown string) []string {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	var out []string
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if level, title, ok := parseCardHeading(trimmed); ok {
			_ = level
			inSection = strings.Contains(strings.ToLower(title), "ссылк")
			continue
		}
		if inSection && trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// cardRefFromLine — одна ссылка из строки списка.
//
// Приоритет у markdown-вида: там подпись стоит рядом с адресом. Если строка
// подписана «Страница системы: …» — берём подпись из неё. Голый адрес
// подписывается сам собой.
func cardRefFromLine(line string) (CardRef, bool) {
	if m := reCardRefMarkdown.FindStringSubmatch(line); m != nil {
		label := strings.TrimSpace(m[1])
		if label == "" {
			label = m[2]
		}
		return CardRef{Label: label, URL: m[2], Kind: cardRefKind(label, m[2])}, true
	}
	url := reCardRefURL.FindString(line)
	if url == "" {
		return CardRef{}, false
	}
	label := cardRefLabelFromText(line, url)
	if label == "" {
		label = url
	}
	return CardRef{Label: label, URL: url, Kind: cardRefKind(label, url)}, true
}

// cardRefLabelFromText — подпись из строки вида «Страница системы: https://…».
// Маркер списка и двоеточие снимаются; если перед адресом только пустота —
// подписи нет.
func cardRefLabelFromText(line, url string) string {
	prefix := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), url))
	prefix = strings.TrimSpace(strings.TrimLeft(prefix, "-*+ "))
	prefix = strings.TrimSuffix(prefix, ":")
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || strings.HasPrefix(prefix, "http") {
		return ""
	}
	return prefix
}

// cardRefKind — страница корпоративного сайта или технический документ.
//
// Признак документа — расширение .pdf или слово из подписи. По URL, а не по
// подписи в первую очередь: подпись технолог правит, а адрес ведёт на файл.
func cardRefKind(label, url string) string {
	lowerURL := strings.ToLower(url)
	if strings.Contains(lowerURL, ".pdf") || strings.Contains(lowerURL, "/pim/") {
		return RefKindDoc
	}
	lower := strings.ToLower(label)
	for _, word := range []string{"техлист", "узлы", "инструкц", "руководств", "сто ", "технологическ", "альбом", "каталог узлов"} {
		if strings.Contains(lower, word) {
			return RefKindDoc
		}
	}
	return RefKindPage
}

// SortCardRefs ставит страницы перед документами, внутри вида — по подписи:
// человеку читать сверху вниз, и в КП важно сначала «где посмотреть систему».
func SortCardRefs(refs []CardRef) []CardRef {
	out := make([]CardRef, len(refs))
	copy(out, refs)
	// Простая вставка: ссылок десятки, сортировка по подписи дешевле любых
	// наворотов, а порядок стабилен при равных подписях.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && cardRefLess(out[j], out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func cardRefLess(a, b CardRef) bool {
	if a.Kind != b.Kind {
		return a.Kind == RefKindPage
	}
	return strings.ToLower(a.Label) < strings.ToLower(b.Label)
}
