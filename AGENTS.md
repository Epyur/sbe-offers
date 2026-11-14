# AGENTS.md — sbe-offers (LogicTEAM.КП)

Commercial proposals for the sales department: a catalogue of systems and
materials, the offer-form configurator, offer assembly in the plugin and through
the **plugin's own Telegram bot**, DOCX export. Line managers work in the bot;
the administrator sets up the catalogue and the form in the plugin.

**Both back ends live in this folder**: `offers-service/` (Go API) and
`offers-bot/` (Node, Telegram long polling), on the `backend` branch. `main` is
the clean release slice of the plugin code without the back ends (see the
"Бэки в папках плагинов" rule in the root `plugins/AGENTS.md`).

- Design: `docs/superpowers/specs/2026-10-26-sbe-offers-design.md` + the
  matching plan.
- Materials/systems: `docs/superpowers/specs/2026-10-30-sbe-offers-materials-systems-design.md`.
- Context-first bot and composable form:
  `docs/superpowers/specs/2026-11-01-sbe-offers-context-first-and-form-blocks-design.md`.
- Generation without the model:
  `docs/superpowers/plans/2026-11-06-sbe-offers-generation-without-model-plan.md`.
- Combat sheet (2026-11-08):
  `docs/superpowers/specs/2026-11-08-sbe-offers-combat-sheet-design.md`.

**This file is a reference, not a log.** It holds what the system is *now*:
structure, rules, gotchas. The history of every release (defects, measurements,
hashes, post-mortems) lives in
[../agents_arhiv.md](../agents_arhiv.md) — section «История сессий», entry
«sbe-offers», subsection «Ревизия 2026-11-08». Go there when you need to know
*why* a rule exists or when you meet an old complaint again; work from here.

**Starting cold?** Read [HANDOFF.md](HANDOFF.md) first — one page: versions,
hashes, checks, what is unfinished, where to look when something breaks.

Current version = **0.10.1** — **RELEASED 2026-11-14.** `backend` `b6f2d3b`, `main` `a6d50ca`;
self-hosted `manifest ec3d0eef…` / `main 91ae7171…` / `styles a32ac3df…`,
registry `sbe-apstore-registry` `ef07757`, app `assets/index-wlCTYUrx.js`.

**0.10.1 (2026-11-14): «Технодайджест» и «Скорая помощь» первой.** Главная Mini
App получила четвёртую карточку — **«Технодайджест»**, раздел проектируется
(`DigestScreen`, маршрут `#/digest`), картинка `4.webp` (144×157) вшита скриптом
`embed_sections.mjs` и показывается `object-fit: contain` — `cover` оставил бы от
квадратного макета узкую полосу. Порядок разделов теперь «Скорая помощь» →
«Коммерческие предложения» → «Боевые листки» → «Технодайджест», и на главной, и в
строке чипов над списками (просьба пользователя: «Скорая помощь» первой).
`check_home_screen.mjs` — **9 проверок**. Плагин изменился только версией и
новостью ЦУП; сервис, бот и база не тронуты. **Бюджет картинок разделов почти
исчерпан: 193 812 из 204 800 Б gzip.**

**0.10.0 (2026-11-14): приложение открывается заглавной страницей.** Корень
Mini App (`#/`) — три карточки разделов («Коммерческие предложения», «Боевые
листки», «Скорая помощь») с макетами, вшитыми в бандл как `data:`-строки
webp; из каждого списка — строка разделов и возврат на главную; у отказа
входа причина видна на главной, а не вместо неё. **Дефект, найденный
проверкой:** маршрут читался до подписки на `hashchange`, и адрес из кнопки
бота (`#/list/offers`) открывался приветствием — теперь адрес читается после
подписки (`readHash()` в `offers-app/src/main.tsx`). Бюджет приложения
считается раздельно: код и картинки разделов, у каждого свой потолок 200 КБ
(`offers-app/scripts/check_size.mjs`). Ruчка `POST /api/offers/app/answer`
внесена в OpenAPI (её не хватало с 0.9.28). Проверка экранов —
`offers-app/scripts/check_home_screen.mjs` (реальный браузер, 9 проверок —
четыре карточки, «Скорая помощь» первой, экран `#/digest`).

**0.9.30 (2026-11-14): макет и тексты систем в КП из чата.** Живое КП 031/2026 ушло в чат
упрощённым (313 КБ вместо макета на 2.37 МБ), без описания первой системы и без
второй, а сборка шла минутами. Три правки: `/app/answer` получил длинный предел
(`writeTimeoutFor`), ошибка макета пишется в журнал (а не молча уходит в блочный
путь), а уже написанные тексты пишутся в КП в контексте **без отмены запроса**
(`context.WithoutCancel`) — именно отмена теряла их и заставляла модель работать заново.

**0.9.29 (2026-11-14, RELEASED):** `backend` `a48bdea`, `main` `fb23107`;
self-hosted `9e6ad1f3…` / `ffe7a37b…` / `a32ac3df…`, registry `sbe-apstore-registry` `f4e0e6d`,
app `assets/index-ChpvzRrm.js`. 0.9.28 (`main` `a9e064f`) — файл через бота.

**0.9.29 (2026-11-14): отказ события сейчас виден.** Живая проверка 0.9.28 дала
«закрылось, файла нет», а в журналах Caddy и сервиса **не было ни одного
`/api/offers/app/answer`** — запрос не уходил. Приложение больше не закрывается при
отказе (`close()` — только после подтверждённой отправки), под кнопкой есть
строка «ключ есть / нет ключа Telegram», а сервис пишет в журнал каждую отправку
с `chat_id` и размером файла.

**0.9.28 (2026-11-14, RELEASED):** `backend` `23a2dcb`, `main` `3623635`;
self-hosted `899e87b4…` / `4587c651…` / `a32ac3df…` (verified over https, in `/registry.json`),
registry `sbe-apstore-registry` `8f9b1de`; service and bot rebuilt and recreated
(`*.bak-20261114-0928`), app deployed as `assets/index-BGVw3l4T.js`. См. «0.9.28» ниже.

**0.9.28 (2026-11-14): «Отправить в чат» — файл через бота (`sendDocument`).**
`Telegram.WebApp.sendData` работает **только у Mini App, открытого кнопкой
клавиатуры** (документация Telegram). Приложение КП открывается inline-кнопкой
`📱 NNN/yyyy`, поэтому `sendData` там ничего не делал и **не бросал ошибки** — только
закрывал окно (пустой журнал бота — тому подтверждение). Теперь:
`sendchat.ts` шлёт `POST /app/answer {initData, action, kind, id}`;
`offers-service/appanswer.go` проверяет подпись и собирает теми же путями, что
`handleBotDocx`/`handleBotExportCombat`; `offers-bot` ручка `/internal/answer` — `sendDocument` в чат человека
(`answerWebAppQuery` непригоден: нужен `file_id`, а в свой чат боту Telegram писать не даёт). Путь только через бота: у сервиса нет доступа к `api.telegram.org` (СОКС — у бота).
`chat_id` берётся из подписи, не из тела. Грабли версии — «0.9.28» в `HANDOFF.md`. — **RELEASED 2026-11-14.** Three points converge:
`backend` `85d61ad`, `main` `b7829fe` (release slice; service, bot and
`offers-app` live on `backend` only), self-hosted
`13cdb98b…` / `27a899d8…` / `a32ac3df…` (verified over https, in
`/registry.json` **and** through `raw.githubusercontent.com`), registry
`sbe-apstore-registry` `a01d8bd`. Only `offers-app` (the Mini App), the plugin
version and the CUP news changed; service and bot were **not** rebuilt or
recreated, no compose edit. App deployed to `/app-offers/`
(`assets/index-C6_fhOtG.js`, `index.html`, `vendor/telegram-web-app.js`; the old
bundle removed, read of its old name now answers the SPA fallback).
See «0.9.27» below. The previous release is 0.9.26 (`main` `ab59f03`).

**0.9.27 (2026-11-14): «📤 Отправить в чат» no longer swallows the refusal.**
Live complaint «в мини-аппе выбрать отправить документ в чат — ничего никуда
не приходит», clarified: the App **closed** and the chat stayed empty. Cause is
ours: `sendToChat` (`offers-app/src/tg.ts`) called `app.sendData(...)` without
`try`; the platform throws `WebAppDataInvalid`, the exception went to React and
the next line `app.close()` closed the App anyway. Now `sendToChat` returns the
error text and does **not** close on failure (`close()` only after a successful
`sendData`); `sendDocumentToChat` passes that text to the screen. Guards:
`offers-app/test/tg.test.ts` (2 cases; they fail with `WebAppDataInvalid` on the
old code — verified by reverting). The button type stays `web_app` and the bot
poller still reads `message.web_app_data` — if the screen now shows «Telegram не
принял данные для бота», the platform refuses `sendData` itself and the button
type is the next (separate) decision.

**0.9.26 (2026-11-14, P5b): the agent reads emails linked to a product —
RELEASED.** `get_emails` and `read_email` (the tool list is now **15**: 13
after P6-bis + the two email tools) return only letters whose signed-scan card
carries `product_refs`. Verified live that the mailer route answers `200` with
the secret and `403` without it, that the raw answer carries no `file_key`, and
that on live data 200 scanned letters are currently **all hidden** (no scan has
been tagged yet — that is the model, not a defect). Plugin `main.js` changed
only through the version and the CUP news text. Full write-up —
«ЗАДАЧА ДЛЯ АГЕНТА (P5b)» below.

**0.9.23 (2026-11-14, P6-bis): the agent reads ANY note + competitor tools —
RELEASED.** Also the bot menu compaction in the same batch. See «P6-bis» below.
The CUP news was rewritten (the second needle in `check_bundle_strings.mjs`
replaced the 0.9.22 one); `main.js` changed through the news string only.

**0.9.22 (2026-11-14, P6): the agent in the bot — RELEASED.** See «Agent in
the bot — P6» below for the full map; the plugin version was bumped and the CUP
news rewritten (the «🤖 Помощник» button is user-visible; `main.js` changed
only through the news string).

**0.9.21 (2026-11-13, R3): materials as offer objects — RELEASED.**
A material can now be offered by DESCRIPTION, like a system (user decision
§3 of the plan): the offer gets its text and links, never a price. Model:
`model.Ref` is the single `{sku, noteId}` pair; `OfferSystems` and
`OfferMaterials` are two named `RefList`s over it, so the merge rule exists
once (`Add`/`Remove`/`Same`/JSON tolerance to the old object shape).
- DB: `offers_offer.material_choice JSONB NOT NULL DEFAULT '[]'`; every write
  moves `updated_at`, reads are `RFC3339Nano`. A full save (`PUT /offers/{id}`)
  does NOT clear it — the same "empty means untouched" rule as `system_choice`.
- Routes: `POST/DELETE /offers/{id}/material` (JWT `editor`) and the bot twins
  `POST/DELETE /bot/offers/{id}/material` (service secret + `chat_id`).
  `assembleMaterial`/`removeMaterial` live in `material_notes.go`.
- **Removal accepts either key.** The stored choice carries BOTH `sku` and
  `noteId` (the server writes both on add), while `Same` compares by SKU when
  the SKU is non-empty — so a delete by note id did not match its own row.
  `resolveMaterialChoice` fills the missing half before `Remove`. Found by the
  stand, not by the unit test.
- Text: ONE block, `material.description`, stored in the SAME
  `offers_system_text` table (the table is "texts by note and block"). Second
  table for one block would be one rule in two places.
- Document: block `repeat: "materials"` now prints the CHOSEN materials
  (title/description/links) when the offer has any, and falls back to positions
  for older offers. **One placeholder** — which list a marker iterates over
  comes from the block's `repeat`, not from the marker name (user decision:
  a second marker would split the single master layout).
- Print contexts: `layoutContext.materialCards` (list) and
  `renderCtx.chosenMaterial` (the current pass's card) — the block-form path
  must carry the row's material or `{{материал}}` inside the block prints the
  offer's first position.
- Plugin: `Offer.materials`, `addMaterial`/`removeMaterial` (the shared
  `writeRef` replaces `writeSystem`), card «Материалы в предложении
  (описанием)» next to «Материал в позиции» (the labels differ on purpose).
- App: «Материалы (описанием)» zone with its own search (`/materials/search`).

**0.9.25 (2026-11-14, RELEASED): the bot stops looking frozen.**

Live complaint right after 0.9.24: «при выборе продукта бот долго думает,
кажется, что завис, хочется нажать повторно». Measured on the live service
(`gen:` lines of `docker logs mailers-offers-1`): one product = **two** model
calls, `description` 20.8/21.6 s and `advantages` 20.6/24.3 s — ~45 s of
silence, and the silence lands while the list is shown, before «Добавить
выбранные» is even pressed.

- **`writeBlocksInParallel` (`systemtext.go`)** — the two blocks are written
  concurrently; ~45 s → ~25 s. Independent by construction (own instruction,
  own hash, no shared state). One failing block does not cancel the other, and
  `missing` keeps the block order. Grouped in a pure function so it is tested
  without Postgres (`systemtext_parallel_test.go`).
- **«⌛» before the write** (`progressNote`/`updateProgress`, the same helper the
  analogue picker uses) — the note is replaced by the result, so the chat does
  not grow service lines.
- **A second «Добавить выбранные» after the write answers «уже записано»** and
  keeps the `nd:more`/`nd:enough`/`nd:app` row; before, it said «Ничего не
  отмечено» twice (the same class as `cban:all`).
- **Marking a number no longer promises the «контекст / приложение» fork** —
  those buttons belong to the next step, and the old text sent people looking
  for them.

`generateSystemTextsWith`/`writeAndStoreSystemTextWith` are the seams for tests
with a stub model. Checks: `go test ./...` green, `gofmt`/`go vet` clean, bot
`tsc` 0 and **208** tests, plugin `tsc` 0 + `build` OK, `check_bundle_strings`
0, stand **496/0**.

**0.9.24 (2026-11-14, RELEASED): multi-product КП from the bot.**

Live complaint: «выбрал «ТН-ФУНДАМЕНТ Протект Экстра», бот ответил
«продукт записать не удалось»», and «по запросу Logicbase не предложено ни
одного материала». Four defects behind them:

1. `suggestItems` (`flow/suggest.ts`) filled `SUGGEST_LIMIT` with systems
   first, so when the search returned 8+ systems the materials the server
   DID return were dropped. The limit is now split: materials keep at least
   half, and a half that has nothing in it gives its room to the other.
2. `addPickedOfferProduct` (`callbacks.ts`) sent **both** `noteId` and `sku`
   to `/bot/offers/{id}/system`. `FindSystem` matches the SKU FIRST and never
   looks at the note — a card with a same-named SKU could take the choice, and
   a miss produced the generic «записать не удалось». Now only `noteId` is
   sent (SKU only when there is no note), and the server's error text reaches
   the person instead of a swallowed `catch {}`.
3. Materials were never written at all (`return false` for `kind:'material'`).
   They now go through `api.addMaterial` → `POST /bot/offers/{id}/material`
   (`assembleMaterial`, `material_choice`) — by DESCRIPTION, never as a
   priced position (user decision 2026-11-14).
4. Nobody could pick several products. Numbers are toggles now (`✓ N`),
   «✓ Добавить выбранные (N)» writes the marked set, and after the write the
   bot asks **«Нужно добавить ещё продукт или систему — или этого
   достаточно?»** with `nd:more` / `nd:enough` / `nd:app`. The document row is
   created on the FIRST write (`ensureNewDocDocument`), so an abandoned
   conversation keeps what was already chosen (user decision: write
   immediately, not in a batch at the end).

`NewDocState.step` gained `'more'` and `NewDocState.added` holds what has been
written, so the summary names every product. `onNewDocKind` clears
`draft`/`combat`: a row opened before the dialog must not take the new choice.
Checks: bot `tsc` EXIT=0, **207 passing** (new: materials survive an 8-system
list, toggle + «добавить выбранные», «добавить ещё» keeps the previous pick),
stand **496/0** (new section checks: materials written by description, system
and material in one offer, material is not a position, system found by noteId
alone). Plugin and service code untouched; nothing deployed.

**0.9.20 (2026-11-13, R2): one «Создать новый документ» button.** `🧾 Собрать
КП` and `⚔️ Боевой листок` leave the menu (their captions stay in `routeMenu`
as aliases — chat history lives for years). The bot asks the kind, then the
user's own first question: **«Какой продукт или систему вы хотите предложить
покупателю?»** The name goes to the existing `/bot/suggest`
(`api.suggestions`), hits are shown as a numbered list whose numbers are
buttons, «not found» offers «уточнить название» next to «выбрать в приложении»,
and after a pick the person chooses **«Собрать контекст в чате»** (the existing
brief) or **«Продолжить в приложении»** (creates the row, writes the product,
answers with an inline `web_app` button).
- `flow/newdoc.ts` — pure texts/keyboards/state. `RouterState.newDoc` is a
  **separate map** from `foundSuggest`: the two branches consume the list
  differently (here the pick is REMEMBERED and the decision follows; there it
  is added immediately), and one map for two flows is how "нажал — добавилось
  не туда" happened before. `session.newDoc` holds only the kind.
- **`parseCallbackData` now handles a composite prefix** (`nd:pick:3` →
  `{prefix:'nd:pick', id:3}`). Without it `Number('pick:3')` is `NaN` and the
  press is dropped silently — the same class as `cban:all` (2026-11-11).
  Guard: `callbacks-data.test.ts` (4 cases).
- `handleNewDocText` runs **before** the combat branch and before
  `handleDraftText`: while the dialog is open the text is a PRODUCT NAME, not a
  position — otherwise «Мембрана» would become a row with price 0.
Checks: bot `tsc` 0, bot **193 passing**, App 50, plugin `tsc` 0, `build` OK,
`check_bundle_strings` 0 (new needle), `check_marker_registry` OK,
OpenAPI **110/110**, stand **444/0** (new section «Создать новый документ:
ручки, на которые он опирается»).

**0.9.19 (2026-11-13, R1 of the bot-UX batch): the two lists open in the App.**
`📂 Мои КП` and `📂 Мои боевые листки` now sit in **one row** of the main
keyboard, and each **opens its list in the App**, not in chat. A Telegram
reply-keyboard button cannot carry `web_app` — that exact defect (App opening
without a signature, «Нет доступа: войдите в бота») is why the menu button was
removed on 2026-11-13. So the reply button answers with an **inline**
`web_app` button: `appListButton(text, which)` in `flow/appbutton.ts` is the one
place building `#/list/offers` / `#/list/sheets`; `openAppList` in `routes.ts`
is the one handler. The chat list (`handleMyOffers`, `handleMyCombat` call from
the menu) is gone — two list implementations would drift. `/my` / `/combats`
stay aliases; `handleMyCombat` itself is kept (combat-flow tests use it) but is
no longer reachable from the menu.
Checks: bot `tsc` 0, App 50, plugin `tsc` 0, `build` OK, `check_bundle_strings`
0, `check_marker_registry` OK, OpenAPI **110/110**. Stand section «Списки бота
открываются в приложении (R1)» — menu row, no `web_app` in the reply menu,
both lists asked from the service by the bot secret; skips honestly when
`BOT_DIR`/`OFFERS_BOT_URL` are not mounted.
**Plan for the whole batch:** `docs/superpowers/plans/2026-11-13-sbe-offers-bot-ux-and-materials-plan.md`
(R1–R3 done and released; R4 = the bot agent).

**R4 re-framed 2026-11-14** (user decision + live evidence):
`docs/superpowers/specs/2026-11-13-sbe-offers-single-agent-design.md`.
The agent is NOT a third entity next to КП and the combat sheet — **the agent IS
the assembler**: it edits the same row the App and the bot edit, and the linear
bot flows become its tools (they stay in the code; S5 is still user-gated).
Two observations drove this: the portal agent (`b6e9ac7a…docx`) noticed that
«ТН-КРОВЛЯ Стандарт Проф» does not exist and compared the nearest systems,
while the bot "did not understand" the same request; and the bot has never
recognised a system/material from free text (0 of 27 offers, 0 of 14 sheets).
- **P4 done 2026-11-14**: the agent loop is now ONE file —
  `sbe-core/src/agent/engine.ts` (no imports, deps injected), with copies in the
  portal (`web/sbe-web/src/modules/agent/engineCore.ts`) and here
  (`offers-bot/src/agent/engine.ts`). Parity: `plugins/scripts/check_agent_engine.mjs`
  (25 checks). While doing this a **live defect was found and fixed**: the DSML
  parser never worked (regex missed the tag; the tool name sits BEFORE `>`),
  so a model answering in that shape had its markup shown to the user instead
  of being executed.
- **Code heads:** `backend` `8729842` (only the bot's `src/agent/` copy and the docs changed —
  the plugin version stays 0.9.21, nothing was deployed), `main` `c0ae671`.
- **Next: P6** — tool executor in `offers-service` (whitelist over existing
  functions: `suggestionsFor`, `searchSystems`, `renderOfferDocx`, combat
  renderer), the manager-analysis journal `offers_agent_log` +
  `GET /api/offers/admin/agent-log` (role admin, 180 days), and the bot
  transport (`flow/agent.ts`, `/bot/agent/message`). Then **P5b**
  (`/internal/mailer/agent/*`, PDF-only).

**The App is deployed separately from the plugin.** Current bundle:
`assets/index-…js` + `vendor/telegram-web-app.js`, served from our own domain
(`<script src="./vendor/telegram-web-app.js">` — the VDS cannot reach
`telegram.org`, see the gotcha below). The service and the bot were last
rebuilt and recreated on 2026-11-13 (R1/R2 batch).

Per-plugin state, live hashes and the release procedure live in
[HANDOFF.md](HANDOFF.md) — it is the entry point for a cold start.

**The Mini App migration is done through S7 (S0–S4, S6, S7); S5 is
deliberately NOT done** — the user decided on 2026-11-13 that the bot keeps
all its flows until the App has been verified in real use. Nothing was deleted
from `offers-bot`. Do not start S5 without an explicit instruction. The step
notes (S2–S7) and the five live defects of the first launch are in the
archive, «Ревизия 2026-11-13».

## Purpose (current)

The catalogue split into two entities on 2026-10-31 and the sections were
renamed accordingly.

- **Materials** (`offers_material`, `offers_material_ekn`) — what is actually
  sold: its own SKU (assigned once by the server), ЕКН variants, a price
  *range* only. **There is no price column**: the price for a given offer comes
  from the manager's Excel sheet. Materials are created from Knowledge Base
  cards tagged `material` on every read of the list, and they remember the
  source note (`note_title`, `note_id`) — that is what a system's wiki link
  resolves against.
- **Systems** — Knowledge Base cards tagged `commercial_propose` **without**
  `material`. A system has no price of its own; it justifies why this
  combination of materials was chosen. Its composition links to materials by
  wiki reference (`[[Название]]`); a `MAT-…` code is the fallback. Lines with
  neither a link nor a code are **layers/works**, not errors (they carry
  `layer: true` and stay out of `problems`).
  **Choosing a system does not add its materials as positions** (2026-11-03):
  the offer gets the description, advantages and links, while the priced
  composition is entered by the manager (a file or by hand).
- **Offer form** (`offers_form`) — placeholders + document blocks + required
  fields, edited in a block editor built after the LIMS protocol configurator.
  A block is: `paragraph`, `heading`, `bullet_list`, `table`, `page_break`,
  `links`, `prompt_text`, `image`, `compare_table`, plus placement
  (`spacingBefore`/`spacingAfter` in points, `keepTogether`, `pageBreakBefore`)
  and repeat (`repeat: "systems"` / `"materials"`). An uploaded `.docx` master
  (`layout_docx`) overrides blocks entirely.
- **Offer** (`offers_offer`) — a snapshot: positions with the price at the
  moment they were added, form values, total, `draft`/`final`, number
  `{NNN}/{yyyy}` continuous through the year. Visibility: admin sees all,
  everyone else sees their own. The row is created at the **start** of the bot
  conversation with `items: []` (the REST path still refuses an empty offer).
  Chosen systems live in `system_choice` JSONB as a **list** (`Offer.systems`,
  order of choice); the old single-object shape is still read.
  **Chosen materials** live in `material_choice` JSONB under the same shape
  (`Offer.materials`, 2026-11-13): a material offered by DESCRIPTION, not a
  priced row. Adding one never creates a position; priced rows keep coming from
  the composition table (the editor's «Материал в позиции» and the manager's
  Excel). «Материалы в предложении (описанием)» is the object list.
- **Telegram bot** — sign-in by e-mail and code. "Собрать КП" starts with
  context (object → addressee → deal → systems → terms), then positions. A file
  may be sent at any step. `система <название>` adds a system, never positions.
- **Entry point** — the store: "Installed → Open"
  (`publishService('sbe-offers', {open})`).
- **Legacy** `offers_catalog` remains for old offers only; nothing writes to it.
- **Combat sheet** (`offers_combat_sheet`, 2026-11-08) — a quick reference
  comparing *our* product (the systems of a КП, or hand-picked systems) with
  *competitor* products. Snapshot like an offer: `number` `{NNN}/{yyyy}` with
  its **own** year counter, `our_notes`/`competitor_notes` are `{noteId,title}`
  lists, `form_values` under `mode='combat'`, `status`. Competitor products are
  **not** a new table — they are KB notes **tagged `competitor`** (2026-11-10)
  **and** notes in the folder **«материалы и системы конкурентов»** (historical
  source, merged by note id), read through `KBClient.TagID`/`Notes` and
  `KBClient.FolderIDByName`/`SubtreeFolderIDs`/`FolderNotes`, cached in
  `offers_context` under `mode='competitors'` and refreshed by an admin
  button. A competitor card's **title is the note heading**, never the first
  body line. The combat form is the **same form editor as КП** under
  `mode='combat'` (no second editor). DOCX export is done (`combat_docx.go`):
  rendered from the combat form, our and competitor cards printed as two
  sections.
  **In the bot (2026-11-09, document-style picker since 2026-11-10)**: «⚔️
  Боевой листок» creates an empty sheet with a number. Sides are picked by
  **name search** as a **numbered text list** with number buttons, paging
  («Дальше ▸»/«◀ В начало») and a **checkbox-style multi-select**: tap a number
  to toggle it (✓), then «✓ Добавить выбранные» adds everything picked at once.
  When the search returns more rows than fit one page, a **«☑ Выбрать все
  найденные (N)»** button takes the whole match set (it becomes «✖ Снять все
  отметки» once taken); adding then pages through the server and loads every
  match, not just the visible screen (`api.combatAllPaged`). Tapping a number
  manually drops the "all" mode, because the manager is then editing the set.
  This mirrors the portal Telegram bot's «Документы» mode — long product names
  overflowed the button and only a handful of cards were reachable before.
  «➕ Наш продукт» (materials catalogue + systems) and «🏷️ Добавить
  конкурента» ask for a name; «▼ Все продукты» opens the full list. The same
  product can be added on each side several times. «⬇ DOCX» sends the file;
  «📂 Мои боевые листки» lists the chat's sheets. `botcombat.go` reuses the
  plugin's entity and renderer; the owner is the chat's bound email, and text
  typed **outside** a side pick never lands in an offer draft.
  Competitor cards come from the tag **and** the **whole subtree** of the
  «материалы и системы конкурентов» folder.

## Architecture

```
sbe-offers (plugin)  --JWT app_id=offers-->  offers-service (Go)  -->  Postgres offers
                                                    |                    +--> S3 sbe-offers (DOCX)
offers-bot (Node)  --X-Service-Secret-->  offers-service
        +-- long polling through the SOCKS5 sidecar offers-nl-proxy (nl_egress_proxy)
```

- Identity — the shared `auth-service`: e-mail; roles `viewer` < `editor` <
  `admin` come from the central permissions table (`authperms.go`).
- **Sign-in codes, chat_id links and bot roles live in `offers-service`**
  (`bot_codes`, `offers_bot_link`, `offers_access`); `auth-service` is not
  touched.
- Telegram is unreachable directly from the VDS in Russia, so the poller goes
  through SOCKS5 like `portal-embed`.
- Knowledge Base and LLM are read over the internal network; both need a URL
  *and* a secret in the compose block, and the stand needs a stub for each.

## Plugin structure

| File | What it is |
|---|---|
| `src/main.ts` | `SbeOffersPlugin`: settings, view, publishService, CUP news |
| `src/constants.ts` | `OFFERS_VIEW_TYPE`, `OFFERS_APP_ID`, `OFFERS_PLUGIN_ID` |
| `src/types/offers.ts` | `Material`, `MaterialEkn`, `CompositionItem`, `SystemCard`, `SheetPreview`, `Offer`, … |
| `src/types/settings.ts` | `OffersSettings { apiUrl, lastAnnouncedVersion }` |
| `src/services/offers.service.ts` | HTTP to `/api/offers/*`, `OffersError`, 401 retry, `exportDocx`, `refreshSystems`, 300 s timeouts on long calls |
| `src/ui/offers-view.ts` | facade: top bar + sidebar (grouped: КП / Боевой листок / Справочники / Администрирование) + content |
| `src/ui/picker.ts` | the single product picker: name search + result buttons, taken from the combat view and reused by the offer editor (2026-11-12 removed the two dropdowns) |
| `src/ui/offers-list.ts` | «🧾 КП»: list, card, composition editor, "Добавить систему", DOCX, link, header button «⚙️ Форма КП» |
| `src/ui/combat-view.ts` | «⚔️ Боевой листок»: list of combat sheets, editor card (our systems + competitors from KB), form fields; header button «⚙️ Форма боевого листка» |
| `src/ui/competitors-editor.ts` | «🏷️ Конкуренты» (admin): cached competitor cards from the KB folder, one «↻ Перечитать» button |
| `src/ui/material-editor.ts` | «🧱 Материалы»: list with SKU, card with the ЕКН table |
| `src/ui/systems-editor.ts` | «🧩 Системы»: cached systems, resolved composition, problems; one "↻ Перечитать из Базы знаний" button |
| `src/ui/sheet-upload.ts` | manager's Excel: "what I understood", then apply |
| `src/ui/context-editor.ts` | «📚 Контекст КП»: cached system cards, open note |
| `src/ui/brief-editor.ts` + `brief-model.ts` | the brief card inside the offer editor |
| `src/ui/form-editor.ts` | «⚙️ Форма КП» / «Форма боевого листка»: fields, blocks, required, describe-block, layout card. One editor for both documents — takes `mode` (`'client'` / `'combat'`) |
| `src/ui/block-editor.ts` | block editor: nodes, `{{field}}` chips, placement, image source, repeat, copyable block-id marker |
| `src/ui/form-preview.ts` | live preview of the document under the blocks |
| `src/ui/describe-block-modal.ts` | «✨ Опишите блок словами»: free text → nodes |
| `src/ui/form-layout.ts` | «.docx layout» card: upload / download / delete, tags found in the file, marker cheat sheet |
| `src/ui/markers.ts` | **the single registry of every layout marker** (chips, layout card, help) |
| `src/ui/image-picker.ts` | picks an image from the vault, uploads it to KB attachments |
| `src/ui/access-editor.ts` | «👤 Бот и доступ»: codes, bot token, who signed in |
| `src/ui/help.ts` | help text, built from `markers.ts` (baked into the plugin) |
| `src/utils/{date,money,note-path}.ts` | shared helpers — one copy each |
| `src/styles.css` | `tn-offers-*` classes (the root `styles.css` is **built**, never edit it) |
| `scripts/check_bundle_strings.mjs` | proves the Russian labels reached `main.js` (escaped `\uXXXX`, uppercase) |
| `scripts/check_marker_registry.mjs` | markers in the plugin vs markers the server parses |
| `scripts/check_docs_state.mjs` | docs vs code: version, SHA-256 of the build, HEAD of both branches |
| `scripts/mailer-stub.mjs` + `documents-stub.mjs` | stand stubs for the agent's emails: the mailer stub replays the RECORDED fixture `scripts/mailer-fixture.json` (live response 2026-11-13) and keeps one scanned letter unlinked on purpose; the documents stub serves the one linked card |
| `scripts/offers_local_e2e.mjs` | the stand: **505 checks**, 29 sections |
| `scripts/check_layout_mode_defect.py` | live HTTP check of the layout modes (offer vs combat sheet) |
| `scripts/check_app_auth.mjs` | live check of the Mini App sign-in: initData signed with the real bot token, 8 checks (accept / wrong token / stale / tampered / empty / garbage / config / bare route still 401) |
| `scripts/{auth,kb,llm,bot}-stub.mjs` | stand stubs for the services the plugin talks to |
| `offers-app/` | **Telegram Mini App** (React + Vite + TS, 2026-11-13). Owns the document — КП systems/positions/texts and combat sheets; the bot keeps identity, entry and the brief. Hash routing (`#/list/offers`, `#/offer/:id`, `#/sheet/:id`), `base: './'`, built to `offers-app/dist/`. **S0+S1+B+ done**: scaffold, placeholder screens, sign-in (initData → JWT), and the bot's «📱 Открыть приложение» buttons. Bundle deployed to `/app-offers/` (Caddy). Checks: `npm run typecheck`, `npm run build`, `npm run size` (200 KiB gzip budget — 72 KiB used), `npm test` (12 passing). Plan: `docs/superpowers/plans/2026-11-13-sbe-offers-webapp-{START-HERE,migration-plan}.md` |

**Mini App, S1 — identity (2026-11-13).** `offers-service/appauth.go`:
`verifyInitData` (HMAC per Telegram docs, constant-time compare, `auth_date`
freshness `OFFERS_APP_INITDATA_TTL` default 24 h, duplicate-key and 16 KiB
guards), `signAppJWT` (the same `jwtClaims` the plugin uses, `device_id`
`app:<chat_id>`, 1 h), `POST /api/offers/app/session` (initData → JWT) and
`GET /api/offers/app/config` (`appUrl`, `botUsername`, `initDataTtl`).
The bot token comes from `offers_bot_config` only — never from env.
Codes: 400 malformed, 401 bad/stale signature, 403 `not_linked`, 503 no bot
token. Proven live (`scripts/check_app_auth.mjs`, 8 checks) and by
`go test` (initData paths + the JWT going through the real `authorize` gate).

**Roles for managers — closed 2026-11-13 (user decision).** `offers-service`
reads roles from the central auth-service map, and `offers` had **no entry in
`app_common_access`**, so a manager who only signed in to the bot
(`offers_access` = viewer) got `role: ""` and `403` on every JWT route. The user
chose common access `editor`: written through
`POST /internal/apps/offers/common-access` (actor `polishchuk@tn.ru`), verified
live — ladnaya / m.krylov / romov now get `role=editor`, `/offers` and
`/combat` answer 200, the owner stays `superadmin`. **The App is only as good as
this map: a new employee needs either the bot sign-in (falls under common
access) or a personal role.**

## Server structure

`offers-service/`: `main.go` (routes, migrations), `jwt.go`, `register.go`,
`authperms.go` (a copy of the other services — **edit all copies at once**),
`permissions.go`, `catalog.go`, `material.go`, `material_notes.go`,
`matcher.go`, `sheet.go`, `upload.go`, `systemcards.go`, `systems_compose.go`,
`systems_links.go`, `botcatalog.go`, `botapi.go`, `form.go`, `offers.go`,
`brief.go`, `briefstep.go`, `copystep.go`, `attach.go`, `kbclient.go`,
`combat.go` (combat-sheet entity, own number counter, CRUD; 2026-11-08),
`competitors.go` (competitor cards from the **`competitor` tag** and the KB
folder subtree, merged by note id, cache in `offers_context`
`mode='competitors'`),
`combat_docx.go` (combat-sheet DOCX: blocks by the combat form, both sides printed
as card sections; reuses the block/text pipeline),
`botcombat.go` (combat sheet in the bot, 2026-11-09: our product = catalogue
materials + systems, competitors = tag + whole folder subtree; name search and
server-side paging `offset`/`limit` (`combatOurItems`, `filterCombatItems`,
`paginateCombatItems`, `parseCombatPage`), create, add/remove a side, DOCX —
reuses `combat.go`/`combat_docx.go`, owner is the chat's bound email),
`pickersearch.go` (KB search for the offer-editor pickers and the bot's
`/suggest`/`/bot/suggest`, 2026-11-12), `appauth.go` (Mini App sign-in,
2026-11-13: `verifyInitData` over the bot token from `offers_bot_config`,
`signAppJWT` with the plugin's claims, `POST /app/session`, `GET /app/config`),
`kb_image.go`, `llmclient.go`, `gentrace.go` (per-route timeouts + generation
log), `systemtext.go` (`offers_system_text`, `generateSystemTexts`),
`docx.go` (render entry points, service fields, composition printing),
`docx_numbering.go` (a fresh `numId` per system list), `docx_text.go` (prompt
blocks, cache keys, per-system texts), `docx_layout.go` (printing the user's
`.docx` master), `docx_runs.go` (one text pipeline: wiki links, glued words,
superscripts, `paraOpts`), `docx_links.go`, `docx_image.go`, `docx_media.go`,
`docx_compare.go`, `docx_card_images.go`, `card_images.go`, `layout_tokens.go`
(`[…]`/`{…}` scanner, field aliases), `layout_service.go` (marker printing),
`form_layout.go`, `form_describe.go`, `s3.go`, `email.go`, `static.go`,
`template.go`, `static/openapi.yaml`.

Sub-packages: `model/` (domain types + `jsonrepair.go`, shared with `skills`);
`skills/` (prompts, checklists, card and attachment parsing, `cardrefs.go`,
`filecontext*.go`, `blockstructure.go`).

`offers-bot/`: `src/main.ts`, `src/poller.ts` (transport only), **`src/agent/engine.ts`** (копия
канона цикла агента из `sbe-core/src/agent/engine.ts`, 2026-11-14 — пока не
подключён к маршрутам, подключается шагом P6), `src/routes.ts`
(routing + the single sign-in check + `openAppList`), `src/callbacks.ts` (button
handlers, including the combat sheet — 2026-11-09 — and the `nd:*` family of
«Создать новый документ»), `src/telegram.ts` (Bot API via SOCKS),
`src/session.ts`, `src/attach.ts`, `src/flow/` (`build`, `brief`, `copy`,
`writer`, `resume`, `combat`, `suggest`, `appbutton`, **`newdoc`**),
`src/api.ts`, `Dockerfile`.
Tests: `routes.test.ts`, `newdoc.test.ts` (12 cases of the dialog),
`callbacks-data.test.ts` (4 on `parseCallbackData`), plus per-module suites.

**Opening the Mini App from the bot (2026-11-13, step B+).** `flow/appbutton.ts`
is the single place that builds the App URL (`OFFERS_APP_URL`, default
`https://epyur.fvds.ru/app-offers/`) and the buttons; `MAIN_KEYBOARD` has
«📱 Открыть приложение» and both lists put one `web_app` button under every
record (`📱 NNN/yyyy — <product>`). A `web_app` button opens the App by itself —
no `callback_data`, no handler, and `message.web_app_data` is not involved
(that is S7). Tests: `src/appbutton.test.ts` (9 cases: URL building, env
override, menu, both lists, the bot's own list rendering).

## Settings (data.json)

`apiUrl` (default `https://epyur.fvds.ru`), `lastAnnouncedVersion`.

## Environment variables (server)

`DATABASE_URL`, `PORT`, `JWT_SECRET`, `OFFERS_APP_ID` (`offers`),
`OFFERS_APP_NAME`, `OFFERS_OWNER_EMAIL`, `OFFERS_SERVICE_SECRET`,
`OFFERS_BOT_SECRET`, `AUTH_SERVICE_URL`, `S3_ENDPOINT`/`S3_ACCESS_KEY`/
`S3_SECRET_KEY`/`S3_BUCKET` (`sbe-offers`), `SMTP_*`, `KB_SERVICE_URL`,
`LLM_SERVICE_URL`, `LLM_SERVICE_SECRET`.
Bot: `TELEGRAM_SOCKS_PROXY`, `OFFERS_API_URL`, `OFFERS_BOT_SECRET`,
`OFFERS_BOT_RATE_LIMIT`, `VERIFY_PORT`, `OFFERS_APP_URL` (optional — the Mini
App address for the `web_app` buttons; empty means the production
`https://epyur.fvds.ru/app-offers/`).

**There is no bot-token variable and there must not be one** (2026-10-29): the
token lives in `offers_bot_config` and is entered from the plugin's «Бот и
доступ» section.

## Checks

```
npx tsc --noEmit && npm run build
node scripts/check_bundle_strings.mjs
node scripts/check_marker_registry.mjs
node scripts/check_docs_state.mjs
cd offers-service && gofmt -l . && go vet ./... && go test ./...
cd ../offers-bot && npm run typecheck && npm run build && node --test "src/**/*.test.ts"
cd ../offers-app && npm run typecheck && npm test && npm run build && npm run size
cd .. && node ../scripts/check_offers_openapi.mjs
python scripts/check_layout_mode_defect.py http://127.0.0.1:18080
```

**The stand runs from Docker — it is always available on the dev machine.**
Mount the bot's folder too: two sections verify it (`BOT_DIR`); without the
variables they print «пропущен» instead of failing.

```
docker run --rm --network sbe-offers_offersnet \
  -e OFFERS_BOT_URL=http://offers-local-bot-stub:3000 -e BOT_DIR=/bot \
  -v "$PWD/scripts:/app:ro" -v "$PWD/offers-bot:/bot:ro" -w /app \
  node:22-alpine node offers_local_e2e.mjs http://offers-local-app:3000
```

Expected on 0.9.23 (verified 2026-11-14): plugin `tsc` EXIT=0, `build` OK,
`check_bundle_strings` 0 missing, `check_marker_registry` OK,
`check_docs_state` all OK, `go test` green, `check_agent_engine` green, OpenAPI
**117/117**, stand **491/0**. The bot's own suite: `typecheck` EXIT=0, **202
passing**; the App: `tsc` 0, **51 passing**.

**After `npm run build` check the bundle with the script, never by eye.**
esbuild escapes Cyrillic as `\uXXXX` in **uppercase** hex; searching lowercase
gives a false "missing".

**The stand binary must be rebuilt after service edits**, with `CGO_ENABLED=0`
(a CGO build dies in alpine with "no such file or directory"):

```
docker run --rm -v "$PWD/offers-service:/src" -w /src -e CGO_ENABLED=0 golang:1.24 \
  sh -c "go build -o /src/offers-stand ."
docker cp offers-service/offers-stand offers-local-app:/mailers-offers
docker restart offers-local-app
```

Before a stand run, drop the systems cache — it hides `kb-stub` edits:
`docker exec offers-local-db psql -U offers -d offers -c "delete from
offers_context where mode='systems'"` and restart `offers-local-kb-stub`.

**Moving files to the server: compare md5 AFTER normalising line endings.**
Git checks out `.ts` as CRLF on Windows and LF on the server; the content is
identical and only the bytes differ. Normalise (`\r\n`→`\n`) before declaring
a mismatch. Guard tested 2026-11-13.

**Listing remote files: `find -print0 | xargs -0 md5sum`, NOT `while read`.**
An `ssh '... while read ...'` swallows stdin — the same class as the
`docker exec -i` gotcha.

## Rules

- `catch (e: unknown)` + `errorMessage()`; `requestUrl()`; `window.setTimeout()`;
  no `any`; UI in Russian; `tn-offers-*` classes on semantic tokens;
  **no native `confirm`/`alert`/`prompt`** (they break field focus in Obsidian —
  use the project's own modal).
- Commits, pushes, deploys and self-hosting only on an explicit user command,
  for one batch. «Продолжай», «ок», «делай дальше» are not permission.
- **«Фиксируй»** = bump the version (+0.0.1 in `manifest.json` and
  `package.json`), rewrite the CUP news text, update the docs, prepare the
  commit message, ask before committing. Do not bump for purely back-end
  changes unless the news needs to reach people.
- Go dependencies: permissive licences only (MIT/BSD/Apache-2.0); check
  `LICENSE` before `go get`.
- **Editable formats only — never PDF** (rule 2026-11-14, the user's direct
  requirement: «в пдф мы не конвертируем, нам нужен редактируемый формат,
  менеджеру может быть необходимо внесение изменений уже после генерации»).
  Offers and combat sheets are produced as **DOCX and only DOCX**: no
  conversion step, no headless office, no image-only export. PDF is also never
  *read* (`documentTextOf` returns `""` for it — P5b/P6). A request for a PDF is
  «print the DOCX»; an actual converter is a separate design with its own
  decision, not a flag. Design: `docs/superpowers/specs/2026-11-14-sbe-offers-content-sections-design.md` §13.
- **The generated DOCX must describe itself** (rule 2026-11-14). A cached field
  that came from the master — the table-of-contents entries and their
  `PAGEREF` anchors — is rebuilt from the headings the print actually produced;
  nothing from the template's cache survives into a document whose content
  differs from the template's. `<w:updateFields w:val="true"/>`
  (`ensureUpdateFields`) is kept so Word fills page numbers on open, but it is
  **not** enough: a file read in Telegram never opens Word, and used to show the
  template's stale TOC (`_Toc245067377`…`380`, pages 3/3/4/5). Honest limit:
  page *numbers* need a layout engine we do not run; entry text, order and
  anchors are always the document's own.
- `.gitattributes` with `-text` on `manifest.json`/`main.js`/`styles.css` —
  present, keep it.
- Any data write in the database must move `updated_at`.
- Documentation records in this file are written in English (rule 2026-10-27);
  older Russian entries are not translated retroactively.
- Files are written as UTF-8 **without BOM**, with absolute paths.
- **Run the `ub-mode` skill** at the start of a task and before a commit.
- **Combat sheets are readable by EVERYONE signed in** (rule 2026-11-12, the
  user's direct requirement: «у нас листки должны быть доступны все всем»).
  A sheet is an internal reference for arguing with a customer, so a colleague
  opens the ready comparison and prints it instead of rebuilding the same thing.
  `canSeeCombat` returns `true` unconditionally and `handleListCombat` has no
  owner filter. **Editing stays with the author** — `handleSaveCombat` and
  `handleDeleteCombat` still check the owner (403 otherwise), and in the bot
  `botCanEditCombat` is the gate. Do not re-close reading: the earlier
  "admin sees all, others only their own" rule is superseded. Proven by the
  stand: «боевой листок виден чужому сотруднику», «чужой не может править
  боевой листок», «список боевых листков показывает чужие листки».
- **Every release check MUST verify authorisation and bot function separation**
  (rule 2026-11-12, user's direct requirement: «обязательно проверять:
  1. авторизацию 2. чтобы не было смешения между функциями бота»). Both are
  checked **before** the version bump and again on the deployed service, not
  once at the end of the batch.
  - **Authorisation** — every route keeps its own gate and nothing new is left
    open. Checklist: `requirePerm("viewer"/"editor"/"admin")` on all
    `/api/offers/*`; `requireServiceSecret` on every `/api/offers/bot/*`;
    the bot signs in by e-mail + code and its owner is the chat's bound address;
    `botCanEditCombat` still lets only the author edit a sheet; `offerForEditor`
    still refuses someone else's offer; the plugin's 401 retry still forgets and
    re-requests the token once. A new route without one of these three gates is
    a defect, not a style issue.
  - **Bot function separation** — one handler per intent, no shared "what did
    the user mean" switch. Checklist: free text goes through `handleCopyText` →
    `handleBriefText` → `parseSystemCommand` → stage/step branches → the
    suggestion fallback, in that order and each one returns `true` only when it
    actually handled the text; a side pick inside a combat sheet never lands in
    an offer draft and vice versa (`isCombat(session)` gates the branch);
    `Работа с КП` and `Работа с боевым листком` never share session fields
    (`draft` vs `combat`); signing in, brief, positions and combat are separate
    `RouterState` maps. A new text or button belongs to exactly one of them —
    if it needs to be known by two, that is a missing split, not a shared
    helper.
  - **Where it is proven**: the stand (`offers_local_e2e.mjs` — «чужой КП
    недоступен», «чужой боевой листок недоступен», bot-secret sections) and
    the bot suite (`routes.test.ts` — sign-in path, combat flow). A release
    without a green run of both is not a release.

## Gotchas

Each item names the code it applies to. The stories behind them (dates,
measurements) are in the archive.

### Russian text, regexes and search

- **`\b` is an ASCII word boundary — never use it on Cyrillic.**
  `\b(?:MAT|РАБ)-\d{4}\b` silently finds nothing in `РАБ-МОНТ-ОБЩ-0001`. Write
  the boundary explicitly (`(?:^|[^А-ЯЁA-Z0-9])`) and add a test with a real
  `РАБ-…`/`MAT-…` line. Caught three times: `skills/systemcard.go`,
  `matcher.go`, `skills/cardrefs.go`.
- **Cyrillic in `main.js` is `\uXXXX` in uppercase hex** — use
  `check_bundle_strings.mjs`. esbuild also writes «ёлочки» in a plain string
  literal as `\xAB`, which the checker does not know: pick a needle without
  quotes.
- **A source file rewritten through a PowerShell pipeline comes back as
  mojibake.** Edit with the editor tools or Python with an explicit encoding.

### Building and the stand

- **A full disk on the VDS is Docker's build cache, not the databases**
  (2026-11-13, lived incident: PostgreSQL recovery loop
  `could not write to file "pg_logical/replorigin_checkpoint.tmp": No space
  left on device`, every app down). `df -h /` showed 0 bytes free while
  `/var/lib/containerd` held 66 GB, of which 63 GB were
  `snapshotter.v1.overlayfs/snapshots` and `docker buildx du` reported
  **64 GB cache, 62 GB reclaimable** — the leftovers of every service rebuild.
  Fix: `docker buildx prune -af` (images untouched, only build cache) — freed
  58 GB in one run. **Check `docker buildx du | tail -3` before blaming the
  databases**, and re-check `df -h /` after any big rebuild.

- **A layout belongs to a MODE, and the mode must be visible in the answer.**
  (2026-11-11, lived defect: the offer was exported as an empty combat sheet.)
  `offers_form` keeps one `layout_docx` per `mode` (`client`/`combat`) — that
  part was always right. What was missing: `POST /form/layout` answered only
  `hasLayout`/`layoutName`, so a master uploaded for the wrong mode looked like
  success and the fault surfaced at print time. `uploadLayoutResponse` now
  returns `mode`, the plugin's Notice says which document took the file and
  warns on a mismatch, and the stand section «Макеты: у КП и у боевого листка —
  свои» proves both directions. **Any upload that writes into a per-mode or
  per-entity row must say which row it wrote to.**
- **`up -d` without `--force-recreate` may not recreate the container.** Check
  `CREATED` in `docker ps`, not the word `Started` in compose output.
- **Piping a remote script through `tail` returns `tail`'s exit code.**
  `ssh host "bash x.sh 2>&1 | tail -20"` prints `EXIT=0` for a failed build. Run
  the check as its own line, read `$?` at once, and verify with `md5sum` that the
  file really landed.
- **`docker exec -i` inside a `while read` loop eats the rest of the list.**
- **A stub I write is a claim about someone else's contract.** Pin foreign
  contracts with a recorded real response, and make the stub filter like the
  real service. **Match a stub on the half of the prompt where the service
  actually writes the marker** — `llm-stub.mjs` matched the brief on the system
  prompt while the marker lives in the user prompt, and the stand was red for
  months.
- **A stub that cannot express the live data shape proves nothing.** Two live
  defects hid behind it (no `system_sku` on live cards; no `## Ссылки` on the
  stub card the links check prints).
- **A check that searches the WHOLE document for a construct the document
  legitimately uses elsewhere is not a check.** Scope it (`itemsTableXML`,
  `compositionSection`).
- **A service that talks to another service needs both the URL and the secret
  in its compose block, plus a stub on the stand.**

### Data and documents

- **A screen that reads another service live on every open will eventually show
  that service's error to the user.** `GET /systems` did exactly that. Read from
  a cache, refresh on an explicit button.
- **Service-to-service clients need the same 401 retry as the plugin.**
  `KBClient.get` drops the token and repeats once.
- **Time in API responses must be `RFC3339Nano`, not `RFC3339`** — the short
  layout drops fractions and the server then loses the LWW merge to itself. Any
  UPDATE that changes data must also move `updated_at`.
- **`returning` a CTE subquery sees the row AFTER the update** — read the old
  value with `UPDATE … FROM (SELECT … FOR UPDATE) AS old … RETURNING old.col`.
- **`input.files` is a live `FileList`**: take `files[0]` before clearing
  `value`.
- **Word: only the brand template's styles.** `styles.xml`/`numbering.xml` come
  from `standard.docx` untouched (`a0` Normal, `1` heading 1, `numId 6` bullet,
  `ab` Hyperlink). Replacing them removed footer page numbers. `numbering.xml`
  may be **appended** to, never replaced — that is how each system gets its own
  `numId` with `startOverride`.
- **`wp:inline` breaks the text flow; `wp:anchor` + `wp:wrapSquare` does not.**
- **An offer without positions still produces a document**; never print
  «Итого: 0,00». `validateOffer` still refuses an empty offer on the REST path —
  do not reintroduce that check inside the renderer.
- **A page break at the very start of the document is ignored** by Word.
- **Cached model text must be keyed by everything the model saw.** The data hash
  (`gen.section`) covers the offer row only; KB cards and the chosen-system set
  live elsewhere and are invisible to it. A text that depends on them needs its
  own fingerprint in the key (`gen.systems`, `...sel<hash>`), or it silently
  freezes (2026-11-08: the intro described one system after a second was added).
- **A narrowed card list is for the described system, not for naming the set.**
  A `repeat: "systems"` block feeds only the described system's card to the
  prompt, so building the «выбрано» list from those same cards made the other
  system fall back to «система №N» — the model wrote that number into the prose
  and it reached the client (0.9.2, live КП 003/2026). Pass the full selected
  set (`allCards`) for the names section, separate from the narrowed `cards`
  used for the card body. When an already-written text is wrong and the keys
  hash only the offer row, bump the `hashOfferFacts` version (`v3`) to discard
  every cached block once.
- **Our own `max_tokens` truncates answers; the provider's does not.** Send 0
  and let the provider choose; retry 3 times on timeout/502/504/429.
- **When a block goes empty, read the cached `form_values` first.** Three
  releases were spent tuning prompts while the defect was downstream of them, in
  the post-processing.
- **A layout marker describes the FORMAT, not the content** — strip it, keep the
  text.
- **A hyperlink's visible text is the LABEL, never the URL.** Printing
  «подпись — адрес» put the raw navigation URL (with
  `?response-content-disposition=filename="…"`) on the page, and the layout
  auto-linker turned that tail into a clickable link showing the URL (0.9.4,
  live КП 003/2026). The URL belongs only in the relationship; the reader sees
  the card's label. One helper, `refDisplayText`, on every place that prints a
  link (`renderLinkSection`, `renderLayoutCardLinks`, `applyHyperlinks`), so
  they cannot drift.
- **Navigation card URLs contain spaces and quotes** inside
  `response-content-disposition=filename="…"`. The markdown-link regex must
  allow them: `\[([^\]]*)\]\((https?://[^)]+)\)` stops at the first `)` (the
  link's own) and keeps spaces/quotes in the URL. A `[^)\s]+` pattern fails on
  such a link and silently falls back to a bare-URL parse that truncates at the
  quote — the label is lost and the URL is broken. `applyHyperlinks` must NOT
  skip URLs containing `"`: `escapeXML` writes it as `&quot;` in the
  relationship and Word reads it back.
- **A link in the offer comes from the system card AND from its materials'
  «## Ссылки»** (2026-11-09, one level). `enrichLinkedRefs` resolves each
  composition `[[link]]` against the materials index (by `note_title`), reads
  that material note's refs, and stores them in `SystemCard.LinkedRefs`; this
  runs at **cache refresh** (`refreshSystemCards`), never at export. `Refs`
  alone are not enough — the user wanted product documents too. Deduplicate by
  URL across `Refs` + `LinkedRefs` (`cardAllRefs`/`collectOfferRefs`). Old
  caches carry no `LinkedRefs` until «Перечитать из Базы знаний».
- **Every `<w:hyperlink r:id>` in the body MUST have its relationship in
  `document.xml.rels`, or Word refuses to open the file** (2026-11-09, live
  broken КП 003/2026 after 0.9.4). The `{{ссылки}}` marker prints a real
  hyperlink and collects its relation in `layoutRuntime.links`. A marker placed
  **inside a form block** (`repeat:systems`) ran through `renderCtx` →
  `layoutContext()`, which used to build its OWN `layoutRuntime` and drop the
  collected relations — body had the hyperlink, rels did not. Fix: `renderCtx`
  carries the shared `layoutRuntime` pointer and `layoutContext()` reuses it;
  both the block-form path and the combat sheet create one shared runtime and
  merge its links in `buildBodyWithLinks` (single pack point). Regression
  guards: `TestLinksMarkerInsideBlockLayoutDefinesRelations`,
  `TestLinksMarkerInsideBlockFormDefinesRelations` (`docx_links_broken_test.go`)
  assert every body `r:id` is defined. **Whenever a marker prints a link, prove
  the relation reaches the pack — not just that the text appears.**
- **Bare `{{ссылки}}` is the links marker, and OUTSIDE a `repeat:systems` it
  prints ALL selected systems' links** (2026-11-09, live КП 003/2026 with two
  systems). Two defects found together:
  1. `serviceLayoutKindOf` required a system word after the key, so bare
     `{{ссылки}}` was not a service marker at all — it went to the model as a
     prompt and printed nothing. `links` is now recognised by the key alone
     (`TestServiceMarkersScanAsService`).
  2. `renderLayoutCardLinks` used `layoutRowCard` (first system) even without a
     repeat, so only ТН-Фундамент links printed and ТН-Стилобат plus its
     materials vanished. Now `layoutLinksFor` returns `lc.refs`
     (`collectOfferRefs` over all systems, `Refs`+`LinkedRefs`, deduped) when
     there is no per-iteration system; inside a `repeat:systems` it stays the
     row's own system (`TestLinksMarkerOutsideRepeatCoversAllSystems`).
  The `{{ссылки: система}}` spelling behaves the same outside a repeat. A marker
  that silently does nothing is the worst outcome — a bare key must either
  print or be rejected, not be sent to the model.
- **Links in the document are printed by GROUPS, as a NUMBERED list**
  (2026-11-10, user request). Groups and their order: `page` (product page on
  nav.tn.ru, systems before materials) → `sto` → `techlist` (systems before
  materials) → `uzly` → `cert` (certificates/declarations/instructions and
  anything else, **materials before systems**). Each group gets its own
  `<w:numId>` from the numbering plan, so numbering restarts at 1 per group.
  One classification rule lives in `skills/cardrefs.go` (`cardRefGroupOf`,
  `SortCardRefsGrouped`, `GroupCardRefsByGroup`); `CardRef` now carries
  `group` + `owner`, and `TagCardRefsOwner` sets the owner where the card is
  known (`cardAllRefs`: `Refs`→system, `LinkedRefs`→material). Both the `links`
  node (`renderLinkSection`) and the `{{ссылки}}` marker
  (`renderLayoutCardLinks`) sort again before printing, so externally-supplied
  refs are ordered too. **`applyHyperlinks` now locates the numbered item by its
  text and wraps only its run, keeping `<w:pPr>`/`<w:numPr>`** — rebuilding the
  paragraph as a plain `hyperlinkXML` would silently drop the number.
  Old caches without `group`/`owner` are classified on the fly
  (`effectiveGroup`/`effectiveOwner`), so they print in the right groups without
  a KB re-read.
- **Model text is printed as paragraphs, and its typography is normalised**
  (2026-11-10, user request: «вставлялся единым массивом», «пропуски точек,
  двойные пробелы»). `normalizeBodyTypography` (double spaces → one, no space
  before `.,;:!?…`, space after a sentence end/punctuation when a letter
  follows) runs on write (`cleanBlockText`), on read (`generatedTextFor`) and in
  `paragraphsXML` for the `prompt_text` node. The **space-after-dot rule only
  fires before an UPPERCASE letter**, so `nav.tn.ru` and `т.е.` survive; the
  space-before-punctuation rule and the comma/semicolon rule always fire. The
  `prompt_text` node prints multi-paragraph text with `paragraphsXML` (one
  `<w:p>` per non-empty line), not one `<w:p>` — that was the "single massive
  array". The block system prompt (`skills/prompt.go`) also asks for paragraphs
  and correct punctuation. Tests: `docx_text_typography_test.go`.

### Agent engine (2026-11-14)

- **The agent loop lives in `sbe-core/src/agent/engine.ts` — edit it THERE.**
  `offers-bot/src/agent/engine.ts` and the portal's
  `web/sbe-web/src/modules/agent/engineCore.ts` are verbatim copies; the
  Obsidian plugin keeps only an adapter. Copy the file, never hand-patch a copy:
  `node plugins/scripts/check_agent_engine.mjs` compares them by the rule
  constants and the header.
- **Never write U+FF5C (the DSML bar) as a literal in source or fixtures.** It
  does not survive a rewrite through a PowerShell pipeline, esbuild, or some
  editors — it silently becomes something else and a "green" test proves
  nothing. Build it: `String.fromCharCode(0xff5c)`. Both mistakes were made on
  2026-11-14 and both showed up as a test passing on a broken fixture.
- **The DSML tag cannot be parsed with one regex.** Two shapes are live:
  `<…DSML…>tool_calls>` + `<…DSML…>invoke name="x">` (name before `>`), and
  `<…DSML…>invoke name="x">` alone. `dsmlTagEnd` + `blockNameEnd` scan
  character by character for exactly this reason; a quantifier over a class
  containing U+FF5C also stops matching on long inputs.
- **The tool is the boundary, not the prompt** (design §4.4): documents are
  filtered by `product_refs`, letters by the presence of a PDF, in the route.
  Neither internal route takes an `email` parameter.

### Bot and rights

- **A `callback_data` prefix can be COMPOSITE — parse by the LAST colon**
  (2026-11-13, R2). `nd:pick:3` must become `{prefix:'nd:pick', id:3}`. Parsing
  by the first colon gives `Number('pick:3')` = `NaN`, the press is dropped
  silently, and from outside it is «нажал — ничего не произошло» — exactly the
  `cban:all` class (2026-11-11). Guard: `callbacks-data.test.ts`.
- **While the «Создать новый документ» dialog is open, the text is a PRODUCT
  NAME.** `handleNewDocText` runs **before** the combat branch and before
  `handleDraftText`; otherwise «Мембрана» becomes a position with price 0. It
  must return `true` only when the dialog is really open (checked through
  `session.newDoc`), so the rest of routing keeps working.
- **The new dialog keeps its own state map (`RouterState.newDoc`).** Do not
  reuse `foundSuggest`: there the pick is added IMMEDIATELY, here it is
  REMEMBERED and a decision follows. One map for two flows is how
  «нажал — добавилось не туда» happened with the sheet side picker (2026-11-11).
- **Old menu captions must stay as aliases.** `🧾 Собрать КП` and `⚔️ Боевой
  листок` are gone from `MAIN_KEYBOARD` but handled in `routeMenu` (they open the
  product question for the matching kind). Chat history lives for years; a
  removed handler answers with silence, which is the worst outcome.
- **A reply-keyboard button cannot carry `web_app`.** A reply button with
  `web_app` opens the Mini App WITHOUT the Telegram signature and the person sees
  «Нет доступа: войдите в бота» (live defect 2026-11-13). Any menu button that
  must open the App is a plain text button answering with an **inline**
  `web_app` button — `appListButton`/`appUrl` in `flow/appbutton.ts`.
- **`void someFunction;` is a REFERENCE, not a call — and the compiler says
  nothing** (2026-11-12, live defect: after entering the code the bot asked for
  the e-mail again). In `handleSignIn` the line `void applyCodeRequested;` was
  meant to save the session but only named the function, so the step stayed
  `idle` and the six digits were parsed as an e-mail. `tsc --noEmit` is happy
  with it and the whole bot suite was green — the sign-in path had no test.
  Fix: `await deps.save(chatId, applyCodeRequested(session, result.email))`,
  plus two tests over the whole sign-in path (the step becomes `await_code`; the
  code does not re-prompt and reaches `authed`). **Give every `void something;`
  in this codebase the same look** — the correct discard form is `void fn()`.
- **A session step that is not saved does not exist.** `state.sessions` is the
  only place the sign-in step lives; `handleSignIn` must write it on every
  transition. An in-memory `deps.session` returning an object is not persistence.
- **Roles do not exist in the bot**: anyone signed in with a code builds offers.
  The plugin keeps `viewer < editor < admin < superadmin`; compare through
  `isAdminRole`/`canEditOffers`.
- **Sign-in is checked once, in `routes.ts`**, for text and callbacks.
- **The offer is created at the start of the conversation**; both
  `handleBotCreateOffer` and `handleBotPatchOffer` accept `items: []`, the REST
  path does not. Do not "unify" them.
- **`callback_data` is limited to 64 bytes** — buttons carry an index, not a
  Cyrillic code.
- **A saved offer opens for editing** (`edit:<id>` or the number as text); the
  stage jumps to `positions` — do not restore the context stage, that
  repeat-quizzes the manager.
- **A side picker that sends every product as a button hits two walls at once**
  (2026-11-10, live complaint): long product names overflow the screen, and only
  the first handful of cards is reachable. Use the «Документы» pattern — a
  numbered TEXT list plus number buttons, server-side paging (`offset`/`limit`,
  `total`, `hasMore`), toggle-style multi-select keyed by the **number from the
  start of the list** (so it survives paging), and one «✓ Добавить выбранные».
  `combatPickerKeyboard`/`combatPickerMessage` in `flow/combat.ts`; selection
  state lives in `RouterState.combatPicker` (`{side, query, offset, picked,
  allPicked}`); the query change resets `picked`, because indices belong to the
  old match set.
- **"Add every match" must not be faked by marking the visible page**
  (2026-11-10, user request to compare one product against all competitors).
  `allPicked` is a flag, not hundreds of indices; when it is set,
  `onCombatAddPicked` loads the whole match set through `api.combatAllPaged`
  (pages of 50 until `hasMore` is false), and the button reads «Добавить
  выбранные (total)». A manual number tap clears the flag — otherwise «add all»
  would silently override what the manager just deselected. The button only
  appears when `page.total > page.items.length`: with everything already on the
  screen it would do the same as tapping each number.
- **A competitor note starts with TWO H1s** (2026-11-11, live complaint: the
  whole picker read «Материал конкурента, использовать только для критического
  сравнения»). The indexing script writes a disclaimer heading, then the real
  product name. Take the first heading that is NOT the disclaimer
  (`competitorTitles`/`isCompetitorDisclaimer`), never just `H1[0]` and never
  the raw body's first line. The same note is also the reason the journal
  («Журнал материалов конкурентов», tag `journal`) must be filtered out
  (`isCompetitorJournal`): it lives in the folder root and would otherwise be
  the first row of «Все продукты».
- **An empty list in a JSON response must be an ARRAY, never `null`**
  (2026-11-11, live crash «Cannot read properties of null (reading 'length')»
  right after uploading a layout). Go's `encoding/json` writes a nil slice as
  `null`, and any client doing `fields.length` throws. `describeLayout` now
  returns `[]string{}`; the plugin also normalizes the response on the boundary
  (`normalizeLayoutResponse`). Regression test reads the RAW JSON — a decoded
  nil slice and an empty one are indistinguishable.

- **Bot session state does not survive a restart; the sheet must be
  re-derived** (2026-11-11, live complaint «нажал — ничего не произошло»).
  `restoreSession` rebuilds only `{step, email}`, so `session.combat` is gone
  while every combat button is still on screen — and each answered «листок не
  открыт». `ensureCombat` loads the sheet from the session or falls back to the
  chat's last sheet (`api.listCombat`) and refills the session; all sheet-level
  buttons go through it. Any new button that needs `session.combat` must too.

- **A "similar products" picker must decide the CLASS of the product, not
  count shared words** (2026-11-11, three live complaints in a row). Counting
  words over title+description made a PIR board an analogue of «МОСТОФЛЕКС»
  (both mention «применения») and of «ШАЙБА КРОВЕЛЬНАЯ» (both «кровельная»),
  while real analogues (PLASTFOIL) were missing. Rules that now hold:
  the class comes from the **title only** (`analogClassOf`), application words
  («кровельная», «фасадная») never define a class (`analogWeakWords`), and
  «активатор»/«очиститель»/«шайба» are their OWN classes — the order of
  `analogClasses` decides the first match, so specific words go before general
  ones. The best source is the **journal**, which already groups products by
  catalog section; it is chosen by the model, not by words.
- **The competitor journal has two heading levels that look alike.** Both the
  competitor («PLASTFOIL (plastfoil.ru/catalog/)») and the catalog section
  («ПВХ-мембраны — 28 шт.») are `##`. They are distinguished by the **item
  counter** in the heading; a heading without a counter is a competitor. Markdown
  table separators (`| --- | --- |`) parse as a row — filter them out.
- **Model calls need the provider's real latency budget.** `analogLLMTimeout`
  at 20 s silently forced the fallback on every request (`context deadline
  exceeded` in the log) — the model answers in 20-40 s on a long journal. 60 s
  is the working value; `llmDefaultTimeoutMS` is 180 s.

- **A button gated on `picked` disappears exactly when "all found" is chosen**
  (2026-11-11, live complaint «выбрал все — конкуренты не добавились»). The
  "all found" mode is a flag (`allPicked`) and it CLEARS `picked`, so any
  control conditioned on `picked.size > 0` vanishes at that moment: the user
  could select everything and had no way to add it. Condition such controls on
  `allPicked || picked.size > 0` and count `allPicked ? total : picked.size`.
  Regression test: «в режиме „всё найдено“ кнопка „Добавить“ остаётся, даже
  если номера не отмечены».

- **A Telegram button's keyboard is per message, and returning to a picker page
  must rebuild the marks from state, not from the server response.** 0.9.6
  built an empty picker on «▼ Все продукты», so a mark made a second earlier
  vanished and «Добавить выбранные» answered «Ничего не отмечено»; the page
  also shipped without `cb:oursys`/`cb:comp`/`cb:doc`/`cb:done`, so after
  paging the user was stuck. `pick` state lives in `RouterState.combatPicker`
  only — the server knows nothing about it — and every picker page is wrapped in
  `withCombatMenu`.

- **Competitor cards are found by tag `competitor` first, then by folder**
  (2026-11-10). The body of a competitor note is arbitrary — the card title must
  come from the note heading (`competitorCardFromParsed`), never the parsed
  body's first line, or the list shows sentences instead of product names. The
  folder source stays for notes added before the tag existed; both are merged by
  note id so a note in both places appears once.

### Files, parsing and context

- **The parsing layer is fed by real manager files.** `skills/testdata/legacy-offer.xlsx`
  is an anonymised real file. Every fix has a test named after the symptom.
- **Address cells by `r` (`B35`), never by position** — skipped cells are absent
  from the XML and the row shifts left.
- **«Сметная стоимость» is a sum, «Стоимость единицы» is a price.**
- **A dot is not a sentence end for object names** («по ул. Охотская»,
  «г. Новосибирска»).
- **A file's item names are not object context** (`contextFromParsed(isSpec=true)`
  returns nil).
- **Model extraction never overrides the literal one** (`MergeFileContexts`).
- **`jsonrepair.go` lives in `model`**, shared with `skills`; do not copy it.

## Agent in the bot — P6 (2026-11-14, **released in 0.9.22**)

The agent is the **document assembler** (design
`docs/superpowers/specs/2026-11-13-sbe-offers-single-agent-design.md`, §4): it edits
the same `offers_offer`/`offers_combat_sheet` rows the App and the bot edit, and every
tool wraps an existing service function. This step is server-side plus the bot; **the
plugin (0.9.21) and the release are untouched** — nothing was deployed.

**Where it lives (all `offers-service/`, server-side):**

| file | what |
|---|---|
| `agenttools.go` | tool schema, `objectSchema`/`argString`/`argInt`/`argFloat`/`argBool`, `agentEnv` (email, chat, files, `Fixes`, per-turn `visibleDocs` cache) |
| `agenttools_impl.go` | 13 of the tools; each wraps a real function (`suggestionsFor`, `readNoteCard`, `searchCompetitors`, `assembleSystem`, `assembleMaterial`, `createCombat`, `combatSide`, `handleExportDocx`, `handleExportCombat`) |
| `agentloop.go` | the turn: model → `parseAgentTurn` → tool → observe → final; anti-loop, protocol retry, synthesis, journal write |
| `agentprotocol.go` | JSON-turn parser (`final`/`tool_call`), fence/prose/truncation tolerant |
| `agentprompt.go` | system prompt (sets the JSON shape + “the tool is the boundary”) and transcript |
| `agentlog.go` | `offers_agent_log` write + 180-day trim + `GET /admin/agent-log` |
| `agentapi.go` | bot transport `POST /bot/agent/message`, `GET /bot/agent/history` |
| `documents_client.go` + `document_text.go` | internal `/internal/documents/agent/*` client (P5a) + DOCX text extraction |
| `mailer_client.go` | internal `/internal/mailer/agent/*` client (P5b): `MAILER_SERVICE_URL`, `MAILER_INTERNAL_SECRET`, refusal when not configured |
| `agentmail.go` | the two email tools (P5b): `get_emails` intersects the mailer with the visible documents set (per-turn cache in `agentEnv`), `read_email` resolves each scan's `file_key` through that same set; `foldText`, `visibleScanCards`, `docsMatchProduct`, `emailView` live here |
| `offers-bot/src/flow/agent.ts` + `api.agentMessage` | bot side: «🤖 Помощник» button, «⌛» → answer, files to chat |

**Tools** (whitelist, `agentTools`; after P5b 2026-11-14 — 15):
`get_our_products`, `read_card` (**any** note), `search_competitors`,
`get_documents`, `get_document_link`, `read_document_file`, `get_emails`,
`read_email`, `create_offer`,
`set_system`, `set_material`, `create_combat_sheet`, `set_combat_side`,
`build_docx`, `build_combat_docx`. **Not** in the list: YouGile,
rules/skills/presentation, any write to YouGile. Emails are read-only and only
for letters whose signed-scan card carries `product_refs` (`agentmail.go`).

**Activation is opt-in, S5 stays gated.** The bot gets a «🤖 Помощник» menu button
(`/agent` alias) that sets `session.agent`. While it is on, free text goes to the
agent (`handleAgentText` runs **before** `handleNewDocText`/`handleDraftText`, so a
product name never becomes a ₽0 line). «🏠 В меню» (`agent:off`) turns it off. No
existing flow was removed.

**Rules that now hold (each was a decision, not a default):**
- **The tool is the boundary, not the prompt.** `get_documents` calls only the
  filtered `/internal/documents/agent/*` route (documents with `product_refs`, not
  archived, not `no_agent`); there is no unfiltered path in offers-service. An empty
  documents client (no `DOCUMENTS_SERVICE_URL`/`DOCUMENTS_INTERNAL_SECRET`) is a
  **refusal**, never an empty list.
- **Rendering reuses the export handler.** `build_docx`/`build_combat_docx` build an
  `*http.Request` around the turn's context and call `handleExportDocx`/
  `handleExportCombat` via `httptest`, so the document is printed by the ONE renderer,
  identity included (`permEmailCtx`). Do not add a second render path for the agent.
- **Writes respect ownership.** `offerForWrite` = own row or admin, and a `final`
  offer is refused — same rule as `handleSaveOffer`. Every write records before/after
  into `env.Fixes` → `offers_agent_log.manual_fix`.
- **PDF is not read as text in v1** (no PDF lib; a new dependency is a licence
  decision). `document_text.go` extracts DOCX (zip + XML) and plain text; a PDF/scan
  yields `""` and the agent uses `get_document_link`. Do not return base64 to the model.

**Checks (all green 2026-11-14):** `gofmt`/`go vet`/`go test ./...` ok; bot `tsc` 0
and **201 passing** (8 new in `agent.test.ts`); plugin `tsc` 0, `build` OK (no source
change); `check_offers_openapi.mjs` **117/117**; `check_bundle_strings` 0;
`check_marker_registry` OK; stand **472/0** (new section «Помощник (агент) в боте
(P6)», 11 checks — the stub model drives a real turn: tool → answer → journal).

**Server config — deployed 2026-11-14.** In the `offers` compose block:
`DOCUMENTS_SERVICE_URL: http://documents:3000` and
`DOCUMENTS_INTERNAL_SECRET: ${DOCUMENTS_INTERNAL_SECRET}` (same secret as
documents’ P5a routes; `depends_on: documents`). Inserted pointwise on the server
(`docker-compose.yml.bak-20261114-agent`), `docker compose config -q` clean,
secrets verified equal in both containers, and a live `agent/list` call from the
`offers` container returned 200 with real documents (403 without the header).

**Not verified:** a live turn under a real model and bot token (the stand uses
`llm-stub`, but the deployed route, auth gate, documents path and journal table
were all exercised live); week-long journal analysis is future work
(`GET /api/offers/admin/agent-log`).

### P6-bis — agent reads ANY note + competitor tools (RELEASED in 0.9.23, 2026-11-14)

**Bot menu (same batch, user request 2026-11-14):** «✨ Создать новый документ»,
«🤖 Помощник» and «❓ Справка» now stand in **one row** (`MAIN_KEYBOARD` in
`flow/build.ts`) — three equally-weighted entries, not three one-button rows.
Guards: `appbutton.test.ts` «… стоят в ОДНОМ ряду» and the stand section that
counts **buttons, not rows** (a row-level count broke on the compaction).

Live defect (0.9.22): the bot said material cards (noteId 7547/6256/6260/7701/
7703/7705) "do not open in the KB". **False** — they exist and read fine
(verified live with the service token). Cause: `read_card` → `liveSystemCard` →
`SystemsFromKB` (`systemcards.go:315`), which keeps only `commercial_propose`
notes WITHOUT `material` and drops materials on purpose. **Reading a card must not
depend on whether it is a system or a material** — that was a P6 model error.

**Agreed model (user, 2026-11-14): one axis — a KB card is ANY `noteId`.**
`read_card` reads any note (`KBClient.Note` + `ParseSystemCard`), no tag filter.
The tag decides the ACTION, checked inside the tool (boundary), never in the
prompt. `cardRole` (`agenttools_impl.go`) maps tags to `system` / `material` /
`competitor` / `other`, and the answer carries the role so the model picks the
right tool. **`competitor` wins over every other tag** — a competitor note
sometimes also carries `commercial_propose`, and it must never land on our side
of a battle sheet. A bare `material` tag is a material.

**What was done (all `offers-service/` plus the stand; plugin untouched):**
- `read_card` rewritten on `readNoteCard` (any note, image key attached, role in
  the answer). Closes the LOGICROOF/ГЕОПРУФ complaint.
- `search_competitors` (wraps `searchCompetitors`), `create_offer` (the same
  INSERT as `handleBotCreateOffer` — the agent no longer dead-ends on
  "предложение не найдено" without an id), `create_combat_sheet` (wraps
  `createCombat`), `set_combat_side` (wraps `combatSide`). `combatSideRoleError`
  refuses a material/system on the competitor side and a competitor on ours;
  ownership is checked like `combatSheetForEditor`. Side writes go to
  `offers_agent_log.manual_fix`.
- **Defect the stand found and we fixed:** `build_docx`/`build_combat_docx`
  never worked for a real document — `requestWith` did not set path values, so
  `handleExportDocx` read an empty `{id}` and answered `invalid id`. Now
  `requestWith` takes `pathValues` and calls `SetPathValue`; guard
  `TestAgentRequestCarriesPathValue`.
- The prompt teaches the tag roles and the two document paths (КП / battle
  sheet, and that they can be combined).
- Tests: `agent_test.go` — `TestAgentToolsAfterP6bis`, `TestCardRoleByTags`,
  `TestCombatSideRoleBoundary`, `TestArgFloatAndBool`,
  `TestAgentRequestCarriesPathValue`. Stand section «Помощник (агент): карточки
  любого вида, конкуренты, листок, КП (P6-бис)» — real turns through the stub
  model. **491/0**, repeat runs green.
- Checks: `gofmt`/`go vet`/`go test ./...` green; bot `tsc` 0 / **202**; App `tsc`
  0 / 51; plugin `tsc` 0, `build` OK, `check_bundle_strings` 0,
  `check_marker_registry` OK, `check_docs_state` OK, OpenAPI 117/117,
  `check_agent_engine` green.
- **Released as 0.9.23** (`backend` → `main`), deployed, self-hosted and in the
  registry. The deploy run's gotchas (PowerShell heredoc, UTF-16LE redirect,
  `kb-selfhost.sh` writing hashes to the DB) — `HANDOFF.md`, «Как выкладывать
  выпуск».

**P5b — RELEASED in 0.9.26:** emails (`get_emails` / `read_email`). See the task
section right below.

## ЗАДАЧА ДЛЯ АГЕНТА (P5b, 2026-11-14): письма в контексте КП

> Статус: **ВЫПУЩЕНА 0.9.26 (2026-11-14).** Сторона писем раскатана раньше
> (sbe-mailer 0.1.21, ручки `GET /internal/mailer/agent/emails?q=&limit=` и
> `GET /internal/mailer/agent/email?id=` под `MAILER_INTERNAL_SECRET`). Сторона
> КП сделана в этой папке: `mailer_client.go`, `agentmail.go` (оба инструмента),
> 15 имён в `agentTools()`, правило в `agentSystemPrompt`, стенд
> (`mailer-stub.mjs` на ЗАПИСАННОЙ фикстуре, `documents-stub.mjs`, секция
> `sectionAgentP5b`), тесты `mailer_client_test.go` и `agent_test.go`
> (`TestAgentToolListAfterP5b`, `TestGetEmailsDropsUnlinked`,
> `TestGetEmailsEmptyVisibleSet`).
>
> Проверки: `gofmt`/`go vet`/`go test ./...` зелёные, бот `tsc` 0 и **208**
> тестов, плагин `tsc` 0, `check_bundle_strings` 0, `check_marker_registry` OK,
> `check_docs_state` OK, OpenAPI **117/117**, `check_agent_engine` зелёный,
> стенд **505/0**.
>
> **Что осталось человеку:** (1) поставить отметки «🏷️ Системы и материалы»
> на карточках сканов (на бою на момент выпуска `visible=0` из 200 писем со
> сканами — это модель, а не дефект); (2) после этого — живой ход помощника
> «найди письма по <продукт>» и «прочитай письмо». Compose правки не
> потребовал: `MAILER_SERVICE_URL`/`MAILER_INTERNAL_SECRET` в блоке `offers`
> уже стояли с раскатки писем.
>
> **Про выпуск 0.9.26:** `backend` `21af387` → `main` `8e93230`; сервис и бот
> пересобраны и пересозданы (`*.bak-20261113-1249`), health `ok`, поллер
> запущен; самораздача `08c452e4…` / `f11a8e47…` / `a32ac3df…`, реестр
> `36bc504`. Живая сверка: ручка писем с секретом — `200`, без — `403`,
> `file_key` в ответе нет; набор документов P5a — `200`.
>
> Контракт ниже — заморожен и подтверждён живым ответом; отступать от него
> нельзя:

### Модель (решение пользователя 2026-11-14)

Письмо видно агенту **только если у карточки его скана в «Документах» есть
`product_refs`** — та же граница, что уже раскатана для документов
(`documents-service/agentdocs.go:24,169-173,222-238`). Ни «есть PDF», ни
`doc_type`, ни совпадение названия не используются. Пересечение двух
сервисов делается **здесь** (решение пользователя, вариант «б»): мейлер и
документы друг о друге не знают.

### Как сделано (2026-11-14)

1. **`offers-service/mailer_client.go`** — клиент по образцу
   `documents_client.go`: `MAILER_SERVICE_URL` (default `http://backend:3000`),
   `MAILER_INTERNAL_SECRET`, таймаут 60 с, лимит тела 64 МиБ.
   `configured() == false` → **отказ** (`errMailerNotConfigured`), не пустой
   список. `Emails(ctx, query, limit)` и `Letter(ctx, id)`; `items:null` →
   `[]`, `attachments:null` → `[]`.

2. **`get_emails`** — `offers-service/agentmail.go`:
   - `letters := mailer.Emails(q, limit)` — все, у кого есть карточка скана;
   - `visible` — `NewDocumentsAgentClient().List("", 200)`, кэш **на ход** в
     `agentEnv` (`visibleDocs`/`visibleDocsReady`); ошибка не кэшируется;
   - видимость письма — `kind == "signed"` и `document_id > 0` ХОТЯ БЫ одного
     вложения есть в `visible`; к письму прикладываются **названия привязок**
     (`products: [{noteId,title,sku?}]`, дедуп по noteId, порядок по noteId) и
     карточки сканов с `file_key` видимой карточки;
   - пусто → «Писем, относящихся к продуктам, не нашлось.» — **никогда** не
     «писем нет»;
   - аргументы: `query`, `limit`, `product` (noteId числом или слово из
     названия привязки; слово сравнивается через `foldText` — без регистра и
     «ё»/«е»).

3. **`read_email`** — полный текст письма (`mailer.Letter(id)`), сканы с
   `file_key` по `document_id` из того же `visible`; письмо без видимой
   карточки скана → отказ «письмо недоступно: у него нет карточки скана,
   привязанной к продукту». PDF не парсится: текст скана открывается
   существующими `get_document_link` / `read_document_file`. Нового инструмента
   для PDF не заведено.

4. **Регистрация и промт.** `agentTools()` — **15** имён (`get_emails`,
   `read_email` после документов), они же в `agentToolDescriptors`; в
   `agentSystemPrompt` под «Что важно помнить» — правило писем и прямой запрет
   искать обход.

5. **Конфиг и стенд.** Стенд: `docker-compose.local.yml` — `mailer-stub` и
   `documents-stub`, у `offers` переменные `MAILER_SERVICE_URL`/
   `MAILER_INTERNAL_SECRET` (`test-mailer-internal-secret`) и
   `DOCUMENTS_SERVICE_URL`/`DOCUMENTS_INTERNAL_SECRET`
   (`test-documents-internal-secret`). `scripts/mailer-stub.mjs` отдаёт
   **записанную** фикстуру `scripts/mailer-fixture.json` + одно письмо со
   сканом, которого нет в видимом наборе (проверка, что фильтр не протекает);
   `scripts/documents-stub.mjs` — одна привязанная карточка (`document_id`
   1788168804943 → noteId 7547 «LOGICROOF V-RP»). Секция стенда
   `sectionAgentP5b` — «письма видны только через привязку к продукту (P5b)».

### Осталось для выпуска

- На сервере в блоке `offers` — `MAILER_SERVICE_URL: http://backend:3000`,
  `MAILER_INTERNAL_SECRET: ${MAILER_INTERNAL_SECRET}` (точечно, бэкап,
  `docker compose config -q`; `DOCUMENTS_*` там уже есть с P6).
- Живой ход под настоящей моделью и ботом.
- На бою: «поставил отметку на карточке скана — письмо видно на следующем
  ходе агента».

### Исходное ТЗ (как было поставлено)

1. **`offers-service/mailer_client.go`** — клиент по образцу
   `documents_client.go` (та же форма, те же причины): `MAILER_SERVICE_URL`
   (имя контейнера мейлера сверить с боевым compose — `backend`),
   `MAILER_INTERNAL_SECRET`, таймаут 60 с, лимит тела 64 МиБ.
   `configured() == false` → **отказ**, не пустой список.
   `Emails(ctx, query, limit)` и `Letter(ctx, id)`.

2. **`get_emails`** — новый инструмент в `agenttools_impl.go` рядом с
   документами:
   - `letters := mailer.Emails(q, limit)` — все, у кого есть карточка скана;
   - `visible := NewDocumentsAgentClient().List("", 0)` — набор P5a;
   - оставить письмо, если `document_id` хотя бы одного его вложения есть в
     `visible`; к каждому оставшемуся приложить **названия привязок** его
     карточки (модель должна понимать, к какому продукту относится письмо);
   - пусто → «Писем, относящихся к продуктам, не нашлось.» — **никогда** не
     «писем нет»;
   - `visible` кэшировать **на ход** (в `agentEnv`), не на сервер: привязку
     могли поставить секунду назад;
   - аргументы: `query`, `limit`, опционально `product` (id заметки или слово)
     — агент обычно знает, о каком продукте идёт речь.
   - описание инструмента (видит модель) — про границу: письма видны только
     те, чей скан привязан к системе или материалу; пустой результат не повод
     искать обход.

3. **`read_email`** — полный текст письма (`mailer.Letter(id)`); в результате
   ещё список сканов и для каждого **`file_key`**, разрешённый по
   `document_id` из того же `visible` набора (отдельной ручки для этого не
   нужно). Текст PDF в v1 пустой — отвечать честно, как
   `toolReadDocumentFile` («нет текстового слоя — откройте сканом»), сканы
   открывать **существующими** `get_document_link` / `read_document_file`.
   Нового инструмента для PDF не заводить.

4. **Регистрация и промт.** В `agentTools()` (15 имён после P6-бис + 2),
   в упорядоченном списке `agentToolDescriptors`; заменить комментарий «письма
   — P5b» на правило как оно есть; одна строка в `agentSystemPrompt` под «Что
   важно помнить».

5. **Конфиг и стенд.** В `offers` блок серверного compose: `MAILER_SERVICE_URL`,
   `MAILER_INTERNAL_SECRET` (то же значение, что у мейлера), `depends_on:
   backend` — точечно, с бэкапом, `docker compose config -q`. На локальном
   стенде — `mailer-stub.mjs` в `scripts/` по замороженному контракту, пока не
   придёт записанная фикстура.

### НЕ делать

- Не менять `sbe-mailer/`, `sbe-documents/`, `sbe-core/`.
- Не вводить фильтр по PDF/`doc_type`/названию ни в одном месте.
- Не отдавать модели base64 и не парсить PDF.
- Не поднимать версию и не деплоить без явной команды.

### Чек-лист сдачи

```
cd offers-service && gofmt -l . && go vet ./... && go test ./...
cd .. && node ../scripts/check_offers_openapi.mjs   # 117/117, новые ручки — /internal
cd ../offers-bot && npm run typecheck && node --test "src/**/*.test.ts"
cd .. && npx tsc --noEmit && npm run build && node scripts/check_bundle_strings.mjs
node ../scripts/check_agent_engine.mjs
# стенд: секция «письма видны только через привязку к продукту (P5b)»
```

- тесты: `mailer_client_test.go` (заголовок секрета, `configured()` без env,
  текст 403, `items:null` → `[]`); `agent_test.go` — `TestAgentToolListAfterP5b`
  (15 имён) и `TestGetEmailsDropsUnlinked` (стаб мейлера: две записи, стаб
  документов: одна видимая карточка → остаётся одно письмо, а пустой
  результат — текст про «нет привязанных писем»);
- сообщить: файлы, коды выхода и числа, что не проверено, что ждёт от
  мейлер-агента (фикстура), нужные строки compose.

## Next iterations (backlog)

Ordered by what will bite the commercial department soonest.

1. **KB analogue comparison** (why these systems, not the alternatives) — new
   functionality, not started. `compare_table`/`compareSystemRows` compare the
   CHOSEN systems with each other, which is not what is wanted.
2. **Word documents as a context source.** Old offers arrive as `.docx` more
   often than as Excel; today `readAttachedFile` refuses them. Mostly a
   `word/document.xml` text extraction (same zip walk as xlsx).
3. **Old binary `.xls`** — refused today; cheapest step is a better message plus
   conversion instructions.
4. **`.docx`/`.xls` in the plugin's own upload screen** (`sheet-upload.ts`) —
   not touched since, may share the old positional-cell assumptions.
5. **Multiple sheets** — `ParseXlsxAttachment` stops at the first sheet with a
   composition.
6. **Grouped/hidden rows in xlsx** — `outlineLevel` and `hidden="1"` are ignored.
7. **Manual overrides vs the file** — a file always wins over what the manager
   typed; offer to mark fields as "taken from file".
8. **Editing a saved offer** has no per-item delete; add handlers in
   `flow/resume.ts`, not `callbacks.ts`.
9. **`handleDraftText` is still CC 13** (`callbacks.ts`) — dispatcher mixing
   brief, `система`, free item, quantity and form field. Take it apart with a
   handler table.
10. **`contextWithModel`/`Complete` have no unit coverage** — network glue
    checked only on the stand.
11. **Data work, not code:** `SKU` for materials synced from KB is meaningless
    (`MAT-ПРОЧ-ОБЩ-NNNN`); ~1273 composition lines still have no `[[link]]`.
12. **`{{система}}` inside a `repeat: "materials"` block** is not supported; the
    material repeat prints the chosen material's title (2026-11-13), and positions
    only for offers that have no chosen materials.
13. **A prompt table still costs one model call** in some layout paths —
    `layoutTableFor` builds tables from data, verify no call happens first.
14. **`sbe-kb/src/services/hash.ts`** is a second copy of
    `sbe-core/src/utils/files.ts`.