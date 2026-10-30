/**
 * Машина состояний диалога бота. Чистый модуль без сети: проверяется стендом
 * (src/session.test.ts), а поллер только вызывает nextStep и рассылает ответы.
 */

export type ChatStep = 'idle' | 'await_code' | 'authed';

export interface ChatSession {
  step: ChatStep;
  /** E-mail, на который запрошен код (нужен для проверки). */
  email: string;
  /** Черновик предложения — живёт в памяти процесса. */
  draft?: OfferDraft;
  /**
   * Принятый, но ещё не подтверждённый разбор файла (2026-10-29). Живёт в
   * сессии, а не в сервисе: подтверждение относится к конкретному разговору,
   * и после перезапуска бота старый разбор не должен примениться к другому КП.
   */
  attach?: AttachPending;
}

/** Что бот ждёт от человека после показа разбора файла. */
export type AttachPending =
  | { kind: 'brief'; brief: BriefFromFile }
  | { kind: 'spec'; spec: SpecFromFile };

/** Разобранный файл: повторяет ответ offers-service (см. api.ts). */
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

export interface OfferItem {
  catalogId?: number;
  name: string;
  unit: string;
  qty: number;
  price: number;
}

export interface OfferDraft {
  items: OfferItem[];
  clientName: string;
  clientContact: string;
  formValues: Record<string, string>;
  offerId: number;
  step: 'catalog' | 'qty' | 'manual' | 'fields' | 'done';
  /** Выбранная карточка каталога, для которой вводится количество. */
  pendingCatalog?: { id: number; name: string; unit: string; price: number };
  /** Поля формы, которые ещё предстоит спросить. */
  pendingFields?: string[];
  /** Значение обязательного поля, которое сейчас вводится. */
  currentField?: string;
}

export function newSession(): ChatSession {
  return { step: 'idle', email: '' };
}

export function isEmail(text: string): boolean {
  const t = text.trim();
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(t)) return false;
  return true;
}

export type StepResult =
  | { kind: 'prompt'; text: string }
  | { kind: 'request_code'; email: string; text: string }
  | { kind: 'verify_code'; email: string; code: string }
  | { kind: 'authed'; email: string }
  | { kind: 'ignore' };

/**
 * Ожидаемые действия по текущему шагу:
 * - idle: любой текст — это e-mail; валидный → запрос кода;
 * - await_code: ждём 6 цифр → проверка;
 * - authed: этим занимается flow.ts (сборка КП), здесь — ignore.
 */
export function nextStep(session: ChatSession, text: string): StepResult {
  const t = text.trim();
  if (session.step === 'authed') return { kind: 'ignore' };

  if (session.step === 'idle') {
    if (!isEmail(t)) {
      return {
        kind: 'prompt',
        text: 'Здравствуйте! Я собираю коммерческие предложения.\n\nПришлите ваш рабочий e-mail — на него придёт код входа.',
      };
    }
    return {
      kind: 'request_code',
      email: t.toLowerCase(),
      text: `Отправляю код на ${t.toLowerCase()}. Пришлите его сюда, когда получите.`,
    };
  }

  // await_code
  if (!/^\d{6}$/.test(t)) {
    return { kind: 'prompt', text: 'Код состоит из 6 цифр. Пришлите код из письма.' };
  }
  return { kind: 'verify_code', email: session.email, code: t };
}

/**
 * Применить результат входа к сессии. Роли у бота нет намеренно (2026-10-29):
 * любой, кто прошёл вход по коду, собирает КП. Права в боте не дублируют
 * права плагина и не зависят от них.
 */
export function applyLogin(session: ChatSession, email: string): ChatSession {
  return { ...session, step: 'authed', email };
}

/** Уходит ли поток в ожидание кода после запроса. */
export function applyCodeRequested(session: ChatSession, email: string): ChatSession {
  return { ...session, step: 'await_code', email };
}

/**
 * Восстановить сессию из постоянной привязки chat_id → email (offers_bot_link).
 * Знакомый чат считается уже вошедшим: перезапуск бота больше не заставляет
 * человека заново запрашивать код (2026-10-29).
 *
 * Решение принимается здесь, а не в поллере, чтобы правило «кого считаем
 * вошедшим» жило в одном месте и проверялось без сети.
 */
export function restoreSession(email: string): ChatSession {
  const known = email.trim();
  if (!known) return newSession();
  return applyLogin(newSession(), known);
}
