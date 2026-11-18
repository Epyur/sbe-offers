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

// handleAppAnswerAsync — собрать документ и отправить его в чат В ФОНЕ.
//
// Зачем (2026-11-17, живая жалоба). Первая сборка боевого листка на разделах
// зовёт модель на каждый подраздел и растеризует схемы — это минуты. Mini App
// живёт в WebView Telegram, и `fetch` из приложения не ждёт столько: приходит
// «failed fetch», хотя сервер ещё собирает файл. Проверка подписи и доступа
// быстрая — её делаем синхронно, а сборку+отправку уводим в горутину с
// `context.WithoutCancel` (тот же приём, что у записи текстов, 0.9.30): запрос
// приложения уже закрыт, но работы это не отменяет. Приложение получает
// `{ok:true, sent:true}` сразу и закрывается; файл приходит в чат, когда готов.
func (s *Server) handleAppAnswerAsync(w http.ResponseWriter, r *http.Request) {
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

	// Сразу отвечаем «принято»: приложение закроется, а файл придёт в чат.
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sent": true, "async": true})

	// Контекст НЕ привязан к запросу: ответ уже отдан, но работа продолжается.
	bgCtx := context.WithoutCancel(r.Context())
	go func(req appAnswerRequest, chatID, email string) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("app answer (async): паника при сборке %s %d: %v", req.Kind, req.ID, rec)
			}
		}()
		// Первое, что видит человек, — что документ действительно собирается.
		// До 2026-11-18 в чате было четыре минуты тишины: приложение закрывалось,
		// а файл приходил потом, и отличить «работает» от «сломалось» было нечем.
		s.notifyChat(bgCtx, chatID, answerStartText(req.Kind))
		data, fileName, status, problem := s.answerDocxCtx(bgCtx, req, email)
		if problem != "" {
			log.Printf("app answer (async): %s %d не собран (код %d): %s", req.Kind, req.ID, status, problem)
			s.notifyChat(bgCtx, chatID, "Не удалось собрать файл: "+problem)
			return
		}
		if err := s.deliverToChat(bgCtx, chatID, fileName, data); err != nil {
			log.Printf("app answer (async): отправка в чат %s: %v", chatID, err)
			s.notifyChat(bgCtx, chatID, "Файл собран, но в чат не ушёл: "+err.Error())
			return
		}
		log.Printf("app answer (async): файл %q отправлен в чат %s (%d байт)", fileName, chatID, len(data))
	}(req, chatID, email)
}

// answerStartText — сообщение, которое человек видит СРАЗУ после нажатия
// «Отправить в чат» (2026-11-18). Текст один для бота и приложения: сборка
// идёт в фоне и может занять минуты, и молчание читается как поломка.
func answerStartText(kind string) string {
	if kind == "combat" {
		return "⌛ Формирую боевой листок — пришлю его в этот чат по готовности. " +
			"Сборка может занять несколько минут, страницу можно закрыть."
	}
	return "⌛ Формирую коммерческое предложение — пришлю его в этот чат по готовности. " +
		"Сборка может занять несколько минут, страницу можно закрыть."
}

// notifyChat — короткое сообщение в чат от бота (без файла), чтобы человек не
// остался в неведении, если файл не пришёл. Это ТЕКСТ, а не документ: у бота
// есть отдельная ручка отправить текст — `POST /internal/answer` его не умеет,
// он только файл. Поэтому шлём через тот же транспорт-файл нельзя; пишем в
// журнал и в чат обычным сообщением бота через `OFFERS_BOT_URL`-ручку notify.
func (s *Server) notifyChat(ctx context.Context, chatID, text string) {
	base := strings.TrimRight(getEnv("OFFERS_BOT_URL"), "/")
	if base == "" {
		base = "http://offers-bot:3000"
	}
	payload, err := json.Marshal(map[string]any{"chat_id": chatID, "text": text})
	if err != nil {
		return
	}
	reqCtx, cancel := context.WithTimeout(ctx, answerTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, base+"/internal/notify", bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Secret", getEnv("OFFERS_BOT_SECRET"))
	resp, err := (&http.Client{Timeout: answerTimeout}).Do(req)
	if err != nil {
		log.Printf("app answer (async): уведомление в чат %s не ушло: %v", chatID, err)
		return
	}
	// Статус читаем, а не выбрасываем (2026-11-18): 403 (не тот секрет),
	// 502 (Telegram отказал) и 503 (бот ещё не готов) раньше терялись молча,
	// и «человек ничего не получил» выглядело как зависшая сборка.
	body, _ := io.ReadAll(io.LimitReader(resp.Body, answerMaxReply))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("app answer (async): уведомление в чат %s отклонено кодом %d: %s",
			chatID, resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

// answerDocx готовит файл ТЕМИ ЖЕ путями, что бот: своих сборщиков здесь нет.
func (s *Server) answerDocx(r *http.Request, req appAnswerRequest, email string) ([]byte, string, int, string) {
	return s.answerDocxCtx(r.Context(), req, email)
}

// answerDocxCtx — то же, но на явном контексте: асинхронная отправка держит
// `context.WithoutCancel`, а не контекст уже закрытого запроса.
func (s *Server) answerDocxCtx(ctx context.Context, req appAnswerRequest, email string) ([]byte, string, int, string) {
	if req.Kind == "combat" {
		sheet, err := s.loadCombatSheet(ctx, req.ID)
		if err != nil {
			return nil, "", http.StatusNotFound, "боевой листок не найден"
		}
		// Читать листки можно всем (правило 2026-11-12) — отправка в чат
		// ничего не раскрывает сверх того, что человек и так открыл.
		if !botCanReadCombat(sheet, email) {
			return nil, "", http.StatusForbidden, "нет доступа к боевому листку"
		}
		layout, err := s.loadLayoutForMode(ctx, combatFormMode)
		if err != nil {
			log.Printf("app answer: макет боевого листка не прочитан: %v", err)
			layout = nil
		}
		ourCards, compCards := s.combatSideCards(ctx, sheet)
		data, err := s.renderCombatDocxCached(ctx, layout, sheet, ourCards, compCards, NewKBImageSource(s.kb))
		if err != nil {
			return nil, "", http.StatusBadRequest, err.Error()
		}
		return data, docxFileNameFor("Боевой листок", sheet.Number), http.StatusOK, ""
	}

	offer, err := s.loadOffer(ctx, req.ID)
	if err != nil {
		return nil, "", http.StatusNotFound, "предложение не найдено"
	}
	if !strings.EqualFold(offer.OwnerEmail, email) {
		return nil, "", http.StatusForbidden, "это не ваше предложение"
	}
	data, _, err := s.buildOfferDocx(requestFromCtx(ctx), offer)
	if err != nil {
		return nil, "", http.StatusBadRequest, err.Error()
	}
	return data, docxFileName(offer), http.StatusOK, ""
}

// requestFromCtx — минимальный *http.Request на контексте.
//
// `renderOffer`/`buildOfferDocx` берут из запроса только `Context()` (и
// `withWorkTimeout` перевешивает таймаут сам), поэтому для фоновой сборки
// достаточно пустого запроса с нужным контекстом — без тела и пути.
func requestFromCtx(ctx context.Context) *http.Request {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://offers-service/app/answer", nil)
	return req
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
