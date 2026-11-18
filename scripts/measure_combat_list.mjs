/**
 * Замер списка боевых листков на объёме (2026-11-18, задача №2 из
 * `NEXT-SESSION-PROMPT.md`).
 *
 * Зачем. После 0.11.8 список читает кэш конкурентов ОДИН раз на запрос, но
 * по-прежнему отдаёт ПОЛНЫЙ листок в каждой строке (`form_values` с текстами
 * разделов). Вопрос задачи: хватает ли этого или нужен opt-in `?fields=preview`.
 * Ответ должен быть числом, а не мнением, поэтому здесь два замера:
 *
 *   1. объём ответа — сколько килобайт получает мини-приложение на один листок;
 *   2. время — сколько ждёт человек до первого отрисованного списка.
 *
 * Как. Ручки на бою закрыты JWT, поэтому замер идёт на СТЕНДЕ (`offers-local-*`):
 * скрипт досыпает в его базу синтетические листки нужного размера, меряет
 * `GET /api/offers/combat` и убирает за собой. Размер одного листка берётся
 * живой: на боевой базе 2026-11-18 список из 17 листков весил 133 КБ, то есть
 * ~7.8 КБ на листок.
 *
 * Запуск:
 *   node scripts/measure_combat_list.mjs [количествоЛистков] [baseUrl]
 *   node scripts/measure_combat_list.mjs 200
 *
 * Требует поднятого стенда (`docker compose -f docker-compose.local.yml up -d`)
 * и контейнера базы `offers-local-db`. Данные стенда после прогона чистые:
 * синтетические строки удаляются и при успехе, и при падении замера.
 */
import { createHmac } from 'node:crypto';
import { spawnSync } from 'node:child_process';

const COUNT = Number(process.argv[2] ?? 200);
const BASE = (process.argv[3] ?? 'http://127.0.0.1:18080').replace(/\/+$/, '');
// Секрет стенда — из `docker-compose.local.yml`. На бою его нет в репозитории.
const SECRET = 'test-service-secret';
const OWNER = 'owner@tn.ru';
const MARK = 'zamer-list-2026-11-18';
// Живой размер одного листка: 133 КБ / 17 листков (HANDOFF, 0.11.8).
const TEXT_CHARS = 7600;

function jwt(email) {
  const b64 = (v) => Buffer.from(v).toString('base64url');
  const now = Math.floor(Date.now() / 1000);
  const head = b64(JSON.stringify({ alg: 'HS256', typ: 'JWT' }));
  const body = b64(JSON.stringify({
    email, app_id: 'offers', device_id: 'measure', iss: 'auth-service', aud: 'offers',
    exp: now + 600, iat: now,
  }));
  const sig = createHmac('sha256', SECRET).update(`${head}.${body}`).digest('base64url');
  return `${head}.${body}.${sig}`;
}
const token = jwt(OWNER);

/** psql на контейнере стенда: только ASCII-SQL, без кириллицы в аргументах. */
function psql(sql) {
  const res = spawnSync('docker', ['exec', '-i', 'offers-local-db', 'psql', '-U', 'offers', '-d', 'offers', '-v', 'ON_ERROR_STOP=1', '-q', '-t', '-A'], {
    input: sql,
    encoding: 'utf8',
  });
  if (res.status !== 0) {
    throw new Error(`psql: ${res.status}: ${(res.stderr ?? '').trim()}`);
  }
  return (res.stdout ?? '').trim();
}

function insertSheets(count) {
  // Содержимое — повтор латинской строки: замер меряет РАЗМЕР, а не смысл, и
  // кириллица в SQL через PowerShell/psql уже ломала аргументы (грабля 2026-11-17).
  const sql = `
INSERT INTO offers_combat_sheet (number, owner_email, title, our_notes, competitor_notes, form_values, status, created_at)
SELECT 'ZM-' || i || '/2026', '${OWNER}', '${MARK} ' || i,
       '[]'::jsonb, '[]'::jsonb,
       jsonb_build_object(
         'gen.section', md5('s' || i),
         'gen.sect.general.overview', repeat('a', ${TEXT_CHARS}),
         'gen.sect.general.specs', repeat('b', 900)
       ),
       'draft', now() - (i || ' minutes')::interval
FROM generate_series(1, ${count}) AS i;
`;
  psql(sql);
}

function deleteSheets() {
  psql(`DELETE FROM offers_combat_sheet WHERE title LIKE '${MARK}%';`);
}

async function listOnce(path = '') {
  const started = process.hrtime.bigint();
  const res = await fetch(`${BASE}/api/offers/combat${path}`, { headers: { Authorization: `Bearer ${token}` } });
  const text = await res.text();
  const ms = Number(process.hrtime.bigint() - started) / 1e6;
  return { status: res.status, bytes: Buffer.byteLength(text), ms, sheets: (JSON.parse(text || '{}').sheets ?? []).length };
}

function stat(name, runs) {
  const times = runs.map((r) => r.ms).sort((a, b) => a - b);
  const bytes = runs[runs.length - 1].bytes;
  const sheets = runs[runs.length - 1].sheets;
  const median = times[Math.floor(times.length / 2)];
  console.log(
    `${name}: листков ${sheets}, ответ ${(bytes / 1024).toFixed(0)} КБ` +
    ` (${(bytes / Math.max(sheets, 1) / 1024).toFixed(1)} КБ на листок),` +
    ` время ${times[0].toFixed(0)}/${median.toFixed(0)}/${times[times.length - 1].toFixed(0)} мс (мин/медиана/макс)`,
  );
  return { sheets, bytes, median, min: times[0], max: times[times.length - 1] };
}

async function measure(name, path = '') {
  const runs = [];
  for (let i = 0; i < 5; i += 1) runs.push(await listOnce(path));
  const bad = runs.find((r) => r.status !== 200);
  if (bad) throw new Error(`${name}: список ответил ${bad.status}`);
  return stat(name, runs);
}

console.log(`Стенд ${BASE}, синтетических листков: ${COUNT}\n`);

const before = await measure('до замера, полный список');
const beforePreview = await measure('до замера, ?fields=preview', '?fields=preview');
insertSheets(COUNT);
try {
  const after = await measure(`с синтетикой (+${COUNT}), полный список`);
  const afterPreview = await measure(`с синтетикой (+${COUNT}), ?fields=preview`, '?fields=preview');
  console.log('');
  console.log(`Прирост полного ответа: ${((after.bytes - before.bytes) / 1024).toFixed(0)} КБ,` +
    ` медиана ${before.median.toFixed(0)} → ${after.median.toFixed(0)} мс`);
  console.log(`Короткий список дешевле полного в ${(after.bytes / Math.max(afterPreview.bytes, 1)).toFixed(1)} раза` +
    ` (${(afterPreview.bytes / 1024).toFixed(0)} КБ против ${(after.bytes / 1024).toFixed(0)} КБ)`);
  // Порог — не вкусовой: мини-приложение рисует список в Telegram WebView, и
  // ответ больше ~2 МБ там уже заметен как «страница не отвечает» (телефон
  // разбирает JSON в главном потоке). Время — по живому замеру 0.11.8 (401 мс
  // на 17 листках): до 1 с список считается быстрым.
  const heavyBytes = after.bytes > 2 * 1024 * 1024;
  const slow = after.median > 1000;
  console.log(heavyBytes ? 'ВЫВОД: полный ответ тяжёлый (>2 МБ) — ?fields=preview нужен' : 'ВЫВОД: полный ответ в пределах 2 МБ');
  console.log(slow ? 'ВЫВОД: список медленный (>1 с) — нужен превью-DTO' : 'ВЫВОД: список быстрый (<=1 с), вклад сервера не растёт');
  void beforePreview;
} finally {
  deleteSheets();
  const afterCleanup = await listOnce();
  console.log(`\nСинтетика удалена, в списке снова ${afterCleanup.sheets} листков.`);
}