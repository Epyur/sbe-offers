/**
 * Файлы из чата (шаг 6 плана 2026-10-29): что бот отвечает и что запоминает.
 *
 * Проверяется без Telegram и без offers-service: модуль `attach.ts` чистый, а
 * подтверждение проверяется на подставленных функциях.
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import {
  NO_OFFER_FOR_FILE,
  applyPending,
  attachMessage,
  cancelMessage,
  forgetAttach,
  rememberAttach,
  specPreview,
  type AttachResult,
} from './attach.ts';
import type { ChatSession } from './session.ts';

const briefResult: AttachResult = {
  kind: 'md',
  name: 'бриф.md',
  reply: 'Понял: объект: Склад. Разложить в бриф? ✓ / ✏️',
  needsConfirm: true,
  brief: {
    fields: { object_name: ['Склад'] },
    notes: 'Заказчик: ООО Строй',
    summary: ['объект: Склад'],
  },
};

const specResult: AttachResult = {
  kind: 'xlsx',
  name: 'спецификация.xlsx',
  reply: 'Нашёл таблицу: строк: 2, совпало с каталогом: 1, нет в каталоге: 1.',
  needsConfirm: true,
  spec: {
    rows: [
      {
        name: 'Стилобат',
        unit: 'м2',
        qty: 120,
        price: 1850,
        sum: 222000,
        hasCatalog: true,
        priceDiff: true,
        catalogId: 7,
      },
      { name: 'Песок', unit: 'т', qty: 18, price: 900, sum: 16200, hasCatalog: false, priceDiff: false },
    ],
    matched: 1,
    new: 1,
    priceDiffs: 1,
    summary: 'строк: 2, совпало с каталогом: 1, нет в каталоге: 1',
  },
};

function session(): ChatSession {
  return { step: 'authed', email: 'manager@tn.ru', draft: { offerId: 42 } as ChatSession['draft'] };
}

test('подтверждение: разбор файла показывается с кнопками', () => {
  const { text, keyboard } = attachMessage(briefResult);
  assert.match(text, /Понял/);
  assert.ok(keyboard, 'нет клавиатуры подтверждения');
});

test('без подтверждения — просто текст, кнопок нет', () => {
  const result: AttachResult = {
    kind: 'xlsx',
    name: 'контекст.xlsx',
    reply: 'Прочитал таблицу, но состава в ней не нашёл.',
    needsConfirm: false,
    context: ['Параметр | Значение'],
  };
  const message = attachMessage(result);
  assert.equal(message.keyboard, undefined);
  assert.match(message.text, /состава в ней не нашёл/);
});

test('таблица: строки, пометки каталога и предупреждения', () => {
  const lines = specPreview(specResult.spec!).join('\n');
  assert.match(lines, /Стилобат/);
  assert.match(lines, /цена в файле другая/);
  assert.match(lines, /нет в каталоге/);
});

test('таблица: длинный список обрезается с подписью', () => {
  const rows = Array.from({ length: 20 }, (_, i) => ({
    name: `Позиция ${i + 1}`,
    unit: 'шт',
    qty: 1,
    price: 100,
    sum: 100,
    hasCatalog: false,
    priceDiff: false,
  }));
  const lines = specPreview({ rows, matched: 0, new: 20, priceDiffs: 0, summary: '' });
  assert.ok(lines.length < rows.length + 2, 'список не обрезан');
  assert.match(lines.join('\n'), /и ещё 8/);
});

test('разбор запоминается и забывается', () => {
  const remembered = rememberAttach(session(), briefResult);
  assert.equal(remembered.attach?.kind, 'brief');
  assert.equal(remembered.draft?.offerId, 42, 'черновик потерялся');

  const forgotten = forgetAttach(remembered);
  assert.equal(forgotten.attach, undefined);
  assert.equal(forgotten.draft?.offerId, 42);
});

test('файл без разбора не запоминается', () => {
  const nothing: AttachResult = {
    kind: '',
    name: 'файл',
    reply: 'Не понял формат',
    needsConfirm: false,
  };
  assert.equal(rememberAttach(session(), nothing).attach, undefined);
});

test('применение вызывает сервис тем режимом, что в разборе', async () => {
  const calls: string[] = [];
  const reply = await applyPending(
    { kind: 'spec', spec: specResult.spec! },
    async (mode) => {
      calls.push(mode);
      return { reply: 'Состав обновлён: позиций 2' };
    },
  );
  assert.deepEqual(calls, ['spec']);
  assert.match(reply, /Состав обновлён/);
});

test('отказ: разные слова для брифа и таблицы', () => {
  assert.match(cancelMessage({ kind: 'brief', brief: briefResult.brief! }), /бриф не трогаю/);
  assert.match(cancelMessage({ kind: 'spec', spec: specResult.spec! }), /состав оставляю как был/);
  assert.match(cancelMessage(undefined), /ничего не применяю/);
});

test('подсказка без черновика объясняет, что делать', () => {
  assert.match(NO_OFFER_FOR_FILE, /Собрать КП/);
});