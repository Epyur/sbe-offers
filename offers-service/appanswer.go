package main

// «Отправить в чат» из Mini App (2026-11-14, вместо `sendData`).
//
// Почему ручка понадобилась: `Telegram.WebApp.sendData` работает **только у
// Mini App, открытого кнопкой клавиатуры** (документация Telegram,
// WebApp.sendData). Приложение КП открывается inline-кнопкой `📱 NNN/yyyy`,
// поэтому `sendData` там молча ничего не делает и лишь закрывает окно —
// человек видел «нажал, приложение закрылось, в чате пусто».
//
// Путь, который работает для любой кнопки: приложение отдаёт
// СВОЙ `query_id` (он лежит в `initData`), сервис собирает файл и отвечает
// боту от имени человека.
//
// Границы, которые здесь держатся:
//   - `query_id` и `chat_id` берутся ИЗ ПОДПИСИ `initData`, а не из тела:
//     приложение не может попросить отправить что-то в чужой чат;
//   - документ достаётся теми же проверками, что у кнопки «⬇ DOCX» и у бота
//     (`emailForChatID` + владелец для КП, `botCanReadCombat` для листка);
//   - `query_id` одноразовый: Telegram отвечает на повтор отказом, и текст
//     этого отказа доходит до человека.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// answerInitDataMaxBytes — тот же предел, что у сессии: подпись Telegram
// крупнее этого не бывает, а читать бесконечное тело нельзя.
const answerInitDataMaxBytes = initDataMaxBytes * 2

// answerTimeout — сборка DOCX с текстами может ждать модель, а отправка
// через SOCKS — сеть до Telegram; тот же предел, что у проверки токена.
const answerTimeout = botTokenVerifyTimeout * 6

// answerMaxReply — ответ бота короткий (текст ошибки Telegram).
const answerMaxReply = 8 << 10

// appAnswerRequest — тело POST /api/offers/app/answer.
//
// Поля совпадают с указателем для бота (`webapp.ts`): вид документа и номер.
// `initData` приходит целиком — из него берутся и подпись, и `query_id`.
type appAnswerRequest struct {
	InitData string `json:"initData"`
	Action   string `json:"action"`
	Kind     string `json:"kind"`
	ID       int64  `json:"id"`
}

// botAPIResult — общий ответ Bot API: `ok` и описание отказа.
type botAPIResult struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

// handleAppAnswer — собрать документ по подписи приложения и отправить его
// в чат от имени человека.
func (s *Server) handleAppAnswer(w http.ResponseWriter, r *http.Request) {
	var req appAnswerRequest
	if err := decodeJSON(w, r, &req, answerInitDataMaxBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Action != "docx" || (req.Kind != "offer" && req.Kind != "combat") || req.ID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "документ не указан"})
		return
	}

	botToken, _, err := s.loadBotConfig(r.Context())
	if err != nil {
		dbError(w, "app answer bot config", err)
		return
	}

	now := time.Now()
	user, err := verifyInitData(req.InitData, botToken, now, initDataTTL())
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
		dbError(w, "app answer link", err)
		return
	}
	if email == "" {
		logAppAuth("not-linked", chatID)
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "not_linked"})
		return
	}

	data, fileName, status, problem := s.answerDocx(r, req, email)
	if problem != "" {
		writeJSON(w, status, map[string]any{"error": problem})
		return
	}

	if err := s.deliverToChat(r.Context(), chatID, fileName, data); err != nil {
		log.Printf("app answer: отправка в чат: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// answerDocx готовит файл ТЕМИ ЖЕ путями, что бот: своих сборщиков здесь нет.
func (s *Server) answerDocx(r *http.Request, req appAnswerRequest, email string) ([]byte, string, int, string) {
	if req.Kind == "combat" {
		sheet, err := s.loadCombatSheet(r.Context(), req.ID)
		if err != nil {
			return nil, "", http.StatusNotFound, "боевой листок не найден"
		}
		// Читать листки можно всем (правило 2026-11-12) — отправка в чат
		// ничего не раскрывает сверх того, что человек и так открыл.
		if !botCanReadCombat(sheet, email) {
			return nil, "", http.StatusForbidden, "нет доступа к боевому листку"
		}
		layout, err := s.loadLayoutForMode(r.Context(), combatFormMode)
		if err != nil {
			log.Printf("app answer: макет боевого листка не прочитан: %v", err)
			layout = nil
		}
		ourCards := s.resolveCombatCards(r.Context(), sheet.OurNotes, s.loadCachedSystemCards)
		compCards := s.resolveCombatCards(r.Context(), sheet.CompetitorNotes,
			func(ctx context.Context) ([]SystemCard, error) {
				cards, _, err := s.loadCachedCompetitors(ctx)
				return cards, err
			})
		data, err := s.renderCombatDocxCached(r.Context(), layout, sheet, ourCards, compCards, NewKBImageSource(s.kb))
		if err != nil {
			return nil, "", http.StatusBadRequest, err.Error()
		}
		return data, docxFileNameFor("Боевой листок", sheet.Number), http.StatusOK, ""
	}

	offer, err := s.loadOffer(r.Context(), req.ID)
	if err != nil {
		return nil, "", http.StatusNotFound, "предложение не найдено"
	}
	if !strings.EqualFold(offer.OwnerEmail, email) {
		return nil, "", http.StatusForbidden, "это не ваше предложение"
	}
	data, _, err := s.buildOfferDocx(r, offer)
	if err != nil {
		return nil, "", http.StatusBadRequest, err.Error()
	}
	return data, docxFileName(offer), http.StatusOK, ""
}

// deliverToChat — отдать файл боту: только у него есть путь в Telegram
// (СОКС-туннель). Сервис сам в Bot API не ходит.
//
// Ответ бота передаётся человеку как есть: «WEB_APP_QUERY_ID_INVALID» означает
// повторное нажатие, и общее «не удалось» скрыло бы причину.
func (s *Server) deliverToChat(ctx context.Context, chatID, fileName string, data []byte) error {
	base := strings.TrimRight(getEnv("OFFERS_BOT_URL"), "/")
	if base == "" {
		base = "http://offers-bot:3000"
	}
	payload, err := json.Marshal(map[string]any{
		"chat_id":   chatID,
		"file_name": fileName,
		"caption":   captionForAnswer(fileName),
		"content":   base64.StdEncoding.EncodeToString(data),
	})
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, answerTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, base+"/internal/answer", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Secret", getEnv("OFFERS_BOT_SECRET"))
	resp, err := (&http.Client{Timeout: answerTimeout}).Do(req)
	if err != nil {
		return fmt.Errorf("бот недоступен: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, answerMaxReply))
	var reply struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &reply); err != nil {
		return fmt.Errorf("бот ответил не по формату (%d)", resp.StatusCode)
	}
	if !reply.OK {
		if strings.TrimSpace(reply.Error) != "" {
			return errors.New(reply.Error)
		}
		return fmt.Errorf("Telegram отклонил отправку (%d)", resp.StatusCode)
	}
	return nil
}

// captionForAnswer — подпись под отправленным файлом.
func captionForAnswer(fileName string) string {
	if strings.Contains(strings.ToLower(fileName), "листок") {
		return "Боевой листок"
	}
	return "Коммерческое предложение"
}
