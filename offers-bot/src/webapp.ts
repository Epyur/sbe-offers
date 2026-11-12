/**
 * «Отправить в чат» из Mini App (2026-11-13, шаг S7).
 *
 * Приложение не может само прислать файл: у него нет доступа к Bot API.
 * Поэтому оно отправляет в бот МАЛЕНЬКИЙ УКАЗАТЕЛЬ (`tg.sendData`), а бот по
 * нему собирает документ существующими ручками (`/bot/offers/{id}/docx`,
 * `/bot/combat/{id}/export`) и кладёт файл в чат.
 *
 * Правила, которые здесь важны:
 *   - в чат уходит ПОЛЬЗОВАТЕЛЬСКИЕ данные: `sendData` приходит только из
 *     подписанной сессии Telegram, но содержимое всё равно проверяется — это
 *     чужой ввод, и `any` тут не нужен (`unknown` + разбор).
 *   - `chat_id` берётся из обновления, НЕ из полезной нагрузки: указатель не
 *     может отправить документ в чужой чат.
 *   - документ достаётся тем же путём, что у кнопки «⬇ DOCX», и только для
 *     своего документа: `/bot/*` проверяет привязку чата к адресу.
 */
import * as api from './api.ts';

/** Что приложение вправе попросить: `{ action:'docx', kind:'offer'|'combat', id }`. */
export interface WebAppRequest {
  action: 'docx';
  kind: 'offer' | 'combat';
  id: number;
}

/** Разбор полезной нагрузки: мусор возвращает null, а не исключение. */
export function parseWebAppData(raw: string): WebAppRequest | null {
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return null;
  }
  if (typeof parsed !== 'object' || parsed === null) return null;
  const obj = parsed as Record<string, unknown>;
  if (obj.action !== 'docx') return null;
  if (obj.kind !== 'offer' && obj.kind !== 'combat') return null;
  const id = typeof obj.id === 'number' ? obj.id : Number(obj.id);
  if (!Number.isInteger(id) || id <= 0) return null;
  return { action: 'docx', kind: obj.kind, id };
}

/** Текст для человека, если попросили то, чего бот не знает. */
export function unknownRequestText(): string {
  return 'Приложение прислало непонятную команду. Откройте документ в приложении и нажмите «Отправить в чат» ещё раз.';
}

/** Подпись под отправленным файлом. */
export function captionFor(request: WebAppRequest): string {
  return request.kind === 'offer' ? 'Коммерческое предложение' : 'Боевой листок';
}

/**
 * Собрать документ и отправить в чат. Возвращает `true`, если файл ушёл:
 * вызывающий решает, что показать человеку. Ошибки отдаются текстом — бот
 * не должен падать из-за недоступного сервиса.
 */
export interface WebAppSendResult {
  ok: boolean;
  text: string;
}

export async function deliverDocx(
  chatId: string,
  request: WebAppRequest,
  sendFile: (chatId: string, fileName: string, bytes: Buffer, caption: string) => Promise<void>,
): Promise<WebAppSendResult> {
  try {
    const doc =
      request.kind === 'offer'
        ? await api.docx(request.id, chatId)
        : await api.combatDocx(request.id, chatId);
    const bytes = Buffer.from(doc.docx_base64, 'base64');
    await sendFile(chatId, doc.file_name, bytes, captionFor(request));
    return { ok: true, text: 'Готово: файл в чате.' };
  } catch (e: unknown) {
    return { ok: false, text: `Не удалось отправить файл: ${errorText(e)}` };
  }
}

function errorText(e: unknown): string {
  return e instanceof Error ? e.message : 'неизвестная ошибка';
}