/**
 * Обработчики кнопок черновика (Task 1 плана 2026-10-29).
 *
 * Раньше эти десять веток жили внутри `handleCallback` в поллере, и функция
 * перевалила за CC 24. Теперь каждая ветка — отдельная маленькая функция,
 * которую можно проверить без сети. Ветка проверки входа и защита от повторов
 * остались в `routes.ts`: они одинаковы для текста и для кнопок.
 */
import { answerCallback, editMessage, sendDocument } from './telegram.ts';
import {
  DRAFT_KEYBOARD,
  MAIN_KEYBOARD,
  addCatalogItem,
  addManualItem,
  catalogMessage,
  draftSummary,
  formatMoney,
  newDraft,
  parseManualItem,
  parseSystemCommand,
  pickSystem,
  removeItem,
  systemAddedMessage,
} from './flow/build.ts';
import * as api from './api.ts';
import type { BotDeps } from './routes.ts';
import type { ChatSession, OfferDraft } from './session.ts';
import {
  NO_OFFER_FOR_FILE,
  applyPending,
  cancelMessage,
  forgetAttach,
  type AttachPending,
  type AttachSession,
} from './attach.ts';
import {
  beginBrief,
  handleBriefText,
  handleCopyText,
  onBriefMore,
  onBriefNext,
  onBriefSkip,
  onCopyAccept,
  onCopyEdit,
  onCopySkip,
} from './flow/writer.ts';

const PAGE_SIZE = 8;

export interface CallbackCtx {
  deps: BotDeps;
  chatId: string;
  session: ChatSession;
  /**
   * Сохранить сессию и сделать её текущей в этом контексте.
   *
   * Раньше контекст держал снимок сессии и после сохранения продолжал читать
   * старый: второй шаг сценария видел состояние ДО первого. Теперь контекст
   * обновляется вместе с сохранением — это единственное место, где меняется
   * `ctx.session`.
   */
  save(next: ChatSession): void;
  /** Страница каталога: запрос и смещение, живёт в памяти поллера. */
  pages: Map<string, { query: string; offset: number }>;
  /**
   * Системы, показанные кнопками, — те же, что человек видит сейчас.
   *
   * Кнопка несёт индекс, а не код: callback_data ограничен 64 байтами, и
   * длинный код системы туда не влезет. Список хранится рядом с контекстом,
   * как страницы каталога.
   */
  foundSystems?: api.SystemItem[];
  /** Запомнить показанный список систем, чтобы `sys:<индекс>` его нашёл. */
  rememberSystems?: (systems: api.SystemItem[]) => void;
}

/**
 * Единственное место, где сессия меняется в контексте: и в поллере, и в
 * хранилище — одно состояние. Снимок сессии в контексте после сохранения
 * обновляется, иначе цепочка шагов читает прошлое состояние.
 */
export function apply(ctx: CallbackCtx, next: ChatSession): void {
  ctx.session = next;
  ctx.deps.save(ctx.chatId, next);
}

/** Общая проверка «черновик должен быть» — была скопирована в трёх местах. */
export function requireDraft(ctx: CallbackCtx): OfferDraft | null {
  return ctx.session.draft ?? null;
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

export async function showCatalog(ctx: CallbackCtx, query: string, offset: number): Promise<void> {
  if (!ctx.session.draft) {
    apply(ctx, { ...ctx.session, draft: newDraft() });
  }
  let items: api.CatalogItem[] = [];
  try {
    items = await api.catalog(query);
  } catch (e) {
    await ctx.deps.send(ctx.chatId, `Не удалось загрузить каталог: ${(e as Error).message}`);
    return;
  }
  if (items.length === 0) {
    await ctx.deps.send(ctx.chatId, catalogMessage([], query));
    return;
  }
  const page = items.slice(offset, offset + PAGE_SIZE);
  ctx.pages.set(ctx.chatId, { query, offset });
  const hasMore = items.length > offset + PAGE_SIZE;
  const keyboard = catalogKeyboard(page);
  if (hasMore) {
    (keyboard.inline_keyboard as unknown[][]).splice(0, 0, [
      { text: 'Ещё ▸', callback_data: 'more' },
    ]);
  }
  await ctx.deps.send(ctx.chatId, catalogMessage(page, query), keyboard);
}

export async function startDraft(ctx: CallbackCtx): Promise<void> {
  // Роли в боте нет: кто вошёл по коду — тот и собирает КП (2026-10-29).
  const draft = newDraft();
  apply(ctx, { ...ctx.session, draft });
  await ctx.deps.send(
    ctx.chatId,
    'Собираем предложение.\n\nДобавьте позиции из каталога или впишите свои, затем нажмите «Готово».\n\n' +
      draftSummary(draft),
    DRAFT_KEYBOARD,
  );
}

export async function sendDocx(ctx: CallbackCtx, offerId: number): Promise<void> {
  try {
    const doc = await api.docx(offerId, ctx.chatId);
    const bytes = Buffer.from(doc.docx_base64, 'base64');
    await sendDocument(ctx.chatId, doc.file_name, bytes, 'Коммерческое предложение');
    await ctx.deps.send(ctx.chatId, 'Файл готов. Можно изменить состав или получить ссылку.', {
      inline_keyboard: [
        [{ text: '🔗 Ссылкой', callback_data: `link:${offerId}` }],
        [{ text: '🧾 Новое КП', callback_data: 'new' }],
      ],
    });
  } catch (e) {
    await ctx.deps.send(ctx.chatId, `Не удалось собрать файл: ${(e as Error).message}`, MAIN_KEYBOARD);
  }
}

export async function finishDraft(ctx: CallbackCtx): Promise<void> {
  const draft = ctx.session.draft;
  if (!draft) {
    await ctx.deps.send(ctx.chatId, 'Черновик не найден — начните заново.', MAIN_KEYBOARD);
    return;
  }
  if (draft.items.length === 0) {
    await ctx.deps.send(ctx.chatId, 'В предложении нет позиций. Добавьте хотя бы одну.', DRAFT_KEYBOARD);
    return;
  }
  try {
    const offer = await api.createOffer({
      chatId: ctx.chatId,
      client: { name: draft.clientName, contact: draft.clientContact },
      items: draft.items,
      formValues: draft.formValues,
    });
    const done = { ...draft, offerId: offer.id, number: offer.number, step: 'done' as const };
    apply(ctx, { ...ctx.session, draft: done });
    await ctx.deps.send(ctx.chatId, `Предложение № ${offer.number}.\n\n${draftSummary(done)}`);
    // КП создано — дальше бриф: из ответов менеджера модель напишет вводный
    // текст и описания. Пропустить можно в один клик (2026-10-29).
    await beginBrief(ctx, offer.id);
  } catch (e) {
    await ctx.deps.send(
      ctx.chatId,
      `Не удалось сохранить предложение: ${(e as Error).message}`,
      DRAFT_KEYBOARD,
    );
  }
}

export async function handleDraftText(ctx: CallbackCtx, text: string): Promise<boolean> {
  // Бриф и правка секции перехватывают текст раньше черновика: пока человек
  // отвечает на вопрос, «Монтаж | 12 | 850» — это ответ, а не позиция.
  if (await handleCopyText(ctx, text)) return true;
  if (await handleBriefText(ctx, text)) return true;

  // «система <название>» (2026-10-31) проверяется раньше свободной позиции и
  // количества: команда не похожа на них ни форматом, ни смыслом.
  const systemQuery = parseSystemCommand(text);
  if (systemQuery !== null) {
    await handleSystemCommand(ctx, systemQuery);
    return true;
  }

  const draft = ctx.session.draft;
  if (!draft) return false;

  if (draft.step === 'manual') {
    const parsed = parseManualItem(text);
    if (!parsed) {
      await ctx.deps.send(
        ctx.chatId,
        'Не разобрал строку. Формат: Название | количество | цена.\nНапример: Монтаж водостока | 12 | 850',
      );
      return true;
    }
    apply(ctx, { ...ctx.session, draft: addManualItem(draft, parsed.name, parsed.qty, parsed.price) });
    await ctx.deps.send(ctx.chatId, draftSummary(ctx.session.draft!), DRAFT_KEYBOARD);
    return true;
  }

  if (draft.pendingCatalog) {
    const qty = Number(text.replace(',', '.'));
    if (!Number.isFinite(qty) || qty <= 0) {
      await ctx.deps.send(ctx.chatId, 'Количество должно быть числом больше нуля. Сколько добавить?');
      return true;
    }
    const card = draft.pendingCatalog;
    const next = addCatalogItem(
      draft,
      { id: card.id, name: card.name, unit: card.unit, basePrice: card.price },
      qty,
    );
    apply(ctx, { ...ctx.session, draft: next });
    await ctx.deps.send(ctx.chatId, draftSummary(next), DRAFT_KEYBOARD);
    return true;
  }

  if (draft.step === 'fields' && draft.currentField) {
    const values = { ...draft.formValues, [draft.currentField]: text.trim() };
    const next = { ...draft, formValues: values, currentField: undefined };
    apply(ctx, { ...ctx.session, draft: next });
    await finishDraft(ctx);
    return true;
  }

  return false;
}

// --- Системы в боте (Task 8, 2026-10-31) -------------------------------

/**
 * «система <название>»: найти карточку в Базе знаний и дописать её состав.
 *
 * Каталог бота систем не содержит — у системы нет цены. Поэтому выбор
 * отдельным шагом: если название подошло к нескольким системам, бот показывает
 * их списком и просит уточнить, а дописывает по одному нажатию.
 */
async function handleSystemCommand(ctx: CallbackCtx, query: string): Promise<void> {
  const draft = ctx.session.draft;
  if (!draft || !draft.offerId) {
    await ctx.deps.send(
      ctx.chatId,
      'Систему можно добавить к сохранённому КП. Сначала соберите позиции и нажмите «Готово».',
      MAIN_KEYBOARD,
    );
    return;
  }
  let systems: api.SystemItem[] = [];
  try {
    const res = await api.systems();
    systems = res.systems ?? [];
  } catch (e) {
    await ctx.deps.send(ctx.chatId, `Не удалось получить системы: ${(e as Error).message}`);
    return;
  }
  const found = pickSystem(systems, query);
  if (found.length === 0) {
    await ctx.deps.send(
      ctx.chatId,
      `Системы «${query}» не нашлось. Системы живут в Базе знаний — уточните название или откройте /catalog для материалов.`,
    );
    return;
  }
  if (found.length === 1) {
    await addSystemBySku(ctx, found[0]);
    return;
  }
  // Показанный список сохраняем рядом с контекстом: кнопка несёт индекс, и
  // без этого карта «индекс → система» потерялась бы между сообщениями.
  ctx.foundSystems = found.slice(0, PAGE_SIZE);
  ctx.rememberSystems?.(ctx.foundSystems);
  const keyboard = systemPickerKeyboard(ctx.foundSystems);
  await ctx.deps.send(
    ctx.chatId,
    `Нашлось несколько систем по «${query}». Выберите нужную:`,
    keyboard,
  );
}

/** Клавиатура выбора системы: `sys:<индекс>`, индекс — в списке страницы. */
function systemPickerKeyboard(systems: api.SystemItem[]): Record<string, unknown> {
  const rows = systems.map((s, index) => [
    {
      text: s.problems && s.problems.length > 0 ? `${s.title} (есть проблемы)` : s.title,
      callback_data: `sys:${index}`,
    },
  ]);
  rows.push([{ text: '< Назад', callback_data: 'back' }]);
  return { inline_keyboard: rows };
}

export async function onSystemPick(ctx: CallbackCtx, index: number): Promise<void> {
  // Список приехал из памяти поллера вместе с контекстом: кнопка несёт индекс,
  // а не код (callback_data ограничен 64 байтами).
  const systems = ctx.foundSystems ?? [];
  const system = systems[index];
  if (!system) {
    await ctx.deps.send(ctx.chatId, 'Список устарел — пришлите название системы ещё раз.');
    return;
  }
  await addSystemBySku(ctx, system);
}

/** Дописать состав и показать, что вошло, а что осталось без цены. */
async function addSystemBySku(ctx: CallbackCtx, system: api.SystemItem): Promise<void> {
  const draft = ctx.session.draft;
  if (!draft || !draft.offerId) {
    await ctx.deps.send(ctx.chatId, 'Черновик не найден — начните заново.', MAIN_KEYBOARD);
    return;
  }
  try {
    const res = await api.addSystem(draft.offerId, ctx.chatId, system.sku);
    await ctx.deps.send(ctx.chatId, systemAddedMessage(res), MAIN_KEYBOARD);
  } catch (e) {
    await ctx.deps.send(ctx.chatId, `Не удалось добавить систему: ${(e as Error).message}`);
  }
}

// --- Кнопки ------------------------------------------------------------

export async function onNew(ctx: CallbackCtx): Promise<void> {
  await startDraft(ctx);
}

export async function onCatalog(ctx: CallbackCtx): Promise<void> {
  await showCatalog(ctx, '', 0);
}

export async function onMore(ctx: CallbackCtx): Promise<void> {
  const page = ctx.pages.get(ctx.chatId) ?? { query: '', offset: 0 };
  await showCatalog(ctx, page.query, page.offset + PAGE_SIZE);
}

export async function onBack(ctx: CallbackCtx): Promise<void> {
  const draft = requireDraft(ctx);
  if (!draft) {
    await ctx.deps.send(ctx.chatId, 'Черновик не найден.', MAIN_KEYBOARD);
    return;
  }
  await ctx.deps.send(ctx.chatId, draftSummary(draft), DRAFT_KEYBOARD);
}

export async function onManual(ctx: CallbackCtx): Promise<void> {
  apply(ctx, { ...ctx.session, draft: { ...(ctx.session.draft ?? newDraft()), step: 'manual' } });
  await ctx.deps.send(ctx.chatId, 'Введите позицию строкой: Название | количество | цена');
}

export async function onUndo(ctx: CallbackCtx): Promise<void> {
  const draft = requireDraft(ctx);
  if (!draft || draft.items.length === 0) {
    await ctx.deps.send(ctx.chatId, 'Убирать нечего.', DRAFT_KEYBOARD);
    return;
  }
  const next = removeItem(draft, draft.items.length - 1);
  apply(ctx, { ...ctx.session, draft: next });
  await ctx.deps.send(ctx.chatId, draftSummary(next), DRAFT_KEYBOARD);
}

export async function onDone(ctx: CallbackCtx): Promise<void> {
  const draft = requireDraft(ctx);
  if (!draft) {
    await ctx.deps.send(ctx.chatId, 'Черновик не найден.', MAIN_KEYBOARD);
    return;
  }
  if (!draft.clientName) {
    apply(ctx, { ...ctx.session, draft: { ...draft, step: 'fields', currentField: 'clientName' } });
    await ctx.deps.send(ctx.chatId, 'Название клиента (организация)?');
    return;
  }
  await finishDraft(ctx);
}

export async function onPick(ctx: CallbackCtx, id: number): Promise<void> {
  try {
    const items = await api.catalog('');
    const card = items.find((i) => i.id === id);
    if (!card) {
      await ctx.deps.send(ctx.chatId, 'Карточка не найдена — возможно, её убрали из каталога.');
      return;
    }
    const draft = ctx.session.draft ?? newDraft();
    apply(ctx, {
      ...ctx.session,
      draft: {
        ...draft,
        pendingCatalog: { id: card.id, name: card.name, unit: card.unit, price: card.basePrice },
      },
    });
    await ctx.deps.send(ctx.chatId, `Сколько добавить «${card.name}» (${card.unit})?`);
  } catch (e) {
    await ctx.deps.send(ctx.chatId, `Не удалось получить карточку: ${(e as Error).message}`);
  }
}

export async function onDoc(ctx: CallbackCtx, id: number): Promise<void> {
  await sendDocx(ctx, id);
}

export async function onLink(ctx: CallbackCtx, id: number): Promise<void> {
  try {
    const link = await api.offerLink(id, ctx.chatId);
    await ctx.deps.send(ctx.chatId, `Ссылка действует 48 часов:\n${link.url}`);
  } catch (e) {
    await ctx.deps.send(ctx.chatId, `Не удалось получить ссылку: ${(e as Error).message}`);
  }
}

/**
 * Подтверждение разбора файла (2026-10-29): `att:yes` применяет, `att:no`
 * забывает.
 *
 * Живёт здесь, а не в routes.ts, чтобы не было кольца импортов:
 * routes импортирует таблицы кнопок отсюда.
 */
export async function applyAttachConfirm(ctx: CallbackCtx, yes: boolean): Promise<void> {
  const session = ctx.session as AttachSession;
  const pending = session.attach as AttachPending | undefined;
  if (!pending) {
    await ctx.deps.send(
      ctx.chatId,
      'Разбор файла уже неактуален. Пришлите файл ещё раз.',
      MAIN_KEYBOARD,
    );
    return;
  }
  const offerId = session.draft?.offerId ?? 0;
  if (!yes) {
    apply(ctx, forgetAttach(session));
    await ctx.deps.send(ctx.chatId, cancelMessage(pending), MAIN_KEYBOARD);
    return;
  }
  if (!offerId) {
    apply(ctx, forgetAttach(session));
    await ctx.deps.send(ctx.chatId, NO_OFFER_FOR_FILE, MAIN_KEYBOARD);
    return;
  }
  try {
    const reply = await applyPending(pending, async (mode, payload) =>
      api.applyAttach(offerId, ctx.chatId, mode, {
        brief: payload.kind === 'brief' ? payload.brief : undefined,
        spec: payload.kind === 'spec' ? payload.spec : undefined,
      }),
    );
    apply(ctx, forgetAttach(session));
    await ctx.deps.send(ctx.chatId, reply, MAIN_KEYBOARD);
  } catch (e) {
    await ctx.deps.send(ctx.chatId, `Не удалось применить: ${(e as Error).message}`, MAIN_KEYBOARD);
  }
}

/** Таблица кнопок по префиксу: точное совпадение или `prefix:число`. */
export const BUTTONS: Record<string, (ctx: CallbackCtx) => Promise<void>> = {
  new: onNew,
  cat: onCatalog,
  more: onMore,
  back: onBack,
  manual: onManual,
  undo: onUndo,
  done: onDone,
  'brief:more': onBriefMore,
  'brief:next': onBriefNext,
  'brief:skip': onBriefSkip,
  'copy:accept': onCopyAccept,
  'copy:edit': onCopyEdit,
  'copy:skip': onCopySkip,
};

export const ID_BUTTONS: Record<string, (ctx: CallbackCtx, id: number) => Promise<void>> = {
  pick: onPick,
  doc: onDoc,
  link: onLink,
  // Выбор системы из списка: `sys:<индекс>`. Индекс, а не код: callback_data
  // ограничен 64 байтами, а код системы с кириллицей туда не влезет.
  sys: onSystemPick,
};

export { answerCallback, editMessage };

