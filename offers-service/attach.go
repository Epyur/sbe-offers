package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"mailers/offers/model"
	"mailers/offers/skills"
)

// Приём файлов в КП (шаг 6 пересмотренного плана 2026-10-29).
//
// Менеджер присылает в чат .md или .xlsx — контекст объекта или таблицу
// спецификации. Здесь файл принимается, разбирается и ЖДЁТ ПОДТВЕРЖДЕНИЯ: ничего
// не применяется само. Причины две:
//   - файл от заказчика бывает черновиком со старыми ценами и опечатками;
//   - молчаливое применение уронит КП в глазах заказчика, а сверка занимает у
//     менеджера полминуты.
//
// Поэтому ручка двухшаговая: POST .../attach раскладывает и показывает «что я
// понял», POST .../attach/apply пишет подтверждённое в бриф.

// maxAttachmentBytes — предел размера файла. 10 МБ хватает и на таблицу
// спецификации, и на длинный бриф; всё, что больше, — скорее всего ошибка.
const maxAttachmentBytes = 10 << 20

// attachedFile — что мы приняли и что из него поняли.
type attachedFile struct {
	// Kind — md / xlsx; docx пока не принимается (шаг 2).
	Kind string `json:"kind"`
	// Name — имя файла, как его прислали: по нему человек узнаёт свой файл.
	Name string `json:"name"`
	// Key — ключ в хранилище. Пустой, если хранилище не настроено: тогда файл
	// живёт только в этом ответе, и это честно.
	Key string `json:"key,omitempty"`
	// Brief — что разобрали по брифу (только .md).
	Brief *briefFromFile `json:"brief,omitempty"`
	// Spec — что разобрали как спецификацию (только .xlsx).
	Spec *specFromFile `json:"spec,omitempty"`
	// Context — строки таблицы, когда это контекст, а не состав.
	Context []string `json:"context,omitempty"`
	// Reply — что бот скажет менеджеру: «что я понял» и вопрос-подтверждение.
	Reply string `json:"reply"`
	// NeedsConfirm — есть что применять. false — файл прочитан, но в бриф из
	// него ничего не идёт (например, пустая таблица).
	NeedsConfirm bool `json:"needsConfirm"`
}

// briefFromFile — разложенный по брифу текст из файла.
type briefFromFile struct {
	// Fields — готовые к записи поля: ключ темы → значения.
	Fields map[string][]string `json:"fields"`
	// Notes — что не разложилось по полям.
	Notes string `json:"notes,omitempty"`
	// Summary — человеческое описание «что понял», по пунктам.
	Summary []string `json:"summary"`
}

// specFromFile — распознанная таблица спецификации вместе со сверкой каталога.
type specFromFile struct {
	Rows []specRowView `json:"rows"`
	// Matched / New / PriceDiffs — итоги сверки с каталогом.
	Matched    int      `json:"matched"`
	New        int      `json:"new"`
	PriceDiffs int      `json:"priceDiffs"`
	Warnings   []string `json:"warnings,omitempty"`
	// Summary — «12 позиций, 9 совпало по артикулу, 3 новых, цены отличаются
	// в 2 случаях».
	Summary string `json:"summary"`
}

// specRowView — строка спецификации для показа менеджеру.
type specRowView struct {
	Name       string  `json:"name"`
	Unit       string  `json:"unit"`
	Qty        float64 `json:"qty"`
	Price      float64 `json:"price"`
	Sum        float64 `json:"sum"`
	CatalogID  *int64  `json:"catalogId,omitempty"`
	HasCatalog bool    `json:"hasCatalog"`
	PriceDiff  bool    `json:"priceDiff"`
}

// handleBotAttach принимает файл и разбирает его, ничего не записывая в КП.
//
// Тело — сам файл (не multipart): бот отправляет байты, имя — заголовком
// X-File-Name. Так обе стороны не зависят от разбора multipart, а лимит размера
// проверяется до чтения всего тела в память потоком.
func (s *Server) handleBotAttach(w http.ResponseWriter, r *http.Request) {
	// Тело — файл. Любой ранний отказ (плохой id, не ваш чат) приходит ДО его
	// чтения, и тогда клиент продолжает слать мегабайты в уже отвеченный
	// запрос и висит до таймаута. Поэтому тело дочитывается в defer при любом
	// исходе (поймано живьём 2026-10-30).
	defer drainBody(r.Body)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	offer, _, ok := s.botOfferForChatQ(w, r, id, r.URL.Query().Get("chat_id"))
	if !ok {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxAttachmentBytes))
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{
			"error": "файл больше 10 МБ",
		})
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("file_name"))
	if name == "" {
		name = strings.TrimSpace(r.Header.Get("X-File-Name"))
	}

	result, err := s.readAttachedFile(r.Context(), offer, name, body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// drainBody дочитывает остаток тела, чтобы клиент мог завершить отправку.
//
// Нужен там, где ответ даётся раньше, чем тело прочитано (отказ по доступу,
// плохой идентификатор): без этого отправляющая сторона ждёт, пока не отдаст
// всё, и запрос выглядит зависшим. Читаем ограниченно: гигантское тело нам
// всё равно не нужно, а соединение после отказа закрывается.
func drainBody(r io.Reader) {
	if r == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(r, maxAttachmentBytes+1))
}

// readAttachedFile разбирает файл по типу и собирает ответ «что я понял».
func (s *Server) readAttachedFile(ctx context.Context, offer Offer, name string, data []byte) (*attachedFile, error) {
	kind := skills.DetectAttachmentType(name, data)
	out := &attachedFile{Kind: kind, Name: name}
	switch kind {
	case string(skills.AttachmentMarkdown):
		parsed := skills.ParseMarkdownAttachment(string(data))
		out.Brief = briefFromMarkdown(parsed)
		out.NeedsConfirm = len(out.Brief.Fields) > 0
		out.Reply = attachReplyBrief(out.Brief)
	case string(skills.AttachmentXlsx):
		parsed, err := skills.ParseXlsxAttachment(data)
		if err != nil {
			return nil, errors.New("не удалось прочитать таблицу: " + err.Error())
		}
		if parsed.IsSpecification {
			out.Spec = s.specFromFile(ctx, parsed)
			out.NeedsConfirm = len(out.Spec.Rows) > 0
			out.Reply = attachReplySpec(out.Spec)
			break
		}
		out.Context = parsed.Text
		out.Reply = "Прочитал таблицу, но состава в ней не нашёл. " +
			"Пришлите файл с колонками «Наименование» и «Количество» — или соберите состав из каталога."
	case string(skills.AttachmentDocx):
		return nil, errors.New("файлы Word пока не принимаю: пришлите .md или .xlsx")
	case string(skills.AttachmentOldExcel):
		return nil, errors.New("старый формат .xls не читаю: пересохраните файл как .xlsx")
	case string(skills.AttachmentPDF):
		return nil, errors.New("PDF не разбираю: пришлите .md или .xlsx")
	default:
		return nil, errors.New("не понял формат файла: пришлите .md или .xlsx")
	}

	// Файл сохраняем как вложение предложения: потом видно, откуда взялись
	// данные. Ошибка хранилища файл не отменяет — разбор важнее.
	out.Key = s.storeAttachment(ctx, offer, name, data)
	return out, nil
}

// storeAttachment кладёт файл в хранилище рядом с DOCX предложения.
func (s *Server) storeAttachment(ctx context.Context, offer Offer, name string, data []byte) string {
	if s.s3 == nil {
		return ""
	}
	key := "offers/" + offer.Number + "/attach/" + randomID() + "-" + sanitizeKey(name)
	if err := s.s3.Put(ctx, key, data); err != nil {
		return ""
	}
	return key
}

// briefFromMarkdown превращает разобранный файл в поля брифа.
func briefFromMarkdown(parsed skills.MarkdownAttachment) *briefFromFile {
	out := &briefFromFile{Fields: make(map[string][]string, len(parsed.Known)), Notes: parsed.Notes}
	for key, values := range parsed.Known {
		if len(values) > 0 {
			out.Fields[key] = values
		}
	}
	if model.NonEmpty(parsed.ObjectName) {
		out.Summary = append(out.Summary, "объект: "+parsed.ObjectName)
	}
	if len(parsed.CustomerPains) > 0 {
		out.Summary = append(out.Summary, fmt.Sprintf("болей заказчика: %d", len(parsed.CustomerPains)))
	}
	if model.NonEmpty(parsed.Timeline) {
		out.Summary = append(out.Summary, "сроки: "+parsed.Timeline)
	}
	if model.NonEmpty(parsed.BudgetSignal) {
		out.Summary = append(out.Summary, "бюджет: "+parsed.BudgetSignal)
	}
	if model.NonEmpty(parsed.DecisionMaker) {
		out.Summary = append(out.Summary, "кто решает: "+parsed.DecisionMaker)
	}
	if len(out.Summary) == 0 && model.NonEmpty(parsed.Notes) {
		out.Summary = append(out.Summary, "заметка для брифа")
	}
	return out
}

// specFromFile сверяет строки таблицы с каталогом.
//
// Сверка по наименованию, а не по артикулу: в файлах от заказчиков артикула чаще
// всего нет, а наименование есть. Расхождение цен НЕ исправляется само — оно
// показывается, и решает менеджер.
func (s *Server) specFromFile(ctx context.Context, parsed skills.XlsxAttachment) *specFromFile {
	out := &specFromFile{Warnings: parsed.Warnings}
	items, err := s.listCatalogItems(ctx, false)
	if err != nil {
		// Каталог недоступен — показываем строки как есть, честно сказав, что
		// сверки не было.
		items = nil
	}
	byName := make(map[string]CatalogItem, len(items))
	for _, item := range items {
		byName[strings.ToLower(strings.TrimSpace(item.Name))] = item
	}
	for _, row := range parsed.Rows {
		view := specRowView{Name: row.Name, Unit: row.Unit, Qty: row.Qty, Price: row.Price, Sum: row.Sum}
		if item, ok := byName[strings.ToLower(strings.TrimSpace(row.Name))]; ok {
			view.HasCatalog = true
			catalogID := item.ID
			view.CatalogID = &catalogID
			out.Matched++
			if item.BasePrice != row.Price && row.Price > 0 {
				view.PriceDiff = true
				out.PriceDiffs++
			}
		} else {
			out.New++
		}
		out.Rows = append(out.Rows, view)
	}
	out.Summary = specSummary(len(out.Rows), out.Matched, out.New, out.PriceDiffs)
	return out
}

// specSummary — итог сверки человеческим языком.
func specSummary(total, matched, newRows, priceDiffs int) string {
	parts := []string{fmt.Sprintf("строк: %d", total)}
	if matched > 0 {
		parts = append(parts, fmt.Sprintf("совпало с каталогом: %d", matched))
	}
	if newRows > 0 {
		parts = append(parts, fmt.Sprintf("нет в каталоге: %d", newRows))
	}
	if priceDiffs > 0 {
		parts = append(parts, fmt.Sprintf("цены отличаются: %d", priceDiffs))
	}
	return strings.Join(parts, ", ")
}

// attachReplyBrief — что бот говорит после разбора текстового файла.
func attachReplyBrief(b *briefFromFile) string {
	if len(b.Fields) == 0 {
		return "Файл прочитал, но полей брифа в нём не нашёл. " +
			"Разложить заметки в «прочее»?"
	}
	return "Понял: " + strings.Join(b.Summary, ", ") + ".\n" +
		"Разложить в бриф? ✓ / ✏️"
}

// attachReplySpec — что бот говорит после разбора таблицы.
func attachReplySpec(spec *specFromFile) string {
	reply := "Нашёл таблицу: " + spec.Summary + "."
	if spec.PriceDiffs > 0 {
		reply += "\nЦены в файле отличаются от каталога — заменять состав буду по каталогу, " +
			"если не скажете иначе."
	}
	reply += "\nСобрать состав по этой таблице? ✓ / ✏️"
	return reply
}

// --- Применение подтверждённого ----------------------------------------

// handleBotAttachApply применяет подтверждённый разбор.
//
// Разбор приходит в теле запроса: сервис не хранит промежуточное состояние между
// «показал» и «применил» — это лишнее состояние, которое переживёт перезапуск
// неправильным. Бот получил разбор в ответе на attach и вернул его назад.
func (s *Server) handleBotAttachApply(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	var req struct {
		ChatID string         `json:"chat_id"`
		Mode   string         `json:"mode"`
		Brief  *briefFromFile `json:"brief"`
		Spec   *specFromFile  `json:"spec"`
	}
	if err := decodeJSON(w, r, &req, 2<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	offer, _, ok := s.botOfferForChatQ(w, r, id, req.ChatID)
	if !ok {
		return
	}

	switch req.Mode {
	case "brief":
		if req.Brief == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "нет разбора файла"})
			return
		}
		updated, err := s.applyBriefFromFile(r.Context(), offer, req.Brief)
		if err != nil {
			dbError(w, "attach apply brief", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"brief":   updated,
			"reply":   "Записал в бриф. " + briefProgress(updated),
			"applied": true,
		})
	case "spec":
		if req.Spec == nil || len(req.Spec.Rows) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "в таблице нет позиций"})
			return
		}
		applied, err := s.applySpecFromFile(r.Context(), offer, req.Spec)
		if err != nil {
			dbError(w, "attach apply spec", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"offer":   applied,
			"reply":   fmt.Sprintf("Состав обновлён: позиций %d, итого %s руб.", len(applied.Items), formatMoney(applied.Total)),
			"applied": true,
		})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "mode: brief|spec"})
	}
}

// applyBriefFromFile записывает поля файла в бриф предложения.
//
// Сличение как у шагов разговора: пустое не затирает известное, списки
// объединяются без повторов. Файл дополняет бриф, а не переписывает его — иначе
// присланный черновик стёр бы всё, что менеджер рассказал в чате.
func (s *Server) applyBriefFromFile(ctx context.Context, offer Offer, from *briefFromFile) (model.BriefPayload, error) {
	brief, err := s.createBrief(ctx, offer.ID, "")
	if err != nil {
		return model.BriefPayload{}, err
	}
	next := mergeBriefFields(brief.Payload, from.Fields)
	if model.NonEmpty(from.Notes) {
		if model.NonEmpty(next.Notes) {
			next.Notes = next.Notes + "\n" + from.Notes
		} else {
			next.Notes = from.Notes
		}
	}
	next.Normalize()
	brief.Payload = next
	if _, open := skills.NextOpenTopic(next); !open {
		brief.Status = "ready"
	}
	if err := s.saveBriefPayload(ctx, brief); err != nil {
		return model.BriefPayload{}, err
	}
	return next, nil
}

// mergeBriefFields накладывает поля из файла на бриф без потерь.
func mergeBriefFields(prev model.BriefPayload, fields map[string][]string) model.BriefPayload {
	out := prev
	first := func(key string) string {
		if list := fields[key]; len(list) > 0 {
			return strings.TrimSpace(strings.Join(list, "; "))
		}
		return ""
	}
	setStr := func(dst *string, key string) {
		if value := first(key); model.NonEmpty(value) {
			*dst = value
		}
	}
	appendList := func(dst *[]string, key string) {
		for _, v := range fields[key] {
			v = strings.TrimSpace(v)
			if v == "" || containsString(*dst, v) {
				continue
			}
			*dst = append(*dst, v)
		}
	}
	setStr(&out.ObjectName, "object_name")
	setStr(&out.BudgetSignal, "budget_signal")
	setStr(&out.DecisionMaker, "decision_maker")
	setStr(&out.Timeline, "timeline")
	appendList(&out.CustomerGoals, "customer_goals")
	appendList(&out.CustomerPains, "customer_pains")
	appendList(&out.ProjectRisks, "project_risks")
	appendList(&out.WhyUs, "why_us")

	// Возражения приходят списком строк «возражение — ответ»: без ответа это
	// не работа с возражением, а его запись, поэтому такие строки остаются в
	// заметках брифа.
	for _, raw := range fields["objections"] {
		claim, response, _ := strings.Cut(raw, "—")
		if !model.NonEmpty(response) {
			claim, response, _ = strings.Cut(raw, " - ")
		}
		if !model.NonEmpty(claim) || !model.NonEmpty(response) {
			continue
		}
		out.Objections = append(out.Objections, model.BriefObjection{
			Claim:    strings.TrimSpace(claim),
			Response: strings.TrimSpace(response),
		})
	}
	out.Normalize()
	return out
}

// containsString — есть ли строка в списке (для объединения без повторов).
func containsString(list []string, value string) bool {
	for _, v := range list {
		if strings.EqualFold(v, value) {
			return true
		}
	}
	return false
}

// briefProgress — «N из M» для ответа боту.
func briefProgress(p model.BriefPayload) string {
	return fmt.Sprintf("Заполнено %d из %d.", p.FilledCount(), p.TotalFields())
}

// applySpecFromFile заменяет состав предложения строками таблицы.
//
// Цены берутся ИЗ КАТАЛОГА для позиций, которые в нём нашлись: файл от заказчика
// — черновик, и печатать в КП старые цены из него нельзя. Для новых позиций
// берётся цена из файла: другой нет.
func (s *Server) applySpecFromFile(ctx context.Context, offer Offer, spec *specFromFile) (Offer, error) {
	items := make([]OfferItem, 0, len(spec.Rows))
	for _, row := range spec.Rows {
		price := row.Price
		if row.HasCatalog && row.CatalogID != nil {
			catalogItem, err := s.loadCatalogItem(ctx, *row.CatalogID)
			if err == nil && catalogItem.BasePrice > 0 {
				price = catalogItem.BasePrice
			}
		}
		item := OfferItem{
			CatalogID: row.CatalogID,
			Name:      row.Name,
			Unit:      row.Unit,
			Qty:       row.Qty,
			Price:     price,
		}
		if item.Unit == "" {
			item.Unit = "шт"
		}
		item.Sum = roundMoney(row.Qty * price)
		items = append(items, item)
	}
	offer.Items = items
	if msg := validateOffer(offer); msg != "" {
		return Offer{}, errors.New(msg)
	}
	offer.Total = recalcOfferTotal(items)
	itemsJSON, _ := json.Marshal(offer.Items)
	_, err := s.pool.Exec(ctx, `
UPDATE offers_offer SET items = $2, total = $3, updated_at = now() WHERE id = $1`,
		offer.ID, itemsJSON, offer.Total)
	if err != nil {
		return Offer{}, err
	}
	return s.loadOffer(ctx, offer.ID)
}

// attachmentTitle — подпись файла в ответе боту (имя без пути).
//
// Windows-путь «C:\tmp\бриф.md» тоже чистится: имя приходит из Telegram, но
// человек мог отправить файл с машины, где имя прошло через путь.
func attachmentTitle(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "файл"
	}
	if i := strings.LastIndexAny(name, `/\`); i >= 0 && i+1 < len(name) {
		name = name[i+1:]
	}
	return name
}
