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
    foundSystems: new Map(),
    foundCombat: new Map(),
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
    // С 2026-11-01 «Собрать КП» открывает разговор о контексте, а не каталог:
    // «Готово» до этапа позиций — это «пропустить остаток брифа».
    await route.routeCallback(deps, '42', 'done', 'cb1', state);
    assert.equal(session().draft?.stage, 'positions');
    sent.length = 0;
    await route.routeCallback(deps, '42', 'done', 'cb2', state);
    assert.match(sent[sent.length - 1].text, /клиент/i);
  });

  it('«Собрать КП» начинается с контекста, а не с каталога', async () => {
    const { deps, state, session } = makeDeps(AUTHED);
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '🧾 Собрать КП', state);
    assert.equal(session().draft?.stage, 'context', 'старт разговора — этап контекста');
    assert.match(sent[0].text, /объект/i, 'первый вопрос — про объект');
    assert.ok(
      sent.every((m) => !/Добавьте позици/i.test(m.text)),
      'на старте бот не просит позиции',
    );
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

// --- Боевой листок в боте (2026-11-09) ---------------------------------
//
// Сеть подменяется подменным offers-service: fetch отвечает JSON по адресу.
// Telegram идёт через node:https, а не fetch, поэтому подмена не задевает его —
// но DOCX по сети здесь и не собираем (пустой листок), так что сеть не нужна.

interface FakeCombat {
  id: number;
  number: string;
  ourNotes: Array<{ noteId: number; title: string }>;
  competitorNotes: Array<{ noteId: number; title: string }>;
}

function fakeCombatServer(): { calls: string[]; current: () => FakeCombat } {
  const calls: string[] = [];
  let nextId = 1;
  const sheets: FakeCombat[] = [];

  globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    const method = (init?.method ?? 'GET').toUpperCase();
    calls.push(`${method} ${url}`);
    const json = (body: unknown, status = 200) =>
      new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } });

    if (method === 'POST' && url.endsWith('/bot/combat')) {
      const sheet: FakeCombat = { id: nextId++, number: `00${nextId - 1}/2026`, ourNotes: [], competitorNotes: [] };
      sheets.push(sheet);
      return json({ ...sheet, ownerEmail: '', title: '', offerId: 0, formValues: {}, status: 'draft', createdAt: '', updatedAt: '' });
    }
    if (method === 'GET' && url.endsWith('/bot/combat/systems')) {
      return json({ cards: [{ noteId: 10, title: 'Кровля' }, { noteId: 11, title: 'Фасад' }] });
    }
    if (method === 'GET' && url.endsWith('/bot/combat/competitors')) {
      return json({ cards: [{ noteId: 20, title: 'Конкурент А' }] });
    }
    if (method === 'GET' && url.includes('/bot/combat?') ) {
      return json({ sheets: sheets.map((s) => ({ ...s, ownerEmail: '', title: '', offerId: 0, formValues: {}, status: 'draft', createdAt: '', updatedAt: '' })) });
    }
    const sideAdd = url.match(/\/bot\/combat\/(\d+)\/(our|competitor)$/);
    if (method === 'POST' && sideAdd) {
      const sheet = sheets.find((s) => s.id === Number(sideAdd[1]));
      const body = JSON.parse(String(init?.body ?? '{}')) as { noteId: number; title: string };
      if (!sheet) return json({ error: 'нет листка' }, 404);
      const target = sideAdd[2] === 'our' ? sheet.ourNotes : sheet.competitorNotes;
      if (!target.some((n) => n.noteId === body.noteId)) target.push({ noteId: body.noteId, title: body.title });
      return json({ ...sheet, ownerEmail: '', title: '', offerId: 0, formValues: {}, status: 'draft', createdAt: '', updatedAt: '' });
    }
    return json({ ok: true });
  }) as typeof fetch;

  return {
    calls,
    current: () => sheets[sheets.length - 1],
  };
}

describe('боевой листок в боте', () => {
  it('«Боевой листок» создаёт листок и показывает приветствие', async () => {
    const { deps, state } = makeDeps(AUTHED);
    fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '⚔️ Боевой листок', state);
    assert.match(sent[0].text, /Боевой листок № 001\/2026/);
    // В сессии запомнен листок: дальше кнопки пишут в него.
    assert.equal(session(state)?.combat?.sheetId, 1);
  });

  it('«Наша система» показывает список и добавляет выбранную', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const server = fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'cb:oursys', 'cb1', state);
    assert.match(sent[0].text, /Выберите нашу систему/);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'oursel:0', 'cb2', state);
    assert.match(sent[sent.length - 1].text, /Кровля/);
    assert.equal(server.current().ourNotes.length, 1);
    assert.equal(server.current().ourNotes[0].noteId, 10);
  });

  it('«Добавить конкурента» показывает список и добавляет выбранного', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const server = fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    await route.routeCallback(deps, '42', 'cb:comp', 'cb1', state);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'compsel:0', 'cb2', state);
    assert.match(sent[sent.length - 1].text, /Конкурент А/);
    assert.equal(server.current().competitorNotes.length, 1);
  });

  it('текст в боевом листке не уходит в черновик КП', async () => {
    const { deps, state } = makeDeps(AUTHED);
    fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    sent.length = 0;
    await route.routeText(deps, '42', 'Монтаж | 12 | 850', state);
    assert.match(sent[0].text, /В боевом листке стороны выбираются кнопками/);
  });

  it('«Готово» закрывает листок и возвращает к меню', async () => {
    const { deps, state } = makeDeps(AUTHED);
    fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'cb:done', 'cb1', state);
    assert.match(sent[0].text, /закрыт/);
    const sess = state.sessions.get('42');
    assert.equal(sess?.combat, undefined);
  });

  it('«Мои боевые листки» пусто — честное сообщение', async () => {
    const { deps, state } = makeDeps(AUTHED);
    // Пустой ответ списка: ни одного листка.
    globalThis.fetch = (async () =>
      new Response(JSON.stringify({ sheets: [] }), { status: 200 })) as typeof fetch;
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '📂 Мои боевые листки', state);
    assert.match(sent[0].text, /Боевых листков пока нет/);
  });
});

/** Сессия боевого листка в карте состояния (для проверок). */
function session(state: RouterState): ChatSession | undefined {
  return state.sessions.get('42');
}
