/**
 * Приём просьбы «продолжить в боте» из приложения (2026-11-15).
 *
 * Сервис (`offers-service/appask.go`) проверяет подпись Telegram, берёт
 * `chat_id` из неё и присылает сюда секретом `X-Service-Secret`. Бот не
 * решает про права заново: адресат уже проверен, а `getOffer` ниже отдаёт
 * только КП этого чата — сервис сверяет владельца и отвечает отказом.
 *
 * Два действия — два существующих диалога, ни одного нового:
 *   - `brief` → `beginBrief`, тот же разговор, что «📚 Дополнить контекст»;
 *   - `positions` → `startPositions`, та же строка сборки; присланный файл
 *     разбирается обычным путём (`routeDocument`), как будто его отправили
 *     вручную.
 *
 * Почему через существующие функции: второй путь к тому же диалогу разошёлся
 * бы с первым — в боте уже жило два списка КП, пока их не свели в один
 * (R1, 2026-11-13).
 */
import type { BotDeps } from '../routes.ts';
import { getOffer } from '../api.ts';
import { apply, startPositions, type CallbackCtx } from '../callbacks.ts';
import { newDraft } from './build.ts';
import { beginBrief } from './writer.ts';
import { askIntroText, askKeyboard, parseAskRequest, type SheetAskRequest } from './ask.ts';

export interface AskResult {
  /** Что сервис отдаёт приложению: приложение показывает это человеку. */
  sent: string;
}

/**
 * Открыть запрошенный диалог у бота.
 *
 * Ошибку отдаём как есть — приложение покажет текст, а не «нажал и ничего».
 * Сессия поднимается штатно (`deps.session`): после перезапуска бота она
 * восстанавливается по привязке «чат → e-mail».
 */
export async function runAsk(deps: BotDeps, raw: unknown): Promise<AskResult> {
  const request = parseAskRequest(raw);
  if (request === null) {
    throw new Error('просьба непонятна: нужны action, chat_id и документ (offer_id или sheet_id)');
  }
  if (request.kind === 'sheet') {
    return runSheetAsk(deps, request);
  }
  const session = await deps.session(request.chatId);
  const offer = await getOffer(request.offerId, request.chatId);
  const ctx: CallbackCtx = {
    chatId: request.chatId,
    session,
    deps,
    // Нажатия не было: сохраняем через `deps`, а постраничный поиск каталога
    // здесь не используется (`startPositions`/`beginBrief` его не читают).
    save: (next) => deps.save(request.chatId, next),
    pages: new Map(),
  };
  apply(ctx, {
    ...ctx.session,
    draft: {
      ...(ctx.session.draft ?? newDraft()),
      offerId: offer.id,
      number: offer.number,
      clientName: offer.client?.name ?? '',
      clientContact: offer.client?.contact ?? '',
      formValues: { ...(offer.formValues ?? {}) },
      items: (offer.items ?? []).map((item) => ({
        catalogId: item.catalogId,
        name: item.name,
        unit: item.unit,
        qty: item.qty,
        price: item.price,
      })),
      step: 'catalog',
      stage: 'positions',
    },
    briefTopic: undefined,
    briefDone: true,
    briefSkipped: false,
    copySlot: undefined,
    copyEditing: false,
  });

  if (request.action === 'brief') {
    await deps.send(request.chatId, askIntroText(request));
    await beginBrief(ctx, request.offerId);
    return { sent: 'Открыли бриф в чате: отвечайте на вопросы в боте.' };
  }
  await startPositions(ctx);
  return { sent: 'Написали в чат: пришлите файл для разбора позиций.' };
}

/**
 * Просьба «прислать файл по конкуренту» для боевого листка (2026-11-17).
 *
 * Диалога не открываем: листок уже открыт в приложении, от человека нужен
 * только файл. Поэтому в сессии отмечается ОЖИДАНИЕ файла (`sheetFile`), а
 * разбирает его `routeDocument` — до ветки КП, иначе .xlsx ушёл бы в разбор
 * позиций.
 *
 * Владельца проверяет сервис (он знает автора листка) и повторно — ботовая
 * ручка разбора (`/bot/combat/{id}/competitor-file`). Здесь второй проверки
 * нет намеренно: `getSheet` для чужого листка не нужен, а вход уже под
 * секретом сервиса.
 */
async function runSheetAsk(deps: BotDeps, request: SheetAskRequest): Promise<AskResult> {
  const session = await deps.session(request.chatId);
  await deps.save(request.chatId, {
    ...session,
    sheetFile: { sheetId: request.sheetId, number: request.sheetNumber },
  });
  await deps.send(request.chatId, askIntroText(request), askKeyboard(request));
  return { sent: 'Написали в чат: пришлите файл по конкуренту.' };
}
