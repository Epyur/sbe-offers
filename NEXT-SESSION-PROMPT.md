# Промт для нового контекстного окна (sbe-offers)

Файл — готовое ТЗ для следующей сессии. Открывать новое окно с короткой фразой:

> Открой `C:\Obsidian\mailers\.obsidian\plugins\sbe-offers\NEXT-SESSION-PROMPT.md`
> и выполни его как ТЗ. Начни с задачи №1.

Ниже — полный текст промта (он же копируется целиком, если файл недоступен).

---

Ты — центральный агент по плагину sbe-offers (LogicTEAM.КП). Рабочая папка:
C:\Obsidian\mailers\.obsidian\plugins\sbe-offers (ветка backend). Бэки живут
только на backend: offers-service/ (Go), offers-bot/ (Node), offers-app/
(Mini App). main — релизный срез без них.

СНАЧАЛА прочитай: HANDOFF.md (верхний блок «0.11.8» и «Три точки сходятся»),
затем AGENTS.md — разделы «Purpose (current)» (Combat sheet) и «Gotchas»
(две карточки-кэша, DOCX-кэш, список). Только потом код.

СОСТОЯНИЕ: выпущена 0.11.8 (2026-11-18). Точки выпуска:
backend код 7e5dc9e (доки fe203fe, HEAD f0a8179), main код 68fc53e
(доки 655d3af), реестр sbe-apstore-registry 50fe7a2, uploadedAt
2026-11-18T16:12:58Z. Файлы: manifest 394b742b…, main d3011222…,
styles 50bad65b…; приложение assets/index-nXQtPejD.js (sha d6aefbe6…).
Стенд 599/0.

ЧТО СДЕЛАНО В 0.11.8 (не переделывать):
1) Ссылки наших карточек входят в combatDocxHash (combatOurRefs,
   combatDocxVersion = v8), пустой список печатает видимую строку с адресом
   кнопки; подсказка отправляет в «Контекст КП» (POST /context/refresh), а не
   в «Системы» (POST /systems/refresh — другая строка кэша).
2) Списки GET /combat и GET /bot/combat читают кэш конкурентов один раз на
   запрос (loadCombatSheetWithCards), индекс offers_combat_sheet_created.
3) «Отправить в чат» сразу пишет «⌛ Формирую …» (answerStartText + notifyChat),
   затем файл; notifyChat читает HTTP-статус бота.
4) systemCardsForCombat при пустом кэше зовёт refreshSystemCards, а не
   SystemsFromKB (иначе кэш теряет материалы и LinkedRefs).

ЗАДАЧА №1 (главная, решение пользователя уже принято): вынести СТРУКТУРУ
разделов боевого листка из кода в настройку — вариант A, «только разделы».
Сейчас заготовка — combatDefaultSections() в form.go:515, редактора нет.
Нужно:
- в «⚙️ Настройки боевого листка» (src/ui/combat-settings.ts) добавить
  редактор разделов, переиспользовав src/ui/sections-editor.ts; сохранение —
  тем же PUT /form с mode='combat' (sections уже читаются/пишутся);
- id раздела после создания НЕИЗМЕНЯЕМ (иначе обнулится кэш текстов
  gen.sect.*);
- пустое сохранение отклонять русским текстом + кнопка «↩ Вернуть заготовку»;
- каркас «Продукты сравнения → стороны → разделы → ссылки» НЕ делать
  настраиваемым: это реверс решения 0.11.6;
- помнить: combatFormFingerprint входит в хэш файла, правка разделов
  пересобирает листки (это правильно).

ЗАДАЧА №2 (после замера): если список на большом объёме всё ещё медленный —
добавить OPT-IN ?fields=preview (отдельный DTO: id, number, title, стороны,
status, owner, даты; без form_values и text_competitors). Формат GET /combat
по умолчанию НЕ менять: старый клиент может отправить превью полным PUT и
затереть form_values.

ЗАДАЧА №3 (проверка, не код): после следующей выкладки посмотреть в реальном
Telegram, что «⌛» приходит сразу после нажатия «Отправить в чат», файл — по
готовности, а отказ бота виден словами.

ПРАВИЛА:
- Коммиты, пуши, выкладка и «Фиксируй» — только по явной команде пользователя.
- «Фиксируй» = версия +0.0.1 в manifest.json и package.json, новая новость ЦУП
  в src/main.ts, иглы в scripts/check_bundle_strings.mjs (без «ёлочек» —
  esbuild пишет их как \xAB), доки AGENTS/HANDOFF/specification, спросить перед
  коммитом.
- Правки больших .mjs и .go — только edit/write или python с UTF-8; не
  PowerShell-пайплайнами (грабля CRLF и mojibake).
- Доки: AGENTS «Code heads» обязан называть последний коммит, трогавший
  codePaths (включая specification.md), иначе check_docs_state красный.

ПРОВЕРКИ ПЕРЕД СДАЧЕЙ:
cd offers-service && gofmt -l . && go vet ./... && go test -count=1 ./...
cd .. && node ../scripts/check_offers_openapi.mjs            # 124/124
npx tsc --noEmit && node esbuild.config.mjs production
node scripts/check_bundle_strings.mjs && node scripts/check_marker_registry.mjs
node scripts/check_sections_conditions.mjs && node scripts/check_docs_state.mjs
cd offers-bot && npm run typecheck && node --test "src/**/*.test.ts"   # 220
cd ../offers-app && npm run typecheck && npm test && npm run build && npm run size
# стенд (Docker поднят): бинарь offers-stand собрать локально с GOOS=linux,
# docker cp в offers-local-app, очистить offers_context mode in
# ('systems','system_cards') и offers_combat_docx, затем:
docker run --rm --network sbe-offers_offersnet \
 -e OFFERS_BOT_URL=http://offers-local-bot-stub:3000 -e BOT_DIR=/bot \
 -v "$PWD/scripts:/app:ro" -v "$PWD/offers-bot:/bot:ro" -w /app \
 node:22-alpine node offers_local_e2e.mjs http://offers-local-app:3000

ГРАБЛИ, КОТОРЫЕ УЖЕ СТОИЛИ ВРЕМЕНИ:
- В листке ДВА кэша карточек: «Системы» пишет mode='systems', листок читает
  mode='system_cards' (обновляется только «Контекст КП»).
- Кэш DOCX offers_combat_docx не инвалидируется сам: меняешь печать —
  поднимай combatDocxVersion (сейчас v8).
- Прямая Cyrillic-строка в ssh-команде из PowerShell ломает кавычки: пиши
  .sh-файл локально → scp → sh /tmp/x.sh.
- Стендовые заглушки читают .mjs один раз при старте: после правки — restart
  контейнера заглушки.