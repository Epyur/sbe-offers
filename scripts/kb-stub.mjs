/**
 * Локальная заглушка kb-service для стенда (2026-10-30).
 *
 * Отдаёт ровно те ручки, которые зовёт offers-service:
 *
 *   GET /api/kb/tags                      {tags:[{id,name}]}
 *   GET /api/kb/notes?tag_id=&size=       {notes:[{id,uuid,title,path,updated_at}]}
 *   GET /api/kb/notes/{id}                {id,title,path,content_md,tags,updated_at}
 *   GET /api/kb/notes/{id}/attachments    {attachments:[{filename,mime,key}]}
 *   GET /api/kb/file-link?key=            {url}
 *
 * Зачем нужна: без неё стенд не может проверить, что карточки систем
 * ДОХОДЯТ до плагина. Живой дефект 2026-10-30 — в боевом compose у сервиса
 * `offers` не были заданы KB_SERVICE_URL/KB_SERVICE_SECRET, и плагин честно
 * показывал «База знаний не настроена», хотя она настроена. Стенд такого не
 * ловил, потому что KB в нём не было вовсе.
 *
 * Разметка карточки — по стандарту `docs/sbe-offers-system-card-standard.md`.
 */
import http from 'node:http';

const PORT = Number(process.env.KB_PORT ?? 3000);

const CARD_MD = [
  '---',
  'sbe_kb_id: 11111111-2222-3333-4444-555555555555',
  'tags: [commercial_propose]',
  'system_sku: TN-STILOBAT-EXPERT',
  '---',
  '# ТН-Стилобат Эксперт',
  '',
  '## Описание системы',
  'Эксплуатируемая кровля по плитам перекрытия: гидроизоляция и дренаж в одном контуре.',
  '',
  '## Состав системы',
  '- 1 Геотекстиль разделительный',
  '- 2 Профилированная мембрана',
  '- 3 Гидроизоляционная мембрана ПВХ',
  '',
  '## Свойства и преимущества',
  '- Выдерживает нагрузку пешеходной зоны',
  '- Монтаж без открытого пламени',
  '',
  '## Характеристики',
  '| Параметр | Значение |',
  '| --- | --- |',
  '| Класс нагрузки | 1,5 кН/м² |',
].join('\n');

const NOTES = [
  {
    id: 101,
    uuid: '11111111-2222-3333-4444-555555555555',
    title: 'ТН-Стилобат Эксперт',
    path: 'База знаний/Системы/ТН-Стилобат Эксперт.md',
    updated_at: '2026-10-29T10:00:00Z',
    content_md: CARD_MD,
    tags: [{ id: 7, name: 'commercial_propose' }],
  },
  {
    id: 102,
    uuid: '22222222-3333-4444-5555-666666666666',
    title: 'ТН-Кровля Классик',
    path: 'База знаний/Системы/ТН-Кровля Классик.md',
    updated_at: '2026-10-28T09:30:00Z',
    content_md: [
      '---',
      'sbe_kb_id: 22222222-3333-4444-5555-666666666666',
      'tags: [commercial_propose]',
      'system_sku: TN-ROOF-CLASSIC',
      '---',
      '# ТН-Кровля Классик',
      '',
      '## Описание системы',
      'Классическая наплавляемая кровля для промышленных объектов.',
      '',
      '## Состав системы',
      '- 1 Праймер битумный',
      '- 2 Техноэласт ЭПП',
    ].join('\n'),
    tags: [{ id: 7, name: 'commercial_propose' }],
  },
];

function send(res, status, body) {
  res.writeHead(status, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify(body));
}

http
  .createServer((req, res) => {
    const url = new URL(req.url ?? '/', 'http://localhost');
    const path = url.pathname;

    // Токен обязателен, как в настоящем сервисе: если клиент перестанет его
    // слать, стенд это заметит, а не промолчит.
    if (!(req.headers.authorization ?? '').startsWith('Bearer ')) {
      return send(res, 401, { error: 'нет токена' });
    }

    if (req.method === 'GET' && path === '/api/kb/tags') {
      return send(res, 200, { tags: [{ id: 7, name: 'commercial_propose' }] });
    }

    if (req.method === 'GET' && path === '/api/kb/notes') {
      const tagID = Number(url.searchParams.get('tag_id') ?? 0);
      const items = tagID === 7
        ? NOTES.map(({ content_md, ...rest }) => rest)
        : [];
      // Поле называется items — как у настоящего kb-service (2026-10-30).
      // Пока стенд отдавал notes, клиент читал только notes, а боевой сервис
      // отдаёт items: карточки на бою всегда приходили пустым списком.
      return send(res, 200, { items, total: items.length, page: 1, size: 200 });
    }

    if (req.method === 'GET' && path === '/api/kb/file-link') {
      const key = url.searchParams.get('key') ?? '';
      if (!key) return send(res, 400, { error: 'нет key' });
      return send(res, 200, { url: `http://kb-stub:3000/files/${encodeURIComponent(key)}` });
    }

    const attachMatch = path.match(/^\/api\/kb\/notes\/(\d+)\/attachments$/);
    if (req.method === 'GET' && attachMatch) {
      // Вложений в заглушке нет: карточка без картинки — законное состояние,
      // и DOCX должен собраться без неё.
      return send(res, 200, { attachments: [] });
    }

    const noteMatch = path.match(/^\/api\/kb\/notes\/(\d+)$/);
    if (req.method === 'GET' && noteMatch) {
      const note = NOTES.find((n) => n.id === Number(noteMatch[1]));
      if (!note) return send(res, 404, { error: 'заметка не найдена' });
      return send(res, 200, note);
    }

    return send(res, 404, { error: 'not found' });
  })
  .listen(PORT, '0.0.0.0', () => console.log(`kb-stub слушает :${PORT}`));