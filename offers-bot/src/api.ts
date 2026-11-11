/** Клиент offers-service для бота: только те ручки, что защищены X-Service-Secret. */

export const API_URL = (process.env.OFFERS_API_URL ?? 'http://offers:3000').replace(/\/+$/, '');
const BOT_SECRET = process.env.OFFERS_BOT_SECRET ?? '';
const TIMEOUT_MS = 60000;
/**
 * Сборка документа и выбор системы ждут модель (2026-11-06).
 *
 * Было одно ожидание на все ручки — 60 секунд, и оно было меньше, чем модели
 * разрешено на ответ (180 с на попытку). Длинный текст не мог дойти до
 * человека вовсе: бот бросал запрос, и в чате не появлялось ни файла, ни
 * ошибки. Предел сервера и модели теперь больше, и ждать надо столько же.
 */
const LONG_TIMEOUT_MS = 300000;

export interface CatalogItem {
  id: number;
  kind: string;
  name: string;
  unit: string;
  /** Нижняя граница вилки цен; осмысленна только при `hasPrice`. */
  basePrice: number;
  /** Была ли вилка вообще: ноль без признака значит «цену не загружали». */
  hasPrice?: boolean;
  /** Код материала: по нему менеджер ищет то, что видит на бумаге. */
  sku?: string;
  description?: string;
}

/** Система для выбора в боте: без цены, только код, название и состав. */
export interface SystemItem {
  sku: string;
  /** Заметка Базы знаний: у живых карточек кода нет, выбор идёт по ней. */
  noteId?: number;
  title: string;
  problems?: string[];
}

/**
 * Чем выбирается система: кодом или заметкой.
 *
 * Живые карточки Базы знаний кода не несут вовсе (2026-11-02), и выбор по
 * одному `sku` для них был пустым — бот отвечал «система не найдена».
 */
export interface SystemChoice {
  sku?: string;
  noteId?: number;
}

/** Ответ выбора системы (2026-11-03; тексты — 2026-11-06). */
export interface SystemAddResponse {
  /** Название выбранной системы — его бот показывает в ответе. */
  title: string;
  /**
   * Сколько основных продуктов в составе системы. Позиций в КП это не
   * прибавляет: состав с ценами менеджер заводит сам.
   */
  composition: number;
  /** Названия написанных текстов системы: `description`, `advantages`. */
  texts?: Record<string, string>;
  /** Какие тексты модель написать не смогла. Выбор при этом сохранён. */
  missing?: string[];
}

export interface OfferItemPayload {
  catalogId?: number;
  /** Номер ЕКН (2026-11-02): приходит от сервера и уходит назад в PATCH. */
  ekn?: string;
  name: string;
  unit: string;
  qty: number;
  price: number;
}

export interface OfferPayload {
  id: number;
  number: string;
  ownerEmail: string;
  client: { name?: string; contact?: string };
  items: Array<OfferItemPayload & { sum: number }>;
  formValues: Record<string, string>;
  total: number;
  status: string;
}

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

async function call<T>(
  method: string,
  path: string,
  body?: unknown,
  timeoutMs = TIMEOUT_MS,
): Promise<T> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const res = await fetch(`${API_URL}/api/offers${path}`, {
      method,
      headers: {
        'Content-Type': 'application/json',
        'X-Service-Secret': BOT_SECRET,
      },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: controller.signal,
    });
    const text = await res.text();
    if (!res.ok) {
      let message = `Ошибка сервера (${res.status})`;
      try {
        const parsed = JSON.parse(text) as { error?: string };
        if (parsed.error) message = parsed.error;
      } catch {
        // не JSON
      }
      throw new ApiError(message, res.status);
    }
    return (text ? JSON.parse(text) : undefined) as T;
  } catch (e) {
    // Отмена по таймауту приходит как AbortError без текста — человеку нужен
    // понятный срок, а не «The operation was aborted» (2026-11-06).
    if (e instanceof Error && e.name === 'AbortError') {
      throw new ApiError(`сервер не ответил за ${Math.round(timeoutMs / 1000)} секунд`, 0);
    }
    throw e;
  } finally {
    clearTimeout(timer);
  }
}

// Роли в ответе нет: в боте ролей нет (2026-10-29). Тип описывает реальную
// форму ответа — так компилятор не даст случайно снова опереться на роль.
export interface BotIdentity {
  email: string;
}

export async function identity(chatId: string): Promise<BotIdentity> {
  return call('GET', `/bot/identity?chat_id=${encodeURIComponent(chatId)}`);
}

export async function requestCode(email: string, chatId: string): Promise<void> {
  await call('POST', '/bot/request-code', { email, chat_id: chatId });
}

export async function verifyCode(
  chatId: string,
  email: string,
  code: string,
): Promise<BotIdentity> {
  return call('POST', '/bot/verify-code', { chat_id: chatId, email, code });
}

export async function catalog(query: string): Promise<CatalogItem[]> {
  const q = query ? `?q=${encodeURIComponent(query)}` : '';
  const res = await call<{ items: CatalogItem[] }>('GET', `/bot/catalog${q}`);
  return res.items ?? [];
}

/**
 * Системы из Базы знаний — для команды «система <название>».
 *
 * Каталог бота их не содержит: у системы нет цены. Здесь только код, название
 * и проблемы состава — больше боту ничего не нужно.
 */
export async function systems(): Promise<{ systems: SystemItem[]; kbEnabled: boolean; error?: string }> {
  return call('GET', '/bot/systems');
}

/**
 * Выбрать систему для предложения.
 *
 * Система НЕ добавляет позиций (2026-11-03): в КП попадают обоснование,
 * преимущества и описание продуктов, а состав с ценами менеджер ведёт сам —
 * файлом или руками. Раньше бот дописывал состав, и в таблице КП оказывались
 * одни и те же продукты дважды, с ценой и с нулём.
 */
export async function addSystem(
  offerId: number,
  chatId: string,
  choice: SystemChoice,
): Promise<SystemAddResponse> {
  // При выборе системы модель пишет её тексты — это долго (2026-11-06).
  return call(
    'POST',
    `/bot/offers/${offerId}/system`,
    { chat_id: chatId, ...choice },
    LONG_TIMEOUT_MS,
  );
}

/**
 * Убрать систему из предложения (2026-11-05).
 *
 * Та же ручка, другое действие: менеджер может предложить несколько систем и
 * сравнивать их, а лишнюю убрать прямо из чата.
 */
export async function removeSystem(
  offerId: number,
  chatId: string,
  choice: SystemChoice,
): Promise<SystemAddResponse> {
  return call('DELETE', `/bot/offers/${offerId}/system`, { chat_id: chatId, ...choice });
}

export async function createOffer(payload: {
  chatId: string;
  client: { name: string; contact: string };
  items: OfferItemPayload[];
  formValues: Record<string, string>;
}): Promise<OfferPayload> {
  return call('POST', '/bot/offers', {
    chat_id: payload.chatId,
    client: payload.client,
    items: payload.items,
    formValues: payload.formValues,
  });
}

export async function patchOffer(
  id: number,
  payload: {
    chatId: string;
    client?: { name: string; contact: string };
    items?: OfferItemPayload[];
    formValues?: Record<string, string>;
  },
): Promise<OfferPayload> {
  return call('PATCH', `/bot/offers/${id}`, {
    chat_id: payload.chatId,
    client: payload.client,
    items: payload.items,
    formValues: payload.formValues,
  });
}

export async function docx(
  id: number,
  chatId: string,
): Promise<{ docx_base64: string; file_name: string }> {
  // Сборка может ждать модель: свой предел, не общий 60 секунд (2026-11-06).
  return call(
    'POST',
    `/bot/offers/${id}/docx?chat_id=${encodeURIComponent(chatId)}`,
    undefined,
    LONG_TIMEOUT_MS,
  );
}

export async function offerLink(id: number, chatId: string): Promise<{ url: string; expiresAt: string }> {
  return call('POST', `/bot/offers/${id}/link?chat_id=${encodeURIComponent(chatId)}`);
}

export async function listOffers(chatId: string): Promise<OfferPayload[]> {
  const res = await call<{ offers: OfferPayload[] }>('GET', `/bot/offers?chat_id=${encodeURIComponent(chatId)}`);
  return res.offers ?? [];
}

/**
 * Одно предложение этого чата (2026-11-01).
 *
 * Нужен, чтобы не спрашивать второй раз то, что уже известно: адресат приходит
 * брифом в `formValues["client_name"]`, состав мог приехать файлом.
 */
export async function getOffer(id: number, chatId: string): Promise<OfferPayload> {
  return call('GET', `/bot/offers/${id}?chat_id=${encodeURIComponent(chatId)}`);
}

/** Отметка «бот жив» + последняя ошибка: по ней плагин рисует состояние без
 *  обращения к контейнеру бота. */
export async function heartbeat(lastError: string): Promise<void> {
  await call('POST', '/bot/heartbeat', { last_error: lastError });
}

// --- Файлы из чата (2026-10-29) ----------------------------------------

/** Что бот понял из присланного файла. Формат повторяет offers-service. */
export interface BriefFromFile {
  fields: Record<string, string[]>;
  notes?: string;
  summary: string[];
}

export interface SpecRowFromFile {
  name: string;
  unit: string;
  qty: number;
  price: number;
  sum: number;
  catalogId?: number;
  hasCatalog: boolean;
  priceDiff: boolean;
}

export interface SpecFromFile {
  rows: SpecRowFromFile[];
  matched: number;
  new: number;
  priceDiffs: number;
  warnings?: string[];
  summary: string;
}

export interface AttachResult {
  kind: string;
  name: string;
  key?: string;
  brief?: BriefFromFile;
  spec?: SpecFromFile;
  context?: string[];
  /**
   * Контекст объекта из файла, разложенный по полям брифа (2026-11-01).
   * Приезжает и вместе с таблицей состава: старый КП в Excel — это и позиции,
   * и описание объекта.
   */
  contextFields?: FileContextFromFile;
  reply: string;
  needsConfirm: boolean;
}

/** Контекст объекта из файла: те же ключи, что у полей брифа. */
export interface FileContextFromFile {
  fields: Record<string, string[]>;
  notes?: string;
  summary: string[];
}

/**
 * Разбор файла. Тело — сами байты, имя — в запросе: так не нужен разбор
 * multipart, а лимит размера проверяется до чтения тела целиком.
 */
export async function attachFile(
  id: number,
  chatId: string,
  fileName: string,
  data: Buffer,
): Promise<AttachResult> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), TIMEOUT_MS);
  try {
    const url =
      `${API_URL}/api/offers/bot/offers/${id}/attach` +
      `?chat_id=${encodeURIComponent(chatId)}&file_name=${encodeURIComponent(fileName)}`;
    const res = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/octet-stream', 'X-Service-Secret': BOT_SECRET },
      body: new Uint8Array(data),
      signal: controller.signal,
    });
    const text = await res.text();
    if (!res.ok) {
      let message = `Не удалось принять файл (${res.status})`;
      try {
        const parsed = JSON.parse(text) as { error?: string };
        if (parsed.error) message = parsed.error;
      } catch {
        // не JSON
      }
      throw new ApiError(message, res.status);
    }
    return JSON.parse(text) as AttachResult;
  } finally {
    clearTimeout(timer);
  }
}

/** Применить подтверждённый разбор: в бриф или в состав. */
export async function applyAttach(
  id: number,
  chatId: string,
  mode: 'brief' | 'spec',
  payload: { brief?: BriefFromFile; spec?: SpecFromFile; context?: FileContextFromFile },
): Promise<{ reply: string }> {
  return call('POST', `/bot/offers/${id}/attach/apply`, {
    chat_id: chatId,
    mode,
    brief: payload.brief,
    spec: payload.spec,
    context: payload.context,
  });
}

// --- Бриф и тексты секций (Task 9, 2026-10-29) --------------------------

export interface BriefPayload {
  object_name?: string;
  customer_goals?: string[];
  customer_pains?: string[];
  project_risks?: string[];
  objections?: Array<{ claim: string; response: string }>;
  budget_signal?: string;
  decision_maker?: string;
  timeline?: string;
  why_us?: string[];
  notes?: string;
  skipped_topics?: string[];
}

export interface Brief {
  id: number;
  offerId: number;
  payload: BriefPayload;
  /** open | ready | agreed */
  status: string;
  chatId: string;
  lastTopic: string;
  updatedAt: string;
}

export interface BriefStartReply {
  brief: Brief;
  reply: string;
  done: boolean;
  owner: string;
}

export interface BriefStepReply {
  brief: Brief;
  reply: string;
  done: boolean;
  nextTopic?: string;
  retry?: boolean;
  unavailable?: boolean;
  skipped?: string[];
}

/** Начать бриф по созданному предложению. */
export async function startBrief(id: number, chatId: string): Promise<BriefStartReply> {
  return call('POST', `/bot/offers/${id}/brief/start?chat_id=${encodeURIComponent(chatId)}`);
}

/** Один ответ менеджера в диалоге брифа. */
export async function stepBrief(id: number, chatId: string, text: string): Promise<BriefStepReply> {
  return call('POST', `/bot/offers/${id}/brief/step`, { chat_id: chatId, text });
}

export interface SectionReply {
  key: string;
  text: string;
  warnings?: string[];
  label: string;
}

/** Написать секцию заново (модель). */
export async function writeCopy(
  id: number,
  chatId: string,
  kind: string,
  systemIndex = 0,
): Promise<SectionReply> {
  return call('POST', `/bot/offers/${id}/copy/${kind}`, {
    chat_id: chatId,
    action: 'write',
    systemIndex,
  });
}

/** Вычитать текст менеджера. */
export async function editCopy(
  id: number,
  chatId: string,
  kind: string,
  text: string,
  systemIndex = 0,
): Promise<SectionReply> {
  return call('POST', `/bot/offers/${id}/copy/${kind}`, {
    chat_id: chatId,
    action: 'edit',
    text,
    systemIndex,
  });
}

/** Принять текст в предложение. Пустой текст означает «пропустить». */
export async function acceptCopy(
  id: number,
  chatId: string,
  kind: string,
  text: string,
  systemIndex = 0,
): Promise<{ key: string; accepted: boolean }> {
  return call('POST', `/bot/offers/${id}/copy/${kind}`, {
    chat_id: chatId,
    action: 'accept',
    text,
    systemIndex,
  });
}

/** Прочитать бриф предложения (для сводки). */
export async function getBrief(id: number, chatId: string): Promise<Brief> {
  const res = await call<{ brief: Brief }>(
    'GET',
    `/bot/offers/${id}/brief?chat_id=${encodeURIComponent(chatId)}`,
  );
  return res.brief;
}

// --- Боевой листок в боте (2026-11-09) ---------------------------------
//
// Сущность та же, что в плагине (combat.go): сравнение нашего продукта с
// продуктами конкурентов. Бот выбирает стороны по названиям, а листок хранит
// заметку и подпись (`noteId`+`title`).

/**
 * Что есть у нас по человеческой фразе (2026-11-12).
 *
 * Живая жалоба: менеджер в брифе писал «Гарант» или «Кровля», а бот молча
 * клал это в поле брифа. Здесь системы и материалы ищутся одним запросом тем же
 * полнотекстовым поиском Базы знаний, что и окна выбора в плагине.
 */
export interface SuggestMaterial {
  noteId: number;
  materialId: number;
  sku?: string;
  name: string;
  unit: string;
  priceMin: number;
  priceMax: number;
  ekn?: string[];
}

export async function suggestions(query: string): Promise<{
  systems: CombatSystemItem[];
  materials: SuggestMaterial[];
  error?: string;
}> {
  return call('GET', `/suggest?q=${encodeURIComponent(query)}`);
}

/** Карточка стороны в списке выбора: то, что увидит менеджер. */
export interface CombatSystemItem {
  noteId: number;
  title: string;
  sku?: string;
  problems?: string[];
}

/** Ссылка на карточку в сохранённом листке. */
export interface CombatNoteRef {
  noteId: number;
  title: string;
}

/** Боевой листок — снапшот, как КП. */
export interface CombatSheet {
  id: number;
  number: string;
  ownerEmail: string;
  title: string;
  offerId: number;
  ourNotes: CombatNoteRef[];
  competitorNotes: CombatNoteRef[];
  formValues: Record<string, string>;
  status: string;
  createdAt: string;
  updatedAt: string;
  /** Подпись для списка: продукт листка (2026-11-12, считает сервер). */
  displayName?: string;
}

/**
 * Наш продукт для выбора стороны: материалы каталога и системы Базы знаний.
 *
 * `q` — поиск по названию и коду: каталог 1500+ записей, списком его не выбрать
 * (2026-11-09). `offset`/`limit` — страница: бот листает список, как в режиме
 * «Документы» (2026-11-10).
 */
export async function combatSystems(
  q = '',
  offset = 0,
  limit = 8,
): Promise<{ cards: CombatSystemItem[]; total?: number; offset?: number; hasMore?: boolean; error?: string }> {
  return call('GET', `/bot/combat/systems?${combatPageQuery(q, offset, limit)}`);
}

/**
 * Продукты конкурентов для выбора стороны, поиск по `q` и страница.
 *
 * С 2026-11-12 `q` уходит в полнотекстовый поиск Базы знаний (тег
 * `competitor`), а не в фильтр по кэшу: «ПВХ мембрана» находит и PLASTFOIL, и
 * Sikaplan, и ФАХМАНН. Без запроса — прежний список из кеша (тег + папка).
 */
export async function combatCompetitors(q = '', offset = 0, limit = 8): Promise<{
  cards: CombatSystemItem[];
  folderFound?: boolean;
  total?: number;
  offset?: number;
  hasMore?: boolean;
  error?: string;
}> {
  return call('GET', `/bot/combat/competitors?${combatPageQuery(q, offset, limit)}`);
}

/** Строка запроса списка сторон: поиск `q` и страница `offset`/`limit` — одна
 *  сборка на обе стороны, чтобы параметры не разошлись. */
function combatPageQuery(q: string, offset: number, limit: number): string {
  const params = new URLSearchParams();
  if (q.trim()) params.set('q', q.trim());
  params.set('offset', String(offset));
  params.set('limit', String(limit));
  return params.toString();
}

/** Потолок страницы на сервере (`combatPageSizeMax` в botcombat.go). */
const COMBAT_BULK_LIMIT = 50;

/**
 * Все совпадения по запросу стороны — для «✓ Добавить все» (2026-11-10).
 *
 * Сервер отдаёт страницы с потолком 50 строк, поэтому собираем по страницам,
 * пока `hasMore`. `total` из первого ответа — предел: если сервис обещает
 * больше, чем реально пришло, останавливаемся на нём, а не крутимся вечно.
 * Повтором `q` между страницами не рискуем: запрос неизменен во время сбора.
 */
export async function combatAllPaged(
  side: CombatSide,
  q = '',
): Promise<CombatSystemItem[]> {
  const all: CombatSystemItem[] = [];
  let offset = 0;
  // Всего найдено: сервер отвечает `total` в КАЖДОМ снимке списка, поэтому
  // опираемся и на число, и на `hasMore`. Без `total` (старый сервис) цикл
  // идёт по `hasMore`, а пределы не дают ему крутиться вечно.
  let total = Number.POSITIVE_INFINITY;
  for (let page = 0; page < 100 && all.length < total; page++) {
    const res = side === 'our'
      ? await combatSystems(q, offset, COMBAT_BULK_LIMIT)
      : await combatCompetitors(q, offset, COMBAT_BULK_LIMIT);
    const cards = res.cards ?? [];
    if (cards.length === 0) break;
    all.push(...cards);
    if (typeof res.total === 'number' && res.total >= 0) total = res.total;
    if (!res.hasMore || all.length >= total) break;
    offset = (res.offset ?? offset) + cards.length;
  }
  return all;
}

/** «Мои боевые листки» этого чата. */
export async function listCombat(chatId: string): Promise<CombatSheet[]> {
  const res = await call<{ sheets: CombatSheet[] }>(
    'GET',
    `/bot/combat?chat_id=${encodeURIComponent(chatId)}`,
  );
  return res.sheets ?? [];
}

/** Создать пустой боевой листок с номером. */
export async function createCombat(chatId: string, title: string): Promise<CombatSheet> {
  return call('POST', '/bot/combat', { chat_id: chatId, title });
}

export type CombatSide = 'our' | 'competitor';

/** Добавить сторону (наш продукт или конкурента) в листок. */
export async function addCombatSide(
  id: number,
  chatId: string,
  side: CombatSide,
  ref: CombatNoteRef,
): Promise<CombatSheet> {
  return call('POST', `/bot/combat/${id}/${side}`, {
    chat_id: chatId,
    noteId: ref.noteId,
    title: ref.title,
  });
}

/** Убрать сторону из листка. */
export async function removeCombatSide(
  id: number,
  chatId: string,
  side: CombatSide,
  noteId: number,
): Promise<CombatSheet> {
  return call('DELETE', `/bot/combat/${id}/${side}`, { chat_id: chatId, noteId });
}

/** Уточнение для разбора боевого листка (2026-11-11).
 *
 * Это единственная настройка листка: содержание всегда одно (перечень продуктов
 * и сравнение), а уточнение говорит, на чём сравнение должно сделать акцент.
 * Пустая строка стирает уточнение.
 */
export async function setCombatHint(
  id: number,
  chatId: string,
  hint: string,
): Promise<CombatSheet> {
  return call('PUT', `/bot/combat/${id}/hint`, { chat_id: chatId, hint });
}

/** Наши продукты, ближайшие к продукту конкурента («Подобрать аналоги»). */
export async function combatAnalogues(
  noteId: number,
): Promise<{ cards: CombatSystemItem[]; competitor?: { noteId: number; title: string }; error?: string }> {
  return call('GET', `/bot/combat/analogues?noteId=${encodeURIComponent(String(noteId))}`);
}

/** DOCX боевого листка: та же печать, что в плагине. */
export async function combatDocx(
  id: number,
  chatId: string,
): Promise<{ docx_base64: string; file_name: string }> {
  return call(
    'POST',
    `/bot/combat/${id}/export?chat_id=${encodeURIComponent(chatId)}`,
    undefined,
    LONG_TIMEOUT_MS,
  );
}