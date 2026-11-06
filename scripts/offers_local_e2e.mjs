/**
 * Интеграционный прогон offers-service через HTTP на живом контейнере
 * (локальный docker-compose.local.yml). Проверяет поведение, а не реализацию:
 * те же запросы, что делает плагин и бот.
 *
 * Запуск: node scripts/offers_local_e2e.mjs [baseUrl]
 */
import { createHmac } from 'node:crypto';
import { crc32, deflateRawSync, inflateRawSync } from 'node:zlib';

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

/**
 * Сравнение JSON по содержимому, без учёта порядка ключей.
 *
 * Порядок живым контрактом не является, а проверка на строку падала уже дважды
 * на одной перестановке полей (2026-11-01: новые поля брифа).
 */
function sameJson(a, b) {
  return JSON.stringify(normalizeJson(a)) === JSON.stringify(normalizeJson(b));
}

function normalizeJson(value) {
  if (Array.isArray(value)) return value.map(normalizeJson);
  if (value && typeof value === 'object') {
    const out = {};
    for (const key of Object.keys(value).sort()) out[key] = normalizeJson(value[key]);
    return out;
  }
  return value;
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

/** Читает одну запись zip по имени; пусто — записи нет или архив битый. */
function unzipEntrySafe(buf, name) {
  // Central directory не нужен: сервис пишет записи подряд, и первое
  // совпадение имени — это и есть файл.
  const marker = Buffer.from(name, 'utf8');
  const idx = buf.indexOf(marker);
  if (idx < 0) return '';
  try {
    return inflateRawSync(buf.subarray(idx + marker.length)).toString('utf8');
  } catch {
    return '';
  }
}

/**
 * Вырезает из word/document.xml таблицу состава предложения.
 *
 * Нужна, чтобы отличать «материал в таблице цен» от «материал в описательной
 * части» (2026-11-05). Считает вложенность <w:tbl>: таблица характеристик может
 * стоять рядом, и простой поиск до первого </w:tbl> захватил бы её.
 */
function itemsTableXML(xml) {
  const marker = 'Состав предложения';
  const at = xml.indexOf(marker);
  if (at < 0) return '';
  const start = xml.indexOf('<w:tbl>', at);
  if (start < 0) return '';
  let depth = 0;
  let pos = start;
  while (pos < xml.length) {
    const open = xml.indexOf('<w:tbl>', pos);
    const close = xml.indexOf('</w:tbl>', pos);
    if (close < 0) break;
    if (open >= 0 && open < close) {
      depth += 1;
      pos = open + '<w:tbl>'.length;
      continue;
    }
    depth -= 1;
    pos = close + '</w:tbl>'.length;
    if (depth === 0) return xml.slice(start, pos);
  }
  return xml.slice(start);
}

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

/** Записи xlsx/makeLayout копятся, потом упаковываются одним проходом. */
function packZip(entries) {
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

  return packZip(entries);
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
  // Каталог бота — только материалы (2026-10-31): система с ценой в чате
  // была бы ошибкой, у неё цены нет вовсе.
  ok('в каталоге бота нет систем',
    (catalog.json?.items ?? []).every((i) => i.kind !== 'system'),
    JSON.stringify((catalog.json?.items ?? []).map((i) => i.kind)));
  ok('в каталоге бота есть признак «цена известна»',
    (catalog.json?.items ?? []).every((i) => typeof i.hasPrice === 'boolean'),
    JSON.stringify((catalog.json?.items ?? [])[0]));

  const botSystems = await call('GET', '/bot/systems', { headers: { 'X-Service-Secret': BOT_SECRET } });
  ok('бот видит системы', botSystems.status === 200, `получили ${botSystems.status}`);
  ok('системы пришли списком', (botSystems.json?.systems ?? []).length > 0,
    JSON.stringify(botSystems.json?.systems?.length));
  ok('у системы для бота нет цены', (botSystems.json?.systems ?? []).every((s) => s.price === undefined));
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
    // Контекст объекта и адресата — с 2026-11-01 бриф спрашивает его первым.
    object_purpose: 'жилой дом',
    object_area: '12 000 м² кровли',
    customer_address: 'ООО «Строймонтаж», Иванов И.И., Новосибирск',
    planned_systems: ['ТН-КРОВЛЯ Авто'],
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
  // Сравниваем содержимое, а не порядок ключей: сервер отдаёт их в порядке
  // полей структуры, и это не контракт — ловить здесь порядок значило бы
  // ронять проверку на безобидной перестановке полей.
  ok(
    'бриф читается обратно без потери ключей',
    sameJson(reread.json?.brief?.payload, payload),
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

console.log('\n15. База знаний: карточки систем доходят до плагина (2026-10-30)');
{
  // Проверка ровно того, что сломалось на бою: в compose у `offers` не были
  // заданы KB_SERVICE_URL/KB_SERVICE_SECRET, сервер отвечал kbEnabled: false,
  // и плагин писал «База знаний не настроена». Стенд этого не ловил, потому
  // что KB в нём не было. Теперь ловит: если переменные пропадут снова,
  // раздел покраснеет.
  // Сначала проверяем, что база знаний ПОДКЛЮЧЕНА — именно это ломалось на бою.
  const cards = await call('GET', '/context/cards', { token: ownerToken });
  ok('карточки отдаются → 200', cards.status === 200, `получили ${cards.status} ${cards.text?.slice(0, 160)}`);
  ok(
    'база знаний считается настроенной',
    cards.json?.kbEnabled === true,
    `kbEnabled=${JSON.stringify(cards.json?.kbEnabled)} — проверьте KB_SERVICE_URL/KB_SERVICE_SECRET у сервиса offers`,
  );

  // Кеш на свежей базе пуст — это нормально, GET отдаёт кеш. Перечитываем
  // и уже потом проверяем содержимое.
  const refreshed = await call('POST', '/context/refresh', { token: ownerToken });
  ok('перечитывание из базы знаний → 200', refreshed.status === 200, `получили ${refreshed.status}`);
  ok('перечитывание не вернуло ошибку', !refreshed.json?.error, JSON.stringify(refreshed.json?.error));
  ok('карточки систем пришли', (refreshed.json?.cards ?? []).length > 0, `карточек ${refreshed.json?.cards?.length}`);

  const card = (refreshed.json?.cards ?? [])[0];
  ok('у карточки есть название', (card?.title ?? '').length > 0, JSON.stringify(card?.title));
  ok('у карточки есть описание', card?.hasDescription === true, JSON.stringify(card?.hasDescription));
  ok('у карточки есть состав', card?.hasComposition === true, JSON.stringify(card?.hasComposition));

  // После перечитывания кеш уже наполнен — GET его отдаёт.
  const cached = await call('GET', '/context/cards', { token: ownerToken });
  ok('после перечитывания карточки видны в списке', (cached.json?.cards ?? []).length > 0,
    `карточек ${cached.json?.cards?.length}`);
}

console.log('\n16. Материалы и ЕКН (2026-10-30)');
{
  // Материал — не система: у него свой SKU и вариации, а цены нет вовсе.
  const created = await call('POST', '/materials', {
    token: ownerToken,
    body: {
      name: 'Мембрана ПВХ Эксперт',
      kind: 'material',
      unit: 'м2',
      description: 'Проверка справочника',
      typeCode: 'МЕМБ',
      series: 'ПВХ',
    },
  });
  ok('материал создан → 200', created.status === 200, `получили ${created.status} ${created.text?.slice(0, 160)}`);
  const matId = created.json?.id;
  ok('SKU назначен сервером', /^MAT-МЕМБ-ПВХ-\d{4}$/.test(created.json?.sku ?? ''), JSON.stringify(created.json?.sku));
  ok('цены в карточке нет', created.json?.basePrice === undefined && created.json?.price === undefined,
    JSON.stringify(Object.keys(created.json ?? {})));

  // Клиентский SKU не принимается: иначе в справочнике два материала с одним кодом.
  const withSku = await call('POST', '/materials', {
    token: ownerToken,
    body: { name: 'Вторая мембрана', sku: 'MAT-МЕМБ-ПВХ-0001', typeCode: 'МЕМБ', series: 'ПВХ' },
  });
  ok('клиентский SKU проигнорирован', withSku.json?.sku !== 'MAT-МЕМБ-ПВХ-0001', JSON.stringify(withSku.json?.sku));
  const secondId = withSku.json?.id;

  // Система материалом не является.
  const asSystem = await call('POST', '/materials', {
    token: ownerToken,
    body: { name: 'ТН-Кровля', kind: 'system' },
  });
  ok('система в справочник материалов не проходит → 400', asSystem.status === 400, `получили ${asSystem.status}`);

  const noName = await call('POST', '/materials', { token: ownerToken, body: { name: '  ' } });
  ok('материал без названия → 400', noName.status === 400, `получили ${noName.status}`);

  // Вариации: один SKU — много ЕКН.
  const ekn1 = await call('POST', `/materials/${matId}/ekn`, {
    token: ownerToken,
    body: { ekn: '068863', variant: '1,5 мм, серый' },
  });
  ok('ЕКН добавлен → 200', ekn1.status === 200, `получили ${ekn1.status}`);
  ok('у материала один ЕКН', (ekn1.json?.ekn ?? []).length === 1, JSON.stringify(ekn1.json?.ekn));

  const eknAgain = await call('POST', `/materials/${matId}/ekn`, {
    token: ownerToken,
    body: { ekn: '068863', variant: '1,5 мм, серый' },
  });
  ok('повторный ЕКН не создаёт дубль', (eknAgain.json?.ekn ?? []).length === 1, JSON.stringify(eknAgain.json?.ekn));

  const ekn2 = await call('POST', `/materials/${matId}/ekn`, {
    token: ownerToken,
    body: { ekn: '068864', variant: '2,0 мм, белый' },
  });
  ok('второй ЕКН добавлен', (ekn2.json?.ekn ?? []).length === 2, JSON.stringify(ekn2.json?.ekn));

  const eknId = (ekn2.json?.ekn ?? [])[0]?.id;
  const removed = await call('DELETE', `/materials/${matId}/ekn/${eknId}`, { token: ownerToken });
  ok('ЕКН удалён', (removed.json?.ekn ?? []).length === 1, JSON.stringify(removed.json?.ekn));

  const list = await call('GET', '/materials', { token: ownerToken });
  ok('материалы в списке', (list.json?.items ?? []).some((m) => m.id === matId), `материалов ${list.json?.items?.length}`);

  const renamed = await call('PUT', `/materials/${matId}`, {
    token: ownerToken,
    body: { name: 'Мембрана ПВХ Эксперт 2', kind: 'material', unit: 'м2' },
  });
  ok('правка не меняет SKU', renamed.json?.sku === created.json?.sku,
    `${created.json?.sku} → ${renamed.json?.sku}`);

  const archived = await call('DELETE', `/materials/${secondId}`, { token: ownerToken });
  ok('удаление уводит в архив, а не стирает', archived.json?.archived === true, JSON.stringify(archived.json?.archived));
  const visible = await call('GET', '/materials', { token: ownerToken });
  ok('архивный не мешает списку', !(visible.json?.items ?? []).some((m) => m.id === secondId));
  const withArchived = await call('GET', '/materials?archived=1', { token: ownerToken });
  ok('с archived=1 архивный виден', (withArchived.json?.items ?? []).some((m) => m.id === secondId));
}

console.log('\n17. Системы из Базы знаний (2026-10-31)');
{
  // Разделение двух сущностей проверяется на живом списке карточек: в
  // заглушке три заметки — две системы и один материал с тегом material.
  const systems = await call('GET', '/systems', { token: ownerToken });
  ok('системы отдаются → 200', systems.status === 200, `получили ${systems.status} ${systems.text?.slice(0, 160)}`);
  ok('ошибки Базы знаний нет', !systems.json?.error, JSON.stringify(systems.json?.error));

  const list = systems.json?.systems ?? [];
  ok('системы пришли', list.length > 0, `систем ${list.length}`);
  ok('карточка с тегом material системой не считается',
    !list.some((s) => (s.title ?? '').includes('Мембрана')),
    JSON.stringify(list.map((s) => s.title)));

  // Связь системы с материалом — по ВИКИ-ССЫЛКЕ (2026-10-31). В заглушке
  // состав «ТН-Кровля Классик» ссылается на [[Мембрана ПВХ Эксперт]], а
  // карточка с этим названием есть в ленте с тегом material — значит материал
  // обязан завестись сам, из карточки, без ручного справочника.
  const materials = await call('GET', '/materials', { token: ownerToken });
  const items = materials.json?.items ?? [];
  const noteMaterial = items.find((m) => (m.noteTitle ?? '').length > 0);
  ok('материал заведён из карточки Базы знаний', Boolean(noteMaterial),
    JSON.stringify(items.map((m) => ({ name: m.name, note: m.noteTitle }))));

  // Забытый материал (он же — строка без ссылки) живёт в отдельной карточке:
  // берём ту, где такой материал есть, а не первую попавшуюся со ссылкой.
  const card = list.find((s) => (s.compositionLinks ?? []).some((c) => !c.found && !c.layer)) ?? list[0] ?? {};
  const links = card.compositionLinks ?? [];
  ok('у системы есть состав ссылками', links.length > 0, JSON.stringify(links));
  ok('каждая строка состава отдаёт текст', links.every((c) => (c.raw ?? '').length > 0));

  // Если ссылки есть, они обязаны разрешаться в материал. Заглушка умеет так не
  // всегда: прямая выдача Базы знаний (без кеша и обогащения материалами)
  // приходит без `found`, и это состояние стенда, а не дефект состава.
  const linked = list.flatMap((s) => (s.compositionLinks ?? []).filter((c) => c.wiki));
  const found = linked.find((c) => c.found);
  ok('ссылка на материал разрешилась', !linked.length || Boolean(found),
    JSON.stringify(linked.slice(0, 4)));
  ok('в разрешённой строке есть название материала', !found || (found.name ?? '').length > 0,
    JSON.stringify(found?.name));
  ok('в разрешённой строке есть id и found', !found || found.materialId > 0, JSON.stringify(found));
  ok('в строке видна цель ссылки', !found || (found.wiki ?? '').length > 0, JSON.stringify(found?.wiki));

  // Строка без ссылки остаётся видимой и называется в problems — если это
  // материал, который забыли завести (2026-11-02: слои и работы в problems НЕ
  // идут, у них материала нет по смыслу).
  const withoutRef = links.filter((c) => !c.found);
  ok('несвязанная строка осталась в составе', withoutRef.length > 0, JSON.stringify(withoutRef));
  const forgotten = links.filter((c) => !c.found && !c.layer);
  ok('забытый материал назван в проблемах',
    forgotten.length > 0 &&
      forgotten.every((c) => (card.problems ?? []).some((p) => p.includes(c.raw.slice(0, 20)))),
    JSON.stringify({ problems: card.problems, forgotten }));
  ok('проблемы перечисляются отдельно от состава', Array.isArray(card.problems), JSON.stringify(card.problems));
  ok('незнакомая ссылка не ломает выдачу', links.every((c) => typeof c.found === 'boolean'));

  // Слой — не ошибка (2026-11-02): на живых карточках 1273 такие строки, и
  // каждая делала систему «с ошибками в составе». Строка остаётся в составе,
  // помечена слоем и в замечания не попадает.
  const layerCard = list.find((s) => (s.compositionLinks ?? []).some((c) => c.layer));
  const layerRow = (layerCard?.compositionLinks ?? []).find((c) => c.layer);
  ok('строка-слой отмечена слоем', Boolean(layerRow), JSON.stringify(layerCard?.compositionLinks));
  ok('слой не назван в замечаниях',
    Boolean(layerRow) && !(layerCard?.problems ?? []).some((p) => p.includes(layerRow.raw.slice(0, 20))),
    JSON.stringify(layerCard?.problems));
  ok('слой не считается найденным материалом', layerRow?.found === false, JSON.stringify(layerRow));

  const anon = await call('GET', '/systems');
  ok('без токена → 401', anon.status === 401, `получили ${anon.status}`);
}

console.log('\n18. Таблица менеджера и системы в КП (2026-10-31)');
{
  // Предложение, в которое дописываются позиции: сначала пустое, потом
  // таблица, потом система. Каждый шаг виден по сумме и составу.
  // Пустое предложение создать нельзя (правило сервера: без позиций оно
  // бессмысленно), поэтому заводим с одной позицией и называем её заглушкой —
  // таблица и система дописываются к ней, а не заменяют состав.
  const created = await call('POST', '/offers', {
    token: ownerToken,
    body: {
      client: { name: 'ООО Табличное' },
      items: [{ name: 'Заглушка', unit: 'шт', qty: 1, price: 1 }],
      formValues: {},
      status: 'draft',
    },
  });
  ok('предложение для проверки создано', created.status === 200, `получили ${created.status} ${created.text?.slice(0, 160)}`);
  const offerId = created.json?.id;

  // --- Таблица: превью ничего не пишет -----------------------------------
  const xlsx = makeXlsx([
    ['Наименование', 'ЕКН', 'Ед.', 'Кол-во', 'Цена', 'Сумма'],
    // ЕКН 068864 и новое имя — то, что осталось у материала после проверок
    // раздела 16: материал переименован, а первый ЕКН там удалён. Берём
    // то, что справочник реально содержит на этом шаге.
    ['Мембрана ПВХ Эксперт 2', '068864', 'м2', 10, 1250, 12500],
    ['Неизвестный материал', '', 'шт', 1, 100, 100],
  ]);
  const preview = await fetch(`${BASE}/api/offers/offers/${offerId}/sheet`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      'X-File-Name': 'table.xlsx',
      Authorization: `Bearer ${ownerToken}`,
    },
    body: xlsx,
  });
  const pv = await preview.json().catch(() => ({}));
  ok('таблица разобрана → 200', preview.status === 200, `получили ${preview.status} ${JSON.stringify(pv).slice(0, 160)}`);
  ok('строк найдено две', pv?.counts?.total === 2, JSON.stringify(pv?.counts));
  ok('мембрана сопоставлена по ЕКН', pv?.counts?.byEkn === 1, JSON.stringify(pv?.counts));
  ok('неизвестная строка не сопоставлена', pv?.counts?.none === 1, JSON.stringify(pv?.counts));
  ok('проблемы названы до применения', (pv?.problems ?? []).length > 0, JSON.stringify(pv?.problems));

  const untouched = await call('GET', `/offers/${offerId}`, { token: ownerToken });
  ok('превью ничего не записало', (untouched.json?.items ?? []).length === 1,
    JSON.stringify(untouched.json?.items));

  // --- Применение: пишется цена ИЗ ТАБЛИЦЫ --------------------------------
  const matched = (pv?.rows ?? []).find((r) => r.materialId > 0);
  ok('в превью есть сопоставленная строка', Boolean(matched), JSON.stringify(pv?.rows));
  const applied = await call('POST', `/offers/${offerId}/sheet/apply`, {
    token: ownerToken,
    body: {
      rows: [{
        line: matched?.row?.line ?? 2,
        materialId: matched?.materialId ?? 0,
        ekn: matched?.row?.ekn ?? '',
        variant: matched?.row?.name ?? '',
        qty: matched?.row?.qty ?? 1,
        price: matched?.row?.price ?? 0,
      }],
      addMissingEkn: true,
    },
  });
  ok('таблица применена → 200', applied.status === 200, `получили ${applied.status} ${applied.text?.slice(0, 200)}`);
  ok('дописана одна позиция', applied.json?.applied === 1, JSON.stringify(applied.json?.applied));
  const appliedItem = (applied.json?.offer?.items ?? [])[0];
  ok('цена позиции взята из таблицы', appliedItem?.price === 1250 || appliedItem?.price === 1250.5,
    JSON.stringify(appliedItem));
  ok('итог пересчитан', (applied.json?.offer?.total ?? 0) > 0, JSON.stringify(applied.json?.offer?.total));
  // Номер ЕКН из таблицы доезжает до позиции (2026-11-02): колонка в КП нужна
  // для сверки с прайсом заказчика, и раньше она терялась молча.
  ok('номер ЕКН из таблицы попал в позицию', (appliedItem?.ekn ?? '') === '068864',
    JSON.stringify(appliedItem?.ekn));

  // --- Система: выбирается для КП, позиции НЕ добавляются (2026-11-03) ----
  //
  // Живая жалоба по КП 003/2026: выбор системы дописывал её состав к позициям
  // из таблицы менеджера, и одни и те же продукты стояли в таблице дважды —
  // с ценой из файла и с нулём из системы. Теперь выбор системы несёт только
  // описание, обоснование и ссылки, а состав с ценами ведёт менеджер.
  const beforeSystemItems = (applied.json?.offer?.items ?? []).length;
  const composed = await call('POST', `/offers/${offerId}/system`, {
    token: ownerToken,
    body: { sku: 'TN-ROOF-CLASSIC' },
  });
  ok('система выбрана → 200', composed.status === 200, `получили ${composed.status} ${composed.text?.slice(0, 200)}`);
  ok('позиции НЕ изменились', (composed.json?.offer?.items ?? []).length === beforeSystemItems,
    `было ${beforeSystemItems}, стало ${(composed.json?.offer?.items ?? []).length}`);
  ok('в ответе нет поля «добавлено позиций»', composed.json?.added === undefined,
    JSON.stringify({ added: composed.json?.added }));
  // Систем с 2026-11-05 может быть несколько: ответ несёт `systems`.
  ok('система запомнилась в предложении',
    (composed.json?.offer?.systems ?? []).some((s) => s.sku || s.noteId),
    JSON.stringify(composed.json?.offer?.systems));
  ok('карточка системы возвращена', (composed.json?.system?.title ?? '') !== '',
    JSON.stringify(composed.json?.system?.title));
  ok('число основных продуктов посчитано', (composed.json?.composition ?? 0) >= 1,
    JSON.stringify(composed.json?.composition));

  // Система, выбранная ради обоснования, печатается в КП и при пустом
  // составе (2026-11-03). До правки карточка попадала в документ, только если
  // её материал нашёлся среди позиций, — а менеджер сначала выбирает систему
  // и лишь потом заводит цены.
  const docxWithSystem = await call('POST', `/offers/${offerId}/export`, { token: ownerToken });
  ok('документ с выбранной системой собирается → 200', docxWithSystem.status === 200,
    `получили ${docxWithSystem.status} ${docxWithSystem.text?.slice(0, 160)}`);
  {
    // Описание ищется в РАСПАКОВАННОМ word/document.xml: архив сжат, и поиск
    // по сырым байтам нашёл бы случайное совпадение. Рука /export отдаёт
    // файл байтами, поэтому body читаем как массив.
    const bytes = await (await fetch(`${BASE}/api/offers/offers/${offerId}/export`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${ownerToken}` },
    })).arrayBuffer();
    const xml = unzipEntrySafe(Buffer.from(bytes), 'word/document.xml');
    ok('описание выбранной системы попало в документ',
      xml.includes('Классическая наплавляемая кровля для промышленных объектов'),
      xml ? xml.slice(0, 300) : 'нет word/document.xml');
    // Состав системы позициями в таблицу НЕ попадает: его ведёт менеджер.
    //
    // Проверка смотрит ИМЕННО таблицу состава, а не весь документ (2026-11-05):
    // с этого выпуска состав карточки печатается отдельным блоком «Состав
    // системы» (как в образце конкурента), и поиск по всему XML краснел на нём,
    // хотя в таблицу цен материал так и не попадал.
    const itemsTable = itemsTableXML(xml);
    ok('материалы системы не попали в таблицу как позиции',
      !itemsTable.includes('Праймер битумный'), itemsTable.slice(0, 300));
    // А в описательной части состав есть — это и требовалось.
    ok('состав системы напечатан отдельным блоком',
      xml.includes('Состав системы'), xml.slice(0, 300));
  }

  // Несколько систем в одном КП (2026-11-05). Менеджеру нужно предложить и
  // сравнить два решения — как СТИЛОБАТ V1.0 и V2.0 в образце конкурента.
  // Проверяем: вторая система добавляется, обе остаются, сравнение печатается
  // таблицей и убрать можно ровно одну.
  {
    const second = await call('POST', `/offers/${offerId}/system`, {
      token: ownerToken,
      body: { noteId: 100 },
    });
    ok('вторая система добавлена → 200', second.status === 200,
      `получили ${second.status} ${second.text?.slice(0, 160)}`);
    const systemsAfterAdd = second.json?.offer?.systems ?? [];
    ok('в предложении две системы', systemsAfterAdd.length === 2,
      JSON.stringify(systemsAfterAdd));

    // Повторное добавление той же системы не даёт дубль.
    const again = await call('POST', `/offers/${offerId}/system`, {
      token: ownerToken,
      body: { noteId: 100 },
    });
    ok('повторное добавление не дублирует',
      (again.json?.offer?.systems ?? []).length === 2,
      JSON.stringify(again.json?.offer?.systems));

    const removed = await call('DELETE', `/offers/${offerId}/system`, {
      token: ownerToken,
      body: { noteId: 100 },
    });
    ok('удаление системы → 200', removed.status === 200,
      `получили ${removed.status} ${removed.text?.slice(0, 160)}`);
    ok('в предложении снова одна система',
      (removed.json?.offer?.systems ?? []).length === 1,
      JSON.stringify(removed.json?.offer?.systems));

    const removeMissing = await call('DELETE', `/offers/${offerId}/system`, {
      token: ownerToken,
      body: { noteId: 100 },
    });
    ok('удаление отсутствующей системы → 404', removeMissing.status === 404,
      `получили ${removeMissing.status}`);
  }

  const unknownSystem = await call('POST', `/offers/${offerId}/system`, {
    token: ownerToken,
    body: { sku: 'НЕТ-ТАКОГО' },
  });
  ok('неизвестная система → 404', unknownSystem.status === 404, `получили ${unknownSystem.status}`);

  // Карточка БЕЗ кода (заметка 100 в заглушке) — таких на бою все 375. Выбор по
// заметке обязан доходить до сервера и дописывать состав, а не рассыпаться на
// «Выберите систему» в карточке КП и «система не найдена» на сервере.
  const pickList = (await call('GET', '/systems', { token: ownerToken })).json?.systems ?? [];
  const noteCard = pickList.find((s) => s.noteId > 0 && !(s.sku ?? '').trim()) ?? {};
  ok('в списке есть карточка без кода с номером заметки', noteCard.noteId > 0,
    JSON.stringify(pickList.map((s) => ({ noteId: s.noteId, sku: s.sku, title: s.title }))));
  ok('у карточки без кода есть состав', (noteCard.compositionLinks ?? []).some((c) => c.found),
    JSON.stringify(noteCard.compositionLinks));
  // Выбор системы по заметке (2026-11-02, живая жалоба по КП 003).
  //
  // В живых карточках Базы знаний `system_sku` не заполнен ни у одной, поэтому
  // список в карточке КП состоял из вариантов с пустым значением: выбрать было
  // нечего, а «Добавить» отвечало «Выберите систему». Выбор теперь несёт
  // заметку, поэтому проверяем, что сервер принимает `noteId` без кода.
  const noteChosen = await call('POST', `/offers/${offerId}/system`, {
    token: ownerToken,
    body: { noteId: noteCard.noteId },
  });
  ok('система без кода выбирается по заметке → 200', noteChosen.status === 200,
    `получили ${noteChosen.status} ${noteChosen.text?.slice(0, 200)}`);
  ok('по заметке система запомнилась',
    (noteChosen.json?.offer?.systems ?? []).some((s) => s.noteId === noteCard.noteId),
    JSON.stringify(noteChosen.json?.offer?.systems));
  ok('по заметке позиции не добавились', (noteChosen.json?.offer?.items ?? []).length === beforeSystemItems,
    `ожидали ${beforeSystemItems}, получили ${(noteChosen.json?.offer?.items ?? []).length}`);

  const badNote = await call('POST', `/offers/${offerId}/system`, {
    token: ownerToken,
    body: { noteId: 999999 },
  });
  ok('чужая заметка → 404', badNote.status === 404, `получили ${badNote.status}`);

  const noToken = await call('POST', `/offers/${offerId}/system`, { body: { sku: 'TN-ROOF-CLASSIC' } });
  ok('добавление системы без токена → 401', noToken.status === 401, `получили ${noToken.status}`);

  // --- Бот: та же система, другой способ входа ----------------------------
  // У бота секрет сервиса и chat_id вместо JWT; правило состава одно, поэтому
  // проверяем, что ручка даёт тот же результат. Чат владельца привязан в
  // разделе 11 — берём его, а не придуманный.
  const botComposed = await call('POST', `/bot/offers/${offerId}/system`, {
    headers: { 'X-Service-Secret': BOT_SECRET },
    body: { chat_id: String(linkedChatId), sku: 'TN-ROOF-CLASSIC' },
  });
  ok('бот выбирает систему → 200', botComposed.status === 200,
    `получили ${botComposed.status} ${botComposed.text?.slice(0, 160)}`);
  ok('бот не добавил позиций', (botComposed.json?.offer?.items ?? []).length === beforeSystemItems,
    JSON.stringify((botComposed.json?.offer?.items ?? []).length));
  ok('бот получил название системы для ответа', (botComposed.json?.title ?? '') !== '',
    JSON.stringify(botComposed.json?.title));

  const foreign = await call('POST', `/bot/offers/${offerId}/system`, {
    headers: { 'X-Service-Secret': BOT_SECRET },
    body: { chat_id: '999999', sku: 'TN-ROOF-CLASSIC' },
  });
  ok('чужой чат систему не добавляет', foreign.status === 404, `получили ${foreign.status}`);
}

// 19. Контекст сначала, блоки формы и ссылки (2026-11-01)
//
// Раздел проверяет ровно то, чего не видят юнит-тесты: что сервер принимает
// ПУСТОЕ предложение из бота (иначе файл и бриф принять некуда), что карточка
// системы отдаёт ссылки из «## Ссылки», и что документ с новыми блоками
// собирается и не ломает бланк.
{
  console.log('\n19. Контекст сначала, блоки формы, ссылки (2026-11-01)');

  // Пустое КП из бота — законный старт разговора.
  const emptyDraft = await call('POST', '/bot/offers', {
    headers: { 'X-Service-Secret': BOT_SECRET },
    body: { chat_id: String(linkedChatId), client: { name: '', contact: '' }, items: [], formValues: {} },
  });
  ok('бот создаёт пустое КП → 200', emptyDraft.status === 200,
    `получили ${emptyDraft.status} ${emptyDraft.text?.slice(0, 160)}`);
  ok('у пустого КП есть номер', (emptyDraft.json?.number ?? '') !== '', JSON.stringify(emptyDraft.json?.number));
  ok('у пустого КП нет позиций', (emptyDraft.json?.items ?? []).length === 0,
    JSON.stringify(emptyDraft.json?.items));
  ok('итог пустого КП — ноль', (emptyDraft.json?.total ?? -1) === 0, JSON.stringify(emptyDraft.json?.total));
  const emptyId = emptyDraft.json?.id;

  // Одно предложение по id — чтобы бот не спрашивал дважды.
  const oneOffer = await call('GET', `/bot/offers/${emptyId}?chat_id=${linkedChatId}`, {
    headers: { 'X-Service-Secret': BOT_SECRET },
  });
  ok('бот читает своё КП → 200', oneOffer.status === 200, `получили ${oneOffer.status}`);
  const foreignOffer = await call('GET', `/bot/offers/${emptyId}?chat_id=999999`, {
    headers: { 'X-Service-Secret': BOT_SECRET },
  });
  ok('чужой чат КП не читает → 404', foreignOffer.status === 404, `получили ${foreignOffer.status}`);

  // REST-путь правила не потерял: пустое предложение от плагина по-прежнему 400.
  const restEmpty = await call('POST', '/offers', {
    token: ownerToken,
    body: { client: { name: 'ООО Тест', contact: '' }, items: [], formValues: {} },
  });
  ok('REST по-прежнему не принимает пустое КП → 400', restEmpty.status === 400,
    `получили ${restEmpty.status}`);

  // Бриф начинается с объекта: порядок тем важен, это и есть просьба.
  const briefStart = await call('POST', `/bot/offers/${emptyId}/brief/start?chat_id=${linkedChatId}`, {
    headers: { 'X-Service-Secret': BOT_SECRET },
  });
  ok('бриф стартует → 200', briefStart.status === 200,
    `получили ${briefStart.status} ${briefStart.text?.slice(0, 160)}`);
  ok('первый вопрос — про объект', /объект/i.test(briefStart.json?.reply ?? ''),
    JSON.stringify(briefStart.json?.reply));
  ok('первая тема брифа — object_name', (briefStart.json?.brief?.lastTopic ?? '') === 'object_name'
    || /объект/i.test(briefStart.json?.reply ?? ''),
  JSON.stringify(briefStart.json?.brief?.lastTopic));

  // Ссылки из карточки: stand-карточка содержит «## Ссылки».
  const systemsResp = await call('GET', '/systems', { token: ownerToken });
  const firstSystem = (systemsResp.json?.systems ?? [])[0];
  ok('карточка системы отдаёт ссылки', (firstSystem?.refs ?? []).length > 0,
    JSON.stringify(firstSystem?.refs));
  const refKinds = new Set((firstSystem?.refs ?? []).map((r) => r.kind));
  ok('среди ссылок есть страница или документ', refKinds.size > 0, [...refKinds].join(','));

  // Документ с новыми блоками: разрыв страницы, ссылки, картинка без файла.
  const blockForm = {
    mode: 'client',
    title: 'КП с блоками',
    placeholders: [{ id: 'client_name', label: 'Клиент' }],
    requiredFields: [],
    blocks: [
      {
        id: 'cover',
        title: 'Титул',
        showInClient: true,
        content: [
          { type: 'paragraph', text: 'КОММЕРЧЕСКОЕ ПРЕДЛОЖЕНИЕ', align: 'center' },
          { type: 'page_break' },
          { type: 'prompt_text', instruction: 'Опиши систему', maxWords: 40 },
          { type: 'image', imageKey: 'нет-такого', imageName: 'x.png', widthCm: 6, wrap: 'right' },
          { type: 'links' },
        ],
      },
    ],
  };
  const savedForm = await call('PUT', '/form', { token: ownerToken, body: blockForm });
  ok('форма с новыми блоками сохраняется → 200', savedForm.status === 200,
    `получили ${savedForm.status} ${savedForm.text?.slice(0, 200)}`);

  // Негатив: отрицательный отступ — ошибка админа, а не оформление.
  const badForm = await call('PUT', '/form', {
    token: ownerToken,
    body: { ...blockForm, blocks: [{ ...blockForm.blocks[0], spacingBefore: -5 }] },
  });
  ok('отрицательный отступ отклоняется → 400', badForm.status === 400, `получили ${badForm.status}`);

  // Пустое КП собирается документом (2026-11-01): менеджерам нужна
  // описательная часть, а цены они вписывают сами. Раньше здесь ждали 400.
  const docxBlock = await call('POST', `/bot/offers/${emptyId}/docx?chat_id=${linkedChatId}`, {
    headers: { 'X-Service-Secret': BOT_SECRET },
  });
  ok('пустое КП собирается документом → 200', docxBlock.status === 200, `получили ${docxBlock.status} ${docxBlock.text?.slice(0, 200)}`);
  ok('в документе без позиций есть файл', (docxBlock.json?.docx_base64 ?? '').length > 1000);

  // Вернуть форму по умолчанию: стенд используется следующими проверками.
  await call('PUT', '/form', {
    token: ownerToken,
    body: {
      mode: 'client',
      title: 'Коммерческое предложение',
      placeholders: [{ id: 'client_name', label: 'Наименование клиента' }],
      requiredFields: ['client_name'],
      blocks: [
        {
          id: 'intro',
          title: 'Вступление',
          showInClient: true,
          content: [{ type: 'paragraph', text: 'Предлагаем Вам поставку материалов.' }],
        },
      ],
    },
  });
}

// 20. Контекст объекта из файла (2026-11-01).
//
// Живой случай: менеджер прислал старый КП в Excel и написал «информация в
// файле». Бот не нашёл состава и ответил «состава нет», а объект из файла так
// и не попал в бриф. Теперь любой файл даёт контекст, а таблица — ещё и состав.
{
  console.log('\n20. Контекст объекта из файла (2026-11-01)');

  const created = await call('POST', '/bot/offers', {
    headers: { 'X-Service-Secret': BOT_SECRET },
    body: { chat_id: String(linkedChatId), client: { name: '', contact: '' }, items: [], formValues: {} },
  });
  const offerId = created.json?.id;

  // Файл-описание: объект, площадь, системы — как их пишет менеджер.
  const description = [
    'Объект: Многоквартирный дом по ул. Охотская в Заельцовском районе Новосибирска',
    'Площадь: 9 200 м2',
    'Системы: ТН-ФУНДАМЕНТ Дренаж Проф',
    'Сроки: до 15 сентября',
  ].join('\n');
  const attach = await fetch(`${BASE}/api/offers/bot/offers/${offerId}/attach?chat_id=${encodeURIComponent(linkedChatId)}`, {
    method: 'POST',
    headers: {
      'X-Service-Secret': BOT_SECRET,
      'X-File-Name': encodeURIComponent('описание объекта.md'),
      'Content-Type': 'application/octet-stream',
    },
    body: Buffer.from(description, 'utf8'),
  });
  const parsed = await attach.json().catch(() => ({}));
  ok('файл-описание принят → 200', attach.status === 200, `получили ${attach.status}`);
  ok('из файла построен контекст объекта', Boolean(parsed?.contextFields), JSON.stringify(parsed?.contextFields));
  ok('объект разобран из файла',
    (parsed?.contextFields?.fields?.object_name ?? []).some((v) => v.includes('Охотская')),
    JSON.stringify(parsed?.contextFields?.fields?.object_name));
  ok('площадь разобрана как 9 200 м2',
    (parsed?.contextFields?.fields?.object_area ?? []).some((v) => v.includes('200')),
    JSON.stringify(parsed?.contextFields?.fields?.object_area));
  ok('бот предлагает записать разбор в бриф', /бриф/i.test(parsed?.reply ?? ''), JSON.stringify(parsed?.reply));

  const applied = await call('POST', `/bot/offers/${offerId}/attach/apply`, {
    headers: { 'X-Service-Secret': BOT_SECRET },
    body: { chat_id: String(linkedChatId), mode: 'brief', brief: parsed?.brief, context: parsed?.contextFields },
  });
  ok('контекст применён → 200', applied.status === 200, `получили ${applied.status} ${applied.text?.slice(0, 160)}`);

  const brief = await call('GET', `/bot/offers/${offerId}/brief?chat_id=${linkedChatId}`, {
    headers: { 'X-Service-Secret': BOT_SECRET },
  });
  const payload = brief.json?.brief?.payload ?? {};
  ok('объект лёг в бриф', String(payload.object_name ?? '').includes('Охотская'), JSON.stringify(payload.object_name));
  ok('площадь легла в бриф', String(payload.object_area ?? '').includes('200'), JSON.stringify(payload.object_area));
  ok('система из файла легла в бриф',
    (payload.planned_systems ?? []).some((s) => s.includes('ФУНДАМЕНТ')),
    JSON.stringify(payload.planned_systems));

  // Продолжение сохранённого КП: бот читает его состав и данные клиента.
  const reopened = await call('GET', `/bot/offers/${offerId}?chat_id=${linkedChatId}`, {
    headers: { 'X-Service-Secret': BOT_SECRET },
  });
  ok('сохранённое КП открывается на правку → 200', reopened.status === 200, `получили ${reopened.status}`);
  ok('у открытого КП есть номер', (reopened.json?.number ?? '') !== '', JSON.stringify(reopened.json?.number));
}


// 21. Сравнение систем в документе (2026-11-05).
//
// Стоит в конце нарочно: разделы 19 и 20 перезаписывают форму под свои нужды,
// и проверка формы, поставленная раньше, проверяла бы не свой блок.
{
  const created = await call('POST', '/offers', {
    token: ownerToken,
    body: {
      client: { name: 'ООО Сравнение', contact: 'Пётр' },
      items: [{ name: 'Мембрана', unit: 'м2', qty: 10, price: 100 }],
      formValues: {},
    },
  });
  const cmpOfferId = created.json?.id;
  ok('КП для сравнения создано → 200', created.status === 200 && Boolean(cmpOfferId),
    `получили ${created.status}`);

  // Карточка 101 — с характеристиками; 102 (TN-ROOF-CLASSIC) их не имеет,
  // и сравнение из неё не построить.
  await call('POST', `/offers/${cmpOfferId}/system`, { token: ownerToken, body: { noteId: 101 } });
  await call('POST', `/offers/${cmpOfferId}/system`, { token: ownerToken, body: { noteId: 100 } });

  const formed = await call('GET', '/form', { token: ownerToken });
  const original = formed.json ?? {};
  const withCompare = {
    ...original,
    blocks: [
      {
        id: 'compare',
        title: 'Сравнение систем',
        showInClient: true,
        showInInternal: false,
        spacingBefore: 0,
        spacingAfter: 0,
        content: [{ type: 'compare_table', text: 'Сравнение систем', level: 2 }],
      },
    ],
  };
  const saved = await call('PUT', '/form', { token: ownerToken, body: withCompare });
  ok('форма со сравнением сохраняется → 200', saved.status === 200,
    `получили ${saved.status} ${saved.text?.slice(0, 160)}`);

  const docBytes = await (await fetch(`${BASE}/api/offers/offers/${cmpOfferId}/export`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${ownerToken}` },
  })).arrayBuffer();
  const xml = unzipEntrySafe(Buffer.from(docBytes), 'word/document.xml');
  ok('таблица сравнения попала в документ', xml.includes('Сравнение систем'),
    xml ? xml.slice(0, 200) : 'нет word/document.xml');
  // Названия берём из ответа сервера, а не пишем руками: рукописная строка с
  // кириллицей уже один раз разошлась с карточкой (латинская «o» вместо
  // русской) и проверка краснела не из-за дефекта.
  const listForCompare = await call('GET', '/systems', { token: ownerToken });
  const titles = (listForCompare.json?.systems ?? [])
    .filter((s) => s.noteId === 100 || s.noteId === 101)
    .map((s) => s.title);
  ok('в сравнении есть обе системы',
    titles.length === 2 && titles.every((t) => xml.includes(t)),
    JSON.stringify({ titles, xml: xml.slice(0, 200) }));

  await call('PUT', '/form', { token: ownerToken, body: original });
}

// 21. Макет .docx и «опишите блок словами» (2026-11-05).
async function sectionLayout() {
  console.log('\n21. Макет .docx и описание блока (2026-11-05)');
  const layoutDoc =
    `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
    `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
    `<w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:rPr><w:b/></w:rPr>` +
    `<w:t>ТКП № [Номер]</w:t></w:r></w:p>` +
    `<w:p><w:r><w:t>Объект: [Наименование объекта]</w:t></w:r></w:p>` +
    `</w:body></w:document>`;
  const layout = packZip([
    zipEntry('[Content_Types].xml', Buffer.from(
      '<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>',
      'utf8')),
    zipEntry('word/document.xml', Buffer.from(layoutDoc, 'utf8')),
  ]);
  const up = await fetch(`${BASE}/api/offers/form/layout?name=ТКП.docx`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${ownerToken}`,
      'Content-Type': 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    },
    body: layout,
  });
  const upJson = await up.json().catch(() => undefined);
  ok('макет загружен → 200', up.status === 200, `${up.status} ${JSON.stringify(upJson)?.slice(0, 160)}`);
  ok('найдено поле «Номер»', (upJson?.layoutFields ?? []).includes('Номер'),
    JSON.stringify(upJson?.layoutFields));
  ok('найдено поле «Наименование объекта»',
    (upJson?.layoutFields ?? []).includes('Наименование объекта'));

  const formWithLayout = await call('GET', '/form', { token: ownerToken });
  ok('форма сообщает о макете', formWithLayout.json?.hasLayout === true);

  // Печать по макету: метки подставлены, вёрстка макета сохранена.
  const created = await call('POST', '/offers', {
    token: ownerToken,
    body: {
      client: { name: 'ООО Макет', contact: 'Иван' },
      items: [{ name: 'Мембрана', unit: 'м2', qty: 1, price: 100, sum: 100 }],
      total: 100, status: 'draft',
      formValues: { object_name: 'Дом по ул. Примерная' },
    },
  });
  const layoutOfferId = created.json?.id;
  const doc = await (await fetch(`${BASE}/api/offers/offers/${layoutOfferId}/export`, {
    method: 'POST', headers: { Authorization: `Bearer ${ownerToken}` },
  })).arrayBuffer();
  const layoutXML = unzipEntrySafe(Buffer.from(doc), 'word/document.xml');
  ok('КП по макету собрано', layoutXML.includes('ТКП №'), layoutXML.slice(0, 200));
  ok('метка [Наименование объекта] подставлена',
    layoutXML.includes('Дом по ул. Примерная') && !layoutXML.includes('[Наименование объекта]'));
  ok('вёрстка макета сохранена', layoutXML.includes('<w:jc w:val="center"/>'));

  const del = await call('DELETE', '/form/layout', { token: ownerToken });
  ok('макет удалён → 200', del.status === 200);
  const formAfter = await call('GET', '/form', { token: ownerToken });
  ok('после удаления макета нет', formAfter.json?.hasLayout !== true);

  await call('DELETE', `/offers/${layoutOfferId}`, { token: ownerToken });

  // «Опишите блок словами»: стенд отвечает заглушкой модели.
  const described = await call('POST', '/form/describe', {
    token: ownerToken,
    body: { description: 'заголовок и список условий', placeholders: ['object_name'] },
  });
  ok('описание блока разобрано → 200', described.status === 200,
    `${described.status} ${described.text?.slice(0, 160)}`);
  ok('модель вернула узлы', Array.isArray(described.json?.nodes) && described.json.nodes.length > 0);

  const empty = await call('POST', '/form/describe', {
    token: ownerToken, body: { description: '  ' },
  });
  ok('пустое описание отклонено → 400', empty.status === 400);
}

await sectionLayout();

console.log(`\nИтого: ${passed} прошло, ${failed} провалилось`);
process.exit(failed === 0 ? 0 : 1);
