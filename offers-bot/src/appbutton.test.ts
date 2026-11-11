/**
 * Кнопки «открыть приложение» в боте (2026-11-13, внеплановый шаг B+).
 *
 * Проверяем поведение, которое видит человек: в главном меню и у каждой
 * записи есть кнопка `web_app`, и она ведёт на нужный маршрут приложения
 * (`#/list/offers`, `#/offer/{id}`, `#/sheet/{id}`).
 *
 * В Telegram кнопка `web_app` открывает Mini App сама — боту не нужен ни
 * обработчик нажатия, ни callback_data. Поэтому проверка идёт по разметке
 * клавиатуры, а не по ответу сервера.
 */
import { afterEach, beforeEach, describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { applyLogin, newSession, type ChatSession } from './session.ts';
import type { BotDeps, RouterState } from './routes.ts';
import { MAIN_KEYBOARD } from './flow/build.ts';
import { combatListMessage } from './flow/combat.ts';
import { appUrlFromEnv, appButton, offerAppMarkup, sheetsAppMarkup } from './flow/appbutton.ts';

const REAL_ENV = { ...process.env };
const sent: Array<{ chatId: string; text: string; keyboard?: Record<string, unknown> }> = [];

beforeEach(() => {
  sent.length = 0;
  process.env = { ...REAL_ENV, OFFERS_BOT_SECRET: 'test-secret', OFFERS_API_URL: 'http://offers' };
  globalThis.fetch = (async () => new Response('{"ok":true}', { status: 200 })) as typeof fetch;
});

afterEach(() => {
  process.env = REAL_ENV;
});

const AUTHED = applyLogin(newSession(), 'ivanov@tn.ru');

function makeDeps(initial?: ChatSession): { deps: BotDeps; state: RouterState } {
  const state: RouterState = {
    hits: new Map(),
    lastMessage: new Map(),
    pages: new Map(),
    sessions: new Map(),
    foundSystems: new Map(),
    foundCombat: new Map(),
    combatPicker: new Map(),
    foundSuggest: new Map(),
    lastCompetitor: new Map(),
    foundAnalogs: new Map(),
  };
  const current = initial ?? newSession();
  state.sessions.set('42', current);
  const deps: BotDeps = {
    now: () => 1_000_000,
    send: async (chatId, text, keyboard) => {
      sent.push({ chatId, text, keyboard });
      return sent.length;
    },
    identity: async () => ({ email: 'ivanov@tn.ru' }),
    session: async () => state.sessions.get('42') ?? current,
    save: (chatId, session) => state.sessions.set(chatId, session),
    answerCallback: async () => {},
  };
  return { deps, state };
}

/** Все кнопки `web_app` в разметке клавиатуры, с их адресами. */
function webAppButtons(keyboard: Record<string, unknown> | undefined): Array<{ text: string; url: string }> {
  const rows = (keyboard?.inline_keyboard ?? keyboard?.keyboard ?? []) as Array<Array<Record<string, unknown>>>;
  const out: Array<{ text: string; url: string }> = [];
  for (const row of rows) {
    for (const button of row) {
      const webApp = button.web_app as { url?: string } | undefined;
      if (webApp?.url) out.push({ text: String(button.text), url: webApp.url });
    }
  }
  return out;
}

describe('адрес приложения', () => {
  it('по умолчанию — боевой /app-offers/', () => {
    delete process.env.OFFERS_APP_URL;
    assert.equal(appUrlFromEnv(), 'https://epyur.fvds.ru/app-offers/');
  });

  it('переменная окружения переопределяет адрес и получает хвостовой слеш', () => {
    process.env.OFFERS_APP_URL = 'https://example.test/app-offers';
    assert.equal(appUrlFromEnv(), 'https://example.test/app-offers/');
  });

  it('кнопка ведёт на маршрут, а не на голый корень', () => {
    delete process.env.OFFERS_APP_URL;
    assert.equal(appButton('КП', 'offer', 12).web_app.url, 'https://epyur.fvds.ru/app-offers/#/offer/12');
    assert.equal(appButton('Листки', 'list').web_app.url, 'https://epyur.fvds.ru/app-offers/#/list/offers');
  });
});

describe('кнопка в главном меню', () => {
  it('«Открыть приложение» есть в MAIN_KEYBOARD и ведёт на список КП', () => {
    const buttons = webAppButtons(MAIN_KEYBOARD as unknown as Record<string, unknown>);
    assert.equal(buttons.length, 1, 'в главном меню должна быть ровно одна кнопка приложения');
    assert.match(buttons[0].url, /#\/list\/offers$/);
  });
});

describe('кнопка у записей', () => {
  it('в списке КП у каждой записи есть кнопка приложения с её номером', () => {
    const markup = offerAppMarkup([{ id: 5, number: '001/2026' }, { id: 7, number: '002/2026' }]);
    assert.deepEqual(markup, [
      [{ text: '📱 001/2026', web_app: { url: 'https://epyur.fvds.ru/app-offers/#/offer/5' } }],
      [{ text: '📱 002/2026', web_app: { url: 'https://epyur.fvds.ru/app-offers/#/offer/7' } }],
    ]);
  });

  it('в списке боевых листков у каждого листка есть кнопка приложения', () => {
    const markup = sheetsAppMarkup([
      { id: 3, number: '001/2026', label: 'Мембрана' },
      { id: 4, number: '002/2026', label: 'Плиты' },
    ]);
    assert.deepEqual(markup, [
      [{ text: '📱 001/2026 — Мембрана', web_app: { url: 'https://epyur.fvds.ru/app-offers/#/sheet/3' } }],
      [{ text: '📱 002/2026 — Плиты', web_app: { url: 'https://epyur.fvds.ru/app-offers/#/sheet/4' } }],
    ]);
  });
});

describe('бот отдаёт кнопку в живом списке', () => {
  it('«📂 Мои КП» показывает кнопку приложения рядом с каждой записью', async () => {
    const { deps, state } = makeDeps(AUTHED);
    globalThis.fetch = (async (input: string | URL | Request) => {
      const url = String(input);
      if (url.includes('/bot/offers')) {
        return new Response(
          JSON.stringify({
            offers: [
              { id: 11, number: '003/2026', client: { name: 'СтройСервис' }, total: 1000, items: [] },
            ],
          }),
          { status: 200, headers: { 'content-type': 'application/json' } },
        );
      }
      return new Response('{"ok":true}', { status: 200 });
    }) as typeof fetch;

    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '📂 Мои КП', state);

    const listMessage = sent.find((m) => m.keyboard !== undefined && m.text.includes('003/2026'));
    assert.ok(listMessage, 'список КП должен уйти с клавиатурой');
    const apps = webAppButtons(listMessage.keyboard);
    assert.equal(apps.length, 1);
    assert.match(apps[0].url, /#\/offer\/11$/);
  });

  it('«📂 Мои боевые листки» показывает кнопку приложения с номером листка', async () => {
    const { deps, state } = makeDeps(AUTHED);
    globalThis.fetch = (async (input: string | URL | Request) => {
      const url = String(input);
      if (url.includes('/bot/combat')) {
        return new Response(
          JSON.stringify({
            sheets: [
              {
                id: 21,
                number: '004/2026',
                ourNotes: [{ noteId: 1, title: 'Мембрана LOGICROOF' }],
                competitorNotes: [],
                ownerEmail: 'ivanov@tn.ru',
              },
            ],
          }),
          { status: 200, headers: { 'content-type': 'application/json' } },
        );
      }
      return new Response('{"ok":true}', { status: 200 });
    }) as typeof fetch;

    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '📂 Мои боевые листки', state);

    const listMessage = sent.find((m) => m.keyboard !== undefined && m.text.includes('004/2026'));
    assert.ok(listMessage, 'список листков должен уйти с клавиатурой');
    const apps = webAppButtons(listMessage.keyboard);
    assert.equal(apps.length, 1);
    assert.match(apps[0].url, /#\/sheet\/21$/);
  });
});

describe('разметка списка листков', () => {
  it('строчная функция combatListMessage ставит кнопку приложения на каждой строке', () => {
    const list = combatListMessage(
      [
        {
          id: 8,
          number: '008/2026',
          ourNotes: [{ noteId: 1, title: 'ПВХ мембрана' }],
          competitorNotes: [],
          ownerEmail: 'ivanov@tn.ru',
        },
      ] as never,
      'ivanov@tn.ru',
    );
    assert.ok(list);
    const apps = webAppButtons(list.keyboard);
    assert.equal(apps.length, 1);
    assert.match(apps[0].url, /#\/sheet\/8$/);
  });
});