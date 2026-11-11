package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// tgSign — эталон подписи Telegram. Написан по документации Telegram Mini
// Apps, а не повторяет продовый код: иначе тест проверял бы сам себя.
//
//	data-check-string = "k=v" через \n, ключи по алфавиту, без hash
//	secret_key        = HMAC_SHA256("WebAppData", bot_token)
//	hash              = hex(HMAC_SHA256(secret_key, data_check_string))
func tgSign(t *testing.T, botToken string, fields map[string]string) string {
	t.Helper()

	pairs := make([]string, 0, len(fields))
	for k, v := range fields {
		pairs = append(pairs, k+"="+v)
	}
	sort.Strings(pairs)

	secret := hmacSHA256([]byte("WebAppData"), []byte(botToken))
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(strings.Join(pairs, "\n")))
	return hex.EncodeToString(mac.Sum(nil))
}

func hmacSHA256(key, msg []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(msg)
	return mac.Sum(nil)
}

// initData — строка, которую присылает Telegram: те же поля, `user` в JSON.
func initData(t *testing.T, botToken string, userID int64, authDate time.Time, extra map[string]string) string {
	t.Helper()

	fields := map[string]string{
		"auth_date":     strconv.FormatInt(authDate.Unix(), 10),
		"query_id":      "AAHdF6IQAAAAAN0XohDhrOrc",
		"chat_instance": "-1234567890123456789",
		"chat_type":     "private",
		"user":          `{"id":` + strconv.FormatInt(userID, 10) + `,"first_name":"Иван","last_name":"Петров","username":"ivan"}`,
	}
	for k, v := range extra {
		fields[k] = v
	}

	fields["hash"] = tgSign(t, botToken, fields)

	values := url.Values{}
	for k, v := range fields {
		values.Set(k, v)
	}
	return values.Encode()
}

const testBotToken = "123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"

// Свежая подпись и известный токен — единственное, что пропускается.
func TestVerifyInitDataAcceptsFresh(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	data := initData(t, testBotToken, 555, now.Add(-time.Minute), nil)

	user, err := verifyInitData(data, testBotToken, now, 24*time.Hour)
	if err != nil {
		t.Fatalf("ожидали успех, получили %v", err)
	}
	if user.ID != 555 {
		t.Errorf("user.ID = %d, want 555", user.ID)
	}
	if user.FirstName != "Иван" {
		t.Errorf("user.FirstName = %q", user.FirstName)
	}
}

// Чужой (или отменённый) токен — подпись не сходится.
func TestVerifyInitDataRejectsWrongToken(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	data := initData(t, testBotToken, 555, now, nil)

	if _, err := verifyInitData(data, "999:WRONG", now, 24*time.Hour); err == nil {
		t.Fatal("подпись чужим токеном прошла — это дыра входа")
	}
}

// Одно изменённое поле (подмена user) — подпись не сходится.
func TestVerifyInitDataRejectsTamperedField(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	data := initData(t, testBotToken, 555, now, nil)
	tampered := strings.Replace(data, "%22id%22%3A555", "%22id%22%3A777", 1)
	if tampered == data {
		t.Fatal("тест сломан: подмена не внесена")
	}

	if _, err := verifyInitData(tampered, testBotToken, now, 24*time.Hour); err == nil {
		t.Fatal("подмена user прошла проверку подписи")
	}
}

// Устаревшая подпись — отказ даже при верном hash.
func TestVerifyInitDataRejectsStale(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	data := initData(t, testBotToken, 555, now.Add(-25*time.Hour), nil)

	if _, err := verifyInitData(data, testBotToken, now, 24*time.Hour); err == nil {
		t.Fatal("initData 25 часов назад принят при сроке 24 часа")
	}
}

// Дубликат ключа — попытка провести две разные «даты» одной подписью.
func TestVerifyInitDataRejectsDuplicateKeys(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	data := initData(t, testBotToken, 555, now, nil)
	data += "&auth_date=" + strconv.FormatInt(now.Add(-100*time.Hour).Unix(), 10)

	if _, err := verifyInitData(data, testBotToken, now, 24*time.Hour); err == nil {
		t.Fatal("строка с двумя auth_date прошла проверку")
	}
}

// Слишком большое тело не разбираем вовсе.
func TestVerifyInitDataRejectsOversize(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	data := initData(t, testBotToken, 555, now, nil)
	data += "&pad=" + strings.Repeat("x", 17*1024)

	if _, err := verifyInitData(data, testBotToken, now, 24*time.Hour); err == nil {
		t.Fatal("initData больше 16 КиБ принят")
	}
}

// Без hash, без user, мусор — отказ, а не паника.
func TestVerifyInitDataRejectsMalformed(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	noHash := "auth_date=1&user=%7B%22id%22%3A1%7D"
	badUserHash := initData(t, testBotToken, 555, now, map[string]string{"user": "not-json"})

	cases := map[string]string{
		"пусто":        "",
		"без hash":     noHash,
		"битый user":   badUserHash,
		"просто мусор": "%%%",
	}
	for name, data := range cases {
		if _, err := verifyInitData(data, testBotToken, now, 24*time.Hour); err == nil {
			t.Errorf("%s: ожидали отказ, получили успех", name)
		}
	}
}

// Пустой токен бота — не «пропустить всех», а отказ: настроить бота забыли.
func TestVerifyInitDataRejectsEmptyToken(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	data := initData(t, testBotToken, 555, now, nil)

	if _, err := verifyInitData(data, "", now, 24*time.Hour); err == nil {
		t.Fatal("без токена бота проверка прошла")
	}
}

// Срок из окружения: не число и не положительное — берём сутки, а не 0.
func TestInitDataTTLFromEnv(t *testing.T) {
	t.Setenv("OFFERS_APP_INITDATA_TTL", "")
	if got := initDataTTL(); got != 24*time.Hour {
		t.Errorf("по умолчанию = %v, want 24h", got)
	}
	t.Setenv("OFFERS_APP_INITDATA_TTL", "2h")
	if got := initDataTTL(); got != 2*time.Hour {
		t.Errorf("2h = %v", got)
	}
	t.Setenv("OFFERS_APP_INITDATA_TTL", "abc")
	if got := initDataTTL(); got != 24*time.Hour {
		t.Errorf("мусор = %v, want 24h (fallback)", got)
	}
	t.Setenv("OFFERS_APP_INITDATA_TTL", "-5m")
	if got := initDataTTL(); got != 24*time.Hour {
		t.Errorf("отрицательный = %v, want 24h (fallback)", got)
	}
}
