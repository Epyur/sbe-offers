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

	// Mini App (2026-11-13, S1): обмен подписанного Telegram initData на
	// обычный JWT приложения и публичная информация о нём. Гейт у /app/session
	// — сама подпись initData токеном бота; JWT у этих двух ручек нет
	// намеренно, иначе вход по initData был бы невозможен.
	mux.HandleFunc("POST /api/offers/app/session", s.handleAppSession)
	mux.HandleFunc("GET /api/offers/app/config", s.handleAppConfig)

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
	// Поиск для окна выбора (2026-11-12): дропдауны на сотни строк заменены
	// поиском Базы знаний — тем же, что у конкурентов.
	mux.HandleFunc("GET /api/offers/materials/search", s.requirePerm("viewer")(s.handleSearchMaterials))
	mux.HandleFunc("GET /api/offers/systems/search", s.requirePerm("viewer")(s.handleSearchSystems))
	mux.HandleFunc("GET /api/offers/suggest", s.requirePerm("viewer")(s.handleSearchSuggestions))
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
	// Список — из кеша (2026-11-01): база знаний перечитывается отдельным
	// действием, а не на каждый показ раздела. Перечитывает администратор.
	mux.HandleFunc("GET /api/offers/systems", s.requirePerm("viewer")(s.handleListSystems))
	mux.HandleFunc("POST /api/offers/systems/refresh", s.requirePerm("admin")(s.handleRefreshSystems))
	// «Добавить систему» в редакторе: состав собирает сервер, потому что цену
	// позиции знает только он (вилка из таблиц менеджера), а не клиент.
	mux.HandleFunc("POST /api/offers/offers/{id}/system", s.requirePerm("editor")(s.handleAddSystem))
	mux.HandleFunc("DELETE /api/offers/offers/{id}/system", s.requirePerm("editor")(s.handleRemoveSystem))
	// Тексты системы (2026-11-06, шаг B): модель пишет их ОДИН РАЗ при выборе
	// системы, а сборка документа только вставляет готовое. Путь «по заметке»
	// позволяет пересчитать текст, когда карточка Базы знаний изменилась.
	mux.HandleFunc("GET /api/offers/offers/{id}/system-texts", s.requirePerm("viewer")(s.handleGetSystemTexts))
	mux.HandleFunc("POST /api/offers/offers/{id}/system/{noteId}/recalculate", s.requirePerm("editor")(s.handleRecalculateSystemTexts))
	mux.HandleFunc("PUT /api/offers/form", s.requirePerm("admin")(s.handleSaveForm))
	// Макет документа (2026-11-05): загрузка .docx с метками `[…]` и `{…}`,
	// чтение обратно для правки и удаление (печать снова по блокам формы).
	mux.HandleFunc("POST /api/offers/form/layout", s.requirePerm("admin")(s.handleUploadLayout))
	mux.HandleFunc("GET /api/offers/form/layout", s.requirePerm("admin")(s.handleDownloadLayout))
	// Образец макета со всеми метками (2026-11-06, шаг E): администратор
	// начинает с рабочего документа, а не со слов инструкции.
	mux.HandleFunc("GET /api/offers/form/layout/example", s.requirePerm("admin")(s.handleExampleLayout))
	mux.HandleFunc("DELETE /api/offers/form/layout", s.requirePerm("admin")(s.handleDeleteLayout))
	// «Опиши блок словами» (2026-11-05): свободное описание → готовые узлы
	// формы. Администратор пишет, ЧТО должно быть в блоке, а не как его
	// разложить по узлам; правку он всё равно делает руками.
	mux.HandleFunc("POST /api/offers/form/describe", s.requirePerm("admin")(s.handleDescribeBlock))

	// Боевой листок (2026-11-08): быстрая справка-сравнение нашего продукта с
	// продуктами конкурентов. Та же схема прав, что у КП: смотрят все, кто видит
	// приложение (своё — владельцу, всё — admin); собирают и правят — editor.
	mux.HandleFunc("GET /api/offers/combat", s.requirePerm("viewer")(s.handleListCombat))
	mux.HandleFunc("GET /api/offers/combat/{id}", s.requirePerm("viewer")(s.handleGetCombat))
	mux.HandleFunc("POST /api/offers/combat", s.requirePerm("editor")(s.handleSaveCombat))
	mux.HandleFunc("PUT /api/offers/combat/{id}", s.requirePerm("editor")(s.handleSaveCombat))
	mux.HandleFunc("DELETE /api/offers/combat/{id}", s.requirePerm("editor")(s.handleDeleteCombat))
	// Сборка DOCX боевого листка (2026-11-08): печать по форме combat, карточки
	// сторон — из кешей систем и конкурентов. Смотреть документ может любой, кто
	// видит листок, поэтому роль viewer.
	mux.HandleFunc("POST /api/offers/combat/{id}/export", s.requirePerm("viewer")(s.handleExportCombat))
	// Продукты конкурентов (2026-11-08): карточки из папки Базы знаний
	// «материалы и системы конкурентов». Список — из кеша; перечитывает admin.
	mux.HandleFunc("GET /api/offers/competitors", s.requirePerm("viewer")(s.handleListCompetitors))
	mux.HandleFunc("POST /api/offers/competitors/refresh", s.requirePerm("admin")(s.handleRefreshCompetitors))
	mux.HandleFunc("GET /api/offers/competitors/search", s.requirePerm("viewer")(s.handleSearchCompetitors))
	// Боевой листок в приложении (2026-11-13, шаг S6): стороны правятся
	// ПОЗИЦИОННО, а не полным сохранением листка, — приложение добавляет и
	// убирает по одному продукту. Читают и печатают все вошедшие (правило
	// 2026-11-12), правят автор и admin (`combatSheetForEditor`).
	mux.HandleFunc("GET /api/offers/combat/our", s.requirePerm("viewer")(s.handleCombatOurList))
	mux.HandleFunc("GET /api/offers/combat/competitors-src", s.requirePerm("viewer")(s.handleCombatCompetitorsSrc))
	mux.HandleFunc("GET /api/offers/combat/analogues", s.requirePerm("viewer")(s.handleCombatAnaloguesJWT))
	mux.HandleFunc("POST /api/offers/combat/{id}/our", s.requirePerm("viewer")(func(w http.ResponseWriter, r *http.Request) {
		s.handleCombatOurSide(w, r, true)
	}))
	mux.HandleFunc("DELETE /api/offers/combat/{id}/our", s.requirePerm("viewer")(func(w http.ResponseWriter, r *http.Request) {
		s.handleCombatOurSide(w, r, false)
	}))
	mux.HandleFunc("POST /api/offers/combat/{id}/competitor", s.requirePerm("viewer")(func(w http.ResponseWriter, r *http.Request) {
		s.handleCombatCompetitorSide(w, r, true)
	}))
	mux.HandleFunc("DELETE /api/offers/combat/{id}/competitor", s.requirePerm("viewer")(func(w http.ResponseWriter, r *http.Request) {
		s.handleCombatCompetitorSide(w, r, false)
	}))
	mux.HandleFunc("PUT /api/offers/combat/{id}/hint", s.requirePerm("viewer")(s.handleCombatHintUpdate))

	mux.HandleFunc("GET /api/offers/offers", s.requirePerm("viewer")(s.handleListOffers))
	mux.HandleFunc("GET /api/offers/offers/{id}", s.requirePerm("viewer")(s.handleGetOffer))
	mux.HandleFunc("POST /api/offers/offers", s.requirePerm("editor")(s.handleSaveOffer))
	mux.HandleFunc("PUT /api/offers/offers/{id}", s.requirePerm("editor")(s.handleSaveOffer))
	// Точечные правки карточки в приложении (2026-11-13, шаг S3): заказчик и
	// позиции сохраняются отдельно, каждое поле само двигает `updated_at`.
	// Полное сохранение (`PUT /offers/{id}`) остаётся для плагина и бота —
	// приложению оно не подходит: затирало бы параллельную правку.
	// Тексты разделов (2026-11-13, шаг S4): те же слоты и то же хранилище, что в
	// боте (`form_values["copy.<ключ>"]`), но доступ по JWT — приложение не
	// знает сервисного секрета и знать не должно.
	mux.HandleFunc("GET /api/offers/offers/{id}/sections", s.requirePerm("viewer")(s.handleListSections))
	mux.HandleFunc("POST /api/offers/offers/{id}/sections/{key}", s.requirePerm("editor")(s.handleWriteSection))
	mux.HandleFunc("POST /api/offers/offers/{id}/sections/{key}/edit", s.requirePerm("editor")(s.handleEditSection))
	mux.HandleFunc("PUT /api/offers/offers/{id}/sections/{key}", s.requirePerm("editor")(s.handleAcceptSection))
	mux.HandleFunc("PUT /api/offers/offers/{id}/client", s.requirePerm("editor")(s.handlePatchOfferClient))
	mux.HandleFunc("PUT /api/offers/offers/{id}/items", s.requirePerm("editor")(s.handlePatchOfferItems))
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
	// Системы для команды «система <название>» (2026-10-31): в каталоге бота их
	// нет, но выбрать состав нужно.
	mux.HandleFunc("GET /api/offers/bot/systems", s.requireServiceSecret(s.handleBotSystems))
	mux.HandleFunc("GET /api/offers/bot/suggest", s.requireServiceSecret(s.handleBotSuggest))
	mux.HandleFunc("GET /api/offers/bot/offers", s.requireServiceSecret(s.handleBotListOffers))
	mux.HandleFunc("POST /api/offers/bot/offers", s.requireServiceSecret(s.handleBotCreateOffer))
	mux.HandleFunc("GET /api/offers/bot/offers/{id}", s.requireServiceSecret(s.handleBotGetOffer))
	mux.HandleFunc("PATCH /api/offers/bot/offers/{id}", s.requireServiceSecret(s.handleBotPatchOffer))
	// «система <название>» в боте (2026-10-31): состав из Базы знаний
	// дописывается позициями. Правило состава то же, что у плагина.
	mux.HandleFunc("POST /api/offers/bot/offers/{id}/system", s.requireServiceSecret(s.handleBotAddSystem))
	mux.HandleFunc("DELETE /api/offers/bot/offers/{id}/system", s.requireServiceSecret(s.handleBotRemoveSystem))
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
	// Боевой листок в боте (2026-11-09): списки сторон, создание листка,
	// добавление/удаление стороны, печать DOCX. Сущность та же, что у плагина.
	mux.HandleFunc("GET /api/offers/bot/combat/systems", s.requireServiceSecret(s.handleBotCombatSystems))
	mux.HandleFunc("GET /api/offers/bot/combat/competitors", s.requireServiceSecret(s.handleBotCombatCompetitors))
	mux.HandleFunc("GET /api/offers/bot/combat", s.requireServiceSecret(s.handleBotListCombat))
	mux.HandleFunc("POST /api/offers/bot/combat", s.requireServiceSecret(s.handleBotCreateCombat))
	mux.HandleFunc("POST /api/offers/bot/combat/{id}/our", s.requireServiceSecret(func(w http.ResponseWriter, r *http.Request) {
		s.handleBotCombatSide(w, r, combatSideOur, true)
	}))
	mux.HandleFunc("DELETE /api/offers/bot/combat/{id}/our", s.requireServiceSecret(func(w http.ResponseWriter, r *http.Request) {
		s.handleBotCombatSide(w, r, combatSideOur, false)
	}))
	mux.HandleFunc("POST /api/offers/bot/combat/{id}/competitor", s.requireServiceSecret(func(w http.ResponseWriter, r *http.Request) {
		s.handleBotCombatSide(w, r, combatSideCompetitor, true)
	}))
	mux.HandleFunc("DELETE /api/offers/bot/combat/{id}/competitor", s.requireServiceSecret(func(w http.ResponseWriter, r *http.Request) {
		s.handleBotCombatSide(w, r, combatSideCompetitor, false)
	}))
	mux.HandleFunc("POST /api/offers/bot/combat/{id}/export", s.requireServiceSecret(s.handleBotExportCombat))
	// «Подобрать аналог» (2026-11-11): наш продукт, ближайший к продукту
	// конкурента, по названию и описанию карточек Базы знаний.
	mux.HandleFunc("GET /api/offers/bot/combat/analogues", s.requireServiceSecret(s.handleBotCombatAnalogues))
	// Уточнение для разбора (2026-11-11): единственная настройка боевого листка.
	mux.HandleFunc("PUT /api/offers/bot/combat/{id}/hint", s.requireServiceSecret(s.handleBotCombatHint))

	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		// WriteTimeout снят 2026-11-06 (шаг A плана «генерация без модели»).
		// Было 90 секунд — и длинный ответ модели (до трёх попыток по 180 с)
		// не мог дойти до человека вовсе: сервер обрывал запись, бот получал
		// пустоту и молчал. Предел теперь у каждой ручки свой — см.
		// writeTimeoutFor в gentrace.go.
		IdleTimeout: 120 * time.Second,
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
created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		// Выбранная система (2026-11-03). Отдельной колонкой, а не в
		// form_values: по ней печатаются описание и ссылки, и её же читает
		// раздел «Системы». Пустой JSON — система не выбрана.
		`ALTER TABLE offers_offer ADD COLUMN IF NOT EXISTS system_choice JSONB NOT NULL DEFAULT '{}'`,
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
		// Связь с системой — по ссылке на заметку Базы знаний (2026-10-31):
		// в составе пишут `[[Техноэласт К ЭКП]]`, а не код. Поэтому материал
		// помнит, из какой заметки он заведён. Пусто — материал заведён руками,
		// и ссылка его не найдёт; это законное состояние, не ошибка.
		`ALTER TABLE offers_material ADD COLUMN IF NOT EXISTS note_title TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE offers_material ADD COLUMN IF NOT EXISTS note_id BIGINT NOT NULL DEFAULT 0`,

		// Макет документа (2026-11-05). Пользователь загружает свой .docx с
		// метками `[…]` и `{…}`, и КП печатается по нему: свои стили, шрифты
		// и колонтитулы. Байты лежат в БД, а не в S3: макет один на форму
		// (десятки килобайт), и читается он при каждой сборке документа —
		// лишний поход в хранилище на печати не нужен.
		`ALTER TABLE offers_form ADD COLUMN IF NOT EXISTS layout_docx BYTEA`,
		`ALTER TABLE offers_form ADD COLUMN IF NOT EXISTS layout_name TEXT NOT NULL DEFAULT ''`,

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

		// Тексты системы, написанные моделью ОДИН РАЗ при её выборе
		// (2026-11-06, шаг B плана «генерация без модели»).
		//
		// Почему отдельная таблица, а не form_values: текст привязан к
		// КАРТОЧКЕ (заметке Базы знаний), а не к предложению. Одна и та же
		// система, выбранная в десяти КП, получает один и тот же обоснованный
		// текст, и переписывать его при каждом выборе — это и была причина
		// «система ушла думать» (правка макета обнуляла кэш, модель
		// вызывалась заново, запрос умирал на таймауте).
		//
		// hash — отпечаток содержимого карточки, из которого текст написан.
		// Не совпал — текст устарел, и кнопка «Пересчитать тексты системы»
		// пишет его заново. Сборка документа модель не зовёт вовсе.
		`CREATE TABLE IF NOT EXISTS offers_system_text (
			note_id    BIGINT NOT NULL,
			block      TEXT NOT NULL,
			text       TEXT NOT NULL DEFAULT '',
			hash       TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (note_id, block)
		)`,
		// Поиск «какие тексты есть у этих систем» идёт пачкой по note_id.
		`CREATE INDEX IF NOT EXISTS offers_system_text_note ON offers_system_text (note_id)`,

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

		// Боевой листок (2026-11-08) — быстрая справка-сравнение нашего продукта
		// с продуктами конкурентов. Снапшот, как и КП: номер и набор заметок
		// фиксируются при сохранении. Наш продукт — our_notes, конкурентный —
		// competitor_notes; оба — массивы {noteId,title}. Текст карточек берётся
		// из Базы знаний по noteId (кеш конкурентов), а не копируется сюда.
		`CREATE TABLE IF NOT EXISTS offers_combat_sheet (
			id               BIGSERIAL PRIMARY KEY,
			number           TEXT NOT NULL UNIQUE,
			owner_email      TEXT NOT NULL,
			title            TEXT NOT NULL DEFAULT 'Боевой листок',
			offer_id         BIGINT NULL,
			our_notes        JSONB NOT NULL DEFAULT '[]',
			competitor_notes JSONB NOT NULL DEFAULT '[]',
			form_values      JSONB NOT NULL DEFAULT '{}',
			status           TEXT NOT NULL DEFAULT 'draft',
			created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS offers_combat_sheet_owner ON offers_combat_sheet (lower(owner_email))`,
		// Поиск в списках (2026-11-13, шаг S2). `text_pattern_ops` нужен
		// потому, что база живёт с локалью, отличной от C: только с ним
		// `LIKE 'q%'` по `lower(...)` идёт по индексу. Внутрисловная подстрока
		// («Строй» в «СтройМонтажСервис») индексом не покрыта намеренно — при
		// LIMIT 200 это досмотр в памяти; если строк станет заметно больше,
		// следующий шаг — `pg_trgm`, а не переделка этих индексов.
		`CREATE INDEX IF NOT EXISTS offers_offer_client_name
			ON offers_offer (lower(client->>'name') text_pattern_ops)`,
		`CREATE INDEX IF NOT EXISTS offers_combat_sheet_title
			ON offers_combat_sheet (lower(title) text_pattern_ops)`,
		// Готовый DOCX листка под хэшем сборки (2026-11-12): повторное «⬇ DOCX»
		// не зовёт модель на сравнение. Хэш считает `combatDocxHash` — он и решает,
		// свежий ли файл; не совпал — листок собирается заново.
		`CREATE TABLE IF NOT EXISTS offers_combat_docx (
			sheet_id   BIGINT PRIMARY KEY REFERENCES offers_combat_sheet(id) ON DELETE CASCADE,
			hash       TEXT NOT NULL,
			docx       BYTEA NOT NULL,
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
