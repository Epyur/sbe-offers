/**
 * Боевой листок в приложении (2026-11-13, шаг S6).
 *
 * Модель — та же, что у бота (`offers_combat_sheet`): номер, наши продукты,
 * продукты конкурентов, уточнение, автор. Читают и печатают все вошедшие,
 * правит только автор — поэтому экран показывает кнопки правки, только когда
 * это свой листок.
 *
 * Стороны правятся ПОЗИЦИОННО: добавили/убрали один продукт, а не «сохранили
 * весь листок». Так чужое добавление, сделанное параллельно в боте, не
 * затирается.
 *
 * Здесь транспорт и чистая часть (подписи, состояние «можно править»,
 * номера страниц поиска). Экран — в `combat-screen.tsx`.
 */
import { apiRequest } from './api.ts';

export interface CombatRefDto {
  noteId: number;
  title: string;
}

export interface CombatSheetDto {
  id: number;
  number: string;
  ownerEmail: string;
  title: string;
  offerId?: number;
  ourNotes: CombatRefDto[];
  competitorNotes: CombatRefDto[];
  formValues: Record<string, string>;
  status: string;
  createdAt: string;
  updatedAt: string;
  displayName?: string;
}

/** Карточка в поиске стороны: `noteId` — ключ, `title` — что видит человек. */
export interface CombatPickerItem {
  noteId: number;
  title: string;
  sku?: string;
}

export interface CombatPage {
  items: CombatPickerItem[];
  total: number;
  offset: number;
  hasMore: boolean;
}

export interface AnaloguesResponse {
  source?: { noteId: number; title: string };
  cards?: CombatPickerItem[];
}

export type CombatSide = 'our' | 'competitor';

export async function loadSheet(id: number): Promise<CombatSheetDto> {
  return apiRequest<CombatSheetDto>(`/combat/${id}`);
}

/** Поиск стороны: наш продукт — каталог + системы, конкурент — тег `competitor`. */
export async function searchSide(
  side: CombatSide,
  query: string,
  offset = 0,
  limit = 20,
): Promise<CombatPage> {
  const base = side === 'our' ? '/combat/our' : '/combat/competitors-src';
  const params = new URLSearchParams({ offset: String(offset), limit: String(limit) });
  const q = query.trim();
  if (q !== '') params.set('q', q);
  return apiRequest<CombatPage>(`${base}?${params.toString()}`);
}

export async function addSide(id: number, side: CombatSide, card: CombatPickerItem): Promise<CombatSheetDto> {
  const path = side === 'our' ? `/combat/${id}/our` : `/combat/${id}/competitor`;
  return apiRequest<CombatSheetDto>(path, {
    method: 'POST',
    body: { noteId: card.noteId, title: card.title },
  });
}

export async function removeSide(id: number, side: CombatSide, noteId: number): Promise<CombatSheetDto> {
  const path = side === 'our' ? `/combat/${id}/our` : `/combat/${id}/competitor`;
  return apiRequest<CombatSheetDto>(path, { method: 'DELETE', body: { noteId, title: '' } });
}

export async function saveHint(id: number, hint: string): Promise<CombatSheetDto> {
  return apiRequest<CombatSheetDto>(`/combat/${id}/hint`, { method: 'PUT', body: { hint } });
}

/** Аналоги ищутся от НАШЕГО продукта: `noteId` — карточка нашего продукта. */
export async function findAnalogues(noteId: number): Promise<CombatPickerItem[]> {
  const res = await apiRequest<AnaloguesResponse>(`/combat/analogues?noteId=${noteId}`);
  return res.cards ?? [];
}

/** Уточнение листка: `form_values["hint"]` — единственная настройка. */
export function hintOf(sheet: CombatSheetDto): string {
  return sheet.formValues?.hint ?? '';
}

/**
 * Можно ли править листок: автор — да, admin — да (сервер решает то же самое
 * через `combatSheetForEditor`). Чужой листок читается и печатается, но
 * кнопки правки не показываются — нажимать их значило бы получать 403.
 */
export function canEdit(sheet: CombatSheetDto, email: string, role: string): boolean {
  if (role === 'admin' || role === 'superadmin') return true;
  return email !== '' && sheet.ownerEmail.toLowerCase() === email.toLowerCase();
}

/** Сколько у листка сторон — для строки состояния. */
export function sidesCount(sheet: CombatSheetDto): { our: number; competitor: number } {
  return { our: sheet.ourNotes?.length ?? 0, competitor: sheet.competitorNotes?.length ?? 0 };
}

/** Подпись листка: продукт → конкурент → «пустой листок» (как на сервере). */
export function sheetLabelOf(sheet: CombatSheetDto): string {
  const provided = sheet.displayName?.trim() ?? '';
  if (provided !== '') return provided;
  const ours = sheet.ourNotes?.find((ref) => ref.title.trim() !== '');
  if (ours) return ours.title;
  const theirs = sheet.competitorNotes?.find((ref) => ref.title.trim() !== '');
  if (theirs) return theirs.title;
  return sheet.number ? `пустой листок № ${sheet.number}` : 'пустой листок';
}

/** Следующая страница поиска есть, только если сервер так сказал. */
export function nextOffset(page: CombatPage): number | null {
  if (!page.hasMore || page.items.length === 0) return null;
  return page.offset + page.items.length;
}