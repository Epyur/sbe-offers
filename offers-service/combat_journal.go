package main

import (
	"context"
	"log"
	"regexp"
	"strings"
)

// Журнал конкурентов как источник группировки (2026-11-11, идея пользователя).
//
// «Журнал материалов конкурентов» — служебная заметка, которую ведёт индексатор.
// В ней по каждому конкуренту таблицы по разделам каталога:
//
//	## ПВХ-мембраны — 28 шт.
//	| ✅ | ПВХ мембрана PLASTFOIL CLASSIC | https://…/pvkh-membrana/… | [[ПВХ мембрана PLASTFOIL CLASSIC]] |
//
// Для подбора это ценнее любых слов: раздел каталога и есть класс продукта, а
// последняя колонка — ССЫЛКА НА ЦЕЛЕВУЮ ЗАМЕТКУ. Поэтому модель выбирает не
// строку журнала, а настоящую карточку: заметку, которую потом можно открыть и
// вставить в листок.
//
// Модель получает разделы и строки, а не сырой текст: в журнале 2200 строк, из
// них 1738 — один FACHMANN, и целиком в промпт он не влезет.

// loadCompetitorJournal читает текст журнала конкурентов из Базы знаний.
//
// Журнал — заметка с тегом `journal` (тот же, по которому она исключается из
// списка сторон). Текст нужен целиком: разбор идёт по разделам и строкам
// таблицы. Ошибка — журнал не прочитан; пустая строка — журнала нет.
func (s *Server) loadCompetitorJournal(ctx context.Context) (string, error) {
	if s.kb == nil || !s.kb.configured() {
		return "", nil
	}
	tagID, err := s.kb.TagID(ctx, competitorJournalTagName)
	if err != nil {
		return "", nil
	}
	notes, err := s.kb.Notes(ctx, tagID)
	if err != nil {
		return "", err
	}
	for _, summary := range notes {
		note, err := s.kb.Note(ctx, summary.ID)
		if err != nil {
			continue
		}
		if strings.TrimSpace(note.ContentMD) != "" {
			return note.ContentMD, nil
		}
	}
	return "", nil
}

// journalSection — раздел журнала: заголовок и строки-продукты.
type journalSection struct {
	// Heading — заголовок раздела как он написан («ПВХ-мембраны — 28 шт.»).
	Heading string
	// Owner — конкурент (заголовок первого уровня над разделами), если известен.
	Owner string
	// Items — продукты раздела.
	Items []journalItem
}

// journalItem — строка таблицы журнала.
type journalItem struct {
	Title string
	URL   string
	// Note — имя заметки из ссылки `[[…]]`; пусто — карточки ещё нет.
	Note string
}

// reJournalHeading — заголовок раздела журнала: РОВНО два решётки.
//
// `#{2}` а не `##`: шаблон `^##\s` совпадает и с `### …`, и — что важнее —
// `^#\s` для владельца перехватывался бы общим «заголовком» (2026-11-11,
// разбор ставил владельца в пустоту и терял, кто это).
var reJournalHeading = regexp.MustCompile(`(?m)^ {0,3}##\s+(.+?)\s*$`)

// reJournalOwner — заголовок конкурента (`# …`), но не служебный «Журнал…».
var reJournalOwner = regexp.MustCompile(`(?m)^ {0,3}#\s+(.+?)\s*$`)

// reJournalRow — строка таблицы: | статус | материал | url | [[карточка]] |
var reJournalRow = regexp.MustCompile(`(?m)^\|\s*([^|]*)\|\s*([^|]+?)\s*\|\s*([^|]*?)\s*\|\s*([^|]*?)\s*\|\s*$`)

// reJournalWikilink — имя заметки в последней колонке.
var reJournalWikilink = regexp.MustCompile(`\[\[([^\]|]+)`)

// parseCompetitorJournal разбирает журнал на разделы.
//
// Структура журнала (снято с боевой заметки 2026-11-11):
//
//	# Журнал материалов конкурентов        ← H1, служебный
//	## PLASTFOIL (plastfoil.ru/catalog/)   ← конкурент, БЕЗ числа товаров
//	## ПВХ-мембраны — 28 шт.               ← раздел каталога, С числом товаров
//	| ✅ | ПВХ мембрана … | url | [[заметка]] |
//
// Оба уровня — `##`: индексатор пишет конкурента и раздел одинаково. Различаем
// по заголовку: у раздела есть счётчик («— 28 шт.»), у конкурента — нет.
//
// Всё прочее пропускается: журнал — служебный файл, и его формат мы
// контролируем, а не угадываем.
func parseCompetitorJournal(markdown string) []journalSection {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	sections := make([]journalSection, 0, 32)
	owner := ""
	var current *journalSection
	for _, line := range lines {
		if m := reJournalHeading.FindStringSubmatch(line); m != nil {
			heading := strings.TrimSpace(m[1])
			if heading == "" || currentOwnerIsService(heading) {
				continue
			}
			if isJournalSectionHeading(heading) {
				// Раздел каталога: строки ниже — продукты.
				sections = append(sections, journalSection{Heading: heading, Owner: owner})
				current = &sections[len(sections)-1]
				continue
			}
			// Заголовок без счётчика — конкурент: задаёт владельца для следующих
			// разделов. Своих строк у него нет.
			owner = heading
			current = nil
			continue
		}
		if current == nil {
			continue
		}
		m := reJournalRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		title := strings.TrimSpace(m[2])
		// Шапка таблицы и разделитель — не продукты. Разделитель Markdown
		// (`| --- | --- |`) ловится шаблоном как строка с тремя полями «---».
		if title == "" || title == "Материал" || isJournalSeparator(title) {
			continue
		}
		item := journalItem{Title: title, URL: strings.TrimSpace(m[3])}
		if wl := reJournalWikilink.FindStringSubmatch(m[4]); wl != nil {
			item.Note = strings.TrimSpace(wl[1])
		}
		current.Items = append(current.Items, item)
	}
	// Пустые разделы (заголовок без единой карточки) отбрасываем: модели они
	// ничего не дают.
	out := make([]journalSection, 0, len(sections))
	for _, section := range sections {
		if len(section.Items) > 0 {
			out = append(out, section)
		}
	}
	return out
}

// reJournalCount — счётчик товаров в заголовке раздела («— 28 шт.», «(19 шт.)»).
var reJournalCount = regexp.MustCompile(`\d+\s*(шт|продукт|позиц|item)`)

// isJournalSectionHeading — это раздел каталога, а не конкурент.
//
// Признак — счётчик товаров в заголовке: индексатор пишет «ПВХ-мембраны —
// 28 шт.», «Комплектующие — 9 шт.», а имя конкурента («PLASTFOIL
// (plastfoil.ru/catalog/)») счётчика не имеет.
func isJournalSectionHeading(heading string) bool {
	return reJournalCount.MatchString(strings.ToLower(heading))
}

// isJournalSeparator — строка-разделитель таблицы Markdown.
func isJournalSeparator(title string) bool {
	return strings.Trim(title, "-: ") == ""
}

// currentOwnerIsService — заголовок `# Журнал…` и подобные служебные.
func currentOwnerIsService(heading string) bool {
	lower := strings.ToLower(strings.TrimSpace(heading))
	return strings.HasPrefix(lower, "журнал")
}

// journalSectionCandidates — строки разделов для промпта модели.
//
// Раздел — это единица выбора: модель сначала понимает, ЧТО за товар в разделе,
// и только потом смотрит строки. Так в промпт попадают осмысленные группы, а не
// 2200 строк подряд.
func journalSectionCandidates(sections []journalSection, limit int) ([]journalSection, int) {
	// Разделы мельче трёх строк чаще всего служебные («Комплектующие»), но
	// выбрасывать их нельзя: иногда весь товар конкурента — одна строка.
	kept := make([]journalSection, 0, len(sections))
	total := 0
	for _, section := range sections {
		if total >= limit {
			break
		}
		items := section.Items
		if remain := limit - total; len(items) > remain {
			items = items[:remain]
		}
		kept = append(kept, journalSection{Heading: section.Heading, Owner: section.Owner, Items: items})
		total += len(items)
	}
	return kept, total
}

// buildJournalAnalogPrompt — запрос модели по разделам журнала.
//
// Наш продукт, затем разделы каталога конкурентов с номерами строк. Модель
// отвечает номерами ЛИНЕЙНЫМИ (сквозная нумерация по всем разделам): так ответ
// не зависит от того, сколько разделов и строк ей показали.
func buildJournalAnalogPrompt(source SystemCard, sections []journalSection) string {
	var b strings.Builder
	b.WriteString("НАШ ПРОДУКТ:\n")
	b.WriteString(source.Title)
	b.WriteString("\n")
	if desc := trimForPrompt(source.Description, 600); desc != "" {
		b.WriteString(desc)
		b.WriteString("\n")
	}
	b.WriteString("\nРАЗДЕЛЫ КАТАЛОГА КОНКУРЕНТОВ:\n")
	n := 0
	for _, section := range sections {
		owner := section.Owner
		if owner == "" {
			owner = "конкурент"
		}
		b.WriteString("\n[")
		b.WriteString(section.Heading)
		b.WriteString("] (")
		b.WriteString(owner)
		b.WriteString(")\n")
		for _, item := range section.Items {
			n++
			b.WriteString(strings.TrimSpace(strings.Join([]string{
				itoa(n) + ".", item.Title,
			}, " ")))
			b.WriteString("\n")
		}
	}
	return b.String()
}

// journalProductAt — продукт по сквозному номеру.
func journalProductAt(sections []journalSection, index int) (journalItem, bool) {
	n := 0
	for _, section := range sections {
		for _, item := range section.Items {
			n++
			if n == index {
				return item, true
			}
		}
	}
	return journalItem{}, false
}

// resolveJournalItems превращает выбранные журналом строки в карточки каталога.
//
// Строка журнала ссылается на заметку `[[…]]` — это и есть целевая карточка.
// Если такой заметки в каталоге нет (карточку ещё не завели), берём по
// названию: иногда название в таблице и в карточке совпадает.
func resolveJournalItems(items []journalItem, catalog []SystemCard) []SystemCard {
	byTitle := make(map[string]SystemCard, len(catalog))
	for _, card := range catalog {
		byTitle[strings.ToLower(strings.TrimSpace(card.Title))] = card
	}
	out := make([]SystemCard, 0, len(items))
	seen := map[int64]bool{}
	for _, item := range items {
		key := item.Note
		if key == "" {
			key = item.Title
		}
		if card, ok := byTitle[strings.ToLower(strings.TrimSpace(key))]; ok && !seen[card.NoteID] {
			seen[card.NoteID] = true
			out = append(out, card)
		}
	}
	return out
}

// findAnaloguesViaJournal — подбор по журналу с моделью.
//
// Возвращает nil, nil — «журнала нет или он без разделов»: вызывающий делает
// откат. Ошибка — модель не ответила.
func (s *Server) findAnaloguesViaJournal(ctx context.Context, source SystemCard,
	catalog []SystemCard) ([]SystemCard, error) {
	journal, err := s.loadCompetitorJournal(ctx)
	if err != nil || strings.TrimSpace(journal) == "" {
		return nil, nil
	}
	sections := parseCompetitorJournal(journal)
	if len(sections) == 0 {
		return nil, nil
	}
	shown, _ := journalSectionCandidates(sections, analogJournalRowLimit)
	if len(shown) == 0 {
		return nil, nil
	}
	answer, err := Complete(ctx, analogLLMSystem, buildJournalAnalogPrompt(source, shown), 0)
	if err != nil {
		return nil, err
	}
	nums := parseAnalogAnswer(answer, analogLLMLimit)
	items := make([]journalItem, 0, len(nums))
	for _, n := range nums {
		if item, ok := journalProductAt(shown, n); ok {
			items = append(items, item)
		}
	}
	log.Printf("боевой листок: журнал дал %d аналогов для «%s»", len(items), source.Title)
	return resolveJournalItems(items, catalog), nil
}

// analogJournalRowLimit — сколько строк журнала показывать модели.
//
// 120 строк — это 4-8 разделов целиком: модель видит границы разделов, а промпт
// остаётся в разумных размерах (журнал целиком — 2200 строк).
const analogJournalRowLimit = 120
