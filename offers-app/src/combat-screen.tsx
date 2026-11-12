/**
 * Экран боевого листка (2026-11-13, шаг S6).
 *
 * Состав: номер и кто автор → наши продукты → продукты конкурентов →
 * уточнение для разбора → аналоги. Правка позиционная: одна карточка за
 * нажатие, поэтому параллельная правка в боте не затирается.
 *
 * Чужой листок открывается на чтение: кнопки правки не показываются вовсе —
 * так человек не получает 403 на нажатие, которое выглядело доступным.
 */
import { useCallback, useEffect, useState, type JSX } from 'react';
import { ApiError } from './api.ts';
import {
  addSide,
  canEdit,
  findAnalogues,
  hintOf,
  loadSheet,
  removeSide,
  saveHint,
  searchSide,
  sheetLabelOf,
  type CombatPickerItem,
  type CombatSheetDto,
  type CombatSide,
} from './combat.ts';
import { shortDate, statusLabel } from './lists.ts';
import { SendToChatButton } from './sendchat-button.tsx';

function errorText(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 401 || e.status === 403) return 'Нет доступа к этому листку.';
    return e.message;
  }
  return 'Не удалось связаться с сервером.';
}

export function CombatCard({
  id,
  email,
  role,
}: {
  id: number;
  email: string;
  role: string;
}): JSX.Element {
  const [sheet, setSheet] = useState<CombatSheetDto | null>(null);
  const [error, setError] = useState('');
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);

  const reload = useCallback(() => {
    setSheet(null);
    loadSheet(id)
      .then((loaded) => setSheet(loaded))
      .catch((e: unknown) => setError(errorText(e)));
  }, [id]);

  useEffect(reload, [reload]);

  const run = useCallback((action: () => Promise<CombatSheetDto>, done: string) => {
    setBusy(true);
    setNote('');
    action()
      .then((next) => {
        setSheet(next);
        setNote(done);
      })
      .catch((e: unknown) => setNote(errorText(e)))
      .finally(() => setBusy(false));
  }, []);

  if (error !== '') {
    return (
      <section>
        <h1>Боевой листок #{id}</h1>
        <div className="app-card app-error">
          <p>{error}</p>
          <button className="app-button" type="button" onClick={reload}>
            Повторить
          </button>
        </div>
      </section>
    );
  }
  if (sheet === null) {
    return (
      <section>
        <p className="app-muted">Загружаем листок…</p>
      </section>
    );
  }

  const editable = canEdit(sheet, email, role);
  return (
    <section>
      <h1>
        БЛ {sheet.number} <span className="app-muted">· {statusLabel(sheet.status)}</span>
      </h1>
      <p className="app-muted">
        {sheetLabelOf(sheet)} · автор {sheet.ownerEmail} · {shortDate(sheet.updatedAt)}
      </p>
      {editable ? null : (
        <p className="app-note">
          Это листок коллеги. Смотреть и печатать можно, править — только автору.
        </p>
      )}
      {note === '' ? null : (
        <p className="app-note" role="status">
          {note}
        </p>
      )}

      <SideZone
        title="Наш продукт"
        side="our"
        refs={sheet.ourNotes}
        editable={editable}
        busy={busy}
        onAdd={(card) => run(() => addSide(id, 'our', card), `Добавлено: ${card.title}`)}
        onRemove={(noteId) => run(() => removeSide(id, 'our', noteId), 'Убрано с нашей стороны.')}
      />

      <SideZone
        title="Конкуренты"
        side="competitor"
        refs={sheet.competitorNotes}
        editable={editable}
        busy={busy}
        onAdd={(card) => run(() => addSide(id, 'competitor', card), `Добавлено: ${card.title}`)}
        onRemove={(noteId) => run(() => removeSide(id, 'competitor', noteId), 'Конкурент убран.')}
      />

      <HintZone
        sheet={sheet}
        editable={editable}
        busy={busy}
        onSave={(hint) => run(() => saveHint(id, hint), 'Уточнение сохранено.')}
      />

      <AnaloguesZone
        sheet={sheet}
        editable={editable}
        busy={busy}
        onAdd={(card) => run(() => addSide(id, 'competitor', card), `Аналог добавлен: ${card.title}`)}
      />

      <SendToChatButton kind="combat" id={id} />

      <p className="app-muted">
        <a className="app-link" href="#/list/sheets">
          К списку листков
        </a>
      </p>
    </section>
  );
}

function SideZone({
  title,
  side,
  refs,
  editable,
  busy,
  onAdd,
  onRemove,
}: {
  title: string;
  side: CombatSide;
  refs: Array<{ noteId: number; title: string }>;
  editable: boolean;
  busy: boolean;
  onAdd: (card: CombatPickerItem) => void;
  onRemove: (noteId: number) => void;
}): JSX.Element {
  return (
    <div className="app-card">
      <h2 className="app-h2">{title}</h2>
      {refs.length === 0 ? (
        <p className="app-muted">Пока пусто.</p>
      ) : (
        <ul className="app-chips">
          {refs.map((ref) => (
            <li className="app-chip" key={ref.noteId}>
              <span>{ref.title}</span>
              {editable ? (
                <button
                  className="app-chip-x"
                  type="button"
                  aria-label={`Убрать ${ref.title}`}
                  disabled={busy}
                  onClick={() => onRemove(ref.noteId)}
                >
                  ✕
                </button>
              ) : null}
            </li>
          ))}
        </ul>
      )}
      {editable ? <SideSearch side={side} busy={busy} onPick={onAdd} taken={refs.map((r) => r.noteId)} /> : null}
    </div>
  );
}

function SideSearch({
  side,
  busy,
  taken,
  onPick,
}: {
  side: CombatSide;
  busy: boolean;
  taken: number[];
  onPick: (card: CombatPickerItem) => void;
}): JSX.Element {
  const [query, setQuery] = useState('');
  const [items, setItems] = useState<CombatPickerItem[]>([]);
  const [hint, setHint] = useState('');

  useEffect(() => {
    let alive = true;
    const timer = window.setTimeout(() => {
      searchSide(side, query)
        .then((page) => {
          if (!alive) return;
          setItems(page.items ?? []);
          setHint('');
        })
        .catch((e: unknown) => {
          if (!alive) return;
          setItems([]);
          setHint(errorText(e));
        });
    }, 300);
    return () => {
      alive = false;
      window.clearTimeout(timer);
    };
  }, [side, query]);

  const set = new Set(taken);
  return (
    <div className="app-search-block">
      <input
        className="app-input"
        type="search"
        value={query}
        placeholder={side === 'our' ? 'Найти наш продукт' : 'Найти продукт конкурента'}
        aria-label={side === 'our' ? 'Поиск нашего продукта' : 'Поиск конкурента'}
        onChange={(e) => setQuery(e.target.value)}
      />
      {hint === '' ? null : <p className="app-muted">{hint}</p>}
      {items.length === 0 ? null : (
        <ul className="app-results">
          {items.map((item) => (
            <li key={item.noteId}>
              <button
                className="app-result"
                type="button"
                disabled={busy || set.has(item.noteId)}
                onClick={() => onPick(item)}
              >
                <span>{item.title}</span>
                <span className="app-muted">{set.has(item.noteId) ? 'уже в листке' : 'добавить'}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function HintZone({
  sheet,
  editable,
  busy,
  onSave,
}: {
  sheet: CombatSheetDto;
  editable: boolean;
  busy: boolean;
  onSave: (hint: string) => void;
}): JSX.Element {
  const [hint, setHint] = useState(hintOf(sheet));
  useEffect(() => setHint(hintOf(sheet)), [sheet]);
  if (!editable) {
    return (
      <div className="app-card">
        <h2 className="app-h2">Уточнение для разбора</h2>
        <p className={hint === '' ? 'app-muted' : ''}>{hint === '' ? 'Не задано.' : hint}</p>
      </div>
    );
  }
  const dirty = hint !== hintOf(sheet);
  return (
    <div className="app-card">
      <h2 className="app-h2">Уточнение для разбора</h2>
      <p className="app-muted">На чём сравнение должно сделать акцент.</p>
      <textarea
        className="app-textarea"
        rows={3}
        value={hint}
        aria-label="Уточнение для разбора"
        onChange={(e) => setHint(e.target.value)}
      />
      <button className="app-button" type="button" disabled={busy || !dirty} onClick={() => onSave(hint)}>
        {dirty ? 'Сохранить уточнение' : 'Сохранено'}
      </button>
    </div>
  );
}

function AnaloguesZone({
  sheet,
  editable,
  busy,
  onAdd,
}: {
  sheet: CombatSheetDto;
  editable: boolean;
  busy: boolean;
  onAdd: (card: CombatPickerItem) => void;
}): JSX.Element {
  const [cards, setCards] = useState<CombatPickerItem[] | null>(null);
  const [hint, setHint] = useState('');
  const [loading, setLoading] = useState(false);
  const source = sheet.ourNotes?.[sheet.ourNotes.length - 1];

  if (!editable) return <></>;

  const search = (): void => {
    if (!source) return;
    setLoading(true);
    setHint('');
    findAnalogues(source.noteId)
      .then((found) => setCards(found))
      .catch((e: unknown) => setHint(errorText(e)))
      .finally(() => setLoading(false));
  };

  return (
    <div className="app-card">
      <h2 className="app-h2">Аналоги</h2>
      {source === undefined ? (
        <p className="app-muted">Сначала добавьте наш продукт — аналоги ищутся от него.</p>
      ) : (
        <>
          <p className="app-muted">От продукта «{source.title}» найдём конкурентов того же класса.</p>
          <button
            className="app-button app-button-ghost"
            type="button"
            disabled={busy || loading}
            onClick={search}
          >
            {loading ? '⌛ Ищем…' : '🔍 Найти аналоги'}
          </button>
          {hint === '' ? null : <p className="app-error-text">{hint}</p>}
          {cards !== null && cards.length === 0 ? (
            <p className="app-muted">Аналогов не нашлось — уточните продукт или перечитайте конкурентов.</p>
          ) : null}
          {cards === null || cards.length === 0 ? null : (
            <ul className="app-results">
              {cards.map((card) => (
                <li key={card.noteId}>
                  <button
                    className="app-result"
                    type="button"
                    disabled={busy}
                    onClick={() => onAdd(card)}
                  >
                    <span>{card.title}</span>
                    <span className="app-muted">в конкуренты</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </>
      )}
    </div>
  );
}