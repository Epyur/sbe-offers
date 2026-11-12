/**
 * Боевой листок (2026-11-13, шаг S6).
 *
 * Проверяется то, на чём менеджер останется без работы: кто правит, как
 * листок называется и не уходит ли поиск на бесконечную страницу.
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import {
  canEdit,
  hintOf,
  nextOffset,
  sheetLabelOf,
  sidesCount,
  type CombatPage,
  type CombatSheetDto,
} from '../src/combat.ts';

const sheet: CombatSheetDto = {
  id: 7,
  number: '12/2026',
  ownerEmail: 'manager@tn.ru',
  title: 'Боевой листок',
  ourNotes: [{ noteId: 7547, title: 'LOGICROOF V-RP' }],
  competitorNotes: [{ noteId: 9001, title: 'PLASTFOIL CLASSIC' }],
  formValues: { hint: 'акцент на сроке службы' },
  status: 'draft',
  createdAt: '2026-11-13T08:15:00Z',
  updatedAt: '2026-11-13T08:15:00Z',
};

test('правит автор и admin, чужой — нет (но читать может)', () => {
  assert.equal(canEdit(sheet, 'manager@tn.ru', 'editor'), true);
  assert.equal(canEdit(sheet, 'MANAGER@tn.ru', 'editor'), true);
  assert.equal(canEdit(sheet, 'other@tn.ru', 'editor'), false);
  assert.equal(canEdit(sheet, 'other@tn.ru', 'admin'), true);
  assert.equal(canEdit(sheet, '', 'editor'), false);
});

test('подпись — продукт, а не служебное слово', () => {
  assert.equal(sheetLabelOf(sheet), 'LOGICROOF V-RP');
  assert.equal(sheetLabelOf({ ...sheet, displayName: 'Своё имя' }), 'Своё имя');
  assert.equal(
    sheetLabelOf({ ...sheet, ourNotes: [], competitorNotes: [{ noteId: 1, title: 'Sikaplan' }] }),
    'Sikaplan',
  );
  assert.match(sheetLabelOf({ ...sheet, ourNotes: [], competitorNotes: [] }), /пустой листок/);
});

test('уточнение читается из form_values, его отсутствие — пустая строка', () => {
  assert.equal(hintOf(sheet), 'акцент на сроке службы');
  assert.equal(hintOf({ ...sheet, formValues: {} }), '');
});

test('счёт сторон', () => {
  assert.deepEqual(sidesCount(sheet), { our: 1, competitor: 1 });
});

test('следующая страница — только когда сервер сказал hasMore', () => {
  const page: CombatPage = { items: [{ noteId: 1, title: 'A' }], total: 40, offset: 0, hasMore: true };
  assert.equal(nextOffset(page), 1);
  assert.equal(nextOffset({ ...page, hasMore: false }), null);
  assert.equal(nextOffset({ ...page, items: [] }), null);
});