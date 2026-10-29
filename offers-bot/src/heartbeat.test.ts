/**
 * Отметка «бот жив» должна уходить КАЖДУЮ итерацию поллинга (2026-10-29).
 *
 * Живой дефект: report() глушил повтор одинакового текста ошибки — и после
 * первой успешной итерации («ошибки нет» = пустая строка, потом опять пустая)
 * heartbeat перестал отправляться. bot_last_seen_at замер на 14 минут, и плагин
 * честно показал «Бот не отвечает» при полностью живом боте.
 *
 * Здесь проверяется именно это: два подряд одинаковых состояния — два heartbeat.
 */
import { afterEach, beforeEach, describe, it } from 'node:test';
import assert from 'node:assert/strict';

const REAL_ENV = { ...process.env };
const sent: Array<{ path: string; body: unknown }> = [];

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

beforeEach(() => {
  sent.length = 0;
  process.env = { ...REAL_ENV, OFFERS_BOT_SECRET: 'test-secret', OFFERS_API_URL: 'http://offers' };
  globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    const path = new URL(url).pathname;
    const body = init?.body ? JSON.parse(String(init.body)) : undefined;
    sent.push({ path, body });
    // Представление offers-service: токен есть, heartbeats принимаются.
    if (path.endsWith('/bot/token')) return jsonResponse({ token: '111:AAA', username: 'bot' });
    return jsonResponse({ ok: true });
  }) as typeof fetch;
});

afterEach(() => {
  process.env = { ...REAL_ENV };
});

describe('heartbeat при неизменном состоянии', () => {
  it('два одинаковых «ошибки нет» дают два heartbeat', async () => {
    const api = await import('./api.ts');
    await api.heartbeat('');
    await api.heartbeat('');
    const beats = sent.filter((c) => c.path.endsWith('/bot/heartbeat'));
    assert.equal(beats.length, 2, 'второй heartbeat потерялся — метка замрёт');
  });

  it('одинаковая ошибка тоже повторяется: это измерение, а не сообщение', async () => {
    const api = await import('./api.ts');
    await api.heartbeat('Unauthorized');
    await api.heartbeat('Unauthorized');
    const beats = sent.filter((c) => c.path.endsWith('/bot/heartbeat'));
    assert.equal(beats.length, 2);
    assert.deepEqual(beats[0].body, { last_error: 'Unauthorized' });
  });
});

describe('правило глушения повтора в самом поллере', () => {
  it('report() в poller.ts не выходит раньше отправки', async () => {
    const { readFile } = await import('node:fs/promises');
    const src = await readFile(new URL('./poller.ts', import.meta.url), 'utf8');
    // Регресс: строка «if (error === lastSentError) return;» означала, что
    // heartbeat уходит только при смене текста.
    assert.ok(
      !/if\s*\(\s*error\s*===\s*lastSentError\s*\)\s*return/.test(src),
      'report() снова глушит повтор — метка «бот жив» замрёт',
    );
  });
});