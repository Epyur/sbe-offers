import { Notice } from 'obsidian';
import type { OffersView } from './offers-view';
import { canEditOffers, type CatalogItem, type Offer, type OfferItem } from '../types/offers';
import { errorMessage } from '../../../sbe-core/src/utils/errors';
import { formatMoney, itemsTotal } from '../utils/money';

export function renderOffersList(view: OffersView, container: HTMLElement): void {
  container.createDiv({ cls: 'tn-offers-muted', text: 'Загрузка предложений…' });
  void load(view, container);
}

async function load(view: OffersView, container: HTMLElement): Promise<void> {
  container.empty();
  const actions = container.createDiv({ cls: 'tn-offers-actions' });
  if (canEditOffers(view.role)) {
    const create = actions.createEl('button', { text: '➕ Новое предложение', cls: 'tn-btn tn-btn-primary' });
    create.addEventListener('click', () => {
      void openEditor(view, container, null);
    });
  }
  const reload = actions.createEl('button', { text: '↻ Обновить', cls: 'tn-btn tn-btn-ghost' });
  reload.addEventListener('click', () => {
    void load(view, container);
  });

  if (view.role === '') {
    container.createDiv({
      cls: 'tn-offers-empty',
      text: 'Нет доступа к приложению. Права выдаёт администратор в ЦУП.',
    });
    return;
  }

  let offers: Offer[] = [];
  try {
    offers = await view.plugin.offersService.listOffers();
  } catch (e: unknown) {
    container.createDiv({ cls: 'tn-offers-error', text: `Не удалось загрузить предложения: ${errorMessage(e)}` });
    return;
  }
  if (offers.length === 0) {
    container.createDiv({ cls: 'tn-offers-empty', text: 'Предложений пока нет.' });
    return;
  }

  const table = container.createEl('table', { cls: 'tn-offers-table' });
  const head = table.createEl('thead').createEl('tr');
  for (const title of ['Номер', 'Клиент', 'Сумма', 'Статус', 'Менеджер', '']) {
    head.createEl('th', { text: title });
  }
  const body = table.createEl('tbody');
  for (const offer of offers) {
    const row = body.createEl('tr');
    const numberCell = row.createEl('td');
    const link = numberCell.createEl('a', { text: offer.number, cls: 'tn-offers-link' });
    link.addEventListener('click', () => {
      void openEditor(view, container, offer);
    });
    row.createEl('td', { text: offer.client?.name ?? '—' });
    row.createEl('td', { cls: 'tn-offers-num', text: `${formatMoney(offer.total)} ₽` });
    const statusCell = row.createEl('td');
    statusCell.createSpan({
      cls: 'tn-offers-chip',
      text: offer.status === 'final' ? 'Готово' : 'Черновик',
    });
    row.createEl('td', { cls: 'tn-offers-muted', text: offer.ownerEmail });
    const act = row.createEl('td');
    const docBtn = act.createEl('button', { text: '⬇ DOCX', cls: 'tn-btn tn-btn-ghost' });
    docBtn.addEventListener('click', () => {
      void downloadDocx(view, offer);
    });
  }
}

async function downloadDocx(view: OffersView, offer: Offer): Promise<void> {
  try {
    const fileName = `Коммерческое предложение ${offer.number.replace('/', '-')}.docx`;
    const bytes = await view.plugin.offersService.exportDocx(offer.id);
    const folder = 'yourbase/sbe_offers/exports';
    const adapter = view.app.vault.adapter;
    if (!(await adapter.exists(folder))) await adapter.mkdir(folder);
    const path = `${folder}/${fileName}`;
    const buffer = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
    await adapter.writeBinary(path, buffer);
    new Notice(`Файл сохранён: ${path}`);
  } catch (e: unknown) {
    new Notice(`Не удалось получить файл: ${errorMessage(e)}`);
  }
}

async function openEditor(view: OffersView, container: HTMLElement, offer: Offer | null): Promise<void> {
  let current: Offer;
  let catalog: CatalogItem[] = [];
  try {
    if (offer) {
      current = await view.plugin.offersService.getOffer(offer.id);
    } else {
      current = {
        id: 0,
        number: '',
        ownerEmail: view.email,
        client: { name: '', contact: '' },
        items: [],
        formValues: {},
        total: 0,
        status: 'draft',
        docxKey: '',
        createdAt: '',
        updatedAt: '',
      };
    }
    catalog = await view.plugin.offersService.listCatalog('');
  } catch (e: unknown) {
    new Notice(`Не удалось открыть предложение: ${errorMessage(e)}`);
    return;
  }

  container.empty();
  const back = container.createEl('button', { text: '← К списку', cls: 'tn-btn tn-btn-ghost' });
  back.addEventListener('click', () => {
    void load(view, container);
  });

  const card = container.createDiv({ cls: 'tn-offers-card' });
  card.createDiv({ cls: 'tn-offers-muted', text: 'Клиент' });
  const clientName = card.createEl('input', { cls: 'tn-offers-input', attr: { placeholder: 'Организация' } });
  clientName.value = current.client.name ?? '';
  const clientContact = card.createEl('input', { cls: 'tn-offers-input', attr: { placeholder: 'Контактное лицо' } });
  clientContact.value = current.client.contact ?? '';

  const itemsCard = container.createDiv({ cls: 'tn-offers-card' });
  itemsCard.createDiv({ cls: 'tn-offers-muted', text: 'Позиции' });
  const itemsEl = itemsCard.createDiv();
  let items: OfferItem[] = current.items.map((i) => ({ ...i }));

  const redrawItems = (): void => {
    itemsEl.empty();
    if (items.length === 0) {
      itemsEl.createDiv({ cls: 'tn-offers-muted', text: 'Позиций пока нет — добавьте из каталога или впишите вручную.' });
    }
    items.forEach((item, index) => {
      const row = itemsEl.createDiv({ cls: 'tn-offers-row' });
      const name = row.createEl('input', { cls: 'tn-offers-input tn-offers-grow' });
      name.value = item.name;
      name.addEventListener('input', () => {
        item.name = name.value;
      });
      const unit = row.createEl('input', { cls: 'tn-offers-input tn-offers-unit' });
      unit.value = item.unit;
      unit.addEventListener('input', () => {
        item.unit = unit.value;
      });
      const qty = row.createEl('input', { cls: 'tn-offers-input tn-offers-num-input', attr: { type: 'number', min: '0', step: '0.01' } });
      qty.value = String(item.qty);
      qty.addEventListener('input', () => {
        item.qty = Number(qty.value) || 0;
        sumsEl.setText(totalText(items));
      });
      const price = row.createEl('input', { cls: 'tn-offers-input tn-offers-num-input', attr: { type: 'number', min: '0', step: '0.01' } });
      price.value = String(item.price);
      price.addEventListener('input', () => {
        item.price = Number(price.value) || 0;
        sumsEl.setText(totalText(items));
      });
      const del = row.createEl('button', { text: '✖', cls: 'tn-btn tn-btn-ghost' });
      del.addEventListener('click', () => {
        items.splice(index, 1);
        redrawItems();
      });
    });
    sumsEl.setText(totalText(items));
  };

  const sumsEl = itemsCard.createDiv({ cls: 'tn-offers-total' });

  const catalogSelect = itemsCard.createEl('select', { cls: 'tn-offers-input' });
  catalogSelect.createEl('option', { value: '', text: '— выбрать карточку каталога —' });
  for (const card of catalog) {
    catalogSelect.createEl('option', { value: String(card.id), text: `${card.name} (${formatMoney(card.basePrice)} / ${card.unit})` });
  }
  const addFromCatalog = itemsCard.createEl('button', { text: '➕ Добавить из каталога', cls: 'tn-btn tn-btn-ghost' });
  addFromCatalog.addEventListener('click', () => {
    const id = Number(catalogSelect.value);
    const found = catalog.find((c) => c.id === id);
    if (!found) return;
    items.push({
      catalogId: found.id,
      name: found.name,
      unit: found.unit,
      qty: 1,
      price: found.basePrice,
      sum: found.basePrice,
    });
    redrawItems();
  });
  const addManual = itemsCard.createEl('button', { text: '✏️ Своя позиция', cls: 'tn-btn tn-btn-ghost' });
  addManual.addEventListener('click', () => {
    items.push({ name: '', unit: 'шт', qty: 1, price: 0, sum: 0 });
    redrawItems();
  });
  redrawItems();

  const formCard = container.createDiv({ cls: 'tn-offers-card' });
  formCard.createDiv({ cls: 'tn-offers-muted', text: 'Поля формы' });
  const formValues: Record<string, string> = { ...(current.formValues ?? {}) };
  try {
    const form = await view.plugin.offersService.getForm('client');
    for (const p of form.placeholders) {
      const row = formCard.createDiv({ cls: 'tn-offers-row' });
      row.createSpan({ cls: 'tn-offers-form-label', text: p.label });
      const input = row.createEl('input', { cls: 'tn-offers-input tn-offers-grow' });
      input.value = formValues[p.id] ?? '';
      if (form.requiredFields.includes(p.id)) input.addClass('is-required');
      input.addEventListener('input', () => {
        formValues[p.id] = input.value;
      });
    }
  } catch (e: unknown) {
    formCard.createDiv({ cls: 'tn-offers-error', text: `Не удалось загрузить форму: ${errorMessage(e)}` });
  }

  const actions = container.createDiv({ cls: 'tn-offers-actions' });
  const saveBtn = actions.createEl('button', { text: '💾 Сохранить', cls: 'tn-btn tn-btn-primary' });
  const finBtn = actions.createEl('button', { text: '✅ Сохранить как готовое', cls: 'tn-btn tn-btn-ghost' });
  const docxBtn = actions.createEl('button', { text: '⬇ DOCX', cls: 'tn-btn tn-btn-ghost' });
  const linkBtn = actions.createEl('button', { text: '🔗 Ссылка', cls: 'tn-btn tn-btn-ghost' });

  const collect = (status: Offer['status']): Offer => ({
    ...current,
    client: { name: clientName.value.trim(), contact: clientContact.value.trim() },
    items,
    formValues,
    status,
  });

  saveBtn.addEventListener('click', () => {
    void doSave('draft');
  });
  finBtn.addEventListener('click', () => {
    void doSave('final');
  });
  docxBtn.addEventListener('click', () => {
    if (!current.id) {
      new Notice('Сначала сохраните предложение');
      return;
    }
    void downloadDocx(view, current);
  });
  linkBtn.addEventListener('click', () => {
    if (!current.id) {
      new Notice('Сначала сохраните предложение');
      return;
    }
    void makeLink();
  });

  async function doSave(status: Offer['status']): Promise<void> {
    if (items.length === 0) {
      new Notice('В предложении нет позиций');
      return;
    }
    try {
      current = await view.plugin.offersService.saveOffer(collect(status));
      new Notice(`Предложение ${current.number} сохранено`);
      await load(view, container);
    } catch (e: unknown) {
      new Notice(`Не удалось сохранить: ${errorMessage(e)}`);
    }
  }

  async function makeLink(): Promise<void> {
    try {
      const res = await view.plugin.offersService.offerLink(current.id);
      await navigator.clipboard.writeText(res.url);
      new Notice('Ссылка скопирована (действует 48 часов)');
    } catch (e: unknown) {
      new Notice(`Не удалось получить ссылку: ${errorMessage(e)}`);
    }
  }
}

function totalText(items: OfferItem[]): string {
  return `Итого: ${formatMoney(itemsTotal(items))} ₽`;
}
