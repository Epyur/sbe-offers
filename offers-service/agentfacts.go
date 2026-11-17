package main

// Инструмент агента «запомнить факт у листка» (2026-11-17, шаг 3e плана
// `2026-11-16-sbe-offers-combat-sections-and-agent-tools-plan.md`, §9.1).
//
// Зачем отдельный инструмент, если факт можно запомнить самим `fetch_url` /
// `parse_file`. Три причины, и все три — про границу:
//   - агент решает, ЧТО в листке факт, а что просто справка на один ход.
//     Ответ на «какая сегодня погода» не должен оседать в документах заказчика;
//   - источник называет модель (адрес или имя файла), сервер только
//     проверяет, что такой источник действительно был прочитан в этом ходе;
//   - запись факта — правка чужого листка, поэтому она идёт через ту же
//     проверку владельца, что и остальные записи агента.
//
// Инструмент НЕ читает сеть: он берёт уже добытое (`agentEnv.readSources`).
// Иначе модель могла бы «запомнить» в листке текст, которого никто не читал.

import (
	"context"
	"fmt"
	"strings"
)

// toolRememberSheetFact — запоминает добытый факт у боевого листка.
func (s *Server) toolRememberSheetFact() agentToolSpec {
	return agentToolSpec{
		Name: "remember_sheet_fact",
		Description: "Запомнить добытый факт (ссылку, файл, письмо) у боевого листка. " +
			"После этого факт попадает в разделы листка при следующей сборке: " +
			"если прочитанная ссылка или файл важны для сравнения — запомни их. " +
			"Источник обязан быть прочитан В ЭТОМ ходе (fetch_url, parse_file, read_email). " +
			"Пиши только то, что относится к спору с заказчиком; справка на один ход не нужна.",
		InputSchema: objectSchema(
			[]string{"sheetId", "source"},
			map[string]any{
				"sheetId": intProp("Номер боевого листка (id из create_combat_sheet)"),
				"source":  stringProp("Откуда факт: адрес ссылки или имя приложенного файла"),
				"note":    stringProp("Короткая подпись факта (необязательно, её видит человек)"),
			},
		),
		Run: func(ctx context.Context, env *agentEnv, args map[string]any) (agentToolResult, error) {
			sheetID := argInt(args, "sheetId")
			if sheetID <= 0 {
				return errTool("нужен sheetId — номер боевого листка")
			}
			source := argString(args, "source")
			if source == "" {
				return errTool("нужен source — адрес ссылки или имя файла")
			}
			read, ok := env.readSources[foldText(source)]
			if !ok {
				return errTool("источник <%s> в этом ходе не читался: сначала прочитай ссылку "+
					"или файл, потом запоминай факт", source)
			}
			if msg := agentSheetWriteError(ctx, env, sheetID); msg != "" {
				return errTool("%s", msg)
			}
			text := read.text
			if note := argString(args, "note"); note != "" {
				text = note + "\n" + text
			}
			fact, ok := newCombatFact(read.kind, read.source, text)
			if !ok {
				return errTool("в источнике <%s> нет текста — запоминать нечего", read.source)
			}
			if _, _, err := env.Server.persistCombatFacts(ctx, sheetID, fact); err != nil {
				return errTool("факт не сохранён у листка: %v", err)
			}
			env.noteFix("form_values", sheetID, nil, fact.label())
			return agentToolResult{
				OK:      true,
				Summary: fmt.Sprintf("Факт <%s> записан у листка %d.", fact.label(), sheetID),
				Data:    map[string]any{"sheetId": sheetID, "fact": fact.Source, "kind": fact.Kind},
			}, nil
		},
	}
}

// agentSheetWriteError — можно ли агенту писать в этот листок.
//
// Проверка та же, что у остальных записей: автор или админ. Разница
// «листок есть, но не твой» и «листка нет» важна и модели, и человеку.
func agentSheetWriteError(ctx context.Context, env *agentEnv, sheetID int64) string {
	if env == nil || env.Server == nil {
		return "записать факт некуда: сервер недоступен"
	}
	// Ниль-пул проверяется ДО чтения: `loadCombatSheet` без базы падает паникой
	// (`pgxpool` Acquire на nil), а стенд и юнит-тесты живут без Postgres.
	if env.Server.pool == nil {
		return "записать факт некуда: база недоступна"
	}
	sheet, err := env.Server.loadCombatSheet(ctx, sheetID)
	if err != nil {
		return fmt.Sprintf("боевой листок %d не найден", sheetID)
	}
	if !strings.EqualFold(sheet.OwnerEmail, env.Email) {
		role, _ := env.Server.effectiveRole(ctx, appIDFromEnv(), env.Email)
		if !isAdminRole(role) {
			return "боевой листок создан другим сотрудником: факты в него пишет только автор"
		}
	}
	return ""
}
