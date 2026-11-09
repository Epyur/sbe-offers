/**
 * Боевой листок в боте (2026-11-09) — чистые тексты и клавиатуры.
 *
 * Без сети: стенд проверяет только то, что увидит менеджер — какие кнопки и
 * какие строки. Сетевую часть делает api.ts, разбор нажатий — callbacks.ts.
 *
 * Боевой листок сравнивает наш продукт (системы) с продуктами конкурентов.
 * В боте стороны выбираются по названиям из готовых списков, по одной.
 */
import type { CombatSheet, CombatSystemItem } from '../api.ts';

/** Клавиатура внутри боевого листка: добавить стороны или забрать файл. */
export const COMBAT_KEYBOARD = {
  inline_keyboard: [
    [{ text: '➕ Наш продукт', callback_data: 'cb:oursys' }],
    [{ text: '🏷️ Добавить конкурента', callback_data: 'cb:comp' }],
    [{ text: '⬇ DOCX', callback_data: 'cb:doc' }],
    [{ text: '✅ Готово', callback_data: 'cb:done' }],
  ],
};

/** Приветствие и первый шаг. */
export function combatIntro(number: string): string {
  return (
    `Боевой листок № ${number}.\n\n` +
    'Это быстрая справка для спора с заказчиком: наш продукт и продукты конкурентов ' +
    'рядом, по одной карточке.\n\n' +
    'Добавьте наш продукт («➕ Наш продукт») и конкурентов («🏷️ Добавить конкурента») — ' +
    'можно несколько с каждой стороны: наберите название или часть слова, я покажу совпадения.\n\n' +
    'Когда сравнение готово — «⬇ DOCX».'
  );
}

/** Сводка по листку: что уже выбрано и чего ещё нет. */
export function combatSheetMessage(sheet: CombatSheet, hint?: string): string {
  const our = sheet.ourNotes.map((n) => `• ${n.title}`);
  const comp = sheet.competitorNotes.map((n) => `• ${n.title}`);
  const parts = [`Боевой листок № ${sheet.number}`];
  parts.push(our.length > 0 ? `Наш продукт:\n${our.join('\n')}` : 'Наш продукт: пока не выбран.');
  parts.push(
    comp.length > 0
      ? `Конкуренты:\n${comp.join('\n')}`
      : 'Конкуренты: пока не добавлены.',
  );
  if (hint) parts.push(hint);
  return parts.join('\n\n');
}

/**
 * Приглашение ввести запрос для поиска стороны (2026-11-09).
 *
 * Каталог нашего продукта — 1500+ записей, конкурентов — сотни: список
 * кнопками не поможет. Менеджер вводит название (или его часть, или код), бот
 * показывает совпадения. «▼ Все» — показать весь список, если искать неудобно.
 */
export function combatAskQuery(side: 'our' | 'competitor'): string {
  const who = side === 'our' ? 'нашего продукта' : 'продукта конкурента';
  return (
    `Введите название ${who} — можно часть слова или код.\n\n` +
    'Я покажу совпадения кнопками. Чтобы увидеть весь список, нажмите «▼ Все».'
  );
}

/** Клавиатура выбора стороны: `oursel:<индекс>` / `compsel:<индекс>`. */
export function combatPickerKeyboard(
  items: CombatSystemItem[],
  prefix: 'oursel' | 'compsel',
  showAll = false,
): Record<string, unknown> {
  const rows = items.map((item, index) => [
    {
      text: item.problems && item.problems.length > 0 ? `${item.title} (есть замечания)` : item.title,
      callback_data: `${prefix}:${index}`,
    },
  ]);
  // «▼ Все» показываем, только когда это осмысленно: при поиске — чтобы не
  // гадать, правильно ли набран запрос; при полном списке кнопка ни к чему.
  if (!showAll) rows.push([{ text: '▼ Все продукты', callback_data: 'cb:all' }]);
  rows.push([{ text: '< Назад', callback_data: 'cb:back' }]);
  return { inline_keyboard: rows };
}

/**
 * Текст перед списком выбора стороны; пустой список — честное объяснение.
 *
 * `query` — то, что искал менеджер: без него «ничего не нашлось» читается как
 * «продуктов нет вовсе».
 */
export function combatPickerMessage(
  items: CombatSystemItem[],
  side: 'our' | 'competitor',
  query = '',
): { text: string; show: boolean } {
  const q = query.trim();
  if (items.length === 0) {
    if (q) {
      return {
        text: `По запросу «${q}» ничего не нашлось. Попробуйте другое слово или нажмите «▼ Все продукты».`,
        show: false,
      };
    }
    const text =
      side === 'our'
        ? 'Наших продуктов в каталоге и системах не нашлось. Их заводит администратор — разделы «Материалы» и «Системы».'
        : 'Продуктов конкурентов нет: в Базе знаний не найдена папка «материалы и системы конкурентов» ' +
          'или она пуста. Добавьте конкурентов — раздел «Конкуренты» в плагине.';
    return { text, show: false };
  }
  const title = side === 'our' ? 'Наш продукт' : 'Продукт конкурента';
  const found = q ? `Нашлось по «${q}» — нажмите нужное:` : 'Выберите из списка:';
  return { text: `${title}. ${found}`, show: true };
}

/** Сообщение после добавления стороны. */
export function combatSideAddedMessage(side: 'our' | 'competitor', title: string): string {
  const who = side === 'our' ? 'Наша система' : 'Конкурент';
  return `${who} «${title}» добавлен${side === 'our' ? 'а' : ''} в листок.`;
}

/** Что бот говорит при выгрузке пустого листка — документ пустым не печатаем. */
export function combatNotEmptyMessage(): string {
  return 'В листке ещё нечего сравнивать. Добавьте хотя бы наш продукт и одного конкурента.';
}

/** Список моих листков: строки и кнопки `cbe:<id>` открыть и `cbx:<id>` файл. */
export function combatListMessage(sheets: CombatSheet[]): {
  text: string;
  keyboard: Record<string, unknown>;
} | null {
  if (sheets.length === 0) return null;
  const shown = sheets.slice(0, 15);
  const lines = shown.map((s) => `№ ${s.number} — ${s.title}`);
  const buttons = shown.map((s) => [
    { text: `✏️ ${s.number}`, callback_data: `cbe:${s.id}` },
    { text: `📄 файл`, callback_data: `cbx:${s.id}` },
  ]);
  return {
    text: `Последние боевые листки:\n\n${lines.join('\n')}`,
    keyboard: { inline_keyboard: buttons },
  };
}