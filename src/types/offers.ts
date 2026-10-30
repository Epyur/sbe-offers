/** Одно поле формы КП — то, что администратор заводит в конфигураторе. */
export interface Placeholder {
  id: string;
  label: string;
}

/** Узел блока документа: абзац, заголовок, список или таблица. */
export interface RichNode {
  type: 'paragraph' | 'heading' | 'bullet_list' | 'table';
  level?: 2 | 3 | 4;
  align?: 'center' | 'right' | 'justify';
  text?: string;
  items?: string[];
  rows?: string[][];
}

/** Один блок документа формы КП. */
export interface Block {
  id: string;
  title: string;
  content: RichNode[];
  showInClient: boolean;
  showInInternal: boolean;
}

/** Форма КП — конфигуратор администратора. */
export interface OfferForm {
  id: number;
  mode: string;
  title: string;
  placeholders: Placeholder[];
  blocks: Block[];
  requiredFields: string[];
  updatedAt: string;
}

/** Карточка системы/материала/работы в каталоге. */
export interface CatalogItem {
  id: number;
  kind: 'system' | 'material' | 'work';
  name: string;
  unit: string;
  basePrice: number;
  description: string;
  sortOrder: number;
  archived: boolean;
  createdAt: string;
  updatedAt: string;
}

/** Позиция КП — снимок на момент добавления. */
export interface OfferItem {
  catalogId?: number;
  name: string;
  unit: string;
  qty: number;
  price: number;
  sum: number;
}

/** Коммерческое предложение. */
export interface Offer {
  id: number;
  number: string;
  ownerEmail: string;
  client: { name?: string; contact?: string };
  items: OfferItem[];
  formValues: Record<string, string>;
  total: number;
  status: 'draft' | 'final';
  docxKey: string;
  createdAt: string;
  updatedAt: string;
}

/** Запись о человеке, вошедшем в бота. */
export interface AccessEntry {
  email: string;
  role: 'viewer' | 'editor' | 'admin' | '';
  createdAt: string;
  updatedAt: string;
}

export type Role = 'viewer' | 'editor' | 'admin' | 'superadmin';

/**
 * Настройки Telegram-бота. Токена в ответе сервера нет и не будет — только
 * признак «задан», чей это бот и кто его менял (2026-10-29).
 */
export interface BotConfigInfo {
  configured: boolean;
  botUsername: string;
  updatedBy: string;
  updatedAt: string;
}

/** Живое состояние бота для строки статуса. */
export interface BotStatusInfo {
  configured: boolean;
  running: boolean;
  botUsername: string;
  lastError: string;
  checkedAt: string;
  updatedBy: string;
  updatedAt: string;
}

export const ROLE_RANK: Record<string, number> = {
  '': 0,
  viewer: 1,
  editor: 2,
  admin: 3,
  // Глобальный админ экосистемы (роль центральной таблицы auth-service) —
  // выше админа приложения. Без него административные разделы не показывались
  // владельцу приложения.
  superadmin: 4,
};

/** Доступен ли административный уровень (админ приложения или глобальный). */
export function isAdminRole(role: string): boolean {
  return role === 'admin' || role === 'superadmin';
}

/** Может ли роль собирать предложения. */
export function canEditOffers(role: string): boolean {
  return role === 'editor' || role === 'admin' || role === 'superadmin';
}

/**
 * Карточка системы из Базы знаний (2026-10-29).
 *
 * Источник текста для КП: название, описание, состав, картинка. Цены и
 * артикулы здесь НЕ хранятся — они в каталоге. Связь с каталогом по `sku`.
 */
export interface SystemCard {
  noteId: number;
  path: string;
  sku: string;
  title: string;
  description: string;
  composition: string[];
  specifications?: string[][];
  imageName?: string;
  /** 0 — карточка не связана с каталогом. */
  catalogId: number;
  updatedAt: string;
  hasDescription: boolean;
  hasComposition: boolean;
  empty: boolean;
}

/** Ответ списка карточек: вместе с ними — настроена ли База знаний. */
export interface SystemCardsResponse {
  cards: SystemCard[];
  kbEnabled: boolean;
}

/** Ответ перечитывания карточек: если база знаний упала, придёт `error`. */
export interface RefreshSystemCardsResponse extends SystemCardsResponse {
  count: number;
  error?: string;
}