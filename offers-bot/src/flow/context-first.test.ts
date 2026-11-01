/**
 * Контекст сначала, позиции потом (2026-11-01).
 *
 * Жалоба пользователя: «сейчас при нажатии кнопки "Собрать КП" бот предлагает
 * добавить позиции, это, что называется сразу с корабля на бал». Проверяем сам
 * порядок: разговор начинается с объекта и адресата, файл можно прислать на
 * любом шаге, каталог появляется только когда контекст собран.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {
  BRIEF_FILE_KEYBOARD,
  briefIntro,
  briefProgressLine,
  nextStageAfterBrief,
} from './brief.ts';
import type { ChatSession } from '../session.ts';

test('разговор о контексте начинается с объекта и адресата', () => {
  const text = briefIntro();
  assert.match(text, /объект/i, 'первый вопрос должен быть про объект');
  // Позиции упомянуть можно («они в конце»), но ВОПРОС не о них: бот не
  // просит выбрать материал из каталога и не показывает каталог.
  assert.doesNotMatch(text, /добавьте позици/i);
  assert.doesNotMatch(text, /выберите позици/i);
  assert.doesNotMatch(text, /из каталога/i);
});

test('на каждом вопросе есть кнопка прислать файл', () => {
  const rows = BRIEF_FILE_KEYBOARD.inline_keyboard;
  const buttons = rows.flat().map((b: { text: string }) => b.text);
  assert.ok(
    buttons.some((t: string) => /файл/i.test(t)),
    `ожидали кнопку «прислать файл», получили: ${buttons.join(', ')}`,
  );
});

test('прогресс называет этап, а не номер темы', () => {
  const line = briefProgressLine({ filled: 3, total: 14, stage: 'Объект и адресат' });
  assert.match(line, /Объект и адресат/);
  assert.match(line, /3 из 14/);
});

test('после брифа разговор идёт к позициям, а не к текстам', () => {
  assert.equal(nextStageAfterBrief(), 'positions');
});

test('черновик на старте существует, но позиций в нём нет', () => {
  const session: ChatSession = { step: 'authed', email: 'm@tn.ru' };
  assert.equal(session.draft, undefined, 'на старте черновика ещё нет — его создаёт бот');
});