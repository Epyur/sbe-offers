package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
)

// Подбор аналогов моделью (2026-11-11, решение пользователя).
//
// Словесный подбор по классу продукта на живых данных даёт правильные ответы,
// но он слепой: «мембрана» и «геомембрана» для него одно, а «PIR-плита» и
// «плита минеральная» — тоже. Модель различает это по смыслу карточки.
//
// Подбор вспомогательный: менеджер может собрать общий листок и потом уточнить
// его аналогами, а может и сразу выбрать конкурентов руками. Поэтому сбой модели
// НЕ ошибка для человека — работает откат на класс-подбор, который уже проверен
// на боевых данных.
//
// Это относится только к подбору аналогов. Само сравнение (combat_compare.go)
// живёт своей жизнью: там модель пишет текст документа.

// analogLLMSystem — роль модели в подборе.
//
// Ответ строго JSON-массивом номеров: разбирать текст модели регуляркой значило
// бы ломаться на каждой смене её манеры говорить. Номера, а не названия —
// названия модель перепишет, и сопоставить их с карточками будет нечем.
const analogLLMSystem = `Ты помощник отдела продаж. Тебе дают ОДИН наш продукт и список
продуктов конкурентов с номерами. Выбери те продукты конкурентов, которые
являются аналогами нашего: тот же класс товара, та же задача, взаимозаменяемы
для заказчика.

Правила:
- Замена — по существу, а не по общему слову. Комплектующие (крепёж, воронки,
  очистители, активаторы, клеи, профили, шайбы) НЕ аналоги готового продукта.
- Если товар конкурента — часть системы, а не тот же продукт, это не аналог.
- Не выбирай то, что просто «тоже для кровли»: важно, что это тот же товар.
- Выбирай до 5 самых близких. Если аналогов нет — верни пустой массив.

Ответ — ТОЛЬКО JSON-массив номеров без пояснений, например: [3, 7, 12]
Если аналогов нет: []`

// analogLLMLimit — сколько аналогов модель может выбрать.
const analogLLMLimit = 5

// analogCandidateLimit — сколько карточек конкурентов отдавать модели за раз.
//
// 60 — предел осмысленного промпта: в карточке конкурента название и короткое
// описание, 60 × ~250 знаков ≈ 15 КБ. Больше — медленнее и дороже без выигрыша
// в точности, меньше — рискуем не показать модели настоящий аналог.
const analogCandidateLimit = 60

// analogLLMCandidate — строка списка для модели.
type analogLLMCandidate struct {
	Index int    `json:"index"`
	Title string `json:"title"`
	Info  string `json:"info"`
}

// analogCandidate — кандидат для модели вместе с самой карточкой.
//
// Карточка хранится рядом, а не ищется потом по названию: название карточки
// конкурента не уникально (одноимённые есть), и поиск по нему мог бы вернуть
// чужую карточку.
type analogLLMItem struct {
	Index  int    `json:"index"`
	Title  string `json:"title"`
	Info   string `json:"info"`
	origin SystemCard
}

// analogCandidatesForLLM готовит список кандидатов.
//
// Порядок — словесный подбор (класс, затем вес): самые похожие идут первыми, и
// если кандидатов больше предела, обрезка снимает хвост маловероятных. Так
// модель видит осмысленный срез, а не случайные 60 карточек.
func analogCandidatesForLLM(source SystemCard, catalog []SystemCard) []analogLLMItem {
	ordered := findAnalogues(source, catalog, len(catalog))
	if len(ordered) == 0 {
		// Класс не распознан — отдаём catalogue целиком по алфавиту: модель
		// справится там, где словарь не уверен.
		ordered = append(ordered, catalog...)
		sort.SliceStable(ordered, func(i, j int) bool {
			return ordered[i].Title < ordered[j].Title
		})
	}
	if len(ordered) > analogCandidateLimit {
		ordered = ordered[:analogCandidateLimit]
	}
	out := make([]analogLLMItem, 0, len(ordered))
	for i, card := range ordered {
		info := trimForPrompt(card.Description, 220)
		out = append(out, analogLLMItem{Index: i + 1, Title: card.Title, Info: info, origin: card})
	}
	return out
}

// buildAnalogPrompt — запрос модели: наш продукт и пронумерованные кандидаты.
func buildAnalogPrompt(source SystemCard, candidates []analogLLMItem) string {
	var b strings.Builder
	b.WriteString("НАШ ПРОДУКТ:\n")
	b.WriteString(source.Title)
	b.WriteString("\n")
	if desc := trimForPrompt(source.Description, 600); desc != "" {
		b.WriteString(desc)
		b.WriteString("\n")
	}
	b.WriteString("\nПРОДУКТЫ КОНКУРЕНТОВ:\n")
	for _, c := range candidates {
		b.WriteString(fmt.Sprintf("%d. %s", c.Index, c.Title))
		if c.Info != "" {
			b.WriteString(" — ")
			b.WriteString(c.Info)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// parseAnalogAnswer разбирает ответ модели: номера из JSON-массива.
//
// Всё, что не похоже на массив номеров, — пустой результат. Модель может
// обернуть ответ в ```json … ``` или добавить фразу: снимаем обрамление и берём
// первую скобку — так ответ читается при любом оформлении, а мусор (отрицание
// «аналогов нет») не превращается в выбор.
func parseAnalogAnswer(raw string, limit int) []int {
	text := strings.TrimSpace(raw)
	if at := strings.Index(text, "["); at >= 0 {
		if end := strings.LastIndex(text, "]"); end > at {
			text = text[at : end+1]
		}
	}
	var nums []int
	if err := json.Unmarshal([]byte(text), &nums); err != nil {
		return nil
	}
	seen := map[int]bool{}
	out := make([]int, 0, len(nums))
	for _, n := range nums {
		if n <= 0 || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// pickAnaloguesWithLLM — подбор моделью. Пустой срез и nil-ошибка — «модель не
// выбрала ничего»; ошибка — «подбор не удался, нужен откат».
func pickAnaloguesWithLLM(ctx context.Context, source SystemCard, catalog []SystemCard) ([]SystemCard, error) {
	candidates := analogCandidatesForLLM(source, catalog)
	if len(candidates) == 0 {
		return nil, nil
	}
	answer, err := Complete(ctx, analogLLMSystem, buildAnalogPrompt(source, candidates), 0)
	if err != nil {
		return nil, err
	}
	nums := parseAnalogAnswer(answer, analogLLMLimit)
	chosen := make([]SystemCard, 0, len(nums))
	for _, n := range nums {
		if n < 1 || n > len(candidates) {
			continue
		}
		// Номер → та самая карточка, что уходила модели (в кандидате она рядом).
		chosen = append(chosen, candidates[n-1].origin)
	}
	return chosen, nil
}

// findAnaloguesSmart — подбор с моделью и откатом.
//
// Модель — с ограничением по времени: подбор вспомогательный, и ждать её
// минутами менеджер не должен. Не ответила — класс-подбор, который уже показал
// себя на боевой базе (PLASTFOIL к ПВХ-мембране).
func (s *Server) findAnaloguesSmart(ctx context.Context, source SystemCard,
	catalog []SystemCard, limit int) []SystemCard {
	llmCtx, cancel := context.WithTimeout(ctx, analogLLMTimeout)
	defer cancel()

	// 1. Журнал конкурентов — лучший источник (2026-11-11, идея пользователя):
	// в нём уже есть группировка по разделам каталога и ссылки на целевые
	// заметки. Модель выбирает строки разделов, а не угадывает по словам.
	if viaJournal, err := s.findAnaloguesViaJournal(llmCtx, source, catalog); err == nil && len(viaJournal) > 0 {
		if limit > 0 && len(viaJournal) > limit {
			viaJournal = viaJournal[:limit]
		}
		return viaJournal
	} else if err != nil {
		log.Printf("боевой листок: подбор по журналу не удался (%v)", err)
	}

	// 2. Модель по карточкам — когда журнала нет или он ничего не дал.
	found, err := pickAnaloguesWithLLM(llmCtx, source, catalog)
	if err != nil {
		log.Printf("боевой листок: подбор аналогов моделью не удался (%v) — откат на класс", err)
		return findAnalogues(source, catalog, limit)
	}
	if len(found) == 0 {
		// Модель сказала «аналогов нет». Это ответ, а не сбой: подставлять
		// «похожие по словам» против её решения не нужно.
		return nil
	}
	if limit > 0 && len(found) > limit {
		found = found[:limit]
	}
	return found
}
