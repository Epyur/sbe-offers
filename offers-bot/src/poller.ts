/**
 * Поллер Telegram: транспорт, лимиты, дедуп и маршрутизация в сценарий.
 * Модель portal-embed: getUpdates через SOCKS5 (с VDS нет прямого пути к IP
 * Telegram), отдельное состояние диалога на chat_id в памяти процесса.
 */
import {
  answerCallback,
  editMessage,
  getUpdates,
  sendDocument,
  sendMessage,
  type TelegramUpdate,
} from './telegram.ts';
import {
  applyCodeRequested,
  applyLogin,
  newSession,
  nextStep,
  restoreSession,
  type ChatSession,
} from './session.ts';
import {
  AUTH_OK,
  AUTH_PROMPT,
  DRAFT_KEYBOARD,
  HELP_TEXT,
  MAIN_KEYBOARD,
  addCatalogItem,
  addManualItem,
  catalogMessage,
  draftSummary,
  formatMoney,
  newDraft,
  parseManualItem,
  removeItem,
} from './flow.ts';
import * as api from './api.ts';

const RATE_LIMIT_PER_MIN = Number(process.env.OFFERS_BOT_RATE_LIMIT ?? 30);
const DUPLICATE_WINDOW_MS = 10_000;
const PAGE_SIZE = 8;

const sessions = new Map<string, ChatSession>();
const chatHits = new Map<string, number[]>();
const lastMessage = new Map<string, { text: string; at: number }>();
const catalogPages = new Map<string, { query: string; offset: number }>();

function sessionFor(chatId: string): ChatSession {
  let s = sessions.get(chatId);
  if (!s) {
    s = newSession();
    sessions.set(chatId, s);
  }
  return s;
}

function saveSession(chatId: string, session: ChatSession): void {
  sessions.set(chatId, session);
}

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
async function sessionRestored(chatId: string): Promise<ChatSession> {
  const known = sessions.get(chatId);
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
  sessions.set(chatId, restored);
  if (restored.step === 'authed') {
    console.log(`offers-bot: чат ${chatId} восстановлен — ${restored.email}`);
  }
  return restored;
}

function rateOk(chatId: string): boolean {
  if (!Number.isFinite(RATE_LIMIT_PER_MIN) || RATE_LIMIT_PER_MIN <= 0) return true;
  const now = Date.now();
  const cut = now - 60_000;
  const kept = (chatHits.get(chatId) ?? []).filter((t) => t > cut);
  if (kept.length >= RATE_LIMIT_PER_MIN) {
    chatHits.set(chatId, kept);
    return false;
  }
  kept.push(now);
  chatHits.set(chatId, kept);
  return true;
}

function isDuplicate(chatId: string, text: string): boolean {
  const now = Date.now();
  const last = lastMessage.get(chatId);
  if (last && last.text === text && now - last.at < DUPLICATE_WINDOW_MS) return true;
  lastMessage.set(chatId, { text, at: now });
  return false;
}

function catalogKeyboard(items: api.CatalogItem[]): Record<string, unknown> {
  const rows = items.map((item) => [
    {
      text: `${item.name} — ${formatMoney(item.basePrice)}`,
      callback_data: `pick:${item.id}`,
    },
  ]);
  rows.push([{ text: '« Назад к черновику', callback_data: 'back' }]);
  return { inline_keyboard: rows };
}

async function showCatalog(chatId: string, query: string, offset: number): Promise<void> {
  const session = sessionFor(chatId);
  if (!session.draft) {
    saveSession(chatId, { ...session, draft: newDraft() });
  }
  let items: api.CatalogItem[] = [];
  try {
    items = await api.catalog(query);
  } catch (e) {
    await sendMessage(chatId, `Не удалось загрузить каталог: ${(e as Error).message}`);
    return;
  }
  if (items.length === 0) {
    await sendMessage(chatId, catalogMessage([], query));
    return;
  }
  const page = items.slice(offset, offset + PAGE_SIZE);
  catalogPages.set(chatId, { query, offset });
  const hasMore = items.length > offset + PAGE_SIZE;
  const keyboard = catalogKeyboard(page);
  if (hasMore) {
    (keyboard.inline_keyboard as unknown[][]).splice(0, 0, [
      { text: 'Ещё ▸', callback_data: 'more' },
    ]);
  }
  await sendMessage(chatId, catalogMessage(page, query), keyboard);
}

async function startDraft(chatId: string, session: ChatSession): Promise<void> {
  // Роли в боте нет: кто вошёл по коду — тот и собирает КП (2026-10-29).
  const draft = newDraft();
  saveSession(chatId, { ...session, draft });
  await sendMessage(
    chatId,
    'Собираем предложение.\n\nДобавьте позиции из каталога или впишите свои, затем нажмите «Готово».\n\n' +
      draftSummary(draft),
    DRAFT_KEYBOARD,
  );
}

async function sendDocx(chatId: string, offerId: number): Promise<void> {
  try {
    const doc = await api.docx(offerId, chatId);
    const bytes = Buffer.from(doc.docx_base64, 'base64');
    await sendDocument(chatId, doc.file_name, bytes, 'Коммерческое предложение');
    await sendMessage(chatId, 'Файл готов. Можно изменить состав или получить ссылку.', {
      inline_keyboard: [
        [{ text: '🔗 Ссылкой', callback_data: `link:${offerId}` }],
        [{ text: '🧾 Новое КП', callback_data: 'new' }],
      ],
    });
  } catch (e) {
    await sendMessage(chatId, `Не удалось собрать файл: ${(e as Error).message}`, MAIN_KEYBOARD);
  }
}

async function finishDraft(chatId: string, session: ChatSession): Promise<void> {
  const draft = session.draft;
  if (!draft) {
    await sendMessage(chatId, 'Черновик не найден — начните заново.', MAIN_KEYBOARD);
    return;
  }
  if (draft.items.length === 0) {
    await sendMessage(chatId, 'В предложении нет позиций. Добавьте хотя бы одну.', DRAFT_KEYBOARD);
    return;
  }
  try {
    const offer = await api.createOffer({
      chatId,
      client: { name: draft.clientName, contact: draft.clientContact },
      items: draft.items,
      formValues: draft.formValues,
    });
    const done = { ...draft, offerId: offer.id, step: 'done' as const };
    saveSession(chatId, { ...session, draft: done });
    await sendMessage(chatId, `Предложение № ${offer.number}.\n\n${draftSummary(done)}`);
    await sendDocx(chatId, offer.id);
  } catch (e) {
    await sendMessage(chatId, `Не удалось сохранить предложение: ${(e as Error).message}`, DRAFT_KEYBOARD);
  }
}

async function handleDraftText(chatId: string, session: ChatSession, text: string): Promise<boolean> {
  const draft = session.draft;
  if (!draft) return false;

  if (draft.step === 'manual') {
    const parsed = parseManualItem(text);
    if (!parsed) {
      await sendMessage(
        chatId,
        'Не разобрал строку. Формат: Название | количество | цена.\nНапример: Монтаж водостока | 12 | 850',
      );
      return true;
    }
    const next = { ...session, draft: addManualItem(draft, parsed.name, parsed.qty, parsed.price) };
    saveSession(chatId, next);
    await sendMessage(chatId, draftSummary(next.draft!), DRAFT_KEYBOARD);
    return true;
  }

  if (draft.pendingCatalog) {
    const qty = Number(text.replace(',', '.'));
    if (!Number.isFinite(qty) || qty <= 0) {
      await sendMessage(chatId, 'Количество должно быть числом больше нуля. Сколько добавить?');
      return true;
    }
    const card = draft.pendingCatalog;
    const next = {
      ...session,
      draft: addCatalogItem(draft, { id: card.id, name: card.name, unit: card.unit, basePrice: card.price }, qty),
    };
    saveSession(chatId, next);
    await sendMessage(chatId, draftSummary(next.draft!), DRAFT_KEYBOARD);
    return true;
  }

  if (draft.step === 'fields' && draft.currentField) {
    const values = { ...draft.formValues, [draft.currentField]: text.trim() };
    const next = { ...draft, formValues: values, currentField: undefined };
    saveSession(chatId, { ...session, draft: next });
    await finishDraft(chatId, { ...session, draft: next });
    return true;
  }

  return false;
}

async function handleText(chatId: string, text: string): Promise<void> {
  const trimmed = text.trim();
  if (isDuplicate(chatId, trimmed)) return;
  if (!rateOk(chatId)) {
    await sendMessage(chatId, 'Слишком много сообщений подряд — подождите минуту.');
    return;
  }

  // Восстанавливаем вход из постоянной привязки: после перезапуска бота
    // знакомый человек не должен заново запрашивать код.
  const session = await sessionRestored(chatId);

  if (trimmed === '/start') {
    if (session.step === 'authed') {
      await sendMessage(chatId, `Вы вошли как ${session.email}.`, MAIN_KEYBOARD);
      return;
    }
    // /start до входа сбрасывает только шаг: e-mail сохраняем, чтобы человек
    // не вводил его заново. Роли в боте нет, терять нечего.
    saveSession(chatId, { ...newSession(), step: session.step, email: session.email });
    await sendMessage(chatId, AUTH_PROMPT);
    return;
  }

  if (session.step !== 'authed') {
    const result = nextStep(session, trimmed);
    switch (result.kind) {
      case 'prompt':
        await sendMessage(chatId, result.text);
        return;
      case 'request_code':
        try {
          await api.requestCode(result.email, chatId);
        } catch (e) {
          await sendMessage(chatId, `Не удалось отправить код: ${(e as Error).message}`);
          return;
        }
        saveSession(chatId, applyCodeRequested(session, result.email));
        await sendMessage(chatId, result.text);
        return;
      case 'verify_code': {
        try {
          const who = await api.verifyCode(chatId, result.email, result.code);
          saveSession(chatId, applyLogin(session, who.email));
          await sendMessage(chatId, AUTH_OK(who.email), MAIN_KEYBOARD);
        } catch (e) {
          await sendMessage(chatId, (e as Error).message);
        }
        return;
      }
      default:
        return;
    }
  }

  if (trimmed === '❓ Справка' || trimmed === '/help') {
    await sendMessage(chatId, HELP_TEXT, MAIN_KEYBOARD);
    return;
  }
  if (trimmed === '🧾 Собрать КП' || trimmed === '/new') {
    await startDraft(chatId, session);
    return;
  }
  if (trimmed === '📂 Мои КП' || trimmed === '/my') {
    try {
      const offers = await api.listOffers(chatId);
      if (offers.length === 0) {
        await sendMessage(chatId, 'Предложений пока нет.', MAIN_KEYBOARD);
        return;
      }
      const lines = offers.slice(0, 15).map((o) => {
        const client = o.client?.name ? ` — ${o.client.name}` : '';
        return `№ ${o.number}${client} · ${formatMoney(o.total)} руб.`;
      });
      const buttons = offers.slice(0, 15).map((o) => [
        { text: `📄 ${o.number}`, callback_data: `doc:${o.id}` },
      ]);
      await sendMessage(chatId, `Последние предложения:\n\n${lines.join('\n')}`, { inline_keyboard: buttons });
    } catch (e) {
      await sendMessage(chatId, `Не удалось загрузить список: ${(e as Error).message}`, MAIN_KEYBOARD);
    }
    return;
  }

  if (await handleDraftText(chatId, session, trimmed)) return;

  await sendMessage(chatId, 'Не понял. Выберите действие кнопкой ниже или отправьте /help.', MAIN_KEYBOARD);
}

async function handleCallback(chatId: string, data: string, callbackId: string): Promise<void> {
  await answerCallback(callbackId);
  // Та же логика, что у текста: после перезапуска нажатие на кнопку не должно
  // снова требовать код, если чат уже привязан.
  const session = await sessionRestored(chatId);
  if (session.step !== 'authed') {
    await sendMessage(chatId, AUTH_PROMPT);
    return;
  }

  if (data === 'new') {
    await startDraft(chatId, session);
    return;
  }
  if (data === 'cat') {
    await showCatalog(chatId, '', 0);
    return;
  }
  if (data === 'more') {
    const page = catalogPages.get(chatId) ?? { query: '', offset: 0 };
    await showCatalog(chatId, page.query, page.offset + PAGE_SIZE);
    return;
  }
  if (data === 'back') {
    const draft = session.draft;
    if (!draft) {
      await sendMessage(chatId, 'Черновик не найден.', MAIN_KEYBOARD);
      return;
    }
    await sendMessage(chatId, draftSummary(draft), DRAFT_KEYBOARD);
    return;
  }
  if (data === 'manual') {
    saveSession(chatId, { ...session, draft: { ...(session.draft ?? newDraft()), step: 'manual' } });
    await sendMessage(chatId, 'Введите позицию строкой: Название | количество | цена');
    return;
  }
  if (data === 'undo') {
    const draft = session.draft;
    if (!draft || draft.items.length === 0) {
      await sendMessage(chatId, 'Убирать нечего.', DRAFT_KEYBOARD);
      return;
    }
    const next = removeItem(draft, draft.items.length - 1);
    saveSession(chatId, { ...session, draft: next });
    await sendMessage(chatId, draftSummary(next), DRAFT_KEYBOARD);
    return;
  }
  if (data === 'done') {
    const draft = session.draft;
    if (!draft) {
      await sendMessage(chatId, 'Черновик не найден.', MAIN_KEYBOARD);
      return;
    }
    const missing = [];
    if (!draft.clientName) missing.push('clientName');
    if (missing.length > 0) {
      saveSession(chatId, { ...session, draft: { ...draft, step: 'fields', currentField: 'clientName' } });
      await sendMessage(chatId, 'Название клиента (организация)?');
      return;
    }
    await finishDraft(chatId, session);
    return;
  }
  if (data.startsWith('pick:')) {
    const id = Number(data.slice(5));
    try {
      const items = await api.catalog('');
      const card = items.find((i) => i.id === id);
      if (!card) {
        await sendMessage(chatId, 'Карточка не найдена — возможно, её убрали из каталога.');
        return;
      }
      const draft = session.draft ?? newDraft();
      saveSession(chatId, {
        ...session,
        draft: {
          ...draft,
          pendingCatalog: { id: card.id, name: card.name, unit: card.unit, price: card.basePrice },
        },
      });
      await sendMessage(chatId, `Сколько добавить «${card.name}» (${card.unit})?`);
    } catch (e) {
      await sendMessage(chatId, `Не удалось получить карточку: ${(e as Error).message}`);
    }
    return;
  }
  if (data.startsWith('doc:')) {
    await sendDocx(chatId, Number(data.slice(4)));
    return;
  }
  if (data.startsWith('link:')) {
    const id = Number(data.slice(5));
    try {
      const link = await api.offerLink(id, chatId);
      await sendMessage(chatId, `Ссылка действует 48 часов:\n${link.url}`);
    } catch (e) {
      await sendMessage(chatId, `Не удалось получить ссылку: ${(e as Error).message}`);
    }
  }
}

async function handleUpdate(update: TelegramUpdate): Promise<void> {
  if (update.message?.text && update.message.chat) {
    await handleText(String(update.message.chat.id), update.message.text);
    return;
  }
  const cb = update.callback_query;
  if (cb?.data && cb.message) {
    await handleCallback(String(cb.message.chat.id), cb.data, cb.id);
  }
}

export async function runPoller(): Promise<void> {
  let offset = 0;
  let toldAboutToken = false;
  let lastSentError = '';
  console.log('offers-bot: поллер запущен');

  // Отметка «бот жив» — САМО измерение, а не сообщение: её надо слать каждую
  // итерацию, иначе bot_last_seen_at замирает и плагин объявляет живого бота
  // мёртвым. Поэтому тот же текст ошибки не глушим, а лишь не логируем
  // повторно в консоль.
  const report = async (error: string): Promise<void> => {
    if (error !== lastSentError) {
      lastSentError = error;
    }
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
      } else {
        console.error('offers-bot: getUpdates:', text);
      }
      await report(text);
      await new Promise((resolve) => setTimeout(resolve, 15000));
      continue;
    }
    for (const update of updates) {
      offset = update.update_id + 1;
      try {
        await handleUpdate(update);
      } catch (e) {
        console.error('offers-bot: ошибка обработки сообщения:', (e as Error).message);
      }
    }
  }
}

/** Экспорт для стендов: проверка ответов без сети. */
export { editMessage };