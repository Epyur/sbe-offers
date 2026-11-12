# specification.md — sbe-offers (LogicTEAM.КП)

Форматы обмена плагина с `offers-service` (`https://epyur.fvds.ru/api/offers/*`)
и ручки, которыми пользуется Telegram-бот.

## Авторизация

- Плагин: JWT Bearer `app_id=offers` (ключ из ЦУП —
  `getService('sbe-apstore').auth.getToken('offers')`).
- Роли: `viewer`(1) < `editor`(2) < `admin`(3); действует персональная роль
  или общий доступ приложения (источник — auth-service, `authperms.go`).
  **С 2026-11-13 общий доступ приложения `offers` = `editor`** — так менеджер
  с ролью только в `offers_access` после входа в бота получает полноценный
  доступ и в плагине, и в Mini App.
- Бот: заголовок `X-Service-Secret` = `OFFERS_BOT_SECRET` (ручки `/bot/*`),
  JWT не нужен.
- **Mini App**: JWT, полученный обменом Telegram `initData` на
  `POST /api/offers/app/session` (см. раздел «Mini App»). Этого же формата
  токен знает `requirePerm`.
- 401 — «Ключ недействителен», 403 — «Нет прав».

## Mini App (2026-11-13, шаги S0–S1)

Telegram Mini App (`offers-app/`, React + Vite) раздаётся Caddy по
`https://epyur.fvds.ru/app-offers/`, маршруты — в хэше:
`#/list/offers`, `#/list/sheets?q=…`, `#/offer/{id}`, `#/sheet/{id}`.
Бот передаёт ссылку целиком; серверному роутингу нечего переписывать.

### Внешние ресурсы приложения (2026-11-13, живой дефект)

Приложение **не подключает ничего снаружи**. Скрипт Telegram Mini Apps лежит
рядом с приложением и раздаётся Caddy с нашего домена:

```
/opt/mailers/www/app-offers/vendor/telegram-web-app.js
index.html → <script src="./vendor/telegram-web-app.js">
```

Исходник — `offers-app/public/vendor/telegram-web-app.js` (Vite копирует
`public/` в `dist/` как есть). Обновлять вместе с выпуском приложения.

Почему так, а не `<script src="https://telegram.org/...">`: **сервер в РФ до
`telegram.org` не достаёт** (проверено `curl` с хоста — таймаут). С внешним
скриптом `window.Telegram.WebApp` не появлялся, `initData` был пуст, и
приложение показывало «Открыто вне Telegram» — человек видел «не работает»,
хотя вход был исправен.

`tg.ready()` вызывается из `src/tg.ts`, а `initData` читается оттуда же; при
отсутствии объекта приложение работает как обычная страница (без входа).
Проверка снаружи — `node scripts/check_app_browser.mjs https://epyur.fvds.ru`.

### Кнопки `web_app` в боте (2026-11-13, шаг B+)

`offers-bot/src/flow/appbutton.ts` — единственное место, где собирается адрес
приложения и кнопка. Адрес: `OFFERS_APP_URL` (по умолчанию
`https://epyur.fvds.ru/app-offers/`, хвостовой слеш добавляется сам).

| где | подпись | адрес |
|---|---|---|
| главное меню (`MAIN_KEYBOARD`) | «📱 Открыть приложение» | `#/list/offers` |
| список КП («📂 Мои КП»), на каждую запись | «📱 NNN/yyyy» | `#/offer/{id}` |
| список листков («📂 Мои боевые листки»), на каждую запись | «📱 NNN/yyyy — {продукт}» | `#/sheet/{id}` |

Кнопка Telegram `web_app` открывает приложение **сама**: у неё нет
`callback_data`, боту не нужен обработчик нажатия. Отправка документа обратно
в чат (`tg.sendData` → `message.web_app_data`) — это шаг S7, здесь её нет.

### Вход: `POST /api/offers/app/session`

```
body: { "initData": "<window.Telegram.WebApp.initData>" }
→ 200 { token, user: {email, role}, offers: n, sheets: n }
  400 — строка не той формы
  401 — подпись не сходится или устарела
  403 { error: "not_linked", hint } — верно подписано, но человек не входил в бота
  503 — в сервисе не настроен токен бота
```

Проверка (`offers-service/appauth.go`, `verifyInitData`):

1. разбор `k=v`-строки; **повтор ключа — отказ** (две `auth_date` под одной
   подписью), тела больше 16 КиБ не разбираем;
2. `secret_key = HMAC_SHA256("WebAppData", bot_token)`;
   `hash = hex(HMAC_SHA256(secret_key, data_check_string))`, где
   `data_check_string` — пары `k=v` **по алфавиту через `\n`**, без `hash`;
3. сравнение подписи постоянным временем;
4. свежесть `auth_date`: `OFFERS_APP_INITDATA_TTL` (по умолчанию `24h`);
   подпись из будущего больше минуты тоже отвергается;
5. `user.id` = `chat_id` → `offers_bot_link` → email; нет привязки → `403
   not_linked`.

Токен бота берётся **только** из `offers_bot_config` (правило 2026-10-29:
второй источник однажды отправлял уведомления чужим ботом). Смена токена
автоматически аннулирует выданные `initData`.

Выдаваемый JWT — тот же формат, что у плагина (`app_id=offers`,
`iss=auth-service`, `aud=[offers]`, срок 1 час), `device_id = app:<chat_id>`:
роли проверяет существующий `requirePerm`, новых гейтов нет.

### `GET /api/offers/app/config`

Публичная, без гейта: `{ appUrl, botUsername, initDataTtl }`. Данных
человека и секретов не содержит. Адрес — `OFFERS_APP_URL` (по умолчанию
`https://epyur.fvds.ru/app-offers/`).

### Правила, которые нельзя ломать

- `initData` — **ключ доступа**: только тело POST, никогда строка запроса,
  никогда в журнале (в журнал идёт только причина отказа);
- ручка, которую зовут с `X-Service-Secret`, обязана быть `/bot/*`; ручка с
  JWT — без `/bot/` и без секрета; смешение этих двух уже дало живой 403
  (0.9.17);
- пустой список в JSON — `[]`, не `null`.

## Списки в приложении и поиск (2026-11-13, шаг S2)

`GET /api/offers/offers?q=` и `GET /api/offers/combat?q=` (JWT, `viewer`).
Ответ прежний — `{offers: Offer[]}` / `{sheets: CombatSheet[]}`, не более
`listLimit` = 200 строк, свежие сверху.

| список | по чему ищется `q` | без `q` |
|---|---|---|
| КП (`/offers`) | имя заказчика (`client->>'name'`) | как раньше: 200 свежих, admin — все, остальные — свои |
| боевой листок (`/combat`) | `title` и названия продуктов обеих сторон (`our_notes` / `competitor_notes`) | как раньше: 200 свежих, видны все |

Правила поиска (`offers-service/lists.go`):

1. **Приведение запроса** — схлопывание пробелов, `ё`→`е` (в запросе и в
   значении), экранирование `%`/`_` (иначе «100%» стал бы шаблоном).
   Регистр не важен.
2. **Два индексных вида в SQL** — `LIKE 'q%'` и `LIKE '% q%'` (слово
   начинается с запроса); по ним работают индексы `offers_offer_client_name`
   и `offers_combat_sheet_title` (`text_pattern_ops`).
3. **Добор в памяти** — подстрока внутри слова («Строй» в
   «СтройМонтажСервис»): по индексу она не ходит, а строк не больше 200.
   `ILIKE '%q%'` сознательно не используется: без индекса это Seq Scan.
4. **Пустой `q`** — прежний запрос без условий (регрессия закреплена
   тестом `TestListQueryWithoutSearchKeepsBehaviour` и секцией стенда
   «5b. Поиск в списках»). Плагин, бот и ЦУП вызывают эти ручки без `q` и
   ничего менять не должны.
5. **Права не обходятся поиском**: для не-admin условие владельца остаётся
   в SQL, а чужие строки дополнительно отсекаются до загрузки модели.

Приложение (`offers-app/src/lists.ts`): `listPath(kind, q)` не добавляет `q`,
если строка пустая; подписи строк — заказчик (КП) и продукт (листок,
`displayName` сервера с запасным видом на клиенте). Поиск в экране — с
задержкой 300 мс, `q` живёт в адресе (`#/list/offers?q=…`), поэтому
результат можно передать ссылкой.

#### Порядок поиска и регистр (исправлено 2026-11-13)

1. **Сначала поиск, потом отсечение.** С поиском база просматривает
   `listScanLimit` = 2000 свежих строк, совпадения проверяются (SQL + память),
   отдаются первые `listLimit` = 200. Без поиска — прежние 200. Так совпадения
   не теряются, если они глубже свежих двухсот.
2. **Регистр приводится один раз** — в `parseListQuery` (`lowerASCIIFree`), до
   SQL. Отдельно приводить запрос в памяти нельзя: `lower()` базы зависит от
   локали, а `lowerASCIIFree` — нет.
3. SQL-условие поиска: `lower(name) LIKE 'q%'` **или** `lower(name) LIKE '% q%'`
   **или** `lower(replace(name,'ё','е')) LIKE '%q%'` (подстрока, «ё»→«е»).
   Третья форма без индекса — сознательно: без неё заказчика в середине слова
   не найти, а объём ограничен `listScanLimit`.

Проверки: стенд (`offers_local_e2e.mjs`, секция 5b) и живой
`check_list_search_live.mjs` на боевой базе.

### Карточка КП: точечные правки (2026-11-13, шаг S3)

Приложение сохраняет карточку по частям — у каждого поля своя ручка. Решение
пользователя («вариант В»): полного сохранения документа из приложения нет,
поэтому правка одного поля не затирает то, что параллельно поменяли в боте
или в плагине.

| метод | путь | что меняет | роль |
|---|---|---|---|
| `PUT` | `/api/offers/offers/{id}/client` | имя и контакт заказчика | `editor`, своё или admin |
| `PUT` | `/api/offers/offers/{id}/items` | позиции целиком (список заменяется) | `editor`, своё или admin |
| `POST`/`DELETE` | `/api/offers/offers/{id}/system` | выбранные системы (существующие) | `editor`, своё или admin |
| `POST`/`DELETE` | `/api/offers/offers/{id}/material` | выбранные материалы ОПИСАНИЕМ (2026-11-13) | `editor`, своё или admin |

Тела:

```jsonc
// PUT /client
{ "name": "ООО «Ромашка»", "contact": "Иван, +7…" }
// PUT /items
{ "items": [{ "name": "Плита", "unit": "м²", "qty": 3, "price": 999.99,
              "ekn": "…", "catalogId": 12 }] }
// POST /material, DELETE /material  (то же для /system)
{ "noteId": 103 }          // или { "sku": "MAT-КРОВ-ПВХ-0001" }
```

**Материал как объект предложения** (2026-11-13). `POST /material` выбирает
материал ОПИСАНИЕМ: в `Offer.materials` появляется `{sku, noteId}` (оба
значения пишет сервер, даже если прислали одно), ответ несёт карточку и текст
`material.description`. Позиций это не добавляет — строку с ценой заводят в
составе («Материал в позиции», таблица менеджера). `DELETE /material`
принимает любой из двух ключей: `resolveMaterialChoice` дополняет выбор и
снимает по обоим.

Правила, общие для обеих ручек:

1. Ответ — тот же `Offer`, что отдаёт `GET /offers/{id}` (клиенту не нужно
   домысливать, что теперь на сервере).
2. **Каждая запись двигает `updated_at`** — иначе правка не выиграет слияние.
3. **`sum` и итог считает сервер** (`recalcOfferTotal`); присланные значения
   не читаются вовсе, приложение может считать их только для показа.
4. Заказчик: пустые имя и контакт законны (черновик заполняют по частям),
   но контакт без имени — `400`; больше 300 символов — `400`.
5. Позиции: **пустой список законен** (в отличие от полного
   `PUT /offers/{id}`); пустое наименование, `qty <= 0`, `price < 0`,
   больше 500 строк — `400`.
6. Чужое предложение — `403` у обеих ручек (`offerForEditor`).

### Тексты разделов КП (2026-11-13, шаг S4)

JWT-двойник ботовой `POST /bot/offers/{id}/copy/{key}`. Слоты, хранилище и
логика — общие с ботом; второй копии текста не появляется.

| метод | путь | действие | роль |
|---|---|---|---|
| `GET` | `/api/offers/offers/{id}/sections` | все слоты с текстами | `viewer`, своё или admin |
| `POST` | `/api/offers/offers/{id}/sections/{kind}` | модель пишет текст | `editor`, своё или admin |
| `POST` | `/api/offers/offers/{id}/sections/{kind}/edit` | вычитать правку менеджера | `editor`, своё или admin |
| `PUT` | `/api/offers/offers/{id}/sections/{kind}` | принять / пропустить | `editor`, своё или admin |

```jsonc
// GET → { "sections": [{ "key": "copy.intro", "label": "Вводный текст",
//                         "text": "…", "warnings": [] }] }
// POST → тот же объект одного слота (текст НЕ сохранён)
// PUT  → тело { "text": "…" }; пустой текст = «пропустить» (ключ удаляется)
//        ответ — Offer после записи
```

`kind` — `intro`, `system`, `advantages`, `conditions`. Для `system` обязателен
`?systemIndex=N` (с 1, не больше числа выбранных систем) — иначе `400`.
У фиксированных слотов `systemIndex` игнорируется. `warnings` — замечания
чек-листа: они только сообщают, текст не переписывается молча. Недоступность
модели — не ошибка: приходит пустой текст.

### Боевой листок в приложении (2026-11-13, шаг S6)

JWT-двойники ботовых ручек. Логика общая с ботом, отличия — только вход и
гейт. **Читают и печатают все вошедшие, правят автор и admin.**

| метод | путь | что делает | роль |
|---|---|---|---|
| `GET` | `/api/offers/combat/our?q=&offset=&limit=` | наш продукт (материалы каталога + системы), постранично | `viewer` |
| `GET` | `/api/offers/combat/competitors-src?q=&offset=&limit=` | продукты конкурентов (тег `competitor` + поддерево папки) | `viewer` |
| `GET` | `/api/offers/combat/analogues?noteId=` | аналоги нашему продукту | `viewer` |
| `POST`/`DELETE` | `/api/offers/combat/{id}/our` | добавить / убрать наш продукт | `viewer` + автор/admin |
| `POST`/`DELETE` | `/api/offers/combat/{id}/competitor` | добавить / убрать конкурента | `viewer` + автор/admin |
| `PUT` | `/api/offers/combat/{id}/hint` | уточнение для разбора (`form_values["hint"]`) | `viewer` + автор/admin |

- Сторона добавляется и убирается **позиционно** (`{ noteId, title }`), а не
  полным сохранением листка: параллельная правка в боте не затирается.
- Страница выбора: `{ items: [{noteId,title,sku?}], total, offset, hasMore }`.
- `competitors-src` назван так потому, что `GET /competitors` — уже кеш для
  раздела «Конкуренты» плагина (другая форма и другая роль проверки).
- `analogues` отдаёт `{ source:{noteId,title}, cards:[…] }`; `404` — продукта
  нет в кеше (администратору нужно перечитать «Конкуренты»).
- Печать листка — существующая `POST /api/offers/combat/{id}/export`.

### «Отправить в чат» из приложения (2026-11-13, шаг S7)

Приложение не может приложить файл к сообщению: у него нет Bot API. Кнопка
«📤 Отправить … в чат» делает `tg.sendData(JSON)` и сразу `tg.close()`, а бот
принимает это как обычное сообщение `message.web_app_data`.

```jsonc
// нагрузка (указатель, без адресов и байтов)
{ "action": "docx", "kind": "offer" | "combat", "id": 12 }
```

Правила:

1. **`chat_id` берётся из обновления Telegram**, а не из нагрузки: указатель
   не может отправить документ в чужой чат. Пришедший в нагрузке `chat_id`
   игнорируется.
2. Файл собирается существующими ручками `/api/offers/bot/offers/{id}/docx`
   и `/api/offers/bot/combat/{id}/export` — теми же, что у кнопки «⬇ DOCX»;
   права чата проверяются там же.
3. Любая другая нагрузка — `null` в разборе и понятный ответ человеку
   («откройте документ и нажмите кнопку ещё раз»), а не тишина.
4. Кнопка показывается только внутри Telegram (`inTelegram` в `tg.ts`):
   в обычном браузере отправлять некуда.

## Каталог

`GET /catalog?q=&archived=1` → `{items: CatalogItem[]}`; `POST /catalog`,
`PUT /catalog/{id}`, `DELETE /catalog/{id}` (admin; удаление = архив).

```jsonc
CatalogItem = { id, kind: "system"|"material"|"work", name, unit,
                basePrice, description, sortOrder, archived, createdAt, updatedAt }
```

Поиск: все слова запроса должны найтись в названии или описании (без учёта
регистра). Пустой запрос — весь каталог.

## Форма КП

`GET /form?mode=client` → `OfferForm`; `PUT /form` (admin).
Если форма не настроена — отдаётся форма по умолчанию: титульный лист,
поставщик/получатель/объект, свободный блок о системе и ссылки (2026-11-01).
Сохранённая форма не подменяется: «нет строки в БД» и «сохранена пустая» — разное.

```jsonc
OfferForm = { id, mode, title, placeholders: [{id, label}],
              blocks: [Block], requiredFields: string[], updatedAt }

Block = { id, title, content: RichNode[], showInClient, showInInternal,
          // размещение на странице, пункты (0..200):
          spacingBefore?, spacingAfter?, keepTogether?, pageBreakBefore? }

RichNode =
  | { type: "paragraph"|"heading"|"bullet_list"|"table",
      level?: 2|3|4, align?: "center"|"right"|"justify"|"left",
      text?: string, items?: string[], rows?: string[][] }
  | { type: "page_break" }                       // разрыв страницы в этом месте
  | { type: "links", level?: 2|3|4 }             // ссылки из карточек, в конце КП
  | { type: "prompt_text", instruction: string,  // текст пишет модель
      hint?: string, maxWords?: number, align? }
  | { type: "image", imageKey: string, imageName?: string,  // с обтеканием
      widthCm?: number, wrap?: "both"|"left"|"right"|"topAndBottom"|"none",
      caption?: string, align? }
```

Плейсхолдер в тексте — `{{id}}`; сервер подставляет значения из
`formValues` КП, неизвестные вырезает.

**Размещение.** Абсолютного позиционирования нет: `spacingBefore`/`spacingAfter`
(пункты), `keepTogether` (не разрывать абзацы блока между страницами) и
`pageBreakBefore` (начинать с новой страницы). Флаги относятся к первому
напечатанному абзацу блока. Разрыв страницы в начале документа игнорируется —
Word на нём даёт пустую страницу.

**Изображения.** Узел `image` печатается плавающим объектом (`wp:anchor` с
`wrapSquare`), а не `wp:inline`: текст идёт рядом с картинкой, строка не
разрывается. Файл берётся из вложений Базы знаний по `imageKey` (ключ
возвращает `POST /api/kb/attachments`). Картинка необязательна: нет файла —
блока нет, документ собирается.

**Текст по промту.** Модель пишет один абзац по `instruction`/`hint`, опираясь
на данные КП (объект, адресат, площадь, системы из карточек, состав, суммы).
Результат кэшируется: `formValues["gen.section"]` — хеш данных предложения,
`formValues["gen.<blockId>.<узел>"]` — текст. Хеш не совпал (правили состав,
цены, объект) — текст пишется заново. Модель недоступна — блок молчит.
Добавление системы стирает хеш (`dropStaleBlockTexts`).

**Ссылки.** Раздел `## Ссылки` карточки (системы и материала) разбирается в
`refs: [{label, url, kind: "page"|"doc"}]`. `page` — страница на корпоративном
сайте, `doc` — техническая документация на навигаторе (PDF, техлист, узлы).
Узел `links` печатает их в конце документа; в Word они кликабельные (внешние
отношения `TargetMode="External"`). Пустой раздел не печатается. Предел — 60
ссылок, остальное называется словами, а не обрезается молча.

**Links in the document (2026-11-10, printed by groups, numbered).** Each
`CardRef` now also carries `group` and `owner`. `group` is one of `page`
(product page on nav.tn.ru), `sto`, `techlist`, `uzly`, `cert` (certificates,
declarations, instructions and anything else). `owner` is `system` or `material`
(the card that produced the link, including a material reached through a
system's composition). `SortCardRefsGrouped` orders them for printing: `page`
→ `sto` → `techlist` → `uzly` → `cert`; inside a group systems come before
materials — except `cert`, where materials come first. `GroupCardRefsByGroup`
splits the sorted list into `CardRefGroup{group, title, refs}`. Both the `links`
node and the `{{ссылки}}` layout marker print one numbered list per group, each
with its own `numId` (numbering restarts at 1). The visible item text is the
label; the URL lives only in the relationship. Old caches without `group`/`owner`
are classified on the fly (`effectiveGroup`/`effectiveOwner`) and print
correctly, but `LinkedRefs` (material links) only appear after re-reading the
Knowledge Base.

## Предложения

`GET /offers` — видимость: admin — все, остальные — свои.
`GET /offers/{id}`, `POST /offers`, `PUT /offers/{id}`, `DELETE /offers/{id}`
(editor; правка — только своё), `POST /offers/{id}/export?format=docx`
(бинарный DOCX), `POST /offers/{id}/link` → `{url, expiresAt}` (~48 ч).

```jsonc
Offer = { id, number: "007/2026", ownerEmail,
          client: {name, contact},
          items: [{catalogId?, name, unit, qty, price, sum}],
          formValues: {поле: значение},
          systems: [{sku?, noteId?}],
          materials: [{sku?, noteId?}],
          total, status: "draft"|"final", docxKey, createdAt, updatedAt }
```

- `items` — **снимок**: цена фиксируется при добавлении карточки, правка каталога
  старые предложения не меняет.
- `systems` — выбранные системы в **порядке выбора** (2026-11-05). Пустой
  массив — ни одна не выбрана. Позиций выбор не добавляет: печатаются описание,
  разрез, состав и характеристики из карточки. На чтении сервер понимает и
  старый вид (`system_choice` хранил объект `{}` или `{"sku":…}`) — он
  превращается в пустой массив или массив из одного элемента. На запись всегда
  идёт массив; `null` не отдаётся.
- `materials` — материалы, предложенные **описанием** (2026-11-13). Форма та же,
  что у `systems`; пустой массив — ни один не выбран. Полное сохранение
  предложения (`PUT /offers/{id}`) пустым значением их НЕ стирает — как и
  системы. Позиций они не добавляют: печатаются текст `material.description`
  и ссылки карточки.
- `sum` и `total` пересчитывает сервер: каждая строка округляется до копеек,
  затем итог (3 × 999.99 → 2999.97, без двоичных хвостов).
- Номер — `{NNN}/{yyyy}`, сквозной по году (advisory-lock по году).
- **Пустое предложение (без позиций)** — 400 «в предложении нет позиций» на
  REST-пути `POST /offers`: там клиент присылает готовое КП. На **ботовом** пути
  (`POST/PATCH /bot/offers`) пустой состав законен (2026-11-01): КП создаётся в
  начале разговора, а позиции менеджер может добавить позже.

## DOCX

- Бланк — корпоративный `template/standard.docx` (шапка с логотипом, футер с
  реквизитами), тело заменяется: блоки с `showInClient`, таблица позиций, итог,
  подпись менеджера.
- **Шапку документа печатает первый блок формы** (2026-11-02), обычно титульный
  лист: «Коммерческое предложение № {{offer_number}} от {{offer_date}}»,
  поставщик, «Кому: {{client_name}}». Сборщик документа свою шапку добавляет ТОЛЬКО
  при форме без блоков — иначе название, номер и клиент печатались дважды, а
  служебные заготовки редактора уезжали в готовый файл.
- **Служебные поля** (подставляются при печати, в форме не хранятся):
  `{{offer_number}}`, `{{offer_date}}` (`дд.мм.гггг`), `{{manager}}` (e-mail
  владельца), `{{offer_client}}`, `{{offer_object}}`. Значение из
  `formValues` с тем же ключом главнее вычисленного.
- **Колонка «Код ЕКН»** в таблице состава (2026-11-02). Нет ни одного номера —
  колонки нет вовсе (пустая колонка читается как сбой, а не как «номера нет»).
- Подпись «Менеджер: …» не печатается, если строку с `{{manager}}` уже дала
  форма — иначе в документе две подписи.
- **Документ собирается и без позиций** (2026-11-01): менеджеры на первых шагах
  берут из системы только описательную часть (объект, систему, преимущества,
  условия), а цены вписывают сами. Вместо таблицы в этом случае печатается
  строка «Состав и стоимость материалов передаются отдельным документом»;
  нулевого итога («Итого: 0,00») в документе нет — он читается как «бесплатно».
- Оформление текста — Times New Roman 14pt, полуторный интервал, отступ 1,25 см,
  выравнивание по ширине (тот же `styles.xml`, что у `agent-service`).
- Суммы в документе — русский формат «1 234,56».
- **Позиция КП хранит ЕКН** (`items[].ekn`, снимок на момент добавления):
  номер из таблицы менеджера, иначе первый по справочнику материала. При
  сверке таблицы «ЕКН главнее названия» — сначала номер, потом наименование
  (`specRowView.matchedBy`: `ekn` | `name`).
- Весь текст экранируется: `&`, `<`, `>` и кавычки из названия клиента не
  ломают документ.
- **PDF не делается** — решено на этапе дизайна.

## Доступ к боту

`GET /access/list` (admin) → `{entries: [{email, role, createdAt, updatedAt}]}`;
`POST /access/role` (admin, `{email, role}`); `POST /access/codes` (admin,
`{email}`) → `{code, expiresAt}` — код действует 7 дней.

## Токен Telegram-бота (2026-10-29)

**Один хозяин токена — `offers-service`, таблица `offers_bot_config`.** Переменной
окружения `TELEGRAM_BOT_TOKEN` у бота нет и заводить её нельзя: у соседнего
`portal-embed` второй источник (`.env` рядом с базой) полтора суток отправлял
уведомления чужим ботом (`web/portal-embed/AGENTS.md`, запись 2026-10-29).

Плагин (раздел «Бот и доступ», только admin):

| Ручка | Тело | Ответ |
|---|---|---|
| `GET /bot/config` | — | `{configured, botUsername, updatedBy, updatedAt}` — сам токен наружу не отдаётся никогда |
| `POST /bot/config` | `{token}` | Проверяет токен `getMe` через бота и только потом пишет; 400 — токен пуст, не по форме (`123456789:AA…`) или отвергнут Telegram; 503 — бот недоступен |
| `DELETE /bot/config` | — | `{configured: false}` |
| `GET /bot/status` | — | `{configured, running, botUsername, lastError, checkedAt}`; `running` — отметка бота не старше минуты |

Бот (`X-Service-Secret`):

| Ручка | Тело | Ответ |
|---|---|---|
| `GET /bot/token` | — | `{token, username}`; пустая строка — «ещё не настроен» |
| `POST /bot/heartbeat` | `{last_error}` | `{ok}` — бот отмечается раз в итерацию поллинга |

Проверку токена делает **бот**, а не сервис: с VDS путь в Telegram есть только
через SOCKS-сайдкар. Ручка бота — `POST /internal/verify` (`X-Service-Secret`,
только внутренняя docker-сеть). Бот кэширует токен на 15 секунд: смена в
плагине действует почти сразу и без перезапуска контейнеров. При отказе
`resolveBotToken()` бросает ошибку — отката на окружение нет намеренно.

## Ручки бота (X-Service-Secret)

| Ручка | Тело/параметры | Ответ |
|---|---|---|
| `POST /bot/request-code` | `{email, chat_id}` | `{status: "code_sent"}`; код уходит письмом, в ответе его нет |
| `POST /bot/verify-code` | `{chat_id, email, code}` | `{email, role}` |
| `GET /bot/identity?chat_id=` | — | `{email, role}` / 404 |
| `GET /bot/catalog?q=` | — | `{items}` |
| `GET /bot/offers?chat_id=` | — | `{offers}` (свои, до 50) |
| `GET /bot/offers/{id}?chat_id=` | — | `Offer` / 403 чужое / 404 |
| `POST /bot/offers` | `{chat_id, client, items, formValues}` | `Offer`; **`items: []` разрешено** — так открывается разговор |
| `PATCH /bot/offers/{id}` | `{chat_id, items?, client?, formValues?}` | `Offer` |
| `POST /bot/offers/{id}/docx?chat_id=` | — | `{docx_base64, file_name}` |
| `POST /bot/offers/{id}/link?chat_id=` | — | `{url, expiresAt}` |

### Коды входа

- `kind='signal'` — самозапрос из бота, TTL 15 минут, письмо уходит через
  `sendEmail` (локальный exim).
- `kind='admin'` — выдан администратором в плагине, TTL 7 дней, письмом не
  отправляется (админ передаёт код сам).
- Проверка: `used_at` — «код уже использован»; истёк — «код истёк»;
  больше 5 попыток — «слишком много попыток»; иначе сверка кода.
- После успешной проверки: `offers_bot_link` (chat_id → email) и запись в
  `offers_access` с ролью `viewer`, если её ещё нет.

## Сценарий бота

Меню: «🧾 Собрать КП», «📂 Мои КП», «❓ Справка».

**Сборка — в две фазы (2026-11-01).** Раньше «Собрать КП» сразу открывала
каталог, и менеджер выбирал позиции, не зная ни объекта, ни адресата.

1. **Контекст.** Сразу создаётся КП с номером и **без позиций**
   (`POST /bot/offers` с `items: []`; на REST-пути пустое КП по-прежнему 400 —
   там клиент присылает готовое). Строка нужна, чтобы принять файл и бриф.
   Затем идёт бриф по этапам (`skills.BriefTopics`, поле `Stage`):
   - «Объект и адресат»: `object_name`, `object_purpose`, `object_area`,
     `customer_address`;
   - «Сделка»: `customer_goals`, `customer_pains`, `project_risks`;
   - «Что предлагаем»: `planned_systems`;
   - «Условия»: `objections`, `budget_signal`, `decision_maker`, `timeline`,
     `why_us`, `notes`.

   На каждом вопросе — кнопки «📎 Прислать файл» и «⏭ Пропустить вопрос».
   Файл в любой момент идёт тем же двухшаговым путём `attach` → `apply`.
   «✅ Готово» до этапа позиций = «перейти к позициям».

2. **Позиции.** Каталог (кнопки, поиск текстом, листание) → количество →
   «✏️ Добавить позицию» (строка `Название | количество | цена`) / «🗑 Убрать
   последнюю» → «✅ Готово» → согласование текстов (секции `copy.*`) → DOCX в
   чат + кнопки «🔗 Ссылкой», «🧾 Новое КП».

Имя клиента второй раз не спрашивается, если адресат уже есть в брифе
(`formValues.client_name`): бот читает своё КП (`GET /bot/offers/{id}`).
Ролей в боте нет: кто вошёл по коду — тот и собирает.

## Схема БД (`offers`)

```sql
offers_material      (id, sku UNIQUE, kind material|work, name, unit, description,
                      price_min, price_max, price_seen_at, photo_key, sort_order,
                      archived, created_at, updated_at)          -- 2026-10-30, БЕЗ цены
offers_material_ekn  (id, material_id FK, ekn, variant, source excel|manual,
                      first_seen_at, last_seen_at)              -- UNIQUE (material_id, ekn)
offers_bot_config    (single row: token, bot_username, updated_by,
                      bot_last_seen_at, bot_last_error)         -- 2026-10-29
offers_catalog  (id, kind, name, unit, base_price, description, composition jsonb,
                 photo_key, sort_order, archived, created_at, updated_at)
                 -- прежняя таблица: материалы оттуда перенесены миграцией,
                 -- system-строки заархивированы; нужна старым КП
offers_form     (id, mode UNIQUE, title, placeholders jsonb, blocks jsonb,
                 required_fields jsonb, updated_at)
offers_offer    (id, number UNIQUE, owner_email, client jsonb, items jsonb,
                 form_values jsonb, total, status, docx_key, created_at, updated_at)
bot_codes       (id, email, code, kind, chat_id, expires_at, used_at, attempts, created_at)
offers_bot_link (chat_id PK, email, confirmed_at)
offers_access   (email PK, role, created_at, updated_at)
```

## S3

Бакет `sbe-offers` (rclone CLI, remote `firstvds_offers` из env `S3_*`).
Ключи — `offers/{номер}/{случайный}/.docx`. Ссылки — `rclone link --expire 48h`.
Не использовать `mailers-backup` (там ротация 7 дней).

## Локальная проверка (2026-10-27)

Стенд без сервера: docker-compose.local.yml — Postgres 16 + offers-service +
заглушка auth-service (scripts/auth-stub.mjs, роли задаются
AUTH_STUB_ROLES="owner@tn.ru=superadmin,manager@tn.ru=editor").

`
docker compose -f docker-compose.local.yml up -d --build
node scripts/offers_local_e2e.mjs http://127.0.0.1:18080
docker compose -f docker-compose.local.yml down -v
`

Прогон покрывает 59 проверок по HTTP: авторизация (401/403/просрочка/чужой
app_id), каталог (создание, правка, архив, поиск, валидация, 404 на
несуществующую карточку), форма (сохранение, валидация, форма по умолчанию
только когда строки в БД нет), предложения (расчёт до копеек, нумерация,
видимость по ролям, права на чужое), DOCX (zip, шапка/футер бланка, плейсхолдеры),
ручки /bot/* под секретом, коды входа, идемпотентность.

**S3 необязателен.** Без S3_ENDPOINT сервис стартует и отдаёт DOCX байтами;
ручки ссылок отвечают 503 «хранилище не настроено».
## Бриф КП (2026-10-29)

Бот ведёт с менеджером разговор о объекте и задачах заказчика, потом пишет
текст секций предложения. Модель — общий `llm-service`
(`POST /internal/llm/complete`, `X-Service-Secret`): своего ключа у offers нет.

### Продолжение сохранённого КП (2026-11-01)

Раньше к КП можно было вернуться только из плагина: бот жил одним черновиком в
памяти, и после «Готово» без позиций продолжить разговор было нечем. Теперь
черновик собирается из самого предложения (`flow/resume.ts`
`draftFromOffer`/`resumeSession`):

- «📂 Мои КП» даёт на каждое КП две кнопки — `edit:<id>` («✏️ № …») и
  `doc:<id>` («📄 файл»);
- номер КП, набранный текстом (`003/2026`), тоже открывает его на правку;
- панель правки (`EDIT_KEYBOARD`): добавить из каталога, свою позицию,
  «📚 Дополнить контекст» (`edit:brief` — возврат к брифу с первой незаполненной
  темы), «📄 Документ» (`edit:doc`), «🧾 Готово» (`done` → согласование текстов
  и сборка файла);
- этап ставится сразу `positions`: контекст по этому КП уже собрали, заново его
  спрашивать — повторять пройденный разговор;
- прежний разбор файла из сессии забывается: он относился к другому разговору.

### Бриф

`offers_brief` (одна строка на предложение) + `offers_brief_turn` (журнал
реплик). Поля брифа: `object_name`, `object_purpose`, `object_area`,
`customer_address`, `planned_systems[]`, `customer_pains[]`,
`customer_goals[]`, `project_risks[]`, `objections[]` (возражение + ответ),
`budget_signal`, `decision_maker`, `timeline`, `why_us[]`, `notes`,
`skipped_topics[]` — всего 14 полей.

Темы задаются манифестом (`skills.BriefTopics`), порядок стабилен. Тема
закрыта, когда поле заполнено ИЛИ она в `skipped_topics` («не знаю»).
Этап (`skills.StageOf`/`CurrentStage`) закрыт, когда закрыты все его темы:
бот называет этап, чтобы человек понимал, что до позиций ещё далеко.

Бриф наполняется и файлом: `attach` отдаёт `contextFields` (объект, назначение,
площадь, системы, заметки), а `attach/apply` в режиме `brief` сливает этот
разбор со строгим (`skills.ExtractBriefFacts` + модель, `MergeFileContexts`).

### Ручки брифа (бот, `X-Service-Secret`)

| Ручка | Тело | Ответ |
|---|---|---|
| `POST /bot/offers/{id}/brief/start?chat_id=` | - | `{brief, reply, done, owner}` |
| `POST /bot/offers/{id}/brief/step?chat_id=` | `{chat_id, text}` | `{brief, reply, done, nextTopic, retry, unavailable, skipped}` |

Ответ модели — JSON `{payload, reply, skip}`. Разбор терпимый
(`jsonrepair.go`): fenced-блок, текст вокруг, обрезанный ответ достраивается.
`reply` необязателен — реплику собираем по манифесту. Модель недоступна —
`unavailable: true`, бриф не меняется.

### Текст секций

| Ручка | Тело | Ответ |
|---|---|---|
| `POST /bot/offers/{id}/copy/{key}` | `{chat_id, action, text?, systemIndex?}` | `{key, text, warnings, label}` / `{accepted}` |

`key` = `intro` \| `system` \| `advantages` \| `conditions`;
`action` = `write` \| `edit` \| `accept`.

Согласованный текст живёт ТОЛЬКО в `form_values["copy.<ключ>"]`:
`copy.intro`, `copy.system.<N>`, `copy.advantages`, `copy.conditions`.
Пустая секция = «пропустить» (ключ удаляется).

Деградация: модель недоступна → `write` отдаёт пустой текст, `edit` —
текст менеджера. КП собирается с текстом формы.

### Чек-листы текста

`skills.CheckAiTells`, `CheckSectionStructure`, `CheckClaimNumbers` —
только СООБЩАЮТ о проблемах, текст не переписывают. Числа в тексте
разрешены лишь те, что есть в предложении.

### Бриф в плагине (2026-10-30)

| Ручка | Права | Тело | Ответ |
|---|---|---|---|
| `GET /offers/{id}/brief` | viewer + видимость КП | — | `{brief, turns}` |
| `PUT /offers/{id}/brief` | владелец КП или admin | `{payload, status?}` | `{brief, turns}` |

Брифа нет — `GET` отдаёт `{brief: {status: "absent", payload: <все ключи>},
turns: []}`: для плагина это обычное состояние (КП собрано без разговора в боте),
а не ошибка. `PUT` заменяет `payload` целиком и двигает `updated_at`.

Роль `editor` сама по себе чужой бриф НЕ правит: бриф — разговор владельца
предложения, поэтому `PUT` требует владельца или администратора (403 иначе).

Поля `payload` повторяют `model.BriefPayload` один в один: `object_name`,
`customer_pains[]`, `customer_goals[]`, `project_risks[]`, `objections[]`
(`{claim, response}`), `budget_signal`, `decision_maker`, `timeline`,
`why_us[]`, `notes`, `skipped_topics[]`. Возражение считается отработанным
только с ответом.

### Что попадает в документ (2026-10-30)

Два ключа `form_values` управляют необязательным содержимым DOCX. Оба
**выключены по умолчанию** — существующие предложения печатаются как раньше:

| Ключ | Что печатает |
|---|---|
| `doc.section_copy` | согласованные тексты секций (`copy.*`) |
| `doc.brief_summary` | сводку брифа из ключей `brief.*` (только непустые) |

Включаются отметками при сохранении предложения в плагине. Флаг «включён» —
значение `1` (сервер понимает также `true`/`yes`/`да`).

## Контекст КП: карточки систем из Базы знаний (2026-10-29)

Источник текста системы — **заметка в Базе знаний** с тегом
`commercial_propose`. Цены, артикулы и количества — в каталоге.
Стандарт карточки: `docs/sbe-offers-system-card-standard.md`.

Чтение через `kb-service`: `GET /api/kb/tags`, `/api/kb/notes?tag_id=`,
`/api/kb/notes/{id}`, `/api/kb/notes/{id}/attachments`, `/api/kb/file-link`.
Авторизация — межсервисный JWT от `auth-service`
(`/internal/service-token`, `target_app_id=kb`), кеш токена 50 минут.

| Ручка | Права | Ответ |
|---|---|---|
| `GET /context/cards` | viewer | `{cards: SystemCard[], kbEnabled}` — из кеша |
| `POST /context/refresh` | admin | `{cards, count, kbEnabled, error?}` — перечитать базу знаний |

**`kbEnabled: false` значит только одно: сервису не задан `KB_SERVICE_URL`.**
Это не ошибка конфигурации Базы знаний на диске и не про права — контейнер
просто не знает, куда идти. Та же пара нужна и для секрета:
`KB_SERVICE_URL` + `KB_SERVICE_SECRET` (межсервисный JWT берётся у
`auth-service` через `/internal/service-token`). Без секрета карточки не
прочитаются даже при верном URL.

`GET` отдаёт **кеш**, а не живой поход в базу знаний: на свежей базе он пуст,
и это нормально — наполняется `POST /context/refresh`.

`offers_context` работает как **кеш** (mode = `system_cards`). Ошибка базы
знаний не стирает кеш: возвращаются прежние карточки и `error`.

```jsonc
SystemCard = { noteId, path, sku, title, description, composition: string[],
               specifications?: string[][], imageName?, catalogId,
               updatedAt, hasDescription, hasComposition, empty,
               refs?: [{label, url, kind: "page"|"doc"}] }   // 2026-11-01
```

`catalogId = 0` — карточка не связана с каталогом (нет `system_sku` или кода
нет в каталоге). Связь только по коду, не по названию.

## DOCX: правила бланка (2026-10-29)

- `word/styles.xml` и `word/numbering.xml` берутся из шаблона **как есть**.
  Подмена своими ломала вёрстку: пропадала нумерация страниц в нижнем
  колонтитуле (поля `PAGE`/`SECTIONPAGES` опираются на стили бланка).
- Разметка использует ТОЛЬКО стили бланка: `Normal` = `a0`,
  `heading 1` = `1`, маркированный список = `numId 6`.
- id изображений (`wp:docPr`) вычисляются по всем частям пакета
  (document/header/footer/сноски): у бланка заняты 1–25, дубль ломает
  вёрстку молча. Media пишутся как `offerimgN.*` — `image1/image2` заняты
  логотипом и знаком подписи.
- Согласованный текст секций идёт после блоков формы, перед составом.
  Преимущества рисуются списком бланка.
- Описание системы из карточки Базы знаний печатается, только если менеджер не
  согласовал свой текст для этой системы. Фото продукта вставляется тем же
  сборщиком, что и остальные картинки (`offerimgN.*`, id по занятым в бланке).

## DOCX: текст, оформление, картинки и макет (2026-11-05)

**Один конвейер текста на все места** (`docx_runs.go`): абзац, заголовок, пункт
списка и ячейка таблицы проходят `plainText` + `runsXML`.

| что | как печатается |
|---|---|
| `[[Материал\|подпись]]` из заметки | подпись без скобок |
| `**жирный**`, `[текст](адрес)`, `` `код` `` | без разметки |
| «подземныхсооружений» | «подземных сооружений» (пробел возвращается) |
| `г/м2`, `м2`, `см3`, `10-3` | надстрочная цифра (`w:vertAlign superscript`) |
| `V-SL 2 мм`, `AL2` | не меняется |

**Оформление** задаётся `pPr` каждого абзаца, стиль бланка не правится:

| элемент | правило |
|---|---|
| основной текст | по ширине (`both`), первая строка 709 twips (1,25 см) |
| интервал | 1,5 (`w:line="360"`) |
| заголовки | по центру, без отступа первой строки |
| списки, ячейки, «Итого», подпись | без отступа первой строки |
| таблица | 10 пт, шапка по центру с заливкой `DCE6F1` |

**Ответ модели**: бюджет `maxWords*10 + 600` токенов; если ответ всё же
оборван, `trimTruncatedTail` режет по последней законченной фразе.

**Картинки систем и материалов.** Узел `image`:

```json
{"type":"image","imageOf":"system","systemIndex":1,"widthCm":8,"wrap":"right","caption":"…"}
{"type":"image","imageOf":"material","itemIndex":3,"widthCm":8,"wrap":"left"}
{"type":"image","imageKey":"att/…","imageName":"фото.png"}
```

`imageOf` пуст → статичный ключ. Ключи берутся из карточек: `attachImageKeys`
(системы, по `![[файл]]` и вложениям) и `photo_key` материала (заполняется
`syncMaterialPhotos` при синхронизации справочника). Нет картинки — узел
молчит.

**Макет `.docx`** (`layout_docx` в `offers_form`):

| ручка | что делает |
|---|---|
| `POST /api/offers/form/layout?name=` | байты `.docx` → сохранить макет; ответ `{hasLayout, layoutName, layoutFields, layoutPrompts, bytes}` |
| `GET /api/offers/form/layout` | скачать загруженный макет |
| `DELETE /api/offers/form/layout` | убрать макет (печать снова по блокам) |
| `POST /api/offers/form/describe` | `{description, placeholders}` → `{title, nodes, notes}` |

Метки в макете: `[Наименование объекта]` или `{{object_name}}` — значение;
`{Опиши систему}`, `{{Опиши систему}}`, `{{{Опиши систему}}}` — текст модели
(одна, две или три пары фигурных скобок, в том числе со скобками внутри:
`{{модуль {параметр}}}`). Вид метки решает содержимое, а не число скобок:
имя, известное как поле (в `values`, `fieldAliases` или `builtinFieldAliases`),
подставляется значением; всё прочее уходит в модель промтом. Пределы длины —
в рунах: поле до 200, промт до 3000 (превышение обрезается, а не оставляется
литералом). Понимаемые имена полей — `fieldAliases` и `builtinFieldAliases` в
`layout_tokens.go` (номер, дата, менеджер, объект, получатель, поставщик,
название системы, выбранные системы и поля формы по умолчанию). Метки
распознаются в склейке текста ВСЕГО тела документа, даже когда Word разрезал
текст на несколько `<w:t>` и даже когда метка пересекает границы АБЗАЦЕВ:
живой макет держит инструкцию в девяти абзацах (`{{` в первом, `}}` в
последнем). Абзацы склеиваются через перенос строки, а `normalizePromptText`
сохраняет пункты списка отдельными строками; в колонтитулах метки не
заменяются. При загруженном макете печать идёт по нему, блоки формы в
документ не попадают.

**Таблицы в макете моделью не пишутся** (2026-11-06). Метка, в тексте которой
есть «таблиц»/«table», получает готовые строки:

| инструкция метки | источник строк | функция |
|---|---|---|
| «таблица предложения по материалам, ЕКН, цена, объём, стоимость» | позиции `Offer.items` | `layoutOfferTable` |
| «сравнение вариантов решений», «сравни системы таблицей» | характеристики карточек (`Specifications`) | `layoutCompareTable` → `compareSystemRows` |

Вид определяет `layoutTableKindOf`; «сравнение» проверяется ПЕРВЫМ, потому что
промт сравнения часто упоминает материалы. Таблица предложения печатает
колонки «№, Наименование материала, [ЕКН], Единица поставки, Объём поставки,
Цена за единицу, руб, Стоимость, руб» плюс строку «Итого»; колонка ЕКН
опускается, если ни у одной позиции номера нет. Сравнение при одной системе
молчит (таблица из одного столбца обманывает читателя) и метка уходит в
модель как обычный промт. Ответ печатается настоящим `<w:tbl>`; ячейка с
переводом строки или `|` сводится в одну строку (`tableCell`), иначе таблица
рвётся на куски.

**Объём ответа берётся из подсказки** (`skills.RequestedWords`): «не менее
4000 знаков», «не меньше 500 слов», «4000 символов»; знаки делятся на 6,
предел — 800 слов. Без указания объёма действует 150 слов.

**Ответ модели перед печатью чистится** (2026-11-06, `cleanBlockText`):
снимается Markdown-разметка (`#`, `**`, `- `, строка из одних знаков
выбрасывается), отсеиваются строки-рассуждения и строки, где английских слов
речи больше одного (марки и артикулы не считаются), снимается недописанный
хвост. **Многострочный ответ печатается отдельными абзацами** (`<w:p>` на
строку) с копированием `<w:pPr>` и `<w:rPr>` метки: выравнивание, красная
строка и шрифт макета действуют на весь ответ, а не на первую строку.

**План и черновик в документ не попадают** (2026-11-06, `looksLikePlan`).
Служебные маркеры вёрстки — `h1`…`h6`, `Heading N:`, `Section:`, `Intro:`, `Paragraph`,
`Ссылки (Heading 1)`, «Заголовок 2:» — и короткие вопросы модели к себе
(`ЕКН — артикул?`) означают, что ответ — оглавление, а не текст: блок молчит.
**Нумерованный список сам по себе планом НЕ считается**: состав системы в КП —
это честный нумерованный список слоёв.

**Таймаут запроса к модели — 180 с** (`llmDefaultTimeoutMS` в `llmclient.go`).
При 45 с ответ на 4000 знаков не успевал (замер: 58 с), и в документ шло то,
что модель сказала ДО ответа — её рассуждение. Провайдер держит запрос до 190 с.

## Файлы из чата: .md и .xlsx (2026-10-30)

Менеджер присылает в бота контекст объекта или таблицу спецификации файлом.
**Приём двухшаговый:** `attach` разбирает и показывает «что понял», `apply`
пишет только подтверждённое. Файл от заказчика — черновик (старые цены,
опечатки), молчаливое применение уронило бы КП в глазах заказчика.

| Ручка | Тело | Ответ |
|---|---|---|
| `POST /bot/offers/{id}/attach?chat_id=&file_name=` | байты файла (`application/octet-stream`) | `{kind, name, key?, reply, needsConfirm, brief?, spec?, context?, contextFields?}` |
| `POST /bot/offers/{id}/attach/apply` | `{chat_id, mode: brief\|spec, brief?, spec?, context?}` | `{reply, applied}` |

Обе — под `X-Service-Secret`, проверка «это ваше КП» как у DOCX. Предел
размера — 10 МБ (414/413 на превышение).

**Тип по сигнатуре, не по имени:** zip с `xl/workbook.xml` → xlsx; zip с
`word/document.xml` → docx (пока отказ: «пришлите .md или .xlsx»); OLE-заголовок
→ старый `.xls` (отказ: «пересохраните как .xlsx»); `%PDF` → pdf (отказ);
остальное без управляющих байтов → markdown.

**Разбор `.md`** (`skills.ParseMarkdownAttachment`): YAML-шапка отбрасывается;
строка «Ключ: значение» — поле брифа (подписи: объект, задача, боли, риски,
возражения, бюджет, сроки, почему мы, прочее, заказчик); `Боли:` с пустым
значением открывает список под собой; пункты списка относятся к последнему полю;
незнакомое поле остаётся в `notes` вместе с подписью. Заказчик — не поле брифа,
дописывается в `notes`.

**Разбор `.xlsx`** (`skills.ParseXlsxAttachment`): `xl/sharedStrings.xml` +
`xl/worksheets/sheetN.xml`, оба вида ячеек (словарные и числовые). Предел —
200 строк. `сумма ≠ количество × цена` попадает в `warnings`, а не исправляется
молча. Без колонок состава лист отдаётся как контекст (`context: string[]`).

С 2026-11-01 разбор устроен так, чтобы понимать **живой пользовательский КП**
(старый документ в Excel), а не только аккуратную таблицу:

- **колонка опознаётся по своей подписи, а не по порядку.** Между количеством и
  ценой люди ставят «Коэффициент расхода»; разбор по порядку делал ценой
  коэффициент (1,15 вместо 769,56). «Сметная стоимость» — это сумма, «Стоимость
  единицы» — цена, «ЕКН» — номер, а не наименование;
- **шапок на листе может быть несколько** (две системы в одном КП) — состав
  собирается по всем, а строка следующей шапки разбором не съедается;
- **строки-разделы** («Устройство гидроизоляции фундамента (горизонталь)») и
  строки номеров граф («2 | 3 | 4 | 5 | 6 | 7») в состав не попадают;
- **адрес ячейки (`A1`, `B1`) читается из атрибута `r`**, а не по порядку:
  пропущенная ячейка иначе сдвигает весь ряд.

**Контекст объекта из файла** (`skills.ExtractBriefFacts` + модель,
`contextFields`) — вторая половина разбора (2026-11-01). Живой случай: менеджер
прислал старый КП и написал «информация в файле»; бот не нашёл состава, ответил
«состава нет», а объект из файла так и не попал в бриф. Теперь:

- текст файла всегда идёт в контекст брифа, а таблица вдобавок может дать состав;
- дословный разбор берёт «для объекта: …»/«Объект: …», площадь, названия систем
  без модели; модель (`skills.RenderFileContextSystem`) добавляет тип объекта,
  риски и описание, а дословное главнее модельного (`MergeFileContexts`);
- **из таблицы состава контекст не строится**: строки позиций — наименования
  материалов, а не описание объекта;
- в режиме `spec` контекст того же файла тоже записывается в бриф — одно
  подтверждение, два результата.

**Сверка с каталогом** идёт по наименованию (`spec.matched` / `new` /
`priceDiffs`). **Цены в КП берутся из каталога** для позиций, которые в нём
нашлись: цена файла используется только для новых позиций. Применение состава
двигает `updated_at`.

**Применение брифа** — слияние, а не замена: пустое не затирает известное,
списки объединяются без повторов, возражение без ответа в бриф не попадает.

## Материалы и системы: две сущности (2026-10-30 … 2026-10-31, версия 0.3.0)

Решение пользователя: **система** — сочетание материалов, выбранное ради
результата, своей цены у неё нет, она обосновывает, почему выбраны эти
материалы. **Материал** — то, что продаётся: своё название без `ТН-`, свой код
(SKU) и несколько исполнений, чей номер — **ЕКН** (один SKU, много ЕКН).
Цены в справочнике не живут вовсе: цена приходит из таблицы менеджера для
конкретного предложения.

Дизайн — `docs/superpowers/specs/2026-10-30-sbe-offers-materials-systems-design.md`,
план — `docs/superpowers/plans/2026-10-30-sbe-offers-materials-systems-plan.md`.

### Материалы

Подпись системы в Базе знаний: тег `commercial_propose` + `material` —
материал; только `commercial_propose` — система. Тег, а не название: признак
«есть ТН-» ненадёжен.

`offers_material`: **колонки цены нет**, только `price_min`/`price_max` —
вилка из загруженных таблиц для аналитики. SKU назначает **только сервер**:
`MAT-{ТИП}-{СЕРИЯ}-{NNNN}` (у работы — `РАБ-…`), тип до четырёх знаков, серия
до трёх, пустые — `ПРОЧ`/`ОБЩ`. Код выдаётся один раз и при переименовании не
меняется; удаление уводит в архив, номера не переиспользуются.

Ручки (JWT, `viewer` — чтение, `admin` — запись):

| Метод и путь | Что делает |
|---|---|
| `GET /api/offers/materials[?archived=1]` | список с ЕКН; `kind=system` не бывает |
| `POST /api/offers/materials` | создать; `typeCode`/`series` — подсказки для SKU |
| `PUT /api/offers/materials/{id}` | правка; SKU не меняется |
| `DELETE /api/offers/materials/{id}` | архив |
| `POST /api/offers/materials/{id}/ekn` | добавить ЕКН (повтор идемпотентен) |
| `DELETE /api/offers/materials/{id}/ekn/{eknId}` | убрать ЕКН |

### Таблица менеджера: два шага

| Метод и путь | Что делает |
|---|---|
| `POST /api/offers/offers/{id}/sheet` | разобрать xlsx и сопоставить, **ничего не записывая** |
| `POST /api/offers/offers/{id}/sheet/apply` | записать подтверждённое |

Тело первого шага — сам файл, имя в `X-File-Name`. Колонки ищутся по
заголовку: Наименование (обязательно), ЕКН, Ед., Кол-во, Цена (обязательно),
Сумма; русские числа вида `1 500,50`. Сопоставление по возрастанию доверия:
точный ЕКН → точное название после нормализации → модель только для остатка и
только как предложение (`needsConfirm`). Превью называет догадки модели,
несопоставленные строки, расхождения сумм и нулевые цены. Применение пишет
цену **из таблицы**, дописывает ЕКН, расширяет вилку (сузить нельзя), двигает
`updated_at`.

### Системы

| Метод и путь | Что делает |
|---|---|
| `GET /api/offers/systems` | системы с составом — **из кеша** `offers_context` |
| `POST /api/offers/systems/refresh` | перечитать системы из Базы знаний (admin) |
| `POST /api/offers/offers/{id}/system` | добавить систему в КП (editor); позиций НЕ добавляет |
| `DELETE /api/offers/offers/{id}/system` | убрать систему из КП (editor) |
| `GET /api/offers/bot/systems` | то же для бота (X-Service-Secret) |
| `POST /api/offers/bot/offers/{id}/system` | бот: добавить систему в КП чата |
| `DELETE /api/offers/bot/offers/{id}/system` | бот: убрать систему из КП чата |
| `POST /api/offers/offers/{id}/material` | добавить материал ОПИСАНИЕМ (editor); позиций НЕ добавляет |
| `DELETE /api/offers/offers/{id}/material` | убрать материал из КП (editor), по `sku` или `noteId` |
| `POST /api/offers/bot/offers/{id}/material` | бот: добавить материал описанием |
| `DELETE /api/offers/bot/offers/{id}/material` | бот: убрать материал |

**Материалы как объекты предложения** (2026-11-13). Та же пара ручек, что у
систем, но с другой сущностью: материал попадает в `Offer.materials` и в
документ **описанием** (`material.description` хранится в той же таблице
`offers_system_text`). Позиций не добавляет; цена строки — только через состав.
Блок формы с `repeat: "materials"` печатает выбранные материалы, а у старых КП
без них — позиции. Метка у блока одна.

**Несколько систем в одном КП** (2026-11-05). Системы хранятся списком
(`Offer.systems`, порядок выбора сохраняется); повторное добавление той же
системы не создаёт дубль. `DELETE` принимает ту же пару `sku`/`noteId` и
отвечает тем же телом (`{offer, system, title, composition}`), что добавление.
Удаление **не ищет систему в Базе знаний**: чаще всего убирают именно ту,
которой там уже нет. Системы, которой в КП нет, — 404. Пустое тело (ни `sku`,
ни `noteId`) — 400.

**Сравнение систем в документе:** узел формы `compare_table` печатает таблицу
«показатель — значение», строка на показатель, столбец на систему. Строки берутся
из таблицы `## Свойства и преимущества` карточек; порядок показателей задаёт
первая карточка, у остальных они ищутся по названию. Чего нет — пустая ячейка
(не «нет данных»). Для одной системы узел молчит: таблица из одной колонки —
не сравнение.

`GET /api/offers/systems` отдаёт **кеш** (`offers_context`, `mode='systems'`),
в Базу знаний на каждый показ раздела не ходит (2026-11-01). Пустой кеш —
База знаний читается один раз и кеш заполняется; ошибка Базы знаний — пустой
список и `error` в ответе. Перечитывает только `POST /systems/refresh`: та же
схема, что у `POST /context/refresh` — при ошибке статус 200, прежние данные и
текст в `error`.

Причина: вживую — это сотни запросов на открытие раздела (376 систем, 1165
материалов), и один отказ по токену посреди захода показывал человеку «Ключ
доступа недействителен». Клиент Базы знаний теперь ещё и повторяет запрос один
раз после 401 с новым токеном.

**Выбор системы: `sku` или `noteId`** (2026-11-02). Обе ручки принимают ровно
одно из двух полей; `GET /systems` и `GET /bot/systems` отдают `noteId` у
каждой системы. Причина: ни одна из 375 живых карточек `commercial_propose` не
несёт `system_sku`, поэтому выбор по одному коду был невыразим — список в
карточке КП состоял из вариантов с пустым значением, и кнопка отвечала
«Выберите систему». Код, если задан, главнее заметки.

**Выбор системы НЕ добавляет позиции** (2026-11-03). Ручка запоминает систему в
предложении (`offers_offer.system_choice`, `{sku?, noteId?}`) и возвращает
`{offer, system, title, composition}`. Позиции не меняются, полей `added`,
`missingPrice`, `problems` больше нет.

Причина — живое КП 003/2026: состав системы дописывался к позициям из таблицы
менеджера, и одни и те же продукты оказывались в таблице дважды — с ценой из
файла и с нулём из системы. Сматчить их нечем: в карточке «Мембрана Logicbase
V-SL», в таблице «Гидроизоляционная ПВХ мембрана Logicbase V-SL 2 мм (1 слой)»,
плюс у позиции из системы цена — нижняя граница вилки каталога, а не цена из
файла. Правило пользователя: выбор системы нужен ради **обоснования применения,
преимуществ и описания основных продуктов**, а позиции менеджер заводит сам —
файлом и/или руками.

Следствие для печати: карточка для документа выбирается по сохранённому выбору
(`cardsForOffer`), а не по совпадению названий позиций. Поэтому описание
системы печатается и у КП без позиций — это и есть порядок работы: сначала
система, потом цены.

Пустое значение `system_choice` в запросе полного сохранения предложения
(`POST/PUT /offers`) сохранённый выбор не стирает. С 2026-11-05 пустой выбор —
это `[]`, поэтому условие терпит оба вида (`'{}'` и `'[]'`).

**Связь с материалом — вики-ссылка** (решение пользователя 2026-10-31).
В составе пишут `- **2** [[Техноэласт К ЭКП]]`: связь идёт на заметку
материала в Базе знаний, а не на код. Замер на живых данных: кодов в составе
**0** из 3387 строк, вики-ссылок — 2479, из них 2103 ведут на заметки с тегом
`material`.

Порядок разрешения:

1. **ссылка** → материал, у которого `note_title` совпал с целью ссылки
   (без учёта регистра и краевых пробелов; подпись `|` и якорь `#`
   отбрасываются);
2. **код** (`MAT-…`/`РАБ-…`) — запасной способ, если ссылки нет;
3. ссылка важнее кода, если в строке есть и то, и другое.

Ссылка на заметку **без** материала в справочнике (ГОСТ, протокол) материалом
не считается. Несвязанная строка остаётся в ответе с `found: false`, отдаёт
`wiki` (цель ссылки) и попадает в `problems` — молча терять её нельзя.

**Слой — не материал и не ошибка** (2026-11-02). Строка **без кода и без
ссылки**, называющая слой или работу («Подготовка основания», «Уплотнённый
грунт»), отдаётся с `layer: true` и в `problems` **не** попадает: материала у
неё нет по смыслу, исправлять человеку нечего. На 373 живых карточках таких
строк 1273, и раньше каждая помечала систему «есть ошибки в составе». Строка
со ссылкой или с кодом слоем не считается никогда — там человек заявил
материал, и несвязанная ссылка остаётся замечанием.

**Материалы заводятся из карточек Базы знаний.** Заметка с тегом `material`
становится материалом при каждом чтении списка (`syncMaterialsFromNotes`).
Материал помнит источник (`note_title`, `note_id`). SKU назначается один раз и
синхронизацией не меняется; правки человека в плагине заметка не перетирает.
Удаление заметки материал в архив **не** уводит — архив остаётся решением
человека.

«Добавить систему» **дописывает** позиции к уже собранному КП (не заменяет).
Цена позиции — **нижняя граница вилки**; если вилки нет, позиция идёт с нулём,
а материал называется в `missingPrice` — цену загружают таблицей. Строки без
материала (кроме слоёв с `layer: true`) — в `problems` и в КП не попадают.

### Бот

`GET /api/offers/bot/catalog` возвращает **только материалы и работы**: система
позицией не является, у неё нет цены. Форма ответа прежняя (`id, name, unit,
basePrice`), добавлены `sku` и `hasPrice` — без признака ноль читался бы как
«бесплатно». Поиск идёт по названию, коду и номерам ЕКН.

Команда «система <название>» (или `/system <название>`; слово обязано стоять
первым): бот находит карточку в Базе знаний и дописывает её состав.
Одно совпадение добавляется сразу; несколько — кнопками `sys:<индекс>` (индекс,
а не код: `callback_data` ограничен 64 байтами). В ответе — сколько позиций
добавлено, материалы без цены и строки без материала.

## Боевой листок и конкуренты (2026-11-08)

Быстрая справка-сравнение нашего продукта с продуктами конкурентов. Состоит из
двух частей: **наши заметки** (`ourNotes`) и **заметки конкурентов**
(`competitorNotes`), обе — массивы `{noteId, title}` (снимок на момент
сохранения).

### Таблица `offers_combat_sheet`

`id`, `number` (`{NNN}/{yyyy}`, свой счётчик по году), `owner_email`, `title`,
`offer_id` (NULL — не привязан к КП), `our_notes JSONB`, `competitor_notes JSONB`,
`form_values JSONB`, `status` (`draft`/`final`), `created_at`, `updated_at`.

### Ручки (JWT `app_id=offers`)

- `GET    /api/offers/combat` — список (viewer; свои, admin — все).
- `GET    /api/offers/combat/{id}` — один (видимость: владелец или admin).
- `POST   /api/offers/combat` / `PUT /api/offers/combat/{id}` — сохранить (editor;
  правка — владелец или admin). Любая запись двигает `updated_at`.
- `DELETE /api/offers/combat/{id}` — удалить (editor; владелец или admin).
- `POST   /api/offers/combat/{id}/export` — DOCX боевого листка (viewer, видимость
  как у чтения). Печать по форме `combat`; карточки сторон — из кешей систем и
  конкурентов (по `noteId`, в порядке выбора). Позиции, итог, тексты секций КП и
  сводка брифа НЕ печатаются — у листка их нет.
- `GET    /api/offers/competitors` — карточки конкурентов из кеша:
  `{cards, kbEnabled, folderFound, error}`. `folderFound:false` — папки в Базе
  знаний нет (не ошибка).
- `GET    /api/offers/competitors/search?q=` — **поиск конкурентов по Базе
  знаний** (2026-11-12): полнотекстовый `GET /api/kb/search` с тегом
  `competitor` (русская морфология, префиксы, название весит выше текста) —
  тот же механизм, что у веб-версии БЗ. Ответ
  `{items:[{id,title,path}], total, error?}`; до 100 строк. Пустой `q` — пусто
  без запроса к БЗ; недоступность БЗ — `error` со статусом 200.
- `POST   /api/offers/competitors/refresh` — перечитать папку (admin).

### Боевой листок в боте (2026-11-09, поиск — 0.9.3)

Защита — `X-Service-Secret`, владелец стороны — email привязанного `chat_id`.
Сущность и печать те же, что в плагине (отдельного DOCX нет).

- `GET  /api/offers/bot/combat/systems?q=` — наш продукт: материалы каталога
  (`offers_material`, без архивных и без пустого `noteId`) **плюс** карточки систем
  Базы знаний, без повторов по заметке. Ответ `{cards, total}`; карточка
  `{noteId, title, sku?, problems?}`. `q` — поиск по названию и коду (все слова,
  регистр/«ё» не важны); пусто — весь список.
- `GET  /api/offers/bot/combat/competitors?q=` — с `q` — полнотекстовый поиск
  Базы знаний с тегом `competitor` (2026-11-12); без `q` — кеш поддерева папки.
  Ответ `{cards, folderFound, total, searched?, error?}`.
- `GET  /api/offers/bot/combat?chat_id=` — листки: **все** (2026-11-12), не
  только свои. У чужого править нельзя (`botCanEditCombat`). В каждом листке
  `displayName` — подпись для списка (наш продукт → первый конкурент →
  «пустой листок № …»).
- `POST /api/offers/bot/combat` — создать пустой листок (номер сразу).
- `POST/DELETE /api/offers/bot/combat/{id}/{our|competitor}` — добавить/убрать
  сторону `{chat_id, noteId, title}`; повтор не дублирует. Сторон с каждой
  стороны может быть несколько.
- `POST /api/offers/bot/combat/{id}/export?chat_id=` — DOCX (та же печать).
- `PUT  /api/offers/bot/combat/{id}/hint` — **уточнение для разбора** (2026-11-11):
  `{chat_id, hint}`; хранится в `form_values["hint"]`, пустая строка снимает.
  Уточнение уходит в промпт сравнения (`combat_compare.go`).
- `GET  /api/offers/bot/combat/analogues?noteId=` — **«Найти аналоги и
  сравнить»**. `noteId` — НАШ продукт (последний выбранный в листке). Ищет
  продукты **конкурентов того же класса**; ответ
  `{source:{noteId,title}, cards:[{noteId,title,sku?}]}` — до **8** штук
  (2026-11-12), не больше 2 на бренд.
  Цепочка (`findAnaloguesForSheet`): полнотекстовый поиск Базы знаний по
  запросам от общего к частному (`<класс> <назначение>` → `<класс>` →
  `<ядро> <класс>` → марка), фильтр `isAnalogCandidate` и `diversifyByBrand`;
  откат — прежняя цепочка по кешу (`findAnaloguesSmart`: журнал + модель →
  модель по карточкам → словесный подбор).
  `noteId` неизвестен — `404`.
- **Кэш готового DOCX** (2026-11-12): `offers_combat_docx(sheet_id, hash, docx)`.
  Хэш — `combatDocxHash`: номер, обе стороны в порядке выбора, уточнение, байты
  макета, `combatDocxVersion`. Хэш сошёлся — файл отдаётся из кэша, работа модели
  не повторяется. `ON DELETE CASCADE` от листка; таблица — ускорение, её можно
  снести в любой момент.

### Боевой листок: как он устроен (2026-11-11)

**Формы у боевого листка нет.** Содержание всегда одно: перечень продуктов →
сравнение. Настраивается только **макет** `.docx` режима `combat` с меткой
`{{сравнение}}` — туда печатается весь листок; без макета работает встроенное
оформление. Конфигуратор блоков и поля формы (`combatDefaultForm`) убраны;
единственное поле — `hint` (уточнение менеджера).

`combatDefaultForm` в БД **не сеется**: пока макета нет, отдаётся заготовка без
блоков (единственный плейсхолдер — уточнение). Это осознанно: пустая
«настроенная» строка показала бы пустую форму.

Сравнение пишет модель общим ключом `llm-service` (`combat_compare.go`): общее
сравнение и по каждому продукту конкурента отдельно, факты только из карточек.
Модель недоступна — печатается перечень карточек с характеристиками.

**Подбор аналогов** (`combat_analog.go`, `combat_analog_llm.go`,
`combat_journal.go`): идёт ОТ НАШЕГО продукта и ищет конкурентов того же класса.
Источники по приоритету:

1. **журнал конкурентов** — заметка с тегом `journal` («Журнал материалов
   конкурентов»). Индексатор пишет там группы по разделам каталога; конкурент и
   раздел — оба `##`, различаются счётчиком товаров в заголовке («— 28 шт.»);
   строка таблицы `| статус | материал | url | [[заметка]] |` даёт **ссылку на
   целевую заметку**. Модель выбирает строки, строки превращаются в карточки по
   `[[…]]`;
2. модель по карточкам конкурентов (до 60 кандидатов);
3. словесный подбор по классу продукта (`findAnalogues`, без модели).

Класс продукта считается **только по названию** (`analogClassOf`), признаки
применения («кровельная») класс не задают, «активатор»/«очиститель»/«шайба» —
свои классы. Подбор добавляет найденное **на сторону конкурента**.

### Источник конкурентов

Папка Базы знаний **«материалы и системы конкурентов»** (не тег). Сервис
находит папку по имени (`KBClient.FolderIDByName`, без учёта регистра), берёт
**всё поддерево** (`KBClient.SubtreeFolderIDs` — папка и все потомки на любую
глубину, 2026-11-09), читает заметки каждой папки по `folder_id`
(`FolderNotes`, с пагинацией), разбирает их тем же `skills.ParseSystemCard`
(названием карточки служит заголовок заметки) и кеширует в `offers_context`
`mode='competitors'` (формат `{found, cards}`). Ошибка Базы знаний не стирает
прежний кеш.

### Печать DOCX (`combat_docx.go`)

`renderCombatDocx(form, sheet, ourCards, competitorCards, images)`:
- блоки формы печатаются тем же `renderBlockContentCtx`, что и у КП (подстановки
  `{{поле}}`, служебные поля, данные карточек по меткам);
- после блоков — два раздела: «Наш продукт» и «Продукты конкурентов», каждая
  карточка — название, описание, картинка с обтеканием, состав списком,
  характеристики таблицей;
- ссылки собираются из обеих сторон и дедуплицируются по адресу;
- собирается на фирменном шаблоне (`buildDocxWithNumbering`). Модель при печати
  не вызывается.
## Агент (R4, шаг P4 — 2026-11-14)

**Одно правило, три потребителя.** Цикл «план → tool_call → observe → final»
живёт ОДНИМ файлом: `sbe-core/src/agent/engine.ts`. Он не импортирует ничего —
все зависимости приходят аргументом `AgentEngineDeps`:

| зависимость | кто даёт |
|---|---|
| `complete(system, user, {model})` | LLM-центр (плагин — `sbe-llm`, портал — `llmApi`, бот — `llm-service`) |
| `completeForCompaction` | тот же клиент, другой промпт |
| `buildSystemPrompt(tools)` | промпт + правила + схемы тулов своего потребителя |
| `emit` / `progress` | чат: сообщение и статус |
| `logTurn(AgentTurnLog)` | журнал хода (для P6: разбор переписки менеджеров) |

Типы: `AgentToolSchema` (MCP-совместимая схема), `ToolCallResult`
(`ok/summary/data/error/link`), `LlmTurn` (`final` / `tool_call` /
`protocol_error`), `AgentTurnLog` (`userText`, `tools[]`, `answer`,
`reason`, `latencyMs`).

**Копии:** `web/sbe-web/src/modules/agent/engineCore.ts` (портал),
`sbe-offers/offers-bot/src/agent/engine.ts` (бот КП). Сверка —
`plugins/scripts/check_agent_engine.mjs`: константы правила и шапка «копия
канона», плюс 15 поведенческих проверок на самом каноне.

**Разбор ответа модели** (`parseTurns`):

| форма | что делаем |
|---|---|
| `{"type":"tool_call",…}` / `{"type":"final",…}` | штатно |
| формы без `type` (`{"tool":…}`), OpenAI (`{"name":…,"arguments":"{…}"}`) | приводим к вызову |
| DSML-разметка DeepSeek | посимвольный разбор: тег `<…DSML…>` в любом начертании чёрточек, имя блока (`tool_calls`/`invoke`/`parameter`) перед `>`, параметры `string="true"` остаются строками |
| оборванный на одну `}` JSON | достраиваем скобки |
| обычный текст | законный финальный ответ |
| разметка вызова, которую не привели к своей форме | `protocol_error` → модель переспрашивается (до 2 раз), пользователю не показывается |

**Правила цикла:** не больше 15 шагов; одинаковый вызов (те же аргументы)
более 2 раз и один инструмент подряд более 5 раз останавливают ход, но не
выбрасывают найденное — один ход синтеза ответа по транскрипту; если человек
просил файл, а ход кончается текстом без единого выдающего тула — один
переспрос модели; история длиннее 50 000 символов сжимается в дайджест, 8
последних сообщений остаются как есть.
