/**
 * Сторожа просьбы из приложения «продолжить в боте» (2026-11-15).
 *
 * Проверяется разбор тела: маршрут `/internal/ask` защищён секретом сервиса,
 * но адресат и вид действия обязаны проверяться и здесь — иначе бот принялся
 * бы «продолжать» что угодно в любой чат.
 */
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { askIntroText, askKeyboard, parseAskRequest } from './flow/ask.ts';

test('просьба разбирается: чат, действие, номер КП', () => {
  const got = parseAskRequest({
    chat_id: '810181470',
    action: 'brief',
    offer_id: 12,
    offer_number: '031/2026',
  });
  assert.deepEqual(got, {
    chatId: '810181470',
    kind: 'offer',
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
  assert.equal(got.kind, 'offer');
  if (got.kind !== 'offer') return;
  assert.equal(got.offerNumber, '');
  const text = askIntroText(got);
  assert.match(text, /Разбор позиций/);
  assert.match(text, /xlsx/);
});

test('файл по конкуренту листка: разбор просьбы и текст для человека', () => {
  const got = parseAskRequest({
    chat_id: '1',
    kind: 'sheet',
    action: 'competitor-file',
    sheet_id: 17,
    sheet_number: '017/2026',
  });
  assert.deepEqual(got, {
    chatId: '1',
    kind: 'sheet',
    action: 'competitor-file',
    sheetId: 17,
    sheetNumber: '017/2026',
  });
  assert.ok(got);
  const text = askIntroText(got);
  assert.match(text, /Конкурент из файла/);
  assert.match(text, /017\/2026/);
  assert.match(text, /текстового слоя/);
  const keyboard = askKeyboard(got) as { keyboard?: unknown[] };
  assert.ok(Array.isArray(keyboard.keyboard), 'под просьбой должна быть клавиатура, а не пустой экран');
});

test('действия не путаются между КП и листком', () => {
  // В листке бывает только файл по конкуренту.
  assert.equal(parseAskRequest({ chat_id: '1', kind: 'sheet', action: 'brief', sheet_id: 17 }), null);
  assert.equal(parseAskRequest({ chat_id: '1', kind: 'sheet', action: 'positions', sheet_id: 17 }), null);
  // У листка обязателен свой номер: по нему бот отметит, куда ждёт файл.
  assert.equal(parseAskRequest({ chat_id: '1', kind: 'sheet', action: 'competitor-file' }), null);
  assert.equal(parseAskRequest({ chat_id: '1', kind: 'sheet', action: 'competitor-file', offer_id: 17 }), null);
  // В КП файла по конкуренту нет.
  assert.equal(parseAskRequest({ chat_id: '1', kind: 'offer', action: 'competitor-file', offer_id: 17 }), null);
  // Неизвестный вид документа тоже не принимается.
  assert.equal(parseAskRequest({ chat_id: '1', kind: 'catalog', action: 'brief', offer_id: 17 }), null);
});

test('бриф говорит, откуда просьба, и обещает показать ответы в приложении', () => {
  const got = parseAskRequest({ chat_id: '1', action: 'brief', offer_id: 7, offer_number: '031/2026' });
  assert.ok(got);
  const text = askIntroText(got);
  assert.match(text, /031\/2026/);
  assert.match(text, /в приложении/);
});