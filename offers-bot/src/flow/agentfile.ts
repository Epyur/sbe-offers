/**
 * Файл в режиме помощника (шаг 4 плана 2026-11-16).
 *
 * До этого шага любой присланный файл шёл в разбор состава КП
 * (`routeDocument` → `/attach`) и без заведённого КП отвечал «сначала заведите
 * КП». А помощнику файл нужен раньше: «сравни с присланным документом» — это
 * вопрос по файлу, а не состав предложения. Поэтому пока помощник включён,
 * файл уходит в `parse_file` сервиса агента вместе со следующим сообщением.
 *
 * Почему не сразу зовём агента. Telegram присылает файл и текст двумя
 * сообщениями; отправив ход «прочитай файл» на одном файле, мы бы заставили
 * человека дважды писать одно и то же. Поэтому файл запоминается в сессии
 * (`session.agentFile`), а ход начинается с ближайшим текстом — тот же порядок,
 * что у обычного разговора: сначала приложение, потом вопрос.
 */
import type { ChatSession } from '../session.ts';

/** Один приложенный файл, ждущий ближайшего хода помощника. */
export interface PendingAgentFile {
  name: string;
  /** base64: сессия переживает перезапуск как JSON, Buffer туда не ляжет. */
  base64: string;
}

/** Сессия с приложенным файлом. */
export interface AgentFileSession extends ChatSession {
  agentFile?: PendingAgentFile;
}

/**
 * Верхний предел файла для помощника. Telegram Bot API отдаёт до 20 МБ, мы
 * берём 16 — столько допускает сервис агента; больший файл отказываем сразу, а
 * не после долгой пересылки base64 в JSON.
 */
export const MAX_AGENT_FILE_BYTES = 16 * 1024 * 1024;

/** Запомнить файл до следующего сообщения помощника. */
export function attachAgentFile(session: ChatSession, name: string, data: Buffer): AgentFileSession {
  const next: AgentFileSession = { ...session };
  next.agentFile = { name, base64: data.toString('base64') };
  return next;
}

/** Забрать приложенный файл (и очистить его из сессии). */
export function takeAgentFile(session: ChatSession): { session: ChatSession; file?: PendingAgentFile } {
  const current = session as AgentFileSession;
  if (!current.agentFile) return { session };
  const next: AgentFileSession = { ...session };
  delete next.agentFile;
  return { session: next, file: current.agentFile };
}

/** Что сказать человеку сразу после приёма файла. */
export function agentFileAcceptedText(file: PendingAgentFile): string {
  return `📎 Файл «${file.name}» принят. Напишите вопрос по нему — помощник прочитает файл вместе с вопросом.`;
}