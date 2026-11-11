/**
 * Подсказка бота ходит на БОТОВУЮ ручку (2026-11-12).
 *
 * Дефект нашла живая проверка на сервере, а не стенд: бот отправлял
 * `/live/suggest` со служебным секретом, а на той ручке стоит
 * `requirePerm("viewer")` — секрет там не пускает, ответ 403, и человек
 * вместо подсказки видел «Поиск по Базе знаний сейчас недоступен».
 *
 * Стенд дефект пропустил, потому что auth-stub выдаёт роль по e-mail и
 * секрет в нём не отличается от токена. Этот тест проверяет сам АДРЕС:
 * бот обязан звать `/bot/suggest`, где стоит requireServiceSecret.
 */
import { afterEach, beforeEach, test } from 'node:test';
import assert from 'node:assert/strict';

const REAL_ENV = { ...process.env };
const calls: Array<{ url: string; secret: string }> = [];

beforeEach(() => {
  calls.length = 0;
  process.env = {
    ...REAL_ENV,
    OFFERS_BOT_SECRET: 'test-bot-secret',
    OFFERS_API_URL: 'http://offers',
  };
  globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    const headers = (init?.headers ?? {}) as Record<string, string>;
    calls.push({ url, secret: headers['X-Service-Secret'] ?? '' });
    return new Response(
      JSON.stringify({
        systems: [{ noteId: 1, title: 'ТН-КРОВЛЯ Гарант Плюс' }],
        materials: [],
      }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    );
  }) as typeof fetch;
});

afterEach(() => {
  process.env = REAL_ENV;
});

test('подсказка идёт на /bot/suggest, а не на viewer-ручку', async () => {
  const { suggestions } = await import('./api.ts');
  const res = await suggestions('Гарант');

  assert.equal(calls.length, 1);
  const url = calls[0].url;
  assert.ok(
    url.includes('/api/offers/bot/suggest'),
    `бот обязан звать ботовую ручку, а позвал ${url}`,
  );
  assert.ok(!url.includes('/api/offers/suggest?'), 'viewer-ручка секрет не принимает');
  assert.equal(calls[0].secret, 'test-bot-secret');
  assert.ok(url.includes(encodeURIComponent('Гарант')), 'запрос уходит целиком');
  assert.equal(res.systems.length, 1);
});

test('ошибка сервера доходит как понятный текст, а не молчание', async () => {
  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ error: 'нет доступа' }), {
      status: 403,
      headers: { 'Content-Type': 'application/json' },
    })) as typeof fetch;
  const { suggestions } = await import('./api.ts');
  await assert.rejects(() => suggestions('Гарант'), /нет доступа/);
});