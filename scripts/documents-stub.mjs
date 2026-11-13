/**
 * Заглушка documents-service для стенда писем (2026-11-14, шаг P5b).
 *
 * Отдаёт ТОЛЬКО `/internal/documents/agent/list` — набор «документы с
 * привязками к продукту», по которому offers-service проверяет видимость
 * письма. Форма — как у боевой ручки (`sbe-documents/agentdocs.go`):
 * `document_id`, `title`, `file_name`, `file_key`, `product_refs[{note_id,
 * title}]`, `updated_at`.
 *
 * Одна карточка — скан письма из `mailer-fixture.json` (`document_id`
 * 1788168804943, привязка к заметке 7547 «LOGICROOF V-RP»). Второй скан
 * мейлер-заглушка держит непривязанным НАМЕРЕННО: стенд проверяет, что
 * offers-service отсекает письмо по карточке, а не верит мейлеру.
 *
 * Секрет тот же, что у боевой ручки: `DOCUMENTS_INTERNAL_SECRET`.
 */
import http from 'node:http';

const PORT = Number(process.env.DOCUMENTS_PORT ?? 3000);
const SECRET = process.env.DOCUMENTS_INTERNAL_SECRET ?? '';

const documents = [
  {
    document_id: 1788168804943,
    title: 'ТД/КФ/021 - О преимуществах LOGICROOF V-RP',
    doc_type: 'letter',
    file_name: '№ТД-КФ021 от 27.05.2026 касательно применения ТН-КРОВЛЯ Гарант.pdf',
    file_key: 'documents/9f2/td-kf-021.pdf',
    link_url: '',
    product_refs: [{ note_id: 7547, title: 'LOGICROOF V-RP' }],
    no_agent: false,
    updated_at: '2026-11-13T00:00:00.000000000Z',
  },
];

function send(res, status, body) {
  res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8' });
  res.end(JSON.stringify(body));
}

http
  .createServer((req, res) => {
    if (!SECRET) {
      send(res, 503, { error: 'DOCUMENTS_INTERNAL_SECRET is not set' });
      return;
    }
    if (req.headers['x-service-secret'] !== SECRET) {
      send(res, 403, { error: 'bad service secret' });
      return;
    }
    const url = new URL(req.url ?? '/', 'http://stub');
    if (url.pathname === '/internal/documents/agent/list') {
      const q = (url.searchParams.get('q') ?? '').trim().toLowerCase();
      const items = documents.filter(
        (d) => !q || d.title.toLowerCase().includes(q),
      );
      send(res, 200, { source: 'documents', total: items.length, items });
      return;
    }
    send(res, 404, { error: 'not found' });
  })
  .listen(PORT, () => {
    console.log(`documents-stub: слушаю :${PORT}`);
  });