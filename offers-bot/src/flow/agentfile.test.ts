/**
 * Приложенный файл в режиме помощника (шаг 4 плана 2026-11-16).
 *
 * Проверяем то, что решает, кому достанется файл: сессия запоминает его до
 * ближайшего текста, ход его забирает и очищает, а размер ограничен
 * (сервис агента больше 16 МБ не разберёт — и человек узнаёт об этом сразу).
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  MAX_AGENT_FILE_BYTES,
  agentFileAcceptedText,
  attachAgentFile,
  takeAgentFile,
} from './agentfile.ts';
import type { ChatSession } from '../session.ts';

const base: ChatSession = { step: 'authed', email: 'manager@tn.ru' };

test('файл запоминается и забирается ровно один раз', () => {
  const data = Buffer.from('состав КП');
  const withFile = attachAgentFile(base, 'состав.xlsx', data);
  assert.equal(withFile.agentFile?.name, 'состав.xlsx');
  assert.equal(withFile.agentFile?.base64, data.toString('base64'));
  // Исходная сессия не меняется: она сохранена и её читают другие ветки.
  assert.equal((base as { agentFile?: unknown }).agentFile, undefined);

  const taken = takeAgentFile(withFile);
  assert.equal(taken.file?.name, 'состав.xlsx');
  assert.equal(Buffer.from(taken.file!.base64, 'base64').toString(), 'состав КП');
  // Следующий вопрос уже про другое — старое вложение не переразбирается.
  assert.equal((taken.session as { agentFile?: unknown }).agentFile, undefined);
  assert.equal(takeAgentFile(taken.session).file, undefined);
});

test('без приложенного файла забирать нечего', () => {
  const taken = takeAgentFile(base);
  assert.equal(taken.file, undefined);
  assert.equal(taken.session, base);
});

test('предел файла — 16 МБ, и это сказано человеку', () => {
  assert.equal(MAX_AGENT_FILE_BYTES, 16 * 1024 * 1024);
  const text = agentFileAcceptedText({ name: 'протокол.docx', base64: '' });
  assert.match(text, /протокол\.docx/);
  assert.match(text, /напишите вопрос/i);
});