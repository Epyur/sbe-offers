package main

// Конкуренты из файла и ссылки (2026-11-17).
//
// Менеджер читает страницу конкурента или присылает файл, и прочитанное
// становится конкурентом листка БЕЗ карточки Базы знаний. Модель данных —
// отдельная колонка `text_competitors` (см. `CombatTextRef`), а здесь — то,
// что нужно печати: превратить текстовые записи в карточки и отдать обе
// стороны листка одним вызовом.
//
// Почему одна функция на обе стороны (`combatSideCards`): сегодня карточки
// собираются из кеша в трёх местах — `handleExportCombat` (JWT),
// `handleBotExportCombat` (бот) и `appanswer.go` («отправить в чат»). Второй
// способ сборки, добавленный в одно из них, дал бы документ, который в чате
// печатается иначе, чем в приложении. Текстовые конкуренты обязаны появляться
// во всех трёх одинаково.

import (
	"context"
	"strings"
)

// textCompetitorSKU — устойчивый код текстового конкурента.
//
// Код нужен трём местам, которые различают карточки по коду или заметке:
// ключ кэша подраздела (`systemCacheHandle` → `.sys<handle>`), дедупликация
// `containsCard` и поиск `FindSystem` в `combatSectionOffer`. Без кода все
// текстовые конкуренты получили бы один ключ `.sys0` и перезаписывали бы
// тексты друг друга (находка `gemini-4-argon`, 2026-11-17).
func textCompetitorSKU(ref CombatTextRef, index int) string {
	key := strings.TrimSpace(ref.Key)
	if key == "" {
		// Ключа нет только у испорченной записи; порядковый номер оставляет её
		// различимой, вместо того чтобы молча склеить с соседней.
		key = strings.TrimSpace(ref.Title) + ":" + itoa(index)
	}
	return "text-" + hashShort(key)
}

// textCompetitorCards — карточки для печати из текстовых записей листка.
//
// Заполнено ровно то, что видит читатель: название и прочитанный текст
// (он печатается как описание). Состава и характеристик у такой записи нет —
// их не из чего взять, и выдумывать их нельзя.
func textCompetitorCards(refs []CombatTextRef) []SystemCard {
	out := make([]SystemCard, 0, len(refs))
	for i, ref := range refs {
		title := strings.TrimSpace(ref.Title)
		text := strings.TrimSpace(ref.Text)
		if title == "" || text == "" {
			continue
		}
		out = append(out, SystemCard{
			Title:          title,
			Description:    text,
			HasDescription: true,
			SKU:            textCompetitorSKU(ref, i),
		})
	}
	return out
}

// combatSideCards — стороны листка, готовые к печати.
//
// Наш продукт — только карточки Базы знаний (у нашей стороны текстовых записей
// нет по модели). Конкуренты — карточки из кеша (в порядке выбора) плюс
// текстовые записи (в порядке добавления).
func (s *Server) combatSideCards(ctx context.Context, sheet CombatSheet) ([]SystemCard, []SystemCard) {
	our := s.resolveCombatCards(ctx, sheet.OurNotes, s.loadCachedSystemCards)
	comp := s.resolveCombatCards(ctx, sheet.CompetitorNotes,
		func(ctx context.Context) ([]SystemCard, error) {
			cards, _, err := s.loadCachedCompetitors(ctx)
			return cards, err
		})
	if len(sheet.TextCompetitors) == 0 {
		return our, comp
	}
	merged := make([]SystemCard, 0, len(comp)+len(sheet.TextCompetitors))
	merged = append(merged, comp...)
	merged = append(merged, textCompetitorCards(sheet.TextCompetitors)...)
	return our, merged
}

// addCombatTextRef дописывает текстового конкурента.
//
// Тот же ключ (тот же адрес, тот же файл) ЗАМЕНЯЕТ запись, а не плодит
// вторую: повторное «Прочитать» после правки страницы должно обновить текст,
// иначе предел в 12 записей кончится за три нажатия (то же правило, что у
// фактов листка, шаг 3e).
func addCombatTextRef(refs []CombatTextRef, ref CombatTextRef) []CombatTextRef {
	key := strings.TrimSpace(ref.Key)
	out := make([]CombatTextRef, 0, len(refs)+1)
	for _, existing := range refs {
		if strings.TrimSpace(existing.Key) == key {
			continue
		}
		out = append(out, existing)
	}
	out = append(out, ref)
	if len(out) > combatTextLimit {
		out = out[len(out)-combatTextLimit:]
	}
	return out
}

// removeCombatTextRef убирает запись по ключу. Отсутствующей нет — список не
// меняется (как `removeNoteRef`).
func removeCombatTextRef(refs []CombatTextRef, key string) []CombatTextRef {
	want := strings.TrimSpace(key)
	out := make([]CombatTextRef, 0, len(refs))
	for _, ref := range refs {
		if strings.TrimSpace(ref.Key) != want {
			out = append(out, ref)
		}
	}
	return out
}

// keepStoredTextCompetitors — правило «пусто = не трогали» для полного
// сохранения листка (2026-11-17).
//
// Плагин Obsidian и старая сборка приложения про текстовых конкурентов не
// знают и присылают листок без них. Полное сохранение пишет колонку целиком,
// поэтому без этого правила первое же «Сохранить» из плагина стёрло бы
// прочитанный текст вместе с записью. Та же логика, что у `system_choice` КП.
// Удаление — своей ручкой (`handleCombatCompetitorTextDelete`), поэтому
// «пусто» здесь честно означает «клиент это поле не трогал».
func keepStoredTextCompetitors(incoming, stored []CombatTextRef) []CombatTextRef {
	if len(incoming) > 0 {
		return incoming
	}
	return stored
}

// newCombatTextRef собирает запись: текст обрезается тем же пределом, что и
// прочитанное сетью (`compactFetchTextChars`).
func newCombatTextRef(key, title, text, source string) CombatTextRef {
	return CombatTextRef{
		Key:    strings.TrimSpace(key),
		Title:  strings.TrimSpace(title),
		Text:   truncateText(strings.TrimSpace(text), combatTextTextChars),
		Source: source,
	}
}
