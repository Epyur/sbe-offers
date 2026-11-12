/**
 * Помощник (агент) в боте КП — шаг P6 плана 2026-11-14.
 *
 * Агент — СБОРЩИК: он делает то же, что менеджер кнопками, но понимает
 * свободную фразу. Решение и инструменты живут на сервере (offers-service), бот
 * только отдаёт текст, показывает «⌛» и пересылает готовые файлы в чат.
 *
 * Здесь — чистые тексты и клавиатуры (проверяются без сети: agent.test.ts) и один
 * обработчик, которому нужен только `ctx` и `api`.
 *
 * Что ЭТО меняет в боте: ничего из прежних потоков не убрано (S5 — отдельное
 * решение пользователя). Помощник ВКЛЮЧАЕТСЯ кнопкой «🤖 Помощник» и выключается
 * «В меню»: пока он включён, свободный текст идёт агенту, а не в черновик
 * позиции. Так «Мембрана» не станет строкой с ценой 0, и старый сценарий
 * остаётся на месте.
 */
import type { BotDeps } from '../routes.ts';
import type { ChatSession } from '../session.ts';
import { sendDocument } from '../telegram.ts';
import * as api from '../api.ts';

/** Что открыто у человека в приложении: помощник включён и о каком документе речь. */
export interface AgentSession extends ChatSession {
  /** Помощник включён (свободный текст идёт агенту). */
  agent?: boolean;
}

/**
 * Приглашение в помощника — короткое и по делу. Не инструкция: у человека уже
 * есть меню, помощник не должен его переучивать.
 */
export const AGENT_INTRO =
  '🤖 Помощник на связи.\n\n' +
  'Напишите, что нужно сделать, своими словами — например:\n' +
  '«предложи систему ТН-КРОВЛЯ и подбери материалы»,\n' +
  '«сравни нашу мембрану с конкурентами»,\n' +
  '«найди протокол испытаний на кровельную мембрану».\n\n' +
  'Помощник сам найдёт продукт в Базе знаний, прочитает карточку и соберёт файл. ' +
  'Если название не совпадёт — он честно скажет об этом и предложит ближайшее.';

/** Клавиатура помощника: выход в меню — всегда под рукой. */
export const AGENT_KEYBOARD = {
  inline_keyboard: [[{ text: '🏠 В меню', callback_data: 'agent:off' }]],
};

/** Сообщение «помощник думает» — его заменяют результатом (edit). */
export const AGENT_THINKING = '⌛ Помощник работает…';

/**
 * Включить помощника. Возвращает новую сессию: вид документа не трогаем, черновик
 * и боевой листок остаются — человек вернётся к ним.
 */
export function enterAgent(session: ChatSession): AgentSession {
  return { ...session, agent: true };
}

/** Выключить помощника (возврат к обычному меню). */
export function exitAgent(session: ChatSession): ChatSession {
  const next: ChatSession = { ...session };
  delete (next as AgentSession).agent;
  return next;
}

/** Помощник включён у этой сессии. */
export function agentOn(session: ChatSession): boolean {
  return (session as AgentSession).agent === true;
}

/**
 * Идентификаторы документа, о котором идёт речь: открытое КП или боевой листок
 * из разговора. Агент правит уже начатую строку — новую он не создаёт (решение
 * §1 дизайна: строку заводит «Создать новый документ»).
 */
export function agentDocument(session: ChatSession): { offerId?: number; sheetId?: number } {
  if (session.combat?.sheetId) return { sheetId: session.combat.sheetId };
  if (session.draft?.offerId) return { offerId: session.draft.offerId };
  return {};
}

/** Итог хода: что писать менеджеру. `files` отправляются отдельными сообщениями. */
export interface AgentOutcome {
  text: string;
  files: Array<{ name: string; bytes: Buffer }>;
}

/** Подпись к готовому файлу в чате. */
export function agentFileCaption(name: string): string {
  return /боев/i.test(name) ? 'Боевой листок' : 'Коммерческое предложение';
}

/**
 * Разобрать ответ сервера в готовый итог. base64 → Buffer здесь, чтобы
 * обработчик не знал про кодировку, а тест проверял чистую функцию.
 */
export function agentOutcome(reply: api.AgentReply): AgentOutcome {
  const files = (reply.files ?? []).map((file) => ({
    name: file.name,
    bytes: Buffer.from(file.base64, 'base64'),
  }));
  const text = (reply.answer ?? '').trim() || 'Готово.';
  return { text, files: files.filter((file) => file.bytes.length > 0) };
}

/**
 * Прогнать ход помощника. Обработчик получает уже собранный контекст: `deps`
 * (send/edit), id чата, текст человека и открытый документ. Возвращает true —
 * текст был ходом помощника; false — помощник не включён, пусть идёт обычный
 * маршрут.
 *
 * «⌛» показывается сразу и заменяется ответом (тот же приём, что у долгих кнопок
 * боевого листка, 2026-11-13); нет `edit` — ответ уходит новым сообщением.
 */
export async function handleAgentText(
  ctx: { deps: BotDeps; chatId: string; session: ChatSession },
  text: string,
): Promise<boolean> {
  if (!agentOn(ctx.session)) return false;
  const trimmed = text.trim();
  if (!trimmed) return true;
  const statusId = ctx.deps.edit
    ? await ctx.deps.send(ctx.chatId, AGENT_THINKING).catch(() => undefined)
    : undefined;

  let outcome: AgentOutcome;
  try {
    const reply = await api.agentMessage(ctx.chatId, trimmed, agentDocument(ctx.session));
    outcome = agentOutcome(reply);
  } catch (e) {
    outcome = { text: `Помощник не смог ответить: ${(e as Error).message}`, files: [] };
  }

  // Сначала ответ: человек ждёт текста важнее, чем файла.
  if (statusId && ctx.deps.edit) {
    await ctx.deps.edit(ctx.chatId, statusId, outcome.text, AGENT_KEYBOARD).catch(() => undefined);
  } else {
    await ctx.deps.send(ctx.chatId, outcome.text, AGENT_KEYBOARD);
  }
  for (const file of outcome.files) {
    await sendDocument(ctx.chatId, file.name, file.bytes, agentFileCaption(file.name));
  }
  return true;
}