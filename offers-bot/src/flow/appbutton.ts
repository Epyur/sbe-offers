/**
 * Кнопки «открыть приложение» (2026-11-13, шаг B+: внеплановый, до S7).
 *
 * Одна точка, где строится адрес Mini App и разметка кнопки: бот, список КП
 * и список листков не должны собирать URL каждый по-своему — иначе адрес
 * разойдётся между меню и записями.
 *
 * Кнопка Telegram `web_app` открывает приложение сама: у неё нет
 * `callback_data`, боту не нужен обработчик нажатия. Ссылка несёт маршрут
 * (`#/offer/12`), поэтому один и тот же приём работает и в меню, и в записи.
 */

/** Публичный адрес приложения. Переопределяется окружением для стенда. */
export function appUrlFromEnv(): string {
  const raw = (process.env.OFFERS_APP_URL ?? '').trim();
  const base = raw === '' ? 'https://epyur.fvds.ru/app-offers/' : raw;
  return base.endsWith('/') ? base : `${base}/`;
}

/** Список записей или конкретная запись. */
export type AppTarget = { kind: 'list' } | { kind: 'offer'; id: number } | { kind: 'sheet'; id: number };

/** Полный адрес приложения для маршрута. */
export function appUrl(target: AppTarget): string {
  const base = appUrlFromEnv();
  switch (target.kind) {
    case 'list':
      return `${base}#/list/offers`;
    case 'offer':
      return `${base}#/offer/${target.id}`;
    case 'sheet':
      return `${base}#/sheet/${target.id}`;
  }
}

/** Кнопка `web_app` — единственный вид кнопки, открывающей Mini App. */
export interface WebAppButton {
  text: string;
  web_app: { url: string };
}

export function appButton(text: string, kind: AppTarget['kind'], id?: number): WebAppButton {
  const target: AppTarget = kind === 'offer' ? { kind, id: id ?? 0 } : kind === 'sheet' ? { kind, id: id ?? 0 } : { kind: 'list' };
  return { text, web_app: { url: appUrl(target) } };
}

/** Строка списка КП: одна кнопка приложения на запись, с её номером. */
export function offerAppMarkup(offers: Array<{ id: number; number: string }>): WebAppButton[][] {
  return offers.map((o) => [appButton(`📱 ${o.number}`, 'offer', o.id)]);
}

/** Строка списка листков: номер и продукт в подписи, чтобы не путать записи. */
export function sheetsAppMarkup(sheets: Array<{ id: number; number: string; label: string }>): WebAppButton[][] {
  return sheets.map((s) => [appButton(`📱 ${s.number} — ${s.label}`, 'sheet', s.id)]);
}

/** Главное меню: кнопка на список КП. */
