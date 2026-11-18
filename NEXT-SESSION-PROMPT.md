# Промт для нового контекстного окна (sbe-offers)

## Как открыть новое окно

1. Открой новый чат в папке воркспейса и вставь **одну строку**:

   > Открой `C:\Obsidian\mailers\.obsidian\plugins\sbe-offers\NEXT-SESSION-PROMPT.md`
   > и выполни его как ТЗ.

2. Ничего больше добавлять не нужно: состояние, проверки, грабли и открытые
   задачи — в этом файле, подробности — в `HANDOFF.md` и `AGENTS.md`.

3. Если файл недоступен — вставь как ТЗ раздел «Полное ТЗ» ниже, начиная с
   «Ты — центральный агент…».

## Что прочитать в первую минуту

1. `HANDOFF.md` — верхний блок «0.11.9» и «Три точки сходятся (0.11.9)»: версия,
   хэши, выкладка, что не проверено.
2. `AGENTS.md` — «Purpose (current)» (Combat sheet) и «Gotchas» (две карточки-кэша,
   DOCX-кэш, разделы листка, список).
3. Только потом код.

---

## Полное ТЗ (копируется целиком)

Ты — центральный агент по плагину sbe-offers (LogicTEAM.КП). Рабочая папка:
`C:\Obsidian\mailers\.obsidian\plugins\sbe-offers` (ветка `backend`). Бэки живут
только на `backend`: `offers-service/` (Go), `offers-bot/` (Node), `offers-app/`
(Mini App). `main` — релизный срез без них.

СНАЧАЛА прочитай `HANDOFF.md` (верхний блок «0.11.9» и «Три точки сходятся»),
затем `AGENTS.md` — «Purpose (current)» (Combat sheet) и «Gotchas» (две
карточки-кэша, DOCX-кэш, разделы листка, список). Только потом код.

### СОСТОЯНИЕ

Выпущена **0.11.9** (2026-11-18). Точки выпуска (хэши веток, самораздачи,
реестра, приложения и сервиса) — `HANDOFF.md`, «Три точки сходятся (0.11.9)»;
здесь они не дублируются, чтобы не разъезжаться.

Что вошло в 0.11.9:

1. **Разделы боевого листка правятся в настройках** (решение пользователя:
   вариант A, «только разделы»). Блок «Содержательные разделы боевого листка» в
   «⚙️ Настройки боевого листка» (`src/ui/combat-settings.ts`) — редактор КП
   (`renderSectionsEditor`) в режиме `combat`, сохранение тем же `PUT /form` с
   `mode='combat'`. Правила: `id` раздела после создания не меняется; пустое
   сохранение отклоняется русским текстом (текст один на сервер и плагин);
   «↩ Вернуть заготовку» ставит `sectionsDefault`; пустой список разделов у
   листка ЗНАЧИТ заготовку (так печатает `renderCombatSections`, поэтому и
   `loadForm` её подставляет); каркас «продукты → стороны → разделы → ссылки» не
   настраивается.
2. **Короткий список листков в приложении** — opt-in `GET /combat?fields=preview`
   (`CombatSheetPreview`): id, номер, название, стороны, состояние, автор, даты,
   подпись, без `formValues`/`textCompetitors`. Замер: 200 листков по 8 КБ —
   1610 КБ / 87 мс полный против 56 КБ / 72 мс короткого
   (`scripts/measure_combat_list.mjs`). Формат по умолчанию НЕ менялся: плагин
   читает полный листок из списка и сохраняет целиком.
3. Справка, `specification.md`, `AGENTS.md` (три gotcha), OpenAPI
   (`sections`, `sectionsDefault`, `ContentSection`, `ContentSubsection`,
   параметр `fields`) и сторож `scripts/check_combat_sections_rules.mjs`
   (20 проверок).

### ОТКРЫТЫЕ ЗАДАЧИ (по порядку)

1. **Проверка «⌛» в живом Telegram** (не код): после выкладки нажать «Отправить в
   чат» в карточке листка и убедиться, что сообщение «⌛ Формирую …» приходит
   сразу, файл — по готовности, а отказ бота виден словами. Стенд этого не
   умеет: его заглушка бота не знает `/internal/notify`.
2. **Живая проверка нового окна и списка** (не код): открыть «⚙️ Настройки
   боевого листка» в Obsidian руками (правка раздела, «↩ Вернуть заготовку»,
   отказ пустого сохранения), собрать листок на бою под настоящей моделью,
   открыть список в мини-приложении и убедиться, что он грузится и показывает
   подписи.
3. **Дальше по backlog** — `AGENTS.md`, раздел «Next iterations (backlog)»:
   самый близкий к коммерческому отделу пункт — сравнение с аналогами из Базы
   знаний (не начато), затем `.docx` как источник контекста (сегодня файл
   отклоняется).
4. **Шаг 2c из старого плана** (если всплывёт жалоба): у КП ключ кэша текстов
   систем всё ещё `noteId`, поэтому правка тела карточки не переписывает текст;
   у листка это уже закрыто.

### ПРАВИЛА

- Коммиты, пуши, выкладка и «Фиксируй» — только по явной команде пользователя.
  «Продолжай», «ок», «делай дальше» — не команда.
- **«Фиксируй»** = версия +0.0.1 в `manifest.json` и `package.json`, новая
  новость ЦУП в `src/main.ts`, иглы в `scripts/check_bundle_strings.mjs`
  (без «ёлочек» — esbuild пишет их как `\xAB`, а иглы прошлого выпуска снимаются
  вместе с текстом новости), доки `AGENTS`/`HANDOFF`/`specification`, спросить
  перед коммитом.
- Правки больших `.mjs` и `.go` — только `edit`/`write` или python с UTF-8; не
  PowerShell-пайплайнами (грабля CRLF и mojibake).
- Доки: `AGENTS.md`, «Code heads» обязан называть последний коммит, трогавший
  `codePaths` (включая `specification.md`), иначе `check_docs_state` красный.
- Кириллица в `ssh`-команде из PowerShell ломает кавычки: писать `.sh` локально →
  `scp` → `sh /tmp/x.sh`.

### ПРОВЕРКИ ПЕРЕД СДАЧЕЙ

```
cd offers-service && gofmt -l . && go vet ./... && go test -count=1 ./...
cd .. && node ../scripts/check_offers_openapi.mjs            # 124/124
npx tsc --noEmit && node esbuild.config.mjs production
node scripts/check_bundle_strings.mjs && node scripts/check_marker_registry.mjs
node scripts/check_sections_conditions.mjs                    # 16/16
node scripts/check_combat_sections_rules.mjs                  # 20/20
node scripts/check_docs_state.mjs
cd offers-bot && npm run typecheck && node --test "src/**/*.test.ts"   # 220
cd ../offers-app && npm run typecheck && npm test && npm run build && npm run size  # 90
# стенд (Docker поднят): бинарь offers-stand собрать локально с GOOS=linux,
# docker cp в offers-local-app, очистить offers_context mode in
# ('systems','system_cards') и offers_combat_docx, затем:
docker run --rm --network sbe-offers_offersnet \
 -e OFFERS_BOT_URL=http://offers-local-bot-stub:3000 -e BOT_DIR=/bot \
 -v "$PWD/scripts:/app:ro" -v "$PWD/offers-bot:/bot:ro" -w /app \
 node:22-alpine node offers_local_e2e.mjs http://offers-local-app:3000   # 620/0
# замер списка (нужен поднятый стенд):
node scripts/measure_combat_list.mjs 200
```

### ГРАБЛИ, КОТОРЫЕ УЖЕ СТОИЛИ ВРЕМЕНИ

- В листке ДВА кэша карточек: «Системы» пишет `mode='systems'`, листок читает
  `mode='system_cards'` (обновляется только «Контекст КП»).
- Кэш DOCX `offers_combat_docx` не инвалидируется сам: меняешь печать —
  поднимай `combatDocxVersion` (сейчас **v8**). Форма входит в хэш
  (`combatFormFingerprint`), поэтому правка разделов пересобирает файл сама.
- **Пустой список разделов у листка = заготовка**, а не «печатать без разделов»:
  печать и чтение подставляют одно и то же. Проверять «листок без разделов»
  через базу бессмысленно — такого состояния нет.
- **`id` раздела входит в ключ кэша** (`gen.sect.<раздел>.<подраздел>`):
  переименование оставляет тексты под старым ключом и молча зовёт модель заново.
- **Проверка бандла — только скриптом**: esbuild пишет кириллицу как `\uXXXX` в
  ВЕРХНЕМ регистре, а «ёлочки» — как `\xAB`.
- **Кириллица в SQL через `docker exec -i psql`** — тот же класс: замер пишет в
  базу только ASCII (`repeat('a', N)`), иначе psql получает mojibake.
- Заглушки стенда читают `.mjs` один раз при старте: после правки — restart
  контейнера заглушки.
- `git archive` на Windows применяет `core.autocrlf`: собирать серверный архив
  `git -c core.autocrlf=false archive …`, иначе на сервер уезжает CRLF и
  `CR_TEXT` перестаёт быть нулём.
- `up -d` без `--force-recreate` может не пересоздать контейнер: смотреть
  `CREATED` в `docker ps`, а не слово `Started` в выводе compose.