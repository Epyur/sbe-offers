/**
 * Сборка предложения в боте — чистые функции без сети (стенд — flow.test.ts).
 * Поллер translates нажатия и текст в вызовы этих функций, а сетевую часть
 * делает api.ts.
 */
import type { OfferDraft, OfferItem } from './session.ts';

export interface CatalogCard {
  id: number;
  name: string;
  unit: string;
  basePrice: number;
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
  const lines = items.map((it) => `• ${it.name} — ${formatMoney(it.basePrice)} / ${it.unit}`);
  return `Выберите позицию (нажмите кнопку ниже):\n\n${lines.join('\n')}`;
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
  'Свою позицию вводите строкой: Название | количество | цена.\n' +
  'Например: Монтаж водостока | 12 | 850';

export const AUTH_PROMPT =
  'Здравствуйте! Я собираю коммерческие предложения.\n\n' +
  'Пришлите ваш рабочий e-mail — на него придёт код входа.';

export const AUTH_OK = (email: string, role: string): string => {
  const roleText =
    role === 'admin' ? 'администратор' : role === 'editor' ? 'менеджер' : role === 'viewer' ? 'просмотр' : role;
  return `Вход выполнен: ${email} (${roleText}).\n\nЧем займёмся?`;
};

export const NOT_ALLOWED_TO_EDIT =
  'Ваша роль — «просмотр»: предложения создают менеджеры. За доступом к сборке обратитесь к администратору.';