/**
 * Маленький HTTP-сервер проверки токена (2026-10-29). Нужен потому, что путь в
 * Telegram есть только у бота (SOCKS-туннель к NL), а сохранение токена
 * принимает offers-service. Порядок: плагин → offers-service (права admin) →
 * сюда (X-Service-Secret) → getMe в Telegram → ответ.
 *
 * Наружу через Caddy маршрута нет: сервер живёт только во внутренней
 * docker-сети.
 */
import http from 'node:http';
import { sendDocument, verifyToken } from './telegram.ts';
import { runAsk } from './flow/askrun.ts';
import type { BotDeps } from './routes.ts';

const PORT = Number(process.env.VERIFY_PORT ?? 3000);
const SECRET = process.env.OFFERS_BOT_SECRET ?? '';
const MAX_BODY = 8 << 10;
/** Файл с сервиса приходит base64: документ DOCX до 20 МБ плюс обёртка. */
const MAX_ANSWER_BODY = 32 << 20;

function send(res: http.ServerResponse, status: number, payload: Record<string, unknown>): void {
  res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8' });
  res.end(JSON.stringify(payload));
}

/** Прочитать тело запроса с ограничением размера. null — слишком большое. */
function readBody(req: http.IncomingMessage): Promise<string | null> {
  return readBodyWithLimit(req, MAX_BODY);
}

/** То же чтение тела, но со своим пределом: у файла он больше, чем у токена. */
function readBodyWithLimit(req: http.IncomingMessage, limit: number): Promise<string | null> {
  return new Promise((resolve) => {
    const chunks: Buffer[] = [];
    let size = 0;
    req.on('data', (chunk: Buffer) => {
      size += chunk.length;
      if (size > limit) {
        resolve(null);
        req.destroy();
        return;
      }
      chunks.push(chunk);
    });
    req.on('end', () => resolve(Buffer.concat(chunks).toString('utf8')));
  });
}

async function handleVerify(req: http.IncomingMessage, res: http.ServerResponse): Promise<void> {
  if (!SECRET || req.headers['x-service-secret'] !== SECRET) {
    send(res, 403, { ok: false, error: 'forbidden' });
    return;
  }
  const body = await readBody(req);
  if (body === null) {
    send(res, 413, { ok: false, error: 'request too large' });
    return;
  }
  let token = '';
  try {
    const parsed = JSON.parse(body) as { token?: string };
    token = typeof parsed.token === 'string' ? parsed.token.trim() : '';
  } catch {
    send(res, 400, { ok: false, error: 'invalid json' });
    return;
  }
  if (!token) {
    send(res, 400, { ok: false, error: 'токен пуст' });
    return;
  }
  try {
    const username = await verifyToken(token);
    send(res, 200, { ok: true, username });
  } catch (e) {
    // Ошибку Telegram отдаём как есть: администратору важно увидеть
    // «Unauthorized», а не общее «не удалось».
    send(res, 400, { ok: false, error: (e as Error).message });
  }
}

/**
 * Отправка файла в чат по просьбе Mini App (2026-11-14).
 *
 * Сервис собирает DOCX и присылает его сюда секретом, потому что только у бота
 * есть путь в Telegram (СОКС-туннель). Бот ничего не решает про права: документ уже
 * проверен там, а `query_id` здесь только передаётся.
 */
async function handleAnswer(req: http.IncomingMessage, res: http.ServerResponse): Promise<void> {
  if (!SECRET || req.headers['x-service-secret'] !== SECRET) {
    send(res, 403, { ok: false, error: 'forbidden' });
    return;
  }
  const body = await readBodyWithLimit(req, MAX_ANSWER_BODY);
  if (body === null) {
    send(res, 413, { ok: false, error: 'request too large' });
    return;
  }
  let parsed: { chat_id?: unknown; file_name?: unknown; caption?: unknown; content?: unknown };
  try {
    parsed = JSON.parse(body) as typeof parsed;
  } catch {
    send(res, 400, { ok: false, error: 'invalid json' });
    return;
  }
  // Адресат обязателен: сервис берёт его из подписи initData и пустым не шлёт.
  const chatId = typeof parsed.chat_id === 'string' ? parsed.chat_id.trim() : '';
  if (!/^-?[0-9]+$/.test(chatId)) {
    send(res, 400, { ok: false, error: 'нет chat_id' });
    return;
  }
  if (typeof parsed.content !== 'string' || parsed.content === '') {
    send(res, 400, { ok: false, error: 'нет файла' });
    return;
  }
  const fileName = typeof parsed.file_name === 'string' && parsed.file_name !== '' ? parsed.file_name : 'документ.docx';
  const caption = typeof parsed.caption === 'string' ? parsed.caption : '';
  try {
    // Ошибку Telegram отдаём текстом: человеку важна причина, а не «не удалось».
    await sendDocument(chatId, fileName, Buffer.from(parsed.content, 'base64'), caption);
    send(res, 200, { ok: true });
  } catch (e) {
    send(res, 502, { ok: false, error: (e as Error).message });
  }
}

/**
 * Просьба из приложения «продолжить в боте» (2026-11-15).
 *
 * Сервис проверяет подпись Telegram и присылает сюда `action`, `offer_id` и
 * `chat_id`, взятый ИЗ ПОДПИСИ. Бот открывает нужный диалог через
 * существующие обработчики (`runAsk`), поэтому второй путь к брифу или к
 * разбору позиций не заводится.
 *
 * Поллеру нужны `BotDeps` (сессии, отправка, восстановление входа), а они
 * живут в поллере: он отдаёт свои через `setAskDeps`. Не пришли (поллер не
 * стартовал) — честный отказ, а не молчание.
 */
let askDeps: BotDeps | null = null;

/** Подключить зависимости поллера: без них просьбы из приложения не работают. */
export function setAskDeps(deps: BotDeps): void {
  askDeps = deps;
}

async function handleAsk(req: http.IncomingMessage, res: http.ServerResponse): Promise<void> {
  if (!SECRET || req.headers['x-service-secret'] !== SECRET) {
    send(res, 403, { ok: false, error: 'forbidden' });
    return;
  }
  const body = await readBody(req);
  if (body === null) {
    send(res, 413, { ok: false, error: 'request too large' });
    return;
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(body);
  } catch {
    send(res, 400, { ok: false, error: 'invalid json' });
    return;
  }
  if (askDeps === null) {
    send(res, 503, { ok: false, error: 'бот ещё не готов: попробуйте через минуту' });
    return;
  }
  try {
    const result = await runAsk(askDeps, parsed);
    send(res, 200, { ok: true, sent: result.sent });
  } catch (e) {
    // Ошибку отдаём текстом: приложение показывает её человеку.
    send(res, 502, { ok: false, error: (e as Error).message });
  }
}

/**
 * Сообщение в чат без файла (2026-11-17).
 *
 * Асинхронная отправка из приложения идёт в фоне: если сборка или доставка
 * упала, человек уже закрыл окно, и узнает об этом только из чата. Сервис
 * присылает сюда короткий текст — бот отправляет его обычным сообщением через
 * депсы поллера (`askDeps.send`), тот же путь, что у остальных ответов.
 */
async function handleNotify(req: http.IncomingMessage, res: http.ServerResponse): Promise<void> {
  if (!SECRET || req.headers['x-service-secret'] !== SECRET) {
    send(res, 403, { ok: false, error: 'forbidden' });
    return;
  }
  const body = await readBody(req);
  if (body === null) {
    send(res, 413, { ok: false, error: 'request too large' });
    return;
  }
  let parsed: { chat_id?: unknown; text?: unknown };
  try {
    parsed = JSON.parse(body) as { chat_id?: unknown; text?: unknown };
  } catch {
    send(res, 400, { ok: false, error: 'invalid json' });
    return;
  }
  const chatId = typeof parsed.chat_id === 'string' ? parsed.chat_id.trim() : '';
  const text = typeof parsed.text === 'string' ? parsed.text : '';
  if (!/^-?[0-9]+$/.test(chatId) || text.trim() === '') {
    send(res, 400, { ok: false, error: 'нужны chat_id и text' });
    return;
  }
  if (askDeps === null) {
    send(res, 503, { ok: false, error: 'бот ещё не готов' });
    return;
  }
  try {
    await askDeps.send(chatId, text);
    send(res, 200, { ok: true });
  } catch (e) {
    send(res, 502, { ok: false, error: (e as Error).message });
  }
}

export function startVerifyServer(): http.Server {
  const server = http.createServer((req, res) => {
    if (req.method === 'POST' && req.url === '/internal/answer') {
      void handleAnswer(req, res);
      return;
    }
    if (req.method === 'POST' && req.url === '/internal/ask') {
      void handleAsk(req, res);
      return;
    }
    if (req.method === 'POST' && req.url === '/internal/notify') {
      void handleNotify(req, res);
      return;
    }
    if (req.method !== 'POST' || req.url !== '/internal/verify') {
      send(res, 404, { ok: false, error: 'not found' });
      return;
    }
    void handleVerify(req, res);
  });
  server.listen(PORT, () => {
    console.log(`offers-bot: проверка токена слушает :${PORT}`);
  });
  return server;
}