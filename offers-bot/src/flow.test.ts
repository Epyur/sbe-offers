import { test } from 'node:test';
import assert from 'node:assert/strict';
import { nextStep, newSession, applyCodeRequested, applyLogin } from './session.ts';
import {
  addCatalogItem,
  addManualItem,
  catalogMessage,
  draftTotal,
  formatMoney,
  newDraft,
  parseManualItem,
  removeItem,
} from './flow.ts';

test('вход: e-mail приглашает к вводу и запрашивает код', () => {
  const s = newSession();
  const r = nextStep(s, 'Ivanov@TN.ru');
  assert.equal(r.kind, 'request_code');
  if (r.kind !== 'request_code') return;
  assert.equal(r.email, 'ivanov@tn.ru');
  const next = applyCodeRequested(s, r.email);
  assert.equal(next.step, 'await_code');
  assert.equal(next.email, 'ivanov@tn.ru');
});

test('вход: не e-mail — повтор приглашения', () => {
  const r = nextStep(newSession(), 'привет');
  assert.equal(r.kind, 'prompt');
});

test('вход: код не из 6 цифр отклоняется без запроса', () => {
  const s = applyCodeRequested(newSession(), 'ivanov@tn.ru');
  const r = nextStep(s, '12345');
  assert.equal(r.kind, 'prompt');
  if (r.kind === 'prompt') assert.match(r.text, /6 цифр/);
});

test('вход: ровно 6 цифр — проверка', () => {
  const s = applyCodeRequested(newSession(), 'ivanov@tn.ru');
  const r = nextStep(s, ' 012345 ');
  assert.equal(r.kind, 'verify_code');
  if (r.kind === 'verify_code') {
    assert.equal(r.code, '012345');
    assert.equal(r.email, 'ivanov@tn.ru');
  }
});

test('вход: после входа текст не трогаем машиной входа', () => {
  const s = applyLogin(newSession(), 'ivanov@tn.ru');
  assert.equal(s.step, 'authed');
  assert.equal(nextStep(s, 'что-то').kind, 'ignore');
});

test('каталог: пустой каталог честно сообщает об этом', () => {
  const msg = catalogMessage([], '');
  assert.match(msg, /Каталог пуст/);
  const search = catalogMessage([], 'кровля');
  assert.match(search, /ничего не нашлось/);
});

test('свободная позиция: разбор и отказ', () => {
  assert.deepEqual(parseManualItem('Монтаж | 12 | 850'), { name: 'Монтаж', qty: 12, price: 850 });
  assert.deepEqual(parseManualItem('Клей | 2,5 | 300,5'), { name: 'Клей', qty: 2.5, price: 300.5 });
  assert.equal(parseManualItem('просто текст'), null);
  assert.equal(parseManualItem('Монтаж | 0 | 100'), null);
  assert.equal(parseManualItem('Монтаж | abc | 100'), null);
});

test('итог: копейки без хвостов плавающей точки', () => {
  let draft = newDraft();
  draft = addCatalogItem(draft, { id: 1, name: 'Система', unit: 'шт', basePrice: 999.99 }, 3);
  draft = addManualItem(draft, 'Монтаж', 1, 1500);
  assert.equal(draft.items.length, 2);
  assert.equal(draftTotal(draft), 4499.97);
  assert.equal(formatMoney(draftTotal(draft)), '4 499,97');
});

test('удаление последней позиции', () => {
  let draft = newDraft();
  draft = addManualItem(draft, 'Монтаж', 1, 100);
  draft = addManualItem(draft, 'Клей', 2, 50);
  draft = removeItem(draft, 1);
  assert.equal(draft.items.length, 1);
  assert.equal(draft.items[0].name, 'Монтаж');
  assert.equal(draftTotal(draft), 100);
});

test('пустой черновик даёт нулевой итог', () => {
  assert.equal(draftTotal(newDraft()), 0);
  assert.equal(formatMoney(0), '0,00');
});