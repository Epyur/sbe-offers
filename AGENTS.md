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

Current version = **0.9.20** — **RELEASED 2026-11-13.** Three points converge:
`main` `9144c17`, `backend` `584d733`, self-hosted `1f8a0276…` / `ea877e47…` /
`a32ac3df…` (verified over https **and** in `/registry.json`), registry git copy
`sbe-apstore-registry` `db6465b`. The bot was rebuilt and recreated on the
server. `offers-service` did not change in this batch.

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
(R1, R2 done; R3 = materials as offer objects, R4 = the bot agent).

**The App is deployed separately from the plugin** and has been updated after
the release four times: `assets/index-g0XLDXK5.js` (sha256 `01ab599b…`) plus
`vendor/telegram-web-app.js` (`baa788a6…`, served from our own domain).
The service and the bot were last rebuilt and recreated on 2026-11-13 after the
`db error` fix and the removal of the menu button.

Per-plugin state, live hashes and the release procedure live in
[HANDOFF.md](HANDOFF.md) — it is the entry point for a cold start.

**The Mini App migration is done through S7 (S0–S4, S6, S7); S5 is
deliberately NOT done** — the user decided on 2026-11-13 that the bot keeps
all its flows until the App has been verified in real use. Nothing was deleted
from `offers-bot`. Do not start S5 without an explicit instruction.

**Live defect found on the first real run (2026-11-13): the App showed
«initData: нет».** Root cause was NOT the sign-in code but the page:
`<script src="https://telegram.org/js/telegram-web-app.js">` — and the VDS in
Russia cannot reach `telegram.org` at all, so the script never loaded, so
`window.Telegram.WebApp` did not exist and `initData` was empty. The App then
honestly rendered «Открыто вне Telegram». The service side was fine all along
(`/app/session` → 400 empty body, 401 bad signature, bot token present,
10 chat links).

**Rule that follows: the App has no external dependencies.** The Telegram script
is served from our own domain (`offers-app/public/vendor/telegram-web-app.js`
→ `dist/vendor/`, referenced as `./vendor/telegram-web-app.js`). Any `<script>`/
font/CDN added to `index.html` will silently fail on this host. Proven by
`scripts/check_app_browser.mjs <baseUrl>` (15 checks: no external scripts, the
Telegram script comes from our domain and really is it, the bundle matches the
built one, `/app/config` answers, a bad signature is 401).

**List search, two live defects fixed 2026-11-13** (both showed up as «поиск
ничего не нашёл» while matches existed):

1. **Order first: search, then narrow.** The SQL used to take the newest
   `listLimit` (200) rows and filter those, so matches living deeper never
   appeared — on the stand «Ромаш» occurs in 19 rows, all beyond the newest two
   hundred. Search now scans `listScanLimit` (2000) and returns the first
   `listLimit`; a plain list (no `q`) still uses 200. Guard:
   `TestSearchScansMoreThanItReturns`.
2. **Case: normalise once, in the parser.** `parseListQuery` used to lowercase
   only the SQL pattern, while the in-memory check compared the raw query —
   and `lower()` in Postgres depends on the locale while `lowerASCIIFree` does
   not: «ромаш» returned 143 rows, «Ромаш» returned none. The query is now
   lowercased in `parseListQuery` (`lowerASCIIFree`), so SQL and memory agree.
   Guards: `TestParseListQueryNormalises`, `TestContainsPatternIsSubstringTie`.

Live proof on the production base: `scripts/check_list_search_live.mjs`
(run through `node:22-alpine`, there is no node on the host) — 7/0, including a
capitalised query, lowercase, and a substring from the middle of a word. The
temporary offer `999/2026` used for the check was deleted afterwards.
Local Docker stand: **430/0**.

**Two more live defects, fixed 2026-11-13.**

1. **`db error` on the offer list for everyone except the owner.** For a
   non-admin the visibility filter was appended as ` AND lower(owner_email) = …`
   even when no `WHERE` had been added (no search), producing
   `SELECT … FROM offers_offer AND …` — `syntax error at or near "AND"`. The
   owner never saw it (role `superadmin` skips the branch), which is why the
   report came from `fireservice@tn.ru`. Fix: add `WHERE` when there is no
   search, `AND` otherwise. Guard: `lists_sql_test.go` builds the same SQL in
   three modes and asserts the shape (`AND` must never follow the table name).
   Live check: viewer → 200 with 4 own offers, owner → 200 with 20.
   **Lesson: any query builder that concatenates optional conditions must be
   covered by a test on the STRING, not only on behaviour — the standalone
   branch (no search, no admin) is exactly the one the tests never hit.**
2. **The App button is gone from the bot's main menu** (user decision).
   A reply keyboard with `web_app` opened the App WITHOUT a Telegram signature,
   so the person saw «Нет доступа: войдите в бота». The App opens from the
   offer/sheet lists, where buttons are inline and the signature arrives.
   `OPEN_APP_BUTTON` was removed; a test now asserts `MAIN_KEYBOARD` contains
   no Mini App buttons.

**Not verified by a human yet:** the whole live Telegram path (sign-in, lists,
offer card, section texts, combat sheet, «Отправить в чат») and the local
Docker stand (new sections 5b–5e in `scripts/offers_local_e2e.mjs` were written
but not run — Docker was not started). Run the stand before the next batch.

**Mini App architecture (2026-11-13).** `offers-app/` (React + Vite + TS) is
served by Caddy at `/app-offers/` and talks to the same `/api/offers/*` routes
as the Obsidian plugin, as a JWT client; sign-in is Telegram `initData`
(`offers-service/appauth.go`) exchanged for the normal JWT, and roles come from
the central map (common access `editor`). **The bot keeps identity, entry, the
brief, lists, search and file upload; the App owns the document.** Plan:
`docs/superpowers/plans/2026-11-13-sbe-offers-webapp-{START-HERE,migration-plan}.md`.
**S2 — lists and search (2026-11-13, `backend` only, not deployed).**
`offers-service/lists.go` holds the one query normaliser (`parseListQuery`:
collapsed spaces, `ё`→`е`, `%`/`_` escaped), the two indexable patterns
(`LIKE 'q%'`, `LIKE '% q%'`) and the in-memory substring pass (`containsTie`).
`GET /offers?q=` filters by **client name**, `GET /combat?q=` by the sheet
title **and both sides' product titles**; an empty `q` keeps the old behaviour
(`LIMIT 200`, newest first) and a regression test pins it. Indexes
`offers_offer_client_name` / `offers_combat_sheet_title` (`text_pattern_ops`)
are in `migrate`. The App side is `offers-app/src/lists.ts` (types, paths,
labels — the pure half) plus the list screen in `screens.tsx` (row cards,
300 ms debounced search, loading/error/empty/found states).

**The search model is "indexable prefix first, substring in memory second"**
— and that is deliberate, not an oversight. `ILIKE '%q%'` cannot use an index
in any locale, and a client name almost never *starts* with the query
(«Строй» in «СтройМонтажСервис»): the SQL condition only narrows the row set,
the real match is `containsTie` over at most `listLimit` (200) fetches.
When the tables grow, the next step is `pg_trgm` chosen on purpose — do not
"fix" this by turning the pattern into `ILIKE '%q%'` without an index, which
is a plain sequential scan on every keystroke.

**S3 — the offer card in the App (2026-11-13, `backend` only, not deployed).**
The user chose **"variant V": every field is saved by its own route**, so there
is no "save the whole document" path in the App at all.

- `PUT /offers/{id}/client` (name + contact) and `PUT /offers/{id}/items`
  (the list is REPLACED; `sum`/`total` are computed by the server and the
  client-sent values are never read) live in `offers-service/offerfields.go`.
  Systems keep using the existing `POST/DELETE /offers/{id}/system`.
- Both routes: `requirePerm("editor")` + `offerForEditor` (own or admin),
  an empty edit is rejected, every write moves `updated_at`, the answer is the
  same `Offer` as `GET /offers/{id}`.
- **An empty item list is legal here and illegal in the full `PUT /offers/{id}`**
  — that difference is deliberate: positions are entered one at a time, while
  the plugin/bot path still refuses an offer without positions. Do not
  "unify" the two validators.
- App side: `offers-app/src/offer.ts` (model plus the arithmetic: comma
  decimals, per-row and total rounding, pre-send checks, dirty comparison) and
  `offers-app/src/offer-screen.tsx` (three independently saved zones, system
  search with a 300 ms debounce, item list). Checks: `go test` green,
  OpenAPI 98/98, App `tsc` 0, App tests 30, bundle 76.7 KiB gzip.

**S4 — section texts in the App (2026-11-13, `backend` only, not deployed).**
`offers-service/copysections.go` is the JWT twin of the bot's
`POST /bot/offers/{id}/copy/{key}`: same slots (`SectionSlots`), same storage
(`form_values["copy.<key>"]`) and the same three actions
(`WriteSectionChecked` / `EditSection` / `AcceptSection`) — only the transport
and the gate are new (`GET /offers/{id}/sections`,
`POST /offers/{id}/sections/{kind}`, `POST …/edit`, `PUT …/{kind}`).
**The model being unavailable is not an error**: the text comes back empty and
the document is built without that section, exactly like in the bot.
An unknown section key is 400; for `kind=system` the `systemIndex` query
parameter is required and out of range is 400; for fixed slots a stray
`systemIndex` is ignored (otherwise it would land in the key and the text would
silently stop being found). App side: `offers-app/src/sections.ts` (key parsing
plus the pure helpers) and `sections-screen.tsx` (one card per slot, edits stay
a draft until «Принять»). Checks: `go test` green, OpenAPI 102/102, App `tsc` 0,
App tests 35, bundle 77.8 KiB gzip.

**S6 — the combat sheet in the App (2026-11-13, `backend` only, not deployed).**
`offers-service/combatsections.go` adds JWT twins of the bot routes:
`GET /combat/our`, `GET /combat/competitors-src`, `GET /combat/analogues`,
`POST/DELETE /combat/{id}/our|competitor`, `PUT /combat/{id}/hint`. The logic is
not copied (`combatOurItems`, `searchCompetitors`, `findAnaloguesForSheet`);
only transport and gate are new. Reading and printing stay open to every
signed-in user, editing stays with the author or admin (`combatSheetForEditor`)
— rule 2026-11-12.
- **`competitors-src`, not `combat/competitors`**: `GET /competitors` already
  serves the plugin's cached list, and two different payloads under one path is
  a trap for the next debugging session.
- Sides are edited **positionally** (`POST/DELETE …/our|competitor`), never by
  `PUT /combat/{id}`, so a parallel edit in the bot is not overwritten.
- App: `offers-app/src/combat.ts` (model, `canEdit`, `sheetLabelOf`,
  `nextOffset`) and `combat-screen.tsx`; a foreign sheet opens read-only —
  the edit controls are not rendered at all rather than returning 403.
  Checks: `go test` green, OpenAPI 110/110, YAML parses, App `tsc` 0,
  App tests 40, bundle 79.1 KiB gzip.
- **`openapi.yaml` had a literal BEL byte inside the word «advantages»** (three
  places) and a description line whose `:` broke the YAML. The route-vs-schema
  checker never looks inside descriptions, so it stayed green; only
  `yaml.safe_load` caught it. When editing that file, validate it as YAML.

**User decision 2026-11-13: S5 (deleting the bot flows) happens only AFTER the
App has been verified in real use. Nothing is deleted from the bot right now.**

**S7 — «Отправить в чат» (2026-11-13, `backend` only, not deployed).**
The App cannot attach a file itself (no Bot API), so it sends a POINTER with
`tg.sendData` and the bot builds the document with the existing
`/bot/offers/{id}/docx` and `/bot/combat/{id}/export` — the same routes the
«⬇ DOCX» button uses, so the chat's rights are checked exactly as before.

- `offers-bot/src/webapp.ts`: `parseWebAppData` accepts only
  `{action:'docx', kind:'offer'|'combat', id>0}`; anything else is `null` and
  the person gets an explanation, not silence.
- **`chat_id` comes from the update, never from the payload** — the pointer
  cannot post a document into someone else's chat. A `chat_id` inside the
  payload is ignored (covered by a test).
- `poller.ts` handles `message.web_app_data` before documents and text.
- App: `offers-app/src/sendchat.ts` (payload builder) and
  `sendchat-button.tsx` (button exists only inside Telegram).
- Checks: bot 176 tests, bot `tsc` 0, App 44 tests, App `tsc` 0, bundle
  79.4 KiB gzip. **Not verified by a human in Telegram yet.**

**Next step is S8 (release)** — only on an explicit command.

**0.9.17 (2026-11-12, plugin + service + bot + news).** Three live complaints,
one root cause: picking a system or a material went through a dropdown or a
substring filter instead of the Knowledge Base's own full-text search — and the
bot never looked at all.

1. **The plugin has no dropdowns for the catalogue any more.** `src/ui/picker.ts`
   is the picker taken out of the combat view (one implementation instead of
   two). In the offer editor a system is found by name and added the same way as
   before (`addSystem`, texts written once); a material is found in the KB and
   lands in the table as a position with its own unit, code, ЕКН and the «от»
   price. The old `listCatalog` dropdown is gone — `offers_catalog` has not been
   filled since 2026-10-31, so new materials were never reachable there.
2. **New search endpoints** (`pickersearch.go`), the same `KBClient.Search` the
   competitor picker uses: `GET /systems/search` (`commercial_propose` without
   `material`), `GET /materials/search` (tag `material`), `GET /suggest` (both at
   once, for the bot) and `GET /bot/suggest` (same, by service secret). A KB
   failure answers 200 with `error`, and an empty list is always `[]`, never
   `null`.
3. **The bot understands a human phrase** (`flow/suggest.ts` +
   `offerSuggestions` in `callbacks.ts`). Any text not claimed by copy, the
   brief, `система …`, a stage branch or a manual line goes to the search:
   «Гарант» (КП008) returns both «ТН-Кровля Гарант ПЛЮС» and «… RE30»,
   «Кровля» (КП013) says «несколько системных решений (N)» and offers help.
   A help request («какие есть системы кровли») is stripped of service words
   before searching; finding nothing is not a dead end — the reply says what to
   do. Numbers in buttons are indices into `RouterState.foundSuggest`, because
   `callback_data` cannot hold a Cyrillic title.

**Released and deployed 2026-11-12.** `backend` `dd60b9a` (plugin + service +
bot; this docs commit follows), `main` `fa29644` (plugin slice only, `b` suffix
removed, `main.js` rebuilt on `main`); registry git copy `sbe-apstore-registry`
`bc9ee07`. Self-hosted and verified over https **and** in `/registry.json`:
`manifest 26fb960b…`, `main 1a885d73…`, `styles a32ac3df…` (styles unchanged
from 0.9.16). Service and bot rebuilt and recreated on the server (19:05 and
19:11 MSK), health `ok`, poller running.

**Live defect found DURING the deploy** — worth remembering: the bot called
`/suggest` (a `requirePerm("viewer")` route) with the service secret, got 403
and showed «Поиск по Базе знаний сейчас недоступен» instead of a suggestion.
The stand could not see it: `auth-stub` grants the role by e-mail, so there the
secret and a user token are interchangeable. Fixed in `dd60b9a` (`/bot/suggest`)
plus `offers-bot/src/suggest-api.test.ts`, which asserts the ADDRESS, not the
result. **A route reached with the service secret must be a `/bot/*` route.**

Live check on the production KB after the deploy: `q=Гарант` returns
`ТН-КРОВЛЯ Гарант Плюс` and `ТН-КРОВЛЯ Гарант RE30` first (43 systems, 57
materials), `q=КРОВЛЯ` returns 60 systems — both complaints are closed on real
data.

Checks: `gofmt`/`go vet`/`go test ./...` green, `tsc` EXIT=0, `npm run build`
OK, `check_bundle_strings` 0 missing, OpenAPI **94/94** match, bot **162
passing**, stand **371/0** (new section «Поиск для окна выбора», 8 checks).

Previous version = **0.9.16** — released 2026-11-12. `main` `492a161`
(0.9.16 code, final build without the `b` suffix); `backend` carries the bot fix
(`e192eaf`).
Deployed: the **bot** was rebuilt and recreated 2026-11-12 16:38 MSK with the fix
(`grep` on the server confirms `applyCodeRequested(session, result.email)` at
`offers-bot/src/routes.ts:173`), poller running. `offers` runs the 0.9.15 SMTP
transport since 16:02 MSK (`smtp.yandex.ru:587`, `SMTP_USER/FROM=lpitn@yandex.ru`,
health `ok`; live `POST /api/offers/bot/request-code` returned `200 code_sent`
with no new exim lines). Self-hosted `f910cae9…` / `f5cafcb1…` / `a32ac3df…`
verified over https and in `/registry.json`; registry git copy
`sbe-apstore-registry` `a928f33`.

**0.9.16 (2026-11-12, bot-only + news).** Live complaint: the code arrives by
mail, but entering it makes the bot **ask for the e-mail again**.

**Root cause** — in `handleSignIn` (`offers-bot/src/routes.ts`) the
`request_code` branch never saved the session: the line was
`void applyCodeRequested;` — a **reference to the function instead of a call**.
The step stayed `idle`, and the six digits typed next were parsed as an e-mail,
which fails validation and re-prompts for the address. The compiler said nothing
(a bare expression statement is legal), and the bot suite was green because the
sign-in path had no test.

1. After requesting a code the session is saved (`await_code` + e-mail).
2. After verifying the code the session is saved as `authed` too — sign-in used
   to rely on in-memory state only.
3. Two tests over the whole sign-in path: the step becomes `await_code`, and the
   code neither re-prompts for the address nor fails to reach `authed`.

**History of 0.9.15** — released 2026-11-12 (server-only fix plus news);
`backend` `c2220b1`, `main` `fc680f3`; self-hosted `ecea0b7c…` / `f0711b80…` /
`a32ac3df…`; registry git copy `sbe-apstore-registry` `67ee1e3`. GitHub raw
agreed with the self-host after one retry.

**History of 0.9.14** — released 2026-11-12: `backend` `e93d45e`, `main`
`d418e3f`, self-hosted `4ff10217…` / `df5b0d7c…` / `a32ac3df…`, registry git
copy `sbe-apstore-registry` `a92d98b`. Live KB check after that deploy:
`/bot/combat/competitors?q=ПВХ мембрана` returns PLASTFOIL (9 first) and the tag
filter works; `q=мембрана кровельная` returns FACHMANN and PVC-RP;
`/bot/combat/analogues?noteId=7359` returns `PIRRO®VentiDuct` first.

**0.9.15 (2026-11-12, server-only + news).** Live complaint: sign-in codes for
the **offer bot** stopped arriving by e-mail. Same root cause as the
`auth-service` incident of 2026-11-09: `offers-service/email.go` sent via the
local exim (`host.docker.internal:25`) with the `epyur.fvds.ru` domain, which has
no SPF/DKIM/DMARC — the `tn.ru` corporate filter drops such mail, while exim
answers `250 OK`, so from outside it looks like "delivered".

1. **`offers-service/email.go` rewritten** to mirror `auth-service`: defaults
   `smtp.yandex.ru:587`, STARTTLS when advertised, AUTH PLAIN on
   `SMTP_USER`/`SMTP_PASS`, sender falls back to `SMTP_USER`, `net.DialTimeout`
   30 s + 2-minute session deadline, RFC 2047 Q-encoded Subject. Explicit
   `SMTP_*` still win — rolling back to exim is one compose line, no code.
2. **`sbe-core/docker/docker-compose.yml`, `offers` block**: `SMTP_HOST/PORT`
   from `AUTH_SMTP_*`, `SMTP_FROM/USER=${LAB_MAIL_LOGIN}`,
   `SMTP_PASS=${LAB_MAIL_PASSWORD}`, `SMTP_SKIP_VERIFY` removed (same mailbox as
   auth/documents). The local compose copy diverges from the server — insert the
   block pointwise (see `sbe-core/docker/AGENTS.md`).
3. **`offers-service/email_test.go`** — five guards: defaults, sender fallback,
   explicit override, RFC 2047 subject, recipient/body.

Checks: `gofmt` clean, `go vet` clean, `go test ./...` green (5 new tests),
`go build` OK, `tsc` EXIT=0, `npm run build` OK, `check_bundle_strings` 0
missing, `check_marker_registry` OK, OpenAPI 90/67.

`check_docs_state.mjs` is expected to fail while a release is unfinished — do not
"fix" it by editing docs.

**0.9.14 (2026-11-12, released).** Five live complaints about combat sheets;
four of them had one root cause — search went by substring over a cache instead
of by the Knowledge Base's own full-text search.

1. **Search** — side picking and analogue picking now use `GET /api/kb/search`
   through `KBClient.Search` (`kbsearch.go` contract in `kbclient.go`), the same
   mechanism the web KB uses: Russian morphology, prefixes, title weighted above
   body. Live proof: «пвх мембрана» + tag `competitor` returns PLASTFOIL,
   Sikaplan and ФАХМАНН together; «мембрана кровельная» pulls manufacturers the
   substring filter could never reach. The cache filter remains as a fallback
   when the KB is unreachable.
2. **Analogues** — `findAnaloguesForSheet` (KB search) with the cache chain as
   fallback. Queries go **from general to specific** (`мембрана кровельная` →
   `мембрана` → `ПВХ мембрана` → brand), because a brand query returns 0 rows on
   the live base and a core query returns nine PLASTFOIL. Candidates are filtered
   by `isAnalogCandidate` (no secondary goods: activators, cleaners, fasteners,
   funnels, systems, foams; class must not contradict ours) and diversified by
   brand (`diversifyByBrand`, ≤ 2 per brand), limit `combatAnalogLimit` = 8.
   Live result for LOGICPIR VENT: `PIRRO®VentiDuct` **first**; for LOGICROOF
   V-RP: FACHMANN ×2, PVC-RP/SR, PIRRO, PLASTFOIL, Sikaplan ×2, SikaProof.
3. **Bot access** — the list shows **all** sheets (not only the caller's);
   reading and exporting are open to every signed-in user, editing stays with
   the author (`botCanEditCombat`). `botCombatWith` is the single gate.
4. **Generation cache** — a rendered combat DOCX is stored in
   `offers_combat_docx` under a hash of everything printed (`combatDocxHash`:
   number, both sides in order, hint, layout bytes, `combatDocxVersion`). Second
   «⬇ DOCX» on the same sheet returns the stored file; any change of substance
   misses the hash and rebuilds.
5. **Progress in the bot** — `runWithProgress`/`progressNote` write «⌛ …» first
   and `editMessageText` replaces it with the result (or the error). The poller
   no longer waits for a handler before the next `getUpdates`: updates queue on
   one promise chain (`processUpdate`), which removes the 25-35 s long-poll tail
   that made a button look dead for a minute.
6. **List label** — `combatListLabel` (server) and `sheetLabel` (bot) show the
   product, not «Боевой листок»: our product → first competitor → «пустой
   листок № …». Foreign sheets are marked with the owner and get 👁 instead of ✏️.

Checks: `gofmt`/`go vet`/`go test ./...` green, `tsc` EXIT=0, `npm run build` OK,
`check_bundle_strings` 0 missing, `check_marker_registry` OK, bot **146 passing**,
OpenAPI 90/67, stand **361/0**. Live KB run of the analogue picker is recorded in
`HANDOFF.md` («Что проверено на боевой базе»).

Previous release — 0.9.13 (2026-11-11, released). Two things shipped together:

1. **The layout-mode defect** — an offer was printed as an empty combat sheet.
   Each document has its own `.docx` master (`offers_form.layout_docx` per
   `mode`), and the upload response never said which mode it wrote to, so a
   master uploaded "into the wrong document" looked like success. The response
   now carries `mode`, the plugin's Notice names the target document and warns
   on a mismatch, and the stand has a check for it.
2. **The rebuilt analogue picker** — it now searches **from our product** for
   **competitor products of the same class**, first via the **competitor
   journal** (grouping + target note links) with the model, then via the model
   over cards, then with a word-class fallback.

Previous release — 0.9.12 (2026-11-11, released): combat buttons re-open the
sheet after a bot restart (`ensureCombat`). Before that: 0.9.11 (analogue button
in the main menu), 0.9.10 (layout upload no longer crashes on `null`),
0.9.9 (no form — `{{сравнение}}`, hint, analogues), 0.9.8 («Добавить выбранные»
in the "all found" mode), 0.9.7 (two H1s, journal excluded), 0.9.6 (`competitor`
tag, numbered paged picker).

**Release 0.9.7 (2026-11-11, server + bot change; plugin-side `main.js` logic
unchanged, version bumped for the CUP news).** Live complaints after 0.9.6:
1. competitor notes carry TWO H1s (a disclaimer, then the product) — the title
   now comes from the first real one (`competitorTitles`), and the indexing
   journal (tag `journal`) is dropped from side picking;
2. the bot's picker lost marks on «Все продукты» and had no main menu
   (`withCombatMenu`) — see the "side picker" gotcha below.
   Checks: `gofmt`/`go vet`/`go test ./...` green, bot **123 passing**.

**Release 0.9.6 (2026-11-10, server + bot change; plugin-side `main.js` logic
unchanged, version bumped for the CUP news).** Two user requests:
1. competitor products are read from the **`competitor` tag** as well as the KB
   folder (merged by note id), and the card title is the **note heading**
   (`competitors.go`);
2. the bot's side picker is a **numbered, paged, multi-select list** with
   «☑ Выбрать все найденные» — see the "side picker" gotcha below.
   Checks: `gofmt`/`go vet`/`go test ./...` green, `tsc` EXIT=0, bot **121
   passing**, OpenAPI 87/64, `crapper` ≤ CC 9, `dryer` no new duplicates.

**Release 0.9.5 (2026-11-10, server-only change — `main.js` logic unchanged,
version bumped only for the CUP news).** Two user requests:
1. the link section is printed **by groups and numbered** — `page` (systems
   before materials) → `sto` → `techlist` (systems before materials) → `uzly` →
   `cert` (materials before systems); see the "Links in the document" gotcha
   below;
2. the model's text is printed **as paragraphs** and its typography is normalised
   (double spaces, stray spaces before punctuation, missing space after a
   sentence end) — `normalizeBodyTypography` + `paragraphsXML` in
   `docx_text.go`/`docx.go`, applied on write (`cleanBlockText`), on read
   (`generatedTextFor`) and in the `prompt_text` node. The block system prompt
   also asks for paragraphs and correct punctuation (`skills/prompt.go`).
   Checks: `gofmt`/`go vet`/`go test ./...` green, `tsc` EXIT=0, stand **339/0**.

**Release 0.9.9 (2026-11-11, server + bot + plugin).** The combat sheet is no
longer form-driven:
1. `combatDefaultForm` has **no blocks and no fields** except the hint; the old
   «наша компания / объект / заказчик / менеджер» fields were invalid for an
   internal note.
2. Printing is a layout plus ONE marker `{{сравнение}}` (`packCombatDocx`,
   `combatLayoutData`): the marker prints the whole sheet; without a layout the
   built-in view is used. Form blocks are never read by the combat renderer.
3. The comparison is written by the model (`combat_compare.go`) — overall, then
   per competitor product, steered by the hint; facts only from the cards.
4. «Подобрать аналоги» (`combat_analog.go`) matches our product to a competitor
   card by significant words; a title hit weighs more than a description hit.
   Checks: `go test ./...` green, bot **134 passing**, OpenAPI 89/66.

**Release 0.9.13 (2026-11-11, IN PROGRESS — deployed to the service, not
released).** The user's "find analogues" flow, rebuilt three times in one day:

1. **Direction**: analogues are searched FROM OUR product (`ourNotes`) for
   competitor products of the same class; picked ones are added on the
   **competitor** side. Without our product the bot says which button to press.
2. **Chain** (`findAnaloguesSmart`, `combat_analog_llm.go`):
   journal → model over cards → word-class fallback.
3. **Journal** (`combat_journal.go`): `parseCompetitorJournal` reads the
   «Журнал материалов конкурентов» note (tag `journal`). Competitor and catalog
   section are BOTH `##` and are told apart by the item counter in the heading
   («— 28 шт.»); table rows give `title | url | [[note]]`, and the wikilink
   resolves to the real catalogue card. Sections are pre-filtered by
   `relevantJournalSections` — the journal is huge (FACHMANN 1738 rows, SIKA
   385) and its first 120 rows belong to one competitor.
4. **Limits**: `analogLLMTimeout` 60 s, `analogJournalRowLimit` 120,
   `analogCandidateLimit` 60, `analogLLMLimit` 5, `combatAnalogLimit` 5.
5. **`cban:all`** — "add every found" — is parsed as a word, id 0
   (`parseCallbackData`).

**What to fix first if it stops working**: read the service log for
`журнал дал N аналогов` and `context deadline exceeded` — the chain falls back
silently, and the fallback result looks like a wrong selection rather than an
error.

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
| `scripts/offers_local_e2e.mjs` | the stand: **351 checks**, 26 sections |
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

`offers-bot/`: `src/main.ts`, `src/poller.ts` (transport only), `src/routes.ts`
(routing + the single sign-in check), `src/callbacks.ts` (button handlers,
including the combat sheet — 2026-11-09), `src/telegram.ts`
(Bot API via SOCKS), `src/session.ts`, `src/attach.ts`, `src/flow/` (`build`,
`brief`, `copy`, `writer`, `resume`, `combat`, `suggest`, `appbutton`),
`src/api.ts`, `Dockerfile`.

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
cd .. && node ../scripts/check_offers_openapi.mjs
docker run --rm --network sbe-offers_offersnet -v "$PWD/scripts:/app:ro" -w /app \
  node:22-alpine node offers_local_e2e.mjs http://offers-local-app:3000
python scripts/check_layout_mode_defect.py http://127.0.0.1:18080
```

Expected on 0.9.17: `tsc` EXIT=0, `check_bundle_strings` 0 missing,
`check_marker_registry` OK, `go test` green, OpenAPI **94/94**, stand
**371/0** (the picker-search section is 8 of them, the combat-access section 3).
The bot's own suite (`offers-bot`): `npm run typecheck` EXIT=0,
`node --test "src/**/*.test.ts"` **171 passing** (combat flow, sign-in path,
the suggestion texts and the Mini App buttons are covered by route tests
against a faked service).

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

### Bot and rights

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
    material repeat prints one position per pass.
13. **A prompt table still costs one model call** in some layout paths —
    `layoutTableFor` builds tables from data, verify no call happens first.
14. **`sbe-kb/src/services/hash.ts`** is a second copy of
    `sbe-core/src/utils/files.ts`.