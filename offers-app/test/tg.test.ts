/**
 * Мост к Telegram (2026-11-13, живой дефект).
 *
 * Главное здесь: мост ищется ПРИ КАЖДОМ ОБРАЩЕНИИ. Раньше он захватывался при
 * загрузке модуля, и если скрипт платформы появлялся позже, приложение навсегда
 * оставалось «вне Telegram» — человек видел «initData: нет» до перезагрузки.
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import { inTelegram, initData, platformInfo, sendToChat, waitForTelegram } from '../src/tg.ts';

// В node окна нет вовсе — это и есть самый важный случай: модуль не падает, а
// честно говорит «не Telegram».
test('без окна модуль не падает и отвечает «не Telegram»', () => {
  assert.equal(inTelegram(), false);
  assert.equal(initData(), '');
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

/**
 * Живой дефект 2026-11-13: «выбрал Отправить в чат — приложение закрылось,
 * в чате пусто». `sendData` бросает `WebAppDataInvalid`, прежний вызов это
 * исключение не ловил, а `close()` закрывал приложение — человек не видел ни
 * ошибки, ни файла. Здесь окно подменяется: платформа отказывает, значит
 * человек обязан получить текст, а приложение — остаться открытым.
 */
function withFakeTelegram(sendData: (data: string) => void): { closed: () => boolean; restore: () => void } {
  const win = globalThis as unknown as { Telegram?: unknown; window?: unknown };
  win.window = win;
  const saved = win.Telegram;
  let closed = false;
  win.Telegram = {
    WebApp: {
      initData: '',
      initDataUnsafe: {},
      colorScheme: 'light',
      themeParams: {},
      ready: () => {},
      expand: () => {},
      close: () => {
        closed = true;
      },
      sendData,
      onEvent: () => {},
      offEvent: () => {},
      version: '9.6',
      platform: 'android',
    },
  };
  return {
    closed: () => closed,
    restore: () => {
      win.Telegram = saved;
    },
  };
}

test('отказ платформы виден человеку, а приложение не закрывается', () => {
  const fake = withFakeTelegram(() => {
    throw new Error('WebAppDataInvalid');
  });
  try {
    const text = sendToChat({ action: 'docx', kind: 'offer', id: 3 });
    assert.match(text, /Telegram не принял/i);
    assert.match(text, /WebAppDataInvalid/);
    assert.equal(fake.closed(), false);
  } finally {
    fake.restore();
  }
});

test('успешная отправка закрывает приложение и не возвращает ошибку', () => {
  let sent = '';
  const fake = withFakeTelegram((data) => {
    sent = data;
  });
  try {
    const text = sendToChat({ action: 'docx', kind: 'combat', id: 7 });
    assert.equal(text, '');
    assert.deepEqual(JSON.parse(sent), { action: 'docx', kind: 'combat', id: 7 });
    assert.equal(fake.closed(), true);
  } finally {
    fake.restore();
  }
});
