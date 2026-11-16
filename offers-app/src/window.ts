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

/**
 * Зоны экрана КП. Набор совпадает с `knownWindowZones` сервера.
 *
 * `brief` выведен из зон в 0.11.3: пользователь попросил вызывать бриф из
 * карточки каждого раздела, и отдельный блок стал настройкой ни о чём.
 */
export type WindowZoneId = 'client' | 'systems' | 'items' | 'materials' | 'sections';

/**
 * Порядок зон «как сейчас» — с него начинается любой экран (сервер, шаг 1).
 *
 * Брифа здесь нет намеренно (2026-11-15): КП могло быть собрано в плагине без
 * разговора, и пустая карточка брифа не должна появляться у всех сама.
 * Администратор включает зону в конфигураторе окна — сервер дописывает её в
 * конец невидимой.
 */
export const DEFAULT_ZONES: readonly WindowZoneId[] = ['client', 'systems', 'items', 'materials', 'sections'];

const KNOWN_ZONES: readonly string[] = [...DEFAULT_ZONES];

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
export async function loadOfferWindow(): Promise<OfferFormWindowDto | null> {
  try {
    // Возвращаем ФОРМУ ЦЕЛИКОМ, а не список зон (0.11.3): до этого поля формы
    // терялись прямо здесь — `visibleZones` отдавала только зоны, и до рендера
    // `kind:"field"` просто не доходил. Именно поэтому «поля не видны в
    // приложении» не лечилось правкой экрана.
    return await apiRequest<OfferFormWindowDto>('/form?mode=client');
  } catch {
    return null;
  }
}

/** Один элемент экрана КП: зона или поле формы — в порядке окна. */
export type ScreenItem =
  | { kind: 'zone'; id: WindowZoneId }
  | { kind: 'field'; id: string; label: string };

/**
 * ЭКРАН КП — одним проходом по окну (0.11.3).
 *
 * До 0.11.3 здесь был `visibleZones`, и поля формы отбрасывались насовсем
 * (`if (item.kind !== 'zone') continue`) — в приложении их не было ВООБЩЕ, хотя
 * сервер их отдавал. Разбор 2026-11-16 (пять моделей сошлись) показал, что
 * причина не в настройке, а в отсутствующем рендере.
 *
 * Правила:
 *   - порядок — ровно как в окне: администратор может поставить поле МЕЖДУ
 *     зонами, и отдельный блок полей этот порядок бы сломал;
 *   - скрытое не возвращается;
 *   - незнакомая зона пропускается (её некому нарисовать);
 *   - поле без подписи в окне не рисуется: подпись — из формы, а поле, которого
 *     в форме нет, — «осиротевшая» ссылка (класс «настройка ни о чём»);
 *   - дубли отбрасываются.
 */
export function screenItems(form: OfferFormWindowDto | undefined | null): ScreenItem[] {
  const items = form?.window ?? [];
  const out: ScreenItem[] = [];
  const seen = new Set<string>();
  for (const item of items) {
    if (item.visible === false) continue;
    if (item.kind === 'zone') {
      const zone = zoneOf(item.id);
      if (zone === null) continue;
      const key = `zone:${zone}`;
      if (seen.has(key)) continue;
      seen.add(key);
      out.push({ kind: 'zone', id: zone });
      continue;
    }
    if (item.kind === 'field') {
      const id = item.id.trim();
      if (id === '') continue;
      const key = `field:${id}`;
      if (seen.has(key)) continue;
      seen.add(key);
      // Подпись приходит из формы (`label` в элементе окна уже проставлен
      // сервером); без неё рисовать нечего — показывать id человеку нельзя.
      const label = item.label?.trim() ?? '';
      if (label === '') continue;
      out.push({ kind: 'field', id, label });
    }
  }
  if (out.length === 0 && !hasWindowSettings(form)) {
    return DEFAULT_ZONES.map((zone) => ({ kind: 'zone' as const, id: zone }));
  }
  return out;
}

/**
 * Элементы окна, которые сервер прислал видимыми, а нарисовать нечем.
 *
 * Сторож из совещания 2026-11-16 (`claude-5-opus`): исходный дефект «поля
 * отданы, но не нарисованы» ловился бы в тот же день. Пустой список — норма.
 */
export function unrenderedItems(form: OfferFormWindowDto | undefined | null): string[] {
  const shown = new Set(screenItems(form).map((item) => `${item.kind}:${item.id}`));
  const out: string[] = [];
  for (const item of form?.window ?? []) {
    if (item.visible === false) continue;
    if (item.kind === 'zone' && zoneOf(item.id) === null) out.push(`zone:${item.id}`);
    if (item.kind === 'field' && !shown.has(`field:${item.id}`)) out.push(`field:${item.id}`);
  }
  return out;
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