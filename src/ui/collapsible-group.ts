import { setIcon } from 'obsidian';

/**
 * Сворачиваемая группа полей конфигуратора (2026-11-14, дефект 0.10.2).
 *
 * Просьба пользователя: «сделать все группы полей сворачиваемыми и по
 * умолчанию он должны быть свёрнуты». Причина понятна: в форме КП семь
 * карточек подряд, и до блоков документа человек проматывал экран.
 *
 * Одна реализация на все группы: пять своих копий разъехались бы по
 * раскрытию, иконкам и доступности при первой же правке.
 *
 * Состояние НЕ сохраняется в настройки намеренно: свёрнутость — это способ
 * прочитать один экран, а не режим работы. Сохранять её значило бы, что
 * администратор, открыв группу вчера, ждёт её открытой и сегодня, а человек
 * рядом — наоборот.
 */
export interface CollapsibleGroup {
  /** Тело группы: сюда рисует содержимое вызывающий. */
  body: HTMLElement;
  /** Раскрыть/свернуть программно (например, чтобы показать ошибку). */
  setOpen(open: boolean): void;
}

export interface CollapsibleGroupOptions {
  /** Заголовок группы — он же кнопка раскрытия. */
  title: string;
  /** Пояснение справа от заголовка (счётчик, состояние) — необязательно. */
  hint?: string;
  /** Раскрыть сразу; по умолчанию группа свёрнута. */
  open?: boolean;
  /** Классы на обёртке (например, `tn-offers-preview-card`). */
  cls?: string;
}

/**
 * renderCollapsibleGroup рисует группу в `container` и возвращает её тело.
 *
 * Заголовок — `<button>` с `aria-expanded`: клавиатура и экранный диктор
 * должны уметь раскрыть группу так же, как мышь. Внутри кнопки — иконка
 * `chevron-right`, при раскрытии поворачивается (класс `is-open`).
 */
export function renderCollapsibleGroup(
  container: HTMLElement,
  { title, hint, open = false, cls }: CollapsibleGroupOptions,
): CollapsibleGroup {
  const wrap = container.createDiv({ cls: cls ? `tn-offers-card tn-offers-group ${cls}` : 'tn-offers-card tn-offers-group' });
  const head = wrap.createEl('button', {
    cls: 'tn-offers-group-head',
    attr: { type: 'button', 'aria-expanded': String(open) },
  });
  const chevron = head.createSpan({ cls: 'tn-offers-group-chevron' });
  setIcon(chevron, 'chevron-right');
  head.createSpan({ cls: 'tn-offers-group-title', text: title });
  if (hint) head.createSpan({ cls: 'tn-offers-muted tn-offers-group-hint', text: hint });

  const body = wrap.createDiv({ cls: 'tn-offers-group-body' });

  const apply = (next: boolean): void => {
    wrap.toggleClass('is-open', next);
    head.setAttribute('aria-expanded', String(next));
    body.hidden = !next;
  };
  apply(open);

  head.addEventListener('click', () => {
    // Состояние читается из разметки, а не из замыкания: так двойное
    // нажатие и программный setOpen не рассинхронизируют его с экраном.
    apply(!wrap.hasClass('is-open'));
  });

  return {
    body,
    setOpen: (next: boolean) => apply(next),
  };
}