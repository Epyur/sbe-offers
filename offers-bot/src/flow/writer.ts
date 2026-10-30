/**
 * Бриф и согласование текста в чате — обработчики кнопок и текста
 * (Task 9 плана 2026-10-29).
 *
 * Здесь только оркестровка: что сказать, что запомнить, какой шаг дальше.
 * Формулировки — в `flow/brief.ts` и `flow/copy.ts`, сеть — в `api.ts`.
 * Так каждая часть проверяется отдельно, а поллер остаётся тонким.
 */
import * as api from '../api.ts';
import {
  BRIEF_DONE_KEYBOARD,
  BRIEF_KEYBOARD,
  briefSummary,
  briefTurnState,
} from './brief.ts';
import {
  COPY_EDIT_KEYBOARD,
  COPY_KEYBOARD,
  advanceCopy,
  copyDone,
  copyEditPrompt,
  copySlotLabel,
  copyState,
  copyTextMessage,
  currentSlot,
  numberedSlots,
  systemsIn,
  type SectionSlot,
} from './copy.ts';
import { MAIN_KEYBOARD } from './build.ts';
import type { ChatSession } from '../session.ts';
import { apply, sendDocx, type CallbackCtx } from '../callbacks.ts';

/** Номер КП, с которым работаем: бриф и тексты живут только у него. */
export function activeOfferId(session: ChatSession): number {
  return session.draft?.offerId ?? 0;
}

/** Секции согласования для этого черновика — единственное место их расчёта. */
function slotsFor(session: ChatSession): SectionSlot[] {
  return numberedSlots(systemsIn(session.draft?.items ?? []));
}

/** Предложение и текущая секция разом: оба нужны каждому обработчику. */
function activeSlot(session: ChatSession): { offerId: number; slots: SectionSlot[]; slot: SectionSlot | null } {
  const slots = slotsFor(session);
  return { offerId: activeOfferId(session), slots, slot: currentSlot(session, slots) };
}

/**
 * Начать бриф после того, как КП создано.
 *
 * Бриф стартует НЕ на пустом месте: сервис возвращает первый незаполненный
 * вопрос, а не «расскажите всё». С одним вопросом разговор идёт ровнее —
 * человек не думает, что от него хотят сочинение.
 */
export async function beginBrief(ctx: CallbackCtx, offerId: number): Promise<void> {
  try {
    const res = await api.startBrief(offerId, ctx.chatId);
    apply(ctx, {
      ...ctx.session,
      briefTopic: res.done ? undefined : res.brief.lastTopic || 'object_name',
      briefDone: res.done,
      briefSkipped: false,
    });
    await ctx.deps.send(ctx.chatId, res.reply);
  } catch (e) {
    await ctx.deps.send(
      ctx.chatId,
      `Не удалось начать бриф: ${(e as Error).message}\nМожно собрать КП без него — текст возьмём из формы.`,
      MAIN_KEYBOARD,
    );
    apply(ctx, { ...ctx.session, briefDone: true, briefSkipped: true });
  }
}

/**
 * Текст во время брифа: один ответ — одна тема.
 *
 * Неудачные ответы модели на шаге не считаются ответом человека: сервис
 * различает «переспроси» и «модель недоступна», и бот просто повторяет его
 * текст, ничего не записывая.
 */
export async function handleBriefText(ctx: CallbackCtx, text: string): Promise<boolean> {
  const offerId = activeOfferId(ctx.session);
  if (!offerId || !ctx.session.briefTopic) return false;
  try {
    const res = await api.stepBrief(offerId, ctx.chatId, text);
    apply(ctx, briefTurnState(ctx.session, res));
    await ctx.deps.send(ctx.chatId, res.reply, res.done ? BRIEF_DONE_KEYBOARD : BRIEF_KEYBOARD);
    if (res.done) await showBriefSummary(ctx, offerId);
  } catch (e) {
    await ctx.deps.send(ctx.chatId, `Последи за брифом не вышло: ${(e as Error).message}`);
  }
  return true;
}

/** Сводка перед сборкой: что услышали и на чём промолчим. */
async function showBriefSummary(ctx: CallbackCtx, offerId: number): Promise<void> {
  try {
    const brief = await api.getBrief(offerId, ctx.chatId);
    await ctx.deps.send(
      ctx.chatId,
      briefSummary({ payload: brief.payload as Record<string, unknown>, done: true }),
      BRIEF_DONE_KEYBOARD,
    );
  } catch (e) {
    await ctx.deps.send(ctx.chatId, `Не удалось прочитать бриф: ${(e as Error).message}`);
  }
}

export async function onBriefMore(ctx: CallbackCtx): Promise<void> {
  const offerId = activeOfferId(ctx.session);
  if (!offerId) {
    await ctx.deps.send(ctx.chatId, 'Бриф не найден — начните сборку КП заново.', MAIN_KEYBOARD);
    return;
  }
  // «Дополнить» открывает следующую незаполненную тему заново: сервис сам
  // скажет, какая она.
  apply(ctx, { ...ctx.session, briefDone: false });
  await beginBrief(ctx, offerId);
}

export async function onBriefNext(ctx: CallbackCtx): Promise<void> {
  apply(ctx, { ...ctx.session, briefTopic: undefined, briefDone: true });
  await startCopy(ctx);
}

export async function onBriefSkip(ctx: CallbackCtx): Promise<void> {
  apply(ctx, { ...ctx.session, briefTopic: undefined, briefDone: true, briefSkipped: true });
  await startCopy(ctx);
}

// --- Согласование текста ------------------------------------------------

/** Открыть первую секцию. */
export async function startCopy(ctx: CallbackCtx): Promise<void> {
  const offerId = activeOfferId(ctx.session);
  if (!offerId) {
    await ctx.deps.send(ctx.chatId, 'Черновик не найден — начните заново.', MAIN_KEYBOARD);
    return;
  }
apply(ctx, copyState(ctx.session, 0));
  await showSlot(ctx, offerId, slotsFor(ctx.session));
}

/** Показать текущую секцию: написать заново и спросить. */
async function showSlot(ctx: CallbackCtx, offerId: number, slots: SectionSlot[]): Promise<void> {
  const slot = currentSlot(ctx.session, slots);
  if (copyDone(ctx.session, slots) || !slot) {
    await finishCopy(ctx, offerId);
    return;
  }
  let text = '';
  try {
    const res = await api.writeCopy(offerId, ctx.chatId, slot.kind, slot.systemIndex ?? 0);
    text = res.text;
  } catch (e) {
    await ctx.deps.send(ctx.chatId, `Не удалось написать текст: ${(e as Error).message}`, COPY_KEYBOARD);
    return;
  }
  await ctx.deps.send(ctx.chatId, copyTextMessage(copySlotLabel(slot), text), COPY_KEYBOARD);
}

export async function onCopyAccept(ctx: CallbackCtx): Promise<void> {
  await acceptAndAdvance(ctx);
}

async function acceptAndAdvance(ctx: CallbackCtx, text = ''): Promise<void> {
  const { offerId, slots, slot } = activeSlot(ctx.session);
  if (!offerId || !slot) {
    await ctx.deps.send(ctx.chatId, 'Секция не найдена — начните заново.', MAIN_KEYBOARD);
    return;
  }
  try {
    await api.acceptCopy(offerId, ctx.chatId, slot.kind, text, slot.systemIndex ?? 0);
  } catch (e) {
    await ctx.deps.send(ctx.chatId, `Не удалось сохранить текст: ${(e as Error).message}`, COPY_KEYBOARD);
    return;
  }
  apply(ctx, advanceCopy(ctx.session, slots));
  await showSlot(ctx, offerId, slots);
}

export async function onCopySkip(ctx: CallbackCtx): Promise<void> {
  // Пустой текст у сервиса означает «пропустить»: ключ удаляется, и сборка
  // документа отличает пропуск от пустой строки.
  await acceptAndAdvance(ctx, '');
}

export async function onCopyEdit(ctx: CallbackCtx): Promise<void> {
  const { slot } = activeSlot(ctx.session);
  if (!slot) {
    await ctx.deps.send(ctx.chatId, 'Секция не найдена — начните заново.', MAIN_KEYBOARD);
    return;
  }
  apply(ctx, { ...ctx.session, copyEditing: true });
  await ctx.deps.send(ctx.chatId, copyEditPrompt(copySlotLabel(slot)));
}

/**
 * Текст менеджера во время правки секции.
 *
 * Принятый текст берётся ИЗ ОТВЕТА сервиса: вычитка может вернуть текст,
 * отличный от присланного, а обещать одно и печатать другое нельзя.
 */
export async function handleCopyText(ctx: CallbackCtx, text: string): Promise<boolean> {
  if (!ctx.session.copyEditing) return false;
  const { offerId, slot } = activeSlot(ctx.session);
  if (!offerId || !slot) return false;
  try {
    const res = await api.editCopy(offerId, ctx.chatId, slot.kind, text, slot.systemIndex ?? 0);
    apply(ctx, { ...ctx.session, copyEditing: false });
    await ctx.deps.send(ctx.chatId, copyTextMessage(copySlotLabel(slot), res.text), COPY_EDIT_KEYBOARD);
  } catch (e) {
    await ctx.deps.send(ctx.chatId, `Не удалось вычитать текст: ${(e as Error).message}`, COPY_EDIT_KEYBOARD);
  }
  return true;
}

/** Все секции пройдены — отдаём файл. */
async function finishCopy(ctx: CallbackCtx, offerId: number): Promise<void> {
  apply(ctx, { ...ctx.session, copySlot: undefined, copyEditing: false });
  await ctx.deps.send(ctx.chatId, 'Тексты согласованы — собираю файл.');
  await sendDocx(ctx, offerId);
}
