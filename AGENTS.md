# AGENTS.md - sbe-offers (LogicTEAM.КП)

Commercial proposals: a catalogue of systems and materials, the offer form
configurator, offer assembly in the plugin and through the **plugin's own
Telegram bot**, DOCX export. Line managers work in the bot; the administrator
sets up the catalogue and the form in the plugin.

**Both back ends live in this folder**: `offers-service/` (Go, API) and
`offers-bot/` (Node, Telegram long polling) - on the `backend` branch
(`main` is the clean release slice of the plugin code without the back ends;
see the "Бэки в папках плагинов" rule in the root `plugins/AGENTS.md`).

Design - `docs/superpowers/specs/2026-10-26-sbe-offers-design.md`,
plan - `docs/superpowers/plans/2026-10-26-sbe-offers.md`.
Materials/systems cycle - `docs/superpowers/specs/2026-10-30-sbe-offers-materials-systems-design.md`
and the matching plan; encoding investigation -
`docs/superpowers/plans/2026-10-31-encoding-breakage-investigation-plan.md`.

## Purpose (current)

The catalogue split into two entities on 2026-10-31 and the sections were
renamed accordingly.

- **Materials** (`offers_material`, `offers_material_ekn`) - what is actually
  sold: its own SKU (assigned once by the server), ЕКН variants, a price
  *range* only. **There is no price column**: the price for a given offer comes
  from the manager's Excel sheet. Materials are created from Knowledge Base
  cards tagged `material` on every read of the list, and they remember the
  source note (`note_title`, `note_id`) - that is what a system's wiki link
  resolves against.
- **Systems** - Knowledge Base cards tagged `commercial_propose` **without**
  `material`. A system has no price of its own; it justifies why this
  combination of materials was chosen. Its composition links to materials by
  wiki reference (`[[Название]]`); a `MAT-…` code is the fallback.
- **Offer form** (`offers_form`) - placeholders + document blocks (paragraph,
  heading, list, table) + required fields. The editor is a block editor built
  after the LIMS protocol configurator.
- **Offer** (`offers_offer`) - a snapshot: positions with the price at the
  moment they were added, form values, total, `draft`/`final` status, number
  `{NNN}/{yyyy}` continuous through the year. Visibility: admin sees all,
  everyone else sees their own.
- **Telegram bot** - sign-in by e-mail and code (self-request by mail or a code
  from the administrator), offer assembly from the catalogue and free-form
  lines, the `система <название>` command expanding a system into positions,
  DOCX as a file in the chat or as a link on a button.
- **Entry point** - the store: "Installed → Open" (`publishService('sbe-offers', {open})`).
- **Legacy** `offers_catalog` remains for old offers only: materials were
  migrated out, the `system` rows are archived, and nothing writes to it.

## Architecture

```
sbe-offers (plugin)  --JWT app_id=offers-->  offers-service (Go)  -->  Postgres offers
                                                    |                    +--> S3 sbe-offers (DOCX)
offers-bot (Node)  --X-Service-Secret-->  offers-service
        +-- long polling through the SOCKS5 sidecar offers-nl-proxy (nl_egress_proxy)
```

- Identity - the shared `auth-service`: e-mail; roles `viewer` < `editor` <
  `admin` come from the central permissions table (see `authperms.go`).
- **Sign-in codes, chat_id links and bot roles live in `offers-service`**
  (`bot_codes`, `offers_bot_link`, `offers_access`) - `auth-service` is not
  touched.
- Telegram is unreachable directly from the VDS in Russia (the hoster blocks
  AS62041 ranges), so the poller goes through SOCKS5 like `portal-embed`.
- Knowledge Base (kb-service) and LLM (llm-service) are read over the internal
  network; both need a URL *and* a secret in the compose block, and the stand
  needs a stub for each (2026-10-30 lesson).

## Plugin structure

| File | What it is |
|---|---|
| `src/main.ts` | `SbeOffersPlugin`: settings, view, publishService, CUP news |
| `src/constants.ts` | `OFFERS_VIEW_TYPE`, `OFFERS_APP_ID`, `OFFERS_PLUGIN_ID` |
| `src/types/offers.ts` | `Material`, `MaterialEkn`, `CompositionItem`, `SystemCard`, `SheetPreview`, `Offer`, ... |
| `src/types/settings.ts` | `OffersSettings { apiUrl, lastAnnouncedVersion }` |
| `src/services/offers.service.ts` | HTTP to `/api/offers/*`, `OffersError`, 401 retry, `exportDocx` |
| `src/ui/offers-view.ts` | facade: top bar + sidebar + content |
| `src/ui/offers-list.ts` | "🧾 КП": list, card, composition editor, "Добавить систему", DOCX, link |
| `src/ui/material-editor.ts` | "🧱 Материалы": list with SKU, card with the ЕКН table |
| `src/ui/systems-editor.ts` | "🧩 Системы": KB cards, resolved composition, problems |
| `src/ui/sheet-upload.ts` | manager's Excel: "what I understood", then apply |
| `src/ui/context-editor.ts` | "📚 Контекст КП": cached system cards, open note |
| `src/ui/brief-editor.ts` + `brief-model.ts` | the brief card inside the offer editor |
| `src/ui/form-editor.ts` | "⚙️ Форма КП": fields, blocks, required |
| `src/ui/block-editor.ts` | block editor with `{{field}}` chips |
| `src/ui/access-editor.ts` | "👤 Бот и доступ": codes, bot token, who signed in |
| `src/ui/help.ts` | help text (baked into the plugin) |
| `src/utils/date.ts`, `money.ts`, `note-path.ts` | shared helpers - one copy each |
| `src/styles.css` | `tn-offers-*` classes (the root `styles.css` is **built**, never edit it) |

## Server structure

`offers-service/`: `main.go` (routes, migrations), `jwt.go`, `register.go`,
`authperms.go` (a copy of the other services - **edit all copies at once**),
`permissions.go`, `catalog.go`, `material.go`, `material_notes.go`,
`matcher.go`, `sheet.go`, `upload.go`, `systemcards.go`, `systems_compose.go`,
`systems_links.go`, `botcatalog.go`, `botapi.go`, `form.go`, `offers.go`,
`brief.go`, `briefstep.go`, `copystep.go`, `attach.go`, `kbclient.go`,
`llmclient.go`, `docx.go`, `docx_media.go`, `s3.go`, `email.go`, `static.go`,
`template.go`, `static/openapi.yaml`.
Sub-packages: `model/` (domain types) and `skills/` (prompts, checklists, card
and attachment parsing).

`offers-bot/`: `src/main.ts`, `src/poller.ts` (transport), `src/routes.ts`
(routing), `src/callbacks.ts` (buttons), `src/telegram.ts` (Bot API via SOCKS),
`src/session.ts` (sign-in state machine), `src/flow/` (offer assembly, brief,
copy, writer), `src/api.ts`, `Dockerfile`.

## Settings (data.json)

`apiUrl` (default `https://epyur.fvds.ru`), `lastAnnouncedVersion`.

## Environment variables (server)

`DATABASE_URL`, `PORT`, `JWT_SECRET`, `OFFERS_APP_ID` (`offers`),
`OFFERS_APP_NAME`, `OFFERS_OWNER_EMAIL`, `OFFERS_SERVICE_SECRET`,
`OFFERS_BOT_SECRET`, `AUTH_SERVICE_URL`, `S3_ENDPOINT`/`S3_ACCESS_KEY`/
`S3_SECRET_KEY`/`S3_BUCKET` (`sbe-offers`), `SMTP_*`,
`KB_SERVICE_URL`, `LLM_SERVICE_URL`, `LLM_SERVICE_SECRET`.
Bot: `TELEGRAM_SOCKS_PROXY`, `OFFERS_API_URL`, `OFFERS_BOT_SECRET`,
`OFFERS_BOT_RATE_LIMIT`, `VERIFY_PORT`.

**There is no bot token variable and there must not be one** (2026-10-29): the
token lives in `offers_bot_config` and is entered from the plugin's "Бот и
доступ" section.

## Checks

```
npx tsc --noEmit && npm run build          # plugin
cd offers-service && gofmt -l . && go vet ./... && go test ./...
cd offers-bot && npm run typecheck && npm run build && node --test "src/**/*.test.ts"
node ../../scripts/check_offers_openapi.mjs
docker run --rm --network sbe-offers_offersnet -v "$PWD/scripts:/app:ro" -w /app \
  node:22-alpine node offers_local_e2e.mjs http://offers-local-app:3000
```

## Rules

- `catch (e: unknown)` + `errorMessage()`; `requestUrl()`; `window.setTimeout()`;
  no `any`; UI in Russian; `tn-offers-*` classes on semantic tokens.
- Commits and pushes only on an explicit user command.
- **"Фиксируй"** = bump the version (+0.0.1 in `manifest.json` and
  `package.json`), update the documentation, prepare a commit message and ASK
  for confirmation of commit and push. Do not bump the version for purely
  back-end changes.
- Go dependencies: permissive licences only (MIT/BSD/Apache-2.0).
- `.gitattributes` with `-text` on `manifest.json`/`main.js`/`styles.css` - present.
- Any data write in the database must move `updated_at`.
- **Documentation records in this file are written in English** (rule
  2026-10-27). Older Russian entries are not translated retroactively.
- Files are written as UTF-8 without a BOM; the `manifest.json` BOM rule is in
  the root `AGENTS.md`.

## Выпуск 0.1.0 — 2026-10-27

**Задеплоено на сервер:** `offers-db`, `offers`, `offers-nl-proxy` подняты,
auth-service пересобран (seed приложения `offers`). **`offers-bot` остановлен:**
нужен токен своего бота из @BotFather в `OFFERS_TELEGRAM_BOT_TOKEN` (в `.env`
пусто). После заполнения — `docker compose up -d offers-bot`.

**E2E (зелёный):** `permissions/me` → `superadmin`; каталог (создание, поиск по
слову); форма отдаётся с базовыми полями; КП № 01/2026 с итогом 2999.97
(3 × 999.99 без хвостов); DOCX 77 КБ с `document.xml`, `header1.xml`,
`footer1.xml` бланка и названием клиента со спецсимволами; пустое КП → 400;
без токена → 401.

**Выложено на самораздачу:** `backend` → `main`, обе ветки запушены,
`https://epyur.fvds.ru/plugins/sbe-offers/*` отдаёт те же три sha256, что
локальная сборка; запись `sbe-offers` есть в реестре на сервере и в
`sbe-apstore-registry`.

**Найдено и исправлено при деплое:** `roleRank` не знал `superadmin` (владелец
приложения получал 403); ненастроенная форма отдавалась пустой; `go.mod`
требовал go 1.25 при образе 1.24.

**Скрипты:** `scripts/offers_db_role.sh` (роль `offers_app`),
`scripts/offers_token.sh` (JWT для E2E), `scripts/offers_e2e.sh`,
`scripts/offers_release_check.sh`.

## 0.1.1 — 2026-10-27 (исправление доступа)

Жалоба пользователя: «не могу заполнять каталог, UI выглядит битым».

**Причина.** Роль в центре называется `superadmin` (глобальный администратор), а
плагин проверял ровно `admin`: у владельца приложения не было ни кнопки
«+ Карточка» в каталоге, ни разделов «Форма КП» и «Бот и доступ». Сервер
пропускал (его `roleRank` считает `superadmin` выше `admin` — починено ещё при
деплое), а вьюха молчала.

**Правка.**
- `isAdminRole()` — административный уровень это `admin` **или** `superadmin`;
  `canEditOffers()` — сборка КП для `editor` и выше. Ими пользуются вьюха,
  каталог и список КП вместо строковых сравнений.
- Каркас вьюхи строится до запроса роли: раньше ошибка «Ключ недействителен»
  терялась — подзаголовка, куда её писать, ещё не было.
- В шапке — «Обновить права»: роль можно перечитать без перезапуска.
- При смене роли недоступный раздел сбрасывается на «КП».
- Раздел «Бот и доступ» больше не мигает пустым контейнером при загрузке;
  `superadmin` показывается в списке отдельным пунктом (не выбирается — его
  выдаёт только ЦУП).

**Выпуск:** 0.1.0 → **0.1.1**, самораздача (три sha256 сверены), запись в
реестре на сервере и в git обновлена, текст новости в ЦУП переписан.
`npx tsc --noEmit` EXIT=0, `npm run build` OK.

**Уроки (стоило поймать раньше):** клиент и сервер должны брать одну и ту же
модель ролей из одного места. Серверную часть (`roleRank`) поймал E2E на
сервере, клиентскую — только живая жалоба: локальные тесты и E2E проверяли
`admin`, а в базе лежал `superadmin`. В `src/types/offers.ts` теперь есть
`ROLE_RANK` с `superadmin` — сверять новые проверки ролей по нему.

## 0.1.2 — 2026-10-27 (проверка на локальном стенде, навык ub-mode)

**Как проверяли.** Подняли систему целиком в локальном Docker:
`docker-compose.local.yml` — Postgres 16 + `offers-service` из своего Dockerfile +
заглушка auth-service (`scripts/auth-stub.mjs`, отдаёт карту ролей). Без заглушки
права проверить нельзя: сервис спрашивает их у auth-service.

**Интеграционный прогон** — `scripts/offers_local_e2e.mjs`: **59 проверок по
HTTP** (health, Swagger, авторизация 401/403/просрочка/чужой app_id, каталог
create/update/archive/search/валидация, форма save/read/валидация, предложения —
расчёт, нумерация, права, DOCX, ручки бота под секретом, коды входа,
идемпотентность). Все зелёные. Отдельно проверены сквозной сценарий бота (вход по
коду → сборка → DOCX → «Мои КП» → правка состава) и содержимое готового
`document.xml`.

**Пять дефектов, которых юнит-тесты не видели:**

1. **Сохранённая форма не доходила до DOCX** (главный). Проверка «форма пустая»
   стояла ДО разбора JSON и сравнивала nil-слайс — срабатывала всегда, поэтому
   администратор настраивал форму, а в файле печатался шаблон по умолчанию.
   Теперь «не настроена» = строки нет в БД; пустые массивы — это настроенная
   пустая форма. Тест `TestResolvedFormKeepsSavedContent`.
2. **Сервис не стартовал без S3** (`log.Fatalf`). Теперь работает без хранилища,
   ссылки на файлы честно отвечают «хранилище не настроено».
3. **Экспорт DOCX зависал до 5 минут** при недоступном S3 (rclone ждал таймаут).
   Экспорт байтами больше не обращается к хранилищу: `renderOffer` отделён от
   `buildOfferDocx`.
4. **Правка несуществующей карточки → 500 «db error»** вместо 404 (`pgx.ErrNoRows`).
5. **Имя файла в чате** было `ООО_Из_Бота` — `sanitizeKey` заменял пробелы
   подчёркиваниями; теперь имя читаемое, чистятся только запрещённые символы.

**Правки по навыку ub-mode (первый заход, до локального прогона):**
`isAdminRole()` (superadmin в проверках прав — та же ошибка, что была в плагине),
`dbError()` вместо 30 копий `log.Printf` + `writeJSON`, `prepareOffer()` как
единая граница входа, `src/utils/money.ts` — одно округление денег вместо трёх
копий, удалён мёртвый `formatPrice`, подключён `renderPlaceholderPicker`.
Исправлен **баг денег в боте**: `1.005` печаталось как «1,00» вместо «1,01».

**Покрытие Go:** 16.9% → 23.2% (45 тестов). Ручек HTTP в покрытии пока нет —
они проверяются интеграционным прогоном, пока не подключён Postgres к `go test`.

**Выпуск:** 0.1.1 → **0.1.2**, main запушен, самораздача (три sha256 сверены),
реестр обновлён, новость в ЦУП переписана под этот выпуск.

**Стенд:** `docker compose -f docker-compose.local.yml up -d --build`, затем
`node scripts/offers_local_e2e.mjs`; в репозиторий compose и скрипты входят (они
нужны следующему, кто будет проверять), dist и бинари — нет.

## Серверный код восстановлен в git — 2026-10-27

**Что было.** При выпуске 0.1.2 правки `offers-service/` делались на ветке `main`,
где эти папки закрыты `.gitignore` (коммит `9ffa671` «main — только код плагина»).
Поэтому `git add -A` их не взял: коммит `ef4e987` содержит только плагин, а Go-правки
жили лишь на диске. Ветку `backend` создали от `main` уже ПОСЛЕ удаления папок из
индекса, так что серверный код не попал ни туда, ни туда — `git ls-tree backend` давал
0 файлов в `offers-service/` и `offers-bot/`. Код уцелел на диске и на сервере,
переключение веток его не затёрло.

**Что сделано (на `backend`).**
1. Из `.gitignore` убраны строки `offers-service/` и `offers-bot/`; исключения
   артефактов остались: `.env*`, `*.exe`, `offers-service/*.test`, `offers-service/cover`,
   `offers-bot/dist/`, `**/rclone.conf`.
2. `offers-service/` (36 файлов Go, Swagger, шаблон DOCX) и `offers-bot/` (8 файлов)
   добавлены и закоммичены на `backend`.
3. На `main` `.gitignore` не тронут: там эти папки закрыты намеренно.
4. Проверка: `git ls-tree -r backend --name-only` показывает файлы обеих папок.

**Проверки перед коммитом (ветка `backend`):** `gofmt -l .` — пусто, `go vet ./...` —
чисто, `go test -count=1 ./...` — **45 тестов, покрытие 22.9%**; `offers-bot`:
`tsc --noEmit`, `npm run build`, `node --test` — 10 тестов; плагин: `npx tsc --noEmit`
EXIT=0, `npm run build` OK; `scripts/check_offers_openapi.mjs` — 31 маршрут, схема
совпадает.

**Правило на будущее:** перед коммитом правок сервиса — проверять, что файлы попали
в индекс (`git status --short`), а не просто лежат на диске. Если правка сервиса
нужна, а ветка `main` — переключаться на `backend` ДО правок, не после.

## 0.1.3 — 2026-10-27 (деплой и самораздача)

**Что вошло.** Плагин не менялся по существу: только версия 0.1.2 → **0.1.3** и
переписанный текст новости в ЦУП. Номер поднят потому, что с прошлого выпуска в
`main` появился новый `main.js` (правки 0.1.2 лежали в `backend` и до `main` не
дошли), а повторять номер нельзя — обновление бы не приехало.

**Деплой сервера.** `docker compose up -d --build offers` пересобрал образ, но
контейнер остался старым (`Created` и `StartedAt` не двигались): Caddy проксирует
на него, а сам он жил с 13:49. Понадобилось `docker compose up -d --force-recreate
--no-deps offers`. **Правило: после `--build` проверять, что у контейнера новый
`docker ps` `CREATED`, и только потом закрывать деплой.**

**Проверки на бою.** health 200, `/docs/` 200, `/openapi.yaml` 200, без токена 401;
`permissions/me` → `superadmin`; каталог (4 карточки), форма (`client`,
«Коммерческое предложение»), список КП (№ 001/2026) читаются; в БД шесть таблиц,
данные на месте.

**Не задеплоено по причине, которую нельзя закрыть кодом.** `offers-bot` остаётся
остановленным: `OFFERS_TELEGRAM_BOT_TOKEN` в `.env` пуст, нужен токен своего бота
из @BotFather. После заполнения — `docker compose up -d offers-bot`.

**Самораздача.** `lims_selfhost.sh sbe-offers` — три sha256 сошлись на сервере и
по https: `manifest e1796cf…`, `main 2836063…`, `styles 3092269…` (styles не
менялся с 0.1.1). `POST`-строка `registry_file_overrides` обновлена; git-копия
`sbe-apstore-registry/registry.json` — коммит `46811dc`.

**`registry.json` на диске сервера — не источник выдачи.** Caddy отдаёт
`/registry.json` не файлом, а проксирует в auth-service, который накладывает
сверху `registry_file_overrides` из БД. Поэтому правка файла в `/opt/mailers/www/`
ни на что не влияет; проверять выдачу — только по https или по таблице.

## 0.1.4 - 2026-10-29 (bot token is entered from the plugin)

User request: "it is not clear how to enter or change the bot token in order to
start the bot". Option A was chosen by the user (own setting in offers-service).

**Cause.** The token came only from `TELEGRAM_BOT_TOKEN` in the bot's environment,
i.e. `OFFERS_TELEGRAM_BOT_TOKEN` in the server `.env`: setting it needed SSH, a
change needed a container restart, and an empty token made the poller return silently
while the "Bot and access" section looked healthy.

**What was done.**
- `offers-service`: table `offers_bot_config` (single row: token, bot username,
  author, `bot_last_seen_at`, `bot_last_error`); `botconfig.go` and six routes;
  the plugin's ones behind `requirePerm("admin")` - `GET/POST/DELETE /bot/config`
  and `GET /bot/status`; internal ones behind `requireServiceSecret` -
  `GET /bot/token` and `POST /bot/heartbeat`. The token is never returned to the
  plugin.
- Saving verifies the token first (`getMe` through the bot) and writes only on
  success, so a typo cannot replace a working token. Bot unreachable -> the request
  fails and nothing is stored.
- `offers-bot`: `resolveBotToken()` reads `GET /api/offers/bot/token`, caches
  for 15 s and **throws** on any failure - no fallback to the environment, the lesson
  from the `portal-embed` incident of the same day. The poller no longer exits
  without a token: it heartbeats the reason and retries every 15 s, so the token
  entered in the plugin takes effect without a restart. New `verify-server.ts`
  serves `POST /internal/verify` on the internal network only.
- Plugin: a "Telegram-бот" card at the top of "Bot and access" - masked field,
  "Проверить и сохранить", "Удалить токен", "Проверить сейчас", a coloured state
  line ("Бот работает: @..." / "Токен не задан" / "Бот не отвечает · ошибка") and
  the @BotFather hint.
- `TELEGRAM_BOT_TOKEN` removed from `docker-compose.yml` and both
  `.env.example` files; `offers` got `OFFERS_BOT_URL`, `offers-bot` got
  `VERIFY_PORT`.

**Checks.** `gofmt`/`go vet` clean, `go test` - 51 tests, coverage 23.1%
(6 new: token normalisation, format check, no token field in the response, verify
via bot: ok / Telegram error / bot down). Bot: typecheck, build, 19 tests (9 new,
including "does not fall back to `TELEGRAM_BOT_TOKEN`"). Plugin: `tsc --noEmit`
EXIT=0, `npm run build` OK. Swagger: 37 routes, schema matches
(`check_offers_openapi.mjs`). Local stand: `bot-stub.mjs` plus a new section 10
in `offers_local_e2e.mjs` - 77 checks green, twice in a row.

**Not done yet.** No commit, no deployment, no self-hosting: the server still runs
the old bot with the token from `.env`. Deploying needs
`docker compose up -d --build offers offers-bot` and then entering the token in the
plugin; the `OFFERS_TELEGRAM_BOT_TOKEN` line should be removed from the server
`.env` (backup first).

**ub-mode checks (2026-10-29).** ``dryer`` finds no duplication in the new code
(the remaining pairs are pre-existing). ``crapper`` on the fresh code flagged four
oversized functions, all split before releasing: the bot card (CC 13 -> pieces of 5
and below), the access editor (10/12 -> 1 and 6), ``verifyTokenViaBot`` (9 -> 5 plus
``askBotToVerify`` 6) and ``startVerifyServer`` (10 -> 3 plus ``handleVerify`` 8).
Recommended thresholds are CRAP < 10 and duplication < 0.8; the remaining high CRAP
values come from zero test coverage on UI and HTTP, the same as everywhere else in
this plugin, and are recorded rather than hidden.

## Roles left the bot, sign-in survives a restart — 2026-10-29 (commit fbf036b)

**User's requirement:** plugin roles and bot roles must not overlap at all; the bot
needs no roles — anyone who gets access to the bot may build offers. Two-level roles
had been matched obscurely, so "Менеджер" in the plugin showed up as "просмотр" in the
bot.

**Done.**
- `offers-bot/src/session.ts`: the `role` field is gone from `ChatSession`;
  `applyLogin(session, email)` takes no role.
- `offers-bot/src/flow.ts`: `AUTH_OK(email)` no longer names a role,
  `NOT_ALLOWED_TO_EDIT` deleted.
- `offers-bot/src/poller.ts`: the gate in `startDraft` removed.
- `offers-bot/src/api.ts`: `BotIdentity { email }` replaces `{ email, role }`, so the
  compiler prevents leaning on a role again.
- `offers-service/access.go`: `bot/identity` and `bot/verify-code` return only `email`;
  `roleForEmail` deleted as dead code.
- Plugin `src/ui/access-editor.ts`: the "Роль" column and the role picker are gone;
  `roleLabel` deleted. **Plugin roles (viewer/editor/admin) stay in the plugin only.**

**Sign-in survives a restart.** The user reported being asked for a code after every
bot restart. The permanent `chat_id → email` link was already in `offers_bot_link` and
never lost; what was lost was the in-memory session. New `restoreSession(email)` in
`session.ts` (pure, tested) and `sessionRestored(chatId)` in the poller: for a chat
absent from memory the bot asks `identity`, so a known chat is immediately signed in.
Used in both the text handler and the callback handler. An unknown chat still gets a
code; an unreachable service does not break sign-in.

**Checks.** TDD: tests written first and seen failing for the right reason
(`roles-removed.test.ts` 3 checks, `restore.test.ts` 3). Bot: 28 tests, `tsc` and
`tsc --noUnusedLocals` clean, build OK. `crapper`: `sessionRestored` CC 5,
`restoreSession` CC 2 — within the threshold; `dryer` finds no duplication in the new
code. Dead import `draftTotal` removed.

**Deployed.** Bot container recreated; a known chat is recognised without a code
(`810181470 → polishchuk@tn.ru`, `8905120569 → miloserdov@tn.ru`), an unknown chat gets
404 from `identity`, which means a code will be requested.

### Debt: dispatcher functions (deliberately not refactored)

`handleCallback` (CC 24) and `handleText` (CC 23) in `offers-bot/src/poller.ts` stay as
they are — the user chose to record them as debt rather than refactor now. Why they
are over the CRAP threshold:

- both are dispatchers mixing three concerns: anti-flood/entry guard, sign-in state
  machine, and a table of handlers;
- sign-in is checked twice (once per function) — the restart fix above had to be
  written in both places;
- `back` and `done` each repeat the "draft not found" branch with slightly different
  text; `pick:` and `link:` carry their own `try/catch`;
- `done` builds a one-element `missing` array instead of `if (!draft.clientName)`;
- nothing is testable: checking "undo removes the last item" requires the whole poller
  with network.

Planned shape when taken on: `handleText` → guard → ensureAuthed → command table;
`handleCallback` → answer → ensureAuthed → prefix table; the ten buttons become small
functions in `callbacks.ts`, texts in `flow.ts`. Order: tests on current behaviour
first, then the move, then `crapper`/`dryer`.

## Brief dialog, skill-driven copy, KB system cards - 2026-10-29 (structural cycle)

User request: from "the bot builds a list of positions" to a **brainstorming
conversation** about the object, then model-written section copy agreed with the
manager, with the four marketing skills adapted to a text document (not slides).
Then the customer card moved into the Knowledge Base instead of a form field.

**Architecture added**

- `offers-service/model/` - domain types (`Offer`, `BriefPayload`,
  `ProductContext`). A separate package because `main` cannot be imported:
  prompts and checklists could not be tested otherwise.
- `offers-service/skills/` - writing rules adapted from
  `coreyhaines31/marketingskills` (MIT): `sales-enablement` 2.4.0,
  `copywriting` 2.3.2, `copy-editing` 2.2.1, `product-marketing` 2.2.0.
  Deviations recorded in `skills/README.md`. Own additions:
  `CheckClaimNumbers` (model may only use numbers that exist in the offer) and
  `[НУЖНО: ...]` instead of invented facts.
- `offers-service/skills/systemcard.go` - parses a KB note into a system card.
- `offers-service/kbclient.go` - KB client; inter-service JWT from
  `auth-service` (`/internal/service-token`, `target_app_id=kb`), same path as
  `lab-service` -> `ekn-service`.
- Bot dispatchers split: `routes.ts` + `callbacks.ts`; `poller.ts` is now
  transport only (debt from the previous cycle closed).

**Three defects the tests found before the rules were written**

1. `\b` in Go does not treat Cyrillic as word characters: `\bне\s+...` silently
   matched nothing. Both AI-tell regexes were rewritten without `\b`.
2. `\u00a0` is not a valid Go regexp escape - panic at package init.
3. JSON answer of a "skipped" topic is indistinguishable from "model forgot the
   field": both leave the value empty. Added explicit `skipped_topics` in the
   brief and `skip` in the model answer.

**DOCX was "broken" - and the template was not the cause.** `standard.docx` in
`sbe-offers` and `sbe-lims` is byte-identical (sha256 `1FE4F368...`).
`buildDocx` was replacing `word/styles.xml` (5 styles instead of 58) and
`word/numbering.xml` (2 lists instead of 12). Page numbers in the footer are
`PAGE`/`SECTIONPAGES` fields inside text boxes and depend on brand styles - they
were pushed out. Fixed the same way as LIMS: both parts taken from the template
untouched, our markup moved to brand styles (`Normal` = `a0`, `heading 1` = `1`,
bullet list = `numId 6`). A second, cheaper-to-fix-now issue: `wp:docPr` ids are
computed across all package parts (header/footer take 1..25) instead of a
constant `1000`; media written as `offerimgN.*` so `image1/image2` of the brand
are not overwritten.

**Customer context lives in the Knowledge Base.** A note tagged
`commercial_propose` with `system_sku`, `## Состав системы`,
`## Описание системы`, `## Свойства и преимущества`. Standard:
`docs/sbe-offers-system-card-standard.md`; template note
`_Шаблон карточки системы.md` in the systems folder. `offers_context` became a
cache (mode `system_cards`), not a place to type text. A KB failure does not
clear the cache: previous cards are returned with `error`.

**Checks.** Go coverage 23.4% -> 38.1%, `go test ./...` green in three packages
(`main`, `model` 71.6%, `skills` 89.8%), plugin `tsc --noEmit` EXIT=0, local
stand E2E 77/77, `check_offers_openapi.mjs` 42 routes and schema match.

**Committed** (branch `backend`, no push): `ed7486b` llm client, `e640205`
brief + skills + copy, `54fb992` DOCX fix + KB cards, `8765790` plugin
"Контекст КП". In `sbe-core`: `78352e8` (LLM wiring), `66263ec` (KB wiring) -
only own lines were staged there, other uncommitted work left untouched.

**Open.** Task 9 (bot states for brief/copy), 10 (plugin "Бриф" section), 11
(form blocks, version 0.2.0, docs). Version not bumped yet: the plugin code
changed, so `0.1.5 -> 0.2.0` belongs to the release task, with the ЦУП news text
rewritten for this release.

## Card text in DOCX, file attachments in the bot - 2026-10-30

Closes the three open items of the previous cycle. Committed (branch `backend`,
no push): `f847c47`, `7e23bf1`, and the attachment round.

**Task 7d - card text and photo reach the document.** `renderCardSections`
builds its image collector with `newTemplateMedia()`, not `&docxMedia{}`: the
bare struct has no used-id map and panics with "assignment to entry in nil map"
on the first picture, because the brand template already occupies ids 1..25 in
its headers and footers. A manager-approved section text still wins over the
card: the card only fills what nobody wrote by hand.

**Task 7e - `skills/attachments.go`: reading files from the chat.** The type is
taken from the signature, never from the name: a zip with `xl/workbook.xml` is
xlsx, one with `word/document.xml` is docx, an OLE header is the old `.xls`
(explicit refusal: "resave as .xlsx"), `%PDF` is pdf, anything without control
bytes is markdown. `.md` is laid out by brief keys - "Ключ: значение" is a
field, the list right under it belongs to that field, everything else becomes
notes; an unknown field keeps its label and lands in notes instead of being
dropped. `.xlsx` reads `sharedStrings.xml` plus `sheetN.xml` (both cell kinds),
looks for a header row within the first 20 rows, and recognises a specification
by the "наименование" + "количество" columns; `сумма ≠ количество × цена`
becomes a warning rather than silent correction. Covered by 10 tests,
`skills` coverage 87%.

**Task 7f - two-step file upload.** `POST .../attach` parses and shows "what I
understood", `POST .../attach/apply` writes only what the manager confirmed.
The split exists because a customer's file is a draft: silently applying its
prices would put stale numbers in the offer. Applying a specification takes
prices from the CATALOG for rows matched by name; the file's price is used only
for positions the catalog does not have. Applying a brief merges - it never
overwrites what the manager said in the chat, lists are joined without
duplicates, and an objection without an answer stays out of the brief. The bot
gets raw bytes with the name in `file_name` (no multipart parsing on either
side), 10 MB cap answers 413, ownership is checked like every other bot route.
Bot side: `attach.ts` shows the preview with confirm buttons, `poller.ts`
downloads the document through the same SOCKS proxy as `getUpdates`.

**Checks.** `gofmt`/`go vet` clean, `go test ./...` green (main 38.6%, model
71.6%, skills 87.1%), bot `tsc --noEmit` + build OK and 44 tests (9 new),
`check_offers_openapi.mjs` 44 routes and schema match. Live HTTP run of the
attachment routes was NOT done: Docker Hub was unreachable (TLS timeout on
`auth.docker.io`), so the local stand could not be rebuilt; the plan's section
11 for `offers_local_e2e.mjs` is still to be written.

**Lesson.** Rewriting a Go file through a PowerShell pipeline
(`Get-Content -Raw | Set-Content -Encoding UTF8`) destroyed `attachments.go`:
the file came back as CP1251 mojibake and had to be rewritten from scratch.
Source files are edited with the editor tools only, never re-encoded by shell
text commands.
## Task 9 closed, Task 10-11 done, version 0.2.0 prepared - 2026-10-30 (evening)

Continued the brief-dialog plan (`docs/superpowers/plans/2026-10-29-sbe-offers-brief-dialog-plan.md`).

**Task 9 (bot brief/copy states) was already written but untested live.** Fixed on the way:
- UTF-8 BOM stripped from `callbacks.ts`, `routes.ts`, `flow/writer.ts` (the project's
  known gotcha; a BOM breaks file reading).
- Duplication removed in `flow/writer.ts`: four copies of `numberedSlots(systemsIn(...))`
  became one `activeSlot()` / `slotsFor()`.
- Dead code deleted in `flow/brief.ts`: `briefTopicQuestion`, `canSkipBrief`, `skipBrief`
  were only ever called from their own tests. Those tests now check real behaviour
  (keyboards, summary) instead.
- The plan named `draft.step` values `brief`/`text`; the implementation keeps the single
  `authed` step and tracks the conversation in session fields (`briefTopic`, `copySlot`,
  `copyEditing`). Fewer states, no parallel enum - deliberately better, recorded here.

**Three live defects found by the local stand, all fixed test-first where practical:**

1. `handleBotBriefStep` read `chat_id` from the query string while the bot sends it in
   the JSON body -> 400 `chat_id is required`. Switched to `botOfferForChatQ` (the copy
   route already used it).
2. **Early reject did not drain the request body.** `POST .../attach` answers "not your
   offer" *before* reading the file; Go then never reads the body, so the client kept
   uploading megabytes into an answered request and hung until timeout - a 1 MB file never
   went through. Added `drainBody()` in a `defer` plus `TestAttachEarlyRejectDrainsBody`
   (failed with "4 194 304 bytes left in the body", now green).
3. **One wrong-typed field blanked the whole brief.** The model returns `"why_us": ""`
   instead of `[]`; strict `json.Unmarshal` failed and `mergeBrief` silently returned the
   previous (empty) payload - the manager saw "the answer was not recorded" although every
   other field had arrived. Added `tolerantBriefPayload`: parse field by field, a string
   where a list is expected becomes a one-item list, garbage in one field no longer takes
   the rest. Tests: `TestStepBriefToleratesWrongTypedField`, `TestTolerantBriefPayload`,
   `TestTolerantBriefPayloadGarbage`.

**Task 10 - brief in the plugin.** `GET`/`PUT /api/offers/offers/{id}/brief` (viewer+ for
read; owner or admin for write, role `editor` alone does not edit someone else's brief).
`brief-editor.ts` renders a card inside the offer editor: read mode with honest
"не заполнено" labels, edit mode with `✏️ Править бриф`, and a collapsible turn history.
Pure rules live in `brief-model.ts` (field order mirrors `skills.BriefTopics`, an objection
counts as handled only with an answer). `emptyBriefPayload()` in `types/offers.ts` keeps
the key set in sync with the server.

**Task 11 - document toggles, version, docs.** `doc.brief_summary` and `doc.section_copy`
in `form_values` control the optional DOCX content, **both off by default** so existing
offers print exactly as before - three existing tests had to set the flag explicitly, and
three new tests pin the off state (`TestDocxSectionCopyOffByDefault`,
`TestDocxBriefSummaryToggle`, `TestFormFlagOnNil`). `renderBriefSummary` prints only
non-empty `brief.*` fields. Version `0.1.5 -> 0.2.0`, ЦУП news text rewritten for this
release (it is baked into `main.js` at build time - the 2026-09-17 lesson).

**Local stand.** New `scripts/llm-stub.mjs` (answers `/internal/llm/complete`; the brief
prompt contains "JSON" so it returns `{payload,...}`, otherwise plain section text;
the marker `STUB_FAIL` in the user prompt forces 502 so degradation is exercised on the
real code path) plus an `llm-stub` service in `docker-compose.local.yml`. E2E sections 13
(brief, copy, degradation, foreign chat) and 14 (plugin brief routes, toggles) added.

**Checks.** Go: `gofmt`/`go vet` clean, `go test ./...` green, coverage `main` 39.2%
(model 71.6%, skills 87.1%). Bot: `tsc --noEmit --noUnusedLocals` clean, 62 tests.
Plugin: `tsc --noEmit` EXIT=0, `npm run build` OK at 0.2.0. OpenAPI: 47 routes, schema
matches. Local stand E2E: **131/131 green, twice in a row**.

**ub-mode.** `crapper` on `offers-bot/src`: every function below CC 10
(`routeText` 8, `handleCopyText` 6, `acceptAndAdvance` 5, `handleBriefText` 6).
`dryer` finds one pair at 0.84 in `api.ts` (`catalog`/`listOffers`) - pre-existing, not
from this cycle; folding it would only hide the API shape. Functions still at CRAP > 10
are UI and HTTP handlers with zero coverage, same as everywhere else in this plugin -
recorded rather than hidden.

**Environment note.** Docker Hub was unreachable (TLS timeout on `auth.docker.io`), so the
service image cannot be built from its Dockerfile. Built the binary in the local
`golang:1.23` image (`GOTOOLCHAIN=auto`, `go.mod` asks for 1.24) and layered it over the
existing `sbe-offers-offers:latest`. Host port forwarding to `18080` wedges after aborted
large uploads; the E2E is therefore run inside the `sbe-offers_offersnet` network
(`offers_local_e2e.mjs http://offers-local-app:3000`).

**Not done.** No commit, no push, no deploy, no self-hosting - waiting for an explicit
command.

### Release 0.2.0 - 2026-10-30

**Committed and pushed.** Commit `f7a056f` on `backend`: Task 9 state machine in
the bot, three live defect fixes (`brief/step` body `chat_id`, `drainBody` on early
reject, tolerant brief payload), Task 10 brief routes + plugin card, Task 11 document
toggles. `main.js` rebuilt at 0.2.0 and included, as the store downloads it from `main`.

**`backend` -> `main`.** Per the release rule the plugin part was carried over to `main`
and the suffix dropped; the server folders (`offers-service/`, `offers-bot/`) stay only
on `backend`, they are `.gitignore`d on `main`. Both branches pushed.

**Deployed.** `docker compose up -d --build offers` then verified the container's
`CREATED` actually moved (the 0.1.3 lesson) - a rebuild alone can leave the old container
in place behind Caddy.

**Self-hosting.** Three SHA-256 computed as the last action, after the branch switch and
the build; `scripts/lims_selfhost.sh` uploaded them and updated
`registry_file_overrides`; the three sums were compared against
`https://epyur.fvds.ru/plugins/sbe-offers/*`. The `sbe-apstore-registry` git copy of
`registry.json` was updated to match.

**Recording rule learned here.** The `AGENTS.md` entry was written before the commit, so
the commit hash had to be filled in afterwards; the file then differs from the commit it
describes. Filling the hash in a second, immediately following commit keeps the record
honest - or write the entry after committing and commit it separately.
## "База знаний не настроена" while it is configured - 2026-10-30

User report: the plugin's «Контекст КП» says «База знаний не настроена», although
the Knowledge Base is set up.

**The KB was fine; the container was not wired to it.** In the server
`docker-compose.yml` the `offers` service block had no `KB_SERVICE_URL`,
`KB_SERVICE_SECRET`, `LLM_SERVICE_URL` or `LLM_SERVICE_SECRET`, although the secrets
are present in `.env` and both `kb` and `llm` live on the same `internal` network.
So `NewKBClient()` got an empty base URL, `main.go` logged «KB_SERVICE_URL не задан»,
`s.kb` stayed nil, `GET /context/cards` answered `kbEnabled: false`, and
`context-editor.ts` showed the «не настроена» text. The same missing `LLM_*` pair
means the brief and section copy degrade on the server as well.

**Why the stand did not catch it (my miss).** The plan's Task 7 step 1 says "KB
config in compose", but I never added KB to `docker-compose.local.yml`; every E2E run
up to now had the Knowledge Base absent, so nothing checked that cards reach the
plugin. The message looked like a normal "not configured" state.

**Fixed in the stand first, then on the server.**
- `scripts/kb-stub.mjs` - serves `tags`, `notes`, `notes/{id}`,
  `notes/{id}/attachments`, `file-link`, requires a Bearer token like the real
  service. Two system cards in the standard markup.
- `scripts/auth-stub.mjs` gained `POST /internal/service-token`: without it the
  inter-service JWT is missing and cards cannot be read even with the URL set.
- `docker-compose.local.yml`: `kb-stub` service plus `KB_SERVICE_URL` /
  `KB_SERVICE_SECRET` on `offers`.
- `offers_local_e2e.mjs` section 15: `kbEnabled === true` with a hint naming both
  variables, then `POST /context/refresh` and the card contents. Verified red without
  KB (exactly 2 failures, both naming the cause) and green with it.

**Two more defects found while writing that section:**
1. It checked `GET /context/cards` before `refresh`, but that route serves the
   **cache**, which is empty on a fresh database - the check was wrong, not the code.
   Order fixed: refresh first, then read.
2. The stub card put the description before the first `##`; the parser reads it under
   `## Описание системы`. The stub was wrong, not the parser - fixed to follow
   `docs/sbe-offers-system-card-standard.md`.

**Result.** Stand E2E 140/140 green. Server `docker-compose.yml` updated (backup
`docker-compose.yml.bak-kb-20261030`), `offers` recreated, `kbEnabled` and the brief
checked live.

**Third defect, found only on the live server: the list field name.** After the wiring
fix `kbEnabled` became `true` but `count` stayed `0`. `kb-service` returns the list in
**`items`**, while `KBClient.Notes` read **`notes`** - and my unit test fed it a
made-up `notes` body, so the test agreed with the bug. `Notes` now reads `items` and
falls back to `notes`; `kb-stub` serves `items` like the real service; two tests pin
both shapes (`TestKBClientNotesReadsItemsField`, `TestKBClientNotesReadsLegacyNotesField`).
Live check after redeploy: 2 cards, both with description and composition.

**Lesson that is worth more than the fix:** a stub I write myself is a claim about
someone else's contract. The card list was "checked" in tests that encoded my guess,
not the service's answer. Contracts with foreign services are pinned by a recorded
real response (see `testdata/` in `skills/`), not by an invented fixture.

**Rule for the next service on central permissions.** A plugin-service that talks to
another service needs BOTH values in its compose block - the URL and the secret - and
the stand needs a stub for that service. An empty URL is not an error: the service
starts, reports "not configured" and looks healthy. Add the stub and an E2E check in
the same commit as the client code, not later.
## Materials and systems: cycle started - 2026-10-30 (evening, release 0.2.1)

User requirement: **two separate entities.** A **system** is a combination of
materials chosen for the best result and has **no price of its own**; it exists
to justify *why this combination*. A **material** is what is sold: its own name
without a `ТН-` prefix, its own SKU, and several variants whose number is **ЕКН**
(one SKU, many ЕКН). Prices never live in the catalogue: the manager uploads an
Excel sheet with ЕКН, names and prices, and that price is the truth for this
offer, because the same material costs differently by delivery terms.

Design: `docs/superpowers/specs/2026-10-30-sbe-offers-materials-systems-design.md`
Plan: `docs/superpowers/plans/2026-10-30-sbe-offers-materials-systems-plan.md`

**Live data checked before designing:** the Knowledge Base has **587** cards
tagged `commercial_propose` and **421** tagged `material` (tag ids `11181`,
`11507`). Materials carry both tags; systems only `commercial_propose`. That is
the discriminator the whole split rests on, so it was measured, not assumed.

### Released in 0.2.1 (three live defects)

1. **401 "ключ недействителен"** while the key worked. The plugin never
   re-requested the token, and the service logged no rejections at all - only
   `listening` - so the complaint could not be investigated after the fact.
   Now: first 401 drops the token cache and retries once; a second 401 says
   «Сеанс входа устарел»; 403 says «Нет прав». Every rejection logs
   `auth reject: {method} {path} {cause}` - path and one of `no-token`,
   `bad-token`, `issuer`, `audience`, `app-id`, `role`, never the token or the
   body. The six copies of the gate became one `authorize` (84.2% covered).
   `sbe-core` gained `forgetToken` (additive), `sbe-apstore` exposes it on the
   bridge.
2. **Only 50 cards of 588.** `kb-service` returns at most **50 notes per
   request at any `size`** (measured: 50/200/500/1000 all give 50) while
   reporting `total`. The client asked for 200, got 50 and treated it as the
   whole list - there was no pagination at all. Now it pages until a short page
   or `total`, capped at `maxKBPages = 40`; hitting the cap logs a line instead
   of staying silent.
3. **"Открыть заметку" said the note was not found.** `kb-service` returns the
   **logical** path inside the Knowledge Base
   (`Инженерная изоляция/Системы/ТН-СТИЛОБАТ ЭКСПЕРТ ТРОТУАР.md`), while the
   file lives elsewhere in the vault (`База службы/Write it/…` - checked on
   disk). The plugin now resolves the file by name with normalisation (case,
   ё/е, quotes, spaces); several equal names means it does not guess. The error
   text names the note instead of the misleading path.
   Check: `scripts/check_note_path.mjs` - 14/14 (there is no test runner in the
   plugin; the check lives as a script, same as `check_*_openapi.mjs`).

### Done in code, not yet released (Task 2 of the cycle)

`offers_material` (automatic SKU `MAT-{TYPE}-{SERIES}-{NNNN}`, price **range**
`price_min`/`price_max`, **no price column**) and `offers_material_ekn` (many
versions per material, unique on `(material_id, ekn)`). The migration moves
`kind='material'` rows from `offers_catalog` and archives the system rows; it is
idempotent (three restarts produced no duplicates and no changed SKUs).

A test caught a real defect here: SKU cut `МЕМБ` down to `МЕМ` - type codes are
now four characters, series three.

### Where the cycle stands

Done: Task 1 (401), Task 2 (schema), Task 3b (both user reports above).
Next, in plan order: Task 3 (material/ЕКН routes), 4 (Excel parsing and
matching), 5 (two-step sheet upload), 6 (systems by tags and SKU links),
7 (plugin UI), 8 (bot), 9 (docs, 0.3.0, deploy).

**Not deployed:** the three 0.2.1 fixes exist only in git. The live server still
runs 0.2.0.
## Остановка цикла 2026-10-31: где стоим

Работа остановлена по просьбе пользователя. Состояние на момент остановки.

### Выпущено в 0.2.1 (три живых дефекта) - развёрнуто 2026-10-31

1. **401 «ключ недействителен»** при рабочем ключе. Плагин не перезапрашивал
   токен, а сервис не писал отказов в журнал вовсе. Теперь: первый 401 сбрасы-
   вает кэш и повторяет запрос один раз; второй говорит «Сеанс входа устарел»;
   403 говорит «Нет прав». Отказы пишутся как
   `auth reject: {method} {path} {cause}` - путь и причина, без токена и тела.
   В `sbe-core` добавлен `forgetToken` (аддитивно), в `sbe-apstore` - проброс
   в мост.
2. **«Контекст КП» видел 50 карточек из 588.** `kb-service` отдаёт не больше
   50 заметок на запрос при любом `size` (замерено: 50/200/500/1000 - всюду
   50), но сообщает `total`. Пагинации не было. Теперь листает до неполной
   страницы или `total`, предел `maxKBPages = 40`.
3. **«Открыть заметку» отвечала «не найдена».** Путь из Базы знаний -
   логический (`Инженерная гидроизоляция/Системы/...`), а файл в вольте лежит
   в `База службы/Write it/...` (проверено на диске). Теперь файл ищется по
   названию с нормализацией; несколько одинаковых названий - не угадываем.
   Проверка: `scripts/check_note_path.mjs`, 14/14.

### Сделано в цикле «две сущности» (Tasks 2-5, 6 частично)

- **Task 2.** `offers_material` (авто-SKU `MAT-{ТИП}-{СЕРИЯ}-{NNNN}`, вилка
  цен, **колонки цены нет**) и `offers_material_ekn` (много ЕКН на материал).
  Миграция переносит материалы из каталога и архивирует системы; идемпотентна
  (проверено тремя перезапусками). Тест поймал: SKU обрезал `МЕМБ` до `МЕМ` -
  тип теперь 4 знака, серия 3.
- **Task 3.** Шесть ручек материалов и ЕКН. SKU назначает только сервер;
  `kind=system` отклоняется; правка не меняет SKU; удаление уводит в архив;
  повторный ЕКН не плодит дублей; вилка расширяется, но не сужается.
- **Task 4.** Разбор таблицы (колонки по заголовку, русские числа «1 500,50»,
  ошибка называет недостающую колонку) и сопоставление: точный ЕКН -> точное
  название -> модель только для остатка и только как предложение
  (`needsConfirm`). Модель спрашивают только про неразобранное (проверено
  счётчиком вызовов); «нет» и мусор материал не выдумывают.
- **Task 5.** Двухшаговая загрузка: превью ничего не пишет и называет
  человеку догадки модели, несопоставленные строки, расхождения сумм и
  нулевые цены. Применение пишет цену **из таблицы**, дописывает ЕКН,
  расширяет вилку.
- **Task 6 (незакончена).** Готовы чистые правила: разделение по тегам
  (`commercial_propose`+`material` = материал, иначе система) и извлечение SKU
  из строки состава. Тест снова поймал кириллическую грабле `\b` - код
  `РАБ-...` не находился. **Не сделано:** ручка `GET /api/offers/systems`,
  разрешение состава против материалов и отчёт о проблемах, UI систем.

### Что осталось по плану

| # | Задача | Состояние |
|---|---|---|
| 6 | Системы: ручка, разрешение состава, проблемы | правила готовы, ручки нет |
| 7 | Плагин: «Материалы», «Системы», загрузка таблицы, «Добавить систему» | не начато |
| 8 | Бот: каталог только материалы, разворот системы | не начато |
| 9 | Документация, 0.3.0, деплой, самораздача | не начато |

### Коммиты цикла (backend)

`402662a` 401 и журнал отказов, `c03ae69` материалы и ЕКН (схема),
`4fc2a69` пагинация карточек, `5d61782` поиск заметки по названию,
`e8b0773` 0.2.1b и запись о старте цикла, `97c6f24` ручки материалов,
`805a428` разбор таблицы и сопоставление, `b7c7500` загрузка двумя шагами,
`2f2d3c8` системы и материалы по тегам.

В соседних репозиториях: `sbe-core` `09a3cb2` (`forgetToken`),
`sbe-apstore` `f74a6dd` (проброс в мост).

### Проверки на момент остановки

`go test` зелёный (покрытие `main` 40.0%, `model` 71.6%, `skills` 84.0%),
`tsc --noEmit` EXIT=0, `npm run build` OK, OpenAPI 55 маршрутов совпадают,
стенд E2E 156/156, `check_note_path.mjs` 14/14.

**Развёрнуто 2026-10-31.** Сервис и бот пересобраны и пересозданы, самораздача выложена и сверена по https. Проверено на бою: context/refresh собрал **588 карточек** (было 50), kbEnabled: true, журнал отказов пишется, таблицы offers_material/offers_material_ekn созданы миграцией, 1 материал перенесён из каталога, 3 системных карточки заархивированы.
### Грабли, стоившие работы в этом цикле

Два места, о которые легко споткнуться снова. Записаны сюда, потому что в
прошлый раз они уже подводили, а знание осталось только в комментарии кода.

**1. `\b` в Go не считает кириллицу словом.** Регулярка `\b(?:MAT|РАБ)-\d{4}\b`
не находит `РАБ-МОНТ-ОБЩ-0001`: граница слова в Go определяется по
ASCII-буквам, и кириллическая `Р` не считается «словом» с левой стороны.
Шаблон молча не срабатывает — ошибки нет, просто пустой результат.

- Поймано дважды: 2026-10-29 в `skills` (выражения «ИИ-штампов» не ловили
  ничего) и 2026-10-31 в `matcher.go` (`extractCompositionSku`).
- **Правило:** в регулярках по русскому тексту `\b` не использовать. Границу
  задавать явно — классом символов, `(?:^|[^А-ЯЁA-Z0-9])` или проверкой
  результата. Тест на кириллицу обязателен: одна строка `РАБ-…` ловит всё.

**2. `crapper` показывает 0% покрытия на Go-функциях с табличными тестами.**
Инструмент определяет вызовы эвристикой по тексту и не узнаёт вызовы внутри
`cases := map[...]` с `t.Run`. На функциях этого цикла он печатал `0.0%` и
CRAP 100–300, хотя реальное покрытие 96–100%.

- **Правило:** покрытие Go смотреть через `go tool cover -func`, а не через
  `crapper`. `crapper` годится только для цикломатической сложности (CC) —
  и то как ориентир: CC у `buildPreview` 17 и `matchRows` 16 при 100%
  покрытии, то есть порог CRAP < 10 здесь не нарушен, а перебор веток
  осмысленный (это чистые диспетчеры правил).
- Команда для проверки покрытия по одной функции:

```bash
go test -coverprofile=/tmp/c.out ./... && go tool cover -func=/tmp/c.out | grep -E 'функция|total:'
```

## Task 6 closed: systems from the Knowledge Base - 2026-10-31 (not committed)

Continues the plan `docs/superpowers/plans/2026-10-30-sbe-offers-materials-systems-plan.md`
after the cycle stop. This finishes Task 6 only; Tasks 7-9 (plugin UI, bot,
release) are untouched.

**What the task was.** The split between the two entities was decided by tags in
`matcher.go` (`splitCardsByTags`, commit `2f2d3c8`), but nothing read that rule:
there was no route, the composition was a list of plain strings with no link to
the material catalogue, and a line pointing at a material that does not exist
disappeared into the same string list with no trace.

**Server.**

- `skills/systemcard.go`: `SystemCard` gains `Links []CompositionLink{Raw, SKU}`,
  filled by `linksFromComposition` in `ParseSystemCard`. `Composition []string`
  stays exactly as it was - it feeds the model prompt and the document, and
  changing its type would ripple into DOCX and the brief. `ExtractSku` is exported
  so the service can derive the code from a card read out of the old cache. The
  regex has no `\b` and accepts four digit numbers; a `РАБ-` code is found (test
  `TestCompositionLinksWorkSku`), which is the third catch of that gotcha.
- `systemcards.go`: `CompositionItem{Raw, SKU, MaterialID, Name, Unit, Ekn, Found}`
  and `resolveComposition(card, materials)`. Every line ends up in the result;
  `Found=false` lines are named in `Problems` with the reason: no code, unknown
  code, or "material removed from the catalogue" (an archived material is found
  in the map but reported separately - "not found" would be a lie). An empty
  composition is one problem, not silently fine.
  `systemCardFromParsed` is now the single place that builds a card from a parsed
  note - `SystemCardsFromKB` and `SystemsFromKB` both use it.
- `SystemsFromKB` reads `commercial_propose`, splits by tag, keeps systems only.
- Route `GET /api/offers/systems` (`viewer`), handler `handleListSystems` answers
  `{systems, kbEnabled, count, error?}`. It reads the **Knowledge Base live**, not
  the card cache: the composition has to resolve against the *current* material
  catalogue, while the cache serves names to the offer context. A KB failure
  answers 200 with an empty list and `error`, as the refresh route does.
- `sbe-offers-service.PromptText` now delegates to `skills.SystemCard.PromptText`.
  The two were byte-identical (`dryer` 0.88-1.00 duplicate), and a prompt format
  kept in two places drifts silently.

**Stand.**

- `scripts/kb-stub.mjs`: a third note with tags `commercial_propose` + `material`
  (so the tag split is checked against the same feed), and the classic-roof card
  now has `- 1 Мембрана ПВХ Эксперт (MAT-МЕМБ-ПВХ-0001)` in its composition.
- `scripts/offers_local_e2e.mjs` section 17: tag split, composition resolved
  against the material created in section 16 (name, id, `found`), a code-less
  line staying visible and named in `problems`, 401 without a token.

**Checks.** `gofmt`/`go vet` clean; `go test ./...` green with `main` 40.7%,
`model` 71.6%, `skills` 84.2%. Coverage of the new code (`go tool cover -func`):
`resolveComposition` 90.6, `SystemsFromKB` 84.2, `systemCardFromParsed` 100,
`splitCardsByTags`/`noteHasTag` 100. `check_offers_openapi.mjs`: 56 routes, 44
paths, schema matches. Local stand E2E **171/171**, run twice. 15 tests added in
`skills/systemcard_links_test.go` and `systems_split_test.go`.

**Not done, by the plan's own boundary.** Plugin UI (Task 7), bot (Task 8),
version/docs/0.3.0 (Task 9). Committed and pushed later the same day as `6b1fc45`
(the entry was written before the commit, so the hash is named here rather than
in the commit it describes - the 2026-10-30 lesson).

**Stand lesson (cost an hour).** The local service could not be rebuilt from its
Dockerfile (same Docker Hub reachability as 2026-10-30), so the binary was built
in a `golang:1.24` image. A CGO-enabled Go binary copied over the running
container's `/mailers-offers` fails with `exec /mailers-offers: no such file or
directory` - the kernel needs `ld-linux-x86-64.so.2` and musl has only
`ld-musl-x86_64.so.1`, so the "file is not found" message is about the
*interpreter*, not the file. Build with `CGO_ENABLED=0` for the stand; the
container image itself is fine because it builds the binary on its own base.
Also: `docker cp` into a container whose binary is running replaces the file but
leaves that container's binary unmappable until it is restarted - restart after
cp, then check `docker ps` says `Up`, not `Exited (255)`.

## Task 7: the plugin caught up with the two entities - 2026-10-31

The plugin still had one section called "Каталог" with every card carrying a
price, while the server had already split materials (with SKU and ЕКН) from
systems (KB cards, no price). This closes that gap; the bot (Task 8) and the
release with 0.3.0 (Task 9) are still open.

**Navigation.** "Каталог" is gone, replaced by two sections. `catalog-editor.ts`
became `material-editor.ts` and `catalog-editor.ts` was deleted. The legacy
`/catalog` routes and the `offers_catalog` table stay on the server untouched:
old offers still point at those rows, and removing the routes would break them
for no gain.

**Materials (`ui/material-editor.ts`).**

- List: name with the SKU underneath in monospace, unit, price range, ЕКН count.
  No price column - a material has no single price, and a column would silently
  become "the" price the way `offers_catalog.base_price` once did.
- Search covers name, SKU and ЕКН numbers together: managers look a material up
  by the number printed on the spec sheet, not by the catalogue name.
- "Показать архив" is a checkbox, off by default; archived rows are dimmed.
- Card: name, kind (material/work), unit, description. SKU is shown and never
  editable - it is an identity, assigned once by the server.
- New material: two extra fields, "Тип" and "Серия", which only feed the SKU
  generator (`MAT-{TYPE}-{SERIES}-{NNNN}`). They are hidden when editing,
  because a rename never changes the code.
- ЕКН table with number, variant, source (excel/manual) and the last seen date;
  add and delete inside the same modal.
- `priceRangeText` prints "цена не загружалась" when both ends are `null`. Zero
  is a legitimate price and a different thing.

**Systems (`ui/systems-editor.ts`).**

- One card per system: title, code, and either "состав распознан" or "есть
  ошибки в составе". Problems are listed as text - a line without a code, a code
  that is not in the catalogue, a deleted material.
- The composition table shows each line's material, unit and ЕКН, so a manager
  can see what the system is made of without opening the note.
- "Открыть заметку" reuses `openNote` from the context editor (exported, not
  copied): the KB path is logical and the file lives elsewhere in the vault; the
  resolution rule lives in `utils/note-path.ts` and must not have a second copy.
- `formatShortDate` moved to `utils/date.ts` for the same reason - it was about to
  become a third copy.

**Sheet upload (`ui/sheet-upload.ts`).**

- Only for a saved offer: the sheet writes positions into a specific offer and
  moves its `updated_at`.
- File input, then "what I understood": counts per matching method, the problems
  the server found, and a table with a checkbox per row.
- **Rows matched by the model are NOT checked by default.** That is the whole
  point of the two-step flow: a guess needs a human click. Exact ЕКН and exact
  name hits are checked.
- Quantity defaults to 1 only when the column is absent (`hasQty`), never
  silently for a file that says 0.
- The file is read before `input.value` is cleared - the live `FileList` gotcha
  from sbe-mailer 2026-09-18.
- `.xlsx` only; `.xls` gets a message asking to resave.

**"Добавить систему" in the offer editor.**

- This needed a server route: `POST /api/offers/offers/{id}/system` (editor).
  The price of a position is known only on the server (the range seen in the
  manager's sheets), so client-side assembly would have to invent numbers.
- The composition is **appended** to the existing items, not a replacement: a
  system is a set the manager wanted to add.
- Price = the *lower* bound of the range. No range means price 0 and the material
  is named in `missingPrice`, which the UI shows in the notice - the manager has
  to load a sheet for it.
- Lines that resolve to no material are returned in `problems` and never reach
  the offer silently.
- The system is looked up in the live KB feed (the same call the "Системы"
  section makes), not in the card cache, so what the manager saw on screen is
  what gets added.
- `FindSystemCard` became a thin wrapper over the new `FindSystem` - one rule for
  comparing codes instead of two.

**Defects found by the stand, not by unit tests.**

1. An empty offer cannot be created (the server refuses offers without
   positions), so the E2E had to start from a one-line "Заглушка" offer. That is
   correct server behaviour, the test was wrong.
2. The E2E sheet named ЕКН `068863`, which section 16 deletes at the end of its
   run. The preview showed `byEkn: 0` and the row fell through to the model. The
   test now uses what the catalogue actually holds at that point (`068864`,
   "Мембрана ПВХ Эксперт 2") - a fixture that drifts from real state is a bug in
   the fixture.
3. `handleAddSystem` came out at CC 13 with zero coverage. The loop was moved to
   `appendComposition` (now 100% covered) and `handleAddSystem` dropped to 8.

**Checks.** `gofmt`/`go vet` clean. `go test ./...` green: `main` 40.6%,
`model` 71.6%, `skills` 84.2%; new code - `appendComposition` 100,
`itemForMaterial` 100, `FindSystem` 100, `handleAddSystem` 0 (HTTP, covered by
the stand like every other route in this plugin). `tsc --noEmit` EXIT=0,
`npm run build` OK. `check_offers_openapi.mjs`: 57 routes, 45 paths, schema
matches. Local stand E2E **189/189** (section 18: sheet preview writes nothing,
apply takes the price from the sheet, system appends rather than replaces,
unknown system 404, no token 401), run twice.

**Not done.** Marked as Task 7 complete in the plan. Bot (Task 8), version 0.3.0
with the release notes, deploy and self-hosting (Task 9) remain; nothing was
deployed or self-hosted for this change.

## Task 8: the bot learned materials and systems - 2026-10-31

Closes the split for the Telegram bot. The plan asks for two things: the bot
catalogue must never contain a system, and a system must expand into positions.
Both are done, and the second one needed a server route the plan did not name.

**Catalogue is materials only.**

- `handleBotCatalog` now reads `offers_material`, not `offers_catalog`. The
  response shape is unchanged on purpose (`id, name, unit, basePrice`) so the
  bot keeps working, and gained two fields: `sku` and `hasPrice`.
- `hasPrice` exists because `basePrice` alone lies: with no price range seen it
  is 0, and a manager reads 0 as "free". The bot prints "цена не загружалась"
  when `hasPrice` is false.
- The rule lives in `botcatalog.go` as pure functions (`materialsForBot`,
  `filterMaterialsForBot`) and is covered by tests: a system never reaches the
  list even if it somehow lands in the table, and the search finds a material by
  name, SKU or ЕКН number.
- Search by ЕКН matters: a manager reads the number off the spec sheet, not the
  catalogue name.

**A defect the first test run caught.** `materialKindAllowed("")` was false, so
a row saved before the `kind` column existed would silently disappear from the
bot catalogue - `validateMaterial` already treated an empty kind as a material,
the filter did not. Fixed in `materialKindAllowed`, so one rule now covers both
the write path and the listing.

**Systems in the bot.**

- `GET /api/offers/bot/systems` (service secret) returns `{sku, title, problems}`
  per system, plus `kbEnabled` and `error` when the KB is down. No card text, no
  images: the chat does not render them.
- `POST /api/offers/bot/offers/{id}/system` appends a system's composition to
  the chat author's offer. It is a separate route from the plugin one because
  the entry is different (service secret plus `chat_id` instead of a user JWT);
  the composition logic itself is shared - `appendComposition`, `SystemsFromKB`,
  `FindSystem`.
- Bot side: `система <название>` (or `/system <название>`). `parseSystemCommand`
  requires the word first, so "Монтаж системы вентиляции | 1 | 500" stays a
  free-form position. `pickSystem` prefers an exact title or code match, then
  prefix/substring on the normalised title (case, ё/е).
- One match adds the system straight away; several show a keyboard
  (`sys:<index>`) and the chosen one is added on the tap.
- `callback_data` is limited to 64 bytes and system codes are Cyrillic, so the
  button carries an index into a per-chat list kept next to the catalogue pages
  (`RouterState.foundSystems`), not the code.
- The reply names what happened: positions added, materials without a price
  range, composition lines with no matching material.

**Checks.** `gofmt`/`go vet` clean, `go test ./...` green (`main` 40.2%,
`model` 71.6%, `skills` 84.2%); `materialsForBot` and `filterMaterialsForBot`
100%. Bot: `tsc --noEmit` clean, build OK, **72 tests** (10 new in
`src/flow/system.test.ts`). Plugin `tsc --noEmit` EXIT=0. OpenAPI: 59 operations,
47 paths, schema matches. Local stand E2E **197/197**, run twice (catalogue has
no systems, `hasPrice` present, `bot/systems` answers, the bot appends a system,
a foreign chat gets 404).

**Not done.** Version 0.3.0, release notes, deploy and self-hosting (Task 9).
Committed and pushed as `534a8ef`.

## Task 9: version 0.3.0 and documentation - 2026-10-31

The plugin part of the release is ready; the deployment part is not started
because it needs a word from the user.

- `manifest.json` / `package.json`: `0.2.1 -> 0.3.0`. The step is the larger one
  (+0.1 to a `0.x.0`) because the catalogue stopped being one thing and became
  two, which is an architectural change, not an increment. Written with
  `UTF8Encoding($false)`, first bytes checked `7B 0A 20` - no BOM.
- ЦУП news text rewritten for this release (the 2026-09-17 lesson: the text is
  baked into `main.js` at build time and is not refreshed on its own). It talks
  about "Каталог разделён на два", the ЕКН, the sheet upload and the bot
  command, in plain Russian and without file or function names.
- `src/main.ts` also corrected: the version check compares
  `lastAnnouncedVersion`, so nothing else was needed there.
- `specification.md` gained two sections - the database tables (materials, ЕКН,
  bot config; `offers_catalog` kept for old offers) and a full description of
  materials, the two-step sheet upload, systems and the bot's two new routes.
- The plan `docs/superpowers/plans/2026-10-30-sbe-offers-materials-systems-plan.md`
  has its steps ticked and a status note at the end naming what was added beyond
  the plan (three server routes) and that Task 10 is untouched.

**Checks.** `npm run build` OK at 0.3.0, `tsc --noEmit` EXIT=0, no `0.2.1`
left in the bundle. Go and bot suites unchanged from Task 8 (`go test` green,
72 bot tests), OpenAPI 59 operations.

**Not done (Task 10 at that moment), needs the user's word.** Deploy
(`docker compose up -d --build offers offers-bot` plus the `CREATED` check),
the live catalogue migration check, `backend` -> `main`, the three SHA-256 as
the last action, self-hosting and the https comparison.

## Release 0.3.0: materials and systems are live - 2026-10-31

Done on the user's explicit command: commit, document, push to `main`, deploy
and self-host.

**`backend` -> `main`.** The plugin part was carried over with
`git checkout backend -- src main.js styles.css manifest.json package.json
specification.md tsconfig.json esbuild.config.mjs AGENTS.md scripts/...`, and
`src/ui/catalog-editor.ts` deleted (it no longer exists on `backend`). The
server folders stayed on `backend`, as the rule requires. `main.js` was then
**rebuilt on `main`** rather than carried over: the three sums are taken from
the branch that is actually shipped, and a bundle built on another branch has
been the source of hash mismatches before (2026-09-12). Commit `ea39753`,
pushed; `backend` was already up to date at `d5679e0`.

That left exactly one difference between the branches outside the server
folders: `.gitignore`, which keeps ignoring `offers-service/` and `offers-bot/`
on `main` and does not on `backend`. That is intended.

**Deploy.** The server folders are plain copies, not a git checkout, so the
working tree was sent with `tar` (excluding `node_modules`, `.git`, binaries,
`target`, `.metrics`) through `scp` to `/tmp/offers-deploy`, unpacked over
`/opt/mailers/offers-service` and `/opt/mailers/offers-bot` after backups
(`*.bak-offers030`; `docker-compose.yml` and `.env` backed up too, untouched).
MD5 of `main.go`, `botapi.go`, `botcatalog.go`, `systems_compose.go` matched
between the vault and the server **before** the rebuild - the 2026-10-29 lesson
about verifying with `md5sum` rather than trusting a "successful" copy.

`docker compose up -d --build offers offers-bot` recreated both containers and
`CREATED` moved (18:29); the log says `База знаний: карточки систем читаются с
http://kb:3000` and `offers-service listening on :3000`.

**Live checks.** health `{"status":"ok"}` inside the container, `/docs/` and
`/openapi.yaml` 200 through Caddy (47 paths), `GET /api/offers/systems` 401
without a token and 200 with a service token, `bot/catalog` answers only
materials with `hasPrice`, `bot/systems` answers with `kbEnabled: true`. The
bot container logs `поллер запущен` and `проверка токена слушает :3000`.

**Self-hosting.** `scripts/lims_selfhost.sh sbe-offers <three sums>`; the server
verified each sum with `sha256sum -c` before writing
`registry_file_overrides`, and all three were compared again against
`https://epyur.fvds.ru/plugins/sbe-offers/*`:

- `manifest 1e49dc66…42d1`
- `main 94d547cc…a729`
- `styles 5b3e3db3…4fb6`

`registry.json` on the server reports the same three hashes and
`selfHosted: true`. The git copy in `sbe-apstore-registry` was updated in the
same release (commit `6c59751`, pushed).

**Live data after the migration (checked in SQL).** `offers_catalog` holds 1
archived `material` and 3 archived `system` rows; `offers_material` holds 1
material, `offers_material_ekn` is empty. So the migration ran, the three
system rows are archived as designed, and the material kept its SKU.

**What the live data says about the next step, and it matters.** Of 1541
system cards, **373 carry a parsed composition, and not one line has a material
code** - the cards are written as `- [[Техноэласт К ЭКП]]` (wiki links) or as
plain descriptions, not `- Мембрана (MAT-МЕМБ-ПВХ-0007)`. Every card therefore
reports "есть ошибки в составе", and "Добавить систему" cannot resolve a
single position. This is not a defect in the code: the feature works, the
notes have not been brought to the standard described in
`docs/sbe-offers-system-card-standard.md`. Until a technologist puts SKU links
into the systems that matter, the systems section is informational only.
Recorded here so the first complaint about it is answered with a fact.

## Styles.css shipped mojibake - 2026-10-31 (same evening)

User report: the "Материалы" tab shows broken characters. **The tab was not the
cause** - the whole plugin's stylesheet carried garbage, and so did every other
SBE plugin built from the same base.

**Root cause.** `sbe-core/src/design/components.css` had one comment (the
section header for the sources module, lines 300-302) written as CP1251 bytes
and read as UTF-8. The esbuild hook concatenates `tokens.css` +
`components.css` + the plugin's own `src/styles.css` into `styles.css` on every
build, so the damage was copied into each plugin's stylesheet. It was there
since the sources module landed (2026-10-06, `6da1de7`) and reached production
with every release since - including 0.3.0 earlier this evening.

**How it was found.** `styles.css` contained **121** `U+FFFD` characters: the
bytes were still CP1251 inside the UTF-8 file, so a straight decode with the
`latin1`/CP1251 table from the git blob recovered the text verbatim -
`Пользовательские источники (sbe-core/src/sources/)`. Restoring the comment in
`components.css` and rebuilding removed every `U+FFFD`.

**A false alarm worth writing down.** The first scan claimed **148 tracked files
on `backend` were UTF-16LE**. That was wrong twice: PowerShell's
`git cat-file -p ... > file` writes UTF-16 by default, and the first bytes then
look like a BOM. The same scan through Node (`execFileSync('git', ['cat-file',
'-p', ...])` gets raw bytes) found **exactly one** damaged file - `styles.css`.
Lesson: the tool that reports a problem must be proven not to create it; here
the reporting shell produced the very pattern it reported.

**Fixed in three places.**

- `sbe-core/src/design/components.css` - the comment restored (commit
  `0914fb6`, branch `backend`).
- `sbe-offers` `styles.css` rebuilt and released: commit `0689361` on `backend`,
  `9865073` on `main`, self-hosted with the new sum `46f4c880…83d0`, checked
  over https (0 `U+FFFD`). `manifest.json` and `main.js` are byte-identical to
  the previous release, so the version stays 0.3.0 - a patch release would have
  meant bumping the number for a stylesheet only, and the store compares hashes,
  not versions, to decide there is something new.
- `sbe-apstore-registry` `registry.json` - the styles hash (commit `26d1c12`).

**Still broken elsewhere, not fixed here.** Other SBE plugins bundle the same
`components.css`, so any build of them before this fix carries the same
mojibake. They need a rebuild to pick it up - the base file is now clean, so the
next build of each plugin fixes it for free. That belongs to those plugins'
cycles, not this one.

## Composition links by wiki-reference, materials from KB cards - 2026-10-31

User decision, after the release showed the systems section could not resolve
anything: **SKU stays as a property of the material card, but the link between
a system and its materials is the wiki-reference.** Measured on live data
before writing code:

- 3387 composition lines, **0** of them carry a `MAT-…`/`РАБ-…` code;
- 2479 wiki links, **2103** of them point at a note carrying the `material` tag.

So the old rule ("resolve by code") could not work by construction, and the
standard document never asked for a code at all - the `- Мембрана
(MAT-МЕМБ-ПВХ-0007)` example was mine, invented in the plan, and it went
unnoticed until the release.

**Resolution order now** (`resolveComposition`): wiki link first, code second.
The link wins when a line has both - a person put the link there by hand.

- `systems_links.go`: `wikiTarget` parses `[[Name]]`, `[[Name|alias]]`,
  `[[Name#anchor]]`; alias and anchor dropped; matching is case-insensitive and
  trimmed, like Obsidian. The regex is non-greedy and has no `\b` (that gotcha
  has bitten three times now).
- A link to a note that is **not** a material (a GOST, a protocol) is not a
  material: it stays unresolved and is named in `problems`. Nothing is guessed.
- `CompositionItem` gained `wiki`, so the UI can show what the line points at
  even when it did not resolve.

**Materials now come from Knowledge Base cards** (`material_notes.go`).

- Notes tagged `material` are turned into materials on every read of the list
  (`syncMaterialsFromNotes` inside `listMaterials`). It is a read-time sync, not
  a migration: no "what if the note was renamed" question - the name follows
  the note.
- The material remembers its origin: `note_title` and `note_id` (two idempotent
  `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`).
- **SKU is assigned once** and never touched by this sync; an existing material
  is never rewritten - a human's edit in the plugin outranks the note's title.
- A note deleted in the KB does **not** archive the material: archiving is a
  person's decision in the plugin, not a side effect of cleaning up notes.

**Duplication removed before it could spread.** `handleAddSystem` and
`handleBotAddSystem` were 85% identical; both now call `assembleSystem`, which
returns a typed `composeError` and lets the caller pick the HTTP status. `dryer`
no longer reports that pair.

**A stub lied, and the stand caught it.** `kb-stub.mjs` answered *every* note for
`tag_id=7` and *nothing* for `tag_id=8`, so on the stand no material was ever
created from a card, while on the server all 1165 tagged notes are there. The
stub now filters by the requested tag, exactly as `kb-service` does. This is the
same class of mistake as 2026-10-30 ("a stub I write myself is a claim about
someone else's contract"), and it is worth repeating: a stub simpler than the
contract it imitates produces green tests and broken features.

**Checks.** `gofmt`/`go vet` clean; `go test ./...` green (`main` 40.7%,
`model` 71.6%, `skills` 84.2%); `resolveComposition` 92.3% covered, the new
pure functions are pinned by 15 tests. OpenAPI: 59 operations, 47 paths, schema
matches. Stand E2E **198/198**, twice. 4 checks in section 17 were rewritten:
they asserted the old code-based contract and had to follow the decision.

**Still to do by the user, not by code.** The materials for the systems that
matter must exist as KB cards with the `material` tag - then the links resolve
by themselves. Nothing can be done in code about a card that has no material
note behind it: it stays in `problems` on purpose.

### Deployed and measured on live data - 2026-10-31 20:13

Deployed with the same discipline as the previous release: files through
`tar`/`scp`, MD5 of `systemcards.go`, `systems_links.go`, `material_notes.go`,
`material.go` compared before the rebuild, backup `offers-service.bak-wikilinks`,
`docker compose up -d --build offers`, `CREATED` moved, log shows
`offers-service listening on :3000`.

**The numbers now, against the same database as the measurement before:**

| | before | after |
|---|---|---|
| materials in the catalogue | 1 | **1165** (all from KB cards, all with `note_title`) |
| composition lines | 3387 | 3387 |
| lines with a wiki link | — | 2103 |
| **lines resolved** | **0** | **2103** |
| unresolved lines | 3387 | 1284 - every one of them has no link at all |

So **every single wiki link resolves**. The remaining 1284 lines are plain
text: "Первый материал или слой", "Асфальтобетон", the template placeholder
rows of unfinished cards. That is exactly what `problems` is for - they are
named to the person instead of being dropped, and no code change can guess what
they mean.

Only **5 cards** have a composition where every line has a link; the rest mix
linked and plain-text lines. This is a data situation, not a defect: a
technologist's work remains, and the section now shows precisely where.

**One more thing the live check proved.** The sync turned 1165 notes into
materials in a single read, with SKUs assigned (`MAT-ПРОЧ-ОБЩ-0002` and up).
Note the type code: the sync passes no type/series hints, so everything lands in
`ПРОЧ`/`ОБЩ`. If type codes in SKUs matter for anything, that is the next
decision to make - the plans had them typed by hand, and the automatic ones are
not meaningful.

## Materials leaked into "Системы", and the "Материалы" tab was mojibake - 2026-10-31 (evening)

Two user reports, two different causes. Both fixed, deployed and self-hosted;
commits `b22b6e1` (backend), `8657756` (main), registry `9de88c2`.

### 1. Materials appeared in the systems section

**Not** a bug in the tag split - that function was right both times. The
knowledge base does not send tags in the note **list**: `GET /api/kb/notes`
returns `id, title, path, updated_at` and **no `tags` key at all** (checked on
the live service). `splitCardsByTags` therefore saw an empty tag list and put
**all 1541** `commercial_propose` cards into systems, including the 1165 that
also carry `material`. The full note (`GET /api/kb/notes/{id}`) does return
`tags`, and that call was already being made for the text.

Second, hidden defect on the same path: `KBNote` declared **its own** `Tags`
field next to the embedded `KBNoteSummary.Tags`. `json.Unmarshal` filled the
outer field, the embedded one stayed empty, so a check reading through the
embedding saw nothing. The extra field is gone - one field, no shadowing.

Now the tag is read from the full note, and the note's title is the fallback for
a card that has no heading in the standard format - otherwise live cards, which
mostly have no H1, would all be called "(без названия)".

**Measured on the live server after deploy:** 1541 -> **376 systems**;
materials 1165; composition lines 3387, resolved 2103; 5 cards with the whole
composition linked. Test `systems_split_tags_test.go` reproduces the original
failure (it feeds a list without tags, exactly like the real service) and fails
on the old code with "в системах оказалась не та карточка".

### 2. The "Материалы" tab showed broken characters

The stylesheet had already been fixed earlier the same evening (the sbe-core
comment). This was a **second, independent break**: the source file
`src/ui/material-editor.ts` itself was committed with mangled Russian text - on
`backend` and on `main`, from its very first commit (`1381215`), which is why
every visible label in that one tab was garbage while the rest of the plugin was
fine.

Recovery by transformation was not possible: CP1251->UTF-8, UTF-16, and double
passes were all tried and none reproduced the original without `U+FFFD`. The
file was rewritten by hand. The check that matters: every Russian label
("Новый материал", "Показать архив", "Вариации (ЕКН)", "из карточки", "Убрать в
архив", "Поиск по названию, SKU и ЕКН") is present in the built `main.js`
(esbuild escapes Cyrillic, so the check looks for the `\uXXXX` form).

**Lesson recorded, not just fixed.** Two encoding incidents in one day is a
pattern. An investigation plan is written:
`docs/superpowers/plans/2026-10-31-encoding-breakage-investigation-plan.md`
(what writes files, why no check catches it before commit, one guard instead of
four, and the recovery decision tree). It lists all four incidents of the last
two weeks and is the place to start, not this file.

### State after this evening

- `backend` = `main` = `b22b6e1` / `8657756`, both pushed.
- Server deployed, MD5 of the changed files compared before the rebuild.
- Self-hosted: `main.js 4a8fdef1...97ca`, `styles 46f4c880...4fb6`,
  `manifest 1e49dc66...42d1`; verified over https; registry updated.
- Version stays **0.3.0**: `manifest.json` did not change, and the store decides
  "there is an update" by hashes.
- `go test` green, `tsc` EXIT=0, build OK, stand E2E 198/198.

**Still open for the user.** The 1284 composition lines that resolve to nothing
have no link at all - plain text in the cards. And the automatic SKUs are all
`MAT-ПРОЧ-ОБЩ-...` because the sync passes no type/series hints.

## 0.4.0 - 2026-11-01: context first in the bot, and a form that composes

Two user requests in one cycle: the bot must not jump from the menu to
positions, and the offer form must be composable from standard, free-text,
generated and image blocks. Design and plan:
`docs/superpowers/specs/2026-11-01-sbe-offers-context-first-and-form-blocks-design.md`,
`docs/superpowers/plans/2026-11-01-sbe-offers-context-first-and-form-blocks-plan.md`.

### 1. The bot starts with context

`startDraft` used to open the catalogue keyboard immediately. Now it creates the
offer **empty** (`handleBotCreateOffer` accepts `items: []` on the bot path only;
the REST path still answers 400) and starts the brief: object, purpose, area,
addressee, deal, planned systems, terms - and positions only after that.

- `model.BriefPayload` gained `object_purpose`, `object_area`,
  `customer_address`, `planned_systems[]`; 10 -> 14 fields.
- `skills.Topic` gained `Stage`; four stages (`object`, `deal`, `offer`,
  `terms`), `Stages()`, `StageOf()`, `CurrentStage()`.
- The offer is created up front because a file attaches to an offer row
  (attachment, brief); the whole `attach` pipeline then works unchanged at any
  point of the conversation.
- `routeDocument`'s "no offer" refusal is gone from the normal path: the row
  always exists.
- New `GET /bot/offers/{id}?chat_id=` so the bot can read the addressee the
  brief already collected instead of asking for the client name again.
- `finishDraft` now PATCHes the existing offer instead of creating a second
  one - otherwise one КП would get two numbers.
- At the context stage free-form "name | qty | price" text is not taken as a
  position: during a conversation it is an answer, not a line item.

### 2. The form composes

- `Block` gained `spacingBefore`/`spacingAfter` (points, 0..200),
  `keepTogether`, `pageBreakBefore`. Absolute positioning was rejected: text
  boxes do not survive a human editing the paragraph.
- New nodes: `page_break`, `links`, `prompt_text`, `image`.
- `image` renders `wp:anchor` + `wp:wrapSquare` (text flows beside the picture,
  the line is not broken) instead of `wp:inline`. This is the user's explicit
  request: "images must not create breaks".
- `links` prints the `## Ссылки` section of the system/material cards, grouped,
  deduplicated, capped at 60 with an honest sentence. In Word they are real
  external hyperlink relationships.
- `prompt_text` asks the model for one paragraph, grounded in the offer facts;
  the result is cached in `form_values` under `gen.section` (hash of the offer
  data) + `gen.<blockId>.<node>` and rewritten when the offer changed. Adding a
  system drops the hash (`dropStaleBlockTexts`). Model down - the block is
  silent, the document still builds.
- Default form: cover, page break, supplier/recipient/object table, generated
  description of the system, links. Saved forms are untouched.

### Defects the stand and the checks found

1. **`crapper`/`dryer` on the new code.** `flagXML` replaced two identical
   "if enabled return markup" pairs; the `StageLabel`/`StageOf` loops stay
   separate on purpose (different slices, and a generic would read worse).
   `KBImageSource.Get` (CC 11) is network glue with no unit coverage, recorded
   not hidden.
2. **E2E section 13 compared brief JSON by string**, so adding fields made it
   fail on key ORDER, not content. Now `sameJson` (order-insensitive).
3. **My own bundle check was wrong twice** - searched lowercase `\uXXXX` and
   then had shell-doubled backslashes. Uppercase hex, single backslash, in
   `scripts/check_bundle_strings.mjs`; it now pins the editor's Russian labels.

### Checks

`gofmt`/`go vet` clean; `go test ./...` green (`main` 44.0%); bot 78 tests,
`tsc --noEmit` clean, build OK; plugin `tsc --noEmit` EXIT=0, `npm run build`
OK at 0.4.0; `check_offers_openapi.mjs` 60 operations, schema matches; stand E2E
**213/213, twice** (new section 19: empty draft, addressee route, brief starts
with the object, card refs, form with new blocks, negative spacing, REST still
400). A real DOCX was built and inspected: page break, anchored image with wrap,
clickable links, no leftover placeholders, all parts well-formed XML.

### Not done

Release steps (commit, push, `backend` -> `main`, deploy, self-hosting) need the
user's explicit command. `specification.md` and the in-app help are updated.
