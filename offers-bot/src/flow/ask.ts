/**
 * Просьба из приложения «продолжить в боте» (2026-11-15; листок — 2026-11-17).
 *
 * Приложение не может само начать разговор в чате: `sendData` у него не
 * работает (оно открывается inline-кнопкой — дефект 2026-09.28), а своего
 * пути в Telegram нет вовсе. Поэтому кнопки «Продолжить бриф в боте»,
 * «Прислать файл для позиций» и «Прислать файл по конкуренту» просят бота
 * написать в чат — сервис вызывает `/internal/ask`, бот читает просьбу и
 * открывает нужный диалог у себя.
 *
 * Два документа — КП и боевой листок — различаются полем `kind`:
 *   - `kind: "offer"` (или поля нет — старый сервис): `brief` / `positions`;
 *   - `kind: "sheet"`: `competitor-file` — файл по конкуренту листка.
 *
 * Почему `kind`, а не новое действие в ветке КП: у листка другой владелец
 * (автор или admin) и другой диалог, и общий переключатель «что имел в виду
 * человек» — ровно то, из-за чего в боте уже путались списки (R1, 2026-11-13).
 *
 * Здесь только тексты и чистая проверка: сети нет, поэтому модуль
 * проверяется тестом (`ask.test.ts`), а отправитель — `verify-server.ts`.
 */
import { EDIT_KEYBOARD } from './resume.ts';
import { BRIEF_FILE_KEYBOARD } from './brief.ts';
import { MAIN_KEYBOARD } from './build.ts';

/** Действия, которые понимает бот. Значения совпадают с сервисом. */
export type AskAction = 'brief' | 'positions' | 'competitor-file';

/** Документ, о котором просит приложение: КП или боевой листок. */
export type AskKind = 'offer' | 'sheet';

/** Просьба по КП: бриф или разбор позиций. */
export interface OfferAskRequest {
  chatId: string;
  kind: 'offer';
  action: 'brief' | 'positions';
  offerId: number;
  offerNumber: string;
}

/** Просьба по боевому листку: файл по конкуренту. */
export interface SheetAskRequest {
  chatId: string;
  kind: 'sheet';
  action: 'competitor-file';
  sheetId: number;
  sheetNumber: string;
}

export type AskRequest = OfferAskRequest | SheetAskRequest;

/** Число из тела: строка или число, но обязательно целое и больше нуля. */
function idFrom(value: unknown): number {
  const id = typeof value === 'number' ? value : Number(value);
  return Number.isInteger(id) && id > 0 ? id : 0;
}

/** Разобранная просьба. `null` — просьба непонятна, и это надо сказать. */
export function parseAskRequest(raw: unknown): AskRequest | null {
  if (typeof raw !== 'object' || raw === null) return null;
  const body = raw as Record<string, unknown>;
  const chatId = typeof body.chat_id === 'string' ? body.chat_id.trim() : '';
  if (!/^-?[0-9]+$/.test(chatId)) return null;
  const action = body.action;
  const rawKind = body.kind;
  // Старый сервис поля `kind` не слал вовсе — это просьба по КП.
  if (rawKind !== undefined && rawKind !== 'offer' && rawKind !== 'sheet') return null;
  const kind = rawKind === 'sheet' ? 'sheet' : 'offer';
  if (kind === 'sheet') {
    if (action !== 'competitor-file') return null;
    const sheetId = idFrom(body.sheet_id);
    if (sheetId === 0) return null;
    const sheetNumber = typeof body.sheet_number === 'string' ? body.sheet_number.trim() : '';
    return { chatId, kind, action, sheetId, sheetNumber };
  }
  if (action !== 'brief' && action !== 'positions') return null;
  const offerId = idFrom(body.offer_id);
  if (offerId === 0) return null;
  const offerNumber = typeof body.offer_number === 'string' ? body.offer_number.trim() : '';
  return { chatId, kind: 'offer', action, offerId, offerNumber };
}

/**
 * Текст, который бот пишет в чат по просьбе приложения.
 *
 * Обращение глазами менеджера: он только что нажал кнопку в приложении и
 * вернулся в чат, поэтому строка говорит, ОТКУДА она пришла и что делать
 * дальше. Сам диалог дальше ведёт существующий обработчик (`beginBrief`).
 */
export function askIntroText(request: AskRequest): string {
  if (request.kind === 'sheet') {
    const number = request.sheetNumber === '' ? '' : ` № ${request.sheetNumber}`;
    return (
      `Конкурент из файла для боевого листка${number} — вы попросили об этом в приложении.\n\n` +
      'Пришлите файл вложением: PDF, DOCX, XLSX или текст. Я прочитаю его и добавлю конкурента ' +
      'в листок. Если в файле нет текстового слоя (скан или картинка), добавление не пройдёт — ' +
      'пришлите файл с текстом.'
    );
  }
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
  if (request.kind === 'sheet') return MAIN_KEYBOARD;
  if (request.action === 'brief') return BRIEF_FILE_KEYBOARD;
  return EDIT_KEYBOARD;
}