/**
 * Бюджет бандла приложения (2026-11-13, S0 плана миграции).
 *
 * План держит потолок «до 200 КБ gzip» (START-HERE §5.4) — это КОД. С
 * 2026-11-14 в бандл вшиты три макета разделов (`section-images.ts`,
 * `data:`-строки webp), и они не сжимаются: данные картинки, а не код.
 * Держать картинки внутри кодового бюджета значило бы ссылаться на потолок
 * кода, который они не расходуют, — поэтому бюджет считается раздельно:
 *
 *   код — всё, кроме картинок приветствия;
 *   картинки — отдельный потолок `SECTION_BUDGET_BYTES`, чтобы повод
 *   «только поставим макет покрупнее» упирался в число, а не в терпение.
 *
 * Запуск: `npm run size` (из папки offers-app) после `npm run build`.
 */
import { readdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';
import { gzipSync } from 'node:zlib';

const DIST = new URL('../dist/', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1');
const BUDGET_BYTES = 200 * 1024;
const SECTION_BUDGET_BYTES = 200 * 1024;

function walk(dir) {
  const out = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) out.push(...walk(full));
    else out.push(full);
  }
  return out;
}

/** Картинки приветствия сидят в единственном чанке как `data:`-строки. */
function sectionImagesBytes(js) {
  let sum = 0;
  for (const match of js.matchAll(/data:image\/[^"']+;base64,[A-Za-z0-9+/=]+/g)) {
    sum += gzipSync(match[0]).length;
  }
  return sum;
}

let codeTotal = 0;
let sectionTotal = 0;

for (const file of walk(DIST)) {
  // Карты исходников на телефоне не скачиваются — их в бюджет не считаем.
  if (file.endsWith('.map')) continue;
  const raw = readFileSync(file);
  const gz = gzipSync(raw).length;
  const shown = relative(DIST, file).replace(/\\/g, '/');

  let sections = 0;
  if (file.endsWith('.js')) {
    sections = sectionImagesBytes(raw.toString('utf8'));
    sectionTotal += sections;
  }
  codeTotal += gz - sections;

  const note = sections === 0 ? '' : ` (из них картинки разделов ${sections})`;
  console.log(`${shown.padEnd(30)} ${String(raw.length).padStart(8)} Б → ${String(gz).padStart(7)} Б gzip${note}`);
}

const codeOk = codeTotal <= BUDGET_BYTES;
const sectionsOk = sectionTotal <= SECTION_BUDGET_BYTES;

console.log(`\nкод:       ${codeTotal} Б из ${BUDGET_BYTES} Б gzip — ${codeOk ? 'OK' : 'ФЕЙЛ'}`);
console.log(`картинки:  ${sectionTotal} Б из ${SECTION_BUDGET_BYTES} Б gzip — ${sectionsOk ? 'OK' : 'ФЕЙЛ'}`);

if (!codeOk) {
  console.error('Бюджет кода превышен: см. START-HERE §5.4.');
  process.exit(1);
}
if (!sectionsOk) {
  console.error('Картинки разделов тяжелее потолка: пересожмите макеты в webp (scripts/embed_sections.mjs).');
  process.exit(1);
}