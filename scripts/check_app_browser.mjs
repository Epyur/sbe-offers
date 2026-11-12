/**
 * Проверка приложения Mini App снаружи — как его увидит человек (2026-11-13).
 *
 * Зачем отдельно от `check_app_browser`-мечты: живой дефект 0.9.18.0 — на
 * сервере (РФ) недоступен telegram.org, и внешний `<script
 * src="https://telegram.org/js/telegram-web-app.js">` не загружался. Без него
 * `window.Telegram.WebApp` не существует, `initData` пуст, и приложение честно
 * показывает «Открыто вне Telegram» — а человек видит «не работает».
 *
 * Поэтому проверка смотрит на СОБРАННУЮ страницу снаружи и требует:
 *   1. нет ссылок на внешние скрипты (никакой CDN: домен у людей за стеной);
 *   2. скрипт Telegram отдаётся с нашего же домена и отвечает 200;
 *   3. сборка приложения отдаётся 200 и совпадает с тем, что на диске;
 *   4. `/app/config` отвечает и говорит, что токен бота настроен.
 *
 * Запуск: node scripts/check_app_browser.mjs https://epyur.fvds.ru
 */
import { createHash } from 'node:crypto';
import fs from 'node:fs';

const base = (process.argv[2] ?? '').replace(/\/+$/, '');
if (base === '') {
  console.error('нужно: node check_app_browser.mjs <baseUrl>');
  process.exit(2);
}

const APP_PATH = '/app-offers/';
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

function sha256(bytes) {
  return createHash('sha256').update(bytes).digest('hex');
}

const page = await fetch(base + APP_PATH);
const html = await page.text();
ok('страница приложения отдаётся', page.status === 200, String(page.status));

const scripts = [...html.matchAll(/<script[^>]*src="([^"]+)"/g)].map((m) => m[1]);
ok('на странице есть скрипты', scripts.length > 0, scripts.join(', '));

const external = scripts.filter((src) => /^https?:\/\//i.test(src));
ok(
  'нет внешних скриптов (CDN с домена не достаётся)',
  external.length === 0,
  external.length === 0 ? '' : external.join(', '),
);

const tgLocal = scripts.find((src) => /telegram-web-app\.js$/.test(src));
ok('скрипт Telegram грузится со своего домена', tgLocal !== undefined, tgLocal ?? 'не найден');

if (tgLocal !== undefined) {
  const url = new URL(tgLocal, base + APP_PATH).toString();
  const res = await fetch(url);
  const bytes = new Uint8Array(await res.arrayBuffer());
  ok('скрипт Telegram отвечает 200', res.status === 200, `${res.status}, ${bytes.length} Б`);
  ok('скрипт Telegram не пустой', bytes.length > 50000, `${bytes.length} Б`);
  // Содержимое — то, что положили: скрипт Mini Apps начинается с «// WebView»
  // и содержит ключевые вызовы платформы. Заглушка Caddy (index.html вместо
  // файла) этих строк не даст — именно так выглядел бы отказ вёрстки.
  const text = new TextDecoder().decode(bytes.slice(0, 8000));
  ok(
    'это действительно скрипт Telegram',
    text.includes('WebView') && (text.includes('initParams') || text.includes('TelegramWebviewProxy')),
    '',
  );
  if (fs.existsSync('offers-app/dist/vendor/telegram-web-app.js')) {
    const local = fs.readFileSync('offers-app/dist/vendor/telegram-web-app.js');
    ok('скрипт совпадает с собранным', sha256(bytes) === sha256(local), sha256(bytes).slice(0, 16));
  }
}

const bundles = scripts.filter((src) => /\/assets\/.*\.js$/.test(src));
ok('сборка приложения подключена', bundles.length === 1, bundles.join(', '));
if (bundles.length === 1) {
  const url = new URL(bundles[0], base + APP_PATH).toString();
  const res = await fetch(url);
  const bytes = new Uint8Array(await res.arrayBuffer());
  ok('сборка приложения отдаётся 200', res.status === 200, `${res.status}, ${bytes.length} Б`);
  const distDir = 'offers-app/dist/assets';
  if (fs.existsSync(distDir)) {
    const name = decodeURIComponent(bundles[0].split('/').pop() ?? '');
    const localPath = `${distDir}/${name}`;
    if (fs.existsSync(localPath)) {
      ok('сборка совпадает с собранной', sha256(bytes) === sha256(fs.readFileSync(localPath)), sha256(bytes).slice(0, 16));
    }
  }
}

const cfg = await fetch(base + '/api/offers/app/config');
const cfgJson = cfg.status === 200 ? await cfg.json() : {};
ok('конфиг приложения отвечает', cfg.status === 200, String(cfg.status));
ok(
  'в конфиге адрес приложения',
  typeof cfgJson.appUrl === 'string' && cfgJson.appUrl.startsWith('http'),
  cfgJson.appUrl ?? '',
);
ok(
  'у бота есть имя (токен настроен)',
  typeof cfgJson.botUsername === 'string' && cfgJson.botUsername !== '',
  cfgJson.botUsername ?? '',
);

// Тот же вход, что делает приложение, но с заведомо неверной подписью: ответ
// должен быть 401, а не 500. Это доказывает, что проверка initData жива и
// доходит до сверки подписи (токен бота прочитан).
const session = await fetch(base + '/api/offers/app/session', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ initData: 'auth_date=1&hash=00&user=%7B%22id%22%3A1%7D' }),
});
ok('вход с неверной подписью отвергается 401', session.status === 401, String(session.status));

console.log(`\nИтого: ${passed} ок, ${failed} провалов`);
process.exit(failed === 0 ? 0 : 1);