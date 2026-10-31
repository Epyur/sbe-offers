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
	// kb — карточки систем из Базы знаний (2026-10-29). Может быть выключен:
	// пустой KB_SERVICE_URL означает «работаем без карточек».
	kb *KBClient
}

func newServer(pool *pgxpool.Pool, s3 *S3Store, kb *KBClient) *Server {
	return &Server{pool: pool, s3: s3, kb: kb}
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

	kb := NewKBClient()
	if kb.configured() {
		log.Printf("База знаний: карточки систем читаются с %s", kb.baseURL)
	} else {
		log.Printf("KB_SERVICE_URL не задан: системы работают без карточек")
	}

	s := newServer(pool, s3Store, kb)

	if err := s.migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := s.seedForms(ctx); err != nil {
		log.Fatalf("seedForms: %v", err)
	}
	if err := s.seedContext(ctx); err != nil {
		log.Fatalf("seedContext: %v", err)
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

	// Материалы (2026-10-30). Отдельная сущность с собственным SKU и
	// вариациями ЕКН; системы живут в карточках Базы знаний и сюда не попадают.
	// SKU назначает только сервер — клиентское значение не принимается.
	mux.HandleFunc("GET /api/offers/materials", s.requirePerm("viewer")(s.handleListMaterials))
	mux.HandleFunc("POST /api/offers/materials", s.requirePerm("admin")(s.handleSaveMaterial))
	mux.HandleFunc("PUT /api/offers/materials/{id}", s.requirePerm("admin")(s.handleSaveMaterial))
	mux.HandleFunc("DELETE /api/offers/materials/{id}", s.requirePerm("admin")(s.handleDeleteMaterial))
	mux.HandleFunc("POST /api/offers/materials/{id}/ekn", s.requirePerm("admin")(s.handleAddMaterialEkn))
	mux.HandleFunc("DELETE /api/offers/materials/{id}/ekn/{eknId}", s.requirePerm("admin")(s.handleDeleteMaterialEkn))

	// Таблица менеджера (2026-10-30), двумя шагами. Первый шаг НИЧЕГО не
	// пишет: файл от заказчика бывает черновиком со старыми ценами, и молча
	// применять его нельзя. Второй пишет только подтверждённое.
	mux.HandleFunc("POST /api/offers/offers/{id}/sheet", s.requirePerm("editor")(s.handleSheetPreview))
	mux.HandleFunc("POST /api/offers/offers/{id}/sheet/apply", s.requirePerm("editor")(s.handleSheetApply))

	mux.HandleFunc("GET /api/offers/form", s.requirePerm("viewer")(s.handleGetForm))
	// Контекст КП (2026-10-29): карточки систем из Базы знаний. Список из кеша
	// читает любой, кто видит КП; перечитывает базу знаний — администратор.
	mux.HandleFunc("GET /api/offers/context/cards", s.requirePerm("viewer")(s.handleGetSystemCards))
	mux.HandleFunc("POST /api/offers/context/refresh", s.requirePerm("admin")(s.handleRefreshSystemCards))
	// Системы (Task 6, 2026-10-31): карточки Базы знаний без тега material,
	// с составом, разрешённым против справочника материалов. Своей цены у
	// системы нет — она показывает, ИЗ ЧЕГО складывается стоимость.
	mux.HandleFunc("GET /api/offers/systems", s.requirePerm("viewer")(s.handleListSystems))
	mux.HandleFunc("PUT /api/offers/form", s.requirePerm("admin")(s.handleSaveForm))

	mux.HandleFunc("GET /api/offers/offers", s.requirePerm("viewer")(s.handleListOffers))
	mux.HandleFunc("GET /api/offers/offers/{id}", s.requirePerm("viewer")(s.handleGetOffer))
	mux.HandleFunc("POST /api/offers/offers", s.requirePerm("editor")(s.handleSaveOffer))
	mux.HandleFunc("PUT /api/offers/offers/{id}", s.requirePerm("editor")(s.handleSaveOffer))
	mux.HandleFunc("DELETE /api/offers/offers/{id}", s.requirePerm("editor")(s.handleDeleteOffer))
	mux.HandleFunc("POST /api/offers/offers/{id}/export", s.requirePerm("viewer")(s.handleExportDocx))
	mux.HandleFunc("POST /api/offers/offers/{id}/link", s.requirePerm("viewer")(s.handleOfferLink))
	// Бриф в плагине (Task 10, 2026-10-30): чтение с историей разговора и
	// правка ответов. Правит владелец предложения или admin — проверка прав
	// внутри ручки, потому что роль editor не даёт правки чужого КП.
	mux.HandleFunc("GET /api/offers/offers/{id}/brief", s.requirePerm("viewer")(s.handleGetBrief))
	mux.HandleFunc("PUT /api/offers/offers/{id}/brief", s.requirePerm("editor")(s.handleSaveBrief))

	mux.HandleFunc("GET /api/offers/access/list", s.requirePerm("admin")(s.handleListAccess))
	mux.HandleFunc("POST /api/offers/access/role", s.requirePerm("admin")(s.handleSetAccessRole))
	mux.HandleFunc("POST /api/offers/access/codes", s.requirePerm("admin")(s.handleIssueAdminCode))

	// Токен своего Telegram-бота (2026-10-29): вводится из настроек плагина,
	// хранится здесь и больше нигде. Чтение/запись — только администратору,
	// сам токен наружу не отдаётся никогда.
	mux.HandleFunc("GET /api/offers/bot/config", s.requirePerm("admin")(s.handleGetBotConfig))
	mux.HandleFunc("POST /api/offers/bot/config", s.requirePerm("admin")(s.handleSetBotConfig))
	mux.HandleFunc("DELETE /api/offers/bot/config", s.requirePerm("admin")(s.handleDeleteBotConfig))
	mux.HandleFunc("GET /api/offers/bot/status", s.requirePerm("admin")(s.handleBotStatus))
	mux.HandleFunc("GET /api/offers/bot/token", s.requireServiceSecret(s.handleBotToken))
	mux.HandleFunc("POST /api/offers/bot/heartbeat", s.requireServiceSecret(s.handleBotHeartbeat))

	mux.HandleFunc("POST /api/offers/bot/request-code", s.requireServiceSecret(s.handleBotRequestCode))
	mux.HandleFunc("POST /api/offers/bot/verify-code", s.requireServiceSecret(s.handleBotVerifyCode))
	mux.HandleFunc("GET /api/offers/bot/identity", s.requireServiceSecret(s.handleBotIdentity))
	mux.HandleFunc("GET /api/offers/bot/catalog", s.requireServiceSecret(s.handleBotCatalog))
	mux.HandleFunc("GET /api/offers/bot/offers", s.requireServiceSecret(s.handleBotListOffers))
	mux.HandleFunc("POST /api/offers/bot/offers", s.requireServiceSecret(s.handleBotCreateOffer))
	mux.HandleFunc("PATCH /api/offers/bot/offers/{id}", s.requireServiceSecret(s.handleBotPatchOffer))
	mux.HandleFunc("POST /api/offers/bot/offers/{id}/docx", s.requireServiceSecret(s.handleBotDocx))
	mux.HandleFunc("POST /api/offers/bot/offers/{id}/link", s.requireServiceSecret(s.handleBotOfferLink))
	// Бриф-диалог (2026-10-29): вопросы менеджеру и текст секций пишет
	// llm-service. Ручки под service-token, чужие предложения не отдаются.
	mux.HandleFunc("POST /api/offers/bot/offers/{id}/brief/start", s.requireServiceSecret(s.handleBotBriefStart))
	mux.HandleFunc("GET /api/offers/bot/offers/{id}/brief", s.requireServiceSecret(s.handleBotBriefGet))
	mux.HandleFunc("POST /api/offers/bot/offers/{id}/brief/step", s.requireServiceSecret(s.handleBotBriefStep))
	// Текст секций (2026-10-29): написать, поправить, принять.
	mux.HandleFunc("POST /api/offers/bot/offers/{id}/copy/{key}", s.requireServiceSecret(s.handleBotCopySection))
	// Файлы в чат (2026-10-29): .md и .xlsx. attach разбирает и показывает
	// «что понял», attach/apply пишет только подтверждённое.
	mux.HandleFunc("POST /api/offers/bot/offers/{id}/attach", s.requireServiceSecret(s.handleBotAttach))
	mux.HandleFunc("POST /api/offers/bot/offers/{id}/attach/apply", s.requireServiceSecret(s.handleBotAttachApply))

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
		// Токен Telegram-бота — ОДИН хозяин (2026-10-29): до этой правки он
		// приходил из TELEGRAM_BOT_TOKEN окружения, и в соседнем portal-embed
		// именно второй источник полтора суток слал чужим ботом. Пустая строка —
		// «токен не задан», отдельного признака не нужно.
		`CREATE TABLE IF NOT EXISTS offers_bot_config (
			id              INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
			bot_token       TEXT NOT NULL DEFAULT '',
			bot_username    TEXT NOT NULL DEFAULT '',
			updated_by      TEXT NOT NULL DEFAULT '',
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
			bot_last_seen_at TIMESTAMPTZ,
			bot_last_error  TEXT NOT NULL DEFAULT ''
		)`,
		// Бриф КП (2026-10-29): что бот выяснил в разговоре с менеджером.
		// Одна строка на предложение — UNIQUE(offer_id); «Начать КП» можно
		// нажать дважды, и это не должно обнулять набранное.
		`CREATE TABLE IF NOT EXISTS offers_brief (
			id         BIGSERIAL PRIMARY KEY,
			offer_id   BIGINT NOT NULL UNIQUE REFERENCES offers_offer(id) ON DELETE CASCADE,
			payload    JSONB NOT NULL DEFAULT '{}',
			status     TEXT NOT NULL DEFAULT 'open',
			chat_id    TEXT NOT NULL DEFAULT '',
			last_topic TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS offers_brief_offer_idx ON offers_brief (offer_id)`,
		// Журнал реплик: после перезапуска бота (или по жалобе «мне не то
		// ответили») видно, что именно было сказано.
		`CREATE TABLE IF NOT EXISTS offers_brief_turn (
			id         BIGSERIAL PRIMARY KEY,
			brief_id   BIGINT NOT NULL REFERENCES offers_brief(id) ON DELETE CASCADE,
			role       TEXT NOT NULL,
			text       TEXT NOT NULL,
			topic      TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS offers_brief_turn_brief_idx ON offers_brief_turn (brief_id, created_at)`,
		// Контекст продукта для скилов: короткий набор разделов + журнал
		// выводов. Одна строка на режим формы.
		`CREATE TABLE IF NOT EXISTS offers_context (
			id         BIGSERIAL PRIMARY KEY,
			mode       TEXT NOT NULL UNIQUE DEFAULT 'client',
			sections   JSONB NOT NULL DEFAULT '{}',
			learnings  JSONB NOT NULL DEFAULT '[]',
			updated_by TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		// Фото системы в карточке каталога (2026-10-29): одно на карточку,
		// ключ S3. Пустая строка — «фото нет», отдельного признака не нужно.
		`ALTER TABLE offers_catalog ADD COLUMN IF NOT EXISTS photo_key TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE offers_offer ADD COLUMN IF NOT EXISTS brief_id BIGINT NULL`,

		// Материалы отдельной сущностью (2026-10-30). Почему не kind в
		// offers_catalog: у материала есть ЕКН, авто-SKU и вилка цен, у системы
		// нет ничего этого, а её правда живёт в Базе знаний. Одна таблица с kind
		// заставляла бы держать половину колонок пустыми и допускала бы систему
		// с ценой.
		//
		// Цены НЕТ намеренно: она приходит из таблицы менеджера для конкретного
		// КП. price_min/price_max — вилка из загруженных таблиц, для аналитики.
		`CREATE TABLE IF NOT EXISTS offers_material (
			id            BIGSERIAL PRIMARY KEY,
			sku           TEXT NOT NULL,
			kind          TEXT NOT NULL DEFAULT 'material',
			name          TEXT NOT NULL,
			unit          TEXT NOT NULL DEFAULT 'шт',
			description   TEXT NOT NULL DEFAULT '',
			price_min     NUMERIC(14,2) NULL,
			price_max     NUMERIC(14,2) NULL,
			price_seen_at TIMESTAMPTZ NULL,
			photo_key     TEXT NOT NULL DEFAULT '',
			sort_order    INTEGER NOT NULL DEFAULT 0,
			archived      BOOLEAN NOT NULL DEFAULT false,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS offers_material_sku ON offers_material (sku)`,

		// Вариации материала: один SKU — много ЕКН (толщина, цвет, размер).
		// Уникальность по паре, а не по одному ЕКН: один и тот же материал
		// может встретиться в двух таблицах, и это не дубль.
		`CREATE TABLE IF NOT EXISTS offers_material_ekn (
			id            BIGSERIAL PRIMARY KEY,
			material_id   BIGINT NOT NULL REFERENCES offers_material(id) ON DELETE CASCADE,
			ekn           TEXT NOT NULL,
			variant       TEXT NOT NULL DEFAULT '',
			source        TEXT NOT NULL DEFAULT 'excel',
			first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (material_id, ekn)
		)`,
		`CREATE INDEX IF NOT EXISTS offers_material_ekn_ekn ON offers_material_ekn (ekn)`,

		// Перенос материалов из старого каталога. Идемпотентно: переносим только
		// те карточки, которых ещё нет в новой таблице (по name+unit).
		// Системы из каталога НЕ трогаем — они уже не используются, но на них
		// могут ссылаться старые предложения.
		`INSERT INTO offers_material (sku, kind, name, unit, description, sort_order)
		 SELECT 'MAT-ПРОЧ-ОБЩ-' || lpad(nextval('offers_material_id_seq')::text, 4, '0'),
		        'material', c.name, c.unit, c.description, c.sort_order
		 FROM offers_catalog c
		 WHERE c.kind = 'material'
		   AND NOT EXISTS (
		     SELECT 1 FROM offers_material m WHERE m.name = c.name AND m.unit = c.unit
		   )`,
		// Системы из старого каталога архивируются: у них больше нет места в
		// списке материалов, но строка нужна старым предложениям.
		`UPDATE offers_catalog SET archived = true WHERE kind = 'system' AND NOT archived`,
	}
	for _, stmt := range stmts {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}
