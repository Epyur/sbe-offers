import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  catalogMessage,
  formatMoney,
  parseSystemCommand,
  pickSystem,
  systemAddedMessage,
  type SystemOption,
} from './build.ts';
import { systemChoice } from '../callbacks.ts';

// Системы в боте (Task 8, 2026-10-31).
//
// Система в чате не позиция: у неё нет цены. Менеджер пишет «система кровля» —
// бот находит карточку и выбирает её для КП. Позиций выбор не добавляет
// (2026-11-03). Всё решается чистыми функциями, чтобы проверять без сети.

const SYSTEMS: SystemOption[] = [
  { sku: 'TN-ROOF-CLASSIC', title: 'ТН-Кровля Классик', problems: [] },
  { sku: 'TN-STILOBAT', title: 'ТН-Стилобат Эксперт', problems: ['строка без кода'] },
  { sku: 'TN-ROOF-TRAFFIC', title: 'ТН-Кровля Пешеходная', problems: [] },
];

test('команда «система <название>» разбирается', () => {
  assert.equal(parseSystemCommand('система кровля'), 'кровля');
  assert.equal(parseSystemCommand('  Система   Стилобат  '), 'Стилобат');
  assert.equal(parseSystemCommand('/system кровля'), 'кровля');
});

test('обычный текст командой не считается', () => {
  assert.equal(parseSystemCommand('Монтаж | 12 | 850'), null);
  assert.equal(parseSystemCommand('система'), null);
  assert.equal(parseSystemCommand('кровля'), null);
});

test('система находится по началу названия без учёта регистра и ё', () => {
  const found = pickSystem(SYSTEMS, 'кровл');
  assert.equal(found.length, 2);
  assert.equal(found[0].sku, 'TN-ROOF-CLASSIC');
});

test('система находится по коду', () => {
  const found = pickSystem(SYSTEMS, 'tn-stilobat');
  assert.equal(found.length, 1);
  assert.equal(found[0].sku, 'TN-STILOBAT');
});

test('точное название выигрывает у похожих', () => {
  const found = pickSystem(SYSTEMS, 'ТН-Кровля Классик');
  assert.equal(found[0].sku, 'TN-ROOF-CLASSIC');
});

test('пустой запрос ничего не находит', () => {
  assert.equal(pickSystem(SYSTEMS, '   ').length, 0);
});

// Выбор системы ботом (2026-11-02, живая жалоба по КП 003).
//
// У живых карточек Базы знаний кода нет: все 375 заметок без `system_sku`.
// Бот отправлял пустую строку, сервер отвечал «система не найдена», а в
// карточке КП менеджер видел «Выберите систему». Выбор теперь несёт заметку.
test('система без кода выбирается по заметке', () => {
  assert.deepEqual(systemChoice({ sku: '', noteId: 412, title: 'Кровля' }), { noteId: 412 });
  assert.deepEqual(systemChoice({ sku: '  ', noteId: 7, title: 'Кровля' }), { noteId: 7 });
});

test('система с кодом выбирается по коду', () => {
  assert.deepEqual(systemChoice({ sku: 'TN-ROOF', noteId: 412, title: 'Кровля' }), {
    sku: 'TN-ROOF',
  });
});

test('выбор системы без кода и без заметки не превращается в тишину', () => {
  // Пустой sku уходит на сервер как есть: он ответит «система не найдена»,
  // и это честнее, чем молча ничего не отправить.
  assert.deepEqual(systemChoice({ sku: '', title: 'Кровля' }), { sku: '' });
});

// Система выбрана, позиций не добавилось (2026-11-03).
//
// Живая жалоба по КП 003/2026: выбор системы дописывал её состав к позициям из
// файла менеджера, и одни и те же продукты стояли в таблице дважды — с ценой и
// с нулём. Теперь менеджеру должно быть сказано прямо, что позиции он ведёт
// сам, иначе он ждёт таблицу и решает, что бот не сработал.
test('сообщение о выборе системы говорит название и число продуктов', () => {
  const text = systemAddedMessage({ title: 'ТН-Кровля Классик', composition: 7 });
  assert.match(text, /ТН-Кровля Классик/);
  assert.match(text, /7/);
});

test('сообщение о выборе системы прямо говорит, что позиции не добавлены', () => {
  const text = systemAddedMessage({ title: 'ТН-Кровля Классик', composition: 7 });
  assert.match(text, /Позиции не добавлены/);
  assert.match(text, /файлом/);
});

test('без числа продуктов сообщение всё равно честное', () => {
  const text = systemAddedMessage({ title: 'Пустая система' });
  assert.match(text, /Пустая система/);
  assert.match(text, /Позиции не добавлены/);
  assert.doesNotMatch(text, /основных продуктов/);
});

test('каталог бота без цены не печатает выдуманную цену', () => {
  const text = catalogMessage(
    [{ id: 1, name: 'Песок', unit: 'м3', basePrice: 0, hasPrice: false }],
    '',
  );
  assert.match(text, /Песок/);
  assert.match(text, /цена не загружалась/);
});

test('каталог бота с ценой печатает нижнюю границу вилки', () => {
  const text = catalogMessage(
    [{ id: 1, name: 'Мембрана', unit: 'м2', basePrice: 1200, hasPrice: true }],
    '',
  );
  assert.match(text, new RegExp(formatMoney(1200)));
});