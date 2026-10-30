/**
 * Согласование текста секций в чате (Task 9 плана 2026-10-29).
 *
 * Правило, ради которого это отдельный модуль: принятый текст живёт в
 * предложении (`form_values["copy.*"]`), а сессия хранит только «какая секция
 * сейчас». После перезапуска бот продолжает с той же секции, а не с начала.
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { applyLogin, newSession } from '../session.ts';
import {
  COPY_KEYBOARD,
  advanceCopy,
  copyDone,
  copySlotLabel,
  copyState,
  copyTextMessage,
  currentSlot,
  numberedSlots,
  type SectionSlot,
} from './copy.ts';

const AUTHED = applyLogin(newSession(), 'ivanov@tn.ru');

const SLOTS: SectionSlot[] = [
  { kind: 'intro', key: 'copy.intro', label: 'Вводный текст' },
  { kind: 'system', key: 'copy.system.1', label: 'Описание системы 1', systemIndex: 1 },
  { kind: 'advantages', key: 'copy.advantages', label: 'Преимущества' },
  { kind: 'conditions', key: 'copy.conditions', label: 'Условия' },
];

test('секции: нумерация для систем в ключе, подпись по-русски', () => {
  assert.equal(numberedSlots(2)[1].key, 'copy.system.1');
  assert.equal(numberedSlots(2)[2].key, 'copy.system.2');
  assert.equal(numberedSlots(2)[1].label, 'Описание системы 1');
});

test('секции: без систем остаются три фиксированные', () => {
  assert.equal(numberedSlots(0).length, 3);
});

test('секции: порядок — вводный, системы, преимущества, условия', () => {
  const keys = numberedSlots(1).map((s) => s.key);
  assert.deepEqual(keys, ['copy.intro', 'copy.system.1', 'copy.advantages', 'copy.conditions']);
});

test('состояние хранит текущую секцию', () => {
  const next = copyState(AUTHED, 0);
  assert.equal(next.copySlot, 0);
  assert.equal(currentSlot(next, SLOTS)?.key, 'copy.intro');
});

test('шаг вперёд переводит на следующую секцию', () => {
  const next = advanceCopy(copyState(AUTHED, 0), SLOTS);
  assert.equal(next.copySlot, 1);
  assert.equal(currentSlot(next, SLOTS)?.key, 'copy.system.1');
});

test('последняя секция завершает согласование', () => {
  const last = copyState(AUTHED, SLOTS.length - 1);
  const next = advanceCopy(last, SLOTS);
  assert.equal(copyDone(next, SLOTS), true);
});

test('незаполненная подпись секции не пустая', () => {
  assert.ok(copySlotLabel(undefined).length > 0);
});

test('текст секции показывается целиком и с подписью', () => {
  const text = copyTextMessage('Вводный текст', 'Мы делаем кровли с 1998 года.');
  assert.match(text, /Вводный текст/);
  assert.match(text, /1998/);
});

test('пустой текст — честное сообщение, а не пустая карточка', () => {
  const text = copyTextMessage('Условия', '');
  assert.match(text, /не удалось|пуст|пропуст/i);
});

test('клавиатура секции: принять, поправить, пропустить', () => {
  const labels = (COPY_KEYBOARD.inline_keyboard as unknown[][])
    .map((row) => (row[0] as { text: string }).text);
  assert.ok(labels.some((t) => /Принять/.test(t)));
  assert.ok(labels.some((t) => /Поправить/.test(t)));
  assert.ok(labels.some((t) => /Пропустить/.test(t)));
});

test('состояние без секций не считается завершённым', () => {
  assert.equal(copyDone(newSession(), []), false);
});