/**
 * Поллер Telegram: только транспорт. С VDS нет прямого пути к IP Telegram,
 * поэтому getUpdates идёт через SOCKS5 (сайдкар offers-nl-proxy).
 *
 * Всё, что решает, «что ответить», живёт в `routes.ts` и `callbacks.ts`
 * (Task 1 плана 2026-10-29): раньше здесь были две функции-диспетчера по
 * CC 23-24, и правку входа приходилось делать в двух местах.
 */
import { answerCallback, getUpdates, type TelegramUpdate } from './telegram.ts';
import { restoreSession, type ChatSession } from './session.ts';
import { routeCallback, routeText, newRouterState, type BotDeps, type RouterState } from './routes.ts';
import { sendMessage } from './telegram.ts';
import * as api from './api.ts';

/**
 * Сессия чата с восстановлением из базы (2026-10-29).
 *
 * В памяти сессии нет после перезапуска бота — а постоянная привязка
 * chat_id → email лежит в offers_bot_link и никуда не девается. Поэтому для
 * незнакомого в памяти чата спрашиваем сервис, кто это: знакомый сразу
 * считается вошедшим, и человеку не приходится заново запрашивать код.
 *
 * Ошибку запроса не считаем фатальной: не смогли спросить — человек просто
 * войдёт по коду, как раньше.
 */
async function sessionRestored(
  state: RouterState,
  chatId: string,
): Promise<ChatSession> {
  const known = state.sessions.get(chatId);
  if (known) return known;
  let email = '';
  try {
    const who = await api.identity(chatId);
    email = who.email ?? '';
  } catch {
    // 404 = чат не привязан, прочие ошибки = сервис недоступен. Оба случая
    // означают одно: входа нет, работаем как с новым человеком.
  }
  const restored = restoreSession(email);
  state.sessions.set(chatId, restored);
  if (restored.step === 'authed') {
    console.log(`offers-bot: чат ${chatId} восстановлен — ${restored.email}`);
  }
  return restored;
}

function makeDeps(state: RouterState): BotDeps {
  return {
    now: () => Date.now(),
    send: async (chatId: string, text: string, keyboard?: Record<string, unknown>) => {
      await sendMessage(chatId, text, keyboard);
    },
    identity: (chatId: string) => api.identity(chatId),
    session: (chatId: string) => sessionRestored(state, chatId),
    save: (chatId: string, session: ChatSession) => {
      state.sessions.set(chatId, session);
    },
    answerCallback: (callbackId: string) => answerCallback(callbackId),
  };
}

async function handleUpdate(deps: BotDeps, state: RouterState, update: TelegramUpdate): Promise<void> {
  if (update.message?.text && update.message.chat) {
    await routeText(deps, String(update.message.chat.id), update.message.text, state);
    return;
  }
  const cb = update.callback_query;
  if (cb?.data && cb.message) {
    await routeCallback(deps, String(cb.message.chat.id), cb.data, cb.id, state);
  }
}

export async function runPoller(): Promise<void> {
  let offset = 0;
  let toldAboutToken = false;
  let lastSentError = '';
  const state = newRouterState();
  const deps = makeDeps(state);
  console.log('offers-bot: поллер запущен');

  // Отметка «бот жив» — САМО измерение, а не сообщение: её надо слать каждую
  // итерацию, иначе bot_last_seen_at замирает и плагин объявляет живого бота
  // мёртвым. Поэтому тот же текст ошибки не глушим, а лишь не логируем
  // повторно в консоль.
  const report = async (error: string): Promise<void> => {
    lastSentError = error;
    try {
      await api.heartbeat(error);
    } catch (e) {
      console.error('offers-bot: heartbeat не доставлен:', (e as Error).message);
    }
  };

  for (;;) {
    let updates: TelegramUpdate[];
    try {
      updates = await getUpdates(offset);
      if (toldAboutToken) {
        toldAboutToken = false;
        console.log('offers-bot: токен получен — приём сообщений работает');
      }
      await report('');
    } catch (e) {
      const text = (e as Error).message;
      // Токен ещё не ввели в настройках плагина — это не авария: контейнер
      // продолжает работать и подхватит токен сам, без перезапуска.
      if (text.includes('токен бота не задан')) {
        if (!toldAboutToken) {
          toldAboutToken = true;
          console.warn('offers-bot: токен не задан — ждём, пока его введут в настройках плагина');
        }
      } else if (text !== lastSentError) {
        console.error('offers-bot: getUpdates:', text);
      }
      await report(text);
      await new Promise((resolve) => setTimeout(resolve, 15000));
      continue;
    }
    for (const update of updates) {
      offset = update.update_id + 1;
      try {
        await handleUpdate(deps, state, update);
      } catch (e) {
        console.error('offers-bot: ошибка обработки сообщения:', (e as Error).message);
      }
    }
  }
}
