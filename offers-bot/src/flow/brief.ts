/**
 * Бриф в боте: вопросы по одному и сводка перед сборкой (Task 9, 2026-10-29).
 *
 * Модуль чистый: ни сети, ни БД. Текст вопроса приходит от сервиса (он один
 * знает, какая тема не заполнена и как её спросить) — здесь только обвязка:
 * нумерация, кнопки, сводка. Так правила разговора проверяются стендом, а
 * формулировки вопросов правятся в одном месте — в сервисе.
 */
import type { ChatSession } from '../session.ts';

/** Тема, которую сервис объявил следующей. */
export interface BriefTopic {
  key: string;
  label: string;
  question: string;
}

/** Что сервис ответил на шаг брифа (форма ответа — см. api.ts). */
export interface BriefStepReply {
  reply: string;
  done: boolean;
  nextTopic?: string;
  retry?: boolean;
  unavailable?: boolean;
  skipped?: string[];
}

/** Бриф для сводки: ровно то, что нужно показать человеку. */
export interface BriefForSummary {
  payload: Record<string, unknown>;
  done?: boolean;
}

/** Кнопки после каждого ответа: дописать или идти собирать. */
export const BRIEF_KEYBOARD = {
  inline_keyboard: [
    [{ text: '✏️ Дополнить', callback_data: 'brief:more' }],
    [{ text: '✅ Дальше', callback_data: 'brief:next' }],
  ],
};

/** Сводка в конце брифа: та же пара кнопок плюс пропуск. */
export const BRIEF_DONE_KEYBOARD = {
  inline_keyboard: [
    [{ text: '✅ Дальше', callback_data: 'brief:next' }],
  ],
};

const TOPIC_LABELS: Record<string, string> = {
  object_name: 'Объект',
  customer_goals: 'Задачи клиента',
  customer_pains: 'Боли',
  project_risks: 'Риски',
  budget_signal: 'Бюджет',
  decision_maker: 'Кто решает',
  timeline: 'Сроки',
  why_us: 'Почему мы',
  objections: 'Возражения',
  notes: 'Прочее',
};

/**
 * Применить ответ сервиса к сессии.
 *
 * `done` выставляется по ответу сервиса, а не по состоянию payload: только
 * сервис знает, остались ли незаполненные темы с учётом пропущенных.
 */
export function briefTurnState(session: ChatSession, reply: BriefStepReply): ChatSession {
  return {
    ...session,
    briefTopic: reply.done ? undefined : reply.nextTopic ?? session.briefTopic,
    briefDone: reply.done,
  };
}

const FIELD_LABELS: Array<[string, string]> = [
  ['object_name', 'Объект'],
  ['customer_goals', 'Задачи клиента'],
  ['customer_pains', 'Боли'],
  ['project_risks', 'Риски'],
  ['budget_signal', 'Бюджет'],
  ['decision_maker', 'Кто решает'],
  ['timeline', 'Сроки'],
  ['why_us', 'Почему мы'],
];

function filled(value: unknown): boolean {
  if (Array.isArray(value)) return value.length > 0;
  return typeof value === 'string' ? value.trim() !== '' : false;
}

function show(value: unknown): string {
  if (Array.isArray(value)) return value.map((v) => String(v)).join('; ');
  return String(value ?? '').trim();
}

/**
 * Сводка брифа перед сборкой КП.
 *
 * Показывает то, что услышал, и честно перечисляет пропущенное: менеджер
 * должен увидеть, на чём его КП будет молчать, ДО того как файл уйдёт клиенту.
 */
export function briefSummary(brief: BriefForSummary): string {
  const payload = brief.payload ?? {};
  const lines: string[] = [];
  for (const [key, label] of FIELD_LABELS) {
    if (filled(payload[key])) lines.push(`• ${label}: ${show(payload[key])}`);
  }
  const skipped = Array.isArray(payload.skipped_topics) ? payload.skipped_topics.map(String) : [];
  if (lines.length === 0) {
    const tail = skipped.length > 0
      ? `\n\nНе отвечали: ${skipped.map((k) => TOPIC_LABELS[k] ?? k).join(', ')}.`
      : '';
    return `Бриф пуст: ничего не записал. Это не помешает собрать КП — текст тогда возьмём из формы.${tail}`;
  }
  const out = [`Бриф готов:\n\n${lines.join('\n')}`];
  if (skipped.length > 0) {
    out.push(`\nНе отвечали: ${skipped.map((k) => TOPIC_LABELS[k] ?? k).join(', ')}.`);
  }
  out.push('\nМожно дописать или перейти к сборке.');
  return out.join('');
}