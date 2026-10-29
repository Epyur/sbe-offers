/**
 * Стенд маршрутизации бота (Task 1 плана 2026-10-29).
 *
 * Проверяет поведение, которое было в поллере до разнесения на routes и
 * callbacks. Тесты написаны ДО переноса: они должны покраснеть не по своей
 * причине (модуля ещё нет), а после переноса — зелёными, без изменения
 * поведения. Сеть подменяется, чтобы проверять текст ответов и состояние
 * сессии, а не транспорт.
 */
import { afterEach, beforeEach, describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { applyLogin, newSession, type ChatSession } from './session.ts';
import type { BotDeps, RouterState } from './routes.ts';

const REAL_ENV = { ...process.env };
const sent: Array<{ chatId: string; text: string }> = [];

function stubTelegram(): void {
  globalThis.fetch = (async () => new Response('{"ok":true}', { status: 200 })) as typeof fetch;
}

beforeEach(() => {
  sent.length = 0;
  process.env = { ...REAL_ENV, OFFERS_BOT_SECRET: 'test-secret', OFFERS_API_URL: 'http://offers' };
  stubTelegram();
});

afterEach(() => {
  process.env = REAL_ENV;
});

/** Минимальный набор зависимостей: свои send вместо Telegram, память вместо БД. */
/**
 * Минимальный набор зависимостей: свой send вместо Telegram, память вместо
 * Telegram. Маршруты сохраняют сессию через `deps.save`, поэтому тест всегда
 * передаёт `state` — иначе запись уйдёт в общую карту соседнего теста.
 */
function makeDeps(initial?: ChatSession): {
  deps: BotDeps;
  state: RouterState;
  session(): ChatSession;
} {
  const state: RouterState = {
    hits: new Map(),
    lastMessage: new Map(),
    pages: new Map(),
    sessions: new Map(),
  };
  const current = initial ?? newSession();
  const deps: BotDeps = {
    now: () => 1_000_000,
    send: async (chatId: string, text: string) => {
      sent.push({ chatId, text });
    },
    identity: async () => ({ email: '' }),
    // Стенд ведёт себя как sessionRestored: в памяти нет — спрашиваем identity.
    session: async () => {
      const known = state.sessions.get('42');
      if (known) return known;
      const who = await deps.identity('42');
      const restored = who.email ? applyLogin(current, who.email) : current;
      state.sessions.set('42', restored);
      return restored;
    },
    save: (chatId: string, session: ChatSession) => {
      state.sessions.set(chatId, session);
    },
    answerCallback: async () => {
      // В тестах Telegram нет: подтверждение кнопки — пустое действие.
    },
  };
  return {
    deps,
    state,
    session: () => state.sessions.get('42') ?? (initial ?? newSession()),
  };
}

const AUTHED = applyLogin(newSession(), 'ivanov@tn.ru');

describe('вход', () => {
  it('/start до входа просит e-mail', async () => {
    const { deps, state, session } = makeDeps();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/start', state);
    assert.equal(session().step, 'idle');
    assert.match(sent[0].text, /e-mail/);
  });

  it('известный чат восстанавливается без кода', async () => {
    const { deps, state, session } = makeDeps();
    deps.identity = async () => ({ email: 'ivanov@tn.ru' });
    const route = await import('./routes.ts');
    // Незнакомое в памяти сообщение (не /start) проходит восстановление.
    await route.routeText(deps, '42', 'привет', state);
    assert.equal(session().step, 'authed');
    assert.ok(sent.some((m) => /Не понял|Выберите действие/.test(m.text)));
  });
});

describe('защита от повторов', () => {
  it('одинаковое сообщение в пределах 10 секунд глушится', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', 'Не понял что-то', state);
    const before = sent.length;
    await route.routeText(deps, '42', 'Не понял что-то', state);
    assert.equal(sent.length, before);
  });
});

describe('черновик', () => {
  it('undo на пустом черновике отвечает «Убирать нечего»', async () => {
    const { deps, state } = makeDeps({ ...AUTHED, draft: undefined });
    const route = await import('./routes.ts');
    // Черновик без позиций создаётся кнопкой «Собрать КП».
    await route.routeText(deps, '42', '🧾 Собрать КП', state);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'undo', 'cb1', state);
    assert.match(sent[0].text, /Убирать нечего/);

  });

  it('done без клиента спрашивает название клиента', async () => {
    const { deps, state, session } = makeDeps(AUTHED);
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '🧾 Собрать КП', state);
    await route.routeCallback(deps, '42', 'done', 'cb1', state);
    assert.equal(session().draft?.step, 'fields');
    assert.match(sent[sent.length - 1].text, /клиент/i);
  });

  it('неизвестная кнопка не роняет обработчик', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const route = await import('./routes.ts');
    await assert.doesNotReject(async () => {
      await route.routeCallback(deps, '42', 'nonsense', 'cb1', state);
    });
  });
});

describe('лимит частоты', () => {
  it('сверх лимита бот просит подождать', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const route = await import('./routes.ts');
    for (let i = 0; i < 31; i++) {
      await route.routeText(deps, '42', `сообщение ${i}`, state);
    }
    assert.match(sent[sent.length - 1].text, /Слишком много сообщений/);
  });
});
