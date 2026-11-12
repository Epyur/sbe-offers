/**
 * Живой поиск в списках на боевой базе (2026-11-13, шаг S2).
 *
 * Отвечает на вопрос, который стенд не задаёт: находят ли ЖИВЫЕ данные
 * заказчика по части имени, с большой буквы и без неё. Ровно на этом 0.9.18
 * спотыкалась дважды: сначала поиск резал 200 свежих строк до фильтрации,
 * потом приводил к нижнему регистру только SQL, а не текст в памяти.
 *
 * Запуск: node scripts/check_list_search_live.mjs <baseUrl> <email> <offersServiceSecret>
 * Пример: node scripts/check_list_search_live.mjs https://epyur.fvds.ru you@tn.ru <OFFERS_SERVICE_SECRET>
 */
import { createHmac } from 'node:crypto';

const base = (process.argv[2] ?? '').replace(/\/+$/, '');
const email = process.argv[3] ?? '';
const secret = process.argv[4] ?? ''; // это OFFERS_SERVICE_SECRET сервиса, им же подписаны все JWT
if (base === '' || email === '' || secret === '') {
  console.error('нужно: <baseUrl> <email> <offersServiceSecret>');
  process.exit(2);
}

function b64url(value) {
  return Buffer.from(value).toString('base64url');
}

function makeJwt(forEmail) {
  const header = b64url(JSON.stringify({ alg: 'HS256', typ: 'JWT' }));
  const now = Math.floor(Date.now() / 1000);
  const payload = b64url(
    JSON.stringify({
      email: forEmail,
      app_id: 'offers',
      device_id: 'check',
      iss: 'auth-service',
      aud: 'offers',
      exp: now + 600,
      iat: now,
    }),
  );
  const sig = createHmac('sha256', secret).update(`${header}.${payload}`).digest('base64url');
  return `${header}.${payload}.${sig}`;
}

let passed = 0;
let failed = 0;

function ok(name, condition, detail = '') {
  if (condition) {
    passed++;
    console.log(`OK   ${name}${detail ? ' — ' + detail : ''}`);
  } else {
    failed++;
    console.log(`FAIL ${name}${detail ? ' — ' + detail : ''}`);
  }
}

const token = makeJwt(email);

async function search(q) {
  const res = await fetch(`${base}/api/offers/offers?q=${encodeURIComponent(q)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  const body = res.status === 200 ? await res.json() : {};
  return { status: res.status, offers: body.offers ?? [] };
}

const all = await search('');
ok('список без поиска отдаётся', all.status === 200, String(all.status));

// Ищем по первому слову имени самого свежего своего заказчика — то же, что
// сделает менеджер: помнит начало названия или его часть.
const names = all.offers
  .map((o) => (o.client?.name ?? '').trim())
  .filter((name) => name.length >= 4);

// Лучше искать живого заказчика; чтобы проверка умела прогнать поиск и на
// базе без имён (свежий стенд, пустой отдел). Подставляем слово и смотрим, что
// сервер отвечает 200 и массив — ошибки ворота это тоже ловит.
const sample = names[0] ?? 'Ромашка';
const word = names.length > 0
  ? sample.split(/\s+/).find((part) => part.length >= 4) ?? sample.slice(0, 4)
  : sample;
if (names.length === 0) {
  console.log('у заказчиков нет имён — идём по контрольному слову «Ромашка»');
}

const exact = await search(word);
ok('поиск по слову находит', exact.offers.length > 0, `«${word}» → ${exact.offers.length}`);

const upper = word.charAt(0).toUpperCase() + word.slice(1);
const capital = await search(upper);
ok('поиск с большой буквы находит столько же', capital.offers.length === exact.offers.length, `«${upper}» → ${capital.offers.length}`);

const lower = word.toLowerCase();
const small = await search(lower);
ok('поиск строчными находит столько же', small.offers.length === exact.offers.length, `«${lower}» → ${small.offers.length}`);

const mid = word.slice(1, Math.max(2, word.length - 1));
if (mid.length >= 3) {
  const inside = await search(mid);
  ok('подстрока из середины слова находит', inside.offers.length > 0, `«${mid}» → ${inside.offers.length}`);
}

const none = await search('этого-заказчика-точно-нет-12345');
ok('несуществующее имя даёт пустой массив', Array.isArray(none.offers) && none.offers.length === 0, String(none.offers.length));

const sheets = await fetch(`${base}/api/offers/combat?q=${encodeURIComponent(word)}`, {
  headers: { Authorization: `Bearer ${token}` },
});
const sheetsBody = sheets.status === 200 ? await sheets.json() : {};
ok('поиск листков отвечает массивом', sheets.status === 200 && Array.isArray(sheetsBody.sheets), String(sheets.status));

console.log(`\nИтого: ${passed} ок, ${failed} провалов`);
process.exit(failed === 0 ? 0 : 1);