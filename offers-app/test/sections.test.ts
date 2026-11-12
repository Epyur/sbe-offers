/**
 * Тексты разделов (2026-11-13, шаг S4).
 *
 * Главное, что проверяется, — разбор ключа: по нему строится адрес ручки.
 * Ошибка здесь означает «нажал „Написать“ и получил чужой раздел».
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import { filledCount, sectionAddress, withCurrent, type OfferSectionDto } from '../src/sections.ts';

test('фиксированные секции разбираются без номера', () => {
  assert.deepEqual(sectionAddress('copy.intro'), { kind: 'intro', systemIndex: 0 });
  assert.deepEqual(sectionAddress('copy.advantages'), { kind: 'advantages', systemIndex: 0 });
  assert.deepEqual(sectionAddress('copy.conditions'), { kind: 'conditions', systemIndex: 0 });
});

test('описание системы несёт номер', () => {
  assert.deepEqual(sectionAddress('copy.system.1'), { kind: 'system', systemIndex: 1 });
  assert.deepEqual(sectionAddress('copy.system.12'), { kind: 'system', systemIndex: 12 });
});

test('мусорный ключ не превращается в адрес', () => {
  assert.equal(sectionAddress('copy.system.0'), null);
  assert.equal(sectionAddress('copy.system.abc'), null);
  assert.equal(sectionAddress('copy.иное'), null);
  assert.equal(sectionAddress(''), null);
});

const sections: OfferSectionDto[] = [
  { key: 'copy.intro', label: 'Вводный текст', text: '', warnings: [] },
  { key: 'copy.system.1', label: 'Описание системы 1', text: 'Есть', warnings: [] },
  { key: 'copy.conditions', label: 'Условия', text: '  ', warnings: [] },
];

test('слот с пустым текстом остаётся на месте и не считается заполненным', () => {
  assert.equal(filledCount(sections), 1);
});

test('свежий текст заменяет слот, порядок не меняется', () => {
  const updated = withCurrent(sections, {
    key: 'copy.intro',
    label: 'Вводный текст',
    text: 'Готовый текст',
    warnings: ['проверьте цифры'],
  });
  assert.deepEqual(
    updated.map((s) => s.key),
    ['copy.intro', 'copy.system.1', 'copy.conditions'],
  );
  assert.equal(updated[0]?.text, 'Готовый текст');
  assert.deepEqual(updated[0]?.warnings, ['проверьте цифры']);
  assert.equal(updated[1]?.text, 'Есть');
});