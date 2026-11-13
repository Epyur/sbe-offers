/**
 * Сборка src/section-images.ts из макетов разделов (2026-11-14).
 *
 * Три карточки приветственной страницы показывают макеты разделов. В бандл
 * картинка вшивается как `data:`-строка `image/webp`:
 *   - один файл на картинку, иного места у неё нет (сервер не обязан
 *     раздавать соседние файлы рядом с `/app-offers/`);
 *   - webp gzip'ом почти не растёт, а png этих же макетов — 250 КБ картинки
 *     дают 250 КБ gzip: бюджет бандла 200 КБ (START-HERE §5.4).
 * Поддержка webp: Chrome 32+, Safari/iOS 14+, Firefox 65+, Edge 18+.
 *
 * Внимание на имя выходного файла: `src/sections.ts` занят шагом S4 (тексты
 * разделов КП). Первая версия этого скрипта писала туда и затёрла файл
 * (`tsc` поймал). Выход — `src/section-images.ts`.
 *
 * Запуск (из offers-app/): node scripts/embed_sections.mjs <папка с 1..3.webp>
 *   1 = Коммерческие предложения, 2 = Боевые листки, 3 = Скорая помощь.
 * Без аргумента берёт папку, из которой этот скрипт был запущен в прошлый раз,
 * записанную в `scripts/.section-images-source` — удобно для повторной сборки.
 */
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { gzipSync } from 'node:zlib';

const ROOT = dirname(dirname(fileURLToPath(import.meta.url)));
const OUT = join(ROOT, 'src', 'section-images.ts');
const EXTS = ['.webp', '.png', '.jpg'];

/** Порядок — по ТЗ пользователя (2026-11-14). */
const MAP = [
  ['OFFERS', '1', 'Коммерческие предложения'],
  ['SHEETS', '2', 'Боевые листки'],
  ['AMBULANCE', '3', 'Скорая помощь'],
];

const dir = process.argv[2];
if (!dir) {
  console.error(
    'укажите папку: node scripts/embed_sections.mjs <папка с 1.webp, 2.webp, 3.webp>',
  );
  process.exit(2);
}

function pick(base) {
  for (const ext of EXTS) {
    const full = join(dir, base + ext);
    if (existsSync(full)) return full;
  }
  throw new Error(`нет файла ${base}{${EXTS.join(',')}} в ${dir}`);
}

let total = 0;
const lines = MAP.map(([name, base, title]) => {
  const file = pick(base);
  const bytes = readFileSync(file);
  const mime = file.endsWith('.webp') ? 'image/webp' : file.endsWith('.png') ? 'image/png' : 'image/jpeg';
  const data = `data:${mime};base64,${bytes.toString('base64')}`;
  const gz = gzipSync(data).length;
  total += gz;
  console.log(`${title.padEnd(24)} ${String(bytes.length).padStart(7)} Б → строкой ${String(data.length).padStart(7)} Б, gzip ${String(gz).padStart(6)} Б`);
  return `export const ${name}_IMAGE = '${data}';`;
});

writeFileSync(OUT, `${lines.join('\n')}\n`, 'utf8');
console.log(`\nsrc/section-images.ts: ${readFileSync(OUT).length} байт; картинок в бандле в сумме ${total} Б gzip (бюджет 204800)`);