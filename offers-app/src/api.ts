/**
 * Адрес API и заголовки — одна точка на всё приложение.
 *
 * Приложение ходит в те же `/api/offers/*`, что и плагин Obsidian: JWT берётся
 * в шаге S1 (POST /app/session по initData). Поэтому здесь только транспорт:
 * адрес, токен в памяти и разбор ответа. Никакой бизнес-логики.
 */

const DEFAULT_API_URL = 'https://epyur.fvds.ru';

function apiBase(): string {
  // Пусто → тот же домен, с которого раздаётся приложение (так и работает на
  // сервере). Переменная нужна для запуска на стенде и локально.
  const fromEnv = import.meta.env.VITE_OFFERS_API_URL;
  return (fromEnv && fromEnv.trim() !== '' ? fromEnv : DEFAULT_API_URL).replace(/\/+$/, '');
}

let token = '';

export function setToken(value: string): void {
  token = value;
}

export function authToken(): string {
  return token;
}

export class ApiError extends Error {
  readonly status: number;
  /** Поле `error` ответа сервиса: `not_linked`, `initData`, `invalid json`… */
  readonly code: string;

  constructor(status: number, message: string, code = '') {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
  }
}

interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
  body?: unknown;
}

/**
 * Запрос к сервису. Ошибку отдаёт вместе с полем `error` сервиса — по нему
 * экран входа отличает «не привязан к боту» от «подпись устарела».
 */
export async function apiRequest<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' };
  if (token !== '') headers.Authorization = `Bearer ${token}`;
  if (options.body !== undefined) headers['Content-Type'] = 'application/json';

  const res = await fetch(`${apiBase()}/api/offers${path}`, {
    method: options.method ?? 'GET',
    headers,
    body: options.body === undefined ? null : JSON.stringify(options.body),
  });

  const text = await res.text();
  // Тело ошибки бывает и не-JSON (прокси, пустой 502): тогда разбирать нечего.
  const parsed: unknown = text === '' ? null : tryParse(text);
  if (!res.ok) {
    const message = errorMessageOf(parsed) ?? `Сервер ответил ${res.status}`;
    throw new ApiError(res.status, message, errorCodeOf(parsed));
  }
  return parsed as T;
}

function tryParse(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

function errorCodeOf(parsed: unknown): string {
  if (typeof parsed !== 'object' || parsed === null) return '';
  const value = (parsed as { error?: unknown }).error;
  return typeof value === 'string' ? value : '';
}

function errorMessageOf(parsed: unknown): string | null {
  if (typeof parsed !== 'object' || parsed === null) return null;
  const value = (parsed as { error?: unknown }).error;
  return typeof value === 'string' && value !== '' ? value : null;
}