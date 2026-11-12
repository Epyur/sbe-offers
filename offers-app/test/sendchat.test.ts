/**
 * «Отправить в чат» (2026-11-13, шаг S7).
 *
 * Проверяется нагрузка: она уходит в бот строкой, и от её формы зависит,
 * какой документ человек получит в чате.
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import { payloadOf, sendButtonText, sendDocumentToChat } from '../src/sendchat.ts';

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

test('вне Telegram отправка объясняет, откуда её открыть', () => {
  const text = sendDocumentToChat('offer', 3);
  assert.match(text, /Telegram/i);
  assert.match(text, /из бота/i);
});

test('подпись кнопки разная у КП и листка', () => {
  assert.match(sendButtonText('offer'), /КП/);
  assert.match(sendButtonText('combat'), /листок/i);
});