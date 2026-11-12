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
type cardView struct {
	NoteID      int64    `json:"noteId"`
	SKU         string   `json:"sku,omitempty"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Composition []string `json:"composition,omitempty"`
	Problems    []string `json:"problems,omitempty"`
	Links       []string `json:"links,omitempty"`
}

// toolReadCard — полный текст карточки Базы знаний по номеру заметки.
//
// Карточка читается вживую из Базы знаний (`liveSystemCard`), а не из кеша:
// агент читает её, чтобы заметить, что «ТН-КРОВЛЯ Стандарт Проф» нет, и
// предложить ближайшие. Кеш к этому моменту мог отстать.
func (s *Server) toolReadCard() agentToolSpec {
	return agentToolSpec{
		Name: "read_card",
		Description: "Читает карточку системы или материала по её noteId: название, описание, состав, проблемы состава, ссылки. " +
			"Используй после get_our_products, чтобы понять, что именно можно предложить покупателю.",
		InputSchema: objectSchema(
			[]string{"noteId"},
			map[string]any{
				"noteId": intProp("Номер заметки Базы знаний (noteId из get_our_products)"),
			},
		),
		Run: func(ctx context.Context, env *agentEnv, args map[string]any) (agentToolResult, error) {
			noteID := argInt(args, "noteId")
			if noteID <= 0 {
				return errTool("нужен аргумент noteId — номер заметки")
			}
			card, err := env.Server.liveSystemCard(ctx, noteID)
			if err != nil {
				return errTool("База знаний недоступна: %v", err)
			}
			if card.NoteID == 0 {
				return errTool("карточки с noteId %d нет в Базе знаний", noteID)
			}
			view := cardView{
				NoteID:      card.NoteID,
				SKU:         card.SKU,
				Title:       card.Title,
				Description: card.Description,
				Composition: card.Composition,
				Problems:    card.Problems,
				Links:       cardLinkLabels(card),
			}
			return agentToolResult{
				OK:      true,
				Summary: fmt.Sprintf("Карточка «%s» (заметка %d).", card.Title, card.NoteID),
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

// ============================ правки документа ============================

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
	req := env.requestWith(ctx, http.MethodPost, "/api/offers/offers/"+formatInt(offer.ID)+"/export")
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
	req := env.requestWith(ctx, http.MethodPost, "/api/offers/combat/"+formatInt(sheet.ID)+"/export")
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

// requestWith — запрос вокруг контекста инструмента. Параметры пути `{id}`
// проставляет `PathValue` (он же читает `r.PathValue` внутри ручек).
func (env *agentEnv) requestWith(ctx context.Context, method, path string) *http.Request {
	req := httptest.NewRequestWithContext(ctx, method, path, http.NoBody)
	// Идентичность внутри ручки: `actorEmailFromCtx` читает именно `permEmailCtx`.
	req = req.WithContext(context.WithValue(ctx, permEmailCtx{}, env.Email))
	return req
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
