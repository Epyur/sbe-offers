package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	pool *pgxpool.Pool
	s3   *S3Store
}

func newServer(pool *pgxpool.Pool, s3 *S3Store) *Server {
	return &Server{pool: pool, s3: s3}
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	if err := loadJWTSecret(); err != nil {
		log.Fatalf("JWT: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("ping: %v", err)
	}

	// Хранилище необязательно: экспорт DOCX отдаёт файл байтами и без S3, а
	// ссылки на файлы честно отвечают «хранилище не настроено». Раньше сервис
	// не стартовал вовсе, если S3 не сконфигурирован.
	s3Store, err := NewS3Store()
	if err != nil {
		log.Printf("S3 недоступен, работаем без хранилища: %v", err)
		s3Store = nil
	}

	s := newServer(pool, s3Store)

	if err := s.migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := s.seedForms(ctx); err != nil {
		log.Fatalf("seedForms: %v", err)
	}

	regCtx, regCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer regCancel()
	if err := s.registerApp(regCtx); err != nil {
		log.Printf("registerApp (non-fatal): %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/offers/health", s.handleHealth)
	mux.Handle("GET /api/offers/docs/", http.StripPrefix("/api/offers/docs/", http.FileServer(http.FS(swaggerFS))))
	mux.HandleFunc("GET /api/offers/openapi.yaml", serveOpenAPI)

	mux.HandleFunc("GET /api/offers/permissions/me", s.requirePerm("viewer")(s.handleMyPermission))
	mux.HandleFunc("GET /api/offers/permissions/list", s.requirePerm("admin")(s.handleListPermissions))
	mux.HandleFunc("POST /api/offers/permissions/set", s.requirePerm("admin")(s.handleSetPermission))

	mux.HandleFunc("GET /api/offers/catalog", s.requirePerm("viewer")(s.handleListCatalog))
	mux.HandleFunc("POST /api/offers/catalog", s.requirePerm("admin")(s.handleSaveCatalog))
	mux.HandleFunc("PUT /api/offers/catalog/{id}", s.requirePerm("admin")(s.handleSaveCatalog))
	mux.HandleFunc("DELETE /api/offers/catalog/{id}", s.requirePerm("admin")(s.handleDeleteCatalog))

	mux.HandleFunc("GET /api/offers/form", s.requirePerm("viewer")(s.handleGetForm))
	mux.HandleFunc("PUT /api/offers/form", s.requirePerm("admin")(s.handleSaveForm))

	mux.HandleFunc("GET /api/offers/offers", s.requirePerm("viewer")(s.handleListOffers))
	mux.HandleFunc("GET /api/offers/offers/{id}", s.requirePerm("viewer")(s.handleGetOffer))
	mux.HandleFunc("POST /api/offers/offers", s.requirePerm("editor")(s.handleSaveOffer))
	mux.HandleFunc("PUT /api/offers/offers/{id}", s.requirePerm("editor")(s.handleSaveOffer))
	mux.HandleFunc("DELETE /api/offers/offers/{id}", s.requirePerm("editor")(s.handleDeleteOffer))
	mux.HandleFunc("POST /api/offers/offers/{id}/export", s.requirePerm("viewer")(s.handleExportDocx))
	mux.HandleFunc("POST /api/offers/offers/{id}/link", s.requirePerm("viewer")(s.handleOfferLink))

	mux.HandleFunc("GET /api/offers/access/list", s.requirePerm("admin")(s.handleListAccess))
	mux.HandleFunc("POST /api/offers/access/role", s.requirePerm("admin")(s.handleSetAccessRole))
	mux.HandleFunc("POST /api/offers/access/codes", s.requirePerm("admin")(s.handleIssueAdminCode))

	mux.HandleFunc("POST /api/offers/bot/request-code", s.requireServiceSecret(s.handleBotRequestCode))
	mux.HandleFunc("POST /api/offers/bot/verify-code", s.requireServiceSecret(s.handleBotVerifyCode))
	mux.HandleFunc("GET /api/offers/bot/identity", s.requireServiceSecret(s.handleBotIdentity))
	mux.HandleFunc("GET /api/offers/bot/catalog", s.requireServiceSecret(s.handleBotCatalog))
	mux.HandleFunc("GET /api/offers/bot/offers", s.requireServiceSecret(s.handleBotListOffers))
	mux.HandleFunc("POST /api/offers/bot/offers", s.requireServiceSecret(s.handleBotCreateOffer))
	mux.HandleFunc("PATCH /api/offers/bot/offers/{id}", s.requireServiceSecret(s.handleBotPatchOffer))
	mux.HandleFunc("POST /api/offers/bot/offers/{id}/docx", s.requireServiceSecret(s.handleBotDocx))
	mux.HandleFunc("POST /api/offers/bot/offers/{id}/link", s.requireServiceSecret(s.handleBotOfferLink))

	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("offers-service listening on :%s", port)
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("ListenAndServe: %v", err)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: %v", err)
	}
}

// dbError — единая точка отказа БД: журнал и одинаковый ответ клиенту.
func dbError(w http.ResponseWriter, action string, err error) {
	log.Printf("%s: %v", action, err)
	writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "db error"})
}

// decodeJSON читает JSON-тело с жёстким лимитом размера.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any, maxBytes int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	return json.NewDecoder(r.Body).Decode(v)
}

// migrate создаёт таблицы сервиса. Все миграции идемпотентны.
func (s *Server) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS offers_catalog (
			id          BIGSERIAL PRIMARY KEY,
			kind        TEXT NOT NULL DEFAULT 'system',
			name        TEXT NOT NULL,
			unit        TEXT NOT NULL DEFAULT 'шт',
			base_price  NUMERIC(14,2) NOT NULL DEFAULT 0,
			description TEXT NOT NULL DEFAULT '',
			composition JSONB NOT NULL DEFAULT '[]',
			photo_key   TEXT NOT NULL DEFAULT '',
			sort_order  INT NOT NULL DEFAULT 0,
			archived    BOOLEAN NOT NULL DEFAULT false,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS offers_form (
			id              BIGSERIAL PRIMARY KEY,
			mode            TEXT NOT NULL UNIQUE DEFAULT 'client',
			title           TEXT NOT NULL DEFAULT 'Коммерческое предложение',
			placeholders    JSONB NOT NULL DEFAULT '[]',
			blocks          JSONB NOT NULL DEFAULT '[]',
			required_fields JSONB NOT NULL DEFAULT '[]',
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS offers_offer (
			id          BIGSERIAL PRIMARY KEY,
			number      TEXT NOT NULL UNIQUE,
			owner_email TEXT NOT NULL,
			client      JSONB NOT NULL DEFAULT '{}',
			items       JSONB NOT NULL DEFAULT '[]',
			form_values JSONB NOT NULL DEFAULT '{}',
			total       NUMERIC(14,2) NOT NULL DEFAULT 0,
			status      TEXT NOT NULL DEFAULT 'draft',
			docx_key    TEXT NOT NULL DEFAULT '',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS bot_codes (
			id         BIGSERIAL PRIMARY KEY,
			email      TEXT NOT NULL,
			code       TEXT NOT NULL,
			kind       TEXT NOT NULL DEFAULT 'signal',
			chat_id    TEXT NOT NULL DEFAULT '',
			expires_at TIMESTAMPTZ NOT NULL,
			used_at    TIMESTAMPTZ,
			attempts   INT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS bot_codes_email_idx ON bot_codes (email)`,
		`CREATE TABLE IF NOT EXISTS offers_bot_link (
			chat_id      TEXT PRIMARY KEY,
			email        TEXT NOT NULL,
			confirmed_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS offers_access (
			email      TEXT PRIMARY KEY,
			role       TEXT NOT NULL DEFAULT 'viewer',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}
