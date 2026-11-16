/**
 * Проверка приветствия и переходов между разделами (2026-11-14).
 *
 * Поднимает `dist/` на 127.0.0.1 и гоняет настоящий браузер:
 *   `#/`            — четыре карточки разделов, «Скорая помощь» первой;
 *   `#/list/offers` — список КП и строка разделов;
 *   `#/ambulance`   — экран «раздел ещё готовится»;
 *   `#/digest`      — экран «раздел проектируется»;
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

// Режим ответа (2026-11-15): вторая половина проверки смотрит бриф, поэтому
// окно и ответ брифа переключаются флагом, а не ВТОРЫМ обработчиком маршрута:
// page.route дописывает стопкой, и первый ответил бы раньше второго.
let briefScreen = false;

await page.route('**/api/offers/**', (route) => {
  const url = route.request().url();
  if (url.includes('/app/session')) {
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ token: 'test', user: { email: 'test@tn.ru', role: 'admin' } }),
    });
  }
  // Окно КП (2026-11-15): только две зоны из пяти, в обратном порядке —
  // экран обязан нарисовать ровно их (проверка шага 3 ниже).
  if (url.includes('/brief')) {
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        brief: {
          offerId: 1,
          status: 'open',
          lastTopic: 'object_name',
          payload: { object_name: 'Дом на Охотской', customer_pains: ['дорого', 'долго'] },
        },
        turns: [{ id: 1, role: 'manager', text: 'Дом на Охотской', topic: 'object_name', createdAt: '' }],
      }),
    });
  }
  if (url.includes('/form')) {
    if (briefScreen) {
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          window: [
            { kind: 'zone', id: 'brief', visible: true },
            { kind: 'zone', id: 'items', visible: true },
          ],
        }),
      });
    }
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        window: [
          { kind: 'zone', id: 'items', visible: true },
          { kind: 'zone', id: 'client', visible: true },
          { kind: 'zone', id: 'systems', visible: false },
          { kind: 'zone', id: 'materials', visible: false },
          { kind: 'zone', id: 'sections', visible: false },
        ],
      }),
    });
  }
  if (/\/offers\/\d+$/.test(url.split('?')[0] ?? '')) {
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        id: 1,
        number: '001/2026',
        ownerEmail: 'test@tn.ru',
        client: { name: '', contact: '' },
        items: [],
        formValues: {},
        systems: [],
        total: 0,
        status: 'draft',
        createdAt: '2026-11-15T09:00:00Z',
        updatedAt: '2026-11-15T09:00:00Z',
      }),
    });
  }
  const body = url.includes('/combat') ? { sheets: [] } : url.includes('/systems') ? { systems: [] } : { offers: [] };
  return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
});

let failed = 0;
function check(name, ok, detail = '') {
  console.log(`${ok ? 'OK  ' : 'FAIL'} ${name}${detail === '' ? '' : ` — ${detail}`}`);
  if (!ok) failed += 1;
}

await page.goto(`${base}#/`, { waitUntil: 'networkidle' });
check('главная: 4 карточки разделов', (await page.locator('.app-section-tile').count()) === 4);
check(
  'главная: «Скорая помощь» — первая карточка',
  ((await page.locator('.app-section-title').first().textContent()) ?? '').includes('Скорая'),
);
check(
  'главная: «Технодайджест» — карточка с названием',
  (await page.locator('.app-section-title', { hasText: 'Технодайджест' }).count()) === 1,
);
check('главная: нет ошибок JS', errors.length === 0, errors.slice(0, 2).join(' | '));
// Снимок — рядом с приложением, НЕ в `dist/`: dist уходит на сервер целиком и
// считается в бюджете (`npm run size`), а картинка проверки там лишняя.
await page.screenshot({ path: join(ROOT, 'home-screen.png'), fullPage: true });

await page.goto(`${base}#/list/offers`, { waitUntil: 'networkidle' });
check('список КП: заголовок списка', ((await page.locator('main h1').first().textContent()) ?? '').includes('Коммерческие'));
check(
  'список КП: «Скорая помощь» — первая в строке разделов',
  ((await page.locator('.app-section-chip').first().textContent()) ?? '').includes('Скорая'),
);

await page.goto(`${base}#/ambulance`, { waitUntil: 'networkidle' });
check(
  '«Скорая помощь»: свой экран',
  ((await page.locator('main h1').first().textContent()) ?? '').includes('Скорая'),
);

await page.goto(`${base}#/digest`, { waitUntil: 'networkidle' });
check(
  '«Технодайджест»: свой экран',
  ((await page.locator('main h1').first().textContent()) ?? '').includes('Технодайджест'),
);

// Окно КП (2026-11-15, шаг 3): экран рисует ТОЛЬКО зоны из настройки и в её
// порядке. Подмена `/form` выше отдаёт «позиции, заказчик» и прячет остальные.
await page.goto(`${base}#/offer/1`, { waitUntil: 'networkidle' });
const zoneTitles = (await page.locator('.app-card h2').allTextContents()).map((s) => s.trim());
check('карточка КП: рисуются зоны из окна', zoneTitles.join('|') === 'Позиции|Заказчик', zoneTitles.join('|'));
check('карточка КП: нет ошибок JS', errors.length === 0, errors.slice(0, 2).join(' | '));

// Блок «Бриф» и разбор файла (2026-11-15): зона брифа — отдельная настройка,
// поэтому её показ проверяется на своём окне. Ответы ручек уже стоят выше.
//
// Перезагрузка, а не смена хэша: `#/offer/1` уже открыт, и `Page.goto` с тем же
// адресом React не перемонтирует карточку — окно не перечиталось бы (тест на
// этом и упал: карточка брифа не появилась).
briefScreen = true;
await page.reload({ waitUntil: 'networkidle' });
const briefCard = page.locator('.app-card', { hasText: 'Бриф' });
check('блок брифа: карточка нарисована', (await briefCard.count()) >= 1);
check(
  'блок брифа: «Заполнено 2 из 14 тем»',
  ((await briefCard.locator('h2').first().textContent()) ?? '').includes('Заполнено 2 из 14'),
);
check(
  'блок брифа: значение темы показано',
  ((await briefCard.textContent()) ?? '').includes('Дом на Охотской'),
);
check(
  'блок брифа: открытый вопрос — поле ответа',
  (await briefCard.locator('.app-brief-answer textarea').count()) === 1,
);
check(
  'блок брифа: история разговора свёрнута',
  (await briefCard.locator('.app-brief-turns summary').count()) === 1,
);
check(
  'позиции: кнопка «Прислать файл для разбора»',
  (await page.locator('button', { hasText: 'Прислать файл для разбора' }).count()) === 1,
);
check(
  'блок брифа: кнопка «Продолжить бриф в боте»',
  (await page.locator('button', { hasText: 'Продолжить бриф в боте' }).count()) === 1,
);
check('блок брифа: нет ошибок JS', errors.length === 0, errors.slice(0, 2).join(' | '));

await page.goto(base, { waitUntil: 'networkidle' });
check('открытие без хэша — приветствие', (await page.locator('.app-section-tile').count()) === 4);

await browser.close();
server.close();

if (failed > 0) {
  console.error(`\nпровалено: ${failed}`);
  process.exit(1);
}
console.log('\nвсе проверки прошли; снимок — home-screen.png');