package main

import (
	"log"
	"net/http"
	"sort"
	"strings"
	"unicode"
)

// «Подобрать аналог» (2026-11-11, просьба пользователя).
//
// Менеджер выбрал продукт конкурента, и ему нужен НАШ аналог, чтобы сравнение
// было по делу, а не по всему каталогу. Подбор идёт по Базе знаний — по
// названию и описанию карточек нашего продукта (материалы каталога и системы).
//
// Почему совпадение слов, а не модель: подбор должен быть предсказуемым и
// мгновенным, а промах виден сразу — менеджер видит найденное название и сам
// решает, брать его или выбрать руками. Модель здесь давала бы ответ, который
// нельзя ни проверить, ни объяснить.
//
// Вес: совпадение в НАЗВАНИИ весит больше, чем в описании. Слово «мембрана» в
// названии — это почти наверняка тот же класс продукта; в описании оно может
// стоять в общем перечислении.

// analogCandidate — наш продукт и его вес совпадения.
type analogCandidate struct {
	card  SystemCard
	score int
}

// analogWords разбивает текст на значимые слова.
//
// Служебные слова и очень короткие обрывки не нужны: «для», «или», «тип»
// совпадут с половиной каталога. Кириллица приводится к нижнему регистру, «ё»
// к «е» — менеджер пишет по-разному, и это одно слово.
func analogWords(text string) []string {
	var out []string
	var sb strings.Builder
	flush := func() {
		w := strings.ToLower(sb.String())
		sb.Reset()
		w = strings.ReplaceAll(w, "ё", "е")
		if len([]rune(w)) < 4 || analogStopWords[w] {
			return
		}
		out = append(out, w)
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

// analogStopWords — слова, которые есть почти в любой карточке и в русском
// тексте вообще.
var analogStopWords = map[string]bool{
	"для": true, "или": true, "как": true, "что": true, "при": true, "это": true,
	"все": true, "типа": true, "тип": true, "также": true, "более": true,
	"менее": true, "него": true, "нее": true, "них": true, "этом": true,
	"котор": true, "являе": true, "испол": true, "приме": true, "материал": true,
	"продукт": true, "система": true, "систем": true, "данные": true,
	"может": true, "могут": true, "только": true,
}

// analogScore считает вес совпадения названий карточки с запросом.
//
// Название весит тройку, описание — единицу: карточки конкурентов коротко и
// точно называют класс продукта, и это лучший сигнал. Длинный текст описания
// не должен перевешивать точное совпадение названия.
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
		switch {
		case titleWords[w]:
			score += 3
		case descWords[w]:
			score++
		}
	}
	// Совпадений нет — продукт не аналог: нулевой вес не должен попадать в
	// список только потому, что он есть в каталоге.
	return score
}

// findAnalogues подбирает наши аналоги для продукта конкурента.
//
// `catalog` — карточки нашего продукта (материалы и системы). Отбор — по
// пересечению значимых слов названия и описания; порядок — по весу, при равном
// весе — по алфавиту, чтобы ответ не «прыгал» между запусками.
//
// Пустой результат — законный: аналога может не быть, и тогда бот честно об
// этом скажет. Подставить «похожий по буквам» продукт хуже, чем не подставить
// ничего.
func findAnalogues(competitor SystemCard, catalog []SystemCard, limit int) []SystemCard {
	words := analogWords(competitor.Title + " " + competitor.Description)
	if len(words) == 0 {
		return nil
	}
	candidates := make([]analogCandidate, 0, len(catalog))
	for _, card := range catalog {
		if card.NoteID == competitor.NoteID {
			continue
		}
		score := analogScore(words, card.Title, card.Description+" "+strings.Join(card.Composition, " "))
		if score <= 0 {
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

// handleBotCombatAnalogues — подбор нашего аналога для продукта конкурента.
//
// GET /api/offers/bot/combat/analogues?noteId=… — возвращает до
// `combatAnalogLimit` самых близких наших продуктов. Это подсказка, а не
// решение: бот показывает названия и даёт добавить выбранный.
func (s *Server) handleBotCombatAnalogues(w http.ResponseWriter, r *http.Request) {
	noteID := parseIDParam(r.URL.Query().Get("noteId"))
	if noteID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "не задан продукт конкурента"})
		return
	}
	cards, _, err := s.loadCachedCompetitors(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"cards": []any{}, "error": err.Error()})
		return
	}
	var competitor SystemCard
	for _, card := range cards {
		if card.NoteID == noteID {
			competitor = card
			break
		}
	}
	if competitor.NoteID == 0 {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error": "продукт конкурента не найден в кеше — перечитайте раздел «Конкуренты»",
		})
		return
	}
	our, err := s.combatOurItems(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"cards": []any{}, "error": err.Error()})
		return
	}
	// Наш продукт в форме карточек нужен для описания — у подсказки оно берётся
	// из кеша систем; материал без карточки всё равно остаётся кандидатом по
	// названию.
	catalog := make([]SystemCard, 0, len(our))
	for _, item := range our {
		catalog = append(catalog, SystemCard{NoteID: item.NoteID, Title: item.Title, SKU: item.SKU})
	}
	found := findAnalogues(competitor, catalog, combatAnalogLimit)
	items := combatItems(found)
	log.Printf("боевой листок: аналогов для «%s» — %d", competitor.Title, len(items))
	writeJSON(w, http.StatusOK, map[string]any{
		"competitor": map[string]any{"noteId": competitor.NoteID, "title": competitor.Title},
		"cards":      items,
	})
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
