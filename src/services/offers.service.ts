import { requestUrl } from 'obsidian';
import { getService } from '../../../sbe-core/src/bridge';
import { errorMessage } from '../../../sbe-core/src/utils/errors';
import { OFFERS_APP_ID } from '../constants';
import type {
  AccessEntry,
  BotConfigInfo,
  BotStatusInfo,
  BriefPayload,
  BriefResponse,
  CatalogItem,
  ComposeSystemResponse,
  Material,
  Offer,
  OfferForm,
  RefreshSystemCardsResponse,
  Role,
  SheetApplyRow,
  SheetPreview,
  SystemCardsResponse,
  SystemsResponse,
} from '../types/offers';

const TIMEOUT_MS = 20000;

/**
 * Текст для 401 после повтора: это уже не «токен устарел», а проблема входа.
 *
 * Сказать «ключ недействителен» здесь было бы неправдой: чаще всего ключ
 * цел, а истёк час его JWT и перезапрос не помог. Человеку нужно действие —
 * переоткрыть ЦУП.
 */
const SESSION_STALE = 'Сеанс входа устарел — переоткройте ЦУП (приложение «ЦУП СБЕ ПМиПИР»).';

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

  /**
   * Забыть сохранённый JWT приложения (2026-10-30).
   *
   * Живая жалоба: «Контекст КП» отвечал «Ключ доступа недействителен», хотя
   * ключ был рабочим. Причина — сервис отдал 401 на токен, который в кэше
   * ЦУП ещё считался годным (пересобранный сервис, новый секрет, сдвинутые
   * часы). Плагин при этом токен не перезапрашивал вовсе.
   *
   * Ключ устройства НЕ сбрасываем: он может быть исправен, и заставлять
   * человека заново получать его по почте из-за истёкшего JWT незачем.
   */
  private async forgetToken(): Promise<void> {
    try {
      const apstore = await getService('sbe-apstore');
      apstore.auth.forgetToken(OFFERS_APP_ID);
    } catch (e: unknown) {
      // ЦУП недоступен — показать старый токен всё равно нельзя, идём без сброса.
      console.warn('КП: не удалось сбросить кэш токена:', errorMessage(e));
    }
  }

  private async request<T>(
    method: string,
    path: string,
    body?: unknown,
  ): Promise<T> {
    return this.withTokenRetry(async (token) => {
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
      if (res.status === 401) {
        // Пробрасываем 401 наверх как есть: решение о повторе принимает
        // withTokenRetry, а текста здесь ещё нет.
        throw new OffersError('unauthorized', 401);
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
    });
  }

  /**
   * Выполнить запрос с токеном; при 401 — один повтор с новым токеном.
   *
   * Политика одна на весь плагин: первый 401 означает «этот JWT устарел»
   * (пересобранный сервис, другой секрет, ушедшие часы), кэш токена
   * сбрасывается и запрос повторяется ровно один раз. Второй 401 — уже
   * настоящая проблема входа, и человеку говорится, что делать.
   */
  private async withTokenRetry<T>(run: (token: string) => Promise<T>): Promise<T> {
    const first = await this.getToken();
    try {
      return await run(first);
    } catch (e: unknown) {
      if (!(e instanceof OffersError) || e.status !== 401) throw e;
      await this.forgetToken();
      try {
        return await run(await this.getToken());
      } catch (retry: unknown) {
        if (retry instanceof OffersError && retry.status === 401) {
          throw new OffersError(SESSION_STALE, 401);
        }
        throw retry;
      }
    }
  }

  /**
   * Текст ошибки по статусу.
   *
   * Серверные ответы 401 и 403 — короткие слова `unauthorized`/`forbidden`,
   * и показывать их человеку бессмысленно: он не знает, что это. Поэтому для
   * этих двух статусов текст задаёт клиент, а серверное сообщение берётся
   * только там, где оно написано для человека (400, 404 и прочие).
   */
  private extractError(text: string, status: number): string {
    if (status === 401) return SESSION_STALE;
    if (status === 403) return 'Нет прав для этого действия.';
    if (text) {
      try {
        const parsed = JSON.parse(text) as { error?: string };
        if (parsed.error) return parsed.error;
      } catch {
        // не JSON — покажем общий текст ниже
      }
    }
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

  /** Бриф предложения вместе с историей разговора в боте. */
  async getBrief(id: number): Promise<BriefResponse> {
    return this.request<BriefResponse>('GET', `/offers/${id}/brief`);
  }

  /** Правка ответов брифа руками (владелец предложения или admin). */
  async saveBrief(id: number, payload: BriefPayload, status?: string): Promise<BriefResponse> {
    return this.request<BriefResponse>('PUT', `/offers/${id}/brief`, { payload, status });
  }

  /** Скачивание DOCX байтами: сервер отдаёт файл, а не JSON. */
  async exportDocx(id: number): Promise<Uint8Array> {
    return this.withTokenRetry(async (token) => {
      const res = await requestUrl({
        url: `${this.base()}/api/offers/offers/${id}/export?format=docx`,
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
        throw: false,
      });
      if (res.status === 401) throw new OffersError('unauthorized', 401);
      if (res.status < 200 || res.status >= 300) {
        throw new OffersError(this.extractError(res.text, res.status), res.status);
      }
      return new Uint8Array(res.arrayBuffer);
    });
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


  /** Карточки систем из Базы знаний (из кеша). */
  async getSystemCards(): Promise<SystemCardsResponse> {
    return this.request<SystemCardsResponse>('GET', '/context/cards');
  }

  // --- Материалы и ЕКН (2026-10-31) -------------------------------------

  /** Справочник материалов. Систем сюда не попадает: у них свой раздел. */
  async listMaterials(includeArchived = false): Promise<Material[]> {
    const query = includeArchived ? '?archived=1' : '';
    const res = await this.request<{ items: Material[] | null }>('GET', `/materials${query}`);
    return res.items || [];
  }

  /**
   * Создать или сохранить материал.
   *
   * `typeCode` и `series` — только подсказки для SKU нового материала: сервер
   * собирает из них код сам, а у правки их игнорирует (SKU не меняется).
   */
  async saveMaterial(item: Partial<Material> & { typeCode?: string; series?: string }): Promise<Material> {
    if (item.id) {
      return this.request<Material>('PUT', `/materials/${item.id}`, item);
    }
    return this.request<Material>('POST', '/materials', item);
  }

  /** Убрать материал в архив (не удалить: на него ссылаются старые КП). */
  async archiveMaterial(id: number): Promise<Material> {
    return this.request<Material>('DELETE', `/materials/${id}`);
  }

  /** Дописать вариацию ЕКН. Повтор того же номера ничего не меняет. */
  async addMaterialEkn(id: number, ekn: string, variant: string): Promise<Material> {
    return this.request<Material>('POST', `/materials/${id}/ekn`, { ekn, variant });
  }

  async deleteMaterialEkn(id: number, eknId: number): Promise<Material> {
    return this.request<Material>('DELETE', `/materials/${id}/ekn/${eknId}`);
  }

  // --- Таблица менеджера (2026-10-31) -----------------------------------

  /**
   * Первый шаг загрузки: разобрать и сопоставить, НИЧЕГО не записав.
   *
   * Тело — сам файл xlsx, имя в заголовке: multipart не нужен, а на стороне
   * сервиса байты читаются так же, как файлы из чата бота.
   */
  async previewSheet(offerId: number, fileName: string, data: ArrayBuffer): Promise<SheetPreview> {
    return this.withTokenRetry(async (token) => {
      const url = `${this.base()}/api/offers/offers/${offerId}/sheet`;
      const res = await requestUrl({
        url,
        method: 'POST',
        headers: {
          'Content-Type': 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
          // Имя как есть: заголовки ходят UTF-8, а сервер уже разкодирует
          // то, что пришло из query-строки бота. Двойное кодирование здесь
          // дало бы «%D0%9C…» в имени файла у человека.
          'X-File-Name': fileName,
          Authorization: `Bearer ${token}`,
        },
        body: data,
        throw: false,
      });
      if (res.status === 401) throw new OffersError('unauthorized', 401);
      if (res.status < 200 || res.status >= 300) {
        throw new OffersError(this.extractError(res.text, res.status), res.status);
      }
      return JSON.parse(res.text) as SheetPreview;
    });
  }

  /**
   * Второй шаг: записать только то, что менеджер подтвердил.
   *
   * Сервер отвечает `{applied, offer}` — не самим предложением: сколько строк
   * прошло, тоже надо показать человеку, иначе «применено» без числа.
   */
  async applySheet(
    offerId: number,
    rows: SheetApplyRow[],
    addMissingEkn: boolean,
  ): Promise<{ applied: number; offer: Offer }> {
    return this.request<{ applied: number; offer: Offer }>('POST', `/offers/${offerId}/sheet/apply`, {
      rows,
      addMissingEkn,
    });
  }

  /** Дописать состав системы в предложение. */
  async addSystem(offerId: number, sku: string): Promise<ComposeSystemResponse> {
    return this.request<ComposeSystemResponse>('POST', `/offers/${offerId}/system`, { sku });
  }

  // --- Системы (2026-10-31) ---------------------------------------------

  /** Системы из Базы знаний с составом, разрешённым против материалов. */
  async listSystems(): Promise<SystemsResponse> {
    return this.request<SystemsResponse>('GET', '/systems');
  }

  /** Перечитать карточки из Базы знаний (admin). */
  async refreshSystemCards(): Promise<RefreshSystemCardsResponse> {
    return this.request<RefreshSystemCardsResponse>('POST', '/context/refresh');
  }
  /** Настройки Telegram-бота: задан ли токен и чей это бот. */
  async getBotConfig(): Promise<BotConfigInfo> {
    return this.request<BotConfigInfo>('GET', '/bot/config');
  }

  /** Сохранить токен. Сервер сначала проверяет его через бота и только потом
   *  пишет — неверный токен рабочий не заменит. */
  async saveBotToken(token: string): Promise<BotConfigInfo> {
    return this.request<BotConfigInfo>('POST', '/bot/config', { token });
  }

  async deleteBotToken(): Promise<{ configured: boolean }> {
    return this.request<{ configured: boolean }>('DELETE', '/bot/config');
  }

  /** Работает ли бот сейчас (по отметке «бот жив», без запроса к контейнеру). */
  async getBotStatus(): Promise<BotStatusInfo> {
    return this.request<BotStatusInfo>('GET', '/bot/status');
  }
}
