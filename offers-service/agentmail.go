package main

// Письма в контексте КП (2026-11-14, шаг P5b плана sbe-offers).
//
// Модель (решение пользователя 2026-11-14, вариант «б»):
//
//	письмо видно агенту ТОЛЬКО если у карточки его скана в «Документах» есть
//	`product_refs` — та же граница, что у документов. Пересечение двух
//	сервисов делается ЗДЕСЬ: мейлер и документы друг о друге не знают.
//
// Поэтому инструмент `get_emails` берёт `mailer.Emails(...)` (ручка отдаёт
// письма с карточкой скана, видимость НЕ проверяет) и оставляет только те,
// у которых хотя бы одно вложение по `document_id` есть в видимом наборе
// (`NewDocumentsAgentClient().List` — набор P5a). Второго признака («есть PDF»,
// `doc_type`, совпадение названия) нет и быть не должно.
//
// Инструмент — граница, а не промт (§4.4 дизайна): обхода «а покажи все
// письма» не существует, потому что ручка видимости тут не спрашивается вовсе.
//
// Почему `visible` кэшируется на ХОД (в `agentEnv`), а не на сервер: привязку
// к карточке могли поставить секунду назад, и «письмо не появилось» в тот же
// миг — это не ошибка, а задержка кэша сервиса документов.

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// toolGetEmails — письма, относящиеся к продуктам Базы знаний.
//
// Отдаёт письмо, только если `document_id` хотя бы одного его вложения есть в
// видимом наборе документов (карточка с `product_refs`, не архив, без
// `no_agent`). К письму прикладываются НАЗВАНИЯ привязок его карточки — без
// них модель не понимает, к какому продукту относится письмо, и называет его
// наугад.
//
// Пустой результат — «Писем, относящихся к продуктам, не нашлось», и НИКОГДА
// «писем нет»: первое — правда о границе, второе — враньё о данных.
func (s *Server) toolGetEmails() agentToolSpec {
	return agentToolSpec{
		Name: "get_emails",
		Description: "Ищет письма, относящиеся к продуктам Базы знаний (переписка, согласования, сертификаты). " +
			"Видны только письма, чья карточка скана привязана к системе или материалу; остальная почта " +
			"недоступна намеренно. К каждому письму прикладываются названия привязок (products) — по ним понятно, " +
			"к какому продукту оно относится. Пустой результат не повод искать обход.",
		InputSchema: objectSchema(
			[]string{},
			map[string]any{
				"query": stringProp("Слово из темы, номера, направления или текста; пусто — весь доступный набор"),
				"limit": intProp("Сколько писем вернуть (по умолчанию 20, максимум 200)"),
				"product": stringProp("О каком продукте речь: номер заметки (noteId) или слово из названия — " +
					"письма остальных продуктов в ответ не попадут"),
			},
		),
		Run: func(ctx context.Context, env *agentEnv, args map[string]any) (agentToolResult, error) {
			mailer := NewMailerAgentClient()
			if !mailer.configured() {
				return errTool("сервис писем не подключён: письма через агента недоступны")
			}
			docs := NewDocumentsAgentClient()
			if !docs.configured() {
				return errTool("сервис документов не подключён: без него письма не проверить на привязку к продукту")
			}
			visible, err := env.visibleDocuments(ctx, docs)
			if err != nil {
				return errTool("документы недоступны: %v", err)
			}
			query := argString(args, "query")
			limit := int(argInt(args, "limit"))
			letters, err := mailer.Emails(ctx, query, limit)
			if err != nil {
				return errTool("письма недоступны: %v", err)
			}

			index := indexVisibleDocuments(visible)
			product := strings.TrimSpace(argString(args, "product"))
			productNote := int64(0)
			if id, convErr := strconv.ParseInt(product, 10, 64); convErr == nil {
				productNote = id
			}
			productWord := foldText(product)
			if productNote > 0 {
				productWord = ""
			}

			items := make([]map[string]any, 0, len(letters.Items))
			for _, letter := range letters.Items {
				docs, ok := visibleScanCards(letter, index)
				if !ok {
					// Видимость письма решает карточка скана, а не мейлер: у
					// письма, чья карточка не привязана к продукту, его не видно.
					continue
				}
				if productNote > 0 || productWord != "" {
					if !docsMatchProduct(docs, productNote, productWord) {
						continue
					}
				}
				items = append(items, emailView(letter, docs))
			}

			if len(items) == 0 {
				return agentToolResult{
					OK:      true,
					Summary: "Писем, относящихся к продуктам, не нашлось.",
					Data:    map[string]any{"items": []any{}, "total": 0},
				}, nil
			}
			return agentToolResult{
				OK:      true,
				Summary: fmt.Sprintf("Нашлось писем по продуктам: %d.", len(items)),
				Data:    map[string]any{"items": items, "total": len(items)},
			}, nil
		},
	}
}

// toolReadEmail — полный текст письма и его сканы.
//
// Читает `mailer.Letter(id)` и прикладывает к каждому скану `file_key`,
// разрешённый по `document_id` из того же видимого набора: отдельной ручки
// файла у писем нет, а ключ отдаёт только документ, привязанный к продукту.
// Текст PDF в v1 пуст — PDF не парсится; сканы открываются уже существующими
// `get_document_link` / `read_document_file`. Нового инструмента для PDF не
// заводим (его не существует в других сервисах).
func (s *Server) toolReadEmail() agentToolSpec {
	return agentToolSpec{
		Name: "read_email",
		Description: "Читает письмо целиком по id (из get_emails) и отдаёт его полный текст и сканы. " +
			"Скан открывается ссылкой из get_document_link или читается read_document_file по file_key; " +
			"текст скана (PDF) агент не разбирает — так и скажи менеджеру.",
		InputSchema: objectSchema(
			[]string{"id"},
			map[string]any{
				"id": intProp("Номер письма (id из get_emails)"),
			},
		),
		Run: func(ctx context.Context, env *agentEnv, args map[string]any) (agentToolResult, error) {
			id := argInt(args, "id")
			if id <= 0 {
				return errTool("нужен аргумент id — номер письма")
			}
			mailer := NewMailerAgentClient()
			if !mailer.configured() {
				return errTool("сервис писем не подключён: письма через агента недоступны")
			}
			docs := NewDocumentsAgentClient()
			if !docs.configured() {
				return errTool("сервис документов не подключён: без него письма не проверить на привязку к продукту")
			}
			visible, err := env.visibleDocuments(ctx, docs)
			if err != nil {
				return errTool("документы недоступны: %v", err)
			}
			letter, err := mailer.Letter(ctx, id)
			if err != nil {
				return errTool("письмо %d недоступно: %v", id, err)
			}
			index := indexVisibleDocuments(visible)
			scans, ok := visibleScanCards(letter, index)
			if !ok {
				return errTool("письмо %d недоступно: у него нет карточки скана, привязанной к продукту", id)
			}
			return agentToolResult{
				OK:      true,
				Summary: fmt.Sprintf("Письмо %s «%s» прочитано.", letter.Number, letter.Subject),
				Data:    emailView(letter, scans),
			}, nil
		},
	}
}

// visibleDocuments — видимый набор документов на ХОД, с кэшем в `agentEnv`.
//
// Ошибку не кэширует: следующая попытка за ход должна повторить запрос, а не
// воспроизвести ту же неудачу. `ready` отличает «ещё не спрашивали» от
// «спросили, пришёл пустой набор» — пустой набор это ответ, а не отсутствие.
func (env *agentEnv) visibleDocuments(ctx context.Context, docs *DocumentsAgentClient) ([]AgentDoc, error) {
	if env.visibleDocsReady {
		return env.visibleDocs, nil
	}
	list, err := docs.List(ctx, "", 200)
	if err != nil {
		return nil, err
	}
	env.visibleDocs = list.Items
	env.visibleDocsReady = true
	return env.visibleDocs, nil
}

// indexVisibleDocuments — карточки по `document_id`.
//
// Одному документу соответствует одна карточка (id уникален), но ключ —
// int64, и повтор безопаснее потери: последняя запись не важна, она та же.
func indexVisibleDocuments(docs []AgentDoc) map[int64]AgentDoc {
	out := make(map[int64]AgentDoc, len(docs))
	for _, doc := range docs {
		if doc.DocumentID > 0 {
			out[doc.DocumentID] = doc
		}
	}
	return out
}

// visibleScanCards — карточки сканов письма, которые есть в видимом наборе.
//
// Приложение (вложение) без `kind: signed` видимость письму не даёт: решение
// пользователя 2026-11-14 — привязки ставятся только у скана, а обычное
// приложение не документ общего доступа. Письмо считается видимым, если
// видима ХОТЬ ОДНА его карточка скана (у письма скан один, но старые записи
// могли пережить замену — лишняя карточка не должна прятать видимую).
func visibleScanCards(letter AgentEmail, index map[int64]AgentDoc) ([]AgentDoc, bool) {
	out := make([]AgentDoc, 0, len(letter.Attachments))
	for _, attachment := range letter.Attachments {
		if attachment.Kind != "signed" || attachment.DocumentID <= 0 {
			continue
		}
		doc, ok := index[attachment.DocumentID]
		if !ok {
			continue
		}
		out = append(out, doc)
	}
	return out, len(out) > 0
}

// foldText — сравнение названий без учёта регистра и «ё»/«е» (метод 2026-06-08).
// Своя копия, а не импорт из документов: сервис один и тот же файл не делит,
// и одно правило здесь нужно ровно один раз.
func foldText(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "ё", "е")
}

// docsMatchProduct — относится ли письмо к названному продукту.
//
// Признак — те же карточки, что дают видимость: номер заметки, совпадающий с
// `noteId` или словом из названия привязки. Сравнение по названию — без учёта
// регистра и «ё»/«е» (тот же `foldText`, что у карточек документов).
// `noteID` > 0 — сверка только по номеру; иначе `word` по названиям.
func docsMatchProduct(docs []AgentDoc, noteID int64, word string) bool {
	for _, doc := range docs {
		for _, ref := range doc.ProductRefs {
			if noteID > 0 && ref.NoteID == noteID {
				return true
			}
			if word != "" && strings.Contains(foldText(ref.Title), word) {
				return true
			}
		}
	}
	return false
}

// emailView — письмо для модели: полный текст и карточки сканов (названия
// привязок, ключи файлов). `file_key` берётся у ВИДИМОЙ карточки — без
// привязки к продукту ключа в ответе нет.
func emailView(letter AgentEmail, scans []AgentDoc) map[string]any {
	products := make([]map[string]any, 0)
	attachments := make([]map[string]any, 0, len(scans))
	seen := make(map[int64]bool, len(scans))
	for _, doc := range scans {
		if seen[doc.DocumentID] {
			continue
		}
		seen[doc.DocumentID] = true
		for _, ref := range doc.ProductRefs {
			product := map[string]any{"noteId": ref.NoteID, "title": ref.Title}
			if ref.SKU != "" {
				product["sku"] = ref.SKU
			}
			products = append(products, product)
		}
		attachment := map[string]any{
			"document_id": doc.DocumentID,
			"file_name":   doc.FileName,
			"kind":        "signed",
			"file_key":    doc.FileKey,
		}
		attachments = append(attachments, attachment)
	}
	// Порядок — по номеру привязки, чтобы ответ не перескакивал между ходами
	// (карта в индексе неупорядочена); повторные названия от разных сканов
	// письма схлопываем: читателю важна связь, а не число карточек.
	sort.SliceStable(products, func(i, j int) bool {
		left, _ := products[i]["noteId"].(int64)
		right, _ := products[j]["noteId"].(int64)
		return left < right
	})
	unique := make([]map[string]any, 0, len(products))
	for _, product := range products {
		duplicate := false
		for _, kept := range unique {
			if kept["noteId"] == product["noteId"] {
				duplicate = true
				break
			}
		}
		if !duplicate {
			unique = append(unique, product)
		}
	}
	return map[string]any{
		"id":             letter.ID,
		"number":         letter.Number,
		"subject":        letter.Subject,
		"author":         letter.Author,
		"date":           letter.Date,
		"direction_name": letter.DirectionName,
		"text":           letter.Text,
		"products":       unique,
		"attachments":    attachments,
	}
}
