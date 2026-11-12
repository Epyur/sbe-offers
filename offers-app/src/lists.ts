/**
 * Списки КП и боевых листков (2026-11-13, шаг S2).
 *
 * Модель данных, прежде чем код (правило 2026-11-11):
 *   - список приходит с сервера целиком (LIMIT 200) и уже отфильтрован JWT-правами;
 *   - поиск идёт НА СЕРВЕРЕ (`GET /offers?q=`, `GET /combat?q=`) — клиент только
 *     передаёт строку и показывает ответ. Фильтрация в браузере искала бы одну
 *     страницу и молча врала бы «ничего не найдено» (§9.1 плана);
 *   - строки списка — не полный документ, а то, что нужно для выбора: номер,
 *     заказчик/подпись, состояние, дата. Полный документ открывается в своём
 *     маршруте (`#/offer/:id`, `#/sheet/:id`) следующими шагами.
 *
 * Этот модуль — чистая часть: типы ответа, адреса и подписи. Запрос — в
 * `api.ts`, отрисовка — в `screens.tsx`.
 */
import { apiRequest } from './api.ts';

/** КП в списке (то, что реально нужно экрану из `model.Offer`). */
export interface OfferRow {
  id: number;
  number: string;
  ownerEmail: string;
  client: { name: string; contact: string };
  total: number;
  status: string;
  createdAt: string;
}

/** Боевой листок в списке (`CombatSheet` сервера, усечённый до нужного). */
export interface SheetRow {
  id: number;
  number: string;
  ownerEmail: string;
  title: string;
  ourNotes: Array<{ noteId: number; title: string }>;
  competitorNotes: Array<{ noteId: number; title: string }>;
  status: string;
  createdAt: string;
  displayName?: string;
}

/**
 * Адрес списка: пустой `q` не попадает в адрес вовсе — сервер без `q` обязан
 * вести себя как раньше (регрессия §9.1: плагин, бот и ЦУП не должны
 * почувствовать правку).
 */
export function listPath(kind: 'offers' | 'sheets', query: string): string {
  const base = kind === 'offers' ? '/offers' : '/combat';
  const q = query.trim();
  return q === '' ? base : `${base}?q=${encodeURIComponent(q)}`;
}

/** Загрузить список. Ошибку отдаёт `ApiError` — экран показывает текст. */
export async function loadOffers(query: string): Promise<OfferRow[]> {
  const res = await apiRequest<{ offers: OfferRow[] }>(listPath('offers', query));
  return res.offers ?? [];
}

export async function loadSheets(query: string): Promise<SheetRow[]> {
  const res = await apiRequest<{ sheets: SheetRow[] }>(listPath('sheets', query));
  return res.sheets ?? [];
}

/**
 * Подпись КП в списке. Заказчик — первым: менеджер ищет по нему и он же
 * единственное, что человек помнит.
 */
export function offerLabel(offer: OfferRow): string {
  const client = offer.client?.name?.trim() ?? '';
  if (client !== '') return client;
  const contact = offer.client?.contact?.trim() ?? '';
  if (contact !== '') return contact;
  return 'без заказчика';
}

/**
 * Подпись листка: сервер уже считает её (`displayName` — наш продукт → первый
 * конкурент → «пустой листок»). Здесь только запасной вид для старых ответов,
 * чтобы список не показывал служебное «Боевой листок» вместо продукта.
 */
export function sheetLabel(sheet: SheetRow): string {
  const provided = sheet.displayName?.trim() ?? '';
  if (provided !== '') return provided;
  const ours = sheet.ourNotes?.find((n) => (n.title ?? '').trim() !== '');
  if (ours) return ours.title;
  const theirs = sheet.competitorNotes?.find((n) => (n.title ?? '').trim() !== '');
  if (theirs) return theirs.title;
  const number = sheet.number?.trim() ?? '';
  return number === '' ? 'пустой листок' : `пустой листок № ${number}`;
}

/** Номер с префиксом, как его знает менеджер: «КП 003/2026», «БЛ 12/2026». */
export function numberedLabel(kind: 'offers' | 'sheets', number: string): string {
  const prefix = kind === 'offers' ? 'КП' : 'БЛ';
  return `${prefix} ${number}`;
}

/** Дата из RFC3339 → «13.11.2026»; мусор не роняет список. */
export function shortDate(raw: string): string {
  const at = raw.indexOf('T');
  const datePart = at > 0 ? raw.slice(0, at) : raw;
  const parts = datePart.split('-');
  if (parts.length !== 3) return raw;
  return `${parts[2]}.${parts[1]}.${parts[0]}`;
}

/** Состояние документа словом — черновик/готово, а не `draft`/`final`. */
export function statusLabel(status: string): string {
  return status === 'final' ? 'готово' : 'черновик';
}

/**
 * Что показать, когда список пуст. Это разные вещи: «ничего не нашлось по
 * запросу» и «у вас ещё нет ни одного документа» — человеку нужен разный
 * следующий шаг.
 */
export function emptyText(kind: 'offers' | 'sheets', query: string): string {
  if (query.trim() !== '') {
    return kind === 'offers'
      ? 'По этому заказчику ничего не нашлось.'
      : 'По этому названию ничего не нашлось.';
  }
  return kind === 'offers'
    ? 'Здесь появятся коммерческие предложения.'
    : 'Здесь появятся боевые листки.';
}

/** Подсказка в поле поиска — разная для двух списков (решение §9.1). */
export function searchPlaceholder(kind: 'offers' | 'sheets'): string {
  return kind === 'offers' ? 'Заказчик или часть названия' : 'Название листка или продукт';
}