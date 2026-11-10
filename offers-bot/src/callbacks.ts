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
import type { BotDeps, CombatPickerState } from './routes.ts';
import type { ChatSession, OfferDraft } from './session.ts';
import {
  COMBAT_KEYBOARD,
  COMBAT_PAGE_SIZE,
  withCombatMenu,
  type CombatPickerPage,
  combatAskQuery,
  combatIntro,
  combatListMessage,
  combatNotEmptyMessage,
  combatPickerKeyboard,
  combatPickerMessage,
  combatSheetMessage,
  combatSideAddedMessage,
} from './flow/combat.ts';
import {
  NO_OFFER_FOR_FILE,
  applyPending,
  cancelMessage,
  forgetAttach,
  type AttachPending,
  type AttachSession,
} from './attach.ts';
import { EDIT_KEYBOARD, resumeMessage, resumeSession } from './flow/resume.ts';
import {
  BRIEF_FILE_KEYBOARD,
  beginBrief,
  briefIntro,
  handleBriefText,
  handleCopyText,
  onBriefFile,
  onBriefMore,
  onBriefNext,
  onBriefSkip,
  onCopyAccept,
  onCopyEdit,
  onCopySkip,
  startCopy,
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
  /** Стороны боевого листка, показанные кнопками (2026-11-09). */
  foundCombat?: api.CombatSystemItem[];
  /** Запомнить список сторон, чтобы `oursel:<индекс>`/`compsel:<индекс>` его нашли. */
  rememberCombat?: (items: api.CombatSystemItem[]) => void;
  /**
   * Страница выбора стороны боевого листка (2026-11-10): запрос, смещение и
   * отмеченные строки. Живёт в памяти поллера рядом с найденными сторонами.
   */
  combatPicker?: CombatPickerState;
  /** Сохранить состояние выбора после листания или отметки строки. */
  rememberCombatPicker?: (picker: CombatPickerState) => void;
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

/**
 * «🧾 Собрать КП»: сначала разговор о контексте, а не каталог (2026-11-01).
 *
 * Раньше здесь сразу открывалась клавиатура черновика с «Добавить из
 * каталога» — менеджер начинал с позиций, не зная объекта, адресата и систем.
 * Теперь КП создаётся ПУСТЫМ (номер выдаётся сразу), и начинается бриф: объект,
 * характеристики, адресат, задачи, системы, условия. Каталог появится после —
 * на этапе позиций.
 *
 * Почему создаём сразу, а не после позиций: файл с описанием объекта менеджер
 * присылает в самом начале, а файл прикладывается к строке предложения
 * (вложение, бриф). Без строки принять его некуда.
 */
export async function startDraft(ctx: CallbackCtx): Promise<void> {
  let offerId = 0;
  let number = '';
  try {
    const offer = await api.createOffer({
      chatId: ctx.chatId,
      client: { name: '', contact: '' },
      items: [],
      formValues: {},
    });
    offerId = offer.id;
    number = offer.number;
  } catch (e) {
    // Без строки в базе разговор бессмысленен: файлы и бриф привязать некуда.
    await ctx.deps.send(
      ctx.chatId,
      `Не удалось начать предложение: ${(e as Error).message}`,
      MAIN_KEYBOARD,
    );
    return;
  }
  const draft = { ...newDraft(), offerId, number, stage: 'context' as const };
  apply(ctx, { ...ctx.session, draft });
  await ctx.deps.send(ctx.chatId, `Предложение № ${number}. ${briefIntro()}`);
  await beginBrief(ctx, offerId);
}

/** Этап позиций: только теперь показывается каталог. */
export async function startPositions(ctx: CallbackCtx): Promise<void> {
  const draft = ctx.session.draft;
  if (!draft) {
    await ctx.deps.send(ctx.chatId, 'Черновик не найден — начните заново.', MAIN_KEYBOARD);
    return;
  }
  apply(ctx, { ...ctx.session, draft: { ...draft, stage: 'positions', step: 'catalog' } });
  await ctx.deps.send(
    ctx.chatId,
    'Контекст собран. Теперь позиции.\n\n' +
      'Добавьте материалы из каталога, допишите свои строки или нажмите «✅ Готово» — ' +
      'если позиции уже есть в присланной таблице.\n\n' +
      draftSummary(ctx.session.draft!),
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

/**
 * Закончить черновик: дописать состав и данные клиента в НАЧАТОЕ предложение.
 *
 * С 2026-11-01 КП создаётся в начале разговора (номер выдаётся сразу, чтобы
 * принять файл и бриф), поэтому новый offer здесь не создаётся — иначе на одно
 * КП было бы два номера и две строки в «Мои КП».
 *
 * Пустой состав — законный результат (2026-11-01): менеджерам на первых шагах
 * нужна описательная часть, цены они вписывают сами. Раньше «Готово» без
 * позиций упиралось в отказ, и КП нельзя было ни собрать, ни потом открыть.
 */
export async function finishDraft(ctx: CallbackCtx): Promise<void> {
  const draft = ctx.session.draft;
  if (!draft) {
    await ctx.deps.send(ctx.chatId, 'Черновик не найден — начните заново.', MAIN_KEYBOARD);
    return;
  }
  try {
    // Состав мог приехать и файлом (applyAttach пишет в КП на сервере), поэтому
    // перечитываем предложение: локальный список в сессии — не истина.
    const saved = await api.patchOffer(draft.offerId, {
      chatId: ctx.chatId,
      client: { name: draft.clientName, contact: draft.clientContact },
      items: draft.items,
      formValues: draft.formValues,
    });
    const done = {
      ...draft,
      offerId: saved.id,
      number: saved.number,
      items: saved.items.map((i) => ({
        catalogId: i.catalogId,
        ekn: i.ekn,
        name: i.name,
        unit: i.unit,
        qty: i.qty,
        price: i.price,
      })),
      step: 'done' as const,
    };
    apply(ctx, { ...ctx.session, draft: done });
    const summary = done.items.length > 0
      ? draftSummary(done)
      : 'Позиций нет — документ соберётся с описательной частью, состав и цены впишете сами.';
    await ctx.deps.send(ctx.chatId, `Предложение № ${done.number}.\n\n${summary}`);
    // Состав готов — дальше согласование текстов. Бриф к этому моменту уже
    // собран (он шёл первым), и повторно его не запускаем.
    await startCopy(ctx);
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

  // На этапе контекста позиции не принимаются (2026-11-01): «Монтаж | 12 | 850»
  // посреди разговора про объект — это, скорее всего, ответ на вопрос, и молча
  // записать его строкой КП значило бы потерять ответ менеджера.
  if (draft.stage === 'context') {
    return false;
  }

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
 * «система <название>»: найти карточку в Базе знаний и выбрать её для КП.
 *
 * Каталог бота систем не содержит — у системы нет цены. Поэтому выбор
 * отдельным шагом: если название подошло к нескольким системам, бот показывает
 * их списком и просит уточнить.
 *
 * Позиций выбор системы НЕ добавляет (2026-11-03): в КП попадают обоснование и
 * описание продуктов, а состав с ценами менеджер заводит сам. Раньше состав
 * дописывался, и в таблице КП одни и те же продукты стояли дважды — с ценой
 * из файла и с нулём из системы.
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
    await addPickedSystem(ctx, found[0]);
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
      text: s.problems && s.problems.length > 0 ? `${s.title} (есть замечания)` : s.title,
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
  await addPickedSystem(ctx, system);
}

/** Выбрать систему для КП: позиций она не добавляет (2026-11-03). */
async function addPickedSystem(ctx: CallbackCtx, system: api.SystemItem): Promise<void> {
  const draft = ctx.session.draft;
  if (!draft || !draft.offerId) {
    await ctx.deps.send(ctx.chatId, 'Черновик не найден — начните заново.', MAIN_KEYBOARD);
    return;
  }
  // Тексты системы пишет модель — разговор об этом честно предупреждает, иначе
  // в чате четверть минуты не происходит ничего видимого (2026-11-06).
  await ctx.deps.send(
    ctx.chatId,
    `Система «${system.title}» добавляется. Пишу описание и преимущества — это может занять до минуты.`,
  );
  try {
    const res = await api.addSystem(draft.offerId, ctx.chatId, systemChoice(system));
    await ctx.deps.send(ctx.chatId, systemAddedMessage(res), MAIN_KEYBOARD);
  } catch (e) {
    // Выбор системы мог сохраниться, а тексты — нет: так и говорим, чтобы
    // менеджер не выбирал её повторно (2026-11-06).
    await ctx.deps.send(
      ctx.chatId,
      `Система выбрана, но тексты не записались: ${(e as Error).message}. ` +
        'Повторять выбор не нужно — нажмите «Собрать КП» ещё раз или скачайте документ из плагина.',
    );
  }
}

/**
 * Чем бот называет выбранную систему.
 *
 * Код, если он есть: он есть у систем, заведённых по стандарту. У живых
 * карточек кода нет, и тогда выбор несёт заметку — иначе сервер получает
 * пустую строку и отвечает «система не найдена» (2026-11-02).
 */
export function systemChoice(system: api.SystemItem): api.SystemChoice {
  const sku = system.sku.trim();
  if (sku) return { sku };
  if (system.noteId && system.noteId > 0) return { noteId: system.noteId };
  return { sku };
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
  // «Готово» до этапа позиций (2026-11-01) — это «пропустить остаток
  // разговора»: человек хочет сразу к позициям, а не заполнять бриф до конца.
  if (draft.stage !== 'positions') {
    await startPositions(ctx);
    return;
  }
  if (!draft.clientName) {
    // Имя клиента бриф уже спрашивал (адресат КП) — второй раз не спрашиваем,
    // если оно есть в форме предложения (2026-11-01).
    try {
      const offer = await api.getOffer(draft.offerId, ctx.chatId);
      const fromBrief = (offer.formValues?.client_name ?? '').trim();
      if (fromBrief) {
        apply(ctx, { ...ctx.session, draft: { ...draft, clientName: fromBrief } });
        await finishDraft(ctx);
        return;
      }
    } catch (e) {
      // Не прочитали — спросим как раньше; хуже не станет.
      console.warn('КП: не удалось прочитать адресата из брифа:', (e as Error).message);
    }
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

// --- Продолжение сохранённого КП (2026-11-01) ---------------------------

/**
 * Открыть сохранённое КП на правку.
 *
 * Раньше к КП можно было вернуться только из плагина: бот жил одним
 * черновиком в памяти, и после «Готово» без позиций продолжить разговор было
 * нечем. Теперь состояние черновика собирается из самого предложения, а
 * происхождение данных неважно — файл, каталог или ручные строки.
 */
export async function onEditOffer(ctx: CallbackCtx, id: number): Promise<void> {
  let offer: api.OfferPayload;
  try {
    offer = await api.getOffer(id, ctx.chatId);
  } catch (e) {
    await ctx.deps.send(ctx.chatId, `Не удалось открыть предложение: ${(e as Error).message}`, MAIN_KEYBOARD);
    return;
  }
  apply(ctx, resumeSession(ctx.session, offer));
  await ctx.deps.send(ctx.chatId, resumeMessage(offer), EDIT_KEYBOARD);
}

/**
 * «📚 Дополнить контекст»: вернуться к брифу этого КП.
 *
 * Бриф уже существует — он заводится вместе с КП. Сервис сам скажет, какая
 * тема не заполнена, поэтому разговор продолжается с места, а не с начала.
 */
export async function onEditBrief(ctx: CallbackCtx): Promise<void> {
  const offerId = ctx.session.draft?.offerId ?? 0;
  if (!offerId) {
    await ctx.deps.send(ctx.chatId, 'Сначала откройте предложение из «Мои КП».', MAIN_KEYBOARD);
    return;
  }
  apply(ctx, { ...ctx.session, briefDone: false, briefSkipped: false });
  await beginBrief(ctx, offerId);
}

/** «📄 Документ»: собрать файл по сохранённому КП. */
export async function onEditDoc(ctx: CallbackCtx): Promise<void> {
  const offerId = ctx.session.draft?.offerId ?? 0;
  if (!offerId) {
    await ctx.deps.send(ctx.chatId, 'Сначала откройте предложение из «Мои КП».', MAIN_KEYBOARD);
    return;
  }
  await sendDocx(ctx, offerId);
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
        context: payload.kind === 'spec' ? payload.context : undefined,
      }),
    );
    apply(ctx, forgetAttach(session));
    await ctx.deps.send(ctx.chatId, reply, MAIN_KEYBOARD);
  } catch (e) {
    await ctx.deps.send(ctx.chatId, `Не удалось применить: ${(e as Error).message}`, MAIN_KEYBOARD);
  }
}

// --- Боевой листок в боте (2026-11-09) ---------------------------------
//
// Листок собирается кнопками: выбрать наш продукт, добавить конкурента,
// забрать DOCX. Текстового ввода здесь нет — стороны выбираются из списков,
// которые уже лежат в кеше сервера.

/** «⚔️ Боевой листок»: создать пустой листок с номером и начать диалог. */
export async function startCombat(ctx: CallbackCtx): Promise<void> {
  let sheet: api.CombatSheet;
  try {
    sheet = await api.createCombat(ctx.chatId, 'Боевой листок');
  } catch (e) {
    await ctx.deps.send(
      ctx.chatId,
      `Не удалось начать боевой листок: ${(e as Error).message}`,
      MAIN_KEYBOARD,
    );
    return;
  }
  apply(ctx, {
    ...ctx.session,
    combat: { sheetId: sheet.id, number: sheet.number },
  });
  await ctx.deps.send(ctx.chatId, combatIntro(sheet.number), COMBAT_KEYBOARD);
}

/** Показать сводку по листку с клавиатурой действий. */
async function showCombatSheet(ctx: CallbackCtx, sheet: api.CombatSheet, hint?: string): Promise<void> {
  await ctx.deps.send(ctx.chatId, combatSheetMessage(sheet, hint), COMBAT_KEYBOARD);
}

/** «📂 Мои боевые листки»: последние листки этого чата. */
export async function handleMyCombat(deps: BotDeps, chatId: string): Promise<void> {
  try {
    const sheets = await api.listCombat(chatId);
    const list = combatListMessage(sheets);
    if (!list) {
      await deps.send(chatId, 'Боевых листков пока нет. Начните кнопкой «⚔️ Боевой листок».', MAIN_KEYBOARD);
      return;
    }
    await deps.send(chatId, list.text, list.keyboard);
  } catch (e) {
    await deps.send(chatId, `Не удалось загрузить список: ${(e as Error).message}`, MAIN_KEYBOARD);
  }
}

/** Открыть сохранённый боевой листок: `cbe:<id>`. */
export async function onCombatOpen(ctx: CallbackCtx, id: number): Promise<void> {
  const sheet = await loadOwnCombat(ctx, id);
  if (!sheet) return;
  apply(ctx, { ...ctx.session, combat: { sheetId: sheet.id, number: sheet.number } });
  await showCombatSheet(ctx, sheet);
}

/** Файл по листку из списка: `cbx:<id>`. */
export async function onCombatFile(ctx: CallbackCtx, id: number): Promise<void> {
  const sheet = await loadOwnCombat(ctx, id);
  if (!sheet) return;
  apply(ctx, { ...ctx.session, combat: { sheetId: sheet.id, number: sheet.number } });
  await sendCombatDocx(ctx, sheet);
}

/** Прочитать листок по id, проверяя, что он открыт в этом чате. */
async function loadOwnCombat(ctx: CallbackCtx, id: number): Promise<api.CombatSheet | null> {
  try {
    // Список приходит из «Мои боевые листки»: там только свои. Но листок мог
    // быть удалён — тогда честно говорим.
    const sheets = await api.listCombat(ctx.chatId);
    const found = sheets.find((s) => s.id === id);
    if (!found) {
      await ctx.deps.send(ctx.chatId, 'Боевой листок не найден — возможно, удалён.');
      return null;
    }
    return found;
  } catch (e) {
    await ctx.deps.send(ctx.chatId, `Не удалось открыть листок: ${(e as Error).message}`);
    return null;
  }
}

/** «➕ Наш продукт»: перейти к поиску нашего продукта (каталог + системы). */
export async function onCombatOurSys(ctx: CallbackCtx): Promise<void> {
  await startCombatPicker(ctx, 'our');
}

/** «🏷️ Добавить конкурента»: перейти к поиску продукта конкурента. */
export async function onCombatComp(ctx: CallbackCtx): Promise<void> {
  await startCombatPicker(ctx, 'competitor');
}

/**
 * Начать выбор стороны: спросить запрос, а не вываливать список.
 *
 * Каталог нашего продукта — 1500+ записей (2026-11-09): список кнопками
 * бесполезен, поэтому сначала текст, потом совпадения. Кнопка «▼ Все»
 * открывает полный список для тех, кому так удобнее.
 */
export async function startCombatPicker(ctx: CallbackCtx, side: 'our' | 'competitor'): Promise<void> {
  if (!ctx.session.combat) {
    await ctx.deps.send(ctx.chatId, 'Боевой листок не открыт — начните заново.', MAIN_KEYBOARD);
    return;
  }
  apply(ctx, { ...ctx.session, combat: { ...ctx.session.combat, picking: side } });
  // Смена стороны — новый набор совпадений: отмеченные и запрос прошлой
  // стороны к этой не относятся.
  savePicker(ctx, { side, query: '', offset: 0, picked: [], allPicked: false });
  await ctx.deps.send(ctx.chatId, combatAskQuery(side), {
    inline_keyboard: [
      [{ text: '▼ Все продукты', callback_data: 'cb:all' }],
      [{ text: '< Назад', callback_data: 'cb:back' }],
    ],
  });
}

/** Текущее состояние выбора: из контекста, а если его нет — пустое для стороны. */
function currentPicker(ctx: CallbackCtx, side: 'our' | 'competitor'): CombatPickerState {
  const saved = ctx.combatPicker;
  if (saved && saved.side === side) return saved;
  return { side, query: '', offset: 0, picked: [] };
}

/** Запомнить состояние выбора и в контексте, и в памяти поллера. */
function savePicker(ctx: CallbackCtx, picker: CombatPickerState): void {
  ctx.combatPicker = picker;
  ctx.rememberCombatPicker?.(picker);
}

/** Сторона, которая выбирается сейчас: из сессии, иначе из сохранённой страницы. */
function pickingSide(ctx: CallbackCtx): 'our' | 'competitor' {
  return ctx.session.combat?.picking ?? ctx.combatPicker?.side ?? 'our';
}

/**
 * «▼ Все продукты»: показать полный список стороны сначала.
 *
 * Это ТОТ ЖЕ список, а не новый поиск: запрос пустой у обоих, поэтому
 * отмеченные строки сохраняются. Раньше здесь строился пустой выбор, и у
 * человека, отметившего позицию и нажавшего «Все продукты», отметка молча
 * пропадала — а кнопка «Добавить выбранные» отвечала «Ничего не отмечено».
 */
export async function onCombatAll(ctx: CallbackCtx): Promise<void> {
  const side = pickingSide(ctx);
  const prev = currentPicker(ctx, side);
  await renderCombatPicker(ctx, { ...prev, side, query: '', offset: 0 });
}

/** «Дальше ▸»: следующая страница текущего набора совпадений. */
export async function onCombatMore(ctx: CallbackCtx): Promise<void> {
  const picker = currentPicker(ctx, pickingSide(ctx));
  await renderCombatPicker(ctx, { ...picker, offset: picker.offset + COMBAT_PAGE_SIZE });
}

/** «◀ В начало»: первая страница; отмеченное сохраняется. */
export async function onCombatFirst(ctx: CallbackCtx): Promise<void> {
  const picker = currentPicker(ctx, pickingSide(ctx));
  await renderCombatPicker(ctx, { ...picker, offset: 0 });
}

/**
 * Показать страницу выбора стороны (2026-11-10).
 *
 * Один экран на весь поиск: текст пронумерованным списком, номера — кнопками,
 * листание и «Добавить выбранные». Повторяет приём режима «Документы» в
 * Telegram-боте, но добавление отдельное: боевой листок принимает несколько
 * сторон сразу.
 */
async function renderCombatPicker(
  ctx: CallbackCtx,
  picker: CombatPickerState,
  found?: CombatPickerPage,
): Promise<void> {
  if (!ctx.session.combat) {
    await ctx.deps.send(ctx.chatId, 'Боевой листок не открыт — начните заново.', MAIN_KEYBOARD);
    return;
  }
  const side = picker.side;
  // Страница приходит готовой от навигации (уже есть данные) либо читается при
  // поиске. Так нажатие «Дальше» не повторяет запрос к сервису — сервер отдаёт
  // страницу за нас, а лежащий рядом набор совпадений свежий.
  const page = found ?? (await fetchCombatPage(ctx, picker));
  if (!page) return;
  // ВАЖНО: `picked` и `allPicked` не пересобираются из ответа сервера — сервер
  // о них не знает. Сюда приходит только страница; отметки живут в picker и
  // переживают листание. Раньше `savePicker` получал `{...picker}` и это
  // работало лишь до тех пор, пока вызывающий не строил picker заново.
  savePicker(ctx, { ...picker, offset: page.offset });
  if (page.total === 0) {
    // Ничего не нашлось: даём путь назад, а не тупик.
    await ctx.deps.send(
      ctx.chatId,
      combatPickerMessage(page, side, picker.query),
      withCombatMenu([
        [{ text: '▼ Все продукты', callback_data: 'cb:all' }],
        [{ text: '< Назад', callback_data: 'cb:back' }],
      ]),
    );
    return;
  }
  ctx.foundCombat = page.items;
  ctx.rememberCombat?.(page.items);
  apply(ctx, { ...ctx.session, combat: { ...ctx.session.combat, picking: side } });
  const prefix = side === 'our' ? 'oursel' : 'compsel';
  const keyboard = combatPickerKeyboard(
    page,
    prefix,
    new Set(picker.picked),
    picker.query.trim() === '',
    picker.allPicked === true,
  );
  await ctx.deps.send(
    ctx.chatId,
    combatPickerMessage(page, side, picker.query),
    withCombatMenu(keyboard.inline_keyboard as unknown[]),
  );
}

/** Прочитать страницу стороны; `null` — уже отправлена ошибка человеку. */
async function fetchCombatPage(
  ctx: CallbackCtx,
  picker: CombatPickerState,
): Promise<CombatPickerPage | null> {
  try {
    const res = picker.side === 'our'
      ? await api.combatSystems(picker.query, picker.offset, COMBAT_PAGE_SIZE)
      : await api.combatCompetitors(picker.query, picker.offset, COMBAT_PAGE_SIZE);
    const items = res.cards ?? [];
    return {
      items,
      total: res.total ?? items.length,
      offset: res.offset ?? picker.offset,
      hasMore: res.hasMore ?? false,
    };
  } catch (e) {
    await ctx.deps.send(ctx.chatId, `Не удалось получить список: ${(e as Error).message}`);
    return null;
  }
}

/**
 * Отметка стороны строки: `oursel:<номер>` / `compsel:<номер>`.
 *
 * Номер — от НАЧАЛА списка. Нажатие переключает отметку и перерисовывает ту же
 * страницу: видно, что уже отмечено, и можно отметить несколько. Сторона
 * добавляется в листок только кнопкой «✓ Добавить выбранные».
 */
export async function onCombatPick(ctx: CallbackCtx, index: number): Promise<void> {
  const picker = currentPicker(ctx, pickingSide(ctx));
  const item = (ctx.foundCombat ?? []).find((_it, i) => picker.offset + i === index);
  if (!item) {
    await ctx.deps.send(ctx.chatId, 'Список устарел — откройте выбор стороны ещё раз.');
    return;
  }
  const picked = picker.picked.includes(index)
    ? picker.picked.filter((n) => n !== index)
    : [...picker.picked, index];
  // Ручная отметка отменяет «всё найдено»: менеджер правит набор, а не берёт
  // его целиком.
  await renderCombatPicker(ctx, { ...picker, picked, allPicked: false });
  return;
}

/**
 * «☑ Выбрать все найденные»: взять всё, что нашлось по запросу (2026-11-10).
 *
 * `allPicked` — отдельный признак, а не заполненный `picked`: совпадений бывает
 * сотни, складывать их номера незачем. Отметки отдельных строк сбрасываются —
 * «всё» уже включает их. Экран перерисовывается на той же странице.
 */
export async function onCombatPickAll(ctx: CallbackCtx): Promise<void> {
  await rerenderPicker(ctx, [], true);
}

/** «✖ Снять все отметки»: вернуть пустой выбор, не трогая список. */
export async function onCombatDropAll(ctx: CallbackCtx): Promise<void> {
  await rerenderPicker(ctx, [], false);
}

/** Перерисовать ту же страницу выбора с новым набором отметок. */
async function rerenderPicker(
  ctx: CallbackCtx,
  picked: number[],
  allPicked: boolean,
): Promise<void> {
  const picker = currentPicker(ctx, pickingSide(ctx));
  await renderCombatPicker(ctx, { ...picker, picked, allPicked });
}

/**
 * «✓ Добавить выбранные»: положить в листок всё отмеченное разом (2026-11-10).
 *
 * Обычный путь — отметить номера на странице и нажать кнопку. Второй путь —
 * «Выбрать все найденные»: тогда берём ВЕСЬ список по запросу, листая страницы
 * сервера (`combatAllPaged`), а не только видимую — иначе сравнение «наш
 * продукт против всех аналогов» требовало бы помнить отметки между экранами.
 */
export async function onCombatAddPicked(ctx: CallbackCtx): Promise<void> {
  const sheet = ctx.session.combat;
  if (!sheet) {
    await ctx.deps.send(ctx.chatId, 'Боевой листок не открыт — начните заново.', MAIN_KEYBOARD);
    return;
  }
  const picker = currentPicker(ctx, pickingSide(ctx));
  let chosen: api.CombatSystemItem[];
  try {
    chosen = picker.allPicked
      ? await api.combatAllPaged(picker.side, picker.query)
      : visiblePicked(ctx, picker);
  } catch (e) {
    await ctx.deps.send(ctx.chatId, `Не удалось получить список: ${(e as Error).message}`);
    return;
  }
  if (chosen.length === 0) {
    await ctx.deps.send(ctx.chatId, 'Ничего не отмечено — нажмите номер строки, чтобы отметить.');
    return;
  }
  try {
    let updated: api.CombatSheet | null = null;
    // По одной: так у частичного сбоя остаётся честное «что успели». Сервер
    // пропускает уже добавленную сторону, повтор не плодит записей.
    for (const item of chosen) {
      updated = await api.addCombatSide(sheet.sheetId, ctx.chatId, picker.side, {
        noteId: item.noteId,
        title: item.title,
      });
    }
    // Отметки сбрасываются: листок пополнен, следующий выбор начинается заново.
    savePicker(ctx, { ...picker, picked: [], allPicked: false, offset: 0 });
    apply(ctx, { ...ctx.session, combat: { ...sheet, picking: undefined } });
    if (updated) await showCombatSheet(ctx, updated, combatSideAddedMessage(picker.side, chosenSummary(chosen)));
  } catch (e) {
    await ctx.deps.send(ctx.chatId, `Не удалось добавить: ${(e as Error).message}`);
  }
}

/** Отмеченные строки ТЕКУЩЕЙ страницы: номер — от начала списка. */
function visiblePicked(ctx: CallbackCtx, picker: CombatPickerState): api.CombatSystemItem[] {
  const visible = ctx.foundCombat ?? [];
  return picker.picked
    .map((n) => visible[n - picker.offset])
    .filter((it): it is api.CombatSystemItem => !!it);
}

/**
 * Подпись добавленного: пара названий и «и ещё N» вместо простыни из сотни
 * имён — сообщение иначе не прочитать.
 */
function chosenSummary(items: api.CombatSystemItem[]): string {
  const head = items.slice(0, 3).map((i) => i.title).join(', ');
  return items.length > 3 ? `${head} и ещё ${items.length - 3}` : head;
}

/** «⬇ DOCX»: собрать файл боевого листка. */
export async function onCombatDoc(ctx: CallbackCtx): Promise<void> {
  const combat = ctx.session.combat;
  if (!combat) {
    await ctx.deps.send(ctx.chatId, 'Боевой листок не открыт — начните заново.', MAIN_KEYBOARD);
    return;
  }
  const sheet = await loadOwnCombat(ctx, combat.sheetId);
  if (!sheet) return;
  await sendCombatDocx(ctx, sheet);
}

/** Собрать и отправить DOCX; пустой листок не печатаем. */
async function sendCombatDocx(ctx: CallbackCtx, sheet: api.CombatSheet): Promise<void> {
  if (sheet.ourNotes.length === 0 && sheet.competitorNotes.length === 0) {
    await ctx.deps.send(ctx.chatId, combatNotEmptyMessage(), COMBAT_KEYBOARD);
    return;
  }
  try {
    const doc = await api.combatDocx(sheet.id, ctx.chatId);
    const bytes = Buffer.from(doc.docx_base64, 'base64');
    await sendDocument(ctx.chatId, doc.file_name, bytes, 'Боевой листок');
    await ctx.deps.send(ctx.chatId, 'Файл готов. Можно добавить ещё стороны или нажать «✅ Готово».', COMBAT_KEYBOARD);
  } catch (e) {
    await ctx.deps.send(ctx.chatId, `Не удалось собрать файл: ${(e as Error).message}`, COMBAT_KEYBOARD);
  }
}

/** «✅ Готово»: выходим из боевого листка к меню. */
export async function onCombatDone(ctx: CallbackCtx): Promise<void> {
  const combat = ctx.session.combat;
  if (!combat) {
    await ctx.deps.send(ctx.chatId, 'Боевой листок не открыт.', MAIN_KEYBOARD);
    return;
  }
  apply(ctx, { ...ctx.session, combat: undefined });
  await ctx.deps.send(ctx.chatId, `Боевой листок № ${combat.number} закрыт.`, MAIN_KEYBOARD);
}

/** «< Назад» из списка выбора: вернуться к сводке листка. */
export async function onCombatBack(ctx: CallbackCtx): Promise<void> {
  if (!ctx.session.combat) {
    await ctx.deps.send(ctx.chatId, 'Боевой листок не открыт — начните заново.', MAIN_KEYBOARD);
    return;
  }
  apply(ctx, { ...ctx.session, combat: { ...ctx.session.combat, picking: undefined } });
  // Выбор стороны брошен — невидимая страница не должна всплыть при следующем
  // «➕» со старой отметкой.
  savePicker(ctx, { side: ctx.combatPicker?.side ?? 'our', query: '', offset: 0, picked: [] });
  const sheet = await loadOwnCombat(ctx, ctx.session.combat.sheetId);
  if (!sheet) return;
  await showCombatSheet(ctx, sheet);
}

/**
 * Текст в боевом листке — это запрос на поиск стороны (2026-11-09).
 *
 * Возвращает true, если текст поглощён. Принимается только когда бот ждёт
 * запрос (`picking` выставлен кнопкой «➕»/«🏷️»): в остальное время в листке
 * каталог не ищется, и свободная строка не должна никуда попадать.
 * Если текст набран, а сторона ещё не выбирается, — один раз объясняем, что
 * делать, и не валим разговор.
 */
export async function handleCombatText(ctx: CallbackCtx, text: string): Promise<boolean> {
  const combat = ctx.session.combat;
  if (!combat) return false;
  if (!combat.picking) {
    await ctx.deps.send(
      ctx.chatId,
      'В боевом листке наш продукт и конкурент выбираются кнопками ниже. ' +
        'Чтобы найти по названию, нажмите «➕ Наш продукт» или «🏷️ Добавить конкурента».',
      COMBAT_KEYBOARD,
    );
    return true;
  }
  const query = text.trim();
  if (!query) return true;
  // Новый запрос — новый набор совпадений: прежние отметки к нему не относятся.
  await renderCombatPicker(ctx, { side: combat.picking, query, offset: 0, picked: [], allPicked: false });
  return true;
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
  'brief:file': onBriefFile,
  'copy:accept': onCopyAccept,
  'copy:edit': onCopyEdit,
  'copy:skip': onCopySkip,
  // Продолжение сохранённого КП (2026-11-01).
  'edit:brief': onEditBrief,
  'edit:doc': onEditDoc,
  // Боевой листок (2026-11-09).
  'cb:oursys': onCombatOurSys,
  'cb:comp': onCombatComp,
  'cb:all': onCombatAll,
  'cb:more': onCombatMore,
  'cb:first': onCombatFirst,
  'cb:pickall': onCombatPickAll,
  'cb:unpickall': onCombatDropAll,
  'cb:addpicked': onCombatAddPicked,
  'cb:doc': onCombatDoc,
  'cb:done': onCombatDone,
  'cb:back': onCombatBack,
};

export const ID_BUTTONS: Record<string, (ctx: CallbackCtx, id: number) => Promise<void>> = {
  pick: onPick,
  doc: onDoc,
  link: onLink,
  // Открыть сохранённое КП на правку: `edit:<id>`.
  edit: onEditOffer,
  // Выбор системы из списка: `sys:<индекс>`. Индекс, а не код: callback_data
  // ограничен 64 байтами, а код системы с кириллицей туда не влезет.
  sys: onSystemPick,
  // Боевой листок (2026-11-09): открыть листок, файл по листку, выбор стороны.
  cbe: onCombatOpen,
  cbx: onCombatFile,
  oursel: onCombatPick,
  compsel: onCombatPick,
};

export { answerCallback, editMessage };

