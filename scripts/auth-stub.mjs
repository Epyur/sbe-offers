/**
 * Локальная заглушка auth-service для прогона offers-service на машине.
 * Повторяет только то, что нужно сервису: /internal/apps/{app}/permissions
 * (карта ролей) и /apps/register. Роли задаются переменной AUTH_STUB_ROLES
 * в формате "email=role,email=role".
 */
import http from 'node:http';

const PORT = Number(process.env.AUTH_STUB_PORT ?? 13001);
const SECRET = process.env.OFFERS_SERVICE_SECRET ?? 'test-service-secret';
const ROLES = (process.env.AUTH_STUB_ROLES ?? 'owner@tn.ru=superadmin')
  .split(',')
  .map((pair) => pair.split('='))
  .filter((pair) => pair.length === 2)
  .reduce((acc, [email, role]) => {
    acc[email.trim().toLowerCase()] = role.trim();
    return acc;
  }, {});
const COMMON_ACCESS = process.env.AUTH_STUB_COMMON_ACCESS ?? '';

const server = http.createServer((req, res) => {
  const send = (status, body) => {
    res.writeHead(status, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify(body));
  };

  if (req.method === 'POST' && req.url === '/apps/register') {
    return send(200, { ok: true });
  }

  // Межсервисный токен (2026-10-30): по нему offers-service ходит в kb-service.
  // Без него карточки систем не читаются даже при заданном KB_SERVICE_URL, и
  // стенд молча показывал бы «база знаний не настроена».
  if (req.method === 'POST' && req.url === '/internal/service-token') {
    if (req.headers['x-service-secret'] !== SECRET) {
      return send(403, { error: 'forbidden' });
    }
    let raw = '';
    req.on('data', (c) => {
      raw += c;
    });
    req.on('end', () => {
      let target = '';
      try {
        target = String(JSON.parse(raw || '{}').target_app_id ?? '');
      } catch {
        return send(400, { error: 'invalid json' });
      }
      // Токен — не настоящий JWT: kb-stub его только принимает, а не проверяет.
      send(200, { jwt: `stub-token-for-${target || 'unknown'}`, expiresAt: '' });
    });
    return undefined;
  }

  const permsMatch = req.url?.match(/^\/internal\/apps\/([^/]+)\/permissions$/);
  if (permsMatch && req.method === 'GET') {
    if (req.headers['x-service-secret'] !== SECRET) {
      return send(403, { error: 'forbidden' });
    }
    return send(200, {
      permissions: ROLES,
      common_access: COMMON_ACCESS,
      common_access_set: COMMON_ACCESS !== '',
    });
  }

  if (req.method === 'POST' && req.url?.startsWith('/internal/apps/')) {
    return send(200, { ok: true });
  }

  return send(404, { error: 'not found' });
});

server.listen(PORT, '0.0.0.0', () => {
  console.log(`auth-stub на :${PORT}, роли: ${JSON.stringify(ROLES)}`);
});