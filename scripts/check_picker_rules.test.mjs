/*
 * Правила окна выбора продукта (2026-11-21).
 *
 * Две живые жалобы:
 *   1) «в блоке выбора продукта показываются продукты до начала ввода» —
 *      окно рисовало первые 20 строк кэша, хотя человек ещё не написал ни
 *      буквы. В мини-приложении это уже решено (`shouldSearchSide`).
 *   2) «после выбора показываются коды, а не названия» — у выбора в КП есть
 *      только `noteId`/`sku`; подпись бралась из списка ПОСЛЕДНЕГО поиска, и
 *      после второго запроса (или переоткрытия КП) вылезало `sys-412`.
 *
 * Проверки зовут ту же чистую модель, что и оба окна (`src/ui/picker-model.ts`),
 * а не пересказывают её здесь: DOM и Obsidian в ней нет.
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  PICKER_EMPTY_HINT,
  filterCandidates,
  pickerRows,
  refHandle,
  refTitle,
  rememberNames,
} from '../src/ui/picker-model.ts';

const cache = [
  { noteId: 412, title: 'ТН-Кровля Гарант', code: 'TN-ROOF-GARANT' },
  { noteId: 413, title: 'ТН-Стилобат Эксперт', code: 'TN-STILOBAT' },
  { noteId: 414, title: 'Кровельная ПВХ-мембрана ECOPLAST', code: 'MAT-ПРОЧ-ОБЩ-0414' },
];

test('до начала ввода окно не показывает продукты', () => {
  assert.deepEqual(pickerRows(cache, null, ''), [], 'пустой запрос — пустой список');
  assert.deepEqual(pickerRows(cache, null, '   '), [], 'пробелы — тоже пустой запрос');
  assert.deepEqual(pickerRows(cache, null, '\n\t'), [], 'переводы строк — тоже');
  assert.equal(PICKER_EMPTY_HINT.includes('Начните вводить'), true);
});

test('после ввода окно ищет: сервер сильнее кэша', () => {
  const fromServer = [{ noteId: 900, title: 'Найдено сервером', code: 'SRV-1' }];
  assert.deepEqual(pickerRows(cache, fromServer, 'кров'), fromServer);
  assert.deepEqual(
    pickerRows(cache, null, 'кров').map((i) => i.noteId),
    [412, 414],
    'без сервера работает фильтр по кэшу',
  );
  assert.deepEqual(
    pickerRows(cache, null, 'TN-STILOBAT').map((i) => i.noteId),
    [413],
    'код тоже ищется',
  );
});

test('поиск по кэшу терпит регистр и «ё»', () => {
  assert.equal(filterCandidates(cache, 'ГАРАНТ').length, 1);
  assert.equal(filterCandidates(cache, 'мембрана ecoplast').length, 1);
  assert.equal(filterCandidates(cache, 'ничего такого').length, 0);
});

test('выбранный продукт подписывается названием из ЛЮБОГО прошлого поиска', () => {
  const names = new Map();
  rememberNames(names, [cache[0]]);
  // Второй поиск вернул другой набор — имени первого в нём уже нет.
  const secondSearch = [cache[2]];
  rememberNames(names, secondSearch);
  assert.equal(refTitle(names, { noteId: 412, sku: 'TN-ROOF-GARANT' }), 'ТН-Кровля Гарант');
  assert.equal(refTitle(names, { noteId: 414 }), 'Кровельная ПВХ-мембрана ECOPLAST');
});

test('выбор находится и по коду, и по заметке', () => {
  const names = new Map();
  rememberNames(names, [cache[0]]);
  assert.equal(refHandle({ noteId: 412, sku: 'TN-ROOF-GARANT' }), 'sys-412');
  assert.equal(refTitle(names, { sku: 'TN-ROOF-GARANT' }), 'ТН-Кровля Гарант');
});

test('незнакомое имя не превращается в пустую строку', () => {
  const names = new Map();
  assert.equal(refTitle(names, { noteId: 777 }), 'sys-777');
  assert.equal(refTitle(names, { sku: 'MAT-ПРОЧ-ОБЩ-0999' }), 'MAT-ПРОЧ-ОБЩ-0999');
});