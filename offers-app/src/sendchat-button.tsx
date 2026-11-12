/**
 * Кнопка «Отправить в чат» (2026-11-13, шаг S7).
 *
 * Одна на оба документа: вид передаётся, потому что нагрузка у бота одна и та
 * же (`{action:'docx', kind, id}`), а подпись — разная. Вне Telegram кнопки
 * нет вовсе: там отправлять некуда, и показывать её значило бы обещать
 * несуществующее действие.
 */
import { useState, type JSX } from 'react';
import { inTelegram } from './tg.ts';
import { sendButtonText, sendDocumentToChat, type DocumentKind } from './sendchat.ts';

export function SendToChatButton({ kind, id }: { kind: DocumentKind; id: number }): JSX.Element | null {
  const [problem, setProblem] = useState('');
  if (!inTelegram()) return null;

  return (
    <div className="app-card">
      <button
        className="app-button"
        type="button"
        onClick={() => {
          const text = sendDocumentToChat(kind, id);
          setProblem(text);
        }}
      >
        {sendButtonText(kind)}
      </button>
      <p className="app-muted">Файл соберёт бот и пришлёт его в чат с приложением.</p>
      {problem === '' ? null : <p className="app-error-text">{problem}</p>}
    </div>
  );
}