package skills

import (
	"regexp"
	"strings"
)

// Разбор карточки системы из Базы знаний (шаг 2 пересмотренного плана).
//
// Карточка — обычная заметка Obsidian с тегом commercial_propose. Структура
// снята с живого образца (testdata/system-card.md):
//
//	---
//	sbe_kb_id: …
//	tags: [commercial_propose]
//	---
//	# ТН-Стилобат Эксперт тротуар      ← H1: НАЗВАНИЕ СИСТЕМЫ
//	ТН-Стилобат Эксперт тротуар        ← строкой, без разметки
//	## Состав системы                  ← состав
//	- **1** Тротуарная плитка
//	- **2** Песок
//	  (альтернатива: …)                ← вариант того же пункта, не отдельная позиция
//	## Описание системы
//	текст описания
//	![[картинка.png|335]]              ← картинка продукта
//	## Свойства и преимущества         ← таблица характеристик
//	| Показатель | Ед. | Значение |
//
// Разбор терпимый: заголовки могут называться «Характеристики» вместо
// «Свойства и преимущества», карточка может не иметь картинки или состава.
// Пустое поле — не ошибка.
//
// ВАЖНО: здесь только ЧТЕНИЕ текста. Цены, артикулы и количества в карточке
// не участвуют — они живут в каталоге (решение пользователя 2026-10-29).

// SystemCard — разобранная карточка системы.
type SystemCard struct {
	// Title — название системы. По стандарту это строка под H1 (H1 занят
	// служебным «Карточка системы»), поэтому берётся первая содержательная
	// строка после заголовка первого уровня.
	Title string
	// SKU — код системы из frontmatter. По нему карточка связывается с
	// системой каталога: цены и артикулы живут в каталоге, текст — здесь.
	SKU string
	// Description — текст блока «Описание системы», без разметки. Идёт в
	// описание системы в КП.
	Description string
	// Composition — состав: позиции списка из блока «Состав системы».
	Composition []string
	// ImageName — имя файла картинки из `![[…]]` (без размера).
	ImageName string
	// Specifications — таблица характеристик: строки ячеек.
	Specifications [][]string
}

// Названия блоков карточки. Сверяются по вхождению без учёта регистра:
// «Описание системы» и «## Описание» — одно и то же.
const (
	cardHeadingDescription = "описан"
	cardHeadingComposition = "состав"
	// Таблица характеристик стоит под «Свойства и преимущества» в образце и
	// под «Характеристики» в других карточках — понимаем оба названия.
	cardHeadingSpecs      = "характеристик"
	cardHeadingProperties = "свойства"
)

var (
	// ![[файл.png|335]] — картинка с необязательным размером.
	reCardImage = regexp.MustCompile(`!\[\[([^\]|]+)(?:\|[^\]]*)?\]\]`)
	// [текст](ссылка) — markdown-ссылка.
	reCardLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	// **жирный** и *курсив*.
	reCardBold = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	// **N** в начале позиции состава (после снятия маркера списка).
	reCardNumber = regexp.MustCompile(`^\**\d+\**\s*[.)]?\s*`)
)

// ParseSystemCard разбирает заметку в карточку системы.
func ParseSystemCard(markdown string) SystemCard {
	meta, body := splitCardFrontmatter(markdown)
	blocks := splitCardBlocks(splitCardLines(body))

	card := SystemCard{SKU: strings.TrimSpace(meta["system_sku"])}
	for _, b := range blocks {
		heading := strings.ToLower(b.heading)
		switch {
		// H1 по стандарту служебный («Карточка системы»): название берётся
		// первой содержательной строкой после него. Если технолог написал в H1
		// само название — используем его, разбор терпимый.
		case b.level == 1 && card.Title == "":
			// Порядок: сначала H1, если он содержательный. Служебный заголовок
			// («Карточка системы») пропускаем и берём строку под ним — так
			// устроен стандарт.
			if !isServiceHeading(b.headingText) {
				card.Title = cleanCardText(b.headingText)
			} else if name := firstCardLine(b.text); name != "" {
				card.Title = name
			}
		case strings.Contains(heading, cardHeadingComposition):
			card.Composition = append(card.Composition, b.composition...)
		case strings.Contains(heading, cardHeadingSpecs) || strings.Contains(heading, cardHeadingProperties):
			card.Specifications = b.tableRows
		case strings.Contains(heading, cardHeadingDescription):
			card.Description = b.text
			if card.ImageName == "" {
				card.ImageName = b.image
			}
		default:
			// Картинку берём откуда угодно: в образце она в описании, но может
			// оказаться и рядом с названием.
			if card.ImageName == "" {
				card.ImageName = b.image
			}
			// Название текстом, если H1 нет.
			if card.Title == "" && b.level == 0 && b.text != "" {
				card.Title = firstCardLine(b.text)
			}
		}
	}
	if card.Title == "" {
		card.Title = firstNonEmptyHeading(blocks)
	}
	return card
}

// PromptText — текст карточки для промпта модели: описание и состав.
//
// Служебные данные (путь, uuid, теги, таблица характеристик) сюда не идут:
// модели нужно то, о чём писать, а не метаданные заметки.
func (c SystemCard) PromptText() string {
	var b strings.Builder
	if c.Title != "" {
		b.WriteString("Система: " + c.Title + "\n")
	}
	if c.Description != "" {
		b.WriteString("\nОписание:\n" + c.Description + "\n")
	}
	if len(c.Composition) > 0 {
		b.WriteString("\nСостав:\n")
		for _, item := range c.Composition {
			b.WriteString("- " + item + "\n")
		}
	}
	return strings.TrimSpace(b.String())
}

// Empty — карточка без содержания: связывать её с системой бессмысленно.
func (c SystemCard) Empty() bool {
	return c.Title == "" && c.Description == "" && len(c.Composition) == 0
}

// cardBlock — блок заметки: заголовок и его содержимое.
type cardBlock struct {
	level       int
	heading     string
	headingText string
	// composition — позиции состава. Варианты («(альтернатива: …)», «- ссылка»
	// без номера) приклеиваются к предыдущей позиции, а не становятся новыми:
	// это тот же материал, а не отдельная строка КП.
	composition []string
	tableRows   [][]string
	text        string
	image       string
}

// splitCardFrontmatter отделяет шапку заметки от текста.
//
// Из шапки читается только system_sku — служебный код системы. Остальное
// (sbe_kb_id, tags) принадлежит Obsidian и базе знаний.
func splitCardFrontmatter(markdown string) (map[string]string, string) {
	meta := map[string]string{}
	raw := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	if len(raw) == 0 || strings.TrimSpace(raw[0]) != "---" {
		return meta, markdown
	}
	for i := 1; i < len(raw); i++ {
		line := raw[i]
		if strings.TrimSpace(line) == "---" {
			return meta, strings.Join(raw[i+1:], "\n")
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		meta[strings.ToLower(strings.TrimSpace(key))] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	// Шапка не закрыта — считаем, что её нет: лучше прочитать текст, чем
	// потерять всю карточку.
	return map[string]string{}, markdown
}

// isServiceHeading — служебные заголовки, которые не являются названием
// системы.
func isServiceHeading(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "карточка системы", "название системы", "система":
		return true
	}
	return false
}

// splitCardLines убирает frontmatter и возвращает строки.
func splitCardLines(markdown string) []string {
	_, body := splitCardFrontmatter(markdown)
	raw := strings.Split(body, "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		out = append(out, strings.TrimRight(line, " \t"))
	}
	return out
}

// splitCardBlocks делит заметку на блоки по заголовкам.
func splitCardBlocks(lines []string) []cardBlock {
	var blocks []cardBlock
	current := cardBlock{}
	var paragraph []string
	var table [][]string

	flushParagraph := func() {
		text := strings.TrimSpace(strings.Join(paragraph, "\n"))
		paragraph = nil
		if text == "" {
			return
		}
		if current.text != "" {
			current.text += "\n"
		}
		current.text += text
	}
	flushTable := func() {
		if len(table) > 0 {
			current.tableRows = append(current.tableRows, table...)
			table = nil
		}
	}
	flush := func() {
		flushParagraph()
		flushTable()
		if current.level > 0 || len(current.composition) > 0 || current.text != "" || len(current.tableRows) > 0 || current.image != "" {
			blocks = append(blocks, current)
		}
		current = cardBlock{}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if level, title, ok := parseCardHeading(trimmed); ok {
			flush()
			current = cardBlock{level: level, heading: title, headingText: title}
			continue
		}
		if strings.HasPrefix(trimmed, "|") {
			flushParagraph()
			if row, ok := parseCardTableRow(trimmed); ok {
				table = append(table, row)
			}
			continue
		}
		flushTable()

		switch {
		case trimmed == "":
			flushParagraph()
		case isCardListItem(trimmed):
			flushParagraph()
			item := trimCardListMarker(trimmed)
			numbered := reCardNumber.MatchString(item)
			item = reCardNumber.ReplaceAllString(item, "")
			item = cleanCardText(item)
			if strings.TrimSpace(item) == "" {
				continue
			}
			// Вариант (без номера) дописывается к предыдущей позиции: «(альтернатива:
			// …)» — это тот же материал в другой марке, а не новая строка КП.
			if !numbered && len(current.composition) > 0 {
				last := len(current.composition) - 1
				current.composition[last] += "; " + strings.TrimSpace(item)
				continue
			}
			current.composition = append(current.composition, strings.TrimSpace(item))
		case strings.HasPrefix(trimmed, "("):
			// «(альтернатива: …)» без маркера списка.
			flushParagraph()
			item := cleanCardText(trimmed)
			if len(current.composition) > 0 && strings.TrimSpace(item) != "" {
				last := len(current.composition) - 1
				current.composition[last] += "; " + strings.TrimSpace(item)
			}
		default:
			paragraph = append(paragraph, trimmed)
		}
	}
	flush()

	// Картинку ищем по всему блоку: она может стоять в тексте описания.
	for i := range blocks {
		source := blocks[i].text + "\n" + blocks[i].headingText
		if m := reCardImage.FindStringSubmatch(source); m != nil {
			blocks[i].image = strings.TrimSpace(m[1])
			blocks[i].text = strings.TrimSpace(reCardImage.ReplaceAllString(blocks[i].text, ""))
		}
	}
	return blocks
}

// parseCardHeading — «# Заголовок» / «## Заголовок».
func parseCardHeading(line string) (level int, title string, ok bool) {
	if !strings.HasPrefix(line, "#") {
		return 0, "", false
	}
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n > 6 {
		return 0, "", false
	}
	rest := strings.TrimSpace(line[n:])
	if rest == "" {
		return 0, "", false
	}
	return n, rest, true
}

// isCardListItem — строка списка: «- …», «* …», «+ …», «1. …».
func isCardListItem(line string) bool {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "+ ") {
		return true
	}
	if len(line) > 2 && line[0] >= '0' && line[0] <= '9' {
		for i := 0; i < len(line) && line[i] >= '0' && line[i] <= '9'; i++ {
			if i+1 < len(line) && (line[i+1] == '.' || line[i+1] == ')') {
				return true
			}
		}
	}
	return false
}

// parseCardTableRow — «| a | b |» в ячейки. Строка-разделитель (|---|---|)
// отбрасывается.
func parseCardTableRow(line string) ([]string, bool) {
	body := strings.Trim(line, "|")
	cells := strings.Split(body, "|")
	out := make([]string, 0, len(cells))
	separator := true
	for _, c := range cells {
		cell := strings.TrimSpace(c)
		out = append(out, cleanCardText(cell))
		if cell != "" && strings.Trim(cell, "-: ") != "" {
			separator = false
		}
	}
	if separator {
		return nil, false
	}
	return out, true
}

// trimCardListMarker снимает «- », «* », «+ » с начала строки.
func trimCardListMarker(line string) string {
	line = strings.TrimSpace(line)
	for _, p := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(line, p) {
			return strings.TrimSpace(line[len(p):])
		}
	}
	return line
}

// cleanCardText убирает markdown-разметку: ссылки, жирный, обратные кавычки.
func cleanCardText(s string) string {
	s = reCardImage.ReplaceAllString(s, "")
	s = reCardLink.ReplaceAllString(s, "$1")
	s = reCardBold.ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, "\\", "")
	return strings.TrimSpace(s)
}

// firstCardLine — первая непустая строка блока.
func firstCardLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) != "" {
			return cleanCardText(strings.TrimSpace(line))
		}
	}
	return ""
}

// firstNonEmptyHeading — первый H1 заметки, если название текстом не нашли.
func firstNonEmptyHeading(blocks []cardBlock) string {
	for _, b := range blocks {
		if b.level == 1 && strings.TrimSpace(b.headingText) != "" {
			return cleanCardText(b.headingText)
		}
	}
	return ""
}
