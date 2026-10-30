/** Клиент offers-service для бота: только те ручки, что защищены X-Service-Secret. */

export const API_URL = (process.env.OFFERS_API_URL ?? 'http://offers:3000').replace(/\/+$/, '');
const BOT_SECRET = process.env.OFFERS_BOT_SECRET ?? '';
const TIMEOUT_MS = 60000;

export interface CatalogItem {
  id: number;
  kind: string;
  name: string;
  unit: string;
  basePrice: number;
  description: string;
}

export interface OfferItemPayload {
  catalogId?: number;
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

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), TIMEOUT_MS);
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
  return call('POST', `/bot/offers/${id}/docx?chat_id=${encodeURIComponent(chatId)}`);
}

export async function offerLink(id: number, chatId: string): Promise<{ url: string; expiresAt: string }> {
  return call('POST', `/bot/offers/${id}/link?chat_id=${encodeURIComponent(chatId)}`);
}

export async function listOffers(chatId: string): Promise<OfferPayload[]> {
  const res = await call<{ offers: OfferPayload[] }>('GET', `/bot/offers?chat_id=${encodeURIComponent(chatId)}`);
  return res.offers ?? [];
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
  reply: string;
  needsConfirm: boolean;
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
  payload: { brief?: BriefFromFile; spec?: SpecFromFile },
): Promise<{ reply: string }> {
  return call('POST', `/bot/offers/${id}/attach/apply`, {
    chat_id: chatId,
    mode,
    brief: payload.brief,
    spec: payload.spec,
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