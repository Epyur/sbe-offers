/**
 * Продолжение работы с уже сохранённым КП из «Мои КП» (2026-11-01).
 *
 * Живой разговор: менеджер нажал «Готово», не внеся позиций, и потом не смог
 * вернуться к КП в боте — правка была доступна только из плагина. Здесь
 * собирается состояние, которое позволяет продолжить: состав берётся с сервера
 * (он один знает истину), бриф подтягивается по требованию, а согласование
 * текстов идёт тем же путём, что в обычном разговоре.
 *
 * Модуль чистый: ни сети, ни Telegram. Сеть — в api.ts, вызовы — в callbacks.ts.
 */
import type { ChatSession, OfferDraft, OfferItem } from '../session.ts';
import { newDraft } from './build.ts';

/** Предложение с сервера: только то, что нужно для продолжения. */
export interface ResumedOffer {
  id: number;
  number: string;
  client?: { name?: string; contact?: string };
  items?: Array<{ catalogId?: number; ekn?: string; name: string; unit: string; qty: number; price: number }>;
  formValues?: Record<string, string>;
}

/**
 * Черновик из сохранённого предложения.
 *
 * Этап — сразу позиции: контекст по этому КП уже собрали (или сознательно
 * пропустили), и заново его спрашивать — значит повторять разговор, который
 * человек уже прошёл. Кнопка «✏️ Дополнить контекст» на панели правки запускает
 * бриф заново, если он действительно нужен.
 */
export function draftFromOffer(offer: ResumedOffer): OfferDraft {
  const items: OfferItem[] = (offer.items ?? []).map((item) => ({
    catalogId: item.catalogId,
    name: item.name,
    unit: item.unit,
    qty: item.qty,
    price: item.price,
  }));
  return {
    ...newDraft(),
    offerId: offer.id,
    number: offer.number,
    clientName: offer.client?.name ?? '',
    clientContact: offer.client?.contact ?? '',
    formValues: { ...(offer.formValues ?? {}) },
    items,
    step: 'catalog',
    stage: 'positions',
  };
}

/** Сессия с продолженным черновиком. Прежний разбор файла забывается: он
 *  относился к другому разговору. */
export function resumeSession(session: ChatSession, offer: ResumedOffer): ChatSession {
  const next: ChatSession = {
    ...session,
    draft: draftFromOffer(offer),
    briefTopic: undefined,
    briefDone: true,
    briefSkipped: false,
    copySlot: undefined,
    copyEditing: false,
  };
  delete next.attach;
  return next;
}

/** Что бот говорит, когда человек открыл сохранённое КП. */
export function resumeMessage(offer: ResumedOffer): string {
  const items = (offer.items ?? []).length;
  const client = offer.client?.name?.trim();
  const lines = [`Предложение № ${offer.number}${client ? ` — ${client}` : ''}.`];
  if (items === 0) {
    lines.push(
      'Позиций пока нет — можно дописать их сейчас или собрать документ только с описательной частью.',
    );
  } else {
    lines.push(`Позиций: ${items}.`);
  }
  lines.push('Что сделаем?');
  return lines.join('\n');
}

/**
 * Кнопки панели правки сохранённого КП.
 *
 * Отдельная клавиатура от «черновик после брифа»: там «Готово» ведёт к
 * согласованию текстов, а здесь человек уже мог пройти его — ему нужны
 * документ, ссылка и дополнение контекста.
 */
export const EDIT_KEYBOARD = {
  inline_keyboard: [
    [{ text: '➕ Добавить из каталога', callback_data: 'cat' }],
    [{ text: '✏️ Добавить свою позицию', callback_data: 'manual' }],
    [{ text: '📚 Дополнить контекст', callback_data: 'edit:brief' }],
    [{ text: '📄 Документ', callback_data: 'edit:doc' }],
    [{ text: '🧾 Готово', callback_data: 'done' }],
  ],
};

/** Продолжать разговор или нет: номер КП узнаётся по его форме. */
export function canResume(numberText: string): boolean {
  return /^\d{2,4}\/\d{4}$/.test(numberText.trim());
}
