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

export type Role = 'viewer' | 'editor' | 'admin';

export const ROLE_RANK: Record<string, number> = {
  '': 0,
  viewer: 1,
  editor: 2,
  admin: 3,
};
