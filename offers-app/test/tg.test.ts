/**
 * Мост к Telegram (2026-11-13, живой дефект).
 *
 * Главное здесь: мост ищется ПРИ КАЖДОМ ОБРАЩЕНИИ. Раньше он захватывался при
 * загрузке модуля, и если скрипт платформы появлялся позже, приложение навсегда
 * оставалось «вне Telegram» — человек видел «initData: нет» до перезагрузки.
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import { inTelegram, initData, platformInfo, waitForTelegram } from '../src/tg.ts';

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
