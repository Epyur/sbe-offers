/**
 * Маршрутизация обновлений бота (Task 1 плана 2026-10-29).
 *
 * Транспорт (getUpdates, дедуп, лимит частоты, восстановление сессии) остался
 * в `poller.ts`; здесь — что сделат с сообщением после того, как стало
 * понятно, кто это. Вход проверяется ОДИН раз на оба пути (текст и кнопки),
 * раньше проверка была написана дважды и правку приходилось делать в двух
 * местах.
 *
 * Зависимости передаются снаружи (`BotDeps`), а не берутся из модулей: так
 * маршруты проверяются стендом без сети, а поллер остаётся тонким.
 */
import { applyCodeRequested, applyLogin, isCombat, nextStep, type ChatSession } from './session.ts';
import { AUTH_OK, AUTH_PROMPT, HELP_TEXT, MAIN_KEYBOARD } from './flow/build.ts';
import * as api from './api.ts';
import type { SystemItem } from './api.ts';
import {
  BUTTONS,
  ID_BUTTONS,
  applyAttachConfirm,
  handleCombatText,
  handleDraftText,
  handleNewDocText,
  onEditOffer,
  onNewDocKind,
  startNewDoc,
  type CallbackCtx,
} from './callbacks.ts';
import { canResume } from './flow/resume.ts';
import { appListButton } from './flow/appbutton.ts';
import type { NewDocState } from './flow/newdoc.ts';
import type { SuggestItem } from './flow/suggest.ts';
import {
  NO_OFFER_FOR_FILE,
  attachMessage,
  rememberAttach,
  type AttachResult,
  type AttachSession,
} from './attach.ts';

const RATE_LIMIT_PER_MIN = Number(process.env.OFFERS_BOT_RATE_LIMIT ?? 30);
const DUPLICATE_WINDOW_MS = 10_000;

export interface BotDeps {
  /** Текущее время в мс: подменяется стендом, чтобы проверять окна. */
  now(): number;
  send(chatId: string, text: string, keyboard?: Record<string, unknown>): Promise<number | undefined>;
  /** Заменить сообщение (2026-11-12): «⌛ …» → результат долгого действия. */
  edit?(chatId: string, messageId: number, text: string, keyboard?: Record<string, unknown>): Promise<void>;
  /** Кто это: chat_id → email. Пустой email — чат не привязан. */
  identity(chatId: string): Promise<{ email: string }>;
  /** Текущая сессия чата с восстановлением из базы (см. sessionRestored). */
  session(chatId: string): Promise<ChatSession>;
  /** Сохранить изменённую сессию (в поллере — карта в памяти). */
  save(chatId: string, session: ChatSession): void;
  /** Подтвердить нажатие кнопки (Telegram требует ответа на callback). */
  answerCallback(callbackId: string): Promise<void>;
}

/** Состояние, которое живёт в памяти процесса между вызовами. */
export interface RouterState {
  hits: Map<string, number[]>;
  lastMessage: Map<string, { text: string; at: number }>;
  pages: Map<string, { query: string; offset: number }>;
  sessions: Map<string, ChatSession>;
  /**
   * Системы, показанные кнопками выбора (2026-10-31). Кнопка несёт индекс, а
   * не код: callback_data ограничен 64 байтами и кириллический код системы в
   * него не влезет.
   */
  foundSystems: Map<string, SystemItem[]>;
  /**
   * Стороны боевого листка, показанные кнопками (2026-11-09). Как и системы:
   * кнопка несёт индекс, а не заметку/название.
   */
  foundCombat: Map<string, api.CombatSystemItem[]>;
  /**
   * Страница выбора стороны боевого листка (2026-11-10): сторона, запрос,
   * смещение и отмеченные строки. Номер строки в кнопке — ОТ НАЧАЛА списка,
   * поэтому picked переживает перелистывание, а смена запроса его стирает:
   * индексы относятся к прежнему набору совпадений.
   */
  combatPicker: Map<string, CombatPickerState>;
  /**
   * Последний выбранный продукт конкурента (2026-11-11): по нему «Подобрать
   * аналоги» ищет наш аналог. Живёт в памяти поллера — это подсказка
   * разговора, а не данные листка.
   */
  lastCompetitor: Map<string, api.CombatSystemItem>;
  /** Наши аналоги, показанные кнопками: номер кнопки — заметка. */
  foundAnalogs: Map<string, api.CombatSystemItem[]>;
  /**
   * Подсказка по свободной фразе (2026-11-12): что нашлось по запросу.
   *
   * Номер в кнопке — индекс, поэтому список живёт в памяти поллера, а не в
   * `callback_data` (64 байта кириллического названия не вместят).
   */
  foundSuggest: Map<string, SuggestState>;
  /**
   * Разговор «Создать новый документ» (2026-11-13, R2): что нашли по названию
   * продукта и что отмечено.
   *
   * Отдельная карта, а НЕ общая с `foundSuggest`: две ветки читают её
   * по-разному (здесь выбор ЗАПОМИНАЕТСЯ до развилки, там — сразу
   * добавляется), и общая карта дала бы «нажал — добавилось не туда». Так уже
   * случилось с выбором стороны листка (2026-11-11).
   */
  newDoc: Map<string, NewDocState>;
}

/**
 * Показанная подсказка: что искали и что нашли.
 *
 * Запрос нужен при нажатии номера: система добавляется в КП, материал — в
 * черновик позиции, а человеку в ответе надо назвать то же, что он искал.
 */
export interface SuggestState {
  query: string;
  items: SuggestItem[];
}

/** Состояние выбора стороны: что показано и что отмечено. */
export interface CombatPickerState {
  side: 'our' | 'competitor';
  query: string;
  offset: number;
  /** Индексы отмеченных строк от начала текущего набора совпадений. */
  picked: number[];
  /**
   * Взято всё найденное по запросу (2026-11-10, кнопка «Выбрать все
   * найденные»). Отдельной отметкой, а не заполнением picked: найденное может
   * быть в сотни строк, и складывать их номера незачем. picked тогда пуст,
   * и он же — признак, что часть отметок менеджер снял вручную.
   */
  allPicked?: boolean;
}

export function newRouterState(): RouterState {
  return {
    hits: new Map(),
    lastMessage: new Map(),
    pages: new Map(),
    sessions: new Map(),
    foundSystems: new Map(),
    foundCombat: new Map(),
    combatPicker: new Map(),
    lastCompetitor: new Map(),
    foundAnalogs: new Map(),
    foundSuggest: new Map(),
    newDoc: new Map(),
  };
}

function rateOk(state: RouterState, nowMs: number, chatId: string): boolean {
  if (!Number.isFinite(RATE_LIMIT_PER_MIN) || RATE_LIMIT_PER_MIN <= 0) return true;
  const cut = nowMs - 60_000;
  const kept = (state.hits.get(chatId) ?? []).filter((t) => t > cut);
  if (kept.length >= RATE_LIMIT_PER_MIN) {
    state.hits.set(chatId, kept);
    return false;
  }
  kept.push(nowMs);
  state.hits.set(chatId, kept);
  return true;
}

function isDuplicate(state: RouterState, nowMs: number, chatId: string, text: string): boolean {
  const last = state.lastMessage.get(chatId);
  if (last && last.text === text && nowMs - last.at < DUPLICATE_WINDOW_MS) return true;
  state.lastMessage.set(chatId, { text, at: nowMs });
  return false;
}

/**
 * Поллер создаёт по одному состоянию на процесс. Стенд может передать своё,
 * чтобы тесты не делили карты между собой.
 */
let sharedState: RouterState = newRouterState();

export function resetRouterState(): void {
  sharedState = newRouterState();
}

function saveSession(state: RouterState, chatId: string, session: ChatSession): void {
  state.sessions.set(chatId, session);
}

/** Вход: если человек ещё не вошёл — весь текст идёт в машину входа. */
async function handleSignIn(deps: BotDeps, chatId: string, session: ChatSession, text: string): Promise<void> {
  const result = nextStep(session, text);
  switch (result.kind) {
    case 'prompt':
      await deps.send(chatId, result.text);
      return;
    case 'request_code':
      try {
        await api.requestCode(result.email, chatId);
      } catch (e) {
        await deps.send(chatId, `Не удалось отправить код: ${(e as Error).message}`);
        return;
      }
      // Шаг входа сохраняется ОБЯЗАТЕЛЬНО (2026-11-12): без этого сессия
      // оставалась в idle, и присланный код принимался за e-mail — бот снова
      // просил почту. Раньше здесь стояло `void applyCodeRequested;` —
      // ссылка на функцию вместо вызова, и компилятор об этом молчал.
      await deps.save(chatId, applyCodeRequested(session, result.email));
      await deps.send(chatId, result.text);
      return;
    case 'verify_code':
      try {
        const who = await api.verifyCode(chatId, result.email, result.code);
        await deps.save(chatId, applyLogin(session, who.email));
        await deps.send(chatId, AUTH_OK(who.email), MAIN_KEYBOARD);
      } catch (e) {
        await deps.send(chatId, (e as Error).message);
      }
      return;
    default:
      return;
  }
}

/** Команды основного меню после входа. Возвращает true, если текст разобран. */
async function routeMenu(
  deps: BotDeps,
  ctx: CallbackCtx,
  trimmed: string,
): Promise<boolean> {
  if (trimmed === '❓ Справка' || trimmed === '/help') {
    await deps.send(ctx.chatId, HELP_TEXT, MAIN_KEYBOARD);
    return true;
  }
  if (trimmed === '✨ Создать новый документ' || trimmed === '/new') {
    await startNewDoc(ctx);
    return true;
  }
  if (trimmed === '📂 Мои КП' || trimmed === '/my') {
    await openAppList(deps, ctx.chatId, 'offers');
    return true;
  }
  // Старые кнопки (2026-11-13, R2): их подписи остаются в истории чата и в
  // привычке у людей. Обе ведут в новый диалог, но СРАЗУ с выбранным видом:
  // человек, нажавший «Боевой листок», не должен ещё раз отвечать на вопрос,
  // что он собирает.
  if (trimmed === '🧾 Собрать КП') {
    await onNewDocKind(ctx, 'offer');
    return true;
  }
  if (trimmed === '⚔️ Боевой листок' || trimmed === '/combat') {
    await onNewDocKind(ctx, 'combat');
    return true;
  }
  if (trimmed === '📂 Мои боевые листки' || trimmed === '/combats') {
    await openAppList(deps, ctx.chatId, 'sheets');
    return true;
  }
  // «003/2026» — открыть сохранённое КП на правку (2026-11-01). Номер узнаётся
  // по форме, поэтому команду можно вводить, не открывая список.
  if (canResume(trimmed)) {
    const offers = await api.listOffers(ctx.chatId).catch(() => []);
    const found = offers.find((o) => o.number.trim() === trimmed);
    if (found) {
      await onEditOffer(ctx, found.id);
      return true;
    }
  }
  return false;
}

/**
 * Подписи двух кнопок приложения (2026-11-13, R1).
 *
 * Почему не `web_app` прямо в reply-клавиатуре: такая кнопка открывает Mini App
 * БЕЗ подписи Telegram, человек видит «Нет доступа: войдите в бота» — это и
 * было живым дефектом 2026-11-13, ради которого кнопку убрали из меню. Подпись
 * приходит только у инлайновой `web_app`-кнопки, поэтому reply-кнопка всего
 * лишь отвечает такой кнопкой.
 */
const APP_LIST_BUTTONS: Record<'offers' | 'sheets', string> = {
  offers: '📱 Открыть «Мои КП» в приложении',
  sheets: '📱 Открыть «Мои боевые листки» в приложении',
};

/**
 * «📂 Мои КП» / «📂 Мои боевые листки» и их старые алиасы (2026-11-13, R1):
 * открывают список в приложении. Список в чате больше не дублируется — он
 * живёт в приложении, и две реализации списка неизбежно разошлись бы.
 */
async function openAppList(deps: BotDeps, chatId: string, which: 'offers' | 'sheets'): Promise<void> {
  const label = which === 'offers' ? 'Мои КП' : 'Мои боевые листки';
  await deps.send(
    chatId,
    `${label} открываются в приложении: нажмите кнопку ниже.`,
    { inline_keyboard: [[appListButton(APP_LIST_BUTTONS[which], which)]] },
  );
}

export async function routeText(
  deps: BotDeps,
  chatId: string,
  text: string,
  state: RouterState = sharedState,
): Promise<void> {
  const trimmed = text.trim();
  const nowMs = deps.now();
  if (isDuplicate(state, nowMs, chatId, trimmed)) return;
  if (!rateOk(state, nowMs, chatId)) {
    await deps.send(chatId, 'Слишком много сообщений подряд — подождите минуту.');
    return;
  }

  const session = await deps.session(chatId);

  if (trimmed === '/start') {
    if (session.step === 'authed') {
      await deps.send(chatId, `Вы вошли как ${session.email}.`, MAIN_KEYBOARD);
      return;
    }
    // /start до входа сбрасывает шаг, но сохраняет e-mail: заново вводить не надо.
    saveSession(state, chatId, { step: session.step, email: session.email });
    await deps.send(chatId, AUTH_PROMPT);
    return;
  }

  if (session.step !== 'authed') {
    await handleSignIn(deps, chatId, session, trimmed);
    return;
  }

  const ctx: CallbackCtx = {
    deps,
    chatId,
    session,
    save: (next) => saveSession(state, chatId, next),
    pages: state.pages,
    foundSystems: state.foundSystems.get(chatId) ?? [],
    rememberSystems: (systems) => state.foundSystems.set(chatId, systems),
    foundCombat: state.foundCombat.get(chatId) ?? [],
    rememberCombat: (items) => state.foundCombat.set(chatId, items),
    combatPicker: state.combatPicker.get(chatId),
    rememberCombatPicker: (picker) => state.combatPicker.set(chatId, picker),
    lastCompetitor: state.lastCompetitor.get(chatId),
    rememberCompetitor: (item) => state.lastCompetitor.set(chatId, item),
    foundAnalogs: state.foundAnalogs.get(chatId) ?? [],
    rememberAnalogs: (items) => state.foundAnalogs.set(chatId, items),
    foundSuggest: state.foundSuggest.get(chatId),
    rememberSuggest: (value) => state.foundSuggest.set(chatId, value),
    newDoc: state.newDoc.get(chatId),
    rememberNewDoc: (value) => state.newDoc.set(chatId, value),
  };

  if (await routeMenu(deps, ctx, trimmed)) return;
  // «Создать новый документ» (2026-11-13, R2): пока этот диалог идёт, текст —
  // это НАЗВАНИЕ ПРОДУКТА, а не позиция и не ответ брифа. Проверяется РАНЬШЕ
  // листка и черновика: иначе «Мембрана» стала бы позицией с ценой 0.
  if (await handleNewDocText(ctx, trimmed)) return;
  // В боевом листке текст — это запрос на поиск стороны (2026-11-09): каталог
  // 1500+ записей кнопками не выбрать. Свободная строка «Монтаж | 12 | 850»
  // при этом в черновик КП не попадёт — листок живёт отдельно от КП.
  if (isCombat(session)) {
    if (await handleCombatText(ctx, trimmed)) return;
    return;
  }
  if (await handleDraftText(ctx, trimmed)) return;

  await deps.send(chatId, 'Не понял. Выберите действие кнопкой ниже или отправьте /help.', MAIN_KEYBOARD);
}

/** Разбор `data` кнопки: `pick:12` → { prefix: 'pick', id: 12 }. */
export function parseCallbackData(data: string): { prefix: string; id: number } | null {
  const colon = data.indexOf(':');
  if (colon < 0) return null;
  const rest = data.slice(colon + 1);
  // `cban:all` — не число, а слово (2026-11-11, живая ошибка: кнопка «Добавить
  // все найденные» не работала вовсе). `Number('all')` даёт NaN, и разбор
  // отбрасывал нажатие молча — снаружи это «нажал, ничего не произошло».
  // Слово-ключ передаём как id = 0: ноль не бывает настоящим номером заметки.
  if (rest === 'all') return { prefix: data.slice(0, colon), id: 0 };
  // Составной префикс (R2): `nd:pick:3` = ветка `nd`, действие `pick`, номер 3.
  // Разбор по ПЕРВОМУ `:` дал бы prefix `nd` и `Number('pick:3')` = NaN — то
  // же молчаливое «нажал, ничего не произошло», что и с `cban:all`.
  if (rest.includes(':')) {
    const cut = rest.lastIndexOf(':');
    const id = Number(rest.slice(cut + 1));
    if (!Number.isFinite(id)) return null;
    return { prefix: data.slice(0, colon + cut + 1), id };
  }
  const id = Number(rest);
  if (!Number.isFinite(id)) return null;
  return { prefix: data.slice(0, colon), id };
}

// --- Файлы из чата (2026-10-29) ---------------------------------------

/** Куда приложить файл: к уже созданному КП. Черновик сюда не годится —
 *  у него ещё нет номера и владельца. */
function offerIdForFile(session: ChatSession): number {
  return session.draft?.offerId ?? 0;
}

/**
 * Обработать присланный файл: разобрать, показать «что понял» и запомнить до
 * подтверждения. Применяется только по кнопке «✓ Применить».
 */
export async function routeDocument(
  deps: BotDeps,
  chatId: string,
  fileName: string,
  data: Buffer,
  state: RouterState = sharedState,
): Promise<void> {
  const session = await deps.session(chatId) as AttachSession;
  if (session.step !== 'authed') {
    await deps.send(chatId, AUTH_PROMPT);
    return;
  }
  const offerId = offerIdForFile(session);
  if (!offerId) {
    await deps.send(chatId, NO_OFFER_FOR_FILE, MAIN_KEYBOARD);
    return;
  }
  let result: AttachResult;
  try {
    result = await api.attachFile(offerId, chatId, fileName, data);
  } catch (e) {
    await deps.send(chatId, (e as Error).message, MAIN_KEYBOARD);
    return;
  }
  const message = attachMessage(result);
  saveSession(state, chatId, rememberAttach(session, result));
  await deps.send(chatId, message.text, message.keyboard ?? MAIN_KEYBOARD);
}

export async function routeCallback(
  deps: BotDeps,
  chatId: string,
  data: string,
  callbackId: string,
  state: RouterState = sharedState,
): Promise<void> {
  await deps.answerCallback(callbackId);
  const session = await deps.session(chatId);
  if (session.step !== 'authed') {
    await deps.send(chatId, AUTH_PROMPT);
    return;
  }

const ctx: CallbackCtx = {
    deps,
    chatId,
    session,
    // Долгие действия заменяют своё «⌛» результатом (2026-11-12).
    edit: deps.edit,
    save: (next) => saveSession(state, chatId, next),
    pages: state.pages,
    foundSystems: state.foundSystems.get(chatId) ?? [],
    rememberSystems: (systems) => state.foundSystems.set(chatId, systems),
    foundCombat: state.foundCombat.get(chatId) ?? [],
    rememberCombat: (items) => state.foundCombat.set(chatId, items),
    combatPicker: state.combatPicker.get(chatId),
    rememberCombatPicker: (picker) => state.combatPicker.set(chatId, picker),
    lastCompetitor: state.lastCompetitor.get(chatId),
    rememberCompetitor: (item) => state.lastCompetitor.set(chatId, item),
    foundAnalogs: state.foundAnalogs.get(chatId) ?? [],
    rememberAnalogs: (items) => state.foundAnalogs.set(chatId, items),
    foundSuggest: state.foundSuggest.get(chatId),
    rememberSuggest: (value) => state.foundSuggest.set(chatId, value),
    newDoc: state.newDoc.get(chatId),
    rememberNewDoc: (value) => state.newDoc.set(chatId, value),
  };

  const exact = BUTTONS[data];
  if (exact) {
    await exact(ctx);
    return;
  }

  // Подтверждение разбора файла: `att:yes` / `att:no`. Обработчик живёт в
  // callbacks.ts, чтобы не было кольца импортов routes → callbacks → routes.
  if (data === 'att:yes' || data === 'att:no') {
    await applyAttachConfirm(ctx, data === 'att:yes');
    return;
  }

  const parsed = parseCallbackData(data);
  if (!parsed) return;
  const withId = ID_BUTTONS[parsed.prefix];
  if (withId) {
    await withId(ctx, parsed.id);
    return;
  }
  // Неизвестная кнопка — молча ничего: старая кнопка в истории не должна
  // ронять обработчик и не должна отвечать мусором.
}

export { AUTH_PROMPT, applyLogin };
