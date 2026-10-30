/**
 * Бриф в боте (Task 9 плана 2026-10-29) — чистые шаги диалога.
 *
 * Проверяется только то, что решается без сети: какие кнопки бот показывает,
 * куда двигается состояние и что попадает в сессию. Ответы модели и сервиса
 * подставляются стендом — их форма важнее их содержания.
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { applyLogin, newSession } from '../session.ts';
import {
  BRIEF_DONE_KEYBOARD,
  BRIEF_KEYBOARD,
  briefSummary,
  briefTurnState,
} from './brief.ts';

const AUTHED = applyLogin(newSession(), 'ivanov@tn.ru');

/** Какие кнопки бот покажет: человек видит только подписи. */
function buttonLabels(keyboard: { inline_keyboard: unknown }): string[] {
  return (keyboard.inline_keyboard as unknown[][])
    .map((row) => (row[0] as { text: string }).text);
}

function buttonData(keyboard: { inline_keyboard: unknown }): string[] {
  return (keyboard.inline_keyboard as unknown[][])
    .map((row) => (row[0] as { callback_data: string }).callback_data);
}

test('бриф: после ответа состояние хранит текущую тему', () => {
  const next = briefTurnState(AUTHED, { reply: 'Хорошо.', nextTopic: 'customer_pains', done: false });
  assert.equal(next.step, 'authed');
  assert.equal(next.briefTopic, 'customer_pains');
  assert.equal(next.briefDone, false);
});

test('бриф: последний ответ помечает диалог завершённым', () => {
  const next = briefTurnState(AUTHED, { reply: 'Готово.', nextTopic: '', done: true });
  assert.equal(next.briefDone, true);
});

test('бриф: сводка показывает заполненное и пропущенное', () => {
  const text = briefSummary({
    payload: {
      object_name: 'ЖК «Поэма»',
      customer_pains: ['нужен ввод в эксплуатацию'],
      skipped_topics: ['budget_signal'],
    } as never,
    done: true,
  });
  assert.match(text, /ЖК «Поэма»/);
  assert.match(text, /не отвечали|пропущ/i);
});

test('бриф: сводка без ответов честно говорит, что пусто', () => {
  const text = briefSummary({ payload: { skipped_topics: [] } as never, done: true });
  assert.match(text, /не заполн|пуст/i);
});

test('бриф можно пропустить: кнопка «Дальше» есть и в середине, и в конце', () => {
  assert.ok(buttonData(BRIEF_KEYBOARD).includes('brief:next'));
  assert.ok(buttonData(BRIEF_DONE_KEYBOARD).includes('brief:next'));
});

test('в середине брифа есть «Дополнить», в конце — только «Дальше»', () => {
  assert.ok(buttonLabels(BRIEF_KEYBOARD).some((t) => /Дополнить/.test(t)));
  assert.ok(!buttonLabels(BRIEF_DONE_KEYBOARD).some((t) => /Дополнить/.test(t)));
});

test('сводка перечисляет заполненные поля по-русски', () => {
  const text = briefSummary({
    payload: {
      object_name: 'ЖК «Поэма»',
      timeline: 'до конца квартала',
      skipped_topics: [],
    } as never,
  });
  assert.match(text, /Объект: ЖК «Поэма»/);
  assert.match(text, /Сроки: до конца квартала/);
  assert.doesNotMatch(text, /Бюджет/);
});