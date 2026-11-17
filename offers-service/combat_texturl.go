package main

// Конкурент из ссылки (2026-11-17).
//
// Менеджер вставляет адрес страницы конкурента, сервис читает её и кладёт в
// листок конкурента с текстом — без карточки Базы знаний. Зачем сервер, а не
// приложение: страница читается через `agent-service` (там живут SSRF-защита и
// пределы), а у приложения нет ни доступа в интернет, ни прав.
//
// Границы (находки совещания 2026-11-17):
//   - схема только http/https и разумная длина адреса — проверяются ДО вызова;
//   - метод всегда GET и берётся из кода, а не из тела: `AgentFetchRequest`
//     передаёт `Method`/`Headers` насквозь, и клиент не должен ими управлять;
//   - статус ответа обязан быть 2xx: страница «404» иначе стала бы описанием
//     конкурента;
//   - пустой текст — отказ, не запись (то же правило, что у фактов листка);
//   - текст обрезается `combatTextTextChars`, число записей — `combatTextLimit`.

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// combatURLMaxLen — предел длины адреса. Длинный адрес — не страница, а
// попытка положить его в текст ошибки или обойти проверку схемы.
const combatURLMaxLen = 2000

// combatFetchTimeoutMs — сколько ждём страницу. То же значение по умолчанию,
// что у инструмента `fetch_url` помощника.
const combatFetchTimeoutMs = 30000

// fetchURLProblem — можно ли читать этот адрес; пустая строка — можно.
func fetchURLProblem(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "не указан адрес страницы"
	}
	if len(trimmed) > combatURLMaxLen {
		return "адрес страницы слишком длинный"
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return "адрес страницы непонятен — нужен полный адрес с http:// или https://"
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return ""
	}
	return "читать можно только страницы http:// и https://"
}

// combatURLKey — тождество записи «конкурент из ссылки».
//
// Один адрес — одна запись: повторное «Прочитать» после правки страницы
// обновляет текст вместо второй строки. Якорь (`#…`) отбрасывается: он не
// меняет содержимое страницы.
func combatURLKey(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return "url:" + strings.TrimSpace(raw)
	}
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	return "url:" + parsed.String()
}

// combatPageTitleAndText — что из ответа станет названием и текстом конкурента.
//
// `agent-service` отдаёт СЫРОЙ HTML (проверено на бою 2026-11-17), поэтому
// разбираем сами: выкидываем `<script>`/`<style>` (иначе в описание попадает
// JavaScript), берём `<title>` как название, остальное — текстом. Не HTML
// (обычный текст, JSON) берём как есть.
func combatPageTitleAndText(contentType, body string) (string, string) {
	html := strings.Contains(strings.ToLower(contentType), "html") ||
		strings.HasPrefix(strings.TrimSpace(body), "<!DOCTYPE") ||
		strings.HasPrefix(strings.TrimSpace(body), "<html")
	if !html {
		return "", strings.Join(strings.Fields(body), " ")
	}
	clean := dropHTMLBlocks(body, "script")
	clean = dropHTMLBlocks(clean, "style")
	title := tagText(clean, "title")
	text := strings.Join(strings.Fields(plainHTMLText(clean)), " ")
	return fixHTMLEntities(title), fixHTMLEntities(text)
}

// dropHTMLBlocks вырезает `<tag …>…</tag>` целиком (регистр тега не важен).
func dropHTMLBlocks(s, tag string) string {
	var b strings.Builder
	lower := strings.ToLower(s)
	open, closeTag := "<"+tag, "</"+tag
	for {
		start := strings.Index(lower, open)
		if start < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:start])
		end := strings.Index(lower[start:], closeTag)
		if end < 0 {
			// Блок не закрыт — считаем, что он до конца документа.
			return b.String()
		}
		cut := start + end
		closeEnd := strings.Index(lower[cut:], ">")
		if closeEnd < 0 {
			return b.String()
		}
		s = s[cut+closeEnd+1:]
		lower = lower[cut+closeEnd+1:]
	}
}

// tagText — текст первого `<tag>…</tag>`; пустая строка, если тега нет.
func tagText(s, tag string) string {
	lower := strings.ToLower(s)
	start := strings.Index(lower, "<"+tag)
	if start < 0 {
		return ""
	}
	openEnd := strings.Index(lower[start:], ">")
	if openEnd < 0 {
		return ""
	}
	body := s[start+openEnd+1:]
	end := strings.Index(strings.ToLower(body), "</"+tag)
	if end < 0 {
		return ""
	}
	return strings.Join(strings.Fields(plainHTMLText(body[:end])), " ")
}

// plainHTMLText убирает теги, сохраняя текст между ними.
func plainHTMLText(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			b.WriteRune(' ')
		case !inTag:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// fixHTMLEntities — частые сущности в читаемый вид. Полный разбор HTML не нужен:
// страница конкурента — источник описания, а не разметки.
func fixHTMLEntities(s string) string {
	replacer := strings.NewReplacer(
		"&nbsp;", " ", "&amp;", "&", "&quot;", `"`, "&#39;", "'",
		"&lt;", "<", "&gt;", ">", "&mdash;", "—", "&ndash;", "–",
		"&laquo;", "«", "&raquo;", "»", "&hellip;", "…",
	)
	return replacer.Replace(s)
}

// pageHost — запасное название, когда у страницы нет `<title>`.
func pageHost(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return strings.TrimSpace(raw)
	}
	return strings.TrimPrefix(parsed.Host, "www.")
}

// handleCombatCompetitorFromURL — POST /combat/{id}/competitor-from-url.
//
// Правит только автор или admin (`combatSheetForEditor`) — тот же гейт, что у
// добавления стороны. Читать и печатать чужой листок может любой, но
// дописывать в него конкурента — нет.
func (s *Server) handleCombatCompetitorFromURL(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	var req struct {
		URL string `json:"url"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if msg := fetchURLProblem(req.URL); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
		return
	}
	sheet, ok := s.combatSheetForEditor(w, r, id)
	if !ok {
		return
	}
	client := NewAgentServiceClient()
	if !client.configured() {
		// Отказ словами, а не пустая запись: «не настроено» и «страница пустая» —
		// разные вещи, и менеджер не должен искать причину сам.
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "чтение ссылок не настроено: администратору нужно выложить agent-service",
		})
		return
	}
	// Метод берём из кода: тело клиента его не задаёт (иначе через ручку можно
	// было бы отправить POST на чужой сайт).
	fetched, err := client.Fetch(r.Context(), AgentFetchRequest{
		Method:    http.MethodGet,
		URL:       strings.TrimSpace(req.URL),
		TimeoutMs: combatFetchTimeoutMs,
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": "не удалось прочитать страницу: " + err.Error(),
		})
		return
	}
	if fetched.Status < 200 || fetched.Status > 299 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "страница ответила " + itoa(fetched.Status) + " — конкурент не добавлен",
		})
		return
	}
	title, text := combatPageTitleAndText(fetched.ContentType, fetched.Text)
	if strings.TrimSpace(text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "на странице нет читаемого текста — конкурент не добавлен",
		})
		return
	}
	if strings.TrimSpace(title) == "" {
		title = pageHost(req.URL)
	}
	ref := newCombatTextRef(combatURLKey(req.URL), title, text, combatSourceURL)
	sheet.TextCompetitors = addCombatTextRef(sheet.TextCompetitors, ref)
	s.writeCombatSheet(w, r, sheet, "сохранить конкурента из ссылки")
}

// writeCombatSheet — записать листок и ответить его свежей копией.
//
// Один путь записи для новых ручек источников: собрать колонки, обновить
// строку (она двигает `updated_at`), перечитать и ответить. Отдельный
// обработчик на каждую ручку повторял бы четыре одинаковых шага.
func (s *Server) writeCombatSheet(w http.ResponseWriter, r *http.Request, sheet CombatSheet, action string) {
	normalizeCombat(&sheet)
	cols, err := combatColumns(sheet)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "encode error"})
		return
	}
	if err := s.updateCombat(r.Context(), sheet.ID, sheet, cols); err != nil {
		dbError(w, action, err)
		return
	}
	saved, err := s.loadCombatSheet(r.Context(), sheet.ID)
	if err != nil {
		dbError(w, "reload combat", err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// handleCombatCompetitorTextDelete — DELETE /combat/{id}/competitor-text.
//
// Тело — `{"key": "url:…"}`: удаляем по тождеству, а не по позиции (позиция
// съезжает от параллельной правки). Удаление своей ручкой, а не общим
// сохранением листка, — чтобы старый клиент, не знающий про текстовых
// конкурентов, не мог стереть их пачкой.
func (s *Server) handleCombatCompetitorTextDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	var req struct {
		Key string `json:"key"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if strings.TrimSpace(req.Key) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "не указан конкурент"})
		return
	}
	sheet, ok := s.combatSheetForEditor(w, r, id)
	if !ok {
		return
	}
	sheet.TextCompetitors = removeCombatTextRef(sheet.TextCompetitors, req.Key)
	s.writeCombatSheet(w, r, sheet, "remove text competitor")
}
