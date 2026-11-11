package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"strings"
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
const combatDocxVersion = "v1"

// combatDocxHash — отпечаток того, из чего собирается документ листка.
//
// Входят: номер листка, обе стороны в порядке выбора, уточнение менеджера,
// байты макета и версия сборки. НЕ входят тексты карточек из Базы знаний:
// карточку правят часто, а модель на сравнение — нет, и пересборка всех листков
// от правки одной заметки была бы накладнее, чем сборка при явном нажатии.
// Порядок сторон — часть содержания (менеджер выстроил его сам), поэтому
// склеивается списком, а не сортировкой.
func combatDocxHash(sheet CombatSheet, layout []byte, version string) string {
	var b strings.Builder
	b.WriteString(version)
	b.WriteString("\x1f" + sheet.Number)
	b.WriteString("\x1four\x1f")
	for _, ref := range sheet.OurNotes {
		b.WriteString(itoa(int(ref.NoteID)) + ":" + ref.Title + "\x1f")
	}
	b.WriteString("\x1fcomp\x1f")
	for _, ref := range sheet.CompetitorNotes {
		b.WriteString(itoa(int(ref.NoteID)) + ":" + ref.Title + "\x1f")
	}
	b.WriteString("\x1fhint\x1f" + strings.TrimSpace(combatHint(sheet)))
	b.WriteString("\x1f" + itoa(len(layout)) + "\x1f")
	sum := sha256.Sum256([]byte(b.String()))
	if len(layout) > 0 {
		layoutSum := sha256.Sum256(layout)
		sum = sha256.Sum256(append(sum[:], layoutSum[:]...))
	}
	return hex.EncodeToString(sum[:16])
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
	hash := combatDocxHash(sheet, layout, combatDocxVersion)
	if cached := s.storedCombatDocx(ctx, sheet.ID, hash); cached != nil {
		log.Printf("боевой листок %s: файл из кэша", sheet.Number)
		return cached, nil
	}
	data, err := renderCombatDocx(layout, sheet, ourCards, competitorCards, images)
	if err != nil {
		return nil, err
	}
	s.saveCombatDocx(ctx, sheet.ID, hash, data)
	return data, nil
}
