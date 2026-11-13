/**
 * Заглушка mailer-service для локального стенда (2026-11-14, шаг P5b).
 *
 * Отвечает ровно на две внутренние ручки, которые сделали письма:
 *
 *   GET /internal/mailer/agent/emails?q=&limit=   → список, текст обрезан
 *   GET /internal/mailer/agent/email?id=<id>      → одно письмо, ПОЛНЫЙ текст
 *   X-Service-Secret: $MAILER_INTERNAL_SECRET
 *
 * Тело — не придуманное: `mailer-fixture.json` рядом — ЗАПИСАННЫЙ живой ответ
 * боевой ручки (https://epyur.fvds.ru, 2026-11-13), урезанный до двух писем:
 * со сканом и без. Правило P6: заглушка — это утверждение о чужом контракте,
 * и она не должна быть добрее настоящей ручки. Поэтому:
 *
 *  - `file_key` в ответе НЕТ: ручка его не отдаёт, ключ разрешается в
 *    offers-service по `document_id` через документы;
 *  - письмо без карточки скана (пустые `attachments`) в выдачу НЕ попадает:
 *    ручка отдаёт только письма с `kind = "signed"` и `document_id > 0`;
 *  - нет id; письмо без скана и несуществующий id — ОДНО тело 404
 *    `{"error":"email not found"}`;
 *  - ошибка БД — 500 и ПУСТОЙ `items`;
 *  - `items`/`attachments` — всегда массивы;
 *  - `limit` — 20 по умолчанию, 200 максимум, мусор/0/отрицательное → 20;
 *  - порядок — `date` по убыванию строки, пустые даты в конце, затем id
 *    по убыванию; дата не парсится и не переформатируется;
 *  - текст в списке — до 600 рун (`email?id=` — полный).
 *
 * Дополнительно стенду нужно второе письмо со сканом, чтобы проверить
 * пересечение с документами: письмо 862 в фикстуре — без скана, а видимость
 * письма решает карточка скана. Поэтому в памяти добавляется письмо 863 со
 * сканом `document_id = 1788168804900`, которого НЕТ в видимом наборе
 * документов (`documents-stub`): на стенде видимым остаётся только письмо
 * 1784211415259, и «стало 2 письма» означает, что фильтр протекает.
 *
 * Если фикстура расходится с тем, что пришло от ручки, — права ручка.
 */
import http from 'node:http';
import { readFileSync } from 'node:fs';

const PORT = Number(process.env.MAILER_PORT ?? 3000);
const SECRET = process.env.MAILER_INTERNAL_SECRET ?? '';

const TEXT_LIMIT = 600;
const LIMIT_DEFAULT = 20;
const LIMIT_MAX = 200;

const fixture = JSON.parse(readFileSync(new URL('./mailer-fixture.json', import.meta.url), 'utf8'));

// Письмо со сканом, привязанным к НЕвидимому документу: проверяет, что
// offers-service пересекает наборы, а не верит мейлеру.
const unlinkedScan = {
  id: 863,
  number: '01.02.477',
  subject: 'Письмо со сканом, не привязанным к продукту',
  author: 'И.И. Иванов',
  date: '2026-10-27',
  direction_name: 'Сертификация',
  text: 'Скан этого письма в видимый набор документов не входит.',
  attachments: [
    { document_id: 1788168804900, file_name: 'unlinked-scan.pdf', kind: 'signed' },
  ],
};

/** Все письма в памяти: фикстура + намеренно невидимое. */
const letters = [...fixture.items, unlinkedScan];

/** Письмо с карточкой скана — только такое отдаёт ручка. */
function hasSignedScan(letter) {
  return (letter.attachments ?? []).some(
    (a) => a && a.kind === 'signed' && Number(a.document_id) > 0,
  );
}

/** Срез «как у ручки»: без file_key (его и в фикстуре нет), обрезанный текст. */
function listView(letter) {
  const chars = Array.from(letter.text ?? '');
  return {
    ...letter,
    text: chars.slice(0, TEXT_LIMIT).join(''),
    attachments: (letter.attachments ?? []).map((a) => ({
      document_id: a.document_id,
      file_name: a.file_name,
      kind: a.kind,
    })),
  };
}

function queryMatches(letter, q) {
  if (!q) return true;
  const needle = q.trim().toLowerCase();
  if (!needle) return true;
  const haystack = [letter.subject, letter.number, letter.direction_name, letter.text]
    .filter(Boolean)
    .join('\n')
    .toLowerCase();
  return haystack.includes(needle);
}

function limitOf(raw) {
  const n = Number.parseInt(String(raw ?? '').trim(), 10);
  if (!Number.isFinite(n) || n <= 0) return LIMIT_DEFAULT;
  return Math.min(n, LIMIT_MAX);
}

/** Порядок ручки: дата DESC, пустые даты в конце, затем id DESC. */
function byDateDesc(left, right) {
  const a = String(left.date ?? '');
  const b = String(right.date ?? '');
  if (a !== b) {
    if (a === '') return 1;
    if (b === '') return -1;
    return a < b ? 1 : -1;
  }
  return Number(right.id) - Number(left.id);
}

function send(res, status, body) {
  res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8' });
  res.end(JSON.stringify(body));
}

function authorised(req) {
  if (!SECRET) return false;
  return req.headers['x-service-secret'] === SECRET;
}

http
  .createServer((req, res) => {
    if (!SECRET) {
      send(res, 503, { error: 'MAILER_INTERNAL_SECRET is not set' });
      return;
    }
    if (!authorised(req)) {
      send(res, 403, { error: 'bad service secret' });
      return;
    }
    const url = new URL(req.url ?? '/', 'http://stub');

    if (url.pathname === '/internal/mailer/agent/emails') {
      const q = url.searchParams.get('q') ?? '';
      const limit = limitOf(url.searchParams.get('limit'));
      const items = letters
        .filter(hasSignedScan)
        .filter((letter) => queryMatches(letter, q))
        .sort(byDateDesc)
        .slice(0, limit)
        .map(listView);
      send(res, 200, { source: 'mailer', total: items.length, items });
      return;
    }

    if (url.pathname === '/internal/mailer/agent/email') {
      const id = Number.parseInt(url.searchParams.get('id') ?? '', 10);
      // Письмо без карточки скана и несуществующий id — ОДНО тело ответа.
      const found = letters.find((letter) => Number(letter.id) === id && hasSignedScan(letter));
      if (!found) {
        send(res, 404, { error: 'email not found' });
        return;
      }
      send(res, 200, {
        ...found,
        attachments: (found.attachments ?? []).map((a) => ({
          document_id: a.document_id,
          file_name: a.file_name,
          kind: a.kind,
        })),
      });
      return;
    }

    send(res, 404, { error: 'not found' });
  })
  .listen(PORT, () => {
    console.log(`mailer-stub: слушаю :${PORT}`);
  });