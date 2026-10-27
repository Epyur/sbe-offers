import { Notice } from 'obsidian';
import type { OffersView } from './offers-view';
import type { OfferForm, Placeholder } from '../types/offers';
import { renderBlockEditor } from './block-editor';
import { errorMessage } from '../../../sbe-core/src/utils/errors';

/**
 * Конфигуратор формы КП (раздел «⚙️ Форма КП», только admin): заголовок,
 * поля-плейсхолдеры, блоки документа и обязательные поля.
 */
export function renderFormEditor(view: OffersView, container: HTMLElement): void {
  container.createDiv({ cls: 'tn-offers-muted', text: 'Загрузка формы…' });
  void load(view, container);
}

async function load(view: OffersView, container: HTMLElement): Promise<void> {
  let form: OfferForm;
  try {
    form = await view.plugin.offersService.getForm('client');
  } catch (e: unknown) {
    container.empty();
    container.createDiv({ cls: 'tn-offers-error', text: `Не удалось загрузить форму: ${errorMessage(e)}` });
    return;
  }
  container.empty();
  render(view, container, form);
}

function render(view: OffersView, container: HTMLElement, form: OfferForm): void {
  const placeholders: Placeholder[] = form.placeholders.map((p) => ({ ...p }));
  const blocks = form.blocks.map((b) => ({ ...b, content: JSON.parse(JSON.stringify(b.content)) }));
  const required = new Set(form.requiredFields);

  const titleRow = container.createDiv({ cls: 'tn-offers-card' });
  titleRow.createDiv({ cls: 'tn-offers-muted', text: 'Заголовок документа' });
  const titleInput = titleRow.createEl('input', { cls: 'tn-offers-input' });
  titleInput.value = form.title;

  const fieldsCard = container.createDiv({ cls: 'tn-offers-card' });
  fieldsCard.createDiv({ cls: 'tn-offers-muted', text: 'Поля формы (подставляются вместо {{поле}})' });
  const fieldsList = fieldsCard.createDiv();
  const drawFields = (): void => {
    fieldsList.empty();
    placeholders.forEach((p, i) => {
      const row = fieldsList.createDiv({ cls: 'tn-offers-row' });
      const idInput = row.createEl('input', { cls: 'tn-offers-input tn-offers-grow', attr: { placeholder: 'id (латиницей)' } });
      idInput.value = p.id;
      idInput.addEventListener('change', () => {
        p.id = idInput.value.trim().replace(/[^a-zA-Z0-9_]/g, '_');
        idInput.value = p.id;
      });
      const labelInput = row.createEl('input', { cls: 'tn-offers-input tn-offers-grow', attr: { placeholder: 'подпись для менеджера' } });
      labelInput.value = p.label;
      labelInput.addEventListener('input', () => {
        p.label = labelInput.value;
      });
      const reqLabel = row.createEl('label');
      const reqBox = reqLabel.createEl('input', { attr: { type: 'checkbox' } });
      reqBox.checked = required.has(p.id);
      reqBox.addEventListener('change', () => {
        if (reqBox.checked) required.add(p.id);
        else required.delete(p.id);
      });
      reqLabel.createSpan({ text: ' обязательное' });
      const del = row.createEl('button', { text: '✖', cls: 'tn-btn tn-btn-ghost' });
      del.addEventListener('click', () => {
        placeholders.splice(i, 1);
        required.delete(p.id);
        drawFields();
      });
    });
  };
  drawFields();
  const addField = fieldsCard.createEl('button', { text: '➕ Поле', cls: 'tn-btn tn-btn-ghost' });
  addField.addEventListener('click', () => {
    placeholders.push({ id: `field_${placeholders.length + 1}`, label: 'Новое поле' });
    drawFields();
  });

  const blocksCard = container.createDiv({ cls: 'tn-offers-card' });
  blocksCard.createDiv({ cls: 'tn-offers-muted', text: 'Блоки документа' });
  renderBlockEditor(blocksCard, blocks, { placeholders });

  const actions = container.createDiv({ cls: 'tn-offers-actions' });
  const saveBtn = actions.createEl('button', { text: '💾 Сохранить форму', cls: 'tn-btn tn-btn-primary' });
  saveBtn.addEventListener('click', () => {
    void save();
  });
  const reloadBtn = actions.createEl('button', { text: '↻ Отменить правки', cls: 'tn-btn tn-btn-ghost' });
  reloadBtn.addEventListener('click', () => {
    void load(view, container);
  });

  async function save(): Promise<void> {
    const payload: OfferForm = {
      ...form,
      title: titleInput.value.trim() || 'Коммерческое предложение',
      placeholders,
      blocks,
      requiredFields: Array.from(required),
    };
    try {
      await view.plugin.offersService.saveForm(payload);
      new Notice('Форма КП сохранена');
      await load(view, container);
    } catch (e: unknown) {
      new Notice(`Не удалось сохранить форму: ${errorMessage(e)}`);
    }
  }
}