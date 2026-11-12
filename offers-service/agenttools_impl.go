package main

// Исполнители инструментов агента КП (2026-11-14, шаг P6).
//
// Каждый инструмент — тонкая обёртка над готовой функцией сервиса. Здесь нет
// второй композиции, второго поиска, второго рендера: если функция делает то же
// для кнопки, агент зовёт ЕЁ (правило «одно решение в одном месте»). Поэтому
// некоторые инструменты собирают `*http.Request` вокруг той же функции, которой
// отвечает ручка — иначе рендер шёл бы другим путём (без журнала сборки, без
// таймаута работы) и расходился бы с кнопкой молча.
//
// Права. Гейт человека — в транспорте (`requirePerm`/`requireServiceSecret`).
// Здесь проверка одна и та же, что у ручек: «своё предложение или admin». Она
// нужна, потому что агент может назвать id чужого КП — и без неё получил бы
// чужой документ либо переписал бы чужую строку.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"mailers/offers/skills"
)

// ============================ продукты ============================

// toolGetOurProducts — поиск по нашим системам и материалам (что предложить).
//
// Зовёт `suggestionsFor` — ту же функцию, что бот использует в `/bot/suggest` и
// в подсказке брифа. Ищет по всей Базе знаний (морфология, префиксы), поэтому
// название не обязано совпадать дословно; агент именно этим и пользуется, чтобы
// заметить «такой системы нет, а вот ближайшие».
func (s *Server) toolGetOurProducts() agentToolSpec {
	return agentToolSpec{
		Name: "get_our_products",
		Description: "Ищет наши системы и материалы по названию или свободной фразе менеджера. " +
			"Возвращает найденные системы (noteId, title) и материалы (noteId, sku, name, unit, вилка цены). " +
			"Используй первым на любой запрос про продукт: сначала найди карточку, потом читай её или добавляй.",
		InputSchema: objectSchema(
			[]string{"query"},
			map[string]any{
				"query": stringProp("Название или фраза, например «мембрана ПВХ» или «ТН-КРОВЛЯ Стандарт»"),
			},
		),
		Run: func(ctx context.Context, env *agentEnv, args map[string]any) (agentToolResult, error) {
			query := argString(args, "query")
			if query == "" {
				return errTool("нужен аргумент query — название или фраза для поиска")
			}
			systems, materials, err := env.Server.suggestionsFor(ctx, query)
			if err != nil {
				return errTool("поиск недоступен: %v", err)
			}
			if len(systems) == 0 && len(materials) == 0 {
				return agentToolResult{
					OK:      true,
					Summary: fmt.Sprintf("По запросу «%s» ничего не нашлось.", query),
					Data:    map[string]any{"systems": []any{}, "materials": []any{}},
				}, nil
			}
			return agentToolResult{
				OK: true,
				Summary: fmt.Sprintf("Нашлось систем: %d, материалов: %d по запросу «%s».",
					len(systems), len(materials), query),
				Data: map[string]any{"systems": systems, "materials": materials},
			}, nil
		},
	}
}

// cardView — карточка для модели: то, что она может прочитать, а не вся
// структура SystemCard. Состав и проблема уходят массивом строк — модель
// пересказывает их словами, а не угадывает поля.
//
// `Role` — что это за карточка по тегам: system / material / competitor / other.
// Модель получает роль бесплатно, но решают не она, а инструменты действия:
// `set_system` откажет материалу, `set_material` — системе и конкуренту. Здесь
// поле нужно, чтобы модель сразу понимала, какой инструмент звать, и не звала
// `set_material` для карточки конкурента.
type cardView struct {
	NoteID      int64    `json:"noteId"`
	SKU         string   `json:"sku,omitempty"`
	Title       string   `json:"title"`
	Role        string   `json:"role"`
	Description string   `json:"description"`
	Composition []string `json:"composition,omitempty"`
	Problems    []string `json:"problems,omitempty"`
	Links       []string `json:"links,omitempty"`
}

// cardRole — роль карточки по тегам заметки (согласованная модель 2026-11-14):
// `commercial_propose` без `material` — система, с `material` — материал,
// `competitor` — продукт конкурента, всё остальное — прочий текст.
//
// Тег `material` без `commercial_propose` тоже материал: в живых заметках
// материал и так помечен обоими, но роль не должна зависеть от того, поставил
// ли человек второй тег. Конкурент проверяется ПЕРВЫМ: карточка конкурента не
// должна попасть на нашу сторону, какой бы ещё тег на ней ни стоял (индексатор
// иногда ставит на такие заметки и `commercial_propose`).
func cardRole(tags []KBTag) string {
	note := KBNoteSummary{Tags: tags}
	switch {
	case noteHasTag(note, competitorTagName):
		return "competitor"
	case noteHasTag(note, materialTagName):
		return "material"
	case noteHasTag(note, commercialPropose):
		return "system"
	}
	return "other"
}

// readNoteCard читает ЛЮБУЮ заметку в карточку — одна функция чтения на любой
// noteId (2026-11-14, P6-бис).
//
// Почему не `liveSystemCard`: тот собирает карточки из ленты систем
// (`SystemsFromKB`) и материалы отбрасывает по тегу НАМЕРЕННО. Из-за этого
// `read_card` по материалу отвечал «карточки нет», хотя заметка читается
// (живой дефект 0.9.22: менеджер получил ответ, что карточки LOGICROOF V-RP и
// ГЕОПРУФ «не открываются в Базе знаний» — они открывались). Чтение не должно
// зависеть от роли заметки; роль проверяют инструменты действия.
func (s *Server) readNoteCard(ctx context.Context, noteID int64) (SystemCard, []KBTag, error) {
	if s.kb == nil || !s.kb.configured() {
		return SystemCard{}, nil, errKBNotConfigured
	}
	note, err := s.kb.Note(ctx, noteID)
	if err != nil {
		return SystemCard{}, nil, err
	}
	summary := KBNoteSummary{ID: note.ID, Title: note.Title, Path: note.Path, Tags: note.Tags}
	card := systemCardFromParsed(summary, note, skills.ParseSystemCard(note.ContentMD))
	if keyed := attachImageKeys(ctx, s.kb, []SystemCard{card}); len(keyed) == 1 {
		card = keyed[0]
	}
	return card, note.Tags, nil
}

// toolReadCard — полный текст карточки Базы знаний по номеру заметки.
//
// Читается ЛЮБАЯ заметка: система, материал, продукт конкурента, просто текст.
// Тег не проверяется — это чтение (модель 2026-11-14: «одна ось, карточка —
// любой noteId»). Роль карточки уходит в ответ (`role`), чтобы модель знала,
// какой инструмент действия звать дальше.
func (s *Server) toolReadCard() agentToolSpec {
	return agentToolSpec{
		Name: "read_card",
		Description: "Читает карточку Базы знаний по её noteId — ЛЮБУЮ заметку: систему, материал, продукт конкурента, " +
			"обычный текст. Возвращает название, роль (system/material/competitor/other), описание, состав, ссылки. " +
			"Используй после get_our_products или search_competitors, чтобы понять, что именно можно предложить.",
		InputSchema: objectSchema(
			[]string{"noteId"},
			map[string]any{
				"noteId": intProp("Номер заметки Базы знаний (noteId из get_our_products / search_competitors)"),
			},
		),
		Run: func(ctx context.Context, env *agentEnv, args map[string]any) (agentToolResult, error) {
			noteID := argInt(args, "noteId")
			if noteID <= 0 {
				return errTool("нужен аргумент noteId — номер заметки")
			}
			card, tags, err := env.Server.readNoteCard(ctx, noteID)
			if err != nil {
				if err == errKBNotConfigured {
					return errTool("База знаний не настроена: карточки недоступны")
				}
				return errTool("карточки с noteId %d нет в Базе знаний", noteID)
			}
			role := cardRole(tags)
			view := cardView{
				NoteID:      card.NoteID,
				SKU:         card.SKU,
				Title:       card.Title,
				Role:        role,
				Description: card.Description,
				Composition: card.Composition,
				Links:       cardLinkLabels(card),
			}
			return agentToolResult{
				OK:      true,
				Summary: fmt.Sprintf("Карточка «%s» (заметка %d, роль: %s).", card.Title, card.NoteID, role),
				Data:    view,
			}, nil
		},
	}
}

// cardLinkLabels — подписи ссылок карточки без адресов: читателю (и модели)
// нужна подпись, адрес в модель не идёт (грабля 0.9.4: URL попадал на страницу).
func cardLinkLabels(card SystemCard) []string {
	out := make([]string, 0, len(card.Refs))
	for _, ref := range card.Refs {
		if label := strings.TrimSpace(ref.Label); label != "" {
			out = append(out, label)
		}
	}
	return out
}

// ============================ документы ============================

// toolGetDocuments — документы общего доступа из документов.
//
// Отдаёт только то, что вернула внутренняя ручка `documents-service`
// `/internal/documents/agent/list` (P5a): документы с непустыми `product_refs`,
// не в архиве, без `no_agent`. Фильтр живёт в ЧУЖОЙ ручке, а не в промте
// (правило §4.4 дизайна: «инструмент — граница»). Если сервис документов не
// настроен — честный отказ, а не пустой список: пустой список агент пересказал
// бы как «документов нет».
func (s *Server) toolGetDocuments() agentToolSpec {
	return agentToolSpec{
		Name: "get_documents",
		Description: "Ищет документы, привязанные к продуктам Базы знаний (протоколы, сертификаты, описания). " +
			"Возвращает только документы общего доступа. Документы без привязки к продукту недоступны намеренно.",
		InputSchema: objectSchema(
			[]string{"query"},
			map[string]any{
				"query": stringProp("Слово из названия документа; пусто — весь доступный набор"),
				"limit": intProp("Сколько документов вернуть (по умолчанию 20, максимум 200)"),
			},
		),
		Run: func(ctx context.Context, env *agentEnv, args map[string]any) (agentToolResult, error) {
			client := NewDocumentsAgentClient()
			if !client.configured() {
				return errTool("сервис документов не настроен: документы через агента недоступны")
			}
			limit := int(argInt(args, "limit"))
			docs, err := client.List(ctx, argString(args, "query"), limit)
			if err != nil {
				return errTool("документы недоступны: %v", err)
			}
			if len(docs.Items) == 0 {
				return agentToolResult{
					OK:      true,
					Summary: "Документов, привязанных к продуктам, не нашлось.",
					Data:    map[string]any{"items": []any{}, "total": 0},
				}, nil
			}
			return agentToolResult{
				OK:      true,
				Summary: fmt.Sprintf("Нашлось документов: %d.", docs.Total),
				Data:    docs,
			}, nil
		},
	}
}

// toolGetDocumentLink — временная ссылка на файл документа. Отдаётся кнопкой.
func (s *Server) toolGetDocumentLink() agentToolSpec {
	return agentToolSpec{
		Name: "get_document_link",
		Description: "Даёт временную ссылку на файл документа (PDF, Word и т.п.) по его file_key из get_documents. " +
			"Ссылка показывается менеджеру кнопкой.",
		InputSchema: objectSchema(
			[]string{"key"},
			map[string]any{
				"key": stringProp("Ключ файла документа (поле file_key из get_documents)"),
			},
		),
		Run: func(ctx context.Context, env *agentEnv, args map[string]any) (agentToolResult, error) {
			key := argString(args, "key")
			if key == "" {
				return errTool("нужен аргумент key — ключ файла документа")
			}
			client := NewDocumentsAgentClient()
			if !client.configured() {
				return errTool("сервис документов не настроен")
			}
			url, err := client.FileLink(ctx, key)
			if err != nil {
				return errTool("ссылка на файл недоступна: %v", err)
			}
			return agentToolResult{
				OK:      true,
				Summary: "Ссылка на файл документа готова.",
				Link:    &agentLink{URL: url, Label: "Открыть документ"},
			}, nil
		},
	}
}

// toolReadDocumentFile — текст файла документа.
//
// Читает байты тем же внутренним путём. В модель уходит только текст (для
// PDF/DOCX — извлечённый), а не base64: base64 не читается человеком и раздувает
// контекст. Не удалось извлечь текст (скан без OCR) — честно говорим это.
func (s *Server) toolReadDocumentFile() agentToolSpec {
	return agentToolSpec{
		Name: "read_document_file",
		Description: "Читает содержимое файла документа по его file_key. Возвращает текст, если его удаётся извлечь. " +
			"Скан без текстового слоя вернёт пустой текст — тогда откройте документ ссылкой.",
		InputSchema: objectSchema(
			[]string{"key"},
			map[string]any{
				"key": stringProp("Ключ файла документа (поле file_key из get_documents)"),
			},
		),
		Run: func(ctx context.Context, env *agentEnv, args map[string]any) (agentToolResult, error) {
			key := argString(args, "key")
			if key == "" {
				return errTool("нужен аргумент key — ключ файла документа")
			}
			client := NewDocumentsAgentClient()
			if !client.configured() {
				return errTool("сервис документов не настроен")
			}
			text, err := client.ReadText(ctx, key)
			if err != nil {
				return errTool("файл документа недоступен: %v", err)
			}
			if strings.TrimSpace(text) == "" {
				return agentToolResult{
					OK:      true,
					Summary: "В файле нет текстового слоя (скан). Откройте его ссылкой.",
					Data:    map[string]any{"text": ""},
				}, nil
			}
			return agentToolResult{
				OK:      true,
				Summary: fmt.Sprintf("Текст файла прочитан, символов: %d.", len(text)),
				Data:    map[string]any{"text": text},
			}, nil
		},
	}
}

// ============================ конкуренты и боевой листок ============================

// toolSearchCompetitors — поиск продуктов конкурентов в Базе знаний.
//
// Зовёт `searchCompetitors` — ту же функцию, что бот-поллер для стороны листка
// (тег `competitor`, полнотекстовый поиск Базы знаний). Свой поиск здесь был бы
// третьей копией правила, и на стенде прошёл бы, а на живых названиях — нет
// (живая жалоба 2026-11-14: подстрокой не находились ни ФАХМАНН, ни Sikaplan®).
func (s *Server) toolSearchCompetitors() agentToolSpec {
	return agentToolSpec{
		Name: "search_competitors",
		Description: "Ищет продукты конкурентов по названию или фразе (База знаний, тег «конкурент»). " +
			"Возвращает список (noteId, title). Отсюда берётся сторона конкурента боевого листка; читать карточку — read_card.",
		InputSchema: objectSchema(
			[]string{"query"},
			map[string]any{
				"query": stringProp("Название продукта конкурента или фраза, например «мембрана ПВХ»"),
			},
		),
		Run: func(ctx context.Context, env *agentEnv, args map[string]any) (agentToolResult, error) {
			query := argString(args, "query")
			if query == "" {
				return errTool("нужен аргумент query — название или фраза для поиска конкурентов")
			}
			items, err := env.Server.searchCompetitors(ctx, query)
			if err != nil {
				return errTool("поиск конкурентов недоступен: %v", err)
			}
			if len(items) == 0 {
				return agentToolResult{
					OK:      true,
					Summary: fmt.Sprintf("Конкурентов по запросу «%s» не нашлось.", query),
					Data:    map[string]any{"competitors": []any{}},
				}, nil
			}
			return agentToolResult{
				OK:      true,
				Summary: fmt.Sprintf("Нашлось конкурентов: %d по запросу «%s».", len(items), query),
				Data:    map[string]any{"competitors": items},
			}, nil
		},
	}
}

// toolCreateCombatSheet — завести пустой боевой листок (как кнопка «Боевой
// листок» в боте). Листок создаётся пустым и получает номер сразу; стороны
// добавляются `set_combat_side` (тот же порядок, что в боте).
func (s *Server) toolCreateCombatSheet() agentToolSpec {
	return agentToolSpec{
		Name: "create_combat_sheet",
		Description: "Заводит новый боевой листок (сравнение нашего продукта с конкурентами) и возвращает его id и номер. " +
			"Стороны добавляй после этого инструментом set_combat_side, готовый документ — build_combat_docx.",
		InputSchema: objectSchema(
			[]string{},
			map[string]any{
				"title": stringProp("Название листка; пусто — «Боевой листок»"),
			},
		),
		Run: func(ctx context.Context, env *agentEnv, args map[string]any) (agentToolResult, error) {
			title := argString(args, "title")
			if title == "" {
				title = defaultCombatTitle
			}
			sheet, err := env.createCombatSheet(ctx, title)
			if err != nil {
				return errTool("листок не создан: %v", err)
			}
			return agentToolResult{
				OK:      true,
				Summary: fmt.Sprintf("Боевой листок %s создан (id %d).", sheet.Number, sheet.ID),
				Data: map[string]any{
					"sheetId":         sheet.ID,
					"number":          sheet.Number,
					"ourNotes":        sheet.OurNotes,
					"competitorNotes": sheet.CompetitorNotes,
				},
			}, nil
		},
	}
}

// toolSetCombatSide — добавить карточку на сторону боевого листка.
//
// Граница — ЗДЕСЬ, а не в промте (модель 2026-11-14): нашу сторону получает
// только карточка `commercial_propose` (система или материал), сторону
// конкурента — только карточка `competitor`. Материал нельзя положить нашей
// стороной, а конкурента — нашей: это бессмысленный документ, и модель не
// должна получить возможность его собрать. Роль читается вживую из заметки,
// а не из аргумента модели: иначе модель называла бы роль сама.
func (s *Server) toolSetCombatSide() agentToolSpec {
	return agentToolSpec{
		Name: "set_combat_side",
		Description: "Добавляет продукт на сторону боевого листка (или убирает его). " +
			"Сторона our — только наши системы и материалы; сторона competitor — только продукты конкурентов. " +
			"Листок должен уже существовать (sheetId из create_combat_sheet).",
		InputSchema: objectSchema(
			[]string{"sheetId", "noteId", "side"},
			map[string]any{
				"sheetId": intProp("Номер боевого листка (id из create_combat_sheet)"),
				"noteId":  intProp("Номер заметки Базы знаний"),
				"side":    stringProp("Сторона листка: our (наш продукт) или competitor (продукт конкурента)"),
				"remove":  map[string]any{"type": "boolean", "description": "true — убрать фото со стороны, а не добавить"},
			},
		),
		Run: func(ctx context.Context, env *agentEnv, args map[string]any) (agentToolResult, error) {
			sheetID := argInt(args, "sheetId")
			if sheetID <= 0 {
				return errTool("нужен аргумент sheetId — номер боевого листка")
			}
			noteID := argInt(args, "noteId")
			if noteID <= 0 {
				return errTool("нужен аргумент noteId — номер заметки продукта")
			}
			side := argString(args, "side")
			if side != combatSideOur && side != combatSideCompetitor {
				return errTool("аргумент side — «our» или «competitor»")
			}
			card, tags, err := env.Server.readNoteCard(ctx, noteID)
			if err != nil {
				return errTool("карточки с noteId %d нет в Базе знаний", noteID)
			}
			if msg := combatSideRoleError(side, cardRole(tags)); msg != "" {
				return errTool("%s", msg)
			}
			remove := argBool(args, "remove")
			sheet, err := env.writeCombatSide(ctx, sheetID, side, CombatNoteRef{NoteID: noteID, Title: card.Title}, remove)
			if err != nil {
				return errTool("%v", err)
			}
			verb := "добавлен на"
			if remove {
				verb = "убран с"
			}
			return agentToolResult{
				OK:      true,
				Summary: fmt.Sprintf("«%s» %s сторону %s листка %s.", card.Title, verb, sideLabel(side), sheet.Number),
				Data: map[string]any{
					"sheetId":         sheet.ID,
					"number":          sheet.Number,
					"ourNotes":        sheet.OurNotes,
					"competitorNotes": sheet.CompetitorNotes,
				},
			}, nil
		},
	}
}

// combatSideRoleError — почему карточка не годится для этой стороны. Пустая
// строка — годится. Тексты — для человека, который прочтёт их в ответе агента.
func combatSideRoleError(side, role string) string {
	if side == combatSideOur {
		switch role {
		case "system", "material":
			return ""
		case "competitor":
			return "это карточка конкурента — её место на стороне competitor, а не our"
		}
		return "на сторону our идут только наши системы и материалы (коммерческое предложение)"
	}
	if role == "competitor" {
		return ""
	}
	return "на сторону competitor идут только продукты конкурентов (тег «competitor»)"
}

// sideLabel — сторона по-русски для сводки.
func sideLabel(side string) string {
	if side == combatSideOur {
		return "нашу"
	}
	return "конкурентную"
}

// ============================ правки документа ============================

// toolCreateOffer — открыть коммерческое предложение (сценарий B, P6-бис).
//
// Без этого агент без `offerId` упирался в «предложение не найдено» и человек
// уходил из чата ни с чем. Бот открывает строку в начале диалога; агент
// открывает её, когда она понадобилась. Состав пустой — цены и позиции ведёт
// менеджер (то же правило 2026-11-03, что у `set_system`).
func (s *Server) toolCreateOffer() agentToolSpec {
	return agentToolSpec{
		Name: "create_offer",
		Description: "Открывает новое коммерческое предложение и возвращает его id и номер. " +
			"Позиций и цен это НЕ добавляет: дальше — set_system / set_material, потом build_docx.",
		InputSchema: objectSchema(
			[]string{},
			map[string]any{
				"clientName": stringProp("Наименование заказчика, если менеджер его назвал"),
			},
		),
		Run: func(ctx context.Context, env *agentEnv, args map[string]any) (agentToolResult, error) {
			offer, err := env.createOfferRow(ctx, OfferClient{Name: argString(args, "clientName")})
			if err != nil {
				return errTool("предложение не создано: %v", err)
			}
			return agentToolResult{
				OK:      true,
				Summary: fmt.Sprintf("Коммерческое предложение %s открыто (id %d).", offer.Number, offer.ID),
				Data: map[string]any{
					"offerId":    offer.ID,
					"number":     offer.Number,
					"clientName": offer.Client.Name,
				},
			}, nil
		},
	}
}

// toolSetSystem — добавить систему в предложение (описанием, без цены).
//
// Зовёт `assembleSystem` — ту же функцию, что «Добавить систему» в редакторе и
// `система …` в боте. Позиций не создаёт (правило 2026-11-03): КП получает
// описание и ссылки, цены вводит менеджер.
func (s *Server) toolSetSystem() agentToolSpec {
	return agentToolSpec{
		Name: "set_system",
		Description: "Добавляет систему в коммерческое предложение по её noteId. В документ попадут описание и ссылки системы; " +
			"позиции с ценами это НЕ добавляет. Предложение должно уже существовать (offerId).",
		InputSchema: objectSchema(
			[]string{"offerId", "noteId"},
			map[string]any{
				"offerId": intProp("Номер предложения (id из offers_offer)"),
				"noteId":  intProp("Номер заметки системы в Базе знаний"),
			},
		),
		Run: func(ctx context.Context, env *agentEnv, args map[string]any) (agentToolResult, error) {
			offer, ok := env.offerForWrite(ctx, argInt(args, "offerId"))
			if !ok {
				return errTool("предложение не найдено или нет прав на его правку")
			}
			noteID := argInt(args, "noteId")
			if noteID <= 0 {
				return errTool("нужен аргумент noteId — номер заметки системы")
			}
			before := append(OfferSystems{}, offer.Systems...)
			updated, resp, err := env.Server.assembleSystem(ctx, offer, SystemChoice{NoteID: noteID})
			if err != nil {
				return errTool("%v", err)
			}
			env.noteFix("offer_system", offer.ID, before, updated.Systems)
			return agentToolResult{
				OK:      true,
				Summary: fmt.Sprintf("Система «%s» выбрана в предложении.", resp.Title),
				Data: map[string]any{
					"offerId":     updated.ID,
					"number":      updated.Number,
					"systems":     refsPlain(updated.Systems),
					"title":       resp.Title,
					"composition": resp.Composition,
				},
			}, nil
		},
	}
}

// toolSetMaterial — добавить материал в предложение описанием (без цены).
//
// Зовёт `assembleMaterial` — ту же функцию, что «Материалы в предложении
// (описанием)» в редакторе и ручка бота (2026-11-13). Позицию это не создаёт.
func (s *Server) toolSetMaterial() agentToolSpec {
	return agentToolSpec{
		Name: "set_material",
		Description: "Добавляет материал в коммерческое предложение ОПИСАНИЕМ по его noteId. В документ попадут текст и ссылки материала; " +
			"цену и количество это НЕ задаёт (они вводятся в таблице состава). Предложение должно уже существовать.",
		InputSchema: objectSchema(
			[]string{"offerId", "noteId"},
			map[string]any{
				"offerId": intProp("Номер предложения (id из offers_offer)"),
				"noteId":  intProp("Номер заметки материала в Базе знаний"),
			},
		),
		Run: func(ctx context.Context, env *agentEnv, args map[string]any) (agentToolResult, error) {
			offer, ok := env.offerForWrite(ctx, argInt(args, "offerId"))
			if !ok {
				return errTool("предложение не найдено или нет прав на его правку")
			}
			noteID := argInt(args, "noteId")
			if noteID <= 0 {
				return errTool("нужен аргумент noteId — номер заметки материала")
			}
			before := append(OfferMaterials{}, offer.Materials...)
			updated, resp, err := env.Server.assembleMaterial(ctx, offer, Ref{NoteID: noteID})
			if err != nil {
				return errTool("%v", err)
			}
			env.noteFix("offer_material", offer.ID, before, updated.Materials)
			return agentToolResult{
				OK:      true,
				Summary: fmt.Sprintf("Материал «%s» выбран в предложении.", resp.Title),
				Data: map[string]any{
					"offerId":   updated.ID,
					"number":    updated.Number,
					"materials": refsPlain(updated.Materials),
					"title":     resp.Title,
				},
			}, nil
		},
	}
}

// ============================ сборка документов ============================

// toolBuildDocx — собрать DOCX предложения тем же рендером, что кнопка
// «⬇ DOCX». Возвращает файл кнопкой (байты — в `FileBytes` транспорта, чтобы
// бот переслал их в чат, а движок увидел только ссылку/подпись).
func (s *Server) toolBuildDocx() agentToolSpec {
	return agentToolSpec{
		Name: "build_docx",
		Description: "Собирает готовый DOCX коммерческого предложения и отдаёт файл менеджеру. " +
			"Файл получается тем же рендером, что кнопка «DOCX» в приложении.",
		InputSchema: objectSchema(
			[]string{"offerId"},
			map[string]any{
				"offerId": intProp("Номер предложения (id из offers_offer)"),
			},
		),
		Run: func(ctx context.Context, env *agentEnv, args map[string]any) (agentToolResult, error) {
			offer, ok := env.offerForRead(ctx, argInt(args, "offerId"))
			if !ok {
				return errTool("предложение не найдено или нет прав на его просмотр")
			}
			return env.buildDocx(ctx, offer)
		},
	}
}

// toolBuildCombatDocx — собрать DOCX боевого листка.
func (s *Server) toolBuildCombatDocx() agentToolSpec {
	return agentToolSpec{
		Name: "build_combat_docx",
		Description: "Собирает готовый DOCX боевого листка (сравнение нашего продукта с конкурентами) и отдаёт файл менеджеру. " +
			"Листок должен уже существовать (sheetId).",
		InputSchema: objectSchema(
			[]string{"sheetId"},
			map[string]any{
				"sheetId": intProp("Номер боевого листка (id из offers_combat_sheet)"),
			},
		),
		Run: func(ctx context.Context, env *agentEnv, args map[string]any) (agentToolResult, error) {
			sheetID := argInt(args, "sheetId")
			if sheetID <= 0 {
				return errTool("нужен аргумент sheetId — номер боевого листка")
			}
			sheet, err := env.Server.loadCombatSheet(ctx, sheetID)
			if err != nil {
				return errTool("боевой листок не найден")
			}
			// Читать и печатать может любой (правило 2026-11-12), поэтому здесь
			// нет проверки автора — ровно как у ручки экспорта листка.
			return env.buildCombatDocx(ctx, sheet)
		},
	}
}

// ============================ границы прав ============================

// offerForWrite — предложение, которое ЭТОТ человек вправе править: своё или
// admin. Ровно правило `handleSaveOffer` (2026-11-03): роль editor без владения
// чужую строку не трогает.
func (env *agentEnv) offerForWrite(ctx context.Context, id int64) (Offer, bool) {
	offer, err := env.Server.loadOffer(ctx, id)
	if err != nil {
		return Offer{}, false
	}
	if !sameOwnerOrAdmin(ctx, env.Server, env.Email, offer.OwnerEmail) {
		return Offer{}, false
	}
	if offer.Status == "final" {
		// Финальное КП не переписывается (то же правило, что у ручек правки):
		// оно уже ушло заказчику.
		return Offer{}, false
	}
	return offer, true
}

// createOfferRow открывает предложение тем же путём, что бот в начале
// разговора (`handleBotCreateOffer`): номер, владелец, пустой состав.
//
// Почему агент умеет это сам (2026-11-14, P6-бис, сценарий B): раньше он без
// `offerId` (свежий чат, «собери КП по мембране») упирался в «предложение не
// найдено» — человек оставался без документа и без подсказки. Бот открывает
// строку в начале диалога; агенту нужно открыть её, когда документ
// понадобился. Тот же INSERT и тот же `nextNumber`, что у бота, — не второй
// способ создания.
func (env *agentEnv) createOfferRow(ctx context.Context, client OfferClient) (Offer, error) {
	items := []OfferItem{}
	values := map[string]string{}
	if name := strings.TrimSpace(client.Name); name != "" {
		values["client_name"] = name
	}
	clientJSON, _ := json.Marshal(client)
	itemsJSON, _ := json.Marshal(items)
	valuesJSON, _ := json.Marshal(values)
	number, err := env.Server.nextNumber(ctx, time.Now().Year())
	if err != nil {
		return Offer{}, err
	}
	var id int64
	err = env.Server.pool.QueryRow(ctx, `
INSERT INTO offers_offer (number, owner_email, client, items, form_values, total, status)
VALUES ($1, $2, $3, $4, $5, 0, 'draft') RETURNING id`,
		number, strings.ToLower(strings.TrimSpace(env.Email)), clientJSON, itemsJSON, valuesJSON).Scan(&id)
	if err != nil {
		return Offer{}, err
	}
	return env.Server.loadOffer(ctx, id)
}

// createCombatSheet открывает боевой листок тем же путём, что кнопка в боте
// (`createCombat`): номер своего счётчика, владелец, пустые стороны.
func (env *agentEnv) createCombatSheet(ctx context.Context, title string) (CombatSheet, error) {
	sheet := CombatSheet{Title: title, Status: "draft"}
	normalizeCombat(&sheet)
	cols, err := combatColumns(sheet)
	if err != nil {
		return CombatSheet{}, err
	}
	id, err := env.Server.createCombat(ctx, env.Email, sheet, cols)
	if err != nil {
		return CombatSheet{}, err
	}
	return env.Server.loadCombatSheet(ctx, id)
}

// writeCombatSide добавляет или убирает карточку стороны с проверкой
// «правит только автор» — ровно правило `combatSheetForEditor` (читают и
// печатают все, правит автор или admin; правило 2026-11-12).
func (env *agentEnv) writeCombatSide(ctx context.Context, sheetID int64, side string, ref CombatNoteRef, remove bool) (CombatSheet, error) {
	sheet, err := env.Server.loadCombatSheet(ctx, sheetID)
	if err != nil {
		return CombatSheet{}, fmt.Errorf("боевой листок не найден")
	}
	role := roleFor(ctx, env.Server, env.Email)
	if !botCanEditCombat(sheet, env.Email) && !isAdminRole(role) {
		return CombatSheet{}, fmt.Errorf("это боевой листок коллеги — править может только его автор")
	}
	beforeOur := append([]CombatNoteRef{}, sheet.OurNotes...)
	beforeComp := append([]CombatNoteRef{}, sheet.CompetitorNotes...)
	target := &sheet.OurNotes
	if side == combatSideCompetitor {
		target = &sheet.CompetitorNotes
	}
	if remove {
		*target = removeNoteRef(*target, ref.NoteID)
	} else {
		*target = appendNoteRef(*target, ref)
	}
	normalizeCombat(&sheet)
	cols, err := combatColumns(sheet)
	if err != nil {
		return CombatSheet{}, fmt.Errorf("не удалось сохранить листок")
	}
	if err := env.Server.updateCombat(ctx, sheetID, sheet, cols); err != nil {
		return CombatSheet{}, fmt.Errorf("листок не сохранён: %v", err)
	}
	if side == combatSideOur {
		env.noteFix("combat_our_notes", sheetID, beforeOur, sheet.OurNotes)
	} else {
		env.noteFix("combat_competitor_notes", sheetID, beforeComp, sheet.CompetitorNotes)
	}
	return env.Server.loadCombatSheet(ctx, sheetID)
}

// offerForRead — предложение, которое этот человек вправе видеть (canSeeOffer).
func (env *agentEnv) offerForRead(ctx context.Context, id int64) (Offer, bool) {
	offer, err := env.Server.loadOffer(ctx, id)
	if err != nil {
		return Offer{}, false
	}
	role := roleFor(ctx, env.Server, env.Email)
	if !canSeeOffer(role, env.Email, offer) {
		return Offer{}, false
	}
	return offer, true
}

// roleFor — роль человека в приложении offers по карте auth-service. Пустая
// строка — роль не назначена; тогда правами считается «свой или админ».
func roleFor(ctx context.Context, s *Server, email string) string {
	role, _ := s.effectiveRole(ctx, appIDFromEnv(), email)
	return role
}

// sameOwnerOrAdmin — владелец строки или admin приложения.
func sameOwnerOrAdmin(ctx context.Context, s *Server, email, owner string) bool {
	if strings.EqualFold(strings.TrimSpace(email), strings.TrimSpace(owner)) {
		return true
	}
	return isAdminRole(roleFor(ctx, s, email))
}

// ============================ сборка файла ============================

// buildDocx собирает DOCX предложения через ТУ ЖЕ ручку, что кнопка
// `handleExportDocx`. Запрос строится вокруг настоящего контекста инструмента,
// поэтому рендер идёт одним путём: журнал сборки, таймаут работы, все три
// способа печати (макет, блоки формы, тексты систем).
//
// Файл возвращается байтами в `env.Files` — транспорту бота он нужен, чтобы
// отправить документ в чат; движку уходит ссылка/подпись, а не base64.
func (env *agentEnv) buildDocx(ctx context.Context, offer Offer) (agentToolResult, error) {
	req := env.requestWith(ctx, http.MethodPost, "/api/offers/offers/"+formatInt(offer.ID)+"/export", map[string]string{"id": formatInt(offer.ID)})
	rec := httptest.NewRecorder()
	env.Server.handleExportDocx(rec, req)
	if rec.Code != http.StatusOK {
		return errTool("сборка документа не удалась: %s", recorderError(rec))
	}
	name := docxFileName(offer)
	env.addFile(name, rec.Body.Bytes())
	return agentToolResult{
		OK:      true,
		Summary: fmt.Sprintf("Документ по предложению %s готов: %s.", offer.Number, name),
		Data:    map[string]any{"fileName": name, "number": offer.Number},
		Link:    &agentLink{URL: "", Label: name},
	}, nil
}

// buildCombatDocx собирает DOCX боевого листка через `handleExportCombat`.
func (env *agentEnv) buildCombatDocx(ctx context.Context, sheet CombatSheet) (agentToolResult, error) {
	req := env.requestWith(ctx, http.MethodPost, "/api/offers/combat/"+formatInt(sheet.ID)+"/export", map[string]string{"id": formatInt(sheet.ID)})
	rec := httptest.NewRecorder()
	env.Server.handleExportCombat(rec, req)
	if rec.Code != http.StatusOK {
		return errTool("сборка боевого листка не удалась: %s", recorderError(rec))
	}
	name := docxFileNameFor("Боевой листок", sheet.Number)
	env.addFile(name, rec.Body.Bytes())
	return agentToolResult{
		OK:      true,
		Summary: fmt.Sprintf("Боевой листок %s готов: %s.", sheet.Number, name),
		Data:    map[string]any{"fileName": name, "number": sheet.Number},
		Link:    &agentLink{URL: "", Label: name},
	}, nil
}

// requestWith — запрос вокруг контекста инструмента.
//
// `pathValues` проставляет параметры пути (`{id}`): ручки читают их через
// `r.PathValue`, а `httptest.NewRequest` сам их не разбирает. Без этого
// `handleExportDocx` получал пустой `{id}` и отвечал «invalid id» — то есть
// `build_docx` агента не работал вовсе (найдено стендом 2026-11-14).
func (env *agentEnv) requestWith(ctx context.Context, method, path string, pathValues map[string]string) *http.Request {
	req := httptest.NewRequestWithContext(ctx, method, path, http.NoBody)
	for key, value := range pathValues {
		req.SetPathValue(key, value)
	}
	// Идентичность внутри ручки: `actorEmailFromCtx` читает именно `permEmailCtx`.
	return req.WithContext(context.WithValue(ctx, permEmailCtx{}, env.Email))
}

// recorderError — текст ошибки из записанного ответа ручки.
func recorderError(rec *httptest.ResponseRecorder) string {
	var parsed struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err == nil && parsed.Error != "" {
		return parsed.Error
	}
	return fmt.Sprintf("код ответа %d", rec.Code)
}

// formatInt — панель без strconv в вызовах, где строка собирается из числа.
func formatInt(v int64) string {
	return fmt.Sprintf("%d", v)
}

// refsPlain — выбор систем/материалов в простом виде для модели.
func refsPlain(refs RefList) []map[string]any {
	out := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		item := map[string]any{"noteId": ref.NoteID}
		if strings.TrimSpace(ref.SKU) != "" {
			item["sku"] = ref.SKU
		}
		out = append(out, item)
	}
	return out
}
