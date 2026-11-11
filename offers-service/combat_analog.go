package main

import (
	"context"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"
)

// «Найти аналоги и сравнить» (2026-11-11, просьба пользователя).
//
// Смысл кнопки — ПОМОЧЬ СРАВНИТЬ: когда наш продукт уже выбран, найти ему
// продукты конкурентов того же класса из Базы знаний и предложить добавить их в
// листок. Раньше подбор шёл в другую сторону (аналог к конкуренту), и в листке
// без конкурентов он не работал вовсе — а жалоба ровно про такой листок.
//
// Подбор словесный, но с жёстким порогом: прежняя версия отдавала «МОСТОФЛЕКС»
// к PIR-плите, потому что считала любое пересечение слов совпадением. Здесь
// аналог получает только тот, у кого совпал хотя бы один значимый класс
// продукта — слово из НАЗВАНИЯ нашего продукта.

// analogCandidate — карточка и её вес совпадения.
type analogCandidate struct {
	card  SystemCard
	score int
}

// analogWeakWords — слова-признаки, которые класс продукта не задают.
//
// «Кровельная шайба» и «кровельная мембрана» — не аналоги: общий у них только
// признак применения. Живая жалоба 2026-11-11: к ПВХ-мембране подбирались шайба
// и активатор, а настоящие аналоги (PLASTFOIL, ТЕХНОНИКОЛЬ) не попадали вовсе —
// общие слова описания («кровельная», «материала») давали им вес, а класс
// продукта никто не сравнивал.
var analogWeakWords = map[string]bool{
	"кровель": true, "кровл": true, "кровля": true, "фасадн": true, "фасад": true,
	"стенов": true, "стена": true, "стены": true, "плоск": true, "плоская": true,
	"эксплу": true, "наружн": true, "внутрен": true, "примен": true,
	// «Теплоизоляционные плиты» и «плиты теплоизоляционные» — одна и та же
	// вещь: назначение отсекается, остаётся плита (2026-11-12).
	"теплоиз": true,
	"высокок": true, "высок": true, "специа": true, "станд": true,
	"специаль": true, "современ": true, "различ": true, "систем": true,
}

// analogStopWords — общие слова, которые есть почти в любом описании.
var analogStopWords = map[string]bool{
	"для": true, "или": true, "как": true, "что": true, "при": true, "это": true,
	"все": true, "типа": true, "также": true, "более": true, "менее": true,
	"него": true, "нее": true, "них": true, "этом": true, "котор": true,
	"являе": true, "испол": true, "приме": true, "материал": true,
	"материалы": true, "продукт": true, "продукты": true,
	"система": true, "системы": true, "данные": true, "может": true,
	"могут": true, "только": true, "благодар": true, "обеспеч": true,
	"позволя": true, "пример": true, "техничес": true, "различн": true,
	"различных": true, "разного": true, "широко": true, "основе": true,
	"основа": true, "основ": true, "поверхн": true, "возможн": true,
	"область": true, "применения": true, "имеет": true, "имеют": true,
}

// analogWords разбивает текст на значимые слова-основы.
//
// Кириллица к нижнему регистру, «ё» к «е». Слова короче четырёх букв не нужны —
// они совпадут с половиной каталога. Длинные режутся до основы: «плиты»/«плита»
// и «мембраны»/«мембрана» — одно слово.
func analogWords(text string) []string {
	var out []string
	var sb strings.Builder
	flush := func() {
		w := strings.ToLower(sb.String())
		sb.Reset()
		w = strings.ReplaceAll(w, "ё", "е")
		runes := []rune(w)
		if len(runes) < 4 || analogStopWords[w] {
			return
		}
		out = append(out, analogStem(w))
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

// analogStem — основа слова.
//
// Короткие слова теряют частое окончание («плиты» → «плит»), длинные режутся до
// семи букв («теплоизоляция» и «теплоизоляционные» → «теплоиз»). Словаря нет
// намеренно: он разошёлся бы с живыми названиями карточек, а грубая основа
// здесь и нужна — сравнить класс продукта.
func analogStem(w string) string {
	const stemLen = 7
	runes := []rune(w)
	if len(runes) <= 8 {
		stem := strings.TrimRight(string(runes), "аыиоеяю")
		if len([]rune(stem)) < 4 {
			return string(runes)
		}
		return stem
	}
	return string(runes[:stemLen])
}

// analogClasses — слова, которые задают КЛАСС продукта: по ним и сравниваются
// карточки. Это не словарь продуктов, а короткий список «что это за товар».
//
// Список нужен потому, что названия карточек свободные: «ПВХ мембрана
// PLASTFOIL CLASSIC» и «Гидроизоляционная ПВХ-мембрана ECOBASE V-SL» — один
// класс, хотя общих слов у них почти нет.
//
// Порядок важен: первое найденное слово решает класс. Поэтому «активатор» и
// «очиститель» стоят ПЕРЕД «мембран» — у них в названии тоже встречается
// «мембран» («Активатор для ПВХ мембран»), а класс у них другой.
var analogClasses = []string{
	"активатор", "очистит", "праймер", "грунт", "клей", "гермет", "раствор",
	"анкер", "дюбел", "саморез", "шайб", "воронк", "аэратор", "флюгар",
	"мембран", "геомембр", "пленк", "пароиз", "гидроиз", "теплоиз",
	"утеплит", "изоляц", "руберо", "битум", "профлист", "профилир",
	"сайдинг", "сэндвич", "панел", "профиль", "профил", "штукатур", "краск",
	"сетк", "геотекс", "дренаж", "труб", "уголок",
}

// analogWeakClasses — «класс», который на деле ничего не говорит о товаре.
// «Крепёж» есть и у шайбы, и у анкера, и у самореза: по нему аналоги не
// подбираются, иначе шайба окажется аналогом мембраны.
var analogWeakClasses = map[string]bool{
	"креп": true, "крепеж": true, "лист": true,
}

// analogClassOf — класс продукта ТОЛЬКО по названию.
//
// Описание не участвует: там слово класса стоит в общем перечислении, и добавка
// для бетона с «теплоизоляцией» в описании становилась аналогом плиты
// (2026-11-11, живая проверка на боевой базе: Sika в аналогах PIR-плиты).
// Нет класса в названии — карточка не участвует в подборе вовсе: это честнее,
// чем угадывать по описанию.
func analogClassOf(title string) string {
	return analogClassIn(title)
}

// analogClassIn — первое классовое слово в тексте.
func analogClassIn(text string) string {
	words := analogWords(text)
	for _, class := range analogClasses {
		if analogWeakClasses[class] {
			continue
		}
		prefix := class
		if len([]rune(prefix)) > 5 {
			prefix = string([]rune(prefix)[:5])
		}
		for _, w := range words {
			// Слово-основа тоже режется до семи букв — сравниваем по первым
			// пяти: «мембран» и «мембрана» сходятся, «мембрана» и «крепеж» — нет.
			wp := w
			if len([]rune(wp)) > 5 {
				wp = string([]rune(wp)[:5])
			}
			if wp == prefix {
				return class
			}
		}
	}
	return ""
}

// analogStrongClasses — классы, которые задают товар, а не его назначение.
//
// Отдельно от `analogClasses`: тот список — словарь «что это за товар» для
// подбора, а здесь — то, что ОТЛИЧАЕТ наш товар от соседнего. «Кровельная»
// и «теплоизоляционные» стоят в названиях и мембраны, и плиты, поэтому из
// названия нашего продукта они убираются: иначе ФАХМАНН («Кровельная ПВХ-мембрана
// PVC-RP») и Sikaplan не считались бы мембранами (2026-11-12, живая жалоба).
var analogStrongClasses = []string{
	"мембран", "геомембр", "профлист", "профилир", "сэндвич", "панел",
	"пароиз", "гидроиз", "утеплит", "изоляц", "руберо", "битум",
	"плит", "сайдинг", "штукатур", "краск", "сетк", "геотекс", "дренаж", "труб",
	"анкер", "дюбел", "саморез", "шайб", "воронк", "аэратор", "флюгар",
	"активатор", "очистит", "праймер", "грунт", "клей", "гермет", "раствор",
	"уголок", "профил", "профиль",
}

// analogWeakStems — основы применений из analogWeakWords.
//
// Сами слова там неполные вперемешку: «кровель» — основа, «кровля» — слово, и
// сравнение с основой слова по ним не работает («теплоиз»). Проверять надо по
// основе: одно правило на все применения (2026-11-12).
var analogWeakStems = func() map[string]bool {
	out := make(map[string]bool, len(analogWeakWords)+8)
	for word := range analogWeakWords {
		out[analogStem(word)] = true
	}
	return out
}()

// analogCoreOf — ядро названия продукта: без применений и без торговой марки.
//
// «Кровельная ПВХ-мембрана LOGICROOF® V-RP» → «ПВХ-мембрана». По ядру ищется
// замена нашему продукту: в нём остаётся то, что делает товар товаром, и не
// остаётся того, что одинаково у всех («кровельная»).
func analogCoreOf(title string) string {
	words := strings.Fields(normalizeClassName(title))
	// Слова с латиницей и цифрами — марка и индекс модели («LOGICROOF® V-RP»).
	// Марка идёт ПОСЛЕ товара, поэтому выкидываем их только тогда, когда уже
	// есть слово-товар; иначе «PIR-плита» потеряла бы PIR, а она товар.
	first := make([]string, 0, len(words))
	marks := make([]string, 0, len(words))
	for _, w := range words {
		weak := analogWeakStems[analogStem(strings.ToLower(w))]
		if hasLatinOrDigit(w) && !weak && analogClassOf(w) == "" {
			marks = append(marks, w)
			continue
		}
		first = append(first, w)
	}
	// Применения («кровельная», «теплоизоляционные») убираются: они есть в
	// названиях и мембраны, и плиты, и не отличают товар. Односложные обрывки
	// после вырезания знаков («Ф/Ф» → «Ф Ф») — тоже не товар.
	out := make([]string, 0, len(first))
	for _, w := range first {
		if analogWeakStems[analogStem(strings.ToLower(w))] || len([]rune(w)) < 2 {
			continue
		}
		out = append(out, w)
	}
	// Первое слово товара — класс. Его обязательно оставляем, даже если оно
	// похоже на марку; остальные слова — прилагательные и сокращения.
	if len(out) == 0 {
		out = first
	}
	// Марки возвращаем, только если товарного слова нет вовсе («PROF Ф/Ф»).
	if len(out) == 0 && len(marks) > 0 {
		out = marks
	}
	if len(out) == 0 {
		return strings.Join(words, " ")
	}
	return strings.Join(out, " ")
}

// normalizeClassName приводит название к виду для словесного сравнения:
// «ПВХ-мембрана» → «ПВХ мембрана», «®» и прочие знаки — разделители.
func normalizeClassName(title string) string {
	var b strings.Builder
	for _, r := range title {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= 'А' && r <= 'я') || r == 'Ё' || r == 'ё' {
			b.WriteRune(r)
			continue
		}
		b.WriteRune(' ')
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// hasLatinOrDigit — в слове есть латиница или цифра (марка, индекс модели).
func hasLatinOrDigit(w string) bool {
	for _, r := range w {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return true
		}
	}
	return false
}

// analogBrandOfTitle — марка и модель продукта без классовых слов.
//
// Из «Кровельная ПВХ-мембрана LOGICROOF® V-RP» остаётся «LOGICROOF V RP» — по
// нему ищется замена в Базе знаний; класс берётся отдельно (`analogCoreOf`).
func analogBrandOfTitle(title string) string {
	words := strings.Fields(normalizeClassName(title))
	out := make([]string, 0, len(words))
	for _, w := range words {
		if !hasLatinOrDigit(w) {
			continue
		}
		if analogClassOf(w) != "" {
			continue
		}
		out = append(out, w)
	}
	return strings.Join(out, " ")
}

// analogStrongClassOf — класс товара из ядра названия: мембрана, плита, крепёж.
func analogStrongClassOf(title string) string {
	return analogStrongClassIn(analogCoreOf(title))
}

// analogStrongClassIn — первое классовое слово товара в тексте.
//
// Отличается от `analogClassIn` тем, что назначение («теплоизоляционные»)
// классом не считается: у плиты и у мембран это одно и то же слово, и по нему
// они стали бы одним классом.
func analogStrongClassIn(text string) string {
	words := analogWords(text)
	for _, class := range analogStrongClasses {
		prefix := class
		if len([]rune(prefix)) > 5 {
			prefix = string([]rune(prefix)[:5])
		}
		for _, w := range words {
			wp := w
			if len([]rune(wp)) > 5 {
				wp = string([]rune(wp)[:5])
			}
			if wp == prefix {
				return class
			}
		}
	}
	return ""
}

// analogPerBrandLimit — сколько продуктов одного бренда допустимо в подсказке.
// Два: сравнение с парой моделей одного производителя полезнее одной, но
// подсказка не должна состоять из него одного (2026-11-12, живая жалоба: пять
// мест занимали PLASTFOIL, хотя есть Sikaplan и ФАХМАНН).
const analogPerBrandLimit = 2

// competitorBrandOf — бренд по названию карточки конкурента.
//
// Живые названия: «ПВХ мембрана PLASTFOIL® CLASSIC», «Кровельная ПВХ-мембрана
// FACHMANN PVC-RP …», «Sikaplan® G-12 RU», «Sika Roof …», «PIRRO®VentiDuct».
// Марки брендов — латиница, поэтому первое ЛАТИНСКОЕ слово, не являющееся
// обозначением материала или индекса модели, и есть бренд. Чисто русское
// название бренда не несёт — тогда бренд пуст, и ограничение не действует.
func competitorBrandOf(title string) string {
	var word []rune
	for _, r := range title {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			word = append(word, r)
			continue
		}
		if w := brandWord(word); w != "" {
			return w
		}
		word = word[:0]
	}
	return brandWord(word)
}

// brandWord — слово как бренд или пусто.
func brandWord(word []rune) string {
	if len(word) < 3 {
		return ""
	}
	w := string(word)
	if analogClassOf(w) != "" || analogBrandStopWords[strings.ToLower(w)] {
		return ""
	}
	return w
}

// analogBrandStopWords — латинские слова, которые брендом не являются:
// материал и ходовые обозначения исполнений.
var analogBrandStopWords = map[string]bool{
	"pvc": true, "pur": true, "pir": true, "tpo": true, "epdm": true,
	"xps": true, "eps": true, "rp": true, "sr": true, "gl": true, "gr": true,
	"fb": true, "eco": true, "art": true, "lay": true, "classic": true,
	"polar": true, "industry": true, "cover": true, "geo": true, "wp": true,
	"hl": true, "rus": true, "mm": true,
}

// diversifyByBrand оставляет в подсказке не больше `perBrand` продуктов одного
// бренда, сохраняя порядок. Карточки без бренда ограничению не подлежат — их
// иначе нельзя было бы отличить друг от друга.
func diversifyByBrand(items []SystemCard, perBrand int) []SystemCard {
	if perBrand <= 0 {
		return items
	}
	counts := make(map[string]int, len(items))
	out := make([]SystemCard, 0, len(items))
	for _, card := range items {
		brand := competitorBrandOf(card.Title)
		if brand != "" && counts[brand] >= perBrand {
			continue
		}
		counts[brand]++
		out = append(out, card)
	}
	return out
}

// analogScore — считает вес совпадения.
//
// Класс продукта — обязательное условие (см. findAnalogues). Вес добавляет
// точность: совпадение значимых слов названия (3) и описания (1).
func analogScore(words []string, title, description string) int {
	titleWords := map[string]bool{}
	for _, w := range analogWords(title) {
		titleWords[w] = true
	}
	descWords := map[string]bool{}
	for _, w := range analogWords(description) {
		descWords[w] = true
	}
	score := 0
	for _, w := range words {
		if analogWeakWords[w] {
			continue
		}
		if titleWords[w] {
			score += 3
			continue
		}
		if descWords[w] {
			score++
		}
	}
	return score
}

// findAnalogues подбирает карточки БЗ, похожие на заданную.
//
// Аналог — карточка ТОГО ЖЕ КЛАССА продукта (мембрана к мембране, плита к
// плите). Раньше сравнивались просто слова, и к ПВХ-мембране подбирались шайба
// и активатор, а PLASTFOIL не попадал вовсе.
//
// Порядок — по весу (совпадение названия важнее описания), при равенстве — по
// алфавиту: ответ не должен «прыгать» между запусками.
func findAnalogues(source SystemCard, catalog []SystemCard, limit int) []SystemCard {
	sourceText := source.Title + " " + source.Description
	words := analogWords(sourceText)
	sourceClass := analogClassOf(source.Title)
	if len(words) == 0 {
		return nil
	}
	candidates := make([]analogCandidate, 0, len(catalog))
	for _, card := range catalog {
		if card.NoteID == source.NoteID {
			continue
		}
		cardText := card.Description + " " + strings.Join(card.Composition, " ")
		class := analogClassOf(card.Title)
		// Класс обязателен и должен совпасть. Нераспознанный класс у источника —
		// повод не подбирать вовсе: подставлять «похожее по словам» хуже, чем
		// честно сказать, что аналогов нет.
		if sourceClass == "" || class != sourceClass {
			continue
		}
		score := analogScore(words, card.Title, cardText)
		candidates = append(candidates, analogCandidate{card: card, score: score})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].card.Title < candidates[j].card.Title
	})
	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}
	out := make([]SystemCard, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, c.card)
	}
	return out
}

// handleBotCombatAnalogues — аналоги-конкуренты к нашему продукту.
//
// GET /api/offers/bot/combat/analogues?noteId=… — `noteId` это НАШ продукт
// (обычно уже выбранный в листке). Ответ — продукты конкурентов того же класса.
//
// Старое поведение (аналог к конкуренту) убрано осознанно: в листке с нашим
// продуктом и без конкурентов оно не работало вовсе, а менеджер ждёт подбора
// именно со стороны того продукта, который защищает.
func (s *Server) handleBotCombatAnalogues(w http.ResponseWriter, r *http.Request) {
	noteID := parseIDParam(r.URL.Query().Get("noteId"))
	if noteID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "не задан продукт"})
		return
	}
	source, ok := s.analogSourceCard(r, noteID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error": "продукт не найден в кеше — перечитайте раздел «Конкуренты» или «Системы»",
		})
		return
	}
	found := s.findAnaloguesForSheet(r.Context(), source)
	items := combatItems(found)
	log.Printf("боевой листок: аналогов-конкурентов для «%s» — %d", source.Title, len(items))
	writeJSON(w, http.StatusOK, map[string]any{
		"source": map[string]any{"noteId": source.NoteID, "title": source.Title},
		"cards":  items,
	})
}

// analogSecondaryClassOf — класс ВТОРОСТЕПЕННОГО товара: активатор, очиститель,
// крепёж, воронка, аэратор. Такой товар пишется «Активатор для ПВХ мембран», и
// поиск по «мембрана кровельная» честно его находит — но аналогом мембраны он
// не является (2026-11-12, живой прогон: активаторы и очистители стояли в
// подсказке пятой и шестой позицией).
var analogSecondaryClasses = []string{
	"активатор", "очистит", "праймер", "грунт", "клей", "гермет", "раствор",
	"шайб", "воронк", "аэратор", "флюгар", "фартук", "рейк", "профил", "профиль",
	"анкер", "дюбел", "саморез", "лент", "скотч", "плёнк", "пленк",
	"кронштейн", "крепёж", "крепеж", "систем", "ламел", "скло", "элемент",
	"чистящ", "cleaner", "wash", "промывк", "разбавител",
	// Монтажные пены («SIKA BOOM Foam Fix»): в названии нет ни «пены», ни
	// «герметика», и без этих слов пена попадала в аналоги плиты (2026-11-12).
	"boom", "foam", "пена", "пену", "пеногерметик",
}

// analogSecondaryClassOf — класс второстепенного товара по названию.
//
// Не через analogWords: тот выбрасывает короткие слова и стоп-слова, а «система»
// у него — стоп-слово. Здесь проверяется прямое вхождение основы в нормализованное
// название: «Система кровли …», «активатор для ПВХ мембран».
func analogSecondaryClassOf(title string) string {
	normalized := strings.ToLower(strings.ReplaceAll(normalizeClassName(title), "ё", "е"))
	for _, class := range analogSecondaryClasses {
		if strings.Contains(normalized, class) {
			return class
		}
	}
	return ""
}

// isAnalogCandidate — карточка конкурента может быть заменой нашему продукту.
//
// Правила выведены из живых прогонов 2026-11-12 на боевой базе:
//  1. другое название, чем у нашего (нет «свой аналог»);
//  2. не второстепенный товар: активатор, очиститель, крепёж, воронка, система
//     — не замена мембране, даже если в названии стоит слово «мембрана»;
//  3. собственный класс карточки не должен ПРОТИВОРЕЧИТЬ нашему. У «SIKA BOOM»
//     и «SikaEmaco» класс в названии не виден — такие пропускаем, их описание
//     проверяет поиск; но карточка, которая сама называет себя штукатуркой или
//     герметиком, плитой не станет.
//  4. класс совпадает по ядру названия, когда класс нашего продукта известен.
//     Sikaplan® класс в названии не несёт — её пропускаем: продукт описан
//     мембраной в тексте карточки, а это проверяется отдельно, поиском.
func isAnalogCandidate(source SystemCard, card SystemCard, class string) bool {
	if card.NoteID == source.NoteID || strings.TrimSpace(card.Title) == "" {
		return false
	}
	title := strings.TrimSpace(card.Title)
	if analogSecondaryClassOf(title) != "" {
		return false
	}
	if class == "" {
		return true
	}
	// Собственный класс карточки — по «сырому» списку классов, включая
	// второстепенные: «грунтовка» и «герметик» — точно не плита.
	if own := analogClassIn(title); own != "" && !sameStrongClass(own, class) {
		return false
	}
	if got := analogStrongClassOf(title); got != "" && got != class {
		return false
	}
	return true
}

// sameStrongClass — классы совместимы: «плит» и «утеплит» — родня, «гермет» и
// «плит» — нет.
func sameStrongClass(a, b string) bool {
	if a == b {
		return true
	}
	related := map[string][]string{
		"плит":     {"утеплит", "изоляц", "теплоиз"},
		"мембран":  {"геомембр", "гидроиз", "пароиз", "плёнк", "пленк"},
		"профлист": {"профилир"},
	}
	for _, group := range related[a] {
		if group == b {
			return true
		}
	}
	for _, group := range related[b] {
		if group == a {
			return true
		}
	}
	return false
}

// findAnaloguesForSheet — подбор аналогов к нашему продукту (2026-11-12).
//
// Сначала ищем в Базе знаний тем же полнотекстовым поиском, что и выбор стороны
// (его механизм уже показал себя: на живой базе «пвх мембран*» с тегом
// `competitor` даёт PLASTFOIL, Sikaplan и ФАХМАНН вместе). Порядок запросов:
//
//  1. марка нашего продукта без классовых слов («LOGICROOF V RP»): у продукта
//     одной линейки аналоги — соседние марки того же бренда реже, а чужие чаще;
//  2. ядро названия («ПВХ мембрана»): так находятся все мембраны всех брендов.
//
// Результаты склеиваются по заметке, затем отсеиваются карточки не того класса
// (модель видела «ПВХ мембрана» и в очистителе) и ограничивается число
// продуктов одного бренда. Сбой поиска не гасит подбор: работает прежняя цепочка
// по кешу (`findAnaloguesSmart`).
func (s *Server) findAnaloguesForSheet(ctx context.Context, source SystemCard) []SystemCard {
	searchWords := analogSearchQueries(source)
	if len(searchWords) > 0 {
		if cards, ok := s.searchAnaloguesInKB(ctx, source, searchWords); ok && len(cards) > 0 {
			return cards
		}
	}
	competitors, _, err := s.loadCachedCompetitors(ctx)
	if err != nil {
		return nil
	}
	return s.findAnaloguesSmart(ctx, source, competitors, combatAnalogLimit)
}

// analogSearchQueries — запросы к Базе знаний, от ОБЩЕГО к частному.
//
// Порядок взят с живого прогона (2026-11-12): запрос по марке («LOGICROOF V RP»)
// на боевой базе даёт 0 строк — поиск идёт по русской морфологии, а латинские
// индексы моделей там не бьются. Запрос по ядру («ПВХ мембрана») отдаёт одни
// PLASTFOIL, потому что первые 9 мест занимает один бренд. А вот
// «мембрана кровельная» даёт ФАХМАНН и PVC-RP, и только она подтягивает другие
// бренды. Поэтому:
//
//  1. ОБЩИЙ запрос класса с применением: «мембрана кровельная», «плиты
//     теплоизоляционные» — он и находит замещающие продукты всех брендов;
//  2. частный запрос ядра с классом: «ПВХ мембрана», «плиты»;
//  3. марка — последней и только как дополнение: у самого товара могут найтись
//     соседние исполнения того же производителя.
//
// Запрос идёт в родительном/винительном виде, как его находит веб-поиск БЗ:
// «мембрана кровельная», а не «мембрана кровель» — стем не слово, и поиск по
// префиксам его не узнаёт.
func analogSearchQueries(source SystemCard) []string {
	out := make([]string, 0, 3)
	class := analogStrongClassOf(source.Title)
	classWord := analogStrongClassText(class)
	core := analogCoreOf(source.Title)

	// 1. Класс + назначение: «мембрана кровельная», «плиты воздуховод».
	if classWord != "" {
		for _, use := range analogPurposeWords(source.Title + " " + analogDescriptionHead(source.Description)) {
			out = append(out, classWord+" "+use)
		}
		out = append(out, classWord)
	}
	// 2. Ядро с классом: «ПВХ мембрана» (вхождение товара вроде ПВХ остаётся).
	if phrase := analogSearchPhrase(strings.Fields(normalizeClassName(core)), classWord); phrase != "" {
		out = appendUnique(out, phrase)
	}
	// 3. Марка модели — дополнением.
	if brand := analogBrandOfTitle(source.Title); brand != "" {
		out = appendUnique(out, brand)
	}
	return out
}

// analogPurposeWords — назначение из названия: «кровельная», «воздуховод».
//
// Класс товара тут ни при чём: он уже известен. Назначение ограничивает область,
// и вместе с классом даёт точный запрос («мембрана кровельная», «плиты
// воздуховод»), которым боевой поиск находит других производителей вместо
// девяти PLASTFOIL подряд и вместо сэндвич-панелей вместо плит для воздуховодов
// (2026-11-12, живые прогоны).
//
// Берутся СЛОВОФОРМЫ, а не основы: поиск по префиксам понимает «кровельн» и не
// понимает «кровел».
func analogPurposeWords(text string) []string {
	out := make([]string, 0, 2)
	for _, w := range strings.Fields(normalizeClassName(text)) {
		lower := strings.ReplaceAll(strings.ToLower(w), "ё", "е")
		if lower == "" {
			continue
		}
		// Прилагательное применения: «кровельная», «фасадная», «плоская».
		if analogWeakWords[analogStem(lower)] &&
			(strings.HasSuffix(lower, "ая") || strings.HasSuffix(lower, "яя") ||
				strings.HasSuffix(lower, "ый") || strings.HasSuffix(lower, "ий")) {
			out = append(out, lower)
			continue
		}
		// Существительное назначения: «воздуховод», «воздуховодов».
		if lower == "воздуховод" || strings.HasPrefix(lower, "воздуховод") ||
			strings.HasPrefix(lower, "вентиляц") {
			out = appendUnique(out, "воздуховод")
		}
	}
	return out
}

// analogDescriptionHead — начало описания продукта.
//
// Назначение не всегда в названии: «Плиты теплоизоляционные LOGICPIR® VENT» —
// это плиты ДЛЯ КОРОБОВ ВОЗДУХОВОДОВ, и это сказано в описании («для
// изготовления коробов воздуховодов систем вентиляции»). Без назначения в
// запросе «плиты» боевой поиск отдаёт сэндвич-панели вместо плит для
// воздуховодов (2026-11-12, живой прогон).
func analogDescriptionHead(description string) string {
	trimmed := strings.TrimSpace(description)
	runes := []rune(trimmed)
	if len(runes) > 400 {
		return string(runes[:400])
	}
	return trimmed
}

// appendUnique добавляет запрос, если такого ещё нет.
func appendUnique(list []string, item string) []string {
	item = strings.TrimSpace(item)
	if item == "" {
		return list
	}
	for _, existing := range list {
		if strings.EqualFold(existing, item) {
			return list
		}
	}
	return append(list, item)
}

// analogClassWords — словоформы классов товара для запроса к Базе знаний.
//
// Таблица, а не `switch`: каждое новое слово — строка, а не ветка, и функция
// остаётся плоской (crapper: разветвлённый switch давал CC 18 при CRAP 211).
// Класс, которого здесь нет, уходит в запрос как есть: поиск по префиксам
// стерпит основу.
var analogClassWords = map[string]string{
	"мембран":  "мембрана",
	"плит":     "плиты",
	"панел":    "панели",
	"профлист": "профлист",
	"профилир": "профлист",
	"теплоиз":  "утеплитель",
	"утеплит":  "утеплитель",
	"изоляц":   "изоляция",
	"пароиз":   "пароизоляция",
	"гидроиз":  "гидроизоляция",
	"дренаж":   "дренаж",
	"труб":     "труба",
	"шайб":     "шайба",
	"воронк":   "воронка",
	"анкер":    "анкер",
	"дюбел":    "дюбель",
	"саморез":  "саморез",
	"клей":     "клей",
	"гермет":   "герметик",
	"сэндвич":  "сэндвич-панели",
	"битум":    "битум",
	"руберо":   "рубероид",
	"сайдинг":  "сайдинг",
	"геотекс":  "геотекстиль",
	"геомембр": "геомембрана",
	"штукатур": "штукатурка",
	"краск":    "краска",
}

// analogStrongClassText — человеческое слово класса товара со склонением под
// запрос: «мембран» → «мембрана», «плит» → «плиты».
func analogStrongClassText(class string) string {
	if word, ok := analogClassWords[class]; ok {
		return word
	}
	return class
}

// analogSearchPhrase — строка запроса из основ и класса: «ПВХ мембрана».
//
// Основаны уже основами, класс — слово: поиск kb-service режет запрос на слова
// сам, и ему достаточно «ПВХ мембрана», чтобы через морфологию найти «ПВХ
// мембрана» и «Sikaplan®» (у той в названии слова «мембрана» нет — её находит
// поиск по тексту карточки: там продукт описан как мембрана).
func analogSearchPhrase(stems []string, classWord string) string {
	keep := make([]string, 0, len(stems)+1)
	for _, w := range stems {
		if analogStrongClassOf(w) != "" || analogBrandStopWords[strings.ToLower(w)] {
			continue
		}
		keep = append(keep, w)
	}
	if classWord != "" {
		keep = append(keep, classWord)
	}
	return strings.Join(keep, " ")
}

// searchAnaloguesInKB — поиск аналогов в Базе знаний по тегу `competitor`.
//
// Второе значение — «поиск состоялся»: пустая выдача — это ответ, сбой — повод
// для запасного пути. Карточка читается целиком только у тех строк, что прошли
// фильтр класса: описание и характеристики в строке списка не приходят.
func (s *Server) searchAnaloguesInKB(ctx context.Context, source SystemCard, queries []string) ([]SystemCard, bool) {
	if s.kb == nil || !s.kb.configured() {
		return nil, false
	}
	tagID, err := s.kb.TagID(ctx, competitorTagName)
	if err != nil {
		return nil, false
	}
	class := analogStrongClassOf(source.Title)
	items := s.collectAnalogues(ctx, source, queries, tagID, class)
	items = diversifyByBrand(items, analogPerBrandLimit)
	if len(items) > combatAnalogLimit {
		items = items[:combatAnalogLimit]
	}
	return items, true
}

// collectAnalogues — кандидаты по всем запросам подряд, без повторов и без
// чужих классов.
//
// Вынесено из `searchAnaloguesInKB`: поиск по тегу, сбор по запросам и
// проверка класса — три разных дела в одной функции давали CRAP 210.
func (s *Server) collectAnalogues(ctx context.Context, source SystemCard, queries []string,
	tagID int64, class string) []SystemCard {
	seen := make(map[int64]bool)
	items := make([]SystemCard, 0, combatAnalogLimit*2)
	for _, query := range queries {
		notes, err := s.kb.Search(ctx, query, tagID, analogSearchLimit)
		if err != nil {
			continue
		}
		for _, summary := range notes {
			if summary.ID <= 0 || seen[summary.ID] || isCompetitorDisclaimer(summary.Title) {
				continue
			}
			card := SystemCard{NoteID: summary.ID, Title: summary.Title, Path: summary.Path}
			if !isAnalogCandidate(source, card, class) {
				continue
			}
			seen[summary.ID] = true
			items = append(items, card)
		}
		if len(items) >= analogSearchLimit {
			break
		}
	}
	return items
}

// analogSourceCard находит карточку, к которой ищем аналоги: свой продукт —
// в каталоге материалов или в системах, конкурент — в кеше конкурентов.
func (s *Server) analogSourceCard(r *http.Request, noteID int64) (SystemCard, bool) {
	// Описание нужно целиком — из кеша карточек, а не из списка сторон: там
	// только название и код.
	if cards, err := s.loadCachedSystemCards(r.Context()); err == nil {
		for _, card := range cards {
			if card.NoteID == noteID {
				return card, true
			}
		}
	}
	if cards, _, err := s.loadCachedCompetitors(r.Context()); err == nil {
		for _, card := range cards {
			if card.NoteID == noteID {
				return card, true
			}
		}
	}
	return SystemCard{}, false
}

// analogLLMTimeout — сколько ждать модель в подборе аналогов.
//
// Минута, а не двадцать секунд: провайдер отвечает за 20-40 секунд на разборе
// длинного журнала, и на двадцати модель не успевала — подбор молча уходил в
// откат на класс (2026-11-11, живой лог: context deadline exceeded). Дольше
// минуты менеджер уже не ждёт список, поэтому дальше — откат.
const analogLLMTimeout = 60 * time.Second

// combatAnalogLimit — сколько аналогов подсказывать.
//
// Восемь (2026-11-12, живой прогон на боевой базе): при десяти в подсказку
// хвостом попадали чужие товары — «SIKA BOOM» (монтажная пена) после плит.
// Хороших аналогов классового товара на базе 4-8 на бренд-группу, и восемь мест
// показывают лучшие, не размывая их мусором. Номера стоят по четыре в ряд —
// восемь это два ряда, помещаются на экран.
const combatAnalogLimit = 8

// analogSearchLimit — сколько кандидатов берём из поиска Базы знаний, прежде
// чем ограничить разнообразие брендов. Полсотни — потолок страницы сервиса.
const analogSearchLimit = 50

// parseIDParam разбирает числовой параметр запроса; неверный — 0.
func parseIDParam(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	var n int64
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int64(r-'0')
	}
	return n
}
