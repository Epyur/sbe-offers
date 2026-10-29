/**
 * В боте НЕТ ролей (решение пользователя 2026-10-29).
 *
 * Живая жалоба: в плагине ставили «Менеджер», а бот продолжал отвечать
 * «Ваша роль — просмотр». Причина — роли дублировались в двух местах и
 * сопоставлялись неочевидно. Решение: роли из бота убраны совсем; любой, кто
 * вошёл по коду, собирает КП. Права плагина (viewer/editor/admin) остаются
 * только в плагине и на бота не влияют.
 *
 * Эти тесты фиксируют именно это: вход не несёт роли, сессия её не хранит,
 * и сборка КП не зависит ни от чего, кроме факта входа.
 */
import { afterEach, beforeEach, describe, it } from 'node:test';
import assert from 'node:assert/strict';

import { applyLogin, newSession } from './session.ts';
import { AUTH_OK } from './flow.ts';

const REAL_ENV = { ...process.env };

beforeEach(() => {
  process.env = { ...REAL_ENV };
});

afterEach(() => {
  process.env = { ...REAL_ENV };
});

describe('у бота нет ролей', () => {
  it('новая сессия не содержит поля role', () => {
    const session = newSession();
    assert.equal('role' in session, false, 'роль вернулась в сессию бота');
  });

  it('вход по коду не принимает и не хранит роль', () => {
    const session = applyLogin(newSession(), 'ivanov@tn.ru');
    assert.equal(session.step, 'authed');
    assert.equal(session.email, 'ivanov@tn.ru');
    assert.equal('role' in session, false);
  });

  it('сообщение о входе не называет роль', () => {
    const text = AUTH_OK('ivanov@tn.ru');
    assert.match(text, /ivanov@tn\.ru/);
    for (const word of ['просмотр', 'менеджер', 'администратор']) {
      assert.equal(text.includes(word), false, `в приветствии осталась роль «${word}»`);
    }
  });
});