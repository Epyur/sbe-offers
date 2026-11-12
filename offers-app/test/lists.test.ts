/**
 * Списки и поиск (2026-11-13, шаг S2).
 *
 * Проверяется то, о чём будут жаловаться люди: адрес без `q` не меняется,
 * подпись строки — заказчик/продукт, а не служебное слово, и пустой список
 * различает «не нашлось» и «ещё нет».
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import {
  emptyText,
  listPath,
  numberedLabel,
  offerLabel,
  searchPlaceholder,
  sheetLabel,
  shortDate,
  statusLabel,
  type OfferRow,
  type SheetRow,
} from '../src/lists.ts';

test('без поиска адрес прежний — ни q, ни хвоста', () => {
  assert.equal(listPath('offers', ''), '/offers');
  assert.equal(listPath('offers', '   '), '/offers');
  assert.equal(listPath('sheets', ''), '/combat');
});

test('поиск уходит в адрес параметром q, регистр и кириллица не ломают его', () => {
  assert.equal(listPath('offers', 'Строй Монтаж'), '/offers?q=%D0%A1%D1%82%D1%80%D0%BE%D0%B9%20%D0%9C%D0%BE%D0%BD%D1%82%D0%B0%D0%B6');
  assert.equal(listPath('sheets', 'мембрана'), '/combat?q=%D0%BC%D0%B5%D0%BC%D0%B1%D1%80%D0%B0%D0%BD%D0%B0');
  assert.equal(listPath('sheets', '%'), '/combat?q=%25');
});

const offer: OfferRow = {
  id: 1,
  number: '003/2026',
  ownerEmail: 'manager@tn.ru',
  client: { name: 'СтройМонтажСервис', contact: 'Пётр' },
  total: 0,
  status: 'draft',
  createdAt: '2026-11-12T19:05:00.123456Z',
};

test('подпись КП — заказчик, а при пустом имени — контакт', () => {
  assert.equal(offerLabel(offer), 'СтройМонтажСервис');
  assert.equal(offerLabel({ ...offer, client: { name: '  ', contact: 'Пётр' } }), 'Пётр');
  assert.equal(offerLabel({ ...offer, client: { name: '', contact: '' } }), 'без заказчика');
});

const sheet: SheetRow = {
  id: 7,
  number: '12/2026',
  ownerEmail: 'manager@tn.ru',
  title: 'Боевой листок',
  ourNotes: [{ noteId: 7547, title: 'LOGICROOF V-RP' }],
  competitorNotes: [{ noteId: 9001, title: 'PLASTFOIL CLASSIC' }],
  status: 'final',
  createdAt: '2026-11-13T08:15:00Z',
};

test('подпись листка — продукт, а не служебное «Боевой листок»', () => {
  assert.equal(sheetLabel(sheet), 'LOGICROOF V-RP');
  assert.equal(sheetLabel({ ...sheet, displayName: 'Наш выбор' }), 'Наш выбор');
  assert.equal(
    sheetLabel({ ...sheet, ourNotes: [], competitorNotes: [{ noteId: 1, title: 'Sikaplan' }] }),
    'Sikaplan',
  );
  assert.equal(sheetLabel({ ...sheet, ourNotes: [], competitorNotes: [] }), 'пустой листок № 12/2026');
});

test('номер показывается так, как его знает менеджер', () => {
  assert.equal(numberedLabel('offers', '003/2026'), 'КП 003/2026');
  assert.equal(numberedLabel('sheets', '12/2026'), 'БЛ 12/2026');
});

test('дата из RFC3339 без падений, состояние — словом', () => {
  assert.equal(shortDate('2026-11-12T19:05:00.123Z'), '12.11.2026');
  assert.equal(shortDate('мусор'), 'мусор');
  assert.equal(statusLabel('draft'), 'черновик');
  assert.equal(statusLabel('final'), 'готово');
});

test('пустой список различает «не нашлось» и «ещё нет»', () => {
  assert.match(emptyText('offers', 'Строй'), /не нашлось/i);
  assert.match(emptyText('offers', ''), /появятся/i);
  assert.match(emptyText('sheets', 'мембрана'), /не нашлось/i);
  assert.notEqual(emptyText('sheets', ''), emptyText('sheets', 'мембрана'));
});

test('подсказка поиска у каждого списка своя', () => {
  assert.match(searchPlaceholder('offers'), /Заказчик/i);
  assert.match(searchPlaceholder('sheets'), /продукт/i);
});