// Подставной agent-service для стенда sbe-offers (2026-11-16, шаг 1 плана
// 2026-11-16-sbe-offers-combat-sections-and-agent-tools-plan.md).
//
// Контракт ЗАМОРОЖЕН и совпадает с настоящими ручками sbe-agent
// (`/internal/agent/{fetch,file/parse,render}` под X-Service-Secret):
//
//	POST /internal/agent/fetch      {method,url,body,headers,timeout_ms}
//	                                → {status, content_type, text}
//	POST /internal/agent/file/parse multipart `file`
//	                                → {kind, text} | {kind, sheets} | {kind, data}
//	POST /internal/agent/render     {lang:"mermaid", code}
//	                                → {format:"png", base64, width, height}
//
// Почему заглушка вообще нужна. Стенд sbe-offers крутится отдельным docker-
// составом и настоящим agent-service не пользуется; без заглушки проверки
// «агент читает ссылку и файл» жили бы только на бою. Ответы здесь не
// угадываются: HTML/JSON/текст — ровно те формы, что отдаёт фетч, а PNG — тот
// же минимальный 1×1, что модель не переспорит.
//
// Секрет сверяется как у настоящего сервиса: пустой заголовок или чужой —
// 403; пустой env — 503 (сервис «не настроен»). Так стенд ловит и ошибку
// клиента, и ошибку конфигурации состава.
import http from 'node:http';

const PORT = Number(process.env.AGENT_STUB_PORT ?? 3000);
const SECRET = process.env.AGENT_INTERNAL_SECRET ?? '';

/** Настоящий PNG 1×1: клиент разбирает размеры из заголовка PNG. */
const PNG_1X1 = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
  'base64',
);

function send(res, status, body, contentType = 'application/json; charset=utf-8') {
  res.writeHead(status, { 'Content-Type': contentType });
  res.end(typeof body === 'string' ? body : JSON.stringify(body));
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    req.on('data', (chunk) => chunks.push(chunk));
    req.on('end', () => resolve(Buffer.concat(chunks)));
    req.on('error', reject);
  });
}

/** Разбор multipart без зависимостей: имя файла из Content-Disposition. */
function fileNameOf(buffer, boundary) {
  const text = buffer.toString('latin1');
  const marker = `--${boundary}`;
  const start = text.indexOf(marker);
  if (start < 0) return '';
  const headerEnd = text.indexOf('\r\n\r\n', start);
  if (headerEnd < 0) return '';
  const head = text.slice(start, headerEnd);
  const match = /filename="([^"]*)"/i.exec(head);
  return match ? match[1] : '';
}

function requireSecret(req, res) {
  if (SECRET === '') {
    send(res, 503, { error: 'AGENT_INTERNAL_SECRET is not set' });
    return false;
  }
  if (req.headers['x-service-secret'] !== SECRET) {
    send(res, 403, { error: 'bad service secret' });
    return false;
  }
  return true;
}

async function handleFetch(req, res) {
  const raw = await readBody(req);
  let payload;
  try {
    payload = JSON.parse(raw.toString('utf8'));
  } catch {
    send(res, 400, { error: 'invalid json' });
    return;
  }
  const url = String(payload.url ?? '');
  if (!/^https?:\/\//i.test(url)) {
    send(res, 400, { error: 'invalid url (http/https)' });
    return;
  }
  if (/^https?:\/\/(localhost|127\.0\.0\.1|\[::1\])/i.test(url)) {
    send(res, 400, { error: 'локальные адреса недоступны' });
    return;
  }
  if (/\.json(\?|$)/i.test(url)) {
    send(res, 200, {
      status: 200,
      content_type: 'application/json',
      text: JSON.stringify({ recordsTotal: 2, recordsFiltered: 2, data: [{ id: 1 }, { id: 2 }] }),
    });
    return;
  }
  if (/\.txt$/i.test(url)) {
    send(res, 200, { status: 200, content_type: 'text/plain; charset=utf-8', text: 'Протокол испытаний: 25 МПа.' });
    return;
  }
  send(res, 200, {
    status: 200,
    content_type: 'text/html; charset=utf-8',
    text:
      '<!DOCTYPE html><html><head><script src="/js/reestr.js"></script></head>' +
      '<body><a href="/ajax/datatables/reestr.php">Реестр</a><p>Таблица сертификатов</p></body></html>',
  });
}

async function handleParse(req, res) {
  const raw = await readBody(req);
  const contentType = String(req.headers['content-type'] ?? '');
  const boundary = /boundary=(?:"([^"]+)"|([^;]+))/i.exec(contentType);
  if (!boundary) {
    send(res, 400, { error: 'invalid multipart' });
    return;
  }
  const fileName = fileNameOf(raw, boundary[1] ?? boundary[2]);
  const ext = (fileName.split('.').pop() ?? '').toLowerCase();
  switch (ext) {
    case 'docx':
    case 'pdf':
      send(res, 200, { kind: ext, text: 'Протокол испытаний кровельной мембраны: прочность 25 МПа.' });
      return;
    case 'xlsx':
      send(res, 200, {
        kind: ext,
        sheets: [{ name: 'Состав', headers: ['№', 'Материал'], rows: [['1', 'Мембрана ПВХ'], ['2', 'Утеплитель']] }],
      });
      return;
    case 'json':
      send(res, 200, { kind: ext, data: { ok: true } });
      return;
    default:
      send(res, 400, { error: `unsupported format: ${ext}` });
  }
}

async function handleRender(req, res) {
  const raw = await readBody(req);
  let payload;
  try {
    payload = JSON.parse(raw.toString('utf8'));
  } catch {
    send(res, 400, { error: 'invalid json' });
    return;
  }
  const lang = String(payload.lang ?? '').toLowerCase();
  if (lang !== 'mermaid') {
    send(res, 400, { error: 'supported lang: mermaid (произвольный SVG не поддерживается)' });
    return;
  }
  if (!String(payload.code ?? '').trim()) {
    send(res, 400, { error: 'code is required' });
    return;
  }
  // Настоящий рендер — mmdc в контейнере agent-mermaid; стенду нужен только
  // формат ответа, поэтому PNG минимальный, а размеры честные (1×1 из файла).
  send(res, 200, { format: 'png', base64: PNG_1X1.toString('base64'), width: 1, height: 1 });
}

http
  .createServer(async (req, res) => {
    const path = new URL(req.url, 'http://stub').pathname;
    try {
      if (req.method !== 'POST' || !path.startsWith('/internal/agent/')) {
        send(res, 404, { error: 'not found' });
        return;
      }
      if (!requireSecret(req, res)) return;
      if (path === '/internal/agent/fetch') return await handleFetch(req, res);
      if (path === '/internal/agent/file/parse') return await handleParse(req, res);
      if (path === '/internal/agent/render') return await handleRender(req, res);
      send(res, 404, { error: 'not found' });
    } catch (e) {
      send(res, 500, { error: String(e && e.message ? e.message : e) });
    }
  })
  .listen(PORT, () => {
    console.log(`agent-stub: слушает :${PORT} (секрет ${SECRET ? 'задан' : 'НЕ задан'})`);
  });