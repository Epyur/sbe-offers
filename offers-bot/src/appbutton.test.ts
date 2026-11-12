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
import { appUrlFromEnv, appButton, appListButton, offerAppMarkup, sheetsAppMarkup } from './flow/appbutton.ts';

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
    newDoc: new Map(),
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

  it('у списков есть два маршрута, и они не путаются', () => {
    delete process.env.OFFERS_APP_URL;
    assert.equal(appListButton('КП', 'offers').web_app.url, 'https://epyur.fvds.ru/app-offers/#/list/offers');
    assert.equal(appListButton('Листки', 'sheets').web_app.url, 'https://epyur.fvds.ru/app-offers/#/list/sheets');
  });
});

describe('главное меню (R1, 2026-11-13)', () => {
  it('два списка стоят в ОДНОМ ряду', () => {
    const rows = MAIN_KEYBOARD.keyboard as unknown as Array<Array<{ text: string }>>;
    const rowWithOffers = rows.findIndex((row) => row.some((b) => b.text === '📂 Мои КП'));
    const rowWithSheets = rows.findIndex((row) => row.some((b) => b.text === '📂 Мои боевые листки'));
    assert.ok(rowWithOffers >= 0, 'кнопки «Мои КП» нет в меню');
    assert.equal(rowWithOffers, rowWithSheets, 'обе кнопки списков должны стоять в одном ряду');
  });

  it('в главном меню НЕТ кнопки `web_app`', () => {
    // 2026-11-13, решение пользователя: reply-клавиатура с `web_app`
    // открывала приложение без подписи Telegram, и человек видел «Нет доступа:
    // войдите в бота». Подпись приходит только у инлайновой кнопки — список
    // открывается его ответом, а не самой reply-кнопкой.
    const buttons = webAppButtons(MAIN_KEYBOARD as unknown as Record<string, unknown>);
    assert.equal(buttons.length, 0, 'в главном меню кнопок Mini App быть не должно');
  });

  it('обе кнопки списков отвечают инлайновой кнопкой приложения, каждая на свой маршрут', async () => {
    const route = await import('./routes.ts');
    for (const [label, url] of [
      ['📂 Мои КП', '#/list/offers'],
      ['📂 Мои боевые листки', '#/list/sheets'],
    ] as const) {
      sent.length = 0;
      const { deps, state } = makeDeps(AUTHED);
      await route.routeText(deps, '42', label, state);

      assert.equal(sent.length, 1, `${label} должна ответить ровно одним сообщением`);
      const apps = webAppButtons(sent[0].keyboard);
      assert.equal(apps.length, 1, `${label}: нужна одна инлайновая кнопка приложения`);
      assert.match(apps[0].url, new RegExp(`${url.replace('/', '\\/')}$`));
    }
  });

  it('список в чате больше не дублируется: сервер за ним не зовут', async () => {
    const route = await import('./routes.ts');
    let asked = false;
    globalThis.fetch = (async () => {
      asked = true;
      return new Response('{"ok":true}', { status: 200 });
    }) as typeof fetch;
    const { deps, state } = makeDeps(AUTHED);
    await route.routeText(deps, '42', '📂 Мои КП', state);
    assert.equal(asked, false, 'кнопка списка не должна ходить в сервис за списком');
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

// Разметка списка листков (`combatListMessage`) больше не вызывается из меню
// бота — список открывается в приложении (R1, 2026-11-13). Функция и её
// проверки остаются: `combatListMessage` ещё используют ответы «список листков»
// внутри листка и старый путь «Мои боевые листки» из истории чата.
