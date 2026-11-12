/**
 * Живая проверка входа в Mini App (2026-11-13, S1).
 *
 * Вход проверяется НЕ так, как на стенде: initData подписывается настоящим
 * токеном бота, а токен берётся только из `offers_bot_config` на боевом
 * сервисе. Здесь подпись строится независимо (по документации Telegram) и
 * предъявляется живому сервису — так проверяется и алгоритм, и то, что
 * сервис читает токен из правильного места.
 *
 * Запуск: node scripts/check_app_auth.mjs <baseUrl> <botToken>
 * Токен берётся администратором и НЕ печатается и НЕ сохраняется.
 */

import { createHmac } from 'node:crypto';

const base = (process.argv[2] ?? '').replace(/\/+$/, '');
const botToken = process.argv[3] ?? '';

if (base === '' || botToken === '') {
  console.error('нужно: node check_app_auth.mjs <baseUrl> <botToken>');
  process.exit(2);
}

let failures = 0;
function check(name, ok, detail = '') {
  console.log(`${ok ? 'OK  ' : 'ФЕЙЛ'} ${name}${detail === '' ? '' : ` — ${detail}`}`);
  if (!ok) failures += 1;
}

function sign(fields, token) {
  const pairs = Object.keys(fields)
    .filter((k) => k !== 'hash')
    .sort()
    .map((k) => `${k}=${fields[k]}`);
  const secret = createHmac('sha256', 'WebAppData').update(token).digest();
  return createHmac('sha256', secret).update(pairs.join('\n')).digest('hex');
}

function initData(fields, token) {
  const all = { ...fields };
  all.hash = sign(all, token);
  return new URLSearchParams(all).toString();
}

async function post(initDataValue) {
  const res = await fetch(`${base}/api/offers/app/session`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ initData: initDataValue }),
  });
  const text = await res.text();
  let body = null;
  try {
    body = JSON.parse(text);
  } catch {
    body = { raw: text.slice(0, 120) };
  }
  return { status: res.status, body };
}

const nowSec = Math.floor(Date.now() / 1000);
const chatId = process.env.CHECK_CHAT_ID ?? '1';
const baseFields = {
  auth_date: String(nowSec),
  query_id: 'AAH-check',
  chat_instance: '-100',
  chat_type: 'private',
  user: JSON.stringify({ id: Number(chatId), first_name: 'Проверка' }),
};

const good = await post(initData(baseFields, botToken));
check(
  'подпись настоящим токеном принимается или отвечает not_linked (подпись верна)',
  good.status === 200 || (good.status === 403 && good.body?.error === 'not_linked'),
  `${good.status} ${JSON.stringify(good.body)}`,
);
if (good.status === 403) {
  check('403 несёт код not_linked и подсказку', good.body?.hint !== undefined, String(good.body?.hint));
}

const wrongToken = await post(initData(baseFields, `${botToken}xxxx`));
check('подпись чужим токеном отвергнута 401', wrongToken.status === 401, `${wrongToken.status}`);

const stale = await post(initData({ ...baseFields, auth_date: String(nowSec - 3 * 86400) }, botToken));
check('устаревшая подпись (3 суток) отвергнута 401', stale.status === 401, `${stale.status} ${JSON.stringify(stale.body)}`);

const tamperedRaw = initData(baseFields, botToken).replace(/%22id%22%3A\d+/, '%22id%22%3A999999');
const tampered = await post(tamperedRaw);
check('подменённый user отвергнут 401', tampered.status === 401, `${tampered.status}`);

const empty = await post('');
check('пустой initData отвергнут 400', empty.status === 400, `${empty.status}`);

const garbage = await post('%%%');
check('мусор отвергнут 400', garbage.status === 400, `${garbage.status}`);

const config = await fetch(`${base}/api/offers/app/config`).then((r) => r.json());
check('config отдаёт адрес приложения', typeof config.appUrl === 'string' && config.appUrl.endsWith('/'), config.appUrl);

const openRoute = await fetch(`${base}/api/offers/offers`);
check('список КП без токена по-прежнему 401', openRoute.status === 401, `${openRoute.status}`);

console.log(`\nНе прошло проверок: ${failures}`);
process.exit(failures === 0 ? 0 : 1);