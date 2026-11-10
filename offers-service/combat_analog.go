package main

import (
	"log"
	"net/http"
	"sort"
	"strings"
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

// analogStopWords — общие слова, которые есть почти в любом описании.
var analogStopWords = map[string]bool{
	"для": true, "или": true, "как": true, "что": true, "при": true, "это": true,
	"все": true, "типа": true, "также": true, "более": true, "менее": true,
	"него": true, "нее": true, "них": true, "этом": true, "котор": true,
	"являе": true, "испол": true, "приме": true, "материал": true,
	"материалы": true, "продукт": true, "продукты": true,
	"система": true, "системы": true, "данные": true, "может": true,
	"могут": true, "только": true, "благодар": true,
	"обеспеч": true, "позволя": true, "пример": true, "техничес": true,
	"различн": true, "различных": true, "разного": true, "широко": true,
}

// analogWords разбивает текст на значимые слова.
//
// Кириллица к нижнему регистру, «ё» к «е»: менеджер и карточки пишут по-разному.
// Слова короче четырёх букв не нужны — они совпадут с половиной каталога.
// Совсем длинные слова режем до основы: в русском окончания меняются, а
// «мембран» и «мембранА» — одно слово. Режем осторожно (до 8 букв), чтобы не
// склеить разные корни.
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

// analogStem — основа слова: первая часть длинного слова.
//
// Простейшая, без словаря: «теплоизоляционные» и «теплоизоляция» дают
// «теплоизо», «плиты» и «плита» — «плиты». Этого достаточно, чтобы совпал класс
// продукта, и не настолько много, чтобы склеить разные слова.
func analogStem(w string) string {
	const stemLen = 7
	runes := []rune(w)
	if len(runes) <= 8 {
		// Короткие слова оставляем целиком, но снимаем частые окончания:
		// «плиты»/«плита» — это одно и то же.
		return strings.TrimRight(w, "аыиоеяю")
	}
	return string(runes[:stemLen])
}

// analogScore считает вес совпадения.
//
// Класс продукта определяется НАЗВАНИЕМ: если совпало слово из названия — это
// уже аналог (вес 3). Совпадение только в описании даёт вес 1 и само по себе
// аналогом не делает: описания длинные, и в них встречается всё подряд.
func analogScore(words []string, title, description string) (score int, classHit bool) {
	titleWords := map[string]bool{}
	for _, w := range analogWords(title) {
		titleWords[w] = true
	}
	descWords := map[string]bool{}
	for _, w := range analogWords(description) {
		descWords[w] = true
	}
	for _, w := range words {
		if titleWords[w] {
			score += 3
			classHit = true
			continue
		}
		if descWords[w] {
			score++
		}
	}
	return score, classHit
}

// findAnalogues подбирает карточки БЗ, похожие на заданную.
//
// `source` — продукт, к которому ищем аналоги (наш продукт, когда подбираем
// конкурентов). `catalog` — из какого набора выбирать.
//
// Правило отбора жёсткое: аналогом считается только карточка, у которой
// совпало слово из НАЗВАНИЯ источника (classHit). Иначе к «PIR-плите»
// попадал «МОСТОФЛЕКС» — у них общее лишь слово «применения» в описании.
//
// Порядок — по весу, при равенстве — по алфавиту: ответ не должен «прыгать»
// между запусками.
func findAnalogues(source SystemCard, catalog []SystemCard, limit int) []SystemCard {
	words := analogWords(source.Title + " " + source.Description)
	if len(words) == 0 {
		return nil
	}
	candidates := make([]analogCandidate, 0, len(catalog))
	for _, card := range catalog {
		if card.NoteID == source.NoteID {
			continue
		}
		score, classHit := analogScore(words, card.Title,
			card.Description+" "+strings.Join(card.Composition, " "))
		if !classHit || score <= 0 {
			continue
		}
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
	found := findAnalogues(source, competitors, combatAnalogLimit)
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

// combatAnalogLimit — сколько аналогов подсказывать. Три: больше — это уже
// список, и менеджер снова выбирает из него руками.
const combatAnalogLimit = 3

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
