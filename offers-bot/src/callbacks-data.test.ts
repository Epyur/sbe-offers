/**
 * Разбор `callback_data` (2026-11-13).
 *
 * Простая часть кажется очевидной, и именно поэтому она дважды ломала кнопки
 * молча: `cban:all` (слово вместо числа) и составной префикс `nd:pick:3`. Обе
 * ошибки проявлялись как «нажал — ничего не произошло», а не как ошибка.
 */
import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { parseCallbackData } from './routes.ts';

describe('parseCallbackData', () => {
  it('обычная кнопка: префикс и номер', () => {
    assert.deepEqual(parseCallbackData('pick:12'), { prefix: 'pick', id: 12 });
    assert.deepEqual(parseCallbackData('edit:0'), { prefix: 'edit', id: 0 });
  });

  it('слово `all` — не NaN, а ноль', () => {
    assert.deepEqual(parseCallbackData('cban:all'), { prefix: 'cban', id: 0 });
  });

  it('составной префикс разбирается по последнему двоеточию', () => {
    assert.deepEqual(parseCallbackData('nd:pick:3'), { prefix: 'nd:pick', id: 3 });
  });

  it('мусор не превращается в нажатие', () => {
    assert.equal(parseCallbackData('nonsense'), null);
    assert.equal(parseCallbackData('pick:abc'), null);
    assert.equal(parseCallbackData('nd:pick:abc'), null);
  });
});