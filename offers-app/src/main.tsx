/**
 * Точка входа приложения (S0 — каркас, S1 — вход по initData).
 *
 * Маршрут живёт в хэше адреса: `#/list/offers`, `#/offer/12`, `#/sheet/7`.
 * Такому приложению не нужен серверный роутинг — Caddy отдаёт один
 * `index.html`, а бот передаёт ссылку целиком.
 *
 * Вход (S1) делается один раз на старте: `openSession()` меняет подписанный
 * Telegram `initData` на обычный JWT. Пока сессии нет — экран объясняет, что
 * делать; с сессией — обычные экраны.
 */
import { StrictMode, useEffect, useState, type JSX } from 'react';
import { createRoot } from 'react-dom/client';
import { colorScheme, inTelegram, initData, initTelegram } from './tg.ts';
import { parseRoute } from './router.ts';
import { screenFor } from './screens.tsx';
import { registerStyles } from './styles.ts';
import { failureText, openSession, type SessionResult } from './session.ts';

initTelegram();
registerStyles();

type AuthState = { kind: 'loading' } | { kind: 'ready'; result: SessionResult };

function App(): JSX.Element {
  const [hash, setHash] = useState(() => window.location.hash);
  const [auth, setAuth] = useState<AuthState>({ kind: 'loading' });

  useEffect(() => {
    const onHashChange = (): void => setHash(window.location.hash);
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  useEffect(() => {
    let alive = true;
    void openSession().then((result) => {
      // Размонтировали раньше ответа (StrictMode монтирует дважды) — не пишем.
      if (alive) setAuth({ kind: 'ready', result });
    });
    return () => {
      alive = false;
    };
  }, []);

  if (auth.kind === 'loading') {
    return <p className="app-muted">Входим…</p>;
  }

  const route = parseRoute(hash);
  const allowed = auth.result.ok || inBrowserOnly(auth.result);

  return (
    <>
      <header className="app-muted">LogicTEAM.КП — {inTelegram ? 'Telegram' : 'браузер'}, тема {colorScheme()}</header>
      <main>
        {allowed ? (
          screenFor(route, viewerOf(auth.result))
        ) : (
          <SignInProblem result={auth.result} />
        )}
      </main>
      {auth.result.ok ? (
        <footer className="app-nav">
          <a className="app-link" href="#/list/offers">
            КП
          </a>
          <a className="app-link" href="#/list/sheets">
            Боевые листки
          </a>
        </footer>
      ) : null}
      {auth.result.ok ? null : <DebugLine />}
    </>
  );
}

/**
 * Открыли в обычном браузере (без Telegram) — показываем экраны, но с
 * пометкой: данные без входа не загрузятся. Это удобно для вёрстки и не
 * открывает ничего лишнего: запросы без JWT сервис всё равно отвергнет.
 */
function inBrowserOnly(result: SessionResult): boolean {
  return !result.ok && result.failure.kind === 'no-init-data';
}

/**
 * Кто вошёл — для экранов, которым нужны права (боевой листок показывает
 * кнопки правки только автору). Без входа это пустые строки: запросы всё
 * равно уйдут без JWT и сервис их отвергнет.
 */
function viewerOf(result: SessionResult): { email: string; role: string } {
  if (!result.ok) return { email: '', role: '' };
  return { email: result.session.user.email, role: result.session.user.role };
}

function SignInProblem({ result }: { result: SessionResult }): JSX.Element {
  if (result.ok) return <></>;
  const { title, hint } = failureText(result.failure);
  return (
    <section>
      <h1>{title}</h1>
      <div className="app-card">
        <p>{hint}</p>
      </div>
    </section>
  );
}

/** Строка для S1: показываем, что вход произошёл и кто вошёл. */
function DebugLine(): JSX.Element | null {
  if (!inTelegram) return null;
  const init = initData();
  return (
    <p className="app-muted">
      initData: {init === '' ? 'нет' : `${init.length} байт`}; адрес:{' '}
      <code>{typeof window !== 'undefined' ? window.location.href : ''}</code>
    </p>
  );
}

const rootElement = document.getElementById('root');
if (rootElement === null) {
  throw new Error('Не найден контейнер #root: index.html повреждён');
}

createRoot(rootElement).render(
  <StrictMode>
    <App />
  </StrictMode>,
);