/**
 * Продолжение сохранённого КП (2026-11-01): состояние, а не сеть.
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import { canResume, draftFromOffer, resumeMessage, resumeSession } from './resume.ts';
import type { ChatSession } from '../session.ts';

const offer = {
  id: 7,
  number: '003/2026',
  client: { name: 'ООО Строй', contact: '' },
  items: [{ catalogId: 12, name: 'Мембрана', unit: 'м2', qty: 10, price: 500 }],
  formValues: { 'copy.intro': 'Текст' },
};

test('черновик собирается из сохранённого предложения', () => {
  const draft = draftFromOffer(offer);
  assert.equal(draft.offerId, 7);
  assert.equal(draft.number, '003/2026');
  assert.equal(draft.clientName, 'ООО Строй');
  assert.equal(draft.items.length, 1);
  assert.equal(draft.items[0].catalogId, 12);
  assert.equal(draft.formValues['copy.intro'], 'Текст');
  // Заново контекст не спрашиваем: он уже собран в этом КП.
  assert.equal(draft.stage, 'positions');
});

test('пустое предложение тоже открывается на правку', () => {
  const draft = draftFromOffer({ id: 8, number: '004/2026' });
  assert.equal(draft.items.length, 0);
  assert.equal(draft.offerId, 8);
});

test('сессия забывает прежний разбор файла', () => {
  const session: ChatSession = {
    step: 'authed',
    email: 'm@tn.ru',
    attach: { kind: 'brief', brief: { fields: {}, summary: [] } },
    briefTopic: 'object_name',
  };
  const next = resumeSession(session, offer);
  assert.equal(next.attach, undefined, 'старый разбор уехал в новое КП');
  assert.equal(next.briefTopic, undefined);
  assert.equal(next.briefDone, true);
  assert.equal(next.draft?.offerId, 7);
});

test('сообщение об открытом КП говорит про отсутствие позиций', () => {
  const empty = resumeMessage({ id: 9, number: '005/2026', items: [] });
  assert.match(empty, /005\/2026/);
  assert.match(empty, /Позиций пока нет/);

  const filled = resumeMessage(offer);
  assert.match(filled, /Позиций: 1/);
  assert.doesNotMatch(filled, /Позиций пока нет/);
});

test('номер КП узнаётся по форме', () => {
  assert.equal(canResume('003/2026'), true);
  assert.equal(canResume(' 12/2026 '), true);
  assert.equal(canResume('собрать'), false);
  assert.equal(canResume(''), false);
});