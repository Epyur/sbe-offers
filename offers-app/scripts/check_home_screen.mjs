/**
 * Проверка приветствия и переходов между разделами (2026-11-14).
 *
 * Поднимает `dist/` на 127.0.0.1 и гоняет настоящий браузер:
 *   `#/`            — три карточки разделов;
 *   `#/list/offers` — список КП и строка разделов;
 *   `#/ambulance`   — экран «раздел ещё готовится»;
 *   открытие без хэша — приветствие.
 *
 * Подменяются только SDK Telegram (настоящий `initData` иначе пуст) и
 * `/app/session` — без входа в бот сервис отвечает 401; списки пустые,
 * проверяется вёрстка, а не данные.
 *
 * Запуск: node scripts/check_home_screen.mjs
 */
import { createServer } from 'node:http';
import { existsSync, readFileSync } from 'node:fs';
import { dirname, extname, join, normalize } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = dirname(dirname(fileURLToPath(import.meta.url)));
const DIST = join(ROOT, 'dist');
const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.webp': 'image/webp',
};

if (!existsSync(join(DIST, 'index.html'))) {
  console.error('нет dist/: сначала npm run build');
  process.exit(2);
}

const server = createServer((req, res) => {
  const url = new URL(req.url ?? '/', 'http://127.0.0.1');
  let file = normalize(join(DIST, decodeURIComponent(url.pathname)));
  if (!file.startsWith(DIST) || url.pathname === '/' || !existsSync(file) || !extname(file)) {
    file = join(DIST, 'index.html');
  }
  res.writeHead(200, { 'Content-Type': MIME[extname(file)] ?? 'application/octet-stream' });
  res.end(readFileSync(file));
});
await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
const base = `http://127.0.0.1:${server.address().port}/`;

const { chromium } = await import('playwright');
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
      // Платформа держит initData getter'ом без конфигуратора — переопределяем
      // тем же способом, иначе «Cannot redefine property».
      Object.defineProperty(wa, 'initData', { configurable: true, get: () => 'q=test' });
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

await page.goto(`${base}#/`, { waitUntil: 'networkidle' });
check('главная: 3 карточки разделов', (await page.locator('.app-section-tile').count()) === 3);
check('главная: нет ошибок JS', errors.length === 0, errors.slice(0, 2).join(' | '));
// Снимок — рядом с приложением, НЕ в `dist/`: dist уходит на сервер целиком и
// считается в бюджете (`npm run size`), а картинка проверки там лишняя.
await page.screenshot({ path: join(ROOT, 'home-screen.png'), fullPage: true });

await page.goto(`${base}#/list/offers`, { waitUntil: 'networkidle' });
check('список КП: заголовок списка', ((await page.locator('main h1').first().textContent()) ?? '').includes('Коммерческие'));

await page.goto(`${base}#/ambulance`, { waitUntil: 'networkidle' });
check(
  '«Скорая помощь»: свой экран',
  ((await page.locator('main h1').first().textContent()) ?? '').includes('Скорая'),
);

await page.goto(base, { waitUntil: 'networkidle' });
check('открытие без хэша — приветствие', (await page.locator('.app-section-tile').count()) === 3);

await browser.close();
server.close();

if (failed > 0) {
  console.error(`\nпровалено: ${failed}`);
  process.exit(1);
}
console.log('\nвсе проверки прошли; снимок — home-screen.png');