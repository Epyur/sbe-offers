/**
 * Заглушка offers-bot для локального стенда (2026-10-29): отвечает на ручку
 * проверки токена так, как ответил бы настоящий бот, но без сети.
 *
 *   POST /internal/verify  {token}
 *     - X-Service-Secret обязателен;
 *     - «123456789:GOOD» → {ok:true, username:'offers_test_bot'};
 *     - форма верная, но бота нет → {ok:false, error:'Unauthorized'};
 *     - форма неверная → {ok:false, error:'Not Found'}.
 *
 * Настоящий бот ходит в Telegram через SOCKS, стенду сеть не нужна.
 */
import http from 'node:http';

const PORT = Number(process.env.VERIFY_PORT ?? 3000);
const SECRET = process.env.OFFERS_BOT_SECRET ?? '';
const GOOD = '123456789:GOOD';

http
  .createServer((req, res) => {
    const send = (status, body) => {
      res.writeHead(status, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify(body));
    };
    // Просьба приложения «продолжить в боте» (2026-11-15). Настоящий бот
    // открывает диалог у себя; заглушке достаточно подтвердить приём: её роль —
    // держать контракт `/internal/ask` за секретом сервиса, а тексты диалога
    // проверяет тест бота (`src/ask.test.ts`).
    if (req.method === 'POST' && req.url === '/internal/ask') {
      if (!SECRET || req.headers['x-service-secret'] !== SECRET) {
        send(403, { ok: false, error: 'forbidden' });
        return;
      }
      const askChunks = [];
      req.on('data', (c) => askChunks.push(c));
      req.on('end', () => {
        let action = '';
        try {
          action = String(JSON.parse(Buffer.concat(askChunks).toString('utf8')).action ?? '');
        } catch {
          send(400, { ok: false, error: 'invalid json' });
          return;
        }
        if (action !== 'brief' && action !== 'positions') {
          send(400, { ok: false, error: 'unknown action' });
          return;
        }
        send(200, { ok: true, sent: action === 'brief' ? 'Открыли бриф в чате.' : 'Написали в чат.' });
      });
      return;
    }
    if (req.method !== 'POST' || req.url !== '/internal/verify') {
      send(404, { ok: false, error: 'not found' });
      return;
    }
    if (!SECRET || req.headers['x-service-secret'] !== SECRET) {
      send(403, { ok: false, error: 'forbidden' });
      return;
    }
    const chunks = [];
    req.on('data', (c) => chunks.push(c));
    req.on('end', () => {
      let token = '';
      try {
        token = String(JSON.parse(Buffer.concat(chunks).toString('utf8')).token ?? '').trim();
      } catch {
        send(400, { ok: false, error: 'invalid json' });
        return;
      }
      if (token === GOOD) {
        send(200, { ok: true, username: 'offers_test_bot' });
        return;
      }
      if (token.includes(':')) {
        send(400, { ok: false, error: 'Unauthorized' });
        return;
      }
      send(400, { ok: false, error: 'Not Found' });
    });
  })
  .listen(PORT, () => console.log(`bot-stub слушает :${PORT}`));