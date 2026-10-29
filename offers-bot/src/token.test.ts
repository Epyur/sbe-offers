/**
 * Правила получения токена бота (2026-10-29). Тест фиксирует главное решение:
 * токен приходит ТОЛЬКО из offers-service, а переменная окружения
 * TELEGRAM_BOT_TOKEN не участвует ни при каких обстоятельствах — в соседнем
 * portal-embed второй источник полтора суток слал уведомления чужим ботом.
 */
import { afterEach, beforeEach, describe, it } from 'node:test';
import assert from 'node:assert/strict';

import { forgetCachedToken, hasBotToken, resolveBotToken } from './telegram.ts';

const REAL_ENV = { ...process.env };

type Call = { url: string; secret: string | undefined };

/** Подменяет fetch на подставной offers-service; возвращает список запросов. */
function stubFetch(handler: (url: string) => Response): Call[] {
  const calls: Call[] = [];
  globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    const headers = (init?.headers ?? {}) as Record<string, string>;
    calls.push({ url, secret: headers['X-Service-Secret'] });
    return handler(url);
  }) as typeof fetch;
  return calls;
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

beforeEach(() => {
  forgetCachedToken();
  process.env = { ...REAL_ENV, OFFERS_BOT_SECRET: 'test-secret' };
  delete process.env.TELEGRAM_BOT_TOKEN;
});

afterEach(() => {
  forgetCachedToken();
  process.env = { ...REAL_ENV };
});

describe('resolveBotToken', () => {
  it('берёт токен из offers-service и передаёт служебный секрет', async () => {
    const calls = stubFetch(() => jsonResponse({ token: ' 111:AAA ', username: 'bot' }));
    const token = await resolveBotToken();
    assert.equal(token, '111:AAA');
    assert.equal(calls.length, 1);
    assert.match(calls[0].url, /\/api\/offers\/bot\/token$/);
    assert.equal(calls[0].secret, 'test-secret');
  });

  it('пуста токен в сервисе — ошибка, а не работа без токена', async () => {
    stubFetch(() => jsonResponse({ token: '' }));
    await assert.rejects(resolveBotToken(), /токен бота не задан/);
  });

  it('НЕ откатывается на TELEGRAM_BOT_TOKEN из окружения', async () => {
    // Приманка: ровно то, что три месяца лежало в .env portal-embed.
    process.env.TELEGRAM_BOT_TOKEN = '999:FOREIGN_BOT';
    stubFetch(() => jsonResponse({ token: '' }));
    await assert.rejects(resolveBotToken(), /токен бота не задан/);
    assert.equal(await hasBotToken(), false);
  });

  it('ошибка сервиса — ошибка, а не подмена другим ботом', async () => {
    process.env.TELEGRAM_BOT_TOKEN = '999:FOREIGN_BOT';
    stubFetch(() => jsonResponse({ error: 'forbidden' }, 403));
    await assert.rejects(resolveBotToken(), /ответил 403/);
  });

  it('недоступный сервис — понятная ошибка', async () => {
    globalThis.fetch = (async () => {
      throw new Error('connect ECONNREFUSED');
    }) as typeof fetch;
    await assert.rejects(resolveBotToken(), /сервис недоступен/);
  });

  it('кэш на 15 секунд: повторный вызов не идёт на сервер', async () => {
    const calls = stubFetch(() => jsonResponse({ token: '111:AAA' }));
    await resolveBotToken();
    await resolveBotToken();
    assert.equal(calls.length, 1);
  });

  it('forgetCachedToken заставляет перечитать токен', async () => {
    const calls = stubFetch(() => jsonResponse({ token: '111:AAA' }));
    await resolveBotToken();
    forgetCachedToken();
    await resolveBotToken();
    assert.equal(calls.length, 2);
  });
});

describe('hasBotToken', () => {
  it('true при действующем токене', async () => {
    stubFetch(() => jsonResponse({ token: '111:AAA' }));
    assert.equal(await hasBotToken(), true);
  });

  it('false при недоступном сервисе — это не поломка запуска', async () => {
    globalThis.fetch = (async () => {
      throw new Error('offline');
    }) as typeof fetch;
    assert.equal(await hasBotToken(), false);
  });
});