import { Notice } from 'obsidian';
import type { OffersView } from './offers-view';
import type { BriefObjection, BriefPayload, BriefResponse } from '../types/offers';
import { emptyBriefPayload } from '../types/offers';
import { errorMessage } from '../../../sbe-core/src/utils/errors';
import {
  BRIEF_FIELDS,
  briefStatusLine,
  fieldAsText,
  filledCount,
  isFilled,
  skippedLabels,
  textToField,
} from './brief-model';

/**
 * «Бриф КП» — что бот выяснил про объект (Task 10 плана 2026-10-29).
 *
 * Бриф привязан к предложению, поэтому живёт не отдельной вкладкой, а карточкой
 * внутри предложения. Здесь два режима: чтение (как прошёл разговор) и правка
 * (менеджер дописывает руками — бот спросить уже не успел).
 *
 * Правка — по кнопке, а не сразу: бриф это итог разговора, и случайная правка
 * одного поля не должна переписывать историю.
 */
export async function renderBriefCard(
  view: OffersView,
  container: HTMLElement,
  offerId: number,
): Promise<void> {
  const card = container.createDiv({ cls: 'tn-offers-card tn-offers-brief' });
  card.createDiv({ cls: 'tn-offers-brief-title', text: 'Бриф' });
  const hint = card.createDiv({ cls: 'tn-offers-muted', text: 'Загрузка брифа…' });

  let data: BriefResponse;
  try {
    data = await view.plugin.offersService.getBrief(offerId);
  } catch (e: unknown) {
    hint.setText(`Не удалось загрузить бриф: ${errorMessage(e)}`);
    return;
  }

  renderLoaded(view, card, offerId, data);
}

function renderLoaded(
  view: OffersView,
  card: HTMLElement,
  offerId: number,
  data: BriefResponse,
): void {
  card.empty();
  card.createDiv({ cls: 'tn-offers-brief-title', text: 'Бриф' });

  const payload = data.brief.payload ?? emptyBriefPayload();
  const absent = data.brief.status === 'absent';

  const head = card.createDiv({ cls: 'tn-offers-brief-head' });
  head.createSpan({ cls: 'tn-offers-muted', text: briefStatusLine(payload, absent) });
  head.createDiv({ cls: 'tn-offers-spacer' });

  const skipped = skippedLabels(payload);
  if (skipped.length > 0) {
    card.createDiv({
      cls: 'tn-offers-warn',
      text: `Не отвечали: ${skipped.join(', ')}.`,
    });
  }

  if (absent) {
    card.createDiv({
      cls: 'tn-offers-muted',
      text:
        'КП собрано без разговора в боте. Бриф заполняется, когда менеджер отвечает ' +
        'на вопросы в Telegram — или вручную здесь.',
    });
  }

  const fieldsEl = card.createDiv({ cls: 'tn-offers-brief-fields' });
  renderReadFields(fieldsEl, payload);

  const actions = card.createDiv({ cls: 'tn-offers-actions' });
  const editBtn = actions.createEl('button', { text: '✏️ Править бриф', cls: 'tn-btn tn-btn-ghost' });
  editBtn.addEventListener('click', () => {
    renderEdit(view, card, offerId, data);
  });

  renderTurns(card, data);
}

/** Чтение: каждое поле с подписью, пустые честно помечены. */
function renderReadFields(container: HTMLElement, payload: BriefPayload): void {
  container.empty();
  for (const field of BRIEF_FIELDS) {
    const row = container.createDiv({ cls: 'tn-offers-brief-field' });
    row.createDiv({ cls: 'tn-offers-brief-label', text: field.label });
    const text = fieldAsText(payload, field.key);
    if (text === '') {
      row.createDiv({ cls: 'tn-offers-muted tn-offers-brief-empty', text: 'не заполнено' });
      continue;
    }
    const body = row.createDiv({ cls: 'tn-offers-brief-value' });
    for (const line of text.split('\n')) {
      body.createDiv({ text: line });
    }
  }
}

/** Правка: те же поля, но в textarea, плюс «Сохранить» / «Отмена». */
function renderEdit(
  view: OffersView,
  card: HTMLElement,
  offerId: number,
  data: BriefResponse,
): void {
  card.empty();
  card.createDiv({ cls: 'tn-offers-brief-title', text: 'Бриф — правка' });
  card.createDiv({
    cls: 'tn-offers-muted',
    text: 'Одна строка — один пункт. Возражения записывайте как «возражение → ответ».',
  });

  const payload = data.brief.payload ?? emptyBriefPayload();
  const draft: BriefPayload = clonePayload(payload);
  const fieldsEl = card.createDiv({ cls: 'tn-offers-brief-fields' });

  for (const field of BRIEF_FIELDS) {
    const row = fieldsEl.createDiv({ cls: 'tn-offers-brief-field' });
    row.createDiv({ cls: 'tn-offers-brief-label', text: field.label });
    const value = fieldAsText(payload, field.key);
    if (field.multiline || field.key === 'objections') {
      const area = row.createEl('textarea', {
        cls: 'tn-offers-input tn-offers-brief-area',
        attr: { rows: '3' },
      });
      area.value = value;
      area.addEventListener('input', () => {
        applyField(draft, field.key, area.value);
      });
    } else {
      const input = row.createEl('input', { cls: 'tn-offers-input' });
      input.value = value;
      input.addEventListener('input', () => {
        applyField(draft, field.key, input.value);
      });
    }
  }

  const actions = card.createDiv({ cls: 'tn-offers-actions' });
  const save = actions.createEl('button', { text: '💾 Сохранить бриф', cls: 'tn-btn tn-btn-primary' });
  const cancel = actions.createEl('button', { text: 'Отмена', cls: 'tn-btn tn-btn-ghost' });
  save.addEventListener('click', () => {
    void saveBrief(view, card, offerId, draft, save);
  });
  cancel.addEventListener('click', () => {
    renderLoaded(view, card, offerId, data);
  });
}

/** Записать текст поля в черновик брифа. */
function applyField(draft: BriefPayload, key: keyof BriefPayload, text: string): void {
  if (key === 'objections') {
    draft.objections = parseObjections(text);
    return;
  }
  const value = textToField(text, key);
  if (Array.isArray(value)) {
    (draft[key] as string[]) = value;
  } else {
    (draft[key] as string) = value;
  }
}

/** «возражение → ответ» в список возражений. Строка без ответа остаётся как есть. */
export function parseObjections(text: string): BriefObjection[] {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line !== '')
    .map((line) => {
      const parts = line.split('→');
      const claim = (parts[0] ?? '').trim();
      const response = parts.length > 1 ? parts.slice(1).join('→').trim() : '';
      return { claim, response };
    })
    .filter((o) => o.claim !== '' || o.response !== '');
}

/** Копия брифа для правки: без общего состояния с загруженным ответом. */
function clonePayload(payload: BriefPayload): BriefPayload {
  return {
    object_name: payload.object_name ?? '',
    customer_pains: [...(payload.customer_pains ?? [])],
    customer_goals: [...(payload.customer_goals ?? [])],
    project_risks: [...(payload.project_risks ?? [])],
    objections: (payload.objections ?? []).map((o) => ({ ...o })),
    budget_signal: payload.budget_signal ?? '',
    decision_maker: payload.decision_maker ?? '',
    timeline: payload.timeline ?? '',
    why_us: [...(payload.why_us ?? [])],
    notes: payload.notes ?? '',
    skipped_topics: [...(payload.skipped_topics ?? [])],
  };
}

async function saveBrief(
  view: OffersView,
  card: HTMLElement,
  offerId: number,
  payload: BriefPayload,
  button: HTMLButtonElement,
): Promise<void> {
  button.disabled = true;
  button.setText('Сохраняю…');
  try {
    const fresh = await view.plugin.offersService.saveBrief(offerId, payload);
    new Notice(`Бриф сохранён · заполнено ${filledCount(fresh.brief.payload)} из ${BRIEF_FIELDS.length}`);
    renderLoaded(view, card, offerId, fresh);
  } catch (e: unknown) {
    new Notice(`Не удалось сохранить бриф: ${errorMessage(e)}`);
    button.disabled = false;
    button.setText('💾 Сохранить бриф');
  }
}

/** История разговора: что спросили и что ответили. Свёрнута по умолчанию. */
function renderTurns(card: HTMLElement, data: BriefResponse): void {
  if (!data.turns || data.turns.length === 0) return;
  const details = card.createEl('details', { cls: 'tn-offers-brief-turns' });
  details.createEl('summary', { text: `История разговора (${data.turns.length})` });
  for (const turn of data.turns) {
    const row = details.createDiv({ cls: 'tn-offers-brief-turn' });
    row.createSpan({
      cls: `tn-offers-brief-who${turn.role === 'manager' ? ' is-manager' : ''}`,
      text: turn.role === 'manager' ? 'Менеджер' : 'Бот',
    });
    row.createSpan({ cls: 'tn-offers-muted', text: formatDateTime(turn.createdAt) });
    row.createDiv({ cls: 'tn-offers-brief-turn-text', text: turn.text });
  }
}

/** Дата и время реплики: 29.10.2026 14:05. */
function formatDateTime(raw: string): string {
  if (!raw) return '';
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return raw;
  const pad = (n: number): string => String(n).padStart(2, '0');
  return `${pad(d.getDate())}.${pad(d.getMonth() + 1)}.${d.getFullYear()} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export { isFilled };