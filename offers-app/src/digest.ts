/**
 * Технодайджест в Mini App (2026-11-23).
 *
 * Четвёртая кнопка приветствия раньше вела на заглушку «раздел
 * проектируется». Новости у подразделения уже есть — это раздел «Дайджест»
 * Базы знаний. Приложение читает их через свой сервис (`GET /offers/digest`):
 * ключ Mini App выдан приложению `offers`, а kb-service отвергает чужой
 * `app_id`, поэтому сервис сам ходит в Базу знаний межсервисным токеном.
 *
 * Этот модуль — чистая часть: типы, разбор ответа на границе и подписи.
 * Запрос — в `api.ts`, отрисовка — в `digest-screen.tsx`. Разбор делается один
 * раз, где данные входят: внутри экрана им верим (правило 2026-11-13).
 */
import { ApiError, apiRequest, apiUrlOf } from './api.ts';
import { initData } from './tg.ts';

/** Новость выпуска: то, что реально нужно экрану. */
export interface DigestItem {
  id: number;
  title: string;
  lead: string;
  body: string;
  rubric: string;
  author: string;
  /** Время подачи, RFC3339; мусор не роняет экран. */
  at: string;
  isLead: boolean;
  linkUrl: string;
  linkText: string;
  /**
   * Подписанный путь картинки (сервис отдаёт его, когда картинка есть).
   * Именно ПУТЬ, а не готовый адрес: домен API знает приложение, и склеивать
   * его дважды (у сервиса и у клиента) значило бы держать два адреса правды.
   */
  imagePath: string;
}

/** Выпуск месяца для строки навигации. */
export interface DigestIssueRow {
  ym: string;
  title: string;
  items: number;
}

export interface DigestFeed {
  /** Месяц, в который принимаются новости (называет База знаний, не клиент). */
  current: string;
  issues: DigestIssueRow[];
  issue: DigestIssueRow;
  items: DigestItem[];
}

/** Адрес ленты: пустой `ym` — текущий выпуск (месяц выберет сервер). */
export function digestPath(ym: string): string {
  const month = ym.trim();
  return month === '' ? '/digest' : `/digest?ym=${encodeURIComponent(month)}`;
}

/** Загрузить выпуск. Ошибку отдаёт `ApiError` — экран показывает текст. */
export async function loadDigest(ym: string): Promise<DigestFeed> {
  const raw = await apiRequest<unknown>(digestPath(ym));
  return digestFeedOf(raw);
}

/** Месяцы для подписи выпуска: «Ноябрь 2026». */
const MONTHS = [
  'Январь',
  'Февраль',
  'Март',
  'Апрель',
  'Май',
  'Июнь',
  'Июль',
  'Август',
  'Сентябрь',
  'Октябрь',
  'Ноябрь',
  'Декабрь',
];

/** `2026-11` → «Ноябрь 2026»; мусор остаётся как есть, но не роняет экран. */
export function monthLabel(ym: string): string {
  const parts = ym.trim().split('-');
  if (parts.length !== 2) return ym.trim();
  const month = Number(parts[1]);
  if (!Number.isInteger(month) || month < 1 || month > 12) return ym.trim();
  return `${MONTHS[month - 1]} ${parts[0]}`;
}

/**
 * Разбор ответа сервиса. Пустое и непонятное — пустые значения, а не
 * исключение: лента без новостей должна выглядеть как лента без новостей.
 */
export function digestFeedOf(raw: unknown): DigestFeed {
  const src = objectOf(raw);
  const issues = arrayOf(src.issues).map(issueOf);
  const issue = issueOf(src.issue);
  return {
    current: textOf(src.current),
    issues,
    issue,
    items: arrayOf(src.items).map(itemOf).filter((item) => item.id > 0 && item.title !== ''),
  };
}

/**
 * Текст новости — абзацами. В Базе знаний он размечен урезанным markdown, но
 * своей вёрстки газеты в приложении нет: текст читается как текст, поэтому
 * снимаются только служебные знаки (#, **, __, маркеры списка), а не смысл.
 */
export function digestParagraphs(body: string): string[] {
  return body
    .replace(/\r\n/g, '\n')
    .split(/\n{2,}/)
    .map((block) =>
      block
        .split('\n')
        .map(plainLine)
        .filter((line) => line !== '')
        .join(' '),
    )
    .filter((paragraph) => paragraph !== '');
}

/** Подпись под заголовком: «Иван Петров · 05.11.2026». */
export function newsByline(item: DigestItem): string {
  const parts: string[] = [];
  const author = item.author.trim();
  if (author !== '') parts.push(author);
  const at = item.at.trim();
  if (at !== '') parts.push(dayLabel(at));
  return parts.join(' · ');
}

/** Дата из RFC3339 → «05.11.2026»; мусор отдаётся как есть. */
export function dayLabel(at: string): string {
  const cut = at.indexOf('T');
  const datePart = cut > 0 ? at.slice(0, cut) : at;
  const parts = datePart.split('-');
  if (parts.length !== 3) return at;
  return `${parts[2]}.${parts[1]}.${parts[0]}`;
}

export interface NewsSendOutcome {
  /** Пустая строка — новость ушла в чат. */
  problem: string;
  sent: boolean;
}

/**
 * «Отправить в чат» у новости.
 *
 * Окно НЕ закрывается (в отличие от отправки документа): новостей в выпуске
 * несколько, и человеку обычно нужно отправить не одну. Адресат — из подписи
 * Telegram, попросить чужой чат нельзя.
 */
export async function sendNewsToChat(ym: string, id: number): Promise<NewsSendOutcome> {
  // Признак «здесь можно отправлять» — не наличие объекта платформы (его
  // `vendor/telegram-web-app.js` заводит и в обычном браузере), а подпись:
  // пустой `initData` означает, что приложение открыто вне Telegram.
  const raw = initData();
  if (raw === '') {
    return { problem: 'Отправить в чат можно только из Telegram: откройте приложение из бота.', sent: false };
  }
  if (!Number.isInteger(id) || id <= 0 || ym.trim() === '') {
    return { problem: 'Новость не выбрана: обновите выпуск и попробуйте ещё раз.', sent: false };
  }
  try {
    await apiRequest<{ ok?: boolean }>('/app/news', {
      method: 'POST',
      body: { initData: raw, ym: ym.trim(), id },
    });
  } catch (e: unknown) {
    return { problem: sendFailureText(e), sent: false };
  }
  return { problem: '', sent: true };
}

function sendFailureText(e: unknown): string {
  if (e instanceof ApiError) {
    switch (e.code) {
      case 'not_linked':
        return 'Сначала войдите в бота: /start и код из письма — потом откройте приложение заново.';
      case 'initData':
        return 'Telegram не подтвердил, что это вы. Откройте приложение из бота заново.';
      default:
        return e.message;
    }
  }
  return e instanceof Error ? `Не удалось отправить: ${e.message}` : 'Не удалось отправить: сервер не ответил.';
}

function plainLine(line: string): string {
  return line
    // Ссылка внутри текста: адрес остаётся видимым, разметка — нет. То же
    // правило на сервере (`digestPlainText` в `appdigest.go`): сообщение в чат
    // и абзац на экране читаются одинаково.
    .replace(/\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/g, '$1 — $2')
    .replace(/^\s*(#{1,6}\s+|[-*•]\s+|\d+[.)]\s+)/, '')
    .replace(/\*\*|__/g, '')
    .trim();
}

function objectOf(value: unknown): Record<string, unknown> {
  return typeof value === 'object' && value !== null ? (value as Record<string, unknown>) : {};
}

function arrayOf(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function textOf(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function numberFieldOf(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

function issueOf(value: unknown): DigestIssueRow {
  const src = objectOf(value);
  return { ym: textOf(src.ym), title: textOf(src.title), items: numberFieldOf(src.items) };
}

function itemOf(value: unknown): DigestItem {
  const src = objectOf(value);
  return {
    id: numberFieldOf(src.id),
    title: textOf(src.title),
    lead: textOf(src.lead),
    body: textOf(src.body),
    rubric: textOf(src.rubric),
    author: textOf(src.author),
    at: textOf(src.at),
    isLead: src.isLead === true,
    linkUrl: textOf(src.linkUrl),
    linkText: textOf(src.linkText),
    imagePath: textOf(src.imagePath),
  };
}

/**
 * Адрес картинки новости. Пустая строка — картинки нет: пустой `src` браузер
 * грузит как саму страницу, поэтому подставлять его нельзя.
 */
export function newsImageURL(item: DigestItem): string {
  const path = item.imagePath.trim();
  return path === '' ? '' : apiUrlOf(path);
}

/**
 * Ведущая новость выпуска: та, которую отметил редактор, иначе первая.
 *
 * «Иначе первая» — не поблажка: выпуск может выйти без отмеченной ведущей, и
 * тогда полоса всё равно должна начинаться с самой верхней новости, а не с
 * пустого места.
 */
export function leadNews(items: DigestItem[]): DigestItem | null {
  if (items.length === 0) return null;
  return items.find((item) => item.isLead) ?? items[0] ?? null;
}

/** Остальные новости выпуска — в порядке полосы (его задала База знаний). */
export function otherNews(items: DigestItem[]): DigestItem[] {
  const lead = leadNews(items);
  return lead === null ? [] : items.filter((item) => item !== lead);
}

/** «1 новость», «2 новости», «5 новостей» — по-русски, а не «1 новостей». */
export function newsCountLabel(count: number): string {
  const tail10 = count % 10;
  const tail100 = count % 100;
  let word = 'новостей';
  if (tail10 === 1 && tail100 !== 11) word = 'новость';
  else if (tail10 >= 2 && tail10 <= 4 && (tail100 < 12 || tail100 > 14)) word = 'новости';
  return `${count} ${word}`;
}

/**
 * Вводная строка выпуска. В макете на этом месте стоит письмо редакции; в
 * приложении письма нет, поэтому строка собирается из того, что известно:
 * месяц и число новостей. Выдуманных обещаний («делимся альбомами узлов») здесь
 * нет намеренно — их нечем подтвердить.
 */
export function issueIntro(issue: DigestIssueRow, items: number): string {
  const month = monthLabel(issue.ym);
  if (items <= 0) {
    return month === '' ? 'В этом выпуске новостей пока нет.' : `${month}: новостей пока нет.`;
  }
  const prefix = month === '' ? '' : `${month}: `;
  return `${prefix}${newsCountLabel(items)} — стандарты, решения и сервисы для проектировщиков и подрядчиков.`;
}