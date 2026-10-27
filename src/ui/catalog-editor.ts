import { Modal, Notice, Setting } from 'obsidian';
import type { OffersView } from './offers-view';
import type { CatalogItem } from '../types/offers';
import { errorMessage } from '../../../sbe-core/src/utils/errors';

const KIND_LABELS: Record<string, string> = {
  system: 'Система',
  material: 'Материал',
  work: 'Работа',
};

export function renderCatalogEditor(view: OffersView, container: HTMLElement): void {
  container.createDiv({ cls: 'tn-offers-muted', text: 'Загрузка каталога…' });
  void load(view, container, '');
}

async function load(view: OffersView, container: HTMLElement, query: string): Promise<void> {
  container.empty();
  const actions = container.createDiv({ cls: 'tn-offers-actions' });
  const search = actions.createEl('input', {
    cls: 'tn-offers-input tn-offers-grow',
    attr: { placeholder: 'Поиск по названию и описанию' },
  });
  search.value = query;
  let timer: number | null = null;
  search.addEventListener('input', () => {
    if (timer !== null) window.clearTimeout(timer);
    timer = window.setTimeout(() => {
      void load(view, container, search.value);
    }, 400);
  });
  if (view.admin) {
    const add = actions.createEl('button', { text: '➕ Карточка', cls: 'tn-btn tn-btn-primary' });
    add.addEventListener('click', () => new CatalogItemModal(view, null, () => void load(view, container, search.value)).open());
  }

  let items: CatalogItem[] = [];
  try {
    items = await view.plugin.offersService.listCatalog(query);
  } catch (e: unknown) {
    container.createDiv({ cls: 'tn-offers-error', text: `Не удалось загрузить каталог: ${errorMessage(e)}` });
    return;
  }
  if (items.length === 0) {
    container.createDiv({
      cls: 'tn-offers-empty',
      text: query ? 'По запросу ничего не нашлось.' : 'Каталог пуст. Администратор ещё не завёл ни одной карточки.',
    });
    return;
  }

  const table = container.createEl('table', { cls: 'tn-offers-table' });
  const head = table.createEl('thead').createEl('tr');
  for (const title of ['Название', 'Тип', 'Ед.', 'Цена', '']) {
    head.createEl('th', { text: title });
  }
  const body = table.createEl('tbody');
  for (const item of items) {
    const row = body.createEl('tr');
    row.createEl('td', { text: item.name });
    row.createEl('td', { text: KIND_LABELS[item.kind] ?? item.kind });
    row.createEl('td', { text: item.unit });
    row.createEl('td', { cls: 'tn-offers-num', text: formatPrice(item.basePrice) });
    const actionsCell = row.createEl('td');
    if (view.admin) {
      const edit = actionsCell.createEl('button', { text: '✏️', cls: 'tn-btn tn-btn-ghost' });
      edit.addEventListener('click', () => new CatalogItemModal(view, item, () => void load(view, container, search.value)).open());
      const del = actionsCell.createEl('button', { text: '🗑', cls: 'tn-btn tn-btn-ghost' });
      del.addEventListener('click', () => {
        void archive(view, item, () => void load(view, container, search.value));
      });
    }
  }
}

async function archive(view: OffersView, item: CatalogItem, after: () => void): Promise<void> {
  try {
    await view.plugin.offersService.deleteCatalog(item.id);
    new Notice(`Карточка «${item.name}» убрана в архив`);
    after();
  } catch (e: unknown) {
    new Notice(`Не удалось убрать карточку: ${errorMessage(e)}`);
  }
}

function formatPrice(value: number): string {
  const cents = Math.round(value * 100);
  const whole = Math.floor(cents / 100).toString().replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
  const frac = (cents % 100).toString().padStart(2, '0');
  return `${whole},${frac} ₽`;
}

class CatalogItemModal extends Modal {
  constructor(
    private view: OffersView,
    private item: CatalogItem | null,
    private after: () => void,
  ) {
    super(view.app);
  }

  onOpen(): void {
    const { contentEl } = this;
    contentEl.empty();
    contentEl.createEl('h3', { text: this.item ? 'Карточка каталога' : 'Новая карточка' });

    const draft: CatalogItem = this.item
      ? { ...this.item }
      : {
          id: 0,
          kind: 'system',
          name: '',
          unit: 'шт',
          basePrice: 0,
          description: '',
          sortOrder: 0,
          archived: false,
          createdAt: '',
          updatedAt: '',
        };

    new Setting(contentEl).setName('Название').addText((text) =>
      text.setValue(draft.name).onChange((v) => {
        draft.name = v;
      }),
    );
    new Setting(contentEl).setName('Тип').addDropdown((dd) => {
      for (const [value, label] of Object.entries(KIND_LABELS)) dd.addOption(value, label);
      dd.setValue(draft.kind).onChange((v) => {
        draft.kind = v as CatalogItem['kind'];
      });
    });
    new Setting(contentEl).setName('Единица').addText((text) =>
      text.setValue(draft.unit).onChange((v) => {
        draft.unit = v;
      }),
    );
    new Setting(contentEl).setName('Базовая цена').addText((text) =>
      text.setValue(String(draft.basePrice)).onChange((v) => {
        draft.basePrice = Number(v.replace(',', '.')) || 0;
      }),
    );
    new Setting(contentEl).setName('Описание').addTextArea((area) => {
      area.setValue(draft.description).onChange((v) => {
        draft.description = v;
      });
      area.inputEl.rows = 3;
    });

    new Setting(contentEl).addButton((btn) =>
      btn
        .setButtonText('Сохранить')
        .setCta()
        .onClick(() => {
          void this.save(draft);
        }),
    );
  }

  private async save(draft: CatalogItem): Promise<void> {
    if (!draft.name.trim()) {
      new Notice('Не заполнено название');
      return;
    }
    try {
      await this.view.plugin.offersService.saveCatalog(draft);
      new Notice('Карточка сохранена');
      this.close();
      this.after();
    } catch (e: unknown) {
      new Notice(`Не удалось сохранить: ${errorMessage(e)}`);
    }
  }

  onClose(): void {
    this.contentEl.empty();
  }
}