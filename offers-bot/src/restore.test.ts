/**
 * Вход переживает перезапуск бота (2026-10-29).
 *
 * Живая жалоба: «при каждом перезапуске бота снова просит авторизацию».
 * Причина — сессия жила только в памяти процесса (Map), а постоянная привязка
 * chat_id → email в offers_bot_link не использовалась для восстановления.
 *
 * Здесь проверяется решение: при первом сообщении от знакомого chat_id бот
 * спрашивает сервис, кто это, и сразу считает человека вошедшим — кода не
 * просит. Это единственная проверка, которая не требует ни сети, ни Telegram:
 * она фиксирует правило «знакомый чат = вошёл».
 */
import { afterEach, beforeEach, describe, it } from 'node:test';
import assert from 'node:assert/strict';

import { restoreSession } from './session.ts';
import { newSession, applyLogin } from './session.ts';

const REAL_ENV = { ...process.env };

beforeEach(() => {
  process.env = { ...REAL_ENV };
});

afterEach(() => {
  process.env = { ...REAL_ENV };
});

describe('восстановление сессии из постоянной привязки', () => {
  it('знакомый chat_id — сразу вошедший, без запроса кода', () => {
    const restored = restoreSession('ivanov@tn.ru');
    assert.equal(restored.step, 'authed');
    assert.equal(restored.email, 'ivanov@tn.ru');
  });

  it('незнакомый chat_id — на шаге ожидания e-mail', () => {
    const restored = restoreSession('');
    assert.equal(restored.step, 'idle');
    assert.equal(restored.email, '');
  });

  it('восстановленная сессия равна обычной после входа', () => {
    assert.deepEqual(restoreSession('ivanov@tn.ru'), applyLogin(newSession(), 'ivanov@tn.ru'));
  });
});