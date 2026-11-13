/**
 * Мост к Telegram (2026-11-13, живой дефект).
 *
 * Главное здесь: мост ищется ПРИ КАЖДОМ ОБРАЩЕНИИ. Раньше он захватывался при
 * загрузке модуля, и если скрипт платформы появлялся позже, приложение навсегда
 * оставалось «вне Telegram» — человек видел «initData: нет» до перезагрузки.
 *
 * Второе: `query_id` берётся из `initData`, а не из отдельного поля платформы —
 * у вендорного скрипта его нет в `initDataUnsafe`. Отсутствие `query_id` и
 * есть признак кнопки клавиатуры, где `sendData` работает иначе.
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import { inTelegram, initData, platformInfo, queryId, waitForTelegram } from '../src/tg.ts';

// В node окна нет вовсе — это и есть самый важный случай: модуль не падает, а
// честно говорит «не Telegram».
test('без окна модуль не падает и отвечает «не Telegram»', () => {
  assert.equal(inTelegram(), false);
  assert.equal(initData(), '');
  assert.equal(queryId(), '');
  assert.deepEqual(platformInfo(), { version: '', platform: '' });
});

test('ожидание платформы без окна завершается, а не висит', async () => {
  const found = await waitForTelegram(50);
  assert.equal(found, false);
});

test('мост — функция, а не снимок: значение читается в момент вызова', () => {
  // Проверяем форму API, а не окружение: если кто-то снова сделает `const
  // inTelegram = ...` при импорте, этот тест упадёт на импорте выше.
  assert.equal(typeof inTelegram, 'function');
});

/** Подменить окно Telegram подписанной строкой initData. */
function withInitData(raw: string): () => void {
  const win = globalThis as unknown as { Telegram?: unknown; window?: unknown };
  win.window = win;
  const saved = win.Telegram;
  win.Telegram = {
    WebApp: {
      initData: raw,
      initDataUnsafe: {},
      colorScheme: 'light',
      themeParams: {},
      ready: () => {},
      expand: () => {},
      close: () => {},
      sendData: () => {},
      onEvent: () => {},
      offEvent: () => {},
      version: '9.6',
      platform: 'android',
    },
  };
  return () => {
    win.Telegram = saved;
  };
}

test('query_id берётся из initData — с него начинается отправка', () => {
  const restore = withInitData('query_id=AAHabc123&user=%7B%22id%22%3A810181470%7D&hash=deadbeef');
  try {
    assert.equal(queryId(), 'AAHabc123');
  } finally {
    restore();
  }
});

test('у кнопки клавиатуры query_id нет — вернётся пусто, а не мусор', () => {
  const restore = withInitData('auth_date=1763000000&user=%7B%22id%22%3A1%7D&hash=deadbeef');
  try {
    assert.equal(queryId(), '');
  } finally {
    restore();
  }
});

test('поиск query_id не зависит от порядка полей', () => {
  const restore = withInitData('auth_date=1&query_id=ZZZ&hash=x&user=%7B%7D');
  try {
    assert.equal(queryId(), 'ZZZ');
  } finally {
    restore();
  }
});