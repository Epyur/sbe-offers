package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"strings"

	"mailers/offers/skills"
)

// Готовый DOCX боевого листка хранится рядом с листком (2026-11-12).
//
// Просьба пользователя: «нужно сделать механизм (как в КП), чтобы листок не
// перегенерировался каждый раз (это долго)». Сборка листка зовёт модель на
// сравнение — десятки секунд, и повторное «⬇ DOCX» не должно платить второй раз.
//
// Механизм тот же, что у текстов систем (`offers_system_text`): файл лежит в
// таблице под ХЭШЕМ того, из чего он собран. Хэш сошёлся — отдаём сохранённый
// файл; не сошёлся — собираем заново и сохраняем. Кэш не источник правды: его
// можно снести в любой момент, листок соберётся заново.

// combatDocxVersion — версия сборки листка. Меняется, когда печать меняет вид
// документа (новый раздел, другой порядок), чтобы старые файлы не отдавались
// как свежие. Строка входит в хэш.
//
// v2 (2026-11-17, шаги 2a/5/6): вид листка сменился — содержание печатается
// РАЗДЕЛАМИ формы (с таблицами, схемами mermaid→PNG и текстом по каждому
// продукту), а не одним зашитым сравнением. Файлы, собранные до этого, в кэше
// описывают прежний вид, поэтому их хэш обязан разойтись — иначе менеджер
// получал бы старый листок как «свежий».
//
// v3 (2026-11-17, второй заход): тексты разделов меняются на ту же форму листка —
// в промпт пошли характеристики карточки (без них по нашему продукту стояло «не
// указано»), а из ответа вычищаются швы от «[НУЖНО: …]». Хэш не знает текстов
// карточек (см. ниже), поэтому без смены версии кэш отдал бы файл с пустой
// колонкой нашего продукта как свежий.
//
// v4 (2026-11-17, тот же заход): таблица характеристик больше не режется по 15
// строкам — живая карточка конкурента (23–30 строк) теряла хвост, и модель писала
// «не указано» на гибкость, УФ и сварной шов, которые в карточке есть. Файлы v3
// уже собраны и лежат в кэше, поэтому версия обязана смениться ещё раз.
//
// v5 (2026-11-17, вечер): ссылки печатаются одним списком в порядке упоминания
// (без групп и без ссылок конкурента), а базовый промт документа правится в
// форме. Форма теперь входит в хэш, так что правка промта пересобирает файл;
// версия поднята, чтобы файлы прежней печати не отдавались как свежие.
//
// v6 (2026-11-17): встроенный вид листка (без макета) печатает раздел «Ссылки»
// по нашим продуктам, а карточки несут и документы из приложения «Документы».
// Вид документа сменился — файлы прежней сборки обязаны разойтись по хэшу.
//
// v7 (2026-11-17, источники конкурента): в листке появились конкуренты из файла
// и ссылки — карточки без Базы знаний, со своим текстом. Хэш начал включать их
// (`combatDocxHash`), но файлы, собранные до этого, собраны прежним набором
// карточек, поэтому версия обязана смениться.
//
// v8 (2026-11-18): в хэш вошли ССЫЛКИ наших карточек. Живой листок 017/2026
// печатал две ссылки из «## Ссылки», хотя в заметке есть и «## Иные документы»
// (файлы приложения «Документы»): файл отдавался из кэша, а ссылки меняются
// вместе с карточкой и в прежний хэш не входили. Версия поднята, чтобы уже
// собранные файлы разошлись по хэшу и пересобрались с новым правилом.
const combatDocxVersion = "v9"

// combatDocxHash — отпечаток того, из чего собирается документ листка.
//
// Входят: номер листка, обе стороны в порядке выбора, уточнение менеджера,
// байты макета, отпечаток формы (разделы и базовый промт, 2026-11-17) и версия
// сборки. НЕ входят тексты карточек из Базы знаний: карточку правят часто, а
// модель на сравнение — нет, и пересборка всех листков от правки одной заметки
// была бы накладнее, чем сборка при явном нажатии. Порядок сторон — часть
// содержания (менеджер выстроил его сам), поэтому склеивается списком, а не
// сортировкой.
//
// Форма входит потому, что она задаёт СОДЕРЖАНИЕ: правка базового промта или
// инструкции подраздела обязана пересобрать файл, а без входа в хэш кэш отдал бы
// старый листок как свежий (та же ловушка, что с текстами карточек).
func combatDocxHash(sheet CombatSheet, layout []byte, version, formHash string, refs []skills.CardRef) string {
	var b strings.Builder
	b.WriteString(version)
	b.WriteString("\x1f" + sheet.Number)
	b.WriteString("\x1four\x1f")
	for _, ref := range sheet.OurNotes {
		b.WriteString(itoa(int(ref.NoteID)) + ":" + ref.Title + "\x1f")
	}
	// Ссылки наших карточек — часть документа (2026-11-18). Почему именно здесь,
	// а не «перечитывать карточки при каждом экспорте»: содержимое карточки
	// по-прежнему НЕ входит в хэш (правка описания не должна пересобирать все
	// листки), а вот список ссылок — короткий и меняется редко. Порядок берётся
	// как в заметке: перестановка ссылок тоже меняет документ.
	//
	// Пустой список ничего не пишет: у листков без ссылок хэш остаётся прежним,
	// и старые файлы не пересобираются без причины.
	if len(refs) > 0 {
		b.WriteString("\x1frefs\x1f")
		for _, ref := range refs {
			b.WriteString(hashShort(ref.Label+"\x1f"+ref.URL) + "\x1f")
		}
	}
	b.WriteString("\x1fcomp\x1f")
	for _, ref := range sheet.CompetitorNotes {
		b.WriteString(itoa(int(ref.NoteID)) + ":" + ref.Title + "\x1f")
	}
	// Конкуренты из файла/ссылки — содержание листка, а не карточка Базы знаний:
	// правка их текста обязана пересобрать файл. Без этого «Прочитать» заново
	// отдавал бы старый DOCX как свежий (класс грабель v3/v6).
	if len(sheet.TextCompetitors) > 0 {
		b.WriteString("\x1ftextcomp\x1f")
		for _, ref := range sheet.TextCompetitors {
			b.WriteString(ref.Key + "\x1f" + ref.Title + "\x1f" + ref.Source + "\x1f" +
				hashShort(ref.Text) + "\x1f")
		}
	}
	b.WriteString("\x1fhint\x1f" + strings.TrimSpace(combatHint(sheet)))
	b.WriteString("\x1fform\x1f" + formHash)
	b.WriteString("\x1f" + itoa(len(layout)) + "\x1f")
	sum := sha256.Sum256([]byte(b.String()))
	if len(layout) > 0 {
		layoutSum := sha256.Sum256(layout)
		sum = sha256.Sum256(append(sum[:], layoutSum[:]...))
	}
	return hex.EncodeToString(sum[:16])
}

// combatFormFingerprint — отпечаток формы листка: разделы и базовый промт.
//
// Секции маршалятся как есть: в них инструкции подразделов, условия и бриф —
// всё, что меняет текст документа. Ошибка маршалинга не роняет печать: пустой
// отпечаток хуже свежего, но лучше отказа.
func combatFormFingerprint(form OfferForm) string {
	raw, err := json.Marshal(struct {
		Base     string           `json:"base"`
		Sections []ContentSection `json:"sections"`
	}{Base: form.BasePrompt, Sections: form.Sections})
	if err != nil {
		log.Printf("форма листка: отпечаток не посчитан: %v", err)
		return ""
	}
	return hashShort(string(raw))
}

// storedCombatDocx — сохранённый файл листка по хэшу.
func (s *Server) storedCombatDocx(ctx context.Context, sheetID int64, hash string) []byte {
	var data []byte
	var stored string
	err := s.pool.QueryRow(ctx, `
SELECT docx, hash FROM offers_combat_docx WHERE sheet_id = $1`, sheetID).Scan(&data, &stored)
	if err != nil || stored != hash || len(data) == 0 {
		return nil
	}
	return data
}

// saveCombatDocx — записать файл листка под его хэшем.
//
// Ошибка записи не мешает отдать файл человеку: кэш — ускорение, а не повод
// отказать. Поэтому только в журнал.
func (s *Server) saveCombatDocx(ctx context.Context, sheetID int64, hash string, data []byte) {
	_, err := s.pool.Exec(ctx, `
INSERT INTO offers_combat_docx (sheet_id, hash, docx, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (sheet_id) DO UPDATE
SET hash = EXCLUDED.hash, docx = EXCLUDED.docx, updated_at = now()`,
		sheetID, hash, data)
	if err != nil {
		log.Printf("боевой листок %d: файл не сохранён в кэш: %v", sheetID, err)
	}
}

// renderCombatDocxCached — DOCX листка с кэшем.
//
// Порядок: посчитать хэш, взять сохранённый файл, иначе собрать и сохранить.
// Сборка — это та же `renderCombatDocx`, что и раньше: кэш ничего не меняет в
// содержании, только избавляет от повторной работы.
func (s *Server) renderCombatDocxCached(ctx context.Context, layout []byte, sheet CombatSheet,
	ourCards, competitorCards []SystemCard, images ImageSource) ([]byte, error) {
	// Форма входит в хэш (2026-11-17): правка базового промта или инструкции
	// подраздела меняет содержание файла, и кэш обязан это увидеть.
	form, err := s.loadForm(ctx, combatFormMode)
	if err != nil {
		log.Printf("боевой листок %s: форма не прочитана для хэша: %v", sheet.Number, err)
	}
	hash := combatDocxHash(sheet, layout, combatDocxVersion, combatFormFingerprint(form), combatOurRefs(ourCards))
	if cached := s.storedCombatDocx(ctx, sheet.ID, hash); cached != nil {
		log.Printf("боевой листок %s: файл из кэша", sheet.Number)
		return cached, nil
	}
	data, err := s.renderCombatDocx(ctx, layout, sheet, ourCards, competitorCards, images)
	if err != nil {
		return nil, err
	}
	s.saveCombatDocx(ctx, sheet.ID, hash, data)
	return data, nil
}
