package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"

	"mailers/offers/skills"
)

// Тексты системы, написанные при её выборе (2026-11-06, шаг B плана
// «генерация без модели»).
//
// Решение пользователя: при выборе системы модель пишет её текст ОДИН РАЗ,
// после того как собран контекст объекта, и текст сохраняется по блокам. Сборка
// документа только вставляет готовое — модель в экспорте не вызывается вовсе.
// Именно это даёт «экспорт за секунду вместо трёх-девяти минут» и убирает
// тишину в боте.
//
// Блоков ДВА, а не три: `description` (обоснование применения) и `advantages`
// (преимущества). Состав НЕ генерируется — он уже есть в карточке, и
// пересказ моделью был бы второй правдой рядом со списком.

// Блоки текста системы.
const (
	systemTextDescription = "description"
	systemTextAdvantages  = "advantages"
)

// systemTextBlocks — порядок блоков: тот же, что у печати документа.
var systemTextBlocks = []string{systemTextDescription, systemTextAdvantages}

// materialTextBlock — единственный блок текста МАТЕРИАЛА (2026-11-13).
//
// У материала просим только описание: «обоснование применения системы» про
// отдельную плёнку звучит как реклама, а преимущества пересказывают
// характеристики, которые и так есть в карточке. Блок хранится в ТОЙ ЖЕ
// таблице `offers_system_text` под ключом `material.description`: таблица —
// «тексты по заметке и блоку», и заводить вторую ради одного блока значило бы
// держать одно правило в двух местах.
const materialTextBlock = "material.description"

// materialTextInstruction — что просим у модели для описания материала.
var materialTextInstruction = map[string]string{
	materialTextBlock: "Описание материала для коммерческого предложения: что это, где применяется, " +
		"какие задачи объекта решает и чем важно для выбранного решения. Не менее 900 знаков, " +
		"только по фактам карточки и брифа, ничего не выдумывая.",
}

// systemTextInstruction — что просим у модели для блока.
//
// Текст инструкции — часть КОНТРАКТА хранения: от него считается хеш, которым
// решается «текст свежий». Переписали инструкцию — все сохранённые тексты
// устарели, и это правильно: они отвечали на другую просьбу.
var systemTextInstruction = map[string]string{
	systemTextDescription: "Обоснование применения системы: почему именно эта система подходит объекту " +
		"из брифа — задача заказчика, условия эксплуатации, риски. Не менее 1500 знаков, " +
		"только по фактам карточки и брифа, ничего не выдумывая.",
	systemTextAdvantages: "Преимущества системы для заказчика: что даёт выбранное решение по сравнению " +
		"с альтернативой — срок службы, надёжность, экономия на монтаже и обслуживании, " +
		"соответствие нормам. Не менее 1200 знаков, только по фактам карточки.",
}

// SystemText — текст одного блока одной системы.
type SystemText struct {
	NoteID    int64  `json:"noteId"`
	Block     string `json:"block"`
	Text      string `json:"text"`
	Hash      string `json:"hash"`
	UpdatedAt string `json:"updatedAt"`
	// Stale — карточка изменилась с тех пор, как текст написан.
	Stale bool `json:"stale"`
}

// systemCardHash — отпечаток содержимого карточки, из которого пишется текст.
//
// Берётся только то, что модель реально видит: название, описание, состав,
// характеристики. Заголовок и данные предложения сюда НЕ входят (первое —
// правка названия, второе меняется у каждой КП), иначе текст переписывался бы
// без причины.
func systemCardHash(card SystemCard) string {
	var b strings.Builder
	b.WriteString(card.Title + "\x1f")
	b.WriteString(card.Description + "\x1f")
	for _, line := range card.Composition {
		b.WriteString(line + "\x1f")
	}
	for _, row := range card.Specifications {
		b.WriteString(strings.Join(row, "|") + "\x1f")
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:8])
}

// systemTextHash — хеш, под которым хранится текст: содержимое карточки плюс
// сама инструкция. Инструкция тоже меняет текст — значит, входит в свежесть.
func systemTextHash(card SystemCard, block string) string {
	return hashShort(systemCardHash(card) + "\x1fv2\x1f" + systemTextInstruction[block])
}

// materialTextHash — то же для описания материала: своя инструкция, свой блок.
func materialTextHash(card SystemCard) string {
	return hashShort(systemCardHash(card) + "\x1fv2\x1f" + materialTextInstruction[materialTextBlock])
}

// storedSystemTexts — тексты, которые уже есть у систем предложения.
//
// Читаются пачкой: у КП систем немного, но по одной на каждую — лишние
// запросы при печати документа.
func (s *Server) storedSystemTexts(ctx context.Context, noteIDs []int64) map[int64]map[string]SystemText {
	out := map[int64]map[string]SystemText{}
	if len(noteIDs) == 0 {
		return out
	}
	rows, err := s.pool.Query(ctx, `
SELECT note_id, block, text, hash, updated_at
FROM offers_system_text
WHERE note_id = ANY($1)`, noteIDs)
	if err != nil {
		log.Printf("system texts: не прочитаны: %v", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var t SystemText
		var when time.Time
		if err := rows.Scan(&t.NoteID, &t.Block, &t.Text, &t.Hash, &when); err != nil {
			continue
		}
		t.UpdatedAt = when.Format(time.RFC3339Nano)
		if out[t.NoteID] == nil {
			out[t.NoteID] = map[string]SystemText{}
		}
		out[t.NoteID][t.Block] = t
	}
	return out
}

// saveSystemText пишет текст блока и двигает updated_at.
//
// Правило проекта: любая правка данных двигает метку времени, иначе у неё нет
// причины выиграть слияние. Здесь это ещё и признак «текст посчитан сейчас».
func (s *Server) saveSystemText(ctx context.Context, text SystemText) error {
	_, err := s.pool.Exec(ctx, `
INSERT INTO offers_system_text (note_id, block, text, hash, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (note_id, block) DO UPDATE
SET text = EXCLUDED.text, hash = EXCLUDED.hash, updated_at = now()`,
		text.NoteID, text.Block, text.Text, text.Hash)
	return err
}

// generateSystemTexts пишет тексты системы, у которых нет свежего сохранённого.
//
// force — переписать всё, не глядя на хеш: кнопка «Пересчитать тексты
// системы». Возвращает то, что получилось, и чего не вышло (вторая величина —
// названия блоков для человека: «не записались: преимущества»).
//
// Авария модели НЕ является аварией выбора системы: выбор сохранён, тексты
// остаются пустыми, а ответ говорит какие. Пустой блок честнее черновика —
// то же правило, что у блоков формы.
func (s *Server) generateSystemTexts(ctx context.Context, offer Offer, card SystemCard, force bool) (map[string]string, []string) {
	texts := map[string]string{}
	var missing []string
	if card.IsEmpty() {
		return texts, missing
	}
	existing := s.storedSystemTexts(ctx, []int64{card.NoteID})[card.NoteID]
	for _, block := range systemTextBlocks {
		if !force {
			if old, ok := existing[block]; ok && old.Hash == systemTextHash(card, block) && strings.TrimSpace(old.Text) != "" {
				texts[block] = old.Text
				continue
			}
		}
		text, ok := s.writeAndStoreSystemText(ctx, offer, card, block)
		if !ok {
			missing = append(missing, block)
			continue
		}
		texts[block] = text
	}
	return texts, missing
}

// writeAndStoreSystemText — один блок: модель, затем запись. false — не вышло
// ни написать, ни сохранить; вызывающий решает, как об этом сказать.
func (s *Server) writeAndStoreSystemText(ctx context.Context, offer Offer, card SystemCard, block string) (string, bool) {
	instruction := systemTextInstruction[block]
	started := time.Now()
	raw, err := writeSystemText(ctx, instruction, offer, card, card.Refs)
	logModelCall("system:"+block, fmt.Sprintf("note=%d", card.NoteID), len(instruction), started, err)
	if err != nil || strings.TrimSpace(raw) == "" {
		return "", false
	}
	if err := s.saveSystemText(ctx, SystemText{
		NoteID: card.NoteID, Block: block, Text: raw, Hash: systemTextHash(card, block),
	}); err != nil {
		log.Printf("system texts: не записан блок %s заметки %d: %v", block, card.NoteID, err)
		return "", false
	}
	return raw, true
}

// writeSystemText — один вызов модели для одного блока системы.
//
// Промпт и чистка — ТЕ ЖЕ, что у блоков формы: второй набор правил разошёлся
// бы с первым (правило 2026-11-05 «один конвейер текста»).
func writeSystemText(ctx context.Context, instruction string, offer Offer, card SystemCard, refs []skills.CardRef) (string, error) {
	return writeSystemTextWith(ctx, instruction, offer, card, refs, llmCompleter{})
}

// writeSystemTextWith — то же с явной моделью: тесты подставляют заглушку, а
// не ходят в сеть, и промпт проверяется без llm-service (2026-11-06).
func writeSystemTextWith(ctx context.Context, instruction string, offer Offer,
	card SystemCard, refs []skills.CardRef, c Completer) (string, error) {
	if c == nil {
		return "", nil
	}
	maxWords := skills.RequestedWords(instruction)
	if maxWords <= 0 {
		maxWords = genDefaultMaxWords
	}
	if maxWords > 800 {
		maxWords = 800
	}
	system := skills.RenderBlockSystem(instruction, "", maxWords)
	user := buildBlockUserPromptFor(instruction, offer, []SystemCard{card}, refs)
	raw, err := c.Complete(ctx, system, user, documentTextMaxTokens())
	if err != nil {
		return "", err
	}
	return cleanBlockText(raw), nil
}

// reconcileSystemTextsSystem — тексты выбранной системы; пустые места
// дописываются, ошибка модели не роняет выбор.
//
// Отдельная функция, а не inline в assembleSystem: там важна последовательность
// «сохранить выбор — потом генерировать», и вторая ответственность рядом с
// первой маскировала бы её.
func (s *Server) reconcileSystemTexts(ctx context.Context, offer Offer, card SystemCard) (map[string]string, []string) {
	texts, missing := s.generateSystemTexts(ctx, offer, card, false)
	return texts, missing
}

// recalculateSystem кнопка «Пересчитать тексты системы»: переписать оба блока.
func (s *Server) recalculateSystem(ctx context.Context, offer Offer, card SystemCard) (map[string]string, []string) {
	return s.generateSystemTexts(ctx, offer, card, true)
}

// reconcileMaterialTexts — описание выбранного материала; пустое дописывается.
//
// Ошибка модели не роняет выбор: материал уже сохранён, а описание просто
// останется пустым (то же правило, что у системы).
func (s *Server) reconcileMaterialTexts(ctx context.Context, offer Offer, card SystemCard) (string, []string) {
	if _, ok := s.storedSystemTexts(ctx, []int64{card.NoteID})[card.NoteID][materialTextBlock]; ok {
		if existing := s.storedTextsOfBlock(ctx, card, materialTextBlock); existing != "" {
			return existing, nil
		}
	}
	text, wrote := s.writeAndStoreMaterialText(ctx, offer, card)
	if !wrote {
		return "", []string{materialTextBlock}
	}
	return text, nil
}

// writeAndStoreMaterialText — описание материала: модель, затем запись.
func (s *Server) writeAndStoreMaterialText(ctx context.Context, offer Offer, card SystemCard) (string, bool) {
	instruction := materialTextInstruction[materialTextBlock]
	started := time.Now()
	raw, err := writeSystemText(ctx, instruction, offer, card, card.Refs)
	logModelCall("material:"+materialTextBlock, fmt.Sprintf("note=%d", card.NoteID), len(instruction), started, err)
	if err != nil || strings.TrimSpace(raw) == "" {
		return "", false
	}
	if err := s.saveSystemText(ctx, SystemText{
		NoteID: card.NoteID, Block: materialTextBlock, Text: raw, Hash: materialTextHash(card),
	}); err != nil {
		log.Printf("материал: описание заметки %d не записано: %v", card.NoteID, err)
		return "", false
	}
	return raw, true
}

// storedTextsOfBlock — уже сохранённый текст одного блока материала.
func (s *Server) storedTextsOfBlock(ctx context.Context, card SystemCard, block string) string {
	stored := s.storedSystemTexts(ctx, []int64{card.NoteID})[card.NoteID]
	item, ok := stored[block]
	if !ok {
		return ""
	}
	return strings.TrimSpace(item.Text)
}

// storedTextsOf — уже сохранённые тексты одной системы.
//
// Без вызова модели: нужно, когда человек повторно нажимает «Добавить» на ту
// же систему — тексты написаны ещё при первом выборе, и второй раз платить за
// них временем нечем (2026-11-06).
func (s *Server) storedTextsOf(ctx context.Context, card SystemCard) map[string]string {
	texts := map[string]string{}
	if stored, ok := s.storedSystemTexts(ctx, []int64{card.NoteID})[card.NoteID]; ok {
		for block, item := range stored {
			if strings.TrimSpace(item.Text) != "" {
				texts[block] = item.Text
			}
		}
	}
	return texts
}

// writeMissingSystemText — дописать один текст системы при сборке документа.
//
// Повод (2026-11-07): метка `{{текст: система: description}}` печатала только
// СОХРАНЁННОЕ, а у всех живых систем таблица `offers_system_text` была пуста —
// метка молчала всегда, в макете и в блоке формы одинаково. Человек справедливо
// считал, что она не работает.
//
// Здесь повторяется правило выбора системы: текст пишется один раз и
// сохраняется. Черновик кэшируется в form_values предложения, чтобы вторая
// сборка дорогого ответа не писала: обрыв связи посреди печати не должен
// превращать готовый документ в другой.
//
// nil-приёмник (стенд без БД, тесты) — пусто: метка тогда молчит, как раньше.
func (s *Server) writeMissingSystemText(offer *Offer, cache *layoutPromptCache,
	card SystemCard, block string) string {
	instruction, ok := systemTextInstruction[block]
	if !ok {
		return ""
	}
	if cache != nil {
		return cache.systemText(card.NoteID, block, instruction)
	}
	if s == nil {
		return ""
	}
	text, wrote := s.writeAndStoreSystemText(context.Background(), *offer, card, block)
	if !wrote {
		return ""
	}
	return text
}

// systemTextsByNote — сохранённые тексты систем по номеру заметки: карта
// «заметка → блок → текст».
//
// Нужна печати документа (шаг C): метки `{{текст: …}}` берут готовое и модель
// не зовут. Читается один раз на сборку и раздаётся по карточкам; отдельный
// запрос на каждую систему был бы лишним хождением в БД на каждой метке.
func (s *Server) systemTextsByNote(ctx context.Context, cards []SystemCard) map[int64]map[string]string {
	ids := make([]int64, 0, len(cards))
	for _, card := range cards {
		if card.NoteID > 0 {
			ids = append(ids, card.NoteID)
		}
	}
	out := make(map[int64]map[string]string, len(ids))
	for noteID, stored := range s.storedSystemTexts(ctx, ids) {
		texts := make(map[string]string, len(stored))
		for block, item := range stored {
			if strings.TrimSpace(item.Text) != "" {
				texts[block] = item.Text
			}
		}
		out[noteID] = texts
	}
	return out
}

// systemTextsForSystems — сохранённые тексты выбранных систем с признаком
// устаревания: по нему плагин решает, показывать ли кнопку пересчёта.
func (s *Server) systemTextsForSystems(ctx context.Context, cards []SystemCard) []SystemText {
	ids := make([]int64, 0, len(cards))
	for _, card := range cards {
		if card.NoteID > 0 {
			ids = append(ids, card.NoteID)
		}
	}
	stored := s.storedSystemTexts(ctx, ids)
	out := make([]SystemText, 0, len(ids)*len(systemTextBlocks))
	for _, card := range cards {
		for _, block := range systemTextBlocks {
			want := systemTextHash(card, block)
			item, ok := stored[card.NoteID][block]
			if !ok {
				item = SystemText{NoteID: card.NoteID, Block: block}
			}
			item.Stale = ok && item.Hash != want
			out = append(out, item)
		}
	}
	return out
}
