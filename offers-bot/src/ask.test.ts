/**
 * Сторожа просьбы из приложения «продолжить в боте» (2026-11-15).
 *
 * Проверяется разбор тела: маршрут `/internal/ask` защищён секретом сервиса,
 * но адресат и вид действия обязаны проверяться и здесь — иначе бот принялся
 * бы «продолжать» что угодно в любой чат.
 */
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { askIntroText, parseAskRequest } from './flow/ask.ts';

test('просьба разбирается: чат, действие, номер КП', () => {
  const got = parseAskRequest({
    chat_id: '810181470',
    action: 'brief',
    offer_id: 12,
    offer_number: '031/2026',
  });
  assert.deepEqual(got, {
    chatId: '810181470',
    action: 'brief',
    offerId: 12,
    offerNumber: '031/2026',
  });
});

test('без чата, вида действия или КП просьба не принимается', () => {
  assert.equal(parseAskRequest(null), null);
  assert.equal(parseAskRequest({ action: 'brief', offer_id: 1 }), null);
  assert.equal(parseAskRequest({ chat_id: 'не-чат', action: 'brief', offer_id: 1 }), null);
  assert.equal(parseAskRequest({ chat_id: '1', action: 'docx', offer_id: 1 }), null);
  assert.equal(parseAskRequest({ chat_id: '1', action: 'brief' }), null);
  assert.equal(parseAskRequest({ chat_id: '1', action: 'brief', offer_id: 0 }), null);
});

test('номер КП необязателен: строка всё равно называет, что открываем', () => {
  const got = parseAskRequest({ chat_id: '1', action: 'positions', offer_id: 7 });
  assert.ok(got);
  assert.equal(got.offerNumber, '');
  const text = askIntroText(got);
  assert.match(text, /Разбор позиций/);
  assert.match(text, /xlsx/);
});

test('бриф говорит, откуда просьба, и обещает показать ответы в приложении', () => {
  const got = parseAskRequest({ chat_id: '1', action: 'brief', offer_id: 7, offer_number: '031/2026' });
  assert.ok(got);
  const text = askIntroText(got);
  assert.match(text, /031\/2026/);
  assert.match(text, /в приложении/);
});