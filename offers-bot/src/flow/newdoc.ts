/**
 * «Создать новый документ» (2026-11-13, R2).
 *
 * Одна кнопка вместо двух: бот сначала спрашивает, что собрать — коммерческое
 * предложение или боевой листок. Дальше, для КП, — первый вопрос, который
 * менеджер всё равно решает первым: **какой продукт или систему предложить**.
 * Название ищется в Базе знаний, найденное показывается списком с номерами, и
 * выбор ЗАПОМИНАЕТСЯ. После него — развилка: собрать контекст в чате (бриф) или
 * продолжить в приложении.
 *
 * Почему это отдельный модуль, а не ветка в `callbacks.ts`: разработка R2 —
 * это состояние разговора (что спросили, что нашли, что отметили), и его
 * проверяют без сети. Сеть — `callbacks.ts`/`routes.ts`, тексты и клавиатуры —
 * здесь, чистые.
 *
 * Чего здесь НЕТ и почему:
 * - своей сборки запроса к поиску — используется та же `suggestItems`, что и в
 *   ответе на свободную фразу (правило «один поиск на бота»);
 * - своего списка найденного — состояние живёт в `RouterState.newDoc`, а не в
 *   `foundSuggest`: два потока в одной карте дают «нажал — не отреагировало»
 *   (это уже случалось с листком, 2026-11-11).
 */
import type { SuggestItem } from './suggest.ts';
import { appListButton } from './appbutton.ts';

/** Что создаём. */
export type NewDocKind = 'offer' | 'combat';

/** Шаг разговора. */
export type NewDocStep =
  /** Ждём, какой документ: ещё не выбрано. */
  | 'kind'
  /** Ждём название продукта (текст). */
  | 'query'
  /** Показан список найденного, ждём номера-отметки или решение. */
  | 'pick'
  /** Продукт(ы) отмечены и записаны: спрашиваем, нужно ли ещё (2026-11-14). */
  | 'more';

/**
 * Состояние разговора «создать новый документ».
 *
 * Живёт в памяти поллера (как остальные карты), кроме `kind`: его сохраняем в
 * сессию, чтобы после перезапуска бот не забывал, что собирает листок, а не КП.
 */
export interface NewDocState {
  step: NewDocStep;
  kind: NewDocKind;
  /** Что искали — нужно, чтобы назвать запрос при нажатии номера. */
  query: string;
  /** Что нашлось; номер в кнопке — индекс этого списка. */
  items: SuggestItem[];
  /** Отмеченные номера (для отметки нескольких перед «Дальше»). */
  picked: number[];
  /** Уже записанные карточки в порядке выбора: по ним строится сводка. */
  added?: NewDocAdded[];
}

/** Записанная в документ карточка: так человек видит, что уже легло в КП. */
export interface NewDocAdded {
  kind: 'system' | 'material';
  title: string;
}

export function newDocState(): NewDocState {
  return { step: 'kind', kind: 'offer', query: '', items: [], picked: [] };
}

/** Вопрос ровно из заявки пользователя — формулировку не менять. */
export const NEW_DOC_KIND_QUESTION =
  'Что создаём? Выберите ниже: коммерческое предложение или боевой листок.';

/** Первый вопрос для КП — дословно из заявки. */
export const NEW_DOC_PRODUCT_QUESTION =
  'Какой продукт или систему вы хотите предложить покупателю?\n\n' +
  'Напишите название (можно часть слова) — найду в Базе знаний и покажу варианты. ' +
  'Если ничего не найдётся, можно будет уточнить название или выбрать продукт прямо в приложении.';

/** Для боевого листка: спрашиваем наш продукт, а не «предложение». */
export const NEW_DOC_COMBAT_QUESTION =
  'Какой наш продукт будет в боевом листке?\n\n' +
  'Напишите название (можно часть слова) — найду в Базе знаний и покажу варианты. ' +
  'Сравнивать будем с продуктами конкурентов.';

/** Какая кнопка какого вида документа. */
export const KIND_KEYBOARD = {
  inline_keyboard: [
    [{ text: '🧾 Коммерческое предложение', callback_data: 'nd:kind:offer' }],
    [{ text: '⚔️ Боевой листок', callback_data: 'nd:kind:combat' }],
  ],
};

/** Подпись кнопки «собрать контекст» — разная для двух документов. */
export function contextButtonText(kind: NewDocKind): string {
  return kind === 'combat'
    ? '✍️ Уточнить для разбора в чате'
    : '📚 Собрать контекст в чате';
}

/** Развилка после выбора продукта: контекст или приложение. */
export function decideKeyboard(kind: NewDocKind): Record<string, unknown> {
  const appText = kind === 'combat' ? '📱 Открыть листок в приложении' : '📱 Продолжить в приложении';
  return {
    inline_keyboard: [
      [{ text: contextButtonText(kind), callback_data: 'nd:context' }],
      [{ text: appText, callback_data: 'nd:app' }],
    ],
  };
}

/**
 * Текст вопроса по виду документа.
 *
 * Одна функция на оба вида: если вопрос будет продублирован в двух местах,
 * они разойдутся на первой правке.
 */
export function productQuestion(kind: NewDocKind): string {
  return kind === 'combat' ? NEW_DOC_COMBAT_QUESTION : NEW_DOC_PRODUCT_QUESTION;
}

/** Порог списка: больше не влезает в одну экранную клавиатуру. */
export const NEW_DOC_LIMIT = 8;

/**
 * Что показать, когда по названию ничего не нашлось.
 *
 * Это НЕ тупик — требование заявки: «либо предлагает уточнить название, либо
 * позже выбрать продукт в миниапе». Поэтому «уточнить» рядом с «в приложении»
 * (кнопка приложения добавляется вызывающей стороной через `withAppButton`).
 */
export function retryKeyboard(): Array<Array<{ text: string; callback_data: string }>> {
  return [[{ text: '🔎 Уточнить название', callback_data: 'nd:again' }]];
}

/** Кнопка «уточнить» + «в приложении» — без дублей сборки URL. */
export function retryOrAppKeyboard(kind: NewDocKind): Record<string, unknown> {
  return { inline_keyboard: [...retryKeyboard(), ...decideAppRow(kind)] };
}

/** Одна строка «открыть в приложении» — сборка адреса живёт в appbutton.ts. */
function decideAppRow(kind: NewDocKind): Array<Array<{ text: string; web_app: { url: string } }>> {
  const text = kind === 'combat' ? '📱 Открыть листки в приложении' : '📱 Продолжить в приложении';
  return [[appListButton(text, kind === 'combat' ? 'sheets' : 'offers')]];
}

export function notFoundMessage(query: string): string {
  return (
    `По запросу «${query.trim()}» в Базе знаний ничего не нашлось.\n\n` +
    'Так бывает, если название написано иначе: попробуйте одно-два слова без ' +
    'определений («кровля», «мембрана», «фасад») или марку продукта. ' +
    'А можно выбрать продукт прямо в приложении — кнопка ниже остаётся.'
  );
}

/** Текст над найденным списком. */
export function foundMessage(query: string, items: SuggestItem[]): string {
  const systems = items.filter((i) => i.kind === 'system').length;
  const materials = items.length - systems;
  const head =
    systems > 0 && materials > 0
      ? `По запросу «${query.trim()}» нашлись системы (${systems}) и материалы (${materials}):`
      : systems > 0
        ? `По запросу «${query.trim()}» нашлись системы (${systems}):`
        : `По запросу «${query.trim()}» нашлись материалы (${materials}):`;
  return `${head}\n\nВыберите номер — запомню выбор и предложу собрать контекст или продолжить в приложении.`;
}

/** Строка найденного: `N. Название (код) — материал`. */
export function itemLine(n: number, item: SuggestItem): string {
  const code = item.code ? ` (${item.code})` : '';
  const kind = item.kind === 'material' ? ' — материал' : '';
  return `${n}. ${item.title}${code}${kind}`;
}

/** Текст списка найденного целиком. */
export function foundListMessage(query: string, items: SuggestItem[]): string {
  return `${foundMessage(query, items)}\n\n${items.map((it, i) => itemLine(i + 1, it)).join('\n')}`;
}

/**
 * Клавиатура найденного: номера-отметки по четыре в ряд, «добавить выбранные»,
 * «уточнить» и «в приложении».
 *
 * Номер — переключатель (2026-11-14): человека просили «выбрать ТН-Кровля,
 * потом ТН-Перекрытие, ТН-Фундамент», а кнопка-номер запоминала ровно один
 * выбор. Отмеченные несут галочку, и одна кнопка записывает их все.
 *
 * «В приложении» стоит здесь же, а не только после выбора: заявка прямо
 * разрешает «позже выбрать продукт в миниапе», и возвращаться к поиску ради
 * этого было бы лишним шагом.
 */
export function foundKeyboard(
  items: SuggestItem[],
  kind: NewDocKind,
  picked: number[] = [],
): Record<string, unknown> {
  const rows: Array<Array<{ text: string; callback_data: string }>> = [];
  const chosen = new Set(picked);
  const numbers = items.map((item, i) => ({
    text: chosen.has(i) ? `✓ ${i + 1}` : `${i + 1}`,
    callback_data: `nd:pick:${i}`,
  }));
  for (let i = 0; i < numbers.length; i += 4) rows.push(numbers.slice(i, i + 4));
  if (chosen.size > 0) {
    rows.push([{ text: `✓ Добавить выбранные (${chosen.size})`, callback_data: 'nd:addpicked' }]);
  }
  rows.push([{ text: '🔎 Ничего из этого — уточню название', callback_data: 'nd:again' }]);
  rows.push([{ text: '📱 Выбрать в приложении', callback_data: 'nd:app' }]);
  return { inline_keyboard: rows };
}

/** Подпись выбранного продукта для сводки. */
export function pickedLabel(item: SuggestItem): string {
  const kind = item.kind === 'material' ? 'материал' : 'система';
  return `${item.title} (${kind})`;
}

/**
 * Что сказать после того, как отмеченное записано (2026-11-14).
 *
 * Требование пользователя дословно: после выбора продукта спросить, нужно ли
 * добавить ещё или этого достаточно — человек собирает КП из нескольких систем
 * (ТН-Кровля, затем ТН-Перекрытие, ТН-Фундамент). Поэтому сводка перечисляет
 * уже записанное, а решение — кнопками, а не свободным текстом.
 */
export function addedMessage(kind: NewDocKind, added: NewDocAdded[]): string {
  const systems = added.filter((a) => a.kind === 'system');
  const materials = added.filter((a) => a.kind === 'material');
  const lines: string[] = [];
  if (systems.length > 0) {
    lines.push(`Системы: ${systems.map((a) => `«${a.title}»`).join(', ')}.`);
  }
  if (materials.length > 0) {
    lines.push(`Материалы (описанием): ${materials.map((a) => `«${a.title}»`).join(', ')}.`);
  }
  const what = kind === 'combat' ? 'листок' : 'предложение';
  return (
    `Записано в ${what}:\n${lines.join('\n')}\n\n` +
    'Нужно добавить ещё продукт или систему — или этого достаточно?'
  );
}

/** Развилка после записи: добавить ещё или продолжить. */
export function moreKeyboard(kind: NewDocKind): Record<string, unknown> {
  const done = kind === 'combat' ? '✅ Достаточно — листок собран' : '✅ Достаточно — собрать контекст';
  const appText = kind === 'combat' ? '📱 Открыть листок в приложении' : '📱 Продолжить в приложении';
  return {
    inline_keyboard: [
      [{ text: '➕ Добавить ещё продукт или систему', callback_data: 'nd:more' }],
      [{ text: done, callback_data: 'nd:enough' }],
      [{ text: appText, callback_data: 'nd:app' }],
    ],
  };
}

/** Вопрос при возврате к поиску: отметки уже записанного не теряются. */
export function moreQuestion(kind: NewDocKind): string {
  return kind === 'combat'
    ? 'Какой продукт добавить в листок ещё?\n\nНапишите название (можно часть слова) — найду в Базе знаний.'
    : 'Какой продукт или систему добавить в предложение ещё?\n\n' +
        'Напишите название (можно часть слова) — найду в Базе знаний.';
}

/**
 * «⌛» на время записи продукта в документ.
 *
 * Модель пишет описание системы 20-40 секунд, и без этого сообщения человек
 * видит только «нет ответа» и жмёт кнопку второй раз (живая жалоба
 * 2026-11-14). Текст заменяется результатом — чат не копится служебными
 * строками.
 */
export function writingNoteText(item: SuggestItem): string {
  const what = item.kind === 'material' ? 'материал' : 'систему';
  const title = item.title.length > 80 ? `${item.title.slice(0, 77)}…` : item.title;
  return (
    `⌛ Записываю ${what} «${title}» в документ.\n\n` +
    'Это занимает до минуты — бот пишет описание и преимущества. ' +
    'Повторно нажимать не нужно: сообщение сменится результатом.'
  );
}

/**
 * «Такой продукт уже записан»: нажатие пришло на шаг, который уже отработал.
 *
 * Не ошибка человека — двойной клик по кнопке, пока шла запись. Он не должен
 * ни записывать второй раз, ни пугать красным текстом.
 */
export function alreadyAddedText(added: NewDocAdded[], kind: NewDocKind): string {
  const what = kind === 'combat' ? 'листок' : 'предложение';
  const names = added.map((a) => `«${a.title}»`).join(', ');
  return (
    `В ${what} уже записано: ${names}.\n\n` +
    'Выберите, что делать дальше: добавить ещё продукт или продолжить.'
  );
}

/** Что показать, когда отметок нет: «Добавить выбранные» без отмеченного. */
export function nothingPickedText(): string {
  return 'Ничего не отмечено — нажмите номер продукта, и он подсветится галочкой.';
}

/** Подсказка под списком: сколько отмечено и что дальше. */
export function pickedHint(count: number): string {
  return count === 1
    ? 'Отмечено: 1. Добавить этот продукт в документ?'
    : `Отмечено: ${count}. Добавить все отмеченные в документ?`;
}

/**
 * Совместимость номера кнопки с показанным списком.
 *
 * Если список перезаписали (человек написал новый запрос), номер относится к
 * СТАРОМУ набору — молча добавлять из нового нельзя, это «нажал не то».
 */
export function pickIndex(state: NewDocState, index: number): SuggestItem | null {
  if (!Number.isInteger(index) || index < 0 || index >= state.items.length) return null;
  return state.items[index];
}