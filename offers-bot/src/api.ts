/** Клиент offers-service для бота: только те ручки, что защищены X-Service-Secret. */

const API_URL = (process.env.OFFERS_API_URL ?? 'http://offers:3000').replace(/\/+$/, '');
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

export async function identity(chatId: string): Promise<{ email: string; role: string }> {
  return call('GET', `/bot/identity?chat_id=${encodeURIComponent(chatId)}`);
}

export async function requestCode(email: string, chatId: string): Promise<void> {
  await call('POST', '/bot/request-code', { email, chat_id: chatId });
}

export async function verifyCode(
  chatId: string,
  email: string,
  code: string,
): Promise<{ email: string; role: string }> {
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