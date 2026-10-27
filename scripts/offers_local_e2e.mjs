/**
 * Интеграционный прогон offers-service через HTTP на живом контейнере
 * (локальный docker-compose.local.yml). Проверяет поведение, а не реализацию:
 * те же запросы, что делает плагин и бот.
 *
 * Запуск: node scripts/offers_local_e2e.mjs [baseUrl]
 */
import { createHmac } from 'node:crypto';

const BASE = process.argv[2] ?? 'http://127.0.0.1:18080';
const SECRET = 'test-service-secret';
const BOT_SECRET = 'test-bot-secret';

let passed = 0;
let failed = 0;

function ok(name, condition, detail = '') {
  if (condition) {
    passed += 1;
    console.log(`  ✓ ${name}`);
  } else {
    failed += 1;
    console.log(`  ✗ ${name}${detail ? ' — ' + detail : ''}`);
  }
}

function b64url(input) {
  return Buffer.from(input).toString('base64url');
}

/** JWT HS256 с app_id=offers, как выдаёт auth-service. */
function makeJwt(email, appId = 'offers', ttlSec = 3600) {
  const header = b64url(JSON.stringify({ alg: 'HS256', typ: 'JWT' }));
  const now = Math.floor(Date.now() / 1000);
  const payload = b64url(JSON.stringify({
    email,
    app_id: appId,
    device_id: 'test',
    iss: 'auth-service',
    aud: appId,
    exp: now + ttlSec,
    iat: now,
  }));
  const sig = createHmac('sha256', SECRET).update(`${header}.${payload}`).digest('base64url');
  return `${header}.${payload}.${sig}`;
}

async function call(method, path, { token, body, headers = {} } = {}) {
  const res = await fetch(`${BASE}/api/offers${path}`, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...headers,
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  let json;
  try {
    json = text ? JSON.parse(text) : undefined;
  } catch {
    json = undefined;
  }
  return { status: res.status, json, text, headers: res.headers };
}

const ownerToken = makeJwt('owner@tn.ru');

console.log('\n1. Здоровье и Swagger');
{
  const h = await call('GET', '/health');
  ok('health → 200 ok', h.status === 200 && h.json?.status === 'ok');
  const docs = await fetch(`${BASE}/api/offers/docs/`);
  ok('страница Swagger открывается', docs.status === 200);
  const schema = await fetch(`${BASE}/api/offers/openapi.yaml`);
  const yaml = await schema.text();
  ok('схема OpenAPI отдаётся', schema.status === 200 && yaml.includes('openapi:'));
  ok('в схеме есть каталог', yaml.includes('/api/offers/catalog'));
}

console.log('\n2. Авторизация');
{
  const none = await call('GET', '/catalog');
  ok('без токена → 401', none.status === 401, `получили ${none.status}`);

  const foreign = await call('GET', '/catalog', { token: makeJwt('owner@tn.ru', 'documents') });
  ok('чужой app_id → 403', foreign.status === 403, `получили ${foreign.status}`);

  const expired = await call('GET', '/catalog', { token: makeJwt('owner@tn.ru', 'offers', -3600) });
  ok('просроченный токен → 401', expired.status === 401, `получили ${expired.status}`);

  const bad = await call('GET', '/catalog', { token: 'garbage.garbage.garbage' });
  ok('битый токен → 401', bad.status === 401, `получили ${bad.status}`);

  const me = await call('GET', '/permissions/me', { token: ownerToken });
  ok('me отдаёт email', me.json?.email === 'owner@tn.ru', JSON.stringify(me.json));
}

console.log('\n3. Каталог (правка карточек)');
let cardId = 0;
{
  const created = await call('POST', '/catalog', {
    token: ownerToken,
    body: { kind: 'system', name: 'Кровельная система ПВХ', unit: 'м²', basePrice: 999.99, description: 'тест' },
  });
  cardId = created.json?.id ?? 0;
  ok('карточка создаётся', created.status === 200 && cardId > 0, `status ${created.status}`);
  ok('цена сохраняется как есть', created.json?.basePrice === 999.99, String(created.json?.basePrice));

  const list = await call('GET', '/catalog', { token: ownerToken });
  ok('карточка видна в списке', list.json?.items?.some((i) => i.id === cardId));

  const search = await call('GET', `/catalog?q=${encodeURIComponent('кровель')}`, { token: ownerToken });
  ok('поиск по слову находит', (search.json?.items ?? []).some((i) => i.id === cardId));

  const miss = await call('GET', `/catalog?q=${encodeURIComponent('нетакого')}`, { token: ownerToken });
  ok('поиск по несуществующему пуст', (miss.json?.items ?? []).length === 0);

  const noName = await call('POST', '/catalog', { token: ownerToken, body: { name: '' } });
  ok('без названия → 400', noName.status === 400, `получили ${noName.status}`);

  const badKind = await call('POST', '/catalog', { token: ownerToken, body: { name: 'X', kind: 'прочее' } });
  ok('неизвестный тип → 400', badKind.status === 400, `получили ${badKind.status}`);

  const negPrice = await call('POST', '/catalog', { token: ownerToken, body: { name: 'X', basePrice: -1 } });
  ok('отрицательная цена → 400', negPrice.status === 400, `получили ${negPrice.status}`);

  const updated = await call('PUT', `/catalog/${cardId}`, {
    token: ownerToken,
    body: { kind: 'system', name: 'Кровельная система ПВХ', unit: 'м²', basePrice: 1200, description: 'правка' },
  });
  ok('карточка правится', updated.status === 200 && updated.json?.basePrice === 1200, String(updated.json?.basePrice));
  ok('updatedAt сдвинулся', updated.json?.updatedAt !== created.json?.updatedAt);

  const foreignPut = await call('PUT', '/catalog/999999', {
    token: ownerToken,
    body: { kind: 'system', name: 'Призрак' },
  });
  ok('правка несуществующей → ошибка', foreignPut.status >= 400, `получили ${foreignPut.status}`);
}

console.log('\n4. Форма КП');
{
  const form = await call('GET', '/form', { token: ownerToken });
  ok('форма отдаётся', form.status === 200);
  ok('в форме есть поля по умолчанию', (form.json?.placeholders ?? []).length > 0, JSON.stringify(form.json?.placeholders?.length));

  const saved = await call('PUT', '/form', {
    token: ownerToken,
    body: {
      mode: 'client',
      title: 'Коммерческое предложение',
      placeholders: [
        { id: 'client_name', label: 'Клиент' },
        { id: 'valid_until', label: 'Действует до' },
      ],
      blocks: [
        {
          id: 'intro',
          title: 'Вступление',
          content: [{ type: 'paragraph', text: 'Клиент: {{client_name}}' }],
          showInClient: true,
          showInInternal: false,
        },
      ],
      requiredFields: ['client_name'],
    },
  });
  ok('форма сохраняется', saved.status === 200, `status ${saved.status}`);
  ok('поля сохранились', (saved.json?.placeholders ?? []).length === 2);

  const back = await call('GET', '/form', { token: ownerToken });
  ok('форма читается обратно', back.json?.placeholders?.[0]?.id === 'client_name');

  const dupBlock = await call('PUT', '/form', {
    token: ownerToken,
    body: {
      mode: 'client',
      title: 'X',
      placeholders: [{ id: 'a', label: 'A' }],
      blocks: [
        { id: 'same', title: '1', content: [], showInClient: true, showInInternal: false },
        { id: 'same', title: '2', content: [], showInClient: true, showInInternal: false },
      ],
      requiredFields: [],
    },
  });
  ok('дубль блока → 400', dupBlock.status === 400, `получили ${dupBlock.status}`);

  const badRequired = await call('PUT', '/form', {
    token: ownerToken,
    body: { mode: 'client', title: 'X', placeholders: [], blocks: [], requiredFields: ['неттакого'] },
  });
  ok('обязательное поле вне списка → 400', badRequired.status === 400, `получили ${badRequired.status}`);

  const emptyTitle = await call('PUT', '/form', {
    token: ownerToken,
    body: { mode: 'client', title: '   ', placeholders: [], blocks: [], requiredFields: [] },
  });
  ok('пустой заголовок → 400', emptyTitle.status === 400, `получили ${emptyTitle.status}`);

  // вернуть форму по умолчанию для следующих шагов
  await call('PUT', '/form', {
    token: ownerToken,
    body: {
      mode: 'client',
      title: 'Коммерческое предложение',
      placeholders: [{ id: 'client_name', label: 'Клиент' }],
      blocks: [
        { id: 'intro', title: 'Вступление', content: [{ type: 'paragraph', text: 'Клиент: {{client_name}}' }], showInClient: true, showInInternal: false },
      ],
      requiredFields: [],
    },
  });
}

console.log('\n5. Предложения: расчёт, нумерация, права');
let offerId = 0;
{
  const created = await call('POST', '/offers', {
    token: ownerToken,
    body: {
      client: { name: 'ООО "Ромашка" & <Ко>', contact: 'Иван' },
      items: [{ catalogId: cardId, name: 'Система', unit: 'м²', qty: 3, price: 999.99 }],
      formValues: { client_name: 'ООО Ромашка' },
    },
  });
  offerId = created.json?.id ?? 0;
  ok('предложение создаётся', created.status === 200 && offerId > 0, `status ${created.status}`);
  ok('итог без хвостов float', created.json?.total === 2999.97, String(created.json?.total));
  ok('сумма строки точная', created.json?.items?.[0]?.sum === 2999.97, String(created.json?.items?.[0]?.sum));
  ok('номер вида NNN/ГОД', /^\d{3,}\/\d{4}$/.test(created.json?.number ?? ''), created.json?.number);

  const second = await call('POST', '/offers', {
    token: ownerToken,
    body: { client: { name: 'Второй' }, items: [{ name: 'Позиция', unit: 'шт', qty: 1, price: 10 }] },
  });
  ok('второй номер отличается', second.json?.number !== created.json?.number, `${created.json?.number} / ${second.json?.number}`);
  const n1 = Number((created.json?.number ?? '0/0').split('/')[0]);
  const n2 = Number((second.json?.number ?? '0/0').split('/')[0]);
  ok('нумерация последовательна', n2 === n1 + 1, `${n1} → ${n2}`);

  const empty = await call('POST', '/offers', { token: ownerToken, body: { client: {}, items: [] } });
  ok('пустое предложение → 400', empty.status === 400, `получили ${empty.status}`);
  ok('текст отказа понятен', (empty.json?.error ?? '').includes('нет позиций'), empty.json?.error);

  const zeroQty = await call('POST', '/offers', {
    token: ownerToken,
    body: { items: [{ name: 'X', qty: 0, price: 10 }] },
  });
  ok('нулевое количество → 400', zeroQty.status === 400, `получили ${zeroQty.status}`);

  const list = await call('GET', '/offers', { token: ownerToken });
  ok('чужой админ видит все предложения', (list.json?.offers ?? []).length >= 2, String((list.json?.offers ?? []).length));

  const stranger = makeJwt('stranger@tn.ru');
  const strangerList = await call('GET', '/offers', { token: stranger });
  ok('чужак без роли видит только свои (пусто)', (strangerList.json?.offers ?? []).length === 0, String((strangerList.json?.offers ?? []).length));
}

console.log('\n6. DOCX');
{
  const res = await fetch(`${BASE}/api/offers/offers/${offerId}/export?format=docx`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${ownerToken}` },
  });
  const buf = Buffer.from(await res.arrayBuffer());
  ok('экспорт → 200', res.status === 200, `status ${res.status}`);
  ok('это zip (docx)', buf[0] === 0x50 && buf[1] === 0x4b, `первые байты: ${buf[0]},${buf[1]}`);
  ok('размер правдоподобный', buf.length > 20000, `${buf.length} байт`);
  const contentType = res.headers.get('content-type') ?? '';
  ok('content-type docx', contentType.includes('wordprocessingml'), contentType);

  const noPositions = await call('POST', '/offers', {
    token: ownerToken,
    body: { items: [{ name: 'X', qty: 1, price: 1 }] },
  });
  ok('предложение для проверки создано', noPositions.status === 200);
}

console.log('\n7. Права на чужое предложение');
{
  const otherToken = makeJwt('other@tn.ru');
  const patch = await call('PUT', `/offers/${offerId}`, {
    token: otherToken,
    body: { client: { name: 'Взлом' }, items: [{ name: 'X', qty: 1, price: 1 }] },
  });
  ok('чужая правка не проходит', patch.status >= 400, `получили ${patch.status}`);

  const del = await call('DELETE', `/offers/${offerId}`, { token: otherToken });
  ok('чужое удаление не проходит', del.status >= 400, `получили ${del.status}`);
}

console.log('\n8. Ручки бота защищены секретом');
{
  const noSecret = await call('GET', '/bot/identity?chat_id=1');
  ok('без секрета → 401', noSecret.status === 401, `получили ${noSecret.status}`);

  const wrong = await call('GET', '/bot/identity?chat_id=1', { headers: { 'X-Service-Secret': 'wrong-secret' } });
  ok('неверный секрет → 403', wrong.status === 403, `получили ${wrong.status}`);

  const unknownChat = await call('GET', '/bot/identity?chat_id=zzz', { headers: { 'X-Service-Secret': BOT_SECRET } });
  ok('непривязанный чат → 404', unknownChat.status === 404, `получили ${unknownChat.status}`);

  const noChatDocx = await call('POST', `/bot/offers/${offerId}/docx`, { headers: { 'X-Service-Secret': BOT_SECRET } });
  ok('DOCX без chat_id запрещён', noChatDocx.status >= 400, `получили ${noChatDocx.status}`);

  const noChatLink = await call('POST', `/bot/offers/${offerId}/link`, { headers: { 'X-Service-Secret': BOT_SECRET } });
  ok('ссылка без chat_id запрещена', noChatLink.status >= 400, `получили ${noChatLink.status}`);

  const catalog = await call('GET', '/bot/catalog', { headers: { 'X-Service-Secret': BOT_SECRET } });
  ok('бот видит каталог', catalog.status === 200 && (catalog.json?.items ?? []).length > 0);
}

console.log('\n9. Коды входа (без почты: проверяем отказы)');
{
  const badEmail = await call('POST', '/bot/request-code', {
    headers: { 'X-Service-Secret': BOT_SECRET },
    body: { email: 'not-an-email', chat_id: '100' },
  });
  ok('некорректный e-mail → 400', badEmail.status === 400, `получили ${badEmail.status}`);

  const noCode = await call('POST', '/bot/verify-code', {
    headers: { 'X-Service-Secret': BOT_SECRET },
    body: { chat_id: '100', email: 'nobody@tn.ru', code: '000000' },
  });
  ok('код без запроса → 404', noCode.status === 404, `получили ${noCode.status}`);

  const issueNoAuth = await call('POST', '/access/codes', { body: { email: 'x@tn.ru' } });
  ok('выдача кода без JWT → 401', issueNoAuth.status === 401, `получили ${issueNoAuth.status}`);
}

console.log('\n10. Идемпотентность и повторные запросы');
{
  const a = await call('GET', '/catalog', { token: ownerToken });
  const b = await call('GET', '/catalog', { token: ownerToken });
  ok('повторный GET даёт тот же результат', JSON.stringify(a.json) === JSON.stringify(b.json));

  const del1 = await call('DELETE', `/catalog/${cardId}`, { token: ownerToken });
  const del2 = await call('DELETE', `/catalog/${cardId}`, { token: ownerToken });
  ok('повторная архивация не падает', del1.status === 200 && del2.status === 200, `${del1.status}/${del2.status}`);

  const afterArchive = await call('GET', '/catalog', { token: ownerToken });
  ok('архивная карточка скрыта', !(afterArchive.json?.items ?? []).some((i) => i.id === cardId));
  const withArchived = await call('GET', '/catalog?archived=1', { token: ownerToken });
  ok('с archived=1 карточка видна', (withArchived.json?.items ?? []).some((i) => i.id === cardId));
}

console.log(`\nИтого: ${passed} прошло, ${failed} провалилось`);
process.exit(failed === 0 ? 0 : 1);