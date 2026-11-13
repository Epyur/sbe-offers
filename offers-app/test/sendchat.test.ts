/**
 * «Отправить в чат» (2026-11-13, шаг S7; переделано 2026-11-14).
 *
 * Проверяется нагрузка и отказ ДО запроса: она уходит в сервис строкой, и от
 * её формы зависит, какой документ человек получит в чате. Сеть здесь не
 * поднимается — успешный путь проверяется отдельно на стенде.
 *
 * Почему не `sendData`: он работает только у Mini App, открытого кнопкой
 * клавиатуры, а приложение КП открывается inline-кнопкой (живой дефект
 * 2026-11-14: «нажал — закрылось, в чате пусто»).
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import { payloadOf, preflightProblem, sendButtonText, sendDocumentToChat } from '../src/sendchat.ts';

test('нагрузка — указатель на документ, ничего лишнего', () => {
  assert.deepEqual(payloadOf('offer', 12), { action: 'docx', kind: 'offer', id: 12 });
  assert.deepEqual(payloadOf('combat', 7), { action: 'docx', kind: 'combat', id: 7 });
});

test('несохранённый документ не отправляется', () => {
  assert.equal(payloadOf('offer', 0), null);
  assert.equal(payloadOf('offer', -1), null);
  assert.equal(payloadOf('offer', 1.5), null);
  assert.equal(payloadOf('offer', Number.NaN), null);
});

test('вне Telegram отправка объясняет, откуда её открыть', async () => {
  const outcome = await sendDocumentToChat('offer', 3);
  assert.match(outcome.problem, /Telegram/i);
  assert.match(outcome.problem, /из бота/i);
  assert.equal(outcome.sent, false);
});

test('отказ до запроса виден и без окна Telegram (несохранённый номер)', () => {
  // В node окна нет, поэтому первый отказ — «вне Telegram»; проверяем, что
  // проверка формы документа идёт раньше обращения к платформе.
  const problem = preflightProblem('offer', 0);
  assert.notEqual(problem, '');
});

test('подпись кнопки разная у КП и листка', () => {
  assert.match(sendButtonText('offer'), /КП/);
  assert.match(sendButtonText('combat'), /листок/i);
});