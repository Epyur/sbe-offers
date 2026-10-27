# AGENTS.md — sbe-offers (LogicTEAM.КП)

Коммерческие предложения: каталог систем и материалов, конфигуратор формы КП,
сборка предложений в плагине и через **собственного Telegram-бота**, экспорт в
DOCX. Линейные менеджеры работают в боте, администратор настраивает каталог и
форму в плагине.

**Бэк — в этой же папке**: `offers-service/` (Go, API) и `offers-bot/` (Node,
Telegram long polling) — на ветке `backend` (main — чистый релизный срез кода
плагина, без бэка; см. правило «Бэки в папках плагинов» в корневом
`plugins/AGENTS.md`).

Дизайн — `docs/superpowers/specs/2026-10-26-sbe-offers-design.md`,
план — `docs/superpowers/plans/2026-10-26-sbe-offers.md`.

## Назначение (текущее)

- **Каталог** (`offers_catalog`) — карточки систем/материалов/работ: наименование,
  тип, единица, базовая цена, описание. Ведёт только admin; архивируются, а не
  удаляются (на них ссылаются старые предложения).
- **Форма КП** (`offers_form`) — плейсхолдеры + блоки документа (абзац,
  заголовок, список, таблица) + обязательные поля. Редактор — свой блок-редактор
  по образцу конфигуратора протоколов ЛИМС.
- **КП** (`offers_offer`) — снимок: позиции с ценой на момент добавления,
  значения формы, итог, статус `draft`/`final`, номер `{NNN}/{yyyy}` сквозной по
  году. Видимость: admin видит все, остальные — свои.
- **Telegram-бот** — вход по e-mail и коду (самозапрос на почту либо код от
  администратора), сборка КП из каталога и свободными строками, DOCX файлом в
  чат или ссылкой по кнопке.
- **Точка входа** — магазин: «Установленные → Открыть» (`publishService('sbe-offers', {open})`).

## Архитектура

```
sbe-offers (плагин)  ──JWT app_id=offers──►  offers-service (Go)  ──►  Postgres offers
                                                    │                    └─► S3 sbe-offers (DOCX)
offers-bot (Node)  ──X-Service-Secret──►  offers-service
        └── long polling через SOCKS5-сайдкар offers-nl-proxy (nl_egress_proxy)
```

- Личность — общий `auth-service`: email; роли `viewer` < `editor` < `admin` —
  из центральной таблицы прав (см. `authperms.go`).
- **Коды входа, привязки chat_id и роли бота живут в `offers-service`**
  (`bot_codes`, `offers_bot_link`, `offers_access`) — auth-service не правится.
- Telegram с VDS в РФ недоступен напрямую (диапазоны AS62041 заблокированы
  хостером) — поллер ходит через SOCKS5, как `portal-embed`.

## Структура плагина

| Файл | Что это |
|---|---|
| `src/main.ts` | `SbeOffersPlugin`: настройки, view, publishService, новость в ЦУП |
| `src/constants.ts` | `OFFERS_VIEW_TYPE`, `OFFERS_APP_ID`, `OFFERS_PLUGIN_ID` |
| `src/types/offers.ts` | `CatalogItem`, `OfferForm`, `Offer`, `OfferItem`, `AccessEntry`, `Role` |
| `src/types/settings.ts` | `OffersSettings { apiUrl, lastAnnouncedVersion }` |
| `src/services/offers.service.ts` | HTTP к `/api/offers/*`, `OffersError`, `exportDocx` |
| `src/ui/offers-view.ts` | фасад: топбар + сайдбар + контент |
| `src/ui/offers-list.ts` | «🧾 КП»: список, карточка, редактор состава, DOCX, ссылка |
| `src/ui/catalog-editor.ts` | «🗂 Каталог»: таблица, модалка карточки |
| `src/ui/form-editor.ts` | «⚙️ Форма КП»: поля, блоки, обязательные |
| `src/ui/block-editor.ts` | блок-редактор с чипами `{{поле}}` |
| `src/ui/access-editor.ts` | «👤 Бот и доступ»: коды, роли вошедших |
| `src/ui/help.ts` | текст справки (вшит в плагин) |
| `src/styles.css` | классы `tn-offers-*` |

## Структура сервера

`offers-service/`: `main.go` (роуты, миграции), `jwt.go`, `register.go`,
`authperms.go` (копия из других сервисов — править сразу во всех),
`permissions.go`, `catalog.go`, `form.go`, `offers.go`, `docx.go`,
`access.go`, `botapi.go`, `s3.go`, `email.go`, `static.go`, `template.go`,
`static/openapi.yaml`.

`offers-bot/`: `src/main.ts`, `src/poller.ts` (транспорт и маршрутизация),
`src/telegram.ts` (Bot API через SOCKS), `src/session.ts` (машина состояний
входа), `src/flow.ts` (сборка КП), `src/api.ts`, `Dockerfile`.

## Настройки (data.json)

`apiUrl` (default `https://epyur.fvds.ru`), `lastAnnouncedVersion`.

## Переменные окружения (сервер)

`DATABASE_URL`, `PORT`, `JWT_SECRET`, `OFFERS_APP_ID` (`offers`),
`OFFERS_APP_NAME`, `OFFERS_OWNER_EMAIL`, `OFFERS_SERVICE_SECRET`,
`OFFERS_BOT_SECRET`, `AUTH_SERVICE_URL`, `S3_ENDPOINT`/`S3_ACCESS_KEY`/
`S3_SECRET_KEY`/`S3_BUCKET` (`sbe-offers`), `SMTP_*`.
Бот: `TELEGRAM_BOT_TOKEN` (свой, из @BotFather), `TELEGRAM_SOCKS_PROXY`,
`OFFERS_API_URL`, `OFFERS_BOT_SECRET`, `OFFERS_BOT_RATE_LIMIT`.

## Проверки

```
npx tsc --noEmit && npm run build          # плагин
cd offers-service && gofmt -l . && go vet ./... && go test ./...
cd offers-bot && npm run typecheck && npm run build && node --test "src/flow.test.ts"
node plugins/scripts/check_offers_openapi.mjs
```

## Правила

- `catch(e: unknown)` + `errorMessage()`; `requestUrl()`; `window.setTimeout()`;
  без `any`; UI на русском; классы `tn-offers-*` на семантических токенах.
- Коммиты/пуши — только по явной команде пользователя.
- **«Фиксируй» = поднять версию (+0.0.1 в `manifest.json` и `package.json`),
  обновить документацию, подготовить сообщение коммита и СПРОСИТЬ подтверждение
  commit/push.** Не поднимать версию при коммите чисто бэковых изменений.
- Go — только permissive-лицензии (MIT/BSD/Apache-2.0).
- `.gitattributes` с `-text` на `manifest.json`/`main.js`/`styles.css` — есть.
- Правка данных в БД обязана двигать `updated_at`.

## История работ

### 2026-10-26 — v0.1.0 (создание)

Полный цикл: brainstorming → дизайн-спека → план → реализация. Решения
пользователя, зафиксированные в спеке: отдельный плагин со своим сервером и
своим ботом; форма КП конфигурируется администратором по образцу ЛИМС; карточки
систем и материалов заводит администратор; вход в бота по e-mail и коду
(самозапрос открыт всем, код также может выдать администратор); экспорт
**только DOCX** (PDF не делаем); бот отдаёт файл в чат, ссылкой — по кнопке;
согласования нет, `draft` → `final`.

- **`offers-service`**: каталог, форма КП, КП со снимком цен и нумерацией
  `{NNN}/{yyyy}` (advisory-lock на год), DOCX на корпоративном бланке с
  подстановкой плейсхолдеров, коды входа и роли бота, bot API под
  `X-Service-Secret`, Swagger с первого коммита.
- **`offers-bot`**: long polling через SOCKS5, вход по e-mail+коду, сборка КП
  (каталог, свободные строки «Название | кол-во | цена», «Мои КП»), DOCX файлом
  и ссылкой.
- **Плагин**: фасад из пяти разделов, свой блок-редактор, модалки каталога,
  справка, новость в ЦУП.
- **Инфраструктура**: `sbe-core/docker` — `offers-db`, `offers`, `offers-bot`,
  сайдкар `offers-nl-proxy`, Caddy `/api/offers/*` до общего `/api/*`, seed
  приложения `offers` в `auth-service/seed.go`.
- **Проверки**: `go test` — 24 теста (сумма и округление копеек, номер,
  валидация формы и КП, DOCX с экранированием и защитой от пустого КП, коды и
  их истечение, роли); бот — 10 тестов (машина состояний входа, сборка и итог
  черновика, пустой каталог); `npx tsc --noEmit` и `npm run build` — чисто;
  `check_offers_openapi.mjs` — 31 маршрут, схема сходится.

## Статистика ошибок и отступлений

- Нарушений правил нет: 0 `any`, 0 `fetch` в плагине, 0 инлайн-стилей,
  `window.setTimeout()` корректен, все `catch(e: unknown)` + `errorMessage()`.
- `npx tsc --noEmit` EXIT=0, `npm run build` OK.
- Найдено при прогоне тестов и исправлено: `roundMoney` терял копейку на
  `1.005` (множитель 1000 вместо 100); `headingXML` отдавал `Heading1` вместо
  `Heading2` для level 3; `validateCatalogItem` отвергал карточку без явного
  типа, хотя пустой тип по умолчанию `system`.
