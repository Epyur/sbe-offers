/**
 * Окно КП в приложении — чистые правила редактора (2026-11-15, шаг 4).
 *
 * Модель совпадает с сервером один в один (`offers-service/form_window.go`):
 * окно — список элементов `zone`/`field` с порядком, видимостью и
 * обязательностью. Здесь живёт только то, что можно проверить словами: сборка
 * окна из формы, проверка, нормализация, предпросмотр. DOM и Obsidian — в
 * `window-editor.ts`.
 *
 * Почему отдельный файл: `check_window_rules.test.mjs` гоняет его в обычном
 * node — редактор с DOM так проверить нельзя, а правила «что попадёт в данные»
 * проверить обязательно. Тот же приём, что у `sections-model.ts`.
 */
import type { Placeholder, WindowItem } from '../types/offers';

/** Виды элементов окна — те же строки, что у сервера. */
export type WindowKind = 'field' | 'zone';

/**
 * Зоны экрана КП. Порядок — порядок по умолчанию (прежнее поведение экрана):
 * Заказчик → Системы → Позиции → Материалы → Тексты разделов.
 */
export const KNOWN_ZONES = ['client', 'systems', 'items', 'materials', 'sections'] as const;

export type WindowZoneID = (typeof KNOWN_ZONES)[number];

/** Подписи зон для человека: язык экрана один, в данных они не хранятся. */
export const ZONE_TITLES: Record<WindowZoneID, string> = {
  client: 'Заказчик',
  systems: 'Системы',
  items: 'Позиции с ценами',
  materials: 'Материалы (описанием)',
  sections: 'Тексты разделов',
};

/** Единственная зона, которую можно сделать обязательной (у неё есть поля). */
export const REQUIRED_ZONE: WindowZoneID = 'client';

export function isKnownZone(id: string): id is WindowZoneID {
  return (KNOWN_ZONES as readonly string[]).includes(id);
}

/**
 * Подпись элемента окна: у зоны — из кода, у поля — из формы.
 *
 * Зона без подписи в коде — дефект (её некому нарисовать), поэтому неизвестная
 * зона возвращает свой id: человек должен видеть, что именно он настроил, а не
 * пустую строку.
 */
export function windowTitle(item: WindowItem, placeholders: Placeholder[]): string {
  if (item.kind === 'zone') {
    return isKnownZone(item.id) ? ZONE_TITLES[item.id] : item.id;
  }
  const placeholder = placeholders.find((p) => p.id === item.id);
  return placeholder?.label?.trim() || item.label?.trim() || item.id;
}

/**
 * Окно по умолчанию — «как сейчас»: пять зон видимыми в прежнем порядке,
 * затем все поля формы. `Required` у полей берётся из формы (прежняя
 * галочка «обязательное»), у зон — снят: обязательной бывает только `client`,
 * и это отдельное решение администратора, а не унаследованное.
 */
export function defaultWindow(placeholders: Placeholder[]): WindowItem[] {
  const items: WindowItem[] = KNOWN_ZONES.map((zone) => ({
    kind: 'zone',
    id: zone,
    visible: true,
    required: false,
  }));
  for (const placeholder of placeholders) {
    items.push({
      kind: 'field',
      id: placeholder.id,
      label: placeholder.label,
      visible: true,
      required: false,
    });
  }
  return items;
}

/**
 * Рабочий список для редактора.
 *
 * Пусто с сервера — «как сейчас» (`defaultWindow`): старая форма открывается
 * со знакомым порядком, а не пустым списком. Нормализация — серверная работа
 * (`effectiveWindow`), и повторять её здесь значило бы держать две правды;
 * клиент добивает только то, без чего редактор не нарисует строку (подпись
 * поля из формы — единственный источник).
 */
export function windowForEditor(saved: WindowItem[] | undefined, placeholders: Placeholder[]): WindowItem[] {
  const source = saved ?? [];
  if (source.length === 0) return defaultWindow(placeholders);
  return source.map((item) => ({
    kind: item.kind === 'field' ? 'field' : 'zone',
    id: item.id,
    ...(item.kind === 'field' ? { label: windowTitle(item, placeholders) } : {}),
    visible: item.visible !== false,
    required: item.required === true,
  }));
}

/**
 * Проверка окна на клиенте — до запроса, чтобы человек увидел проблему там, где
 * она возникла. Правила те же, что у `validateWindow` сервера:
 *
 *   - `kind` — `zone` или `field`;
 *   - зона известна, поле — заведённый плейсхолдер;
 *   - нет дублей пары (kind, id);
 *   - `required` только у полей и у зоны `client` (у остальных зон нечего
 *     заполнять, галочка была бы обещанием без последствий);
 *   - **скрытое обязательное запрещено** (решение пользователя 2026-11-15:
 *     «обязательное не может быть скрытым»).
 *
 * Пустое окно законно. Возвращает текст для человека или пустую строку.
 */
export function windowProblem(items: WindowItem[], placeholders: Placeholder[]): string {
  if (items.length === 0) return '';
  const ids = new Set(placeholders.map((p) => p.id));
  const seen = new Set<string>();
  for (const item of items) {
    if (item.kind !== 'zone' && item.kind !== 'field') {
      return `в окне КП неизвестный элемент «${String(item.kind)}»`;
    }
    const id = item.id.trim();
    if (id === '') return 'в окне КП есть элемент без идентификатора';
    if (item.kind === 'zone' && !isKnownZone(id)) return `в окне КП неизвестная зона «${id}»`;
    if (item.kind === 'field' && !ids.has(id)) {
      return `в окне КП поле «${id}» — его нет среди полей формы`;
    }
    const key = `${item.kind}\u0000${id}`;
    if (seen.has(key)) return `в окне КП элемент «${windowTitle(item, placeholders)}» повторяется`;
    seen.add(key);
    if (item.required === true) {
      if (item.kind === 'zone' && id !== REQUIRED_ZONE) {
        return `зона «${ZONE_TITLES[id as WindowZoneID]}» не может быть обязательной`;
      }
      if (item.visible === false) {
        return (
          `«${windowTitle(item, placeholders)}» скрыта и обязательна одновременно: ` +
          'пока элемент скрыт, он не обязателен — снимите галочку'
        );
      }
    }
  }
  return '';
}

/**
 * Окно к сохранению: `required` снимается у всего скрытого.
 *
 * Это не поблажка, а то же правило, что `normalizeWindow` сервера: скрытое
 * необязательно по определению, и отправлять серверу состояние, которое он
 * отвергнет, значит показать человеку ошибку без причины и совета.
 */
export function windowForSave(items: WindowItem[]): WindowItem[] {
  return items.map((item) =>
    item.visible === false ? { ...item, required: false } : { ...item },
  );
}

/**
 * Что увидит менеджер — строки предпросмотра, в порядке окна.
 *
 * Скрытое в предпросмотр не попадает вовсе (его не будет и на экране);
 * обязательное помечено, чтобы администратор видел требование там же, где его
 * задал.
 */
export function windowPreview(items: WindowItem[], placeholders: Placeholder[]): string[] {
  return items
    .filter((item) => item.visible !== false)
    .map((item) => `${windowTitle(item, placeholders)}${item.required === true ? ' — обязательное' : ''}`);
}

/**
 * Поля формы, которые карточка КП показывает менеджеру в плагине, и какие из
 * них обязательные.
 *
 * Зачем: окно настраивает ЗОНЫ приложения, но поля формы живут и в плагине.
 * Если администратор скрыл поле, плагин обязан его тоже не показывать —
 * иначе настройка «скрыть поле» действует только на телефоне, а менеджер в
 * вольте видит поле и заполняет то, что администратор убрал (класс
 * «настройка, которая ничего не делает»).
 *
 * Зоны здесь не участвуют: у плагина своя вёрстка карточки.
 * Пустое окно — прежнее поведение: все поля, обязательность из формы.
 */
export function formFieldWindow(
  saved: WindowItem[] | undefined,
  placeholders: Placeholder[],
  requiredFields: string[],
): { placeholders: Placeholder[]; required: Set<string> } {
  if (saved === undefined || saved.length === 0) {
    return { placeholders, required: new Set(requiredFields) };
  }
  const byId = new Map(placeholders.map((p) => [p.id, p]));
  const visible: Placeholder[] = [];
  const required = new Set<string>();
  for (const item of saved) {
    if (item.kind !== 'field') continue;
    const placeholder = byId.get(item.id);
    if (placeholder === undefined) continue;
    if (item.visible === false) continue;
    visible.push(placeholder);
    // Скрытое обязательное запрещено, но проверка `visible` — второй рубеж,
    // как у `missingWindowFields` сервера: список не должен зависеть от
    // порядка вызовов.
    if (item.required === true) required.add(item.id);
  }
  return { placeholders: visible, required };
}
export function windowSummary(items: WindowItem[]): string {
  const visible = items.filter((item) => item.visible !== false).length;
  const required = items.filter((item) => item.visible !== false && item.required === true).length;
  if (items.length === 0) return 'как сейчас';
  return required > 0 ? `видно ${visible}, обязательных ${required}` : `видно ${visible}`;
}

/**
 * Перестановка на месте: ▲ поднимает, ▼ опускает. Возвращает `false`, если
 * двигать некуда — кнопку в этом случае надо погасить, а не оставлять
 * нажимаемой без последствий.
 */
export function moveWindowItem(items: WindowItem[], index: number, delta: number): boolean {
  const to = index + delta;
  if (index < 0 || index >= items.length || to < 0 || to >= items.length) return false;
  const [moved] = items.splice(index, 1);
  if (moved === undefined) return false;
  items.splice(to, 0, moved);
  return true;
}

/**
 * Поле, убранное из формы, убирается и из окна: иначе окно ссылается на
 * несуществующее поле, и `validateWindow` сервера отвергнет СОХРАНЕНИЕ всей
 * формы из-за строки, которую человек уже удалил. Обратного правила нет: поле,
 * добавленное в форму, дописывается сервером скрытым (это его работа —
 * «недостающее в конец невидимым»), а клиент не угадывает, куда его ставить.
 */
export function dropMissingFields(items: WindowItem[], placeholders: Placeholder[]): WindowItem[] {
  const ids = new Set(placeholders.map((p) => p.id));
  return items.filter((item) => item.kind !== 'field' || ids.has(item.id));
}

/**
 * Поле, добавленное администратором в форму, дописывается в конец видимым.
 *
 * Единственное место, где клиент поступает иначе, чем сервер (`normalizeWindow`
 * дописывает скрытым): сервер обрабатывает старую форму, где окно не
 * настраивали, а здесь человек только что завёл поле и ждёт его на экране.
 * Правило действует только пока группа раскрыта и человек видит результат.
 */
export function appendNewFields(items: WindowItem[], placeholders: Placeholder[]): WindowItem[] {
  const known = new Set(items.filter((item) => item.kind === 'field').map((item) => item.id));
  const added: WindowItem[] = [];
  for (const placeholder of placeholders) {
    if (known.has(placeholder.id)) continue;
    added.push({
      kind: 'field',
      id: placeholder.id,
      label: placeholder.label,
      visible: true,
      required: false,
    });
  }
  return added.length === 0 ? items.map((item) => ({ ...item })) : [...items.map((item) => ({ ...item })), ...added];
}