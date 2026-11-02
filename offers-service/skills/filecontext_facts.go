package skills

import (
	"regexp"
	"strings"
)

// Якоря в свободном тексте файла (2026-11-01).
//
// Менеджер присылает не анкету: «Предлагаем Вам, для объекта: Многоквартирный
// дом … по ул. Охотская …, использовать материалы «Корпорации ТехноНИКОЛЬ» по
// специальным ценам». Это живой текст старого КП, и объект из него берётся
// дословно — модель тут не нужна и только испортит.
//
// Правило проекта про кириллицу: `\b` в Go считает словом только ASCII, поэтому
// границы задаются явно.

var (
	// reFileObject — наименование объекта.
	//
	// Две формы: «для объекта: …» из старого КП и «Объект: …» из обычного файла.
	// Конец фразы — «, использовать» (живой файл пишет так), либо конец строки.
	// Точку концом НЕ считаем: в наименовании их полно — «по ул. Охотская»,
	// «г. Новосибирска». Первая попытка обрывала объект на «по ул.»
	// (поймано тестом).
	reFileObject = regexp.MustCompile(`(?i)(?:для\s+объекта|объект(?:\s+исследования)?)\s*[:\-–—]\s*(.+?)(?:,\s*использовать\b|$)`)
	// reFileAreaRow — подпись «Площадь: 4800 м2» в тексте и в таблице.
	//
	// В таблице площадь бывает разбита: «Общая площадь: | м2 | 9200» — единица
	// стоит ДО числа. Поэтому число берётся последнее в строке, а не первое:
	// иначе из «м2» выковыривается двойка (поймано тестом).
	reFileArea = regexp.MustCompile(`(?i)(?:общая\s+)?площад[ьи]`)
	// reAreaNumber — число вместе с единицей площади, если она рядом:
	// «9 200 м2», «4800 м²», «1200 кв. м».
	reAreaNumber = regexp.MustCompile(`[0-9][0-9\s]*(?:[.,][0-9]+)?\s*(?:м2|м²|кв\.?\s?м)`)
	// reAreaBareNumber — число без единицы: форма таблицы «| м2 | 9200».
	reAreaBareNumber = regexp.MustCompile(`[0-9][0-9\s]*(?:[.,][0-9]+)?`)
	// reAreaUnit — единица площади рядом с подписью.
	reAreaUnit = regexp.MustCompile(`(?i)(м2|м²|кв\.?\s?м)`)
	// reFileSystemTitle — заголовок системы: «ТН-ФУНДАМЕНТ Дренаж Проф».
	//
	// Начинается с «ТН-» или с существительного «СИСТЕМА», за которым идёт
	// «ТН-…». Регистр после «ТН-» бывает любым: «ТН-ФУНДАМЕНТ Дренаж Проф» —
	// это живой файл, а требование капса в первой версии отсекло обе системы
	// (поймано тестом). Хвост после «|» отрезается: там «СОСТАВ СИСТЕМЫ».
	reFileSystem = regexp.MustCompile(`(?i)^(?:система\s+)?(ТН[\s-][^|]{2,60})`)
	// reFileObjectKind — тип объекта из наименования.
	reFileObjectKind = regexp.MustCompile(`(?i)^(жилой\s+дом|многоквартирн[а-яё]*\s+(?:многоэтажн[а-яё]*\s+)?дом|жилой\s+комплекс|административн[а-яё]*\s+здани[а-яё]*|производственн[а-яё]*\s+здани[а-яё]*|склад|паркинг|подземн[а-яё]*\s+паркинг|торгов[а-яё]*\s+центр|школ|детск[а-яё]*\s+сад|больниц|и?ндустриальн[а-яё]*\s+парк|логистическ[а-яё]*\s+комплекс)`)
)

// ExtractBriefFacts достаёт из строк файла то, что написано дословно.
//
// Без модели: подпись и значение видно глазами, а модель на таком тексте
// добавляет от себя («жилой комплекс», которого в файле нет). Всё, что не
// опознано по подписи, остаётся в заметках — вместе с описанием и
// преимуществами, они нужны секциям текста.
func ExtractBriefFacts(lines []string) FileContext {
	out := emptyFileContext()
	var notes []string
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if i == 0 {
			// UTF-8 BOM из редактора на Windows — не часть первой строки:
			// без этого «Объект:» в начале файла не опознаётся (2026-11-01).
			line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		}
		if line == "" {
			continue
		}
		if name := matchFileObject(line); name != "" {
			out.put("object_name", name)
			if kind := matchObjectPurpose(name); kind != "" {
				out.put("object_purpose", kind)
			}
			continue
		}
		if area := matchFileArea(line); area != "" {
			out.put("object_area", area)
			continue
		}
		if system := matchFileSystem(line); system != "" {
			out.put("planned_systems", system)
			continue
		}
		notes = append(notes, line)
	}
	out.Notes = strings.Join(notes, "\n")
	out.Summary = fileContextSummary(out)
	return out
}

// matchFileObject — наименование объекта из фразы «для объекта: …».
func matchFileObject(line string) string {
	m := reFileObject.FindStringSubmatch(line)
	if len(m) < 2 {
		return ""
	}
	name := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(m[1]), ","))
	// Слишком короткое — это не наименование, а обрывок («для объекта: наш»).
	if len([]rune(name)) < 10 {
		return ""
	}
	return name
}

// matchObjectPurpose — тип объекта из наименования. Только если он назван:
// «жилой дом», «паркинг». Догадку в бриф не пишем.
func matchObjectPurpose(name string) string {
	m := reFileObjectKind.FindString(name)
	return strings.TrimSpace(m)
}

// matchFileArea — площадь из строки, где встречается слово «площадь».
//
// Две формы записи, обе живые:
//   - «Площадь: 9 200 м2» — число вместе с единицей;
//   - «Общая площадь: | м2 | 9200» — таблица, единица перед числом.
//
// Сначала ищем число с единицей рядом; если такой пары нет — берём последнее
// число строки. Брать всегда последнее нельзя: в «Площадь: 9 200 м2» последняя
// цифра — двойка из «м2» (поймано на живом файле).
func matchFileArea(line string) string {
	if !reFileArea.MatchString(line) {
		return ""
	}
	if m := reAreaNumber.FindString(line); m != "" {
		return normalizeArea(m)
	}
	numbers := reAreaBareNumber.FindAllString(line, -1)
	if len(numbers) == 0 {
		return ""
	}
	return normalizeArea(numbers[len(numbers)-1] + " м2")
}

// normalizeArea приводит площадь к «<число> м2»: пробелы между разрядами
// сохраняются, единица — к «м2».
func normalizeArea(raw string) string {
	value := strings.Join(strings.Fields(raw), " ")
	for _, unit := range []string{"м²", "кв.м", "кв. м", "квм"} {
		value = strings.ReplaceAll(value, unit, "м2")
	}
	value = strings.ReplaceAll(value, " кв ", " ")
	if strings.HasSuffix(value, " м2") {
		return value
	}
	return strings.TrimSpace(value) + " м2"
}

// matchFileSystem — заголовок системы в файле.
//
// Запретные слова проверяются на ЗАГОЛОВКЕ, а не на всей строке: в таблице за
// названием системы идёт «| СОСТАВ СИСТЕМЫ», и проверка по строке отсекала бы
// каждую систему (поймано тестом).
func matchFileSystem(line string) string {
	trimmed := strings.TrimSpace(line)
	m := reFileSystem.FindStringSubmatch(trimmed)
	if len(m) < 2 {
		return ""
	}
	title := strings.Join(strings.Fields(m[1]), " ")
	lower := strings.ToLower(title)
	if strings.Contains(lower, "состав") || strings.Contains(lower, "описание") ||
		strings.Contains(lower, "итого") || strings.Contains(lower, "сметная") {
		return ""
	}
	return title
}

// fileContextSummary — «что понял» по дословному разбору.
func fileContextSummary(c FileContext) []string {
	summary := make([]string, 0, len(c.Fields))
	if v := c.Fields["object_name"]; len(v) > 0 {
		summary = append(summary, "объект: "+v[0])
	}
	if v := c.Fields["object_area"]; len(v) > 0 {
		summary = append(summary, "площадь: "+v[0])
	}
	if v := c.Fields["planned_systems"]; len(v) > 0 {
		summary = append(summary, "системы из файла: "+strings.Join(v, "; "))
	}
	return summary
}
