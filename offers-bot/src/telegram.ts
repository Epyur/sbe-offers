/**
 * Низкоуровневые примитивы Telegram Bot API — те же, что у portal-embed:
 * запросы идут через SOCKS5-туннель, потому что с VDS в РФ прямого пути к IP
 * Telegram нет (диапазоны AS62041 заблокированы на уровне хостера).
 */
import https from 'node:https';
import { SocksProxyAgent } from 'socks-proxy-agent';
import { API_URL } from './api.ts';

const SOCKS_PROXY_URL = process.env.TELEGRAM_SOCKS_PROXY ?? 'socks5://127.0.0.1:1080';
const TELEGRAM_API_BASE = 'https://api.telegram.org';
const socksAgent = new SocksProxyAgent(SOCKS_PROXY_URL);

// --- Токен бота живёт ТОЛЬКО в offers-service (таблица offers_bot_config): его
// вводит администратор из настроек плагина (2026-10-29). Переменной окружения
// TELEGRAM_BOT_TOKEN здесь больше нет и не будет: в соседнем portal-embed именно
// второй источник (.env рядом с базой) полтора суток слал уведомления чужим
// ботом, потому что при 403 сервер откатывался на значение из окружения.
// Не получили токен — это отказ, а не повод работать другим ботом.
// Кэш 15 с: смена токена в плагине действует почти сразу, без перезапуска
// контейнера, и при этом мы не дёргаем offers-service на каждый запрос. ---
const TOKEN_CACHE_TTL_MS = 15_000;
let tokenCache: { token: string; expiresAt: number } | null = null;

/** Забыть кэш токена — нужно проверке из настроек плагина: она должна видеть,
 *  что сохранённый токен сейчас другой, а не предыдущий. */
export function forgetCachedToken(): void {
  tokenCache = null;
}

/** Действующий токен бота. Бросает, если токен не задан или сервис недоступен:
 *  лучше видимая ошибка, чем молча чужой бот. */
export async function resolveBotToken(): Promise<string> {
  if (tokenCache && tokenCache.expiresAt > Date.now()) return tokenCache.token;
  const AUTH = process.env.OFFERS_BOT_SECRET ?? '';
  let res: Response;
  try {
    res = await fetch(`${API_URL}/api/offers/bot/token`, {
      headers: { 'X-Service-Secret': AUTH },
      signal: AbortSignal.timeout(10_000),
    });
  } catch (e) {
    throw new Error(`не удалось получить токен бота: сервис недоступен (${(e as Error).message})`);
  }
  if (!res.ok) {
    throw new Error(`не удалось получить токен бота: /api/offers/bot/token ответил ${res.status}`);
  }
  const data = (await res.json()) as { token?: string };
  const token = typeof data.token === 'string' ? data.token.trim() : '';
  if (!token) {
    throw new Error('токен бота не задан: администратор ещё не ввёл его в настройках плагина');
  }
  tokenCache = { token, expiresAt: Date.now() + TOKEN_CACHE_TTL_MS };
  return token;
}

/** Есть ли действующий токен — по нему решается, включать ли приём сообщений.
 *  Ошибку не бросает: отсутствие токена означает «бот выключен», а не поломку. */
export async function hasBotToken(): Promise<boolean> {
  try {
    await resolveBotToken();
    return true;
  } catch {
    return false;
  }
}

export interface TelegramApiResult<T> {
  ok: boolean;
  result?: T;
  description?: string;
}

export interface TelegramMessage {
  message_id: number;
  chat: { id: number };
  text?: string;
}

export interface TelegramCallbackQuery {
  id: string;
  data?: string;
  message?: TelegramMessage;
  from?: { id: number };
}

export interface TelegramUpdate {
  update_id: number;
  message?: TelegramMessage;
  callback_query?: TelegramCallbackQuery;
}

export async function telegramRequest<T>(
  method: string,
  body: Record<string, unknown>,
  timeoutMs = 15000,
): Promise<TelegramApiResult<T>> {
  const token = await resolveBotToken();
  return new Promise((resolve, reject) => {
    const payload = JSON.stringify(body);
    const req = https.request(
      `${TELEGRAM_API_BASE}/bot${token}/${method}`,
      {
        method: 'POST',
        agent: socksAgent,
        headers: { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(payload) },
      },
      (res) => {
        const chunks: Buffer[] = [];
        res.on('data', (c: Buffer) => chunks.push(c));
        res.on('end', () => {
          try {
            resolve(JSON.parse(Buffer.concat(chunks).toString('utf8')) as TelegramApiResult<T>);
          } catch (e) {
            reject(e);
          }
        });
      },
    );
    req.setTimeout(timeoutMs, () => req.destroy(new Error(`Telegram API не ответил за ${timeoutMs}мс`)));
    req.on('error', reject);
    req.write(payload);
    req.end();
  });
}

/** Предел файла для чата: у ботов Telegram лимит отправки ~50 МБ, но путь
 *  идёт через туннель — держимся заведомо безопасных 20 МБ. */
export const MAX_DOCUMENT_BYTES = 20 << 20;

/** Сверить предлагаемый токен с Telegram (getMe) — этим живёт ручка
 *  /internal/verify: сервис сам в Telegram не ходит, туда есть путь только
 *  отсюда, через SOCKS. Возвращает @username бота. */
export async function verifyToken(token: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const req = https.request(
      `${TELEGRAM_API_BASE}/bot${token.trim()}/getMe`,
      { method: 'POST', agent: socksAgent, headers: { 'Content-Type': 'application/json' } },
      (res) => {
        const chunks: Buffer[] = [];
        res.on('data', (c: Buffer) => chunks.push(c));
        res.on('end', () => {
          try {
            const parsed = JSON.parse(Buffer.concat(chunks).toString('utf8')) as TelegramApiResult<{
              username?: string;
              first_name?: string;
            }>;
            if (!parsed.ok) {
              reject(new Error(parsed.description || 'Telegram отклонил токен'));
              return;
            }
            resolve(parsed.result?.username ?? parsed.result?.first_name ?? '');
          } catch (e) {
            reject(new Error(`Telegram ответил не по формату: ${(e as Error).message}`));
          }
        });
      },
    );
    req.setTimeout(15000, () => req.destroy(new Error('Telegram не ответил за 15 секунд')));
    req.on('error', reject);
    req.end();
  });
}

/** Делит длинный текст по границам строк/предложений под лимит Telegram. */
export function splitForTelegram(text: string, limit = 4000): string[] {
  if (text.length <= limit) return [text];
  const parts: string[] = [];
  let rest = text;
  while (rest.length > limit) {
    const slice = rest.slice(0, limit);
    const cut = Math.max(slice.lastIndexOf('\n'), slice.lastIndexOf('. '), slice.lastIndexOf(' '));
    const at = cut > limit / 2 ? cut + 1 : limit;
    parts.push(rest.slice(0, at).trimEnd());
    rest = rest.slice(at).trimStart();
  }
  if (rest) parts.push(rest);
  return parts;
}

export async function sendMessage(
  chatId: string,
  text: string,
  replyMarkup?: Record<string, unknown>,
): Promise<number | undefined> {
  const parts = splitForTelegram(text);
  let firstId: number | undefined;
  for (let i = 0; i < parts.length; i++) {
    const body: Record<string, unknown> = { chat_id: chatId, text: parts[i] };
    if (replyMarkup && i === parts.length - 1) body.reply_markup = replyMarkup;
    const res = await telegramRequest<TelegramMessage>('sendMessage', body);
    if (!res.ok) throw new Error(res.description || 'Telegram sendMessage не удался');
    if (firstId === undefined && typeof res.result?.message_id === 'number') firstId = res.result.message_id;
  }
  return firstId;
}

export async function editMessage(
  chatId: string,
  messageId: number,
  text: string,
  replyMarkup?: Record<string, unknown>,
): Promise<void> {
  const body: Record<string, unknown> = { chat_id: chatId, message_id: messageId, text };
  if (replyMarkup) body.reply_markup = replyMarkup;
  const res = await telegramRequest('editMessageText', body);
  if (!res.ok && !/message is not modified/i.test(res.description ?? '')) {
    throw new Error(res.description || 'Telegram editMessageText не удался');
  }
}

export async function answerCallback(callbackId: string, text?: string): Promise<void> {
  await telegramRequest('answerCallbackQuery', text ? { callback_query_id: callbackId, text } : { callback_query_id: callbackId });
}

/** Отправка документа (multipart/form-data): файл лежит у нас, file_id не подходит. */
export async function sendDocument(
  chatId: string,
  fileName: string,
  content: Buffer,
  caption?: string,
): Promise<void> {
  if (content.length === 0) throw new Error('пустой документ');
  if (content.length > MAX_DOCUMENT_BYTES) throw new Error('документ тяжелее 20 МБ');
  const boundary = `offers${Date.now().toString(16)}`;
  const parts: Buffer[] = [];
  const text = (name: string, value: string): void => {
    parts.push(Buffer.from(`--${boundary}\r\nContent-Disposition: form-data; name="${name}"\r\n\r\n${value}\r\n`, 'utf8'));
  };
  text('chat_id', chatId);
  if (caption) text('caption', caption);
  parts.push(
    Buffer.from(
      `--${boundary}\r\nContent-Disposition: form-data; name="document"; filename="${fileName}"\r\n` +
        'Content-Type: application/vnd.openxmlformats-officedocument.wordprocessingml.document\r\n\r\n',
      'utf8',
    ),
  );
  parts.push(content);
  parts.push(Buffer.from('\r\n', 'utf8'));
  parts.push(Buffer.from(`--${boundary}--\r\n`, 'utf8'));
  const body = Buffer.concat(parts);
  const token = await resolveBotToken();

  const res = await new Promise<TelegramApiResult<unknown>>((resolve, reject) => {
    const req = https.request(
      `${TELEGRAM_API_BASE}/bot${token}/sendDocument`,
      {
        method: 'POST',
        agent: socksAgent,
        headers: { 'Content-Type': `multipart/form-data; boundary=${boundary}`, 'Content-Length': body.length },
      },
      (r) => {
        const chunks: Buffer[] = [];
        r.on('data', (c: Buffer) => chunks.push(c));
        r.on('end', () => {
          try {
            resolve(JSON.parse(Buffer.concat(chunks).toString('utf8')) as TelegramApiResult<unknown>);
          } catch (e) {
            reject(e);
          }
        });
      },
    );
    req.setTimeout(60000, () => req.destroy(new Error('Telegram sendDocument не ответил за 60000мс')));
    req.on('error', reject);
    req.write(body);
    req.end();
  });
  if (!res.ok) throw new Error(res.description || 'Telegram sendDocument не удался');
}

export async function getUpdates(offset: number, timeoutSec = 25): Promise<TelegramUpdate[]> {
  const res = await telegramRequest<TelegramUpdate[]>(
    'getUpdates',
    { offset, timeout: timeoutSec, allowed_updates: ['message', 'callback_query'] },
    (timeoutSec + 10) * 1000,
  );
  if (!res.ok) throw new Error(res.description || 'Telegram getUpdates не удался');
  return res.result ?? [];
}