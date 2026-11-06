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
whole. The section "Текущее состояние" there is always the current truth; the
newest "STATE" block at the bottom of this file is the short version of it.

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
  **Choosing a system does not add its materials as positions** (2026-11-03):
  the offer gets the description, advantages and links, while the priced
  composition is entered by the manager (a file or by hand).
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
  refuses an empty offer). Since 2026-11-03 it also carries the chosen system;
  **since 2026-11-05 systems are a list** (`system_choice` JSONB read as
  `Offer.systems`, in order of choice) - that is what prints the justification,
  the advantages, the links and the comparison table. The old object shape is
  still read, so no migration was needed.
- **Telegram bot** - sign-in by e-mail and code (self-request by mail or a code
  from the administrator). "Собрать КП" now **starts with context**: object,
  purpose, area, addressee, the deal, planned systems, terms - and only then
  positions. A file can be sent at any step. The `система <название>` command
  adds the system to the offer - it does **not** add positions (2026-11-03);
  DOCX arrives as a file in the chat or as a link on a button.
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
| `src/services/offers.service.ts` | HTTP to `/api/offers/*`, `OffersError`, 401 retry, `exportDocx`, `refreshSystems` |
| `src/ui/offers-view.ts` | facade: top bar + sidebar + content |
| `src/ui/offers-list.ts` | "🧾 КП": list, card, composition editor, "Добавить систему", DOCX, link |
| `src/ui/material-editor.ts` | "🧱 Материалы": list with SKU, card with the ЕКН table |
| `src/ui/systems-editor.ts` | "🧩 Системы": cached systems, resolved composition, problems; one "↻ Перечитать из Базы знаний" button |
| `src/ui/sheet-upload.ts` | manager's Excel: "what I understood", then apply |
| `src/ui/context-editor.ts` | "📚 Контекст КП": cached system cards, open note |
| `src/ui/brief-editor.ts` + `brief-model.ts` | the brief card inside the offer editor |
| `src/ui/form-editor.ts` | "⚙️ Форма КП": fields, blocks, required, describe-block button, layout card |
| `src/ui/block-editor.ts` | block editor: nodes (text/list/table/page break/links/prompt/image), `{{field}}` chips, block placement, image source |
| `src/ui/form-preview.ts` | live preview of the document under the blocks (alignment, indent, superscripts, tables, images) |
| `src/ui/describe-block-modal.ts` | "✨ Опишите блок словами": free text → nodes, with a preview before applying |
| `src/ui/form-layout.ts` | ".docx layout" card: upload / download / delete, shows the tags found in the file |
| `src/ui/image-picker.ts` | picks an image from the vault, uploads it to KB attachments, returns the key |
| `src/ui/access-editor.ts` | "👤 Бот и доступ": codes, bot token, who signed in |
| `src/ui/help.ts` | help text (baked into the plugin) |
| `src/utils/date.ts`, `money.ts`, `note-path.ts` | shared helpers - one copy each |
| `src/styles.css` | `tn-offers-*` classes (the root `styles.css` is **built**, never edit it) |
| `scripts/check_bundle_strings.mjs` | proves the Russian labels reached `main.js` (escaped `\uXXXX`, uppercase) |
| `scripts/check_docs_state.mjs` | docs vs code: version in `manifest`/`package`, SHA-256 of the three build files, HEAD of both branches present in `AGENTS.md` |
| `scripts/offers_local_e2e.mjs` | the stand: **264** HTTP checks, 21 sections |
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
`docx_compare.go` (`compare_table` node - system comparison),
`docx_runs.go` (one text pipeline: wiki links, glued words, superscripts; the
`paraOpts` document formatting), `docx_card_images.go` + `card_images.go`
(images of systems and materials, attachment → key), `docx_layout.go` (printing
the user's `.docx` master), `layout_tokens.go` (`[…]` / `{…}` scanner and the
human field names), `form_layout.go` (layout upload/read/delete),
`form_describe.go` ("describe a block in words"),
`s3.go`, `email.go`, `static.go`, `template.go`, `static/openapi.yaml`.
Sub-packages: `model/` (domain types, plus `jsonrepair.go` - tolerant JSON
extraction shared with `skills`), `skills/` (prompts, checklists, card and
attachment parsing, `cardrefs.go` = `## Ссылки` parser, `filecontext*.go` =
object context out of a file: literal facts, model prompt, merge,
`blockstructure.go` = the "describe a block" prompt).

`offers-bot/`: `src/main.ts`, `src/poller.ts` (transport only), `src/routes.ts`
(routing), `src/callbacks.ts` (the draft flow and buttons), `src/telegram.ts`
(Bot API via SOCKS), `src/session.ts` (sign-in state machine), `src/attach.ts`
(file preview → confirm), `src/flow/` (`build` catalogue, `brief` dialog,
`copy` sections, `writer` orchestration, `resume` = reopen a saved offer),
`src/api.ts`, `Dockerfile`.

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
node scripts/check_docs_state.mjs          # документация не разошлась с кодом
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

## The .docx layout placeholders and model answers (2026-11-06, release 0.7.3)

Version (manifest.json = package.json) = **0.7.3**.

Release branches: `backend` `8dbb0bd` (release) + `1c10d7d` (fix), `main` `f981e24` (plugin part); both pushed, build sums identical on both branches.



Six live defects of offer 003/2026, found by running the REAL service prompt
through the REAL model on live offer data (pulled from the production DB,
executed inside `golang:1.24` on the `mailers_internal` network). None of them
showed up in a unit test. **Do the same for the next complaint: the model
provider answers differently from what a fixture assumes.**

1. **A placeholder may span several Word paragraphs.** The live master
   (`temple.docx`) carries the description instruction as NINE paragraphs: `{{`
   opens in the first, `}}` closes in the ninth. The old code scanned each
   paragraph separately (`reWordParagraph`), found the tag in none, and printed
   all nine paragraphs of instruction to the customer verbatim. `fillLayoutDocument`
   now scans the JOINED body text (`paragraphSpans`) and distributes the
   substitution back over paragraphs (`replaceInSpan` → `paragraphChange`).
   **Never bound placeholder scanning to one paragraph again.**
2. **Paragraphs join through a line break.** Joined without separators, the
   instruction reached the model as "…входящих в состав решения;- преимущества…"
   and "…технического решенияОписания найди в базе знаний…": list items and
   words glued together. `paragraphSpans` joins with `
`, and
   `normalizePromptText` (`layout_tokens.go`) keeps list items on their own line
   while ordinary paragraphs continue through a space.
3. **A table is NEVER written by the model.** The layout asks for a table, the
   general block prompt forbade lists — the model spent its answer arguing with
   itself ("ЕКН — артикул? … Хм … Сделаю проще"). Now `layoutTableFor` builds
   the rows from offer data: positions for "предложение по материалам"
   (`layoutOfferTable`), card specifications for "сравнение вариантов"
   (`layoutCompareTable` → `compareSystemRows`). `layoutTableKindOf` picks the
   kind from the instruction text, and **compare is checked first** — a compare
   prompt usually mentions materials too. `tableMarkdown` renders the rows and
   `splitAnswerTables` + `tableXML` print a real `<w:tbl>`. The model is not
   called at all for a table.
4. **A cell with a line break tears the table apart.** Live offer item names
   contain `\n` ("Утеплитель ХPS Carbon Prof 100 мм, на глубину промерзания").
   A markdown row cannot contain `\n`: 41 positions produced **5 tables**. `tableCell`
   collapses a cell to one line and replaces `|`.
5. **A plan is not text.** The model answers with an outline ("2. h2 \"Состав
   решения\"…"), with layout descriptions ("Heading 2: …", "Ссылки (Heading 1)"),
   with reasoning ("Задача: …", "Хм…") or with questions to itself ("ЕКН —
   артикул?"). `reReasoningHeadingMarker`, `reHeadingWord`, `rePlanMarker`,
   `reSelfQuestion` and `looksLikePlan` drop those. **The plan marker is ONLY a
   layout marker (`h1`…`h6`, `Heading`, `Section`, `Intro`)** — a numbered list
   on its own is NOT a plan (user decision 2026-11-06): the composition of a
   system in an offer is exactly an honest numbered list of layers, and
   discarding it would empty the most needed section. A detected plan makes the
   block silent: an empty block is more honest than an outline.
6. **The answer volume comes from the instruction.** The live master asks for
   "не менее 4000 знаков на одну систему", while the budget stayed at
   `genDefaultMaxWords` = 150 words. `skills.RequestedWords` reads the volume out
   of the instruction (characters / 6, capped at 800 words).

### The real cause of "the model answered with its reasoning": the timeout

`llmDefaultTimeoutMS` in `llmclient.go` was **45 000 ms**, while a 4000-character
answer takes **58 seconds** (measured). The client aborted the request and the
document received whatever the model had said BEFORE answering — its reasoning.
Raised to **180 000 ms**; the provider itself allows 190 s
(`doProviderRequest` in llm-service). **When an answer looks like a draft, check
the timeout first, not the prompt.**

### Model substitution: checked, there is none

Suspicion: the DB says `deepseek-v4.1-flash`, the model says "I am ChatGPT".
Checked four ways:

- our service sends `model="deepseek-v4.1-flash"` (temporary logging in
  `internal.go`, removed after the check);
- the provider ACCEPTS the name, echoes `"model":"deepseek-v4.1-flash"` in the
  response, and rejects an unknown name with HTTP 400 — it cannot silently pick
  the first model of the list;
- the provider catalogue (`GET /api/v1/models`, 93 entries) contains
  `deepseek-v4.1-flash`, `owned_by=deepseek`, `is_old_model=false`;
- behaviourally the models differ: on one question DeepSeek produced 810
  characters, `gpt-4.1` 458, `gpt-4.1-nano` 384, in different voices.

The model says "ChatGPT" because it was trained on ChatGPT dialogues, not
because it was substituted. If reasoning returns after 0.7.3, look at the
shared-model configuration in `sbe-llm`, not at this client.

Supporting files: `docx_layout.go` (`paragraphSpans`, `replaceInSpan`,
`layoutTableFor`, `layoutOfferTable`, `layoutCompareTable`, `tableMarkdown`,
`tableCell`, `layoutAnswer`, `splitAnswerTables`), `docx_text.go`
(`looksLikePlan`, `rePlanMarker`, `reSelfQuestion`), `layout_tokens.go`
(`normalizePromptText`), `skills/prompt.go` (`tableDoc`, `asksForTable`,
`RequestedWords`), `llmclient.go` (timeout), `botapi.go` (`items`/`cards` wired
into `layoutContext`). Tests: `docx_layout_multiparagraph_test.go`,
`docx_layout_table_prompt_test.go`, `docx_layout_table_test.go`,
`docx_text_plan_test.go`.

## Document text, formatting and the .docx layout (2026-11-05)

Four rules that came out of the live offer 003/2026 and the user's requests of
that day. They overrule anything written earlier that contradicts them.

1. **One text pipeline for every writer.** Paragraph, heading, list item and
   table cell all go through `plainText` + `runsXML` (`docx_runs.go`). Never add
   a writer that formats text on its own: the moment a second path exists, the
   table prints `[[…]]` while the paragraph does not, and nobody notices until
   the customer reads it. `plainText` strips wiki links (keeping the label),
   markdown links/bold and repairs spaces lost in the note
   (`подземныхсооружений` → `подземных сооружений`). `runsXML` turns unit
   powers into real superscripts (`г/м2`, `м2`, `10-3`) - and must not touch
   `V-SL 2 мм` or `AL2`.
2. **`styles.xml` of the brand is never edited.** The brand's Normal style
   (`a0`) is Arial 11pt, left-aligned, no first-line indent - so "no alignment"
   in the export is the brand style, not a bug in the block editor. The document
   rules (justified, 1.25 cm first line = 709 twips, 1.5 line spacing, headings
   centred without indent) live in each paragraph's `pPr` (`paraOpts`). Editing
   `styles.xml` cost the page numbers in the footer once (2026-10-29) and will
   again; `TestDocxOnlyKnownStyles` guards it.
3. **A truncated model answer must never reach the customer.** Two layers:
   `blockMaxTokens` gives `words*10 + 600` (was `*6 + 400`), and
   `trimTruncatedTail` cuts the tail at the last finished sentence. The live
   defect was "…закрывает подземную часть одним кон" in the exported offer -
   a text cut by the token budget, printed as is.
4. **The uploaded layout wins over the form blocks.** When
   `offers_form.layout_docx` is set, `renderOffer` prints the manager's own
   document (`docx_layout.go`) and the blocks are not used - they stay in the
   form so the administrator can return by deleting the layout. The layout is
   an `.docx` master, never a rebuilt document: its styles, fonts, headers and
   footers are the point. Placeholders are `[Поле]`/`{{field}}` for fixed
   values and `{промт}` for model text; they are recognised inside a paragraph
   even when Word split the text across several `<w:t>`, because it always
   does. Labels in headers/footers are deliberately not replaced.
   **The prompt may be written with one, two or three braces** and may contain
   braces of its own (`{{модуль {параметр}}}`). The selector is the *value*,
   not the brace count: a doubled brace is a field only if the name is known
   (`isLayoutFieldName`), otherwise it is a prompt. Length limits are in runes
   (prompt 3000, field 200) and a too-long prompt is truncated, never left as
   a literal. **Never bound these with a byte count again** - the live defect
   of 2026-11-06 was a 364-character Russian prompt against a 400-**byte**
   limit, so it matched nothing and reached the customer verbatim.
   **The model's answer is cleaned before printing and printed as paragraphs**
   (2026-11-06, part 2): `cleanBlockText` strips Markdown (`#`, `**`, `- `),
   drops reasoning lines and lines carrying English speech (brands and article
   codes are not speech), and cuts a truncated tail. `layoutAsParagraphs` emits
   one `<w:p>` per non-empty line, copying the placeholder's `<w:pPr>` and
   `<w:rPr>` - otherwise Word shows the answer as one undifferentiated block
   because it does not render `\n` inside `<w:t>`.

Supporting files: `docx_runs.go` (text runs), `docx_card_images.go` +
`card_images.go` (images of systems and materials, `imageOf` on the `image`
node), `layout_tokens.go` (scanner and field aliases), `form_layout.go`
(upload/read/delete), `form_describe.go` + `skills/blockstructure.go`
("describe a block in words"). Plugin side: `src/ui/form-preview.ts` (live
preview), `src/ui/describe-block-modal.ts`, `src/ui/form-layout.ts`.

## Rules from the agent's own mistakes (2026-11-02)

Every line below is something an agent did wrong in this repository and had to
undo. **They are rules, not anecdotes** - read them before the first edit of a
session, they cost minutes and each one cost a broken working tree.

1. **Never rewrite a source file through a PowerShell pipeline.**
   `(Get-Content -Raw x) -replace … | Set-Content -Encoding utf8 x` both writes a
   BOM and re-encodes Cyrillic as mojibake: `jsonrepair.go` came back as
   `Р Р°Р·Р±РѕСЂ JSON…` and had to be restored from git. Use the editor tools,
   or Python with an explicit `io.open(..., encoding='utf-8', newline='')`.
   Verify after any bulk edit: first three bytes must be `7B 0A 20`, and
   `U+FFFD` count must be 0.
2. **Do not move or rename a file with a shell command and then "fix" it in
   place.** `Move-Item` + `Set-Content` lost the encoding and produced the
   mojibake of rule 1. Copy with the file tools, edit the copy, delete the
   original only after `go build` passes.
3. **A narrow replace must be checked for what else it hits.** Removing
   `isUnitColumnLabel` with a block replace also deleted the function that a
   neighbouring block needed; the build caught it, the diff did not look wrong.
   Read the surrounding 30 lines before an `edit`, and re-read after.
4. **TDD means watching the test fail for the RIGHT reason.** Three parsing
   tests passed on the broken code because they only asserted "no bad rows" and
   there were no rows at all. An assertion that is true on an empty result
   proves nothing - add a positive assertion (count, expected value) next to
   every negative one.
5. **Do not build a test fixture the service cannot actually consume.** A test
   written against a home-made xlsx builder proved nothing about the real file;
   it had to be replaced by the real one. If the defect came from a real file,
   the regression test uses a real file (anonymised, see rule 6).
6. **A real customer file in `testdata/` must be anonymised before commit.**
   `Epyur/sbe-offers` is a **public** repository. The live fixture carried an
   address, a cadastral number, a person's name and a corporate signature; all
   were replaced (`Охотская` → `Примерная`, `54:35:…` → `00:00:…`) while keeping
   the structure that causes the defect. Check with a grep over
   `xl/sharedStrings.xml` before committing.
7. **`go vet` output through a pipeline is not a result.** `go build … | Select-Object
   -Last 5` printed nothing for a failing build and looked green. Run the
   command on its own line and read `$LASTEXITCODE` (the root rule about `tail`
   over ssh, in Go form).
8. **Do not leave throwaway probe scripts inside the repository.** `cmd/tmpprobe`
   and `zz_dbg_test.go` were written to inspect a file and had to be deleted;
   build them under the temp directory, and delete before `git status`.
9. **A build artefact is not part of the change.** `offers-service/offers-stand`
   appeared in `git status`; added to `.gitignore` rather than committed. Check
   `git status --short` for untracked binaries before the first `git add`.
10. **A test that pins the old behaviour is a decision to make, not an
    obstacle.** `TestRenderOfferDocxRejectsEmptyOffer` and two E2E checks
    asserted the refusal the user asked to remove. Rewrite them to assert the
    NEW behaviour and say so in the comment; never delete them silently.
11. **Stage by intent, not by file, when one file carries two themes.**
    `callbacks.ts` got the edit-offer handlers into the "context out of a file"
    commit because the whole file was staged. The history still tells the truth
    about the content, but the commit message lies about its scope. If a file
    serves two commits, either split the hunks or admit both themes in the
    message.
12. **`git checkout <branch>` deletes the untracked-looking server folders on
    `main`.** They are `.gitignore`d there, so switching branches makes
    `offers-service/` and `offers-bot/` inert: the files are still on disk and
    still written to, but git no longer tracks edits. Do server work on
    `backend`, and re-check `git status` after every branch switch.
13. **`Invoke-WebRequest` without `-OutFile` gives a decoded string, not the
    bytes.** A 468-byte `manifest.json` reported `720 bytes` and a wrong hash
    after the self-host upload. Always download to a file and hash the file.

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
- **An offer without positions still produces a document** (2026-11-01). The
  reason is commercial: on their first weeks managers take only the descriptive
  part out of the system and price it themselves. `renderOfferDocxFull` prints a
  line saying the composition is supplied separately instead of an empty table;
  never print "Итого: 0,00" - a reader takes it as "free". `validateOffer` still
  refuses an empty offer on the REST path; do not reintroduce the check inside
  the renderer.
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
  file can attach to it. `handleBotCreateOffer` **and** `handleBotPatchOffer`
  accept `items: []`; the REST path still refuses an empty offer - do not
  "unify" these two. An empty offer is a legitimate end state: the document is
  built without positions (see "Data and documents").
- **`callback_data` is limited to 64 bytes**; system codes are Cyrillic, so
  buttons carry an index into the per-chat list, not the code.
- **A saved offer is opened for editing from the list** (2026-11-01): `edit:<id>`
  in "Мои КП", or the number typed as text. The session draft is rebuilt from
  the offer (`flow/resume.ts`), the stage jumps straight to `positions`, and the
  previous file-attach pending is dropped - it belonged to another conversation.
  Do not "restore" the context stage here: that repeat-quizzes the manager.

### Files, parsing and context

- **The parsing layer is fed by real manager files, not tidy fixtures.** A live
  old offer (2026-11-01) broke every assumption at once: cells without `r`
  refs, a "Коэффициент расхода" column between quantity and price, several
  header blocks on one sheet, section rows with a quantity but no material, and
  a column-numbering row (`2 | 3 | 4 | 5 | 6 | 7`). The fixture
  `skills/testdata/legacy-offer.xlsx` is a real file (header + both systems,
  images and external links stripped). Every fix has a test named after the
  symptom; keep it that way.
- **Address cells by `r` (`B35`), never by position.** Skipped cells are simply
  absent from the XML, so counting shifts the whole row left - the price ends up
  in the coefficient column. `xlsxColumnIndex` does the conversion; it is
  covered by `TestParseXlsxUsesCellReferences`.
- **"Сметная стоимость" is a sum, "Стоимость единицы" is a price.** Matching a
  column by the substring "стоимость" puts the block total where the unit price
  belongs.
- **`\b` in Go is ASCII-only** - already the first gotcha; it bit again in
  `filecontext_facts.go` anchors. On Cyrillic write the boundary explicitly.
- **A dot is not a sentence end for object names**: «по ул. Охотская»,
  «г. Новосибирска». Cutting the object at `\.` truncated it to «…по ул».
- **A file's item names are not object context.** Positions from a
  spec table must not become brief notes - the bot then promises "записать
  контекст", which it cannot. `contextFromParsed(isSpec=true, …)` returns nil;
  cover this when touching the attach path.
- **Model extraction never overrides the literal one.** `MergeFileContexts`
  keeps the value from `ExtractBriefFacts` and lets the model fill only what is
  missing; otherwise "Многоквартирный дом" from the file becomes "жилой
  комплекс" from the model.
- **`jsonrepair.go` lives in `model`, not in `main`** (moved 2026-11-01 so
  `skills` can use `ExtractJSON`). Both packages import it; do not copy it back.

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

### Released and self-hosted as 0.4.1 (2026-11-01 15:45 MSK)

| | value |
|---|---|
| version | **0.4.1** (`manifest.json` = `package.json`) |
| remote `backend` | `bdbf801` |
| remote `main` | `83a8e4c` (plugin part only; `offers-service/` stays deleted there) |
| `sbe-apstore-registry` | `9a9f987` |

| file | sha256 | bytes |
|---|---|---|
| `manifest.json` | `76d189e6150ad28228db0ca5f5bdbb2dd469f3fe8993744d185b8bbd90c5a5ca` | 469 |
| `main.js` | `4231c8f0db924a78bf8902b9eda59e5c4816fca305dc59c4e411a9c871020206` | 112989 |
| `styles.css` | `46f4c8801248eb87cac43b6c4ac00df86af55c0d3c0ff55401224d634f1d83d0` | 31438 |

`styles.css` unchanged (no CSS edits). Upload via `/opt/mailers/kb-selfhost.sh
sbe-offers` (it takes the hashes from the files it just copied). Checked over
https with `_t=` cache-buster: all three sums match, the served manifest says
`0.4.1`, `registry.json` reports the same three hashes, `selfHosted: true`,
`uploadedAt 2026-11-01T12:45:02Z`, `U+FFFD` count in the served stylesheet 0.
The working tree is back on `backend` and its bytes equal the published ones.

CUP news text was rewritten for this release (the old text still described
0.4.0 - the rule about rewriting it every release).

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

## 2026-11-01/02 - real manager files, object context, empty offers, editing

Live user run of the bot in Telegram, offer `003/2026`. The dialogue itself was
fine; the file was not. The manager sent an old offer in Excel (`Копия_ЖК_Поэма_
Гидроизоляция_V_SL_26_1.xlsx`) and wrote "информация в файле". The bot answered
"состава в ней не нашёл" and the object never reached the brief. Two further
requests came with the same message (see the last two subsections).

### Why the file was not understood, exactly

Reproduced on the stand against the live file:

1. **Cells were placed by order, not by address.** `parseXlsxSheetRows` appended
   values as they came, ignoring the `r` attribute; the sheet skips empty cells,
   so every row after a gap shifted left and the price landed in the wrong
   column. `TestParseXlsxUsesCellReferences`.
2. **Columns were matched by position, not by label.** "Коэффициент расхода"
   sits between quantity and price; a summary "Сметная стоимость" matched
   "стоимость" before "Стоимость единицы" did. `matchSpecColumns` now identifies
   each column by its own heading, and "ЕКН" is never a name.
3. **Only the first header was read.** The file holds two systems with their own
   header rows; `findXlsxHeaderRow` returned the first and `buildXlsxSpec` ran
   to the end of the sheet. Now every header block is parsed and the next header
   stops the previous block (`collectXlsxSpec`, third return value = rows used,
   otherwise the second block parsed twice).
4. **Section rows and column-numbering rows became positions.**
   "Устройство гидроизоляции фундамента (горизонталь)" had a quantity and a
   block sum; the row "2 | 3 | 4 | 5 | 6 | 7" numbered the columns.
   `isXlsxSectionLabel` drops both.

Result on the live file: **41 positions**, each with the right unit, quantity and
price (checked line by line against the sheet); section rows, footnotes, the
"Итого" line and the reference block are excluded.

5. **And the context had nowhere to go.** `attachedFile.Context` was filled and
   never read by anything - the brief, the sections and the form did not know
   about the file. A file that did not produce a spec produced nothing at all.

### What was added

- **Object context out of any file** (`skills/filecontext*.go`,
  `attach.go:contextWithModel`). A literal pass (`ExtractBriefFacts`) reads
  "для объекта: …" / "Объект: …", area, system titles without a model; the
  model pass adds purpose, risks and the description
  (`RenderFileContextSystem`). `MergeFileContexts` keeps the literal value and
  lets the model fill only the gaps. The result travels as `contextFields`
  and is written to the brief on confirmation - alone in `mode: brief`, or
  together with the composition in `mode: spec`.
- **Context is NOT built from a spec table**: item names are not an object
  description. This was a live bug of this same change, caught on the stand
  (`contextFromParsed(isSpec=true, …) == nil`, `TestSpecFileDoesNotBecomeBriefContext`).
- **The document builds without positions** (`docx.go`): a line saying the
  composition is supplied separately instead of a table and "Итого: 0,00".
  `handleBotPatchOffer` now accepts an empty offer like `handleBotCreateOffer`
  does; the REST path still refuses one.
- **A saved offer is reopened for editing** (`flow/resume.ts`, `edit:<id>`,
  `edit:brief`, `edit:doc`, number typed as text). The draft is rebuilt from the
  offer, the stage is `positions`, the previous attach pending is dropped.
- `jsonrepair.go` moved `main` → `model` so `skills` can use `ExtractJSON`
  (one copy, not two). `containsString` now delegates to `skills.ContainsFold`
  (dryer flagged the copy).

### Verified

`go test ./...` green (`main` 43.3%, `model` 82.4%, `skills` 86.2%), `gofmt -l`
empty, `go vet` clean; bot `tsc --noEmit` clean and **86 tests**; plugin
`tsc --noEmit` EXIT=0, `npm run build` OK, `check_bundle_strings.mjs` 0 missing;
`check_offers_openapi.mjs` **61 operations / 48 paths, schema matches** (the
schema gained `contextFields` and `context`); stand **225/225**, 20 sections;
`dryer` no new duplicates (three pre-existing remain); `crapper`: every new
function under CRAP 10, `handleDraftText` (CC 13) is the same pre-existing debt.

Live checks on the stand with the real file: 41 positions with correct prices;
`contextFields` empty for a spec table (by design); a descriptive file gives
object / area / systems / timeline, and after apply the brief holds
`object_name`, `object_purpose`, `object_area`, `planned_systems`, `timeline`.

**Released as 0.4.2** on the explicit "фиксируй" command - see
"STATE 2026-11-02 (release 0.4.2)" below for versions, hashes and the
deployment record.

### Release log 0.4.2 (2026-11-02)

1. Version `0.4.1` → `0.4.2` in `manifest.json` and `package.json` (byte-safe
   write, no BOM), CUP news text rewritten for this release and two of its lines
   added to `check_bundle_strings.mjs` so their presence in the bundle is
   asserted, not eyeballed.
2. Six commits on `backend`: `jsonrepair` moved to `model`; the Excel parsing
   fix; object context out of a file; a document without positions; the offer
   reopened for editing; the release commit (version, news, docs, stand).
3. `backend` pushed. Plugin part copied to `main` by **file**, not by
   cherry-pick: `offers-bot/` does not exist on `main` (it is `.gitignore`d
   there), and a cherry-pick of the bot commit conflicts on `routes.ts`. The
   nine plugin files were taken from `origin/backend` into a clean `main`, and
   `.gitignore` was deliberately **not** taken - on `main` it must keep
   ignoring the server folders.
4. Deployed: `git ls-files` of both server folders packed into a tar (160
   files, `jsonrepair.go` removed from the old path first, otherwise the stale
   copy breaks the build), unpacked into `/opt/mailers`, `docker compose build`
   + `up -d --force-recreate offers offers-bot`.
5. Self-hosted via `kb-selfhost.sh sbe-offers` (the script computes the sums
   from the files it just copied), three sums checked over https, registry
   entry updated and pushed.

**Mistake made in this release, recorded so the next session does not repeat
it:** `callbacks.ts` was staged whole for the "context" commit, so the
edit-offer handlers and `EDIT_KEYBOARD` landed in a commit titled "context out
of a file". The content is correct and verified; only the grouping is off. It
was not rewritten, because a history rewrite on someone else's working tree is
more risk than the benefit of a tidy split. Rule: **stage by intent, not by
file, when a file carries two themes** - or say in the commit message that it
carries both.

## Next iterations (backlog, 2026-11-02)

Ordered by what will bite the commercial department soonest. Each item says
where it hurts, so the next session can start without re-discovering it.

1. **Word documents as a context source.** Old offers arrive as `.docx` more
   often than as Excel; today `readAttachedFile` answers "пришлите .md или
   .xlsx". Removing the refusal is mostly `word/document.xml` text extraction
   (same zip walk as xlsx, `w:p` → line). **The reason it matters:** the whole
   point of 0.4.2 is "we understand the manager's files, not only tidy ones",
   and `.docx` is the most common untidy one.
2. **Old binary `.xls`.** Refusal today. A minimal BIFF reader is a large task;
   the cheap step first is a better message plus a link to the conversion
   instructions, then a decision.
3. **`.docx` and `.xls` in the plugin's own upload screen
   (`src/ui/sheet-upload.ts`)** - was not touched in this cycle and may share
   the old positional-cell assumptions. Verify with the anonymised fixture
   before promising anything.
4. **Multiple sheets.** `ParseXlsxAttachment` takes the first sheet with a
   composition and stops. A workbook with one system per sheet loses the rest.
   Merge all sheets that have a header block.
5. **Grouped / hidden rows in xlsx.** Row grouping (`outlineLevel`) is ignored;
   a sheet with subtotals inside groups can produce junk positions. Detect
   `hidden="1"` and skip, or at least warn in `warnings`.
6. **Manual overrides vs the file.** A file always wins over what the manager
   typed in chat (`MergeFileContexts` keeps the literal value). If the manager
   corrected the object by hand and then attached an old file, the old address
   returns. Decide: mark fields as "taken from file" and ask before replacing.
7. **Editing a saved offer** has no per-item delete (only "убрать последнюю")
   and does not re-ask for the client if it is empty. `flow/resume.ts` was built
   to make this easy - add the handlers there, not in `callbacks.ts`.
8. **`handleDraftText` is still CC 13** (`callbacks.ts`): a dispatcher mixing
   brief, `система`, free item, quantity and form field. Pre-existing debt,
   carried into 0.4.2 unchanged. Take it apart with a handler table.
9. **`contextWithModel` and `Complete` have no unit coverage** - network glue
   checked only on the stand. A test with a stub that returns garbage proves the
   important part: the literal extraction survives a broken model.
10. **`SKU` for materials synced from KB is meaningless** (`MAT-ПРОЧ-ОБЩ-NNNN`),
    and 1284 composition lines still have no `[[link]]`. Both are data work, not
    code - hand to the technologist.

## 2026-11-02 (part 2) - ЕКН in positions, document head from the form

Intermediate batch (P0 + P1 of the four-item report from the user). **Not yet
released to self-hosting** — the version was bumped to 0.4.3 and committed, the
service is not redeployed yet (see "STATE 2026-11-02 (0.4.3, in progress)").

### What the user reported

1. "В таблице КП отсутствует столбец ЕКН; в каталоге материалов ЕКН не
   привязались к соответствующим продуктам, в целом не было проведено
   сопоставление с базой." Reproduced on the live export
   `Коммерческое предложение 003-2026.docx`: no ЕКН column at all.
2. The exported document carried a title nobody ordered above the cover page,
   the editor placeholder «Текст с полем», the line «Условия:» with nothing
   after it, and **the whole English task prompt of the model** as body text.

### Root causes (both live, both in code)

- **The ЕКН column was read and thrown away.** `skills.ParseXlsxAttachment`
  recognised the «ЕКН» column only so that it would not be mistaken for a name
  (`isEknColumnLabel`), and never put the value into `SpecRow`. Matching to the
  catalogue then ran by name only (`specFromFile`), so a row whose name was
  written "66950 Гидроизоляционная ПВХ мембрана…" (the number ran into the name
  in the legacy fixture) could never match. `OfferItem` had no ЕКН field and the
  documents table had six columns without it.
- **The document head was printed twice**, once by `renderOfferDocxFull` and
  once by the first form block (`cover`). The template phrases were content
  saved into the form itself: the «Текст с полем» paragraph came from the block
  editor's default node, the English paragraph from a prompt block whose
  instructions were pasted in as text. Nothing in the pipeline could tell a
  draft from a finished form.

### What changed

Server (`offers-service`):

- `SpecRow.EKN` + `specColumns.ekn`: the parser keeps the number. A headerless
  layout («ЕКН | Наименование | …» with column numbers underneath) and a header
  where «ЕКН» sits one row below the labels are both recognised
  (`withEknFrom`, `findEknLayoutHeader`). Covered by
  `TestParseLegacyOfferKeepsEkn` and the existing EKN test now also asserts the
  value.
- `specFromFile` matches **ЕКН first, then name** (`matchSheetRow`,
  `materialIndexByEkn`); `specRowView` gained `ekn`, `materialEkn`, `matchedBy`
  so the manager sees what a row was matched by.
- `OfferItem.EKN` (`model/offer.go`) — snapshot at the moment of adding, filled
  by `itemForMaterial` (`primaryEkn`), by `handleSheetApply` (`itemEkn`: row
  number first, catalogue fallback) and by `applySpecFromFile` (`rowEkn`).
- `offerItemsTable` prints a «Код ЕКН» column; when no position has a number
  the column is omitted (an empty column looks like a failure, not like «нет
  номера»).
- **Document head**: `renderOfferDocxFull` no longer prints its own title, number
  and client when the form has blocks; `writeOfferHead` does it only for a form
  with no blocks. The form title is printed as a paragraph when no block carries
  it (`documentHasTitle`). The manager line is skipped when a block already has
  `{{manager}}` (`formBlocksMention`).
- **Service fields**: `{{offer_number}}`, `{{offer_date}}` (dd.mm.yyyy),
  `{{manager}}`, `{{offer_client}}`, `{{offer_object}}` are computed at render
  time (`serviceValues`, `ServiceFieldIDs`); a `form_values` value with the same
  key wins. The default cover block now uses the number and date.
- Tests: `docx_head_test.go` (title once, number and date once, form value wins,
  manager line not duplicated, empty form still titled, ЕКН column present,
  column omitted when unknown), `TestItemForMaterialCarriesFirstEkn`.

Plugin (`src`):

- `SERVICE_FIELDS` in `block-editor.ts` — the same five ids as chips in every
  text field, so the administrator does not type them by hand.
- `draftBlockHints` + `renderDraftHints` in `form-editor.ts`: the editor warns
  about lines that will read as a draft to the client (the default editor
  text, an English task prompt, «Условия:» with nothing after it). Warns — does
  not delete: the decision is the administrator's.
- The offer card has an ЕКН field next to each position name; the sheet preview
  has an ЕКН column and says whether a row was matched «по ЕКН» or «по
  названию».

Bot: `OfferItem.ekn` is carried through `resume.ts`, `callbacks.ts` and
`api.ts` — without it the bot's PATCH would erase the column on the next edit.

Stand: a new check that the number from the sheet reaches the position
(**226/226**).

### Verified

`go test ./...` green (`main`, `model`, `skills`), `gofmt -l` empty, `go vet`
clean; bot `tsc --noEmit`, build, **86 tests**; plugin `tsc --noEmit` EXIT=0,
`npm run build`, `check_bundle_strings.mjs` 0 missing (two lines of the 0.4.2
news removed, four lines of this release added); `check_offers_openapi.mjs`
**61 operations / 48 paths, schema matches** (`OfferItem.ekn` documented, sheet
row shape described); stand **226/226**.

### Still open (not done in this batch)

- ~~The "ошибки в составе" label marks any unlinked line, including real
  layers~~ - **fixed 2026-11-02** (see "part 4" below).

## 2026-11-02 (part 3) - "Добавить систему" answered "Выберите систему"

The user's report named the exact flow: open offer 003/2026, pick a system in
the offer card, press "➕ Добавить" - the Notice says "Выберите систему" and
nothing is added, although the brief shows the system was chosen.

### Root cause (measured on the live database, not guessed)

**Not one of the 375 live `commercial_propose` notes carries `system_sku`**
(`select count(*) filter (where content_md ~ 'system_sku\s*:\s*\S')` on
`mailers-kb-db-1` -> 0; the `systems` cache row has 375 cards and every `sku`
is `""`). The plugin built `<option value={system.sku || ''}>`, so every option
in the list had the same empty value and `if (!sku)` in `offers-list.ts` fired
on any pick. The same hole sat on the bot (`SystemItem.sku` from
`/bot/systems`) and on the server, where `FindSystem` could only match a code.

This is why the defect never reproduced on the stand: the stub cards all had
`system_sku` filled in. A fixture that cannot express the live data shape
proves nothing about it.

### What changed

- Server: `SystemChoice{SKU, NoteID}` (`systems_compose.go`),
  `SystemCard.Choice()` / `SystemChoice.Handle()` and `ParseSystemChoice`.
  The handle is `sys-<noteId>` for a note and the code otherwise - the prefix
  keeps a numeric code from being read as a note id. `FindSystem` matches the
  code first, then the note. Both system endpoints (`/offers/{id}/system`,
  `/bot/offers/{id}/system`) accept `sku` or `noteId`; `/bot/systems` returns
  `noteId`. `FindSystemCard` goes through `ParseSystemChoice`, so one parsing
  rule serves every caller.
- Plugin: `<option value="sys-<noteId>">`; `offersService.addSystem(choice)`
  parses the handle and sends `noteId` (the server contract is typed, not
  positional).
- Bot: `systemChoice(item)` - the code when present, the note otherwise.
- Stand: `kb-stub` gained a card **without** a code (note 100). The systems
  cache lives in the stand database, so after a stub edit run
  `delete from offers_context where mode='systems'` and restart
  `offers-local-kb-stub`, otherwise the old cache hides the change.

### Verified

`go test ./...` green, `gofmt -l` empty, `go vet` clean; bot `tsc --noEmit` and
**89 tests** (3 new on `systemChoice`); plugin `tsc --noEmit` EXIT=0,
`npm run build`, `check_bundle_strings.mjs` 0 missing (two 0.4.3 news lines
removed with the old text, three lines of this release added);
`check_offers_openapi.mjs` **61 operations / 48 paths, schema matches**
(`sku`/`noteId` documented on both endpoints); stand **231/231** (five new
checks).

## 2026-11-02 (part 4) - layers in the composition are not errors

Second half of item 1 of the user's report: *"в карточке КП у всех систем —
есть ошибки в составе"*. Measured on the stand first (the cache row dropped so
the section re-read the stub): **every** card showed the red badge, and the
reason was identical everywhere - a line without a code and without a link.

### Root cause

`resolveComposition` treated every unresolved line as a problem. On the live
cards that is 1273 lines out of 373 cards, and every one of them names a **layer
or a work**: «Подготовка основания», «Уплотнённый грунт», «Монтаж». A material
cannot appear there and must not — the line is the justification of the system,
not a forgotten item. So the badge was true for nobody, and the user learned to
ignore it; a real defect (a forgotten material, a link pointing at the wrong
note) would have been invisible.

### What changed

- `CompositionItem.Layer` (`systemcards.go`) plus `isLayerLine` /
  `reWorkWord`. Only a line with **neither** a wiki link **nor** a code can be
  a layer: once a person wrote a link or a code they claimed a material, and an
  unresolved claim stays a problem. `isLayerLine` is therefore the *only* place
  that decides, and it decides one thing.
- The layer stays in `compositionLinks` with `found: false, layer: true`, so the
  table still shows it; it just does not reach `problems`.
- `appendComposition` (`systems_compose.go`) skips layers when filling
  `problems` of "Добавить систему" - same reason, same place.
- Plugin: `CompositionItem.layer` in `types/offers.ts`; the "Системы" card says
  «есть замечания к составу» instead of «есть ошибки в составе», lists the
  layer count separately («Слои без материала: N…»), and the material cell
  reads «слой / работа» instead of a warning. The offer card's picker and the
  Notice after adding a system say «замечания» / «состав без материала».
- Bot: `(есть замечания)` in the system keyboard, nothing else to change -
  `systemAddedMessage` prints only what the server sends in `problems`.
- OpenAPI: `layer` documented on `CompositionItem`, and both `problems`
  descriptions say layers are excluded.
- Stand: `kb-stub` note 100 now has `- 2 Подготовка основания` next to the
  linked membrane; note 102 keeps «Праймер битумный» as a **forgotten
  material**, so the test proves the new rule does not swallow real gaps.
  Checks: layer is marked, layer is not in `problems`, layer is not `found`,
  forgotten material is in `problems`, and «Праймер битумный» is named after
  "Добавить систему".

### Verified

`go test ./...` green (`main`, `model`, `skills`), `gofmt -l` empty, `go vet`
clean; bot `tsc --noEmit` and **89 tests**; plugin `tsc --noEmit` EXIT=0,
`npm run build`, `check_bundle_strings.mjs` 0 missing (two lines added);
`check_offers_openapi.mjs` **61 operations / 48 paths, schema matches**;
stand **235/235** (four new checks).

### Released as 0.4.5 (2026-11-02, explicit user command "фиксируй …")

`manifest.json` = `package.json` = **0.4.5**, no `b` suffix (the whole change
is on both branches). CUP news rewritten for this release; the 0.4.4 news line
was removed from `check_bundle_strings.mjs` together with the old text.
Deployed and self-hosted the same evening - the live state is in the
"STATE 2026-11-02 evening" block above.

| | value |
|---|---|
| `backend` | `22298b5` (fix) + `122b384` (release) + `74953e8`, `5967a8d` (docs) - pushed |
| `main` | `3c4e6ee` (release) + `7016da6`, `354d99a` (docs) - plugin part copied by file from `git archive 122b384`; pushed |
| `main` vs `backend`, plugin part | identical: `git diff --quiet 122b384 origin/main -- main.js manifest.json styles.css src` exits 0 |
| server deploy | tar of `122b384` → `/opt/mailers`, `docker compose build` + `up -d offers offers-bot`, recreated 18:37Z |
| local build | `manifest.json` `094ac35d…` 468 B, `main.js` `b7ae1ee3…` 117609 B, `styles.css` `13a62f18…` 31633 B (unchanged since 0.4.4) |

**A new trap, and it will hit the next release too:** esbuild escapes
«ёлочки» in a JS **string literal** as `\xAB`/`\xBB`, not as `\u00AB`/`\u00BB`,
while the same characters inside template literals stay `\u00AB`. The checker
only knows `\uXXXX`, so a news line starting with «…» is reported missing even
though it is in the bundle (`probe` on `main.js` showed the real form). Do not
"fix" the checker by teaching it `\xAB` blindly - choose a needle without
quotes (done here: `Убрана ложная тревога в разделе систем`), or escape the
guillemets in the source.

### Trap met while writing the tests

An `if item.Layer = …; !item.Layer` guard that appended the item to `items`
*before* the assignment compiled fine, passed `go vet`, and silently produced a
copy with `layer: false` - the test caught it (`TestResolveCompositionLayerIsNotProblem`
red for the right reason). Assign first, append second. Same class as rule 4 in
"Rules from the agent's own mistakes".

## 2026-11-05 (part 2) - several systems per offer, side-by-side comparison

Version (manifest.json = package.json) = **0.6.0**, **released 2026-11-05**.
Item C of the plan: the competitor sample offers СТИЛОБАТ V1.0 and V2.0 side by
side and compares them in one table. Until now an offer could carry exactly one
system, so that document could not be produced.

### What changed

- **`Offer.System` → `Offer.Systems`** (`[]OfferSystem`, in order of choice).
  `OfferSystems.UnmarshalJSON` accepts the old shape as well: `{}`, `null`,
  `""` and a single `{"sku":…}` object all read as an empty list or a list of
  one. That is what makes the change migration-free - every live offer stores
  `system_choice` as an object today. `MarshalJSON` always writes an array and
  never `null`, so a client cannot meet an unfamiliar type on the way back.
  `Add` skips duplicates and keeps the order (the order numbers the document
  sections); `Remove` drops exactly one. `Same` compares by code first, case and
  edge spaces ignored, then by note id - the same rule `FindSystem` uses.
- **`DELETE /api/offers/offers/{id}/system`** and
  **`DELETE /api/offers/api/offers/bot/offers/{id}/system`**. Both go through
  `systemWriteHandler`/`removeSystem`; the body is the same `sku`/`noteId`
  pair. Removing a system does **not** look it up in the Knowledge Base: the
  usual reason to remove one is that it is gone from there. Removing a system
  the offer does not carry answers 404 - the button was pressed on a stale row.
- **`cardsForOffer`** maps every chosen system to its card, in choice order,
  and prints each card once even if it was chosen both by code and by note.
  A chosen system missing from the cache is skipped, the rest still print.
- **New form node `compare_table`** (`docx_compare.go`): one table, a row per
  indicator, a column per system. Rows come from the cards'
  `## Свойства и преимущества` table; the first card fixes the indicator order,
  the others are looked up by name. A missing value stays **empty** - an empty
  cell is honest, "нет данных" would be an invention. The node prints nothing
  for fewer than two systems: a one-column "comparison" misleads the reader.
- **Plugin**: the offer card lists the chosen systems with a "Убрать" button
  and says how many are chosen; `offersService.removeSystem` shares the handle
  parsing with `addSystem` (`writeSystem`). **Bot**: `removeSystem` in `api.ts`.
- **OpenAPI**: both DELETE routes documented, `Offer.systems` replaces
  `Offer.system` (63 operations / 48 paths).

### Traps met here, each one cost a red stand run

1. **A header row is not an indicator.** `isSpecHeader` knew «Показатель» but
   not «Параметр» - and «Параметр | Значение» is how live cards (and stub card
   101) label the table. The compare table therefore contained a "Параметр"
   row. Added, with `TestCompareSystemRowsSkipsParametrHeader`.
2. **A check placed before a section that rewrites the form tests the wrong
   thing.** The compare check sat in section 18; sections 19 and 20 save their
   own form, so by export time the node was gone. Moved to the end, on its own
   offer.
3. **Do not hand-type a Cyrillic label in a check.** `ТН-СтилoБат` (Latin `o`)
   went red against a correct document. The check now takes the titles from
   `/systems`.
4. **A card without specifications cannot be compared.** `TN-ROOF-CLASSIC`
   (note 102) has none; the check compares 101 and 100 instead.
5. **The systems cache hides stub edits.** The new `## Характеристики` of stub
   card 100 did not reach the stand until `delete from offers_context where
   mode='systems'` and a `kb-stub` restart - the rule from 2026-11-01 again.

### Verified

`go test ./...` green (main/model/skills), `gofmt -l` empty, `go vet` clean; bot
`tsc --noEmit` and **90 tests**; plugin `tsc --noEmit` EXIT=0, `npm run build`,
`check_bundle_strings.mjs` 0 missing; `check_offers_openapi.mjs` **63 operations
/ 48 paths, schema matches**; stand **252/252**.

New tests: `model/offer_systems_test.go` (old object shape, empty object, array,
null/empty string, array on the way out, `Same`, `Add`, `Remove`),
`docx_compare_test.go` (rows, missing value empty, one system, header rows,
`compare_table` in the document, silent for one system), three new
`cardsForOffer` cases, and five stand checks around add/remove/compare.

### Released and deployed 2026-11-05

| | value |
|---|---|
| plugin | **0.6.0** (`manifest.json` = `package.json`), no `b` suffix |
| `backend` | `e2ac383` (feature) + `92bff5c` (release) + `8a3c02f` (docs) - pushed |
| `main` | `6419c50` (plugin part) + `590c933` (docs) - pushed |
| plugin part identical | `git diff --quiet origin/backend -- main.js manifest.json styles.css src scripts` exits 0 |
| server deploy | `git archive --format=tar 92bff5c` → `/tmp` → `scp` → `tar xf` in `/opt/mailers`; backups `offers-service.bak-0.5.1-20261105`, `offers-bot.bak-0.5.1-20261105`; `docker compose build` + `up -d --force-recreate`; both containers recreated 17:01 MSK, health `ok`, bot poller started |
| self-hosting | `kb-selfhost.sh sbe-offers`; three sums checked over https with `-OutFile` and against `/registry.json` |
| registry | `sbe-apstore-registry` `8d16177`; `www/registry.json` replaced (backup `registry.json.bak-20261105-0.5.1`, sha256 `0f4ec4c4…` on both) |

| file | sha256 | bytes |
|---|---|---|
| `manifest.json` | `e43157360500c7e984799f9c3bb067ecef92b6c92f39af05101dc9ce908bd101` | 468 |
| `main.js` | `13d5e8140710ba10db0368a3523c0c43f8aba00a1107eb2b6286090b02182824` | 120601 |
| `styles.css` | `13a62f1820d50c6683d184a94e6c0c057d1ce626016924ec97716be1917a03fe` | 31633 |

**Live check on the deployed service:** all 5 live offers store `system_choice`
as `{}` (nobody has chosen a system on production yet), so the old-shape
compatibility is covered by tests, not by live data. The new DELETE routes are
registered and gated - they answer 401 without a token, as they should. The
served `/api/offers/openapi.yaml` contains 7 `delete:` operations.

`styles.css` did not change in this release (no CSS edits).

## 2026-11-05 - card composition printed; the brief reaches the model

Version (manifest.json = package.json) = **0.5.1**, **released 2026-11-05**.
Both halves of one live complaint about offer 003/2026: the document was nearly
empty in its descriptive part and the model wrote `[НУЖНО: что за объект и какие
грунтовые воды]` even though the manager had filled the brief.

### 1. Composition and characteristics of the card reach the document

`Composition` and `Specifications` were parsed from the Knowledge Base card,
fed to the model prompt and **never printed**. The document showed a paragraph
of model text and a price table. Now `renderCardSectionsWithMedia` prints, in
this order: the card image (the system section drawing), a "Состав системы"
bullet list and a "Свойства и преимущества" table. The image moved **before**
the composition: the drawing explains the list that follows it, which is how
the competitor sample is laid out.

Only the `Description` path is guarded by the accepted-copy check: a manager
who agreed a section text still gets that text instead of the card.

### 2. The brief never reached the model (`buildBlockUserPrompt`)

Three separate breaks, each enough to empty the context on its own:

- the brief lives in `offers_brief` (its own table) and **nothing** copied it
  into `form_values`; the comment in `docx.go` claimed "the plugin writes them"
  and no such code existed in the plugin, the bot or `handleSaveBrief`;
- `buildBlockUserPrompt` read `object_name`/`object_purpose`/`object_area`/
  `customer_address` **without** the `brief.` prefix, while the brief keys are
  prefixed;
- only four fields were offered to the model anyway - no goals, pains, risks,
  timeline or decision maker, so no justification could be written at all.

Now `briefFormValues` is the single place that maps a brief onto form values
(`brief.<field>`, lists joined with `; `), and `offerWithBrief` calls it inside
`renderOffer`, which every document path goes through (plugin export, bot DOCX,
offer link). Precedence: a value already in `form_values` wins - the form is
filled by a human and the brief must not overwrite his correction. Empty brief
fields write nothing. `renderBriefForPrompt` gives the model the whole brief and
accepts both key shapes, so offers created before the prefix still show their
object.

### 3. `[НУЖНО: …]` no longer reaches the customer

The marker is written by the model and read by the customer. `documentText`
drops it both when the text is stored in the cache and when it is read back, so
offers built before this fix are cleaned at print time as well. A line that
consisted only of the marker is dropped entirely, `«»` included.

### 4. The answer was cut mid-word

`maxTokens = maxWords*4` gave 800 tokens for a 200-word block, and the model
spends part of it on reasoning that is stripped afterwards. The live text ended
at "…закрывает подземную часть одним кон". `blockMaxTokens` now returns
`maxWords*6 + 400` (1600 for the default block).

### Verified

`go test ./...` green (main/model/skills), `gofmt -l` empty, `go vet` clean; bot
`tsc --noEmit` and **90 tests**; plugin `tsc --noEmit` EXIT=0, `npm run build`,
`check_bundle_strings.mjs` 0 missing; `check_offers_openapi.mjs` **61 operations
/ 48 paths, schema matches**; stand **242/242** (one check rewritten, one added).

New tests: `docx_card_test.go` (composition printed, empty heading skipped,
specifications printed as a table, image before composition),
`docx_text_brief_test.go` (brief in the prompt, both key shapes, empty fields
skipped, token budget, marker dropped from text and from legacy cache),
`brief_values_test.go` (mapping, form wins over brief, nil map).

### Test rewritten, not deleted

`TestCleanBlockTextKeepsNeedMarker` pinned the old decision ("the marker stays,
the manager reads it"). The decision changed, so the test now asserts the marker
is gone and the useful text survives - rule 10 of "Rules from the agent's own
mistakes".

The stand check "материалы системы не попали в таблицу как позиции" searched
the whole `word/document.xml`. While the composition was not printed that was a
fair test; with the new block it went red although the price table was still
untouched. It now looks only inside the composition table (`itemsTableXML`,
nesting-aware), and a second check asserts the new block exists.

**Environment note:** Docker Hub is unreachable from this machine
(`TLS handshake timeout`); images were pulled through `mirror.gcr.io` and
retagged as `postgres:16-alpine`, `node:22-alpine`, `golang:1.24`.

### Released and deployed 2026-11-05

| | value |
|---|---|
| plugin | **0.5.1** (`manifest.json` = `package.json`), no `b` suffix |
| `backend` | `505664c` (fix) + `59bce8d` (release) + `264b64a` (deploy docs) - pushed |
| `main` | `bf32d6d` (plugin part) + `a720bff` (deploy docs) - pushed |
| plugin part identical | `git diff --quiet origin/backend -- main.js manifest.json styles.css src scripts` exits 0 |
| server deploy | `git archive --format=tar 59bce8d` → `/tmp` → `scp` → `tar xf` in `/opt/mailers`; backups `offers-service.bak-0.5.0-20261105`, `offers-bot.bak-0.5.0-20261105`; `docker compose build` + `up -d`; both containers recreated, health `ok`, bot poller started |
| self-hosting | `kb-selfhost.sh sbe-offers`; three sums checked over https with `-OutFile` and against `/registry.json` |
| registry | `sbe-apstore-registry` `9a68b2e`; `www/registry.json` replaced (backup `registry.json.bak-20261105-0.5.0`, sha256 `7bbc0681…` on both) |

| file | sha256 | bytes |
|---|---|---|
| `manifest.json` | `f3fcc507c36997d3c687d84e0ff124c1fad2f9434297a56d30a80fb8e23e0e0c` | 468 |
| `main.js` | `aa97cfed5c7cbcb56ecc67763212e187f3b5864771881cdd3a448a3ebada676a` | 118812 |
| `styles.css` | `13a62f1820d50c6683d184a94e6c0c057d1ce626016924ec97716be1917a03fe` | 31633 |

**A trap of this deploy, already known but met again:** `docker compose up -d
offers` did **not** recreate the container (`Up 13 minutes` stayed). The fix is
`docker rm -f` + `up -d`, or `--force-recreate`. Check `CREATED` moved; do not
trust "Started" in the compose output.

**A second trap:** `kb-selfhost.sh` writes the file sums but leaves the
`version`/`hashes` in `/registry.json` untouched. After the upload the entry
still said `0.5.0` while the files were `0.5.1`. Both the git copy and
`/opt/mailers/www/registry.json` were updated separately, as the root rule
requires.

`styles.css` did not change in this release (no CSS edits).

## 2026-11-03 - system choice adds no positions; model reasoning kept out of the document

Version (manifest.json = package.json) = **0.5.0**, **released 2026-11-03**:
both branches pushed, `offers` and `offers-bot` redeployed (health `ok`, bot
poller started), the plugin self-hosted (three sums checked over https and in
`/registry.json`), registry `sbe-apstore-registry` `dce3e5a`. Two live defects
from the exported `Коммерческое предложение 003-2026.docx`; state and hashes are
in [HANDOFF.md](HANDOFF.md).

### 1. The system composition was added to positions from the manager's file

The user's report: *"система добавляется, но добавляемые при этом позиции не
матчатся с теми позициями, которые уже заведены в КП из пользовательского
файла, в результате в таблице у нас есть одни и те же продукты с ценами и
без"*. Measured in the live document: rows 42-51 are eight products with
quantity 1 and price 0,00 - the expanded composition of the chosen system,
sitting next to the same products priced from the manager's Excel.

**Why matching could not fix it.** `cardsForOffer`/`appendComposition` matched by
name, and the names never agree. The card says "Мембрана Logicbase V-SL"; the
sheet says "Гидроизоляционная ПВХ мембрана Logicbase V-SL 2 мм (1 слой)". A
composed item also carried `PriceMin` from the catalogue range (or zero), while
the real price lived in the sheet row - so even a successful match would have
produced two rows with two different prices, and no field said which one came
from where.

**The user's rule, now the behaviour.** System choice exists so that the offer
carries **the justification of the system, its advantages and a description of
the main products in it**. Positions are entered by the manager alone - by
uploading a file and/or by hand.

What changed:

- `assembleSystem` (`systems_compose.go`) no longer touches `offer.Items`. It
  stores the choice (`OfferSystem{SKU, NoteID}` in `offers_offer.system_choice`)
  and returns `{offer, system, title, composition}`. `added`, `missingPrice` and
  `problems` are gone from the response; `itemForMaterial` and
  `appendComposition` were deleted with them (`primaryEkn` stays - `upload.go`
  still uses it).
- `handleSaveOffer` keeps the stored choice when the incoming offer carries an
  empty one (`CASE WHEN $7::jsonb = '{}' ...`). Without this a plain save from
  the plugin or the bot - neither sends the field - would have wiped it.
- `cardsForOffer` (`docx_text.go`) now selects the card **by the stored choice**,
  not by name matching. This also fixes a second bug found while reading the
  code: a system chosen for its justification but with no matching position never
  reached the document at all, so the descriptive part could not be printed
  before prices were entered - which is exactly the order the user described.
- Plugin: `Offer.system`, `ComposeSystemResponse{system, title, composition}`;
  the offer card block is titled "Система в КП:", the button reads "✔ Выбрать",
  it preselects the current choice and says in a line under it that positions
  are not added and the description will go into the document.
- Bot: `SystemAddResponse{title, composition}`; `systemAddedMessage` says the
  system is chosen, how many main products it has, and that positions were not
  added.
- Migration: `ALTER TABLE offers_offer ADD COLUMN IF NOT EXISTS system_choice
  JSONB NOT NULL DEFAULT '{}'` (in `main.go`, next to the CREATE TABLE).
- OpenAPI: both endpoints rewritten, `Offer.system` documented.

### 2. The LLM block contained the model's reasoning, in English

The user's report: *"описание системы от ЛЛМ в КП выглядит больше как процесс
рассуждения, нежели ответ"*. The document paragraph was the whole chain of
thought: "The task: write a block…", "Brief info: no explicit object
description given…", "Let me think about what's actually known…", "Word limit
150…", interleaved with Russian notes about the composition and ending in
"Let me draft / I need to write…". The prompt's own words ("Under 150 words",
"Only numbers from the calculation") matched the document verbatim, which is how
it was traced to `RenderBlockSystem`.

**Two causes, both fixed.**

- `skills.RenderBlockSystem` forbade nothing about reasoning. It now says the
  text is for the customer, must be in Russian, and has a "ЧЕГО НЕ ДЕЛАТЬ"
  section: no retelling of the task, no "Let me think"/"I should", no remarks
  about the text itself, and a missing fact is one short `[НУЖНО: …]` marker
  instead of a discussion.
- `cleanBlockText` (`docx_text.go`) stored the reply verbatim. It now runs
  `stripReasoning` over it. **The rule was rewritten against the real reply, not
  guessed:** the first version cut a "reasoning prefix" and passed its own
  tests, and the live text showed reasoning is *interleaved* with Russian lines
  and also starts mid-sentence ("… даёт герметичный контур... hmm, "сварка" is
  not in the brief"). The final version drops reasoning lines, lines with more
  Latin than Cyrillic, and reasoning tails inside a line, then collapses blank
  lines. Russian text is never rewritten.

Verified on the live reply itself (temporary test with `OFFERS_LIVE_LEAK`
pointing at the extracted paragraph, not committed because the public repo must
not carry a customer's object text): the output is six readable Russian bullets
and the `[НУЖНО: …]` marker, with none of `The task`, `Let me`, `hmm`, `Brief
info`, `Word limit`, `Header:`.

### Verified

`go test ./...` green (main/model/skills), `gofmt -l` empty, `go vet` clean;
bot `tsc --noEmit` and **90 tests**; plugin `tsc --noEmit` EXIT=0,
`npm run build`, `check_bundle_strings.mjs` 0 missing; `check_offers_openapi.mjs`
**61 operations / 48 paths, schema matches**; stand **241/241**.

New stand checks in section 18: positions do not change, the response has no
`added`, the choice is stored, `title`/`composition` are returned, the bot path
behaves the same, the chosen system's description reaches `word/document.xml`
(unpacked, not searched in the compressed bytes) and its materials do **not**
appear as positions.

### Tests pinned to the old behaviour, and what happened to them

Following rule 10 ("a test that pins the old behaviour is a decision to make,
not an obstacle"): `TestItemForMaterial*` and `TestAppendComposition*` were
**replaced** by `TestCountCompositionMaterials*`, `TestComposeSystemResponseHasNoPositions`
and the four `TestCardsForOffer*` selection tests; `TestItemForMaterialCarriesFirstEkn`
became `TestPrimaryEknTakesFirstNonEmpty` (the function is still used by
`upload.go`). In `system.test.ts` the two `systemAddedMessage` tests asserting
"added N positions" were rewritten to assert the new message. E2E assertions
"ДОПИСАНЫ, а не заменены" / "бот дописал хотя бы одну позицию" became "позиции
НЕ изменились" / "бот не добавил позиций".

### Trap met while writing `stripReasoning`

`` in Go is not Cyrillic-aware (known gotcha) **and it also matches between a
Latin letter and a Cyrillic one**: `let me` matched "Let me сказать прямо",
and the guard deleted a legitimate sentence. The final code does not rely on
`` after English words - it decides per line by counting Cyrillic vs Latin
letters (`mostlyCyrillic`), which is what actually separates the model's notes
from the offer text.

## ## STATE 2026-11-02 evening (0.4.5: released - plugin self-hosted, service deployed)

This block is the one to read first; older STATE blocks below are history.
Short version is in [HANDOFF.md](HANDOFF.md).

| | value |
|---|---|
| plugin | **0.4.5** (`manifest.json` = `package.json`), no `b` suffix |
| `backend` | `0d45ec2` (release 0.5.0) - pushed |
| `main` | `bc656be` (release 0.5.0, plugin part copied from `git archive 0d45ec2`) - pushed |
| **self-hosting** | **0.4.5, uploaded 2026-11-02 18:32Z**: `manifest 094ac35d…`, `main b7ae1ee3…`, `styles 13a62f18…`; all three verified over https (downloaded to a file and hashed) and match `/registry.json` (`selfHosted: true`, `uploadedAt 2026-11-02T18:31:56Z`) |
| registry | `sbe-apstore-registry` `4b918f5`; server copy `www/registry.json` replaced (previous `registry.json.bak-20261102-0.4.4`, sha256 identical to the local file, `a3b82913…`) |
| **live service / bot** | **0.4.5**, recreated 2026-11-02 18:37Z, health `ok`, bot poller started; backups `offers-service.bak-0.4.4-20261102`, `offers-bot.bak-0.4.4-20261102` |
| working tree | clean; checked out on `backend`; stand binary = current code |

### Verified live after the deploy (not assumed)

- Container files equal the working tree: `md5sum` of `systemcards.go`,
  `systems_compose.go`, `callbacks.ts`, `build.ts` on the server = the local
  ones, and `git diff 122b384 -- offers-service offers-bot` is empty.
- `GET /api/offers/systems/refresh` then `GET /api/offers/systems` with the
  owner's JWT: **375 systems**, `error:""`, **51 rows carry `layer: true`**,
  **0 layers leak into `problems`**.
- Old cards still name real gaps: e.g. ТН-ДОРОГА Труба Водопропускная keeps
  «Песок» and «Фундаментное основание» in `problems` while
  «Гравийно-песчаная подготовка» is a layer. The fix does not silence what a
  technologist has to fill in.

**Trap met during this deploy:** the section's cache (`offers_context`,
`mode='systems'`) was written before the deploy, so the first read showed
**0 layers** - the server was right, the cache was old. The cache stores cards
without `Layer`, so `refresh` rewrites it. Same rule as 2026-11-01: after a
deploy that changes card shape, press (or call) «Перечитать из Базы знаний»
before concluding that the fix did not work.

## STATE 2026-11-02 (release 0.4.4 - deployed and self-hosted)

| | value |
|---|---|
| plugin | **0.4.4** (`manifest.json` = `package.json`), no `b` suffix: `main` and `backend` match |
| `backend` | `2c192b0` (fix) + documentation commit - pushed |
| `main` | `c59d059` - plugin part including the 0.4.3 changes that had never left `backend`; pushed |
| `sbe-apstore-registry` | `31b759e`; the server copy `www/registry.json` was replaced too (backup `registry.json.bak-20261102-0.4.2`) |
| live containers | `mailers-offers-1`, `mailers-offers-bot-1` rebuilt from `git archive 2c192b0`, recreated 2026-11-02, health `ok`, bot poller started |
| self-hosting | `kb-selfhost.sh sbe-offers`, three sums checked over https and against `/registry.json` |

| file | sha256 | bytes |
|---|---|---|
| `manifest.json` | `10b7f0c57cd5d717c052f6647394b60ed3c2f28cb706782a045b968874feff53` | 468 |
| `main.js` | `b9295e63f3904305c97dfbaa016b5416fcf652166beb9475ae81ebd2351884a3` | 115875 |
| `styles.css` | `13a62f1820d50c6683d184a94e6c0c057d1ce626016924ec97716be1917a03fe` | 31633 |

`styles.css` changed for the first time since 0.3.0: the 0.4.3 CSS edits
(`tn-offers-ekn`, form chips) had never been shipped before this release.

The "Добавить систему" item of the user report is closed: every live card
lacks `system_sku`, and the choice now travels by note id.

**Two traps of this release** (both cost time):

1. **Switching `backend` → `main` does not reset the working tree to `main`.**
   After `git checkout main`, `src/ui/offers-list.ts` still held the `backend`
   version (with the EKN field) while `src/types/offers.ts` came from `main` -
   `tsc` failed on `OfferItem.ekn`. The port must copy **the whole `src`
   folder** from `origin/backend`, not a hand-picked list of files.
2. **`git archive` on Windows applies the CRLF filter to the file list**, and
   the tar that reached the server carried paths with a trailing `\r`; the
   extraction silently overwrote nothing. Use `git archive --format=tar
   <commit> offers-service offers-bot` (it packs the commit's bytes) and
   `tar xf` on the server, then compare `md5sum` with the local files - done
   here, all four compared files matched.

Also: `kb-selfhost.sh` writes the file sums into `registry_file_overrides`
but does **not** touch `www/registry.json`; the registry update is a separate
step (git copy in `sbe-apstore-registry` and the server file).

## STATE 2026-11-02 (0.4.3b committed, not deployed)

Working tree clean on `backend`, HEAD `3abd91f` (pushed). Three commits:
`1e4730e` (document head from the form block + service fields), `367204c`
(EKN in positions, in the document table and in sheet matching), `3abd91f`
(version 0.4.3, CUP news, docs, bundle-string checks). The suffix `b` is
implied while `main` stays at 0.4.2.

**Live service, registry and self-hosting are still 0.4.2** - the deploy and
self-host is a separate command; before it, refresh HANDOFF.md and this block
with the new hashes.

### What is left from the user report of 2026-11-02

The report had four items; P0 (document head) and P1 (EKN) are done.

1. **"Добавить систему" does not attach (first half of item 1).** ~~NOT
   reproducible on the stand~~ - **FIXED in 0.4.4** (see "2026-11-02 (part
   3)" above): the live cards carry no `system_sku`, the choice now travels by
   note id. The second hypothesis (`editor` editing someone else's offer gets
   403) was not the cause.
2. **"ошибки в составе" (second half of item 1).** Does NOT block the system
   - measured on all 373 live cards: 1273 composition lines without a link,
   all of them real layers (concrete, gravel, membrane) or general works.
   Reword in `resolveComposition` (`systemcards.go`) and
   `systems-editor.ts`: "слои без материала", no red badge.
3. **DOCX layout template upload (item 2).** Not started. Load a `.docx`
   master into the form editor with placeholders (`{{object_name}}`,
   `{{manager}}`) and content markers (`{{#blocks}}`, `{{#positions}}`,
   `{{#per-system}}`); the service fills the master and keeps the brand
   styles. Open decision: own blank vs the corporate `standard.docx`.
4. **Repeatable form block "per system" (item 2).** Not started. A new
   `repeat` node with `source: "systems"` and child nodes: printed once per
   system of the offer (`cardsForOffer`).

## STATE 2026-11-02 (release 0.4.2)

| | value |
|---|---|
| plugin | **0.4.2** (`manifest.json` = `package.json`) |
| remote `backend` | `854df42` |
| remote `main` | `99211fe` (plugin part only; server folders `.gitignore`d there) |
| `sbe-apstore-registry` | `9fc4c76` |
| live containers | `mailers-offers-1` (image `mailers-offers`), `mailers-offers-bot-1` recreated 2026-11-02 04:53 UTC, health `ok` |
| deploy | `tar` of the 160 tracked files → `/opt/mailers`, backups `offers-service.bak-0.4.1-20261102`, `offers-bot.bak-0.4.1-20261102` |

Self-hosting, checked over https with `_t=` cache-buster, downloaded with
`-OutFile` so the bytes are compared, not a decoded string:

| file | sha256 | bytes |
|---|---|---|
| `manifest.json` | `5c283e0cf6bb0847aa24a436304aa0660258a69f5c33557a1f31bc054ce21652` | 468 |
| `main.js` | `9bbf1000c4de06c34bb60ee7215ff21655eb03e87a91341195cb97054241cff4` | 113597 |
| `styles.css` | `46f4c8801248eb87cac43b6c4ac00df86af55c0d3c0ff55401224d634f1d83d0` | 31438 |

`styles.css` unchanged since 0.3.0 (no CSS edits). Served manifest reads
`version 0.4.2`. `/registry.json` reports the same three hashes,
`selfHosted: true`, `uploadedAt 2026-11-02T04:55:42Z`.

**A trap met during this release, worth knowing:** `Invoke-WebRequest | .Content`
returns a *decoded string*, and the byte count becomes 720 for a 468-byte file.
The hash then differs and looks like a failed upload. Download to a file
(`-OutFile`) and hash the file.

### Verified after deploy (live, 2026-11-02)

Service health `{"status":"ok"}`, `offers-service listening on :3000`,
`registerApp: offers registered`, Knowledge Base reachable; bot poller started
and its token check listens on `:3000`. No errors in either log.

What went into 0.4.2: parsing of real manager files (address cells, columns by
label, several header blocks, section rows), object context out of any file
(`contextFields` → brief, literal over model), a document without positions,
an offer reopened for editing from "Мои КП". Details - the entry
"2026-11-01/02" above.

## STATE 2026-11-01 (release 0.4.1 - supersedes the 0.4.0 block below)

Current version is **0.4.1**: the "Системы" section reads a cache and refreshes
on an explicit action (see the entry above). Everything below about 0.4.0 still
describes the system correctly except the two lines that said `GET
/api/offers/systems` reads the Knowledge Base live - it does not any more.

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
| `sbe-core` | `eea98f6` (`backend`) - `src/utils/files.ts` added (unchanged in 0.4.1) |

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

**These are the 0.4.0 numbers, kept as a record.** The current ones are in the
0.4.1 entry above (`main` 44.6%, 61 operations / 48 paths).

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
8. **The "Системы" cache is refreshed only by the button** (0.4.1, by the
   user's direction). Editing a KB card does not reach the section by itself -
   expect "изменил карточку, а в системе старое" and answer with "Перечитать из
   Базы знаний". Do not "fix" it by restoring the live read.

### Where to look first when something breaks

| symptom | start here |
|---|---|
| «Ключ доступа недействителен» on a screen | the screen is reading another service live on every open; `offers_context` cache + explicit refresh, and `KBClient.get` retries 401 once. Check `docker logs mailers-kb-1` for `отказ 401: ... — <reason>`: that line names the cause, the client does not. |
| bot asks positions immediately | `callbacks.ts` `startDraft` must create the offer and call `beginBrief`; `draft.stage` must be `context` |
| «в предложении нет позиций» on save | `finishDraft` PATCHes the existing offer; an empty `items` from the sheet means `attach/apply` did not run |
| a block is missing from the DOCX | model down (`prompt_text` is silent by design), image key not in the store, or `showInClient` off |
| links absent in the document | the card has no `## Ссылки` section, or `cardsForOffer` did not match the system to the composition |
| old prompt text in a new document | `form_values["gen.section"]` hash matched; adding a system drops it, editing an item directly does not |
## 2026-11-06 (part 2) - the model's answer reached the customer raw

Version (manifest.json = package.json) = **0.7.2**. The live export of offer
003/2026 (11:21, after the 0.7.1 deploy) showed that the prompt was now
recognised, but its *answer* went into the document untouched. Three defects,
all visible in one paragraph.

### 1. Markdown headings printed as `##`

The layout instruction says "помечай заголовки второго уровня в разметке
ворда". The model read that as Markdown and wrote fourteen lines of
`## Отмостка`. `fillLayoutParagraph` prints text as is, so the customer got the
hashes. `stripMarkdownHeadings` now removes `#{1,6}`, `-`/`*`/`+` bullets and
`**`/`__` while keeping the words, and drops a line left with nothing but
punctuation (the live answer ended with a bare `##.`).

The system prompt also gained an explicit rule: a heading is a plain line, no
markup. The code rule is the guarantee; the prompt merely stops producing it.

### 2. An English sentence survived inside Russian text

`Given the risk, I'll frame group 3 as "Конструкция отмостки и примыканий."`
reached the document. `mostlyCyrillic` counted **letters**, and the Russian in
the quotes outnumbered the Latin in the English clause. It now counts **words**
of English speech, recognised by an explicit list of function words (`the`,
`and`, `for`, `with`, `from`, `given`, `risk`, `group`, …). A first attempt
sized words instead ("shorter than five letters is a brand") and failed on the
live line: `risk`, `frame`, `group` are ordinary English words of brand length.

**Order matters inside `isLatinBrandWord`:** the function-word list is checked
**first**. Checking "starts with a capital letter" first passed `Given` and
`The` as brands and let the whole line through.

**Order matters in `stripReasoning`:** the reasoning tail is cut **before** the
language test. The useful half of `ПВХ мембрана — …; сварка швов… hmm, "сварка"
is not in the brief` carries all its Latin in the tail, so testing the whole
line dropped a perfectly good sentence.

### 3. The whole answer became one paragraph

Every line of the answer was written into a single `<w:t>`. Word does not
render `\n` inside `<w:t>`, so the document showed one continuous block with no
paragraphs, no headings, no indents — "problems with the formatting" in the
user's words.

`layoutAsParagraphs` now emits one `<w:p>` per non-empty line, copying the
placeholder's `<w:pPr>` (justification, 1.25 cm first line, style) and its
`<w:rPr>` (font) onto every one. The layout's formatting therefore applies to
the whole answer, not just its first line.

### Tests

- `docx_layout_answer_test.go` - the live answer verbatim: no `##`, no English
  line, no punctuation-only line, paragraphs preserved.
- `TestLayoutPromptPrintsParagraphs` - three-line answer gives three `<w:p>`,
  each carrying `jc=both` and `firstLine=709`.
- `docx_layout_e2e_test.go` - `cleanBlockText` then `renderLayoutDocx` on the
  real answer: no markup, no English, at least five paragraphs. Runs against a
  master layout when `OFFERS_LIVE_LAYOUT` is set.
- `layout_live_test.go` gained `TestLiveExportIsClean` (`OFFERS_LIVE_EXPORT`):
  **red on the current export**, which is the defect, and green after the fix.

### Verified

`go test ./...` green (`main` 50.3 %, `model` 82.6 %, `skills` 81.9 %),
`gofmt -l` empty, `go vet` clean; `crapper` no function at or above CRAP 10
(`mostlyCyrillic` was 11 and was split into `countCyrillicLetters` +
`countSpeechLatinWords`); `dryer` no new duplicate pairs.

### Trap met in this change

Writing `manifest.json`/`package.json` with a **bare relative path** after a
`cd` that the shell tool does not honour (`workdir` is the parameter, `cd`
inside the command does not persist) created two empty files at the plugins
root and left the real ones untouched. `npm run build` then failed with
`../package.json: Unexpected end of file in JSON`, because esbuild walks up
from the entry point and found the empty file. The two files did not exist
before (both were *created* at the moment of the mistake), so deleting them
restored the tree. **Use absolute paths for every file write**, and after a
bulk edit check `git status` before building.

### Released and deployed 2026-11-06

| | value |
|---|---|
| plugin | **0.7.2** (`manifest.json` = `package.json`), no `b` suffix |
| `backend` | `86130a4` (release) + `18dedf5` (docs) — pushed |
| `main` | `0e68b9e` (plugin part) + `0c1b144` (docs) — pushed |
| plugin part identical | `main.js`/`manifest.json`/`styles.css` have the same SHA-256 on both branches |
| server deploy | `git archive --format=tar 86130a4 offers-service offers-bot` -> `scp` -> `tar xf`; backups `offers-service.bak-0.7.1-20261106`, `offers-bot.bak-0.7.1-20261106`; `md5sum` of the three changed Go files matched before the build; containers recreated 2026-11-06 12:22 MSK, health `ok`, bot poller started |
| self-hosting | `kb-selfhost.sh sbe-offers`; three sums checked over https (`-OutFile` + hash of the file) |
| registry | `sbe-apstore-registry` `8c56195`; `www/registry.json` replaced separately (backup `registry.json.bak-20261106-0.7.1`, sha256 `6e186098…`); **`version` set by hand** - the script writes sums only |
| sums | `manifest 6d032c19…` 468 B, `main 21301b26…` 145047 B, `styles 50358b4a…` 34072 B (CSS unchanged since 0.7.0) |

**Not done on purpose: the CUP news.** `data.json` still reads
`lastAnnouncedVersion: 0.7.1`. `announceOnce` runs in `onload`, and hot-reload
does not watch files on Windows, so it publishes on the next Obsidian restart
or when the plugin is toggled off/on in Settings.

## 2026-11-06 - long model prompts in the .docx layout were silently lost

Version (manifest.json = package.json) = **0.7.1**. The user's own layout for
offer 003/2026 carried the descriptive block as a 364-character instruction
inside **double** braces. Nothing was replaced: the instruction text went to
the customer verbatim, braces included.

### Root cause, measured on the live file

`layout_tokens.go` recognised exactly two shapes:

- `{…}` — a prompt, but with a limit of **400 bytes** in the regex;
- `{{…}}` — only ever a form field name, on the assumption that a doubled
  brace could only mean `{{object_name}}`.

The live instruction is 364 *characters* = **823 bytes** of Russian, and it is
not a field name. So it matched neither rule, and `scanLayoutTokens` returned
no token at all — `fillLayoutParagraph` found nothing to replace and printed
the paragraph as it stood. The limit was a unit mistake, not a threshold to
raise slightly: the regex counts bytes, the person counts letters.

### What changed

- `reLayoutPrompt` now accepts **one to three braces**, in any mix, and does
  not forbid braces inside the instruction (`{{модуль {параметр}}}` parses as
  one prompt). Go's RE2 has no backreferences, so the pattern allows an
  arbitrary nesting depth up to three braces at each edge and `promptInner`
  strips whatever braces ended up adjacent to the edges.
- **The value decides, not the brace count.** `reLayoutBraceField` still runs
  first, but a doubled-brace match is accepted as a field only when
  `isLayoutFieldName` finds it in the form values, in `fieldAliases` or in
  `builtinFieldAliases`. Everything else falls through to the prompt pass.
  With `values == nil` (the admin's "what did the system see" view) every
  doubled brace counts as a field, as before.
- **Limits are in runes and generous**: `layoutFieldMaxRunes` 200,
  `layoutPromptMaxRunes` 3000. A prompt over the limit is truncated, not
  dropped — leaving a literal in the document is worse than a long instruction
  that lost its tail. The field limit is fed into the regex through
  `fmt.Sprintf` so the two cannot drift.
- `layoutPlainText` (`form.go`) now joins paragraphs **with no separator**. It
  used to insert `\n` between `<w:t>` groups, which broke any prompt spanning
  two paragraphs and made the admin view disagree with what gets printed.

Plugin: the layout card names all three accepted forms and states that the
prompt list below is "how this pass understood the document".

### Tests

- `layout_tokens_test.go` rewritten for the new contract: doubled-brace prompt
  (the live case, full text compared), tripled braces, nested braces, runes
  vs bytes (800-character prompt), doubled-brace field by id and by Russian
  alias, field limit boundary.
- `layout_user_docx_test.go` reproduces the live paragraph exactly, including
  Word splitting it into five runs around two `{` and two `}` — the shape that
  made the old code fail — and asserts the model receives the full instruction
  and the document contains neither the instruction text nor a stray brace.
  `TestLiveLayoutRendersPrompt` runs the same check against the real file when
  `OFFERS_LIVE_LAYOUT` points at it (verification only; the customer document
  is never committed).

### Verified

`go test ./...` green (`main` 49.5 %, `model` 82.6 %, `skills` 82.3 %),
`gofmt -l` empty, `go vet` clean; live layout test passes on
`Коммерческое предложение 003-2026.docx` (439 runes of instruction, model text
printed, no literal left); plugin `tsc --noEmit` EXIT=0, `npm run build`,
`check_bundle_strings.mjs` 0 missing; `crapper` every new function under CRAP 10
(`scanLayoutTokens` 5.0, `isLayoutFieldName` 6.0, `promptInner` 3.0,
`clampRunes` 3.3); `dryer` no new duplicate pairs.

### Trap met in this change

The first regex draft used `{1,3000}` and **panicked at `MustCompile`** — Go
does not accept a repeat count above 1000. The length limit therefore lives in
code (`clampRunes`), not in the pattern, and the field limit that *can* be
interpolated is interpolated rather than typed twice.

### Released and deployed 2026-11-06

| | value |
|---|---|
| plugin | **0.7.1** (`manifest.json` = `package.json`), no `b` suffix |
| `backend` | `1dbc10f` (release) + `cea457e` (docs) — pushed |
| `main` | `bdfb65c` (plugin part) + `0294a18` (docs) — pushed |
| plugin part identical | `main.js`/`manifest.json`/`styles.css` have the same SHA-256 on both branches |
| server deploy | `git archive --format=tar 1dbc10f offers-service offers-bot` → `scp` → `tar xf` in `/opt/mailers`; backups `offers-service.bak-0.7.0-20261106`, `offers-bot.bak-0.7.0-20261106`; `md5sum` of the three changed Go files matched the local ones before the build; `docker compose build` + `up -d --force-recreate`, containers recreated 2026-11-06 11:08 MSK, health `ok`, `registerApp: offers registered`, bot poller started |
| self-hosting | `kb-selfhost.sh sbe-offers`; three sums checked over https (`-OutFile` + hash of the file) |
| registry | `sbe-apstore-registry` `55b81d7`; `www/registry.json` replaced separately (backup `registry.json.bak-20261106-0.7.0`, sha256 `57f99fca…`); **`version` fixed by hand** — the script writes sums only and left `0.7.0` next to 0.7.1 files, the same trap as 0.5.1 |

| file | sha256 | bytes |
|---|---|---|
| `manifest.json` | `c4130e570b6f2447a27febf1b955460d47dc77052a45552d40cb76d4b08248d5` | 468 |
| `main.js` | `5f2bad5993d30b13888688ef1cc3c5118b9d357e8699e824f23f995ee8ec999a` | 145003 |
| `styles.css` | `50358b4a2ecd849e48b4ee74f783218c8345ca8c049f5f4b8e70d90826c6d152` | 34072 |

`styles.css` did not change in this release (no CSS edits) — same sum as 0.7.0.

**Not done on purpose: the CUP news.** `data.json` still reads
`lastAnnouncedVersion: 0.7.0`. `announceOnce` runs in `onload`, and hot-reload
does not watch files on Windows (`watchNeeded = !Platform.isWin`), so it
publishes by itself on the next Obsidian restart or when the plugin is toggled
off/on in Settings. Do not call the endpoint by hand.

**SSH alias.** `scp`/`ssh` to the bare address `root@176.12.69.109` answers
`Permission denied (publickey)` with the default key; the working alias is
`epyur` from `~/.ssh/config` (`IdentityFile ~/.ssh/id_ed25519_20261103`).

## STATE 2026-11-05 (part 3) - the document became readable; the form became visual

Version (manifest.json = package.json) = **0.7.0**, **not committed and not
deployed yet**. Everything below was verified on the stand; the release is
waiting for the user's command.

### The user's report (live offer 003/2026)

"В тексте есть оборванные фразы/слова, отсутствует выравнивание текста. Текст
заметки вставляется с ошибками (теряются пробелы), вставляются вики-ссылки.
Настройка блоков крайне не дружелюбная. Хочу загрузить макет ТКП с [] и {}.
Индексы в единицах измерения должны отображаться правильно. Нужно проработать
вставку картинок систем и материалов."

### What was actually broken (measured, not guessed)

- **The text was printed raw.** `paragraphXML` put the note's markdown straight
  into `<w:t>`: `[[Геотекст…]]` around every composition item, `**Область
  применения:**`, «подземныхсооружений», «г/м2». One function served the
  paragraph, another the table cell - the table path and the paragraph path
  drifted, and the degree was nowhere.
- **"No alignment" was the brand style.** `styles.xml` → `a0` = Arial 11pt,
  left, no indent. The document rules (justified, 1.25 cm, 1.5 spacing) were
  written in `stylesXML` at the top of `docx.go` but that constant had been
  dead since 2026-10-29, when the brand's styles were adopted. Nobody noticed
  because the rules lived in a string that was no longer used.
- **The cut phrase was the token budget**, not the model: `maxWords*6 + 400`
  with reasoning spent inside the same budget.
- **Image keys were never resolved.** `SystemCard.ImageKey` existed and was
  always empty (`ParseSystemCard` returns `ImageName`, not the attachment key);
  materials had a `photo_key` column nobody filled. So the `image` node could
  only ever show a hand-picked file.

### What changed (server)

`docx_runs.go` - `plainText` (wiki/markdown links, bold, glued words) and
`runsXML` (superscript degrees), applied by every writer; `paraOpts` gained
`Align`/`NoIndent` + `spacingXML`/`alignXML`/`indentXML` (document rules per
paragraph, brand style untouched); `paragraphsXML` splits a card description
into real paragraphs (the `\n` inside one `<w:t>` was invisible in Word).
`blockMaxTokens` → `words*10 + 600`; `trimTruncatedTail` cuts at the last
finished sentence. `card_images.go` resolves `ImageName` → attachment key and
fills `photo_key` on material sync; `docx_card_images.go` resolves `imageOf`
(`system`/`material`) against the offer. `docx_layout.go` + `layout_tokens.go`
+ `form_layout.go` print the manager's `.docx`; `form_describe.go` +
`skills/blockstructure.go` turn a description into nodes.

### What changed (plugin)

`form-preview.ts` (live document preview below the blocks), `describe-block-
modal.ts` (!"Опишите блок словами"), `form-layout.ts` (layout card),
`block-editor.ts` (image source: system / material / file). New `compare_table`
in the `RichNode` type (it existed on the server since 0.6.0 but not in the
TypeScript union - the preview would not compile against it).

### Verified

`go test ./...` green, `gofmt -l` empty, `go vet` clean; plugin `tsc --noEmit`
EXIT=0, `npm run build`, `check_bundle_strings.mjs` 0 missing; stand **264/264**
(21 sections, the new one prints by the layout and checks tags and formatting).
`crapper`: every new function under CRAP 10 (`splitGluedWord` split into
`gluedSecondWordCut`/`gluedJoinerCut`, `describeBlock` into
`parseDescribedBlock`, `pickImageAttachment` into `firstImage`,
`resolveNodeImage` into per-source helpers). `dryer`: two new pairs reported,
both false positives (`escapeXML`/`unescapeXML` are inverses;
`persistOfferFormValues`/`persistOfferSystem` write different columns).

Live check on a real rendered DOCX (stand, real card text): wiki link gone,
"подземных сооружений", superscript `г/м²`, body `jc=both ind=709`, "Итого"
right with no indent, layout tags replaced, centering and bold of the layout
kept, no leftover `[...]`/`{...}`.

### A test found a real defect before the release

`TestLayoutValuesFormWins`: `layoutValues` let `serviceValues` overwrite what
the administrator had typed into the form. Fixed - service values fill only
empty places, the same precedence the form blocks use.

### Released and deployed 2026-11-06

| | value |
|---|---|
| plugin | **0.7.0** (`manifest.json` = `package.json`), no `b` suffix |
| `backend` | `9181978` (release) - pushed |
| `main` | `957bfbf` (plugin part copied from `backend`, whole `src`/`scripts`) - pushed |
| plugin part identical | build sums are the same on both branches |
| server deploy | `git archive --format=tar 9181978 offers-service offers-bot` → `scp` → `tar xf` in `/opt/mailers`; backups `offers-service.bak-0.6.0-20261105`, `offers-bot.bak-0.6.0-20261105`; `md5sum` of eight new files matched the local blobs before the build; `docker compose build` + `up -d --force-recreate`, containers recreated 07:38 UTC, health `ok`, bot poller started |
| self-hosting | `kb-selfhost.sh sbe-offers`; three sums checked over https (`-OutFile` + hash of the file) |
| registry | `sbe-apstore-registry` `7caf6c9`; `www/registry.json` replaced (backup `registry.json.bak-20261106-0.6.0`, sha256 `6cb291a9…`); local and server copies equal (`57f99fca…`), 22 entries |
| migration | checked on the live DB: `offers_form.layout_docx`, `layout_name` and `offers_material.photo_key` all present |

| file | sha256 | bytes |
|---|---|---|
| `manifest.json` | `9b3a3724d38543ec21e769d152e6cbc8225c9110653fa5d6043498373cf1d684` | 468 |
| `main.js` | `b3262b36ab6cab3653409703a52e6c2efa4e144eeb8b59bb59d931f019c0f2d6` | 145384 |
| `styles.css` | `50358b4a2ecd849e48b4ee74f783218c8345ca8c049f5f4b8e70d90826c6d152` | 34072 |

**Live check after the deploy:** `/api/offers/health` → `{"status":"ok"}`; the
served `/api/offers/openapi.yaml` contains `form/layout` and `form/describe`;
`GET /form/layout` without a token answers 401.

**Not done in this release: the CUP news.** `data.json` still reads
`lastAnnouncedVersion: 0.6.0`. `announceOnce` runs in `onload`, and hot-reload
does not watch files on Windows (`watchNeeded = !Platform.isWin`), so touching
`main.js` does not reload the plugin. The news publishes by itself on the next
Obsidian restart or when the plugin is toggled off/on in Settings. Do not
"fix" this by calling the endpoint by hand: the rule is one call per version
driven by the plugin itself.

### Traps met here

1. **Go's RE2 has no lookarounds** - `(?![0-9])` in a unit-degree regex panics
   at `MustCompile`, before any test runs. The right-digit check is code
   (`insertSupBeforeLastDigit`), not a regex.
2. **A marker inserted by rewriting the whole match lands in the wrong byte.**
   Marking `г/м2` by `ReplaceAllStringFunc` produced `г/м\x002`; mark by the
   match's end index instead.
3. **The glued-word splitter must be prefix-anchored.** A first version split
   anywhere a known stem appeared and turned «уровнемподземных» into «у ровнем
   подземных», «подземныхсооружений» into «по дземных сооружений» - the rune
   cut was fine, the *choice* of cut was not. Cut only where a known stem
   starts, and require the left half to end like a Russian word.
4. **Single-letter joiners are ordinary word endings.** «на» split «мембрана»
   into «мембра на». Only two-letter, unambiguous joiners (`от`, `их`, `ее`)
   are used for a tail.
5. **`SplitSupsub` accepted the marker but never made the following rune
   superscript** - a `pending` flag was missing; the test caught it.
6. **The stand's LLM stub answered every "JSON" prompt with the brief** - so
   the describe endpoint looked broken (502, "no nodes") while the code was
   fine. The stub now answers the structure prompt (`DESCRIBE_REPLY`).
7. **`zipEntry` in the stand returns a record, not bytes** - packing a layout
   needed the shared `packZip`; the packing loop in `makeXlsx` was extracted
   instead of copied.
8. **`stripImageExt` on both sides.** The extension-stripping pass compared the
   wanted name (`Схема`, no extension) against a stripped attachment
   (`Схема.png` → `Схема`) but had already stripped the wanted one into an empty
   stem (`extOf("Схема")` = ""), so it never matched. Both sides go through the
   same normal form now.
