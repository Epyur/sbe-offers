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

Current version = **0.9.10**, released 2026-11-11: deployed, self-hosted,
registry updated. Code commits: `backend` `89f4265` (plugin part +
`offers-service`), `main` `f42e0c2` (plugin part byte-identical across branches;
`offers-service/` lives only on `backend`; `check_docs_state.mjs` looks at the
last commit that touched code). 0.9.10: layout upload no longer crashes on an empty marker
list (nil slices were sent as `null`), and `{{сравнение}}` shows in the marker
list. Previous release — 0.9.9: the combat sheet has NO form — content is always the product
list plus the comparison; the layout is printed with a single `{{сравнение}}`
marker; the manager's hint steers the model; «Подобрать аналоги» finds our
product for a competitor. Previous release — 0.9.8: the «Добавить выбранные»
button no longer vanishes in the "all found" mode.
Previous release — 0.9.6 (2026-11-10): competitors from the `competitor` tag
plus the KB folder, the bot's side picker as a numbered, paged, multi-select
list with «выбрать все найденные».

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
| `scripts/offers_local_e2e.mjs` | the stand: **339 checks**, 25 sections |
| `scripts/{auth,kb,llm,bot}-stub.mjs` | stand stubs for the services the plugin talks to |

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
`brief`, `copy`, `writer`, `resume`, `combat`), `src/api.ts`, `Dockerfile`.

## Settings (data.json)

`apiUrl` (default `https://epyur.fvds.ru`), `lastAnnouncedVersion`.

## Environment variables (server)

`DATABASE_URL`, `PORT`, `JWT_SECRET`, `OFFERS_APP_ID` (`offers`),
`OFFERS_APP_NAME`, `OFFERS_OWNER_EMAIL`, `OFFERS_SERVICE_SECRET`,
`OFFERS_BOT_SECRET`, `AUTH_SERVICE_URL`, `S3_ENDPOINT`/`S3_ACCESS_KEY`/
`S3_SECRET_KEY`/`S3_BUCKET` (`sbe-offers`), `SMTP_*`, `KB_SERVICE_URL`,
`LLM_SERVICE_URL`, `LLM_SERVICE_SECRET`.
Bot: `TELEGRAM_SOCKS_PROXY`, `OFFERS_API_URL`, `OFFERS_BOT_SECRET`,
`OFFERS_BOT_RATE_LIMIT`, `VERIFY_PORT`.

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
```

Expected on 0.9.3: `tsc` EXIT=0, `check_bundle_strings` 0 missing,
`check_marker_registry` 24/24, `check_docs_state` EXIT=0, `go test` green,
OpenAPI 87/64, stand **333/0** (stand not re-run for 0.9.3: the plugin-side
stand count is unchanged, and the added combat checks run in the Docker stand).
The bot's own suite (`offers-bot`): `npm run typecheck` EXIT=0,
`node --test "src/**/*.test.ts"` **112 passing** (combat flow — search, folder
subtree, multiple sides — covered by route tests against a faked service).

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