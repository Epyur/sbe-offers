/**
 * Согласование текста секций в чате (Task 9 плана 2026-10-29).
 *
 * Модуль чистый: тексты пишет сервис, здесь — порядок секций, кнопки и то,
 * что запоминается в сессии. Принятый текст НЕ хранится в сессии: он уже в
 * предложении (`form_values["copy.*"]`). Второй копии нет намеренно — после
 * перезапуска бот показал бы старое.
 */
import type { ChatSession } from '../session.ts';
import type { OfferItemPayload } from '../api.ts';

/** Одна секция, которую предстоит согласовать. */
export interface SectionSlot {
  kind: 'intro' | 'system' | 'advantages' | 'conditions';
  key: string;
  label: string;
  /** Номер системы для `system`, с 1. */
  systemIndex?: number;
}

/** Кнопки показа текста. */
export const COPY_KEYBOARD = {
  inline_keyboard: [
    [{ text: '✅ Принять', callback_data: 'copy:accept' }],
    [{ text: '✏️ Поправить', callback_data: 'copy:edit' }],
    [{ text: '⏭ Пропустить', callback_data: 'copy:skip' }],
  ],
};

/** Клавиатура после правки: принять или поправить снова. */
export const COPY_EDIT_KEYBOARD = {
  inline_keyboard: [
    [{ text: '✅ Принять', callback_data: 'copy:accept' }],
    [{ text: '✏️ Поправить ещё', callback_data: 'copy:edit' }],
    [{ text: '⏭ Пропустить', callback_data: 'copy:skip' }],
  ],
};

/**
 * Секции в порядке согласования: вводный текст, описания систем,
 * преимущества, условия. Тот же порядок, что в сервисе (`SectionSlots`) —
 * он задаёт ключи `form_values`, и разойтись они не должны.
 */
export function numberedSlots(systems: number): SectionSlot[] {
  const slots: SectionSlot[] = [{ kind: 'intro', key: 'copy.intro', label: 'Вводный текст' }];
  for (let i = 1; i <= systems; i += 1) {
    slots.push({ kind: 'system', key: `copy.system.${i}`, label: `Описание системы ${i}`, systemIndex: i });
  }
  slots.push({ kind: 'advantages', key: 'copy.advantages', label: 'Преимущества' });
  slots.push({ kind: 'conditions', key: 'copy.conditions', label: 'Условия' });
  return slots;
}

/**
 * Сколько систем в предложении. Считаются позиции, пришедшие из каталога:
 * одна система — один текст описания. Свободные строки системой не считаются,
 * у них нет карточки, из которой берётся описание.
 */
export function systemsIn(items: OfferItemPayload[]): number {
  const seen = new Set<number>();
  for (const item of items) {
    if (item.catalogId) seen.add(item.catalogId);
  }
  return seen.size;
}

/** Текущая секция согласования (или null, если согласовывать нечего). */
export function currentSlot(session: ChatSession, slots: SectionSlot[]): SectionSlot | null {
  if (slots.length === 0) return null;
  const index = session.copySlot ?? 0;
  return slots[index] ?? null;
}

/** Поставить согласование на конкретную секцию. */
export function copyState(session: ChatSession, slot: number): ChatSession {
  return { ...session, copySlot: slot, copyEditing: false };
}

/**
 * Шаг вперёд. Возвращает сессию; завершённость проверяется отдельно
 * (`copyDone`), потому что «последняя секция» — свойство списка, не сессии.
 */
export function advanceCopy(session: ChatSession, slots: SectionSlot[]): ChatSession {
  const next = (session.copySlot ?? 0) + 1;
  return { ...session, copySlot: next, copyEditing: false };
}

/** Все секции пройдены. */
export function copyDone(session: ChatSession, slots: SectionSlot[]): boolean {
  if (slots.length === 0) return false;
  return (session.copySlot ?? 0) >= slots.length;
}

/** Подпись секции: пустая строка подписи в чате выглядела бы поломкой. */
export function copySlotLabel(slot: SectionSlot | undefined): string {
  return slot?.label || 'Текст предложения';
}

/**
 * Карточка текста в чате. Пустой текст — не пустая карточка: честно говорим,
 * что модель не справилась, и предлагаем следующий шаг.
 */
export function copyTextMessage(label: string, text: string): string {
  const body = text.trim();
  if (!body) {
    return `${label}\n\nНе удалось написать текст — модель недоступна или не ответила. ` +
      'Можно поправить своим текстом, пропустить секцию или принять пустую.';
  }
  return `${label}\n\n${body}`;
}

/** Сообщение «правьте, пришлите свой вариант». */
export function copyEditPrompt(label: string): string {
  return `${label}\n\nПришлите свой вариант текста одним сообщением — я вычитаю и покажу результат.`;
}