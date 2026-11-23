package main

// Картинки Технодайджеста в Mini App и в чате (2026-11-23).
//
// Почему ручка понадобилась. У новости выпуска есть картинка (`image_hash` и
// `image_name`), но взять её приложению негде: файл лежит в хранилище Базы
// знаний, а её ручки закрыты JWT с `app_id=kb` — ключ Mini App туда не пускают
// («ключ выдан другому приложению»). Прямой адрес в ответе тоже не годится: он
// требовал бы заголовка авторизации, которого у `<img>` нет.
//
// Поэтому тот же приём, что у постоянных ссылок на документы
// (`documents_share.go`): сервис отдаёт СВОЙ адрес с подписью
// `GET /api/offers/digest/image?hash=…&name=…&sig=…`. Подпись считается от
// хеша, имени и секрета сервиса — угадать её нельзя, поэтому ручка публичная
// (у `<img>` нет и нашего JWT), а её гейт — подпись. Байты забирает сервис:
// `kb.FileLink` по ключу хранилища → presigned-ссылка → чтение.
//
// Ключ хранилища собирается как `kb/<content_hash>/<имя>` — это правило самого
// хранилища Базы знаний (`scripts/kb_s3_gc.sh`: «первый сегмент пути — хэш»).
// Имя проверяется: без косых черт и переходов вверх, иначе подписанный адрес
// стал бы способом попросить из хранилища произвольный объект.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const (
	// digestImageSigLen — сколько символов подписи оставляем в адресе: 128 бит
	// хватает, а адрес остаётся коротким (то же правило, что у документов).
	digestImageSigLen = 32

	// digestPhotoMaxBytes — предел Telegram для `sendPhoto`. Тяжелее он не
	// примет; тогда новость уйдёт текстом, как раньше. У самой отдачи картинки
	// свой предел — в источнике (`KBImageSource.Get`, 12 МБ): второй предел
	// здесь означал бы две правды об одном ограничении.
	digestPhotoMaxBytes = 10 << 20

	// digestCaptionLimit — предел подписи у `sendPhoto`, в СИМВОЛАХ (Telegram
	// считает знаки, а не байты: кириллица идёт по одному).
	digestCaptionLimit = 1000

	// digestMessageLimit — предел одного сообщения Telegram (4096) с запасом:
	// новость уходит ОДНИМ сообщением, и предел держим здесь, а не надеемся,
	// что бот не станет её резать. Запас 296 знаков — под эмодзи и суррогатные
	// пары: Telegram считает символы, а строка в JavaScript — единицы UTF-16.
	digestMessageLimit = 3800

	// digestImageFolder — первый сегмент ключа хранилища Базы знаний.
	digestImageFolder = "kb/"
)

// digestImageHashRe — хеш содержимого: hex, 32–128 знаков. Мусор отсекается до
// сети и до хранилища.
var digestImageHashRe = regexp.MustCompile(`^[0-9a-f]{32,128}$`)

// digestImageSignature — подпись картинки.
//
// Префикс «digest-image» отделяет её от подписи документов тем же секретом:
// одна функция на одно назначение, иначе ссылку одного вида можно предъявить
// вместо другого.
func digestImageSignature(hash, name, secret string) string {
	if strings.TrimSpace(hash) == "" || strings.TrimSpace(secret) == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("digest-image\x00" + hash + "\x00" + name))
	return hex.EncodeToString(mac.Sum(nil))[:digestImageSigLen]
}

// digestImageSignatureValid — подпись совпала. Сравнение постоянное по времени:
// по ответу нельзя угадывать подпись посимвольно.
func digestImageSignatureValid(hash, name, sig string) bool {
	expected := digestImageSignature(hash, name, serviceSecret())
	if expected == "" || sig == "" {
		return false
	}
	return hmac.Equal([]byte(expected), []byte(sig))
}

// digestImageNameOK — имя файла безопасно для ключа хранилища.
//
// Пустое имя — не ошибка клиента, а честное «картинки нет»: у старой новости
// мог остаться только хеш. Косые черты и «..» запрещены: подписанный адрес не
// должен давать доступ к чужим объектам хранилища.
func digestImageNameOK(name string) bool {
	if name == "" || len(name) > 200 {
		return false
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return false
	}
	return true
}

// digestImageObjectKey — ключ хранилища по хешу и имени.
func digestImageObjectKey(hash, name string) string {
	return digestImageFolder + hash + "/" + name
}

// digestImageSignedPath — наш подписанный адрес картинки (путь БЕЗ `/api/offers`).
//
// Путь, а не полный адрес: сервис не знает, по какому имени его видит клиент, а
// приложение склеивает адрес из своего `apiBase` — одна правда об адресе API.
// Пустой секрет (стенд без окружения) — пустая строка: лучше показать новость
// без картинки, чем ссылку, которая всегда 403.
func digestImageSignedPath(hash, name string) string {
	hash = strings.TrimSpace(hash)
	name = strings.TrimSpace(name)
	if !digestImageHashRe.MatchString(hash) || !digestImageNameOK(name) {
		return ""
	}
	sig := digestImageSignature(hash, name, serviceSecret())
	if sig == "" {
		return ""
	}
	q := url.Values{}
	q.Set("hash", hash)
	q.Set("name", name)
	q.Set("sig", sig)
	return "/digest/image?" + q.Encode()
}

// handleAppDigestImage — отдать картинку новости по подписанной ссылке.
//
// Гейт здесь — подпись, а не JWT: картинку грузит `<img>`, у которого нет
// заголовка авторизации. Без подписи ручка не отдаёт ничего.
func (s *Server) handleAppDigestImage(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	hash := strings.TrimSpace(query.Get("hash"))
	name := strings.TrimSpace(query.Get("name"))
	if !digestImageHashRe.MatchString(hash) || !digestImageNameOK(name) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "картинка не указана"})
		return
	}
	if !digestImageSignatureValid(hash, name, strings.TrimSpace(query.Get("sig"))) {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": "ссылка на картинку недействительна — обновите выпуск",
		})
		return
	}
	if s.kb == nil || !s.kb.configured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "картинки новостей недоступны: База знаний не настроена",
		})
		return
	}
	src := NewKBImageSource(s.kb)
	if src == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "картинки новостей недоступны: База знаний не настроена",
		})
		return
	}
	data, err := src.Get(r.Context(), digestImageObjectKey(hash, name))
	if err != nil {
		// Причина (нет объекта, сбой хранилища) наружу не уходит: наличие
		// закрытого файла — не информация для того, кто открыл картинку.
		log.Printf("digest: картинка %s не прочитана: %v", hash, err)
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "картинка недоступна"})
		return
	}
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		log.Printf("digest: по хешу %s пришёл не образ (%s)", hash, mime)
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "в хранилище не картинка"})
		return
	}
	w.Header().Set("Content-Type", mime)
	// Картинка новости не меняется: хеш — это её содержимое. Час кэша хватает,
	// чтобы лента не перезагружала один и тот же кадр при каждом возврате.
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
