/** Одно поле формы КП — то, что администратор заводит в конфигураторе. */
export interface Placeholder {
  id: string;
  label: string;
}

/**
 * Узел блока документа (2026-11-01).
 *
 * `paragraph` / `heading` / `bullet_list` / `table` — текст с подстановкой
 * `{{полей}}`. Новые виды:
 * - `page_break` — разрыв страницы в этом месте блока;
 * - `links` — ссылки из карточек систем и материалов, печатаются в конце КП;
 * - `prompt_text` — текст пишет модель по подсказке администратора;
 * - `image` — картинка с обтеканием текстом (не разрывает абзацы).
 */
export interface RichNode {
  type:
    | 'paragraph'
    | 'heading'
    | 'bullet_list'
    | 'table'
    | 'page_break'
    | 'links'
    | 'prompt_text'
    | 'image'
    /** Таблица сравнения выбранных систем (2026-11-05): строки — показатели
     *  из характеристик карточек, столбцы — системы. Молчит при одной системе. */
    | 'compare_table';
  level?: 2 | 3 | 4;
  align?: 'center' | 'right' | 'justify' | 'left';
  text?: string;
  items?: string[];
  rows?: string[][];
  /** Подсказка администратора для `prompt_text`: о чём писать. */
  instruction?: string;
  /** Необязательное уточнение к подсказке. */
  hint?: string;
  /** Предел длины текста по промту; 0 — значение по умолчанию на сервере. */
  maxWords?: number;
  /** Ключ картинки в хранилище Базы знаний (узел `image`). */
  imageKey?: string;
  imageName?: string;
  /**
   * Откуда брать картинку (2026-11-05). `system` — выбранная система КП,
   * `material` — материал из позиций. Форма общая, поэтому в ней хранится
   * источник, а конкретную карточку даёт само предложение.
   */
  imageOf?: 'system' | 'material';
  /** Номер выбранной системы (1 — первая в порядке выбора). */
  systemIndex?: number;
  /** Номер позиции предложения (1 — первая). */
  itemIndex?: number;
  /** Ширина в сантиметрах; 0 — серверное значение по умолчанию (8). */
  widthCm?: number;
  /** Сторона обтекания: текст потечёт с противоположной. */
  wrap?: 'both' | 'left' | 'right' | 'topAndBottom' | 'none';
  caption?: string;
}

/** Один блок документа формы КП. */
export interface Block {
  id: string;
  title: string;
  content: RichNode[];
  showInClient: boolean;
  showInInternal: boolean;
  /** Отступ до блока в пунктах (0..200): размещение на странице. */
  spacingBefore?: number;
  /** Отступ после блока в пунктах (0..200). */
  spacingAfter?: number;
  /** Не разрывать абзацы блока между страницами. */
  keepTogether?: boolean;
  /** Начинать блок с новой страницы. */
  pageBreakBefore?: boolean;
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
  /** Загружен ли макет .docx (2026-11-05): печать идёт по нему, а не по блокам. */
  hasLayout?: boolean;
  layoutName?: string;
  /** Какие метки `[…]`/`{…}` система нашла в макете. */
  layoutFields?: string[];
  layoutPrompts?: string[];
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
  /**
   * Номер ЕКН на момент добавления (2026-11-02). Печатается колонкой в
   * таблице состава: по нему менеджер и заказчик сверяют позиции.
   */
  ekn?: string;
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
  /**
   * Выбранные системы в порядке выбора (2026-11-05). Пусто — ни одной.
   * Сервер отдаёт массив; старые КП с одной системой читаются так же.
   */
  systems?: OfferSystem[];
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
 * Ссылка из карточки Базы знаний.
 *
 * `page` — страница материала или системы на корпоративном сайте, `doc` —
 * технический документ (техлист, узлы, инструкция) на навигаторе.
 */
export interface CardRef {
  label: string;
  url: string;
  kind: 'page' | 'doc';
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
  /**
   * Ссылки из раздела «## Ссылки» карточки (2026-11-01): страница на
   * корпоративном сайте и техническая документация на навигаторе.
   */
  refs?: CardRef[];
  imageName?: string;
  /** 0 — карточка не связана с каталогом. */
  catalogId: number;
  updatedAt: string;
  hasDescription: boolean;
  hasComposition: boolean;
  empty: boolean;
}

/**
 * Материал — то, что продаётся (2026-10-31).
 *
 * Отдельная сущность от системы: у него собственный SKU, который назначает
 * только сервер, и вариации ЕКН. **Цены здесь нет намеренно**: цена одного и
 * того же материала зависит от условий поставки и приходит из таблицы
 * менеджера для конкретного КП. Вилка `priceMin`/`priceMax` — то, что видели
 * в загруженных таблицах, и только для аналитики.
 */
export interface Material {
  id: number;
  sku: string;
  kind: 'material' | 'work';
  name: string;
  unit: string;
  description: string;
  /** null — цены ещё не видели. Ноль значит «бесплатно», это разные вещи. */
  priceMin?: number | null;
  priceMax?: number | null;
  priceSeenAt?: string;
  /**
   * Заметка Базы знаний, из которой материал заведён (2026-10-31). По этому
   * заголовку состав системы находит материал по вики-ссылке `[[Название]]`.
   * Пусто у материала, заведённого руками в плагине.
   */
  noteTitle?: string;
  noteId?: number;
  photoKey: string;
  sortOrder: number;
  archived: boolean;
  createdAt: string;
  updatedAt: string;
  ekn: MaterialEkn[];
}

/** Одна вариация материала: номер ЕКН и как она называется. */
export interface MaterialEkn {
  id: number;
  materialId: number;
  ekn: string;
  variant: string;
  source: 'excel' | 'manual';
  firstSeenAt: string;
  lastSeenAt: string;
}

/**
 * Строка состава системы, разрешённая против справочника материалов.
 *
 * `found: false` — строка осталась видимой, но материала в справочнике нет:
 * без кода, с незнакомым кодом или с кодом удалённого материала.
 *
 * `layer: true` — это вообще не материал, а слой или работа («Подготовка
 * основания»). Материала у неё нет по смыслу, поэтому такие строки не попадают
 * в `problems`: иначе «есть ошибки в составе» появлялось бы у каждой системы.
 */
export interface CompositionItem {
  raw: string;
  sku?: string;
  wiki?: string;
  materialId: number;
  name?: string;
  unit?: string;
  ekn?: string[];
  found: boolean;
  layer?: boolean;
}

/** Система из Базы знаний с составом, разрешённым против материалов. */
export interface SystemWithComposition extends SystemCard {
  compositionLinks?: CompositionItem[];
  problems?: string[];
}

/** Выбранная система предложения (2026-11-03). */
export interface OfferSystem {
  sku?: string;
  noteId?: number;
}

/** Ответ выбора системы: какая система выбрана и что о ней известно. */
export interface ComposeSystemResponse {
  offer: Offer;
  /** Карточка выбранной системы: её текст печатается в КП. */
  system: SystemCard;
  /**
   * Сколько основных продуктов в составе системы. Позиций в КП это не
   * прибавляет — число нужно, чтобы показать, о какой системе речь.
   */
  composition: number;
}

/** Ответ раздела «Системы». */
export interface SystemsResponse {
  systems: SystemWithComposition[];
  kbEnabled: boolean;
  count: number;
  /** Ошибка Базы знаний; список при этом пустой. */
  error?: string;
}

/**
 * Строка таблицы менеджера, прочитанная сервером.
 *
 * `has*` говорят, была ли колонка в файле: ноль цены и отсутствие колонки —
 * разные вещи, и показывать их одинаково значит соврать человеку.
 */
export interface SheetRow {
  line: number;
  name: string;
  ekn: string;
  unit: string;
  qty: number;
  price: number;
  sum: number;
  hasQty: boolean;
  hasPrice: boolean;
  hasSum: boolean;
  missingName?: boolean;
}

/** Чем сопоставлена строка таблицы с материалом. */
export type MatchMethod = 'ekn' | 'name' | 'ai' | 'none';

/** Решение по одной строке таблицы. */
export interface MatchResult {
  row: SheetRow;
  materialId: number;
  sku: string;
  name: string;
  /** Чем строка найдена: `ekn` — по номеру, `name` — по названию (2026-11-02). */
  method: MatchMethod;
  reason?: string;
  /** Материал предложен моделью — применять только после подтверждения. */
  needsConfirm: boolean;
}

/** Сводка превью: сколько строк сопоставлено каждым способом. */
export interface SheetCounts {
  total: number;
  byEkn: number;
  byName: number;
  byAi: number;
  none: number;
}

/**
 * «Что я понял из таблицы» — ответ первого шага загрузки. Ничего не
 * записано: применение отдельным шагом после проверки человеком.
 */
export interface SheetPreview {
  fileName: string;
  rows: MatchResult[];
  counts: SheetCounts;
  problems: string[];
}

/** Строка, которую менеджер подтвердил к применению. */
export interface SheetApplyRow {
  line: number;
  materialId: number;
  ekn: string;
  variant: string;
  qty: number;
  price: number;
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

/** Возражение клиента и согласованный ответ на него. */
export interface BriefObjection {
  claim: string;
  response: string;
}

/**
 * Бриф КП — то, что бот выяснил у менеджера про объект.
 *
 * Поля повторяют `model.BriefPayload` на сервере один в один: это контракт
 * JSON, а не удобный для UI вид. Пустое значение значит «не спрашивали» —
 * показывать его надо как «не заполнено», а не как пустую строку.
 */
export interface BriefPayload {
  object_name: string;
  /** Назначение объекта (жилой дом, склад, паркинг…), с 2026-11-01. */
  object_purpose: string;
  /** Площадь или объём работ. */
  object_area: string;
  /** Адресат КП: организация и лицо, кому его посылают. */
  customer_address: string;
  /** Какие системы менеджер собирается предложить. */
  planned_systems: string[];
  customer_pains: string[];
  customer_goals: string[];
  project_risks: string[];
  objections: BriefObjection[];
  budget_signal: string;
  decision_maker: string;
  timeline: string;
  why_us: string[];
  notes: string;
  skipped_topics: string[];
}

/** Бриф в хранилище: `absent` — брифа нет (КП собрано без диалога в боте). */
export interface Brief {
  id: number;
  offerId: number;
  payload: BriefPayload;
  status: 'open' | 'ready' | 'agreed' | 'absent';
  chatId: string;
  lastTopic: string;
  updatedAt: string;
}

/** Одна реплика разговора в боте. */
export interface BriefTurn {
  id: number;
  briefId: number;
  role: 'bot' | 'manager';
  text: string;
  topic: string;
  createdAt: string;
}

/** Ответ сервера: бриф вместе с журналом реплик. */
export interface BriefResponse {
  brief: Brief;
  turns: BriefTurn[];
}

/** Пустой бриф для нового КП — те же ключи, что у сервера. */
export function emptyBriefPayload(): BriefPayload {
  return {
    object_name: '',
    object_purpose: '',
    object_area: '',
    customer_address: '',
    planned_systems: [],
    customer_pains: [],
    customer_goals: [],
    project_risks: [],
    objections: [],
    budget_signal: '',
    decision_maker: '',
    timeline: '',
    why_us: [],
    notes: '',
    skipped_topics: [],
  };
}