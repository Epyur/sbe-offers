/**
 * Сборка предложения в боте — чистые функции без сети (стенд — flow.test.ts).
 * Поллер переводит нажатия и текст в вызовы этих функций, а сетевую часть
 * делает api.ts.
 */
import type { OfferDraft, OfferItem } from '../session.ts';

export interface CatalogCard {
  id: number;
  name: string;
  unit: string;
  /** Нижняя граница вилки цен; осмысленна только при `hasPrice`. */
  basePrice: number;
  /**
   * Была ли вилка вообще. Без признака ноль читался бы как «бесплатно», а на
   * деле значит «цену ещё не загружали таблицей» (2026-10-31).
   */
  hasPrice?: boolean;
}

/** Система в списке для выбора: код, название и проблемы состава. */
export interface SystemOption {
  sku: string;
  title: string;
  /** Проблемы состава; пусто, если сервер их не прислал. */
  problems?: string[];
}

/** Что ответил сервер, когда состав системы дописан в КП. */
export interface SystemAddResult {
  added: number;
  missingPrice?: string[];
  problems?: string[];
}

export function newDraft(): OfferDraft {
  return {
    items: [],
    clientName: '',
    clientContact: '',
    formValues: {},
    offerId: 0,
    step: 'catalog',
    pendingFields: [],
  };
}

/** Копейки без хвостов плавающей точки. */
export function roundMoney(v: number): number {
  const scaled = Math.round(v * 1000) / 10;
  return Math.round(scaled) / 100;
}

export function itemSum(item: OfferItem): number {
  return roundMoney(item.qty * item.price);
}

export function draftTotal(draft: OfferDraft): number {
  let total = 0;
  for (const item of draft.items) total += itemSum(item);
  return roundMoney(total);
}

/** Сумма для показа в чате: «1 234,56». */
export function formatMoney(v: number): string {
  const sign = v < 0 ? '-' : '';
  const abs = Math.abs(v);
  const cents = Math.round(abs * 100);
  const whole = Math.floor(cents / 100).toString();
  const frac = (cents % 100).toString().padStart(2, '0');
  const grouped = whole.replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
  return `${sign}${grouped},${frac}`;
}

/** Текст списка карточек каталога. Пустой каталог — честное сообщение. */
export function catalogMessage(items: CatalogCard[], query: string): string {
  if (items.length === 0) {
    if (query.trim()) {
      return `По запросу «${query.trim()}» ничего не нашлось. Попробуйте другое слово или /catalog — весь каталог.`;
    }
    return 'Каталог пуст — обратитесь к администратору, чтобы он добавил системы и материалы.';
  }
  const lines = items.map((it) => {
    // Цены нет — так и пишем. Ноль в чате читается как «бесплатно», и
    // менеджер уедет с этим в КП.
    const price = it.hasPrice === false ? 'цена не загружалась' : `${formatMoney(it.basePrice)}`;
    return `• ${it.name} — ${price} / ${it.unit}`;
  });
  return `Выберите позицию (нажмите кнопку ниже):\n\n${lines.join('\n')}`;
}

// --- Системы в боте (Task 8, 2026-10-31) -------------------------------
//
// В каталоге бота только материалы: система позицией не является, у неё нет
// цены. Но собрать КП «из системы» менеджеру нужно, поэтому есть команда
// «система <название>» — бот находит карточку в Базе знаний и просит сервер
// дописать её состав.

/** Нормализация для сравнения: регистр, ё и лишние пробелы. */
function normalizeSystemText(raw: string): string {
  return raw.toLowerCase().replace(/ё/g, 'е').replace(/\s+/g, ' ').trim();
}

/**
 * Разбор команды «система <название>».
 *
 * Возвращает название или null. Слово обязано стоять первым: иначе «Монтаж
 * системы вентиляции | 1 | 500» — свободная позиция, а не команда.
 */
export function parseSystemCommand(text: string): string | null {
  const match = text.trim().match(/^(?:система|\/system)\s+(.+)$/i);
  if (!match) return null;
  const query = match[1].trim();
  return query === '' ? null : query;
}

/**
 * Поиск систем по запросу.
 *
 * Сначала точное совпадение названия или кода — оно выигрывает у похожих.
 * Иначе по началу названия (менеджер пишет «кровля», а не полное имя) и по
 * вхождению кода. Порядок ответа сохраняется: так список читается сверху вниз.
 */
export function pickSystem(systems: SystemOption[], query: string): SystemOption[] {
  const q = normalizeSystemText(query);
  if (!q) return [];
  const exact = systems.filter(
    (s) => normalizeSystemText(s.title) === q || normalizeSystemText(s.sku) === q,
  );
  if (exact.length > 0) return exact;
  return systems.filter((s) => {
    const title = normalizeSystemText(s.title);
    const sku = normalizeSystemText(s.sku);
    return title.startsWith(q) || title.includes(q) || sku.includes(q);
  });
}

/**
 * Текст ответа после добавления системы.
 *
 * Число добавленных позиций, материалы без цены и несвязанные строки
 * называются всегда, если они есть: молча добавить неполный состав хуже, чем
 * сказать, чего не хватает.
 */
export function systemAddedMessage(result: SystemAddResult): string {
  const parts = [`Система добавлена: позиций — ${result.added}.`];
  if ((result.missingPrice ?? []).length > 0) {
    parts.push(
      `Без цены (загрузите таблицу ценой): ${(result.missingPrice ?? []).join(', ')}.`,
    );
  }
  if ((result.problems ?? []).length > 0) {
    parts.push(
      `В составе не нашлось материала: ${(result.problems ?? []).join('; ')}.`,
    );
  }
  return parts.join('\n');
}

export function addCatalogItem(draft: OfferDraft, card: CatalogCard, qty: number): OfferDraft {
  const items = [
    ...draft.items,
    {
      catalogId: card.id,
      name: card.name,
      unit: card.unit,
      qty,
      price: card.basePrice,
    },
  ];
  return { ...draft, items, step: 'catalog', pendingCatalog: undefined };
}

export function addManualItem(
  draft: OfferDraft,
  name: string,
  qty: number,
  price: number,
): OfferDraft {
  const items = [
    ...draft.items,
    { name: name.trim(), unit: 'шт', qty, price },
  ];
  return { ...draft, items, step: 'catalog' };
}

export function removeItem(draft: OfferDraft, index: number): OfferDraft {
  const items = draft.items.filter((_, i) => i !== index);
  return { ...draft, items };
}

export function draftSummary(draft: OfferDraft): string {
  if (draft.items.length === 0) {
    return 'Пока пусто. Добавьте позицию из каталога или впишите свою.';
  }
  const lines = draft.items.map((item, i) => {
    return `${i + 1}. ${item.name} — ${item.qty} ${item.unit} × ${formatMoney(item.price)} = ${formatMoney(itemSum(item))}`;
  });
  return `${lines.join('\n')}\n\nИтого: ${formatMoney(draftTotal(draft))} руб.`;
}

/** Разбор строки свободной позиции: «Название | кол-во | цена». */
export function parseManualItem(text: string): { name: string; qty: number; price: number } | null {
  const parts = text.split('|').map((p) => p.trim());
  if (parts.length !== 3) return null;
  const name = parts[0];
  const qty = Number(parts[1].replace(',', '.'));
  const price = Number(parts[2].replace(',', '.'));
  if (!name || !Number.isFinite(qty) || qty <= 0 || !Number.isFinite(price) || price < 0) return null;
  return { name, qty, price };
}

export const MAIN_KEYBOARD = {
  keyboard: [
    [{ text: '🧾 Собрать КП' }, { text: '📂 Мои КП' }],
    [{ text: '❓ Справка' }],
  ],
  resize_keyboard: true,
  is_persistent: true,
};

export const DRAFT_KEYBOARD = {
  inline_keyboard: [
    [{ text: '➕ Добавить из каталога', callback_data: 'cat' }],
    [{ text: '✏️ Добавить свою позицию', callback_data: 'manual' }],
    [{ text: '🗑 Убрать последнюю', callback_data: 'undo' }],
    [{ text: '✅ Готово', callback_data: 'done' }],
  ],
};

export const HELP_TEXT =
  'Коммерческие предложения.\n\n' +
  '🧾 Собрать КП — выбрать позиции из каталога или вписать свои, затем заполнить поля клиента. ' +
  'Готовое предложение приходит файлом Word прямо в чат.\n\n' +
  '📂 Мои КП — последние предложения и ссылки на них.\n\n' +
  'Добавить готовую систему целиком: система <название>.\n' +
  'Например: система кровля\n\n' +
  'Свою позицию вводите строкой: Название | количество | цена.\n' +
  'Например: Монтаж водостока | 12 | 850';

export const AUTH_PROMPT =
  'Здравствуйте! Я собираю коммерческие предложения.\n\n' +
  'Пришлите ваш рабочий e-mail — на него придёт код входа.';

// Ролей в боте нет (2026-10-29): любой, кто вошёл по коду, собирает КП.
// Раньше здесь была подпись роли в приветствии и отказ «ваша роль — просмотр»;
// и то и другое убрано вместе с самой ролью в сессии.
export const AUTH_OK = (email: string): string => `Вход выполнен: ${email}.\n\nЧем займёмся?`;
