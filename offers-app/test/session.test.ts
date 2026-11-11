/**
 * Экран входа: что человек увидит, если вход не удался (2026-11-13, S1).
 *
 * Проверяем текст, а не реализацию: экран — это последнее, что видит человек,
 * и именно по нему он понимает следующий шаг.
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import { failureText, type SessionFailure } from '../src/session.ts';

test('человек не входил в бота — сказано, что нажать', () => {
  const { title, hint } = failureText({ kind: 'not-linked' });
  assert.match(title, /войти в бота/i);
  assert.match(hint, /\/start/);
});

test('открыто вне Telegram — объясняем, где вход работает', () => {
  const { title, hint } = failureText({ kind: 'no-init-data' });
  assert.match(title, /вне Telegram/i);
  assert.match(hint, /из чата|Открыть приложение/i);
});

test('устаревшая подпись — видно причину и что делать', () => {
  const { title, hint } = failureText({ kind: 'bad-init-data', detail: 'stale' });
  assert.match(title, /не подтверждён/i);
  assert.match(hint, /stale/);
  assert.match(hint, /заново/i);
});

test('бот не настроен — адресован администратору, а не менеджеру', () => {
  const { title, hint } = failureText({ kind: 'bot-not-configured', detail: 'no-bot-token' });
  assert.match(title, /не настроен/i);
  assert.match(hint, /администратор/i);
});

test('каждая причина отказа имеет свой текст', () => {
  const kinds: Array<SessionFailure['kind']> = [
    'no-init-data',
    'not-linked',
    'bad-init-data',
    'bot-not-configured',
    'network',
    'server',
  ];
  const titles = new Set<string>();
  for (const kind of kinds) {
    const failure: SessionFailure =
      kind === 'bad-init-data' || kind === 'bot-not-configured' || kind === 'network' || kind === 'server'
        ? { kind, detail: 'x' }
        : { kind };
    titles.add(failureText(failure).title);
  }
  assert.equal(titles.size, kinds.length, 'две разные причины дали одинаковый заголовок');
});