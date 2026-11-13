import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  formatSuggestLine,
  parseHelpRequest,
  suggestHeadline,
  suggestItems,
  suggestKeyboard,
  suggestMessage,
  suggestPickedMessage,
  type SuggestItem,
} from './suggest.ts';

// Подсказка по человеческой фразе (2026-11-12).
//
// Стенд проверяет то, что увидит менеджер: какие строки и какие кнопки. Сеть
// проверяется в api.ts, разбор нажатий — в callbacks.ts.

// КП013: человек написал «Кровля» и ждал, что бот скажет про несколько
// системных решений и предложит помощь. Просьба о помощи разбирается отдельно
// от поиска: служебные слова в запрос не уходят. Слово оставляем как есть —
// морфологию знает поиск Базы знаний, а не бот.
test('просьба о помощи разбирается в запрос', () => {
  assert.equal(parseHelpRequest('Какие есть системы кровли?'), 'кровли');
  assert.equal(parseHelpRequest('помогите подобрать мембрану для кровли'), 'мембрану кровли');
  assert.equal(parseHelpRequest('Что можете предложить по фасаду'), 'фасаду');
});

test('обычная фраза не считается просьбой о помощи', () => {
  assert.equal(parseHelpRequest('Гарант'), null);
  assert.equal(parseHelpRequest('Кровля'), null);
  assert.equal(parseHelpRequest(''), null);
});

test('просьба без предмета даёт пустой запрос', () => {
  assert.equal(parseHelpRequest('помогите'), '');
  assert.equal(parseHelpRequest('какие есть системы?'), '');
});

// КП008: «Гарант» обязан дать обе системы, а не только точное совпадение.
test('несколько систем — так и говорим', () => {
  const items: SuggestItem[] = [
    { kind: 'system', noteId: 1, title: 'ТН-Кровля Гарант ПЛЮС' },
    { kind: 'system', noteId: 2, title: 'ТН-Кровля Гарант RE30' },
  ];
  const text = suggestMessage('Гарант', items);
  assert.match(text, /несколько системных решений \(2\)/);
  assert.match(text, /ТН-Кровля Гарант ПЛЮС/);
  assert.match(text, /ТН-Кровля Гарант RE30/);
  assert.match(text, /Нажмите номер системы/);
});

test('одна система — без слова «несколько»', () => {
  assert.match(
    suggestHeadline('логикруф', 1, 0),
    /нашлась система/,
  );
});

test('системы и материалы — два числа в заголовке', () => {
  assert.match(suggestHeadline('кровля', 3, 5), /систем: 3 и материалов: 5/);
});

test('ничего не нашлось — не тупик, а что делать', () => {
  const text = suggestMessage('абракадабра', []);
  assert.match(text, /ничего не нашлось/);
  assert.match(text, /система <название>/);
});

test('строки нумеруются, материал помечен', () => {
  const items: SuggestItem[] = [
    { kind: 'system', noteId: 1, title: 'ТН-Кровля Гарант', code: 'TN-1' },
    { kind: 'material', noteId: 2, title: 'Мембрана ПВХ' },
  ];
  assert.equal(formatSuggestLine(1, items[0]), '1. ТН-Кровля Гарант (TN-1)');
  assert.equal(formatSuggestLine(2, items[1]), '2. Мембрана ПВХ — материал');
});

// Системы идут первыми и переживают обрезку: система — то, ради чего собирают
// КП, материал — её состав.
test('системы идут первыми и не теряются при обрезке', () => {
  const items = suggestItems(
    [{ noteId: 1, title: 'Система А' }, { noteId: 2, title: 'Система Б' }],
    [
      { noteId: 3, materialId: 3, name: 'Материал 1', unit: 'м2', priceMin: 0, priceMax: 0 },
      { noteId: 4, materialId: 4, name: 'Материал 2', unit: 'м2', priceMin: 0, priceMax: 0 },
    ],
    3,
  );
  assert.deepEqual(items.map((i) => i.noteId), [1, 2, 3]);
  assert.equal(items[0].kind, 'system');
  assert.equal(items[2].kind, 'material');
});

// 2026-11-14, живая жалоба: по запросу «Logicbase» бот не предложил ни одного
// материала. Восемь систем выбирали лимит целиком, и материалы, которые сервер
// отдал, терялись при обрезке. Половина лимита теперь принадлежит материалам.
test('материалы не теряются, когда систем ровно по лимиту', () => {
  const systems = Array.from({ length: 8 }, (_, i) => ({ noteId: 100 + i, title: `Система ${i + 1}` }));
  const materials = [
    { noteId: 201, materialId: 201, name: 'Логическая смола', unit: 'кг', priceMin: 0, priceMax: 0 },
    { noteId: 202, materialId: 202, name: 'Логическая панель', unit: 'м2', priceMin: 0, priceMax: 0 },
  ];
  const items = suggestItems(systems, materials, 8);
  assert.equal(items.length, 8);
  const kinds = items.map((i) => i.kind);
  assert.ok(kinds.includes('material'), 'материалы должны остаться в списке');
  assert.equal(kinds.filter((k) => k === 'material').length, 2);
  assert.equal(kinds.filter((k) => k === 'system').length, 6);
});

test('одна половина пустая — место забирает другая', () => {
  const systems = Array.from({ length: 8 }, (_, i) => ({ noteId: i + 1, title: `Система ${i + 1}` }));
  assert.equal(suggestItems(systems, [], 8).length, 8);
  const materials = [
    { noteId: 9, materialId: 9, name: 'Мембрана', unit: 'м2', priceMin: 0, priceMax: 0 },
  ];
  const only = suggestItems([], materials, 8);
  assert.deepEqual(only.map((i) => i.kind), ['material']);
});

test('кнопки-номера по четыре в ряд и «продолжить» всегда', () => {
  const items: SuggestItem[] = Array.from({ length: 6 }, (_, i) => ({
    kind: 'system' as const,
    noteId: i + 1,
    title: `Система ${i + 1}`,
  }));
  const kb = suggestKeyboard(items, []);
  const rows = kb.inline_keyboard as Array<Array<{ text: string; callback_data: string }>>;
  assert.equal(rows[0].length, 4);
  assert.equal(rows[1].length, 2);
  assert.equal(rows[1][0].callback_data, 'sug:4');
  // «Добавить все системы» + «продолжить».
  assert.ok(rows.some((r) => r[0].callback_data === 'sug:systems'));
  assert.ok(rows.some((r) => r[0].callback_data === 'sug:skip'));
});

test('при пустом списке нет ни номеров, ни «добавить все»', () => {
  const kb = suggestKeyboard([], []);
  const rows = kb.inline_keyboard as Array<Array<{ text: string; callback_data: string }>>;
  assert.ok(!rows.some((r) => r.some((b) => b.callback_data.startsWith('sug:systems'))));
  assert.ok(rows.some((r) => r.some((b) => b.callback_data === 'sug:skip')));
});

test('после добавления говорим про позиции честно', () => {
  assert.match(suggestPickedMessage(1, 'system'), /позиции не добавлялись/);
  assert.match(suggestPickedMessage(2, 'system'), /Систем добавлено: 2/);
  assert.match(suggestPickedMessage(1, 'material'), /добавлен позицией/);
});