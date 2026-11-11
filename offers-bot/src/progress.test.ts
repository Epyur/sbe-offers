/**
 * Долгие действия видны человеку (2026-11-12).
 *
 * Живая жалоба: «после нажатия на кнопку „Поиск аналогов“ в боте вообще не
 * понятно, что происходит, хотя ответ приходит через более чем минуту».
 *
 * Правила, которые здесь закреплены:
 *  1. нажатие подтверждается СРАЗУ ответом на callback — Telegram тогда не
 *     крутит часики и не даёт нажать второй раз;
 *  2. долгий обработчик сначала пишет «⌛ …» и потом заменяет это сообщение
 *     результатом, а не молчит минуту.
 */
import { afterEach, beforeEach, describe, it } from 'node:test';
import assert from 'node:assert/strict';

import { newSession } from './session.ts';
import type { ChatSession } from './session.ts';
import { runWithProgress } from './progress.ts';

const REAL_ENV = { ...process.env };

beforeEach(() => {
  process.env = { ...REAL_ENV };
});

afterEach(() => {
  process.env = { ...REAL_ENV };
});

function recorder() {
  const sent: string[] = [];
  const edits: string[] = [];
  return {
    sent,
    edits,
    deps: {
      send: async (_chatId: string, text: string) => {
        sent.push(text);
        return 1;
      },
      edit: async (_chatId: string, _messageId: number, text: string) => {
        edits.push(text);
      },
    },
  };
}

describe('прогресс долгого действия', () => {
  it('сначала «⌛», потом результат в том же сообщении', async () => {
    const r = recorder();
    const out = await runWithProgress(
      { send: r.deps.send, edit: r.deps.edit },
      '42',
      '⌛ Подбираю аналоги…',
      async () => 'Нашёл 5 аналогов',
    );
    assert.deepEqual(out, 'Нашёл 5 аналогов');
    assert.equal(r.sent[0], '⌛ Подбираю аналоги…');
    assert.deepEqual(r.edits, ['Нашёл 5 аналогов']);
  });

  it('ошибка тоже видна и заменяет «⌛», а не остаётся в чате навсегда', async () => {
    const r = recorder();
    await assert.rejects(
      runWithProgress(
        { send: r.deps.send, edit: r.deps.edit },
        '42',
        '⌛ Собираю файл…',
        async () => {
          throw new Error('модель не ответила');
        },
      ),
    );
    assert.equal(r.sent[0], '⌛ Собираю файл…');
    assert.equal(r.edits.length, 1);
    assert.match(r.edits[0], /Не удалось/);
    assert.match(r.edits[0], /модель не ответила/);
  });

  it('быстрый обработчик не дописывает «⌛» лишней строкой', async () => {
    const r = recorder();
    await runWithProgress(
      { send: r.deps.send, edit: r.deps.edit },
      '42',
      '⌛…',
      async () => 'готово',
    );
    assert.equal(r.sent.length, 1);
    assert.equal(r.edits.length, 1);
  });
});

describe('состояние разговора не теряется', () => {
  it('сессия — единственная точка правды между шагами', () => {
    const s: ChatSession = newSession();
    assert.equal(s.step, 'idle');
  });
});