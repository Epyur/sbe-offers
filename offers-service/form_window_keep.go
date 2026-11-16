package main

// Защита настройки окна от старого клиента (2026-11-16, после совещания
// с `gemini-4-argon` и `claude-5-opus`).
//
// Дефект, который нашёл `gemini`: `handleSaveForm` пишет окно целиком
// (`window_config = EXCLUDED.window_config`). Клиент, не знающий зоны `brief`
// (плагин 0.11.0, закэшированный бандл приложения), присылает окно БЕЗ неё —
// и включённая администратором зона молча откатывается. Следующее чтение
// дописывает её невидимой, и человек видит «настройка не сохранилась».
//
// Правило восстановления узкое, чтобы не тащить в сохранение чужое состояние:
//   - зона известна серверу и она ВИДИМА в базе;
//   - клиент её не прислал ВООБЩЕ (не «прислал скрытой» — явное «скрыть»
//     уважается и сохраняется);
//   - тогда берём запись из базы.
//
// Порядок: восстановленная зона встаёт на своё место в базе относительно
// присланного (`insertAfterAnchor`), а не в конец — иначе «Бриф» переезжал бы
// вниз при каждом сохранении из старого клиента.

import (
	"context"
	"encoding/json"
)

// mergeStoredZones — ЧИСТАЯ часть: не теряет включённые зоны, которых нет во
// входе. Вынесена из `keepKnownZonesFromDB`, чтобы правило проверялось без
// базы (brief_zone_test.go).
func mergeStoredZones(incoming, stored []WindowItem) []WindowItem {
	if len(incoming) == 0 {
		// Пустое окно — законная настройка «как сейчас». Не подмешиваем сюда
		// прошлое состояние: администратор сознательно его сбросил.
		return incoming
	}
	sent := make(map[string]bool, len(incoming))
	for _, item := range incoming {
		sent[item.Kind+"\x00"+item.ID] = true
	}
	out := incoming
	for _, zone := range knownWindowZones {
		key := windowKindZone + "\x00" + zone
		if sent[key] {
			continue
		}
		var kept *WindowItem
		for i := range stored {
			if stored[i].Kind == windowKindZone && stored[i].ID == zone {
				kept = &stored[i]
				break
			}
		}
		if kept == nil || !kept.Visible {
			// В базе зона скрыта или её там нет — дописывать нечего: сервер
			// и так допишет её невидимой на чтении.
			continue
		}
		out = insertAfterAnchor(out, *kept, zone)
	}
	return out
}

// keepKnownZonesFromDB читает прежнее окно из базы и применяет
// `mergeStoredZones`. Ошибка чтения не мешает сохранению: хуже потерять окно,
// чем отказать в сохранении формы (правило «ошибка хранилища не отменяет
// работу»).
func keepKnownZonesFromDB(ctx context.Context, s *Server, mode string, incoming []WindowItem) []WindowItem {
	if len(incoming) == 0 {
		return incoming
	}
	var raw []byte
	if err := s.pool.QueryRow(ctx,
		`SELECT window_config FROM offers_form WHERE mode = $1`, mode).Scan(&raw); err != nil {
		return incoming
	}
	if len(raw) == 0 {
		return incoming
	}
	var stored []WindowItem
	if err := json.Unmarshal(raw, &stored); err != nil {
		return incoming
	}
	return mergeStoredZones(incoming, stored)
}

// insertAfterAnchor ставит восстановленную зону рядом с её якорем (как
// `appendMissingZones`), а без якоря — в конец.
func insertAfterAnchor(items []WindowItem, item WindowItem, zone string) []WindowItem {
	if anchor := zoneAnchorAfter(zone); anchor != "" {
		if at := indexOfZone(items, anchor); at >= 0 {
			return append(items[:at+1], append([]WindowItem{item}, items[at+1:]...)...)
		}
	}
	return append(items, item)
}
