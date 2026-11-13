/**
 * «Отправить в чат» (2026-11-13, шаг S7; переделано 2026-11-14).
 *
 * Приложение не может приложить файл к сообщению само — у него нет Bot API,
 * а `tg.sendData` работает **только у кнопки клавиатуры**. Приложение
 * открывается inline-кнопкой, поэтому `sendData` там молча ничего не делает
 * и просто закрывает окно (живой дефект: «нажал — закрылось, в чате пусто»).
 *
 * Теперь работает путь без ограничений кнопки: приложение отдаёт сервису
 * подпись Telegram (`initData`), сервис собирает DOCX и просит бота прислать файл
 * в чат. Адресат берётся из самой подписи, так что попросить чужой чат нельзя.
 */
import { ApiError, apiRequest } from './api.ts';
import { close, inTelegram, initData } from './tg.ts';

export type DocumentKind = 'offer' | 'combat';

export interface SendChatRequest {
  action: 'docx';
  kind: DocumentKind;
  id: number;
}

/** Нагрузка для бота: проверяемая по форме, без лишних полей. */
export function payloadOf(kind: DocumentKind, id: number): SendChatRequest | null {
  if (!Number.isInteger(id) || id <= 0) return null;
  return { action: 'docx', kind, id };
}

export interface SendChatOutcome {
  /** Пустая строка — файл ушёл в чат. */
  problem: string;
  /** Отправка принята сервисом: приложение можно закрывать. */
  sent: boolean;
}

/** Текст, который видит человек, когда отправить нельзя ещё до запроса. */
export function preflightProblem(kind: DocumentKind, id: number): string {
  if (!inTelegram()) {
    return 'Отправить в чат можно только из Telegram: откройте приложение из бота.';
  }
  if (payloadOf(kind, id) === null) {
    return 'Документ не сохранён: сначала сохраните карточку.';
  }
  return '';
}

/**
 * Попросить сервис отправить документ в чат. Успех закрывает приложение
 * (Telegram сам подставит файл в диалог), отказ — показывается на экране.
 */
export async function sendDocumentToChat(kind: DocumentKind, id: number): Promise<SendChatOutcome> {
  const problem = preflightProblem(kind, id);
  if (problem !== '') return { problem, sent: false };
  const payload = payloadOf(kind, id);
  if (payload === null) return { problem, sent: false };

  const raw = initData();
  if (raw === '') {
    return {
      problem:
        'Это окно открыто без ключа Telegram. Откройте документ в приложении и нажмите кнопку ещё раз.',
      sent: false,
    };
  }

  let answer: { ok?: boolean; sent?: boolean; detail?: string };
  try {
    answer = await apiRequest<{ ok?: boolean; sent?: boolean; detail?: string }>('/app/answer', {
      method: 'POST',
      body: { initData: raw, ...payload },
    });
  } catch (e: unknown) {
    // Отказ НЕ закрываем: человек должен увидеть причину в самом
    // приложении: после close() её уже некому показать.
    return { problem: failureText(e), sent: false };
  }
  if (answer?.sent === false) {
    return {
      problem: answer.detail === undefined || answer.detail === '' ? 'Файл не ушёл. Попробуйте ещё раз.' : answer.detail,
      sent: false,
    };
  }
  // Сервис подтвердил отправку — закрываем окно.
  close();
  return { problem: '', sent: true };
}

/** Подпись кнопки — по ней человек понимает, что произойдёт. */
export function sendButtonText(kind: DocumentKind): string {
  return kind === 'offer' ? '📤 Отправить КП в чат' : '📤 Отправить листок в чат';
}

/** Отдельная «×» для случая, когда отправка не удалась: закрыть без файла. */
export function closeApp(): void {
  close();
}

function failureText(e: unknown): string {
  if (e instanceof ApiError) {
    switch (e.code) {
      case 'not_linked':
        return 'Сначала войдите в бота: /start и код из письма — потом откройте приложение заново.';
      case 'initData':
        return 'Telegram не подтвердил, что это вы. Откройте документ из бота заново.';
      default:
        return e.message;
    }
  }
  return e instanceof Error ? `Не удалось отправить: ${e.message}` : 'Не удалось отправить: сервер не ответил.';
}