/*
 * Сторож правил окна КП в приложении (2026-11-15, шаг 4 конфигуратора окна).
 *
 * Проверяется чистая часть редактора (`src/ui/window-model.ts`) — то, что
 * уйдёт на сервер и то, что увидит менеджер. DOM и Obsidian здесь ни при чём,
 * поэтому файл гоняется обычным `node --test`, а не через плагин.
 *
 * Зачем отдельный сторож, если правила есть и на сервере: редактор — второй
 * рубеж. Он обязан отвергнуть негодное окно ДО запроса, иначе человек увидит
 * «не удалось сохранить форму» без указания, что именно не так. И он обязан
 * повторить два решения пользователя буквально: скрытое необязательно и
 * обязательность мягкая (никого не блокирует).
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  ZONE_TITLES,
  appendNewFields,
  defaultWindow,
  dropMissingFields,
  formFieldWindow,
  instructionFields,
  moveWindowItem,
  windowForEditor,
  windowForSave,
  windowPreview,
  windowProblem,
  windowSummary,
  windowTitle,
} from '../src/ui/window-model.ts';

/** Поля формы, как их отдаёт сервер. */
const placeholders = [
  { id: 'supplier_name', label: 'Поставщик' },
  { id: 'object_name', label: 'Объект' },
];

const zone = (id, visible = true, required = false) => ({ kind: 'zone', id, visible, required });
const field = (id, options = {}) => ({
  kind: 'field',
  id,
  visible: options.visible ?? true,
  required: options.required ?? false,
});

test('пустое окно = «как сейчас»: пять зон в прежнем порядке, затем поля', () => {
  const items = windowForEditor(undefined, placeholders);
  assert.deepEqual(
    items.map((i) => `${i.kind}:${i.id}`),
    [
      'zone:client',
      'zone:systems',
      'zone:items',
      'zone:materials',
      'zone:sections',
      'field:supplier_name',
      'field:object_name',
    ],
  );
  assert.ok(items.every((i) => i.visible === true));
  assert.ok(items.every((i) => i.required === false));
  assert.deepEqual(windowForEditor([], placeholders), items);
});

test('подпись зоны — из кода, подпись поля — из формы', () => {
  assert.equal(windowTitle(zone('items'), placeholders), 'Позиции с ценами');
  assert.equal(windowTitle(field('object_name'), placeholders), 'Объект');
  // Поле, которого уже нет в форме, всё равно называется: пустая строка
  // спрятала бы от администратора, ЧТО он настраивает.
  assert.equal(windowTitle({ kind: 'field', id: 'gone', label: 'Старое' }, placeholders), 'Старое');
  assert.equal(windowTitle({ kind: 'field', id: 'gone' }, placeholders), 'gone');
  // Неизвестная зона — свой id: её некому нарисовать, и это надо видеть.
  assert.equal(windowTitle(zone('unknown'), placeholders), 'unknown');
  // Бриф (0.11.2) — известная зона с подписью.
  assert.equal(windowTitle(zone('brief'), placeholders), 'Бриф');
  assert.deepEqual(Object.keys(ZONE_TITLES), ['client', 'systems', 'items', 'materials', 'sections', 'brief']);
});

// Сторож дефекта 0.11.1 «не вижу зоны бриф в настройках формы» (2026-11-16,
// совещание gemini-4-argon + claude-5-opus). Редактор рисовал ровно то, что
// пришло с сервера: если зоны не было в ответе, включить её было нечем.
test('недостающие зоны дописываются в редактор невидимыми', () => {
  // Живая payload 0.11.1: 5 зон + поля, брифа нет.
  const saved = [
    zone('client'),
    zone('systems'),
    zone('items'),
    zone('materials'),
    zone('sections'),
    field('supplier_name', { visible: true }),
  ];
  const items = windowForEditor(saved, placeholders);
  const brief = items.find((i) => i.kind === 'zone' && i.id === 'brief');
  assert.ok(brief, 'зона brief обязана быть в редакторе, иначе её не включить');
  assert.equal(brief.visible, false, 'дописанная зона невидима: раздел не появляется сам');
});

test('бриф встаёт сразу за Заказчиком, а не в конец', () => {
  const items = windowForEditor([zone('client'), field('object_name')], placeholders);
  const ids = items.map((i) => i.id);
  assert.equal(ids[ids.indexOf('client') + 1], 'brief', `порядок: ${ids.join(',')}`);
});

test('окно без зон вообще тоже получает все зоны редактора', () => {
  const items = windowForEditor([field('object_name')], placeholders);
  const zones = items.filter((i) => i.kind === 'zone').map((i) => i.id);
  // Клиент уже был в присланном окне, поэтому бриф встаёт сразу за ним —
  // та же логика якоря, что у сервера.
  assert.deepEqual(zones, ['client', 'brief', 'systems', 'items', 'materials', 'sections']);
});

test('порядок и видимость записанного сохраняются', () => {
  const items = windowForEditor([zone('sections'), zone('client', false), field('object_name')], placeholders);
  // Записанный порядок двух первых зон не изменился. Бриф (0.11.2) встаёт за
  // «Заказчиком» — это и есть якорь, так что третьим идёт он, а не поле.
  assert.deepEqual(
    items.map((i) => `${i.id}:${i.visible}`).slice(0, 3),
    ['sections:true', 'client:false', 'brief:false'],
  );
  // Поле формы никуда не делось, просто стоит после дописанных зон.
  assert.ok(items.some((i) => i.kind === 'field' && i.id === 'object_name' && i.visible === true));
});

test('проверка окна: неизвестная зона, поле-призрак, дубль', () => {
  assert.equal(windowProblem([zone('client')], placeholders), '');
  assert.match(windowProblem([zone('nope')], placeholders), /неизвестная зона/);
  assert.match(windowProblem([field('gone')], placeholders), /его нет среди полей формы/);
  assert.match(windowProblem([zone('client'), zone('client')], placeholders), /повторяется/);
  assert.equal(windowProblem([], placeholders), '');
});

test('проверка окна: обязательной бывает только зона «Заказчик»', () => {
  assert.equal(windowProblem([zone('client', true, true)], placeholders), '');
  assert.match(windowProblem([zone('systems', true, true)], placeholders), /не может быть обязательной/);
  // У поля обязательность законна.
  assert.equal(windowProblem([field('object_name', { required: true })], placeholders), '');
});

test('скрытое обязательное отвергается — решение пользователя', () => {
  // Дословно: «обязательное не может быть скрытым, если мы скрываем
  // обязательное, то пока оно скрыто, оно не обязательно».
  assert.match(
    windowProblem([field('object_name', { visible: false, required: true })], placeholders),
    /скрыта и обязательна/,
  );
  // А скрытое НЕобязательное — норма.
  assert.equal(windowProblem([field('object_name', { visible: false })], placeholders), '');
});

test('скрытое необязательное переживает сохранение, скрытое обязательное теряет галочку', () => {
  const saved = windowForSave([
    field('object_name', { visible: false, required: true }),
    zone('systems', false, false),
    zone('client', true, true),
  ]);
  assert.equal(saved[0]?.required, false);
  assert.equal(saved[0]?.visible, false);
  assert.equal(saved[2]?.required, true, 'видимое обязательное остаётся');
  // И после нормализации окно проходит проверку: человек не получает отказ
  // из-за состояния, которое редактор обязан был исправить сам.
  assert.equal(windowProblem(saved, placeholders), '');
});

test('предпросмотр: скрытое не показывается, обязательное помечено', () => {
  const lines = windowPreview(
    [zone('client', true, true), zone('systems', false), field('object_name', { required: true })],
    placeholders,
  );
  assert.deepEqual(lines, ['Заказчик — обязательное', 'Объект — обязательное']);
});

test('предпросмотр пустого экрана говорит об этом словами', () => {
  assert.deepEqual(windowPreview([zone('client', false)], placeholders), []);
});

test('сводка группы считает видимое и обязательное', () => {
  assert.equal(windowSummary([]), 'как сейчас');
  assert.equal(windowSummary([zone('client')]), 'видно 1');
  assert.equal(windowSummary([zone('client', true, true), zone('systems', false)]), 'видно 1, обязательных 1');
});

test('▲ и ▼ двигают, на краях ничего не делают', () => {
  const items = [zone('client'), zone('systems'), zone('items')];
  assert.equal(moveWindowItem(items, 1, -1), true);
  assert.deepEqual(items.map((i) => i.id), ['systems', 'client', 'items']);
  assert.equal(moveWindowItem(items, 0, -1), false);
  assert.equal(moveWindowItem(items, 2, 1), false);
  assert.equal(moveWindowItem(items, 9, 1), false);
  assert.deepEqual(items.map((i) => i.id), ['systems', 'client', 'items']);
});

test('поле, убранное из формы, уходит и из окна; зоны остаются', () => {
  const items = dropMissingFields(
    [zone('client'), field('supplier_name'), field('gone')],
    placeholders,
  );
  assert.deepEqual(items.map((i) => i.id), ['client', 'supplier_name']);
});

test('новое поле дописывается в конец видимым, существующие не дублируются', () => {
  const items = appendNewFields([zone('client'), field('supplier_name')], [
    { id: 'supplier_name', label: 'Поставщик' },
    { id: 'new_field', label: 'Новое' },
  ]);
  assert.deepEqual(items.map((i) => i.id), ['client', 'supplier_name', 'new_field']);
  assert.equal(items[2]?.visible, true);
  assert.equal(items[2]?.required, false);
  // Ничего не добавилось — порядок и содержимое те же.
  assert.deepEqual(
    appendNewFields([zone('client')], []).map((i) => i.id),
    ['client'],
  );
});

test('карточка КП в плагине: скрытое поле не показывается, обязательность из окна', () => {
  const explicit = formFieldWindow(
    [field('supplier_name', { visible: false }), field('object_name', { required: true })],
    placeholders,
    ['supplier_name'],
  );
  assert.deepEqual(explicit.placeholders.map((p) => p.id), ['object_name']);
  // Обязательность берётся из окна, а не из `requiredFields`: администратор
  // только что снял галочку с поля, и плагин не должен её помнить.
  assert.deepEqual([...explicit.required], ['object_name']);
});

test('карточка КП в плагине: пустое окно — прежнее поведение', () => {
  const legacy = formFieldWindow(undefined, placeholders, ['supplier_name']);
  assert.deepEqual(legacy.placeholders.map((p) => p.id), ['supplier_name', 'object_name']);
  assert.deepEqual([...legacy.required], ['supplier_name']);
});

test('карточка КП в плагине: скрытое обязательное не подсвечивается', () => {
  const res = formFieldWindow([field('object_name', { visible: false, required: true })], placeholders, []);
  assert.deepEqual(res.placeholders, []);
  assert.deepEqual([...res.required], []);
});

test('окно по умолчанию не знает обязательности — её задаёт администратор', () => {
  assert.ok(defaultWindow(placeholders).every((i) => i.required === false));
});

/* --- Шаг 5: бриф вместо промта в предпросмотре -------------------------- */

test('бриф: поля из инструкции называются подписями формы', () => {
  // `{{id}}` — всегда id; `[Подпись]` ищется по подписи и по id.
  const fields = instructionFields(
    'Опиши объект {{object_name}} для [Поставщик]. Адрес: [object_name].',
    placeholders,
  );
  assert.deepEqual(fields, ['Объект', 'Поставщик']);
});

test('бриф: повтор поля не дублируется, порядок — по встрече', () => {
  const fields = instructionFields('[Поставщик], затем [Объект], снова [Поставщик]', placeholders);
  assert.deepEqual(fields, ['Поставщик', 'Объект']);
});

test('бриф: незнакомое поле остаётся своим именем — опечатку видно', () => {
  // Молча проглотить нельзя: администратор ищет, почему в документе пусто.
  assert.deepEqual(instructionFields('Про [Несуществующее поле]', placeholders), ['Несуществующее поле']);
  assert.deepEqual(instructionFields('Про {{no_such}}', placeholders), ['no_such']);
});

test('бриф: инструкция без подстановок не даёт полей', () => {
  assert.deepEqual(instructionFields('Просто напиши вступление по данным КП.', placeholders), []);
  assert.deepEqual(instructionFields('', placeholders), []);
});