/**
 * Экран «Технодайджест» (2026-11-23).
 *
 * Показывает новости выпуска из Базы знаний: месяц, рубрику, заголовок, лид,
 * текст, автора и ссылку на источник. Месяцы переключаются строкой ссылок —
 * так же, как разделы на экранах списков.
 *
 * У каждой новости своя кнопка «Отправить в чат»: человек читает в приложении,
 * а переслать коллеге в рабочий чат удобнее сообщением бота. Вне Telegram
 * (пустой `initData`) кнопки нет — там отправлять некуда.
 */
import { useCallback, useEffect, useState, type JSX } from 'react';
import { ApiError } from './api.ts';
import {
  digestParagraphs,
  loadDigest,
  monthLabel,
  newsByline,
  sendNewsToChat,
  type DigestFeed,
  type DigestItem,
} from './digest.ts';
import { routeHash } from './router.ts';
import { initData } from './tg.ts';

type FeedState = { kind: 'loading' } | { kind: 'ready'; feed: DigestFeed } | { kind: 'error'; error: string };

function errorText(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 401 || e.status === 403) {
      return 'Нет доступа: войдите в бота и откройте приложение из чата заново.';
    }
    return e.message;
  }
  return 'Не удалось связаться с сервером. Проверьте связь и повторите.';
}

export function DigestCard({ ym }: { ym: string }): JSX.Element {
  const [state, setState] = useState<FeedState>({ kind: 'loading' });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let alive = true;
    setState({ kind: 'loading' });
    void loadDigest(ym)
      .then((feed) => {
        if (alive) setState({ kind: 'ready', feed });
      })
      .catch((e: unknown) => {
        if (alive) setState({ kind: 'error', error: errorText(e) });
      });
    return () => {
      alive = false;
    };
  }, [ym, attempt]);

  const retry = useCallback(() => setAttempt((n) => n + 1), []);

  return (
    <section>
      <HomeLink />
      <h1>Технодайджест</h1>
      {state.kind === 'loading' ? <p className="app-muted">Загружаем новости…</p> : null}
      {state.kind === 'error' ? (
        <div className="app-card app-error">
          <p>{state.error}</p>
          <p>
            <button className="app-button" type="button" onClick={retry}>
              Повторить
            </button>
          </p>
          {ym === '' ? null : (
            // Ссылка на закрытый месяц (например, из старого сообщения) — не
            // тупик: человеку нужен путь к тому, что выходит сейчас.
            <p>
              <a className="app-link" href={routeHash({ name: 'digest', ym: '' })}>
                К текущему выпуску
              </a>
            </p>
          )}
        </div>
      ) : null}
      {state.kind === 'ready' ? <Feed feed={state.feed} ym={ym} /> : null}
    </section>
  );
}

function Feed({ feed, ym }: { feed: DigestFeed; ym: string }): JSX.Element {
  const shown = feed.issue.ym === '' ? ym : feed.issue.ym;
  return (
    <>
      {feed.issues.length > 0 ? <Months issues={feed.issues} shown={shown} current={feed.current} /> : null}
      <p className="app-muted">{feed.issue.title}</p>
      {feed.items.length === 0 ? <p className="app-muted">В этом выпуске новостей пока нет.</p> : null}
      {feed.items.map((item) => (
        <News key={item.id} item={item} ym={shown} />
      ))}
    </>
  );
}

/** Строка месяцев: текущий помечен и не кликается, остальные ведут на выпуск. */
function Months({
  issues,
  shown,
  current,
}: {
  issues: DigestFeed['issues'];
  shown: string;
  current: string;
}): JSX.Element {
  return (
    <div className="app-months">
      {issues.map((issue) => {
        const label = monthLabel(issue.ym);
        if (issue.ym === shown) {
          return (
            <span key={issue.ym} className="app-month app-month-current" aria-current="page">
              {label}
              {issue.ym === current ? ' · текущий' : ''}
            </span>
          );
        }
        return (
          <a key={issue.ym} className="app-month" href={routeHash({ name: 'digest', ym: issue.ym })}>
            {label}
          </a>
        );
      })}
    </div>
  );
}

function News({ item, ym }: { item: DigestItem; ym: string }): JSX.Element {
  return (
    <article className={item.isLead ? 'app-news app-news-lead' : 'app-news'}>
      {item.rubric === '' ? null : <p className="app-news-rubric">{item.rubric}</p>}
      <h2 className="app-news-title">{item.title}</h2>
      {newsByline(item) === '' ? null : <p className="app-news-byline">{newsByline(item)}</p>}
      {item.lead === '' ? null : <p className="app-news-lead-text">{item.lead}</p>}
      {digestParagraphs(item.body).map((paragraph, index) => (
        <p key={index}>{paragraph}</p>
      ))}
      {item.linkUrl === '' ? null : (
        <p>
          <a className="app-link" href={item.linkUrl} target="_blank" rel="noopener noreferrer">
            {item.linkText === '' ? item.linkUrl : item.linkText}
          </a>
        </p>
      )}
      <NewsSendButton ym={ym} id={item.id} />
    </article>
  );
}

/** «Отправить в чат» у новости: окно не закрывается — новостей в выпуске много. */
function NewsSendButton({ ym, id }: { ym: string; id: number }): JSX.Element | null {
  const [busy, setBusy] = useState(false);
  const [sent, setSent] = useState(false);
  const [problem, setProblem] = useState('');
  // Признак Telegram — подпись, а не наличие её объекта: `vendor/telegram-web-app.js`
  // заводит `window.Telegram.WebApp` и в обычном браузере.
  if (initData() === '') return null;

  const onClick = (): void => {
    if (busy || sent) return;
    setBusy(true);
    setProblem('');
    void sendNewsToChat(ym, id)
      .then((outcome) => {
        setSent(outcome.sent);
        if (!outcome.sent) setProblem(outcome.problem);
      })
      .finally(() => setBusy(false));
  };

  return (
    <div className="app-news-send">
      {/* После успеха кнопка выключена: «Отправлено» — это не «отправить ещё
          раз», иначе одно нажатие по подписи дарило бы второе сообщение. */}
      <button className="app-button app-button-ghost" type="button" onClick={onClick} disabled={busy || sent}>
        {busy ? '⏳ Отправляю…' : sent ? '✅ Отправлено в чат' : '📤 Отправить в чат'}
      </button>
      {problem === '' ? null : <p className="app-error-text">{problem}</p>}
    </div>
  );
}

function HomeLink(): JSX.Element {
  return (
    <p className="app-muted">
      <a className="app-link" href={routeHash({ name: 'home' })}>
        ← На главную
      </a>
    </p>
  );
}