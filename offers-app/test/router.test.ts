/**
 * Маршруты приложения — проверяются как поведение: на вход адрес из бота,
 * на выход экран, который человек увидит (2026-11-13, S0).
 *
 * Тесты гоняются обычным `node --test` (как в offers-bot) — отдельного
 * раннера в приложении нет.
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import { parseRoute, routeHash } from '../src/router.ts';

test('пустой адрес открывает список КП', () => {
  assert.deepEqual(parseRoute(''), { name: 'home' });
  assert.deepEqual(parseRoute('#/'), { name: 'home' });
});

test('список листков и поиск по клиенту', () => {
  assert.deepEqual(parseRoute('#/list/sheets'), { name: 'list', kind: 'sheets', query: '' });
  assert.deepEqual(parseRoute('#/list/offers?q=Строй'), {
    name: 'list',
    kind: 'offers',
    query: 'Строй',
  });
});

test('карточка КП и боевого листка несут номер', () => {
  assert.deepEqual(parseRoute('#/offer/123'), { name: 'offer', id: 123 });
  assert.deepEqual(parseRoute('#/sheet/7'), { name: 'sheet', id: 7 });
});

test('номер КП нечисловой или нулевой — экран не найден, а не исключение', () => {
  assert.equal(parseRoute('#/offer/abc').name, 'notFound');
  assert.equal(parseRoute('#/offer/0').name, 'notFound');
  assert.equal(parseRoute('#/offer/-5').name, 'notFound');
});

test('неизвестный путь не роняет приложение', () => {
  assert.equal(parseRoute('#/nonsense').name, 'notFound');
  assert.equal(parseRoute('#/list/whatever').name, 'notFound');
});

test('адрес для бота собирается и читается тем же разбором', () => {
  assert.equal(routeHash({ name: 'offer', id: 12 }), '#/offer/12');
  assert.equal(routeHash({ name: 'sheet', id: 3 }), '#/sheet/3');
  assert.equal(routeHash({ name: 'home' }), '#/');

  const withQuery = routeHash({ name: 'list', kind: 'sheets', query: 'мембрана' });
  const parsed = parseRoute(withQuery);
  assert.deepEqual(parsed, { name: 'list', kind: 'sheets', query: 'мембрана' });
});

test('база в адресе бота — путь /app-offers/ перед хэшем', () => {
  assert.equal(routeHash({ name: 'offer', id: 1 }, 'https://epyur.fvds.ru/app-offers/'), 'https://epyur.fvds.ru/app-offers/#/offer/1');
  assert.equal(routeHash({ name: 'offer', id: 1 }, 'https://epyur.fvds.ru/app-offers'), 'https://epyur.fvds.ru/app-offers/#/offer/1');
});

test('хэш от Telegram (tgWebAppData) не превращается в «экран не найден»', () => {
  // Живой случай 2026-11-13: платформа дописывает в хэш свои параметры, и без
  // их отбрасывания человек видел «Адрес не распознан» на самом первом экране.
  const tg =
    '#tgWebAppData=query_id%3DAAFe&tgWebAppVersion=9.6&tgWebAppPlatform=tdesktop';
  assert.deepEqual(parseRoute(tg), { name: 'home' });
  assert.notEqual(parseRoute(tg).name, 'notFound');
});

test('маршрут из startapp платформы уважается', () => {
  const tg = '#tgWebAppData=abc&tgWebAppStartParam=%2Foffer%2F12&tgWebAppVersion=9.6';
  assert.deepEqual(parseRoute(tg), { name: 'offer', id: 12 });
});

test('приветствие и «Скорая помощь» — свои маршруты', () => {
  assert.deepEqual(parseRoute(routeHash({ name: 'home' })), { name: 'home' });
  assert.deepEqual(parseRoute(routeHash({ name: 'ambulance' })), { name: 'ambulance' });
  assert.equal(routeHash({ name: 'ambulance' }), '#/ambulance');
  assert.equal(parseRoute('#/ambulance/whatever').name, 'notFound');
});

test('свой поиск в адресе от бота сохраняется', () => {
  assert.deepEqual(parseRoute('#/list/offers?q=Строй'), {
    name: 'list',
    kind: 'offers',
    query: 'Строй',
  });
});
