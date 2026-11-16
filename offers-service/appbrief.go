package main

// Бриф КП в приложении (2026-11-15, шаг «блок Брифа в окне КП»).
//
// Бриф жил в двух местах: карточка в плагине (`GET/PUT /offers/{id}/brief`) и
// диалог в боте (`POST /bot/offers/{id}/brief/step`). В приложении его не было
// вовсе — менеджер в телефоне видел пустую карточку «Бриф» и не мог ни
// прочитать, что бот выяснил, ни дописать ответ руками.
//
// Здесь две вещи, все — те же данные, что у плагина и бота (`offers_brief`):
//   - GET  /api/offers/offers/{id}/brief          — бриф и журнал реплик;
//   - POST /api/offers/offers/{id}/brief/{topic}  — ответ на вопрос в
//     приложении: тот же шаг диалога, что в боте (`StepBrief`), но под JWT.
//
// Правка полей руками живёт в плагинной ручке `PUT /offers/{id}/brief`
// (`handleSaveBrief`, права `offerForEditor`) — она уже подходит приложению,
// и второй путь для того же поля разошёлся бы при первой правке.
//
// Границы:
//   - читать — `offerForReader` (viewer и видимость), править — `offerForEditor`
//     (владелец или admin): бриф — разговор конкретного менеджера, и роль
//     editor сама по себе чужой бриф не открывает;
//   - пустой ответ отвергается: шаг диалога без текста ничего не значит;
//   - каждая запись двигает `updated_at` (`saveBriefPayload`), иначе правка из
//     приложения не выиграет слияние у плагина;
//   - подсказка для человека («Готово: 6 из 14 тем») считается ТЕМ ЖЕ
//     `briefProgress`, что у файлов и телеграма.

import (
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"mailers/offers/model"
)

// appBriefRequest — тело ответа на вопрос в приложении.
type appBriefRequest struct {
	Text string `json:"text"`
	// Topic — тема, на которую отвечают. Необязательно: сервер берёт текущую
	// (`last_topic`), а расхождение — признак устаревшего экрана, и тогда
	// ответ приходит текстом, а не молча пишется не в то поле.
	Topic string `json:"topic"`
}

// respondBrief — то же тело, что у `handleGetBrief`: бриф + журнал.
func (s *Server) respondBrief(w http.ResponseWriter, r *http.Request, offer Offer) {
	brief, err := s.loadBrief(r.Context(), offer.ID)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusOK, briefResponse{
			Brief: Brief{
				OfferID: offer.ID,
				Payload: model.EmptyBriefPayload(),
				Status:  "absent",
			},
			Turns: []BriefTurn{},
		})
		return
	}
	if err != nil {
		dbError(w, "load brief", err)
		return
	}
	turns, err := s.listTurns(r.Context(), brief.ID, 200)
	if err != nil {
		dbError(w, "list brief turns", err)
		return
	}
	writeJSON(w, http.StatusOK, briefResponse{Brief: brief, Turns: turns})
}

// briefTopicConflict — ответ относится к устаревшему вопросу.
//
// `409` только при ЯВНОМ расхождении двух непустых тем. Пустой `last_topic` —
// законное состояние брифа, который ещё не начинали (КП собрано в плагине:
// приложение показывает «Объект» первым вопросом, а разговора в боте не было).
// Живой дефект стенда 2026-11-15: такой бриф нельзя было заполнить вовсе —
// первый же ответ отвергался «вопрос сменился».
func briefTopicConflict(requested, current string) bool {
	return requested != "" && current != "" && requested != current
}

// handleAppBriefAnswer — ответ на вопрос брифа из приложения.
//
// Шаг диалога ОДИН (`StepBrief`): приложение и бот не должны задавать вопрос
// по-разному. В боте шаг живёт за секретом сервиса и адресатом из `chat_id`;
// здесь адресат — JWT, и тех же прав достаточно: приложение правит КП
// владельца и никакого чужого чата не касается.
//
// Ошибка модели не пишет в бриф: `StepBrief` различает «переспроси» и «модель
// недоступна», и тогда ответ уходит человеку текстом, а набранное не стирается
// (то же правило, что в `handleBotBriefStep`).
func (s *Server) handleAppBriefAnswer(w http.ResponseWriter, r *http.Request) {
	offer, ok := s.offerForEditor(w, r)
	if !ok {
		return
	}
	var req appBriefRequest
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "пустой ответ: напишите, что ответить"})
		return
	}
	topic := strings.TrimSpace(r.PathValue("topic"))

	brief, err := s.createBrief(r.Context(), offer.ID, "")
	if err != nil {
		dbError(w, "brief answer start", err)
		return
	}
	// Устаревший экран: человек отвечал на вопрос, который уже сменился. Писать
	// ответ в новую тему значило бы привязать слова не к тому полю.
	//
	// Пустой `last_topic` — законное состояние брифа, который ещё не начинали
	// (КП собрано в плагине, приложение спрашивает «объект» в первом шаге):
	// тогда ответ принимается и становится первым. Отвергаем только явное
	// расхождение двух разных тем.
	if briefTopicConflict(topic, brief.LastTopic) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "вопрос уже сменился — обновите бриф и ответьте заново",
		})
		return
	}
	// Тема из адреса становится темой ответа: первый ответ на пустой бриф
	// должен попасть в ту тему, которую человек видел на экране.
	if topic != "" {
		brief.LastTopic = topic
	}
	_ = s.appendTurn(r.Context(), brief.ID, "manager", text, brief.LastTopic)

	result, err := StepBrief(r.Context(), brief.Payload, text, llmCompleter{})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": "бриф не ответил: " + err.Error(),
		})
		return
	}
	if !result.Retry && !result.Unavailable {
		brief.Payload = result.Payload
		if result.Done {
			brief.Status = "ready"
		}
		brief.LastTopic = result.NextTopic
		if err := s.saveBriefPayload(r.Context(), brief); err != nil {
			dbError(w, "brief save", err)
			return
		}
	}
	_ = s.appendTurn(r.Context(), brief.ID, "bot", result.Reply, result.NextTopic)
	writeJSON(w, http.StatusOK, map[string]any{
		"brief":       brief,
		"reply":       result.Reply,
		"done":        result.Done,
		"nextTopic":   result.NextTopic,
		"retry":       result.Retry,
		"unavailable": result.Unavailable,
		"skipped":     result.Skipped,
		"progress":    briefProgress(brief.Payload),
	})
}
