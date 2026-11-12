/**
 * Поллер Telegram: только транспорт. С VDS нет прямого пути к IP Telegram,
 * поэтому getUpdates идёт через SOCKS5 (сайдкар offers-nl-proxy).
 *
 * Всё, что решает, «что ответить», живёт в `routes.ts` и `callbacks.ts`
 * (Task 1 плана 2026-10-29): раньше здесь были две функции-диспетчера по
 * CC 23-24, и правку входа приходилось делать в двух местах.
 */
import {
  answerCallback,
  downloadDocument,
  getUpdates,
  sendDocument,
  type TelegramUpdate,
} from './telegram.ts';
import { deliverDocx, parseWebAppData, unknownRequestText } from './webapp.ts';
import { restoreSession, type ChatSession } from './session.ts';
import { routeCallback, routeDocument, routeText, newRouterState, type BotDeps, type RouterState } from './routes.ts';
import { sendMessage, editMessage } from './telegram.ts';
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
    send: async (chatId: string, text: string, keyboard?: Record<string, unknown>) =>
      sendMessage(chatId, text, keyboard),
    // Долгие действия пишут «⌛ …» и заменяют ЕГО результатом (2026-11-12):
    // человек видит, что бот работает, а не гадает минуту.
    edit: async (chatId: string, messageId: number, text: string, keyboard?: Record<string, unknown>) => {
      await editMessage(chatId, messageId, text, keyboard);
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
  const message = update.message;
  if (message?.chat && message.web_app_data) {
    // «Отправить в чат» из Mini App (шаг S7): приложение не может приложить
    // файл само, поэтому присылает указатель — а документ собирает бот.
    await handleWebAppData(deps, String(message.chat.id), message.web_app_data.data);
    return;
  }
  if (message?.chat && message.document) {
    // Файл: скачиваем через тот же SOCKS, что и getUpdates, и отдаём в
    // маршруты. Скачивание здесь, потому что путь файла и прокси должны
    // жить в одном месте (telegram.ts).
    const chatId = String(message.chat.id);
    let data: Buffer;
    try {
      data = await downloadDocument(message.document.file_id);
    } catch (e) {
      await deps.send(chatId, `Не удалось забрать файл: ${(e as Error).message}`);
      return;
    }
    await routeDocument(deps, chatId, message.document.file_name ?? 'файл', data, state);
    return;
  }
  if (message?.text && message.chat) {
    await routeText(deps, String(message.chat.id), message.text, state);
    return;
  }
  const cb = update.callback_query;
  if (cb?.data && cb.message) {
    await routeCallback(deps, String(cb.message.chat.id), cb.data, cb.id, state);
  }
}

/**
 * Обработчик `web_app_data`: разобрать указатель и отправить документ.
 *
 * `chat_id` здесь берётся из обновления, а не из полезной нагрузки — иначе
 * приложение могло бы попросить отправить файл в чужой чат. Непонятная
 * нагрузка — понятный ответ, а не молчание: человек нажал кнопку и ждёт.
 */
export async function handleWebAppData(
  deps: BotDeps,
  chatId: string,
  raw: string,
): Promise<void> {
  const request = parseWebAppData(raw);
  if (request === null) {
    await deps.send(chatId, unknownRequestText());
    return;
  }
  const result = await deliverDocx(chatId, request, sendDocument);
  await deps.send(chatId, result.text);
}

export async function runPoller(): Promise<void> {
  let offset = 0;
  let toldAboutToken = false;
  let lastSentError = '';
  const state = newRouterState();
  const deps = makeDeps(state);
  /** Очередь обработки: обновления идут одним порядком, но не ждут сети. */
  let chain = Promise.resolve();
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
    // Обработка НЕ ждёт следующего getUpdates (2026-11-12).
    //
    // Живая жалоба «нажал — непонятно, что происходит» имела вторую причину,
    // кроме тишины: цикл обрабатывал сообщения по одной и ДО ОТВЕТА Telegram
    // (`await handleUpdate`), а getUpdates — это long poll на 25-35 секунд.
    // Пока бот собирал листок, следующее нажатие ждало окончания не только
    // работы, но и чужого ожидания: до минуты. Telegram при этом повторял
    // обновление (409 Conflict) — отсюда «Too Many Requests» и «не ответил за
    // 35000мс» в журнале.
    //
    // Теперь обновления уходят в работу без ожидания, а следующий getUpdates
    // начинается сразу. Очередь одна и последовательная (`processUpdate`), то
    // есть порядок действий человека сохраняется; растягивается только ожидание
    // сети, которое раньше съедало время обработки.
    for (const update of updates) {
      offset = update.update_id + 1;
      chain = chain.then(() => processUpdate(deps, state, update));
    }
  }
}

/** Обработать одно обновление; ошибка не роняет поллер. */
async function processUpdate(
  deps: BotDeps,
  state: RouterState,
  update: TelegramUpdate,
): Promise<void> {
  try {
    await handleUpdate(deps, state, update);
  } catch (e) {
    console.error('offers-bot: ошибка обработки сообщения:', (e as Error).message);
  }
}
