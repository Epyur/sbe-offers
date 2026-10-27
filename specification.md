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
