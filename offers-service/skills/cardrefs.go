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
// Kind — информационное поле «страница или документ». Порядок печати по нему НЕ
// решается: с 2026-11-17 ссылки печатаются одним списком в порядке упоминания в
// заметке (требование пользователя), а деление на типы убрано.
const (
	// RefKindPage — страница материала или системы на корпоративном сайте.
	RefKindPage = "page"
	// RefKindDoc — технический документ: техлист, узлы, инструкция, СТО.
	RefKindDoc = "doc"
)

// CardRef — ссылка из раздела «Ссылки» карточки.
//
// Label — то, как её назвал человек; пустая подпись заменяется адресом, иначе
// в документе получилась бы строка « — https://…».
//
// Порядок печати задаёт ПОРЯДОК РАЗБОРА (2026-11-17): как ссылки записаны в
// заметке, так они и идут в документе. Поля группы и владельца, которые раньше
// это решали, удалены — старый кеш с `group`/`owner` читается нормально, лишние
// ключи JSON просто игнорируются.
type CardRef struct {
	Label string `json:"label"`
	URL   string `json:"url"`
	Kind  string `json:"kind,omitempty"`
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
	// (`filename="…"`) при этом спокойно входят в адрес.
	//
	// Конца строки в шаблоне НЕТ (2026-11-17): в разделе «Иные документы»
	// приложения «Документы» после ссылки стоит пояснение и служебный
	// комментарий (`… — ДС ФЗ 123 <!-- kb-doc:… -->`), и прежний якорь `$`
	// заставлял строку проваливаться в «голый адрес» — подпись терялась.
	reCardRefMarkdown = regexp.MustCompile(`\[([^\]]*)\]\((https?://[^)]+)\)`)
	// reCardRefHTMLComment — служебный комментарий карточки. В документ он
	// попасть не должен ни подписью, ни адресом.
	reCardRefHTMLComment = regexp.MustCompile(`<!--[\s\S]*?-->`)
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

// ParseCardRefs вытаскивает ссылки из разделов «Ссылки» и «Иные документы»
// карточки (2026-11-01, «Иные документы» — 2026-11-17).
//
// «Иные документы» — блок приложения «Документы», который индексатор дописывает
// в заметку: там ссылки на декларации, сертификаты и подписанные письма, чьи
// файлы лежат в S3. Пока раздел не читался, эти документы не доходили ни до КП,
// ни до боевого листка, хотя человек видел их в самой заметке.
//
// Остальные разделы (описание, состав) не берутся: там ссылки на материалы и
// стандарты, и печатать их в конце документа — не то, что просили. Повторы по
// адресу схлопываются: живые карточки содержат один и тот же адрес и
// подписанным, и голым.
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

// cardRefSectionLines собирает строки разделов ссылок (любого уровня, регистр
// не важен): «Ссылки» и «Иные документы». Раздел кончается на следующем
// заголовке; порядок строк — порядок заметки, поэтому документы идут там, где
// их видит человек.
func cardRefSectionLines(markdown string) []string {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	var out []string
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if level, title, ok := parseCardHeading(trimmed); ok {
			_ = level
			inSection = cardRefSectionTitle(title)
			continue
		}
		if inSection && trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// cardRefSectionTitle — заголовок раздела, который считается ссылками карточки.
func cardRefSectionTitle(title string) bool {
	lower := strings.ToLower(title)
	return strings.Contains(lower, "ссылк") ||
		strings.Contains(lower, "иные документы") ||
		strings.Contains(lower, "иных документов")
}

// cardRefFromLine — одна ссылка из строки списка.
//
// Приоритет у markdown-вида: там подпись стоит рядом с адресом. Если строка
// подписана «Страница системы: …» — берём подпись из неё. Голый адрес
// подписывается сам собой.
func cardRefFromLine(line string) (CardRef, bool) {
	line = strings.TrimSpace(reCardRefHTMLComment.ReplaceAllString(line, ""))
	if m := reCardRefMarkdown.FindStringSubmatch(line); m != nil {
		return newCardRef(strings.TrimSpace(m[1]), m[2]), true
	}
	url := reCardRefURL.FindString(line)
	if url == "" {
		return CardRef{}, false
	}
	return newCardRef(cardRefLabelFromText(line, url), url), true
}

// newCardRef собирает ссылку: пустая подпись заменяется адресом, вид выводится
// из подписи и адреса.
func newCardRef(label, url string) CardRef {
	if strings.TrimSpace(label) == "" {
		label = url
	}
	return CardRef{
		Label: label,
		URL:   url,
		Kind:  cardRefKind(label, url),
	}
}

// cardRefLabelFromText — подпись из строки вида «Страница системы: https://…».
// Маркер списка и двоеточие снимаются; если перед адресом только пустота —
// подписи нет.
//
// Пояснение после тире («— ДС ФЗ 123») подписью не считается (2026-11-17):
// так выглядят строки раздела «Иные документы». В документе пояснение читалось
// бы частью названия файла.
func cardRefLabelFromText(line, url string) string {
	prefix := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), url))
	prefix = strings.TrimSpace(strings.TrimLeft(prefix, "-*+ "))
	prefix = trimCardRefNote(prefix)
	prefix = strings.TrimSuffix(prefix, ":")
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || strings.HasPrefix(prefix, "http") {
		return ""
	}
	return prefix
}

// trimCardRefNote снимает пояснение, которое человек дописал после ссылки.
// Режем только по тире с пробелами — дефис внутри названия («V-SL 2 мм»)
// остаётся частью подписи.
func trimCardRefNote(text string) string {
	for _, sep := range []string{" — ", " – "} {
		if idx := strings.Index(text, sep); idx >= 0 {
			return strings.TrimSpace(text[:idx])
		}
	}
	return strings.TrimSpace(text)
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
	// Документы приложения «Документы» (файлы в S3): адрес ведёт на файл, а не
	// на страницу продукта (2026-11-17).
	if strings.Contains(lowerURL, "/api/documents/") {
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
