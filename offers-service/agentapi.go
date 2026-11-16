package main

// Транспорт агента у бота (2026-11-14, шаг P6).
//
// Бот отдаёт сюда текст менеджера и свой chat_id; сервис проверяет привязку
// chat_id → email (та же таблица `offers_bot_link`, что у входа), исполняет ход
// и возвращает ответ и готовые файлы. Бот пересылает файлы в чат.
//
// Почему секрет сервиса, а не JWT: бот не держит ключа человека (правило
// 2026-11-12 — «в боте ролей нет»). Ручка живёт под `/bot/*` (правило
// 2026-11-13: маршрут под секретом — только `/bot/*` или `/internal/*`).
//
// Тот же путь, что у остальных ручек бота: `emailForChat` → своё/чужое. Агент
// дальше работает правами этого email, поэтому чужое КП он не тронет: гейт
// внутри инструментов (`offerForWrite`/`offerForRead`).

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// botAgentMessageRequest — тело хода от бота.
type botAgentMessageRequest struct {
	ChatID  string `json:"chat_id"`
	Text    string `json:"text"`
	OfferID int64  `json:"offer_id"`
	SheetID int64  `json:"sheet_id"`
	// Attachments — файлы, приложенные человеком (шаг 4 плана 2026-11-16).
	// Бот передаёт их base64: другого транспорта в ходе нет, а `parse_file`
	// разбирает именно то, что человек прислал, без угадывания пути.
	Attachments []botAgentAttachment `json:"attachments,omitempty"`
}

// botAgentAttachment — приложенный файл в теле хода (имя + base64).
type botAgentAttachment struct {
	Name   string `json:"name"`
	Base64 string `json:"base64"`
}

// botAgentFile — готовый файл в ответе бота (base64, потому что JSON).
type botAgentFile struct {
	Name   string `json:"name"`
	Base64 string `json:"base64"`
}

// botAgentMessageResponse — ответ хода.
type botAgentMessageResponse struct {
	Answer string         `json:"answer"`
	Files  []botAgentFile `json:"files"`
}

// handleBotAgentMessage — POST /api/offers/bot/agent/message.
func (s *Server) handleBotAgentMessage(w http.ResponseWriter, r *http.Request) {
	var req botAgentMessageRequest
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "пустой запрос"})
		return
	}
	email, ok := s.emailForChat(w, r, req.ChatID)
	if !ok {
		return
	}
	attachments, err := decodeAgentAttachments(req.Attachments)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	turn, err := s.runAgentTurn(r.Context(), agentRequest{
		Email:       email,
		ChatID:      req.ChatID,
		Text:        text,
		OfferID:     req.OfferID,
		SheetID:     req.SheetID,
		Attachments: attachments,
	}, nil)
	if err != nil {
		log.Printf("агент: ход не выполнен: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "агент не смог ответить"})
		return
	}
	files := make([]botAgentFile, 0, len(turn.Files))
	for _, file := range turn.Files {
		files = append(files, botAgentFile{
			Name:   file.Name,
			Base64: base64.StdEncoding.EncodeToString(file.Bytes),
		})
	}
	writeJSON(w, http.StatusOK, botAgentMessageResponse{Answer: turn.Answer, Files: files})
}

// handleBotAgentHistory — GET /api/offers/bot/agent/history?chat_id=.
//
// История диалога ведётся ботом (шаг P6 не переносит её в сервис), поэтому эта
// ручка пока возвращает пустой список — она объявлена, чтобы бот не падал на
// ожидании маршрута и чтобы следующий шаг (переезд истории) не менял контракт.
func (s *Server) handleBotAgentHistory(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.emailForChat(w, r, r.URL.Query().Get("chat_id")); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": []any{}})
}

// decodeAgentAttachments — base64 → байты с проверкой имени и предела.
//
// Предел файла — 16 МБ: больше бот в Telegram не перешлёт без отдельного
// разговора, а разбирать «накопительную» пачку в ходе агента не нужно.
// Ошибка — отказ всего хода, а не «пропустим молча»: человек должен узнать,
// что файл не дошёл, вместо того чтобы получить ответ «в файле ничего нет».
func decodeAgentAttachments(raw []botAgentAttachment) ([]agentAttachment, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	const maxAgentAttachmentBytes = 16 << 20
	out := make([]agentAttachment, 0, len(raw))
	for _, item := range raw {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return nil, fmt.Errorf("приложенный файл без имени")
		}
		data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(item.Base64))
		if err != nil {
			return nil, fmt.Errorf("приложенный файл «%s» повреждён", name)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("приложенный файл «%s» пуст", name)
		}
		if len(data) > maxAgentAttachmentBytes {
			return nil, fmt.Errorf("приложенный файл «%s» слишком большой (предел 16 МБ)", name)
		}
		out = append(out, agentAttachment{Name: name, Data: data})
	}
	return out, nil
}

// parseOptionalID — необязательный числовой параметр из query.
func parseOptionalID(raw string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
