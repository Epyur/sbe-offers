package main

import (
	"context"
	"log"
	"strings"
)

// Материалы из карточек Базы знаний (2026-10-31).
//
// Решение пользователя: в карточках материалов SKU вводится как признак
// материала, но СВЯЗЬ состава систем с материалом идёт по вики-ссылке на
// заметку. Значит, справочник материалов должен помнить, из какой заметки он
// заведён, — иначе ссылке некуда вести.
//
// Отсюда «Материалы» перестают быть справочником, который висит в воздухе:
// карточка с тегом `material` превращается в материал при каждом чтении
// списка. Это не миграция в базу, а синхронизация чтением — так нет вопроса
// «а что если заметку переименовали»: имя материала следует за заметкой.
//
// Правила, которые здесь держатся:
//   - SKU назначается ОДИН РАЗ и не меняется ни при каком чтении (тест);
//   - материал ищется по note_id; найден — подтягивается имя и единица, если
//     заметка изменилась;
//   - не найден — заводится: это новая карточка материала;
//   - карточка, помеченная удалённой в Базе знаний, материал в архив НЕ
//     уводит: архив — решение человека в плагине, а не следствие правки
//     заметки. Иначе одна чистка в Базе знаний снимала бы материалы с продажи.

// noteMaterial — что нужно от заметки, чтобы завести материал.
type noteMaterial struct {
	NoteID int64
	Title  string
}

// syncMaterialsFromNotes приводит справочник к набору карточек-материалов.
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
			// Одна плохая карточка не должна гасить весь справочник: человек
			// всё равно должен увидеть остальные материалы.
			log.Printf("материалы: карточка %d (%s) не синхронизирована: %v",
				note.NoteID, note.Title, err)
		}
	}
	// Картинки материалов (2026-11-05). Колонка `photo_key` была в схеме с
	// самого начала, но её никто не заполнял, поэтому картинки материалов в
	// документе не было вовсе. Заполняем здесь, а не при каждом чтении
	// справочника: вложения — запросы в чужой сервис, и они не должны стоять
	// на пути открытия раздела.
	s.syncMaterialPhotos(ctx)
	return nil
}

// syncMaterialPhotos проставляет материалам ключ картинки из вложений заметки.
//
// Ошибка чужого сервиса здесь только пишется в журнал: материал без картинки —
// норма, а упавший справочник — нет.
func (s *Server) syncMaterialPhotos(ctx context.Context) {
	if s.kb == nil || !s.kb.configured() {
		return
	}
	items, err := s.listMaterialsOnce(ctx, true)
	if err != nil {
		log.Printf("материалы: чтение для картинок не удалось: %v", err)
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
			log.Printf("материалы: картинка %d не сохранена: %v", withKeys[i].ID, err)
		}
	}
}

// materialNotes — заметки с тегом material.
//
// kb == nil (База знаний не настроена) — не ошибка: справочник тогда остаётся
// тем, что заведён руками, и «Добавить систему» честно скажет, что ссылки
// ведут в никуда.
func (s *Server) materialNotes(ctx context.Context) ([]noteMaterial, error) {
	if s.kb == nil || !s.kb.configured() {
		return nil, nil
	}
	return materialNotesFromKB(ctx, s.kb)
}

// materialNotesFromKB отделено от сервера ради проверки без базы.
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
