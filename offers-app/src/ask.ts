/**
 * «Продолжить в боте» из приложения (2026-11-15).
 *
 * Две кнопки в карточке КП отдают работу Telegram:
 *   - в зоне брифа — «✏️ Продолжить бриф в боте»: бот задаёт вопросы по одному,
 *     и разговор идёт там, где человеку удобно отвечать голосом;
 *   - в зоне позиций — «📎 Прислать файл для разбора»: файл .xlsx разбирается
 *     ботом (двухшагово, «что я понял» → подтверждение), а приложение такого
 *     пути не имеет вовсе.
 *
 * Почему через бота, а не напрямую: приложение КП открывается inline-кнопкой,
 * а `Telegram.WebApp.sendData` работает только у кнопки клавиатуры (дефект
 * 2026-09.28). Поэтому сервис просит бота написать в чат — `chat_id` берётся
 * из подписи `initData`, а не из тела.
 */
import { ApiError, apiRequest } from './api.ts';
import { inTelegram, initData } from './tg.ts';

export type AskAction = 'brief' | 'positions';

export interface AskOutcome {
  /** Пустая строка — бот написал в чат. */
  problem: string;
  /** Что именно написал бот: показываем человеку. */
  sent: string;
}

/** Есть ли чем просить: подпись Telegram и Telegram вокруг. */
export function askPreflight(): string {
  if (!inTelegram()) {
    return 'Продолжить в боте можно только из Telegram: откройте приложение из бота.';
  }
  if (initData() === '') {
    return 'Это окно открыто без ключа Telegram. Откройте КП из бота и нажмите кнопку ещё раз.';
  }
  return '';
}

/**
 * Попросить бота открыть диалог. Приложение НЕ закрывается: человек остаётся
 * в карточке и видит строку «что написали в чат» (класс 0.9.27 — молчаливое
 * закрытие без результата).
 */
export async function askBot(action: AskAction, id: number): Promise<AskOutcome> {
  const problem = askPreflight();
  if (problem !== '') return { problem, sent: '' };
  if (!Number.isInteger(id) || id <= 0) {
    return { problem: 'Документ не сохранён: сначала сохраните карточку.', sent: '' };
  }
  try {
    const answer = await apiRequest<{ ok?: boolean; sent?: string }>('/app/ask', {
      method: 'POST',
      body: { initData: initData(), action, kind: 'offer', id },
    });
    return { problem: '', sent: answer?.sent ?? 'Бот написал в чат.' };
  } catch (e: unknown) {
    return { problem: askFailure(e), sent: '' };
  }
}

function askFailure(e: unknown): string {
  if (e instanceof ApiError) {
    switch (e.code) {
      case 'not_linked':
        return 'Сначала войдите в бота: /start и код из письма — потом откройте приложение заново.';
      default:
        return e.message;
    }
  }
  return e instanceof Error ? `Не удалось попросить бота: ${e.message}` : 'Не удалось попросить бота: сервер не ответил.';
}

/** Подпись кнопки: человек должен понимать, что сейчас откроется чат. */
export function askButtonText(action: AskAction): string {
  return action === 'brief' ? '✏️ Продолжить бриф в боте' : '📎 Прислать файл для разбора';
}