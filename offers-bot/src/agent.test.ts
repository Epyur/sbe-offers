/**
 * Проверки помощника (2026-11-14, шаг P6): чистые правила — включение и
 * выключение, документ разговора, разбор ответа сервера. Сеть и модель не
 * нужны: серверная часть проверяется go-тестами, а здесь — то, что решает,
 * куда уйдёт свободный текст.
 */
import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import {
  agentDocument,
  agentFileCaption,
  agentOn,
  agentOutcome,
  enterAgent,
  exitAgent,
} from './flow/agent.ts';
import { newSession } from './session.ts';
import type { AgentReply } from './api.ts';

describe('помощник: включение и выключение', () => {
  it('включается и выключается, не теряя черновик', () => {
    const base = newSession();
    const withDraft = { ...base, step: 'authed' as const, email: 'a@tn.ru' };
    const on = enterAgent(withDraft);
    assert.equal(agentOn(on), true);
    const off = exitAgent(on);
    assert.equal(agentOn(off), false);
    assert.equal(off.email, 'a@tn.ru');
    assert.equal(off.step, 'authed');
  });

  it('по умолчанию выключен', () => {
    assert.equal(agentOn(newSession()), false);
  });
});

describe('помощник: документ разговора', () => {
  it('без документа возвращает пусто', () => {
    assert.deepEqual(agentDocument(newSession()), {});
  });

  it('берёт открытое КП', () => {
    const session = {
      ...newSession(),
      draft: { items: [], clientName: '', clientContact: '', formValues: {}, offerId: 42, step: 'catalog' as const, pendingFields: [] },
    };
    assert.deepEqual(agentDocument(session), { offerId: 42 });
  });

  it('боевой листок главнее черновика КП', () => {
    const session = {
      ...newSession(),
      draft: { items: [], clientName: '', clientContact: '', formValues: {}, offerId: 42, step: 'catalog' as const, pendingFields: [] },
      combat: { sheetId: 7, number: '001/2026' },
    };
    assert.deepEqual(agentDocument(session), { sheetId: 7 });
  });
});

describe('помощник: разбор ответа', () => {
  it('раскодирует файлы и отбрасывает пустые', () => {
    const reply: AgentReply = {
      answer: ' Документ готов. ',
      files: [
        { name: 'КП 003-2026.docx', base64: Buffer.from('PK-data').toString('base64') },
        { name: 'пустой.docx', base64: '' },
      ],
    };
    const outcome = agentOutcome(reply);
    assert.equal(outcome.text, 'Документ готов.');
    assert.equal(outcome.files.length, 1);
    assert.equal(outcome.files[0].name, 'КП 003-2026.docx');
  });

  it('пустой ответ — «Готово», а не пустое сообщение', () => {
    assert.equal(agentOutcome({ answer: '  ', files: [] }).text, 'Готово.');
  });

  it('подпись файла зависит от вида документа', () => {
    assert.equal(agentFileCaption('Боевой листок 001-2026.docx'), 'Боевой листок');
    assert.equal(agentFileCaption('КП 003-2026.docx'), 'Коммерческое предложение');
  });
});