/**
 * Экран «Технодайджест» (2026-11-23).
 *
 * Показывает новости выпуска из Базы знаний: месяц, рубрику, заголовок, лид,
 * текст, автора, картинку и ссылку на источник. Месяцы переключаются строкой
 * ссылок — так же, как разделы на экранах списков.
 *
 * Вёрстка — по макету брендовой полосы (тёмная тема ТЕХНОНИКОЛЬ): красная
 * марка сверху, шапка с эмблемой и плашкой месяца, вводный блок, ведущая
 * новость крупной карточкой с кадром, остальные — карточками, подвал.
 * Тёмная палитра живёт ТОЛЬКО здесь (класс `app-digest`): остальные экраны
 * приложения остаются в теме Telegram — решение пользователя 2026-11-23.
 *
 * Блоки макета, которых нет в данных (письмо редакции, анонс курса Академии),
 * собраны из того, что известно: месяц и число новостей в вводной строке,
 * ссылка на источник ведущей новости в нижнем баннере. Выдуманных обещаний
 * здесь нет — их нечем подтвердить.
 *
 * У каждой новости своя кнопка «Отправить в чат»: человек читает в приложении,
 * а переслать коллеге в рабочий чат удобнее сообщением бота (картинкой и
 * текстом). Вне Telegram (пустой `initData`) кнопки нет — там отправлять некуда.
 */
import { useCallback, useEffect, useState, type ChangeEvent, type JSX } from 'react';
import { ApiError } from './api.ts';
import {
  digestParagraphs,
  issueIntro,
  leadNews,
  loadDigest,
  monthLabel,
  newsByline,
  newsCountLabel,
  newsImageURL,
  otherNews,
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
      <h1>Технодайджест</h1>
      <HomeLink />
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

/** Полоса выпуска: шапка, вводный блок, новости, баннер источника, подвал. */
function Feed({ feed, ym }: { feed: DigestFeed; ym: string }): JSX.Element {
  const shown = feed.issue.ym === '' ? ym : feed.issue.ym;
  // Считаем РИСУЕМЫЕ новости, а не поле выпуска: в нём База знаний может
  // держать и очередь модерации, и тогда подпись «4 новости» рядом с тремя
  // карточками читалась бы как пропажа.
  const total = feed.items.length;
  const lead = leadNews(feed.items);
  const rest = otherNews(feed.items);
  const issueTitle = feed.issue.title === '' ? 'Технические новости' : feed.issue.title;

  return (
    <div className="app-digest">
      <div className="app-digest-bar" />

      <header className="app-digest-head">
        <div className="app-digest-brand">
          {/* Эмблема — та же, что в шапке полосы Базы знаний
              (`kb-service/templates/logo.png`, белая на прозрачном).
              Файл лежит рядом с приложением (`public/digest-logo.png`), а не
              вшит в бандл: 4 КБ картинки незачем гонять через бюджет кода. */}
          <img className="app-digest-emblem" src="./digest-logo.png" alt="" width={32} height={32} />
          <span className="app-digest-brand-text">
            <span className="app-digest-word">
              ТЕХНО<span className="app-digest-word-red">ДАЙДЖЕСТ</span>
              <span className="app-digest-word-tail">.LOGICTEAM</span>
            </span>
            <span className="app-digest-tagline">Инженерные решения и сервисы</span>
          </span>
        </div>
      </header>

      {/* Месяц — отдельной строкой под названием (2026-11-23: плашка месяца
          наезжала на «LOGICTEAM») и сразу выпадающим списком выпусков. */}
      <IssuePicker issues={feed.issues} shown={shown} current={feed.current} />

      <div className="app-digest-intro">
        <div className="app-digest-eyebrow">Инженерная практика</div>
        <h2 className="app-digest-title">{issueTitle}</h2>
        <p className="app-digest-lead">{issueIntro(feed.issue, total)}</p>
      </div>

      {lead === null ? <p className="app-digest-empty">В этом выпуске новостей пока нет.</p> : null}

      {lead === null ? null : <Hero item={lead} ym={shown} />}

      {rest.length === 0 ? null : (
        <>
          <div className="app-digest-divider">
            <span className="app-digest-divider-title">Новости выпуска</span>
            <span className="app-digest-divider-side">{newsCountLabel(total)}</span>
          </div>
          {rest.map((item) => (
            <News key={item.id} item={item} ym={shown} />
          ))}
        </>
      )}

      {lead === null || lead.linkUrl === '' ? null : (
        <div className="app-digest-banner">
          <span className="app-digest-banner-body">
            <span className="app-digest-banner-eyebrow">Выпуск {monthLabel(shown)}</span>
            <span className="app-digest-banner-title">{lead.linkText === '' ? 'Источник ведущей новости' : lead.linkText}</span>
            <span className="app-digest-banner-sub">{newsCountLabel(total)} · База знаний</span>
          </span>
          <a className="app-digest-outline" href={lead.linkUrl} target="_blank" rel="noopener noreferrer">
            Открыть
          </a>
        </div>
      )}

      <footer className="app-digest-foot">
        <div className="app-digest-foot-brand">
          LOGIC<span className="app-digest-word-red">TEAM</span>
        </div>
        <div className="app-digest-foot-sub">Техническая дирекция СБЕ ПМиПИР</div>
      </footer>
    </div>
  );
}

/**
 * Выбор выпуска — выпадающий список (2026-11-23, просьба пользователя: строка
 * месяцев занимала место, а месяц нужен одной строкой под названием).
 *
 * Значение — месяц из ленты; выбирает человек → адрес меняется на выпуск, а
 * экран перечитывает ленту (маршрут `#/digest/:ym`). Если База знаний ещё не
 * назвала выпуски, в списке остаётся единственный — тот, что показан: список
 * из одного пункта честнее пустого места.
 */
function IssuePicker({
  issues,
  shown,
  current,
}: {
  issues: DigestFeed['issues'];
  shown: string;
  current: string;
}): JSX.Element | null {
  const rows = issues.length > 0 ? issues : [{ ym: shown, title: '', items: 0 }];
  const usable = rows.filter((issue) => issue.ym !== '');
  if (usable.length === 0) return null;

  const onChange = (event: ChangeEvent<HTMLSelectElement>): void => {
    const ym = event.target.value;
    if (ym === shown) return;
    window.location.hash = routeHash({ name: 'digest', ym }).replace(/^.*#/, '#');
  };

  return (
    <div className="app-digest-pick">
      <label className="app-digest-pick-label" htmlFor="digest-issue">
        Выпуск
      </label>
      <select
        id="digest-issue"
        className="app-digest-select"
        value={shown}
        onChange={onChange}
        aria-label="Выпуск технодайджеста"
      >
        {usable.map((issue) => (
          <option key={issue.ym} value={issue.ym}>
            {monthLabel(issue.ym)}
            {issue.ym === current ? ' · текущий' : ''}
          </option>
        ))}
      </select>
    </div>
  );
}

/**
 * Кадр новости. Ошибка загрузки гасит картинку, а не оставляет битый значок:
 * новость читается и без неё (сервис мог не отдать файл из хранилища).
 */
function NewsImage({ item, className }: { item: DigestItem; className: string }): JSX.Element | null {
  const url = newsImageURL(item);
  const [broken, setBroken] = useState(false);
  if (url === '' || broken) return null;
  return (
    <img
      className={className}
      src={url}
      alt=""
      loading="lazy"
      decoding="async"
      onError={() => setBroken(true)}
    />
  );
}

/** Ведущая новость — крупная карточка с кадром и плашкой поверх него. */
function Hero({ item, ym }: { item: DigestItem; ym: string }): JSX.Element {
  const image = newsImageURL(item);
  const paragraphs = digestParagraphs(item.body);
  return (
    <article className="app-news app-news-lead app-digest-hero">
      <div className={image === '' ? 'app-digest-hero-media app-digest-hero-media-empty' : 'app-digest-hero-media'}>
        <NewsImage item={item} className="app-digest-photo" />
        {image === '' ? (
          // Без кадра — чертёжный паттерн макета: пустое место читалось бы как
          // «картинка не загрузилась».
          <svg
            className="app-digest-hero-icon"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
            aria-hidden="true"
          >
            <path d="M3 21h18" />
            <path d="M5 21V7l8-4v18" />
            <path d="M19 21V11l-6-4" />
            <path d="M9 9h1" />
            <path d="M9 13h1" />
            <path d="M9 17h1" />
          </svg>
        ) : null}
        <span className="app-digest-badge">{item.rubric === '' ? 'Ведущая новость' : item.rubric}</span>
      </div>
      <div className="app-digest-hero-body">
        <h3 className="app-news-title">{item.title}</h3>
        {newsByline(item) === '' ? null : <p className="app-digest-byline">{newsByline(item)}</p>}
        {item.lead === '' ? null : <p className="app-digest-hero-lead">{item.lead}</p>}
        {paragraphs.map((paragraph, index) => (
          <p key={index} className="app-digest-text">
            {paragraph}
          </p>
        ))}
        {item.linkUrl === '' ? null : (
          <p className="app-digest-hero-cta">
            <a className="app-digest-button" href={item.linkUrl} target="_blank" rel="noopener noreferrer">
              {item.linkText === '' ? 'Открыть источник' : item.linkText} →
            </a>
          </p>
        )}
        <NewsSendButton ym={ym} id={item.id} />
      </div>
    </article>
  );
}

/** Новость полосы: рубрика, заголовок, лид, полный текст под раскрытием, ссылка. */
function News({ item, ym }: { item: DigestItem; ym: string }): JSX.Element {
  const paragraphs = digestParagraphs(item.body);
  const title = item.linkUrl === '' ? (
    item.title
  ) : (
    <a href={item.linkUrl} target="_blank" rel="noopener noreferrer">
      {item.title}
    </a>
  );
  return (
    <article className="app-news app-digest-news">
      <NewsImage item={item} className="app-digest-thumb" />
      {item.rubric === '' ? null : <span className="app-digest-chip">{item.rubric}</span>}
      <h3 className="app-news-title">{title}</h3>
      {newsByline(item) === '' ? null : <p className="app-digest-byline">{newsByline(item)}</p>}
      {item.lead === '' ? null : <p className="app-digest-news-lead">{item.lead}</p>}
      {paragraphs.length === 0 ? null : (
        <details className="app-digest-full">
          <summary>Читать полностью</summary>
          {paragraphs.map((paragraph, index) => (
            <p key={index} className="app-digest-text">
              {paragraph}
            </p>
          ))}
        </details>
      )}
      {item.linkUrl === '' ? null : (
        <a className="app-digest-more" href={item.linkUrl} target="_blank" rel="noopener noreferrer">
          {item.linkText === '' ? 'Открыть источник' : item.linkText} →
        </a>
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