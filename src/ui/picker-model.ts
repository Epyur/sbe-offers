/**
 * Правила окна выбора продукта — без DOM (2026-11-21).
 *
 * Окна КП (`offers-list.ts`) и боевого листка (`combat-view.ts`) показывают
 * выбранные продукты и найденное по запросу. Два правила живут здесь, чтобы
 * оба окна не разошлись:
 *
 * 1. **До начала ввода списка нет.** Раньше окно сразу показывало первые 20
 *    строк кэша: человек открывал выбор и читал чужой каталог вместо того,
 *    чтобы писать название. В мини-приложении это уже решено
 *    (`shouldSearchSide`, 2026-11-17) — здесь то же правило на оба окна.
 * 2. **Выбранное подписывается НАЗВАНИЕМ, а не кодом.** У выбора в КП есть
 *    только `noteId`/`sku` (сервер названий не хранит), а окно помнит имена
 *    всего, что встречалось. Иначе после второго поиска — или после
 *    переоткрытия КП — в списке выбранного появлялось `sys-412` / `MAT-…`.
 *
 * Модуль чистый: `node --test` проверяет его без Obsidian.
 */

/** Вариант выбора: заметка Базы знаний и её подпись. */
export interface PickerCandidate {
  noteId: number;
  title: string;
  code?: string;
}

/** Выбор продукта так, как он лежит у КП: заметка и/или код. */
export interface PickerRef {
  noteId?: number;
  sku?: string;
}

/** Что написать вместо списка, пока человек не начал вводить название. */
export const PICKER_EMPTY_HINT =
  'Начните вводить название — совпадения появятся здесь.';

/**
 * Совпадение по запросу: слова ищутся по названию и коду, регистр и «ё» не
 * важны. То же правило, что в боте — одно место на все окна.
 */
export function filterCandidates<T extends { title: string; code?: string }>(
  items: T[],
  query: string,
): T[] {
  const q = query.trim().toLowerCase().replace(/ё/g, 'е');
  if (!q) return items;
  const words = q.split(/\s+/).filter((w) => w !== '');
  return items.filter((item) => {
    const hay = `${item.title} ${item.code ?? ''}`.toLowerCase().replace(/ё/g, 'е');
    return words.every((w) => hay.includes(w));
  });
}

/**
 * Строки, которые окно рисует под полем поиска.
 *
 * Пустой запрос — пустой список, без исключений: «показать всё» здесь не
 * режим, а случай, из которого человек не понимает, что от него хотят.
 * Найденное сервером сильнее локального фильтра по кэшу.
 */
export function pickerRows<T extends { title: string; code?: string }>(
  cache: T[],
  foundByServer: T[] | null,
  query: string,
): T[] {
  if (query.trim() === '') return [];
  return foundByServer ?? filterCandidates(cache, query);
}

/**
 * Хэндл выбранного продукта — тот же, что уходит на удаление и в подписи.
 * Заметка главнее кода: у живых карточек кода нет вовсе (2026-11-02).
 */
export function refHandle(ref: PickerRef): string {
  if (ref.noteId && ref.noteId > 0) return `sys-${ref.noteId}`;
  return (ref.sku ?? '').trim();
}

/** Имена продуктов, встреченных окном: хэндл → название. */
export type PickerNames = Map<string, string>;

/**
 * Запомнить варианты окна. Один продукт кладётся под ОБА ключа — по заметке
 * и по коду: сервер может прислать выбор в любом виде, а человеку нужно имя.
 */
export function rememberNames(
  names: PickerNames,
  items: Array<{ noteId?: number; title: string; code?: string }>,
): void {
  for (const item of items) {
    const title = item.title.trim();
    if (title === '') continue;
    if (item.noteId && item.noteId > 0) names.set(`sys-${item.noteId}`, title);
    const code = (item.code ?? '').trim();
    if (code !== '') names.set(code, title);
  }
}

/**
 * Подпись выбранного: название, если оно встречалось, иначе сам хэндл.
 *
 * Хэндл остаётся запасным путём на честный случай «имени ещё никто не видел»
 * (кэш не прочитался) — пустая строка была бы хуже: человек не понял бы, что
 * продукт вообще выбран.
 */
export function refTitle(names: PickerNames, ref: PickerRef): string {
  const handle = refHandle(ref);
  return names.get(handle) ?? handle;
}