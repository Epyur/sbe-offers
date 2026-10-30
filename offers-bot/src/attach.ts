/**
 * Файлы из чата: «что я понял» → подтверждение → бриф или состав
 * (шаг 6 пересмотренного плана 2026-10-29).
 *
 * Два правила, ради которых это отдельный модуль:
 *  1. Файл ничего не меняет сам. Сначала бот показывает, что понял, и
 *     спрашивает. Файл от заказчика — черновик, в нём бывают старые цены.
 *  2. Понятое живёт в веб-сервисе, а бот при подтверждении возвращает его
 *     назад. Второй копии в сессии нет: после перезапуска старая копия
 *     применилась бы к другому КП.
 *
 * Модуль чистый: сеть приходит через `deps`, поэтому поведение проверяется
 * стендом (attach.test.ts) без Telegram и без сервиса.
 */
import type { AttachResult } from './api.ts';
import type { AttachPending, ChatSession, SpecFromFile } from './session.ts';

export type AttachSession = ChatSession;
export type { AttachPending, AttachResult };

/** Сеть, которая нужна разбору файла. */
export interface AttachDeps {
  send(chatId: string, text: string, keyboard?: Record<string, unknown>): Promise<void>;
}

/** Подпись кнопок подтверждения: одна форма на оба вида файла. */
const CONFIRM_KEYBOARD = {
  inline_keyboard: [
    [
      { text: '✓ Применить', callback_data: 'att:yes' },
      { text: '✏️ Не надо', callback_data: 'att:no' },
    ],
  ],
};

/**
 * Предел показа строк таблицы в чате: остальное — в документе.
 */
const MAX_SPEC_LINES = 12;

/**
 * Что бот говорит после разбора файла.
 *
 * Сам текст «что я понял» собирает сервис (он один знает, что разобрал), а
 * здесь добавляется то, что зависит от разговора: клавиатура и строка про
 * пропущенное.
 */
export function attachMessage(result: AttachResult): { text: string; keyboard?: Record<string, unknown> } {
  if (!result.needsConfirm) {
    return { text: result.reply };
  }
  const lines = [result.reply];
  if (result.spec) {
    lines.push('', ...specPreview(result.spec));
  }
  if (result.brief && result.brief.notes?.trim()) {
    lines.push('', 'Прочее из файла:', truncate(result.brief.notes.trim(), 400));
  }
  return { text: lines.join('\n'), keyboard: CONFIRM_KEYBOARD };
}

/** Строки таблицы для показа в чате: название, количество, цена. */
export function specPreview(spec: SpecFromFile): string[] {
  const out: string[] = [];
  for (const row of spec.rows.slice(0, MAX_SPEC_LINES)) {
    const unit = row.unit ? ` ${row.unit}` : '';
    const mark = row.priceDiff ? ' (цена в файле другая)' : row.hasCatalog ? '' : ' (нет в каталоге)';
    out.push(`• ${row.name} — ${row.qty}${unit} × ${row.price}${mark}`);
  }
  if (spec.rows.length > MAX_SPEC_LINES) {
    out.push(`…и ещё ${spec.rows.length - MAX_SPEC_LINES}`);
  }
  if (spec.warnings && spec.warnings.length > 0) {
    out.push('', ...spec.warnings.slice(0, 3).map((w) => `⚠️ ${w}`));
  }
  return out;
}

function truncate(text: string, limit: number): string {
  return text.length <= limit ? text : `${text.slice(0, limit - 1)}…`;
}

/** Запомнить разбор до подтверждения. */
export function rememberAttach(session: AttachSession, result: AttachResult): AttachSession {
  if (!result.needsConfirm) return session;
  if (result.spec) return { ...session, attach: { kind: 'spec', spec: result.spec } };
  if (result.brief) return { ...session, attach: { kind: 'brief', brief: result.brief } };
  return session;
}

/** Забыть разбор: после применения или отказа он больше не нужен. */
export function forgetAttach(session: AttachSession): AttachSession {
  const next = { ...session };
  delete next.attach;
  return next;
}

/**
 * Что бот делает с подтверждённым разбором. Возвращает текст ответа.
 *
 * Применение идёт в сервисе: он один знает, чьё это КП, и он же сверяет цены
 * с каталогом. Бот ничего не пересчитывает — иначе цены в чате и в файле
 * разошлись бы.
 */
export async function applyPending(
  pending: AttachPending,
  apply: (mode: 'brief' | 'spec', payload: AttachPending) => Promise<{ reply: string }>,
): Promise<string> {
  const res = await apply(pending.kind, pending);
  return res.reply;
}

/** Ответ на «✏️ Не надо». */
export function cancelMessage(pending: AttachPending | undefined): string {
  if (!pending) return 'Хорошо, ничего не применяю.';
  if (pending.kind === 'spec') {
    return 'Хорошо, состав оставляю как был. Соберите его из каталога или пришлите таблицу ещё раз.';
  }
  return 'Хорошо, бриф не трогаю. Можете рассказать всё в чате — вопросы я задам сам.';
}

/** Подсказка, если файл прислали, когда нечего к нему привязать. */
export const NO_OFFER_FOR_FILE =
  'Чтобы приложить файл, сначала соберите предложение: «🧾 Собрать КП». ' +
  'Файл дополнит бриф и состав уже созданного КП.';