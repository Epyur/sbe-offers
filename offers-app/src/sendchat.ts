/**
 * «Отправить в чат» (2026-11-13, шаг S7).
 *
 * Приложение не может приложить файл к сообщению само — у него нет Bot API.
 * Оно отправляет боту УКАЗАТЕЛЬ (`tg.sendData`), а бот собирает документ
 * своими ручками и кладёт файл в чат. Поэтому здесь всего одна чистая
 * функция построения нагрузки и один вызов Telegram.
 *
 * В указателе нет ни адреса, ни байтов — только вид документа и номер:
 * чужие данные в эту строку не попадают, а `chat_id` бот берёт из самого
 * обновления.
 */
import { close, inTelegram, sendToChat } from './tg.ts';

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

/**
 * Отправить документ в чат и закрыть приложение. Возвращает текст, если
 * отправить не удалось (вне Telegram, несохранённый документ или отказ
 * платформы) — экран покажет его человеку.
 */
export function sendDocumentToChat(kind: DocumentKind, id: number): string {
  if (!inTelegram()) {
    return 'Отправить в чат можно только из Telegram: откройте приложение из бота.';
  }
  const payload = payloadOf(kind, id);
  if (payload === null) {
    return 'Документ не сохранён: сначала сохраните карточку.';
  }
  return sendToChat({ ...payload });
}

/** Подпись кнопки — по ней человек понимает, что произойдёт. */
export function sendButtonText(kind: DocumentKind): string {
  return kind === 'offer' ? '📤 Отправить КП в чат' : '📤 Отправить листок в чат';
}

/** Отдельная «×» для случая, когда отправка не удалась: закрыть без файла. */
export function closeApp(): void {
  close();
}