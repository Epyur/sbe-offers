/**
 * Экраны приложения.
 *
 * S0 — заглушки карточек, S1 — вход, S2 — списки КП и боевых листков с
 * поиском по заказчику / названию (2026-11-13). Карточки документов —
 * шаги S3 (КП) и S6 (боевой листок), поэтому они пока только называют номер.
 *
 * Список одинаков для двух видов: разница — адрес, подсказка поиска, подпись
 * строки и текст пустого состояния (`lists.ts`). Второй почти такой же экран
 * был бы второй копией правила — поэтому он один.
 */
import { useCallback, useEffect, useState, type JSX } from 'react';
import { ApiError } from './api.ts';
import {
  emptyText,
  loadOffers,
  loadSheets,
  numberedLabel,
  offerLabel,
  searchPlaceholder,
  sheetLabel,
  shortDate,
  statusLabel,
  type OfferRow,
  type SheetRow,
} from './lists.ts';
import { CombatCard } from './combat-screen.tsx';
import { OfferCard } from './offer-screen.tsx';
import { routeHash, type Route } from './router.ts';

type ListKind = 'offers' | 'sheets';

interface ListState<T> {
  kind: 'loading' | 'ready' | 'error';
  rows: T[];
  error: string;
}

/** Задержка поиска: человек набирает слово, а не заводит по запросу на букву. */
const SEARCH_DEBOUNCE_MS = 300;

function useList<T>(kind: ListKind, query: string, load: (q: string) => Promise<T[]>): ListState<T> {
  const [state, setState] = useState<ListState<T>>({ kind: 'loading', rows: [], error: '' });

  useEffect(() => {
    let alive = true;
    // Запросов ещё нет — не затираем уже показанный список «загрузкой».
    void load(query)
      .then((rows) => {
        if (alive) setState({ kind: 'ready', rows, error: '' });
      })
      .catch((e: unknown) => {
        if (!alive) return;
        setState({ kind: 'error', rows: [], error: errorText(e) });
      });
    return () => {
      alive = false;
    };
    // `kind` меняется вместе с `load`; зависимость от адреса — по `query`.
  }, [kind, query]); // eslint-disable-line react-hooks/exhaustive-deps

  return state;
}

function errorText(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 401 || e.status === 403) {
      return 'Нет доступа: войдите в бота и откройте приложение из чата заново.';
    }
    return e.message;
  }
  return 'Не удалось связаться с сервером. Проверьте связь и повторите.';
}

/** Поиск: поле только меняет `q` в адресе — запрос делает экран списка. */
function SearchBox({ kind, query, onQuery }: { kind: ListKind; query: string; onQuery: (q: string) => void }): JSX.Element {
  const [value, setValue] = useState(query);
  const submit = useCallback((q: string) => onQuery(q), [onQuery]);

  // Адрес поменяли извне (перешли на другой список) — поле следует за ним.
  useEffect(() => setValue(query), [query]);

  useEffect(() => {
    if (value === query) return;
    const timer = window.setTimeout(() => submit(value), SEARCH_DEBOUNCE_MS);
    return () => window.clearTimeout(timer);
  }, [value, query, submit]);

  return (
    <div className="app-search">
      <input
        className="app-input"
        type="search"
        inputMode="search"
        value={value}
        placeholder={searchPlaceholder(kind)}
        aria-label="Поиск"
        onChange={(e) => setValue(e.target.value)}
      />
      {value === '' ? null : (
        <button className="app-clear" type="button" aria-label="Сбросить поиск" onClick={() => setValue('')}>
          ✕
        </button>
      )}
    </div>
  );
}

function OffersRow({ offer }: { offer: OfferRow }): JSX.Element {
  return (
    <a className="app-row" href={routeHash({ name: 'offer', id: offer.id })}>
      <span className="app-row-main">{offerLabel(offer)}</span>
      <span className="app-row-side">{numberedLabel('offers', offer.number)}</span>
      <span className="app-row-meta">
        {statusLabel(offer.status)} · {shortDate(offer.createdAt)}
      </span>
    </a>
  );
}

function SheetsRow({ sheet }: { sheet: SheetRow }): JSX.Element {
  return (
    <a className="app-row" href={routeHash({ name: 'sheet', id: sheet.id })}>
      <span className="app-row-main">{sheetLabel(sheet)}</span>
      <span className="app-row-side">{numberedLabel('sheets', sheet.number)}</span>
      <span className="app-row-meta">
        {statusLabel(sheet.status)} · {shortDate(sheet.createdAt)}
        {sheet.ownerEmail === '' ? null : ` · ${sheet.ownerEmail}`}
      </span>
    </a>
  );
}

function ListScreen<T>({
  kind,
  query,
  title,
  state,
  renderRow,
}: {
  kind: ListKind;
  query: string;
  title: string;
  state: ListState<T>;
  renderRow: (row: T) => JSX.Element;
}): JSX.Element {
  const otherHref = kind === 'offers' ? '#/list/sheets' : '#/list/offers';
  const onQuery = useCallback((q: string) => goList(kind, q), [kind]);
  return (
    <section>
      <h1>{title}</h1>
      <SearchBox kind={kind} query={query} onQuery={onQuery} />
      {state.kind === 'loading' ? <p className="app-muted">Загружаем…</p> : null}
      {state.kind === 'error' ? (
        <div className="app-card app-error">
          <p>{state.error}</p>
          <button className="app-button" type="button" onClick={() => goList(kind, query)}>
            Повторить
          </button>
        </div>
      ) : null}
      {state.kind === 'ready' && state.rows.length === 0 ? (
        <p className="app-muted">{emptyText(kind, query)}</p>
      ) : null}
      {state.kind === 'ready' && state.rows.length > 0 ? (
        <div className="app-list">{state.rows.map((row) => renderRow(row))}</div>
      ) : null}
      {state.kind === 'ready' && query.trim() !== '' ? (
        <p className="app-muted">Найдено: {state.rows.length}</p>
      ) : null}
      <p className="app-muted">
        <a className="app-link" href={otherHref}>
          {kind === 'offers' ? 'Боевые листки' : 'Коммерческие предложения'}
        </a>
      </p>
    </section>
  );
}

/** Переход по списку: новый `q` живёт в адресе, а не в состоянии экрана. */
export function goList(kind: ListKind, query: string): void {
  window.location.hash = routeHash({ name: 'list', kind, query }).replace(/^.*#/, '#');
}

export function OffersListScreen({ query }: { query: string }): JSX.Element {
  const state = useList<OfferRow>('offers', query, loadOffers);
  return (
    <ListScreen
      kind="offers"
      query={query}
      title="Коммерческие предложения"
      state={state}
      renderRow={(offer) => <OffersRow offer={offer} />}
    />
  );
}

export function SheetsListScreen({ query }: { query: string }): JSX.Element {
  const state = useList<SheetRow>('sheets', query, loadSheets);
  return (
    <ListScreen
      kind="sheets"
      query={query}
      title="Боевые листки"
      state={state}
      renderRow={(sheet) => <SheetsRow sheet={sheet} />}
    />
  );
}

export function OfferScreen({ id }: { id: number }): JSX.Element {
  return <OfferCard id={id} />;
}

export function SheetScreen({
  id,
  email,
  role,
}: {
  id: number;
  email: string;
  role: string;
}): JSX.Element {
  return <CombatCard id={id} email={email} role={role} />;
}

export function NotFoundScreen({ hash }: { hash: string }): JSX.Element {
  return (
    <section>
      <h1>Экран не найден</h1>
      <div className="app-card">
        <p>Адрес не распознан:</p>
        <p className="app-muted">
          <code>{hash === '' ? '(пусто)' : hash}</code>
        </p>
        <a className="app-link" href="#/list/offers">
          Открыть список КП
        </a>
      </div>
    </section>
  );
}

/** Кто вошёл: листку нужно знать, показывать ли кнопки правки. */
export interface Viewer {
  email: string;
  role: string;
}

export function screenFor(route: Route, viewer: Viewer): JSX.Element {
  switch (route.name) {
    case 'list':
      return route.kind === 'offers' ? <OffersListScreen query={route.query} /> : <SheetsListScreen query={route.query} />;
    case 'offer':
      return <OfferScreen id={route.id} />;
    case 'sheet':
      return <SheetScreen id={route.id} email={viewer.email} role={viewer.role} />;
    case 'notFound':
      return <NotFoundScreen hash={route.hash} />;
  }
}