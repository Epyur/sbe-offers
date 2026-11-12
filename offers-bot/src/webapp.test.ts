/**
 * «Отправить в чат» из Mini App (2026-11-13, шаг S7).
 *
 * Проверяется разбор чужого ввода: приложение присылает строку, и любая её
 * испорченная форма обязана дать null, а не отправить не тот документ.
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import { captionFor, parseWebAppData, unknownRequestText } from './webapp.ts';

test('правильный указатель разбирается', () => {
  assert.deepEqual(parseWebAppData('{"action":"docx","kind":"offer","id":12}'), {
    action: 'docx',
    kind: 'offer',
    id: 12,
  });
  assert.deepEqual(parseWebAppData('{"action":"docx","kind":"combat","id":"7"}'), {
    action: 'docx',
    kind: 'combat',
    id: 7,
  });
});

test('мусор и подделка не превращаются в запрос', () => {
  const bad = [
    '',
    'not json',
    'null',
    '"docx"',
    '{"action":"delete","kind":"offer","id":1}',
    '{"action":"docx","kind":"sheet","id":1}',
    '{"action":"docx","kind":"offer","id":0}',
    '{"action":"docx","kind":"offer","id":-3}',
    '{"action":"docx","kind":"offer","id":1.5}',
    '{"action":"docx","kind":"offer","id":"abc"}',
    '{"action":"docx","kind":"offer"}',
  ];
  for (const raw of bad) {
    assert.equal(parseWebAppData(raw), null, `разобрано: ${raw}`);
  }
});

test('в указателе нет chat_id — отправить в чужой чат нельзя', () => {
  const parsed = parseWebAppData('{"action":"docx","kind":"offer","id":1,"chat_id":"999"}');
  assert.deepEqual(parsed, { action: 'docx', kind: 'offer', id: 1 });
});

test('непонятная команда объясняет, что делать', () => {
  assert.match(unknownRequestText(), /Открыть документ|Откройте документ/i);
});

test('подпись файла зависит от вида документа', () => {
  assert.equal(captionFor({ action: 'docx', kind: 'offer', id: 1 }), 'Коммерческое предложение');
  assert.equal(captionFor({ action: 'docx', kind: 'combat', id: 1 }), 'Боевой листок');
});