import { Modal, Notice, Setting } from 'obsidian';
import type { OffersView } from './offers-view';
import type { Material, MaterialEkn } from '../types/offers';
import { errorMessage } from '../../../sbe-core/src/utils/errors';
import { formatMoney } from '../utils/money';
import { formatShortDate } from '../utils/date';

/**
 * «Материалы» — то, что продаётся (2026-10-31).
 *
 * Прежний «Каталог» смешивал системы, материалы и работы, а у каждой строки
 * была базовая цена. Теперь материалам своя таблица, у каждого собственный SKU,
 * который назначает сервер, и вариации ЕКН. **Колонки цены нет**: цена приходит
 * из таблицы менеджера для конкретного КП, здесь — только вилка для аналитики.
 *
 * Материалы заводятся из карточек Базы знаний с тегом material: сервер делает
 * это при чтении списка, и тогда вики-ссылка из состава системы находит нужный
 * материал. Поэтому в списке есть пометка «из карточки».
 *
 * Системы сюда не попадают по построению: они живут карточками Базы знаний,
 * раздел «Системы».
 */
export function renderMaterialEditor(view: OffersView, container: HTMLElement): void {
  container.createDiv({ cls: 'tn-offers-muted', text: 'Загрузка материалов…' });
  void load(view, container, { query: '', archived: false });
}

interface MaterialFilter {
  query: string;
  archived: boolean;
}

async function load(view: OffersView, container: HTMLElement, filter: MaterialFilter): Promise<void> {
  container.empty();
  const actions = container.createDiv({ cls: 'tn-offers-actions' });
  const search = actions.createEl('input', {
    cls: 'tn-offers-input tn-offers-grow',
    attr: { placeholder: 'Поиск по названию, SKU и ЕКН' },
  });
  search.value = filter.query;
  let timer: number | null = null;
  search.addEventListener('input', () => {
    if (timer !== null) window.clearTimeout(timer);
    timer = window.setTimeout(() => {
      filter = { ...filter, query: search.value };
      void load(view, container, filter);
    }, 400);
  });

  const archiveBox = actions.createEl('label', { cls: 'tn-offers-check' });
  const archiveInput = archiveBox.createEl('input', { attr: { type: 'checkbox' } });
  archiveInput.checked = filter.archived;
  archiveBox.createSpan({ text: ' Показать архив' });
  archiveInput.addEventListener('change', () => {
    filter = { ...filter, archived: archiveInput.checked };
    void load(view, container, filter);
  });

  // Ручное заведение осталось: работа без карточки (монтаж, доставка) и
  // материал, которого в Базе знаний нет, — законные случаи.
  if (view.admin) {
    const add = actions.createEl('button', { text: '➕ Материал', cls: 'tn-btn tn-btn-primary' });
    add.addEventListener('click', () => {
      new MaterialModal(view, null, () => void load(view, container, filter)).open();
    });
  }

  let items: Material[] = [];
  try {
    items = await view.plugin.offersService.listMaterials(filter.archived);
  } catch (e: unknown) {
    container.createDiv({ cls: 'tn-offers-error', text: `Не удалось загрузить материалы: ${errorMessage(e)}` });
    return;
  }
  const shown = filterByQuery(items, filter.query);
  if (shown.length === 0) {
    container.createDiv({
      cls: 'tn-offers-empty',
      text: filter.query
        ? 'По запросу ничего не нашлось.'
        : 'Материалов пока нет. Заведите карточку с тегом material в Базе знаний или добавьте материал вручную.',
    });
    return;
  }

  const table = container.createEl('table', { cls: 'tn-offers-table' });
  const head = table.createEl('thead').createEl('tr');
  for (const title of ['Название и код', 'Ед.', 'Вилка цен', 'ЕКН', '']) {
    head.createEl('th', { text: title });
  }
  const body = table.createEl('tbody');
  for (const item of shown) {
    const row = body.createEl('tr');
    if (item.archived) row.addClass('tn-offers-row-archived');

    const nameCell = row.createEl('td');
    nameCell.createDiv({ text: item.name });
    const codeLine = nameCell.createDiv({ cls: 'tn-offers-muted tn-offers-mono' });
    codeLine.createSpan({ text: item.sku });
    if (item.noteTitle) {
      codeLine.createSpan({ cls: 'tn-offers-chip', text: 'из карточки' });
    }

    row.createEl('td', { text: item.unit });
    row.createEl('td', { cls: 'tn-offers-num', text: priceRangeText(item) });
    row.createEl('td', { cls: 'tn-offers-num', text: String(item.ekn.length) });

    const cell = row.createEl('td');
    const open = cell.createEl('button', { text: 'Карточка', cls: 'tn-btn tn-btn-ghost' });
    open.addEventListener('click', () => {
      new MaterialModal(view, item, () => void load(view, container, filter)).open();
    });
  }
}

/** Вилка цен словами. Пустая вилка — «цена не загружалась», а не ноль. */
export function priceRangeText(item: Material): string {
  const min = item.priceMin ?? null;
  const max = item.priceMax ?? null;
  if (min === null && max === null) return 'цена не загружалась';
  if (min !== null && max !== null && min !== max) {
    return `${formatMoney(min)} – ${formatMoney(max)} ₽`;
  }
  return `${formatMoney(min ?? max ?? 0)} ₽`;
}

/**
 * Отбор для поиска.
 *
 * Ищем по названию, коду и номерам ЕКН: менеджер ищет материал по бумаге, где
 * написан ЕКН, а не по названию в справочнике. Регистр и лишние пробелы не
 * важны — это то, что человек набирает руками.
 */
export function filterByQuery(items: Material[], query: string): Material[] {
  const q = query.trim().toLowerCase().replace(/\s+/g, ' ');
  if (!q) return items;
  return items.filter((item) => {
    const haystack = [item.name, item.sku, item.noteTitle ?? '', ...item.ekn.map((e) => e.ekn)]
      .join(' ')
      .toLowerCase()
      .replace(/\s+/g, ' ');
    return haystack.includes(q);
  });
}

/** Карточка материала: поля справочника и таблица вариаций ЕКН. */
class MaterialModal extends Modal {
  private draft: Material;
  private typeCode = '';
  private series = '';

  constructor(
    private view: OffersView,
    private item: Material | null,
    private after: () => void,
  ) {
    super(view.app);
    this.draft = item
      ? { ...item, ekn: [...item.ekn] }
      : {
          id: 0,
          sku: '',
          kind: 'material',
          name: '',
          unit: 'шт',
          description: '',
          priceMin: null,
          priceMax: null,
          photoKey: '',
          sortOrder: 0,
          archived: false,
          createdAt: '',
          updatedAt: '',
          ekn: [],
        };
  }

  onOpen(): void {
    this.render();
  }

  private render(): void {
    const { contentEl } = this;
    contentEl.empty();
    contentEl.createEl('h3', { text: this.draft.id ? 'Материал' : 'Новый материал' });

    // SKU показывается, но не редактируется: это личность материала, а не
    // ярлык. Сервер назначит код при первом сохранении.
    contentEl.createDiv({
      cls: 'tn-offers-muted tn-offers-mono',
      text: this.draft.sku ? `Код: ${this.draft.sku}` : 'Код материала назначит сервер при сохранении',
    });
    if (this.draft.noteTitle) {
      contentEl.createDiv({
        cls: 'tn-offers-muted',
        text: `Из карточки Базы знаний: ${this.draft.noteTitle}`,
      });
    }

    new Setting(contentEl).setName('Название').addText((text) => {
      text.setValue(this.draft.name).onChange((v) => {
        this.draft.name = v;
      });
    });
    new Setting(contentEl)
      .setName('Вид')
      .setDesc('Работа продаётся так же, но вариаций ЕКН у неё не бывает.')
      .addDropdown((dd) => {
        dd.addOption('material', 'Материал');
        dd.addOption('work', 'Работа');
        dd.setValue(this.draft.kind).onChange((v) => {
          this.draft.kind = v === 'work' ? 'work' : 'material';
        });
      });
    new Setting(contentEl).setName('Единица').addText((text) => {
      text.setValue(this.draft.unit).onChange((v) => {
        this.draft.unit = v;
      });
    });
    new Setting(contentEl).setName('Описание').addTextArea((area) => {
      area.setValue(this.draft.description).onChange((v) => {
        this.draft.description = v;
      });
      area.inputEl.rows = 3;
    });

    if (!this.draft.id) {
      contentEl.createDiv({ cls: 'tn-offers-muted', text: 'Из каких букв собрать код:' });
      new Setting(contentEl)
        .setName('Тип')
        .setDesc('Короткое слово: МЕМБ, УТЕПЛ, КРЕП, ГРУНТ, ПЛИТ.')
        .addText((text) => {
          text.setValue(this.typeCode).onChange((v) => {
            this.typeCode = v;
          });
        });
      new Setting(contentEl)
        .setName('Серия')
        .setDesc('ПВХ, ПИР, ЭПП — первые три знака попадут в код.')
        .addText((text) => {
          text.setValue(this.series).onChange((v) => {
            this.series = v;
          });
        });
    }

    contentEl.createEl('h4', { text: 'Вариации (ЕКН)' });
    this.renderEkn(contentEl);

    new Setting(contentEl)
      .addButton((btn) =>
        btn.setButtonText('Сохранить').setCta().onClick(() => {
          void this.save();
        }),
      )
      .addButton((btn) =>
        btn.setButtonText('Закрыть').onClick(() => {
          this.close();
          this.after();
        }),
      );

    if (this.draft.id && !this.draft.archived) {
      new Setting(contentEl)
        .setName('Архив')
        .setDesc('Удаление не стирает материал: на него ссылаются старые предложения.')
        .addButton((btn) =>
          btn.setButtonText('🗄 Убрать в архив').setWarning().onClick(() => {
            void this.archive();
          }),
        );
    }
  }

  private renderEkn(container: HTMLElement): void {
    if (!this.draft.id) {
      container.createDiv({
        cls: 'tn-offers-muted',
        text: 'Вариации добавляются после сохранения материала и при загрузке таблицы менеджера.',
      });
      return;
    }
    if (this.draft.ekn.length === 0) {
      container.createDiv({ cls: 'tn-offers-muted', text: 'Вариаций пока нет.' });
    } else {
      const table = container.createEl('table', { cls: 'tn-offers-table' });
      const head = table.createEl('thead').createEl('tr');
      for (const title of ['ЕКН', 'Вариант', 'Откуда', 'Когда видели', '']) {
        head.createEl('th', { text: title });
      }
      const body = table.createEl('tbody');
      for (const ekn of this.draft.ekn) {
        const row = body.createEl('tr');
        row.createEl('td', { cls: 'tn-offers-mono', text: ekn.ekn });
        row.createEl('td', { text: ekn.variant || '—' });
        row.createEl('td', { text: ekn.source === 'excel' ? 'таблица' : 'вручную' });
        row.createEl('td', { text: formatShortDate(ekn.lastSeenAt) });
        const del = row.createEl('td').createEl('button', { text: '✖', cls: 'tn-btn tn-btn-ghost' });
        del.addEventListener('click', () => {
          void this.removeEkn(ekn);
        });
      }
    }
    const addRow = container.createDiv({ cls: 'tn-offers-row tn-offers-ekn-add' });
    const eknInput = addRow.createEl('input', {
      cls: 'tn-offers-input tn-offers-ekn-code',
      attr: { placeholder: 'ЕКН' },
    });
    const variantInput = addRow.createEl('input', {
      cls: 'tn-offers-input tn-offers-grow',
      attr: { placeholder: 'Вариант: 1,5 мм, серый' },
    });
    const add = addRow.createEl('button', { text: '➕ Добавить', cls: 'tn-btn tn-btn-ghost' });
    add.addEventListener('click', () => {
      void this.addEkn(eknInput, variantInput);
    });
  }

  private async save(): Promise<void> {
    if (!this.draft.name.trim()) {
      new Notice('Не заполнено название материала');
      return;
    }
    try {
      const saved = await this.view.plugin.offersService.saveMaterial({
        ...this.draft,
        typeCode: this.typeCode,
        series: this.series,
      });
      this.draft = { ...saved, ekn: saved.ekn ?? [] };
      new Notice(this.item ? 'Материал сохранён' : `Материал заведён: ${saved.sku}`);
      this.after();
      this.render();
    } catch (e: unknown) {
      new Notice(`Не удалось сохранить: ${errorMessage(e)}`);
    }
  }

  private async addEkn(codeInput: HTMLInputElement, variantInput: HTMLInputElement): Promise<void> {
    const code = codeInput.value.trim();
    if (!code) {
      new Notice('Не указан ЕКН');
      return;
    }
    // Значения забираем ДО сброса: input.value — живое поле, и очистка ниже
    // иначе обнулила бы то, что ещё не ушло на сервер.
    const variant = variantInput.value.trim();
    try {
      const saved = await this.view.plugin.offersService.addMaterialEkn(this.draft.id, code, variant);
      this.draft = { ...saved, ekn: saved.ekn ?? [] };
      this.after();
      this.render();
    } catch (e: unknown) {
      new Notice(`Не удалось добавить ЕКН: ${errorMessage(e)}`);
    }
  }

  private async removeEkn(ekn: MaterialEkn): Promise<void> {
    try {
      const saved = await this.view.plugin.offersService.deleteMaterialEkn(this.draft.id, ekn.id);
      this.draft = { ...saved, ekn: saved.ekn ?? [] };
      this.after();
      this.render();
    } catch (e: unknown) {
      new Notice(`Не удалось убрать ЕКН: ${errorMessage(e)}`);
    }
  }

  private async archive(): Promise<void> {
    try {
      await this.view.plugin.offersService.archiveMaterial(this.draft.id);
      new Notice(`Материал «${this.draft.name}» в архиве`);
      this.close();
      this.after();
    } catch (e: unknown) {
      new Notice(`Не удалось убрать в архив: ${errorMessage(e)}`);
    }
  }

  onClose(): void {
    this.contentEl.empty();
  }
}