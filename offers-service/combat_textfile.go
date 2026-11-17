package main

// Конкурент из файла (2026-11-17).
//
// Приложение Telegram файл отправить не может (inline-кнопка, `sendData` не
// работает — дефект 0.9.28), поэтому файл по конкуренту принимает БОТ и отдаёт
// байты сюда. Разбор — через `agent-service` (`/internal/agent/file/parse`),
// тот же, что у инструмента `parse_file` помощника: один разбор на два входа.
//
// Почему своя ручка, а не «помощник прочитает и запомнит факт»: факты листка —
// источник для модели, а здесь нужен КОНКУРЕНТ в списке, который менеджер
// увидит и напечатает. Это разные вещи, и смешивать их значило бы просить
// модель решать, что считать конкурентом (решение пользователя 2026-11-17:
// источники читаются напрямую, без модели).

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// combatFileMaxBytes — предел присланного файла. Совпадает с пределом
// приложенного файла в помощнике (16 МиБ): больше в Telegram и не пролезет без
// отдельного согласования.
const combatFileMaxBytes = 16 << 20

// combatFileBodyMax — предел тела запроса. Base64 раздувает файл примерно в
// 4/3, поэтому тело больше самого файла; отдельное число здесь нужно, чтобы
// отказ «файл больше 16 МБ» приходил от нас, а не общим «invalid json».
const combatFileBodyMax = combatFileMaxBytes*3/2 + 1024

// combatFileRequest — тело POST /bot/combat/{id}/competitor-file.
type combatFileRequest struct {
	ChatID string `json:"chat_id"`
	Name   string `json:"name"`
	Base64 string `json:"base64"`
}

// combatFileTitle — название конкурента из имени файла.
//
// Расширение снимается: «Конкурент ПВХ.pdf» — это конкурент «Конкурент ПВХ».
// Пустое имя не повод отказать в добавлении, но и названия тогда нет — ручка
// ответит словами, что имя файла нужно.
func combatFileTitle(name string) string {
	base := strings.TrimSpace(name)
	if idx := strings.LastIndexAny(base, `/\`); idx >= 0 {
		base = base[idx+1:]
	}
	if dot := strings.LastIndex(base, "."); dot > 0 {
		base = base[:dot]
	}
	return strings.TrimSpace(base)
}

// combatFileParseProblem — человеческий текст отказа разбора файла.
//
// Форматы задаёт agent-service (`docx`, `xlsx`, `pdf`, `json`), и «unsupported
// format: txt» человеку ничего не говорит: он видит технический код и не знает,
// что делать. Остальные ошибки сервиса отдаём как есть — там уже есть причина.
func combatFileParseProblem(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "unsupported format") {
		return "такой файл не читается: пришлите PDF, DOCX или XLSX"
	}
	return "не удалось разобрать файл: " + msg
}

// handleBotCombatCompetitorFile — POST /bot/combat/{id}/competitor-file.
//
// Гейт: сервисный секрет на маршруте + владелец листка (`botCanEditCombat`).
// Тот же, что у остальных ботовых ручек стороны: читать листки может любой,
// дописывать — только автор.
func (s *Server) handleBotCombatCompetitorFile(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
		return
	}
	var req combatFileRequest
	if err := decodeJSON(w, r, &req, combatFileBodyMax); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	email, ok := s.emailForChat(w, r, req.ChatID)
	if !ok {
		return
	}
	sheet, err := s.loadCombatSheet(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "боевой листок не найден"})
		return
	}
	if !botCanEditCombat(sheet, email) {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": "это боевой листок коллеги — править может только его автор",
		})
		return
	}
	title := combatFileTitle(req.Name)
	if title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "у файла нет имени — назовите файл по конкуренту"})
		return
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.Base64))
	if err != nil || len(data) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "файл не прочитан"})
		return
	}
	if len(data) > combatFileMaxBytes {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "файл больше 16 МБ — пришлите меньше"})
		return
	}
	client := NewAgentServiceClient()
	if !client.configured() {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "разбор файлов не настроен: администратору нужно выложить agent-service",
		})
		return
	}
	parsed, err := client.ParseFile(r.Context(), req.Name, data)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": combatFileParseProblem(err)})
		return
	}
	text := strings.TrimSpace(parseFileText(parsed))
	if text == "" {
		// Честный отказ вместо пустого конкурента: у скана без текстового слоя
		// описания нет, и придумывать его нельзя (правило правила PDF: не читаем).
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "в файле нет читаемого текста — конкурент не добавлен",
		})
		return
	}
	key := "file:" + strings.ToLower(title) + ":" + strconv.FormatInt(time.Now().Unix(), 10)
	ref := newCombatTextRef(key, title, text, combatSourceFile)
	sheet.TextCompetitors = addCombatTextRef(sheet.TextCompetitors, ref)
	s.writeCombatSheet(w, r, sheet, "save competitor from file")
}
