package main

// «Продолжить в боте» из приложения (2026-11-15, шаг «блок Брифа в окне КП»).
//
// Задача с двух сторон одна: в приложении есть работа, которую делает только
// Telegram, — разговор с ботом. Поэтому приложение просит бота ОТКРЫТЬ нужный
// диалог у себя, а не повторяет его.
//
// Теперь это работает: `Telegram.WebApp.sendData` из приложения КП заведомо не
// срабатывал (оно открывается inline-кнопкой, а не кнопкой клавиатуры —
// дефект 2026-09.28). Путь без ограничений — тот же, что у «Отправить в чат»:
// приложение отдаёт подпись `initData`, сервис проверяет её, берёт `chat_id` ИЗ
// ПОДПИСИ и просит бота написать в этот чат.
//
// Два действия, оба про бота:
//   - `brief` — продолжить бриф этого КП в чате (кнопка «✏️ Продолжить в боте»);
//   - `positions` — попросить в чате файл разбора позиций (кнопка в зоне
//     «Позиции»).
//
// Границы:
//   - `chat_id` — из подписи, не из тела: попросить чужой чат нельзя;
//   - КП проверяется владельцу (`email` из привязки чата): «продолжить» чужое
//     КП бот бы всё равно отказался, но отказ должен быть здесь и словами;
//   - ошибка отправки в бот видна человеку текстом (класс 0.9.29: молча
//     закрытое окно без файла и без строки в журнале).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// askAction — что попросить у бота. Значения совпадают с тем, что читает
// `/internal/ask` в боте: одно слово на одно действие.
const (
	askActionBrief     = "brief"
	askActionPositions = "positions"
)

// appAskRequest — тело POST /api/offers/app/ask.
type appAskRequest struct {
	InitData string `json:"initData"`
	Action   string `json:"action"`
	Kind     string `json:"kind"`
	ID       int64  `json:"id"`
}

// handleAppAsk — попросить бота продолжить разговор в чате.
//
// Ответ короткий: `{ok:true, sent:<что напишет бот>}`. Приложение показывает
// строку «написали в чат» и НЕ закрывается: закрытие — дело человека, а
// отказавшее приложение обязано остаться на экране с текстом (0.9.27).
func (s *Server) handleAppAsk(w http.ResponseWriter, r *http.Request) {
	var req appAskRequest
	if err := decodeJSON(w, r, &req, answerInitDataMaxBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Action != askActionBrief && req.Action != askActionPositions {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "действие не указано"})
		return
	}
	if req.Kind != "offer" || req.ID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "документ не указан"})
		return
	}

	botToken, _, err := s.loadBotConfig(r.Context())
	if err != nil {
		dbError(w, "app ask bot config", err)
		return
	}
	user, err := verifyInitData(req.InitData, botToken, time.Now(), initDataTTL())
	if err != nil {
		status, body := appSessionError(err)
		var appErr *appInitDataError
		if errors.As(err, &appErr) {
			logAppAuth(appErr.reason, "")
		}
		writeJSON(w, status, body)
		return
	}
	chatID := strconv.FormatInt(user.ID, 10)
	email, err := s.emailForChatID(r.Context(), chatID)
	if err != nil {
		dbError(w, "app ask link", err)
		return
	}
	if email == "" {
		logAppAuth("not-linked", chatID)
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "not_linked"})
		return
	}

	offer, err := s.loadOffer(r.Context(), req.ID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "предложение не найдено"})
		return
	}
	if !strings.EqualFold(offer.OwnerEmail, email) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "это не ваше предложение"})
		return
	}

	notice, err := s.askBot(r.Context(), chatID, req.Action, offer)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error":  err.Error(),
			"detail": "не удалось попросить бота: " + err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sent": notice})
}

// askBot — отдать боту просьбу и вернуть, что он напишет в чат.
//
// Путь только через бота: у сервиса нет доступа к api.telegram.org (СОКС —
// у бота), и заводить второй канал значило бы второй токен и второй путь.
func (s *Server) askBot(ctx context.Context, chatID, action string, offer Offer) (string, error) {
	base := strings.TrimRight(getEnv("OFFERS_BOT_URL"), "/")
	if base == "" {
		base = "http://offers-bot:3000"
	}
	payload, err := json.Marshal(map[string]any{
		"chat_id":      chatID,
		"action":       action,
		"offer_id":     offer.ID,
		"offer_number": offer.Number,
	})
	if err != nil {
		return "", err
	}
	reqCtx, cancel := context.WithTimeout(ctx, botTokenVerifyTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, base+"/internal/ask", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Secret", getEnv("OFFERS_BOT_SECRET"))
	resp, err := (&http.Client{Timeout: botTokenVerifyTimeout}).Do(req)
	if err != nil {
		return "", fmt.Errorf("бот недоступен: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, answerMaxReply))
	var reply struct {
		OK    bool   `json:"ok"`
		Sent  string `json:"sent"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &reply); err != nil {
		return "", fmt.Errorf("бот ответил не по формату (%d)", resp.StatusCode)
	}
	if !reply.OK {
		if strings.TrimSpace(reply.Error) != "" {
			return "", errors.New(reply.Error)
		}
		return "", fmt.Errorf("бот отклонил просьбу (%d)", resp.StatusCode)
	}
	return reply.Sent, nil
}
