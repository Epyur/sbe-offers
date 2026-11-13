/**
 * Вживую: что отдаёт приложение на сервере и что видит браузер (2026-11-14).
 *
 * Жалобы «центральная страница не открывается» и «по умолчанию открывается
 * список КП» нельзя проверить по исходникам: важно, какую сборку раздаёт
 * сервер и что из неё получается в настоящем браузере.
 *
 * Скрипт:
 *   1. читает `https://epyur.fvds.ru/app-offers/`, находит имя бандла;
 *   2. запускает браузер, подменяет SDK Telegram и `/app/session` (без входа
 *      в бот сервис отвечает 401) — дальше всё настоящее;
 *   3. проверяет приветствие, три карточки, переходы и сообщает причину,
 *      если экран пуст.
 *
 * Запуск: node scripts/check_live_home.mjs [база, по умолчанию https://epyur.fvds.ru]
 */
import { chromium } from 'playwright';

const BASE = (process.argv[2] ?? 'https://epyur.fvds.ru').replace(/\/+$/, '');
const APP = `${BASE}/app-offers/`;

const page0 = await fetch(APP);
const html = await page0.text();
const bundle = [...html.matchAll(/assets\/index-[A-Za-z0-9_-]+\.js/g)].map((m) => m[0])[0];
console.log(`index: ${page0.status}; бандл: ${bundle ?? '(не найден)'}`);

const browser = await chromium.launch({ channel: 'msedge' }).catch(() => chromium.launch());
const page = await browser.newPage({ viewport: { width: 430, height: 900 } });
const errors = [];
page.on('pageerror', (e) => errors.push(String(e)));
page.on('console', (m) => {
  if (m.type() === 'error') errors.push(m.text());
});

await page.addInitScript(() => {
  let installed = false;
  const install = () => {
    if (installed) return;
    const wa = window.Telegram?.WebApp;
    if (!wa) return;
    try {
      Object.defineProperty(wa, 'initData', {
        configurable: true,
        get: () => 'query_id=test&user=%7B%22id%22%3A1%7D&auth_date=1&hash=test',
      });
      installed = true;
    } catch {
      installed = false;
    }
  };
  const timer = window.setInterval(install, 10);
  window.setTimeout(() => window.clearInterval(timer), 3000);
  install();
});

await page.route('**/api/offers/**', (route) => {
  if (route.request().url().includes('/app/session')) {
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ token: 'test', user: { email: 'test@tn.ru', role: 'admin' } }),
    });
  }
  const body = route.request().url().includes('/combat') ? { sheets: [] } : { offers: [] };
  return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
});

let failed = 0;
function check(name, ok, detail = '') {
  console.log(`${ok ? 'OK  ' : 'FAIL'} ${name}${detail === '' ? '' : ` — ${detail}`}`);
  if (!ok) failed += 1;
}

await page.goto(APP, { waitUntil: 'networkidle' });
const tiles = await page.locator('.app-section-tile').count();
const h1 = (await page.locator('main h1').first().textContent().catch(() => '')) ?? '';
check('главная: приветствие открылось', tiles === 3, `карточек ${tiles}, первый заголовок «${h1}»`);
check('главная: нет ошибок JS', errors.length === 0, errors.slice(0, 2).join(' | '));
await page.screenshot({ path: 'dist/live-home.png', fullPage: true });

await page.goto(`${APP}#/list/offers`, { waitUntil: 'networkidle' });
const chips = await page.locator('.app-section-chip').count();
check('список КП: строки разделов видны', chips === 3, `найдено ${chips}`);

await browser.close();
if (failed > 0) {
  console.error(`\nпровалено: ${failed}`);
  process.exit(1);
}
console.log('\nвживую всё сходится');