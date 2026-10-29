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
import { verifyToken } from './telegram.ts';

const PORT = Number(process.env.VERIFY_PORT ?? 3000);
const SECRET = process.env.OFFERS_BOT_SECRET ?? '';
const MAX_BODY = 8 << 10;

function send(res: http.ServerResponse, status: number, payload: Record<string, unknown>): void {
  res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8' });
  res.end(JSON.stringify(payload));
}

/** Прочитать тело запроса с ограничением размера. null — слишком большое. */
function readBody(req: http.IncomingMessage): Promise<string | null> {
  return new Promise((resolve) => {
    const chunks: Buffer[] = [];
    let size = 0;
    req.on('data', (chunk: Buffer) => {
      size += chunk.length;
      if (size > MAX_BODY) {
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

export function startVerifyServer(): http.Server {
  const server = http.createServer((req, res) => {
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