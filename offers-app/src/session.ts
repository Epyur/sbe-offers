/**
 * Вход в приложение (2026-11-13, шаг S1).
 *
 * Приложение — такой же JWT-клиент, как плагин Obsidian. Единственное
 * отличие: ключ входа берётся не у auth-service, а у Telegram — это строка
 * `initData`, подписанная ботом. Обмен делается один раз на старте:
 *
 *   initData → POST /api/offers/app/session → {token, user:{email, role}}
 *
 * Дальше все запросы идут с обычным `Authorization: Bearer`, значит права
 * проверяет существующий `requirePerm` — своих проверок в приложении нет.
 */
import { ApiError, apiRequest, setToken } from './api.ts';
import { initData } from './tg.ts';

export interface AppUser {
  email: string;
  role: string;
}

export interface AppSession {
  user: AppUser;
  offers: number;
  sheets: number;
}

/** Причины, по которым вход не дал сессию — их видно человеку дословно. */
export type SessionFailure =
  | { kind: 'no-init-data' }
  | { kind: 'not-linked' }
  | { kind: 'bad-init-data'; detail: string }
  | { kind: 'bot-not-configured'; detail: string }
  | { kind: 'network'; detail: string }
  | { kind: 'server'; detail: string };

export type SessionResult =
  | { ok: true; session: AppSession }
  | { ok: false; failure: SessionFailure };

interface SessionResponse {
  token: string;
  user: AppUser;
  offers: number;
  sheets: number;
}

/**
 * Обменять initData на сессию. Токен сразу кладётся в `api.ts`, чтобы
 * следующие вызовы были авторизованы без передачи его по рукам.
 */
export async function openSession(): Promise<SessionResult> {
  const raw = initData();
  if (raw === '') {
    // Открыто в браузере или вне Telegram: входа не будет, и это нормально.
    return { ok: false, failure: { kind: 'no-init-data' } };
  }

  try {
    const res = await apiRequest<SessionResponse>('/app/session', {
      method: 'POST',
      body: { initData: raw },
    });
    setToken(res.token);
    return { ok: true, session: { user: res.user, offers: res.offers, sheets: res.sheets } };
  } catch (e: unknown) {
    if (e instanceof ApiError && e.status === 403) {
      return { ok: false, failure: { kind: 'not-linked' } };
    }
    if (e instanceof ApiError && e.status === 503) {
      return { ok: false, failure: { kind: 'bot-not-configured', detail: e.message } };
    }
    if (e instanceof ApiError && (e.status === 400 || e.status === 401)) {
      return { ok: false, failure: { kind: 'bad-init-data', detail: e.message } };
    }
    if (e instanceof ApiError) {
      return { ok: false, failure: { kind: 'server', detail: e.message } };
    }
    return { ok: false, failure: { kind: 'network', detail: errorText(e) } };
  }
}

function errorText(e: unknown): string {
  return e instanceof Error ? e.message : 'Не удалось связаться с сервером';
}

/** Текст для экрана входа: что произошло и что сделать. */
export function failureText(failure: SessionFailure): { title: string; hint: string } {
  switch (failure.kind) {
    case 'no-init-data':
      return {
        title: 'Открыто вне Telegram',
        hint: 'Вход работает только из чата с ботом: нажмите «Открыть приложение».',
      };
    case 'not-linked':
      return {
        title: 'Нужно войти в бота',
        hint: 'Откройте чат с ботом, нажмите /start и введите код из письма. После этого бот снова даст ссылку.',
      };
    case 'bad-init-data':
      return {
        title: 'Вход не подтверждён',
        hint: `Telegram передал устаревшую или неверную подпись (${failure.detail}). Откройте приложение из чата заново.`,
      };
    case 'bot-not-configured':
      return {
        title: 'Бот не настроен',
        hint: 'Администратору нужно ввести токен бота в плагине, раздел «Бот и доступ».',
      };
    case 'network':
      return { title: 'Нет связи с сервером', hint: failure.detail };
    case 'server':
      return { title: 'Сервер не ответил как ожидалось', hint: failure.detail };
  }
}