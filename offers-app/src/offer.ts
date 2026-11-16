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
  /** Материалы, предложенные описанием (2026-11-13). Позиций не добавляют. */
  materials?: OfferSystemDto[];
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

/**
 * Ответ ручек правки документа (2026-11-15, шаг 1 конфигуратора окна).
 *
 * `PUT /client` и `/items` больше не отдают голый `Offer`: сервер прикладывает
 * незаполненные обязательные поля окна (`missingWindowFields`). Обязательность
 * МЯГКАЯ — сохранение прошло, список только говорит, что осталось дополнить.
 */
export interface WindowSaveResponse {
  offer: OfferDto;
  missingRequired?: MissingFieldDto[];
}

/** Пустое обязательное поле окна — то же, что уходит `missingWindowFields`. */
export interface MissingFieldDto {
  id: string;
  label: string;
}

/**
 * Чтение ответа на сохранение.
 *
 * Сервер отвечает `{"offer":…}`; на случай старого ответа (голый `Offer`, как
 * до 0.11.0) разбираем обе формы. Иначе приложение показало бы пустую карточку
 * на живом сервере прежней сборки — класс «сохранение прошло, экран пустой».
 */
export function readWindowSave(res: unknown): WindowSaveResponse {
  if (typeof res !== 'object' || res === null) return { offer: res as OfferDto };
  const candidate = res as Partial<WindowSaveResponse> & Partial<OfferDto>;
  if (candidate.offer !== undefined && typeof candidate.offer === 'object' && candidate.offer !== null) {
    return { offer: candidate.offer, missingRequired: candidate.missingRequired ?? [] };
  }
  return { offer: candidate as OfferDto };
}

export async function saveClient(id: number, client: OfferClientDto): Promise<WindowSaveResponse> {
  const res = await apiRequest<unknown>(`/offers/${id}/client`, { method: 'PUT', body: client });
  return readWindowSave(res);
}

/**
 * Поля формы — свои ручкой (0.11.3).
 *
 * `form_values` — общий склад: там же тексты разделов (`copy.*`) и бриф
 * (`brief.*`). Поэтому отправляем ТОЛЬКО изменённые ключи и PATCH, а не
 * полную замену: иначе правка «Условий» стёрла бы принятые тексты и бриф.
 * Сервер сливает ключи в SQL, так что гонка с приёмом текста раздела не
 * теряет ничью правку.
 */
export async function saveFormValues(
  id: number,
  values: Record<string, string>,
): Promise<WindowSaveResponse> {
  const res = await apiRequest<unknown>(`/offers/${id}/form-values`, {
    method: 'PATCH',
    body: { values },
  });
  return readWindowSave(res);
}

/** Какие ключи реально изменились — по ним и уходит запрос. */
export function changedValues(
  draft: Record<string, string>,
  saved: Record<string, string>,
): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [key, value] of Object.entries(draft)) {
    if ((saved[key] ?? '') !== value) out[key] = value;
  }
  return out;
}

export async function saveItems(id: number, items: OfferItemDto[]): Promise<WindowSaveResponse> {
  const res = await apiRequest<unknown>(`/offers/${id}/items`, { method: 'PUT', body: { items } });
  return readWindowSave(res);
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

/** Ответ выбора материала: карточка и написанное описание. */
export interface ComposeMaterialResponse {
  offer: OfferDto;
  material?: SystemCardDto;
  title?: string;
  texts?: Record<string, string>;
  missing?: string[];
  error?: string;
}

export async function removeSystem(id: number, system: ChosenSystem): Promise<ComposeSystemResponse> {
  return apiRequest<ComposeSystemResponse>(`/offers/${id}/system`, {
    method: 'DELETE',
    body: { sku: system.sku, noteId: system.noteId },
  });
}

/**
 * Материал — ОБЪЕКТ предложения (2026-11-13): в документ идут описание и
 * ссылки, а не цена. Позицию он даёт только через таблицу состава, и это
 * отдельная зона экрана с отдельной подписью.
 */
export async function addMaterial(id: number, material: ChosenSystem): Promise<ComposeMaterialResponse> {
  return apiRequest<ComposeMaterialResponse>(`/offers/${id}/material`, {
    method: 'POST',
    body: { sku: material.sku, noteId: material.noteId },
  });
}

export async function removeMaterial(id: number, material: ChosenSystem): Promise<ComposeMaterialResponse> {
  return apiRequest<ComposeMaterialResponse>(`/offers/${id}/material`, {
    method: 'DELETE',
    body: { sku: material.sku, noteId: material.noteId },
  });
}

/** Поиск систем: тот же искомый механизм, что у выбора в плагине (База знаний). */
export async function searchSystems(query: string): Promise<SearchSystemsResponse> {
  const q = query.trim();
  if (q === '') return { items: [], total: 0 };
  return apiRequest<SearchSystemsResponse>(`/systems/search?q=${encodeURIComponent(q)}`);
}

/** Карточка материала в поиске: название и ключ выбора. */
export interface MaterialSearchItem {
  noteId: number;
  sku?: string;
  name: string;
  unit?: string;
}

export interface SearchMaterialsResponse {
  items?: MaterialSearchItem[];
  total?: number;
  kbEnabled?: boolean;
  error?: string;
}

/** Поиск материалов — тот же серверный поиск, что у выбора материала в позиции. */
export async function searchMaterials(query: string): Promise<SearchMaterialsResponse> {
  const q = query.trim();
  if (q === '') return { items: [], total: 0 };
  return apiRequest<SearchMaterialsResponse>(`/materials/search?q=${encodeURIComponent(q)}`);
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
  return chosenRefs(offer.systems, catalog);
}

/**
 * Названия выбранных МАТЕРИАЛОВ (2026-11-13). Каталог материалов в приложении
 * не грузится: у материала название приходит из описания и продукта, а здесь
 * достаточно кода или номера заметки — человек видит, что материал выбран.
 */
export function chosenMaterials(offer: OfferDto): ChosenSystem[] {
  return chosenRefs(offer.materials, []);
}

function chosenRefs(refs: OfferSystemDto[] | undefined, catalog: SystemCardDto[]): ChosenSystem[] {
  const byKey = new Map<string, SystemCardDto>();
  for (const card of catalog) byKey.set(systemKey(card), card);
  return (refs ?? []).map((ref) => {
    const card = byKey.get(systemKey(ref));
    return {
      sku: (ref.sku ?? '').trim(),
      noteId: ref.noteId ?? 0,
      title: card?.title?.trim() || (ref.sku ?? '').trim() || `материал ${ref.noteId ?? ''}`.trim(),
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