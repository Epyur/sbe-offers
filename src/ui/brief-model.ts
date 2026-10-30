/**
 * Чистые правила показа брифа (Task 10 плана 2026-10-29).
 *
 * Отдельно от UI: «сколько заполнено», «как называется тема» и «что показать
 * вместо пустого поля» решаются без DOM, поэтому проверяются стендом.
 */
import type { BriefPayload } from '../types/offers';

/** Одна тема брифа: что это и как называется в интерфейсе. */
export interface BriefField {
  key: keyof BriefPayload;
  label: string;
  multiline: boolean;
}

/**
 * Порядок полей в брифе. Совпадает с порядком тем разговора в боте
 * (`skills.BriefTopics`): менеджер видит тот же маршрут, что прошёл в чате.
 */
export const BRIEF_FIELDS: BriefField[] = [
  { key: 'object_name', label: 'Объект', multiline: false },
  { key: 'customer_goals', label: 'Задачи клиента', multiline: true },
  { key: 'customer_pains', label: 'Боли', multiline: true },
  { key: 'project_risks', label: 'Риски', multiline: true },
  { key: 'decision_maker', label: 'Кто решает', multiline: false },
  { key: 'timeline', label: 'Сроки', multiline: false },
  { key: 'budget_signal', label: 'Бюджет', multiline: false },
  { key: 'why_us', label: 'Почему мы', multiline: true },
  { key: 'objections', label: 'Возражения', multiline: true },
  { key: 'notes', label: 'Прочее', multiline: true },
];

/** Заполнено ли значение поля. Список пуст — поле не заполнено. */
export function isFilled(payload: BriefPayload, key: keyof BriefPayload): boolean {
  const value = payload[key];
  if (Array.isArray(value)) {
    if (key === 'objections') {
      // Возражение считается отработанным только с ответом: возражение без
      // ответа — это запись, а не работа с ним (то же правило, что в боте).
      return value.some((o) => {
        const item = o as { claim?: string; response?: string };
        return String(item.claim ?? '').trim() !== '' && String(item.response ?? '').trim() !== '';
      });
    }
    return value.length > 0;
  }
  return typeof value === 'string' ? value.trim() !== '' : false;
}

/** Сколько полей заполнено — для подписи «6 из 10». */
export function filledCount(payload: BriefPayload): number {
  return BRIEF_FIELDS.filter((f) => isFilled(payload, f.key)).length;
}

/** Подпись состояния брифа в списке КП. */
export function briefStatusLine(payload: BriefPayload | undefined, absent: boolean): string {
  if (absent || !payload) return 'брифа нет';
  const filled = filledCount(payload);
  if (filled === 0) return 'бриф пуст';
  return `бриф заполнен ${filled} из ${BRIEF_FIELDS.length} полей`;
}

/** Значение поля одной строкой — для чтения без редактирования. */
export function fieldAsText(payload: BriefPayload, key: keyof BriefPayload): string {
  const value = payload[key];
  if (Array.isArray(value)) {
    if (key === 'objections') {
      return value
        .map((o) => {
          const item = o as { claim?: string; response?: string };
          const claim = String(item.claim ?? '').trim();
          const response = String(item.response ?? '').trim();
          if (!claim && !response) return '';
          return response ? `${claim} → ${response}` : claim;
        })
        .filter((line) => line !== '')
        .join('\n');
    }
    return (value as string[]).join('\n');
  }
  return String(value ?? '').trim();
}

/** Обратный разбор: строка из textarea обратно в значение поля. */
export function textToField(text: string, key: keyof BriefPayload): string | string[] {
  const value = text
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line !== '');
  if (key === 'object_name' || key === 'budget_signal' || key === 'decision_maker' || key === 'timeline' || key === 'notes') {
    return text.trim();
  }
  return value;
}

/** Темы, которые менеджер закрыл словом «не знаю». */
export function skippedLabels(payload: BriefPayload): string[] {
  return (payload.skipped_topics ?? []).map((key) => {
    const field = BRIEF_FIELDS.find((f) => f.key === key);
    return field ? field.label : key;
  });
}