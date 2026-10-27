package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// registerApp сообщает auth-service о приложении offers (service_secret +
// owner_email). Ошибка не фатальна: сервис работает и без регистрации, права
// при этом резолвятся по уже заведённой карте.
func (s *Server) registerApp(ctx context.Context) error {
	authURL := os.Getenv("AUTH_SERVICE_URL")
	if authURL == "" {
		authURL = "http://auth-service:3000"
	}
	serviceSecret := os.Getenv("OFFERS_SERVICE_SECRET")
	if serviceSecret == "" {
		return fmt.Errorf("OFFERS_SERVICE_SECRET is required for /apps/register")
	}
	name := os.Getenv("OFFERS_APP_NAME")
	if name == "" {
		name = "Offers"
	}

	body, err := json.Marshal(map[string]string{
		"app_id":         appIDFromEnv(),
		"name":           name,
		"owner_email":    ownerEmailFromEnv(),
		"service_secret": serviceSecret,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authURL+"/apps/register", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("apps/register status %d", resp.StatusCode)
	}
	log.Printf("registerApp: %s registered", appIDFromEnv())
	return nil
}

func ownerEmailFromEnv() string {
	return os.Getenv("OFFERS_OWNER_EMAIL")
}
