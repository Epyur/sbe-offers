import { Notice } from 'obsidian';
import type { OffersView } from './offers-view';
import type { SystemCard } from '../types/offers';
import { errorMessage } from '../../../sbe-core/src/utils/errors';
import { openNote } from './context-editor';

/**
 * «Конкуренты» (2026-11-08) — карточки продуктов конкурентов из папки Базы
 * знаний «материалы и системы конкурентов».
 *
 * Раздел ТОЛЬКО ДЛЯ ЧТЕНИЯ: показывает то, что сервис уже прочитал из Базы
 * знаний. «Перечитать из Базы знаний» — отдельное действие (admin), как у
 * систем. Карточку конкурента выбирают в боевом листке — здесь видно, что есть
 * в источнике и свежо ли это.
 *
 * Папка — не тег: у карточек конкурентов своего тега нет, они лежат в разделе.
 * Поэтому «папки нет» — законное состояние, а не ошибка: боевой листок всё
 * равно собирается.
 */
export function renderCompetitorsEditor(view: OffersView, container: HTMLElement): void {
  container.createDiv({ cls: 'tn-offers-muted', text: 'Читаю конкурентов…' });
  void load(view, container);
}

async function load(view: OffersView, container: HTMLElement, refresh = false): Promise<void> {
  let cards: SystemCard[] = [];
  let kbEnabled = true;
  let folderFound = true;
  let error = '';
  try {
    const res = refresh
      ? await view.plugin.offersService.refreshCompetitors()
      : await view.plugin.offersService.listCompetitors();
    cards = res.cards ?? [];
    kbEnabled = res.kbEnabled;
    folderFound = res.folderFound;
    error = res.error ?? '';
    if (refresh) {
      new Notice(
        error
          ? `База знаний недоступна, показан прежний список: ${error}`
          : folderFound
            ? `Обновлено карточек конкурентов: ${cards.length}`
            : 'В Базе знаний нет папки «материалы и системы конкурентов»',
      );
    }
  } catch (e: unknown) {
    container.empty();
    container.createDiv({ cls: 'tn-offers-error', text: `Не удалось загрузить конкурентов: ${errorMessage(e)}` });
    return;
  }
  container.empty();
  render(view, container, cards, kbEnabled, folderFound, error);
}

function render(
  view: OffersView,
  container: HTMLElement,
  cards: SystemCard[],
  kbEnabled: boolean,
  folderFound: boolean,
  error: string,
): void {
  const hint = container.createDiv({ cls: 'tn-offers-card' });
  hint.createDiv({
    text:
      'Продукты конкурентов берутся из Базы знаний — из папки «материалы и системы конкурентов». ' +
      'Они используются в боевом листке как сторона сравнения.',
  });
  if (!kbEnabled) {
    container.createDiv({
      cls: 'tn-offers-error',
      text: 'База знаний не настроена (KB_SERVICE_URL). Конкуренты недоступны.',
    });
    return;
  }
  if (error) {
    container.createDiv({ cls: 'tn-offers-error', text: `База знаний ответила ошибкой: ${error}` });
  }

  const toolbar = container.createDiv({ cls: 'tn-offers-actions' });
  toolbar.createSpan({ cls: 'tn-offers-muted tn-offers-self-center', text: `Карточек: ${cards.length}` });
  if (view.admin) {
    const reload = toolbar.createEl('button', { text: '↻ Перечитать из Базы знаний', cls: 'tn-btn tn-btn-ghost' });
    reload.addEventListener('click', () => {
      reload.disabled = true;
      reload.setText('Читаю…');
      container.empty();
      container.createDiv({ cls: 'tn-offers-muted', text: 'Перечитываю карточки конкурентов из Базы знаний…' });
      void load(view, container, true);
    });
  }

  if (!folderFound) {
    container.createDiv({
      cls: 'tn-offers-empty',
      text: 'В Базе знаний нет папки «материалы и системы конкурентов». Заведите её и карточки в ней.',
    });
    return;
  }
  if (cards.length === 0) {
    container.createDiv({ cls: 'tn-offers-empty', text: 'Карточек конкурентов пока нет.' });
    return;
  }

  for (const card of cards) {
    renderCard(view, container, card);
  }
}

function renderCard(view: OffersView, container: HTMLElement, card: SystemCard): void {
  const box = container.createDiv({ cls: 'tn-offers-card tn-offers-system-card' });
  const head = box.createDiv({ cls: 'tn-offers-row-between' });
  const titleWrap = head.createDiv();
  titleWrap.createDiv({ cls: 'tn-offers-card-title', text: card.title || '(без названия)' });
  if (card.sku) {
    titleWrap.createDiv({ cls: 'tn-offers-muted', text: `Код: ${card.sku}` });
  }
  if (card.description) {
    box.createDiv({ cls: 'tn-offers-muted', text: card.description.slice(0, 400) });
  }
  if ((card.composition ?? []).length > 0) {
    const list = box.createEl('ul', { cls: 'tn-offers-problems' });
    for (const line of card.composition) {
      list.createEl('li', { text: line });
    }
  }
  if (card.path) {
    const footer = box.createDiv({ cls: 'tn-offers-row tn-offers-system-footer' });
    const open = footer.createEl('button', { text: 'Открыть заметку', cls: 'tn-btn tn-btn-ghost' });
    open.addEventListener('click', () => {
      void openNote(view, card.path);
    });
  }
}