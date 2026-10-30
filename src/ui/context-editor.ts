import { Notice } from 'obsidian';
import type { OffersView } from './offers-view';
import type { SystemCard } from '../types/offers';
import { errorMessage } from '../../../sbe-core/src/utils/errors';

/**
 * «Контекст КП» — карточки систем из Базы знаний (2026-10-29).
 *
 * Раздел ТОЛЬКО ДЛЯ ЧТЕНИЯ. Карточки пишут в Базе знаний обычными заметками с
 * тегом commercial_propose: там история версий, поиск и привычное место
 * документации. Здесь — список с состоянием связи с каталогом и кнопкой
 * «Открыть заметку».
 *
 * Кнопка «Перечитать» есть у администратора: она сбрасывает кеш и заново
 * читает базу знаний.
 */
export function renderContextEditor(view: OffersView, container: HTMLElement): void {
  container.createDiv({ cls: 'tn-offers-muted', text: 'Карточки систем из Базы знаний…' });
  void load(view, container);
}

async function load(view: OffersView, container: HTMLElement): Promise<void> {
  let cards: SystemCard[];
  let kbEnabled: boolean;
  try {
    const res = await view.plugin.offersService.getSystemCards();
    cards = res.cards;
    kbEnabled = res.kbEnabled;
  } catch (e: unknown) {
    container.empty();
    container.createDiv({ cls: 'tn-offers-error', text: `Не удалось загрузить карточки: ${errorMessage(e)}` });
    return;
  }
  container.empty();
  render(view, container, cards, kbEnabled);
}

function render(
  view: OffersView,
  container: HTMLElement,
  cards: SystemCard[],
  kbEnabled: boolean,
): void {
  const hint = container.createDiv({ cls: 'tn-offers-card tn-offers-context-hint' });
  hint.createDiv({
    text:
      'Карточки систем создаются в Базе знаний — заметка с тегом commercial_propose. ' +
      'Здесь только список: текст, состав и картинку берутся из заметки, цены и артикулы — из каталога.',
  });
  hint.createDiv({
    cls: 'tn-offers-muted',
    text: 'Стандарт карточки: docs/sbe-offers-system-card-standard.md в вольте.',
  });

  if (!kbEnabled) {
    container.createDiv({
      cls: 'tn-offers-error',
      text: 'База знаний не настроена (KB_SERVICE_URL). Карточки недоступны, КП собирается без них.',
    });
    return;
  }

  const toolbar = container.createDiv({ cls: 'tn-offers-card tn-offers-context-toolbar' });
  toolbar.createSpan({ cls: 'tn-offers-muted', text: `Карточек: ${cards.length}` });
  const spacer = toolbar.createDiv({ cls: 'tn-offers-spacer' });
  void spacer;
  if (view.admin) {
    const refresh = toolbar.createEl('button', { text: '↻ Перечитать из Базы знаний', cls: 'tn-btn' });
    refresh.addEventListener('click', () => {
      void refreshCards(view, container, refresh);
    });
  } else {
    toolbar.createSpan({
      cls: 'tn-offers-muted',
      text: 'Обновить список может администратор.',
    });
  }

  if (cards.length === 0) {
    container.createDiv({
      cls: 'tn-offers-muted',
      text: 'Карточек пока нет. Создайте заметку по стандарту — она появится здесь после «Перечитать».',
    });
    return;
  }

  const table = container.createEl('table', { cls: 'tn-offers-table' });
  const head = table.createEl('thead').createEl('tr');
  for (const title of ['Система', 'Код', 'Состав', 'Связь с каталогом', 'Правка', '']) {
    head.createEl('th', { text: title });
  }
  const body = table.createEl('tbody');
  for (const card of cards) {
    const row = body.createEl('tr');
    row.createEl('td', { text: card.title || '(без названия)' });
    row.createEl('td', { text: card.sku || '—' });
    row.createEl('td', { text: card.hasComposition ? `${card.composition.length}` : '—' });

    const link = row.createEl('td');
    if (card.catalogId) {
      link.createSpan({ cls: 'tn-offers-ok', text: 'связана' });
    } else {
      link.createSpan({ cls: 'tn-offers-warn', text: card.sku ? 'нет в каталоге' : 'нет кода' });
    }

    row.createEl('td', { text: formatDate(card.updatedAt) });

    const actions = row.createEl('td', { cls: 'tn-offers-context-actions' });
    const open = actions.createEl('button', { text: 'Открыть заметку', cls: 'tn-btn tn-btn-ghost' });
    open.addEventListener('click', () => {
      void openNote(view, card.path);
    });
  }
}

/** Открыть заметку в Obsidian по пути из Базы знаний. */
async function openNote(view: OffersView, path: string): Promise<void> {
  if (!path) {
    new Notice('У карточки нет пути к заметке');
    return;
  }
  // Путь может вести в хранилище без расширения — добавляем .md, как это
  // делает Obsidian для заметок.
  const target = path.endsWith('.md') ? path : `${path}.md`;
  const file = view.app.vault.getAbstractFileByPath(target);
  if (!file) {
    new Notice(`Заметка не найдена: ${path}`);
    return;
  }
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  await view.app.workspace.getLeaf(false).openFile(file as any);
}

async function refreshCards(
  view: OffersView,
  container: HTMLElement,
  button: HTMLButtonElement,
): Promise<void> {
  button.disabled = true;
  button.setText('Читаю…');
  try {
    const res = await view.plugin.offersService.refreshSystemCards();
    if (res.error) {
      new Notice(`База знаний недоступна, показаны прежние карточки: ${res.error}`);
    } else {
      new Notice(`Обновлено карточек: ${res.count}`);
    }
    container.empty();
    render(view, container, res.cards, res.kbEnabled);
  } catch (e: unknown) {
    new Notice(`Не удалось обновить: ${errorMessage(e)}`);
    button.disabled = false;
    button.setText('↻ Перечитать из Базы знаний');
  }
}

/** Дата в коротком виде: 29.10.2026. */
function formatDate(raw: string): string {
  if (!raw) return '—';
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return raw;
  const pad = (n: number): string => String(n).padStart(2, '0');
  return `${pad(d.getDate())}.${pad(d.getMonth() + 1)}.${d.getFullYear()}`;
}