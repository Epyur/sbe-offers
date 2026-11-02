// Проверка, что документация для нового контекстного окна не разошлась с кодом.
//
// Правило проекта: HANDOFF.md и AGENTS.md читаются первыми, и устаревшая цифра
// в них стоит дороже — новое окно начинает работу по неверным данным (так и
// случилось 2026-11-01: HANDOFF показывал 0.4.0 после выпуска 0.4.1).
//
// Проверяется только то, что можно сверить с файлами, — никакой вольной
// интерпретации текста.
import fs from 'node:fs';
import crypto from 'node:crypto';

let bad = 0;
function check(name, ok, detail = '') {
  if (!ok) bad++;
  console.log((ok ? 'OK   ' : 'MISS ') + name + (ok || !detail ? '' : ' — ' + detail));
}

const plugin = JSON.parse(fs.readFileSync('manifest.json', 'utf8')).version;
const pkg = JSON.parse(fs.readFileSync('package.json', 'utf8')).version;
check('версия manifest = package', plugin === pkg, `${plugin} / ${pkg}`);

const handoff = fs.readFileSync('HANDOFF.md', 'utf8');
const agents = fs.readFileSync('AGENTS.md', 'utf8');

check('HANDOFF называет текущую версию', handoff.includes(`версия ${plugin}`), plugin);
check('AGENTS называет текущую версию', agents.includes(`= **${plugin}**`), plugin);

// Хэши сборки: документация должна нести те же суммы, что лежат на диске.
const hashes = {};
for (const f of ['manifest.json', 'main.js', 'styles.css']) {
  hashes[f] = crypto.createHash('sha256').update(fs.readFileSync(f)).digest('hex');
}
for (const f of ['manifest.json', 'main.js', 'styles.css']) {
  const short = hashes[f].slice(0, 8);
  check(`HANDOFF содержит хэш ${f}`, handoff.includes(short), short);
}

// Коммиты: обе ветки и их короткие хэши упоминаются в AGENTS.
const head = (await import('node:child_process')).execFileSync('git', ['rev-parse', '--short', 'HEAD']).toString().trim();
const mainHead = (await import('node:child_process'))
  .execFileSync('git', ['rev-parse', '--short', 'origin/main']).toString().trim();
check('AGENTS содержит HEAD backend', agents.includes(head), head);
check('AGENTS содержит HEAD main', agents.includes(mainHead), mainHead);

// Шапка HANDOFF не должна утверждать, что текущая версия уже выложена:
// версия на сервере меняется отдельной командой, и её сдвиг сюда попадёт
// только вместе с записью о выкладке.
check('HANDOFF не выдаёт текущую версию за развёрнутую',
  !handoff.slice(0, 1200).includes(`выпуск ${plugin} — развёрнут`), plugin);

process.exit(bad === 0 ? 0 : 1);