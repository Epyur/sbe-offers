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
//
// Kind — СТАРОЕ поле «страница или документ» (2026-11-01). Оно остаётся ради
// совместимости со старым кешем карточек: решать судьбу ссылки по нему больше
// нельзя — теперь ссылку описывают группе печати и владельцу (см. ниже).
const (
	// RefKindPage — страница материала или системы на корпоративном сайте.
	RefKindPage = "page"
	// RefKindDoc — технический документ: техлист, узлы, инструкция, СТО.
	RefKindDoc = "doc"
)

// Группы ссылок — порядок печати в документе (2026-11-10, требование
// пользователя). Числовой ранг задаёт, какая группа идёт раньше; порядок
// констант здесь и в cardRefGroupRank — одна правда.
const (
	// RefGroupPage — страница продукта на корпоративном сайте (nav.tn.ru).
	RefGroupPage = "page"
	// RefGroupSTO — стандарт организации (СТО).
	RefGroupSTO = "sto"
	// RefGroupTechList — технический лист (техлист).
	RefGroupTechList = "techlist"
	// RefGroupUzly — альбом узлов.
	RefGroupUzly = "uzly"
	// RefGroupCert — сертификаты, декларации и всё остальное, что не попало в
	// группы выше. Сюда же идут инструкции и руководства: отдельной группы для
	// них пользователь не завёл, а «остальное» — это и есть их место.
	RefGroupCert = "cert"
)

// Владелец ссылки: чья карточка её дала. От него зависит порядок ВНУТРИ
// группы — «сначала системы, потом материалы» (у сертификатов наоборот).
const (
	// RefOwnerSystem — ссылка из карточки системы.
	RefOwnerSystem = "system"
	// RefOwnerMaterial — ссылка из карточки материала (в том числе из ссылок
	// материала, на который ведёт состав).
	RefOwnerMaterial = "material"
)

// CardRef — ссылка из раздела «Ссылки» карточки.
//
// Label — то, как её назвал человек; пустая подпись заменяется адресом, иначе
// в документе получилась бы строка « — https://…».
//
// Group — группа печати (страница, СТО, техлист, узлы, сертификаты), Owner —
// чья карточка дала ссылку (система или материал). Вместе они задают порядок
// печати в документе. Пустая группа у ссылки, прочитанной старым кешем, — не
// ошибка: её выведет cardRefGroupOf по Kind и адресу.
type CardRef struct {
	Label string `json:"label"`
	URL   string `json:"url"`
	Kind  string `json:"kind,omitempty"`
	Group string `json:"group,omitempty"`
	Owner string `json:"owner,omitempty"`
}

// CardRefGroup — группа ссылок, готовая к печати: заголовок и её ссылки.
//
// Печать идёт группой, а не сплошным списком (2026-11-10): «Страницы
// продуктов», «Стандарты организации (СТО)», «Технические листы», «Узлы»,
// «Сертификаты и декларации». Нумерация в каждой группе начинается с единицы.
type CardRefGroup struct {
	Group string
	Title string
	Refs  []CardRef
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
		return newCardRef(strings.TrimSpace(m[1]), m[2]), true
	}
	url := reCardRefURL.FindString(line)
	if url == "" {
		return CardRef{}, false
	}
	return newCardRef(cardRefLabelFromText(line, url), url), true
}

// newCardRef собирает ссылку: пустая подпись заменяется адресом, группа и вид
// выводятся из подписи и адреса.
//
// Owner здесь НЕ выставляется: он зависит от того, чья карточка дала ссылку,
// а это знание есть только у сборщика ссылок предложения, а не у разбора
// одной строки (см. cardRefsOfCard).
func newCardRef(label, url string) CardRef {
	if strings.TrimSpace(label) == "" {
		label = url
	}
	return CardRef{
		Label: label,
		URL:   url,
		Kind:  cardRefKind(label, url),
		Group: cardRefGroupOf(label, url),
	}
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

// reCardRefSTO — стандарт организации в подписи: «СТО 72746455-4.8.1-2023 …».
// Граница слева явная, а не `\b`: в Go `\b` считается по ASCII и на кириллице
// не находит ничего (грабля поймана трижды — см. skills/systemcard.go).
var reCardRefSTO = regexp.MustCompile(`(?:^|[^А-ЯЁа-яёA-Za-z])сто\s`)

// cardRefGroupOf — группа печати ссылки по её подписи и адресу.
//
// Порядок проверок — от точного признака к общему (2026-11-10). Адрес решает
// первым: страница навигатора `/systems/` и `/catalog/` — всегда страница
// продукта, что бы ни было написано в подписи. Затем подпись: «СТО», «узлы»,
// «техлист»; всё остальное — сертификаты и декларации (сюда же инструкции и
// руководства, отдельной группы для них пользователь не завёл).
//
// Живые данные (замер по 375 системам и 1166 материалам): страницы систем —
// `/systems/…`, страницы материалов — `/catalog/…`, техлисты и узлы и СТО лежат
// под `/pim/…pdf`. По подписи без адреса решается то, что стоит на `/pim/`:
// там и СТО, и техлисты, и узлы, и инструкции.
func cardRefGroupOf(label, url string) string {
	lowerURL := strings.ToLower(url)
	switch {
	case strings.Contains(lowerURL, "/systems/"), strings.Contains(lowerURL, "/catalog/"),
		strings.Contains(lowerURL, "/catalogue/"):
		return RefGroupPage
	}
	lower := strings.ToLower(strings.TrimSpace(label))
	if reCardRefSTO.MatchString(lower) {
		return RefGroupSTO
	}
	// «Узлы», «Узлов», «узел» — корень «узл», а не «узел»: в живых подписях
	// пишут «Узлы PDF …», а в слове «узлы» буквы «е» нет, и поиск по «узел»
	// оставлял альбом узлов в группе «остальное» (замер 2026-11-10).
	if strings.Contains(lower, "узл") {
		return RefGroupUzly
	}
	if strings.Contains(lower, "техлист") || strings.Contains(lower, "тех. лист") ||
		strings.Contains(lower, "тех.лист") || strings.Contains(lower, "технический лист") ||
		strings.HasPrefix(lower, "тл ") {
		return RefGroupTechList
	}
	// Не страница и не документ известного вида — «остальное»: сертификаты,
	// декларации, инструкции, руководства. Пользователь просил заложить эту
	// группу заранее, даже пока сертификатов в карточках нет.
	return RefGroupCert
}

// CardRefGroupTitle — заголовок группы для документа.
func CardRefGroupTitle(group string) string {
	switch group {
	case RefGroupPage:
		return "Страницы продуктов"
	case RefGroupSTO:
		return "Стандарты организации (СТО)"
	case RefGroupTechList:
		return "Технические листы"
	case RefGroupUzly:
		return "Узлы"
	default:
		return "Сертификаты и декларации"
	}
}

// cardRefGroupRank — место группы в порядке печати: меньше — раньше.
func cardRefGroupRank(group string) int {
	switch group {
	case RefGroupPage:
		return 0
	case RefGroupSTO:
		return 1
	case RefGroupTechList:
		return 2
	case RefGroupUzly:
		return 3
	default:
		return 4
	}
}

// effectiveGroup — группа ссылки; у старого кеша поле пустое — выводим по
// подписи и адресу. Одна точка вывода: второй копии быть не должно.
func (r CardRef) effectiveGroup() string {
	if r.Group != "" {
		return r.Group
	}
	return cardRefGroupOf(r.Label, r.URL)
}

// effectiveOwner — владелец ссылки. Пустой у старого кеша: такой кеш приходит
// только из карточек систем (материальные ссылки связываются заново), поэтому
// система и есть честное умолчание.
func (r CardRef) effectiveOwner() string {
	if r.Owner != "" {
		return r.Owner
	}
	return RefOwnerSystem
}

// TagCardRefsOwner ставит владельца ссылкам карточки (2026-11-10).
//
// Разбор одной строки не знает, чья это карточка — системы или материала, —
// а порядок внутри группы от этого зависит. Владелец выставляется там, где он
// известен: при чтении карточки системы (`RefOwnerSystem`) и при чтении ссылок
// материала, на который ведёт состав (`RefOwnerMaterial`).
//
// Исходный срез не меняется, иначе кеш карточки получил бы владельца одного
// вида на все свои ссылки после второго вызова с другим владельцем.
func TagCardRefsOwner(refs []CardRef, owner string) []CardRef {
	out := make([]CardRef, len(refs))
	for i, r := range refs {
		r.Owner = owner
		if r.Group == "" {
			r.Group = cardRefGroupOf(r.Label, r.URL)
		}
		out[i] = r
	}
	return out
}

// SortCardRefsGrouped упорядочивает ссылки по группам для печати (2026-11-10).
//
// Порядок: страницы продуктов → СТО → техлисты → узлы → сертификаты и прочее.
// Внутри группы — «сначала системы, потом материалы» (у сертификатов наоборот:
// сертификаты на материалы приходят первыми), внутри одного владельца — по
// подписи, регистр и «ё» не для читателя не важны.
//
// Исходный срез не меняется: вызывающий держит его как кеш карточки.
func SortCardRefsGrouped(refs []CardRef) []CardRef {
	out := make([]CardRef, len(refs))
	copy(out, refs)
	// Простая вставка: ссылок десятки, сортировка по ключу дешевле любых
	// наворотов, а порядок стабилен при равных ключах.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && cardRefGroupLess(out[j], out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// cardRefGroupLess — правило порядка двух ссылок в разделе.
func cardRefGroupLess(a, b CardRef) bool {
	ga, gb := a.effectiveGroup(), b.effectiveGroup()
	if ga != gb {
		return cardRefGroupRank(ga) < cardRefGroupRank(gb)
	}
	// Внутри группы — владелец: у основной массы групп система раньше
	// материала, у сертификатов наоборот.
	if ao, bo := ownerRankInGroup(ga, a.effectiveOwner()), ownerRankInGroup(gb, b.effectiveOwner()); ao != bo {
		return ao < bo
	}
	return normalizeCardRefLabel(a.Label) < normalizeCardRefLabel(b.Label)
}

// ownerRankInGroup — место владельца внутри группы: меньше — раньше.
//
// В общем случае системы идут первыми (пользователь: «сначала ссылки на
// страницы систем, потом на страницы продуктов»; та же логика у техлистов).
// Сертификаты — исключение: материала сначала, системы потом.
func ownerRankInGroup(group, owner string) int {
	systemFirst := group != RefGroupCert
	if owner == RefOwnerSystem {
		if systemFirst {
			return 0
		}
		return 1
	}
	if systemFirst {
		return 1
	}
	return 0
}

// normalizeCardRefLabel — ключ сравнения подписей: регистр и «ё» не важны.
func normalizeCardRefLabel(label string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(label), "ё", "е"))
}

// GroupCardRefsByGroup раскладывает ссылки на группы в порядке печати.
//
// Вход берётся уже отсортированным (SortCardRefsGrouped): здесь только
// разрезание на группы для печати заголовков. Пустые группы не попадают:
// заголовок без ссылок в документе читается как пропуск.
func GroupCardRefsByGroup(refs []CardRef) []CardRefGroup {
	if len(refs) == 0 {
		return nil
	}
	var out []CardRefGroup
	index := make(map[string]int, 5)
	for _, ref := range refs {
		group := ref.effectiveGroup()
		i, ok := index[group]
		if !ok {
			index[group] = len(out)
			out = append(out, CardRefGroup{Group: group, Title: CardRefGroupTitle(group)})
			i = len(out) - 1
		}
		out[i].Refs = append(out[i].Refs, ref)
	}
	return out
}

// SortCardRefs ставит страницы перед документами, внутри вида — по подписи.
//
// Оставлена ради старых проверок и мест, где групп ещё нет; печать документа
// пользуется SortCardRefsGrouped (2026-11-10). Одно правило порядка лучше
// делить на два: «страницы раньше документов» — частный случай групп.
func SortCardRefs(refs []CardRef) []CardRef {
	return SortCardRefsGrouped(refs)
}
