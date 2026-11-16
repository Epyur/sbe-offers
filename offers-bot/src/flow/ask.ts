/**
 * Просьба из приложения «продолжить в боте» (2026-11-15).
 *
 * Приложение КП не может само начать разговор в чате: `sendData` у него не
 * работает (оно открывается inline-кнопкой — дефект 2026-09.28), а своего
 * пути в Telegram нет вовсе. Поэтому кнопки «Продолжить бриф в боте» и
 * «Прислать файл для позиций» просят бота написать в чат — сервис вызывает
 * `/internal/ask`, бот читает просьбу и открывает нужный диалог у себя.
 *
 * Здесь только тексты и чистая проверка: сети нет, поэтому модуль
 * проверяется тестом (`ask.test.ts`), а отправитель — `main.ts`.
 */
import { EDIT_KEYBOARD } from './resume.ts';
import { BRIEF_FILE_KEYBOARD } from './brief.ts';

/** Действия, которые понимает бот. Значения совпадают с сервисом. */
export type AskAction = 'brief' | 'positions';

/** Тело просьбы: что открыть и по какому КП. */
export interface AskRequest {
  chatId: string;
  action: AskAction;
  offerId: number;
  offerNumber: string;
}

/** Разобранная просьба. `null` — просьба непонятна, и это надо сказать. */
export function parseAskRequest(raw: unknown): AskRequest | null {
  if (typeof raw !== 'object' || raw === null) return null;
  const body = raw as Record<string, unknown>;
  const chatId = typeof body.chat_id === 'string' ? body.chat_id.trim() : '';
  if (!/^-?[0-9]+$/.test(chatId)) return null;
  const action = body.action;
  if (action !== 'brief' && action !== 'positions') return null;
  const offerId = typeof body.offer_id === 'number' ? body.offer_id : Number(body.offer_id);
  if (!Number.isInteger(offerId) || offerId <= 0) return null;
  const offerNumber = typeof body.offer_number === 'string' ? body.offer_number.trim() : '';
  return { chatId, action, offerId, offerNumber };
}

/**
 * Текст, который бот пишет в чат по просьбе приложения.
 *
 * Обращение глазами менеджера: он только что нажал кнопку в приложении и
 * вернулся в чат, поэтому строка говорит, ОТКУДА она пришла и что делать
 * дальше. Сам диалог дальше ведёт существующий обработчик (`beginBrief`).
 */
export function askIntroText(request: AskRequest): string {
  const number = request.offerNumber === '' ? '' : ` № ${request.offerNumber}`;
  if (request.action === 'brief') {
    return (
      `Продолжаем бриф по КП${number} — вы попросили об этом в приложении.\n\n` +
      'Отвечайте на вопросы прямо здесь, в чате. Приложение покажет эти ответы в карточке КП.'
    );
  }
  return (
    `Разбор позиций по КП${number} — вы попросили об этом в приложении.\n\n` +
    'Пришлите файл вложением: .xlsx с таблицей (наименование, количество, цена). ' +
    'Я покажу, что понял, и спрошу подтверждение — само ничего не запишется.'
  );
}

/** Кнопки под текстом просьбы: у брифа — «прислать файл», у позиций — список КП. */
export function askKeyboard(request: AskRequest): Record<string, unknown> {
  if (request.action === 'brief') return BRIEF_FILE_KEYBOARD;
  return EDIT_KEYBOARD;
}