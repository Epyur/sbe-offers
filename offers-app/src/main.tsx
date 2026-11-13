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
import { colorScheme, initData, inTelegram, platformInfo, waitForTelegram } from './tg.ts';
import { parseRoute } from './router.ts';
import { screenFor } from './screens.tsx';
import { registerStyles } from './styles.ts';
import { failureText, openSession, type SessionResult } from './session.ts';

registerStyles();

type AuthState = { kind: 'loading' } | { kind: 'ready'; result: SessionResult };

/** Адрес из браузера; в тестах (`node --test`) окна нет — пусто. */
function readHash(): string {
  if (typeof window === 'undefined') return '';
  return window.location.hash;
}

function App(): JSX.Element {
  // Хэш берётся из подписки, и адрес читается после её установки: экран,
  // открытый сразу с маршрутом (`#/list/offers` из кнопки бота), иначе мог
  // показать приветствие (дефект 2026-11-14).
  const [hash, setHash] = useState(() => readHash());
  const [auth, setAuth] = useState<AuthState>({ kind: 'loading' });

  useEffect(() => {
    const onHashChange = (): void => setHash(readHash());
    window.addEventListener('hashchange', onHashChange);
    setHash(readHash());
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  useEffect(() => {
    // Вход в бот идёт в фоне: пока шёл запрос, человек мог сменить маршрут.
    setHash(readHash());
  }, [auth.kind]);

  useEffect(() => {
    let alive = true;
    // Сначала дождаться скрипта платформы: он мог ещё не загрузиться. Без
    // ожидания `initData` читался пустым, и человек видел «Открыто вне
    // Telegram» (дефект 2026-11-13). Ждём до 2 секунд — этого хватает даже
    // на медленном мобильном соединении.
    void waitForTelegram()
      .then(() => openSession())
      .then((result) => {
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
      <header className="app-muted">
        LogicTEAM.КП — {inTelegram() ? 'Telegram' : 'браузер'}, тема {colorScheme()}
        {platformInfo().version === '' ? '' : `, ${platformInfo().platform} ${platformInfo().version}`}
      </header>
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
  if (!inTelegram()) return null;
  const init = initData();
  const info = platformInfo();
  return (
    <p className="app-muted">
      initData: {init === '' ? 'нет' : `${init.length} байт`}; платформа: {info.platform || '?'}{' '}
      {info.version || ''}
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