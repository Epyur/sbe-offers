/**
 * Проверка поиска заметки в вольте (2026-10-30).
 *
 * Запуск: node scripts/check_note_path.mjs
 * Собирает src/utils/note-path.ts через esbuild и гоняет правила. Раннера
 * тестов в плагине нет (Obsidian-плагины его не заводят), поэтому проверка
 * живёт отдельным скриптом — тот же приём, что у check_*_openapi.mjs.
 */
import { build } from 'esbuild';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const here = path.dirname(fileURLToPath(import.meta.url));
const entry = path.join(here, '..', 'src', 'utils', 'note-path.ts');

const out = await build({
  entryPoints: [entry],
  bundle: true,
  format: 'esm',
  write: false,
  platform: 'node',
});
const code = out.outputFiles[0].text;
const mod = await import(`data:text/javascript;base64,${Buffer.from(code).toString('base64')}`);

let passed = 0;
let failed = 0;

function ok(name, condition, detail = '') {
  if (condition) {
    passed += 1;
    console.log(`  ✓ ${name}`);
  } else {
    failed += 1;
    console.log(`  ✗ ${name}${detail ? ` — ${detail}` : ''}`);
  }
}

const VAULT = [
  'База службы/Write it/ТН-СТИЛОБАТ ЭКСПЕРТ ТРОТУАР.md',
  'База службы/Write it/ТН-СТИЛОБАТ КЛАССИК ТРОТУАР.md',
  'docs/протокол.md',
  'Заметки/Дубль.md',
  'Другая/Дубль.md',
];

const KB_PATH = 'Инженерная гидроизоляция/Системы/ТН-СТИЛОБАТ ЭКСПЕРТ ТРОТУАР.md';

console.log('\n1. Точное совпадение пути');
ok('совпал — открываем его', mod.resolveNotePath(['А/Б.md', KB_PATH], KB_PATH) === KB_PATH);

console.log('\n2. Живой случай: логический путь КБ не совпадает с вольтом');
ok(
  'нашли по названию в другой папке',
  mod.resolveNotePath(VAULT, KB_PATH) === 'База службы/Write it/ТН-СТИЛОБАТ ЭКСПЕРТ ТРОТУАР.md',
  mod.resolveNotePath(VAULT, KB_PATH),
);

console.log('\n3. Различия в написании названия');
ok(
  'ё и е не мешают',
  mod.resolveNotePath(['X/Елка.md'], 'Системы/Ёлка.md') === 'X/Елка.md',
);
ok(
  'регистр не мешает',
  mod.resolveNotePath(['X/тн-стилобат эксперт тротуар.md'], KB_PATH) === 'X/тн-стилобат эксперт тротуар.md',
);
ok(
  'кавычки не мешают',
  mod.resolveNotePath(['X/ЖК Поэма.md'], 'Системы/ЖК «Поэма».md') === 'X/ЖК Поэма.md',
);
ok(
  'лишние пробелы не мешают',
  mod.resolveNotePath(['X/ТН  Стилобат.md'], 'Системы/ТН Стилобат.md') === 'X/ТН  Стилобат.md',
);

console.log('\n4. Неоднозначность — не угадываем');
ok(
  'две заметки с одним названием — пустая строка',
  mod.resolveNotePath(VAULT, 'Системы/Дубль.md') === '',
);
ok(
  'но совпадение по хвосту пути разрешает выбор',
  mod.resolveNotePath(['А/Системы/Дубль.md', 'Б/Дубль.md'], 'Системы/Дубль.md') === 'А/Системы/Дубль.md',
);

console.log('\n5. Не найдено и пустые входы');
ok('ничего не нашли — пустая строка', mod.resolveNotePath(VAULT, 'Системы/Нет такой.md') === '');
ok('пустой логический путь — пустая строка', mod.resolveNotePath(VAULT, '') === '');
ok('пустой список вольта — пустая строка', mod.resolveNotePath([], KB_PATH) === '');

console.log('\n6. Текст ошибки говорит о названии, а не о пути');
const msg = mod.noteNotFoundMessage(KB_PATH);
ok('в сообщении есть название заметки', msg.includes('ТН-СТИЛОБАТ ЭКСПЕРТ ТРОТУАР'), msg);
ok('в сообщении нет логической папки КБ', !msg.includes('Инженерная гидроизоляция'), msg);
ok('пустой путь — понятный текст', mod.noteNotFoundMessage('') === 'Заметка не найдена в хранилище.');

console.log(`\nИтого: ${passed} прошло, ${failed} провалилось`);
process.exit(failed === 0 ? 0 : 1);
