/**
 * Окно КП в приложении (2026-11-15, шаг 2 плана 0.11.0).
 *
 * Проверяется чистая часть: пустое окно = «как сейчас», скрытая зона не
 * возвращается, порядок записанного главный, незнакомая зона игнорируется.
 * Обязательность мягкая: `requiredOf` только называет пустое, ничего не
 * блокирует.
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import {
  DEFAULT_ZONES,
  loadOfferWindow,
  requiredOf,
  visibleZones,
  zoneOf,
  zonesForScreen,
  type OfferFormWindowDto,
  type WindowItemDto,
} from '../src/window.ts';

function zone(id: string, visible = true, required = false): WindowItemDto {
  return { kind: 'zone', id, visible, required };
}

function field(id: string, label: string, options: { visible?: boolean; required?: boolean } = {}): WindowItemDto {
  return { kind: 'field', id, label, visible: options.visible ?? true, required: options.required ?? false };
}

test('пустое окно — пять зон в нынешнем порядке', () => {
  assert.deepEqual(visibleZones(undefined), [...DEFAULT_ZONES]);
  assert.deepEqual(visibleZones(null), [...DEFAULT_ZONES]);
  assert.deepEqual(visibleZones({}), [...DEFAULT_ZONES]);
  assert.deepEqual(visibleZones({ window: [] }), [...DEFAULT_ZONES]);
  assert.deepEqual([...DEFAULT_ZONES], ['client', 'systems', 'items', 'materials', 'sections']);
});

test('скрытая зона не возвращается', () => {
  const form: OfferFormWindowDto = {
    window: [zone('client'), zone('systems', false), zone('items'), zone('materials'), zone('sections')],
  };
  assert.deepEqual(visibleZones(form), ['client', 'items', 'materials', 'sections']);
});

test('порядок записанного — главный', () => {
  const form: OfferFormWindowDto = {
    window: [zone('sections'), zone('items'), zone('client'), zone('materials'), zone('systems')],
  };
  assert.deepEqual(visibleZones(form), ['sections', 'items', 'client', 'materials', 'systems']);
});

test('незнакомая зона игнорируется', () => {
  const form: OfferFormWindowDto = {
    window: [zone('client'), zone('unknown_zone'), zone('items')],
  };
  assert.deepEqual(visibleZones(form), ['client', 'items']);
  assert.equal(zoneOf('unknown_zone'), null);
  assert.equal(zoneOf('client'), 'client');
});

test('поле формы зоной не становится', () => {
  const form: OfferFormWindowDto = {
    window: [zone('client'), field('object', 'Объект'), zone('systems')],
  };
  assert.deepEqual(visibleZones(form), ['client', 'systems']);
});

test('дубль зоны не рисуется дважды', () => {
  const form: OfferFormWindowDto = {
    window: [zone('client'), zone('client'), zone('systems')],
  };
  assert.deepEqual(visibleZones(form), ['client', 'systems']);
});

test('окно, где все зоны скрыты, — пустой экран, а не «как сейчас»', () => {
  const form: OfferFormWindowDto = {
    window: [zone('client', false), zone('systems', false), zone('items', false)],
  };
  assert.deepEqual(visibleZones(form), []);
});

test('окно, где одной зоны нет вовсе, не дописывает её', () => {
  // Сервер (`effectiveWindow`) дописывает недостающее скрытым — повторять
  // правило на клиенте значило бы держать две правды.
  const form: OfferFormWindowDto = { window: [zone('sections'), zone('client')] };
  assert.deepEqual(visibleZones(form), ['sections', 'client']);
});

test('ошибка чтения окна — все зоны в прежнем порядке', () => {
  assert.deepEqual(zonesForScreen(null), [...DEFAULT_ZONES]);
  assert.deepEqual(zonesForScreen(['items']), ['items']);
});

test('ошибка запроса окна не роняет экран', async () => {
  // Без токена и сети `apiRequest` отвергается — `loadOfferWindow` обязан
  // вернуть `null`, а не бросить: экран покажет все зоны (шаг 3).
  const loaded = await loadOfferWindow().catch(() => 'бросил');
  assert.notEqual(loaded, 'бросил');
  assert.ok(loaded === null || Array.isArray(loaded));
});

test('обязательность: пустое обязательное поле видно, заполненное — нет', () => {
  const form: OfferFormWindowDto = {
    window: [field('object', 'Объект', { required: true }), field('manager', 'Менеджер', { required: true })],
  };
  const missing = requiredOf(form, { client: { name: '', contact: '' }, formValues: { object: 'Школа' } });
  assert.deepEqual(missing, [{ id: 'manager', label: 'Менеджер' }]);
});

test('обязательность: скрытое не подсвечивается, даже если пусто', () => {
  const form: OfferFormWindowDto = {
    window: [field('object', 'Объект', { required: true, visible: false })],
  };
  assert.deepEqual(requiredOf(form, { client: { name: '', contact: '' }, formValues: {} }), []);
});

test('обязательность зоны заказчика — по паре имени и контакта', () => {
  const form: OfferFormWindowDto = { window: [zone('client', true, true)] };
  assert.deepEqual(requiredOf(form, { client: { name: '', contact: '' }, formValues: {} }), [
    { id: 'client', label: 'Заказчик' },
  ]);
  assert.deepEqual(requiredOf(form, { client: { name: 'ООО «Ромашка»', contact: '' }, formValues: {} }), []);
  assert.deepEqual(requiredOf(form, { client: { name: '', contact: '+7 900 000-00-00' }, formValues: {} }), []);
});

test('обязательность: зона не-заказчик обязательной не бывает', () => {
  const form: OfferFormWindowDto = { window: [zone('systems', true, true)] };
  assert.deepEqual(requiredOf(form, { client: { name: '', contact: '' }, formValues: {} }), []);
});

test('обязательность: пустое окно — ничего не подсвечивает', () => {
  assert.deepEqual(requiredOf(undefined, { client: { name: '', contact: '' }, formValues: {} }), []);
});

test('пустой список остаётся массивом, а не null', () => {
  assert.ok(Array.isArray(requiredOf({ window: [] }, { client: { name: '', contact: '' }, formValues: {} })));
  assert.ok(Array.isArray(visibleZones({ window: [zone('client', false)] })));
});