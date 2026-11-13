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

const REAL_ENV = { ...process.env };
const sent: Array<{ chatId: string; text: string; keyboard?: Record<string, unknown> }> = [];

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
  const deps: BotDeps = {
    now: () => 1_000_000,
    send: async (chatId, text, keyboard) => {
      sent.push({ chatId, text, keyboard });
      return sent.length;
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

    assert.match(sent[0].text, /Отмечено: 1/);
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
    await route.routeCallback(deps, '42', 'nd:addpicked', 'cb3', state);

    assert.match(sent[0].text, /Системы: «ТН-КРОВЛЯ Гарант RE30»/);
    assert.match(sent[0].text, /Нужно добавить ещё/);
    const all = buttons(sent[0].keyboard);
    assert.ok(all.some((b) => b.callback_data === 'nd:more'));
    assert.ok(all.some((b) => b.callback_data === 'nd:enough'));
    assert.ok(all.some((b) => b.callback_data === 'nd:app'));
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