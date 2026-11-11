/**
 * Долгие действия видны человеку (2026-11-12).
 *
 * Живая жалоба: «после нажатия на кнопку „Поиск аналогов“ в боте вообще не
 * понятно, что происходит, хотя ответ приходит через более чем минуту».
 * Пока подбор шёл, бот молчал: ни индикатора, ни сообщения — человек нажимал
 * кнопку повторно и не понимал, работает ли он вовсе.
 *
 * Приём: сразу после нажатия пишем сообщение «⌛ …», а когда работа закончена —
 * заменяем ЕГО результатом (`editMessageText`). В чате не копится десяток
 * служебных строк, а ожидание видно с первой секунды.
 *
 * Ошибка тоже заменяет «⌛»: иначе «идёт поиск…» осталось бы в чате навсегда и
 * выглядело бы зависанием.
 */
import { errorMessage } from './errors.ts';

/** Что нужно от транспорта: отправить сообщение и заменить его текст. */
export interface ProgressChannel {
  send(chatId: string, text: string, keyboard?: Record<string, unknown>): Promise<number | undefined>;
  edit(chatId: string, messageId: number, text: string, keyboard?: Record<string, unknown>): Promise<void>;
}

/**
 * Выполнить долгую работу с видимым ожиданием.
 *
 * `work` возвращает готовый текст результата. Сообщение «⌛ …» отправляется
 * ДО работы, поэтому не зависит от того, сколько она идёт. Если отправить его
 * не удалось (сеть, лимит Telegram), работа всё равно выполняется: ожидание —
 * удобство, а не условие.
 */
export async function runWithProgress<T>(
  channel: ProgressChannel,
  chatId: string,
  pendingText: string,
  work: () => Promise<T>,
  render: (result: T) => string = (result) => String(result),
): Promise<T> {
  let messageId: number | undefined;
  try {
    messageId = await channel.send(chatId, pendingText);
  } catch {
    // «⌛» не ушёл — не повод не начинать работу.
  }
  try {
    const result = await work();
    const text = render(result);
    await replacePending(channel, chatId, messageId, text);
    return result;
  } catch (e: unknown) {
    await replacePending(channel, chatId, messageId, `Не удалось: ${errorMessage(e)}`);
    throw e;
  }
}

/**
 * Заменить «⌛» результатом. Не вышло — дописываем новым сообщением: человек
 * обязан увидеть, чем кончилось дело, даже если правка не прошла.
 */
async function replacePending(
  channel: ProgressChannel,
  chatId: string,
  messageId: number | undefined,
  text: string,
): Promise<void> {
  if (messageId !== undefined) {
    try {
      await channel.edit(chatId, messageId, text);
      return;
    } catch {
      // Сообщение могли удалить или правка не прошла — скажем отдельно.
    }
  }
  try {
    await channel.send(chatId, text);
  } catch {
    // Последний шанс потерян: результат есть, но показать его нечем.
  }
}