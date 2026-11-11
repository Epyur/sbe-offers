/**
 * Подсказка «что у нас есть» по человеческой фразе (2026-11-12).
 *
 * Живые жалобы, из которых это выросло:
 *   - КП008: в брифе человек написал «Гарант», а бот не предложил ни «ТН-Кровля
 *     Гарант ПЛЮС», ни «… RE30»;
 *   - КП013: человек написал «Кровля» — бот должен был ответить, что кровельных
 *     системных решений несколько, и предложить помощь в поиске.
 *
 * Свободная фраза раньше молча уходила в поле брифа. Теперь та же фраза
 * ищется по Базе знаний — одним запросом по системам и материалам, — и бот
 * показывает найденное списком с кнопками-номерами.
 *
 * Модуль без сети: тексты, разбор просьбы о помощи и клавиатуры. Запросы —
 * `api.suggestions`, нажатия — `callbacks.ts`.
 */
import type { CombatSystemItem, SuggestMaterial } from '../api.ts';

/** Сколько строк показываем в одном ответе: больше человек не прочтёт. */
export const SUGGEST_LIMIT = 8;

/** Один найденный вариант: система или материал. */
export interface SuggestItem {
  kind: 'system' | 'material';
  noteId: number;
  title: string;
  code?: string;
}

/**
 * Разбор просьбы о помощи — «какие есть системы кровли», «помоги подобрать»,
 * «что можете предложить».
 *
 * Возвращает то, что надо искать: слова, оставшиеся после служебных («какие»,
 * «есть», «системы», «помоги»). Пустая строка — значит, человек просит помощь
 * вообще, и тогда искать нечего: важно показать, как это делается.
 */
export function parseHelpRequest(text: string): string | null {
  const normalized = text.trim().toLowerCase().replace(/ё/g, 'е');
  if (normalized === '') return null;
  // `\b` в JS считается по ASCII и на кириллице не находит ничего (грабля
  // проекта, ловилась трижды). Поэтому слова разбираются в токены, а признаки
  // просьбы — это префиксы токенов.
  const tokens = normalized
    .replace(/[.,!?;:()«»"']/g, ' ')
    .split(/\s+/)
    .filter((w) => w !== '');
  const hasHelp =
    tokens.some((w) => HELP_PREFIXES.some((p) => w.startsWith(p))) ||
    tokens.some((w) => ASK_WORDS.has(w));
  if (!hasHelp) return null;
  return tokens
    .filter(
      (w) => !HELP_STOP.has(w) && !ASK_WORDS.has(w) && !HELP_PREFIXES.some((p) => w.startsWith(p)),
    )
    .join(' ');
}

/** Начала слов просьбы: «помог…», «подбер…», «подскаж…» — всё это просьба. */
const HELP_PREFIXES = ['помог', 'подбер', 'подобр', 'подскаж', 'посовету', 'предлож'];

/** Вопросы, с которых начинают просьбу о помощи. */
const ASK_WORDS = new Set(['какие', 'какой', 'какая', 'какое', 'каких', 'что']);

/** Служебные слова, в запрос не уходящие. */
const HELP_STOP = new Set([
  'есть', 'ли', 'у', 'вас', 'вы', 'можете', 'можем', 'нам', 'мне', 'надо',
  'нужно', 'системы', 'система', 'систем', 'системных', 'системное',
  'решения', 'решение', 'по', 'для', 'и', 'в', 'на', 'с', 'от', 'это',
  'пожалуйста', 'дайте', 'расскажите', 'покажите', 'а', 'же', 'бы', 'мы',
]);

/**
 * Дополнения к тексту вопроса, если человек просит помощь «вообще».
 *
 * Это не поиск: он объясняет, что бот умеет, и показывает два способа — поиск
 * словами и явную команду.
 */
export function helpHints(): string {
  return (
    'Я ищу системы и материалы в Базе знаний. Напишите, что нужно, — например ' +
    '«кровля», «Гарант», «ПВХ мембрана», — и я покажу, что у нас есть. ' +
    'Можно и точнее: система <название>.'
  );
}

/** Заголовок ответа для найденного. */
export function suggestHeadline(query: string, systems: number, materials: number): string {
  const q = query.trim();
  if (systems > 0 && materials > 0) {
    return `По запросу «${q}» у нас есть систем: ${systems} и материалов: ${materials}.`;
  }
  if (systems > 0) {
    // Живая формулировка из КП013: человеку важно понять, что вариантов
    // несколько, а не что «нашлась одна система».
    return systems === 1
      ? `По запросу «${q}» нашлась система:`
      : `По запросу «${q}» у нас есть несколько системных решений (${systems}):`;
  }
  if (materials > 0) {
    return materials === 1
      ? `По запросу «${q}» нашёлся материал:`
      : `По запросу «${q}» нашлись материалы (${materials}):`;
  }
  return `По запросу «${q}» ничего не нашлось.`;
}

/** Строка списка: `N. Название (код) — материал`. */
export function formatSuggestLine(n: number, item: SuggestItem): string {
  const kind = item.kind === 'material' ? ' — материал' : '';
  const code = item.code ? ` (${item.code})` : '';
  return `${n}. ${item.title}${code}${kind}`;
}

/**
 * Собрать список найденного: сначала системы, потом материалы.
 *
 * Порядок не случаен: система — это то, ради чего собирают КП, материал —
 * её состав. Обрезка до `SUGGEST_LIMIT` идёт по этому же порядку, чтобы при
 * длинном списке первыми остались системы.
 */
export function suggestItems(
  systems: CombatSystemItem[],
  materials: SuggestMaterial[],
  limit = SUGGEST_LIMIT,
): SuggestItem[] {
  const items: SuggestItem[] = [];
  for (const s of systems) {
    if (items.length >= limit) return items;
    items.push({ kind: 'system', noteId: s.noteId, title: s.title, code: s.sku });
  }
  for (const m of materials) {
    if (items.length >= limit) return items;
    items.push({ kind: 'material', noteId: m.noteId, title: m.name, code: m.sku });
  }
  return items;
}

/**
 * Текст подсказки целиком.
 *
 * `items` пуст — не тупик: человеку объясняется, что искать и как, а не просто
 * «ничего нет». Именно этой фразы не хватало в КП008.
 */
export function suggestMessage(query: string, items: SuggestItem[]): string {
  const systems = items.filter((i) => i.kind === 'system').length;
  const materials = items.length - systems;
  if (items.length === 0) {
    return (
      `По запросу «${query.trim()}» в Базе знаний ничего не нашлось.\n\n` +
      'Попробуйте одно-два слова без лишних определений: «кровля», «мембрана», ' +
      '«фасад». Если знаете точное название — напишите «система <название>».'
    );
  }
  const lines = items.map((item, i) => formatSuggestLine(i + 1, item));
  const tail =
    systems > 0
      ? '\n\nНажмите номер системы, чтобы добавить её в КП. Материал добавляется позицией в таблицу.'
      : '\n\nНажмите номер материала, чтобы добавить его позицией в таблицу.';
  return `${suggestHeadline(query, systems, materials)}\n\n${lines.join('\n')}${tail}`;
}

/**
 * Клавиатура подсказки: номера по четыре в ряд (как в выборе стороны боевого
 * листка — на телефоне помещается без переноса).
 *
 * `shown` — это `foundSuggest`, сохранённое в сессии: номер в кнопке — индекс,
 * потому что `callback_data` ограничена 64 байтами, а кириллица в названии не
 * поместится.
 */
export function suggestKeyboard(
  items: SuggestItem[],
  menu: Array<Array<{ text: string; callback_data: string }>>,
): Record<string, unknown> {
  const rows: Array<Array<{ text: string; callback_data: string }>> = [];
  const numbers = items.map((_, i) => ({ text: `${i + 1}`, callback_data: `sug:${i}` }));
  for (let i = 0; i < numbers.length; i += 4) rows.push(numbers.slice(i, i + 4));
  if (items.length > 0) {
    rows.push([{ text: '✔ Добавить все системы', callback_data: 'sug:systems' }]);
  }
  rows.push([{ text: 'Ничего из этого — продолжить', callback_data: 'sug:skip' }]);
  rows.push(...menu);
  return { inline_keyboard: rows };
}

/** Реплика после добавления выбранного из подсказки. */
export function suggestPickedMessage(added: number, kind: 'system' | 'material'): string {
  if (added === 0) return 'Ничего не добавлено.';
  if (kind === 'system') {
    return added === 1
      ? 'Система добавлена в КП: описание, обоснование и ссылки попадут в документ; позиции не добавлялись.'
      : `Систем добавлено: ${added}. Описания и ссылки попадут в документ; позиции не добавлялись.`;
  }
  return added === 1
    ? 'Материал добавлен позицией — количество поправьте в таблице или файлом.'
    : `Материалов добавлено: ${added}. Количество поправьте в таблице или файлом.`;
}