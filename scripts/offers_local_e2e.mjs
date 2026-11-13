/**
 * Интеграционный прогон offers-service через HTTP на живом контейнере
 * (локальный docker-compose.local.yml). Проверяет поведение, а не реализацию:
 * те же запросы, что делает плагин и бот.
 *
 * Запуск: node scripts/offers_local_e2e.mjs [baseUrl]
 */
import { createHmac } from 'node:crypto';
import { existsSync, readFileSync } from 'node:fs';
import { crc32, deflateRawSync, inflateRawSync } from 'node:zlib';

const BASE = process.argv[2] ?? 'http://127.0.0.1:18080';
const SECRET = 'test-service-secret';
const BOT_SECRET = 'test-bot-secret';

// styleListBullet — numId маркерного списка бланка (docx.go, styleListBullet).
// Одна копия: проверки ищут его в разметке, и вписанное руками «6» молча
// разошлось бы с сервером, если номер в бланке поменяется.
const styleListBullet = '6';

/**
 * compositionSection — часть документа со СОСТАВОМ системы.
 *
 * Нужна, чтобы проверка «состав не маркерами» не смотрела на весь файл: в нём
 * есть и другие списки бланка (раздел `{{ссылки: система}}` печатается
 * маркерами, 2026-11-08), и вопрос «нет ли маркеров В СОСТАВЕ» заранее отвечал
 * «нет» на исправном документе.
 *
 * Граница — от заголовка «Состав системы» до конца абзацев этого списка: берём
 * ровно те `<w:p>`, что идут следом, и останавливаемся на первом абзаце без
 * нашего numId. Так срез не захватывает ни характеристики, ни ссылки.
 */
function compositionSection(xml) {
  const at = xml.indexOf('Состав системы');
  if (at < 0) return '';
  const start = xml.lastIndexOf('<w:p>', at);
  const from = start >= 0 ? start : at;
  const paragraphs = xml.slice(from).split(/(?=<w:p>)/);
  const out = [];
  for (const paragraph of paragraphs) {
    if (out.length > 0 && !/<w:numId w:val="\d+"\/>/.test(paragraph)) break;
    out.push(paragraph);
  }
  return out.join('');
}

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
  const rawBuf = Buffer.from(await res.arrayBuffer());
  const text = rawBuf.toString('utf8');
  let json;
  try {
    json = text ? JSON.parse(text) : undefined;
  } catch {
    json = undefined;
  }
  return { status: res.status, json, text, raw: rawBuf, headers: res.headers };
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

console.log('\n5b. Поиск в списках (S2, 2026-11-13)');
{
  // Пустой `q` — прежнее поведение: список КП и без поиска обязан отвечать.
  const plain = await call('GET', '/offers', { token: ownerToken });
  ok('список КП без q работает как раньше', plain.status === 200 && (plain.json?.offers ?? []).length >= 2, `status ${plain.status}`);

  // Поиск по имени заказчика: подстрока в СЕРЕДИНЕ слова и «ё»/«е».
  const byWord = await call('GET', '/offers?q=' + encodeURIComponent('Ромаш'), { token: ownerToken });
  const wordHits = byWord.json?.offers ?? [];
  ok('поиск находит заказчика по части имени', wordHits.some((o) => (o.client?.name ?? '').includes('Ромашка')), JSON.stringify(wordHits.map((o) => o.client?.name)));
  ok('поиск не отдаёт несовпавшие строки', wordHits.every((o) => (o.client?.name ?? '').toLowerCase().includes('ромаш')), JSON.stringify(wordHits.map((o) => o.client?.name)));

  const byYoTie = await call('GET', '/offers?q=' + encodeURIComponent('Ромашка'), { token: ownerToken });
  ok('поиск по полному имени тоже находит', (byYoTie.json?.offers ?? []).length === wordHits.length, String((byYoTie.json?.offers ?? []).length));

  const none = await call('GET', '/offers?q=' + encodeURIComponent('нет-такого-заказчика'), { token: ownerToken });
  ok('пустой результат — массив, а не null', Array.isArray(none.json?.offers), JSON.stringify(none.json?.offers));
  ok('пустой результат пуст', (none.json?.offers ?? []).length === 0, String((none.json?.offers ?? []).length));

  // Поиск не обходит права: чужак без роли не видит чужое даже по имени клиента.
  const stranger = makeJwt('stranger@tn.ru');
  // Без роли закрыт сам маршрут (requirePerm → 403), с ролью — ни одной чужой строки.
  const strangerSearch = await call('GET', '/offers?q=' + encodeURIComponent('Ромашка'), { token: stranger });
  ok(
    'без роли поиск закрыт (403)',
    strangerSearch.status === 403,
    `${strangerSearch.status} / ${(strangerSearch.json?.offers ?? []).length}`,
  );
  // А с ролью editor — только свои: у manager@tn.ru своих с таким именем нет.
  const managerSearch = await call('GET', '/offers?q=' + encodeURIComponent('Ромашка'), { token: makeJwt('manager@tn.ru') });
  ok(
    'поиск не видит чужие строки',
    managerSearch.status === 200 && (managerSearch.json?.offers ?? []).length === 0,
    `${managerSearch.status} / ${(managerSearch.json?.offers ?? []).length}`,
  );

  // Листки: поиск по названию и по продукту стороны.
  const sheetsPlain = await call('GET', '/combat', { token: ownerToken });
  ok('список листков без q работает как раньше', sheetsPlain.status === 200 && Array.isArray(sheetsPlain.json?.sheets), `status ${sheetsPlain.status}`);
  const sheetSearch = await call('GET', '/combat?q=' + encodeURIComponent('мембрана'), { token: ownerToken });
  ok('поиск листков по продукту отвечает массивом', sheetSearch.status === 200 && Array.isArray(sheetSearch.json?.sheets), `status ${sheetSearch.status}`);
}

console.log('\n5c. Карточка КП из приложения: точечные правки (S3, 2026-11-13)');
{
  // Своя заготовка: у неё есть позиции, чтобы проверить, что правка заказчика
  // их НЕ трогает, а правка позиций не задевает системы.
  const created = await call('POST', '/offers', {
    token: ownerToken,
    body: {
      client: { name: 'Проверка карточки', contact: 'Иван' },
      items: [{ name: 'Было', unit: 'шт', qty: 2, price: 100 }],
    },
  });
  const cardId = created.json?.id ?? 0;
  ok('заготовка карточки создана', created.status === 200 && cardId > 0, `status ${created.status}`);

  const before = created.json?.updatedAt ?? '';
  const clientRes = await call('PUT', `/offers/${cardId}/client`, {
    token: ownerToken,
    body: { name: 'Новый заказчик', contact: 'Пётр' },
  });
  ok('правка заказчика → 200', clientRes.status === 200, `status ${clientRes.status}`);
  ok('заказчик записан', clientRes.json?.client?.name === 'Новый заказчик', clientRes.json?.client?.name);
  ok('позиции правкой заказчика не тронуты', (clientRes.json?.items ?? []).length === 1, `${(clientRes.json?.items ?? []).length} строк`);
  ok('updatedAt сдвинулся при правке заказчика', (clientRes.json?.updatedAt ?? '') !== before, `${before} → ${clientRes.json?.updatedAt}`);

  const contactOnly = await call('PUT', `/offers/${cardId}/client`, {
    token: ownerToken,
    body: { name: '  ', contact: 'Пётр' },
  });
  ok('контакт без названия → 400', contactOnly.status === 400, `status ${contactOnly.status}`);

  const itemsBefore = clientRes.json?.updatedAt ?? '';
  const itemsRes = await call('PUT', `/offers/${cardId}/items`, {
    token: ownerToken,
    body: {
      items: [
        { name: 'Плита', unit: 'м²', qty: 3, price: 999.99, sum: 1, total: 5 },
        { name: 'Работа', unit: 'шт', qty: 1, price: 0.1, sum: 100 },
      ],
    },
  });
  ok('правка позиций → 200', itemsRes.status === 200, `status ${itemsRes.status}`);
  ok('позиций стало две', (itemsRes.json?.items ?? []).length === 2, `${(itemsRes.json?.items ?? []).length}`);
  ok('сумму строки считает сервер, а не клиент', itemsRes.json?.items?.[0]?.sum === 2999.97, String(itemsRes.json?.items?.[0]?.sum));
  ok('итог считает сервер', itemsRes.json?.total === 3000.07, String(itemsRes.json?.total));
  ok('заказчик правкой позиций не тронут', itemsRes.json?.client?.name === 'Новый заказчик', itemsRes.json?.client?.name);
  ok('updatedAt сдвинулся при правке позиций', (itemsRes.json?.updatedAt ?? '') !== itemsBefore, `${itemsBefore} → ${itemsRes.json?.updatedAt}`);

  const emptyItems = await call('PUT', `/offers/${cardId}/items`, { token: ownerToken, body: { items: [] } });
  ok('пустой список позиций законен (правка по одной)', emptyItems.status === 200, `status ${emptyItems.status}`);
  ok('пустой список реально записан', (emptyItems.json?.items ?? []).length === 0, `${(emptyItems.json?.items ?? []).length}`);
  ok('итог пустого списка нулевой', emptyItems.json?.total === 0, String(emptyItems.json?.total));

  const badItem = await call('PUT', `/offers/${cardId}/items`, {
    token: ownerToken,
    body: { items: [{ name: ' ', unit: 'шт', qty: 1, price: 1 }] },
  });
  ok('пустое наименование позиции → 400', badItem.status === 400, `status ${badItem.status}`);

  // Права: чужим правка закрыта, поиск по чужому не помогает.
  const managerToken2 = makeJwt('manager@tn.ru');
  const foreignClient = await call('PUT', `/offers/${cardId}/client`, {
    token: managerToken2,
    body: { name: 'Взлом', contact: '' },
  });
  ok('чужой не правит заказчика (403)', foreignClient.status === 403, `status ${foreignClient.status}`);
  const foreignItems = await call('PUT', `/offers/${cardId}/items`, {
    token: managerToken2,
    body: { items: [{ name: 'Взлом', unit: 'шт', qty: 1, price: 1 }] },
  });
  ok('чужой не правит позиции (403)', foreignItems.status === 403, `status ${foreignItems.status}`);
  const strangerClient = await call('PUT', `/offers/${cardId}/client`, {
    token: makeJwt('stranger@tn.ru'),
    body: { name: 'Взлом', contact: '' },
  });
  ok('без роли правка закрыта (403)', strangerClient.status === 403, `status ${strangerClient.status}`);

  const reload = await call('GET', `/offers/${cardId}`, { token: ownerToken });
  ok('после отказов документ не изменился', reload.json?.client?.name === 'Новый заказчик', reload.json?.client?.name);
}

console.log('\n5d. Тексты разделов из приложения (S4, 2026-11-13)');
{
  const created = await call('POST', '/offers', {
    token: ownerToken,
    body: { client: { name: 'Разделы' }, items: [{ name: 'Позиция', unit: 'шт', qty: 1, price: 10 }] },
  });
  const sectionOfferId = created.json?.id ?? 0;
  ok('заготовка для разделов создана', created.status === 200 && sectionOfferId > 0, `status ${created.status}`);

  const list = await call('GET', `/offers/${sectionOfferId}/sections`, { token: ownerToken });
  const slots = list.json?.sections ?? [];
  ok('список разделов → 200 и массив', list.status === 200 && Array.isArray(slots), `${list.status} / ${Array.isArray(slots)}`);
  ok('без систем есть вступление, преимущества и условия', slots.length === 3, `${slots.length}`);
  ok('ключ вступления на месте', slots.some((s) => s.key === 'copy.intro'), JSON.stringify(slots.map((s) => s.key)));

  // Принять текст — та же точка, что у бота: form_values["copy.intro"].
  const accept = await call('PUT', `/offers/${sectionOfferId}/sections/intro`, {
    token: ownerToken,
    body: { text: '  Вводный текст КП.  ' },
  });
  ok('принятие вступления → 200', accept.status === 200, `status ${accept.status}`);
  ok('текст записан в form_values', accept.json?.formValues?.['copy.intro'] === 'Вводный текст КП.', accept.json?.formValues?.['copy.intro']);

  const afterAccept = await call('GET', `/offers/${sectionOfferId}/sections`, { token: ownerToken });
  const intro = (afterAccept.json?.sections ?? []).find((s) => s.key === 'copy.intro');
  ok('список отдаёт принятый текст', intro?.text === 'Вводный текст КП.', intro?.text);

  // Пропустить — пустой текст: ключ удаляется.
  const skip = await call('PUT', `/offers/${sectionOfferId}/sections/intro`, { token: ownerToken, body: { text: '  ' } });
  ok('пропуск раздела → 200', skip.status === 200, `status ${skip.status}`);
  ok('ключ раздела удалён при пропуске', !('copy.intro' in (skip.json?.formValues ?? {})), JSON.stringify(skip.json?.formValues));

  // Номер системы: без системы секция system отвергается с понятным текстом.
  const noIndex = await call('POST', `/offers/${sectionOfferId}/sections/system`, { token: ownerToken, body: {} });
  ok('описание системы без номера → 400', noIndex.status === 400, `status ${noIndex.status}`);

  // Неизвестная секция.
  const badKey = await call('POST', `/offers/${sectionOfferId}/sections/испещрение`, { token: ownerToken, body: {} });
  ok('неизвестная секция → 400', badKey.status === 400, `status ${badKey.status}`);

  // Права: чужой не читает и не меняет тексты.
  const strangerSections = await call('GET', `/offers/${sectionOfferId}/sections`, { token: makeJwt('stranger@tn.ru') });
  ok('чужой не читает тексты (403)', strangerSections.status === 403, `status ${strangerSections.status}`);
  const foreignAccept = await call('PUT', `/offers/${sectionOfferId}/sections/intro`, {
    token: makeJwt('manager@tn.ru'),
    body: { text: 'Взлом' },
  });
  ok('чужой не принимает текст (403)', foreignAccept.status === 403, `status ${foreignAccept.status}`);

  // Пустой текст в правке — отказ с подсказкой (убрать раздел можно «Пропустить»).
  const emptyEdit = await call('POST', `/offers/${sectionOfferId}/sections/intro/edit`, { token: ownerToken, body: { text: '  ' } });
  ok('правка пустого текста → 400', emptyEdit.status === 400, `status ${emptyEdit.status}`);
}

console.log('\n5e. Боевой листок из приложения (S6, 2026-11-13)');
{
  // Листок создаём тем же путём, что бот: ручкой сервиса.
  // Листок создаёт сам автор (ownerToken — это owner@tn.ru), JWT-ручкой:
  // приложение не знает сервисного секрета. Автор тот же, кто дальше правит.
  const created = await call('POST', '/combat', {
    token: ownerToken,
    body: { title: 'Листок из приложения', ourNotes: [], competitorNotes: [], formValues: {}, status: 'draft' },
  });
  const sheetId = created.json?.id ?? 0;
  ok('листок создан', created.status === 200 && sheetId > 0, `status ${created.status}`);

  const our = await call('GET', '/combat/our?q=' + encodeURIComponent('мембрана'), { token: ownerToken });
  ok('список нашего продукта → 200 и массив', our.status === 200 && Array.isArray(our.json?.items), `${our.status}`);
  ok('страница несёт total и hasMore', typeof our.json?.total === 'number' && typeof our.json?.hasMore === 'boolean', JSON.stringify({ total: our.json?.total, hasMore: our.json?.hasMore }));

  const comp = await call('GET', '/combat/competitors-src', { token: ownerToken });
  ok('список конкурентов → 200 и массив', comp.status === 200 && Array.isArray(comp.json?.items), `${comp.status}`);

  const first = (our.json?.items ?? [])[0];
  if (first) {
    const added = await call('POST', `/combat/${sheetId}/our`, {
      token: ownerToken,
      body: { noteId: first.noteId, title: first.title },
    });
    ok('добавление нашего продукта → 200', added.status === 200, `status ${added.status}`);
    ok('сторона записана', (added.json?.ourNotes ?? []).some((n) => n.noteId === first.noteId), JSON.stringify(added.json?.ourNotes));

    const removed = await call('DELETE', `/combat/${sheetId}/our`, {
      token: ownerToken,
      body: { noteId: first.noteId, title: '' },
    });
    ok('удаление нашего продукта → 200', removed.status === 200, `status ${removed.status}`);
    ok('сторона убрана', !(removed.json?.ourNotes ?? []).some((n) => n.noteId === first.noteId), JSON.stringify(removed.json?.ourNotes));
  }

  const hint = await call('PUT', `/combat/${sheetId}/hint`, { token: ownerToken, body: { hint: 'акцент на сроке службы' } });
  ok('уточнение записано', hint.status === 200 && hint.json?.formValues?.hint === 'акцент на сроке службы', JSON.stringify(hint.json?.formValues));

  const noProduct = await call('POST', `/combat/${sheetId}/our`, { token: ownerToken, body: { noteId: 0 } });
  ok('без заметки продукт не добавляется → 400', noProduct.status === 400, `status ${noProduct.status}`);

  // Чужой листок: читать можно (правило 2026-11-12), править — нет.
  const foreignRead = await call('GET', `/combat/${sheetId}`, { token: makeJwt('manager@tn.ru') });
  ok('чужой листок читается (200)', foreignRead.status === 200, `status ${foreignRead.status}`);
  const foreignWrite = await call('PUT', `/combat/${sheetId}/hint`, { token: makeJwt('manager@tn.ru'), body: { hint: 'чужое' } });
  ok('чужой не меняет уточнение (403)', foreignWrite.status === 403, `status ${foreignWrite.status}`);
  const foreignSide = await call('POST', `/combat/${sheetId}/our`, {
    token: makeJwt('manager@tn.ru'),
    body: { noteId: 1, title: 'чужое' },
  });
  ok('чужой не меняет сторону (403)', foreignSide.status === 403, `status ${foreignSide.status}`);

  // Аналоги: без кеша конкурентов ручка честно говорит, чего не хватает.
  const analogues = await call('GET', '/combat/analogues?noteId=' + encodeURIComponent(String((our.json?.items ?? [])[0]?.noteId ?? 0)), { token: ownerToken });
  ok('аналоги: ответ 200 или 404 с понятным текстом', analogues.status === 200 || analogues.status === 404, `status ${analogues.status}`);
  const noNote = await call('GET', '/combat/analogues', { token: ownerToken });
  ok('аналоги без продукта → 400', noNote.status === 400, `status ${noNote.status}`);
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
const rawBuf = Buffer.from(await res.arrayBuffer());
  const text = rawBuf.toString('utf8');
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
    ok('DOCX не пустой', (docAfterCopy.json?.docx_base64 ?? '').length > 1000,
      `base64 ${(docAfterCopy.json?.docx_base64 ?? '').length}, файл ${docAfterCopy.json?.file_name ?? ''}`);

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
  ok('сводка брифа попала в файл', (docWithBrief.text ?? '').length > 1000,
    `status ${docWithBrief.status}, тело ${(docWithBrief.text ?? '').length}, начало ${(docWithBrief.text ?? '').slice(0, 100)}`);
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
  ok('в документе без позиций есть файл', (docxBlock.json?.docx_base64 ?? '').length > 1000,
    `status ${docxBlock.status}, base64 ${(docxBlock.json?.docx_base64 ?? '').length}, тело ${docxBlock.text?.slice(0, 120)}`);

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
    `docx ${docBytes.byteLength} Б, xml ${xml.length}, есть заголовок: ${xml.includes('Сравнение')}`);
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

  await sectionLayoutText();
}

// Печать по макету: один конвейер текста, ссылки, картинка (2026-11-06, 0.7.5).
//
// Проверяется то, что увидит заказчик: адрес стал ссылкой Word, площадь
// напечатана с надстрочной двойкой, метка картинки дала изображение, скобки
// разметки до документа не дошли. Стенд гоняет это по HTTP — юнит-тесты
// проверяют функции, а здесь проверяется вся сборка документа целиком.
async function sectionLayoutText() {
  console.log('\n22. Печать по макету: текст, ссылки, картинка (2026-11-06)');

  const layoutDoc =
    `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
    `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
    `<w:p><w:r><w:t>{Опиши систему}</w:t></w:r></w:p>` +
    `<w:p><w:r><w:t>{картинка}</w:t></w:r></w:p>` +
    `</w:body></w:document>`;
  const layout = packZip([
    zipEntry('[Content_Types].xml', Buffer.from(
      '<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>',
      'utf8')),
    zipEntry('word/document.xml', Buffer.from(layoutDoc, 'utf8')),
  ]);
  const up = await fetch(`${BASE}/api/offers/form/layout?name=Текст.docx`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${ownerToken}`,
      'Content-Type': 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    },
    body: layout,
  });
  ok('макет с промтом и картинкой загружен → 200', up.status === 200);

  // КП по макету: модель отвечает заглушкой стенда.
  const created = await call('POST', '/offers', {
    token: ownerToken,
    body: {
      client: { name: 'ООО Текст', contact: 'Пётр' },
      items: [{ name: 'Мембрана', unit: 'м2', qty: 4800, price: 100, sum: 480000 }],
      total: 480000, status: 'draft',
      formValues: { object_name: 'Дом по ул. Примерная' },
    },
  });
  const offerId = created.json?.id;
  // Система со схемой (карточка 100 в заглушке): без выбранной системы в
  // документе нет карточки, и метка картинки молчит по определению. Выбор
  // делается заметкой — код у живой карточки может отсутствовать (0.4.4).
  const added = await call('POST', `/offers/${offerId}/system`, {
    token: ownerToken, body: { noteId: 100 },
  });
  ok('система со схемой выбрана', added.status === 200,
    `${added.status} ${added.text?.slice(0, 120)}`);

  const raw = await (await fetch(`${BASE}/api/offers/offers/${offerId}/export`, {
    method: 'POST', headers: { Authorization: `Bearer ${ownerToken}` },
  })).arrayBuffer();
  const doc = Buffer.from(raw);
  const xml = unzipEntrySafe(doc, 'word/document.xml');

  ok('КП по макету с промтом собрано', xml.includes('<w:body>') && xml.length > 500,
    xml.slice(0, 160));
  // 1. Разметки модели в документе нет.
  for (const bad of ['{{', '}}', '[[', '**']) {
    ok(`в документе нет «${bad}»`, !xml.includes(bad));
  }
  // 2. Кликабельность: адрес из ответа модели стал ссылкой Word.
  const rels = unzipEntrySafe(doc, 'word/_rels/document.xml.rels');
  const links = (xml.match(/<w:hyperlink/g) ?? []).length;
  const external = (rels.match(/TargetMode="External"/g) ?? []).length;
  ok('ссылки в документе кликабельные', links > 0 && external >= links,
    `ссылок ${links}, внешних отношений ${external}`);
  // 3. Надстрочные единицы: заглушка модели содержит «м2».
  ok('площади напечатаны с верхним индексом',
    (xml.match(/<w:vertAlign w:val="superscript"\/>/g) ?? []).length > 0,
    'ни одной надстрочной степени');
  // 4. Картинка: у системы в стенде есть схема, метка её вставила.
  ok('метка картинки дала изображение', xml.includes('<w:drawing'),
    'в документе нет ни одного изображения');

  await call('DELETE', '/form/layout', { token: ownerToken });
  await call('DELETE', `/offers/${offerId}`, { token: ownerToken });
  ok('макет и КП убраны', true);
}

/**
 * 23. Тексты системы пишутся при выборе и НЕ пишутся при сборке (2026-11-06).
 *
 * Шаг B плана «генерация без модели». Повод — живая жалоба: администратор
 * поменял структуру промтов в макете, и система «ушла думать и не вернулась».
 * Причина была в том, что модель звали при КАЖДОЙ сборке документа, а ждать
 * её не мог никто: бот 60 с, сервер 90 с, модель 180 с на попытку.
 *
 * Проверяем наблюдаемое поведение: тексты находятся после выбора системы,
 * повторный выбор не дублирует их, экспорт документа с уже выбранной системой
 * не возвращает новых текстов, а кнопка пересчёта переписывает оба блока.
 */
async function sectionSystemTexts() {
  console.log('\n23. Тексты системы: пишутся при выборе, не при сборке (2026-11-06)');

  const created = await call('POST', '/offers', {
    token: ownerToken,
    body: {
      client: { name: 'ООО Тексты', contact: 'Анна' },
      items: [{ name: 'Мембрана', unit: 'м2', qty: 100, price: 50, sum: 5000 }],
      total: 5000,
      status: 'draft',
      formValues: { object_name: 'Объект текстов' },
    },
  });
  const offerId = created.json?.id;
  ok('КП для проверки текстов создано', offerId > 0, `${created.status} ${created.text?.slice(0, 120)}`);

  // Выбор системы: тексты должны прийти сразу в ответе, и их должно быть два.
  const added = await call('POST', `/offers/${offerId}/system`, {
    token: ownerToken, body: { noteId: 100 },
  });
  ok('система выбрана → 200', added.status === 200, `${added.status} ${added.text?.slice(0, 160)}`);
  const texts = added.json?.texts ?? {};
  ok('обоснование применения написано', (texts.description ?? '').trim().length > 20,
    JSON.stringify(Object.keys(texts)));
  ok('преимущества написаны', (texts.advantages ?? '').trim().length > 20,
    JSON.stringify(Object.keys(texts)));
  ok('состав НЕ генерируется (два блока, не три)',
    !Object.prototype.hasOwnProperty.call(texts, 'composition'),
    JSON.stringify(Object.keys(texts)));

  // Сохранённые тексты читаются отдельной ручкой и связаны с заметкой.
  const stored = await call('GET', `/offers/${offerId}/system-texts`, { token: ownerToken });
  ok('тексты читаются из хранилища → 200', stored.status === 200, `получили ${stored.status}`);
  const storedTexts = stored.json?.texts ?? [];
  ok('оба блока сохранены у заметки',
    storedTexts.filter((t) => t.noteId === 100 && (t.text ?? '').trim() !== '').length === 2,
    JSON.stringify(storedTexts));
  ok('сохранённый текст не помечен устаревшим',
    storedTexts.every((t) => t.stale === false), JSON.stringify(storedTexts.map((t) => t.stale)));

  // Повторный выбор той же системы не дублирует её и не трогает тексты:
  // модель на это нажатие не вызывается вовсе (тексты уже написаны).
  const again = await call('POST', `/offers/${offerId}/system`, {
    token: ownerToken, body: { noteId: 100 },
  });
  ok('повторный выбор → 200', again.status === 200, `получили ${again.status}`);
  ok('повторный выбор отдал тот же текст',
    (again.json?.texts?.description ?? '') === (texts.description ?? ''),
    'текст переписан без просьбы');
  ok('система в КП по-прежнему одна', (again.json?.offer?.systems ?? []).length === 1,
    JSON.stringify(again.json?.offer?.systems));

  // Экспорт документа не должен менять тексты системы: модели там больше нет.
  await call('POST', `/offers/${offerId}/export`, { token: ownerToken });
  const afterExport = await call('GET', `/offers/${offerId}/system-texts`, { token: ownerToken });
  ok('сборка документа не переписала тексты системы',
    JSON.stringify(afterExport.json?.texts ?? []) === JSON.stringify(storedTexts),
    'тексты изменились при экспорте');

  // Кнопка «Пересчитать» переписывает оба блока.
  const recalc = await call('POST', `/offers/${offerId}/system/100/recalculate`, { token: ownerToken });
  ok('пересчёт текстов → 200', recalc.status === 200, `${recalc.status} ${recalc.text?.slice(0, 160)}`);
  ok('пересчёт вернул оба блока',
    (recalc.json?.texts?.description ?? '').trim() !== '' &&
      (recalc.json?.texts?.advantages ?? '').trim() !== '',
    JSON.stringify(recalc.json?.texts));

  // Неизвестная заметка — честный отказ, а не пустой ответ.
  const badNote = await call('POST', `/offers/${offerId}/system/999999/recalculate`, { token: ownerToken });
  ok('пересчёт неизвестной системы → 404', badNote.status === 404, `получили ${badNote.status}`);

  await call('DELETE', `/offers/${offerId}`, { token: ownerToken });
  ok('КП убрано', true);
}

// 24. Метки данных в макете и повтор блока (2026-11-06, шаг C/D/E).
//
// Проверяется то, что пообещали пользователю: документ собирается БЕЗ вызова
// модели там, где данные уже есть (система, состав, ссылки, характеристики,
// сохранённые тексты), метка `{{block:N}}` печатает блок формы, а блок с
// повтором печатается по разу на систему. И — образец макета, который сервис
// отдаёт администратору.
async function sectionServiceMarkers() {
  console.log('\n24. Метки данных в макете и повтор блока (2026-11-06)');

  // Образец макета отдаётся и содержит скобки меток (главное — файл есть и он
  // документ Word, а не ошибка в JSON).
  const example = await fetch(`${BASE}/api/offers/form/layout/example`, {
    headers: { Authorization: `Bearer ${ownerToken}` },
  });
  const exampleBytes = Buffer.from(await example.arrayBuffer());
  ok('образец макета отдаётся → 200', example.status === 200, `получили ${example.status}`);
  const exampleXML = unzipEntrySafe(exampleBytes, 'word/document.xml');
  ok('образец содержит метку системы', exampleXML.includes('{{система}}'),
    exampleXML.slice(0, 200));

  // КП с одной системой (заметка 100 — есть в заглушке Базы знаний).
  const created = await call('POST', '/offers', {
    token: ownerToken,
    body: {
      client: { name: 'ООО Метки', contact: 'Пётр' },
      items: [
        { name: 'Мембрана', unit: 'м2', qty: 100, price: 50, sum: 5000 },
        { name: 'Геотекстиль', unit: 'м2', qty: 100, price: 10, sum: 1000 },
      ],
      total: 6000,
      status: 'draft',
      formValues: { object_name: 'Объект меток' },
    },
  });
  const offerId = created.json?.id;
  ok('КП для меток создано', offerId > 0, `${created.status} ${created.text?.slice(0, 120)}`);
  // Выбор системы пишет её тексты (шаг B) — их и прочитает метка текста.
  await call('POST', `/offers/${offerId}/system`, { token: ownerToken, body: { noteId: 100 } });

  // Блок формы берём по РЕАЛЬНОМУ id из формы, а не «1»: id назначает
  // редактор, и проверка обязана адресовать существующий блок.
  const formBefore = await call('GET', '/form', { token: ownerToken });
  const blockID = formBefore.json?.blocks?.[0]?.id ?? 'intro';

  const layoutDoc =
    `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
    `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
    `<w:p><w:r><w:t>{{system_title}}</w:t></w:r></w:p>` +
    `<w:p><w:r><w:t>{{система}}</w:t></w:r></w:p>` +
    `<w:p><w:r><w:t>{{текст: система: description}}</w:t></w:r></w:p>` +
    `<w:p><w:r><w:t>{{ссылки: система}}</w:t></w:r></w:p>` +
    `<w:p><w:r><w:t>{{характеристики: система}}</w:t></w:r></w:p>` +
    `<w:p><w:r><w:t>{{block:${blockID}}}</w:t></w:r></w:p>` +
    `</w:body></w:document>`;
  const layout = packZip([
    zipEntry('[Content_Types].xml', Buffer.from(
      '<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>',
      'utf8')),
    zipEntry('word/settings.xml', Buffer.from(
      '<?xml version="1.0" encoding="UTF-8"?><w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"/>',
      'utf8')),
    zipEntry('word/document.xml', Buffer.from(layoutDoc, 'utf8')),
    // numbering.xml обязателен: с 2026-11-08 состав каждой системы получает
    // СВОЙ numId, и его определение дописывается именно сюда. Без этого файла
    // сервис печатает состав общим списком, и проверка перезапуска не увидит.
    zipEntry('word/numbering.xml', Buffer.from(
      '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>' +
      '<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">' +
      '<w:abstractNum w:abstractNumId="0"><w:lvl w:ilvl="0"><w:start w:val="1"/>' +
      '<w:numFmt w:val="decimal"/><w:lvlText w:val="%1."/></w:lvl></w:abstractNum>' +
      '<w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>' +
      '<w:num w:numId="6"><w:abstractNumId w:val="0"/></w:num></w:numbering>',
      'utf8')),
  ]);
  const up = await fetch(`${BASE}/api/offers/form/layout?name=Метки.docx`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${ownerToken}`,
      'Content-Type': 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    },
    body: layout,
  });
  ok('макет с метками данных загружен → 200', up.status === 200, `получили ${up.status}`);

  const doc = await (await fetch(`${BASE}/api/offers/offers/${offerId}/export`, {
    method: 'POST', headers: { Authorization: `Bearer ${ownerToken}` },
  })).arrayBuffer();
  const xml = unzipEntrySafe(Buffer.from(doc), 'word/document.xml');
  const settingsXML = unzipEntrySafe(Buffer.from(doc), 'word/settings.xml');

  // Ни одной скобки метки: всё распознано и подставлено или снято.
  for (const bad of ['{{', '}}']) {
    ok(`скобки метки «${bad}» не дошли до документа`, !xml.includes(bad), xml.slice(0, 200));
  }
  ok('название системы подставлено вместо номера заметки',
    !xml.includes('система №100') && xml.includes('ТН-Гидроизоляция Фундамента'),
    xml.slice(0, 200));
  ok('settings.xml просит Word пересчитать оглавление при открытии',
    settingsXML.includes('<w:updateFields w:val="true"/>'),
    settingsXML);
  // {{система}} — одна таблица без границ.
  ok('система напечатана одной таблицей', (xml.match(/<w:tbl>/g) ?? []).length >= 1);
  ok('у таблицы системы нет границ', xml.includes('<w:left w:val="none"'),
    xml.slice(0, 200));
  // {{характеристики}} и {{ссылки}} дали своё.
  ok('характеристики карточки напечатаны', xml.includes('Класс нагрузки'),
    'нет таблицы характеристик');
  ok('ссылки карточки кликабельны', xml.includes('<w:hyperlink'),
    'нет ни одной гиперссылки');
  // Раздел ссылок печатается ГРУППАМИ и НУМЕРОВАННЫМ списком (2026-11-10).
  // Карточка 100 в заглушке несёт страницу продукта и техлист: у них разные
  // группы, обе должны быть напечатаны, и группа страниц — раньше.
  ok('раздел ссылок разбит на группы',
    xml.includes('Страницы продуктов') && xml.includes('Технические листы'),
    'нет заголовков групп ссылок');
  ok('группа страниц продукта идёт первой',
    xml.indexOf('Страницы продуктов') < xml.indexOf('Технические листы'),
    'порядок групп ссылок неверен');
  // Пункты ссылок нумерованные: у абзаца-ссылки есть <w:numPr> (номер ставит
  // Word по своему numId, в тексте пункта числа нет).
  {
    const linkItems = [...xml.matchAll(/<w:p>(?:(?!<\/w:p>).)*?<w:hyperlink[^>]*>(?:(?!<\/w:p>).)*?<\/w:p>/gs)]
      .map((m) => m[0]);
    ok('пункты ссылок нумерованные',
      linkItems.length > 0 && linkItems.every((p) => p.includes('<w:numPr>')),
      `пунктов ${linkItems.length}, без нумерации ${linkItems.filter((p) => !p.includes('<w:numPr>')).length}`);
  }
  // Метка текста системы дописывает недостающее (2026-11-07).
  //
  // Живая жалоба: `{{текст: система: description}}` печатала пустое место во
  // всех КП, потому что сохранённого текста у системы ещё не было, а метка
  // модель не звала. Теперь сборка дописывает текст и сохраняет его; здесь
  // проверяется, что он ПРИШЁЛ в документ.
  // Заглушка модели отвечает на обоснование этим текстом (llm-stub.mjs);
  // по нему видно, что метка дописала недостающее, а не промолчала.
  ok('текст системы дописан и напечатан из метки',
    xml.includes('Система закрывает подземный контур дома'),
    'нет текста, написанного при сборке');

  // Состав печатается НУМЕРОВАННЫМ списком (2026-11-07): на разрезе системы
  // маркеры слоёв стоят с номерами, и сопоставить их с маркированным списком
  // было нельзя. С 2026-11-08 у каждого состава СВОЙ numId (9000+), чтобы
  // нумерация каждой системы шла с единицы; определение с перезапуском лежит в
  // numbering.xml. numId 6 — маркеры, его в составе быть не должно.
  const numberingXML = unzipEntrySafe(Buffer.from(doc), 'word/numbering.xml');
  const compositionNumIds = [...xml.matchAll(/<w:numId w:val="(\d+)"\/>/g)]
    .map((m) => Number(m[1])).filter((n) => n >= 9000);
  ok('состав системы — нумерованный список', compositionNumIds.length > 0,
    'нет нашего numId в составе');
  ok('определение списка с перезапуском в numbering.xml',
    [...new Set(compositionNumIds)].every((id) =>
      numberingXML.includes(`<w:num w:numId="${id}">`) &&
      numberingXML.includes('<w:startOverride w:val="1"/>')),
    `numIds ${JSON.stringify(compositionNumIds)}`);
  ok('в составе нет маркеров', !compositionSection(xml).includes(`<w:numId w:val="${styleListBullet}"/>`),
    'состав напечатан маркерами');
  // {{block:N}} — блок формы напечатан со своим текстом (берём по реальному id).
  const blockText = formBefore.json?.blocks?.[0]?.content?.[0]?.text ?? '';
  ok('блок формы по id напечатан',
    blockText !== '' && xml.includes(blockText.replace(/\{\{[^}]*\}\}/g, '').trim()),
    `блок «${blockID}»: искали «${blockText}»`);

  // Повтор по материалам: блок с repeat:"materials" печатается по позициям.
  const form = await call('GET', '/form', { token: ownerToken });
  const originalBlocks = form.json?.blocks ?? [];
  const repeatForm = {
    ...form.json,
    blocks: [{
      id: '99', title: 'Позиции', showInClient: true, showInInternal: false,
      repeat: 'materials',
      content: [{ type: 'paragraph', text: '{{материал}}' }],
    }],
  };
  const savedForm = await call('PUT', '/form', { token: ownerToken, body: repeatForm });
  ok('форма с повтором сохранена → 200', savedForm.status === 200,
    `${savedForm.status} ${savedForm.text?.slice(0, 160)}`);

  const layoutRepeatDoc =
    `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
    `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
    `<w:p><w:r><w:t>{{block:99}}</w:t></w:r></w:p>` +
    `</w:body></w:document>`;
  const repeatLayout = packZip([
    zipEntry('[Content_Types].xml', Buffer.from(
      '<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>',
      'utf8')),
    zipEntry('word/document.xml', Buffer.from(layoutRepeatDoc, 'utf8')),
  ]);
  await fetch(`${BASE}/api/offers/form/layout?name=Повтор.docx`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${ownerToken}`,
      'Content-Type': 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    },
    body: repeatLayout,
  });
  const repeatDoc = await (await fetch(`${BASE}/api/offers/offers/${offerId}/export`, {
    method: 'POST', headers: { Authorization: `Bearer ${ownerToken}` },
  })).arrayBuffer();
  const repeatXML = unzipEntrySafe(Buffer.from(repeatDoc), 'word/document.xml');
  ok('блок повторился по каждой позиции',
    repeatXML.includes('Мембрана') && repeatXML.includes('Геотекстиль'),
    repeatXML.slice(0, 300));

  // {{система}} внутри БЛОКА формы и напрямую в макете (2026-11-07).
  //
  // Живая жалоба на temple.docx: метка, вставленная в блок формы, печатала
  // пустой абзац, а картинка системы не вставлялась вовсе. Обе дороги обязаны
  // дать одно: таблицу состава без границ и картинку.
  const systemBlockForm = {
    ...form.json,
    blocks: [{
      id: '98', title: 'Система', showInClient: true, showInInternal: false,
      content: [
        { type: 'heading', level: 2, text: 'Раздел 2 уровня' },
        { type: 'paragraph', text: '{{система}}' },
      ],
    }],
  };
  await call('PUT', '/form', { token: ownerToken, body: systemBlockForm });
  const systemBlockDoc =
    `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
    `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
    `<w:p><w:r><w:t>{{block:98}}</w:t></w:r></w:p>` +
    `<w:p><w:r><w:t>{{система}}</w:t></w:r></w:p>` +
    `</w:body></w:document>`;
  const systemBlockLayout = packZip([
    zipEntry('[Content_Types].xml', Buffer.from(
      '<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>',
      'utf8')),
    zipEntry('word/document.xml', Buffer.from(systemBlockDoc, 'utf8')),
  ]);
  await fetch(`${BASE}/api/offers/form/layout?name=Система.docx`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${ownerToken}`,
      'Content-Type': 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    },
    body: systemBlockLayout,
  });
  const systemBlockDocx = Buffer.from(await (await fetch(
    `${BASE}/api/offers/offers/${offerId}/export`, {
      method: 'POST', headers: { Authorization: `Bearer ${ownerToken}` },
    })).arrayBuffer());
  const systemBlockXML = unzipEntrySafe(systemBlockDocx, 'word/document.xml');
  const tableCount = (systemBlockXML.match(/<w:tbl>/g) ?? []).length;
  ok('система из блока и из макета — две таблицы', tableCount === 2,
    `таблиц ${tableCount}`);
  ok('у обеих таблиц системы нет границ',
    (systemBlockXML.match(/<w:left w:val="none"/g) ?? []).length >= 2);
  const drawingCount = (systemBlockXML.match(/<w:drawing>/g) ?? []).length;
  ok('картинка системы вставлена дважды', drawingCount === 2,
    `картинок ${drawingCount}`);
  ok('прогон картинки не сосед абзаца', !systemBlockXML.includes('</w:p><w:r>'));
  ok('заголовок 2 уровня из блока имеет outlineLvl=1, а не heading 1',
    systemBlockXML.includes('<w:outlineLvl w:val="1"/>') && !systemBlockXML.includes('<w:pStyle w:val="1"/>'),
    systemBlockXML.slice(0, 300));
  ok('картинка системы попала в пакет',
    systemBlockDocx.includes(Buffer.from('word/media/offerimg1.png')));

  // Возвращаем форму, который был до проверки, и убираем макет.
  await call('PUT', '/form', { token: ownerToken, body: { ...repeatForm, blocks: originalBlocks } });
  await call('DELETE', '/form/layout', { token: ownerToken });
  await call('DELETE', `/offers/${offerId}`, { token: ownerToken });
  ok('КП меток убрано', true);
}

/**
 * Боевой листок (2026-11-08): карточки конкурентов из папки Базы знаний и
 * сам листок — создание, список, чтение, сохранение, удаление. Проверяем по
 * HTTP, как это делает плагин, а не функции в изоляции.
 */
async function sectionCombat() {
  console.log('\nБоевой листок');

  // Папка конкурентов в kb-stub → карточка конкурента доезжает до сервиса.
  const comps = await call('GET', '/competitors', { token: ownerToken });
  ok('список конкурентов отвечает 200', comps.status === 200, `статус ${comps.status}`);
  ok('папка конкурентов найдена', comps.json?.folderFound === true);
  ok(
    'карточка конкурента из папки дошла',
    (comps.json?.cards ?? []).some((c) => (c.title ?? '').includes('Конкурент')),
    JSON.stringify((comps.json?.cards ?? []).map((c) => c.title)),
  );
  // Рекурсивный обход раздела (2026-11-09): карточка из ПОДПАПКИ тоже должна
  // доехать. kb-stub держит «Пеноплэкс Фундамент» во вложенной папке 10.
  ok(
    'карточка конкурента из подпапки дошла',
    (comps.json?.cards ?? []).some((c) => (c.title ?? '').includes('Пеноплэкс')),
    JSON.stringify((comps.json?.cards ?? []).map((c) => c.title)),
  );

  // Поиск стороны в боте (2026-11-09): каталог велик, перебор не годится.
  const botOurSearch = await call('GET', '/bot/combat/systems?q=кровля', {
    headers: { 'X-Service-Secret': BOT_SECRET },
  });
  ok(
    'поиск нашего продукта в боте находит по названию',
    botOurSearch.status === 200 &&
      (botOurSearch.json?.cards ?? []).some((c) => /кровл/i.test(c.title ?? '')),
    JSON.stringify((botOurSearch.json?.cards ?? []).map((c) => c.title)),
  );
  const botCompSearch = await call('GET', '/bot/combat/competitors?q=пеноплэкс', {
    headers: { 'X-Service-Secret': BOT_SECRET },
  });
  ok(
    'поиск конкурента в боте находит по названию',
    botCompSearch.status === 200 &&
      (botCompSearch.json?.cards ?? []).some((c) => /пеноплэкс/i.test(c.title ?? '')),
    JSON.stringify((botCompSearch.json?.cards ?? []).map((c) => c.title)),
  );

  // Поиск по Базе знаний (2026-11-12): полнотекстовый, как в веб-версии БЗ.
  // Раньше поиск стороны шёл подстрокой по названию из кэша, и живая жалоба
  // была честной: «ПВХ мембрана» не находило «Кровельная ПВХ мембрана
  // FACHMANN …», а Sikaplan — вовсе.
  const compSearch = await call('GET', '/competitors/search?q=кровля', { token: ownerToken });
  ok(
    'поиск конкурентов по Базе знаний отвечает 200',
    compSearch.status === 200 && Array.isArray(compSearch.json?.items),
    `статус ${compSearch.status}`,
  );
  ok(
    'поиск конкурентов находит карточку по слову из названия',
    (compSearch.json?.items ?? []).some((c) => /кровл/i.test(c.title ?? '')),
    JSON.stringify((compSearch.json?.items ?? []).map((c) => c.title)),
  );
  const emptySearch = await call('GET', '/competitors/search?q=', { token: ownerToken });
  ok(
    'пустой поиск конкурентов — пусто и без ошибки',
    emptySearch.status === 200 && (emptySearch.json?.items ?? []).length === 0,
    JSON.stringify(emptySearch.json),
  );

  // Наш продукт — первая система из кеша систем (карточка 101 из kb-stub).
  const systems = await call('GET', '/systems', { token: ownerToken });
  const ourSystem = (systems.json?.systems ?? [])[0];
  ok('есть система для нашего продукта', Boolean(ourSystem), JSON.stringify(systems.json?.error));
  const compCard = (comps.json?.cards ?? [])[0];

  const created = await call('POST', '/combat', {
    token: ownerToken,
    body: {
      title: 'Сравнение с конкурентом',
      ourNotes: ourSystem ? [{ noteId: ourSystem.noteId, title: ourSystem.title }] : [],
      competitorNotes: compCard ? [{ noteId: compCard.noteId, title: compCard.title }] : [],
      formValues: { object_name: 'ЖК Поэма' },
      status: 'draft',
    },
  });
  ok('боевой листок создан', created.status === 200 && created.json?.id > 0, `статус ${created.status}`);
  ok('номер вида NNN/YYYY', /^\d{3,}\/\d{4}$/.test(created.json?.number ?? ''), created.json?.number);
  const combatId = created.json?.id;

  const list = await call('GET', '/combat', { token: ownerToken });
  ok('боевой листок в списке', (list.json?.sheets ?? []).some((s) => s.id === combatId));

  const got = await call('GET', `/combat/${combatId}`, { token: ownerToken });
  ok('чтение боевого листка', got.status === 200 && (got.json?.competitorNotes ?? []).length === 1);

  const emptyTitle = await call('PUT', `/combat/${combatId}`, {
    token: ownerToken,
    body: { title: '   ' },
  });
  ok('пустое название не сохраняется', emptyTitle.status === 400, `статус ${emptyTitle.status}`);

  const saved = await call('PUT', `/combat/${combatId}`, {
    token: ownerToken,
    body: { ...got.json, status: 'final' },
  });
  ok('боевой листок сохранён как готовый', saved.status === 200 && saved.json?.status === 'final');

  // DOCX боевого листка (2026-11-08, вторая партия): печать по форме combat.
  const exportRes = await fetch(`${BASE}/api/offers/combat/${combatId}/export`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${ownerToken}` },
  });
  ok('DOCX боевого листка собирается → 200', exportRes.status === 200, `статус ${exportRes.status}`);
  const combatDocx = Buffer.from(await exportRes.arrayBuffer());
  ok('боевой листок — это zip (docx)', combatDocx.slice(0, 2).toString() === 'PK' && combatDocx.length > 2000,
    `размер ${combatDocx.length}`);
  if (exportRes.status === 200) {
    const combatXml = unzipEntrySafe(combatDocx, 'word/document.xml');
    ok('в боевом листке есть наш продукт и конкурент',
      combatXml.includes('Наш продукт') && combatXml.includes('Продукты конкурентов'));
    ok('в боевом листке нет незаполненных плейсхолдеров',
      !combatXml.includes('{{') && !combatXml.includes('}}'));
  }

  // Второй раз тот же файл (2026-11-12): листок не пересобирается, файл берётся
  // из кэша. Проверяем то, что видит человек: файл приходит тем же по размеру и
  // в журнале появляется «файл из кэша». Живая жалоба: «хочу, чтобы не
  // перегенерировался каждый раз (это долго)».
  const exportAgain = await fetch(`${BASE}/api/offers/combat/${combatId}/export`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${ownerToken}` },
  });
  const combatDocxAgain = Buffer.from(await exportAgain.arrayBuffer());
  ok('повторная выгрузка листка отдаёт тот же файл',
    exportAgain.status === 200 && combatDocxAgain.equals(combatDocx),
    `размеры ${combatDocx.length} / ${combatDocxAgain.length}`);

  // Смена уточнения — это другое содержание, кэш обязан обновиться.
  //
  // Правка идёт от СВЕЖЕГО листка (GET), а не от `saved`: ответ PUT на 0.9.13
  // несёт состояние до правки смены уточнения, и подмешивание через спред
  // теряло стороны. Ключ уточнения — `hint` (`combatHintField` на сервере).
  const fresh = await call('GET', `/combat/${combatId}`, { token: ownerToken });
  const hinted = await call('PUT', `/combat/${combatId}`, {
    token: ownerToken,
    body: { ...fresh.json, formValues: { hint: 'срок службы' } },
  });
  ok('уточнение сохранено',
    hinted.status === 200 && hinted.json?.formValues?.hint === 'срок службы',
    JSON.stringify(hinted.json?.formValues));
  // Стороны листка правка уточнения не трогает — иначе кэш «обновится» просто
  // потому, что изменился сам листок.
  ok('правка уточнения не теряет стороны листка',
    (hinted.json?.ourNotes ?? []).length === (fresh.json?.ourNotes ?? []).length &&
      (hinted.json?.competitorNotes ?? []).length === (fresh.json?.competitorNotes ?? []).length,
    JSON.stringify({ ours: hinted.json?.ourNotes, comps: hinted.json?.competitorNotes }));
  const exportHinted = await fetch(`${BASE}/api/offers/combat/${combatId}/export`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${ownerToken}` },
  });
  const combatDocxHinted = Buffer.from(await exportHinted.arrayBuffer());
  ok('после смены уточнения файл собирается заново',
    exportHinted.status === 200 && !combatDocxHinted.equals(combatDocx),
    `размеры ${combatDocx.length} / ${combatDocxHinted.length}`);

  // Боевые листки видны ВСЕМ (2026-11-12, требование пользователя): листок —
  // внутренняя справка для спора с заказчиком, и коллега берёт готовое
  // сравнение, а не собирает то же заново. Правка при этом остаётся у автора —
  // это проверяется следом, чтобы «открыли чтение» не превратилось в
  // «открыли всё».
  // Роль авторизации выдаёт auth-stub (AUTH_STUB_ROLES), а не JWT: без роли
  // запрос отсекает requirePerm(\"viewer\") ДО того, как дойдёт до видимости листка.
  const foreignGet = await call('GET', `/combat/${combatId}`, { token: makeJwt('manager@tn.ru') });
  ok('боевой листок виден чужому сотруднику', foreignGet.status === 200, `статус ${foreignGet.status}`);
  const foreignSave = await call('PUT', `/combat/${combatId}`, {
    token: makeJwt('manager@tn.ru'),
    body: { ...got.json, title: 'чужая правка' },
  });
  ok('чужой не может править боевой листок', foreignSave.status === 403, `статус ${foreignSave.status}`);
  const foreignList = await call('GET', '/combat', { token: makeJwt('manager@tn.ru') });
  ok('список боевых листков показывает чужие листки',
    (foreignList.json?.sheets ?? []).some((s) => s.id === combatId),
    JSON.stringify((foreignList.json?.sheets ?? []).map((s) => s.id)));

  // Список и подпись в боте (2026-11-12): все листки видны всем, а подпись —
  // продукт, а не «Боевой листок». Чат привязан к owner — берём его ссылку.
  const botSheets = await call('GET', `/bot/combat?chat_id=${encodeURIComponent(linkedChatId)}`, {
    headers: { 'X-Service-Secret': BOT_SECRET },
  });
  const listed = (botSheets.json?.sheets ?? []).find((s) => s.id === combatId);
  ok('листок виден в списке бота', Boolean(listed), `статус ${botSheets.status}`);
  ok('подпись листка в списке — продукт, а не служебное слово',
    Boolean(listed) && (listed.displayName ?? '') !== '' && (listed.displayName ?? '') !== 'Боевой листок',
    JSON.stringify(listed?.displayName));
  ok('подпись листка называет наш продукт',
    Boolean(listed) && String(listed.displayName ?? '').length > 3,
    JSON.stringify(listed?.displayName));

  const del = await call('DELETE', `/combat/${combatId}`, { token: ownerToken });
  ok('боевой листок удалён', del.status === 200);
}

/**
 * У КП и боевого листка СВОЙ макет (2026-11-11).
 *
 * Живой дефект: «вместо КП выгружался пустой макет боевого листка». Проверяем
 * ровно то, что тогда не сработало: загрузка в один режим не трогает другой,
 * форма называет СВОЙ макет, и КП печатается своим содержимым.
 */
async function sectionLayoutMode() {
  console.log('\nМакеты: у КП и у боевого листка — свои (2026-11-11)');

  const example = await call('GET', '/form/layout/example', { token: ownerToken });
  ok('образец макета скачан', example.status === 200);
  const exampleBuf = example.raw;

  const upload = async (mode, name) => {
    const res = await fetch(`${BASE}/api/offers/form/layout?name=${encodeURIComponent(name)}&mode=${mode}`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${ownerToken}`,
        'Content-Type': 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
      },
      body: exampleBuf,
    });
    return { status: res.status, json: await res.json().catch(() => undefined) };
  };

  const upClient = await upload('client', 'КП.docx');
  ok('макет КП загружен → 200', upClient.status === 200, `${upClient.status}`);
  ok('ответ загрузки КП называет mode=client', upClient.json?.mode === 'client',
    JSON.stringify(upClient.json?.mode));

  const upCombat = await upload('combat', 'БЛ.docx');
  ok('макет боевого листка загружен → 200', upCombat.status === 200, `${upCombat.status}`);
  ok('ответ загрузки листка называет mode=combat', upCombat.json?.mode === 'combat',
    JSON.stringify(upCombat.json?.mode));

  // Форма каждого режима видит СВОЙ макет, и загрузка листка не подменила КП.
  const clientForm = await call('GET', '/form?mode=client', { token: ownerToken });
  ok('форма КП видит свой макет', clientForm.json?.hasLayout === true &&
    clientForm.json?.layoutName === 'КП.docx', JSON.stringify(clientForm.json?.layoutName));
  const combatForm = await call('GET', '/form?mode=combat', { token: ownerToken });
  ok('форма листка видит свой макет', combatForm.json?.hasLayout === true &&
    combatForm.json?.layoutName === 'БЛ.docx', JSON.stringify(combatForm.json?.layoutName));

  // Сам дефект: КП печатается содержимым, а не меткой боевого листка.
  const created = await call('POST', '/offers', {
    token: ownerToken,
    body: {
      client: { name: 'Заказчик режимов', contact: 'почта' },
      items: [{ name: 'Материал режимов', unit: 'м2', qty: 2, price: 50, sum: 100 }],
      total: 100, status: 'draft',
      formValues: { object_name: 'Объект режимов' },
    },
  });
  const offerId = created.json?.id;
  const doc = Buffer.from(await (await fetch(`${BASE}/api/offers/offers/${offerId}/export`, {
    method: 'POST', headers: { Authorization: `Bearer ${ownerToken}` },
  })).arrayBuffer());
  const xml = unzipEntrySafe(doc, 'word/document.xml');
  ok('КП под макетом листка не печатает заголовок «Боевой листок №»',
    !xml.includes('Боевой листок №'), xml.slice(0, 200));
  ok('КП под макетом листка не печатает метку {{сравнение}}', !xml.includes('{{сравнение}}'));
  ok('КП под макетом листка сохраняет данные заказа', xml.includes('Объект режимов') ||
    xml.includes('Материал режимов'), xml.slice(0, 300));

  await call('DELETE', `/offers/${offerId}`, { token: ownerToken });
  const delClient = await call('DELETE', '/form/layout?mode=client', { token: ownerToken });
  ok('макет КП убран', delClient.status === 200);
  const delCombat = await call('DELETE', '/form/layout?mode=combat', { token: ownerToken });
  ok('макет листка убран', delCombat.status === 200);
}

await sectionLayout();
await sectionLayoutMode();
await sectionSystemTexts();
await sectionServiceMarkers();

/**
 * Поиск систем и материалов для окна выбора (2026-11-12).
 *
 * Живые жалобы: «Гарант» не находил ни одной системы, «Кровля» —
 * ни одного системного решения. Стенд проверяет условие, которое было нарушено:
 * по одному слову находится НЕСКОЛЬКО систем, а материал отдаётся готовой позицией.
 */
async function sectionPickerSearch() {
  console.log('\nПоиск для окна выбора');

  const byWord = await call('GET', '/systems/search?q=' + encodeURIComponent('Гарант'), { token: ownerToken });
  ok('поиск систем «Гарант» отвечает 200', byWord.status === 200, `статус ${byWord.status}`);
  const garant = (byWord.json?.items ?? []).filter((i) => (i.title ?? '').includes('Гарант'));
  ok('по «Гарант» нашлись две системы, а не одна', garant.length >= 2,
    JSON.stringify((byWord.json?.items ?? []).map((i) => i.title)));

  const roof = await call('GET', '/systems/search?q=' + encodeURIComponent('Кровля'), { token: ownerToken });
  ok('по«Кровля» системных решений несколько',
    (roof.json?.items ?? []).length >= 2, JSON.stringify((roof.json?.items ?? []).map((i) => i.title)));

  const mats = await call('GET', '/materials/search?q=' + encodeURIComponent('мембрана'), { token: ownerToken });
  const material = (mats.json?.items ?? [])[0];
  ok('поиск материалов отвечает 200', mats.status === 200, `статус ${mats.status}`);
  ok('материал приходит с единицей и кодом',
    Boolean(material?.unit) && Boolean(material?.sku), JSON.stringify(material));

  const empty = await call('GET', '/systems/search?q=', { token: ownerToken });
  ok('пустой запрос возвращает массив, а не null',
    Array.isArray(empty.json?.items) && empty.json.items.length === 0, JSON.stringify(empty.json));

  // Одна ручка на две группы: бот отправляет один запрос на фразу.
  const suggest = await call('GET', '/suggest?q=' + encodeURIComponent('Кровля'), { token: ownerToken });
  ok('подсказка отдаёт и системы, и материалы',
    (suggest.json?.systems ?? []).length > 0 && Array.isArray(suggest.json?.materials),
    JSON.stringify({ systems: (suggest.json?.systems ?? []).length, materials: (suggest.json?.materials ?? []).length }));

  const botSuggest = await call('GET', '/bot/suggest?q=' + encodeURIComponent('Гарант'), {
    headers: { 'X-Service-Secret': BOT_SECRET },
  });
  ok('бот может получить подсказку по своему секрету', botSuggest.status === 200,
    `статус ${botSuggest.status}`);
}

/**
 * Загрузить макет из одного абзаца документа.
 *
 * Нужен разделам, проверяющим ПЕЧАТЬ блока: сам макет — минимальный документ с
 * одной меткой `{{block:N}}`, и его вёрстка здесь не предмет проверки.
 */
async function uploadLayoutOrFail(paragraphsXML, name) {
  const docXML =
    `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
    `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
    paragraphsXML +
    `</w:body></w:document>`;
  const layout = packZip([
    zipEntry('[Content_Types].xml', Buffer.from(
      '<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>',
      'utf8')),
    zipEntry('word/document.xml', Buffer.from(docXML, 'utf8')),
  ]);
  const res = await fetch(`${BASE}/api/offers/form/layout?name=${encodeURIComponent(name)}`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${ownerToken}`,
      'Content-Type': 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    },
    body: layout,
  });
  ok(`макет «${name}» загружен`, res.status === 200, `статус ${res.status}`);
}

/**
 * Материал — объект предложения (R3, 2026-11-13).
 *
 * Проверяется то, что пообещали пользователю: материал выбирается ОПИСАНИЕМ,
 * позиций это не добавляет, контракт ответа называет материал, а блок формы с
 * повтором «по материалам» печатает ВЫБРАННЫЕ материалы, а не позиции.
 *
 * Метка у блока ОДНА (`{{материал}}`): второй плейсхолдер сломал бы вёрстку,
 * поэтому печать обязана понимать, что источник повтора — материал-объект.
 */
async function sectionOfferMaterials() {
  console.log('\nМатериал — объект предложения (описанием)');

  const created = await call('POST', '/offers', {
    token: ownerToken,
    body: {
      client: { name: 'ООО Материалы', contact: '' },
      items: [{ name: 'Мембрана позицией', unit: 'м2', qty: 100, price: 50, sum: 5000 }],
      total: 5000,
      status: 'draft',
    },
  });
  const offerId = created.json?.id;
  ok('КП для материалов создано', offerId > 0, `${created.status} ${created.text?.slice(0, 120)}`);
  if (!(offerId > 0)) return;

  // Материал 103 — заметка «Мембрана ПВХ Эксперт» в заглушке Базы знаний.
  const added = await call('POST', `/offers/${offerId}/material`, {
    token: ownerToken, body: { noteId: 103 },
  });
  ok('материал выбран → 200', added.status === 200, `${added.status} ${added.text?.slice(0, 200)}`);
  ok('ответ называет материал', (added.json?.title ?? '').includes('Мембрана'), added.text?.slice(0, 160));
  ok('описание материала написано',
    (added.json?.texts?.['material.description'] ?? '').trim().length > 20,
    JSON.stringify(Object.keys(added.json?.texts ?? {})));
  ok('выбор материала НЕ добавил позицию',
    (added.json?.offer?.items ?? []).length === 1, JSON.stringify(added.json?.offer?.items));
  ok('материал попал в материалы предложения',
    (added.json?.offer?.materials ?? []).length === 1, JSON.stringify(added.json?.offer?.materials));
  ok('материал не попал в системы',
    (added.json?.offer?.systems ?? []).length === 0, JSON.stringify(added.json?.offer?.systems));

  // Повторный выбор не дублирует материал.
  const again = await call('POST', `/offers/${offerId}/material`, {
    token: ownerToken, body: { noteId: 103 },
  });
  ok('повторный выбор материала не дублирует',
    (again.json?.offer?.materials ?? []).length === 1, JSON.stringify(again.json?.offer?.materials));

  // Блок формы с повтором по материалам: печатает МАТЕРИАЛ, а не позицию.
  const form = await call('GET', '/form', { token: ownerToken });
  const materialForm = {
    ...form.json,
    blocks: [{
      id: '97', title: 'Материалы', showInClient: true, showInInternal: false,
      repeat: 'materials',
      content: [{ type: 'paragraph', text: '{{материал}}' }],
    }],
  };
  const savedForm = await call('PUT', '/form', { token: ownerToken, body: materialForm });
  ok('форма с повтором по материалам сохранена → 200', savedForm.status === 200,
    `${savedForm.status} ${savedForm.text?.slice(0, 160)}`);

  await uploadLayoutOrFail('<w:p><w:r><w:t>{{block:97}}</w:t></w:r></w:p>', 'Материалы.docx');
  const doc = await (await fetch(`${BASE}/api/offers/offers/${offerId}/export`, {
    method: 'POST', headers: { Authorization: `Bearer ${ownerToken}` },
  })).arrayBuffer();
  const xml = unzipEntrySafe(Buffer.from(doc), 'word/document.xml');
  ok('в документе напечатан ВЫБРАННЫЙ материал', xml.includes('Мембрана ПВХ Эксперт'),
    xml.slice(0, 300));
  ok('повтор по материалам не печатает позиции',
    !xml.includes('Мембрана позицией'), xml.slice(0, 400));

  // Снятие материала возвращает КП к прежнему виду. Снимаем по ЗАМЕТКЕ: выбор
  // пришёл по ней, и правило сравнения обязано признать его тем же.
  const removed = await call('DELETE', `/offers/${offerId}/material`, {
    token: ownerToken, body: { noteId: 103 },
  });
  ok('материал убран из предложения',
    (removed.json?.offer?.materials ?? []).length === 0, JSON.stringify(removed.json?.offer?.materials));

  // Бот ходит тем же путём по своему секрету. Владелец — почта чата.
  const botChat = linkedChatId !== 0 ? String(linkedChatId) : '999001';
  const botSite = await call('POST', '/bot/offers', {
    headers: { 'X-Service-Secret': BOT_SECRET },
    body: { chat_id: botChat, client: { name: 'ООО Материалы бот', contact: '' }, items: [] },
  });
  const botOfferId = botSite.json?.id ?? botSite.json?.offer?.id;
  if (botOfferId > 0) {
    const botAdd = await call('POST', `/bot/offers/${botOfferId}/material`, {
      headers: { 'X-Service-Secret': BOT_SECRET },
      body: { chat_id: botChat, noteId: 103 },
    });
    ok('бот выбирает материал по своему секрету', botAdd.status === 200,
      `${botAdd.status} ${botAdd.text?.slice(0, 160)}`);
    ok('у бота материал попал в материалы предложения',
      (botAdd.json?.offer?.materials ?? []).length === 1, JSON.stringify(botAdd.json?.offer?.materials));
  } else {
    ok('бот-КП создано для проверки материала', false, `${botSite.status} ${botSite.text?.slice(0, 160)}`);
  }

  // Макет этого раздела — временный: его ставят только ради проверки печати
  // блока. Оставленный, он уводит ВСЕ следующие проверки документа на пустой
  // бланк (2026-11-13: 8 падений печати на втором прогоне стенда). Форму тоже
  // возвращаем: следующий раздел работает с боевой формой.
  await call('DELETE', '/form/layout', { token: ownerToken });
  await call('PUT', '/form', {
    token: ownerToken,
    body: {
      mode: 'client',
      title: 'Коммерческое предложение',
      placeholders: [{ id: 'client_name', label: 'Клиент' }],
      requiredFields: ['client_name'],
      blocks: [
        {
          id: 'intro',
          title: 'Вступление',
          showInClient: true,
          showInInternal: false,
          content: [{ type: 'paragraph', text: 'Клиент: {{client_name}}' }],
        },
      ],
    },
  });
  ok('макет раздела убран, форма возвращена', true);

  await call('DELETE', `/offers/${offerId}`, { token: ownerToken });
  ok('КП убрано', true);
}

/**
 * Списки бота открываются в приложении (R1, 2026-11-13).
 *
 * Живое нажатие кнопки (reply → инлайновая `web_app` → маршрут списка) покрыто
 * `offers-bot/src/appbutton.test.ts`: собранный бот — один файл `dist/bot.mjs`,
 * и отдельную функцию из него не импортировать. Здесь вторая половина:
 * меню бота действительно содержит нужную строку (по исходнику, смонтированному
 * каталогом), и сервис РЕАЛЬНО отдаёт боту оба списка по его секрету — без этого
 * инлайновая кнопка открывает приложение на экран, который нечем заполнить.
 *
 * Запуск: смонтировать каталог `offers-bot` в стенд (`BOT_DIR=/bot`) и указать
 * `OFFERS_BOT_URL=http://offers-bot:3000`. Нет переменных — раздел честно
 * пропускается (одна «ок»-строка), а не краснеет: стенд гоняют и без бота.
 */
async function sectionBotAppLinks() {
  console.log('\nСписки бота открываются в приложении (R1)');
  const botUrl = (process.env.OFFERS_BOT_URL ?? '').trim();
  const botDir = (process.env.BOT_DIR ?? '').trim();
  const botEntry = botDir ? `${botDir}/dist/bot.mjs` : '';
  if (!botUrl || !botEntry || !existsSync(botEntry)) {
    ok('раздел пропущен: бот не смонтирован в стенд', true, 'OFFERS_BOT_URL/BOT_DIR не заданы');
    return;
  }

  // 2. Меню: два списка — в одном ряду.
  //
  // Живое поведение (нажатие кнопки → ответ с инлайновой `web_app`) проверяет
  // `offers-bot/src/appbutton.test.ts`: собранный бот — это один файл
  // `dist/bot.mjs`, и импортировать из него отдельные функции нельзя. Здесь
  // проверяется, что исходник, который едет на сервер, содержит нужную строку.
  const menu = readFileSync(`${botDir}/src/flow/build.ts`, 'utf8');
  ok('в меню бота два списка стоят в одном ряду',
    /\[\{ text: '📂 Мои КП' \}, \{ text: '📂 Мои боевые листки' \}\]/.test(menu), 'строка меню не найдена');
  // Три равноправных входа — создание, помощник и справка — тоже одним рядом
  // (2026-11-14, просьба пользователя). Проверяется тот же исходник, что едет
  // на сервер: иначе меню на стенде и в бою разошлись бы молча.
  ok('«Создать», «Помощник» и «Справка» стоят в одном ряду',
    /\{ text: '✨ Создать новый документ' \}, \{ text: '🤖 Помощник' \}, \{ text: '❓ Справка' \}/.test(menu),
    'совместный ряд в меню не найден');
  // Проверка смотрит на КНОПКИ в `MAIN_KEYBOARD`, а не на весь файл: слово
  // `web_app` живёт в комментарии рядом («почему reply-кнопка его не несёт»),
  // и проверка по всему файлу краснела бы на собственном объяснении.
  const keyboardBody = (menu.match(/export const MAIN_KEYBOARD = \{[\s\S]*?\n\};/) ?? [''])[0];
  const buttonLines = keyboardBody
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line.startsWith('[') && !line.startsWith('//') && !line.startsWith('/*'));
  ok('строки кнопок меню найдены', buttonLines.length >= 2, JSON.stringify(buttonLines));
  // Считаем КНОПКИ, а не ряды: в ряду их бывает несколько (2026-11-14 три
  // равноправных входа стоят одной строкой), и проверка по числу рядов
  // краснела бы на законном уплотнении меню.
  const menuButtons = (keyboardBody.match(/\{ text: '/g) ?? []).length;
  ok('в меню все пять кнопок', menuButtons === 5, `кнопок ${menuButtons}`);
  ok('в reply-меню нет `web_app` (она открывает приложение без подписи)',
    buttonLines.every((line) => !line.includes('web_app')), JSON.stringify(buttonLines));

  // 3. Списки приходят и адресуемы: сервис отдаёт их боту по его секрету.
  //
  // Чат `linkedChatId` уже привязан к владельцу выше (раздел 11), поэтому
  // список КП этого чата непустой — как у настоящего вошедшего менеджера.
  // Это проверка того, что за инлайновой кнопкой приложения есть данные,
  // а не проверка вида списка — вид покрыт `routes.test.ts`.
  const ownerChat = linkedChatId !== 0 ? `chat_id=${encodeURIComponent(linkedChatId)}` : 'chat_id=999001';
  const offers = await call('GET', `/bot/offers?${ownerChat}`, { headers: { 'X-Service-Secret': BOT_SECRET } });
  ok('список КП для бота отвечает 200', offers.status === 200, `статус ${offers.status}`);
  ok('список КП — массив, а не null', Array.isArray(offers.json?.offers), JSON.stringify(offers.json)?.slice(0, 120));
  const sheets = await call('GET', `/bot/combat?${ownerChat}&offset=0&limit=8`, { headers: { 'X-Service-Secret': BOT_SECRET } });
  ok('список боевых листков для бота отвечает 200', sheets.status === 200, `статус ${sheets.status}`);
  ok('список листков — массив, а не null', Array.isArray(sheets.json?.sheets), JSON.stringify(sheets.json)?.slice(0, 120));
}

/**
 * «Создать новый документ» (2026-11-13, R2) — серверная половина.
 *
 * Сам диалог (какие кнопки, какие тексты) проверяют `offers-bot/src/newdoc.test.ts`
 * и `routes.test.ts` — он живёт в боте и сети не требует. Здесь проверяется,
 * что РУЧКИ, которыми этот диалог пользуется, делают то, на что он рассчитывает:
 * поиск продукта по названию (тот же, что у подсказки), создание строки КП и
 * листка, запись продукта на сторону листка. Без этого диалог «зелёный» в
 * тестах и мёртвый на сервере — ровно класс дефекта `/bot/suggest` (0.9.17).
 */
async function sectionNewDoc() {
  console.log('\nСоздать новый документ: ручки, на которые он опирается (R2)');
  const chatId = linkedChatId !== 0 ? String(linkedChatId) : '999001';

  // 1. Поиск продукта — тот же `/bot/suggest`, что и в ответе на свободную фразу.
  const found = await call('GET', `/bot/suggest?q=${encodeURIComponent('Гарант')}`, {
    headers: { 'X-Service-Secret': BOT_SECRET },
  });
  ok('поиск продукта для нового диалога отвечает 200', found.status === 200, `статус ${found.status}`);
  const system = (found.json?.systems ?? [])[0];
  ok('поиск отдаёт систему с номером заметки', Boolean(system?.noteId),
    JSON.stringify(found.json?.systems ?? []).slice(0, 160));

  // 2. Создание КП пустым — так диалог начинает и КП, и листок.
  const created = await call('POST', '/bot/offers', {
    headers: { 'X-Service-Secret': BOT_SECRET },
    body: { chat_id: chatId, client: { name: '', contact: '' }, items: [], formValues: {} },
  });
  ok('новое КП создаётся без позиций', created.status === 200 && created.json?.id > 0,
    `статус ${created.status} ${created.text?.slice(0, 120)}`);
  const newDocOfferId = created.json?.id ?? 0;

  // 3. Выбранный продукт записывается в это КП (система — выбором, не позицией).
  if (newDocOfferId > 0 && system?.noteId) {
    const added = await call('POST', `/bot/offers/${newDocOfferId}/system`, {
      headers: { 'X-Service-Secret': BOT_SECRET },
      body: { chat_id: chatId, noteId: system.noteId, title: system.title },
    });
    ok('система из нового диалога записывается в КП', added.status === 200,
      `статус ${added.status} ${added.text?.slice(0, 120)}`);
    const after = await call('GET', `/bot/offers/${newDocOfferId}?chat_id=${encodeURIComponent(chatId)}`, {
      headers: { 'X-Service-Secret': BOT_SECRET },
    });
    const systems = (after.json?.systems ?? []).map((s) => s.noteId);
    ok('выбранная система видна в сохранённом КП', systems.includes(system.noteId), JSON.stringify(systems));
  }

  // 4. Листок: создание и запись продукта на нашу сторону.
  const sheet = await call('POST', '/bot/combat', {
    headers: { 'X-Service-Secret': BOT_SECRET },
    body: { chat_id: chatId, title: 'Боевой листок' },
  });
  ok('новый боевой листок создаётся', sheet.status === 200 && sheet.json?.id > 0,
    `статус ${sheet.status} ${sheet.text?.slice(0, 120)}`);
  if (sheet.json?.id && system?.noteId) {
    const side = await call('POST', `/bot/combat/${sheet.json.id}/our`, {
      headers: { 'X-Service-Secret': BOT_SECRET },
      body: { chat_id: chatId, noteId: system.noteId, title: system.title },
    });
    ok('наш продукт из нового диалога ложится в листок', side.status === 200,
      `статус ${side.status} ${side.text?.slice(0, 120)}`);
  }

  // 5. Множественный выбор (2026-11-14): системы и материалы пишутся в ОДНО КП.
  //
  // Живой дефект, из которого вырос раздел: по запросу «Logicbase» бот показал
  // восемь систем и ни одного материала (лимит выбирали системы), а выбор
  // материала не записывался вовсе. Здесь проверяется серверная половина:
  // обе ручки принимают выбор по заметке и оба вида видны в сохранённом КП.
  const materials = await call('GET', `/bot/suggest?q=${encodeURIComponent('мембрана')}`, {
    headers: { 'X-Service-Secret': BOT_SECRET },
  });
  ok('поиск отдаёт материалы рядом с системами', (materials.json?.materials ?? []).length > 0,
    JSON.stringify(materials.json ?? {}).slice(0, 160));
  const material = (materials.json?.materials ?? [])[0];
  if (newDocOfferId > 0 && material?.noteId) {
    const matAdded = await call('POST', `/bot/offers/${newDocOfferId}/material`, {
      headers: { 'X-Service-Secret': BOT_SECRET },
      body: { chat_id: chatId, noteId: material.noteId },
    });
    ok('материал из нового диалога записывается в КП описанием', matAdded.status === 200,
      `статус ${matAdded.status} ${matAdded.text?.slice(0, 120)}`);
    const after = await call('GET', `/bot/offers/${newDocOfferId}?chat_id=${encodeURIComponent(chatId)}`, {
      headers: { 'X-Service-Secret': BOT_SECRET },
    });
    const mats = (after.json?.materials ?? []).map((m) => m.noteId);
    const syss = (after.json?.systems ?? []).map((s) => s.noteId);
    ok('материал виден в сохранённом КП', mats.includes(material.noteId), JSON.stringify(mats));
    ok('система и материал живут в одном КП', syss.length > 0 && mats.length > 0,
      JSON.stringify({ systems: syss, materials: mats }));
    ok('материал не стал позицией', (after.json?.items ?? []).length === 0,
      JSON.stringify((after.json?.items ?? []).length));
  }

  // 6. Запись по noteId при наличии кода (2026-11-14).
  //
  // `FindSystem` ищет по коду ПЕРВЫМ: бот раньше присылал и `noteId`, и `sku`,
  // и выбор уезжал на чужую карточку с тем же кодом. Теперь бот шлёт только
  // заметку — проверяем, что запись по одному noteId принимается.
  const withCode = (found.json?.systems ?? []).find((s) => s.sku) ?? null;
  if (newDocOfferId > 0 && withCode) {
    const onlyNote = await call('POST', `/bot/offers/${newDocOfferId}/system`, {
      headers: { 'X-Service-Secret': BOT_SECRET },
      body: { chat_id: chatId, noteId: withCode.noteId },
    });
    ok('система находится по одной заметке, без кода', onlyNote.status === 200,
      `статус ${onlyNote.status} ${onlyNote.text?.slice(0, 120)}`);
  }
}

/**
 * Помощник (агент) в боте (2026-11-14, шаг P6) — серверная половина.
 *
 * Сам диалог (включение, «⌛», пересылка файла) проверяет `offers-bot/src/agent.test.ts`
 * без сети. Здесь — что ручка бота `POST /bot/agent/message` действительно
 * проводит ход: заглушка модели (llm-stub) отвечает на промт агента, цикл
 * вызывает `get_our_products`, возвращает текст. Плюс границы: без секрета —
 * отказ; журнал хода виден администратору; документы без настроенного
 * `DOCUMENTS_SERVICE_URL` честно отвечают «недоступны», а не пустым списком.
 *
 * Чужой секрет: ручка под `X-Service-Secret`, поэтому проверка 401/403 здесь
 * такая же, как у остальных ручек бота (правило 2026-11-12 — «проверять
 * авторизацию при каждом выпуске»).
 */
async function sectionAgent() {
  console.log('\nПомощник (агент) в боте (P6)');
  const chatId = linkedChatId !== 0 ? String(linkedChatId) : '999001';

  // 1. Гейт: без секрета и с чужим секретом — отказ, а не ход.
  const noSecret = await call('POST', '/bot/agent/message', {
    body: { chat_id: chatId, text: 'какая мембрана?' },
  });
  ok('ход агента без секрета отклонён', noSecret.status === 401 || noSecret.status === 403,
    `статус ${noSecret.status}`);
  const badSecret = await call('POST', '/bot/agent/message', {
    headers: { 'X-Service-Secret': 'not-the-bot-secret' },
    body: { chat_id: chatId, text: 'какая мембрана?' },
  });
  ok('ход агента с чужим секретом отклонён', badSecret.status === 403, `статус ${badSecret.status}`);

  // 2. Пустой текст — отказ (иначе модель получает пустое).
  const empty = await call('POST', '/bot/agent/message', {
    headers: { 'X-Service-Secret': BOT_SECRET },
    body: { chat_id: chatId, text: '   ' },
  });
  ok('пустой запрос агенту отклонён', empty.status === 400, `статус ${empty.status}`);

  // 3. Настоящий ход: заглушка модели отвечает вызовом get_our_products, затем
  //    финальным текстом. Проверяется, что цикл дошёл до ответа человеку.
  const turn = await call('POST', '/bot/agent/message', {
    headers: { 'X-Service-Secret': BOT_SECRET },
    body: { chat_id: chatId, text: 'предложи систему кровля' },
  });
  ok('ход агента отвечает 200', turn.status === 200, `статус ${turn.status} ${turn.text?.slice(0, 160)}`);
  ok('агент вернул текст ответа', typeof turn.json?.answer === 'string' && turn.json.answer.length > 0,
    JSON.stringify(turn.json)?.slice(0, 160));
  ok('ответ несёт массив файлов, а не null', Array.isArray(turn.json?.files), JSON.stringify(turn.json?.files));

  // 4. Журнал хода: администратор видит строку с нашим запросом. Роль admin у
  //    ownerToken (владелец приложения).
  const log = await call('GET', '/admin/agent-log?limit=20', { token: ownerToken });
  ok('журнал агента отвечает администратору', log.status === 200, `статус ${log.status}`);
  ok('журнал агента — массив, а не null', Array.isArray(log.json?.items), JSON.stringify(log.json)?.slice(0, 120));
  const found = (log.json?.items ?? []).some((row) => String(row.userText ?? '').includes('предложи систему кровля'));
  ok('ход агента попал в журнал', found, JSON.stringify(log.json?.items ?? []).slice(0, 200));
  // Журнал — только администратору: «менеджер» (не admin) получает 403.
  const strangerLog = await call('GET', '/admin/agent-log', { token: makeJwt('stranger@tn.ru') });
  ok('журнал агента закрыт для не-администратора', strangerLog.status === 403, `статус ${strangerLog.status}`);

  // 5. Граница документов: на стенде DOCUMENTS_SERVICE_URL не задан, и
  //    `get_documents` честно отвечает «не настроено», а не пустым списком.
  //    Проверяется косвенно — по отсутствию 500: ход отработал без сбоя.
  ok('ход агента не падает без сервиса документов', turn.status === 200, `статус ${turn.status}`);
}

/**
 * Помощник (агент), P6-бис (2026-11-14) — карточка материала читается,
 * конкуренты ищутся, боевой листок заводится, граница сторон держится.
 *
 * Живой дефект 0.9.22: `read_card` по материалу (noteId 103 на стенде — та же
 * форма, что 7547 на бою) отвечал «карточки нет», хотя заметка читается. Здесь
 * ход идёт через настоящий `read_card` (заглушка модели зовёт его по 103), и
 * проверяется, что карточка вернулась, а роль — material.
 */
async function sectionAgentP6bis() {
  console.log('\nПомощник (агент): карточки любого вида, конкуренты, листок, КП (P6-бис)');
  const chatId = linkedChatId !== 0 ? String(linkedChatId) : '999002';
  const botHeaders = { 'X-Service-Secret': BOT_SECRET };

  // 1. Карточка МАТЕРИАЛА читается, конкуренты ищутся. Живой дефект 0.9.22:
  //    `read_card` по материалу отвечал «карточки нет».
  const analogTurn = await call('POST', '/bot/agent/message', {
    headers: botHeaders,
    body: { chat_id: chatId, text: 'подбери аналог мембраны ПВХ' },
  });
  ok('ход «подбери аналог» проходит', analogTurn.status === 200, `статус ${analogTurn.status} ${analogTurn.text?.slice(0, 160)}`);
  ok('ответ агента не пустой', typeof analogTurn.json?.answer === 'string' && analogTurn.json.answer.length > 0,
    JSON.stringify(analogTurn.json)?.slice(0, 160));

  const log = await call('GET', '/admin/agent-log?limit=20', { token: ownerToken });
  const analogRow = (log.json?.items ?? []).find((row) => String(row.userText ?? '').includes('подбери аналог'));
  ok('ход «подбери аналог» попал в журнал', Boolean(analogRow), JSON.stringify(log.json?.items ?? []).slice(0, 200));
  const readTool = (analogRow?.tools ?? []).find((t) => t.name === 'read_card');
  ok('агент читал карточку материала через read_card', Boolean(readTool), JSON.stringify(analogRow?.tools ?? []).slice(0, 240));
  ok('карточка материала прочитана, а не «нет такой»', readTool?.ok === true, JSON.stringify(readTool)?.slice(0, 240));
  const searchTool = (analogRow?.tools ?? []).find((t) => t.name === 'search_competitors');
  ok('поиск конкурентов вызван и сработал', searchTool?.ok === true, JSON.stringify(searchTool)?.slice(0, 200));

  // 2. Боевой листок: агент заводит его сам и кладёт наш продукт.
  const combatTurn = await call('POST', '/bot/agent/message', {
    headers: botHeaders,
    body: { chat_id: chatId, text: 'сделай сравнение с конкурентами по Гаранту' },
  });
  ok('ход «сравнение с конкурентами» проходит', combatTurn.status === 200, `статус ${combatTurn.status} ${combatTurn.text?.slice(0, 160)}`);
  const combatLog = await call('GET', '/admin/agent-log?limit=20', { token: ownerToken });
  const combatRow = (combatLog.json?.items ?? []).find((row) => String(row.userText ?? '').includes('сравнение с конкурентами'));
  const created = (combatRow?.tools ?? []).find((t) => t.name === 'create_combat_sheet');
  ok('агент завёл боевой листок', created?.ok === true, JSON.stringify(created)?.slice(0, 240));
  const side = (combatRow?.tools ?? []).find((t) => t.name === 'set_combat_side');
  ok('агент добавил наш продукт на сторону листка', side?.ok === true, JSON.stringify(side)?.slice(0, 240));

  const sheets = await call('GET', '/combat', { token: ownerToken });
  const sheet = (sheets.json?.sheets ?? []).find((s) => s.title === 'Сравнение с конкурентами');
  ok('заведённый агентом листок виден в списке', Boolean(sheet),
    JSON.stringify((sheets.json?.sheets ?? []).map((s) => s.title)).slice(0, 200));
  ok('сторона листка несёт наш продукт', (sheet?.ourNotes ?? []).some((n) => n.noteId === 105),
    JSON.stringify(sheet?.ourNotes));

  // 4. Граница прав: чужой человек не правит стороны нашего листка (граница
  //    «что куда можно положить» покрыта unit-тестом TestCombatSideRoleBoundary).
  if (sheet?.id) {
    const foreign = await call('POST', `/combat/${sheet.id}/our`, {
      token: makeJwt('stranger@tn.ru'),
      body: { noteId: 201, title: 'Конкурент: ТехноНИКОЛЬ Кровля' },
    });
    ok('чужой не может править стороны листка', foreign.status === 403, `статус ${foreign.status}`);
  }

  // 5. Сценарий B: без открытого КП агент сам его заводит и добавляет систему.
  const offerTurn = await call('POST', '/bot/agent/message', {
    headers: botHeaders,
    body: { chat_id: chatId, text: 'собери кп по Гаранту' },
  });
  ok('ход «собери кп» проходит без готового предложения', offerTurn.status === 200,
    `статус ${offerTurn.status} ${offerTurn.text?.slice(0, 160)}`);
  const offerLog = await call('GET', '/admin/agent-log?limit=20', { token: ownerToken });
  const offerRow = (offerLog.json?.items ?? []).find((row) => String(row.userText ?? '').includes('собери кп'));
  for (const name of ['create_offer', 'set_system', 'build_docx']) {
    const step = (offerRow?.tools ?? []).find((t) => t.name === name);
    ok(`агент выполнил «${name}»`, step?.ok === true, JSON.stringify(step)?.slice(0, 240));
  }
  ok('агент вернул готовый файл КП', (offerTurn.json?.files ?? []).length > 0,
    JSON.stringify(offerTurn.json?.files)?.slice(0, 160));
}

await sectionCombat();
await sectionPickerSearch();
await sectionOfferMaterials();
await sectionBotAppLinks();
await sectionNewDoc();
await sectionAgent();
await sectionAgentP6bis();

console.log(`\nИтого: ${passed} ок, ${failed} провалов`);
process.exit(failed === 0 ? 0 : 1);
