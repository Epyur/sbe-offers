/**
 * Мост к Telegram Mini Apps (2026-11-13, шаг S0 плана миграции).
 *
 * Приложение должно открываться и вне Telegram — тогда же удобно смотреть
 * вёрстку в браузере. Поэтому здесь ОДНА точка, которая решает, есть ли
 * окружение Telegram, а не проверка `window.Telegram` в каждом экране.
 *
 * Что наружу: `tg` — методы, которые нужны приложению (ready, expand,
 * цветовая схема, initData, sendData, закрыть), и `inTelegram` — флаг.
 */

// Описываем только то, чем пользуемся: полный тип для API Telegram здесь
// пришлось бы держать вручную, а `@types` у него нет в реестре npm.
export interface TelegramWebApp {
  initData: string;
  initDataUnsafe: {
    user?: { id: number; first_name?: string; last_name?: string; username?: string };
  };
  colorScheme: 'light' | 'dark';
  themeParams: Record<string, string | undefined>;
  ready(): void;
  expand(): void;
  close(): void;
  sendData(data: string): void;
  onEvent(event: string, handler: () => void): void;
  offEvent(event: string, handler: () => void): void;
  setHeaderColor?(color: string): void;
  setBackgroundColor?(color: string): void;
  enableClosingConfirmation?(): void;
  version: string;
  platform: string;
}

interface TelegramGlobal {
  WebApp?: TelegramWebApp;
}

function readTelegram(): TelegramWebApp | null {
  // В тестах (node --test) окна нет вовсе: без этой проверки модуль падал бы
  // при первом импорте, а не при вызове.
  if (typeof window === 'undefined') return null;
  const g = (window as unknown as { Telegram?: TelegramGlobal }).Telegram;
  const app = g?.WebApp;
  // Скрипт Telegram грузится синхронно до модуля приложения, но в браузере
  // его нет вовсе — отсюда и null, а не ошибка.
  return app ?? null;
}

const webApp = readTelegram();

export const inTelegram = webApp !== null;

/** Инициализация окружения: без этого Telegram показывает приложение свёрнутым. */
export function initTelegram(): void {
  if (!webApp) return;
  webApp.ready();
  webApp.expand();
}

/**
 * `initData` — ключ входа в приложение (POST /api/offers/app/session, шаг S1).
 * В S0 он только читается и показывается в отладочной строке.
 */
export function initData(): string {
  return webApp?.initData ?? '';
}

/** Вернуть результат в чат (шаг S7): `tg.sendData` → `message.web_app_data`. */
export function sendToChat(payload: Record<string, unknown>): void {
  if (!webApp) return;
  webApp.sendData(JSON.stringify(payload));
  webApp.close();
}

export function close(): void {
  webApp?.close();
}

/** Название темы для строки состояния. */
export function colorScheme(): 'light' | 'dark' {
  if (webApp) return webApp.colorScheme;
  if (typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches) return 'dark';
  return 'light';
}