# specification.md — sbe-offers (LogicTEAM.КП)

Форматы обмена плагина с `offers-service` (`https://epyur.fvds.ru/api/offers/*`)
и ручки, которыми пользуется Telegram-бот.

## Авторизация

- Плагин: JWT Bearer `app_id=offers` (ключ из ЦУП —
  `getService('sbe-apstore').auth.getToken('offers')`).
- Роли: `viewer`(1) < `editor`(2) < `admin`(3); действует персональная роль
  или общий доступ приложения (источник — auth-service, `authperms.go`).
- Бот: заголовок `X-Service-Secret` = `OFFERS_BOT_SECRET` (ручки `/bot/*`),
  JWT не нужен.
- 401 — «Ключ недействителен», 403 — «Нет прав».

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
Если форма не настроена — отдаётся форма по умолчанию (клиент, контакт,
условия, срок действия).

```jsonc
OfferForm = { id, mode, title, placeholders: [{id, label}],
              blocks: [{id, title, content: RichNode[], showInClient, showInInternal}],
              requiredFields: string[], updatedAt }

RichNode = { type: "paragraph"|"heading"|"bullet_list"|"table",
             level?: 2|3|4, align?: "center"|"right"|"justify",
             text?: string, items?: string[], rows?: string[][] }
```

Плейсхолдер в тексте — `{{id}}`; сервер подставляет значения из
`formValues` КП, неизвестные вырезает.

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
          total, status: "draft"|"final", docxKey, createdAt, updatedAt }
```

- `items` — **снимок**: цена фиксируется при добавлении карточки, правка каталога
  старые предложения не меняет.
- `sum` и `total` пересчитывает сервер: каждая строка округляется до копеек,
  затем итог (3 × 999.99 → 2999.97, без двоичных хвостов).
- Номер — `{NNN}/{yyyy}`, сквозной по году (advisory-lock по году).
- Пустое предложение (без позиций) — 400 «в предложении нет позиций».

## DOCX

- Бланк — корпоративный `template/standard.docx` (шапка с логотипом, футер с
  реквизитами), тело заменяется: заголовок формы, номер и дата, клиент, блоки
  с `showInClient`, таблица позиций, итог, подпись менеджера.
- Оформление текста — Times New Roman 14pt, полуторный интервал, отступ 1,25 см,
  выравнивание по ширине (тот же `styles.xml`, что у `agent-service`).
- Суммы в документе — русский формат «1 234,56».
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
| `POST /bot/offers` | `{chat_id, client, items, formValues}` | `Offer` |
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
Сборка: каталог (кнопки, поиск текстом, листание) → количество → «➕ Ещё» /
«✏️ Добавить позицию» (строка `Название | количество | цена`) / «🗑 Убрать
последнюю» → «✅ Готово» → вопрос «Название клиента» → создание КП → DOCX в чат
+ кнопки «🔗 Ссылкой», «🧾 Новое КП».
Роль `viewer` получить КП не может — бот отвечает просьбой обратиться к
администратору.

## Схема БД (`offers`)

```sql
offers_catalog  (id, kind, name, unit, base_price, description, composition jsonb,
                 photo_key, sort_order, archived, created_at, updated_at)
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
pp_id), каталог (создание, правка, архив, поиск, валидация, 404 на
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

### Бриф

`offers_brief` (одна строка на предложение) + `offers_brief_turn` (журнал
реплик). Поля брифа: `object_name`, `customer_pains`, `customer_goals`,
`project_risks`, `objections` (возражение + ответ), `budget_signal`,
`decision_maker`, `timeline`, `why_us`, `notes`, `skipped_topics`.

Темы задаются манифестом (`skills.BriefTopics`), порядок стабилен. Тема
закрыта, когда поле заполнено ИЛИ она в `skipped_topics` («не знаю»).

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
               updatedAt, hasDescription, hasComposition, empty }
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

## Файлы из чата: .md и .xlsx (2026-10-30)

Менеджер присылает в бота контекст объекта или таблицу спецификации файлом.
**Приём двухшаговый:** `attach` разбирает и показывает «что понял», `apply`
пишет только подтверждённое. Файл от заказчика — черновик (старые цены,
опечатки), молчаливое применение уронило бы КП в глазах заказчика.

| Ручка | Тело | Ответ |
|---|---|---|
| `POST /bot/offers/{id}/attach?chat_id=&file_name=` | байты файла (`application/octet-stream`) | `{kind, name, key?, reply, needsConfirm, brief?, spec?, context?}` |
| `POST /bot/offers/{id}/attach/apply` | `{chat_id, mode: brief\|spec, brief?, spec?}` | `{reply, applied}` |

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
`xl/worksheets/sheetN.xml`, оба вида ячеек (словарные и числовые). Шапка
ищется в первых 20 строках; таблица считается спецификацией, если есть колонки
«наименование» и «количество». Предел — 200 строк. `сумма ≠ количество × цена`
попадает в `warnings`, а не исправляется молча. Без колонок состава лист
отдаётся как контекст (`context: string[]`).

**Сверка с каталогом** идёт по наименованию (`spec.matched` / `new` /
`priceDiffs`). **Цены в КП берутся из каталога** для позиций, которые в нём
нашлись: цена файла используется только для новых позиций. Применение состава
двигает `updated_at`.

**Применение брифа** — слияние, а не замена: пустое не затирает известное,
списки объединяются без повторов, возражение без ответа в бриф не попадает.