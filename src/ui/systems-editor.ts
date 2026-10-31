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
 * Раздел ТОЛЬКО ДЛЯ ЧТЕНИЯ и, в отличие от «Контекста КП», читает базу знаний
 * вживую: состав проверяется против ТЕКУЩЕГО справочника материалов, и кеш
 * здесь показывал бы устаревшие данные после правки материала.
 */
export function renderSystemsEditor(view: OffersView, container: HTMLElement): void {
  container.createDiv({ cls: 'tn-offers-muted', text: 'Читаю карточки систем из Базы знаний…' });
  void load(view, container);
}

async function load(view: OffersView, container: HTMLElement): Promise<void> {
  let systems: SystemWithComposition[] = [];
  let kbEnabled = true;
  let error = '';
  try {
    const res = await view.plugin.offersService.listSystems();
    systems = res.systems ?? [];
    kbEnabled = res.kbEnabled;
    error = res.error ?? '';
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
  const reload = toolbar.createEl('button', { text: '↻ Обновить', cls: 'tn-btn tn-btn-ghost' });
  reload.addEventListener('click', () => {
    container.empty();
    container.createDiv({ cls: 'tn-offers-muted', text: 'Читаю карточки систем…' });
    void load(view, container);
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
  const ok = problems.length === 0 && links.length > 0;

  const box = container.createDiv({ cls: 'tn-offers-card tn-offers-system-card' });
  const head = box.createDiv({ cls: 'tn-offers-row-between' });
  const titleWrap = head.createDiv();
  titleWrap.createDiv({ cls: 'tn-offers-card-title', text: card.title || '(без названия)' });
  titleWrap.createDiv({ cls: 'tn-offers-muted', text: card.sku ? `Код системы: ${card.sku}` : 'Кода системы нет' });
  const state = head.createSpan({
    cls: ok ? 'tn-offers-ok' : 'tn-offers-warn',
    text: ok ? 'состав распознан' : 'есть ошибки в составе',
  });
  void state;

  if (problems.length > 0) {
    const list = box.createEl('ul', { cls: 'tn-offers-problems' });
    for (const p of problems) {
      list.createEl('li', { text: p });
    }
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

/** Ячейка «материал»: найден — название, нет — честная пометка причины. */
function materialCell(row: HTMLElement, item: CompositionItem): void {
  const cell = row.createEl('td');
  if (item.found) {
    cell.createSpan({ text: item.name || '' });
    return;
  }
  if (!item.sku) {
    cell.createSpan({ cls: 'tn-offers-warn', text: 'ссылка не проставлена' });
    return;
  }
  cell.createSpan({ cls: 'tn-offers-warn', text: 'нет в справочнике' });
}