/**
 * «Создать новый документ» (2026-11-13, R2).
 *
 * Проверяется поведение, которое видит человек: одна кнопка спрашивает, что
 * собрать; первым вопросом КП идёт «какой продукт или систему вы хотите
 * предложить»; найденное показывается списком; срабатывает развилка «собрать
 * контекст в чате» или «продолжить в приложении»; пустой поиск не тупик.
 *
 * Сеть подменяется подставным offers-service: ответы строятся по URL, чтобы
 * проверять тексты и состояние, а не транспорт.
 */
import { afterEach, beforeEach, describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { applyLogin, newSession, type ChatSession } from './session.ts';
import type { BotDeps, RouterState } from './routes.ts';
import { addedMessage, moreKeyboard } from './flow/newdoc.ts';

const REAL_ENV = { ...process.env };
const sent: Array<{ chatId: string; text: string; keyboard?: Record<string, unknown> }> = [];
const edits: Array<{ chatId: string; messageId: number; text: string; keyboard?: Record<string, unknown> }> = [];

beforeEach(() => {
  sent.length = 0;
  process.env = { ...REAL_ENV, OFFERS_BOT_SECRET: 'test-secret', OFFERS_API_URL: 'http://offers' };
});

afterEach(() => {
  process.env = REAL_ENV;
  globalThis.fetch = (async () => new Response('{"ok":true}', { status: 200 })) as typeof fetch;
});

const AUTHED = applyLogin(newSession(), 'ivanov@tn.ru');

function makeDeps(): { deps: BotDeps; state: RouterState; session(): ChatSession | undefined } {
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
  state.sessions.set('42', { ...AUTHED });
  edits.length = 0;
  const deps: BotDeps = {
    now: () => 1_000_000,
    send: async (chatId, text, keyboard) => {
      sent.push({ chatId, text, keyboard });
      return sent.length;
    },
    // Замена «⌛» результатом (как в poller.ts): без неё проверка «⌛ сменяется
    // ответом» не отличила бы правку от второго сообщения.
    edit: async (chatId, messageId, text, keyboard) => {
      edits.push({ chatId, messageId, text, keyboard });
    },
    identity: async () => ({ email: 'ivanov@tn.ru' }),
    session: async () => state.sessions.get('42') ?? { ...AUTHED },
    save: (chatId, session) => state.sessions.set(chatId, session),
    answerCallback: async () => {},
  };
  return { deps, state, session: () => state.sessions.get('42') };
}

/** Подставной сервис: поиск отдаёт две системы, создание — строку документа. */
function fakeService(opts: { systems?: number; failSearch?: string } = {}): void {
  const systems = opts.systems ?? 2;
  globalThis.fetch = (async (input: string | URL | Request) => {
    const url = String(input);
    const json = (body: unknown): Response =>
      new Response(JSON.stringify(body), { status: 200, headers: { 'content-type': 'application/json' } });
    if (url.includes('/bot/suggest')) {
      if (opts.failSearch) return json({ systems: [], materials: [], error: opts.failSearch });
      const rows = Array.from({ length: systems }, (_, i) => ({
        noteId: 100 + i,
        sku: i === 0 ? '' : `TN-${i}`,
        title: i === 0 ? 'ТН-КРОВЛЯ Гарант Плюс' : 'ТН-КРОВЛЯ Гарант RE30',
      }));
      return json({ systems: rows, materials: [] });
    }
    if (url.endsWith('/bot/offers') && String((input as Request).method ?? '') !== 'GET') {
      return json({ id: 77, number: '007/2026' });
    }
    if (url.includes('/bot/offers')) return json({ id: 77, number: '007/2026', systems: [] });
    if (url.includes('/bot/combat')) return json({ id: 55, number: '005/2026' });
    if (url.includes('/bot/offers/')) return json({ id: 77, number: '007/2026', systems: [] });
    return json({ ok: true });
  }) as typeof fetch;
}

/** Все кнопки клавиатуры ответа: подпись и то, чем она является. */
function buttons(keyboard: Record<string, unknown> | undefined): Array<Record<string, unknown>> {
  const rows = (keyboard?.inline_keyboard ?? keyboard?.keyboard ?? []) as Array<Array<Record<string, unknown>>>;
  return rows.flat();
}

/**
 * Дождаться, пока карты маршрута сохранятся.
 *
 * `state.newDoc`/сессия пишутся не сразу: `apply` откладывает сохранение до
 * конца обработчика, а тест зовёт следующий сразу. Без паузы следующий вызов
 * читает состояние ДО нажатия и отвечает «список устарел».
 */
async function settle(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0));
}

describe('«Создать новый документ» (R2)', () => {
  it('кнопка спрашивает, что собираем, и даёт ровно два вида', async () => {
    const { deps, state } = makeDeps();
    const route = await import('./routes.ts');
    await route.routeText(deps, '42', '✨ Создать новый документ', state);

    assert.equal(sent.length, 1);
    assert.match(sent[0].text, /Что создаём/);
    const labels = buttons(sent[0].keyboard).map((b) => String(b.text));
    assert.deepEqual(labels, ['🧾 Коммерческое предложение', '⚔️ Боевой листок']);
  });

  it('первый вопрос КП дословно из заявки', async () => {
    const { deps, state } = makeDeps();
    const route = await import('./routes.ts');
    await route.routeCallback(deps, '42', 'nd:kind:offer', 'cb1', state);

    assert.match(sent[0].text, /Какой продукт или систему вы хотите предложить покупателю\?/);
    assert.equal(state.sessions.get('42')?.newDoc, 'offer');
  });

  it('название продукта идёт в поиск, найденное — списком с номерами', async () => {
    const { deps, state } = makeDeps();
    fakeService({ systems: 2 });
    const route = await import('./routes.ts');
    await route.routeCallback(deps, '42', 'nd:kind:offer', 'cb1', state);
    sent.length = 0;
    await route.routeText(deps, '42', 'Гарант', state);

    assert.equal(sent.length, 1);
    assert.match(sent[0].text, /нашлись системы \(2\)/);
    assert.match(sent[0].text, /1\. ТН-КРОВЛЯ Гарант Плюс/);
    assert.match(sent[0].text, /2\. ТН-КРОВЛЯ Гарант RE30 \(TN-1\)/);
    const picks = buttons(sent[0].keyboard).filter((b) => String(b.callback_data ?? '').startsWith('nd:pick:'));
    assert.equal(picks.length, 2, 'на каждый найденный — своя кнопка-номер');
    // Найденное лежит в СВОЕЙ карте, а не в подсказке по свободной фразе.
    assert.equal(state.newDoc.get('42')?.items.length, 2);
    assert.equal(state.foundSuggest.get('42'), undefined);
  });

  it('«ничего не нашлось» — не тупик: уточнить или в приложении', async () => {
    const { deps, state } = makeDeps();
    fakeService({ systems: 0 });
    const route = await import('./routes.ts');
    await route.routeCallback(deps, '42', 'nd:kind:offer', 'cb1', state);
    sent.length = 0;
    await route.routeText(deps, '42', 'нетакого', state);

    assert.match(sent[0].text, /По запросу «нетакого» в Базе знаний ничего не нашлось/);
    const all = buttons(sent[0].keyboard);
    assert.ok(
      all.some((b) => b.callback_data === 'nd:again'),
      'должна быть кнопка «уточнить название»',
    );
    assert.ok(
      all.some((b) => Boolean(b.web_app)),
      'должна быть кнопка «в приложении»',
    );
    // Список поиска остаётся пустым: старый не должен подмениться новым.
    assert.equal(state.newDoc.get('42')?.items.length, 0);
  });

  it('недоступный поиск не выдумывает ответ и оставляет выход в приложение', async () => {
    const { deps, state } = makeDeps();
    fakeService({ failSearch: 'База знаний не настроена' });
    const route = await import('./routes.ts');
    await route.routeCallback(deps, '42', 'nd:kind:offer', 'cb1', state);
    sent.length = 0;
    await route.routeText(deps, '42', 'Гарант', state);

    assert.match(sent[0].text, /Поиск по Базе знаний сейчас недоступен/);
    assert.ok(buttons(sent[0].keyboard).some((b) => Boolean(b.web_app)));
  });

  it('одна отметка не пишет сразу: сначала «Добавить выбранные»', async () => {
    const { deps, state } = makeDeps();
    fakeService({ systems: 2 });
    const route = await import('./routes.ts');
    await route.routeCallback(deps, '42', 'nd:kind:offer', 'cb1', state);
    await route.routeText(deps, '42', 'Гарант', state);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'nd:pick:1', 'cb2', state);

    assert.match(sent[0].text, /Отмечено: «ТН-КРОВЛЯ Гарант RE30»/);
    assert.match(sent[0].text, /Добавить этот продукт в документ/);
    const all = buttons(sent[0].keyboard);
    assert.ok(all.some((b) => b.callback_data === 'nd:addpicked'));
  });

  it('«Добавить выбранные» пишет отмеченное и спрашивает, нужно ли ещё', async () => {
    const { deps, state } = makeDeps();
    fakeService({ systems: 2 });
    const route = await import('./routes.ts');
    await route.routeCallback(deps, '42', 'nd:kind:offer', 'cb1', state);
    await route.routeText(deps, '42', 'Гарант', state);
    await route.routeCallback(deps, '42', 'nd:pick:1', 'cb2', state);
    sent.length = 0;
    edits.length = 0;
    await route.routeCallback(deps, '42', 'nd:addpicked', 'cb3', state);

    // Ожидание видно СРАЗУ: «⌛» уходит до записи, а не через полминуты.
    assert.match(sent[0].text, /⌛ Записываю систему «ТН-КРОВЛЯ Гарант RE30»/);
    assert.match(sent[0].text, /Повторно нажимать не нужно/);
    // И заменяется результатом — чат не копится служебными строками.
    const last = edits[edits.length - 1];
    assert.ok(last, 'результат должен заменять «⌛», а не идти новым сообщением');
    assert.match(last.text, /Системы: «ТН-КРОВЛЯ Гарант RE30»/);
    assert.match(last.text, /Нужно добавить ещё/);
    const all = buttons(last.keyboard);
    assert.ok(all.some((b) => b.callback_data === 'nd:more'));
    assert.ok(all.some((b) => b.callback_data === 'nd:enough'));
    assert.ok(all.some((b) => b.callback_data === 'nd:app'));
  });

  // Живая жалоба 2026-11-14: запись идёт десятки секунд, человек жмёт кнопку
  // второй раз и получает «Ничего не отмечено» дважды подряд.
  it('повторное нажатие «Добавить выбранные» не пишет второй раз и не пугает', async () => {
    const { deps, state } = makeDeps();
    fakeService({ systems: 2 });
    const route = await import('./routes.ts');
    await route.routeCallback(deps, '42', 'nd:kind:offer', 'cb1', state);
    await route.routeText(deps, '42', 'Гарант', state);
    await route.routeCallback(deps, '42', 'nd:pick:1', 'cb2', state);
    await route.routeCallback(deps, '42', 'nd:addpicked', 'cb3', state);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'nd:addpicked', 'cb4', state);

    assert.equal(sent.length, 1);
    assert.match(sent[0].text, /В предложение уже записано: «ТН-КРОВЛЯ Гарант RE30»/);
    assert.doesNotMatch(sent[0].text, /Ничего не отмечено/);
    const all = buttons(sent[0].keyboard);
    assert.ok(all.some((b) => b.callback_data === 'nd:more'), 'развилка должна остаться на месте');
  });

  it('несколько систем подряд: «добавить ещё» сохраняет прежний выбор', async () => {
    const { deps, state } = makeDeps();
    fakeService({ systems: 2 });
    const route = await import('./routes.ts');
    await route.routeCallback(deps, '42', 'nd:kind:offer', 'cb1', state);
    await route.routeText(deps, '42', 'Гарант', state);
    await route.routeCallback(deps, '42', 'nd:pick:0', 'cb2', state);
    await route.routeCallback(deps, '42', 'nd:addpicked', 'cb3', state);
    await route.routeCallback(deps, '42', 'nd:more', 'cb4', state);
    sent.length = 0;
    // Второй запрос — другими словами: два одинаковых сообщения подряд бот
    // считает повтором (окно 10 с) и второй поиск не выполняет.
    await route.routeText(deps, '42', 'Гарант RE30', state);
    await route.routeCallback(deps, '42', 'nd:pick:1', 'cb5', state);

    // Второй продукт пишется в ТО ЖЕ предложение, а сводка помнит первый.
    assert.match(sent[0].text, /ТН-КРОВЛЯ Гарант Плюс/);
    assert.match(sent[0].text, /ТН-КРОВЛЯ Гарант RE30/);
  });

  it('отметка нескольких номеров даёт кнопку «Добавить выбранные»', async () => {
    const { deps, state } = makeDeps();
    fakeService({ systems: 2 });
    const route = await import('./routes.ts');
    await route.routeCallback(deps, '42', 'nd:kind:offer', 'cb1', state);
    await route.routeText(deps, '42', 'Гарант', state);
    await route.routeCallback(deps, '42', 'nd:pick:0', 'cb2', state);
    await route.routeCallback(deps, '42', 'nd:addpicked', 'cb3', state);
    await route.routeCallback(deps, '42', 'nd:more', 'cb4', state);
    sent.length = 0;
    await route.routeText(deps, '42', 'Гарант RE30', state);
    await route.routeCallback(deps, '42', 'nd:pick:0', 'cb5', state);
    await route.routeCallback(deps, '42', 'nd:pick:1', 'cb6', state);

    const marked = sent[sent.length - 1];
    assert.match(marked.text, /Отмечено: 2/);
    const all = buttons(marked.keyboard);
    assert.ok(all.some((b) => b.callback_data === 'nd:addpicked'));
    const labels = all.map((b) => String(b.text));
    assert.ok(labels.some((l) => l.startsWith('✓ 1')));
    assert.ok(labels.some((l) => l.startsWith('✓ 2')));
  });

  it('«Собрать контекст» создаёт КП и начинает бриф', async () => {
    const { deps, state, session } = makeDeps();
    fakeService({ systems: 1 });
    const route = await import('./routes.ts');
    await route.routeCallback(deps, '42', 'nd:kind:offer', 'cb1', state);
    await route.routeText(deps, '42', 'Гарант', state);
    await route.routeCallback(deps, '42', 'nd:pick:0', 'cb2', state);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'nd:context', 'cb3', state);

    assert.equal(session()?.draft?.stage, 'context');
    assert.match(sent[0].text, /Предложение № 007\/2026/);
    // Диалог нового документа закрыт: следующий текст — уже бриф.
    assert.equal(session()?.newDoc, undefined);
  });

  it('«Продолжить в приложении» создаёт КП и даёт кнопку на него', async () => {
    const { deps, state } = makeDeps();
    fakeService({ systems: 1 });
    const route = await import('./routes.ts');
    await route.routeCallback(deps, '42', 'nd:kind:offer', 'cb1', state);
    await route.routeText(deps, '42', 'Гарант', state);
    await route.routeCallback(deps, '42', 'nd:pick:0', 'cb2', state);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'nd:app', 'cb3', state);

    assert.match(sent[0].text, /Предложение № 007\/2026 создано/);
    const app = buttons(sent[0].keyboard).find((b) => Boolean(b.web_app));
    assert.ok(app, 'должна быть кнопка приложения');
    const url = String((app.web_app as { url: string }).url);
    assert.match(url, /#\/offer\/77$/);
  });

  it('боевой листок: свой вопрос про наш продукт', async () => {
    const { deps, state } = makeDeps();
    const route = await import('./routes.ts');
    await route.routeCallback(deps, '42', 'nd:kind:combat', 'cb1', state);

    assert.match(sent[0].text, /Какой наш продукт будет в боевом листке\?/);
    assert.equal(state.sessions.get('42')?.newDoc, 'combat');
  });

  it('боевой листок: выбранный продукт ложится в листок, а не в КП', async () => {
    // Живой дефект 2026-11-17: «Отмечено: 1…» → «Записано в листок:» (пусто) →
    // «Не удалось записать …: предложение не найдено». Диалог листка писал
    // продукт путём КП (`addPickedOfferProducts`), у листка нет `draft.offerId`.
    const { deps, state } = makeDeps();
    const calls: string[] = [];
    globalThis.fetch = (async (input: string | URL | Request) => {
      const url = String(input);
      const method = String((input as Request).method ?? 'GET');
      calls.push(`${method} ${url}`);
      const json = (body: unknown): Response =>
        new Response(JSON.stringify(body), { status: 200, headers: { 'content-type': 'application/json' } });
      if (url.includes('/bot/suggest')) {
        return json({ systems: [], materials: [{ noteId: 425, sku: 'MAT-ПРОЧ-ОБЩ-0425', name: 'Кровельная ПВХ-мембрана ECOPLAST' }] });
      }
      if (url.includes('/bot/combat') && method === 'POST') return json({ id: 55, number: '005/2026' });
      if (url.includes('/bot/combat/55/our')) return json({ ok: true });
      if (url.includes('/bot/combat')) return json({ id: 55, number: '005/2026' });
      return json({ ok: true });
    }) as typeof fetch;

    const route = await import('./routes.ts');
    await route.routeCallback(deps, '42', 'nd:kind:combat', 'cb1', state);
    await route.routeText(deps, '42', 'ECOPLAST', state);
    await route.routeCallback(deps, '42', 'nd:pick:0', 'cb2', state);
    sent.length = 0;
    await settle();
    await route.routeCallback(deps, '42', 'nd:addpicked', 'cb3', state);
    await settle();

    const texts = [...sent.map((m) => m.text), ...edits.map((m) => m.text)].join('\n');
    assert.ok(!/предложение не найдено/.test(texts), 'листок не должен падать «предложение не найдено»:\n' + texts);
    assert.match(texts, /Записано в листок/, 'сводка «Записано в листок» должна прийти:\n' + texts);
    assert.match(texts, /ECOPLAST/, 'в сводке должен быть записанный продукт:\n' + texts);
    assert.ok(
      calls.some((c) => c.includes('/bot/combat/55/our')),
      'продукт должен уйти на НАШУ сторону листка (POST /bot/combat/55/our), вызовы:\n' + calls.join('\n'),
    );
    assert.ok(
      !calls.some((c) => /POST .*\/bot\/offers\/\d+\/system/.test(c)),
      'в листке не должно быть записи системой в КП',
    );
  });

  it('текст вне нового диалога не перехватывается', async () => {
    const { deps, state } = makeDeps();
    fakeService({ systems: 1 });
    const route = await import('./routes.ts');
    await route.routeCallback(deps, '42', 'nd:kind:offer', 'cb1', state);
    await route.routeText(deps, '42', 'Гарант', state);
    await route.routeCallback(deps, '42', 'nd:context', 'cb2', state);
    sent.length = 0;
    // После выбора «контекст» диалог закрыт: «Монтаж | 12 | 850» — позиция.
    await route.routeText(deps, '42', 'Монтаж водостока | 12 | 850', state);
    assert.ok(
      sent.every((m) => !/По запросу «Монтаж водостока \| 12 \| 850»/.test(m.text)),
      'позиция не должна уходить в поиск нового диалога',
    );
  });

  it('устаревший номер списка не добавляет «что-то похожее»', async () => {
    const { deps, state } = makeDeps();
    fakeService({ systems: 1 });
    const route = await import('./routes.ts');
    await route.routeCallback(deps, '42', 'nd:kind:offer', 'cb1', state);
    await route.routeText(deps, '42', 'Гарант', state);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'nd:pick:9', 'cb2', state);

    assert.match(sent[0].text, /Этот список устарел/);
  });

  it('«уточню название» возвращает к вопросу и забывает старый список', async () => {
    const { deps, state } = makeDeps();
    fakeService({ systems: 1 });
    const route = await import('./routes.ts');
    await route.routeCallback(deps, '42', 'nd:kind:offer', 'cb1', state);
    await route.routeText(deps, '42', 'Гарант', state);
    sent.length = 0;
    await route.routeCallback(deps, '42', 'nd:again', 'cb2', state);

    assert.match(sent[0].text, /Какой продукт или систему/);
    assert.equal(state.newDoc.get('42')?.items.length, 0);
  });
});

/**
 * «Достаточно» в боевом листке (2026-11-20).
 *
 * Живая жалоба: главный вариант «✅ Достаточно — листок собран» вводил людей в
 * стопор. Листку не хватает продукта конкурента — без него сравнивать нечего, —
 * а бот показывал меню из семи кнопок и об этом не говорил. Теперь кнопка
 * называет следующий шаг и сразу открывает его.
 */
describe('боевой листок: «достаточно» — следующий шаг, а не конец', () => {
  /** Стенд сервиса: наш продукт пишется, листок читается по номеру. */
  function combatService(calls: string[]): void {
    globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      const method = String(init?.method ?? 'GET').toUpperCase();
      calls.push(`${method} ${url}`);
      const json = (body: unknown): Response =>
        new Response(JSON.stringify(body), { status: 200, headers: { 'content-type': 'application/json' } });
      if (url.includes('/bot/suggest')) {
        return json({ systems: [], materials: [{ noteId: 425, name: 'Кровельная ПВХ-мембрана LOGICROOF PRO V-RP' }] });
      }
      if (method === 'POST' && url.endsWith('/bot/combat')) return json({ id: 55, number: '005/2026' });
      if (url.includes('/bot/combat/55/our') || url.includes('/bot/combat/55/competitor')) {
        return json({ id: 55, number: '005/2026', ourNotes: [], competitorNotes: [] });
      }
      if (method === 'GET' && url.includes('/bot/combat?')) {
        return json({ sheets: [{ id: 55, number: '005/2026', ownerEmail: 'ivanov@tn.ru', ourNotes: [], competitorNotes: [] }] });
      }
      if (method === 'GET' && url.includes('/bot/combat/competitors')) return json({ cards: [], total: 0, offset: 0, hasMore: false });
      return json({ ok: true });
    }) as typeof fetch;
  }

  it('кнопка называет следующий шаг, а не «листок собран»', () => {
    const rows = moreKeyboard('combat').inline_keyboard as Array<Array<{ text: string; callback_data: string }>>;
    assert.equal(rows[0][0].text, '🏷️ Выбрать конкурента');
    assert.equal(rows[0][0].callback_data, 'nd:enough');
    assert.ok(rows.flat().some((b) => b.callback_data === 'nd:more'));
    assert.ok(
      rows.flat().every((b) => !/собран/i.test(b.text)),
      'слово «собран» обещало конец, хотя листку не хватало конкурента',
    );
  });

  it('сообщение о записи называет конкурента; в КП вопрос прежний', () => {
    const combat = addedMessage('combat', [{ kind: 'material', title: 'LOGICROOF PRO V-RP' }], '005/2026');
    assert.match(combat, /№ 005\/2026/);
    assert.match(combat, /конкурент/i);
    assert.doesNotMatch(combat, /этого достаточно/i);

    const offer = addedMessage('offer', [{ kind: 'system', title: 'ТН-КРОВЛЯ' }]);
    assert.match(offer, /Нужно добавить ещё/);
  });

  it('нажатие открывает поиск конкурента, а меню листка остаётся на экране', async () => {
    const { deps, state, session } = makeDeps();
    const calls: string[] = [];
    combatService(calls);
    const route = await import('./routes.ts');
    await route.routeCallback(deps, '42', 'nd:kind:combat', 'c1', state);
    await route.routeText(deps, '42', 'LOGICROOF', state);
    await route.routeCallback(deps, '42', 'nd:pick:0', 'c2', state);
    await route.routeCallback(deps, '42', 'nd:addpicked', 'c3', state);
    sent.length = 0;

    await route.routeCallback(deps, '42', 'nd:enough', 'c4', state);

    const last = sent[sent.length - 1];
    assert.match(last.text, /конкурент/i);
    assert.doesNotMatch(last.text, /листок собран/i);
    assert.equal(session()?.combat?.picking, 'competitor', 'поиск стороны переключился на конкурента');
    assert.equal(session()?.newDoc, undefined, 'диалог нового документа закрыт');
    const buttons = ((last.keyboard?.inline_keyboard ?? []) as Array<Array<{ callback_data?: string }>>).flat();
    assert.ok(
      buttons.some((b) => b.callback_data === 'cb:doc:55'),
      'из поиска конкурента нужен выход к DOCX этого листка',
    );
  });

  it('после «Достаточно» название ищет конкурента, а не наш продукт', async () => {
    const { deps, state } = makeDeps();
    const calls: string[] = [];
    combatService(calls);
    const route = await import('./routes.ts');
    await route.routeCallback(deps, '42', 'nd:kind:combat', 'c1', state);
    await route.routeText(deps, '42', 'LOGICROOF', state);
    await route.routeCallback(deps, '42', 'nd:pick:0', 'c2', state);
    await route.routeCallback(deps, '42', 'nd:addpicked', 'c3', state);
    await route.routeCallback(deps, '42', 'nd:enough', 'c4', state);
    calls.length = 0;

    await route.routeText(deps, '42', 'PIRRO', state);

    assert.ok(
      calls.some((c) => c.includes('/bot/combat/competitors')),
      `запрос должен идти в поиск конкурентов, вызовы:\n${calls.join('\n')}`,
    );
    assert.ok(
      !calls.some((c) => c.includes('/bot/suggest') && c.includes('PIRRO')),
      'название конкурента не должно попадать в поиск наших продуктов',
    );
  });

  it('в КП «Достаточно» по-прежнему начинает бриф', async () => {
    const { deps, state, session } = makeDeps();
    const calls: string[] = [];
    globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      const method = String(init?.method ?? 'GET').toUpperCase();
      calls.push(`${method} ${url}`);
      const json = (body: unknown): Response =>
        new Response(JSON.stringify(body), { status: 200, headers: { 'content-type': 'application/json' } });
      if (url.includes('/bot/suggest')) {
        return json({ systems: [{ noteId: 100, title: 'ТН-КРОВЛЯ Гарант' }], materials: [] });
      }
      if (url.endsWith('/bot/offers') && method === 'POST') return json({ id: 77, number: '007/2026' });
      if (url.includes('/brief/start')) {
        return json({ reply: 'Расскажите про объект', done: false, brief: { lastTopic: 'object_name' } });
      }
      return json({ ok: true });
    }) as typeof fetch;
    const route = await import('./routes.ts');
    await route.routeCallback(deps, '42', 'nd:kind:offer', 'c1', state);
    await route.routeText(deps, '42', 'Гарант', state);
    await route.routeCallback(deps, '42', 'nd:pick:0', 'c2', state);
    await route.routeCallback(deps, '42', 'nd:addpicked', 'c3', state);
    calls.length = 0;

    await route.routeCallback(deps, '42', 'nd:enough', 'c4', state);

    assert.ok(
      calls.some((c) => c.includes('/brief/start')),
      `КП должно перейти к брифу, вызовы:\n${calls.join('\n')}`,
    );
    assert.ok(!calls.some((c) => c.includes('/bot/combat')), 'КП не должно трогать боевой листок');
    assert.equal(session()?.newDoc, undefined);
  });
});