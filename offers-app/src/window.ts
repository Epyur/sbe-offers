/**
 * Окно КП в приложении (2026-11-15, шаг 2 плана 0.11.0).
 *
 * Администратор в плагине задаёт порядок, видимость и обязательность зон
 * экрана и полей формы; настройка приходит с формой КП (`offers_form.window_config`)
 * и уже нормализована сервером (`effectiveWindow`, шаг 1). Здесь — только
 * чистая часть, которую можно проверить словами (`test/window.test.ts`):
 * какие зоны рисовать и в каком порядке, и что из обязательного пусто.
 *
 * Пустое окно = «как сейчас»: зоны идут в прежнем порядке экрана. Поэтому
 * пустая настройка не может спрятать раздел — она может только не мешать.
 */
import { apiRequest } from './api.ts';
import type { OfferDto } from './offer.ts';

/** Зоны экрана КП. Набор совпадает с `knownWindowZones` сервера. */
export type WindowZoneId = 'client' | 'systems' | 'items' | 'materials' | 'sections';

/** Порядок зон «как сейчас» — с него начинается любой экран (сервер, шаг 1). */
export const DEFAULT_ZONES: readonly WindowZoneId[] = ['client', 'systems', 'items', 'materials', 'sections'];

const KNOWN_ZONES: readonly string[] = DEFAULT_ZONES;

/** Подпись зоны для человека. Язык экрана один, поэтому подпись живёт здесь. */
export const ZONE_TITLES: Record<WindowZoneId, string> = {
  client: 'Заказчик',
  systems: 'Системы',
  items: 'Позиции с ценами',
  materials: 'Материалы (описанием)',
  sections: 'Тексты разделов',
};

/**
 * Один элемент окна, как его отдаёт сервер. `zone` рисуется зоной экрана,
 * `field` — полем формы; поля приложение рисует шагом 3 (`FieldsZone`).
 */
export interface WindowItemDto {
  kind: 'field' | 'zone';
  id: string;
  label?: string;
  visible?: boolean;
  required?: boolean;
}

/** Ответ `GET /form?mode=client`: нам нужна только часть окна. */
export interface OfferFormWindowDto {
  window?: WindowItemDto[];
  error?: string;
}

/** Пустое обязательное — что показать человеку (подсветка, мягкое правило). */
export interface MissingField {
  id: string;
  label: string;
}

/**
 * Видимые зоны в порядке окна.
 *
 * Правила:
 *   - пустое окно (нет настройки) → `DEFAULT_ZONES`, порядок прежний;
 *   - в окне учитываются только `kind:"zone"`; поля формы зонами не являются;
 *   - скрытая зона не возвращается вовсе;
 *   - незнакомая зона игнорируется (её некому нарисовать — это класс
 *     «настройка, которая ничего не делает»);
 *   - зона, которой в окне нет, тоже не возвращается: сервер дописывает
 *     недостающие зоны скрытыми, и `effectiveWindow` уже сделал это за нас —
 *     повторять правило здесь значило бы держать две правды.
 */
export function visibleZones(form: OfferFormWindowDto | undefined | null): WindowZoneId[] {
  const items = form?.window ?? [];
  const zones: WindowZoneId[] = [];
  for (const item of items) {
    if (item.kind !== 'zone') continue;
    if (item.visible === false) continue;
    const zone = zoneOf(item.id);
    if (zone === null) continue;
    if (zones.includes(zone)) continue;
    zones.push(zone);
  }
  if (zones.length === 0 && !hasWindowSettings(form)) return [...DEFAULT_ZONES];
  return zones;
}

/** В окне вообще есть настройка зон — по этому отличаем «пусто» от «всё скрыли». */
function hasWindowSettings(form: OfferFormWindowDto | undefined | null): boolean {
  const items = form?.window ?? [];
  return items.some((item) => item.kind === 'zone');
}

/** Строка сервера → известная зона или `null` (незнакомое не рисуем). */
export function zoneOf(id: string): WindowZoneId | null {
  for (const zone of KNOWN_ZONES) {
    if (zone === id) return zone as WindowZoneId;
  }
  return null;
}

/**
 * Окно с сервера. Ошибка чтения формы — не повод прятать экран: клиент
 * показывает все зоны и строку «настройку окна прочитать не удалось»
 * (шаг 3). Поэтому здесь `null` означает именно «не прочитали», а не «пусто».
 */
export async function loadOfferWindow(): Promise<WindowZoneId[] | null> {
  try {
    const form = await apiRequest<OfferFormWindowDto>('/form?mode=client');
    return visibleZones(form);
  } catch {
    return null;
  }
}

/** Зоны для экрана: не прочиталось — все в прежнем порядке. */
export function zonesForScreen(loaded: WindowZoneId[] | null): WindowZoneId[] {
  return loaded ?? [...DEFAULT_ZONES];
}

/**
 * Какие обязательные пусты — для подсветки.
 *
 * Обязательность МЯГКАЯ (решение пользователя): список только показывает, что
 * осталось незаполненным, сохранение не блокируется. Правило сервера
 * (`missingWindowFields`): элемент `Required`, `Visible` и пустой; зона
 * `client` — пусто и имя, и контакт.
 *
 * Окно берётся из формы; предложение — из ответа ручки записи (шаг 3 зовёт
 * это после `PUT /client` и `PUT /items`).
 */
export function requiredOf(
  form: OfferFormWindowDto | undefined | null,
  offer: Pick<OfferDto, 'client' | 'formValues'>,
): MissingField[] {
  const items = form?.window ?? [];
  const missing: MissingField[] = [];
  for (const item of items) {
    if (item.required !== true) continue;
    if (item.visible === false) continue;
    if (item.kind === 'field') {
      const value = offer.formValues?.[item.id] ?? '';
      if (value.trim() !== '') continue;
      missing.push({ id: item.id, label: item.label?.trim() || item.id });
      continue;
    }
    if (item.kind === 'zone' && item.id === 'client') {
      const name = (offer.client?.name ?? '').trim();
      const contact = (offer.client?.contact ?? '').trim();
      if (name !== '' || contact !== '') continue;
      missing.push({ id: 'client', label: item.label?.trim() || ZONE_TITLES.client });
    }
  }
  return missing;
}