package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Роль, которую приложение показывает человеку, должна быть ТОЙ ЖЕ, что
// проверит requirePerm на следующем запросе (2026-11-13, живая проверка S1:
// вход отдавал role:"", а список КП — 403; расхождение искали наугад).
//
// Правило одно и то же: карта app_permissions → общий доступ приложения.
func TestAppRoleMatchesPermissionGate(t *testing.T) {
	now := time.Now()
	defer withPermMutation(t, map[string]string{
		"editor@tn.ru":  "editor",
		"common@tn.ru":  "",
		"blocked@tn.ru": "",
	}, "viewer", true)

	s := stubServer(map[string]string{"editor@tn.ru": "editor"}, "viewer", true)
	_ = now

	cases := map[string]struct {
		role     string
		minRole  string
		wantPass bool
	}{
		"персональная роль": {"editor@tn.ru", "editor", true},
		"общий доступ":      {"common@tn.ru", "viewer", true},
	}
	for name, c := range cases {
		role, err := s.effectiveRole(t.Context(), "offers", c.role)
		if err != nil {
			t.Fatalf("%s: effectiveRole: %v", name, err)
		}
		passed := roleRank(role) >= roleRank(c.minRole)
		if passed != c.wantPass {
			t.Errorf("%s: роль %q даёт %v для %q, хотели %v", name, role, passed, c.minRole, c.wantPass)
		}
	}
}

// JWT приложения — тот же формат, что у плагина: тот же гейт, те же роли.
func TestSignAppJWTIsAcceptedByGate(t *testing.T) {
	withPermMutation(t, map[string]string{"app-user@tn.ru": "editor"}, "", false)

	old := jwtSecret
	jwtSecret = []byte("test-service-secret")
	t.Cleanup(func() { jwtSecret = old })

	now := time.Now()
	token, err := signAppJWT("app-user@tn.ru", "app:555", now)
	if err != nil {
		t.Fatalf("подпись: %v", err)
	}

	s := stubServer(map[string]string{"app-user@tn.ru": "editor"}, "", false)
	req := httptest.NewRequest(http.MethodGet, "/api/offers/offers", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	email, status, cause, errText := s.authorize(req, "editor")
	if status != 0 {
		t.Fatalf("токен приложения не прошёл гейт: %d %s %s", status, cause, errText)
	}
	if email != "app-user@tn.ru" {
		t.Errorf("email = %q", email)
	}
}

// Токен приложения не должен проходить выше своей роли.
func TestSignAppJWTCannotEscalate(t *testing.T) {
	old := jwtSecret
	jwtSecret = []byte("test-service-secret")
	t.Cleanup(func() { jwtSecret = old })

	token, err := signAppJWT("viewer@tn.ru", "app:1", time.Now())
	if err != nil {
		t.Fatalf("подпись: %v", err)
	}

	s := stubServer(map[string]string{"viewer@tn.ru": "viewer"}, "", false)
	req := httptest.NewRequest(http.MethodPost, "/api/offers/offers", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, status, cause, _ := s.authorize(req, "editor")
	if status != http.StatusForbidden || cause != authCauseRole {
		t.Fatalf("просмотрщик получил %d %s, ожидали 403 role", status, cause)
	}
}

// Без секрета токен не подписывается вовсе: иначе вход выдал бы токен,
// который не проверит никто (или проверит чужой ключ).
func TestSignAppJWTRequiresSecret(t *testing.T) {
	old := jwtSecret
	jwtSecret = nil
	t.Cleanup(func() { jwtSecret = old })

	if _, err := signAppJWT("a@tn.ru", "app:1", time.Now()); err == nil {
		t.Fatal("токен подписан без ключа")
	}
}

// device_id отличает приложение от устройства вольта: одна и та же почта
// может быть и в плагине, и в боте.
func TestSignAppJWTCarriesDeviceID(t *testing.T) {
	old := jwtSecret
	jwtSecret = []byte("test-service-secret")
	t.Cleanup(func() { jwtSecret = old })

	token, err := signAppJWT("a@tn.ru", "app:777", time.Now())
	if err != nil {
		t.Fatalf("подпись: %v", err)
	}
	claims, err := parseJWT(token)
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if claims.DeviceID != "app:777" {
		t.Errorf("device_id = %q", claims.DeviceID)
	}
	if claims.AppID != "offers" {
		t.Errorf("app_id = %q", claims.AppID)
	}
	if !strings.EqualFold(claims.Email, "a@tn.ru") {
		t.Errorf("email = %q", claims.Email)
	}
	if claims.ExpiresAt == nil || claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time) != appJWTTTL {
		t.Errorf("срок токена не appJWTTTL")
	}
}
