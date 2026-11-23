/**
 * Проверка приветствия и переходов между разделами (2026-11-14).
 *
 * Поднимает `dist/` на 127.0.0.1 и гоняет настоящий браузер:
 *   `#/`            — четыре карточки разделов, «Скорая помощь» первой;
 *   `#/list/offers` — список КП и строка разделов;
 *   `#/ambulance`   — экран «раздел ещё готовится»;
 *   `#/digest`      — новости Технодайджеста и кнопка «Отправить в чат» у каждой;
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

// SDK платформы подменяется крошечной заглушкой (2026-11-23). Настоящий
// `vendor/telegram-web-app.js` вне клиента Telegram отдаёт ПУСТОЙ initData, и
// тогда «Отправить в чат» честно отвечает «окно открыто без ключа Telegram» —
// проверить отправку было бы нельзя. Заглушка даёт подпись и ровно те методы,
// которые приложение у платформы спрашивает (`src/tg.ts`).
const TELEGRAM_STUB = `
window.Telegram = { WebApp: {
  initData: 'query_id=AAFe&user=%7B%22id%22%3A1%7D&auth_date=1',
  initDataUnsafe: { user: { id: 1 } },
  colorScheme: 'light',
  themeParams: {},
  version: '9.6',
  platform: 'web',
  ready() {}, expand() {}, close() {}, sendData() {},
  onEvent() {}, offEvent() {},
}};
`;

// Заглушка подменяет файл SDK целиком — что сам файл есть в сборке,
// проверяется ниже, когда объявлен счётчик проверок.

await page.route('**/vendor/telegram-web-app.js', (route) =>
  route.fulfill({ status: 200, contentType: 'text/javascript; charset=utf-8', body: TELEGRAM_STUB }),
);

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
  if (url.includes('/app/news')) {
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) });
  }
  // Технодайджест (2026-11-23): два выпуска и две новости — проверяется, что
  // лента нарисована и у КАЖДОЙ новости есть своя кнопка отправки.
  //
  // Картинка новости (2026-11-23) — подписанная ссылка сервиса. Отдаём
  // настоящий PNG: без байтов проверка «кадр нарисован» ничего не доказывала
  // бы (битый <img> тоже существует в разметке). Ветка стоит ВЫШЕ общей
  // `/digest`: адрес картинки тоже содержит это слово.
  if (url.includes('/digest/image')) {
    return route.fulfill({
      status: 200,
      contentType: 'image/png',
      body: Buffer.from(
        'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=',
        'base64',
      ),
    });
  }
  if (url.includes('/digest')) {
    // Несуществующий выпуск — для проверки, что отказ не тупик.
    if (url.includes('ym=2025-01')) {
      return route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ error: 'такого выпуска нет' }) });
    }
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        current: '2026-11',
        issues: [
          { ym: '2026-11', title: 'Технические новости ноября 2026', items: 2 },
          { ym: '2026-10', title: 'Технические новости октября 2026', items: 1 },
        ],
        issue: { ym: '2026-11', title: 'Технические новости ноября 2026', items: 2 },
        items: [
          {
            id: 7,
            title: 'Новая мембрана',
            lead: 'Коротко о главном',
            body: 'Первый абзац.\n\nВторой абзац.',
            rubric: 'Материалы',
            author: 'Иван Петров',
            at: '2026-11-05T09:00:00Z',
            isLead: true,
            linkUrl: 'https://example.org/lead',
            linkText: 'К расчёту на nav.tn.ru',
            imagePath: '/digest/image?hash=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&name=kadr.png&sig=ff',
          },
          {
            id: 8,
            title: 'Вторая новость',
            lead: '',
            body: 'Текст второй новости.',
            rubric: '',
            author: '',
            at: '2026-11-07T09:00:00Z',
            isLead: false,
            linkUrl: 'https://example.org/news',
            linkText: 'Источник',
            // Кадр есть и у обычной новости: в мини-аппе он идёт миниатюрой,
            // и путь к нему проверяется так же, как у ведущей.
            imagePath: '/digest/image?hash=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&name=mini.png&sig=ff',
          },
          {
            id: 9,
            title: 'Третья новость',
            lead: 'Третья — чтобы карточек было ДВЕ и можно было проверить зазор.',
            body: 'Текст третьей новости.',
            rubric: 'Инструкция',
            author: '',
            at: '2026-11-08T09:00:00Z',
            isLead: false,
            linkUrl: '',
            linkText: '',
            imagePath: '',
          },
        ],
      }),
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
      // Смешанное окно (0.11.3): зона → поле → зона → поле. Именно этот
      // порядок ломался бы, если поля рисовать отдельным блоком.
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          window: [
            { kind: 'zone', id: 'client', visible: true },
            { kind: 'field', id: 'object_name', label: 'Объект', visible: true },
            { kind: 'zone', id: 'sections', visible: true },
            { kind: 'field', id: 'conditions', label: 'Условия', visible: true },
            { kind: 'field', id: 'hidden_one', label: 'Скрытое', visible: false },
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

// Заглушка SDK подменяет файл целиком, поэтому отдельно убеждаемся, что сам
// файл попал в сборку: иначе его пропажа прошла бы незамеченной.
check('сборка: SDK Telegram на месте', existsSync(join(DIST, 'vendor', 'telegram-web-app.js')));

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
// Лента приходит запросом ПОСЛЕ отрисовки заголовка: ждём новости, а не
// считаем их сразу — иначе счётчик видит ещё пустой экран.
await page.locator('.app-news-title').first().waitFor({ timeout: 10000 });
check('«Технодайджест»: новости выпуска нарисованы', (await page.locator('.app-news-title').count()) === 3);
check(
  '«Технодайджест»: заголовки новостей — из Базы знаний',
  ((await page.locator('.app-news-title').allTextContents()).join('|')) === 'Новая мембрана|Вторая новость|Третья новость',
);
check(
  '«Технодайджест»: выпуски для переключения месяцев',
  (await page.locator('.app-month').count()) === 2,
);
check(
  '«Технодайджест»: у КАЖДОЙ новости своя кнопка «Отправить в чат»',
  (await page.locator('.app-news-send button').count()) === 3,
);
// Карточки новостей (2026-11-23, жалоба «разъезжаются по ширине и наезжают друг
// на друга»): каждая новость — в рамке своего цвета, как ведущая, и между ними
// есть зазор. Проверяются вычисленные стили и расстояния, а не имена классов.
const articleCards = page.locator('.app-digest-news');
const cardStyle = await articleCards.first().evaluate((el) => {
  const css = getComputedStyle(el);
  return { bg: css.backgroundColor, border: css.borderTopWidth, radius: css.borderTopLeftRadius };
});
check(
  '«Технодайджест»: карточка новости — в рамке на своей подложке',
  cardStyle.bg !== 'rgba(0, 0, 0, 0)' && cardStyle.border !== '0px' && cardStyle.radius !== '0px',
  JSON.stringify(cardStyle),
);
const firstCard = await articleCards.nth(0).boundingBox();
const secondCard = await articleCards.nth(1).boundingBox();
check(
  '«Технодайджест»: карточки новостей не наезжают друг на друга',
  firstCard !== null && secondCard !== null && secondCard.y - (firstCard.y + firstCard.height) >= 8,
  firstCard && secondCard ? `зазор ${secondCard.y - (firstCard.y + firstCard.height)}px` : 'нет карточек',
);
check(
  '«Технодайджест»: карточка уже полосы (есть отступы по краям)',
  firstCard !== null && firstCard.x > 15 && firstCard.width < 430 - 30,
  firstCard ? `x=${firstCard.x}, ширина=${firstCard.width}` : 'нет карточки',
);
// Брендовая полоса (2026-11-23, макет тёмной темы): шапка с эмблемой и
// плашкой месяца, кадр ведущей новости, подвал. Проверяются те части макета,
// которые видит человек, а не наличие классов в бандле.
check(
  '«Технодайджест»: эмблема ТН в шапке полосы',
  ((await page.locator('.app-digest-mark').first().textContent()) ?? '').trim() === 'TN',
);
check(
  '«Технодайджест»: название полосы в шапке',
  ((await page.locator('.app-digest-word').first().textContent()) ?? '').includes('ТЕХНОДАЙДЖЕСТ'),
);
check(
  '«Технодайджест»: плашка месяца выпуска',
  ((await page.locator('.app-digest-stamp').first().textContent()) ?? '').includes('Ноябрь 2026'),
);
// Кадр грузится лениво (`loading="lazy"`) — ждём его, иначе проверка увидела
// бы «complete ещё false» на честно работающей картинке.
await page
  .waitForFunction(
    () => {
      const img = document.querySelector('.app-digest-photo');
      return img instanceof HTMLImageElement && img.complete && img.naturalWidth > 0;
    },
    undefined,
    { timeout: 10000 },
  )
  .catch(() => {});
check(
  '«Технодайджест»: картинка ведущей новости загружена',
  await page
    .locator('.app-digest-photo')
    .first()
    .evaluate((img) => img.complete && img.naturalWidth > 0),
);
// Миниатюра ниже первого экрана и грузится лениво: прокручиваем к ней, иначе
// проверка честно не увидит загрузку.
await page.locator('.app-digest-thumb').first().scrollIntoViewIfNeeded();
await page
  .waitForFunction(
    () => {
      const img = document.querySelector('.app-digest-thumb');
      return img instanceof HTMLImageElement && img.complete && img.naturalWidth > 0;
    },
    undefined,
    { timeout: 10000 },
  )
  .catch(() => {});
check(
  '«Технодайджест»: у обычной новости картинка идёт миниатюрой',
  await page
    .locator('.app-digest-thumb')
    .first()
    .evaluate((img) => img.complete && img.naturalWidth > 0),
);
check(
  '«Технодайджест»: у ведущей новости кнопка-ссылка на источник',
  ((await page.locator('.app-digest-button').first().textContent()) ?? '').includes('nav.tn.ru'),
);
check(
  '«Технодайджест»: баннер источника ведёт на адрес ведущей новости',
  (await page.locator('.app-digest-outline').first().getAttribute('href')) === 'https://example.org/lead',
);
check(
  '«Технодайджест»: подвал — техническая дирекция',
  ((await page.locator('.app-digest-foot-sub').first().textContent()) ?? '').includes('ПМиПИР'),
);
await page.locator('.app-news-send button').first().click();
await page.waitForFunction(
  () => (document.querySelector('.app-news-send button')?.textContent ?? '').includes('Отправлено'),
);
check(
  '«Технодайджест»: отправка в чат подтверждена на экране',
  ((await page.locator('.app-news-send button').first().textContent()) ?? '').includes('Отправлено'),
);
// Повторное нажатие по «Отправлено» не должно слать второе сообщение.
check(
  '«Технодайджест»: после отправки кнопка выключена',
  await page.locator('.app-news-send button').first().isDisabled(),
);
check('«Технодайджест»: нет ошибок JS', errors.length === 0, errors.slice(0, 2).join(' | '));

// Отказ по месяцу не оставляет человека в тупике: есть путь к текущему выпуску.
// Ошибки считаем с этой точки отдельно: намеренный 404 ниже попадёт в консоль
// как «Failed to load resource» и не должен краснить следующие экраны.
const errorsBeforeMonth = errors.length;
await page.goto(`${base}#/digest/2025-01`, { waitUntil: 'networkidle' });
await page.locator('.app-error').first().waitFor({ timeout: 10000 });
check(
  '«Технодайджест»: у отказа есть ссылка «К текущему выпуску»',
  (await page.getByRole('link', { name: 'К текущему выпуску' }).count()) === 1,
);
for (let i = errors.length - 1; i >= errorsBeforeMonth; i -= 1) {
  if (/404/.test(errors[i] ?? '')) errors.splice(i, 1);
}

// Окно КП (2026-11-15, шаг 3): экран рисует ТОЛЬКО зоны из настройки и в её
// порядке. Подмена `/form` выше отдаёт «позиции, заказчик» и прячет остальные.
await page.goto(`${base}#/offer/1`, { waitUntil: 'networkidle' });
// Карточка собирается запросом после перехода: ждём её, иначе список зон
// измеряется на ещё пустом экране (гонка, поймана проверкой 2026-11-23).
await page.locator('main .app-card').first().waitFor({ timeout: 10000 });
const zoneTitles = (await page.locator('.app-card h2').allTextContents()).map((s) => s.trim());
check('карточка КП: рисуются зоны из окна', zoneTitles.join('|') === 'Позиции|Заказчик', zoneTitles.join('|'));
check('карточка КП: нет ошибок JS', errors.length === 0, errors.slice(0, 2).join(' | '));

// Поля формы и порядок окна (0.11.3). До 0.11.3 поля не рисовались вовсе:
// visibleZones отбрасывала kind:"field", а FieldsZone не существовал.
// Сторож из совещания 2026-11-16: проверяем, что поле ОТДАНО и НАРИСОВАНО,
// и что порядок «зона → поле → зона → поле» сохранён.
briefScreen = true;

await page.reload({ waitUntil: 'networkidle' });

const order = await page.evaluate(() => {
  const nodes = Array.from(document.querySelectorAll('main .app-card, main .app-section'));
  return nodes.map((n) => (n.querySelector('.app-h2')?.textContent ?? n.textContent ?? '').trim().slice(0, 40));
});
check('экран: поле «Объект» нарисовано', (await page.getByLabel('Объект').count()) === 1);
check('экран: поле «Условия» нарисовано', (await page.getByLabel('Условия').count()) === 1);
check('экран: скрытое поле не нарисовано', (await page.getByLabel('Скрытое').count()) === 0);
check(
  'экран: порядок «зона → поле → зона → поле» сохранён',
  (() => {
    const text = order.join('|');
    const z = text.indexOf('Заказчик');
    const f1 = text.indexOf('Объект');
    const z2 = text.indexOf('Тексты разделов');
    const f2 = text.indexOf('Условия');
    return z >= 0 && f1 > z && z2 > f1 && f2 > z2;
  })(),
  order.join(' | '),
);
check('блок брифа как зоны больше не рисуется', (await page.locator('.app-brief-fields').count()) === 0);
check('экран: нет ошибок JS', errors.length === 0, errors.slice(0, 2).join(' | '));

await page.goto(base, { waitUntil: 'networkidle' });
check('открытие без хэша — приветствие', (await page.locator('.app-section-tile').count()) === 4);

await browser.close();
server.close();

if (failed > 0) {
  console.error(`\nпровалено: ${failed}`);
  process.exit(1);
}
console.log('\nвсе проверки прошли; снимок — home-screen.png');