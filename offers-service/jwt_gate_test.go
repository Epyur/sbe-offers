package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// makeTestJWT — подписанный токен для проверки прав.
func makeTestJWT(t *testing.T, appID, email string, secret []byte, exp time.Time) string {
	t.Helper()
	claims := &jwtClaims{Email: email, AppID: appID}
	claims.ExpiresAt = jwt.NewNumericDate(exp)
	claims.Issuer = "auth-service"
	claims.Audience = jwt.ClaimStrings{appID}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("подпись токена: %v", err)
	}
	return signed
}

// stubServer — сервер с заданной картой прав (без БД и сети).
func stubServer(roles map[string]string, commonAccess string, commonSet bool) *Server {
	s := &Server{}
	appPerms.mu.Lock()
	appPerms.snap = permSnapshot{
		Roles:           roles,
		CommonAccess:    commonAccess,
		CommonAccessSet: commonSet,
		FetchedAt:       time.Now(),
	}
	appPerms.loaded = true
	appPerms.mu.Unlock()
	return s
}

func withPermMutation(t *testing.T, roles map[string]string, commonAccess string, commonSet bool) {
	t.Helper()
	old := appPerms.snap
	oldLoaded := appPerms.loaded
	stubServer(roles, commonAccess, commonSet)
	t.Cleanup(func() {
		appPerms.mu.Lock()
		appPerms.snap = old
		appPerms.loaded = oldLoaded
		appPerms.mu.Unlock()
	})
}

func TestRequirePermAllowsSuperadmin(t *testing.T) {
	secret := []byte("test-secret")
	jwtSecret = secret
	t.Setenv("OFFERS_APP_ID", "offers")
	withPermMutation(t, map[string]string{"owner@tn.ru": "superadmin"}, "", false)

	s := &Server{}
	handler := s.requirePerm("admin")(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+makeTestJWT(t, "offers", "owner@tn.ru", secret, time.Now().Add(time.Hour)))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("superadmin должен проходить admin-гейт, получили %d", rec.Code)
	}
}

func TestRequirePermRejectsViewerOnAdminGate(t *testing.T) {
	secret := []byte("test-secret")
	jwtSecret = secret
	t.Setenv("OFFERS_APP_ID", "offers")
	withPermMutation(t, map[string]string{"user@tn.ru": "viewer"}, "", false)

	s := &Server{}
	handler := s.requirePerm("admin")(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+makeTestJWT(t, "offers", "user@tn.ru", secret, time.Now().Add(time.Hour)))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer не должен проходить admin-гейт, получили %d", rec.Code)
	}
}

func TestRequirePermWithoutToken(t *testing.T) {
	jwtSecret = []byte("test-secret")
	t.Setenv("OFFERS_APP_ID", "offers")
	s := &Server{}
	handler := s.requirePerm("viewer")(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("без токена ожидали 401, получили %d", rec.Code)
	}
}

func TestRequirePermRejectsForeignAppID(t *testing.T) {
	secret := []byte("test-secret")
	jwtSecret = secret
	t.Setenv("OFFERS_APP_ID", "offers")
	withPermMutation(t, map[string]string{"owner@tn.ru": "superadmin"}, "", false)

	s := &Server{}
	handler := s.requirePerm("viewer")(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+makeTestJWT(t, "documents", "owner@tn.ru", secret, time.Now().Add(time.Hour)))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("токен чужого приложения должен получать 403, получили %d", rec.Code)
	}
}

func TestRequirePermRejectsExpiredToken(t *testing.T) {
	secret := []byte("test-secret")
	jwtSecret = secret
	t.Setenv("OFFERS_APP_ID", "offers")
	withPermMutation(t, map[string]string{"owner@tn.ru": "superadmin"}, "", false)

	s := &Server{}
	handler := s.requirePerm("viewer")(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+makeTestJWT(t, "offers", "owner@tn.ru", secret, time.Now().Add(-time.Hour)))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("просроченный токен — 401, получили %d", rec.Code)
	}
}

func TestRequirePermUsesCommonAccessWhenNoPersonalRole(t *testing.T) {
	secret := []byte("test-secret")
	jwtSecret = secret
	t.Setenv("OFFERS_APP_ID", "offers")
	withPermMutation(t, map[string]string{}, "editor", true)

	s := &Server{}
	var seen string
	handler := s.requirePerm("editor")(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = r.Context().Value(permEmailCtx{}).(string)
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+makeTestJWT(t, "offers", "any@tn.ru", secret, time.Now().Add(time.Hour)))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("общий доступ editor должен пускать на editor-гейт, получили %d", rec.Code)
	}
	if seen != "any@tn.ru" {
		t.Fatalf("email должен попасть в контекст, получили %q", seen)
	}
}

func TestLoadJWTSecretRequiresEnv(t *testing.T) {
	t.Setenv("OFFERS_APP_ID", "offers")
	os.Unsetenv("OFFERS_SERVICE_SECRET")
	if err := loadJWTSecret(); err == nil {
		t.Fatal("без OFFERS_SERVICE_SECRET должна быть ошибка")
	} else if !strings.Contains(err.Error(), "OFFERS_SERVICE_SECRET") {
		t.Fatalf("в ошибке должно быть имя переменной, получили %q", err.Error())
	}
}

func TestAppIDFromEnvDefault(t *testing.T) {
	os.Unsetenv("OFFERS_APP_ID")
	if got := appIDFromEnv(); got != "offers" {
		t.Fatalf("по умолчанию app_id=offers, получили %q", got)
	}
	_ = context.Background()
}
