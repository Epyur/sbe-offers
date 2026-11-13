/**
 * Мост к Telegram Mini Apps (2026-11-13, шаг S0 плана миграции).
 *
 * Приложение должно открываться и вне Telegram — тогда же удобно смотреть
 * вёрстку в браузере. Поэтому здесь ОДНА точка, которая решает, есть ли
 * окружение Telegram, а не проверка `window.Telegram` в каждом экране.
 *
 * Что наружу: `tg` — методы, которые нужны приложению (ready, expand,
 * цветовая схема, initData, sendData, закрыть), и `inTelegram` — флаг.
 *
 * **Важно (дефект 2026-11-13).** Раньше мост захватывался ПРИ ЗАГРУЗКЕ модуля:
 * `const webApp = readTelegram()`. Если в этот момент `window.Telegram` ещё не
 * появился (скрипт платформы догружается после модуля, кэш, медленная сеть
 * телефона), мост оставался `null` навсегда — приложение показывало «Открыто
 * вне Telegram» до перезагрузки. Теперь мост ищется при каждом обращении, а
 * при медленной загрузке приложение ждёт появления скрипта.
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
  return g?.WebApp ?? null;
}

/** Мост «здесь и сейчас», а не снимок момента загрузки модуля. */
export function tg(): TelegramWebApp | null {
  return readTelegram();
}

/**
 * Открыто ли приложение внутри Telegram. Вызывать в начале экрана, не в
 * модуле: к этому моменту скрипт платформы уже загружен (см. `waitForTelegram`).
 */
export function inTelegram(): boolean {
  return readTelegram() !== null;
}

/**
 * Дождаться появления скрипта платформы. До 2 секунд, шаг 50 мс.
 *
 * Зачем: `<script src=...>` подключается перед модулем, но в webview телефона
 * порядок не гарантирован при загрузке из кэша. Ждём, а не читаем флаг один
 * раз — иначе получаем ровно тот дефект, ради которого это написано.
 * Возвращает true, если Telegram найден.
 */
export function waitForTelegram(timeoutMs = 2000): Promise<boolean> {
  const found = readTelegram();
  if (found !== null) {
    found.ready();
    found.expand();
    return Promise.resolve(true);
  }
  if (typeof window === 'undefined') return Promise.resolve(false);
  return new Promise<boolean>((resolve) => {
    const started = Date.now();
    const timer = window.setInterval(() => {
      const app = readTelegram();
      if (app !== null) {
        window.clearInterval(timer);
        app.ready();
        app.expand();
        resolve(true);
        return;
      }
      if (Date.now() - started >= timeoutMs) {
        window.clearInterval(timer);
        resolve(false);
      }
    }, 50);
  });
}

/** Инициализация окружения: без этого Telegram показывает приложение свёрнутым. */
export function initTelegram(): void {
  const app = readTelegram();
  if (app === null) return;
  app.ready();
  app.expand();
}

/**
 * `initData` — ключ входа в приложение (POST /api/offers/app/session, шаг S1).
 * Читается в момент вызова: значение появляется после загрузки скрипта.
 */
export function initData(): string {
  return readTelegram()?.initData ?? '';
}

/** Версия и платформа платформы — для строки диагностики на экране. */
export function platformInfo(): { version: string; platform: string } {
  const app = readTelegram();
  return { version: app?.version ?? '', platform: app?.platform ?? '' };
}

/**
 * `query_id` — ключ диалога для `answerWebAppQuery` (2026-11-14).
 *
 * Telegram кладёт его в `initData` строкой `query_id=AAH…`. Значение есть у приложения,
 * открытого **inline**-кнопкой или из меню; у кнопки клавиатуры его нет — там
 * работает `sendData`.
 */
export function queryId(): string {
  const raw = initData();
  if (raw === '') return '';
  for (const chunk of raw.split('&')) {
    if (chunk.startsWith('query_id=')) {
      try {
        return decodeURIComponent(chunk.slice('query_id='.length));
      } catch {
        return '';
      }
    }
  }
  return '';
}

export function close(): void {
  readTelegram()?.close();
}

/** Название темы для строки состояния. */
export function colorScheme(): 'light' | 'dark' {
  const app = readTelegram();
  if (app !== null) return app.colorScheme;
  if (typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches) return 'dark';
  return 'light';
}