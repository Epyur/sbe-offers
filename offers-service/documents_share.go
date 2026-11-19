package main

// Постоянные подписанные ссылки на файлы документов (2026-11-19).
//
// Живая жалоба: в готовом боевом листке две ссылки на документы отвечали
// «unauthorized». Причина не в правах человека: База знаний пишет в карточку
// ПОСТОЯННУЮ ссылку `GET /api/documents/file?key=…`, а эта ручка закрыта JWT
// (`requirePerm("viewer")`). Документ Word и Telegram открывает аноним — у него
// сессии нет, значит ссылка в принципе не могла работать у заказчика.
//
// Решение — своя подписанная ссылка. Печатаем
// `https://<тот же хост>/api/offers/files?key=…&sig=…`, где `sig` — HMAC от
// ключа файла и секрета сервиса. Два свойства выбраны осознанно:
//   - подпись ДЕТЕРМИНИРОВАНА: один ключ всегда даёт одну ссылку, поэтому хэш
//     кэша DOCX не «дрожит» и файл не пересобирается зря;
//   - срока у подписи НЕТ: документ можно переслать через месяц, и он откроется.
//
// Видимость файла по-прежнему решает сервис документов: ручка лишь просит у
// `/internal/documents/agent/file-link` свежую presigned-ссылку (7 дней), а та
// отдаётся только для общего документа (`product_refs` не пусто,
// `no_agent=false`). Отвязали карточку от продукта — ссылка перестала работать,
// и это правильно: доступ определяется данными, а не текстом в документе.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"net/url"
	"strings"

	"mailers/offers/skills"
)

const (
	// documentsFilePath — путь, который База знаний пишет в карточку.
	documentsFilePath = "/api/documents/file"
	// offersFilesPath — наша подписанная ссылка на тот же файл.
	offersFilesPath = "/api/offers/files"
	// documentsShareSigLen — сколько символов подписи оставляем в адресе.
	// 128 бит хватает: подделать перебором нельзя, а адрес остаётся коротким.
	documentsShareSigLen = 32
)

// documentShareSignature — подпись ключа файла.
//
// Префикс «documents» отделяет подпись от любых будущих подписей тем же
// секретом: одна функция на одно назначение, иначе ссылки одного вида можно
// предъявить вместо другого.
func documentShareSignature(key, secret string) string {
	if strings.TrimSpace(key) == "" || strings.TrimSpace(secret) == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("documents\x00" + key))
	return hex.EncodeToString(mac.Sum(nil))[:documentsShareSigLen]
}

// documentShareSignatureValid — подпись совпала. Сравнение постоянное по
// времени: по ответу нельзя угадывать подпись посимвольно.
func documentShareSignatureValid(key, sig string) bool {
	expected := documentShareSignature(key, serviceSecret())
	if expected == "" || sig == "" {
		return false
	}
	return hmac.Equal([]byte(expected), []byte(sig))
}

// sharedDocumentURL — наша ссылка на тот же ключ, в том же origin.
//
// Хост и схема берутся из исходной ссылки: База знаний пишет абсолютный адрес,
// и подменять его своим (с ещё одной настройкой в окружении) незачем.
func sharedDocumentURL(rawURL, secret string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return "", false
	}
	if u.Path != documentsFilePath {
		return "", false
	}
	key := strings.TrimSpace(u.Query().Get("key"))
	if key == "" {
		return "", false
	}
	sig := documentShareSignature(key, secret)
	if sig == "" {
		return "", false
	}
	u.Path = offersFilesPath
	u.Fragment = ""
	q := url.Values{}
	q.Set("key", key)
	q.Set("sig", sig)
	u.RawQuery = q.Encode()
	return u.String(), true
}

// shareDocumentRefs — ссылки документов заменяются подписанными.
//
// Одна точка на все пути печати (КП, боевой листок, макет `{{ссылки}}`): они
// сходятся в `collectOfferRefs`, поэтому второй копии правила нет. Остальные
// ссылки (сайт, техлист в nav.tn.ru) не трогаются.
//
// Пустой секрет (тесты, стенд без окружения) — ссылки остаются как есть: лучше
// рабочая для сотрудника ссылка, чем неподписанная наша, которая всегда 403.
func shareDocumentRefs(refs []skills.CardRef) []skills.CardRef {
	if len(refs) == 0 {
		return refs
	}
	secret := serviceSecret()
	if secret == "" {
		return refs
	}
	out := make([]skills.CardRef, 0, len(refs))
	changed := false
	for _, ref := range refs {
		if shared, ok := sharedDocumentURL(ref.URL, secret); ok {
			if shared != ref.URL {
				changed = true
			}
			ref.URL = shared
		}
		out = append(out, ref)
	}
	if !changed {
		return refs
	}
	return out
}

// handleSharedFile — открыть файл документа по постоянной подписанной ссылке.
//
// Гейт здесь — подпись, а не JWT: ссылку открывает заказчик, у которого сессии
// нет. Поэтому ручка публичная, но без подписи ничего не отдаёт.
func (s *Server) handleSharedFile(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "не задан документ"})
		return
	}
	if !documentShareSignatureValid(key, strings.TrimSpace(r.URL.Query().Get("sig"))) {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": "ссылка недействительна — запросите документ заново",
		})
		return
	}
	docs := NewDocumentsAgentClient()
	if !docs.configured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "сервис документов не подключён",
		})
		return
	}
	link, err := docs.FileLink(r.Context(), key)
	if err != nil {
		// Причина (нет файла, документ не общий, сбой S3) наружу не уходит:
		// наличие закрытого файла — не информация для того, кто открыл ссылку.
		log.Printf("общая ссылка на документ: %v", err)
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "документ недоступен"})
		return
	}
	http.Redirect(w, r, link, http.StatusFound)
}
