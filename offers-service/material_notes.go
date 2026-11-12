package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"mailers/offers/skills"
)

// ╨Ь╨░╤В╨╡╤А╨╕╨░╨╗╤Л ╨╕╨╖ ╨║╨░╤А╤В╨╛╤З╨╡╨║ ╨С╨░╨╖╤Л ╨╖╨╜╨░╨╜╨╕╨╣ (2026-10-31).
//
// ╨а╨╡╤И╨╡╨╜╨╕╨╡ ╨┐╨╛╨╗╤М╨╖╨╛╨▓╨░╤В╨╡╨╗╤П: ╨▓ ╨║╨░╤А╤В╨╛╤З╨║╨░╤Е ╨╝╨░╤В╨╡╤А╨╕╨░╨╗╨╛╨▓ SKU ╨▓╨▓╨╛╨┤╨╕╤В╤Б╤П ╨║╨░╨║ ╨┐╤А╨╕╨╖╨╜╨░╨║
// ╨╝╨░╤В╨╡╤А╨╕╨░╨╗╨░, ╨╜╨╛ ╨б╨Т╨п╨Ч╨м ╤Б╨╛╤Б╤В╨░╨▓╨░ ╤Б╨╕╤Б╤В╨╡╨╝ ╤Б ╨╝╨░╤В╨╡╤А╨╕╨░╨╗╨╛╨╝ ╨╕╨┤╤С╤В ╨┐╨╛ ╨▓╨╕╨║╨╕-╤Б╤Б╤Л╨╗╨║╨╡ ╨╜╨░
// ╨╖╨░╨╝╨╡╤В╨║╤Г. ╨Ч╨╜╨░╤З╨╕╤В, ╤Б╨┐╤А╨░╨▓╨╛╤З╨╜╨╕╨║ ╨╝╨░╤В╨╡╤А╨╕╨░╨╗╨╛╨▓ ╨┤╨╛╨╗╨╢╨╡╨╜ ╨┐╨╛╨╝╨╜╨╕╤В╤М, ╨╕╨╖ ╨║╨░╨║╨╛╨╣ ╨╖╨░╨╝╨╡╤В╨║╨╕ ╨╛╨╜
// ╨╖╨░╨▓╨╡╨┤╤С╨╜, тАФ ╨╕╨╜╨░╤З╨╡ ╤Б╤Б╤Л╨╗╨║╨╡ ╨╜╨╡╨║╤Г╨┤╨░ ╨▓╨╡╤Б╤В╨╕.
//
// ╨Ю╤В╤Б╤О╨┤╨░ ┬л╨Ь╨░╤В╨╡╤А╨╕╨░╨╗╤Л┬╗ ╨┐╨╡╤А╨╡╤Б╤В╨░╤О╤В ╨▒╤Л╤В╤М ╤Б╨┐╤А╨░╨▓╨╛╤З╨╜╨╕╨║╨╛╨╝, ╨║╨╛╤В╨╛╤А╤Л╨╣ ╨▓╨╕╤Б╨╕╤В ╨▓ ╨▓╨╛╨╖╨┤╤Г╤Е╨╡:
// ╨║╨░╤А╤В╨╛╤З╨║╨░ ╤Б ╤В╨╡╨│╨╛╨╝ `material` ╨┐╤А╨╡╨▓╤А╨░╤Й╨░╨╡╤В╤Б╤П ╨▓ ╨╝╨░╤В╨╡╤А╨╕╨░╨╗ ╨┐╤А╨╕ ╨║╨░╨╢╨┤╨╛╨╝ ╤З╤В╨╡╨╜╨╕╨╕
// ╤Б╨┐╨╕╤Б╨║╨░. ╨н╤В╨╛ ╨╜╨╡ ╨╝╨╕╨│╤А╨░╤Ж╨╕╤П ╨▓ ╨▒╨░╨╖╤Г, ╨░ ╤Б╨╕╨╜╤Е╤А╨╛╨╜╨╕╨╖╨░╤Ж╨╕╤П ╤З╤В╨╡╨╜╨╕╨╡╨╝ тАФ ╤В╨░╨║ ╨╜╨╡╤В ╨▓╨╛╨┐╤А╨╛╤Б╨░
// ┬л╨░ ╤З╤В╨╛ ╨╡╤Б╨╗╨╕ ╨╖╨░╨╝╨╡╤В╨║╤Г ╨┐╨╡╤А╨╡╨╕╨╝╨╡╨╜╨╛╨▓╨░╨╗╨╕┬╗: ╨╕╨╝╤П ╨╝╨░╤В╨╡╤А╨╕╨░╨╗╨░ ╤Б╨╗╨╡╨┤╤Г╨╡╤В ╨╖╨░ ╨╖╨░╨╝╨╡╤В╨║╨╛╨╣.
//
// ╨Я╤А╨░╨▓╨╕╨╗╨░, ╨║╨╛╤В╨╛╤А╤Л╨╡ ╨╖╨┤╨╡╤Б╤М ╨┤╨╡╤А╨╢╨░╤В╤Б╤П:
//   - SKU ╨╜╨░╨╖╨╜╨░╤З╨░╨╡╤В╤Б╤П ╨Ю╨Ф╨Ш╨Э ╨а╨Р╨Ч ╨╕ ╨╜╨╡ ╨╝╨╡╨╜╤П╨╡╤В╤Б╤П ╨╜╨╕ ╨┐╤А╨╕ ╨║╨░╨║╨╛╨╝ ╤З╤В╨╡╨╜╨╕╨╕ (╤В╨╡╤Б╤В);
//   - ╨╝╨░╤В╨╡╤А╨╕╨░╨╗ ╨╕╤Й╨╡╤В╤Б╤П ╨┐╨╛ note_id; ╨╜╨░╨╣╨┤╨╡╨╜ тАФ ╨┐╨╛╨┤╤В╤П╨│╨╕╨▓╨░╨╡╤В╤Б╤П ╨╕╨╝╤П ╨╕ ╨╡╨┤╨╕╨╜╨╕╤Ж╨░, ╨╡╤Б╨╗╨╕
//     ╨╖╨░╨╝╨╡╤В╨║╨░ ╨╕╨╖╨╝╨╡╨╜╨╕╨╗╨░╤Б╤М;
//   - ╨╜╨╡ ╨╜╨░╨╣╨┤╨╡╨╜ тАФ ╨╖╨░╨▓╨╛╨┤╨╕╤В╤Б╤П: ╤Н╤В╨╛ ╨╜╨╛╨▓╨░╤П ╨║╨░╤А╤В╨╛╤З╨║╨░ ╨╝╨░╤В╨╡╤А╨╕╨░╨╗╨░;
//   - ╨║╨░╤А╤В╨╛╤З╨║╨░, ╨┐╨╛╨╝╨╡╤З╨╡╨╜╨╜╨░╤П ╤Г╨┤╨░╨╗╤С╨╜╨╜╨╛╨╣ ╨▓ ╨С╨░╨╖╨╡ ╨╖╨╜╨░╨╜╨╕╨╣, ╨╝╨░╤В╨╡╤А╨╕╨░╨╗ ╨▓ ╨░╤А╤Е╨╕╨▓ ╨Э╨Х
//     ╤Г╨▓╨╛╨┤╨╕╤В: ╨░╤А╤Е╨╕╨▓ тАФ ╤А╨╡╤И╨╡╨╜╨╕╨╡ ╤З╨╡╨╗╨╛╨▓╨╡╨║╨░ ╨▓ ╨┐╨╗╨░╨│╨╕╨╜╨╡, ╨░ ╨╜╨╡ ╤Б╨╗╨╡╨┤╤Б╤В╨▓╨╕╨╡ ╨┐╤А╨░╨▓╨║╨╕
//     ╨╖╨░╨╝╨╡╤В╨║╨╕. ╨Ш╨╜╨░╤З╨╡ ╨╛╨┤╨╜╨░ ╤З╨╕╤Б╤В╨║╨░ ╨▓ ╨С╨░╨╖╨╡ ╨╖╨╜╨░╨╜╨╕╨╣ ╤Б╨╜╨╕╨╝╨░╨╗╨░ ╨▒╤Л ╨╝╨░╤В╨╡╤А╨╕╨░╨╗╤Л ╤Б ╨┐╤А╨╛╨┤╨░╨╢╨╕.

// noteMaterial тАФ ╤З╤В╨╛ ╨╜╤Г╨╢╨╜╨╛ ╨╛╤В ╨╖╨░╨╝╨╡╤В╨║╨╕, ╤З╤В╨╛╨▒╤Л ╨╖╨░╨▓╨╡╤Б╤В╨╕ ╨╝╨░╤В╨╡╤А╨╕╨░╨╗.
type noteMaterial struct {
	NoteID int64
	Title  string
}

// syncMaterialsFromNotes ╨┐╤А╨╕╨▓╨╛╨┤╨╕╤В ╤Б╨┐╤А╨░╨▓╨╛╤З╨╜╨╕╨║ ╨║ ╨╜╨░╨▒╨╛╤А╤Г ╨║╨░╤А╤В╨╛╤З╨╡╨║-╨╝╨░╤В╨╡╤А╨╕╨░╨╗╨╛╨▓.
func (s *Server) syncMaterialsFromNotes(ctx context.Context) error {
	notes, err := s.materialNotes(ctx)
	if err != nil {
		return err
	}
	if len(notes) == 0 {
		return nil
	}
	for _, note := range notes {
		if err := s.ensureMaterialForNote(ctx, note); err != nil {
			// ╨Ю╨┤╨╜╨░ ╨┐╨╗╨╛╤Е╨░╤П ╨║╨░╤А╤В╨╛╤З╨║╨░ ╨╜╨╡ ╨┤╨╛╨╗╨╢╨╜╨░ ╨│╨░╤Б╨╕╤В╤М ╨▓╨╡╤Б╤М ╤Б╨┐╤А╨░╨▓╨╛╤З╨╜╨╕╨║: ╤З╨╡╨╗╨╛╨▓╨╡╨║
			// ╨▓╤Б╤С ╤А╨░╨▓╨╜╨╛ ╨┤╨╛╨╗╨╢╨╡╨╜ ╤Г╨▓╨╕╨┤╨╡╤В╤М ╨╛╤Б╤В╨░╨╗╤М╨╜╤Л╨╡ ╨╝╨░╤В╨╡╤А╨╕╨░╨╗╤Л.
			log.Printf("╨╝╨░╤В╨╡╤А╨╕╨░╨╗╤Л: ╨║╨░╤А╤В╨╛╤З╨║╨░ %d (%s) ╨╜╨╡ ╤Б╨╕╨╜╤Е╤А╨╛╨╜╨╕╨╖╨╕╤А╨╛╨▓╨░╨╜╨░: %v",
				note.NoteID, note.Title, err)
		}
	}
	// ╨Ъ╨░╤А╤В╨╕╨╜╨║╨╕ ╨╝╨░╤В╨╡╤А╨╕╨░╨╗╨╛╨▓ (2026-11-05). ╨Ъ╨╛╨╗╨╛╨╜╨║╨░ `photo_key` ╨▒╤Л╨╗╨░ ╨▓ ╤Б╤Е╨╡╨╝╨╡ ╤Б
	// ╤Б╨░╨╝╨╛╨│╨╛ ╨╜╨░╤З╨░╨╗╨░, ╨╜╨╛ ╨╡╤С ╨╜╨╕╨║╤В╨╛ ╨╜╨╡ ╨╖╨░╨┐╨╛╨╗╨╜╤П╨╗, ╨┐╨╛╤Н╤В╨╛╨╝╤Г ╨║╨░╤А╤В╨╕╨╜╨║╨╕ ╨╝╨░╤В╨╡╤А╨╕╨░╨╗╨╛╨▓ ╨▓
	// ╨┤╨╛╨║╤Г╨╝╨╡╨╜╤В╨╡ ╨╜╨╡ ╨▒╤Л╨╗╨╛ ╨▓╨╛╨▓╤Б╨╡. ╨Ч╨░╨┐╨╛╨╗╨╜╤П╨╡╨╝ ╨╖╨┤╨╡╤Б╤М, ╨░ ╨╜╨╡ ╨┐╤А╨╕ ╨║╨░╨╢╨┤╨╛╨╝ ╤З╤В╨╡╨╜╨╕╨╕
	// ╤Б╨┐╤А╨░╨▓╨╛╤З╨╜╨╕╨║╨░: ╨▓╨╗╨╛╨╢╨╡╨╜╨╕╤П тАФ ╨╖╨░╨┐╤А╨╛╤Б╤Л ╨▓ ╤З╤Г╨╢╨╛╨╣ ╤Б╨╡╤А╨▓╨╕╤Б, ╨╕ ╨╛╨╜╨╕ ╨╜╨╡ ╨┤╨╛╨╗╨╢╨╜╤Л ╤Б╤В╨╛╤П╤В╤М
	// ╨╜╨░ ╨┐╤Г╤В╨╕ ╨╛╤В╨║╤А╤Л╤В╨╕╤П ╤А╨░╨╖╨┤╨╡╨╗╨░.
	s.syncMaterialPhotos(ctx)
	return nil
}

// syncMaterialPhotos ╨┐╤А╨╛╤Б╤В╨░╨▓╨╗╤П╨╡╤В ╨╝╨░╤В╨╡╤А╨╕╨░╨╗╨░╨╝ ╨║╨╗╤О╤З ╨║╨░╤А╤В╨╕╨╜╨║╨╕ ╨╕╨╖ ╨▓╨╗╨╛╨╢╨╡╨╜╨╕╨╣ ╨╖╨░╨╝╨╡╤В╨║╨╕.
//
// ╨Ю╤И╨╕╨▒╨║╨░ ╤З╤Г╨╢╨╛╨│╨╛ ╤Б╨╡╤А╨▓╨╕╤Б╨░ ╨╖╨┤╨╡╤Б╤М ╤В╨╛╨╗╤М╨║╨╛ ╨┐╨╕╤И╨╡╤В╤Б╤П ╨▓ ╨╢╤Г╤А╨╜╨░╨╗: ╨╝╨░╤В╨╡╤А╨╕╨░╨╗ ╨▒╨╡╨╖ ╨║╨░╤А╤В╨╕╨╜╨║╨╕ тАФ
// ╨╜╨╛╤А╨╝╨░, ╨░ ╤Г╨┐╨░╨▓╤И╨╕╨╣ ╤Б╨┐╤А╨░╨▓╨╛╤З╨╜╨╕╨║ тАФ ╨╜╨╡╤В.
func (s *Server) syncMaterialPhotos(ctx context.Context) {
	if s.kb == nil || !s.kb.configured() {
		return
	}
	items, err := s.listMaterialsOnce(ctx, true)
	if err != nil {
		log.Printf("╨╝╨░╤В╨╡╤А╨╕╨░╨╗╤Л: ╤З╤В╨╡╨╜╨╕╨╡ ╨┤╨╗╤П ╨║╨░╤А╤В╨╕╨╜╨╛╨║ ╨╜╨╡ ╤Г╨┤╨░╨╗╨╛╤Б╤М: %v", err)
		return
	}
	withKeys := attachMaterialImageKeys(ctx, s.kb, items)
	for i := range withKeys {
		if withKeys[i].PhotoKey == "" || withKeys[i].PhotoKey == items[i].PhotoKey {
			continue
		}
		if _, err := s.pool.Exec(ctx,
			`UPDATE offers_material SET photo_key = $2, updated_at = now() WHERE id = $1`,
			withKeys[i].ID, withKeys[i].PhotoKey); err != nil {
			log.Printf("╨╝╨░╤В╨╡╤А╨╕╨░╨╗╤Л: ╨║╨░╤А╤В╨╕╨╜╨║╨░ %d ╨╜╨╡ ╤Б╨╛╤Е╤А╨░╨╜╨╡╨╜╨░: %v", withKeys[i].ID, err)
		}
	}
}

// materialNotes тАФ ╨╖╨░╨╝╨╡╤В╨║╨╕ ╤Б ╤В╨╡╨│╨╛╨╝ material.
//
// kb == nil (╨С╨░╨╖╨░ ╨╖╨╜╨░╨╜╨╕╨╣ ╨╜╨╡ ╨╜╨░╤Б╤В╤А╨╛╨╡╨╜╨░) тАФ ╨╜╨╡ ╨╛╤И╨╕╨▒╨║╨░: ╤Б╨┐╤А╨░╨▓╨╛╤З╨╜╨╕╨║ ╤В╨╛╨│╨┤╨░ ╨╛╤Б╤В╨░╤С╤В╤Б╤П
// ╤В╨╡╨╝, ╤З╤В╨╛ ╨╖╨░╨▓╨╡╨┤╤С╨╜ ╤А╤Г╨║╨░╨╝╨╕, ╨╕ ┬л╨Ф╨╛╨▒╨░╨▓╨╕╤В╤М ╤Б╨╕╤Б╤В╨╡╨╝╤Г┬╗ ╤З╨╡╤Б╤В╨╜╨╛ ╤Б╨║╨░╨╢╨╡╤В, ╤З╤В╨╛ ╤Б╤Б╤Л╨╗╨║╨╕
// ╨▓╨╡╨┤╤Г╤В ╨▓ ╨╜╨╕╨║╤Г╨┤╨░.
func (s *Server) materialNotes(ctx context.Context) ([]noteMaterial, error) {
	if s.kb == nil || !s.kb.configured() {
		return nil, nil
	}
	return materialNotesFromKB(ctx, s.kb)
}

// materialNotesFromKB ╨╛╤В╨┤╨╡╨╗╨╡╨╜╨╛ ╨╛╤В ╤Б╨╡╤А╨▓╨╡╤А╨░ ╤А╨░╨┤╨╕ ╨┐╤А╨╛╨▓╨╡╤А╨║╨╕ ╨▒╨╡╨╖ ╨▒╨░╨╖╤Л.
func materialNotesFromKB(ctx context.Context, kb KBReader) ([]noteMaterial, error) {
	tagID, err := kb.TagID(ctx, materialTagName)
	if err != nil {
		return nil, err
	}
	notes, err := kb.Notes(ctx, tagID)
	if err != nil {
		return nil, err
	}
	out := make([]noteMaterial, 0, len(notes))
	for _, n := range notes {
		title := strings.TrimSpace(n.Title)
		if title == "" || n.ID == 0 {
			continue
		}
		out = append(out, noteMaterial{NoteID: n.ID, Title: title})
	}
	return out, nil
}

// ensureMaterialForNote заводит материал по заметке, если его ещё нет.
//
// Существующий материал не переписывается по существу: ручная правка имени,
// единицы и описания в плагине важнее названия заметки. Подтягивается только
// пустое имя — так материал, заведённый мимо карточки, не остаётся безымянным.
func (s *Server) ensureMaterialForNote(ctx context.Context, note noteMaterial) error {
	var id int64
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM offers_material WHERE note_id = $1 LIMIT 1`, note.NoteID).Scan(&id)
	if err == nil {
		return nil
	}
	// Материала нет — заводим. SKU назначается здесь же и больше не меняется.
	_, err = s.insertMaterial(ctx, Material{
		Kind:      "material",
		Name:      note.Title,
		Unit:      "шт",
		NoteTitle: note.Title,
		NoteID:    note.NoteID,
	}, "", "")
	return err
}

// --- Материал как объект предложения (2026-11-13) ----------------------
//
// До этой правки материал попадал в КП только ПОЗИЦИЕЙ: строкой с ценой из
// таблицы менеджера. Просьба пользователя — предложить материал и ОПИСАНИЕМ, как
// систему: выбор материала кладёт в документ его текст и ссылки, а цен не
// трогает. Это разные вещи и разные пути, и они остаются раздельными: позицию
// добавляет выбор в таблице состава, объект — эта ручка.

// loadCachedMaterialCards читает карточки материалов из того же кеша
// `system_cards` (2026-11-13).
//
// Отдельного кеша не заводим: материал — такая же карточка Базы знаний, и
// «перечитать из Базы знаний» один раз читает обе половины. Не нашли материал
// в кеше — он печатается по справочнику без ссылок, а не пропадает.
func (s *Server) loadCachedMaterialCards(ctx context.Context) ([]SystemCard, error) {
	rows, err := s.pool.Query(ctx, `
SELECT m.note_id, COALESCE(NULLIF(m.name, ''), m.note_title), m.sku, m.description
FROM offers_material m
WHERE m.note_id > 0 AND NOT m.archived
ORDER BY m.sort_order, m.name`)
	if err != nil {
		return []SystemCard{}, err
	}
	defer rows.Close()
	out := make([]SystemCard, 0, 32)
	for rows.Next() {
		var card SystemCard
		var noteID int64
		if err := rows.Scan(&noteID, &card.Title, &card.SKU, &card.Description); err != nil {
			return []SystemCard{}, err
		}
		card.NoteID = noteID
		out = append(out, card)
	}
	return out, rows.Err()
}

// materialCardsForOffer — карточки выбранных материалов, дополненные ссылками
// из Базы знаний.
//
// Сначала берём сохранённое (список), потом обогащаем заметкой: при печати
// лишний заход в Базу знаний на каждый материал недопустим, а без заметки не
// будет ни ссылок, ни картинки. Если База знаний не настроена — остаётся то,
// что есть.
func (s *Server) materialCardsForOffer(ctx context.Context, offer Offer) []SystemCard {
	if len(offer.Materials) == 0 {
		return nil
	}
	cached, _ := s.loadCachedMaterialCards(ctx)
	chosen := materialsForOffer(cached, offer.Materials)
	if s.kb == nil || !s.kb.configured() {
		return chosen
	}
	materials, err := s.listMaterials(ctx, false)
	if err != nil {
		log.Printf("материалы предложения: справочник не прочитан: %v", err)
		return chosen
	}
	out := make([]SystemCard, 0, len(offer.Materials))
	for _, ref := range offer.Materials {
		card, found := s.materialCardForChoice(ctx, materials, ref)
		if !found {
			continue
		}
		if !containsCard(out, card) {
			out = append(out, card)
		}
	}
	return out
}

// materialCardForChoice — карточка материала для печати: имя, описание, ссылки.
//
// Читается из Базы знаний вживую, как у системы: человек выбрал материал из
// раздела, который читает базу знаний вживую, и кеш мог отстать.
func (s *Server) materialCardForChoice(ctx context.Context, materials []Material, choice Ref) (SystemCard, bool) {
	want := strings.ToUpper(strings.TrimSpace(choice.SKU))
	for _, m := range materials {
		if want != "" {
			if strings.ToUpper(strings.TrimSpace(m.SKU)) != want {
				continue
			}
		} else if choice.NoteID <= 0 || m.NoteID != choice.NoteID {
			continue
		}
		card := materialToCard(ctx, s.kb, m)
		return card, true
	}
	return SystemCard{}, false
}

// materialToCard собирает карточку печати из справочника и заметки.
//
// Описание берётся из справочника (его мог править человек), а ссылки и
// картинка — из заметки Базы знаний: у материала они живут там. Ошибка Базы
// знаний не мешает выбору: материал без ссылок честнее отказа.
func materialToCard(ctx context.Context, kb *KBClient, m Material) SystemCard {
	card := SystemCard{
		NoteID:      m.NoteID,
		SKU:         m.SKU,
		Title:       strings.TrimSpace(m.Name),
		Description: strings.TrimSpace(m.Description),
		Refs:        []skills.CardRef{},
	}
	if card.Title == "" {
		card.Title = strings.TrimSpace(m.NoteTitle)
	}
	if kb == nil || !kb.configured() || m.NoteID <= 0 {
		return card
	}
	note, err := kb.Note(ctx, m.NoteID)
	if err != nil {
		log.Printf("материал %d: заметка не прочитана: %v", m.NoteID, err)
		return card
	}
	parsed := skills.ParseSystemCard(note.ContentMD)
	if strings.TrimSpace(card.Description) == "" {
		card.Description = strings.TrimSpace(parsed.Description)
	}
	card.Composition = parsed.Composition
	card.Specifications = parsed.Specifications
	card.Refs = parsed.Refs
	card.ImageName = parsed.ImageName
	if withKey := attachImageKeys(ctx, kb, []SystemCard{card}); len(withKey) == 1 {
		card = withKey[0]
	}
	return card
}

// ComposeMaterialResponse — какой материал предложен и что о нём известно.
type ComposeMaterialResponse struct {
	Offer    Offer      `json:"offer"`
	Material SystemCard `json:"material"`
	// Title — название материала строкой: клиентам оно нужно для ответа
	// человеку, и вытаскивать его из карточки незачем.
	Title string `json:"title"`
	// Texts — описание, которое модель написала при выборе материала
	// (блок `material.description`). Сборка документа берёт его из таблицы.
	Texts map[string]string `json:"texts,omitempty"`
	// Missing — какие блоки не написались. Выбор сохранён: пустой блок честнее
	// черновика, а терять выбор из-за аварии модели нельзя.
	Missing []string `json:"missing,omitempty"`
}

// assembleMaterial отмечает предложение выбранным материалом.
//
// Материал ищется в справочнике (он же синхронизирован с карточками Базы
// знаний), поэтому один и тот же выбор принимается и по коду, и по заметке.
// Позиции НЕ дописываются: цена и количество — дело менеджера.
func (s *Server) assembleMaterial(ctx context.Context, offer Offer, choice Ref) (Offer, ComposeMaterialResponse, error) {
	if choice.Empty() {
		return offer, ComposeMaterialResponse{}, composeError{
			status: http.StatusBadRequest, text: "не выбран материал",
		}
	}
	materials, err := s.listMaterials(ctx, false)
	if err != nil {
		return offer, ComposeMaterialResponse{}, composeError{
			status: http.StatusInternalServerError, text: "не удалось прочитать материалы",
		}
	}
	card, found := s.materialCardForChoice(ctx, materials, choice)
	if !found {
		return offer, ComposeMaterialResponse{}, composeError{
			status: http.StatusNotFound, text: "материал не найден",
		}
	}
	if strings.TrimSpace(card.Title) == "" {
		return offer, ComposeMaterialResponse{}, composeError{
			status: http.StatusBadRequest, text: "у материала нет названия",
		}
	}
	before := len(offer.Materials)
	offer.Materials = offer.Materials.Add(Ref{SKU: card.SKU, NoteID: card.NoteID})
	resp := ComposeMaterialResponse{
		Offer:    offer,
		Material: card,
		Title:    card.Title,
	}
	alreadyChosen := len(offer.Materials) == before
	if err := s.persistOfferMaterials(ctx, offer); err != nil {
		return offer, resp, composeError{
			status: http.StatusInternalServerError, text: "не удалось сохранить предложение",
		}
	}
	if alreadyChosen {
		if text := s.storedTextsOfBlock(ctx, card, materialTextBlock); text != "" {
			resp.Texts = map[string]string{materialTextBlock: text}
		}
		return offer, resp, nil
	}
	// Набор материалов сменился — текст блоков «по промту» про прежний набор
	// больше не годится, кэш стирается при записи (как у систем).
	offer.FormValues = dropStaleBlockTexts(offer.FormValues)
	if err := s.persistOfferFormValues(ctx, offer); err != nil {
		log.Printf("add material: тексты блоков не сброшены у КП %s: %v", offer.Number, err)
	}
	resp.Offer = offer
	text, missing := s.reconcileMaterialTexts(ctx, offer, card)
	resp.Texts = map[string]string{materialTextBlock: text}
	resp.Missing = missing
	return offer, resp, nil
}

// persistOfferMaterials пишет выбранные материалы и двигает updated_at.
func (s *Server) persistOfferMaterials(ctx context.Context, offer Offer) error {
	raw, err := json.Marshal(offer.Materials)
	if err != nil {
		return err
	}
	return s.persistOfferColumn(ctx, offer.ID, "material_choice", raw)
}

// removeMaterial убирает материал из предложения по выбору.
func (s *Server) removeMaterial(w http.ResponseWriter, r *http.Request, offer Offer, choice Ref) {
	if choice.Empty() {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "не выбран материал"})
		return
	}
	// Снимающий может прислать и номер заметки, и код. Выбор в предложении
	// помнит ОБА значения (сервер пишет их при добавлении), а `Same` при
	// непустом коде сравнивает только код — поэтому по одной заметке он не
	// находит запись, у которой код уже есть. Дополняем выбор известными
	// значениями: снять можно любой из сторон.
	full := s.resolveMaterialChoice(r.Context(), offer, choice)
	offer.Materials = offer.Materials.Remove(full)
	if err := s.persistOfferMaterials(r.Context(), offer); err != nil {
		dbError(w, "remove material", err)
		return
	}
	writeJSON(w, http.StatusOK, ComposeMaterialResponse{Offer: offer})
}

// resolveMaterialChoice дополняет выбор материалом из предложения: у него
// известны и код, и заметка, и снятие идёт по обоим.
func (s *Server) resolveMaterialChoice(ctx context.Context, offer Offer, choice Ref) Ref {
	for _, existing := range offer.Materials {
		if existing.Same(choice) || sameRefCard(choice, SystemCard{NoteID: existing.NoteID, SKU: existing.SKU}) {
			return existing
		}
	}
	// Материала в предложении нет — снять нечего, писать нечего.
	if materials, err := s.listMaterials(ctx, false); err == nil {
		if card, found := s.materialCardForChoice(ctx, materials, choice); found {
			return Ref{SKU: card.SKU, NoteID: card.NoteID}
		}
	}
	return choice
}

// materialWriteHandler — общий разбор тела и ответ для добавления и удаления.
func (s *Server) materialWriteHandler(w http.ResponseWriter, r *http.Request, remove bool) {
	offer, ok := s.offerForEditor(w, r)
	if !ok {
		return
	}
	var req struct {
		SKU    string `json:"sku"`
		NoteID int64  `json:"noteId"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	choice := Ref{SKU: req.SKU, NoteID: req.NoteID}
	if remove {
		s.removeMaterial(w, r, offer, choice)
		return
	}
	_, resp, err := s.assembleMaterial(r.Context(), offer, choice)
	if err != nil {
		writeComposeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleAddMaterial — «Добавить материал» в редакторе плагина.
func (s *Server) handleAddMaterial(w http.ResponseWriter, r *http.Request) {
	s.materialWriteHandler(w, r, false)
}

// handleRemoveMaterial — убрать материал из предложения.
func (s *Server) handleRemoveMaterial(w http.ResponseWriter, r *http.Request) {
	s.materialWriteHandler(w, r, true)
}

// handleBotAddMaterial — то же для бота (X-Service-Secret, владелец — почта чата).
func (s *Server) handleBotAddMaterial(w http.ResponseWriter, r *http.Request) {
	s.botMaterialHandler(w, r, false)
}

// handleBotRemoveMaterial — убрать материал из КП через бота.
func (s *Server) handleBotRemoveMaterial(w http.ResponseWriter, r *http.Request) {
	s.botMaterialHandler(w, r, true)
}

func (s *Server) botMaterialHandler(w http.ResponseWriter, r *http.Request, remove bool) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		ChatID string `json:"chat_id"`
		SKU    string `json:"sku"`
		NoteID int64  `json:"noteId"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	offer, _, ok := s.botOfferForChatQ(w, r, id, req.ChatID)
	if !ok {
		return
	}
	choice := Ref{SKU: req.SKU, NoteID: req.NoteID}
	if remove {
		s.removeMaterial(w, r, offer, choice)
		return
	}
	_, resp, err := s.assembleMaterial(r.Context(), offer, choice)
	if err != nil {
		writeComposeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
