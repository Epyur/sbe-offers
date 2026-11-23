/**
 * Технодайджест в ленте (2026-11-23) — проверяется как поведение: на вход
 * ответ сервиса или текст новости, на выход то, что человек увидит на экране.
 *
 * Главное правило — на границе: непонятный ответ не роняет экран, а даёт
 * пустую ленту; новость без номера или заголовка в ленту не попадает.
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import {
  dayLabel,
  digestFeedOf,
  digestParagraphs,
  digestPath,
  monthLabel,
  newsByline,
  sendNewsToChat,
} from '../src/digest.ts';

test('лента: ответ сервиса разбирается в новости и месяцы', () => {
  const feed = digestFeedOf({
    current: '2026-11',
    issues: [
      { ym: '2026-11', title: 'Технические новости ноября 2026', items: 2 },
      { ym: '2026-10', title: 'Октябрь', items: 5 },
    ],
    issue: { ym: '2026-11', title: 'Технические новости ноября 2026', items: 2 },
    items: [
      {
        id: 7,
        title: 'Новая мембрана',
        lead: 'Коротко о главном',
        body: 'Первый абзац.',
        rubric: 'Материалы',
        author: 'Иван Петров',
        at: '2026-11-05T09:00:00Z',
        isLead: true,
        linkUrl: 'https://example.org/news',
        linkText: 'Подробности',
      },
    ],
  });
  assert.equal(feed.current, '2026-11');
  assert.equal(feed.issues.length, 2);
  assert.equal(feed.issue.ym, '2026-11');
  assert.equal(feed.items.length, 1);
  assert.equal(feed.items[0].title, 'Новая мембрана');
  assert.equal(feed.items[0].rubric, 'Материалы');
  assert.equal(feed.items[0].isLead, true);
  assert.equal(feed.items[0].linkUrl, 'https://example.org/news');
});

test('лента: новость без номера или заголовка не показывается', () => {
  const feed = digestFeedOf({
    items: [
      { id: 0, title: 'Без номера' },
      { id: 5, title: '   ' },
      { id: 6, title: 'Нормальная' },
    ],
  });
  assert.deepEqual(feed.items.map((i) => i.id), [6]);
});

test('лента: непонятный ответ — пустая лента, а не исключение', () => {
  for (const raw of [null, undefined, 'что-то', 42, { offers: [] }]) {
    const feed = digestFeedOf(raw);
    assert.deepEqual(feed.issues, []);
    assert.deepEqual(feed.items, []);
    assert.equal(feed.current, '');
    assert.equal(feed.issue.ym, '');
  }
});

test('адрес ленты: пустой месяц — текущий выпуск', () => {
  assert.equal(digestPath(''), '/digest');
  assert.equal(digestPath('   '), '/digest');
  assert.equal(digestPath('2026-11'), '/digest?ym=2026-11');
});

test('месяц выпуска читается словом', () => {
  assert.equal(monthLabel('2026-11'), 'Ноябрь 2026');
  assert.equal(monthLabel('2026-01'), 'Январь 2026');
  assert.equal(monthLabel('2026-13'), '2026-13');
  assert.equal(monthLabel(''), '');
});

test('текст новости — абзацами, без служебной разметки', () => {
  const body =
    '## Подзаголовок\n\n**Первый** абзац,\nпродолжение.\n\n- пункт списка\n- второй пункт\n\n[Подробности](https://example.org/news)';
  assert.deepEqual(digestParagraphs(body), [
    'Подзаголовок',
    'Первый абзац, продолжение.',
    'пункт списка второй пункт',
    'Подробности — https://example.org/news',
  ]);
  assert.deepEqual(digestParagraphs(''), []);
  assert.deepEqual(digestParagraphs('\n\n'), []);
});

test('подпись новости: автор и дата; пустое не оставляет висячих точек', () => {
  const base = {
    id: 1,
    title: 'Т',
    lead: '',
    body: '',
    rubric: '',
    author: 'Иван Петров',
    at: '2026-11-05T09:00:00Z',
    isLead: false,
    linkUrl: '',
    linkText: '',
  };
  assert.equal(newsByline(base), 'Иван Петров · 05.11.2026');
  assert.equal(newsByline({ ...base, author: '' }), '05.11.2026');
  assert.equal(newsByline({ ...base, at: '' }), 'Иван Петров');
  assert.equal(newsByline({ ...base, author: '', at: '' }), '');
});

test('дата: мусор не превращается в «NaN.NaN.NaN»', () => {
  assert.equal(dayLabel('2026-11-05T09:00:00Z'), '05.11.2026');
  assert.equal(dayLabel('не дата'), 'не дата');
  assert.equal(dayLabel(''), '');
});

test('«Отправить в чат» вне Telegram не делает запрос, а объясняет почему', async () => {
  const outcome = await sendNewsToChat('2026-11', 7);
  assert.equal(outcome.sent, false);
  assert.match(outcome.problem, /только из Telegram/);
});