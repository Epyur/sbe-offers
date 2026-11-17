/**
 * Блок «Бриф» в карточке КП (2026-11-15).
 *
 * Что видит менеджер:
 *   - заголовок и «заполнено N из 14 тем»;
 *   - транспортные читаемые строки: объект, задачи, боли, риски, бюджет, сроки…
 *     каждая тема — подпись и значение (или честное «не заполнено»);
 *   - если вопрос открыт — поле ответа и кнопка «Ответить»: шаг диалога тот
 *     же, что в боте (`POST /offers/{id}/brief/{topic}`), только под JWT;
 *   - «✏️ Продолжить бриф в боте» — разговор голосом в чате;
 *   - «✏️ Править бриф» — ручная правка полей (та же ручка, что у плагина).
 *
 * Чего здесь СОЗНАТЕЛЬНО нет: второй формы вопроса. Формулировки вопросов
 * живут в сервисе (`skills.BriefTopics`), и копия в приложении разошлась бы с
 * ботом при первой же правке.
 */
import { useCallback, useEffect, useState, type JSX } from 'react';
import { ApiError } from './api.ts';
import {
  answerBrief,
  BRIEF_TOPICS,
  briefSummaryLine,
  emptyPayload,
  filledCount,
  loadBrief,
  openTopic,
  saveBrief,
  textToTopic,
  topicFilled,
  topicLabel,
  topicText,
  type BriefDto,
  type BriefPayloadDto,
  type BriefTurnDto,
} from './brief.ts';
import { askBot } from './ask.ts';

function errorText(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 401 || e.status === 403) return 'Нет доступа к этому предложению.';
    if (e.status === 409) return 'Вопрос уже сменился — обновите бриф и ответьте заново.';
    return e.message;
  }
  return 'Не удалось связаться с сервером.';
}

type LoadState =
  | { kind: 'loading' }
  | { kind: 'error'; text: string }
  | { kind: 'ready'; brief: BriefDto; turns: BriefTurnDto[] };

export function BriefContent({ id }: { id: number }): JSX.Element {
  const [state, setState] = useState<LoadState>({ kind: 'loading' });
  const [editing, setEditing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState('');
  const [askNote, setAskNote] = useState('');

  const reload = useCallback(() => {
    setState({ kind: 'loading' });
    loadBrief(id)
      .then((res) => setState({ kind: 'ready', brief: res.brief, turns: res.turns }))
      .catch((e: unknown) => setState({ kind: 'error', text: errorText(e) }));
  }, [id]);

  useEffect(reload, [reload]);

  const askContinue = (): void => {
    setBusy(true);
    setAskNote('');
    askBot('brief', 'offer', id)
      .then((res) => setAskNote(res.problem === '' ? res.sent : res.problem))
      .finally(() => setBusy(false));
  };

  if (state.kind === 'loading') {
    return (
      <div className="app-card">
        <h2 className="app-h2">Бриф</h2>
        <p className="app-muted">Загружаем бриф…</p>
      </div>
    );
  }
  if (state.kind === 'error') {
    return (
      <div className="app-card">
        <h2 className="app-h2">Бриф</h2>
        <p className="app-error-text">{state.text}</p>
        <button className="app-button app-button-ghost" type="button" onClick={reload}>
          Повторить
        </button>
      </div>
    );
  }

  if (editing) {
    return (
      <BriefEdit
        id={id}
        brief={state.brief}
        onCancel={() => setEditing(false)}
        onSaved={(res) => {
          setState({ kind: 'ready', brief: res.brief, turns: res.turns });
          setEditing(false);
          setNote('Бриф сохранён.');
        }}
      />
    );
  }

  const { brief, turns } = state;
  const payload = brief.payload ?? emptyPayload();
  const absent = brief.status === 'absent';
  const topic = openTopic(brief);
  const skipped = (payload.skipped_topics ?? []).map((key) => topicLabel(String(key)));
  return (
    <div className="app-card">
      <h2 className="app-h2">
        Бриф <span className="app-muted">{briefSummaryLine(payload, absent)}</span>
      </h2>
      {note === '' ? null : (
        <p className="app-note" role="status">
          {note}
        </p>
      )}
      {askNote === '' ? null : (
        <p className="app-note" role="status">
          {askNote}
        </p>
      )}
      {absent ? (
        <p className="app-muted">
          Бриф не заполнен: КП собрано без разговора в боте. Заполните здесь или продолжите бриф в чате.
        </p>
      ) : null}
      {skipped.length === 0 ? null : <p className="app-muted">Не отвечали: {skipped.join(', ')}.</p>}

      {topic === '' ? null : <BriefAnswer id={id} topic={topic} onStep={handleStep} onError={setNote} />}

      <ul className="app-brief-fields">
        {BRIEF_TOPICS.map((key) => (
          <li className="app-brief-field" key={key}>
            <span className="app-brief-label">{topicLabel(key)}</span>
            {topicFilled(payload, key) ? (
              topicText(payload, key)
                .split('\n')
                .map((line, index) => (
                  <span className="app-brief-value" key={index}>
                    {line}
                  </span>
                ))
            ) : (
              <span className="app-muted">не заполнено</span>
            )}
          </li>
        ))}
      </ul>

      <div className="app-item-row">
        <button className="app-button app-button-ghost" type="button" disabled={busy} onClick={askContinue}>
          {busy ? '⌛ Пишем в чат…' : '✏️ Продолжить бриф в боте'}
        </button>
        <button className="app-button app-button-ghost" type="button" onClick={() => setEditing(true)}>
          ✏️ Править бриф
        </button>
      </div>
      <BriefTurns turns={turns} />
    </div>
  );

  function handleStep(res: { brief: BriefDto; reply: string; done: boolean }): void {
    setState((prev) =>
      prev.kind === 'ready'
        ? { kind: 'ready', brief: res.brief, turns: res.done ? prev.turns : prev.turns }
        : prev,
    );
    setNote(res.done ? res.reply : `${res.reply}`);
    // История разговора изменилась — перечитываем её вместе с брифом.
    reloadAfterStep();
  }

  function reloadAfterStep(): void {
    loadBrief(id)
      .then((res) => setState({ kind: 'ready', brief: res.brief, turns: res.turns }))
      .catch(() => {
        /* экран уже показывает ответ; тихая ошибка перечитывания не стирает его */
      });
  }
}

/**
 * Шторка «Бриф» из карточки раздела (0.11.3).
 *
 * Пользователь попросил вызывать бриф ПО КАЖДОМУ разделу: он стоит в
 * «Преимуществах», видит слабый текст и хочет дополнить контекст, не уходя
 * наверх. Бриф при этом ОДИН на КП (решение 2026-11-16): четыре входа в один и
 * тот же набор вопросов, а не четыре разных брифа.
 *
 * Заголовок честно говорит, что бриф общий и откуда его вызвали: иначе менеджер
 * решит, что правит только этот раздел.
 *
 * Бриф спрашивает ТОЛЬКО незаполненное (так устроен `StepBrief` на сервере:
 * `nextOpenTopic` берёт первую пустую тему), а после последнего ответа
 * показывает сводку и предлагает просмотреть и поправить самому — это и есть
 * просьба пользователя «предлагаем ознакомиться и указать, что скорректировать».
 */
export function BriefSheet({
  id,
  sectionLabel,
  onClose,
}: {
  id: number;
  /** Из какого раздела вызвали — для заголовка, не для фильтра данных. */
  sectionLabel: string;
  onClose: () => void;
}): JSX.Element {
  return (
    <div className="app-sheet-backdrop" role="presentation" onClick={onClose}>
      <div
        className="app-sheet"
        role="dialog"
        aria-modal="true"
        aria-label="Бриф КП"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="app-sheet-head">
          <span className="app-sheet-title">Бриф КП</span>
          <button className="app-link-button" type="button" onClick={onClose} aria-label="Закрыть бриф">
            ✕
          </button>
        </div>
        <p className="app-muted">
          Общий контекст для всех разделов. Открыт из раздела «{sectionLabel}» — изменения учтутся при
          следующей генерации.
        </p>
        <BriefContent id={id} />
      </div>
    </div>
  );
}

/** Поле ответа на открытый вопрос: шаг диалога — тот же, что в боте. */
function BriefAnswer({
  id,
  topic,
  onStep,
  onError,
}: {
  id: number;
  topic: string;
  onStep: (res: { brief: BriefDto; reply: string; done: boolean }) => void;
  onError: (text: string) => void;
}): JSX.Element {
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);

  const send = (): void => {
    if (text.trim() === '') return;
    setBusy(true);
    onError('');
    answerBrief(id, topic, text)
      .then((res) => {
        setText('');
        onStep({ brief: res.brief, reply: res.reply, done: res.done });
      })
      .catch((e: unknown) => onError(errorText(e)))
      .finally(() => setBusy(false));
  };

  return (
    <div className="app-brief-answer">
      <label className="app-field">
        <span className="app-field-label">Вопрос: {topicLabel(topic)}</span>
        <textarea
          className="app-textarea"
          rows={2}
          value={text}
          aria-label={`Ответ на вопрос ${topicLabel(topic)}`}
          onChange={(e) => setText(e.target.value)}
        />
      </label>
      <button className="app-button" type="button" disabled={busy || text.trim() === ''} onClick={send}>
        {busy ? '⌛ Отвечаем…' : 'Ответить'}
      </button>
    </div>
  );
}

/** Ручная правка: те же темы, одна строка — один пункт (как в плагине). */
function BriefEdit({
  id,
  brief,
  onCancel,
  onSaved,
}: {
  id: number;
  brief: BriefDto;
  onCancel: () => void;
  onSaved: (res: { brief: BriefDto; turns: BriefTurnDto[] }) => void;
}): JSX.Element {
  const payload = brief.payload ?? emptyPayload();
  const [draft, setDraft] = useState<Record<string, string>>(() => {
    const out: Record<string, string> = {};
    for (const key of BRIEF_TOPICS) out[key] = topicText(payload, key);
    return out;
  });
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState('');

  const save = (): void => {
    setBusy(true);
    setProblem('');
    const next: Record<string, unknown> = { ...payload };
    for (const key of BRIEF_TOPICS) {
      next[key] = textToTopic(draft[key] ?? '', key);
    }
    saveBrief(id, next as BriefPayloadDto)
      .then((res) => onSaved(res))
      .catch((e: unknown) => setProblem(errorText(e)))
      .finally(() => setBusy(false));
  };

  return (
    <div className="app-card">
      <h2 className="app-h2">Бриф — правка</h2>
      <p className="app-muted">Одна строка — один пункт. Возражения записывайте как «возражение → ответ».</p>
      {BRIEF_TOPICS.map((key) => (
        <label className="app-field" key={key}>
          <span className="app-field-label">{topicLabel(key)}</span>
          <textarea
            className="app-textarea"
            rows={2}
            value={draft[key] ?? ''}
            aria-label={topicLabel(key)}
            onChange={(e) => setDraft({ ...draft, [key]: e.target.value })}
          />
        </label>
      ))}
      {problem === '' ? null : <p className="app-error-text">{problem}</p>}
      <div className="app-item-row">
        <button className="app-button" type="button" disabled={busy} onClick={save}>
          {busy ? 'Сохраняем…' : 'Сохранить бриф'}
        </button>
        <button className="app-button app-button-ghost" type="button" disabled={busy} onClick={onCancel}>
          Отмена
        </button>
      </div>
    </div>
  );
}

/** История разговора: свёрнута по умолчанию, как в плагине. */
function BriefTurns({ turns }: { turns: BriefTurnDto[] }): JSX.Element | null {
  if (turns.length === 0) return null;
  return (
    <details className="app-brief-turns">
      <summary>История разговора ({turns.length})</summary>
      <ul className="app-brief-turns-list">
        {turns.map((turn) => (
          <li className="app-brief-turn" key={turn.id}>
            <span className="app-muted">{turn.role === 'manager' ? 'Менеджер' : 'Бот'}: </span>
            {turn.text}
          </li>
        ))}
      </ul>
    </details>
  );
}

/** Сколько всего тем — для экранов, которым нужен масштаб («6 из 14»). */
export const BRIEF_TOTAL = BRIEF_TOPICS.length;

export { filledCount };