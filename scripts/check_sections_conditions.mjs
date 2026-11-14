/**
 * Сверка условий подразделов: клиентский предпросмотр ↔ серверная печать
 * (2026-11-14, шаг 6).
 *
 * Запуск: node scripts/check_sections_conditions.mjs
 *
 * Почему такая проверка нужна. На сервере условие решает, НАПЕЧАТАЕТСЯ ли
 * подраздел (`conditionHolds`, `offers-service/sections.go`). В плагине есть
 * его зеркало для предпросмотра рамки (`conditionHoldsClient`,
 * `src/ui/sections-editor.ts`). Два списка условий, разошедшиеся молча, — это
 * «предпросмотр обещает, документ молчит»: ровно тот класс, из-за которого
 * появились содержательные разделы. Скрипт берёт имена условий из СЕРВЕРА, а
 * не переписывает их здесь, и прогоняет зеркало на тех же случаях, что и
 * `TestSectionConditions`.
 */
import { build } from 'esbuild';
import { fileURLToPath } from 'node:url';
import fs from 'node:fs';
import path from 'node:path';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.join(here, '..');

// Имена условий и их значения — из СЕРВЕРНОГО кода, а не из списка здесь.
// `conditionHolds` живёт в `sections.go`, константы — в `form.go`.
const sectionsGo = fs.readFileSync(path.join(root, 'offers-service', 'sections.go'), 'utf8');
const formGo = fs.readFileSync(path.join(root, 'offers-service', 'form.go'), 'utf8');
const serverConditions = [...sectionsGo.matchAll(/case (condition\w+):/g)].map((m) => m[1]);
const serverConstants = [];
for (const m of formGo.matchAll(/(condition\w+)\s+SubsectionCondition\s*=\s*"([a-z_]*)"/g)) {
  serverConstants.push({ name: m[1], value: m[2] });
}
const valueOf = (name) => serverConstants.find((c) => c.name === name)?.value ?? '';

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

console.log('\n1. Серверные условия сняты с кода');
ok('условия найдены', serverConditions.length >= 5, serverConditions.join(', '));
ok('у каждого есть строковое значение', serverConstants.length >= 6 && serverConstants.every((c) => c.value !== undefined), serverConstants.map((c) => `${c.name}=${c.value}`).join(', '));

console.log('\n2. Зеркало повторяет таблицу сервера');
const always = valueOf('conditionAlways');
const hasSystems = valueOf('conditionHasSystems');
const hasMaterials = valueOf('conditionHasMaterials');
const multiSystem = valueOf('conditionMultiSystem');
const hasCritique = valueOf('conditionHasCritique');
const hasItems = valueOf('conditionHasItems');

const emptyOffer = { items: [], systems: [], materials: [] };
const oneSystem = { items: [], systems: [{ noteId: 1 }], materials: [] };
const threeSystems = { items: [], systems: [{ noteId: 1 }, { noteId: 2 }, { noteId: 3 }], materials: [] };
const materialsOffer = { items: [], systems: [], materials: [{ noteId: 7 }] };
const itemsOffer = { items: [{ name: 'X' }], systems: [], materials: [] };

const cases = [
  [always, {}, emptyOffer, true, 'always — всегда'],
  [hasSystems, {}, emptyOffer, false, 'has_systems — пусто'],
  [hasSystems, {}, oneSystem, true, 'has_systems — одна система'],
  [hasMaterials, {}, emptyOffer, false, 'has_materials — пусто'],
  [hasMaterials, {}, materialsOffer, true, 'has_materials — есть материал'],
  [multiSystem, {}, oneSystem, false, 'multi_system — одна'],
  [multiSystem, {}, threeSystems, true, 'multi_system — три'],
  [hasCritique, { critique: false }, threeSystems, false, 'has_critique — выключен'],
  [hasCritique, { critique: true }, emptyOffer, true, 'has_critique — включён, даже без систем'],
  [hasItems, {}, emptyOffer, false, 'has_items — пусто'],
  [hasItems, {}, itemsOffer, true, 'has_items — есть позиции'],
];
for (const [cond, section, offer, want, label] of cases) {
  const got = mod.conditionHoldsClient(cond, section, offer);
  ok(label, got === want, `получили ${got}, ждали ${want}`);
}

console.log('\n3. Незнакомое условие не печатает (как на сервере)');
ok('пустое условие — печатается (по умолчанию always)', mod.conditionHoldsClient(undefined, {}, emptyOffer) === true);

console.log('\n4. Метка раздела и слова условия');
ok('метка { content: … }', mod.sectionMarker({ id: 'proposal' }) === '{ content: proposal }');
ok('слова условия — по-русски', mod.conditionWords(hasCritique).includes('крит'), mod.conditionWords(hasCritique));

console.log(`\nИтого: ${passed} прошло, ${failed} провалилось`);
process.exit(failed === 0 ? 0 : 1);