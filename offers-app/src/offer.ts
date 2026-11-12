/**
 * Карточка КП в приложении (2026-11-13, шаг S3).
 *
 * Модель, согласованная с пользователем («вариант В»): каждое поле сохраняется
 * СВОЕЙ ручкой, полного «сохранить всё» здесь нет.
 *   - заказчик  → `PUT /offers/{id}/client`
 *   - позиции   → `PUT /offers/{id}/items`
 *   - системы   → `POST/DELETE /offers/{id}/system` (существующие)
 * Поэтому правка одного поля не затирает другое, даже если в боте или в плагине
 * его меняли параллельно.
 *
 * Всё, что можно проверить словами (суммы, разбор строки цены, проверка
 * правки), — чистые функции: экран только показывает и вызывает.
 */
import { apiRequest } from './api.ts';

export interface OfferClientDto {
  name: string;
  contact: string;
}

export interface OfferItemDto {
  catalogId?: number | undefined;
  ekn?: string | undefined;
  name: string;
  unit: string;
  qty: number;
  price: number;
  sum?: number;
}

export interface OfferSystemDto {
  sku?: string;
  noteId?: number;
}

export interface OfferDto {
  id: number;
  number: string;
  ownerEmail: string;
  client: OfferClientDto;
  items: OfferItemDto[];
  formValues: Record<string, string>;
  systems: OfferSystemDto[];
  total: number;
  status: string;
  createdAt: string;
  updatedAt: string;
}

/** Карточка системы из справочника (`GET /systems` и `/systems/search`). */
export interface SystemCardDto {
  noteId: number;
  sku?: string;
  title: string;
  problems?: string[];
}

export interface SearchSystemsResponse {
  items?: Array<{ noteId: number; sku?: string; title: string; problems?: string[] }>;
  total?: number;
  kbEnabled?: boolean;
  error?: string;
}

/** Строка системы в карточке: название для человека + ключ выбора. */
export interface ChosenSystem {
  sku: string;
  noteId: number;
  title: string;
}

export async function loadOffer(id: number): Promise<OfferDto> {
  return apiRequest<OfferDto>(`/offers/${id}`);
}

export async function saveClient(id: number, client: OfferClientDto): Promise<OfferDto> {
  return apiRequest<OfferDto>(`/offers/${id}/client`, { method: 'PUT', body: client });
}

export async function saveItems(id: number, items: OfferItemDto[]): Promise<OfferDto> {
  return apiRequest<OfferDto>(`/offers/${id}/items`, { method: 'PUT', body: { items } });
}

/** Состав КП не меняет: `POST` возвращает выбранную систему и само предложение. */
export interface ComposeSystemResponse {
  offer: OfferDto;
  system?: SystemCardDto;
  title?: string;
  composition?: number;
  error?: string;
}

export async function addSystem(id: number, system: ChosenSystem): Promise<ComposeSystemResponse> {
  return apiRequest<ComposeSystemResponse>(`/offers/${id}/system`, {
    method: 'POST',
    body: { sku: system.sku, noteId: system.noteId },
  });
}

export async function removeSystem(id: number, system: ChosenSystem): Promise<ComposeSystemResponse> {
  return apiRequest<ComposeSystemResponse>(`/offers/${id}/system`, {
    method: 'DELETE',
    body: { sku: system.sku, noteId: system.noteId },
  });
}

/** Поиск систем: тот же искомый механизм, что у выбора в плагине (База знаний). */
export async function searchSystems(query: string): Promise<SearchSystemsResponse> {
  const q = query.trim();
  if (q === '') return { items: [], total: 0 };
  return apiRequest<SearchSystemsResponse>(`/systems/search?q=${encodeURIComponent(q)}`);
}

/** Справочник систем: по нему карточка показывает названия уже выбранных. */
export async function loadSystemCatalog(): Promise<SystemCardDto[]> {
  const res = await apiRequest<{ systems?: SystemCardDto[]; error?: string }>('/systems');
  return res.systems ?? [];
}

/** Ключ системы — тот же, что уходит на сервер: `sku` сильнее `noteId`. */
export function systemKey(system: { sku?: string; noteId?: number }): string {
  const sku = (system.sku ?? '').trim().toUpperCase();
  return sku !== '' ? `sku:${sku}` : `note:${system.noteId ?? 0}`;
}

/**
 * Названия выбранных систем. Каталог может не знать систему (её убрали из
 * Базы знаний или справочник ещё не перечитан) — тогда показываем ключ, а не
 * пустую строку: человек должен видеть, что в документе что-то есть.
 */
export function chosenSystems(offer: OfferDto, catalog: SystemCardDto[]): ChosenSystem[] {
  const byKey = new Map<string, SystemCardDto>();
  for (const card of catalog) byKey.set(systemKey(card), card);
  return (offer.systems ?? []).map((ref) => {
    const card = byKey.get(systemKey(ref));
    return {
      sku: (ref.sku ?? '').trim(),
      noteId: ref.noteId ?? 0,
      title: card?.title?.trim() || (ref.sku ?? '').trim() || `система ${ref.noteId ?? ''}`.trim(),
    };
  });
}

/** Строка позиции, которую показывает и правит человек (всё числовое — строкой). */
export interface ItemDraft {
  name: string;
  unit: string;
  qty: string;
  price: string;
  ekn: string;
  catalogId?: number | undefined;
}

export const EMPTY_ITEM: ItemDraft = { name: '', unit: 'шт', qty: '1', price: '', ekn: '' };

export function itemDraftOf(item: OfferItemDto): ItemDraft {
  return {
    name: item.name ?? '',
    unit: item.unit ?? '',
    qty: numberText(item.qty),
    price: numberText(item.price),
    ekn: item.ekn ?? '',
    catalogId: item.catalogId,
  };
}

/**
 * Число в поле ввода. Запятая — десятичный разделитель по-русски, её
 * принимаем как точку; хвост «0.» сохраняем, чтобы поле не прыгало под рукой.
 */
export function numberText(value: number): string {
  if (!Number.isFinite(value)) return '';
  const rounded = Math.round(value * 1e6) / 1e6;
  return String(rounded).replace('.', ',');
}

/** Разбор числа из поля ввода: пусто → 0, «1,5» → 1.5, мусор → NaN. */
export function parseNumber(raw: string): number {
  const trimmed = raw.trim().replace(',', '.').replace(/\s+/g, '');
  if (trimmed === '') return 0;
  const value = Number(trimmed);
  return Number.isFinite(value) ? value : Number.NaN;
}

/** Сумма строки, как её считает сервер: до копеек. */
export function rowSum(draft: ItemDraft): number {
  const qty = parseNumber(draft.qty);
  const price = parseNumber(draft.price);
  if (!Number.isFinite(qty) || !Number.isFinite(price)) return 0;
  return Math.round(qty * price * 100) / 100;
}

/** Итог по всем строкам. Отдельно от суммы строки: сервер считает так же. */
export function draftTotal(items: ItemDraft[]): number {
  let total = 0;
  for (const item of items) total += rowSum(item);
  return Math.round(total * 100) / 100;
}

export function money(value: number): string {
  const fixed = value.toFixed(2);
  const [whole = '0', frac = '00'] = fixed.split('.');
  const grouped = whole.replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
  return `${grouped},${frac}`;
}

/**
 * Проверка перед отправкой. Возвращает текст для человека или пустую строку.
 * Правила те же, что у сервера (`validateOfferItems`): пустое наименование и
 * неположительное количество — ошибка. Пустой список законен.
 */
export function itemsProblem(items: ItemDraft[]): string {
  if (items.length > 500) return 'Слишком много строк: не больше 500.';
  for (const [index, item] of items.entries()) {
    if (item.name.trim() === '') return `Строка ${index + 1}: не заполнено наименование.`;
    const qty = parseNumber(item.qty);
    if (!Number.isFinite(qty)) return `Строка ${index + 1}: количество не число.`;
    if (qty <= 0) return `Строка ${index + 1}: количество должно быть больше нуля.`;
    const price = parseNumber(item.price);
    if (!Number.isFinite(price)) return `Строка ${index + 1}: цена не число.`;
    if (price < 0) return `Строка ${index + 1}: цена не может быть отрицательной.`;
  }
  return '';
}

/** Есть ли несохранённые правки — по этому экран спрашивает при уходе. */
export function itemsChanged(drafts: ItemDraft[], saved: OfferItemDto[]): boolean {
  if (drafts.length !== saved.length) return true;
  return drafts.some((draft, index) => {
    const item = saved[index];
    if (!item) return true;
    return (
      draft.name !== (item.name ?? '') ||
      draft.unit !== (item.unit ?? '') ||
      parseNumber(draft.qty) !== item.qty ||
      parseNumber(draft.price) !== item.price ||
      draft.ekn !== (item.ekn ?? '')
    );
  });
}

export function clientChanged(client: OfferClientDto, saved: OfferClientDto): boolean {
  return client.name !== (saved.name ?? '') || client.contact !== (saved.contact ?? '');
}

/** Тело для сервера из черновика: пустые поля так же пусты, а не NaN. */
export function draftToItem(draft: ItemDraft): OfferItemDto {
  const qty = parseNumber(draft.qty);
  const price = parseNumber(draft.price);
  return {
    catalogId: draft.catalogId,
    ekn: draft.ekn.trim() === '' ? undefined : draft.ekn.trim(),
    name: draft.name.trim(),
    unit: draft.unit.trim(),
    qty: Number.isFinite(qty) ? qty : 0,
    price: Number.isFinite(price) ? price : 0,
  };
}