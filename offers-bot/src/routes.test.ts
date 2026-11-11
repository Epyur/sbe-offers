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
const sent: Array<{ chatId: string; text: string; keyboard?: Record<string, unknown> }> = [];

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
    combatPicker: new Map(),
    lastCompetitor: new Map(),
    foundAnalogs: new Map(),
  };
  const current = initial ?? newSession();
  const deps: BotDeps = {
    now: () => 1_000_000,
    send: async (chatId: string, text: string, keyboard?: Record<string, unknown>) => {
      sent.push({ chatId, text, keyboard });
      // Стенд: id сообщения есть, дальше «⌛» могут заменить результатом.
      return sent.length;
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

  // Живой дефект 2026-11-12: после запроса кода сессия оставалась в idle, и
  // присланный код принимался за e-mail — бот снова просил почту.
  it('после запроса кода шаг переходит в await_code', async () => {
    const { deps, state, session } = makeDeps();
    const route = await import('./routes.ts');
    const calls: string[] = [];
    globalThis.fetch = (async (url: string | URL) => {
      calls.push(String(url));
      return new Response('{"status":"code_sent"}', { status: 200 });
    }) as typeof fetch;

    await route.routeText(deps, '42', 'ivanov@tn.ru', state);

    assert.equal(session().step, 'await_code');
    assert.equal(session().email, 'ivanov@tn.ru');
    assert.ok(calls.some((u) => u.includes('/bot/request-code')));
  });

  it('код из письма не спрашивает почту повторно и входит', async () => {
    const { deps, state, session } = makeDeps();
    const route = await import('./routes.ts');
    globalThis.fetch = (async (url: string | URL) => {
      const u = String(url);
      if (u.includes('/bot/request-code')) return new Response('{"status":"code_sent"}', { status: 200 });
      return new Response('{"email":"ivanov@tn.ru"}', { status: 200 });
    }) as typeof fetch;

    await route.routeText(deps, '42', 'ivanov@tn.ru', state);
    const asked = sent.filter((m) => /Пришлите ваш рабочий e-mail/.test(m.text)).length;
    await route.routeText(deps, '42', '123456', state);

    assert.equal(sent.filter((m) => /Пришлите ваш рабочий e-mail/.test(m.text)).length, asked);
    assert.equal(session().step, 'authed');
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
  hint?: string;
}

/** Хвост списка конкурентов: проверкам поиска и «выбрать всё» нужно больше
 *  одной страницы, а названия — с «pir», как в живой жалобе. */
function manyCompetitors(): Array<{ noteId: number; title: string }> {
  return Array.from({ length: 12 }, (_, i) => ({ noteId: 200 + i, title: `PIRRO продукт ${i + 1}` }));
}

/** Длинный хвост нашего каталога: проверкам листания и «выбрать всё» нужно
 *  больше одной страницы. */
function manyItems(): Array<{ noteId: number; title: string }> {
  return Array.from({ length: 12 }, (_, i) => ({ noteId: 100 + i, title: `Материал ${i + 1}` }));
}

/** Страница списка, как её отдаёт offers-service: offset/limit/total/hasMore. */
function pageOf<T>(items: T[], params: URLSearchParams): { cards: T[]; total: number; offset: number; hasMore: boolean } {
  const offset = Number(params.get('offset') ?? '0') || 0;
  const limit = Number(params.get('limit') ?? '8') || 8;
  const page = items.slice(offset, offset + limit);
  return { cards: page, total: items.length, offset, hasMore: offset + limit < items.length };
}

function fakeCombatServer(): {
  calls: string[];
  current: () => FakeCombat;
  /** Номера заметок, которые сервис отверг при добавлении стороны. */
  failures: () => string[];
} {
  const calls: string[] = [];
  const sideAddFailures: string[] = [];
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
    if (method === 'GET' && url.includes('/bot/combat/systems')) {
      // Поиск и страница (2026-11-10): фильтруем по названию, режем по offset/limit,
      // как настоящий сервис.
      const params = new URL(url).searchParams;
      const q = params.get('q')?.toLowerCase() ?? '';
      const all = [{ noteId: 10, title: 'Кровля' }, { noteId: 11, title: 'Фасад' }, ...manyItems()];
      const filtered = q ? all.filter((c) => c.title.toLowerCase().includes(q)) : all;
      return json(pageOf(filtered, params));
    }
    if (method === 'GET' && url.includes('/bot/combat/competitors')) {
      const params = new URL(url).searchParams;
      const q = params.get('q')?.toLowerCase() ?? '';
      const all = [{ noteId: 30, title: 'Конкурент А' }, ...manyCompetitors()];
      const filtered = q ? all.filter((c) => c.title.toLowerCase().includes(q)) : all;
      return json(pageOf(filtered, params));
    }
    if (method === 'GET' && url.includes('/bot/combat?') ) {
      return json({ sheets: sheets.map((s) => ({ ...s, ownerEmail: '', title: '', offerId: 0, formValues: {}, status: 'draft', createdAt: '', updatedAt: '' })) });
    }
    const hintPut = url.match(/\/bot\/combat\/(\d+)\/hint$/);
    if (method === 'PUT' && hintPut) {
      const sheet = sheets.find((sh) => sh.id === Number(hintPut[1]));
      if (!sheet) return json({ error: 'нет листка' }, 404);
      const body = JSON.parse(String(init?.body ?? '{}')) as { hint?: string };
      sheet.hint = String(body.hint ?? '');
      return json({ ...sheet, ownerEmail: '', title: '', offerId: 0, formValues: { hint: sheet.hint }, status: 'draft', createdAt: '', updatedAt: '' });
    }
    if (method === 'GET' && url.includes('/bot/combat/analogues')) {
      const noteId = Number(new URL(url).searchParams.get('noteId') ?? '0');
      // Два аналога: проверкам нужен и выбор одного, и «добавить все».
      return json({
        cards: [
          { noteId: 500 + noteId, title: `Аналог ${noteId}-A` },
          { noteId: 600 + noteId, title: `Аналог ${noteId}-B` },
        ],
      });
    }
    const sideAdd = url.match(/\/bot\/combat\/(\d+)\/(our|competitor)$/);
    if (method === 'POST' && sideAdd) {
      const sheet = sheets.find((s) => s.id === Number(sideAdd[1]));
      const body = JSON.parse(String(init?.body ?? '{}')) as { noteId?: number; title?: string };
      if (!sheet) return json({ error: 'нет листка' }, 404);
      // Номер заметки — обязателен: листок хранит сторону по заметке.
      if (!Number.isFinite(body.noteId) || Number(body.noteId) <= 0) {
        sideAddFailures.push(String(body.noteId));
        return json({ error: 'not found' }, 404);
      }
      const target = sideAdd[2] === 'our' ? sheet.ourNotes : sheet.competitorNotes;
      const noteId = Number(body.noteId);
      if (!target.some((n) => n.noteId === noteId)) {
        target.push({ noteId, title: String(body.title ?? '') });
      }
      return json({ ...sheet, ownerEmail: '', title: '', offerId: 0, formValues: {}, status: 'draft', createdAt: '', updatedAt: '' });
    }
    return json({ ok: true });
  }) as typeof fetch;

  return {
    calls,
    current: () => sheets[sheets.length - 1],
    failures: () => sideAddFailures,
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

  it('«Наш продукт» просит название, ищем и добавляем выбранное', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const server = fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'cb:oursys', 'cb1', state);
    assert.match(sent[0].text, /Введите название/);
    // Текст — это запрос; находим «кровлю» и выбираем.
    sent.length = 0;
    await route.routeText(deps, '42', 'кровля', state);
    assert.match(sent[0].text, /Нашлось по «кровля»/);
    // Номер строки переключает отметку, а не добавляет сразу (2026-11-10).
    await route.routeCallback(deps, '42', 'oursel:0', 'cb2', state);
    assert.match(sent[sent.length - 1].text, /Кровля/);
    assert.equal(server.current().ourNotes.length, 0, 'отметка не добавляет сторону');
    sent.length = 0;
    await route.routeCallback(deps, '42', 'cb:addpicked', 'cb3', state);
    assert.match(sent[sent.length - 1].text, /Кровля/);
    assert.equal(server.current().ourNotes.length, 1);
    assert.equal(server.current().ourNotes[0].noteId, 10);
  });

  it('«▼ Все продукты» показывает весь список стороны', async () => {
    const { deps, state } = makeDeps(AUTHED);
    fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    await route.routeCallback(deps, '42', 'cb:oursys', 'cb1', state);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'cb:all', 'cb2', state);
    assert.match(sent[0].text, /Всего: 14/);
  });

  it('«Добавить конкурента» ищет по названию и добавляет выбранного', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const server = fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    await route.routeCallback(deps, '42', 'cb:comp', 'cb1', state);
    sent.length = 0;
    await route.routeText(deps, '42', 'конкурент', state);
    await route.routeCallback(deps, '42', 'compsel:0', 'cb2', state);
    await route.routeCallback(deps, '42', 'cb:addpicked', 'cb3', state);
    assert.match(sent[sent.length - 1].text, /Конкурент А/);
    assert.equal(server.current().competitorNotes.length, 1);
  });

  it('можно добавить несколько сторон с каждой стороны', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const server = fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    // Два наших продукта: ищем, отмечаем, добавляем (2026-11-10).
    await route.routeCallback(deps, '42', 'cb:oursys', 'c1', state);
    await route.routeText(deps, '42', 'кровля', state);
    await route.routeCallback(deps, '42', 'oursel:0', 'c2', state);
    await route.routeCallback(deps, '42', 'cb:addpicked', 'c3', state);
    await route.routeCallback(deps, '42', 'cb:oursys', 'c4', state);
    await route.routeText(deps, '42', 'фасад', state);
    await route.routeCallback(deps, '42', 'oursel:0', 'c5', state);
    await route.routeCallback(deps, '42', 'cb:addpicked', 'c6', state);
    assert.equal(server.current().ourNotes.length, 2);
  });

  it('несколько отмеченных строк добавляются одной кнопкой', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const server = fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    await route.routeCallback(deps, '42', 'cb:oursys', 'c1', state);
    // «▼ Все» — обе стороны в одном списке, отметить надо обе.
    await route.routeCallback(deps, '42', 'cb:all', 'c2', state);
    await route.routeCallback(deps, '42', 'oursel:0', 'c3', state);
    await route.routeCallback(deps, '42', 'oursel:1', 'c4', state);
    assert.equal(server.current().ourNotes.length, 0, 'отметки ещё не добавили стороны');
    sent.length = 0;
    await route.routeCallback(deps, '42', 'cb:addpicked', 'c5', state);
    assert.equal(server.current().ourNotes.length, 2);
    assert.deepEqual(server.current().ourNotes.map((n) => n.noteId), [10, 11]);
    assert.match(sent[sent.length - 1].text, /Кровля, Фасад/);
  });

  it('«Выбрать все найденные» берёт совпадения со всех страниц', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const server = fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    // Запрос сужает до списка длиннее одной страницы: «Кровля» плюс много позиций.
    await route.routeCallback(deps, '42', 'cb:oursys', 'c1', state);
    await route.routeCallback(deps, '42', 'cb:all', 'c2', state);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'cb:pickall', 'c3', state);
    // Кнопка «Добавить» теперь считает всё найденное, а не строку экрана.
    assert.equal(state.combatPicker.get('42')?.allPicked, true);
    await route.routeCallback(deps, '42', 'cb:addpicked', 'c4', state);
    assert.equal(server.current().ourNotes.length, 2 + manyItems().length);
  });

  it('поиск + «Выбрать все найденные» + «Добавить» кладёт всех конкурентов', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const server = fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    await route.routeCallback(deps, '42', 'cb:comp', 'c1', state);
    // Живой сценарий жалобы: набрали «pir», взяли все найденные, добавили.
    await route.routeText(deps, '42', 'pir', state);
    await route.routeCallback(deps, '42', 'cb:pickall', 'c2', state);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'cb:addpicked', 'c3', state);
    assert.deepEqual(server.failures(), [], 'сервис отверг часть сторон');
    assert.ok(server.current().competitorNotes.length > 0, 'конкуренты не добавились');
    assert.ok(server.current().competitorNotes.every((n) => n.noteId > 0));
  });

  it('отметка только номерами строк тоже доходит до листка', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const server = fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    await route.routeCallback(deps, '42', 'cb:comp', 'c1', state);
    await route.routeText(deps, '42', 'pir', state);
    // «pir» находит и «Конкурент А»? Нет: только PIRRO-хвост. Первая строка —
    // первый PIRRO, и именно она должна доехать до листка.
    await route.routeCallback(deps, '42', 'compsel:0', 'c2', state);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'cb:addpicked', 'c3', state);
    assert.deepEqual(server.failures(), []);
    assert.equal(server.current().competitorNotes.length, 1);
    assert.equal(server.current().competitorNotes[0].noteId, 200);
  });

  it('«Снять все отметки» возвращает пустой выбор', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const server = fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    await route.routeCallback(deps, '42', 'cb:oursys', 'c1', state);
    await route.routeCallback(deps, '42', 'cb:all', 'c2', state);
    await route.routeCallback(deps, '42', 'cb:pickall', 'c3', state);
    await route.routeCallback(deps, '42', 'cb:unpickall', 'c4', state);
    assert.equal(state.combatPicker.get('42')?.allPicked, false);
    await route.routeCallback(deps, '42', 'cb:addpicked', 'c5', state);
    assert.equal(server.current().ourNotes.length, 0);
  });

  it('повторное нажатие номера снимает отметку', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const server = fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    await route.routeCallback(deps, '42', 'cb:oursys', 'c1', state);
    await route.routeCallback(deps, '42', 'cb:all', 'c2', state);
    await route.routeCallback(deps, '42', 'oursel:0', 'c3', state);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'oursel:0', 'c4', state);
    // Кнопка расчерчена без «✓» на первой строке: отметка снята.
    assert.ok(state.combatPicker.get('42')?.picked.length === 0);
    await route.routeCallback(deps, '42', 'cb:addpicked', 'c5', state);
    assert.equal(server.current().ourNotes.length, 0);
  });

  it('отметка не слетает при «Дальше» и «Все продукты», выбор работает дальше', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const server = fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    await route.routeCallback(deps, '42', 'cb:oursys', 'c1', state);
    await route.routeCallback(deps, '42', 'cb:all', 'c2', state);
    // Отметить первую строку.
    await route.routeCallback(deps, '42', 'oursel:0', 'c3', state);
    assert.deepEqual(state.combatPicker.get('42')?.picked, [0]);
    // «Дальше» — отметка и запрос остаются прежними.
    await route.routeCallback(deps, '42', 'cb:more', 'c4', state);
    assert.deepEqual(state.combatPicker.get('42')?.picked, [0], 'перелистывание сбрасывает отметку');
    assert.equal(state.combatPicker.get('42')?.query, '');
    // «▼ Все продукты» — это тот же полный список, а не новый поиск: отметка жива.
    await route.routeCallback(deps, '42', 'cb:all', 'c5', state);
    assert.deepEqual(state.combatPicker.get('42')?.picked, [0], '«Все продукты» сбрасывает отметку');
    // И выбор строки после этого работает, а не отвечает «список устарел».
    sent.length = 0;
    await route.routeCallback(deps, '42', 'oursel:1', 'c6', state);
    assert.ok(!/устарел/.test(sent[sent.length - 1].text), 'выбор после листания сломан');
    assert.deepEqual(state.combatPicker.get('42')?.picked, [0, 1]);
  });

  it('страница выбора сохраняет главное меню боевого листка', async () => {
    const { deps, state } = makeDeps(AUTHED);
    fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'cb:oursys', 'c1', state);
    await route.routeCallback(deps, '42', 'cb:all', 'c2', state);
    const kb = sent[sent.length - 1].keyboard as {
      inline_keyboard: Array<Array<{ callback_data: string }>>;
    };
    const ids = kb.inline_keyboard.flat().map((b) => b.callback_data);
    for (const want of ['cb:oursys', 'cb:comp', 'cb:doc', 'cb:done']) {
      assert.ok(ids.includes(want), `на странице выбора нет кнопки ${want}`);
    }
  });

  it('листание зависит от общего числа совпадений', async () => {
    const { deps, state } = makeDeps(AUTHED);
    fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    await route.routeCallback(deps, '42', 'cb:oursys', 'c1', state);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'cb:all', 'c2', state);
    // Список из двух строк помещается на одну страницу — «Дальше» нет.
    assert.ok(!/Дальше/.test(sent[0].text));
    // Смещение растёт от нажатия, даже если строк на странице меньше.
    await route.routeCallback(deps, '42', 'cb:more', 'c3', state);
    assert.equal(state.combatPicker.get('42')?.offset, 8);
  });

  it('«Уточнение для разбора» сохраняет акцент и не добавляет стороны', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const server = fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    await route.routeCallback(deps, '42', 'cb:hint', 'c1', state);
    sent.length = 0;
    await route.routeText(deps, '42', 'огнестойкость', state);
    assert.equal(server.current().hint, 'огнестойкость');
    assert.equal(server.current().ourNotes.length, 0);
    assert.match(sent[sent.length - 1].text, /Уточнение сохранено/);
  });

  it('точка снимает уточнение', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const server = fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    await route.routeCallback(deps, '42', 'cb:hint', 'c1', state);
    await route.routeText(deps, '42', 'срок службы', state);
    await route.routeCallback(deps, '42', 'cb:hint', 'c2', state);
    await route.routeText(deps, '42', '.', state);
    assert.equal(server.current().hint, '');
  });

  it('«Найти аналоги и сравнить» подбирает конкурентов к нашему продукту', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const server = fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    // Наш продукт выбран, конкурентов нет — ровно листок 005 из жалобы.
    await route.routeCallback(deps, '42', 'cb:oursys', 'c1', state);
    await route.routeCallback(deps, '42', 'cb:all', 'c2', state);
    await route.routeCallback(deps, '42', 'oursel:0', 'c3', state);
    await route.routeCallback(deps, '42', 'cb:addpicked', 'c4', state);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'cb:analog', 'c5', state);
    const text = sent[sent.length - 1].text;
    assert.ok(!/Наш продукт»|не нашлось|Сначала нажмите/.test(text), `подбор не сработал: ${text}`);
    assert.match(text, /Конкуренты, похожие на наш/);
    // Подобранный продукт добавляется на сторону КОНКУРЕНТА.
    await route.routeCallback(deps, '42', 'cban:510', 'c6', state);
    assert.ok(
      server.current().competitorNotes.some((n) => n.noteId === 510),
      'аналог не добавлен как конкурент',
    );
    assert.equal(server.current().ourNotes.length, 1, 'наш продукт не должен меняться');
  });

  it('«Добавить все найденные» кладёт в листок всех подобранных конкурентов', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const server = fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    await route.routeCallback(deps, '42', 'cb:oursys', 'c1', state);
    await route.routeCallback(deps, '42', 'cb:all', 'c2', state);
    await route.routeCallback(deps, '42', 'oursel:0', 'c3', state);
    await route.routeCallback(deps, '42', 'cb:addpicked', 'c4', state);
    await route.routeCallback(deps, '42', 'cb:analog', 'c5', state);
    // Заглушка отдаёт один аналог — добавим его и проверим, что сторона конкурента.
    const found = state.foundAnalogs.get('42') ?? [];
    assert.ok(found.length > 1, 'нужно больше одного аналога для проверки «всех»');
    sent.length = 0;
    await route.routeCallback(deps, '42', 'cban:all', 'c6', state);
    assert.equal(server.current().competitorNotes.length, found.length);
  });

  it('без нашего продукта подбор объясняет, что выбрать', async () => {
    const { deps, state } = makeDeps(AUTHED);
    fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'cb:analog', 'c1', state);
    assert.match(sent[sent.length - 1].text, /Наш продукт/);
  });

  it('аналог подбирается и по конкуренту из листка (после перезапуска бота)', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const server = fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    // Конкурента выбрали и добавили — дальше память разговора забыта: так
    // выглядит листок, открытый заново.
    await route.routeCallback(deps, '42', 'cb:comp', 'c1', state);
    await route.routeText(deps, '42', 'конкурент', state);
    await route.routeCallback(deps, '42', 'compsel:0', 'c2', state);
    await route.routeCallback(deps, '42', 'cb:addpicked', 'c3', state);
    state.lastCompetitor.delete('42');
    sent.length = 0;
    await route.routeCallback(deps, '42', 'cb:analog', 'c4', state);
    assert.ok(
      !/Добавить конкурента/.test(sent[sent.length - 1].text),
      'бот не нашёл конкурента, хотя он есть в листке',
    );
    assert.match(sent[sent.length - 1].text, /аналоги/i);
    assert.ok(server.current().competitorNotes.length > 0);
  });

  it('кнопки листка работают после перезапуска бота (сессия восстановлена без combat)', async () => {
    const { deps, state } = makeDeps(AUTHED);
    const server = fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    await route.routeCallback(deps, '42', 'cb:comp', 'c1', state);
    await route.routeText(deps, '42', 'конкурент', state);
    await route.routeCallback(deps, '42', 'compsel:0', 'c2', state);
    await route.routeCallback(deps, '42', 'cb:addpicked', 'c3', state);
    // Имитация перезапуска: сессия восстановлена по привязке, листка в ней нет.
    state.sessions.set('42', { step: 'authed', email: 'm@tn.ru' });
    state.lastCompetitor.delete('42');
    sent.length = 0;
    await route.routeCallback(deps, '42', 'cb:analog', 'c4', state);
    const last = sent[sent.length - 1].text;
    assert.ok(!/не открыт|Боевых листков пока нет/.test(last), `листок не восстановлен: ${last}`);
    assert.match(last, /аналоги/i);
    assert.ok(server.current().competitorNotes.length > 0);
    // И сам листок снова открыт в сессии — дальше по нему работают кнопки.
    assert.ok((state.sessions.get('42')?.combat?.sheetId ?? 0) > 0);
  });

  it('текст в боевом листке не уходит в черновик КП', async () => {
    const { deps, state } = makeDeps(AUTHED);
    fakeCombatServer();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '/combat', state);
    sent.length = 0;
    // Без выбора стороны текст объясняет, что делать, и в КП не попадает.
    await route.routeText(deps, '42', 'Монтаж | 12 | 850', state);
    assert.match(sent[0].text, /выбираются кнопками/);
    const sess = state.sessions.get('42');
    assert.equal(sess?.draft, undefined);
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
