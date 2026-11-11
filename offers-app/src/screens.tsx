/**
 * Экраны-заглушки (2026-11-13, S0).
 *
 * Каждый экран уже получает те данные, которые у него будут в своём шаге
 * (S2 — списки и поиск, S3 — карточка КП, S6 — боевой листок). Сейчас они
 * только показывают, что маршрут распознан: это доказательство S0 и точка,
 * от которой начинается каждый следующий шаг.
 */
import type { JSX } from 'react';
import type { Route } from './router.ts';

export function OffersListScreen({ query }: { query: string }): JSX.Element {
  return (
    <section>
      <h1>Коммерческие предложения</h1>
      <div className="app-card">
        <p>Список КП с поиском по клиенту — шаг S2.</p>
        {query === '' ? null : (
          <p className="app-muted">
            Поиск: <code>{query}</code>
          </p>
        )}
      </div>
      <div className="app-placeholder">Здесь появится список предложений</div>
    </section>
  );
}

export function SheetsListScreen({ query }: { query: string }): JSX.Element {
  return (
    <section>
      <h1>Боевые листки</h1>
      <div className="app-card">
        <p>Список листков с поиском по названию и продукту — шаг S6.</p>
        {query === '' ? null : (
          <p className="app-muted">
            Поиск: <code>{query}</code>
          </p>
        )}
      </div>
      <div className="app-placeholder">Здесь появится список листков</div>
    </section>
  );
}

export function OfferScreen({ id }: { id: number }): JSX.Element {
  return (
    <section>
      <h1>КП #{id}</h1>
      <div className="app-card">
        <p>Форма, системы, позиции и тексты разделов — шаги S3 и S4.</p>
      </div>
      <div className="app-placeholder">Здесь появится карточка предложения</div>
    </section>
  );
}

export function SheetScreen({ id }: { id: number }): JSX.Element {
  return (
    <section>
      <h1>Боевой листок #{id}</h1>
      <div className="app-card">
        <p>Наши продукты, конкуренты, аналоги и уточнение — шаг S6.</p>
      </div>
      <div className="app-placeholder">Здесь появится карточка листка</div>
    </section>
  );
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

export function screenFor(route: Route): JSX.Element {
  switch (route.name) {
    case 'list':
      return route.kind === 'offers' ? <OffersListScreen query={route.query} /> : <SheetsListScreen query={route.query} />;
    case 'offer':
      return <OfferScreen id={route.id} />;
    case 'sheet':
      return <SheetScreen id={route.id} />;
    case 'notFound':
      return <NotFoundScreen hash={route.hash} />;
  }
}