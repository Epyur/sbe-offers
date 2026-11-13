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
import { answerWebAppDocument, verifyToken } from './telegram.ts';

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
 * Ответ на нажатие в Mini App (2026-11-14).
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
  let parsed: { query_id?: unknown; file_name?: unknown; content?: unknown };
  try {
    parsed = JSON.parse(body) as typeof parsed;
  } catch {
    send(res, 400, { ok: false, error: 'invalid json' });
    return;
  }
  if (typeof parsed.query_id !== 'string' || parsed.query_id === '') {
    send(res, 400, { ok: false, error: 'нет query_id' });
    return;
  }
  if (typeof parsed.content !== 'string' || parsed.content === '') {
    send(res, 400, { ok: false, error: 'нет файла' });
    return;
  }
  const fileName = typeof parsed.file_name === 'string' && parsed.file_name !== '' ? parsed.file_name : 'документ.docx';
  try {
    // Ошибку Telegram отдаём текстом: человеку важно увидеть «отправка уже была»,
    // а не общее «не удалось».
    await answerWebAppDocument(parsed.query_id, fileName, Buffer.from(parsed.content, 'base64'));
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