package main

import (
	"regexp"
	"strings"
)

// Текст абзаца как последовательность «прогонов» (w:r) с оформлением.
//
// Зачем отдельный слой (2026-11-05, живой КП 003/2026). Текст из заметок Базы
// знаний и от модели приходит с тем, чего Word не понимает буквально:
//
//   - вики-ссылки `[[Гидрошпонка ТЕХНОНИКОЛЬ ЕС-220-3]]` — раньше они
//     печатались как есть, и заказчик видел квадратные скобки вокруг материалов
//     состава. В документ должна идти подпись, а не разметка Obsidian;
//   - потерянные пробелы: «подземныхсооружений», «уровнемподземных»,
//     «независимоот» — в карточке так и написано, а в предложении это ошибка;
//   - степени и индексы: «г/м2», «м2», «L/20» — в единицах измерения двойка
//     обязана быть надстрочной, а число в «V-SL 2 мм» — нет.
//
// Одна точка на всё: абзац, список, заголовок, ячейку таблицы. Разные точки
// дали бы «в таблице степень обычная, в тексте надстрочная» — расхождение,
// которое видно только глазами и ищется долго.

// textRun — кусок текста с одинаковым оформлением.
type textRun struct {
	Text string
	// Sup/Sub — надстрочный/подстрочный: w:vertAlign superscript/subscript.
	Sup bool
	Sub bool
}

var (
	// reWikiTarget — цель вики-ссылки: `[[цель|подпись]]` или `[[цель]]`.
	reWikiTarget = regexp.MustCompile(`\[\[([^\[\]|]*)(?:\|([^\[\]]*))?\]\]`)
	// reMdLink — markdown-ссылка и картинка: `[текст](адрес)`, `![текст](адрес)`.
	reMdLink = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	// reBold — **жирный** и *курсив* из заметки. Своя копия, а не skills.reCardBold:
	// тот не экспортируется, а тянуть ради одной регулярки весь пакет разбора
	// карточек — лишняя связность печати с чтением.
	reBold = regexp.MustCompile(`\*\*([^*]+)\*\*`)

	// reUnitDegree — единица измерения с показателем: «м2», «см3», «мм2», «га2».
	// Граница слева обязательна: иначе «AL2» в марке материала потерял бы
	// двойку в надстрочную, хотя это часть названия. Правую границу проверяет
	// markTrailingDigit (lookahead RE2 не поддерживает).
	reUnitDegree = regexp.MustCompile(`(?:^|[^А-ЯЁа-яёA-Za-z0-9])(?:мм|см|дм|м|км)(?:2|3)`)
	// rePerUnitDegree — плотность и модуль: «г/м2», «кг/м3», «Н/мм2», «т/м3».
	rePerUnitDegree = regexp.MustCompile(`(?:[А-ЯЁа-яёA-Za-z]+/м(?:м)?)(?:2|3)`)
	// rePowerOfTen — отрицательная степень: «10-3», «10-6». t2 — только цифра
	// степени, у остальных разрядов нет.
	rePowerOfTen = regexp.MustCompile(`(10)-([0-9])`)

	// reAreaWords — единицы словами с числом перед ними: «гектаров2» не бывает,
	// а «м.кв.» в карточках встречается — бригада технологов пишет по-разному.
)

// plainText — человеческий текст из разметки заметки.
//
// Убирает вики-ссылки (оставляет подпись), markdown-ссылки, жирный/курсив,
// обратные кавычки и возвращает пробел в слипшиеся слова. Тот же результат
// нужен документу, поэтому делается один раз — до разбиения на прогоны.
func plainText(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	s = reWikiTarget.ReplaceAllStringFunc(s, func(m string) string {
		sub := reWikiTarget.FindStringSubmatch(m)
		if label := strings.TrimSpace(sub[2]); label != "" {
			return label
		}
		return strings.TrimSpace(sub[1])
	})
	s = reMdLink.ReplaceAllString(s, "$1")
	s = reBold.ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, "`", "")
	return repairGluedWords(s)
}

// gluedSecondPart — приставки и корни, с которых начинается ВТОРОЕ слово в
// слипшейся паре из живых карточек: «|сооружений», «|подземных»,
// «|местных» (в местных). Порядок — от длинных к коротким: «подземн» должен
// проверяться раньше короткого «под», иначе «подземных» развалится на
// «по дземных».
var gluedSecondPart = []string{
	"сооруж", "помещен", "конструкц", "эксплуатац", "строительств", "фундамент",
	"подземн", "надземн", "песчан", "глинист", "мерзл", "независим", "уровн",
	"высот", "котлован", "откос", "вертикальн", "горизонтальн", "перекрыт",
	"мембран", "гидроизоляц", "теплоизоляц", "геотекстил", "дренаж", "прочн",
	"плотност", "температур", "влажност", "подготовк", "основан", "местн",
}

// gluedJoiner — короткие слова, прилипшие в КОНЕЦ: «независимо|от».
//
// Только двухбуквенные: односложные «в», «с», «к», «на» — обычные окончания
// русских слов («мембрана»), и попытка отделить их калечит исправный текст
// (поймано тестом TestPlainTextDoesNotSplitNormalWords).
var gluedJoiner = []string{"от", "их", "ее"}

// repairGluedWords возвращает пробел, потерянный в живой заметке.
//
// Вставка в Obsidian иногда съедает пробел на переносе строки, и в карточке
// остаётся «подземныхсооружений». Чинить это глазами в тысяче карточек
// невозможно, а в документе ошибка видна заказчику. Разбираем по словам:
// короткие не трогаем вовсе — там нечего разделять.
func repairGluedWords(s string) string {
	if len(s) < 12 {
		return s
	}
	var out []string
	for _, word := range strings.Split(s, " ") {
		out = append(out, splitGluedWord(word))
	}
	return strings.Join(out, " ")
}

// splitGluedWord делит одно слово, если оно склеено из двух.
//
// Возвращает слово без изменений, если признаков слипания нет. Разбор ведётся
// по рунам, а не по байтам: русская буква в UTF-8 занимает два байта, и
// байтовый срез разрезал бы символ пополам (поймано первым же прогоном теста).
func splitGluedWord(word string) string {
	trimmed := strings.Trim(word, ".,;:!?()«»\"'")
	if trimmed == "" {
		return word
	}
	suffix := word[len(trimmed):]
	jsPrefix := word[:len(word)-len(trimmed)]
	runes := []rune(trimmed)
	if len(runes) < 6 {
		return word
	}
	lower := strings.ToLower(trimmed)
	if cut := gluedSecondWordCut(runes, lower); cut > 0 {
		return jsPrefix + string(runes[:cut]) + " " + string(runes[cut:]) + suffix
	}
	if cut := splitLeadingPreposition(runes, lower); cut > 0 {
		return jsPrefix + string(runes[:cut]) + " " + string(runes[cut:]) + suffix
	}
	if cut, joiner, ok := gluedJoinerCut(runes, lower); ok {
		return jsPrefix + string(runes[:cut]) + " " + joiner + suffix
	}
	return word
}

// gluedSecondWordCut — место, где начинается второе слово с известной
// приставкой: «независимо|от», «подземных|сооружений», «уровнем|подземных».
//
// Берётся САМОЕ ДЛИННОЕ совпадение, начиная с 3-й буквы: срез в пределах
// первого слога — это не слово, а обрубок. 0 — разрез не найден.
func gluedSecondWordCut(runes []rune, lower string) int {
	best := 0
	for _, p := range gluedSecondPart {
		idx := strings.Index(lower, p)
		if idx < 0 {
			continue
		}
		cut := len([]rune(lower[:idx]))
		if cut < 3 || cut > len(runes)-2 {
			continue
		}
		// Левая половина кончается так, как кончается русское слово
		// («подземных», «уровнем», «независимо»). Согласная на стыке
		// значит, что это одно слово, и резать его нельзя.
		if len(p) > len(string(runes[best:])) || best == 0 {
			if endsWordLike(string(runes[:cut])) {
				best = cut
			}
		}
	}
	return best
}

// gluedJoinerCut — короткое слово, прилипшее в конец: «независимо|от».
//
// Слева должна остаться целая половина (endsWordLike + looksLikeWord): иначе
// «мембрана» развалилась бы на «мембра на».
func gluedJoinerCut(runes []rune, lower string) (int, string, bool) {
	for _, p := range gluedJoiner {
		if !strings.HasSuffix(lower, p) {
			continue
		}
		cut := len(runes) - len([]rune(p))
		if cut < 4 {
			continue
		}
		left := string(runes[:cut])
		if !endsWordLike(left) || !looksLikeWord(left) {
			continue
		}
		return cut, p, true
	}
	return 0, "", false
}

// looksLikeWord — грубая проверка, что слева от предлога стоит слово, а не
// обрубок. Нужна из-за односложных предлогов: «в» и «с» — обычные окончания
// («мембрана» без неё дала бы «мембра на»), и без этой проверки починка
// калечила бы исправный текст.
//
// Признак простой и его достаточно: слово любой длины заканчивается гласной
// или «ь», а перед окончанием-предлогом («на», «от», «по») должен стоять
// слог, то есть слева не меньше четырёх букв и хотя бы одна гласная не в
// последней позиции.
func looksLikeWord(s string) bool {
	runes := []rune(strings.ToLower(s))
	if len(runes) < 4 {
		return false
	}
	vowels := 0
	for _, r := range runes {
		if strings.ContainsRune("аеёиоуыэюя", r) {
			vowels++
		}
	}
	return vowels >= 2
}

// endsWordLike — кончается ли левая половина так, как кончается русское слово:
// гласной, «й», «м» или «х» («уровнем», «подземных», «независимо»).
func endsWordLike(s string) bool {
	runes := []rune(s)
	if len(runes) == 0 {
		return false
	}
	switch runes[len(runes)-1] {
	case 'а', 'е', 'ё', 'и', 'о', 'у', 'ы', 'э', 'ю', 'я', 'й', 'м', 'х':
		return true
	}
	return false
}

// hasVowel — есть ли в слове хоть одна гласная. Обрубок из согласных
// («АЛЬБО м») словом не бывает, и резать по нему нельзя.
func hasVowel(s string) bool {
	return strings.ContainsAny(strings.ToLower(s), "аеёиоуыэюя")
}

// leadingPrepositions — предлоги, прилипшие к началу слова в живых карточках.
// Порядок — от длинных к коротким: «на» проверяется раньше «н».
var leadingPrepositions = []string{"под", "над", "при", "про", "без", "за", "на", "по", "в", "с", "к", "о", "у"}

// splitLeadingPreposition — сколько рун занимает предлог в начале слова,
// если после него идёт известное слово из gluedSecondPart. 0 — не делить.
func splitLeadingPreposition(runes []rune, lower string) int {
	for _, p := range leadingPrepositions {
		if !strings.HasPrefix(lower, p) {
			continue
		}
		pl := len([]rune(p))
		if pl >= len(runes)-3 {
			continue
		}
		rest := string(runes[pl:])
		for _, word := range gluedSecondPart {
			if strings.HasPrefix(rest, word) {
				return pl
			}
		}
	}
	return 0
}

// splitSupsub разбивает текст абзаца на прогоны с над/подстрочными цифрами.
//
// Возвращает один прогон, если выделять нечего: обычный текст не должен
// обрастать лишними w:r — он читается в тестах и в отладке.
func splitSupsub(text string) []textRun {
	if text == "" {
		return nil
	}
	marked := markSupsub(text)
	if !strings.ContainsAny(marked, "\x00\x01") {
		return []textRun{{Text: text}}
	}
	var runs []textRun
	var cur strings.Builder
	// curSup/curSub — оформление ПРОГОНА, который сейчас набирается. Маркер
	// поднимает следующую за ним цифру: он не создаёт пустой прогон, а
	// переключает набор на надстрочный и снимается на первом же обычном
	// символе.
	var curSup, curSub, pending bool
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		runs = append(runs, textRun{Text: cur.String(), Sup: curSup, Sub: curSub})
		cur.Reset()
	}
	for _, r := range marked {
		if r == '\x00' || r == '\x01' {
			flush()
			curSup, curSub, pending = r == '\x00', r == '\x01', true
			continue
		}
		if !pending {
			// Обычный символ: предыдущий над/подстрочный фрагмент кончился.
			if curSup || curSub {
				flush()
				curSup, curSub = false, false
			}
		}
		pending = false
		cur.WriteRune(r)
	}
	flush()
	return runs
}

// markSupsub проставляет маркеры перед цифрами, которые надо поднять или
// опустить. '\x00' — надстрочная, '\x01' — подстрочная.
//
// Порядок важен: сначала плотность и степень десятки (они точнее), потом
// одиночная единица. Обратный порядок разнял бы «г/м2» на «г/м» + цифру.
// Маркер ставится ПО ИНДЕКСУ конца совпадения, а не переписыванием самого
// совпадения: переписав весь кусок «г/м2», получили бы «г/м\x002» — ровно
// на байте, которого в исходном тексте не было (поймано тестом).
func markSupsub(text string) string {
	s := rePowerOfTen.ReplaceAllString(text, "$1-\x00$2")
	// Готовые знаки степени, которые модель уже прислала (2026-11-06).
	// Живой ответ КП 003/2026 после 0.7.5 написал «4800 м²» символом U+00B2,
	// а не «м2»: правила ниже ищут АСКИ-цифру и такой текст не трогали. Word
	// показывает `²` строчным символом того же размера — заказчик и просил
	// «правильные верхние индексы». Знак заменяется на цифру с маркером, чтобы
	// у степени было настоящее оформление, а не «похожий символ».
	s = strings.ReplaceAll(s, "²", "\x00"+"2")
	s = strings.ReplaceAll(s, "³", "\x00"+"3")
	s = insertSupBeforeLastDigit(s, rePerUnitDegree)
	s = insertSupBeforeLastDigit(s, reUnitDegree)
	return s
}

// insertSupBeforeLastDigit вставляет маркер перед последней цифрой каждого
// совпадения, если за ней не идёт ещё цифра.
func insertSupBeforeLastDigit(s string, re *regexp.Regexp) string {
	matches := re.FindAllStringIndex(s, -1)
	if len(matches) == 0 {
		return s
	}
	var out strings.Builder
	prev := 0
	for _, m := range matches {
		end := m[1]
		if end <= prev || s[end-1] < '0' || s[end-1] > '9' {
			continue
		}
		// «м25» — число 25, а не площадь: цифра после нашей — не степень.
		if end < len(s) && s[end] >= '0' && s[end] <= '9' {
			continue
		}
		out.WriteString(s[prev : end-1])
		out.WriteString("\x00")
		prev = end - 1
	}
	out.WriteString(s[prev:])
	return out.String()
}

// runsXML — разметка прогонов для абзаца. baseRPr — оформление прогона из
// строки (шрифт, размер, жирность), к нему добавляется vertAlign.
func runsXML(text, baseRPr string) string {
	var sb strings.Builder
	for _, r := range splitSupsub(text) {
		if r.Text == "" {
			continue
		}
		rPr := baseRPr
		switch {
		case r.Sup:
			rPr += `<w:vertAlign w:val="superscript"/>`
		case r.Sub:
			rPr += `<w:vertAlign w:val="subscript"/>`
		}
		sb.WriteString(`<w:r>`)
		if rPr != "" {
			sb.WriteString(`<w:rPr>` + rPr + `</w:rPr>`)
		}
		sb.WriteString(`<w:t xml:space="preserve">` + escapeXML(r.Text) + `</w:t></w:r>`)
	}
	return sb.String()
}
