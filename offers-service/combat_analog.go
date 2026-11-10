package main

import (
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

// analogScore считает вес совпадения.
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
	competitors, _, err := s.loadCachedCompetitors(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"cards": []any{}, "error": err.Error()})
		return
	}
	found := s.findAnaloguesSmart(r.Context(), source, competitors, combatAnalogLimit)
	items := combatItems(found)
	log.Printf("боевой листок: аналогов-конкурентов для «%s» — %d", source.Title, len(items))
	writeJSON(w, http.StatusOK, map[string]any{
		"source": map[string]any{"noteId": source.NoteID, "title": source.Title},
		"cards":  items,
	})
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
// Пять, а не три (2026-11-11, живая проверка): у PIR-плиты в журнале нашлось
// пять аналогов, а три места занимал один — карточки разных конкурентов про
// один товар. Пять помещается на экран номерами и даёт выбор без листания.
const combatAnalogLimit = 5

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
