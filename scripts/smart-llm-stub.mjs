/**
 * Стабильный «умный» ответ для кэшевых проверок (2026-11-16, шаг 2b).
 *
 * Зачем. Кэш раздела листка проверяется ДВУМЯ сборками подряд: первый DOCX
 * пишет текст разделов моделью и сохраняет его в листок, второй обязан взять
 * тот же текст из базы и не позвать модель. Стоит заглушке ответить по-разному
 * (или добавить метку времени, как можно отличить повторы), и второй файл
 * станет другим — проверка «повторная выгрузка отдаёт тот же файл» упадёт или,
 * хуже, начнёт молча проверять не то.
 *
 * Ответ — по ключевым словам инструкции, без случайности: у одинакового
 * запроса одинаковый текст. Метка `[stub]` в начале помогает найти место в
 * журнале по времени, а не по догадке.
 *
 * Где используется. Только обёртка бота, названная в
 * `offers_local_e2e.mjs` секцией «Кэш раздела листка»: `/bot/*` идёт к
 * сервису с ОТДЕЛЬНЫМ ключом, поэтому листки, заведённые ботом, не портят
 * кэш DOCX, который проверяют секции с JWT (`llm-stub.mjs`).
 */

import http from 'node:http';

const PORT = Number(process.env.SMART_STUB_PORT ?? 3000);

/**
 * Сколько раз модель спрашивали за жизнь контейнера.
 *
 * Счётчик НЕ попадает в текст ответа намеренно: второй файл обязан совпасть с
 * первым, поэтому различающее число уходит только в журнал заглушки
 * (`docker logs offers-local-smart-llm-stub`) и в счётную ручку ниже. Так
 * проверка «второй раз модель не звалась» видна человеку, а кэш она не ломает.
 */
let calls = 0;

/** Секрет службы: тот же, что контейнер `offers` шлёт в `X-Service-Secret`. */
const SECRET = process.env.LLM_SERVICE_SECRET ?? '';

/** Компактная таблица для подраздела сравнения характеристик. */
function specsTable() {
  return JSON.stringify({
    blocks: [
      {
        kind: 'table',
        caption: 'Таблица — сравнение характеристик',
        rows: [
          ['Характеристика', 'Наш продукт', 'Конкурент'],
          ['Толщина', '1,5 мм', '1,2 мм'],
          ['Гарантия', '15 лет', '10 лет'],
        ],
      },
    ],
  });
}

function paragraph(text) {
  return JSON.stringify({ blocks: [{ kind: 'paragraph', text }] });
}

/** Ответ по инструкции: одинаковый запрос — одинаковый текст. */
function sectionAnswer(user) {
  if (/характеристик/i.test(user) || /таблиц/i.test(user)) return specsTable();
  if (/сильнее/i.test(user) && /конкурент/i.test(user)) {
    return paragraph('[stub] Конкурент сильнее в цене монтажа; отвечаем сроком службы и сервисом.');
  }
  if (/о продукте|что это|применяется/i.test(user)) {
    return paragraph('[stub] Продукт применяется в кровельном пироге и работает как гидроизоляция.');
  }
  return paragraph('[stub] Короткий разбор по карточкам.');
}

/**
 * Ответ «бота-помощника» для секций, которые зовут инструменты. Просто текст,
 * без вызова инструментов: эти секции проверяют кэш DOCX, а не агента.
 */
function agentAnswer() {
  return JSON.stringify({ type: 'final', text: '[stub] Ответ помощника (кэш-проверки).' });
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    req.on('data', (c) => chunks.push(c));
    req.on('end', () => resolve(Buffer.concat(chunks)));
    req.on('error', reject);
  });
}

/** Последняя реплика человека в промте — по ней и отвечаем. */
function lastUserLine(prompt) {
  const lines = String(prompt ?? '').split('\n');
  for (let i = lines.length - 1; i >= 0; i -= 1) {
    if (lines[i].trim()) return lines[i];
  }
  return '';
}

http
  .createServer(async (req, res) => {
    // Счётная ручка для человека: сколько раз модель спрашивали. Секрет не
    // нужен — это только число, а сеть стенда локальная.
    if (req.method === 'GET' && req.url === '/calls') {
      res.writeHead(200, { 'Content-Type': 'application/json; charset=utf-8' });
      res.end(JSON.stringify({ calls }));
      return;
    }
    // Тот же контракт, что у llm-stub: только /internal/llm/complete и только
    // с секретом службы.
    if (req.method !== 'POST' || req.url !== '/internal/llm/complete') {
      res.writeHead(404).end();
      return;
    }
    if (!SECRET || req.headers['x-service-secret'] !== SECRET) {
      res.writeHead(403).end('{"error":"forbidden"}');
      return;
    }
    let payload;
    try {
      payload = JSON.parse((await readBody(req)).toString('utf8'));
    } catch {
      res.writeHead(400).end('{"error":"invalid json"}');
      return;
    }
    const system = String(payload.system ?? '');
    const user = String(payload.user ?? '');
    const isAgent = /помощник менеджера отдела продаж/.test(system);
    const content = isAgent ? agentAnswer() : sectionAnswer(user || lastUserLine(system));
    calls += 1;
    console.log(`smart-llm-stub: вызов №${calls} (${isAgent ? 'агент' : 'раздел'})`);
    res.writeHead(200, { 'Content-Type': 'application/json; charset=utf-8' });
    res.end(JSON.stringify({
      content,
      model: 'smart-stub-1',
      usage: { prompt_tokens: 0, completion_tokens: 0, total_tokens: 0 },
    }));
  })
  .listen(PORT, () => {
    console.log(`smart-llm-stub: слушает :${PORT}`);
  });