/**
 * Экран текстов разделов КП (2026-11-13, шаг S4).
 *
 * Одна карточка на каждый слот, порядок — серверный. У слота три действия:
 * «Написать» (модель), «Выправить» (модель вычитывает то, что человек уже
 * правил), «Принять» / «Пропустить». Правки живут в черновике экрана и
 * уходят на сервер только по «Принять» — как в боте, где менеджер сначала
 * читает, что получилось.
 */
import { useCallback, useEffect, useState, type JSX } from 'react';
import { ApiError } from './api.ts';
import {
  acceptSection,
  editSection,
  filledCount,
  loadSections,
  withCurrent,
  writeSection,
  type OfferSectionDto,
} from './sections.ts';

function errorText(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 401 || e.status === 403) return 'Нет доступа к этому предложению.';
    return e.message;
  }
  return 'Не удалось связаться с сервером.';
}

export function SectionsZone({ id }: { id: number }): JSX.Element {
  const [sections, setSections] = useState<OfferSectionDto[] | null>(null);
  const [error, setError] = useState('');
  const [busyKey, setBusyKey] = useState('');

  useEffect(() => {
    let alive = true;
    loadSections(id)
      .then((list) => {
        if (alive) setSections(list);
      })
      .catch((e: unknown) => {
        if (alive) {
          setSections([]);
          setError(errorText(e));
        }
      });
    return () => {
      alive = false;
    };
  }, [id]);

  const replace = useCallback((next: OfferSectionDto) => {
    setSections((prev) => (prev === null ? prev : withCurrent(prev, next)));
  }, []);

  if (sections === null) return <p className="app-muted">Загружаем разделы…</p>;
  if (sections.length === 0) {
    return (
      <div className="app-card">
        <h2 className="app-h2">Тексты разделов</h2>
        <p className="app-muted">{error === '' ? 'Разделов нет: выберите систему.' : error}</p>
      </div>
    );
  }

  return (
    <div className="app-card">
      <h2 className="app-h2">
        Тексты разделов{' '}
        <span className="app-muted">
          {filledCount(sections)} из {sections.length}
        </span>
      </h2>
      {error === '' ? null : <p className="app-error-text">{error}</p>}
      {sections.map((section) => (
        <SectionCard
          key={section.key}
          id={id}
          section={section}
          busy={busyKey}
          onBusy={setBusyKey}
          onReplace={replace}
          onError={setError}
        />
      ))}
    </div>
  );
}

function SectionCard({
  id,
  section,
  busy,
  onBusy,
  onReplace,
  onError,
}: {
  id: number;
  section: OfferSectionDto;
  busy: string;
  onBusy: (key: string) => void;
  onReplace: (next: OfferSectionDto) => void;
  onError: (text: string) => void;
}): JSX.Element {
  const [draft, setDraft] = useState(section.text);

  useEffect(() => setDraft(section.text), [section.text]);

  const run = (action: () => Promise<void>): void => {
    onBusy(section.key);
    onError('');
    action()
      .catch((e: unknown) => onError(errorText(e)))
      .finally(() => onBusy(''));
  };

  const isBusy = busy === section.key;
  const dirty = draft !== section.text;

  return (
    <div className="app-section">
      <div className="app-section-head">
        <span className="app-section-label">{section.label}</span>
        <span className="app-muted">{section.text === '' ? 'не заполнено' : 'заполнено'}</span>
      </div>
      {section.warnings.length === 0 ? null : (
        <ul className="app-warnings">
          {section.warnings.map((warning) => (
            <li key={warning}>{warning}</li>
          ))}
        </ul>
      )}
      <textarea
        className="app-textarea"
        rows={4}
        value={draft}
        aria-label={section.label}
        onChange={(e) => setDraft(e.target.value)}
      />
      <div className="app-item-row">
        <button
          className="app-button app-button-ghost"
          type="button"
          disabled={isBusy}
          onClick={() =>
            run(async () => {
              const next = await writeSection(id, section.key);
              setDraft(next.text);
              onReplace(next);
            })
          }
        >
          {isBusy ? '⌛ Пишем…' : 'Написать'}
        </button>
        <button
          className="app-button app-button-ghost"
          type="button"
          disabled={isBusy || draft.trim() === ''}
          onClick={() =>
            run(async () => {
              const next = await editSection(id, section.key, draft);
              setDraft(next.text);
              onReplace(next);
            })
          }
        >
          Выправить
        </button>
        <button
          className="app-button"
          type="button"
          disabled={isBusy || (!dirty && section.text !== '')}
          onClick={() =>
            run(async () => {
              await acceptSection(id, section.key, draft);
              onReplace({ ...section, text: draft.trim(), warnings: [] });
            })
          }
        >
          {dirty || section.text === '' ? 'Принять' : 'Принято'}
        </button>
        <button
          className="app-link-button"
          type="button"
          disabled={isBusy}
          onClick={() =>
            run(async () => {
              await acceptSection(id, section.key, '');
              setDraft('');
              onReplace({ ...section, text: '', warnings: [] });
            })
          }
        >
          Пропустить
        </button>
      </div>
    </div>
  );
}