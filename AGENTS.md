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
and the matching plan; context-first bot and composable form -
`docs/superpowers/specs/2026-11-01-sbe-offers-context-first-and-form-blocks-design.md`
and the matching plan; encoding investigation -
`docs/superpowers/plans/2026-10-31-encoding-breakage-investigation-plan.md`.

**This file is a reference, not a log** (revision 2026-11-01). It holds what the
system is now: structure, rules, gotchas, state. **The history of releases
0.1.0 … 0.4.0 - nine live defects with dates, measurements and commit hashes -
lives in [../agents_arhiv.md](../agents_arhiv.md)**, section "История сессий",
entry "sbe-offers". Go there when you need to know *why* a rule exists or are
hitting an old complaint again; work from here.

**Starting cold?** Read [HANDOFF.md](HANDOFF.md) first - versions, hashes, the
check commands, what is unfinished. It is one page and it is meant to be read
whole.

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
- **Offer form** (`offers_form`) - placeholders + document blocks + required
  fields. The editor is a block editor built after the LIMS protocol
  configurator. Since 2026-11-01 a block is more than text: `paragraph`,
  `heading`, `bullet_list`, `table`, `page_break`, `links`, `prompt_text` and
  `image`, plus placement (`spacingBefore`/`spacingAfter` in points,
  `keepTogether`, `pageBreakBefore`). The default form is cover -> page break ->
  supplier/recipient/object -> generated description -> positions -> links.
- **Offer** (`offers_offer`) - a snapshot: positions with the price at the
  moment they were added, form values, total, `draft`/`final` status, number
  `{NNN}/{yyyy}` continuous through the year. Visibility: admin sees all,
  everyone else sees their own. Since 2026-11-01 the row is created at the
  **start** of the bot conversation with `items: []` (the REST path still
  refuses an empty offer).
- **Telegram bot** - sign-in by e-mail and code (self-request by mail or a code
  from the administrator). "Собрать КП" now **starts with context**: object,
  purpose, area, addressee, the deal, planned systems, terms - and only then
  positions. A file can be sent at any step. The `система <название>` command
  expands a system into positions; DOCX arrives as a file in the chat or as a
  link on a button.
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
| `src/ui/block-editor.ts` | block editor: nodes (text/list/table/page break/links/prompt/image), `{{field}}` chips, block placement |
| `src/ui/image-picker.ts` | picks an image from the vault, uploads it to KB attachments, returns the key |
| `src/ui/access-editor.ts` | "👤 Бот и доступ": codes, bot token, who signed in |
| `src/ui/help.ts` | help text (baked into the plugin) |
| `src/utils/date.ts`, `money.ts`, `note-path.ts` | shared helpers - one copy each |
| `src/styles.css` | `tn-offers-*` classes (the root `styles.css` is **built**, never edit it) |
| `scripts/check_bundle_strings.mjs` | proves the Russian labels reached `main.js` (escaped `\uXXXX`, uppercase) |
| `scripts/offers_local_e2e.mjs` | the stand: 213 HTTP checks, 19 sections |
| `scripts/{auth,kb,llm,bot}-stub.mjs` | stand stubs for the services the plugin talks to |

## Server structure

`offers-service/`: `main.go` (routes, migrations), `jwt.go`, `register.go`,
`authperms.go` (a copy of the other services - **edit all copies at once**),
`permissions.go`, `catalog.go`, `material.go`, `material_notes.go`,
`matcher.go`, `sheet.go`, `upload.go`, `systemcards.go`, `systems_compose.go`,
`systems_links.go`, `botcatalog.go`, `botapi.go`, `form.go`, `offers.go`,
`brief.go`, `briefstep.go`, `copystep.go`, `attach.go`, `kbclient.go`,
`kb_image.go` (KB attachments as an `ImageSource`), `llmclient.go`,
`docx.go`, `docx_media.go`, `docx_links.go` (link section + hyperlink rels),
`docx_image.go` (`wp:anchor` + wrap), `docx_text.go` (prompt blocks + cache),
`s3.go`, `email.go`, `static.go`, `template.go`, `static/openapi.yaml`.
Sub-packages: `model/` (domain types) and `skills/` (prompts, checklists, card
and attachment parsing, `cardrefs.go` = `## Ссылки` parser).

`offers-bot/`: `src/main.ts`, `src/poller.ts` (transport only), `src/routes.ts`
(routing), `src/callbacks.ts` (the draft flow and buttons), `src/telegram.ts`
(Bot API via SOCKS), `src/session.ts` (sign-in state machine), `src/attach.ts`
(file preview → confirm), `src/flow/` (`build` catalogue, `brief` dialog,
`copy` sections, `writer` orchestration), `src/api.ts`, `Dockerfile`.

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
node scripts/check_bundle_strings.mjs      # русские строки в бандле (экранированы!)
cd offers-service && gofmt -l . && go vet ./... && go test ./...
cd offers-bot && npm run typecheck && npm run build && node --test "src/**/*.test.ts"
node ../../scripts/check_offers_openapi.mjs
docker run --rm --network sbe-offers_offersnet -v "$PWD/scripts:/app:ro" -w /app \
  node:22-alpine node offers_local_e2e.mjs http://offers-local-app:3000
```

**После `npm run build` проверять строки через `scripts/check_bundle_strings.mjs`,
а не глазами.** esbuild с `minify` не трогает кириллицу по-человечески: она уходит
в `\uXXXX` **верхним** регистром hex. Поиск строчными (`\u0430`) даёт ложное
«строки нет», и я на этом ошибся дважды за один цикл (2026-11-01), плюс
PowerShell дважды задвоил бэкслеш в самодельной команде. Скрипт делает это
правильно и падает с ненулевым кодом.

**Стенд обновлять после правок сервиса.** Бинарь на стенде подменяется так
(Docker Hub на этой машине недоступен, поэтому сборка в `golang:1.24`):

```
docker run --rm -v "$PWD/offers-service:/src" -w /src -e CGO_ENABLED=0 golang:1.24 \
  sh -c "go build -o /src/offers-stand . "
docker cp offers-service/offers-stand offers-local-app:/mailers-offers
docker restart offers-local-app
```

`CGO_ENABLED=0` обязателен: CGO-бинарь не запустится в alpine-образе
(«no such file or directory» — про интерпретатор, а не про файл).

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

## Gotchas

Rules learned the hard way, distilled. **The stories behind them (nine live
defects, dates, measurements) are in
[../agents_arhiv.md](../agents_arhiv.md), section "История сессий", entry
"sbe-offers".** Read the archive only when you need to know *why* a rule exists
or are hitting the same complaint again - these lines are enough to work by.

Each item names the code it applies to, so you can tell in one look whether
your change is affected.

### Russian text, regexes and search

- **`\b` is an ASCII word boundary - never use it on Cyrillic.**
  `\b(?:MAT|РАБ)-\d{4}\b` silently finds nothing in `РАБ-МОНТ-ОБЩ-0001`: the
  boundary is computed from ASCII letters, and `Р` does not count. Write the
  boundary explicitly (`(?:^|[^А-ЯЁA-Z0-9])`) and always add a test with one
  real `РАБ-…` / `MAT-…` line. Caught three times:
  `skills/systemcard.go`, `matcher.go` (`extractCompositionSku`),
  `skills/cardrefs.go`.
- **Cyrillic in the built `main.js` is escaped as `\uXXXX` in UPPERCASE hex.**
  Searching the bundle for a plain Russian label proves nothing; searching for
  lowercase escapes is a false negative too. Use
  `node scripts/check_bundle_strings.mjs` - it escapes correctly and exits
  non-zero. Do not rebuild the escaping by hand in a shell (PowerShell doubles
  backslashes).
- **A Go file rewritten through a PowerShell pipeline comes back as mojibake**
  (`Get-Content -Raw | Set-Content -Encoding UTF8` turned `attachments.go` into
  CP1251 garbage). Edit sources with the editor tools only.

### Building and the stand

- **The stand binary must be built with `CGO_ENABLED=0`.** A CGO build copied
  into the alpine container fails with `no such file or directory` - the message
  is about the missing `ld-linux` interpreter, not the file. After
  `docker cp`, restart the container and check `docker ps` says `Up`, not
  `Exited (255)`:
  ```
  docker run --rm -v "$PWD/offers-service:/src" -w /src -e CGO_ENABLED=0 golang:1.24 \
    sh -c "go build -o /src/offers-stand ."
  docker cp offers-service/offers-stand offers-local-app:/mailers-offers
  docker restart offers-local-app
  ```
- **`--build` alone may leave the old container running.** After
  `docker compose up -d --build offers`, check that `docker ps` `CREATED` moved
  before calling the deploy done.
- **A stub I write myself is a claim about someone else's contract.** `kb-stub`
  once answered every note for a tag and lied in exactly the way that made tests
  green and the feature broken (materials were "verified" by a fixture that
  encoded my guess). Pin foreign contracts with a recorded real response
  (`skills/testdata/`), and make the stub filter like the real service.
- **A service that talks to another service needs BOTH the URL and the secret
  in its compose block, plus a stub on the stand.** An empty URL is not an
  error: the service starts, reports "not configured" and looks healthy. This
  is how "База знаний не настроена" passed every check while the KB worked.
- **`docker exec -i` inside a `while read` loop eats the rest of the list** - it
  reads the same stdin. Feed the loop from a file or give the command
  `< /dev/null`.
- **Piping a remote script through `tail` returns `tail`'s exit code.**
  `ssh host "bash x.sh 2>&1 | tail -20"` prints `EXIT=0` for a failed build.
  Run the check as its own line and read `$?` immediately; verify with
  `md5sum`/`sha256sum` that the file really landed before trusting a green run.

### Data and documents

- **A screen that reads another service live on every open will eventually show
  that service's error to the user.** `GET /systems` did exactly that: hundreds
  of KB requests per open, and one stale token poisoned the whole load - the
  person saw «Ключ доступа недействителен» while the key was fine. Read from a
  cache, refresh on an explicit action (2026-11-01).
- **Service-to-service clients need the same 401 retry as the plugin.** The
  plugin had it since 0.2.1; `KBClient` did not, so one rejected token inside a
  long fan-out was final. `get` now drops the token and repeats once.
- **Time in API responses must be `RFC3339Nano`, not `RFC3339`.** The shorter
  layout drops fractions, so a server row `12:17:09.284` reaches a client as
  `12:17:09Z` - older than the client's own copy - and the server loses the
  LWW merge to itself. Any UPDATE that changes data must also move
  `updated_at`, or the edit never wins the merge.
- **`returning` a CTE subquery sees the row AFTER the update** - it returns
  `NULL`. Read the old value with `UPDATE … FROM (SELECT … FOR UPDATE) AS old
  … RETURNING old.col`.
- **Reading the file input value before the file is taken.** `input.files` is a
  live `FileList`: `value = ''` empties it, and the "no files" check then fires
  on the file the person just picked. Take `files[0]` first, clear after.
- **Word: only the brand template's styles.** `styles.xml` and `numbering.xml`
  come from `standard.docx` untouched (`a0` = Normal, `1` = heading 1,
  `numId 6` = bullet, `ab` = Hyperlink). Replacing them removed the footer page
  numbers. Image ids (`wp:docPr`) are computed across all package parts (the
  brand occupies 1..25) and media is written as `offerimgN.*` so the brand's
  `image1/image2` survive.
- **`wp:inline` breaks the text flow; `wp:anchor` + `wp:wrapSquare` does not.**
  This is why the image block is anchored: a picture must not push a paragraph
  onto its own line.
- **A page break at the very start of the document is ignored** - Word renders
  an empty page and the letter looks broken.
- **Text written by the model has to be cached against the offer's data hash**
  (`form_values["gen.section"]`), otherwise the same offer yields different text
  on every download and a client comparing two PDFs sees the difference.

### Encoding

- **Never write a file with `Set-Content`/`Out-File` without an explicit
  encoding**, and never `>` redirect. PowerShell 5.1 `-Encoding UTF8` writes a
  BOM, `git cat-file -p … > file` writes UTF-16. A BOM in `manifest.json` makes
  the plugin disappear from the store.
- **`styles.css` in the plugin root is built**, never edited: the esbuild hook
  concatenates `sbe-core/src/design/tokens.css` + `components.css` + the
  plugin's `src/styles.css`.

### Bot and rights

- **Roles do not exist in the bot** (2026-10-29): anyone signed in with a code
  builds offers. The plugin keeps `viewer < editor < admin < superadmin`;
  `superadmin` is an administrative level too - compare through
  `isAdminRole`/`canEditOffers`, never by string equality.
- **Sign-in is checked once, in `routes.ts`**, for both text and callbacks.
  Writing the check twice already cost a duplicated fix.
- **The offer is created at the start of the conversation** (2026-11-01), so a
  file can attach to it. `handleBotCreateOffer` accepts `items: []`; the REST
  path still refuses an empty offer - do not "unify" these two.
- **`callback_data` is limited to 64 bytes**; system codes are Cyrillic, so
  buttons carry an index into the per-chat list, not the code.

## Изменения правил по итогам ревизии 2026-11-01

- История выпусков 0.1.0 … 0.4.0 вынесена в
  [../agents_arhiv.md](../agents_arhiv.md) (раздел «История сессий»). Здесь
  остался справочник: что это, как устроено, какие правила, что сейчас живо.
- Раздел «STATE» ниже обновляется при каждом выпуске и держится коротким:
  номера версий, суммы, что проверено, что не сделано.

## 2026-11-01 (evening) - the "Системы" tab stops hammering the Knowledge Base

Live complaint: the **Системы** tab answered 401 «Ключ доступа недействителен»,
the same text appeared in **Контекст КП** on "Перечитать", and the offer editor's
"Добавить систему" picker failed the same way. The user's direction: *"load it
once and use what we know; refresh when refreshing"*.

### What was actually broken

Two separate things, and the first one is why this came back after 0.2.1.

1. **`GET /api/offers/systems` read the Knowledge Base live on every call** -
   all `commercial_propose` cards, then a full-note request per card (376
   systems = ~380 requests, 1165 materials on the material path). A single 401
   anywhere in that run was cached by `KBClient` for the whole run and surfaced
   to the person. `listMaterials` did the same thing every time the materials
   list was shown. So the tab was not flaky - it was a long fan-out of requests
   that had to survive minutes without one rejection, and an expired hour-long
   JWT (or one rejected request) killed the whole load.
2. **`KBClient.get` had no retry.** The plugin had 401-retry since 0.2.1, but
   the *service-to-service* client did not: one rejection = error for the caller.

### What changed

- **`GET /api/offers/systems` reads the `offers_context` cache** (`mode =
  'systems'`). Empty cache - read the KB once and fill it; KB down - empty list
  plus `error`, so the section says what is wrong instead of showing 401 text.
- **`POST /api/offers/systems/refresh`** (admin) is the only path that reads the
  KB for this section. On failure it returns the previous cache with status 200
  and `error` filled - a temporary outage must not empty the section. Same
  pattern as `POST /context/refresh`.
- **`KBClient.get` retries once on 401**: drops the cached token, takes a new
  one, repeats. A second 401 is a real error. Two tests pin this
  (`TestKBClientRetriesOn401`, `TestKBClientGivesUpAfterSecond401`).
- **`SystemsWithCompositionFromKB`** is now its own function and type: the
  section returns `compositionLinks`/`problems`, the Контекст cache keeps
  `SystemCard` without them. `listMaterialsOnce` was split out of
  `listMaterials` so a refresh does not trigger a second KB fan-out.
- **Plugin**: `systems-editor.ts` loads from the cache and has one
  "↻ Перечитать из Базы знаний" button; `offersService.refreshSystems()` is the
  new call. `styles.css` unchanged.

### Verified live (2026-11-01, mailers-offers-1 rebuilt 15:19 MSK)

| check | result |
|---|---|
| cold `GET /systems` (cache row deleted) | 376 systems, `error:""`, **4 s**, cache row written |
| warm `GET /systems` | same 376, **0 s**, no KB traffic, no new rows on iteration |
| `POST /systems/refresh`, owner token | 376 systems, 4 s |
| refresh with a **viewer** role | 403 (`role`) - admin only, as the route says |
| `GET /systems` with a **viewer** role | 200 from cache, 0 s |
| no token on either route | 401 (`no-token`) |
| 3x full materials sync (1165 cards) | 1 s / 1 s / 0 s, **0** KB rejections |

`gofmt -l` empty, `go vet` clean, `go test ./...` green, plugin `npx tsc
--noEmit` EXIT=0, `npm run build` OK, `check_bundle_strings.mjs` 0 missing,
`check_offers_openapi.mjs` **61 operations / 48 paths, schema matches**.

**Not fixed, on purpose**: the KB JWT still lives an hour and the service still
caches it for 50 minutes. The retry makes a stale token self-heal, which is
enough. If a *systematic* rejection ever shows up, the next place to look is
`kb-service`'s gate, not this client - it logs every refusal with a reason
(`отказ 401: ... — токен не разобран`), and those lines are the evidence.

## STATE 2026-11-01 (release 0.4.0, deployed and self-hosted)

One place to read what the system is right now. History and reasons live in
the entries above; this section answers "what is live, what works, what is
still missing".

### Versions and branches

| | value |
|---|---|
| plugin | **0.4.0** (`manifest.json` = `package.json`) |
| remote `main` | `a3dab26` |
| remote `backend` | `5e8c148` (plugin part = `main`, plus the server folders) |
| server code | `offers-service/`, `offers-bot/` - **`backend` only**, `.gitignore`d on `main` |
| live containers | `mailers-offers-1`, `mailers-offers-bot-1` recreated 2026-11-01 14:09 MSK |
| `sbe-core` | `eea98f6` (`backend`) - `src/utils/files.ts` added |

### Self-hosting (checked over https, `_t=` cache-buster)

| file | sha256 | bytes |
|---|---|---|
| `manifest.json` | `a46eff000b3ec366d675b0e929b32a61fe96e58fda5dec05f942a48a0d1a527a` | 469 |
| `main.js` | `bd42d5a1943aa092f210c2f5a9f2e70b597a8a97a1908a6b25e2f171e143a2b1` | 113242 |
| `styles.css` | `46f4c8801248eb87cac43b6c4ac00df86af55c0d3c0ff55401224d634f1d83d0` | 31438 |

`styles.css` did not change in this release (no CSS edits) - the same sum as
0.3.0. All three were compared byte-for-byte against
`https://epyur.fvds.ru/plugins/sbe-offers/*`; `U+FFFD` count in the delivered
stylesheet is 0. `registry.json` on the server reports the same three hashes
and `selfHosted: true`, `uploadedAt 2026-11-01T11:11:10Z`. The git copy in
`sbe-apstore-registry` is commit `dcbe363`.

### What works (verified, not assumed)

- **Bot**: "Собрать КП" creates an empty offer with a number, then walks the
  brief by stages (object/addressee -> deal -> systems -> terms), file button
  on every question, positions afterwards, then section copy, then DOCX.
  Verified live: `GET /bot/offers/{id}` answers 200 for the owning chat, 404
  for a chat that is not linked, 401 without the service secret.
- **Links**: `skills.ParseCardRefs` run against the **real vault**: 120 system
  cards -> 919 links (120 page + 799 docs), 1166 material cards -> 2353 links
  (1165 page + 1188 docs). Every link had a label and a URL. One material card
  of 1166 has no `## Ссылки` section - that is in the data, not the code.
- **Document**: a real DOCX was built and unpacked - page break, anchored image
  with `wrapSquare`, caption, clickable hyperlink relationships, no leftover
  `{{placeholders}}`, all parts well-formed XML, image and hyperlink relations
  coexist.
- **Form**: a saved form is NOT replaced by the new default. Live check: the
  server keeps the form saved on 2026-10-27 (`intro`, 2 blocks, 4 fields).
  The new default applies only when no row exists.
- **Schema**: `/api/offers/openapi.yaml` live contains `bot/offers/{id}`,
  `page_break`, `prompt_text`, `spacingBefore`.

### Checks at the moment of release

`gofmt -l .` empty, `go vet ./...` clean, `go test ./...` green (`main` 44.0%,
`model` 70.1%, `skills` 84%), bot `tsc --noEmit` clean and **78 tests**,
plugin `tsc --noEmit` EXIT=0 and `npm run build` OK at 0.4.0,
`check_bundle_strings.mjs` 0 missing, `check_offers_openapi.mjs`
**60 operations / 47 paths, schema matches**, local stand E2E **213/213 twice**.
`crapper`: every new function under CRAP 10 except `KBImageSource.Get` (CC 11,
network glue with no unit coverage) and **`handleDraftText` in
`callbacks.ts` (CC 13)** - both recorded, not hidden. `handleDraftText` grew in
this cycle (the context-stage guard); it is the same class as the dispatchers
retired in 2026-10-29 - a function mixing brief, the `система` command, free
position, quantity and form field. Take it apart at the next bot change with a
handler table instead of an `if` chain; it has no unit coverage (bot routes are
checked by the stand). `dryer`: two pairs reported in this cycle were mine and
were removed (`flagXML`, the stage loops); the two that remain are pre-existing
(`offer_boundary.go`/`offers.go` and `botapi.go`/`offers.go`, both present before
this cycle).

### What is still missing (honest list)

1. **The systems' composition links.** 1284 composition lines have no
   `[[link]]` at all - plain text ("Первый материал или слой", the template
   placeholders of unfinished cards). A technologist's work; no code can guess
   them. This is the same number as before the release.
2. **Automatic SKUs are meaningless** - everything synced from KB cards lands
   in `MAT-ПРОЧ-ОБЩ-NNNN`, because the sync passes no type/series hints. If the
   type code matters, that is a decision to make.
3. **Prompt text is not agreed with a human.** Blocks written by the model are
   cached and reprinted, but nobody signs them off - unlike section copy, which
   the manager approves in the chat. If the administrator wants approval, that
   is the next step.
4. **The form editor has no live preview** of page breaks and image wrapping.
   The admin sees the result only in the built DOCX.
5. **Images live in KB attachments** (`POST /api/kb/attachments`), so a broken
   KB means no new pictures in blocks. Existing documents keep working: the
   image key is stored in the block.
6. **`sbe-kb` still has its own `services/hash.ts`** - a second copy of what is
   now `sbe-core/src/utils/files.ts`. Merge at that plugin's next change.
7. **The bot conversation was not walked end-to-end by a human in Telegram**
   after this release. The unit tests and the stand cover the state machine;
   the live checks covered the routes, not a real dialogue.

### Where to look first when something breaks

| symptom | start here |
|---|---|
| bot asks positions immediately | `callbacks.ts` `startDraft` must create the offer and call `beginBrief`; `draft.stage` must be `context` |
| «в предложении нет позиций» on save | `finishDraft` PATCHes the existing offer; an empty `items` from the sheet means `attach/apply` did not run |
| a block is missing from the DOCX | model down (`prompt_text` is silent by design), image key not in the store, or `showInClient` off |
| links absent in the document | the card has no `## Ссылки` section, or `cardsForOffer` did not match the system to the composition |
| old prompt text in a new document | `form_values["gen.section"]` hash matched; adding a system drops it, editing an item directly does not |