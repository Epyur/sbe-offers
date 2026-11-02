import { Notice } from 'obsidian';
import type { OffersView } from './offers-view';
import type { CompositionItem, SystemWithComposition } from '../types/offers';
import { errorMessage } from '../../../sbe-core/src/utils/errors';
import { formatShortDate } from '../utils/date';
import { openNote } from './context-editor';

/**
 * «Системы» — сочетание материалов из Базы знаний (2026-10-31).
 *
 * Система — не строка справочника и не то, что продаётся отдельно: у неё нет
 * своей цены, она обосновывает, ПОЧЕМУ выбраны именно эти материалы. Поэтому
 * карточки живут в Базе знаний, а здесь только список с проверкой состава.
 *
 * Раздел ТОЛЬКО ДЛЯ ЧТЕНИЯ. Показывает то, что сервис уже прочитал из Базы
 * знаний; «Перечитать из Базы знаний» — отдельное действие (2026-11-01).
 *
 * Раньше раздел читал базу знаний вживую на каждом показе, ради свежего
 * состава. Цена оказалась выше пользы: сотни запросов на вход, минуты ожидания
 * и «Ключ доступа недействителен» при единичном сбое посреди захода.
 */
export function renderSystemsEditor(view: OffersView, container: HTMLElement): void {
  container.createDiv({ cls: 'tn-offers-muted', text: 'Читаю системы…' });
  void load(view, container);
}

/**
 * Показать раздел: по умолчанию — то, что сервис уже знает.
 *
 * `refresh` — осознанное «перечитать из Базы знаний». Оно одно ходит наружу;
 * обычный показ раздела берёт кеш (2026-11-01, требование пользователя:
 * «загрузились и используем то, что знаем, при обновлении обновляем данные»).
 * Иначе каждый вход в раздел стоил сотен запросов в Базу знаний, и любой сбой
 * посреди захода показывал «Ключ доступа недействителен» на ровном месте.
 */
async function load(view: OffersView, container: HTMLElement, refresh = false): Promise<void> {
  let systems: SystemWithComposition[] = [];
  let kbEnabled = true;
  let error = '';
  try {
    const res = refresh
      ? await view.plugin.offersService.refreshSystems()
      : await view.plugin.offersService.listSystems();
    systems = res.systems ?? [];
    kbEnabled = res.kbEnabled;
    error = res.error ?? '';
    if (refresh) {
      new Notice(
        error
          ? `База знаний недоступна, показаны прежние системы: ${error}`
          : `Обновлено систем: ${systems.length}`,
      );
    }
  } catch (e: unknown) {
    container.empty();
    container.createDiv({ cls: 'tn-offers-error', text: `Не удалось загрузить системы: ${errorMessage(e)}` });
    return;
  }
  container.empty();
  render(view, container, systems, kbEnabled, error);
}

function render(
  view: OffersView,
  container: HTMLElement,
  systems: SystemWithComposition[],
  kbEnabled: boolean,
  error: string,
): void {
  const hint = container.createDiv({ cls: 'tn-offers-card' });
  hint.createDiv({
    text:
      'Система — сочетание материалов, выбранное ради результата. Своей цены у неё нет: ' +
      'стоимость складывается из материалов, поэтому в карточке важна только связанность состава.',
  });
  hint.createDiv({
    cls: 'tn-offers-muted',
    text: 'Карточки создаются в Базе знаний — заметка с тегом commercial_propose (без тега material).',
  });

  if (!kbEnabled) {
    container.createDiv({
      cls: 'tn-offers-error',
      text: 'База знаний не настроена (KB_SERVICE_URL). Системы недоступны, КП собирается без них.',
    });
    return;
  }
  if (error) {
    container.createDiv({ cls: 'tn-offers-error', text: `База знаний ответила ошибкой: ${error}` });
  }

  const toolbar = container.createDiv({ cls: 'tn-offers-actions' });
  toolbar.createSpan({ cls: 'tn-offers-muted tn-offers-self-center', text: `Систем: ${systems.length}` });
  const reload = toolbar.createEl('button', { text: '↻ Перечитать из Базы знаний', cls: 'tn-btn tn-btn-ghost' });
  reload.addEventListener('click', () => {
    reload.disabled = true;
    reload.setText('Читаю…');
    container.empty();
    container.createDiv({ cls: 'tn-offers-muted', text: 'Перечитываю карточки систем из Базы знаний…' });
    void load(view, container, true);
  });

  if (systems.length === 0) {
    container.createDiv({
      cls: 'tn-offers-empty',
      text: 'Систем пока нет. Карточка появляется здесь, как только в Базе знаний есть заметка с тегом commercial_propose.',
    });
    return;
  }

  for (const card of systems) {
    renderCard(view, container, card);
  }
}

function renderCard(view: OffersView, container: HTMLElement, card: SystemWithComposition): void {
  const problems = card.problems ?? [];
  const links = card.compositionLinks ?? [];
  const layers = links.filter((l) => l.layer).length;
  const ok = problems.length === 0 && links.length > 0;

  const box = container.createDiv({ cls: 'tn-offers-card tn-offers-system-card' });
  const head = box.createDiv({ cls: 'tn-offers-row-between' });
  const titleWrap = head.createDiv();
  titleWrap.createDiv({ cls: 'tn-offers-card-title', text: card.title || '(без названия)' });
  titleWrap.createDiv({ cls: 'tn-offers-muted', text: card.sku ? `Код системы: ${card.sku}` : 'Кода системы нет' });
  // «Есть ошибки в составе» на всех системах подряд — ложная тревога
  // (2026-11-02): у живых карточек в составе есть слои и работы, материала у
  // них нет по смыслу. Ошибка — только то, что можно исправить.
  const state = head.createSpan({
    cls: ok ? 'tn-offers-ok' : 'tn-offers-warn',
    text: ok ? 'состав распознан' : 'есть замечания к составу',
  });
  void state;

  if (problems.length > 0) {
    const list = box.createEl('ul', { cls: 'tn-offers-problems' });
    for (const p of problems) {
      list.createEl('li', { text: p });
    }
  }
  if (layers > 0) {
    box.createDiv({
      cls: 'tn-offers-muted',
      text:
        `Слои без материала: ${layers}. Это работы и слои («Подготовка основания», ` +
        '«Уплотнённый грунт») — материала у них нет по смыслу, в позиции они не попадают.',
    });
  }

  if (links.length > 0) {
    const table = box.createEl('table', { cls: 'tn-offers-table' });
    const headRow = table.createEl('thead').createEl('tr');
    for (const title of ['Состав', 'Код', 'Материал', 'Ед.', 'ЕКН']) {
      headRow.createEl('th', { text: title });
    }
    const body = table.createEl('tbody');
    for (const item of links) {
      const row = body.createEl('tr');
      row.createEl('td', { text: item.raw });
      row.createEl('td', { cls: 'tn-offers-mono', text: item.sku || '—' });
      materialCell(row, item);
      row.createEl('td', { text: item.unit || '—' });
      row.createEl('td', { cls: 'tn-offers-mono', text: (item.ekn ?? []).join(', ') || '—' });
    }
  }

  const footer = box.createDiv({ cls: 'tn-offers-row tn-offers-system-footer' });
  const open = footer.createEl('button', { text: 'Открыть заметку', cls: 'tn-btn tn-btn-ghost' });
  open.addEventListener('click', () => {
    void openNote(view, card.path);
  });
  if (card.updatedAt) {
    footer.createSpan({ cls: 'tn-offers-muted', text: `Правка: ${formatShortDate(card.updatedAt)}` });
  }
}

/** Ячейка «материал»: найден — название, слой — так и называется, нет — причина. */
function materialCell(row: HTMLElement, item: CompositionItem): void {
  const cell = row.createEl('td');
  if (item.found) {
    cell.createSpan({ text: item.name || '' });
    return;
  }
  if (item.layer) {
    cell.createSpan({ cls: 'tn-offers-muted', text: 'слой / работа' });
    return;
  }
  if (!item.sku) {
    cell.createSpan({ cls: 'tn-offers-warn', text: 'ссылка не проставлена' });
    return;
  }
  cell.createSpan({ cls: 'tn-offers-warn', text: 'нет в справочнике' });
}