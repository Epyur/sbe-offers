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
    // «Найти аналоги и сравнить» (2026-11-11) — в главном меню листка, а не
    // только в сводке: менеджер ищет этот пункт там, где начинает работу, и
    // раньше он видел его лишь после выбора конкурента в этой же сессии.
    [{ text: '🔍 Найти аналоги и сравнить', callback_data: 'cb:analog' }],
    // Уточнение для разбора — единственная настройка листка:
    // содержание всегда одно, а акцент менеджер задаёт сам.
    [{ text: '🎯 Уточнение для разбора', callback_data: 'cb:hint' }],
    [{ text: '⬇ DOCX', callback_data: 'cb:doc' }],
    [{ text: '✅ Готово', callback_data: 'cb:done' }],
  ],
};

/** Сколько строк списка выбора на одной странице. Как в режиме «Документы»:
 *  восемь строк читаются на телефоне без прокрутки, номера помещаются в ряд. */
export const COMBAT_PAGE_SIZE = 8;

/**
 * Клавиатура во время выбора стороны (2026-11-11).
 *
 * Раньше страница списка оставалась без основной клавиатуры боевого листка, и
 * после перелистывания или смены запроса кнопки «Наш продукт / Добавить
 * конкурента / DOCX / Готово» пропадали с экрана: вернуться к работе можно было
 * только «< Назад». Эти кнопки ведут в свои же обработчики и список не теряют
 * (`cb:oursys`/`cb:comp` начинают выбор заново, `cb:doc` берёт листок из
 * сессии, `cb:done` закрывает его).
 */
export const COMBAT_PICKER_KEYBOARD = {
  inline_keyboard: [
    [{ text: '➕ Наш продукт', callback_data: 'cb:oursys' }],
    [{ text: '🏷️ Добавить конкурента', callback_data: 'cb:comp' }],
    [{ text: '🔍 Найти аналоги и сравнить', callback_data: 'cb:analog' }],
    [{ text: '🎯 Уточнение для разбора', callback_data: 'cb:hint' }],
    [{ text: '⬇ DOCX', callback_data: 'cb:doc' }],
    [{ text: '✅ Готово', callback_data: 'cb:done' }],
  ],
};

/**
 * Кнопки-подсказка под страницей выбора стороны (2026-11-11).
 *
 * `inline_keyboard` в Telegram заменяет прежнюю клавиатуру этого сообщения, но
 * НЕ убирает её с экрана — у каждой кнопки своя клавиатура. Поэтому страницу
 * списка несут два ряда: навигация (номера, листание, добавить) и это меню,
 * чтобы после перелистывания человек не искал главные действия заново.
 */
export function withCombatMenu(rows: unknown[]): Record<string, unknown> {
  return { inline_keyboard: [...rows, ...COMBAT_PICKER_KEYBOARD.inline_keyboard] };
}

/**
 * Страница списка выбора стороны (2026-11-10).
 *
 * Повторяет приём режима «Документы» в Telegram-боте: по запросу выводится
 * ПРОНУМЕРОВАННЫЙ ТЕКСТ, ниже — кнопки-номера (не названия: длинные названия не
 * помещаются на экран и не читаются кнопкой). Нажатие номера переключает выбор,
 * «Дальше»/«В начало» листают, «✓ Добавить выбранные» добавляет всё отмеченное
 * разом.
 */
export interface CombatPickerPage {
  /** Строки текущей страницы (уже сдвинуты на offset). */
  items: CombatSystemItem[];
  /** Сколько всего совпадений по запросу. */
  total: number;
  /** Смещение: номер строки, с которого начинается страница. */
  offset: number;
  /** Есть ли ещё строки после этой страницы. */
  hasMore: boolean;
}

/** Приветствие и первый шаг. */
export function combatIntro(number: string): string {
  return (
    `Боевой листок № ${number}.\n\n` +
    'Это быстрая справка для спора с заказчиком: наш продукт и продукты конкурентов ' +
    'рядом, по одной карточке.\n\n' +
    'Добавьте наш продукт («➕ Наш продукт») и конкурентов («🏷️ Добавить конкурента») — ' +
    'можно несколько с каждой стороны: наберите название или часть слова, я покажу совпадения.\n\n' +
    'Кнопка «🔍 Найти аналоги и сравнить» подберёт наш аналог к продукту конкурента: ' +
    'сравнение пойдёт по делу, а не по случайному набору.\n\n' +
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
    'Я покажу совпадения списком: нажмите номер, чтобы отметить одну или несколько позиций, ' +
    'и «✓ Добавить выбранные». Чтобы увидеть весь список, нажмите «▼ Все».'
  );
}

/** Строка списка выбора: `N. Название (код) — замечания`. */
export function formatCombatLine(n: number, item: CombatSystemItem): string {
  const title = item.title || '(без названия)';
  const parts = [`${n}. ${title}`];
  if (item.sku) parts.push(`(${item.sku})`);
  const line = parts.join(' ');
  return item.problems && item.problems.length > 0 ? `${line} — есть замечания` : line;
}

/**
 * Текст страницы выбора стороны: пронумерованный список и подсказка, что делать
 * (2026-11-10). Пустой список — честное объяснение с учётом запроса.
 */
export function combatPickerMessage(page: CombatPickerPage, side: 'our' | 'competitor', query = ''): string {
  const q = query.trim();
  if (page.total === 0) {
    if (q) return `По запросу «${q}» ничего не нашлось. Попробуйте другое слово или нажмите «▼ Все продукты».`;
    return side === 'our'
      ? 'Наших продуктов в каталоге и системах не нашлось. Их заводит администратор — разделы «Материалы» и «Системы».'
      : 'Продуктов конкурентов нет: в Базе знаний не найдены карточки с тегом competitor и папка «материалы и системы конкурентов». ' +
          'Добавьте конкурентов — раздел «Конкуренты» в плагине.';
  }
  const title = side === 'our' ? 'Наш продукт' : 'Продукт конкурента';
  const head = q ? `${title}. Нашлось по «${q}»: ${page.total}` : `${title}. Всего: ${page.total}`;
  const lines = page.items.map((it, i) => formatCombatLine(page.offset + i + 1, it));
  return `${head}\n\n${lines.join('\n')}`;
}

/**
 * Клавиатура выбора стороны (2026-11-10): кнопки-номера текущей страницы,
 * перелистывание и добавление отмеченного.
 *
 * `picked` — индексы отмеченных строк ОТ НАЧАЛА списка: так «✓ Добавить
 * выбранные» добавляет их все разом, даже если отметили на разных страницах.
 * Номер кнопки — `page.offset + i + 1`, поэтому «Дальше» и «Назад» продолжают
 * нумерацию, а не начинают с единицы.
 *
 * `allPicked` — отмечено всё найденное по запросу (в том числе с других
 * страниц): тогда кнопка «выбрать всё» меняется на «снять все отметки», иначе
 * менеджер не видит, что набор уже взят целиком.
 */
export function combatPickerKeyboard(
  page: CombatPickerPage,
  prefix: 'oursel' | 'compsel',
  picked: ReadonlySet<number>,
  showAll = false,
  allPicked = false,
): Record<string, unknown> {
  const rows: Array<Array<{ text: string; callback_data: string }>> = [];
  const numbers = page.items.map((_, i) => {
    const index = page.offset + i;
    const mark = picked.has(index) ? '✓ ' : '';
    return { text: `${mark}${index + 1}`, callback_data: `${prefix}:${index}` };
  });
  // По четыре номера в ряд: на телефоне помещается без переноса.
  for (let i = 0; i < numbers.length; i += 4) rows.push(numbers.slice(i, i + 4));

  const nav: Array<{ text: string; callback_data: string }> = [];
  if (page.offset > 0) nav.push({ text: '◀ В начало', callback_data: 'cb:first' });
  if (page.hasMore) nav.push({ text: 'Дальше ▸', callback_data: 'cb:more' });
  if (nav.length > 0) rows.push(nav);

  // Всё найденное — одной кнопкой: сравнить «наш продукт против всех
  // аналогов» или взять весь каталог по поиску, не листая страницы.
  if (allPicked) {
    rows.push([{ text: '✖ Снять все отметки', callback_data: 'cb:unpickall' }]);
  } else if (page.total > page.items.length) {
    // Кнопка осмысленна, только когда найденное не поместилось на экран.
    rows.push([{ text: `☑ Выбрать все найденные (${page.total})`, callback_data: 'cb:pickall' }]);
  }
  // «Добавить» есть всегда, когда что-то отмечено: по номерам ИЛИ режимом
  // «всё найдено». Раньше кнопка показывалась только при непустом `picked`, а
  // «Выбрать все найденные» его очищает — кнопка исчезала, и добавить было
  // нечем (2026-11-11, живая жалоба «выбрал все — не добавились»).
  if (allPicked || picked.size > 0) {
    const count = allPicked ? page.total : picked.size;
    rows.push([{ text: `✓ Добавить выбранные (${count})`, callback_data: 'cb:addpicked' }]);
  }
  // «▼ Все» показываем, только когда это осмысленно: при поиске — чтобы не
  // гадать, правильно ли набран запрос; при полном списке кнопка ни к чему.
  if (!showAll) rows.push([{ text: '▼ Все продукты', callback_data: 'cb:all' }]);
  rows.push([{ text: '< Назад', callback_data: 'cb:back' }]);
  return { inline_keyboard: rows };
}

/**
 * Приглашение ввести уточнение для разбора (2026-11-11).
 *
 * У боевого листка нет формы: содержание всегда одно — продукты и их сравнение.
 * Менеджер задаёт только акцент: на чём ИИ должен сфокусироваться в разборе.
 * Пустое сообщение снимает уточнение.
 */
export function combatAskHint(current = ''): string {
  const head =
    'Уточнение для разбора.\n\n' +
    'Напишите, на чём сделать акцент в сравнении — например, «огнестойкость», ' +
    '«срок службы», «стоимость владения», «что спросить у проектировщика». ' +
    'Содержание листка не меняется: продукты и сравнение остаются прежними.';
  if (current.trim()) {
    return `${head}\n\nСейчас: «${current.trim()}».\nПришлите новый текст или точку — чтобы снять.`;
  }
  return `${head}\n\nЧтобы снять уточнение, пришлите точку.`;
}

/** Клавиатура приглашения: «Назад» и (если есть) «Снять». */
export function combatHintKeyboard(hasHint: boolean): Record<string, unknown> {
  const rows: Array<Array<{ text: string; callback_data: string }>> = [];
  if (hasHint) rows.push([{ text: '✖ Снять уточнение', callback_data: 'cb:hintclear' }]);
  rows.push([{ text: '< Назад', callback_data: 'cb:back' }]);
  return { inline_keyboard: rows };
}

/**
 * Кнопка «Подобрать аналоги» показана у продукта конкурента (2026-11-11).
 *
 * Нажатие подбирает наш аналог по Базе знаний и предлагает добавить его в
 * листок: сравнение «наш продукт против своего же аналога» полезнее, чем против
 * случайного конкурента из списка.
 */
/**
 * Просьба выбрать конкурента, если аналог искать ещё не под что.
 *
 * Аналог подбирается к продукту конкурента — без него искать нечего. Ведём
 * человека прямо к кнопке, а не оставляем в тупике.
 */
export function combatNeedCompetitorMessage(): string {
  return (
    'Аналоги подбираются к продукту конкурента.\n\n' +
    'Сначала нажмите «🏷️ Добавить конкурента» и выберите продукт — ' +
    'после этого «🔍 Найти аналоги и сравнить» найдёт наш аналог и предложит добавить его в листок.'
  );
}

export function combatAnalogueMessage(
  competitorTitle: string,
  analogues: CombatSystemItem[],
): { text: string; keyboard: Record<string, unknown> } {
  if (analogues.length === 0) {
    return {
      text:
        `По «${competitorTitle}» аналога в Базе знаний не нашлось.\n\n` +
        'Добавьте наш продукт руками: «➕ Наш продукт».',
      keyboard: { inline_keyboard: [[{ text: '< Назад', callback_data: 'cb:back' }]] },
    };
  }
  const lines = analogues.map((a, i) => `${i + 1}. ${a.title}${a.sku ? ` (${a.sku})` : ''}`);
  const rows = analogues.map((a, i) => [
    { text: `${i + 1}`, callback_data: `cban:${a.noteId}` },
  ]);
  rows.push([{ text: '< Назад', callback_data: 'cb:back' }]);
  return {
    text: `Наши аналоги по «${competitorTitle}»:\n\n${lines.join('\n')}\n\nНажмите номер, чтобы добавить.`,
    keyboard: { inline_keyboard: rows },
  };
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