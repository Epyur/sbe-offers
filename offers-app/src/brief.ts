/**
 * Бриф КП в приложении (2026-11-15).
 *
 * Бриф — это то, что бот выяснил про объект: объект, задачи, боли, риски,
 * бюджет, кто решает, сроки. Он жил в плагине и в боте; в приложении его не
 * было вовсе, и менеджер в телефоне видел документ без контекста, по которому
 * пишется текст.
 *
 * Здесь чистая часть (проверяется `test/brief.test.ts`) и сеть:
 *   - `GET  /offers/{id}/brief`           — бриф и история разговора;
 *   - `POST /offers/{id}/brief/{topic}`   — ответ на вопрос (тот же шаг
 *     диалога, что в боте, но под JWT);
 *   - `PUT  /offers/{id}/brief`           — дописать поля руками (та же ручка,
 *     что у плагина).
 *
 * Почему ответ на вопрос здесь, а не в боте: человек уже в приложении и
 * пишет ответ по делу; гонять его в Telegram значило бы потерять мысль.
 */
import { apiRequest } from './api.ts';

/** Темы брифа — те же ключи, что у сервера (`model.BriefPayload`). */
export const BRIEF_TOPICS = [
  'object_name',
  'object_purpose',
  'object_area',
  'customer_address',
  'planned_systems',
  'customer_goals',
  'customer_pains',
  'project_risks',
  'decision_maker',
  'timeline',
  'budget_signal',
  'why_us',
  'objections',
  'notes',
] as const;

export type BriefTopicKey = (typeof BRIEF_TOPICS)[number];

/** Подпись темы для человека: порядок и слова — как в плагине (`brief-model.ts`). */
export const BRIEF_LABELS: Record<BriefTopicKey, string> = {
  object_name: 'Объект',
  object_purpose: 'Назначение объекта',
  object_area: 'Площадь',
  customer_address: 'Адресат КП',
  planned_systems: 'Системы',
  customer_goals: 'Задачи клиента',
  customer_pains: 'Боли',
  project_risks: 'Риски',
  decision_maker: 'Кто решает',
  timeline: 'Сроки',
  budget_signal: 'Бюджет',
  why_us: 'Почему мы',
  objections: 'Возражения',
  notes: 'Прочее',
};

/** Ответ на вопрос брифа (то, что сервер отдаёт в `reply`). */
export interface BriefReplyDto {
  brief: BriefDto;
  reply: string;
  done: boolean;
  nextTopic?: string;
  retry?: boolean;
  unavailable?: boolean;
  progress?: string;
}

/** Поле брифа: строка или список (списки сервер печатает через «;»). */
export interface BriefPayloadDto {
  object_name?: string;
  object_purpose?: string;
  object_area?: string;
  customer_address?: string;
  planned_systems?: string[];
  customer_goals?: string[];
  customer_pains?: string[];
  project_risks?: string[];
  decision_maker?: string;
  timeline?: string;
  budget_signal?: string;
  why_us?: string[];
  objections?: Array<{ claim: string; response: string }>;
  notes?: string;
  skipped_topics?: string[];
  [key: string]: unknown;
}

export interface BriefTurnDto {
  id: number;
  role: string;
  text: string;
  topic: string;
  createdAt: string;
}

export interface BriefDto {
  id?: number;
  offerId?: number;
  payload?: BriefPayloadDto;
  /** open | ready | agreed | absent */
  status?: string;
  lastTopic?: string;
  updatedAt?: string;
}

export interface BriefResponseDto {
  brief: BriefDto;
  turns: BriefTurnDto[];
}

export async function loadBrief(id: number): Promise<BriefResponseDto> {
  const res = await apiRequest<BriefResponseDto>(`/offers/${id}/brief`);
  return { brief: res.brief ?? {}, turns: res.turns ?? [] };
}

/** Дописать бриф руками: сервер заменяет payload целиком (как в плагине). */
export async function saveBrief(id: number, payload: BriefPayloadDto): Promise<BriefResponseDto> {
  const res = await apiRequest<BriefResponseDto>(`/offers/${id}/brief`, {
    method: 'PUT',
    body: { payload },
  });
  return { brief: res.brief ?? {}, turns: res.turns ?? [] };
}

/**
 * Ответить на текущий вопрос брифа.
 *
 * `topic` уходит в путь: сервер отказывает 409, если вопрос уже сменился
 * (например, человек ответил в боте параллельно). Иначе слова попали бы не в
 * то поле — молча и навсегда.
 */
export async function answerBrief(id: number, topic: string, text: string): Promise<BriefReplyDto> {
  return apiRequest<BriefReplyDto>(`/offers/${id}/brief/${encodeURIComponent(topic)}`, {
    method: 'POST',
    body: { text },
  });
}

/** Пустое значение темы: пустая строка для текста, пустой список для списка. */
export function emptyPayload(): BriefPayloadDto {
  return {};
}

/** Заполнена ли тема. Возражение считается отработанным только с ответом. */
export function topicFilled(payload: BriefPayloadDto | undefined, key: BriefTopicKey): boolean {
  const value = payload?.[key];
  if (key === 'objections') {
    const list = Array.isArray(value) ? value : [];
    return list.some((item) => {
      const entry = item as { claim?: string; response?: string };
      return String(entry.claim ?? '').trim() !== '' && String(entry.response ?? '').trim() !== '';
    });
  }
  if (Array.isArray(value)) return value.length > 0;
  return typeof value === 'string' ? value.trim() !== '' : false;
}

/** Сколько тем заполнено — «6 из 14» в заголовке карточки. */
export function filledCount(payload: BriefPayloadDto | undefined): number {
  return BRIEF_TOPICS.filter((key) => topicFilled(payload, key)).length;
}

/**
 * Значение темы строкой для показа: списки — построчно, возражения —
 * «возражение → ответ» (тот же вид, что в плагине).
 */
export function topicText(payload: BriefPayloadDto | undefined, key: BriefTopicKey): string {
  const value = payload?.[key];
  if (key === 'objections') {
    const list = Array.isArray(value) ? value : [];
    return list
      .map((item) => {
        const entry = item as { claim?: string; response?: string };
        const claim = String(entry.claim ?? '').trim();
        const response = String(entry.response ?? '').trim();
        if (claim === '' && response === '') return '';
        return response === '' ? claim : `${claim} → ${response}`;
      })
      .filter((line) => line !== '')
      .join('\n');
  }
  if (Array.isArray(value)) return value.map((line) => String(line)).join('\n');
  return typeof value === 'string' ? value.trim() : '';
}

/**
 * Текст из поля обратно в значение темы.
 *
 * Вводятся СТРОКИ (одна строка — один пункт), а пустые строки отбрасываются:
 * иначе пустое поле превращалось бы в список из одной пустой строки и тема
 * выглядела заполненной. Одиночные текстовые поля сохраняют текст как есть —
 * в них бывают переносы (объект с адресом, примечания).
 */
export function textToTopic(text: string, key: BriefTopicKey): string | string[] {
  if (SINGLE_LINE_TOPICS.has(key)) return text.trim();
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line !== '');
}

const SINGLE_LINE_TOPICS: ReadonlySet<BriefTopicKey> = new Set([
  'object_name',
  'object_purpose',
  'object_area',
  'customer_address',
  'decision_maker',
  'timeline',
  'budget_signal',
]);

/** Подпись темы, даже если сервер прислал незнакомый ключ: не теряем слово. */
export function topicLabel(key: string): string {
  return (BRIEF_LABELS as Record<string, string>)[key] ?? key;
}

/** Текст «Бриф пуст» — заголовок состояния, когда ничего не заполнено. */
export function briefSummaryLine(payload: BriefPayloadDto | undefined, absent: boolean): string {
  if (absent) return 'Брифа нет';
  const filled = filledCount(payload);
  if (filled === 0) return 'Бриф пуст';
  return `Заполнено ${filled} из ${BRIEF_TOPICS.length} тем`;
}

/**
 * Вопрос, на который ждём ответ, или пустая строка.
 *
 * Сервер отдаёт `lastTopic` — тему, которую спросили последней, — а сам текс
 * вопроса в `GET /brief` не приходит (формулировки живут в сервисе). Поэтому
 * карточка спрашивает только тогда, когда тема известна, и подписывает
 * вопрос подписью темы; готовая формулировка есть в `reply` шага.
 */
export function openTopic(brief: BriefDto | undefined): string {
  if (!brief) return '';
  // Готовый бриф (ready/agreed) вопросов не задаёт, «absent» — брифа нет вовсе.
  if (brief.status === 'ready' || brief.status === 'agreed' || brief.status === 'absent') return '';
  const topic = (brief.lastTopic ?? '').trim();
  return BRIEF_TOPICS.includes(topic as BriefTopicKey) ? topic : '';
}