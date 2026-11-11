/**
 * Бюджет бандла приложения (2026-11-13, S0 плана миграции).
 *
 * План держит потолок «до 200 КБ gzip» (START-HERE §5.4). Предупреждение
 * самой Vite меряет НЕсжатый размер, поэтому бюджет проверяется здесь — по
 * файлам в dist/, ровно так, как их получит телефон.
 *
 * Запуск: `npm run size` (из папки offers-app) после `npm run build`.
 */
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { gzipSync } from 'node:zlib';

const DIST = new URL('../dist/', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1');
const BUDGET_BYTES = 200 * 1024;

function walk(dir) {
  const out = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) out.push(...walk(full));
    else out.push(full);
  }
  return out;
}

let total = 0;
let overBudget = false;

for (const file of walk(DIST)) {
  // Карты исходников на телефоне не скачиваются — их в бюджет не считаем.
  if (file.endsWith('.map')) continue;
  const raw = readFileSync(file);
  const gz = gzipSync(raw).length;
  total += gz;
  const shown = relative(DIST, file).replace(/\\/g, '/');
  console.log(`${shown.padEnd(34)} ${String(raw.length).padStart(8)} Б → ${String(gz).padStart(7)} Б gzip`);
}

const status = total <= BUDGET_BYTES ? 'OK' : 'ФЕЙЛ';
if (total > BUDGET_BYTES) overBudget = true;

console.log(`\nИтого gzip: ${total} Б из ${BUDGET_BYTES} Б — ${status}`);
if (overBudget) {
  console.error('Бюджет плана превышен: см. START-HERE §5.4.');
  process.exit(1);
}

// Пустой dist (сборку забыли) — тоже ошибка: иначе проверка «зелёная» на нуле.
if (!statSync(DIST).isDirectory()) process.exit(1);