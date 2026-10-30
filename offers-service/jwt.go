package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret []byte

// loadJWTSecret — per-service ключ: токены подписаны нашим *_SERVICE_SECRET
// (тем же, что у /apps/register), а не общим JWT_SECRET.
func loadJWTSecret() error {
	key := strings.ToUpper(appIDFromEnv()) + "_SERVICE_SECRET"
	secret := os.Getenv(key)
	if secret == "" {
		return fmt.Errorf("%s is required", key)
	}
	jwtSecret = []byte(secret)
	return nil
}

type jwtClaims struct {
	Email    string `json:"email"`
	DeviceID string `json:"device_id"`
	AppID    string `json:"app_id"`
	jwt.RegisteredClaims
}

func parseJWT(tokenStr string) (*jwtClaims, error) {
	claims, err := parseWithKey(tokenStr, jwtSecret)
	if err != nil {
		if legacy := os.Getenv("JWT_SECRET"); legacy != "" {
			if c, e2 := parseWithKey(tokenStr, []byte(legacy)); e2 == nil {
				return c, nil
			}
		}
	}
	return claims, err
}

func parseWithKey(tokenStr string, key []byte) (*jwtClaims, error) {
	claims := &jwtClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return key, nil
	}, jwt.WithExpirationRequired(), jwt.WithLeeway(30*time.Second))
	if err != nil || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func claimStringsContains(a jwt.ClaimStrings, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}

func appIDFromEnv() string {
	if v := os.Getenv("OFFERS_APP_ID"); v != "" {
		return v
	}
	return "offers"
}

// isAdminRole — админ приложения ИЛИ глобальный superadmin из auth-service.
// Одна функция на весь сервис: проверки ole == "admin" в четырёх местах
// не пускали владельца приложения.
func isAdminRole(role string) bool {
	return role == "admin" || role == "superadmin"
}

func roleRank(role string) int {
	switch role {
	case "superadmin":
		// Глобальный админ (роль из центральной таблицы auth-service) — выше
		// админа приложения; без этой ветки владелец приложения получал 403.
		return 4
	case "admin":
		return 3
	case "editor":
		return 2
	case "viewer":
		return 1
	default:
		return 0
	}
}

type permEmailCtx struct{}

// Причины отказа по токену. Строки уходят в журнал, поэтому это константы, а
// не свободный текст: по ним ищут в журнале, когда приходит жалоба.
const (
	authCauseNoToken  = "no-token"
	authCauseBadToken = "bad-token"
	authCauseIssuer   = "issuer"
	authCauseAudience = "audience"
	authCauseAppID    = "app-id"
	authCauseRole     = "role"
)

// authLogWriter — куда пишется отказ. Переменная, а не прямой log.Printf,
// чтобы тест мог проверить и содержимое, и отсутствие в нём токена.
var authLogWriter io.Writer = os.Stderr

// logAuthRejection оставляет след об отказе: метод, путь, причину.
//
// Токена и тела здесь нет намеренно: токен — действующий ключ доступа, а в
// теле бывают данные заказчика. Без такой записи живая жалоба «ключ
// недействителен» неразбираема (2026-10-30: в журнале было только «listening»).
func logAuthRejection(r *http.Request, cause string) {
	if r == nil {
		return
	}
	path := r.URL.Path
	_, _ = fmt.Fprintf(authLogWriter, "auth reject: %s %s %s\n", r.Method, path, cause)
}

// effectiveRole — роль пользователя в приложении: персональная, иначе общий
// доступ приложения (источник правды — auth-service, см. authperms.go).
func (s *Server) effectiveRole(ctx context.Context, appID, email string) (string, error) {
	_ = appID
	return roleFromSnapshot(s.permissionsSnapshot(ctx), email, ""), nil
}

// reject отвечает отказом и пишет причину в журнал — одна точка на все ветки,
// чтобы новая ветка не забыла запись.
func reject(w http.ResponseWriter, r *http.Request, status int, cause, errText string) {
	logAuthRejection(r, cause)
	writeJSON(w, status, map[string]any{"error": errText})
}

// authorize проверяет токен и права. Возвращает email и код отказа; пустая
// строка кода значит «пропущен». Вместо шести почти одинаковых writeJSON в
// замыкании: гейт читается сверху вниз, причины названы в одном месте.
func (s *Server) authorize(r *http.Request, minRole string) (string, int, string, string) {
	appID := appIDFromEnv()

	tokenStr := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer"))
	if tokenStr == "" {
		return "", http.StatusUnauthorized, authCauseNoToken, "unauthorized"
	}
	claims, err := parseJWT(tokenStr)
	if err != nil {
		return "", http.StatusUnauthorized, authCauseBadToken, "unauthorized"
	}
	if claims.AppID != appID {
		return "", http.StatusForbidden, authCauseAppID, "forbidden"
	}
	if claims.Issuer != "" && claims.Issuer != "auth-service" {
		return "", http.StatusUnauthorized, authCauseIssuer, "unauthorized"
	}
	if len(claims.Audience) > 0 && !claimStringsContains(claims.Audience, appID) {
		return "", http.StatusUnauthorized, authCauseAudience, "unauthorized"
	}

	role, err := s.effectiveRole(r.Context(), claims.AppID, claims.Email)
	if err != nil {
		return "", http.StatusInternalServerError, "", "db error"
	}
	if roleRank(role) < roleRank(minRole) {
		return "", http.StatusForbidden, authCauseRole, "forbidden: insufficient role"
	}
	return claims.Email, 0, "", ""
}

func (s *Server) requirePerm(minRole string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			email, status, cause, errText := s.authorize(r, minRole)
			if status != 0 {
				if cause == "" {
					// Сбой БД: писать его как отказ по токену нельзя — это
					// другая причина, и она уже попадёт в журнал через dbError.
					writeJSON(w, status, map[string]any{"error": errText})
					return
				}
				reject(w, r, status, cause, errText)
				return
			}
			ctx := context.WithValue(r.Context(), permEmailCtx{}, email)
			next(w, r.WithContext(ctx))
		}
	}
}
