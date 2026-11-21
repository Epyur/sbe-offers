/**
 * Карточка КП (2026-11-13, шаг S3).
 *
 * Проверяется расчётная часть — то, из-за чего менеджер позвонит первым:
 * суммы строк и итога, разбор «1,5» и проверки перед отправкой.
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import {
  chosenMaterials,
  chosenSystems,
  clientChanged,
  draftToItem,
  draftTotal,
  EMPTY_ITEM,
  itemDraftOf,
  itemsChanged,
  itemsProblem,
  money,
  numberText,
  parseNumber,
  readWindowSave,
  rowSum,
  systemKey,
  type ItemDraft,
  type OfferDto,
  type SystemCardDto,
} from '../src/offer.ts';

test('запятая — десятичный разделитель, пробелы не мешают', () => {
  assert.equal(parseNumber('1,5'), 1.5);
  assert.equal(parseNumber(' 1 000,25 '), 1000.25);
  assert.equal(parseNumber(''), 0);
  assert.ok(Number.isNaN(parseNumber('abc')));
});

test('число в поле не теряет значение при выводе', () => {
  assert.equal(numberText(1.5), '1,5');
  assert.equal(numberText(3), '3');
  assert.equal(parseNumber(numberText(0.1 + 0.2)), 0.3);
});

test('сумма строки и итог считаются до копеек', () => {
  const items: ItemDraft[] = [
    { ...EMPTY_ITEM, name: 'Система', qty: '3', price: '999,99' },
    { ...EMPTY_ITEM, name: 'Работа', qty: '1', price: '0,1' },
  ];
  assert.equal(rowSum(items[0]!), 2999.97);
  assert.equal(draftTotal(items), 3000.07);
});

test('мусор в количестве не превращается в деньги', () => {
  assert.equal(rowSum({ ...EMPTY_ITEM, name: 'X', qty: 'два', price: '10' }), 0);
});

test('деньги печатаются по-русски', () => {
  assert.equal(money(0), '0,00');
  assert.equal(money(3000.07), '3 000,07');
  assert.equal(money(1234567.5), '1 234 567,50');
});

test('проверка перед отправкой объясняет строку', () => {
  assert.equal(itemsProblem([]), '');
  assert.match(itemsProblem([{ ...EMPTY_ITEM, name: ' ' }]), /Строка 1: не заполнено/);
  assert.match(itemsProblem([{ ...EMPTY_ITEM, name: 'X', qty: '0' }]), /больше нуля/);
  assert.match(itemsProblem([{ ...EMPTY_ITEM, name: 'X', qty: '1', price: '-1' }]), /отрицательной/);
  assert.equal(itemsProblem([{ ...EMPTY_ITEM, name: 'X', qty: '1', price: '10' }]), '');
});

test('правки сравниваются с сохранённым, а не со «всё равно сохраняю»', () => {
  const saved = [
    { name: 'Плита', unit: 'м²', qty: 2, price: 100, sum: 200 },
  ];
  const same = [itemDraftOf(saved[0]!)];
  assert.equal(itemsChanged(same, saved), false);
  assert.equal(itemsChanged([{ ...same[0]!, qty: '3' }], saved), true);
  assert.equal(itemsChanged([...same, { ...EMPTY_ITEM }], saved), true);
  assert.equal(clientChanged({ name: 'А', contact: '' }, { name: 'А', contact: '' }), false);
  assert.equal(clientChanged({ name: 'Б', contact: '' }, { name: 'А', contact: '' }), true);
});

test('черновик уходит на сервер без NaN и с обрезанными полями', () => {
  const dto = draftToItem({ ...EMPTY_ITEM, name: '  Плита ', qty: '2', price: '1,5', ekn: ' ' });
  assert.deepEqual(dto, { catalogId: undefined, ekn: undefined, name: 'Плита', unit: 'шт', qty: 2, price: 1.5 });
});

const offer: OfferDto = {
  id: 1,
  number: '003/2026',
  ownerEmail: 'manager@tn.ru',
  client: { name: 'СтройМонтажСервис', contact: '' },
  items: [],
  formValues: {},
  systems: [{ sku: 'TN-ROOF-1', noteId: 7547 }, { noteId: 6363 }],
  total: 0,
  status: 'draft',
  createdAt: '2026-11-13T09:00:00Z',
  updatedAt: '2026-11-13T09:00:00Z',
};

const catalog: SystemCardDto[] = [
  { noteId: 7547, sku: 'tn-roof-1', title: 'ТН-КРОВЛЯ Гарант' },
  { noteId: 6363, title: 'ТН-Титан PIR' },
];

test('выбранные системы называются по карточке, ключ не зависит от регистра', () => {
  assert.equal(systemKey({ sku: ' tn-roof-1 ' }), 'sku:TN-ROOF-1');
  assert.equal(systemKey({ noteId: 5 }), 'note:5');
  const chosen = chosenSystems(offer, catalog);
  assert.deepEqual(
    chosen.map((c) => c.title),
    ['ТН-КРОВЛЯ Гарант', 'ТН-Титан PIR'],
  );
});

test('система без карточки в справочнике всё равно видна', () => {
  const chosen = chosenSystems({ ...offer, systems: [{ sku: 'OLD-1', noteId: 9 }] }, catalog);
  assert.equal(chosen[0]?.title, 'OLD-1');
});

test('выбранные материалы показываются отдельно от систем', () => {
  const withMaterials = {
    ...offer,
    systems: [{ noteId: 7547 }],
    materials: [{ sku: 'MAT-КРОВ-ПВХ-0001', noteId: 100 }],
  };
  const systems = chosenSystems(withMaterials, catalog);
  const materials = chosenMaterials(withMaterials);
  assert.equal(systems.length, 1);
  assert.equal(materials.length, 1);
  assert.equal(materials[0]?.sku, 'MAT-КРОВ-ПВХ-0001');
  assert.equal(chosenMaterials({ ...offer, materials: undefined }).length, 0);
});

test('выбранный материал подписан НАЗВАНИЕМ из справочника, а не кодом', () => {
  // Живая жалоба 2026-11-21: в карточке КП после выбора материала показывался
  // `MAT-…`. Сервер хранит в `material_choice` только код и заметку; имя
  // приходит справочником, который теперь грузится вместе с карточкой.
  const materialCatalog: SystemCardDto[] = [
    { noteId: 100, sku: 'MAT-КРОВ-ПВХ-0001', title: 'Кровельная ПВХ-мембрана ECOPLAST' },
  ];
  const withMaterials = { ...offer, materials: [{ sku: 'MAT-КРОВ-ПВХ-0001', noteId: 100 }] };
  assert.equal(chosenMaterials(withMaterials, materialCatalog)[0]?.title, 'Кровельная ПВХ-мембрана ECOPLAST');
  // Справочник не прочитался — остаётся код: пустая строка была бы хуже.
  assert.equal(chosenMaterials(withMaterials)[0]?.title, 'MAT-КРОВ-ПВХ-0001');
});

test('ответ сохранения читается в новой форме — документ и пустые обязательные', () => {
  const res = readWindowSave({
    offer: { ...offer, number: '007/2026' },
    missingRequired: [{ id: 'object_name', label: 'Объект' }],
  });
  assert.equal(res.offer.number, '007/2026');
  assert.deepEqual(res.missingRequired, [{ id: 'object_name', label: 'Объект' }]);
});

test('ответ сохранения без missingRequired — пустой список, а не undefined', () => {
  const res = readWindowSave({ offer });
  assert.deepEqual(res.missingRequired, []);
});

test('старый ответ (голый Offer) не превращает экран в пустую карточку', () => {
  // Сервер прежней сборки (до 0.11.0) отвечал самим документом.
  const res = readWindowSave(offer);
  assert.equal(res.offer.number, '003/2026');
  assert.equal(res.missingRequired, undefined);
  assert.equal(readWindowSave(null).offer, null);
});