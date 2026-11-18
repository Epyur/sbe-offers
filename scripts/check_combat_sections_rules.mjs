/**
 * Разделы боевого листка в настройках (2026-11-18).
 *
 * Запуск: node scripts/check_combat_sections_rules.mjs
 *
 * Что здесь сторожится и почему именно так.
 *
 * 1. Один и тот же отказ на пустое сохранение живёт в двух местах: на сервере
 *    (`combatEmptySectionsProblem`, `offers-service/form.go`) и в плагине
 *    (`COMBAT_SECTIONS_EMPTY`, `src/ui/sections-model.ts`). Тексты сверяются
 *    здесь, а не «на глаз»: разойтись им нельзя — человек увидит один отказ до
 *    запроса и другой после, и решит, что дело в разных вещах.
 * 2. Клиентское правило обязано быть вызвано на пути сохранения. Функция,
 *    которую никто не зовёт, зеленеет в тесте и не защищает ничего (этот класс
 *    уже стоил времени в боте: `void someFunction;`).
 * 3. id раздела в режиме листка не правится, и редактор получает `combat: true`.
 *    Иначе переименование id разошлось бы с ключом кэша `gen.sect.*`, и листок
 *    заново звал бы модель на каждый раздел.
 * 4. «↩ Вернуть заготовку» ставит КОПИЮ заготовки, а не саму заготовку с сервера:
 *    редактор правит разделы на месте, и вторая правка испортила бы заготовку.
 */
import { build } from 'esbuild';
import { fileURLToPath } from 'node:url';
import fs from 'node:fs';
import path from 'node:path';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.join(here, '..');

const formGo = fs.readFileSync(path.join(root, 'offers-service', 'form.go'), 'utf8');
const editorTs = fs.readFileSync(path.join(root, 'src', 'ui', 'sections-editor.ts'), 'utf8');
const settingsTs = fs.readFileSync(path.join(root, 'src', 'ui', 'combat-settings.ts'), 'utf8');

const entry = path.join(root, 'src', 'ui', 'sections-model.ts');
const out = await build({ entryPoints: [entry], bundle: true, format: 'esm', write: false, platform: 'node' });
const mod = await import(`data:text/javascript;base64,${Buffer.from(out.outputFiles[0].text).toString('base64')}`);

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

/** Тело функции по имени: от объявления до закрывающей скобки на её отступе. */
function functionBody(src, marker) {
  const at = src.indexOf(marker);
  if (at < 0) return '';
  const open = src.indexOf('{', at);
  let depth = 0;
  for (let i = open; i < src.length; i += 1) {
    if (src[i] === '{') depth += 1;
    else if (src[i] === '}') {
      depth -= 1;
      if (depth === 0) return src.slice(open, i + 1);
    }
  }
  return '';
}

/** Текст Go-константы по имени (двойные кавычки, как в коде). */
function goConst(src, name) {
  const m = src.match(new RegExp(`${name}\\s*=\\s*"([^"]*)"`));
  return m ? m[1] : '';
}

/** Текст TS-константы по имени (одинарная кавычка или шаблон). */
function tsConst(src, name) {
  const m = src.match(new RegExp(`${name}\\s*=\\s*\\n?\\s*'([^']*)'`));
  return m ? m[1] : '';
}

console.log('\n1. Отказ на пустое сохранение — один текст на две стороны');
const serverText = goConst(formGo, 'combatEmptySectionsProblem');
ok('на сервере отказ объявлен', serverText !== '', 'не нашёл combatEmptySectionsProblem в form.go');
ok('клиентский текст совпадает с серверным', mod.COMBAT_SECTIONS_EMPTY === serverText, `клиент: ${mod.COMBAT_SECTIONS_EMPTY}`);
ok('отказ называет выход («Вернуть заготовку»)', mod.COMBAT_SECTIONS_EMPTY.includes('Вернуть заготовку'));
ok('сервер проверяет пустые разделы ТОЛЬКО у листка',
  formGo.includes('f.Mode == combatFormMode && len(f.Sections) == 0'));
ok('сервер возвращает именно этот отказ', formGo.includes('return combatEmptySectionsProblem'));

console.log('\n2. Клиентское правило');
ok('пустой список — проблема', mod.combatSectionsProblem([]) === mod.COMBAT_SECTIONS_EMPTY);
ok('нет поля (старый ответ сервера) — проблема', mod.combatSectionsProblem(undefined) === mod.COMBAT_SECTIONS_EMPTY);
ok('один раздел — можно сохранять', mod.combatSectionsProblem([{ id: 'general', title: 'Общее', subsections: [] }]) === '');

console.log('\n3. Заготовка подставляется копией');
const serverDefault = [
  { id: 'general', title: 'Общее', numbered: true, subsections: [{ id: 'overview', title: 'Коротко', instruction: 'Сравни' }] },
];
const copy = mod.cloneSections(serverDefault);
ok('копия глубокая по разделам', copy[0] !== serverDefault[0] && copy[0].id === 'general');
ok('копия глубокая по подразделам', copy[0].subsections[0] !== serverDefault[0].subsections[0]);
copy[0].title = 'Правка';
copy[0].subsections[0].instruction = 'Другой текст';
ok('правка копии не портит заготовку',
  serverDefault[0].title === 'Общее' && serverDefault[0].subsections[0].instruction === 'Сравни');
ok('пустая заготовка — пустой массив', mod.cloneSections(undefined).length === 0);

console.log('\n4. Окно настроек листка действительно проверяет и возвращает');
ok('проверка вызвана на пути сохранения', settingsTs.includes('combatSectionsProblem(sections)'));
ok('редактор разделов открыт в режиме листка', settingsTs.includes('combat: true'));
ok('кнопка «Вернуть заготовку» есть', settingsTs.includes('↩ Вернуть заготовку'));
ok('возврат идёт через копию заготовки', /cloneSections\(/.test(settingsTs) && settingsTs.includes('sectionsDefault'));

console.log('\n5. id раздела в режиме листка не правится');
const identity = functionBody(editorTs, 'function renderIdentityRow(');
const combatBranch = identity.indexOf('if (deps.combat)');
const idInput = identity.indexOf("row.createEl('input'");
ok('в строке id есть ветка листка', combatBranch >= 0);
ok('ветка листка стоит ДО поля ввода', combatBranch >= 0 && idInput > combatBranch);
ok('ветка листка выходит из функции до создания поля',
  combatBranch >= 0 && idInput > combatBranch && /return;/.test(identity.slice(combatBranch, idInput)));
ok('чип метки { content: … } у листка не показывается', editorTs.includes('if (deps.combat) return;'));

console.log(`\nИтого: ${passed} прошло, ${failed} провалилось`);
process.exit(failed === 0 ? 0 : 1);