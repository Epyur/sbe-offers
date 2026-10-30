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
import { applyCodeRequested, applyLogin, nextStep, type ChatSession } from './session.ts';
import { AUTH_OK, AUTH_PROMPT, HELP_TEXT, MAIN_KEYBOARD } from './flow/build.ts';
import * as api from './api.ts';
import {
  BUTTONS,
  ID_BUTTONS,
  applyAttachConfirm,
  handleDraftText,
  startDraft,
  type CallbackCtx,
} from './callbacks.ts';
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
  send(chatId: string, text: string, keyboard?: Record<string, unknown>): Promise<void>;
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
}

export function newRouterState(): RouterState {
  return {
    hits: new Map(),
    lastMessage: new Map(),
    pages: new Map(),
    sessions: new Map(),
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
      await deps.send(chatId, result.text);
      void applyCodeRequested;
      return;
    case 'verify_code':
      try {
        const who = await api.verifyCode(chatId, result.email, result.code);
        await deps.send(chatId, AUTH_OK(who.email), MAIN_KEYBOARD);
      } catch (e) {
        await deps.send(chatId, (e as Error).message);
      }
      return;
    default:
      return;
  }
}

async function handleMyOffers(deps: BotDeps, chatId: string): Promise<void> {
  try {
    const offers = await api.listOffers(chatId);
    if (offers.length === 0) {
      await deps.send(chatId, 'Предложений пока нет.', MAIN_KEYBOARD);
      return;
    }
    const lines = offers.slice(0, 15).map((o) => {
      const client = o.client?.name ? ` — ${o.client.name}` : '';
      return `№ ${o.number}${client} · ${formatTotal(o.total)} руб.`;
    });
    const buttons = offers.slice(0, 15).map((o) => [
      { text: `📄 ${o.number}`, callback_data: `doc:${o.id}` },
    ]);
    await deps.send(chatId, `Последние предложения:\n\n${lines.join('\n')}`, {
      inline_keyboard: buttons,
    });
  } catch (e) {
    await deps.send(chatId, `Не удалось загрузить список: ${(e as Error).message}`, MAIN_KEYBOARD);
  }
}

function formatTotal(v: number): string {
  const cents = Math.round(Math.abs(v) * 100);
  const whole = Math.floor(cents / 100)
    .toString()
    .replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
  const frac = (cents % 100).toString().padStart(2, '0');
  return `${v < 0 ? '-' : ''}${whole},${frac}`;
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
  if (trimmed === '🧾 Собрать КП' || trimmed === '/new') {
    await startDraft(ctx);
    return true;
  }
  if (trimmed === '📂 Мои КП' || trimmed === '/my') {
    await handleMyOffers(deps, ctx.chatId);
    return true;
  }
  return false;
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
  };

  if (await routeMenu(deps, ctx, trimmed)) return;
  if (await handleDraftText(ctx, trimmed)) return;

  await deps.send(chatId, 'Не понял. Выберите действие кнопкой ниже или отправьте /help.', MAIN_KEYBOARD);
}

/** Разбор `data` кнопки: `pick:12` → { prefix: 'pick', id: 12 }. */
export function parseCallbackData(data: string): { prefix: string; id: number } | null {
  const colon = data.indexOf(':');
  if (colon < 0) return null;
  const id = Number(data.slice(colon + 1));
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
    save: (next) => saveSession(state, chatId, next),
    pages: state.pages,
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
