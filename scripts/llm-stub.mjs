/**
 * Заглушка llm-service для локального стенда (2026-10-30).
 *
 * Отвечает на ту же ручку, что настоящий сервис:
 *
 *   POST /internal/llm/complete  {system, user, max_tokens}
 *     - X-Service-Secret обязателен;
 *     - система содержит «JSON» (шаг брифа) → ответ — {"payload": {…}};
 *     - иначе (описания секций)             → ответ — обычный текст;
 *     - в ПОЛЬЗОВАТЕЛЬСКОМ промпте есть метка STUB_FAIL → ответ 502,
 *       чтобы проверить деградацию. Метка в тексте, а не в заголовке: сервис
 *       не пересылает произвольные заголовки, и проверять надо то, что он
 *       действительно отправляет.
 *
 * Сеть и провайдер стенду не нужны: проверяется поведение offers-service,
 * а не модель. Ответы нарочно простые — важно, что сервис их разбирает.
 */
import http from 'node:http';

const PORT = Number(process.env.LLM_PORT ?? 3000);
const SECRET = process.env.LLM_SERVICE_SECRET ?? '';

const FAIL_MARKER = 'STUB_FAIL';

const BRIEF_REPLY = JSON.stringify({
  payload: {
    object_name: 'ЖК «Поэма»',
    customer_goals: ['ввод в эксплуатацию в срок'],
    customer_pains: ['срывы сроков подрядчиками'],
    project_risks: [],
    budget_signal: '',
    decision_maker: 'главный инженер',
    timeline: 'до конца квартала',
    why_us: '',
    objections: [],
    notes: 'ответ заглушки',
    skipped_topics: [],
  },
  reply: 'Записал. Расскажите про боли заказчика.',
  skip: [],
});

const COPY_REPLY =
  'Террасы и кровли «Поэма» получают систему, рассчитанную на нагрузку ' +
  'эксплуатации и на срок ввода объекта.';

function reply(system) {
  return /JSON/i.test(system) ? BRIEF_REPLY : COPY_REPLY;
}

http
  .createServer((req, res) => {
    const send = (status, body) => {
      res.writeHead(status, { 'Content-Type': 'application/json' });
      res.end(typeof body === 'string' ? body : JSON.stringify(body));
    };
    if (req.method !== 'POST' || req.url !== '/internal/llm/complete') {
      send(404, { error: 'not found' });
      return;
    }
    if (!SECRET || req.headers['x-service-secret'] !== SECRET) {
      send(403, { error: 'forbidden' });
      return;
    }
    const chunks = [];
    req.on('data', (c) => chunks.push(c));
    req.on('end', () => {
      let system = '';
      let user = '';
      try {
        const body = JSON.parse(Buffer.concat(chunks).toString('utf8'));
        system = String(body.system ?? '');
        user = String(body.user ?? '');
      } catch {
        send(400, { error: 'invalid json' });
        return;
      }
      if (user.includes(FAIL_MARKER)) {
        send(502, { error: 'заглушка: ошибка провайдера по метке' });
        return;
      }
      if (process.env.LLM_STUB_LOG === '1') {
        const content = reply(system);
        console.log(`→ system: ${system.slice(0, 80)}\n→ user: ${user.slice(0, 120)}\n← content: ${content.slice(0, 100)}`);
      }
      send(200, { content: reply(system) });
    });
  })
  .listen(PORT, () => console.log(`llm-stub слушает :${PORT}`));
