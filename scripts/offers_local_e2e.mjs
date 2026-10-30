/**
 * Интеграционный прогон offers-service через HTTP на живом контейнере
 * (локальный docker-compose.local.yml). Проверяет поведение, а не реализацию:
 * те же запросы, что делает плагин и бот.
 *
 * Запуск: node scripts/offers_local_e2e.mjs [baseUrl]
 */
import { createHmac } from 'node:crypto';
import { crc32, deflateRawSync } from 'node:zlib';

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

/** Минимальная запись zip: без неё сервис не отличит .xlsx от прочей мешанины. */
function zipEntry(name, data) {
  const nameBuf = Buffer.from(name, 'utf8');
  const body = deflateRawSync(data);
  const crc = crc32(data);
  const local = Buffer.alloc(30);
  local.writeUInt32LE(0x04034b50, 0);
  local.writeUInt16LE(20, 4);
  local.writeUInt16LE(0, 6);
  local.writeUInt16LE(8, 8);
  local.writeUInt32LE(crc, 14);
  local.writeUInt32LE(body.length, 18);
  local.writeUInt32LE(data.length, 22);
  local.writeUInt16LE(nameBuf.length, 26);
  local.writeUInt16LE(0, 28);
  return { name: nameBuf, data, body, crc, offset: 0 };
}

/**
 * Собирает .xlsx с одним листом. Строки таблицы превращаются в inline-строки
 * (без sharedStrings): сервис читает оба вида ячеек, а такой файл короче.
 *
 * Записи пишутся вручную, включая центральный каталог: без него zip нечитаем,
 * а Go не прощает ни одного несовпавшего поля.
 */
function makeXlsx(rows) {
  const colName = (i) => String.fromCharCode(65 + i);
  const sheetRows = rows.map((row, r) => {
    const cells = row.map((value, c) => {
      const ref = `${colName(c)}${r + 1}`;
      if (value === '' || value === null || value === undefined) return '';
      if (/^-?\d+(?:[.,]\d+)?$/.test(String(value))) {
        return `<c r="${ref}"><v>${String(value).replace(',', '.')}</v></c>`;
      }
      return `<c r="${ref}" t="inlineStr"><is><t>${String(value)}</t></is></c>`;
    }).join('');
    return `<row r="${r + 1}">${cells}</row>`;
  }).join('');

  const entries = [
    zipEntry('[Content_Types].xml', Buffer.from(
      '<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">' +
      '<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>' +
      '<Default Extension="xml" ContentType="application/xml"/>' +
      '<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>' +
      '<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>' +
      '</Types>', 'utf8')),
    zipEntry('_rels/.rels', Buffer.from(
      '<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">' +
      '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>' +
      '</Relationships>', 'utf8')),
    zipEntry('xl/workbook.xml', Buffer.from(
      '<?xml version="1.0" encoding="UTF-8"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" ' +
      'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">' +
      '<sheets><sheet name="Лист1" sheetId="1" r:id="rId1"/></sheets></workbook>', 'utf8')),
    zipEntry('xl/_rels/workbook.xml.rels', Buffer.from(
      '<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">' +
      '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>' +
      '</Relationships>', 'utf8')),
    zipEntry('xl/worksheets/sheet1.xml', Buffer.from(
      '<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">' +
      `<sheetData>${sheetRows}</sheetData></worksheet>`, 'utf8')),
  ];

  const locals = [];
  const central = [];
  let offset = 0;
  for (const e of entries) {
    const local = Buffer.alloc(30);
    local.writeUInt32LE(0x04034b50, 0);
    local.writeUInt16LE(20, 4);
    local.writeUInt16LE(0, 6);
    local.writeUInt16LE(8, 8);
    local.writeUInt32LE(e.crc, 14);
    local.writeUInt32LE(e.body.length, 18);
    local.writeUInt32LE(e.data.length, 22);
    local.writeUInt16LE(e.name.length, 26);
    local.writeUInt16LE(0, 28);
    const record = Buffer.concat([local, e.name, e.body]);
    locals.push(record);

    const cd = Buffer.alloc(46);
    cd.writeUInt32LE(0x02014b50, 0);
    cd.writeUInt16LE(20, 4);
    cd.writeUInt16LE(20, 6);
    cd.writeUInt16LE(0, 8);
    cd.writeUInt16LE(8, 10);
    cd.writeUInt32LE(e.crc, 16);
    cd.writeUInt32LE(e.body.length, 20);
    cd.writeUInt32LE(e.data.length, 24);
    cd.writeUInt16LE(e.name.length, 28);
    cd.writeUInt16LE(0, 30);
    cd.writeUInt16LE(0, 32);
    cd.writeUInt16LE(0, 34);
    cd.writeUInt16LE(0, 36);
    cd.writeUInt32LE(0, 38);
    cd.writeUInt32LE(offset, 42);
    central.push(Buffer.concat([cd, e.name]));

    offset += record.length;
  }
  const centralBuf = Buffer.concat(central);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(0, 4);
  end.writeUInt16LE(0, 6);
  end.writeUInt16LE(entries.length, 8);
  end.writeUInt16LE(entries.length, 10);
  end.writeUInt32LE(centralBuf.length, 12);
  end.writeUInt32LE(offset, 16);
  end.writeUInt16LE(0, 20);

  return Buffer.concat([...locals, centralBuf, end]);
}

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

console.log('\n10. Токен Telegram-бота (2026-10-29)');
{
  const managerToken = makeJwt('manager@tn.ru');
  const noAuth = await call('GET', '/bot/config');
  ok('настройки бота без JWT → 401', noAuth.status === 401, `получили ${noAuth.status}`);

  const notAdmin = await call('GET', '/bot/config', { token: managerToken });
  ok('настройки бота не админу → 403', notAdmin.status === 403, `получили ${notAdmin.status}`);

  const empty = await call('GET', '/bot/config', { token: ownerToken });
  ok('до настройки токен не задан', empty.status === 200 && empty.json?.configured === false,
    `получили ${empty.status} ${JSON.stringify(empty.json)}`);
  ok('токен наружу не отдаётся', !JSON.stringify(empty.json ?? {}).includes('token'));

  const emptyToken = await call('POST', '/bot/config', { token: ownerToken, body: { token: '   ' } });
  ok('пустой токен → 400', emptyToken.status === 400, `получили ${emptyToken.status}`);

  const wrongShape = await call('POST', '/bot/config', { token: ownerToken, body: { token: 'abc' } });
  ok('токен не по форме → 400', wrongShape.status === 400, `получили ${wrongShape.status}`);

  const rejected = await call('POST', '/bot/config', { token: ownerToken, body: { token: '123456:WRONG' } });
  ok('токен, отвергнутый Telegram → 400', rejected.status === 400, `получили ${rejected.status}`);
  ok('отвергнутый токен не сохранён',
    (await call('GET', '/bot/config', { token: ownerToken })).json?.configured === false);

  const saved = await call('POST', '/bot/config', { token: ownerToken, body: { token: '123456789:GOOD\n' } });
  ok('верный токен сохранён', saved.status === 200 && saved.json?.configured === true,
    `получили ${saved.status} ${JSON.stringify(saved.json)}`);
  ok('имя бота пришло от бота', saved.json?.botUsername === 'offers_test_bot');
  ok('в ответе нет самого токена', !JSON.stringify(saved.json ?? {}).includes('GOOD'));

  const botToken = await call('GET', '/bot/token', { headers: { 'X-Service-Secret': BOT_SECRET } });
  ok('бот читает токен из сервиса', botToken.status === 200 && botToken.json?.token === '123456789:GOOD');

  const botTokenNoSecret = await call('GET', '/bot/token');
  ok('токен без секрета не выдаётся', botTokenNoSecret.status === 401, `получили ${botTokenNoSecret.status}`);

  const hb = await call('POST', '/bot/heartbeat', {
    headers: { 'X-Service-Secret': BOT_SECRET },
    body: { last_error: '' },
  });
  ok('отметка «бот жив» принята', hb.status === 200, `получили ${hb.status}`);

  const status = await call('GET', '/bot/status', { token: ownerToken });
  ok('статус: настроен и работает', status.json?.configured === true && status.json?.running === true,
    JSON.stringify(status.json));

  const off = await call('DELETE', '/bot/config', { token: ownerToken });
  ok('токен удалён', off.status === 200 && off.json?.configured === false);

  const afterDelete = await call('GET', '/bot/status', { token: ownerToken });
  ok('после удаления бот не работает', afterDelete.json?.configured === false);

  const emptyAgain = await call('GET', '/bot/token', { headers: { 'X-Service-Secret': BOT_SECRET } });
  ok('боту возвращается пустой токен', emptyAgain.json?.token === '');
}

console.log('\n11. Файлы в бота: .md, .xlsx, отказы');
let linkedChatId = 0;
{
  const chatId = `attach-${Date.now()}`;
  // Чат привязывается тем же путём, что в боте: администратор выдаёт код,
  // бот проверяет его и связывает chat_id с e-mail.
  const issued = await call('POST', '/access/codes', { token: ownerToken, body: { email: 'owner@tn.ru' } });
  const code = issued.json?.code ?? '';
  const verified = await call('POST', '/bot/verify-code', {
    headers: { 'X-Service-Secret': BOT_SECRET },
    body: { chat_id: chatId, email: 'owner@tn.ru', code },
  });
  if (verified.status === 200) linkedChatId = chatId;
  ok('чат привязан к владельцу для приёма файлов', linkedChatId !== '',
    `код ${issued.status}, проверка ${verified.status} ${verified.text?.slice(0, 120)}`);

  const send = async (fileName, bytes, headers = {}) => {
    const res = await fetch(`${BASE}/api/offers/bot/offers/${offerId}/attach?chat_id=${encodeURIComponent(linkedChatId)}`, {
      method: 'POST',
      headers: { 'X-Service-Secret': BOT_SECRET, 'X-File-Name': encodeURIComponent(fileName), ...headers },
      body: bytes,
    });
    const text = await res.text();
    let json;
    try {
      json = text ? JSON.parse(text) : undefined;
    } catch {
      json = undefined;
    }
    return { status: res.status, json, text };
  };

  const md = [
    'Объект: ЖК «Поэма», корпус 3',
    'Сроки: до 15 сентября',
    'Бюджет: до 12 млн',
    'Заказчик: ООО Строй-Инвест',
    'Боли:',
    '- нужен ввод в эксплуатацию к сентябрю',
  ].join('\n');
  const brief = await send('brief.md', Buffer.from(md, 'utf8'));
  ok('.md разобран → 200', brief.status === 200, `получили ${brief.status} ${brief.text?.slice(0, 160)}`);
  ok('.md просит подтверждение', brief.json?.needsConfirm === true, JSON.stringify(brief.json?.needsConfirm));
  ok('.md: объект распознан',
    (brief.json?.brief?.fields?.object_name ?? []).some((v) => String(v).includes('Поэма')),
    JSON.stringify(brief.json?.brief?.fields?.object_name));
  ok('.md: ответ «что понял» не пустой', (brief.json?.reply ?? '').length > 0);

  const applyBrief = await call('POST', `/bot/offers/${offerId}/attach/apply`, {
    headers: { 'X-Service-Secret': BOT_SECRET },
    body: { chat_id: linkedChatId, mode: 'brief', brief: brief.json?.brief },
  });
  ok('бриф применён → 200', applyBrief.status === 200, `получили ${applyBrief.status} ${applyBrief.text?.slice(0, 160)}`);
  ok('объект записан в бриф', (applyBrief.json?.brief?.object_name ?? '').includes('Поэма'),
    JSON.stringify(applyBrief.json?.brief?.object_name));

  const applyAgain = await call('POST', `/bot/offers/${offerId}/attach/apply`, {
    headers: { 'X-Service-Secret': BOT_SECRET },
    body: { chat_id: linkedChatId, mode: 'brief', brief: brief.json?.brief },
  });
  ok('повторное применение не дублирует поля', applyAgain.status === 200 &&
    (applyAgain.json?.brief?.object_name ?? '') === (applyBrief.json?.brief?.object_name ?? ''));

  const xlsx = makeXlsx([
    ['Наименование', 'Количество', 'Цена', 'Сумма'],
    ['Кровельная система ПВХ', '2', '100', '200'],
    ['Несуществующий профиль', '5', '50', '250'],
  ]);
  const spec = await send('spec.xlsx', xlsx);
  ok('.xlsx разобран → 200', spec.status === 200, `получили ${spec.status} ${spec.text?.slice(0, 160)}`);
  ok('.xlsx просит подтверждение', spec.json?.needsConfirm === true);
  ok('.xlsx: две позиции', (spec.json?.spec?.rows ?? []).length === 2, String((spec.json?.spec?.rows ?? []).length));
  ok('.xlsx: совпадение с каталогом найдено',
    (spec.json?.spec?.rows ?? []).some((r) => r.hasCatalog === true),
    JSON.stringify((spec.json?.spec?.rows ?? []).map((r) => r.hasCatalog)));
  ok('.xlsx: расхождение цены замечено',
    (spec.json?.spec?.rows ?? []).some((r) => r.priceDiff === true),
    JSON.stringify((spec.json?.spec?.rows ?? []).map((r) => r.priceDiff)));

  const applySpec = await call('POST', `/bot/offers/${offerId}/attach/apply`, {
    headers: { 'X-Service-Secret': BOT_SECRET },
    body: { chat_id: linkedChatId, mode: 'spec', spec: spec.json?.spec },
  });
  ok('состав обновлён по таблице → 200', applySpec.status === 200, `получили ${applySpec.status} ${applySpec.text?.slice(0, 160)}`);
  ok('в составе две позиции', (applySpec.json?.offer?.items ?? []).length === 2);
  const catalogRow = (applySpec.json?.offer?.items ?? []).find((i) => i.catalogId);
  ok('цена взята из каталога, а не из файла', catalogRow?.price === 1200, String(catalogRow?.price));

  const docx = await send('письмо.docx', Buffer.concat([Buffer.from([0x50, 0x4b, 0x03, 0x04]), Buffer.from('word/document.xml')]));
  ok('.docx вежливо отклонён → 400', docx.status === 400, `получили ${docx.status}`);
  ok('.docx: сказано, что присылать', (docx.json?.error ?? '').includes('.md') || (docx.json?.error ?? '').includes('xlsx'),
    docx.json?.error);

  const xls = await send('старый.xls', Buffer.from([0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1]));
  ok('.xls отклонён с подсказкой', xls.status === 400 && (xls.json?.error ?? '').includes('xlsx'), xls.json?.error);

  const pdf = await send('счёт.pdf', Buffer.from('%PDF-1.4\n'));
  ok('.pdf отклонён', pdf.status === 400, `получили ${pdf.status}`);

  const junk = await send('данные.bin', Buffer.from([0x00, 0x01, 0x02, 0x03]));
  ok('неизвестный формат отклонён', junk.status === 400, `получили ${junk.status}`);

  const big = Buffer.alloc(11 * 1024 * 1024, 0x41);
  const tooBig = await send('большой.md', big);
  ok('файл больше 10 МБ → 413', tooBig.status === 413, `получили ${tooBig.status}`);

  const foreignChat = await fetch(`${BASE}/api/offers/bot/offers/${offerId}/attach?chat_id=чужой`, {
    method: 'POST',
    headers: { 'X-Service-Secret': BOT_SECRET, 'X-File-Name': encodeURIComponent('brief.md') },
    body: Buffer.from(md, 'utf8'),
  });
  ok('чужой чат не может приложить файл', foreignChat.status === 403 || foreignChat.status === 404,
    `получили ${foreignChat.status}`);

  const noSecret = await fetch(`${BASE}/api/offers/bot/offers/${offerId}/attach?chat_id=${encodeURIComponent(linkedChatId)}`, {
    method: 'POST',
    body: Buffer.from(md, 'utf8'),
  });
  ok('без секрета файл не принимается', noSecret.status === 401, `получили ${noSecret.status}`);

  const wrongMode = await call('POST', `/bot/offers/${offerId}/attach/apply`, {
    headers: { 'X-Service-Secret': BOT_SECRET },
    body: { chat_id: linkedChatId, mode: 'что-то' },
  });
  ok('неизвестный режим применения → 400', wrongMode.status === 400, `получили ${wrongMode.status}`);
}

console.log('\n12. Идемпотентность и повторные запросы');
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

console.log('\n13. Бриф, тексты секций и деградация без модели (2026-10-30)');
{
  // Все проверки раздела требуют привязанного чата: без него сервис откажет
  // на каждой ручке, и раздел превратится в шум. Пропуск виден в итоге.
  if (!linkedChatId) {
    ok('раздел 13 требует привязанного чата (см. раздел 11)', false, 'чат не привязан');
  } else {
    // Чужой чат должен быть ПРИВЯЗАН к другому человеку: непривязанный получает
    // 404 «chat_id не привязан» — это законно, но проверяет не владение, а вход.
    const foreignChatId = `foreign-${Date.now()}`;
    const foreignCode = await call('POST', '/access/codes', { token: ownerToken, body: { email: 'manager@tn.ru' } });
    await call('POST', '/bot/verify-code', {
      headers: { 'X-Service-Secret': BOT_SECRET },
      body: { chat_id: foreignChatId, email: 'manager@tn.ru', code: foreignCode.json?.code ?? '' },
    });
    // Предложение под этот раздел: своё, чтобы не зависеть от черновика выше.
    const created = await call('POST', '/bot/offers', {
      headers: { 'X-Service-Secret': BOT_SECRET },
      body: {
        chat_id: linkedChatId,
        client: { name: 'ООО «Бриф-Тест»', contact: '+7 900 000-00-00' },
        items: [{ name: 'Стилобат Эксперт', unit: 'м2', qty: 10, price: 1500 }],
        formValues: {},
      },
    });
    ok('КП для брифа создано → 200', created.status === 200, `получили ${created.status} ${created.text?.slice(0, 160)}`);
    const briefOfferId = created.json?.id;

    const start = await call('POST', `/bot/offers/${briefOfferId}/brief/start?chat_id=${encodeURIComponent(linkedChatId)}`, {
      headers: { 'X-Service-Secret': BOT_SECRET },
    });
    ok('бриф начат → 200', start.status === 200, `получили ${start.status} ${start.text?.slice(0, 160)}`);
    ok('бриф задаёт первый вопрос', (start.json?.reply ?? '').length > 0, JSON.stringify(start.json?.reply));

    const step = await call('POST', `/bot/offers/${briefOfferId}/brief/step`, {
      headers: { 'X-Service-Secret': BOT_SECRET },
      body: { chat_id: linkedChatId, text: 'Объект — ЖК «Поэма», сроки до конца квартала.' },
    });
    ok('шаг брифа → 200', step.status === 200, `получили ${step.status} ${step.text?.slice(0, 400)}`);
    ok(
      'ответ менеджера попал в бриф',
      JSON.stringify(step.json?.brief ?? {}).includes('Поэма'),
      JSON.stringify(step.json).slice(0, 500),
    );

    const read = await call('GET', `/bot/offers/${briefOfferId}/brief?chat_id=${encodeURIComponent(linkedChatId)}`, {
      headers: { 'X-Service-Secret': BOT_SECRET },
    });
    ok('бриф читается обратно → 200', read.status === 200, `получили ${read.status}`);
    ok(
      'прочитанный бриф совпадает с записанным',
      (read.json?.brief?.payload?.object_name ?? '') === (step.json?.brief?.payload?.object_name ?? ''),
    );

    const written = await call('POST', `/bot/offers/${briefOfferId}/copy/intro`, {
      headers: { 'X-Service-Secret': BOT_SECRET },
      body: { chat_id: linkedChatId, action: 'write', systemIndex: 0 },
    });
    ok('текст секции написан → 200', written.status === 200, `получили ${written.status} ${written.text?.slice(0, 160)}`);
    ok('текст секции не пустой (заглушка модели)', (written.json?.text ?? '').length > 0, JSON.stringify(written.json?.text));

    const accepted = await call('POST', `/bot/offers/${briefOfferId}/copy/intro`, {
      headers: { 'X-Service-Secret': BOT_SECRET },
      body: { chat_id: linkedChatId, action: 'accept', text: written.json?.text ?? '', systemIndex: 0 },
    });
    ok('текст принят → 200', accepted.status === 200, `получили ${accepted.status}`);
    ok('принятый текст отмечен как непустой', accepted.json?.accepted === true, JSON.stringify(accepted.json));

    const docAfterCopy = await call('POST', `/bot/offers/${briefOfferId}/docx?chat_id=${encodeURIComponent(linkedChatId)}`, {
      headers: { 'X-Service-Secret': BOT_SECRET },
    });
    ok('DOCX с принятым текстом собирается', docAfterCopy.status === 200, `получили ${docAfterCopy.status}`);
    ok('DOCX не пустой', (docAfterCopy.json?.docx_base64 ?? '').length > 1000);

    // Деградация: модель отказывает. КП обязано собраться, секция — пустая.
    // Метка кладётся в название позиции: оно попадает в промпт, а заглушка
    // модели на ней отвечает 502 — так проверяется боевой путь ошибки, а не
    // выдуманная ветка теста.
    const failing = await call('POST', '/bot/offers', {
      headers: { 'X-Service-Secret': BOT_SECRET },
      body: {
        chat_id: linkedChatId,
        client: { name: 'ООО «Без модели»', contact: '' },
        items: [{ name: 'STUB_FAIL', unit: 'м2', qty: 1, price: 100 }],
        formValues: {},
      },
    });
    const failingId = failing.json?.id;
    const forced = await call('POST', `/bot/offers/${failingId}/copy/advantages`, {
      headers: { 'X-Service-Secret': BOT_SECRET },
      body: { chat_id: linkedChatId, action: 'write', systemIndex: 0 },
    });
    ok('при отказе модели ручка отвечает 200', forced.status === 200, `получили ${forced.status} ${forced.text?.slice(0, 120)}`);
    ok('при отказе модели текст пустой, а не выдуманный', (forced.json?.text ?? '') === '', JSON.stringify(forced.json?.text));

    const docDegraded = await call('POST', `/bot/offers/${failingId}/docx?chat_id=${encodeURIComponent(linkedChatId)}`, {
      headers: { 'X-Service-Secret': BOT_SECRET },
    });
    ok('без модели КП всё равно собирается', docDegraded.status === 200, `получили ${docDegraded.status}`);

    const foreignBrief = await call('GET', `/bot/offers/${briefOfferId}/brief?chat_id=${encodeURIComponent(foreignChatId)}`, {
      headers: { 'X-Service-Secret': BOT_SECRET },
    });
    ok('чужой чат не читает бриф', foreignBrief.status === 403, `получили ${foreignBrief.status}`);
  }
}

console.log('\n14. Бриф в плагине: чтение и правка (Task 10, 2026-10-30)');
{
  const created = await call('POST', '/offers', {
    token: ownerToken,
    body: {
      client: { name: 'ООО «Бриф-Плагин»', contact: '' },
      items: [{ name: 'Стилобат Эксперт', unit: 'м2', qty: 2, price: 1500 }],
      formValues: {},
    },
  });
  ok('КП для брифа плагина создано → 200', created.status === 200, `получили ${created.status} ${created.text?.slice(0, 160)}`);
  const id = created.json?.id;

  const empty = await call('GET', `/offers/${id}/brief`, { token: ownerToken });
  ok('брифа нет → 200 с пустой заготовкой', empty.status === 200 && empty.json?.brief?.status === 'absent',
    `получили ${empty.status} ${JSON.stringify(empty.json?.brief?.status)}`);
  ok('пустой бриф отдаёт все ключи', empty.json?.brief?.payload?.skipped_topics !== undefined,
    JSON.stringify(Object.keys(empty.json?.brief?.payload ?? {})));

  const payload = {
    object_name: 'ЖК «Поэма», корпус 3',
    customer_pains: ['протечки на стилобате'],
    customer_goals: ['сдать в срок'],
    project_risks: [],
    objections: [{ claim: 'дорого', response: 'сравним по циклу службы' }],
    budget_signal: 'до 12 млн',
    decision_maker: 'главный инженер',
    timeline: 'до 15 сентября',
    why_us: [],
    notes: '',
    skipped_topics: ['why_us'],
  };
  const saved = await call('PUT', `/offers/${id}/brief`, { token: ownerToken, body: { payload } });
  ok('правка брифа → 200', saved.status === 200, `получили ${saved.status} ${saved.text?.slice(0, 160)}`);
  ok('объект записан', (saved.json?.brief?.payload?.object_name ?? '') === payload.object_name,
    JSON.stringify(saved.json?.brief?.payload?.object_name));

  const reread = await call('GET', `/offers/${id}/brief`, { token: ownerToken });
  ok(
    'бриф читается обратно без потери ключей',
    JSON.stringify(reread.json?.brief?.payload) === JSON.stringify({ ...payload }),
    JSON.stringify(reread.json?.brief?.payload),
  );
  ok('правка двигает updatedAt', (reread.json?.brief?.updatedAt ?? '') !== '',
    JSON.stringify(reread.json?.brief?.updatedAt));

  const viewerForbidden = await call('PUT', `/offers/${id}/brief`, {
    token: makeJwt('manager@tn.ru'),
    body: { payload },
  });
  ok('чужой editor не правит бриф → 403', viewerForbidden.status === 403, `получили ${viewerForbidden.status}`);

  const adminRead = await call('GET', `/offers/${id}/brief`, { token: ownerToken });
  ok('admin читает бриф', adminRead.status === 200, `получили ${adminRead.status}`);

  // Переключатели показа в документе (Task 11, 2026-10-30): по умолчанию
  // выключены, включаются ключами form_values.
  const withFlags = await call('PUT', `/offers/${id}`, {
    token: ownerToken,
    body: {
      ...created.json,
      client: { name: 'ООО «Бриф-Плагин»', contact: '' },
      items: [{ name: 'Стилобат Эксперт', unit: 'м2', qty: 2, price: 1500 }],
      formValues: { 'doc.brief_summary': '1', 'brief.object_name': 'ЖК «Поэма», корпус 3' },
    },
  });
  ok('флаги показа сохраняются в КП', withFlags.status === 200, `получили ${withFlags.status}`);

  const docWithBrief = await call('POST', `/offers/${id}/export?format=docx`, { token: ownerToken });
  ok('DOCX со сводкой брифа собирается', docWithBrief.status === 200, `получили ${docWithBrief.status}`);
  ok('сводка брифа попала в файл', (docWithBrief.text ?? '').length > 1000);
}

console.log(`\nИтого: ${passed} прошло, ${failed} провалилось`);
process.exit(failed === 0 ? 0 : 1);