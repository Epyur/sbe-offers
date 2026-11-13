/**
 * Кнопка «Отправить в чат» (2026-11-13, шаг S7; переделано 2026-11-14).
 *
 * Одна на оба документа: вид передаётся, потому что нагрузка у бота одна и та
 * же (`{action:'docx', kind, id}`), а подпись — разная. Вне Telegram кнопки
 * нет вовсе: там отправлять некуда, и показывать её значило бы обещать
 * несуществующее действие.
 *
 * Отправка идёт HTTP-запросом к сервису (см. `sendchat.ts`), поэтому кнопка
 * ждёт ответа: пока его нет, повторное нажатие не шлёт второй запрос.
 */
import { useState, type JSX } from 'react';
import { initData, inTelegram, platformInfo } from './tg.ts';
import { preflightProblem, sendButtonText, sendDocumentToChat, type DocumentKind } from './sendchat.ts';

export function SendToChatButton({ kind, id }: { kind: DocumentKind; id: number }): JSX.Element | null {
  const [problem, setProblem] = useState('');
  const [busy, setBusy] = useState(false);
  if (!inTelegram()) return null;

  const onClick = (): void => {
    if (busy) return;
    const early = preflightProblem(kind, id);
    if (early !== '') {
      setProblem(early);
      return;
    }
    setBusy(true);
    setProblem('');
    void sendDocumentToChat(kind, id)
      .then((outcome) => {
        // Отказ виден словами синим текстом и НЕ закрывает окно: если окно
        // закрылось без текста — значит сервис подтвердил отправку.
        if (!outcome.sent) setProblem(outcome.problem);
      })
      .finally(() => setBusy(false));
  };

  return (
    <div className="app-card">
      <button className="app-button" type="button" onClick={onClick} disabled={busy}>
        {busy ? '⏳ Отправляю…' : sendButtonText(kind)}
      </button>
      <p className="app-muted">Файл соберёт бот и пришлёт его в чат с приложением.</p>
      <p className="app-muted">
        Отправка: {initData() === '' ? 'нет ключа Telegram' : 'ключ есть'}
        {platformInfo().version === '' ? '' : `, Telegram ${platformInfo().version}`}
      </p>
      {problem === '' ? null : <p className="app-error-text">{problem}</p>}
    </div>
  );
}