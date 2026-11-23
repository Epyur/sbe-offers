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

**Ready-made ТЗ for the next context window** — [NEXT-SESSION-PROMPT.md](NEXT-SESSION-PROMPT.md)
(state, what is done, what is open, checks, gotchas). The short phrase that starts
a new window:

> Открой `C:\Obsidian\mailers\.obsidian\plugins\sbe-offers\NEXT-SESSION-PROMPT.md`
> и выполни его как ТЗ.

Current version = **0.11.15** — released 2026-11-23. The Mini App's fourth card
«Технодайджест» is no longer a stub: it shows the month's news from the Knowledge
Base digest (month switcher, rubric, lead, body, source) and gives every item its
own «📤 Отправить в чат» button (a bot message; the window stays open). The
previous version (0.11.14) made the product list appear only after typing in the
КП/combat windows and signed chosen products with their names.

The Mini App key cannot read kb-service (a token is issued to ONE app), so
`offers-service` proxies the digest with its service-to-service token; see
`appdigest.go` and the gotcha about `app_id`. Deploy the service **before** the
new app bundle. Two independent reviews (server/access, client/product) before
deploy found **no blockers**; their findings are fixed and listed in
`HANDOFF.md` («Две независимые ревизии…»).

> **0.11.14 (2026-11-21) — окна выбора продукта: список после ввода, выбранное — названиями.**
> Две живые жалобы по окнам создания/редактирования КП и боевого листка.
> **(1) «В блоке выбора продукта показываются продукты до начала ввода».**
> Окно рисовало первые 20 строк кэша, хотя человек ещё не написал ни буквы;
> в мини-приложении это уже было решено (`shouldSearchSide`, 0.11.7). Правило
> вынесено в чистый `src/ui/picker-model.ts` (`pickerRows`: пустой запрос —
> пустой список), им пользуются **оба** окна — `picker.ts` (системы и материалы
> КП) и `combat-view.ts` (наш продукт / конкуренты листка); вместо каталога
> показывается подсказка «Начните вводить название — совпадения появятся
> здесь». Заодно убрана вторая копия `filterCandidates`.
> **(2) «После выбора показываются коды, а не названия».** У выбора в КП сервер
> хранит только `noteId`/`sku`, а подпись бралась из результатов **последнего**
> поиска: после второго запроса или переоткрытия КП появлялись `sys-412` /
> `MAT-…`. Теперь окно помнит имена всего, что встречало (`rememberNames`), и
> подписывает выбранное через `refTitle`; справочник систем/материалов
> дочитывается при открытии и перерисовывает список выбранного. В
> мини-приложении `chosenMaterials(offer, catalog)` получает справочник
> материалов (`loadMaterialCatalog`) — чипы материалов тоже названиями. Если
> справочник не прочитался, код остаётся запасным путём: пустая строка была бы
> хуже.
> **Проверки:** плагин `tsc` 0 / `build` OK / `check_bundle_strings` 0 /
> `check_marker_registry` OK / `check_sections_conditions` 16/16 /
> `check_combat_sections_rules` 20/20 / новый `check_picker_rules` **6/6**
> (оба сторожа прогнаны откатом: возврат первых 20 строк — 1 красный, подпись
> хэндлом — 3 красных); приложение `typecheck` 0 / **91** тест (+1 новый,
> красный до правки) / `build` OK / `size` код 111 811 Б, картинки 192 618 Б.
> Сервер, бот и база не менялись. Подробности выкладки — `HANDOFF.md`,
> «Три точки сходятся (0.11.14)». Точки: `backend` `36148c3`, `main` `efa492e`.

> **0.11.13 (2026-11-20) — бот: DOCX печатает листок из кнопки, «Достаточно» ведёт к конкуренту.**
> Две живые жалобы. **(1) «Нажимаю DOCX — приходит файл самого первого
> листка».** Кнопка не знала своего листка; `ensureCombat` при пустой сессии
> брал `sheets[sheets.length - 1]` из `api.listCombat`, а список идёт
> `ORDER BY created_at DESC` — то есть **самый старый листок всей базы** (и не
> обязательно свой), и записывал догадку в `session.combat`, поэтому следующая
> «добавить сторону» правила чужой листок. Теперь `cb:doc:<id>` /
> `cb:done:<id>` (`combatKeyboard(sheetId)`); старые сообщения без номера
> получают вопрос «какой листок» (`cbx:<id>`, спрашиваем всегда — сообщение
> могло быть о листке коллеги); `ensureCombat` — только для действий и берёт
> самый свежий листок автора по `ownerEmail`; `loadCombatById` читает один
> листок через `GET /bot/combat?id=N` (сервер: `ORDER BY created_at DESC, id
> DESC`), а не сканирует топ-50; «Готово» закрывает только совпадающий листок.
> **(2) «✅ Достаточно — листок собран» вводил в стопор.** У листка не хватало
> конкурента, а бот не говорил. Теперь первая кнопка — **«🏷️ Выбрать
> конкурента»**, она сразу открывает поиск конкурента, в приглашении остаётся
> меню листка, а в сводке есть номер листка и «дальше — продукт конкурента».
> Ветка КП по смыслу не менялась. **Проверки:** бот `tsc` 0, **231 тест**,
> сторожа прогнаны откатом; Go `gofmt`/`vet`/`test` зелёные (+3), OpenAPI
> **125/125**, стенд **630/0**, плагин `tsc` 0 / `build` / `check_bundle_strings`
> 0. Разбор совета — `docs/superpowers/specs/2026-11-20-sbe-offers-combat-doc-button-and-enough-step-council.md`.

> **0.11.12 (2026-11-19) — документы в листке и КП открываются по постоянной ссылке.**
> Живая жалоба: «при попытке открыть документы в s3 из боевого листка
> возвращается `unauthorized`». Причина — в ссылке, которую пишет База знаний
> (`/api/documents/file` под JWT): документ открывает заказчик без сессии.
> **(1) `documents_share.go`:** при печати ссылка документов заменяется на свою
> подписанную `GET /api/offers/files?key=…&sig=…` — `sig` = первые 32 hex
> HMAC-SHA256(`documents\0` + ключ, `OFFERS_SERVICE_SECRET`), детерминированная и
> бессрочная. Ручка публичная намеренно (гейт — подпись) и отвечает `302` на
> свежую presigned (7 дней) через внутреннюю `/internal/documents/agent/file-link`;
> видимость решает сервис документов (`product_refs`, `no_agent`).
> **(2) Одна точка правки** — `collectOfferRefs`: КП, листок, макет `{{ссылки}}`,
> бот и хэш файла идут через неё. **(3) OpenAPI:** путь `/api/offers/files`
> (302/400/403/404/503). **Проверки:** `gofmt`/`go vet`/`go test -count=1 ./...`
> зелёные (+9 сторожей), OpenAPI **125/125**, стенд **628/0**, плагин `tsc` 0 /
> `build` OK (срез воспроизводит backend байт в байт) / `check_bundle_strings` 0.
> **Живая сверка:** листок 017/2026 — 94 369 Б, ссылок на закрытую ручку **0**,
> подписанных **2**; ссылка из готового DOCX открыта **без токена** —
> `302` → S3 (`X-Amz-Expires=604800`), скачивание `200`, **296 541 Б**, `%PDF-`;
> испорченная подпись — `403`. Точки выпуска — `HANDOFF.md`, «Три точки сходятся
> (0.11.12)».

> **0.11.11 (2026-11-19) — листок без данных КП и без обрывков фраз.**
> Живая жалоба по листку 017/2026. **(1) USER-промт подраздела** собирался
> промтом КП: `combatSectionOffer` кладёт в `Systems` карточки ОБЕИХ сторон, и
> `renderSystemsForPrompt` добавлял правило комплекта («каждая — на свой участок
> объекта»), а шапка несла цены предложения. Теперь у листка свой
> `combatSectionFacts` (номер, название, НАШ ПРОДУКТ / ПРОДУКТЫ КОНКУРЕНТОВ), общий
> хвост — `buildSectionUserPrompt`, у КП промт байт в байт прежний.
> **(2) Встроенный контракт:** `effectiveBasePrompt("")` отдавал КП-текст
> («картинка из КП»); у листка свой `combatAnswerContract` и
> `effectiveCombatBasePrompt`, `basePromptDefault` формы листка — он же.
> **(3) Обрывки:** `stripFactNotes` убирает «— <строчная> …» после пометки, но
> только если пометка стояла после конца фразы (`noteOwnSentence`); авторское
> тире посреди предложения остаётся.
> **Кэш:** `combatSectionPromptRev` **p2**, `combatDocxVersion` **v9**.
> **Проверки:** `gofmt`/`go vet`/`go test -count=1 ./...` зелёные (+4 сторожа
> `combat_prompt_leak_test.go`, +5 случаев `TestStripFactNotes`), стенд **623/0**,
> OpenAPI 124/124, плагин `tsc` 0 / `build` OK / `check_bundle_strings` 0.
> **Живая сверка:** листок 017/2026 пересобран ручкой бота — 76 933 Б,
> `комплект` 0, `участок объекта` 0, обрывков 0, `[НУЖНО]` 0. Точки выпуска —
> `HANDOFF.md`, «Три точки сходятся (0.11.11)».

> **0.11.10 (2026-11-19) — файл Word из чата как источник контекста.**
> Пункт backlog, следующий после аналогов. **(1) Сервис** (`attach.go`): ветка
> `AttachmentDocx` читает текст `documentTextOf` (тот же разбор, что у агента:
> `w:t`, абзацы `w:p`, `w:tab`) и идёт **путём `.md`** — строгие подписи в бриф
> (`ParseMarkdownAttachment`) плюс контекст объекта (`ExtractBriefFacts` +
> модель); пустой текст (скан без слоя) — отказ с причиной; подсказки PDF и
> неизвестного формата называют `.docx`. **(2) Граница названа:** состав
> позиций из Word НЕ собирается (`w:tbl` доезжает текстом), `attachReply`
> говорит это прямо. **(3) Бот:** подсказка «📎 Прислать файл» называет `.docx`
> и ту же границу. **(4) Стенд:** прежняя проверка `.docx` была фиктивной (не
> zip → ветка «неизвестный формат»); теперь настоящий docx + пустой Word.
> **Проверки:** `gofmt`/`go vet`/`go test -count=1 ./...` зелёные (+3 сторожа),
> OpenAPI **124/124**, плагин `tsc` 0 / `build` OK (срез воспроизводит backend
> байт в байт) / `check_bundle_strings` 0 / `check_marker_registry` OK /
> `check_sections_conditions` 16/16 / `check_combat_sections_rules` 20/20, бот
> **220**, приложение **90**, стенд **623/0**. Точки выпуска — `HANDOFF.md`,
> «Три точки сходятся (0.11.10)». **Не проверено:** живой Word из Telegram под
> настоящей моделью.

> **0.11.9 (2026-11-18) — разделы боевого листка правятся в настройках, короткий список.**
> Две задачи из `NEXT-SESSION-PROMPT.md`. **(1) Структура разделов — настройка
> (решение пользователя: вариант A, «только разделы»).** До этого состав
> сравнения жил ТОЛЬКО в коде (`combatDefaultSections`), и правка ждала выпуска
> плагина. Теперь в «⚙️ Настройки боевого листка» есть блок «Содержательные
> разделы боевого листка» — тот же редактор, что у КП (`renderSectionsEditor`,
> `combat: true`), сохранение тем же `PUT /form` с `mode='combat'`. Правила:
> **id раздела после создания не меняется** (он префикс ключа кэша
> `gen.sect.<раздел>.<подраздел>`, и переименование оставило бы тексты под старым
> ключом); **пустое сохранение отклоняется** русским текстом на обеих сторонах
> (`combatEmptySectionsProblem` = `COMBAT_SECTIONS_EMPTY`), кнопка
> «↩ Вернуть заготовку» ставит встроенную заготовку (`sectionsDefault`, только
> чтение, глубокой копией); **пустой список разделов у листка ЗНАЧИТ заготовку**
> — так печатает `renderCombatSections`, поэтому и `loadForm` заполняет его
> заготовкой (на стенде нашёлся ровно такой сохранённый листок: окно сказало бы
> «разделов нет», документ печатал четыре раздела). Каркас «продукты → стороны →
> разделы → ссылки» не настраивается — это решение 0.11.6. **(2) Список листков:**
> замер `scripts/measure_combat_list.mjs` (стенд, 200 листков по 8 КБ) показал
> **1610 КБ / 87 мс** у полного списка против **56 КБ / 72 мс** у
> `?fields=preview` — сервер после 0.11.8 не медленный, тяжёлый ОТВЕТ. Добавлен
> opt-in `CombatSheetPreview` (id, номер, название, стороны, состояние, автор,
> даты, подпись) без `formValues` и `textCompetitors`; формат по умолчанию НЕ
> менялся (плагин читает полный листок из списка и сохраняет целиком), незнакомое
> `fields` отдаёт полный листок. Мини-приложение просит короткий список, плагин —
> полный. **Проверки:** `gofmt`/`go vet`/`go test -count=1 ./...` зелёные (+9
> тестов), OpenAPI **124/124**, плагин `tsc` 0 / `build` OK /
> `check_bundle_strings` 0 / `check_marker_registry` OK /
> `check_sections_conditions` 16/16 / **новый** `check_combat_sections_rules`
> **20/20**, бот **220** (не менялся), приложение `tsc` 0 / **90** / `build` OK /
> `size` код 111 746 Б, **стенд 620/0** (+14 «4a. Настройки боевого листка», +7
> «короткий список»). Точки выпуска — `HANDOFF.md`, «Три точки сходятся
> (0.11.9)». **Не проверено:** окно настроек руками в Obsidian, живой листок под
> настоящей моделью, живой список с `fields=preview` на бою после выкладки.

> **0.11.8 (2026-11-18) — ссылки карточек в листке, быстрый список, сообщение о сборке.**
> Четыре жалобы по живому листку 017/2026 и по работе мини-приложения. **(1) Ссылки:**
> в конце листка стояли две ссылки из `## Ссылки`, хотя в заметке был и
> `## Иные документы` (файлы приложения «Документы»). Причина — две вещи сразу:
> листок читает кэш `offers_context(mode='system_cards')`, а подсказка в
> «Настройках боевого листка» отправляла перечитывать **«Системы»**
> (`POST /systems/refresh` → `mode='systems'`, на листок не влияет; нужен
> **«Контекст КП»** → `POST /context/refresh`); и готовый DOCX лежал в кэше под
> хэшем, в который ссылки не входили. Теперь ссылки наших карточек входят в
> `combatDocxHash`, `combatDocxVersion` → **v8**, а при пустом списке у карточки
> с `noteId` документ печатает видимую строку с правильным адресом кнопки.
> `combatOurRefs` — один набор ссылок для печати и для хэша. **(2) Список листков:**
> `handleListCombat`/`handleBotListCombat` читают кэш конкурентов **один раз на
> запрос** (было 1 + 2N запросов и N разборов одного JSONB: `withRepairedCompetitorTitles`
> звался на каждой строке после 0.11.7); добавлен индекс
> `offers_combat_sheet_created (created_at DESC)`. **(3) «Отправить в чат»:** первая
> строка фоновой сборки — «⌛ Формирую … пришлю в чат по готовности» через
> `notifyChat`; `notifyChat` теперь читает HTTP-статус бота, а не глотает 403/502/503.
> Подпись в приложении объясняет, что файл придёт в чат и страницу можно закрыть.
> **(4) Кэш карточек:** `systemCardsForCombat` при пустом кэше зовёт
> `refreshSystemCards`, а не `SystemsFromKB`: прежний путь сохранял набор **без
> материалов и без `LinkedRefs`**, и материал на нашей стороне листка переставал
> находиться при печати. **Проверки:** `gofmt`/`go vet`/`go test -count=1 ./...`
> зелёные (новые сторожа ссылок в хэше, дедупа/порядка, предупреждения и текста
> «⌛»), OpenAPI **124/124**, плагин `tsc` 0 / `build` OK /
> `check_bundle_strings` 0 / `check_marker_registry` OK /
> `check_sections_conditions` 16/16, бот **220**, приложение **89**, `size` код
> 111 712 Б, **стенд 599/0** (+2: ссылки карточек в DOCX и документ из «Иных
> документов» в листке). **Не проверено:** живое сообщение «⌛» в Telegram
> (стендовая заглушка бота не знает `/internal/notify`) и реальное ускорение
> списка на боевой базе.

> **0.11.7 (2026-11-17) — источники конкурента, дисклеймер вместо названия, поиск по пяти строкам.**
> Три просьбы пользователя по боевому листку в мини-приложении плюс дефект,
> найденный по живой базе. **(1) Источники конкурента:** в блоке «Конкуренты»
> переключатель **База знаний / Файл / URL**; в Базе знаний прежний поиск и под
> ним кнопка **«Подобрать»** (бывший отдельный блок «Аналоги» убран); файл
> принимает бот, ссылку читает сервер. Конкурент из файла/ссылки живёт в **своей
> колонке** `offers_combat_sheet.text_competitors` (`{key,title,text,source}`),
> без карточки Базы знаний, и печатается одной функцией `combatSideCards`
> (`combat_text.go`) во всех трёх путях выгрузки. **(2) Дисклеймер вместо
> названия:** живой листок 017/2026 хранил `{"title":"Материал конкурента…",
> "noteId":6654}` — агентский путь `readNoteCard`/`systemCardFromParsed` брал
> первый H1 заметки. Теперь карточка конкурента собирается
> `competitorCardFromParsed` независимо от двери чтения, вход ручек стороны
> берёт название из карточки (`combatCompetitorTitle`), а старые листки чинятся
> **на чтении** (`withRepairedCompetitorTitles`). **(3) Поиск стороны:** пустое
> поле не ищет вовсе, при вводе не больше пяти строк (`shouldSearchSide`,
> `sideSearchResults`, `COMBAT_SEARCH_LIMIT`). `combatDocxVersion` → **v7**
> (текстовые конкуренты входят в хэш файла). Полная запись решений и находок
> моделей — `../docs/superpowers/specs/2026-11-17-sbe-offers-combat-competitor-sources-council.md`.
> Проверки: `gofmt`/`go vet`/`go test -count=1 ./...` зелёные, OpenAPI
> **124/124**, плагин `tsc` 0 / `build` OK / `check_bundle_strings` 0 /
> `check_marker_registry` OK / `check_sections_conditions` 16/16, бот **219**,
> приложение **89**, стенд **597/0** (+20: новая секция «Источники конкурента в
> боевом листке» — гейты, отказ не-http адреса, ссылка через agent-stub,
> замена по тому же адресу, «пусто = не трогали» при полном сохранении, файл
> DOCX, отказ `.txt`, удаление по ключу, печать DOCX). Точки выпуска и
> живая сверка — `HANDOFF.md`, «0.11.7».

> **0.11.6 (2026-11-17) — настройки боевого листка в одном окне, ссылки с документами.**
> Просьба пользователя по живому листку: кнопку «📄 Макет боевого листка» убрать,
> а настройки собрать в одно окно. **(1) «⚙️ Настройки боевого листка»**
> (`src/ui/combat-settings.ts`): базовый промт листка, метки `{{сравнение}}` и
> `{{ссылки}}` с копированием по клику, макет — загрузить / скачать / удалить.
> Мёртвый `mode='combat'` в общем редакторе формы удалён: у листка нет полей формы,
> блоков и окна приложения. **(2) «## Иные документы» читаются** (`skills/cardrefs.go`):
> документы приложения «Документы» (файлы в S3) попадают в карточку, а значит в КП и
> в листок; пояснение «— ДС ФЗ 123» и `<!-- kb-doc:… -->` снимаются, порядок —
> порядок заметки. **(3) Встроенный вид листка печатает раздел «Ссылки»** сам; по
> макету его печатает метка `{{ссылки}}` (второй список в тело не дописывается).
> **(4) Ответ загрузки называет найденные метки** (`layoutLinks`/`layoutComparison`),
> чтобы пропущенная метка была видна сразу. `combatDocxVersion` → **v6**.
> Проверки: `gofmt`/`go vet`/`go test -count=1 ./...` зелёные, OpenAPI **121/121**,
> плагин `tsc` 0 / `build` OK / `check_bundle_strings` 0 / `check_marker_registry`
> OK / `check_sections_conditions` 16/16, бот **217**, приложение **85**,
> стенд **577/0**. Подробности и точки выпуска — `HANDOFF.md`, «0.11.6».

> **0.11.5 (2026-11-17, вечер) — базовый промт в форме, характеристики в промпте,
> ссылки одним списком.** Четыре правки по живым жалобам на листок 017/2026.
> **(1) Базовый промт — настройка формы.** `offers_form.base_prompt` (миграция),
> поле `OfferForm.BasePrompt` + `BasePromptDefault`, правка в «Форме КП» и «Форме
> боевого листка» (карточка «Базовый промт», кнопка «↩ Вернуть базовый текст»).
> Раньше общая часть промта (`sectionAnswerContract`) жила в коде: подразделы
> пишутся отдельными вызовами модели и друг друга не видят, поэтому требования
> к согласованности («не называй продукты равноценными при разных значениях»)
> должен задавать администратор. Свой промт входит в ключ кэша (`.bp<hash>`) и в
> отпечаток содержания листка, поэтому правка переписывает разделы.
> **(2) Характеристики карточки доходят до модели** (`skills.SpecsPromptLines`,
> ≤60 строк / ≤6000 знаков; `SystemCard.PromptText` передаёт `Specifications`).
> **(3) Ссылки — один список в порядке упоминания**, без групп; деление на типы и
> редактор правил удалены. **(4) В боевом листке печатаются только наши ссылки.**
> `combatDocxVersion` → **v5**, и хэш файла листка теперь включает форму
> (`combatFormFingerprint`): правка промта/разделов пересобирает файл.
> Проверки: `gofmt`/`go vet`/`go test -count=1 ./...` зелёные, OpenAPI **121/121**,
> плагин `tsc` 0 / `build` OK / `check_bundle_strings` 0 / `check_marker_registry`
> OK / `check_sections_conditions` 16/16, стенд **574/0**. **Выпущена и выложена
> 2026-11-17:** `backend` `7b97098`, `main` `5a1719b` (доки `ba62719`), self-hosted `manifest 9c7fa4d1…` /
> `main 6577e472…` / `styles 50bad65b…`, реестр `0.11.5`
> (`uploadedAt 2026-11-17T16:29:56Z`). Живая пересборка 017/2026: заголовков
> групп ссылок нет, в конце — две наши ссылки в порядке заметки, адреса
> конкурента нет; характеристики на месте. Подробности — `HANDOFF.md`, «0.11.5».
> **Стенд-грабля:** `auth-stub` обязан понимать список секретов через запятую —
> у стенда два экземпляра сервиса (основной и «умный» двойник), и точное
> сравнение с одним значением даёт `403 insufficient role` на каждой ручке.

> **0.11.4 (2026-11-17) — sections in the combat sheet, diagrams, agent file/link
> reading.** Three steps of the 2026-11-16 plan landed on `backend` and are released
> as 0.11.4. **(1) Combat sheet on sections** (steps 2a/2b/6): the sheet prints the
> `mode='combat'` form's content sections through the same pipeline as the offer
> (`combat_sections.go`), with per-section text cached in the sheet's own
> `form_values` (`gen.sect.*` + `gen.section`); the default form now has five
> sections — «Общее сравнение» (with a characteristics TABLE), «Схема сравнения»
> (mermaid diagram), «Где конкурент сильнее — и что отвечать», «По каждому
> продукту» (`repeat: systems`). The section brief now carries the manager's
> `hint` (found by the stand: without it changing the hint rebuilt the file but left
> the text unchanged). **(2) Diagram blocks** (step 5): the frozen `{blocks:[…]}`
> contract has a sixth kind `{"kind":"diagram","lang":"mermaid","code":…,
> "caption":…}`; `sections_print.go` prints it via `sections_diagram.go` —
> `agent-service` `Render` → PNG → `docxMedia.add` (`wp:inline`), caption «Схема N»
> numbered by a document-wide `sectionDiagramCounter`, width ≤16 cm / height by PNG
> proportion (capped at 20 cm). `lang` other than `mermaid` is DROPPED with a log
> (spec §7.2 — no SVG); a failed render prints a VISIBLE «[Схема не отрисована:
> причина]» (§9.2), not silence. `docxMedia.register` is now mutex-guarded (sections
> print in parallel goroutines). `combatDocxVersion` bumped v1→v2 (the sheet's look
> changed). **(3) Agent reads files/links** (steps 1/3/4/3e): `fetch_url` and
> `parse_file` tools via the `agent-service` client, `remember_sheet_fact` stores what
> was read in that turn under the sheet — 18 tools total. Also fixed a
> plugin-lifecycle defect: `onload` re-registering the view on HotReload threw
> «existing view type»; `onunload` now detaches the leaves and `registerView` is
> idempotent. Checks: `gofmt`/`go vet`/`go test -count=1 ./...` green, OpenAPI
> 121/121, plugin `tsc` 0 / `build` OK / `check_bundle_strings` 0 /
> `check_marker_registry` OK / `check_sections_conditions` 16/16, stand **575/0**.
> **Released and deployed 2026-11-17:** `backend` `1b3c4be`, `main` `e991975`,
> self-hosted `manifest 731a4792…` / `main ba0aab29…` / `styles 152ebdb3…`,
> registry `sbe-apstore-registry` `d0bb5cb`. The `sbe-agent` step-1 route
> (`/internal/agent/*`) was deployed for the first time here, with
> `AGENT_SERVICE_URL`/`AGENT_INTERNAL_SECRET` added to both compose blocks; live
> check: a real Cyrillic mermaid rendered to a 127×132 PNG, `render` answers 403
> without the secret and 400 for `lang:"svg"`. **Not verified:** a live sheet under
> a real model (the stand's model is a stub; the diagram render itself was proven
> live).

> **Read before touching the offer screen:** the screen is built in ONE pass over
> the window (`screenItems` in `offers-app/src/window.ts`) — zones AND form fields,
> in the administrator's order. There is no `FieldsZone` and there must not be one:
> the configurator allows mixing a field between zones, and a separate fields block
> would break that order. Fields save through `PATCH /offers/{id}/form-values`
> (partial merge); `PUT /client` would wipe `copy.*` and `brief.*` that share
> `form_values`. Client name/contact are part of the `client` zone, not separate
> fields (user decision 2026-11-16).

> **Council write-ups (how these two defects were found):**
> `C:\Obsidian\mailers\docs\superpowers\specs\2026-11-16-sbe-offers-brief-zone-postmortem.md`
> and `...-form-fields-council.md` — the second one also carries the ALGORITHM for
> bringing in other models (round 1: codex+astra+glm in parallel with the full fact
> pack; round 2: opus+gemini only on a contradiction; verify every claim before
> showing it).


> **Postmortem of this defect — read before touching the window again:**
> `C:\Obsidian\mailers\docs\superpowers\specs\2026-11-16-sbe-offers-brief-zone-postmortem.md`.
> It records the three-way review (`gemini-4-argon` → `claude-5-opus`), what each
> found, which claim was wrong and how it was checked against live data.

Previous version 0.11.1 — RELEASED 2026-11-16 (block «Бриф» in the app, repeat
brief from the app, file parsing in «Позиции»).

Code heads: `backend` `3516da9` (code; docs — следующим коммитом), `main` `4eabd60`
(release slice of 0.11.15; code; `offers-service/`, `offers-bot/`,
`offers-app/` live on `backend` only).

> **0.11.1 — блок «Бриф» в карточке КП, повторный бриф из приложения и файл
> разбора позиций (2026-11-15).** Три просьбы пользователя в одной версии.
> **(1) Бриф в Mini App.** Новая зона окна `brief` (`knownWindowZones`, подпись
> «Бриф»): темы брифа с «Заполнено N из 14 тем» (`offers-app/src/brief.ts` +
> `brief-screen.tsx`), ответ на открытый вопрос прямо в приложении
> (`POST /offers/{id}/brief/{topic}` — тот же `StepBrief`, что в боте, но под
> JWT), ручная правка тех же полей и свёрнутая история разговора. **Зона по
> умолчанию выключена**: `defaultWindow` строится по `legacyWindowZones` (пять
> прежних), а `normalizeWindow` дописывает бриф в конец **невидимым** — старая
> форма не показывает новый раздел сама. **(2) Повторить бриф в боте.**
> Кнопка «✏️ Продолжить бриф в боте» → `POST /app/ask {action:'brief'}`:
> сервис проверяет `initData`, берёт `chat_id` из подписи и зовёт бота
> `/internal/ask`; бот открывает тот же диалог (`beginBrief`). **(3) Файл для
> разбора позиций.** В зоне «Позиции» кнопка «📎 Прислать файл для разбора» →
> `action:'positions'`: бот пишет «пришлите .xlsx», файл разбирается обычным
> `routeDocument`/`handleBotAttach` (двухшагово, с подтверждением). Почему
> через бота: `Telegram.WebApp.sendData` у inline-кнопки не работает (0.9.27),
> а разбора таблиц и подтверждения в приложении нет вовсе. Подробности и
> грабли — `HANDOFF.md`, «0.11.1 — блок Брифа».

**Previous version 0.11.0 — RELEASED 2026-11-15.** `backend` `f373a44`,
`main` `4f65789` (release slice; `offers-service/`, `offers-bot/`,
`offers-app/` live on `backend` only); self-hosted `manifest 7685465c…` /
`main ac2f1069…` / `styles fcb9758f…`, registry `sbe-apstore-registry`
`03bd167`. Service `offers` rebuilt and recreated (migration `window_config`);
bot unchanged; App deployed as `assets/index-Ba6bkNrw.js`. Stand **535/0**.

> **RELEASED — конфигуратор окна КП в приложении (0.11.0, 2026-11-15).**
> **Семь шагов из семи сделаны.** Администратор задаёт
> порядок, видимость и обязательность полей формы **и** зон экрана приложения
> (`client`/`systems`/`items`/`materials`/`sections`); настройка живёт в
> `offers_form.window_config JSONB`; пусто = «как сейчас». Обязательность —
> **мягкая** (предупреждение, сохранение проходит). Предпросмотр `prompt_text`
> вместо сырого промта показывает заглушку текста и свёрнутый «Бриф для модели».
> Читать: спека `docs/superpowers/specs/2026-11-15-sbe-offers-window-configurator-design.md`,
> план `docs/superpowers/plans/2026-11-15-sbe-offers-window-configurator-plan.md`,
> вход нового контекста `docs/superpowers/plans/2026-11-15-sbe-offers-window-START-HERE.md`.
> **Эти три файла лежат вне папки плагина** — `C:\Obsidian\mailers\docs\superpowers\`;
> путь `sbe-offers/docs/…` внутри плагина пуст.
> Состояние шагов 1–7 — `HANDOFF.md`, блок «Конфигуратор окна КП — состояние на
> 2026-11-15», там же таблица семи шагов, точки выпуска и принятые решения (в
> том числе «скрытое обязательное запрещено»). Стенд — **535/0**.

**Конфигуратор окна КП — шаг 1, код сервера (2026-11-15).** `offers-service/form_window.go`:
`WindowItem{Kind,ID,Label,Visible,Required}`, `knownWindowZones` (`client`,
`systems`, `items`, `materials`, `sections`), `windowZoneTitles`,
`effectiveWindow` (пусто → нынешнее поведение), `defaultWindow`,
`normalizeWindow` (порядок записанного главный; неизвестное и дубли
выкидываются; недостающее дописывается в конец **невидимым**; при `!Visible`
снимается `Required`), `validateWindow` (скрытое обязательное —
**отвергается**), `missingWindowFields` (`Required && Visible && пусто`),
`windowMissing`. `form.go`: `OfferForm.Window`, `validateWindow` в
`validateForm`, `window_config` в `loadForm`/`handleSaveForm`. `main.go`:
колонка в `CREATE` и `ALTER`. `offerfields.go`: `respondWithOffer` отвечает
`{"offer":…,"missingRequired":[…]}`, поэтому `PUT /offers/{id}/client` и
`/items` **больше не отдают голый `Offer`** — приложение обязано читать
`res.offer` (шаг 3). Тесты: `form_window_test.go`, `form_window_schema_test.go`,
`window_required_test.go` — 16 проверок окна, все сторожа прогнаны откатом.
Проверки шага: `gofmt` чисто, `go vet` 0, `go test -count=1 ./...` зелёный.
**Окно читается всегда из формы `clientFormMode`** — у этих ручек других
режимов нет.

**Конфигуратор окна КП — шаг 2, чистая часть приложения (2026-11-15).**
`offers-app/src/window.ts` (без DOM, проверяется `node --test`):
`WindowZoneId`, `DEFAULT_ZONES` (нынешний порядок экрана), `ZONE_TITLES`,
`WindowItemDto`/`OfferFormWindowDto` (форма `GET /form`), `visibleZones`
(пусто → умолчание; скрытое не возвращается; незнакомое и дубли зон
отбрасываются; поля зонами не становятся), `zoneOf`, `loadOfferWindow`
(`GET /form?mode=client`; **ошибка → `null`**, не исключение),
`zonesForScreen` (`null` → все зоны), `requiredOf` (пустые обязательные для
подсветки; правила те же, что у `missingWindowFields`, **мягко**).
Тест `test/window.test.ts` — **16 проверок**, сторожа прогнаны откатом
(убрать проверку `visible`/перевернуть «пусто → умолчание» → 2 красных).
Проверки: приложение `typecheck` 0, `npm test` **69/69** (было 53), `build`
OK, `size` код 105 734 / картинки 192 618 Б из 204 800. Плагин, сервис, бот и
база не менялись — только два новых файла в `offers-app`.

**Конфигуратор окна КП — шаг 3, экран рисует зоны окна (2026-11-15).**
`offer-screen.tsx`: `OfferCard` грузит окно третьим запросом того же
`Promise.all`, зоны рисуются `zones.map(...)` через компонент `Zone` (`switch`
по зоне), невидимые не доходят вовсе; над зонами — `app-window-note` («настройку
окна прочитать не удалось — показаны все разделы», когда `/form` не прочитался)
и мягкое «Не заполнено обязательное: …»; `ClientZone` получает `required` и
класс `app-required`. `offer.ts`: `readWindowSave` / `WindowSaveResponse` —
`PUT /client` и `/items` читают `res.offer` и отдают `missingRequired`;
**старый голый `Offer` тоже разбирается** (иначе сервер прежней сборки дал бы
пустую карточку). `styles.ts`: `app-window-note`, `app-required` (мягкая рамка,
сохранение не блокируется). Тесты: `offer.test.ts` +3 (новая форма ответа,
отсутствие `missingRequired` → `[]`, старый ответ); `check_home_screen.mjs`
подменяет `/form` окном «позиции, заказчик» и проверяет на `#/offer/1`, что
нарисованы ровно эти зоны — **11/11**, сторож прогнан откатом. Проверки:
`typecheck` 0, `npm test` **72/72**, `build` OK, `size` код 107 025 / картинки
192 618 Б. Без `FieldsZone` намеренно: элементы `kind:"field"` рисуются вместе с
редактором (шаг 4), иначе заводится вторая правда о полях.

**Конфигуратор окна КП — шаг 4, редактор в плагине (2026-11-15).**
`src/ui/window-model.ts` (чистые правила, без DOM): `KNOWN_ZONES`/`ZONE_TITLES`
(набор и порядок равны серверу), `windowTitle` (зона — подпись из кода, поле —
из формы), `defaultWindow` (пусто = «как сейчас»), `windowForEditor`,
`windowProblem` (неизвестная зона, поле-призрак, дубль; `required` только у
полей и зоны `client`; **скрытое обязательное отвергается**), `windowForSave`
(снимает `required` у скрытого), `windowPreview`, `windowSummary`,
`moveWindowItem`, `dropMissingFields` (поле убрали из формы — уходит из окна),
`appendNewFields` (новое поле — в конец **видимым**, осознанно иначе, чем
серверная нормализация старой формы), `formFieldWindow` (что рисовать в
карточке КП в плагине). `src/ui/window-editor.ts` — DOM: строки с ▲/▼, ✖
только у полей, галочки `виден`/`обязателен` (снимается при скрытии),
read-only подпись, «↩ Вернуть все поля формы», предпросмотр «Что увидит
менеджер». `form-editor.ts` — аккордеон «Окно КП в приложении» сразу после
«Полей формы», сводка в заголовке (видна свёрнутой), `redrawFields` — функция,
а не `const` (слушатели объявлены выше: `const` был бы в TDZ), при сохранении
окно нормализуется и проверяется с раскрытием группы. `offers-list.ts` —
скрытое поле не показывается и в плагине (`formFieldWindow`), иначе настройка
действовала бы только на телефоне. `help.ts` — раздел про окно. Тесты:
`scripts/check_window_rules.test.mjs` — **17 проверок**, сторожа (скрытое
обязательное, снятие `required` при сохранении) прогнаны откатом. Проверки:
`npx tsc --noEmit` 0, `build` OK, `check_bundle_strings` 0 (новые иглы),
`check_marker_registry` OK, `check_sections_conditions` 16/16.

**Конфигуратор окна КП — шаг 5, бриф вместо промта (2026-11-15).**
`form-preview.ts`, ветка `prompt_text`: в теле документа — заглушка «Здесь
будет текст, который напишет модель.», под ней свёрнутый
`<details class="tn-offers-brief-preview">` «📋 Бриф для модели» с инструкцией
целиком, «Уточнением», «Длиной», списком «Поля, которые получат значения из
КП» и строкой «В готовом документе этого брифа нет…». Тексты — константы
`PROMPT_STUB`/`PROMPT_BRIEF_*`, чтобы их проверял `check_bundle_strings`.
`window-model.ts`: `instructionFields` (обе формы `{{id}}` и `[Подпись]`,
подписи из формы, порядок встречи, без дублей, **незнакомое имя остаётся
своим** — опечатку видно). Стили `tn-offers-preview-prompt-stub`,
`tn-offers-brief-preview*`. Тесты: `check_window_rules` **21/21**
(+4 про `instructionFields`), сторожа (дубли, незнакомое имя) прогнаны
откатом. Проверки: `tsc` 0, `build` OK, `check_bundle_strings` 0,
`check_marker_registry` OK, `check_sections_conditions` 16/16. **Следующий —
шаг 6** (стенд: секция «Окно КП» + справка).

**Конфигуратор окна КП — шаг 6, стенд и справка (2026-11-15).**
`scripts/offers_local_e2e.mjs`: новая `sectionWindow()` — **20 проверок**
(порядок зон пережил сохранение; пустое окно = пять зон в прежнем порядке;
неизвестная зона → 400 русским текстом с id; обязательная зона кроме
«Заказчика» → 400; скрытое обязательное → 400; скрытое поле не попадает в
`missingRequired`; `PUT /offers/{id}/client` с пустым обязательным → 200 и
`missingRequired` — обязательность мягкая, документ сохранён; пустой список —
массив). **Там же починены 9 протухших проверок** секций 5c/5d: с шага 1 эти
ручки отвечают `{offer, missingRequired}`, а стенд читал голый `Offer`.
`src/ui/help.ts` — раздел «Окно КП в приложении: что увидит менеджер»
вычитан под фактическое поведение (подписи зон, `required_fields`, снятие
обязательности у скрытого на чтении, кнопка «↩ Вернуть все поля формы»,
проверка при сохранении и запрет убирать зону). `check_bundle_strings` +3
иглы. **Дефект, найденный проверкой:** отказ «помечено обязательным, но
скрыто» печатал `«»` вместо подписи поля — `validateWindow` собирал
`WindowItem` без `Label`; теперь подпись берётся из `placeholders`
(`windowTitle`). Проверки: стенд **535/0** (515 + 20), плагин `tsc` 0,
`build` OK, `check_bundle_strings` 0, `check_window_rules` 21/21,
сервис `gofmt`/`go vet`/`go test -count=1` зелёные. Сторожа прогнаны
откатом (неизвестная зона — 2 красных, подпись — 1 красный).

**Конфигуратор окна КП — шаг 7, фиксация (2026-11-15, ВЫПУЩЕНО 0.11.0).**
`manifest.json` и `package.json`: `0.10.4` → **`0.11.0`**. `src/main.ts` —
новость ЦУП переписана под конфигуратор окна (какие разделы и поля видит
менеджер, порядок, скрытие без потери данных, мягкая обязательность) и под
бриф вместо промта в предпросмотре; новость 0.10.4 снята.
`scripts/check_bundle_strings.mjs` — иглы 0.10.4 сняты, +4 иглы 0.11.0;
**все иглы без кавычек-«ёлочек»** (esbuild пишет их как `\xAB`/`\xBB`, проверка
знает только `\uXXXX` — грабля 2026-11-02 подтвердилась снова). Точки выпуска
(хэши, ветки, реестр) — `HANDOFF.md`, «Три точки сходятся (0.11.0)».
**Права проверены на стенде:** `PUT /offers/{id}/client` и `/items` под
`requirePerm("editor")` — без токена `401`, чужак `403`; `/bot/*` без секрета
`401`. Новых ручек нет; бот, агент и печать DOCX не трогались.

**0.10.4 (2026-11-15): ввод названия раздела больше не обрывается на первой
букве.** Жалоба пользователя: в «Разделах» заголовок набирался «не более чем по
одной букве». Причина — обработчик `input` (`src/ui/sections-editor.ts`) звал
`redraw()`, то есть пересобирал весь список карточек на каждое нажатие клавиши;
вместе с карточкой уничтожалось само поле ввода и фокус, и следующая буква
уходила в никуда. Тот же класс, что `input.value = ''` из 0.1.18 — тихая потеря
введённого. Задержки в коде не было ни одной (`setTimeout` в редакторе нет):
эффект «парализующей задержки» давала именно перерисовка.

Что сделано: `redraw()` из обработчика убран, заголовок рамки предпросмотра
обновляется точечно (`syncFramePreviewTitle`), остальное от заголовка не
зависит. Сторож — `scripts/check_sections_typing.test.mjs` (`node --test`,
2 проверки): читает исходник, вырезает комментарии и требует, чтобы НИ ОДИН
обработчик `input` не звал `redraw()`. Проверен откатом: на старом файле
красный, на новом зелёный.

Изменился только плагин; сервис, бот, приложение и база не тронуты.

**0.10.3 (2026-11-15): четыре дефекта формы КП после 0.10.2.** `backend`
`6b1165b`, `main` `aece47c`; self-hosted `23bcbe67…` / `60d8aa1b…` / `b15d618d…`,
registry `5fea4b3`→`49f87c3`. Service `offers` rebuilt (`*.bak-20261114-10103`);
live: all three auth gates `401` without credentials, `health ok`, `offers_form`
really has `sections` and `link_groups`. Живое
использование 0.10.2 дало «собрать КП пока не получится». Всё изменилось
**в плагине и в `offers-service`**; бот, приложение и база не тронуты.

- **Раздел не сохранялся** (блокер): в `offers_form` не было колонки
  `sections`, `handleSaveForm` её не писал, `loadForm` не читал. Теперь есть —
  миграция, запись, чтение; сторож `form_sections_schema_test.go` (миграции
  прогоняются в память, без Postgres).
- **«Разделы» выше предпросмотра** — предпросмотр показывает то, что задано
  выше, а сверху теперь раздел.
- **Группы формы сворачиваются**, все свёрнуты по умолчанию: один компонент
  `src/ui/collapsible-group.ts`, `<button>` + `aria-expanded`; кнопки
  сохранения — вне групп.
- **Раскладка ссылок — данные формы**: `skills/linkgroups.go` (правила, умолчание
  = прежнее поведение, проверка) + `offers_form.link_groups` + редактор
  `src/ui/link-groups-editor.ts`. Группы, порядок и названия задаёт
  администратор; **ИИ сюда не допускается** (решение пользователя).

Подробности и грабли — блок «ALL FOUR FIXED 2026-11-14» ниже. Проверки:
`go test` зелёный, плагин `tsc` 0 / `build` OK / `check_bundle_strings` 0,
OpenAPI 118/118, стенд **515/0**, бот 208, приложение 53.

**0.10.2 (2026-11-14): содержательные разделы в «Форме КП».** Раздел тела ТКП
теперь не фиксированная структура блока, а бриф для модели (`{ content: <id> }`):
администратор задаёт подразделы, их условия и инструкции, оформление берётся из
макета Word. Оглавление в готовом DOCX пересобирается из напечатанных
заголовков. **Изменился только плагин** — сервис, бот, приложение и база не
тронуты. Подробности и грабли — блок «Content sections» ниже и `HANDOFF.md`.

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

**Content sections — RELEASED in 0.10.2 (2026-11-14), but with an OPEN
BLOCKER defect.** The two-placeholder model (`[attr]` values vs `{ content: <id> }`
briefs), the per-subsection fan-out, printing through the Word master and
rebuilding the table of contents from the printed headings are done and green
(steps 1–7 of the plan). **The editor sends `sections`, but the server never
saves or reads them — a section is lost on «Сохранить форму»** (see "OPEN
DEFECTS" below; this is why a live КП could not be assembled).

> **OPEN DEFECTS / TASKS after 0.10.2 (2026-11-14, NOT DONE).** Recorded for
> the next context window; the user said «собрать КП пока не получится». The
> full write-up with code locations — `docs/superpowers/plans/2026-11-14-sbe-offers-content-sections-START-HERE.md`,
> section «ОТКРЫТЫЕ ДЕФЕКТЫ И ЗАДАЧИ 0.10.2». Priority order:
> 1. **BLOCKER — the section does not persist.** `handleSaveForm`
>    (`offers-service/form.go:560–609`) marshals only `placeholders`, `blocks`,
>    `required_fields`; `loadForm` (`:439–480`) reads only those; and
>    `offers_form` has **no `sections` column** (`main.go:381`, no `ALTER`).
>    Fix: migration `ADD COLUMN IF NOT EXISTS sections JSONB NOT NULL DEFAULT
>    '[]'`, marshal + write it in `handleSaveForm`, select + unmarshal it in
>    `loadForm`, and a `TestFormSectionsRoundTrip` that is RED on today's code.
> 2. The «Разделы» card must sit **above** the preview card in
>    `form-editor.ts` (`render()`), not below it.
> 3. **Every field group in the form is collapsible and collapsed by default**
>    («Поля формы», «Блоки документа», «Разделы документа», …) — one accordion
>    component, `aria-expanded`, the save/cancel buttons stay outside so they are
>    reachable when all groups are collapsed.
> 4. **Links grouping becomes configurable in the form, not in code.**
>    `skills/cardrefs.go` hard-codes it: `cardRefGroupOf` (URL `/systems/`
>    `/catalog/` `/catalogue/` → `page`; label «СТО» → `sto`; «узл» → `uzly`;
>    «техлист»/«ТЛ » → `techlist`; else `cert`), `CardRefGroupTitle`,
>    `cardRefGroupRank`. Store the rules on `OfferForm` (a new `linkGroups`
>    field) and read them in `GroupCardRefsByGroup`. **Do not hand this to the
>    AI** — the user's explicit decision: the administrator sets the grouping
>    rules. Migrate forms to a default rule set equal to today's behaviour so
>    existing documents do not change silently.

> **ALL FOUR FIXED 2026-11-14 — released in 0.10.3.** Worked through in order.
> 1. **Blocker closed.** `offers_form.sections JSONB NOT NULL DEFAULT '[]'` in
>    `CREATE` **and** `ALTER` (`main.go`); `handleSaveForm` marshals + inserts +
>    `ON CONFLICT … sections = EXCLUDED.sections`; `loadForm` selects and
>    unmarshals it with `[]ContentSection{}` default. Proven RED first on the
>    live stand (PUT echoed it, GET returned nothing), then GREEN on the same
>    probe. Guard: `form_sections_schema_test.go` — runs the real migrations
>    into memory (`runMigrations` with an injected `execFunc`, no Postgres) and
>    asserts the column exists in both statements, that `form.Sections` is
>    marshalled and read, and that the `INSERT`/`SELECT` list it. Stand section
>    «4. Форма КП» gained 5 checks (save → read → subsections → clear).
> 2. **Order fixed**: заголовок → поля → блоки → **разделы** → предпросмотр →
>    макет → кнопки (`form-editor.ts`).
> 3. **Accordions**: one component, `src/ui/collapsible-group.ts`
>    (`renderCollapsibleGroup`), `<button>` + `aria-expanded` + `data:`-free
>    state, all groups collapsed by default; save/cancel stay OUTSIDE. The
>    «✨ Опишите блок словами» button and the «какой макет печатает» hint stay
>    reachable while collapsed (counters/names in the group header).
> 4. **Link grouping is data.** `skills/linkgroups.go`: `LinkGroupRule`
>    (`id`, `title`, `urlContains`, `labelContains`), `DefaultLinkGroupRules()`
>    == the old hard-coded behaviour, `ValidateLinkGroupRules` (empty = OK, the
>    last group must have no criteria), `LinkGroupOf`/`LinkGroupTitle`/
>    `GroupCardRefsWith`/`SortCardRefsWith`. Stored in
>    `offers_form.link_groups JSONB NOT NULL DEFAULT '[]'`, marshalled in
>    `handleSaveForm`, read in `loadForm` with the default when empty. The
>    plugin editor is `src/ui/link-groups-editor.ts` (add / move up / move down
>    / remove, the last group locked), one more accordion in the form editor.
>    `groupOfRef` — one rule for grouping and sorting: **explicit** form rules
>    win over the group mark baked into the cached refs; the default rules
>    honour the mark (the address alone cannot tell узлы from техлисты — both
>    live under `/pim/`).
>
> **Checks (2026-11-14):** `gofmt`/`go vet` clean, `go test -count=1 ./...`
> green; plugin `tsc` 0, `build` OK, `check_bundle_strings` **0 missing**
> (needle «Предпросмотр (как увидит клиент)» replaced — the header is now built
> from the group title), `check_marker_registry` OK,
> `check_sections_conditions` 16/16, OpenAPI 118/118, `check_agent_engine`
> green, bot `tsc` 0 / **208**, app `tsc` 0 / **53**, stand **515/0**
> (+10: 5 for sections, 5 for link rules). After the version bump the CUP news
> was rewritten («Разделы в „Форме КП“ … сохраняются; карточки формы
> сворачиваются; раскладку ссылок задаёт администратор») and the 0.10.2
> needles were removed; `check_bundle_strings` 0. **Needle gotcha:** the news
> starts with «ёлочки», which esbuild writes as `\xAB` — the needle is
> «Разделы в » without them (same trap as 2026-11-02).
>
> **Gotchas hit again:** `Add-Content`/`Set-Content`/PowerShell pipelines wrote
> the new `.go` file in cp866 and later in mojibake; the edit/write tools and
> Python with `io.open(..., encoding='utf-8', newline='\n')` are the only safe
> ways. `go test` caches results — when a fix «does not take», re-run with
> `-count=1` before hunting a phantom.
>
> **Not verified:** opening the accordion/link-group UI in Obsidian by hand;
> a live КП assembled with custom link rules (the stand runs `llm-stub`).

- **Step 5a (TOC) is fixed.** The real cause was not the rebuild code: the test
  printed a section into `temple.docx`, which carried no `{ content: … }` marker
  (only `{{block:…}}`), so the section never printed and no heading was ever
  registered. `tocHeadingXML` again calls `addTOCHeading` and wraps the heading
  text in `<w:bookmarkStart/End>`; the section title is registered FIRST (document
  order), and a section where every subsection is skipped drops its heading
  bookmark (`dropTOCHeading`, §10.1). The debug logs are gone.
- **Word splits `{{block:…}}` across several `<w:t>`** (`{{`, `block:`, `marteh`,
  `}}`) — replace a marker in a docx by the paragraph's JOINED text
  (`paragraphTexts`), never by a byte-for-byte `strings.Replace`.
- **Step 5b**: `sections_temple_test.go`, `TestSectionPrintsIntoReferenceTemple` —
  prints a section into the live `temple.docx`, proves it lands under its heading,
  the landscape `АЛЬБОМ ТИПОВЫХ РЕШЕНИЙ` `sectPr` stays the second and last, and
  the TOC picks the new heading up.
- **Step 6**: `src/ui/sections-model.ts` (pure rules — `conditionWords`,
  `conditionHoldsClient`, `sectionMarker`; no Obsidian/DOM, so a node script can
  run it) and `src/ui/sections-editor.ts` (section cards, subsection table,
  frame preview without calling the model, `{ content: <id> }` copy chip), wired
  into `form-editor.ts`. **The editor puts `sections` into the `saveForm` payload,
  but the server drops them — see the OPEN DEFECTS block above.**
- **Step 7**: `markers.ts` group «Содержательные разделы»; `help.ts` paragraph on
  the two kinds of placeholder and style-in-master; `check_marker_registry.mjs`
  now knows the `content` key as a `section` kind and requires it.
- **Step 5c** stays a rule, not code: delivery is DOCX only, no conversion step
  (see the "Editable formats only — never PDF" rule).

Files: `offers-service/sections.go` (marker printing, conditions, fan-out, TOC
order), `sections_print.go` (frozen `{blocks:[…]}` contract, master constructs,
cache), `docx_toc.go` (TOC registration + rebuild), `docx_layout.go` (rebuild
call), `form.go` (`ContentSection` etc.), `src/types/offers.ts`,
`src/ui/sections-model.ts`, `src/ui/sections-editor.ts`, `src/ui/markers.ts`,
`src/ui/help.ts`, `testdata/temple.docx` (2.37 MB, a copy of
`C:\Obsidian\mailers\temple.docx`). Tests: `sections_test.go`,
`sections_form_test.go`, `sections_conditions_test.go`, `sections_calls_test.go`,
`sections_print_test.go`, `sections_temple_test.go`, `docx_toc_test.go` (nothing
skipped). Scripts: `check_sections_conditions.mjs` (16 checks — the client
condition mirror against the server's `form.go`/`sections.go`).

**Checks (2026-11-14):** `gofmt`/`go vet` clean, `go test ./...`
green (882 PASS / 6 SKIP — live-layout tests needing `OFFERS_LIVE_LAYOUT`),
`tsc` 0, `build` OK, `check_bundle_strings` 0, `check_marker_registry` OK,
`check_sections_conditions` 16/16, OpenAPI 118/118, `check_docs_state` OK,
stand **505/0**. **Not verified:** a live offer with a content section under the
real model (the stand runs `llm-stub`); uploading the new `temple.docx` with
`{ content: … }` markers is the administrator's step (old blocks are not
migrated).

**The full state, the fixed TOC defect and the next-context checklist are in
`docs/superpowers/plans/2026-11-14-sbe-offers-content-sections-START-HERE.md`,
section «Что не работает (открытие нового контекста)».**

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
  body line. The combat form lives in `offers_form(mode='combat')`: content
  sections plus the sheet's base prompt. **Its settings window is
  «⚙️ Настройки боевого листка»** (2026-11-17, `combat-settings.ts`, admin): the
  content sections, the base prompt, the layout markers `{{сравнение}}` /
  `{{ссылки}}` with click-to-copy, and the layout buttons (upload / download /
  delete). It is NOT the КП form editor: the sheet has no form fields, blocks or
  app window, and the `'combat'` branch of `form-editor.ts` was deleted — do not
  bring it back.
  **Sections are editable since 2026-11-18** (user decision: option A, «only the
  sections»): the editor is the КП one (`renderSectionsEditor`, `combat: true`),
  saved through the same `PUT /form` with `mode='combat'`. The skeleton
  (products → sides → sections → links) stays in code — making it configurable
  would reverse the 0.11.6 decision. Rules: a section's `id` is immutable after
  creation (it prefixes the `gen.sect.*` text cache), an empty save is refused in
  Russian (`combatEmptySectionsProblem` = `COMBAT_SECTIONS_EMPTY`), and the
  built-in default travels to the editor as the read-only `sectionsDefault` so
  «↩ Вернуть заготовку» has something to restore. An empty stored `sections`
  MEANS the default on read, because that is what the sheet prints
  (`renderCombatSections`). Editing sections changes `combatFormFingerprint`, so
  sheets rebuild — correct, the content changed.
  DOCX export is done (`combat_docx.go`): rendered from the combat form, our and
  competitor cards printed as two sections. **Refs come from `## Ссылки` AND
  `## Иные документы`** (documents app, S3 files — since 2026-11-17), one list in
  mention order, our products only. The refs are read from the card cache
  (`system_cards`, refreshed via «Контекст КП»), and since 2026-11-18 they are
  part of `combatDocxHash`, so a changed link set really rebuilds the file.
  `![[…pdf]]` attachments and `[[note]]` wiki
  links are still NOT read. The built-in view (no layout) prints the links
  section itself; with a layout the `{{ссылки}}` marker decides where it goes.
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

**Competitors from a file or a link (2026-11-17) — the sheet's OWN column.**
`offers_combat_sheet.text_competitors JSONB` keeps `{key,title,text,source}`
(`source` = `url` | `file`): a page read through `agent-service` or a file the
bot accepted, stored as text, **without a KB card**. Why a separate column and
not a field in `CombatNoteRef`: `noteId` is the identity in nine places
(`appendNoteRef`, `removeNoteRef`, `resolveCombatCards`,
`withRepairedCompetitorTitles`, `combatDocxHash`, both section handles,
`combatSide`, `toolSetCombatSide`, `validateCombatSheet`) — a second key inside
the same type means every one of them must pick a branch, and a forgotten branch
silently merges records. A separate column makes the illegal state impossible.
Printing goes through ONE function, `combatSideCards` (`combat_text.go`), called
from all three render paths (JWT export, bot export, «отправить в чат»);
synthesized cards get `SKU: "text:<hash>"` — see the gotcha below.
Routes: `POST /combat/{id}/competitor-from-url`, `DELETE /combat/{id}/competitor-text`
(body `{key}`), `POST /bot/combat/{id}/competitor-file` (service secret, base64).
In the Mini App the competitor block has a source switch **База знаний / Файл /
URL**: KB keeps the search (≤5 rows, empty field does not search) plus the
**«Подобрать»** button (the old separate «Аналоги» block moved here and was
removed); «Файл» asks the bot (`/app/ask`, `kind:"sheet"`,
`action:"competitor-file"`); «URL» reads the page server-side. The plugin
(Obsidian) and the bot show the sheet's KB sides only — they do not lose text
competitors (see the merge rule) but they do not display them either.

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
| `src/ui/picker.ts` | the single product picker: name search + result buttons, taken from the combat view and reused by the offer editor (2026-11-12 removed the two dropdowns). Since 0.11.14 it shows no rows before typing; rules live in `picker-model.ts` |
| `src/ui/picker-model.ts` | pure picker rules (no DOM): `pickerRows` (empty query — no list), `filterCandidates`, `rememberNames`/`refTitle` (chosen is signed with a name, not `sys-…`/`MAT-…`) |
| `src/ui/offers-list.ts` | «🧾 КП»: list, card, composition editor, "Добавить систему", DOCX, link, header button «⚙️ Форма КП» |
| `src/ui/combat-view.ts` | «⚔️ Боевой листок»: list of combat sheets, editor card (our systems + competitors from KB), form fields; header button «⚙️ Настройки боевого листка» |
| `src/ui/combat-settings.ts` | «⚙️ Настройки боевого листка» (admin): content sections (2026-11-18, the КП editor in `combat` mode), base prompt, links marker, layout upload/download/delete |
| `src/ui/competitors-editor.ts` | «🏷️ Конкуренты» (admin): cached competitor cards from the KB folder, one «↻ Перечитать» button |
| `src/ui/material-editor.ts` | «🧱 Материалы»: list with SKU, card with the ЕКН table |
| `src/ui/systems-editor.ts` | «🧩 Системы»: cached systems, resolved composition, problems; one "↻ Перечитать из Базы знаний" button |
| `src/ui/sheet-upload.ts` | manager's Excel: "what I understood", then apply |
| `src/ui/context-editor.ts` | «📚 Контекст КП»: cached system cards, open note |
| `src/ui/brief-editor.ts` + `brief-model.ts` | the brief card inside the offer editor |
| `src/ui/form-editor.ts` | «⚙️ Форма КП»: fields, blocks, required, content sections, describe-block, layout card, app window. The `'combat'` branch was deleted in 0.11.6 — the sheet has its own window (`combat-settings.ts`) |
| `src/ui/block-editor.ts` | block editor: nodes, `{{field}}` chips, placement, image source, repeat, copyable block-id marker |
| `src/ui/form-preview.ts` | live preview of the document under the blocks |
| `src/ui/describe-block-modal.ts` | «✨ Опишите блок словами»: free text → nodes |
| `src/ui/form-layout.ts` | «.docx layout» card: upload / download / delete, tags found in the file, marker cheat sheet |
| `src/ui/markers.ts` | **the single registry of every layout marker** (chips, layout card, help) |
| `src/ui/sections-model.ts` | pure rules for content sections — `conditionWords`, `conditionHoldsClient`, `sectionMarker`, `combatSectionsProblem`, `cloneSections` (no Obsidian/DOM; run by `check_sections_conditions.mjs` and `check_combat_sections_rules.mjs`) |
| `src/ui/sections-editor.ts` | «Содержательные разделы»: section cards, subsection table, frame preview (no model), `{ content: <id> }` copy chip. Reused by the combat sheet's settings with `combat: true` (immutable section id, no copy chip) |
| `src/ui/image-picker.ts` | picks an image from the vault, uploads it to KB attachments |
| `src/ui/access-editor.ts` | «👤 Бот и доступ»: codes, bot token, who signed in |
| `src/ui/help.ts` | help text, built from `markers.ts` (baked into the plugin) |
| `src/utils/{date,money,note-path}.ts` | shared helpers — one copy each |
| `src/styles.css` | `tn-offers-*` classes (the root `styles.css` is **built**, never edit it) |
| `scripts/check_bundle_strings.mjs` | proves the Russian labels reached `main.js` (escaped `\uXXXX`, uppercase) |
| `scripts/check_marker_registry.mjs` | markers in the plugin vs markers the server parses |
| `scripts/check_sections_conditions.mjs` | the client condition mirror (`sections-model.ts`) vs the server's `conditionHolds` (`form.go`/`sections.go`); 16 checks |
| `scripts/check_combat_sections_rules.mjs` | 20 checks: the empty-save refusal text is the SAME on the server (`form.go`) and in the plugin, the rule is wired into the save path, the restore button uses a copy of `sectionsDefault`, and a combat sheet's section id is not editable |
| `scripts/check_docs_state.mjs` | docs vs code: version, SHA-256 of the build, HEAD of both branches |
| `scripts/mailer-stub.mjs` + `documents-stub.mjs` | stand stubs for the agent's emails: the mailer stub replays the RECORDED fixture `scripts/mailer-fixture.json` (live response 2026-11-13) and keeps one scanned letter unlinked on purpose; the documents stub serves the one linked card |
| `scripts/offers_local_e2e.mjs` | the stand: **620 checks**, 23 sections |
| `scripts/measure_combat_list.mjs` | measurement tool for the sheet list: inserts N synthetic sheets (8 KB each, the live per-sheet size), measures the full list vs `?fields=preview`, then deletes them. `node scripts/measure_combat_list.mjs 200` |
| `scripts/check_layout_mode_defect.py` | live HTTP check of the layout modes (offer vs combat sheet) |
| `scripts/check_app_auth.mjs` | live check of the Mini App sign-in: initData signed with the real bot token, 8 checks (accept / wrong token / stale / tampered / empty / garbage / config / bare route still 401) |
| `scripts/{auth,kb,llm,bot}-stub.mjs` | stand stubs for the services the plugin talks to |
| `offers-app/` | **Telegram Mini App** (React + Vite + TS, 2026-11-13). Owns the document — КП systems/positions/texts and combat sheets; the bot keeps identity, entry and the brief. Hash routing (`#/list/offers`, `#/offer/:id`, `#/sheet/:id`, `#/digest/:ym`), `base: './'`, built to `offers-app/dist/`. **S0+S1+B+ done**: scaffold, placeholder screens, sign-in (initData → JWT), and the bot's «📱 Открыть приложение» buttons. **Since 2026-11-23** the fourth card «Технодайджест» is real: `src/digest.ts` (pure: `digestFeedOf` boundary parse, `digestParagraphs`, `monthLabel`, `sendNewsToChat`) + `src/digest-screen.tsx` (news of the month from the KB digest, month switcher, «📤 Отправить в чат» per item, window not closed). Bundle deployed to `/app-offers/` (Caddy). Checks: `npm run typecheck`, `npm run build`, `npm run size` (200 KiB gzip budget — 113 677 B of code used), `npm test` (100 passing), `scripts/check_home_screen.mjs` (26 checks, real browser). Plan: `docs/superpowers/plans/2026-11-13-sbe-offers-webapp-{START-HERE,migration-plan}.md` |

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
`appdigest.go` (Технодайджест in the Mini App, 2026-11-23: `GET /digest` from
the KB digest — published only, month from the service — and `POST /app/news`,
which re-reads the item and sends it to the chat as a bot message;
`sendChatText` in `appanswer.go` is the transport),
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
`LLM_SERVICE_URL`, `LLM_SERVICE_SECRET`, `AGENT_SERVICE_URL` (default
`http://agent:3000`), `AGENT_INTERNAL_SECRET` (secret of the agent-service
`/internal/agent/*` routes — **not** `AGENT_SERVICE_SECRET`; empty means «not
configured» and the tools refuse instead of returning empty).
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
node scripts/check_sections_conditions.mjs
node scripts/check_combat_sections_rules.mjs
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

**If that container cannot reach the Go module proxy** (2026-11-18: `lookup
proxy.golang.org … i/o timeout` — the image is fresh and its module cache is
empty), build on the host instead and copy the LINUX binary:

```
$env:CGO_ENABLED='0'; $env:GOOS='linux'; $env:GOARCH='amd64'; go build -o offers-stand .
docker cp offers-service/offers-stand offers-local-app:/mailers-offers
docker restart offers-local-app
docker exec offers-local-app sh -c "grep -c competitor-from-url /mailers-offers"
```

The last line is the real proof the container runs the NEW code — a `docker cp`
that silently wrote to a wrong path leaves the old binary in place, and the stand
then "passes" against yesterday's service.

Before a stand run, drop the systems cache — it hides `kb-stub` edits:
`docker exec offers-local-db psql -U offers -d offers -c "delete from
offers_context where mode='systems'"` and restart `offers-local-kb-stub`.

**`git archive` on Windows applies `core.autocrlf`** (2026-11-17): the first
deploy tar brought CRLF into 16 text files (go.mod, go.sum, openapi.yaml,
swagger assets, Dockerfile) and the server ended with `CR_TEXT=16`. Build the
archive with `git -c core.autocrlf=false archive …` — then `md5sum` matches and
only binaries carry `\r`. The reverse trap (archive LF against a CRLF deployed
copy — “all files differ”) is in the same family.

**Moving files to the server: compare md5 AFTER normalising line endings.**
Git checks out `.ts` as CRLF on Windows and LF on the server; the content is
identical and only the bytes differ. Normalise (`\r\n`→`\n`) before declaring
a mismatch. Guard tested 2026-11-13. **It fired again on 2026-11-17:** the
deployed `offers-service` copy had CRLF while `git archive` from this machine
gives LF, so the pre-copy comparison reported **all 294 files** as different
though the content was identical — the copy itself was right, and after
`rsync` the server files are LF (checked with `grep -c $'\r'`: 0).

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
  per-entity row must say which row it wrote to.** Since 2026-11-17 the answer
  also names the sheet markers it found (`layoutLinks`, `layoutComparison`):
  a layout missing `{{ссылки}}` would print no reference list at all, and that
  must be visible at upload, not in the finished document.
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
- **A stub that cannot express the live data shape proves nothing.** Three live
  defects hid behind it (no `system_sku` on live cards; no `## Ссылки` on the
  stub card the links check prints; and `routes.test.ts` returning sheets in
  insertion order with `createdAt: ''` — the reverse of the live
  `ORDER BY created_at DESC`, so the «buttons work after a restart» check was
  green on a stub whose order made the bug invisible, while the live bot picked
  the oldest sheet in the database).
- **A check that searches the WHOLE document for a construct the document
  legitimately uses elsewhere is not a check.** Scope it (`itemsTableXML`,
  `compositionSection`).
- **A service that talks to another service needs both the URL and the secret
  in its compose block, plus a stub on the stand.**

### Data and documents

- **A screen that reads another service live on every open will eventually show
  that service's error to the user.** `GET /systems` did exactly that. Read from
  a cache, refresh on an explicit button.
  **The digest feed is the named exception (2026-11-23, deliberate).**
  `GET /digest` asks kb-service twice per open (`DigestIssues` + `DigestIssue`)
  and has no cache: news are read once, freshness is the point, and the refusal
  is shown honestly with a «Повторить» button and a way back to the current
  issue. Do not copy this to catalogue screens (systems, materials, competitors
  — those keep caches), and do not "fix" the digest by freezing the month's news
  in `offers_context`. If traffic ever matters, a ≤60 s in-memory cache is the
  sanctioned step.
- **Model/ledger text goes to Telegram as PLAIN text — strip markdown once, in
  one place per side** (2026-11-23). The news body is authored in KB's reduced
  markdown (`**bold**`, `[label](url)`, list markers, `#`), and the bot sends
  without `parse_mode`, so `digestPlainText` (`appdigest.go`) cleans it for the
  chat while `plainLine`/`digestParagraphs` (`offers-app/src/digest.ts`) do the
  same for the screen. Two languages cannot share a function, but the rule must
  match — change both or neither. Do not switch the bot to `parse_mode=Markdown`:
  user text would become markup and a stray `_`/`*` breaks the message.
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
- **A card's characteristics table is a FACT and must reach the model**
  (2026-11-17, live sheets 016/2026 and 017/2026). `SystemCard.PromptText()`
  used to DROP `Specifications` as "note metadata", so in a combat sheet our
  product's column was «не указано» in almost every row while the competitor's
  numbers were filled — those came from a `remember_sheet_fact` fact fetched
  from the internet. The numbers had been in the KB card all along (note 6363
  carries a full table: λ10, compressive strength, water absorption, operating
  temperature). Now `skills.SpecsPromptLines` prints them (header row skipped,
  ≤60 rows, ≤6000 characters per card, ≤240 per line) under «Характеристики из
  карточки (числа бери только отсюда, не выдумывай)», and
  `combat_compare.writeCardFacts` uses the same function — one rule for both
  paths. Guards: `TestCombatSectionPromptCarriesSpecs`,
  `TestSystemCardPromptTextIncludesSpecs`, `TestCombatSectionHandleSeesCardEdit`.
  **A row cap is not a safety valve, it is a silent lie**: the first version cut
  at 15 rows and live competitor cards (23–30 rows) lost the tail, so the sheet
  said «не указано» about flexibility, UV ageing and weld strength that ARE in
  the card — an error in OUR favour. Cap by characters, keep row count high.
- **A prompt change is invisible to the DOCX cache — bump `combatDocxVersion`.**
  `combatDocxHash` knows the number, both sides, the hint, the layout bytes,
  the sheet form fingerprint, **our cards' links** (`combatOurRefs`, v8) and
  the version, but NOT the card or section texts (on purpose: a card edit must
  not rebuild every sheet). So a fix that only changes what the model writes
  would still serve the old file from `offers_combat_docx` as "fresh". v3 was
  set for exactly this (2026-11-17, second pass); **v6** — when the built-in
  view gained the links section and the cards started carrying documents-app
  refs (2026-11-17, third pass); **v8** — when the links themselves entered the
  hash (2026-11-18): before that, adding a document to a KB note never reached an
  already built sheet, and the stand measured it (files (4) and (5) byte-equal);
  **v9** — when the sheet stopped receiving the КП user prompt (2026-11-19).
  The section-text cache has its own freshness (`combatSectionHandle` includes
  `PromptText()`), so it rebuilds by itself — the file cache does not.
- **A prompt built for one document crept into another, and the model repeated
  it verbatim** (2026-11-19, live sheet 017/2026). The user's complaint: «в
  листке остаётся какой-то базовый скрытый промт … фраза про „в комплекте
  каждый материал закрывает свой участок объекта“». Two real leaks, both from
  the КП path: (a) `sectionGen.text` built the USER prompt with
  `buildBlockUserPromptSelection`, and the sheet passes `combatSectionOffer`, whose
  `Systems` holds BOTH sides — so `renderSystemsForPrompt` saw 3 systems and added
  its «каждая — на свой участок объекта, и в документе идут в комплекте» rule,
  and the model wrote «В предложении три системы … идут комплектом»; (b)
  `effectiveBasePrompt("")` fell back to `sectionAnswerContract`, which says
  «Рисунок — ссылка на картинку из КП». Now the sheet has its own data text
  (`combatSectionFacts`: number, title, НАШ ПРОДУКТ / ПРОДУКТЫ КОНКУРЕНТОВ, no
  prices) and its own contract (`combatAnswerContract` — the same frozen answer
  with that one sentence replaced), wired through `sectionGen.userPrompt`; the
  shared tail (cards, refs, "напиши только текст этого блока") stays one function
  (`buildSectionUserPrompt`). **A document-specific vocabulary is a data seam, not
  a prompt detail**: if two documents describe different things, they must not
  share the facts text. Cache: `combatSectionPromptRev` in `combatSectionHandle`
  (the prompt itself is invisible to the key) + `combatDocxVersion` v9. Guards:
  `combat_prompt_leak_test.go` — the sheet prompt has no «предложени/комплект/
  участок объекта», the КП prompt keeps its комплект rule, the sheet's default
  contract does not say «КП».
- **A combat sheet's section `id` is an identity, not a label** (2026-11-18).
  The text cache key is `gen.sect.<sectionID>.<subsectionID>[.ins<hash>]…`
  (`sectionCacheKey`), so renaming a section id orphans every text written under
  the old key: the sheet silently calls the model again for that section while
  the old rows sit in `form_values` forever. That is why the combat editor shows
  the id read-only (`combat: true` in `SectionsEditorDeps`) and why a NEW
  section gets `section_<timestamp>`. A subsection id has the same property.
- **An empty `sections` list MEANS the default for a combat sheet** (2026-11-18).
  `renderCombatSections` substitutes `combatDefaultSections()` when the list is
  empty, so "empty" cannot mean "print only cards" — the document always has
  comparison sections. Therefore `loadForm` fills an empty list with the default
  too (`withCombatSectionsDefault`); before that the settings window said
  «разделов нет» while the document printed four sections (found on the stand,
  which had exactly such a stored row). Saving an empty list is REFUSED
  (`combatEmptySectionsProblem`, same text client-side) — otherwise the admin
  would "configure" something that does not exist. The narrow exception to
  "empty arrays in the DB are a configured empty form" (0.10.3) applies here and
  only here: for the sheet, the empty state does not exist.
- **The built-in default must travel to the editor** (2026-11-18). «↩ Вернуть
  заготовку» cannot read Go code, so `GET /form?mode=combat` carries
  `sectionsDefault` (read-only, never written, not part of
  `combatFormFingerprint`). The plugin puts a DEEP COPY into the editor
  (`cloneSections`): the editor mutates sections in place, so reusing the
  server's object would spoil the default after the first edit.

- **TWO card caches, and the combat sheet uses `system_cards`** (2026-11-18,
  live sheet 017/2026). `POST /systems/refresh` writes `offers_context(mode='systems')`
  (the «Системы» screen); the sheet and the offer print from
  `mode='system_cards'`, refreshed by **`POST /context/refresh`** («Контекст КП»,
  admin). The settings hint used to send people to «Системы», so a manager
  refreshed the wrong row and the document kept the old links. The sheet prints
  from the cache and never reads the KB at export. If a card with `noteId` has no
  refs, the document now says so in a visible line. `systemCardsForCombat` must
  seed the cache through `refreshSystemCards`, never `SystemsFromKB`: the latter
  drops `material` cards and `LinkedRefs`, and the poisoned cache then hides our
  own material from every sheet and offer.
- **A list must not re-read a cache per row** (2026-11-18). `loadCombatSheet`
  repairs competitor titles and used to call `loadCachedCompetitors` itself; in
  the list that meant 1 + 2N SQL queries and N unmarshals of the same large
  JSONB (200 sheets → 401 queries). Lists now load the cache once and pass it to
  `loadCombatSheetWithCards`. **The list still returns the full sheet by
  default**; the short DTO is an OPT-IN (`GET /combat?fields=preview`,
  `CombatSheetPreview`) because an old client reads the full sheet from the list
  and saves it back whole — a preview row would wipe `form_values`. The
  Mini App asks for the preview (`listPath` in `offers-app/src/lists.ts`), the
  Obsidian plugin does not. Measured 2026-11-18 on the stand with 200 sheets of
  8 KB: full answer 1610 KB / 87 ms, preview 56 KB / 72 ms — the server was
  never the problem after 0.11.8, the payload was. Tool:
  `scripts/measure_combat_list.mjs`. An unknown `fields` value returns the full
  sheet: it is an option, not a mode.
- **Deleting a model's `[НУЖНО: …]` marker leaves a SEAM** (2026-11-17, live
  sheet 017/2026): «камеры, [НУЖНО: уточнить], иначе» → «камеры,, иначе» and
  «по техлистам: [НУЖНО: цифры].» → «по техлистам:.». `stripFactNotes` now
  collapses `,,` / `;:` / `:.` / `..` and drops leading separators, and keeps
  the words (inventing the missing text is worse). **The semantic hole stays**
  — the sentence reads odd; the real cure is not putting the marker
  mid-sentence, which is a prompt decision, not a printer one. Guard:
  `TestStripFactNotes` (the reverted printer reproduces «камеры,, иначе»).
  **A marker used as a sentence of its own leaves a DANGLING CLAUSE** (2026-11-19,
  the same live sheet): the model wrote «… ГОСТ Р 59985-2022. [НУЖНО: сверить]
  — тогда расчёт толщины пойдёт по сопоставимым величинам.», and after the cut
  the page carried «…. — тогда расчёт толщины…» with no start of phrase (four
  such fragments in one export). `reSeamDanglingDash` drops «— <lowercase …>.»
  **only when the marker itself followed a sentence end** (`noteOwnSentence`):
  a dash in the middle of a sentence is the author's punctuation and must stay
  («Плита — не мембрана» survives; the test pins both directions). The same
  limit as above applies — texts cached before the fix keep their fragments
  until their section is regenerated; the sheet now has
  `combatSectionPromptRev p2`, so it regenerates itself.
- **A layout marker describes the FORMAT, not the content** — strip it, keep the
  text.
- **A link printed into a customer document must open for the CUSTOMER**
  (2026-11-19, live complaint «при попытке открыть документы в s3 из боевого
  листка возвращается `unauthorized`»). The Knowledge Base writes a PERMANENT
  link `GET /api/documents/file?key=…` into a card, and that route sits behind a
  person's JWT (`requirePerm("viewer")` in documents-service). A DOCX opened in
  Word or Telegram has no session, so the link could never work — the sheet
  017/2026 carried two of them. Printing must replace such a link with OUR
  permanent signed link: `GET /api/offers/files?key=…&sig=…`, `sig` = first 32
  hex of HMAC-SHA256(`documents\0` + key, `OFFERS_SERVICE_SECRET`). The signature
  is deterministic (the DOCX cache hash must not wobble) and has no expiry (the
  document may be forwarded a month later); the route is public ON PURPOSE — its
  gate is the signature — and answers `302` to a fresh 7-day presigned URL from
  the existing internal `/internal/documents/agent/file-link`, which still
  enforces `product_refs` + `no_agent`. Detach the card from the product and the
  link dies: access is decided by data, not by the text in the document. One
  place does the rewrite — `collectOfferRefs` (offer, sheet, layout `{{ссылки}}`,
  bot door, file hash all funnel through it). **Check: print a link and open it
  without a token** — a test that only greps the URL out of the DOCX proves the
  text, not the access.
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
- **Links are printed as ONE numbered list, in the order of mention**
  (2026-11-17, user's direct reversal of the 2026-11-10 grouping). «Нужно убрать
  деление ссылок на типы, все ссылки (в том числе на иные документы) приводим в
  порядке их упоминания в заметке. Ссылки на ресурсы конкурента не приводим».
  So: `collectOfferRefs` does NOT sort (dedupe by URL only); `cardAllRefs` is
  `card.Refs` (parse order of «## Ссылки» then «## Иные документы» — documents
  from the documents app, S3 files, since 2026-11-17) then `card.LinkedRefs`
  (composition order); `renderLinkSection` and `renderLayoutCardLinks` print one
  list with one `<w:numId>`, no group headings.
  **A documents-app line carries a tail** — `… — ДС ФЗ 123 <!-- kb-doc:… -->` —
  so the markdown regex must NOT be anchored at `$`, HTML comments are stripped,
  and a bare URL's label is cut at « — » (`trimCardRefNote`). Live fragment and
  guards: note 6363, `TestParseCardRefsIncludesOtherDocuments`.
  The whole grouping machinery is DELETED
  (`skills/cardrefs.go` group/owner code, `skills/linkgroups.go`, the
  `link_groups` form field, the plugin's link-groups editor); the DB column
  `offers_form.link_groups` stays unused so old data is not destroyed. **The
  combat sheet prints OUR links only** — `buildCombatBody` no longer appends
  `collectOfferRefs(competitorCards)`: a competitor's site in the reference list
  reads as our recommendation to the customer. **The built-in sheet view prints
  the links section itself** (`buildCombatBody` with `hasLayout=false`); with a
  layout the `{{ссылки}}` marker prints them, and the body is NOT appended —
  otherwise the document would carry two lists.
  **`applyHyperlinks` locates the numbered item by its text and wraps only its
  run, keeping `<w:pPr>`/`<w:numPr>`** — rebuilding the paragraph as a plain
  `hyperlinkXML` would silently drop the number.
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

- **A Mini App key is issued to ONE app and cannot read another app's API**
  (2026-11-23). `kb-service` compares `claims.app_id` with its own and answers
  `403 «ключ выдан другому приложению»`, so the offers JWT cannot open
  `/api/kb/digest/*` — and the same holds for every service in the system. Data
  of a foreign service goes through OUR service: `offers-service` reads the KB
  with its service-to-service token (`KBClient`) and hands the Mini App a short
  view (`appdigest.go`). Do not «fix» this by loosening the foreign check and do
  not put the KB token into the browser. `requireServiceSecret`/JWT gates stay
  as they are: the digest feed is `requirePerm("viewer")`, the chat send is
  gated by the Telegram signature.

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
  **2026-11-17 — the same complaint came back from another door.** Live sheet
  017/2026 stored `competitor_notes = [{"title":"Материал конкурента…","noteId":6654}]`
  while 016/2026 had the same note as «PIRRO®Membrane»: the agent added the side
  through `readNoteCard` → `systemCardFromParsed`, which takes `parsed.Title`
  (the FIRST H1). Now (a) `readNoteCard` builds a competitor card with
  `competitorCardFromParsed` (the title depends on the note's ROLE, not on the
  door it was read through), (b) at the side-write boundary the server takes a
  competitor's title from the card itself (`combatCompetitorTitle`,
  `competitorRefTitle` in `competitors.go`) and refuses to store a disclaimer or
  an empty one, (c) already-broken sheets are repaired ON READ
  (`withRepairedCompetitorTitles` from `loadCombatSheet`, titles from the
  competitors cache — no silent UPDATE, the next save persists it), and
  (d) `competitorCardFromParsed` falls back to the note's file name when the
  body has no second H1. Guards: `combat_titles_test.go` (the reverted
  `readNoteCard` prints the disclaimer — verified red).
- **The side search shows nothing until the manager types** (2026-11-17, user
  request). Both combat-sheet blocks («Наш продукт» and «Конкуренты») use ONE
  component (`SideSearch` in `offers-app/src/combat-screen.tsx`), and its rule
  is in `offers-app/src/combat.ts`: `shouldSearchSide` (empty field — no request
  at all, only the hint «начните вводить название») and `sideSearchResults`
  (`COMBAT_SEARCH_LIMIT = 5`, the server is asked with `limit=5`). An empty
  query used to return the whole cache and the manager started by reading a
  page of foreign products. Tests: `offers-app/test/combat.test.ts`.
  **The Obsidian windows follow the same rule since 0.11.14** (live complaint
  «показываются продукты до начала ввода»): `pickerRows` in
  `src/ui/picker-model.ts` returns nothing for an empty query, and BOTH
  `picker.ts` and `combat-view.ts` use it. They used to show `found.slice(0, 20)`.
- **A chosen product must be signed with its NAME, not its handle** (2026-11-21,
  live complaint «после выбора показываются коды»). The server stores only
  `noteId`/`sku` in `system_choice`/`material_choice`, and the window used to
  resolve the label from the LAST search result — so after a second query, or
  after reopening the offer, the list showed `sys-412` / `MAT-…`. The window now
  keeps a name map (`rememberNames`) filled by the catalogue load and every
  search, and `refTitle` signs the selection. The catalogue is re-rendered after
  `load()`; the Mini App loads the material catalogue for the same reason
  (`loadMaterialCatalog`, `chosenMaterials(offer, catalog)`). A missing
  catalogue still falls back to the handle — an empty label is worse.
- **A card without a KB note needs a non-empty SKU, or it vanishes / overwrites**
  (2026-11-17, `claude-5-opus` + `gemini-4-argon`, both confirmed in code).
  Text competitors have `noteId == 0`; `Ref.Empty()` (`model/offer.go:60`) is
  `SKU == "" && NoteID <= 0`, and `RefList.Add` **drops** such a ref, so
  `combatSectionOffer` would silently lose it and `systemCacheHandle`
  (`docx_text.go:87`) would return `"0"` for ALL of them — one `.sys0` cache key,
  sections overwriting each other's text. That is why `textCompetitorSKU`
  hashes the key into `text-<hash>`: unique cache key, `FindSystem` finds it,
  `containsCard` does not merge two competitors. Guard:
  `TestTextCompetitorCardsGetDifferentCodes`.
- **Full sheet save does not know about the new column** (2026-11-17). `PUT
  /combat/{id}` (plugin, old app builds) marshals `competitor_notes`,
  `form_values`, `title` — and, since this release, `text_competitors` too. But a
  client that has never heard of `text_competitors` sends nothing, so the rule
  is «empty means untouched»: `keepStoredTextCompetitors` keeps the stored list,
  and DELETION is its own route (`DELETE /combat/{id}/competitor-text`) — never
  the full save. Without this, opening a sheet in Obsidian and pressing
  «Сохранить» erased everything read from files and links.
  Guard: `TestKeepStoredTextCompetitorsOnSave`.
- **Reading a URL: the method and the status are part of the check**
  (2026-11-17). `POST /combat/{id}/competitor-from-url` takes ONLY `url` from
  the body: the method is `GET` in code (the `agent-service` client passes
  `Method`/`Headers` through, so a client-supplied method would be a hole), the
  scheme is `http`/`https` only, the length is capped, and a non-2xx answer is a
  REFUSAL — otherwise a «404» page became the competitor's description. An empty
  text is a refusal too (same rule as `newCombatFact`). `/internal/agent/fetch`
  returns RAW HTML (checked live 2026-11-17), so the page is cleaned here
  (`combatPageTitleAndText`: `<script>`/`<style>` dropped, `<title>` → name,
  entities decoded). Guards: `TestFetchURLProblemRules`,
  `TestCombatPageTitleAndText`.
- **An empty list in a JSON response must be an ARRAY, never `null`**
  (2026-11-11, live crash «Cannot read properties of null (reading 'length')»
  right after uploading a layout). Go's `encoding/json` writes a nil slice as
  `null`, and any client doing `fields.length` throws. `describeLayout` now
  returns `[]string{}`; the plugin also normalizes the response on the boundary
  (`normalizeLayoutResponse`). Regression test reads the RAW JSON — a decoded
  nil slice and an empty one are indistinguishable.

- **A document button must carry the document's id; a lost session is not a
  hint** (2026-11-20, live complaint «нажимаю DOCX — приходит файл самого
  первого листка»). `restoreSession` rebuilds only `{step, email}`, so
  `session.combat` is gone after any restart while every button in the chat
  history stays live forever. The DOCX button used to resolve its sheet through
  `ensureCombat`, whose fallback took `sheets[sheets.length - 1]` from
  `api.listCombat` — and that list is `ORDER BY created_at DESC`, so the LAST
  element is the **oldest sheet in the whole database**, and not even the
  author's. Worse, the guessed id was written into `session.combat`, so the next
  «добавить сторону» edited the wrong sheet (same author ⇒ the server allowed
  it, so it failed silently). Rules that now hold: the printing/closing buttons
  carry the id (`cb:doc:<id>`, `cb:done:<id>`, built by
  `combatKeyboard(sheetId)`); old messages without an id get an honest «какой
  листок» list (`cbx:<id>`) — always, even when the author has exactly one sheet,
  because the message may have been about a colleague's sheet (sheets are read by
  everyone); `ensureCombat` is for ACTIONS only and falls
  back to the **newest sheet of the chat's author** (`ownerEmail`), never to the
  oldest and never to someone else's; `loadCombatById` reads that one sheet
  through `GET /bot/combat?id=N` instead of scanning the top 50; «Готово» closes
  only when the id matches the open sheet. Server order is
  `ORDER BY created_at DESC, id DESC` (the `id` tie-breaker makes "newest"
  defined). Guards: `routes.test.ts` (the stub now returns DESC with real
  `createdAt` and an `ownerEmail`, foreign sheets are seeded) and
  `combat_list_query_test.go`; the four bot tests were red on the old code, and
  reverting the fallback turns «самый свежий свой» red again.

- **Bot session state does not survive a restart; the sheet must be
  re-derived** (2026-11-11, live complaint «нажал — ничего не произошло»).
  `restoreSession` rebuilds only `{step, email}`, so `session.combat` is gone
  while every combat button is still on screen — and each answered «листок не
  открыт». `ensureCombat` loads the sheet from the session or falls back to the
  newest sheet of this author (`api.listCombat`) and refills the session; all
  sheet-level ACTIONS go through it. Any new button that needs `session.combat`
  must too — but a button that PRINTS or CLOSES a document must not: it carries
  the id (see the rule above).

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

- **Word is a CONTEXT source, not a composition source** (2026-11-18). A `.docx`
  from the chat is read through `documentTextOf` — the same parser the agent uses
  (`w:t` nodes, a newline per `w:p`, a space per `w:tab`), so there is no second
  copy of "how to read a docx". After that it follows the `.md` path exactly:
  strict labels (`ParseMarkdownAttachment`) → brief, `ExtractBriefFacts` + model →
  object context. **Positions are NOT collected from Word**: `w:tbl` arrives as
  plain text lines, and `attachReply` names that boundary instead of pretending.
  A file with no text at all (a scan without a text layer) is refused with the
  reason, not with «такие файлы не принимаю». Guards: `attach_docx_test.go` (text
  reaches the brief; a text-less Word is refused for the right reason; the reply
  says composition is not parsed) and the stand section «11. Файлы в бота» — the
  stand's old `.docx` check sent `PK\x03\x04` + the string `word/document.xml`,
  which is not a zip at all, so it exercised the unknown-format branch and proved
  nothing about Word. **A check whose fixture cannot reach the code it names is
  not a check.**
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
| `agentservice_client.go` | internal `/internal/agent/{fetch,file/parse,render}` client (step 3 of the 2026-11-16 plan): `AGENT_SERVICE_URL`, `AGENT_INTERNAL_SECRET`, refusal when not configured. **No second network path**: SSRF, limits and docx/xlsx/pdf parsing stay in `agent-service` |
| `agenttools_step4_test.go` | the step-4 tools and the code boundary: 17 names, refusal without the service, `parse_file` needs an attachment, compact HTML view, base64 attachments |
| `offers-bot/src/flow/agent.ts` + `api.agentMessage` | bot side: «🤖 Помощник» button, «⌛» → answer, files to chat, `attachments` in the turn body |
| `offers-bot/src/flow/agentfile.ts` | приложенный файл в режиме помощника: файл запоминается в сессии до ближайшего текста, ход его забирает и очищает, предел 16 МБ |

**Tools** (whitelist, `agentTools`; after P5b 2026-11-14 — 15; **after step 4 of
the 2026-11-16 plan — 17**; **after step 3e 2026-11-17 — 18**):
`get_our_products`, `read_card` (**any** note), `search_competitors`,
`fetch_url`, `parse_file`, `remember_sheet_fact`,
`get_documents`, `get_document_link`, `read_document_file`, `get_emails`,
`read_email`, `create_offer`,
`set_system`, `set_material`, `create_combat_sheet`, `set_combat_side`,
`build_docx`, `build_combat_docx`. **Not** in the list: YouGile,
rules/skills/presentation, any write to YouGile. Emails are read-only and only
for letters whose signed-scan card carries `product_refs` (`agentmail.go`).
`fetch_url` reads only an address the manager named, `parse_file` only a file
attached to the turn (`agentEnv.Attachments`); both are boundaries in CODE.
`remember_sheet_fact` takes text **only from what was read in this turn**
(`agentEnv.readSources`, filled by `fetch_url`/`parse_file`) — a fact cannot be
invented; writes are gated by the sheet's owner (author or admin).

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

## План 2026-11-16 — боевой листок на разделах + агент (файлы/интернет) + схемы

План: `C:\Obsidian\mailers\docs\superpowers\plans\2026-11-16-sbe-offers-combat-sections-and-agent-tools-plan.md`,
вход: `...-START-HERE.md`, спека: `...-design.md` (§3.2, §7).
Версия плагина не меняется до шага 7; всё, что ниже, живёт на `backend`.

**Шаг 1 (sbe-agent) — готов и запушен 2026-11-16.** Три внутренние ручки
`/internal/agent/{fetch,file/parse,render}` под `X-Service-Secret`
(`AGENT_INTERNAL_SECRET`), `render` — только mermaid→PNG. Коммиты
`sbe-agent` `e32aadd` (код) и `f1e178e` (v0.5.6 + новость ЦУП),
`sbe-core` `f89a71a` (compose + `.env.example`). Подробности —
`sbe-agent/agent-service/AGENTS.md` и `sbe-agent/AGENTS.md`.

**Шаг 3 (клиент agent-service) — сделан 2026-11-16, не коммичен.**
`offers-service/agentservice_client.go`: `AGENT_SERVICE_URL` (default
`http://agent:3000`), `AGENT_INTERNAL_SECRET`, `configured()`,
`Fetch`/`ParseFile`/`Render`; `configured()==false` → `errAgentServiceNotConfigured`
(отказ, не пустой ответ); ошибки сервиса — текстом с кодом и причиной;
`data:null` толерантно. Тесты `agentservice_client_test.go` — 4 проверки
(секрет уходит на все три ручки, отказ без окружения, текст ошибки сервиса,
разбор ответа).

**Шаг 4 (инструменты `fetch_url` / `parse_file`) — сделан 2026-11-16,
не коммичен.** В `agentTools()` теперь **17** имён, в `agentToolDescriptors`
порядок обновлён. `agenttools_impl.go`: `toolFetchURL` (адрес называет человек,
метод — белый список, SSRF — в agent-service; `compactFetchView` режет HTML до
SCRIPTS/LINKS/TEXT, JSON — до 12 КБ), `toolParseFile` (файл — только из
`agentEnv.Attachments`, имя не угадывается; несколько файлов — по `name`).
`agentEnv.Attachments` + `agentRequest.Attachments` + `botAgentMessageRequest
.attachments[{name,base64}]` → `decodeAgentAttachments` (пусто/битое/больше
16 МБ — 400 всего хода, а не молчаливый пропуск). Промт: две строки под «Что
важно помнить». Бот: `flow/agentfile.ts` (файл в сессии до ближайшего текста,
ход забирает и очищает, предел 16 МБ; `agentfile.test.ts` — 3 проверки),
`poller.ts` (файл при включённом помощнике не идёт в состав КП), `api
.agentMessage(..., attachments)`. Стенд: `scripts/agent-stub.mjs` (форма
настоящих ручек, секрет сверяется), `docker-compose.local.yml` (+`agent-stub`,
+`AGENT_SERVICE_URL`/`AGENT_INTERNAL_SECRET` в `offers`), `llm-stub.mjs`
(сценарии «прочитай ссылку»/«прочитай файл»), `offers_local_e2e.mjs` —
секция «Агент читает файл и ссылку» (9 проверок).

**Проверки шага 3–4 (2026-11-16):** `gofmt -l` пусто, `go vet` 0,
`go test -count=1 ./...` зелёный (новые 4 + 5 проверок); бот `tsc` 0,
`node --test` **215/215** (+3), `build` OK; OpenAPI **121/121**; стенд
**565/0** (+9). Грабля стенда: заглушки монтируют `scripts/` только для
чтения, а Node читает `.mjs` один раз при старте — после правки `llm-stub.mjs`
нужен `docker restart offers-local-llm-stub`, иначе сценарии не видны.

**Не проверено:** живой вызов ручек agent-service (секрет на бою не задан,
раскатка — по команде), живой ход помощника с настоящей моделью и Telegram,
характеристики настоящего mermaid-рендера (стенд отдаёт PNG 1×1).

**Шаг 2a (разделы боевого листка) — сделан 2026-11-16, не коммичен.**
Печать листка (`combat_docx.go`) больше не зашита: после перечня продуктов
идут СОДЕРЖАТЕЛЬНЫЕ РАЗДЕЛЫ формы `mode='combat'` тем же конвейером, что у КП
(`sectionCalls` → `renderSectionSubsection`), новый файл `combat_sections.go`.
Запасной вид — прежний: общий текст сравнения и карточки (`writeCombatCards`),
так документ никогда не пуст. Уточнения модели: бриф подраздела называет
**обе стороны** (у листка их две, у КП карточки «наши»); контекст модели —
карточки обеих сторон; отпечаток содержания (`combatSectionHandle`) включает
текст карточек — правка карточки конкурента переписывает раздел.
`sectionGen` получил `brief` (подмена брифа; разбор ответа и кэш те же),
`sectionMaxWords` — одно правило длины на оба пути.
`combatDefaultForm()` теперь несёт **заготовку разделов**: «Общее сравнение»
(коротко + характеристики ТАБЛИЦЕЙ), «Где конкурент сильнее — и что
отвечать» (всегда), «По каждому продукту» (`repeat: systems`). Блоков вёрстки
по-прежнему нет — у листка только макет с меткой `{{сравнение}}`.
Тесты: `combat_sections_test.go` (6), `combat_form_default_test.go` (5),
старые печати через `combatNoForm` (сервер без базы = запасной вид).

**Проверки шага 2a (2026-11-16):** `gofmt -l` пусто, `go vet` 0,
`go test -count=1 ./...` зелёный (прогнан дважды; пойман неустойчивый тест —
заглушка модели искала маркер только в user-промте, а бриф листка живёт в
system); OpenAPI 121/121; стенд **573/0** (+8, секции листка и агента).

**Не проверено по шагу 2a:** живой листок под настоящей моделью (стенд —
`llm-stub`).

**Шаг 2a запушен: `backend` `d48434a`** (HEAD).

---

## ТЕКУЩЕЕ СОСТОЯНИЕ — 2026-11-17 (0.11.4 выпущена; план 2026-11-16 закрыт)

**Ветка `backend` плагина `sbe-offers`.** План 2026-11-16 пройден целиком:
шаги **1** (`sbe-agent` `e32aadd`/`f1e178e`, `sbe-core` `f89a71a`), **2a**
(`d48434a`), **2b** (`709b72e`), **3**, **4** (`1d94d4f`), **3e** (`673524e`),
**4m**, **5**, **6** вошли в 0.11.4 (`1b3c4be`), шаг **7** — выпуск 0.11.4
(2026-11-17). Точки выпуска, хэши сборки и живая сверка — `HANDOFF.md`,
«Три точки сходятся (0.11.4)»; здесь не дублируются.

**Что сделано после выпуска (без bump версии, только бэки):** правка пяти живых
дефектов листка 016/2026 (`f226a05`) и второй заход по листку 017/2026 —
характеристики карточки доходят до модели, чистка швов от «[НУЖНО: …]»
(`38f8d79`), затем снятие обрезки характеристик по 15 строкам (`b4fa415`). Все
три кода выложены. Живая пересборка после второго захода: 017/2026 — колонка
нашего продукта заполнена (λ10, сжатие, паропроницаемость, температуры, толщины,
габариты), «не указано» осталось 6 — все честные; 016/2026 — наша колонка без
пропусков из 15 строк, сравнение свелось в одну таблицу. Подробности и точки
выкладки — `HANDOFF.md` (2026-11-17).

**Сверка кода с выкладкой (2026-11-17):** `/opt/mailers/offers-service` и
`/opt/mailers/offers-bot` байт в байт совпадают с `git archive HEAD` (образы
собраны после коммита), самораздача и `registry.json` отдают те же хэши, что
локальная сборка. Пересборка `main.js` воспроизводит те же байты.

**Что дальше (по порядку):** **2c** — у КП ключ кэша систем всё ещё `noteId`,
поэтому правка тела карточки не переписывает текст (у листка закрыто в 2b);
кнопка «файл/ссылка» на экране листка в Mini App (второй вход в помощника —
4m); затем «Next iterations (backlog)» ниже.

### Шаг 3e — устойчивые факты листка (2026-11-17)

Добытое агентом (ссылка заказчика, приложенный файл) больше не живёт только в
разговоре: оно записывается **у самого листка** и входит в отпечаток
содержания раздела, поэтому правка факта переписывает раздел, а повторная
сборка без правки берёт готовый текст (спека §9.1).

| файл | что |
|---|---|
| `offers-service/combat_facts.go` | `combatFact{Kind,Source,Digest,Text}`, `newCombatFact`, `factDigest`, `combatFactsFromValues`, `parseCombatFact` (формат `kind \| source \| digest` + текст второй строкой — читается в psql), `combatFactKey` → `gen.combatfact.NN`, `withCombatFacts` (тот же источник ЗАМЕНЯЕТСЯ, предел 12, обрезка текста 6 КБ), `combatFactHandle`, `combatFactsPrompt`, `persistCombatFacts` (только `form_values` листка, стороны не трогаются) |
| `offers-service/agentfacts.go` | инструмент `remember_sheet_fact` + `agentSheetWriteError` (автор или админ; ниль-пул проверяется ДО `loadCombatSheet` — иначе паника pgxpool) |
| `offers-service/agenttools.go` | `agentEnv.readSources` / `noteRead`; инструмент в `agentTools()` и в `agentToolDescriptors` (18 имён) |
| `offers-service/agenttools_impl.go` | `fetch_url` (2xx) и `parse_file` пишут прочитанное в `readSources`; `parseFileText` (таблица → строки `\ | `, json → JSON, иначе текст; лист приходит `map[string]any`, а не нашим типом) |
| `offers-service/combat_sections.go` | факты в брифе (`combatSectionBrief`), в отпечатке содержания (`combatSectionHandle`) и в признаке набора (`combatSelectionHandle`) |
| `offers-service/agentprompt.go` | правило: запоминать только прочитанное в этом ходе, справку — нет |
| тесты | `combat_facts_test.go` (9), `agentfacts_test.go` (5) — границы: пустой/чужой источник, порядок по номеру ключа, замена того же адреса, предел, правка факта меняет ключ, порядок в брифе, чужие ключи не факты |

**Правила, которые теперь держатся:**
- **Факт не придумывается**: `remember_sheet_fact` берёт текст только из
  `readSources` этого хода — модели «вспомнить» нечего, если никто не читал.
- **Тот же адрес — один факт**: повторное чтение заменяет текст (ради этого
  хранится sha256), а не плодит записи; иначе предел кончался бы за три хода.
- **Пустой набор фактов = пустой отпечаток**: у листков без фактов старые
  тексты разделов не переписываются при выкатке.
- **Предикат `errTool` — второй величиной НЕ возвращается**: отказ приходит в
  `agentToolResult{OK:false, Error:…}`. Первая версия тестов этого не знала и
  «зеленела» на отказах — исправлено (`factRefused`).

**Не проверено по 3e:** живой ход агента под настоящей моделью (стенд —
заглушки); запись факта на стенде (нужен живой `combat_sheet` под тем же
пользователем — проверено юнит-тестами и ручными вызовами `persistCombatFacts`
на двойнике), поэтому в `offers_local_e2e.mjs` новых проверок нет.

### Шаг 4m — Mini App и помощник (2026-11-17, исследовано: править нечего)

Пункт плана звучит как «Mini App не донесёт файл/ссылку». Для файлов это
по-прежнему верно (inline-кнопка, `sendData` не работает — 0.9.27), и путь к
помощнику — бот. Проверено чтением кода: `ask.ts` покрывает «продолжить в
боте»/«прислать файл для позиций», у листка есть `SendToChatButton`;
`flow/agent.ts` зовёт `api.agentMessage(chatId, text, agentDocument(session),
files)`, а `agentDocument` **уже отдаёт `sheetId`** — значит
`remember_sheet_fact` (шаг 3e) работает из бота по открытому листку без правок
Mini App.

Чего нет: кнопки «файл/ссылка» внутри экрана листка как отдельного действия
помощника. Второй вход (файл из приложения → тот же бот) — дубль потока, а не
починка дефекта. Оставляем до живой жалобы.

### Шаг 2b — кэш текста раздела листка (2026-11-17, запушен `709b72e`)

Тексты разделов листка пишутся в `form_values` самого листка (`gen.sect.*` +
`gen.section`) и берутся из базы на повторной сборке; правка карточки
конкурента переписывает текст. Файлы: `combat_sections.go`
(`persistCombatSectionTexts`, `combatSelectionHandle` с отпечатком содержания),
`combat_docx.go` (`renderCombatDocx`/`buildCombatBody` — методы `*Server`),
`combat_sections_cache_test.go` (3 сторожа). Стендовой секции нет (решение
пользователя, вариант «б»): юнит-сторожа + существующая проверка «повторная
выгрузка листка отдаёт тот же файл».

### Как устроен кэш 2b (уже работает)

- Ключ: `sectionCacheKey(sectionID, call, selection)` —
  `gen.sect.<раздел>.<подраздел>[.ins<hash>][.sel<hash>][.sys<noteId>]`.
- `sectionGen.section` и `sectionGen.selection` **обязаны совпадать по
  содержимому**: свежесть карты проверяется как
  `previous[genKeySection] == g.section`, а ключ строится по `selection`.
  Поэтому `combatSelectionHandle` принимает **и отпечаток содержания**
  (`content`), иначе правка тела карточки не меняла ключ (грабля 2026-11-08).
- `combatSectionHandle` = номер листка + уточнение + `noteId:title` и
  `card.PromptText()` **по обеим сторонам**. Ссылки и картинка в отпечаток
  не входят намеренно.
- `persistCombatSectionTexts` пишет `merged[genKeySection] = gen.section`
  вместе с текстами — **без этой строки кэш не работал бы**: тексты лежали бы
  в базе, но свежесть не подтверждалась, и модель звалась заново на каждой
  сборке. Настоящая находка, не косметика.
- `sectionGen` получил поле `brief` (шаг 2a) — подмена брифа; разбор ответа
  и кэш общие.

### Почему НЕ секция в `llm-stub` (история, 2026-11-16/17)

Проверки стенда идут через **общий `llm-stub.mjs`**, который обязан
различать **три** вида промта:

1. подраздел документа (в системном промте есть `Ответь ТОЛЬКО JSON`) —
   структурный ответ `{"blocks":[…]}`;
2. старый промт сравнения листка (`combatCompareSystem`, **без** JSON) —
   текст, зависящий от уточнения; на нём держится проверка «после смены
   уточнения файл собирается заново»;
3. промт помощника-агента и остальные 573 проверки.

Любая перестановка порядка ветвей роняет одну из групп. Проверенные тупики:
резать бриф по первому `"blocks"` — отрезается инструкция (в контракте есть
пример таблицы); по последнему `Ответь ТОЛЬКО JSON` — бриф пустой; ветка 2
раньше ветки 1 — перехватывает подразделы; условие `/Результат тула/`
в ветке 2 — ломает агентские сценарии (8 проверок).

**Решение, к которому я пришёл:** не смешивать в `llm-stub` — кэшевые
проверки жили бы **отдельной секцией против `smart-llm-stub`** через
сервис-«двойник» `offers-smart`. `smart-llm-stub.mjs` и compose-сервисы
(`smart-llm-stub`, `offers-smart`) в дереве ЕСТЬ и поднимаются.
**Стендовой секции кэша НЕТ** — решение пользователя 2026-11-17: вариант «б»,
юнит-сторожа + существующая проверка «повторная выгрузка листка отдаёт тот же
файл». Причина, по которой секцию не довели: `offers_local_e2e.mjs` — файл,
который правится только целиком, а одноразовый «прогон одной секции» требует
вставлять в него код, которого нет в репозитории; три подхода (снятие вызовов
секций, комментарии вместо кода, сборка из «верхнеуровневых блоков по скобкам»)
все упёрлись в одно: скобочный баланс файла держится на строках-шаблонах с
`${}`, и вырезание блока ломает синтаксис в НЕОЖИДАННОМ месте (ошибка
приходила на 300–800 строк ниже вырезанного). Вывод: если секция когда-нибудь
понадобится — писать её **в сам файл** и гонять полный прогон, а не собирать
файл из частей.

### Что подтверждено вручную на двойнике (2026-11-17, не стендовая секция)

Двойник `offers-smart` (та же база, `LLM_SERVICE_URL=http://smart-llm-stub:3000`,
**свой** `OFFERS_SERVICE_SECRET`) поднят и отвечает; проверено ручными вызовами
его ручек: создание листка, первая сборка DOCX, вторая сборка тем же файлом,
`gen.sect.*` + `gen.section` в `form_values` листка, стороны не тронуты,
смена `hint` пересобирает файл. Это подтверждение поведения, а не сторож в
прогоне — при следующем заходе его надо оформить проверкой в самом стенде.

### Две грабли двойника (стоили времени, записать)

- **`OFFERS_APP_ID` порождает имя переменной с дефисом.** `appIDFromEnv()`
  (`jwt.go:72`) — это `OFFERS_APP_ID`, и ключ JWT собирается как
  `strings.ToUpper(appID) + "_SERVICE_SECRET"`. С `OFFERS_APP_ID=offers-smart`
  ключ становится `OFFERS-SMART_SERVICE_SECRET`, а контейнер в него **не
  передаёт**: имя с дефисом shell отвергает, а `docker compose config`
  показывает его как обычную переменную окружения — расхождение находится
  только после старта. Поэтому у двойника `OFFERS_APP_ID: offers` (как у
  основного), а отличается он **значением** `OFFERS_SERVICE_SECRET`
  (`test-service-secret-smart`).
- **Секрет двойника обязан быть известен заглушке ролей.** `roles`-ручка
  `auth-stub.mjs` сверяет `X-Service-Secret` со **своим** `OFFERS_SERVICE_SECRET`;
  без второго значения двойник получал `403 forbidden: insufficient role` на
  каждой JWT-ручке (то есть «первая сборка листка» отвечала 403, а не 200).
  Если у двойника свой ключ, в `auth-stub` он добавляется **вторым через
  запятую** — правило «один секрет = одно значение» здесь намеренно нарушено
  ради двойника. `scripts/auth-stub.mjs` в HEAD этот формат **не понимает** —
  правка была, но её откатили вместе с секцией; при возврате к двойнику
  правку надо вернуть (два `SECRETS.includes(...)`).

### Следующий шаг (по порядку)

1. ~~Стендовая секция кэша~~ **и 2b, и 3e — сделаны и запушены** (решение
   пользователя 2026-11-17: вариант «б» — юнит-сторожа + существующая проверка
   «повторная выгрузка листка отдаёт тот же файл»).
2. **Шаг 2c** (backlog плана) — если карточки КП начнут расходиться так же:
   у КП `systemCacheHandle` всё ещё `noteId`, поэтому правка тела карточки
   системы не переписывает текст. У листка это уже закрыто (2b).
3. **Шаг 4m** — Mini App не доносит файл/ссылку; следующая задача по порядку.
4. Дальше: 5 (`diagram`: mermaid→PNG), 6 (предпросмотр/маркеры/справка),
   7 (фиксация — версия из `manifest.json`).

### Грабли этого захода (стоили времени; записаны как правило)

- **Не править большие `.mjs` построчно через PowerShell**
  (`Get-Content` → фильтр → `Set-Content`): куски `offers_local_e2e.mjs`
  терялись и сдвигались несколько раз. Только `edit`/`write` или
  node-скрипт по **номерам строк** + `node --check` после каждой правки.
- **`\\` в regex внутри PowerShell-строки** схлопывается — синтаксис
  ломается в другом месте файла. Такие правки — только целым файлом.
- **Заглушки монтируют `scripts/` на чтение, а Node читает `.mjs` один раз
  при старте**: после любой правки — `docker restart offers-local-llm-stub`
  (или `…-smart-llm-stub`), иначе «исправление не работает» без единой ошибки.
- **Кэш DOCX листка — в `offers_combat_docx`**: перед проверкой «соберётся
  заново» чистить (`delete from offers_combat_docx`), иначе читаешь старый
  файл из кэша.
- **`GET /bot/combat/{id}` не существует** — листок читается JWT-ручкой
  `GET /combat/{id}`.
- **Экспорт бота → JSON `{docx_base64}`, экспорт приложения → zip**: читать
  по первому байту (`0x7b` = `{`), иначе `unzipEntrySafe` вернёт пустую
  строку и «нет таблицы» будет ложным.

### Проверки, которые сейчас зелёные (прогнаны 2026-11-17)

```
cd offers-service && gofmt -l . && go vet ./... && go test -count=1 ./...
cd .. && node ../scripts/check_offers_openapi.mjs     # 121/121
node scripts/check_bundle_strings.mjs                 # 0 отсутствует
node scripts/check_marker_registry.mjs && node scripts/check_sections_conditions.mjs  # 16/16
npx tsc --noEmit && npm run build && node scripts/check_docs_state.mjs
cd offers-bot && npx tsc --noEmit && node --test       # 217 пройдено
docker run --rm --network sbe-offers_offersnet -v "$PWD/scripts:/app:ro" \
  -v "$PWD/offers-bot:/bot:ro" -w /app \
  -e OFFERS_BOT_URL=http://offers-local-bot-stub:3000 -e BOT_DIR=/bot \
  node:22-alpine node offers_local_e2e.mjs http://offers-local-app:3000   # 575/0 (прогон выпуска)
```

Стенд **2026-11-17 не прогонялся**: Docker Desktop на рабочей машине был выключен.
Число 575/0 — прогон при выпуске 0.11.4; последние правки бэков (f226a05 и второй
заход) стендом не проверялись, только юнит-тестами и живой выкладкой.

### Что НЕ проверено

- **Живой листок под настоящей моделью по качеству текста.** Живые листки
  016/2026 и 017/2026 собраны на бою и как раз показали два дефекта (характеристики
  нашего продукта «не указано», швы от `[НУЖНО: …]`). После правки 2026-11-17
  пересборки под моделью ещё не было.
- **Кэш 2b на стенде не сторожится**: второй файл тот же — да (проверка
  `sectionCombat`), но «модель во второй раз не звалась» подтверждено только
  ручными вызовами двойника `offers-smart`, не прогоном (см. выше).
- **Факты 3e на стенде**: юнит-тестами доказано правило и границы, ручными
  вызовами — запись в листок; живого хода «прочитал ссылку → запомнил факт →
  собрал DOCX» под моделью нет (стенд — `llm-stub`).
- **Ручки `agent-service` живьём**: проверены на бою 2026-11-17 — без секрета
  `403`, с секретом `200`, настоящий mermaid с кириллицей отдаёт PNG.

**Дальше по плану:** 2c (у КП ключ кэша систем всё ещё `noteId`), кнопка
«файл/ссылка» на экране листка в Mini App (4m, второй вход в помощника), затем
«Next iterations (backlog)» ниже.

## Next iterations (backlog)

Ordered by what will bite the commercial department soonest.

1. **KB analogue comparison** (why these systems, not the alternatives) — new
   functionality, not started. `compare_table`/`compareSystemRows` compare the
   CHOSEN systems with each other, which is not what is wanted. **The combat
   sheet's own analogue picker IS done** (2026-11-11/12): `/combat/analogues`,
   `/bot/combat/analogues`, «🔍 Подобрать» in the App — what is open here is the
   offer (КП), per the user's correction of 2026-11-18.
2. **Old binary `.xls`** — refused today; cheapest step is a better message plus
   conversion instructions.
3. **`.docx`/`.xls` in the plugin's own upload screen** (`sheet-upload.ts`) —
   not touched since, may share the old positional-cell assumptions.
4. **Multiple sheets** — `ParseXlsxAttachment` stops at the first sheet with a
   composition.
5. **Grouped/hidden rows in xlsx** — `outlineLevel` and `hidden="1"` are ignored.
6. **Manual overrides vs the file** — a file always wins over what the manager
   typed; offer to mark fields as "taken from file".
7. **Editing a saved offer** has no per-item delete; add handlers in
   `flow/resume.ts`, not `callbacks.ts`.
8. **`handleDraftText` is still CC 13** (`callbacks.ts`) — dispatcher mixing
   brief, `система`, free item, quantity and form field. Take it apart with a
   handler table.
9. **`contextWithModel`/`Complete` have no unit coverage** — network glue
   checked only on the stand.
10. **Data work, not code:** `SKU` for materials synced from KB is meaningless
    (`MAT-ПРОЧ-ОБЩ-NNNN`); ~1273 composition lines still have no `[[link]]`.
11. **`{{система}}` inside a `repeat: "materials"` block** is not supported; the
    material repeat prints the chosen material's title (2026-11-13), and positions
    only for offers that have no chosen materials.
12. **A prompt table still costs one model call** in some layout paths —
    `layoutTableFor` builds tables from data, verify no call happens first.
13. **`sbe-kb/src/services/hash.ts`** is a second copy of
    `sbe-core/src/utils/files.ts`.

**Done from this list:** «Word documents as a context source» — `.docx` from the
chat is read as context (2026-11-18, released in **0.11.10** on 2026-11-19; see
the gotcha above). What is NOT done there: parsing Word TABLES into positions —
that is its own step, and the bot says so instead of pretending.