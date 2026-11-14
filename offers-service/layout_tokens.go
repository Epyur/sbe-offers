package main

import (
	"fmt"
	"regexp"
	"strings"
)

// Макет КП (.docx) с плейсхолдерами (2026-11-05).
//
// Пользователь загружает свой документ Word и расставляет в нём два вида
// меток:
//
//	[Наименование объекта]  — ЖЁСТКАЯ подстановка: значение поля формы или
//	                          служебное значение предложения. Пишется как
//	                          есть, ничего не сочиняется.
//	{Опиши систему в двух    — ПРОМТ для модели: текст пишет модель по данным
//	 абзацах}                 конкретного КП, как узел `prompt_text`.
//
// Макет — образец вёрстки: в самом документе остаются его стили, шрифты,
// колонтитулы, таблицы и картинки. Меняется только текст внутри меток.
//
// Почему квадратные и фигурные скобки, а не `{{…}}`: пользователь так и
// сформулировал. `{{…}}` в макете тоже понимается — это имя поля формы
// (`{{object_name}}`) для совместимости с уже существующими формами.
//
// С 2026-11-06 промт можно писать ЛЮБЫМ числом скобок от одной до трёх
// (`{…}`, `{{…}}`, `{{{…}}}`), в том числе когда фигурных скобок больше
// внутри (`{{модуль {параметр}}}`). Живой макет 003/2026 показал, почему это
// обязательно: текст промта длиной 364 символа не помещался в старое
// ограничение 400 БАЙТ, и метка молча не находилась вовсе — в документ
// уходил весь текст промта литералом. Ограничение теперь в рунах и щедрое:
// споткнуться об него человек не должен, ответ всё равно режет модель. Если
// человек набрал `{{…}}` как имя поля (`{{object_name}}`), это проверяется
// по значению формы, а не по числу скобок: см. scanLayoutTokens.

// Виды меток макета.
//
// Служебные виды (с 2026-11-06, шаг C плана «генерация без модели») печатают
// ДАННЫЕ предложения и карточки Базы знаний и модель не зовут вовсе:
//
//	{{block:1}}                     — блок формы целиком, по id;
//	{{система}}                     — картинка и состав карточки одной таблицей;
//	{{ссылки: система}}             — `## Ссылки` карточки;
//	{{характеристики: система}}     — `## Свойства и преимущества` таблицей;
//	{{текст: система: description}} — сохранённый текст системы (шаг B).
//
// Это главный смысл шага: администратор правит макет сколько угодно, а экспорт
// по-прежнему укладывается в секунды, потому что модель для этих меток не
// нужна.
const (
	layoutTokenField    = "field"    // [имя] или {{имя}} — жёсткая подстановка
	layoutTokenPrompt   = "prompt"   // {текст}, {{текст}}, {{{текст}}} — промт модели
	layoutTokenImage    = "image"    // {картинка}, {{картинка: система 1}} — изображение
	layoutTokenBlock    = "block"    // {{block:1}} — блок формы по id
	layoutTokenSystem   = "system"   // {{система}}, {{система: таблица}}
	layoutTokenLinks    = "links"    // {{ссылки: система}}
	layoutTokenSpecs    = "specs"    // {{характеристики: система}}
	layoutTokenText     = "text"     // {{текст: система: description|advantages}}
	layoutTokenMaterial = "material" // {{материал}} — строка состава по позиции
	// layoutTokenComparison — `{{сравнение}}`: весь боевой листок одним куском
	// (2026-11-11). Перечень продуктов, затем сравнение — общее и по каждому.
	// Единственная метка боевого листка: форму с блоками у него убрали, и
	// администратор вставляет в макет ровно её.
	layoutTokenComparison = "comparison"
	// layoutTokenSection — `{ content: <id> }`: содержательный раздел
	// (2026-11-14). Называет НАСТРОЙКУ раздела, заведённую администратором в
	// форме: какие подразделы писать, при каком условии и о чём. Текст пишет
	// модель по подразделам, оформление — макет Word.
	//
	// Почему новый ключ, а не прочие `{…}`: голая метка `{…}` — промт модели
	// (`layoutTokenPrompt`), и `{content: …}` иначе ушла бы в модель литералом.
	// `content:` — зарезервированный префикс, разбираемый ДО промта, ровно как
	// `block:`.
	layoutTokenSection = "section"
)

// serviceLayoutMarkers — слова-ключи служебных меток.
//
// Порядок важен: «характеристики» начинается не с «систем», поэтому проверка
// по списку не путается. Все они, кроме «block», сами по себе НЕ метка: слово
// должно стоять в НАЧАЛЕ метки или сразу после «имя=», иначе живая инструкция
// администратора («опиши систему: состав, преимущества, ссылки») приняла бы
// вид служебной и потеряла бы текст модели.
var serviceLayoutMarkers = []struct {
	kind string
	key  string
}{
	{layoutTokenLinks, "ссылки"},
	{layoutTokenSpecs, "характеристик"},
	{layoutTokenText, "текст"},
	{layoutTokenBlock, "block"},
	{layoutTokenSystem, "систем"},
	{layoutTokenMaterial, "материал"},
	{layoutTokenComparison, "сравнени"},
	{layoutTokenSection, "content"},
	{layoutTokenSection, "содержани"},
}

// parseLayoutServiceSpec узнаёт служебную метку по содержимому.
//
// Понимает:
//
//	block:1                      → блок формы с ID «1»
//	система / система: таблица   → карточка системы (таблица или блок)
//	ссылки: система              → ссылки карточки
//	характеристики: система      → характеристики карточки
//	текст: система: description  → сохранённый текст системы
//	система name="S"             → то же плюс повтор по системам (шаг D)
//
// Второе значение — вид метки; ложь значит «не служебная», и метка уходит в
// модель обычным промтом.
func parseLayoutServiceSpec(inner string) (layoutServiceSpec, bool) {
	trimmed := strings.TrimSpace(inner)
	if trimmed == "" {
		return layoutServiceSpec{}, false
	}
	key, rest := splitLayoutMarkerKey(trimmed)
	name := layoutMarkerName(rest)
	rest = stripLayoutMarkerName(rest)
	normalized := normalizeFieldKey(markerHead(rest))
	spec, ok := layoutServiceSpec{name: name}.withKind(serviceLayoutKindOf(key, normalized), rest)
	if !ok {
		return layoutServiceSpec{}, false
	}
	return spec, true
}

// splitLayoutMarkerKey отделяет слово-ключ метки от остатка.
//
// Ключ — ПЕРВОЕ слово метки, а не всё до двоеточия: живая инструкция
// администратора «Опиши систему: состав, преимущества, ссылки» тоже содержит
// двоеточие, и разбор по нему принял бы её за служебную метку с ключом
// «опиши систему» — текст модели потерялся бы вовсе.
//
// «текст системы» и «ссылки системы» при этом понимаются: остаток начинается
// со слова «системы», и вид метки решает serviceLayoutKindOf.
func splitLayoutMarkerKey(s string) (key, rest string) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return "", ""
	}
	// Первое слово может нести двоеточие: «block:1», «система: таблица».
	// Тогда ключ — до двоеточия, а остаток — после него вместе с хвостом.
	first := fields[0]
	if at := strings.Index(first, ":"); at >= 0 {
		// Остаток — всё после двоеточия: и хвост первого слова (`block:1`),
		// и слова за ним (`ссылки: система`).
		tail := strings.TrimSpace(first[at+1:] + " " + strings.Join(fields[1:], " "))
		return strings.ToLower(strings.TrimSpace(first[:at])), strings.TrimSpace(tail)
	}
	return strings.ToLower(first), strings.TrimSpace(strings.TrimPrefix(s, first))
}

// markerHead — первые два слова остатка: «система: description» → «система».
//
// Нужен, чтобы «текст: система: description» и «текст системы: преимущества»
// привели к одному виду метки.
func markerHead(rest string) string {
	if at := strings.Index(rest, ":"); at >= 0 {
		return strings.TrimSpace(rest[:at])
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// serviceLayoutKindOf решает вид по слову-ключу и первому слову остатка.
func serviceLayoutKindOf(key, head string) string {
	switch key {
	case "block", "блок":
		return layoutTokenBlock
	case "текст", "text":
		return layoutTokenText
	case "сравнение", "comparison":
		return layoutTokenComparison
	case "content", "содержание":
		// `{ content: <id> }` — содержательный раздел (2026-11-14). Ключ
		// проверяется ДО промта и до общего списка слов-ключей: это не
		// инструкция модели, а имя настройки раздела.
		return layoutTokenSection
	}
	for _, marker := range serviceLayoutMarkers {
		if !strings.HasPrefix(key, marker.key) {
			continue
		}
		switch marker.kind {
		// «система …» — картинка и состав.
		case layoutTokenSystem:
			return layoutTokenSystem
		// «материал …» — строка состава позиции.
		case layoutTokenMaterial:
			return layoutTokenMaterial
		// «ссылки» без уточнения — ссылки ВСЕХ выбранных систем и их
		// материалов (2026-11-09). Раньше голая `{{ссылки}}` не была меткой
		// вовсе (уходила в модель как промт), и человек получал пустое место,
		// хотя просил ровно то, что метка умеет.
		case layoutTokenLinks:
			return layoutTokenLinks
		}
		// Прочие слова-ключи требуют, чтобы речь шла о системе: иначе
		// «картинки» сюда не попадут.
		if isSystemWord(head) {
			return marker.kind
		}
		return ""
	}
	return ""
}

// layoutServiceSpec — разобранная служебная метка.
type layoutServiceSpec struct {
	// kind — вид метки: block, system, links, specs, text. Пусто — не служебная.
	kind string
	// blockID — id блока формы для `{{block:1}}`.
	blockID string
	// textBlock — какой текст системы просит метка: description | advantages.
	textBlock string
	// variant — «таблица» у `{{система: таблица}}`: печатать друг под другом.
	variant string
	// name — имя повтора (`name="S"`), по нему связываются метка и её блок.
	name string
	// sectionID — id содержательного раздела для `{ content: <id> }`.
	sectionID string
}

// withKind доделывает разбор: id блока, блок текста и вариант.
func (s layoutServiceSpec) withKind(kind, rest string) (layoutServiceSpec, bool) {
	if kind == "" {
		return layoutServiceSpec{}, false
	}
	s.kind = kind
	switch kind {
	case layoutTokenBlock:
		if id := strings.TrimSpace(strings.TrimPrefix(rest, ":")); id != "" {
			// `block:1` и `block 1` — одно и то же; дополнительно снимаются
			// кавычки, если id в них завёрнут.
			if space := strings.IndexAny(id, " \t;,"); space >= 0 {
				id = id[:space]
			}
			s.blockID = strings.Trim(id, `"'«» `)
		}
		if s.blockID == "" {
			return layoutServiceSpec{}, false
		}
	case layoutTokenText:
		// `{{текст системы: обоснование}}`: после слова «текст» первым стоит
		// «системы», и вид блока лежит во ВТОРОМ остатке.
		textRest := rest
		if isSystemWord(markerHead(rest)) {
			textRest = rest[strings.Index(rest, markerHead(rest))+len(markerHead(rest)):]
		}
		s.textBlock = systemTextBlockOf(textRest)
		if s.textBlock == "" {
			return layoutServiceSpec{}, false
		}
		// `{{текст: S: description}}` — имя повтора стоит первым остатком, а
		// `{{текст: система: description}}` — словом «система», то есть
		// повтора нет и печатается первая система.
		if s.name == "" {
			if head := markerHead(rest); head != "" && !isSystemWord(head) {
				s.name = strings.Trim(head, `"'«» `)
			}
		}
	case layoutTokenSystem:
		if strings.Contains(normalizeFieldKey(rest), "таблиц") {
			s.variant = "table"
		}
	case layoutTokenComparison:
		// `{{сравнение}}` — метка без уточнений: весь боевой листок одним куском.
		// Остаток игнорируется, чтобы «сравнение продуктов» тоже сработало.
	case layoutTokenSection:
		// `{ content: proposal_systems }` — id раздела после двоеточия. Кавычки
		// снимаются: id может быть записан как `content: "proposal_systems"`.
		s.sectionID = strings.Trim(strings.TrimSpace(rest), `"'«» `)
		if at := strings.IndexAny(s.sectionID, " \t;,}"); at >= 0 {
			s.sectionID = strings.Trim(s.sectionID[:at], `"'«» `)
		}
		if s.sectionID == "" {
			// `{ content: }` без имени — метка, а не инструкция модели: пустой
			// раздел печатает ничего и пишет в журнал (renderContentSection).
			// Отказывать в разборе нельзя — иначе пустое имя ушло бы в модель
			// литералом.
			return s, true
		}
	}
	return s, true
}

// systemTextBlockOf — какой блок системы просит метка текста.
//
// Понимает и английские ключи (`description`/`advantages`), и русские слова
// администратора: «обоснование», «преимущества». Непонятное слово — не метка
// текста вовсе: подставить вместо него описание значило бы напечатать не то,
// что просили.
func systemTextBlockOf(rest string) string {
	lower := strings.ToLower(rest)
	switch {
	case strings.Contains(lower, systemTextDescription), strings.Contains(lower, "обоснован"),
		strings.Contains(lower, "описани"):
		return systemTextDescription
	case strings.Contains(lower, systemTextAdvantages), strings.Contains(lower, "преимуществ"),
		strings.Contains(lower, "advantage"):
		return systemTextAdvantages
	}
	return ""
}

// isSystemWord — слово «система»/«system» в любом падеже.
func isSystemWord(word string) bool {
	normalized := normalizeFieldKey(word)
	return strings.HasPrefix(normalized, "систем") || strings.HasPrefix(normalized, "system")
}

// layoutMarkerName — имя повтора из метки: `система name="S"` → S.
func layoutMarkerName(rest string) string {
	at := strings.Index(rest, `name=`)
	if at < 0 {
		return ""
	}
	value := strings.TrimSpace(rest[at+len(`name=`):])
	value = strings.Trim(value, `"'«» `)
	if space := strings.IndexAny(value, " \t"); space >= 0 {
		value = value[:space]
	}
	return strings.Trim(value, `"'«» `)
}

// stripLayoutMarkerName убирает имя повтора из остатка метки.
func stripLayoutMarkerName(rest string) string {
	at := strings.Index(rest, `name=`)
	if at < 0 {
		return rest
	}
	return strings.TrimSpace(rest[:at])
}

// Пределы длины текста внутри метки, в рунах.
//
// У промта предел щедрый намеренно: инструкция — это абзац-два обычного
// текста, и 3000 рун вмещают примерно два листа. Поле — имя или короткая
// подпись, 200 рун с запасом.
const (
	layoutFieldMaxRunes  = 200
	layoutPromptMaxRunes = 3000
)

// layoutToken — метка вместе с её местом в тексте.
type layoutToken struct {
	Kind  string
	Raw   string // как написано в макете, вместе со скобками
	Inner string // что внутри скобок
	Start int
	End   int
	// Service — разбор служебной метки (шаг C). Заполнен только у служебных
	// видов: второй раз те же слова в печати не разбираются.
	Service layoutServiceSpec
}

var (
	// reLayoutField — квадратные скобки: `[Наименование объекта]`.
	//
	// Внутри не должно быть квадратных скобок и переводов строк: цель ссылки
	// Markdown `[текст](адрес)` в макете тоже квадратная, и без ограничения
	// в метку попала бы половина ссылки. Перевод строки ограничивает метку
	// абзацем: плейсхолдер в макете — всегда одна строка.
	//
	// Число — в рунах, а не в байтах: русская буква — два байта, и предел в
	// символах обрезал бы вдвое более короткий текст. Значение дублируется в
	// выражении, потому что Go-регэкспы не принимают повтор больше 1000, а
	// подстановка в шаблон из константы досталась бы ценой сборки выражения
	// на старте — за это платить нечем: тест ниже сверяет обе величины.
	reLayoutField = regexp.MustCompile(fmt.Sprintf(`\[([^\[\]\n]{1,%d})\]`, layoutFieldMaxRunes))
	// reLayoutBraceField — `{{object_name}}`: имя поля формы, как в блоках.
	//
	// Находится первым и «съедает» свои скобки: иначе `{{object_name}}`
	// разобралось бы как промт «object_name». Если двойная скобка оказалась
	// не именем поля, а инструкцией, она отдаётся в промт при разборе
	// (scanLayoutTokens): вид метки решает содержимое, а не число скобок.
	reLayoutBraceField = regexp.MustCompile(`\{\{([^{}\n]{1,200})\}\}`)
	// reLayoutPrompt — фигурные скобки любой глубины от одной до трёх.
	//
	// Главное в этом виде: скобок внутри не запрещаем. Инструкция может
	// ссылаться на название в фигурных скобках (`{параметр}`), и старый
	// разбор на этом месте обрывался — метка не находилась вовсе.
	// Разбор идёт от открывающей скобки нежадным поиском закрывающей
	// ПЕРВОЙ группы из стольких же скобок; лишние скобки внутри уходят в
	// promptInner, и сам промт получает текст без крайних скобок метки.
	// Общая ширина ограничена 3000 рун в коде, а не в выражении:
	// Go-регэкспы не принимают повтор больше 1000.
	reLayoutPrompt = regexp.MustCompile(`\{\{?(?s:([^{}]*?(?:\{[^{}]*\}[^{}]*?)*))\}\}?`)
)

// promptInner отсекает у промта скобки, оставшиеся от метки.
//
// Сканер нарочно разрешает скобки внутри промта (`{{модуль {параметр}}}`):
// нежадный разбор оставляет лишние фигурные скобки по соседству с краями,
// и метка «двойная» должна отдать модели ровно свой текст. Снимается не
// больше того, что действительно примкнуло к краю — по символу за проход,
// чтобы не съесть скобку, которая принадлежит тексту (`{параметр}` в конце).
func promptInner(inner string) string {
	for strings.HasPrefix(inner, "{") && strings.HasSuffix(inner, "}") {
		inner = strings.TrimSpace(inner[1 : len(inner)-1])
	}
	return normalizePromptText(inner)
}

// normalizePromptText приводит многоабзацный промт к виду, пригодному для
// модели.
//
// Промт в макете человек набирает как удобно: списком, абзацами, с пустыми
// строками между частями. Через метку такой текст обязан дойти до модели
// связным — на живом КП 003/2026 (2026-11-06) до неё доходило
// «…входящих в состав решения;- преимущества применения…», где пункты списка
// слиплись в одну строку, а перенос строки между абзацами потерялся вовсе.
//
// Что делается:
//   - пустые строки убираются (разделитель абзацев в макете — вёрстка
//     документа, а не часть инструкции);
//   - строка сохраняет свою роль: пункт списка (`- `, `• `, `1. `) остаётся
//     пунктом и начинается с новой строки, обычный абзац продолжает
//     предыдущий через пробел.
//
// Смысл: модель получает ровно тот текст, который человек набрал, — с
// дефисами списков и без склеенных слов.
func normalizePromptText(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if isPromptListItem(trimmed) {
			out = append(out, trimmed)
			continue
		}
		// Продолжение предыдущего — через пробел.
		if n := len(out); n > 0 && !isPromptListItem(out[n-1]) {
			out[n-1] = strings.TrimRight(out[n-1], " ") + " " + trimmed
			continue
		}
		out = append(out, trimmed)
	}
	return strings.Join(out, "\n")
}

// isPromptListItem — строка выглядит пунктом списка: маркер или номер.
func isPromptListItem(line string) bool {
	for _, marker := range []string{"- ", "– ", "— ", "• ", "* "} {
		if strings.HasPrefix(line, marker) {
			return true
		}
	}
	// «1. », «2)» — номер пункта: до трёх цифр и точка или скобка.
	i := 0
	for i < len(line) && i < 3 && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 0 || i >= len(line) {
		return false
	}
	return (line[i] == '.' || line[i] == ')') && (i+1 == len(line) || line[i+1] == ' ')
}

// clampRunes обрезает строку до n рун.
func clampRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// scanLayoutTokens находит все метки макета в порядке появления.
//
// Возвращает метки ПО ВОЗРАСТАНИЮ Start; пересечений нет — `{{…}}` разбирается
// первым и «съедает» свои скобки, чтобы `{object_name}` внутри не стало
// промтом.
//
// values — что известно формы: двойная скобка считается именем поля, только
// если это имя там есть. Иначе она промт, и это главное отличие от старого
// разбора: человек вправе написать длинную инструкцию в `{{…}}`, как он и
// сделал в макете 003/2026, — и получить пустое место вместо текста было бы
// молчаливой потерей. nil — проверять не по чему, двойная скобка остаётся
// полем (так вызывают описание макета для администратора).
func scanLayoutTokens(text string, values map[string]string) []layoutToken {
	var tokens []layoutToken
	masked := text
	for _, m := range reLayoutBraceField.FindAllStringSubmatchIndex(text, -1) {
		inner := strings.TrimSpace(text[m[2]:m[3]])
		if !isLayoutFieldName(inner, values) {
			continue
		}
		token := layoutToken{
			Kind: layoutTokenField, Raw: text[m[0]:m[1]],
			Inner: inner, Start: m[0], End: m[1],
		}
		tokens = append(tokens, token)
		// Затираем найденное одинаковым по длине мусором: индексы остальных
		// совпадений не сдвинутся, а фигурная скобка внутри уже не найдётся.
		masked = replaceRange(masked, m[0], m[1])
	}
	for _, m := range reLayoutField.FindAllStringSubmatchIndex(masked, -1) {
		tokens = append(tokens, layoutToken{
			Kind: layoutTokenField, Raw: text[m[0]:m[1]],
			Inner: strings.TrimSpace(text[m[2]:m[3]]), Start: m[0], End: m[1],
		})
	}
	for _, m := range reLayoutPrompt.FindAllStringSubmatchIndex(masked, -1) {
		inner := promptInner(text[m[2]:m[3]])
		token := layoutToken{Raw: text[m[0]:m[1]], Start: m[0], End: m[1]}
		// Вид метки решает СОДЕРЖИМОЕ, а не число скобок (правило 2026-11-06):
		//
		//	{картинка}     — картинка (проверяется первой: «картинка: система»
		//	                 обычный текст инструкции про картинку не ломает);
		//	{система}      — карточка системы одной таблицей;
		//	{block:1}      — блок формы по id;
		//	{ссылки: …}    — ссылки карточки и т.д.
		//
		// Всё прочее — промт модели.
		if _, ok := parseLayoutImageSpec(inner); ok {
			token.Kind = layoutTokenImage
		} else {
			token.Kind = layoutTokenPrompt
			if service, ok := parseLayoutServiceSpec(inner); ok {
				token.Kind = service.kind
				token.Service = service
			}
		}
		// Слишком длинный промт не выбрасываем: лишний раз человека не
		// наказываем, а режем по пределу — метка всё равно должна заменить
		// собой абзац, а не остаться литералом в документе.
		token.Inner = clampRunes(inner, layoutPromptMaxRunes)
		tokens = append(tokens, token)
	}
	sortLayoutTokens(tokens)
	return tokens
}

// layoutImageSpec — что просит метка картинки.
type layoutImageSpec struct {
	// of: system или material.
	of string
	// index — номер системы/материала (1 = первый); 0 — первый.
	index int
}

// parseLayoutImageSpec узнаёт метку картинки по содержимому.
//
// Понимает «картинка», «картинка: система», «картинка: материал 2»,
// «картинка системы 1», «изображение», «image: material 2». Других слов не
// требуется: администратор пишет по-русски и по-человечески.
//
// Отказ важнее согласия: «картинки нет», «не надо изображение» — обычный текст
// инструкции, и подставить туда картинку значит выполнить противоположное
// написанному. Поэтому отрицание и «нет» после слова-ключа — не метка.
func parseLayoutImageSpec(inner string) (layoutImageSpec, bool) {
	lower := strings.ToLower(strings.TrimSpace(inner))
	if !startsWithImageWord(lower) {
		return layoutImageSpec{}, false
	}
	if strings.Contains(lower, " нет") || strings.HasPrefix(lower, "не ") ||
		strings.Contains(lower, "не надо") || strings.Contains(lower, "без ") {
		return layoutImageSpec{}, false
	}
	spec := layoutImageSpec{of: imageOfSystem}
	switch {
	case strings.Contains(lower, "материал"), strings.Contains(lower, "material"):
		spec.of = imageOfMaterial
	case strings.Contains(lower, "систем"), strings.Contains(lower, "system"):
		spec.of = imageOfSystem
	}
	// Номер — первая цифра в метке: «картинка: система 2», «картинка 2».
	for _, r := range lower {
		if r >= '0' && r <= '9' {
			spec.index = int(r - '0')
			break
		}
	}
	return spec, true
}

// startsWithImageWord — строка начинается со слова-ключа картинки.
//
// Проверяется целое первое слово: «картинка», «картинки», «картинку» — это всё
// про картинку, а «картинный» — нет. Хвост слова после основы разрешён только
// в пределах русских окончаний.
func startsWithImageWord(lower string) bool {
	for _, stem := range []string{"картинк", "изображени", "рисунок", "рисунк", "image", "picture"} {
		if strings.HasPrefix(lower, stem) {
			return true
		}
	}
	return false
}

// isLayoutFieldName — двойная скобка это имя поля формы?
//
// values == nil — проверять не по чему (описание макета), считаем полем, НО
// только если это не служебная метка: `{{система}}` — не имя поля, хотя поля
// с таким именем нет; без этой проверки описание макета считало бы её полем и
// администратор не увидел бы, что метка распознана как карточка.
func isLayoutFieldName(inner string, values map[string]string) bool {
	if inner == "" {
		return false
	}
	if _, ok := parseLayoutServiceSpec(inner); ok {
		return false
	}
	if values == nil {
		// Описание макета для администратора: значения формы unknown, поэтому
		// полем считаем только то, что ПОХОЖЕ на имя поля, — короткий
		// идентификатор без пробелов. Длинную инструкцию с двоеточием
		// (`{{Опиши систему: состав…}}`) описываем как промт: иначе описание
		// макета показало бы её полем и человек решил бы, что текст модели из
		// макета потерян.
		return looksLikeFieldName(inner)
	}
	if _, ok := values[strings.TrimSpace(inner)]; ok {
		return true
	}
	if alias, ok := builtinFieldAliases[normalizeFieldKey(inner)]; ok && alias != "" {
		return true
	}
	_, ok := fieldAliases[strings.TrimSpace(inner)]
	return ok
}

// replaceRange заполняет диапазон символами-заглушками.
func replaceRange(s string, start, end int) string {
	if start < 0 || end > len(s) || start >= end {
		return s
	}
	return s[:start] + strings.Repeat("\x00", end-start) + s[end:]
}

// sortLayoutTokens — порядок по месту в тексте (сортировка вставками: меток в
// абзаце единицы, и лишний пакет ради неё не нужен).
func sortLayoutTokens(t []layoutToken) {
	for i := 1; i < len(t); i++ {
		for j := i; j > 0 && t[j-1].Start > t[j].Start; j-- {
			t[j-1], t[j] = t[j], t[j-1]
		}
	}
}

// layoutFieldValue — значение жёсткой подстановки.
//
// Имя в макете пишется по-человечески («Наименование объекта»), поэтому
// сравнивается без учёта регистра, «ё/е» и пробелов. Неизвестное имя
// подменяется пустой строкой, как и `{{…}}` в блоках: служебные скобки в
// готовом документе недопустимы.
func layoutFieldValue(inner string, values map[string]string) string {
	if v := layoutAliasValue(inner, values); v != "" {
		return v
	}
	if alias, ok := builtinFieldAliases[normalizeFieldKey(inner)]; ok {
		// Служебное поле может быть заполнено из формы под другим ключом:
		// объект в форме КП — `object_name`, а служебное — `offer_object`.
		// Оба описывают одно и то же, и держать это в двух местах — забыть
		// об одном.
		if v := firstNonEmpty(values, alias, fallbackAlias[alias]); v != "" {
			return v
		}
	}
	return ""
}

// isServiceKind — метка печатает данные, а не модель (шаг C).
func isServiceKind(kind string) bool {
	switch kind {
	case layoutTokenBlock, layoutTokenSystem, layoutTokenLinks,
		layoutTokenSpecs, layoutTokenText, layoutTokenMaterial,
		layoutTokenComparison, layoutTokenSection:
		return true
	}
	return false
}

// looksLikeFieldName — строка похожа на имя поля: одно слово, без пробелов и
// знаков конца фразы.
func looksLikeFieldName(inner string) bool {
	if inner == "" || len([]rune(inner)) > 64 {
		return false
	}
	if strings.ContainsAny(inner, " \t\n.,;:!?") {
		return false
	}
	return true
}

// fallbackAlias — второй ключ для служебного поля.
var fallbackAlias = map[string]string{
	fieldObjectName: "object_name",
	fieldClientName: "client_name",
}

// layoutAliasValue — значение по человеческому имени или идентификатору поля.
func layoutAliasValue(inner string, values map[string]string) string {
	key := normalizeFieldKey(inner)
	// Сначала точное имя поля формы (как в `{{…}}`), потом человеческое.
	if v, ok := values[strings.TrimSpace(inner)]; ok {
		return v
	}
	for id, label := range fieldAliases {
		if normalizeFieldKey(label) == key {
			return firstNonEmpty(values, id)
		}
	}
	// Ещё одна попытка: имя поля без префикса `brief.`. Бриф кладёт туда же.
	if v, ok := values["brief."+strings.TrimSpace(inner)]; ok {
		return v
	}
	return ""
}

// firstNonEmpty — первое непустое значение из ключей.
func firstNonEmpty(values map[string]string, keys ...string) string {
	for _, k := range keys {
		if k == "" {
			continue
		}
		if v := strings.TrimSpace(values[k]); v != "" {
			return v
		}
	}
	return ""
}

// normalizeFieldKey — «Наименование объекта» → «наименованиеобъекта».
func normalizeFieldKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "ё", "е")
	return strings.Join(strings.Fields(s), "")
}

// fieldAliases — как поле называется в макете по-человечески.
//
// Пользователь пишет в документе `[Наименование объекта]`, а не
// `[object_name]`: макет — документ для людей, и набирать служебные имена в
// Word никто не станет. Идентификаторы совпадают с полями формы, чтобы одно и
// то же значение можно было взять и `[]`, и `{{…}}`.
var fieldAliases = map[string]string{
	// Служебные — есть у каждого предложения.
	fieldOfferNumber: "Номер предложения",
	fieldOfferDate:   "Дата предложения",
	fieldManager:     "Имя менеджера",
	fieldClientName:  "Получатель из КП",
	fieldObjectName:  "Объект из формы КП",
	"system_title":   "Название системы",
	"systems_list":   "Выбранные системы",

	// Поля формы по умолчанию.
	"supplier_name":     "Поставщик",
	"supplier_contacts": "Контакты поставщика",
	"client_name":       "Получатель (организация)",
	"client_contact":    "Контактное лицо получателя",
	"object_name":       "Объект",
	"object_address":    "Адрес объекта",
	"valid_until":       "Предложение действительно до",
	"conditions":        "Условия",
}

// builtinFieldAliases — дополнительные имена для служебных полей: в макете
// человек пишет «Название объекта» и «Название системы», а не «Объект».
var builtinFieldAliases = map[string]string{
	"названиеобъекта":     fieldObjectName,
	"наименованиеобъекта": fieldObjectName,
	"объект":              fieldObjectName,
	"названиесистемы":     "system_title",
	"система":             "system_title",
	"менеджер":            fieldManager,
	"номер":               fieldOfferNumber,
	"дата":                fieldOfferDate,
	"получатель":          fieldClientName,
	"поставщик":           "supplier_name",
}
