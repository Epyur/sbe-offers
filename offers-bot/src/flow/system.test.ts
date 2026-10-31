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

// Системы в боте (Task 8, 2026-10-31).
//
// Система в чате не позиция: у неё нет цены, а состав берётся из Базы знаний.
// Менеджер пишет «система кровля» — бот находит карточку и дописывает её
// позиции. Всё решается чистыми функциями, чтобы проверять без сети.

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

test('сообщение о добавлении называет число и проблемы', () => {
  const text = systemAddedMessage({ added: 3, missingPrice: ['Песок'], problems: ['строка X'] });
  assert.match(text, /3/);
  assert.match(text, /Песок/);
  assert.match(text, /строка X/);
});

test('сообщение без цен и проблем говорит только число', () => {
  const text = systemAddedMessage({ added: 2 });
  assert.match(text, /2/);
  assert.doesNotMatch(text, /без цены/);
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