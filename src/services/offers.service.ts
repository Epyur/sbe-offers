import { requestUrl } from 'obsidian';
import { getService } from '../../../sbe-core/src/bridge';
import { errorMessage } from '../../../sbe-core/src/utils/errors';
import { OFFERS_APP_ID } from '../constants';
import type {
  AccessEntry,
  CatalogItem,
  Offer,
  OfferForm,
  Role,
} from '../types/offers';

const TIMEOUT_MS = 20000;

export class OffersError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

/** Кто я в приложении: роль, выданная сервером. */
export interface MeResponse {
  email: string;
  role: Role | '';
  hasAccess: boolean;
}

/**
 * Тонкая обёртка над `/api/offers/*`. Авторизация — JWT из ЦУП под app_id
 * `offers`; права проверяет сервер, плагин лишь показывает ошибку.
 */
export class OffersService {
  constructor(private apiUrl: () => string) {}

  private base(): string {
    return this.apiUrl().replace(/\/+$/, '');
  }

  private async getToken(): Promise<string> {
    const apstore = await getService('sbe-apstore');
    return apstore.auth.getToken(OFFERS_APP_ID);
  }

  private async request<T>(
    method: string,
    path: string,
    body?: unknown,
  ): Promise<T> {
    const token = await this.getToken();
    const url = `${this.base()}/api/offers${path}`;
    const started = Date.now();
    let res: Awaited<ReturnType<typeof requestUrl>>;
    try {
      res = await requestUrl({
        url,
        method,
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: body === undefined ? undefined : JSON.stringify(body),
        throw: false,
      });
    } catch (e: unknown) {
      throw new OffersError(`Сервер недоступен: ${errorMessage(e)}`, 0);
    }
    const elapsed = Date.now() - started;
    if (elapsed > TIMEOUT_MS) {
      console.warn(`КП: долгий ответ ${method} ${path} — ${elapsed} мс`);
    }
    if (res.status < 200 || res.status >= 300) {
      throw new OffersError(this.extractError(res.text, res.status), res.status);
    }
    if (!res.text) return undefined as T;
    try {
      return JSON.parse(res.text) as T;
    } catch (e: unknown) {
      throw new OffersError(`Сервер вернул не JSON: ${errorMessage(e)}`, res.status);
    }
  }

  private extractError(text: string, status: number): string {
    if (text) {
      try {
        const parsed = JSON.parse(text) as { error?: string };
        if (parsed.error) return parsed.error;
      } catch {
        // не JSON — покажем общий текст ниже
      }
    }
    if (status === 401) return 'Ключ недействителен — войдите заново через ЦУП.';
    if (status === 403) return 'Нет прав для этого действия.';
    return `Ошибка сервера (${status}).`;
  }

  async me(): Promise<MeResponse> {
    return this.request<MeResponse>('GET', '/permissions/me');
  }

  async listCatalog(q: string): Promise<CatalogItem[]> {
    const query = q ? `?q=${encodeURIComponent(q)}` : '';
    const res = await this.request<{ items: CatalogItem[] }>('GET', `/catalog${query}`);
    return res.items || [];
  }

  async saveCatalog(item: Partial<CatalogItem>): Promise<CatalogItem> {
    if (item.id) {
      return this.request<CatalogItem>('PUT', `/catalog/${item.id}`, item);
    }
    return this.request<CatalogItem>('POST', '/catalog', item);
  }

  async deleteCatalog(id: number): Promise<void> {
    await this.request<unknown>('DELETE', `/catalog/${id}`);
  }

  async getForm(mode = 'client'): Promise<OfferForm> {
    return this.request<OfferForm>('GET', `/form?mode=${encodeURIComponent(mode)}`);
  }

  async saveForm(form: OfferForm): Promise<OfferForm> {
    return this.request<OfferForm>('PUT', '/form', form);
  }

  async listOffers(): Promise<Offer[]> {
    const res = await this.request<{ offers: Offer[] }>('GET', '/offers');
    return res.offers || [];
  }

  async getOffer(id: number): Promise<Offer> {
    return this.request<Offer>('GET', `/offers/${id}`);
  }

  async saveOffer(offer: Partial<Offer>): Promise<Offer> {
    if (offer.id) {
      return this.request<Offer>('PUT', `/offers/${offer.id}`, offer);
    }
    return this.request<Offer>('POST', '/offers', offer);
  }

  async deleteOffer(id: number): Promise<void> {
    await this.request<unknown>('DELETE', `/offers/${id}`);
  }

  /** Ссылка на DOCX (presigned S3, ~2 суток). */
  async offerLink(id: number): Promise<{ url: string; expiresAt: string }> {
    return this.request<{ url: string; expiresAt: string }>('POST', `/offers/${id}/link`);
  }

  async listAccess(): Promise<AccessEntry[]> {
    const res = await this.request<{ entries: AccessEntry[] }>('GET', '/access/list');
    return res.entries || [];
  }

  async setRole(email: string, role: Role): Promise<AccessEntry> {
    return this.request<AccessEntry>('POST', '/access/role', { email, role });
  }

  async issueCode(email: string): Promise<{ code: string; expiresAt: string }> {
    return this.request<{ code: string; expiresAt: string }>('POST', '/access/codes', { email });
  }
}
