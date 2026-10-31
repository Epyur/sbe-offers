import { Modal, Notice, Setting } from 'obsidian';
import type { OffersView } from './offers-view';
import type { Material, MaterialEkn } from '../types/offers';
import { errorMessage } from '../../../sbe-core/src/utils/errors';
import { formatMoney } from '../utils/money';
import { formatShortDate } from '../utils/date';

/**
 * В«РњР°С‚РµСЂРёР°Р»С‹В» вЂ” С‚Рѕ, С‡С‚Рѕ РїСЂРѕРґР°С‘С‚СЃСЏ (2026-10-31).
 *
 * РџСЂРµР¶РЅРёР№ В«РљР°С‚Р°Р»РѕРіВ» СЃРјРµС€РёРІР°Р» СЃРёСЃС‚РµРјС‹, РјР°С‚РµСЂРёР°Р»С‹ Рё СЂР°Р±РѕС‚С‹, Р° Сѓ РєР°Р¶РґРѕР№ СЃС‚СЂРѕРєРё Р±С‹Р»Р°
 * Р±Р°Р·РѕРІР°СЏ С†РµРЅР°. РўРµРїРµСЂСЊ РјР°С‚РµСЂРёР°Р»Р°Рј СЃРІРѕСЏ С‚Р°Р±Р»РёС†Р°, Сѓ РєР°Р¶РґРѕРіРѕ СЃРѕР±СЃС‚РІРµРЅРЅС‹Р№ SKU,
 * РєРѕС‚РѕСЂС‹Р№ РЅР°Р·РЅР°С‡Р°РµС‚ СЃРµСЂРІРµСЂ, Рё РІР°СЂРёР°С†РёРё Р•РљРќ. **РљРѕР»РѕРЅРєРё С†РµРЅС‹ РЅРµС‚**: С†РµРЅР° РїСЂРёС…РѕРґРёС‚
 * РёР· С‚Р°Р±Р»РёС†С‹ РјРµРЅРµРґР¶РµСЂР° РґР»СЏ РєРѕРЅРєСЂРµС‚РЅРѕРіРѕ РљРџ, Р·РґРµСЃСЊ вЂ” С‚РѕР»СЊРєРѕ РІРёР»РєР° РґР»СЏ Р°РЅР°Р»РёС‚РёРєРё.
 *
 * РЎРёСЃС‚РµРјС‹ СЃСЋРґР° РЅРµ РїРѕРїР°РґР°СЋС‚ РїРѕ РїРѕСЃС‚СЂРѕРµРЅРёСЋ: РѕРЅРё Р¶РёРІСѓС‚ РєР°СЂС‚РѕС‡РєР°РјРё Р‘Р°Р·С‹ Р·РЅР°РЅРёР№,
 * СЂР°Р·РґРµР» В«РЎРёСЃС‚РµРјС‹В».
 */
export function renderMaterialEditor(view: OffersView, container: HTMLElement): void {
  container.createDiv({ cls: 'tn-offers-muted', text: 'Р—Р°РіСЂСѓР·РєР° РјР°С‚РµСЂРёР°Р»РѕРІвЂ¦' });
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
    attr: { placeholder: 'РџРѕРёСЃРє РїРѕ РЅР°Р·РІР°РЅРёСЋ, SKU Рё Р•РљРќ' },
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
  archiveBox.createSpan({ text: ' РџРѕРєР°Р·Р°С‚СЊ Р°СЂС…РёРІ' });
  archiveInput.addEventListener('change', () => {
    filter = { ...filter, archived: archiveInput.checked };
    void load(view, container, filter);
  });

  if (view.admin) {
    const add = actions.createEl('button', { text: 'вћ• РњР°С‚РµСЂРёР°Р»', cls: 'tn-btn tn-btn-primary' });
    add.addEventListener('click', () => {
      new MaterialModal(view, null, () => void load(view, container, filter)).open();
    });
  }

  let items: Material[] = [];
  try {
    items = await view.plugin.offersService.listMaterials(filter.archived);
  } catch (e: unknown) {
    container.createDiv({ cls: 'tn-offers-error', text: `РќРµ СѓРґР°Р»РѕСЃСЊ Р·Р°РіСЂСѓР·РёС‚СЊ РјР°С‚РµСЂРёР°Р»С‹: ${errorMessage(e)}` });
    return;
  }
  const shown = filterByQuery(items, filter.query);
  if (shown.length === 0) {
    container.createDiv({
      cls: 'tn-offers-empty',
      text: filter.query
        ? 'РџРѕ Р·Р°РїСЂРѕСЃСѓ РЅРёС‡РµРіРѕ РЅРµ РЅР°С€Р»РѕСЃСЊ.'
        : 'РњР°С‚РµСЂРёР°Р»РѕРІ РїРѕРєР° РЅРµС‚. РђРґРјРёРЅРёСЃС‚СЂР°С‚РѕСЂ РµС‰С‘ РЅРµ Р·Р°РІС‘Р» РЅРё РѕРґРЅРѕРіРѕ.',
    });
    return;
  }

  const table = container.createEl('table', { cls: 'tn-offers-table' });
  const head = table.createEl('thead').createEl('tr');
  for (const title of ['РќР°Р·РІР°РЅРёРµ Рё РєРѕРґ', 'Р•Рґ.', 'Р’РёР»РєР° С†РµРЅ', 'Р•РљРќ', '']) {
    head.createEl('th', { text: title });
  }
  const body = table.createEl('tbody');
  for (const item of shown) {
    const row = body.createEl('tr');
    if (item.archived) row.addClass('tn-offers-row-archived');

    const nameCell = row.createEl('td');
    nameCell.createDiv({ text: item.name });
    nameCell.createDiv({ cls: 'tn-offers-muted tn-offers-mono', text: item.sku });

    row.createEl('td', { text: item.unit });
    row.createEl('td', { cls: 'tn-offers-num', text: priceRangeText(item) });
    row.createEl('td', { cls: 'tn-offers-num', text: String(item.ekn.length) });

    const cell = row.createEl('td');
    const open = cell.createEl('button', { text: 'РљР°СЂС‚РѕС‡РєР°', cls: 'tn-btn tn-btn-ghost' });
    open.addEventListener('click', () => {
      new MaterialModal(view, item, () => void load(view, container, filter)).open();
    });
  }
}

/** Р’РёР»РєР° С†РµРЅ СЃР»РѕРІР°РјРё. РџСѓСЃС‚Р°СЏ РІРёР»РєР° вЂ” В«С†РµРЅР° РЅРµ Р·Р°РіСЂСѓР¶Р°Р»Р°СЃСЊВ», Р° РЅРµ РЅРѕР»СЊ. */
export function priceRangeText(item: Material): string {
  const min = item.priceMin ?? null;
  const max = item.priceMax ?? null;
  if (min === null && max === null) return 'С†РµРЅР° РЅРµ Р·Р°РіСЂСѓР¶Р°Р»Р°СЃСЊ';
  if (min !== null && max !== null && min !== max) {
    return `${formatMoney(min)} вЂ“ ${formatMoney(max)} в‚Ѕ`;
  }
  return `${formatMoney(min ?? max ?? 0)} в‚Ѕ`;
}

/**
 * РћС‚Р±РѕСЂ РґР»СЏ РїРѕРёСЃРєР°.
 *
 * РС‰РµРј РїРѕ РЅР°Р·РІР°РЅРёСЋ, РєРѕРґСѓ Рё РЅРѕРјРµСЂР°Рј Р•РљРќ: РјРµРЅРµРґР¶РµСЂ РёС‰РµС‚ РјР°С‚РµСЂРёР°Р» РїРѕ Р±СѓРјР°РіРµ, РіРґРµ
 * РЅР°РїРёСЃР°РЅ Р•РљРќ, Р° РЅРµ РїРѕ РЅР°Р·РІР°РЅРёСЋ РІ СЃРїСЂР°РІРѕС‡РЅРёРєРµ. Р РµРіРёСЃС‚СЂ Рё Р»РёС€РЅРёРµ РїСЂРѕР±РµР»С‹ РЅРµ
 * РІР°Р¶РЅС‹ вЂ” СЌС‚Рѕ С‚Рѕ, С‡С‚Рѕ С‡РµР»РѕРІРµРє РЅР°Р±РёСЂР°РµС‚ СЂСѓРєР°РјРё.
 */
export function filterByQuery(items: Material[], query: string): Material[] {
  const q = query.trim().toLowerCase().replace(/\s+/g, ' ');
  if (!q) return items;
  return items.filter((item) => {
    const haystack = [item.name, item.sku, ...item.ekn.map((e) => e.ekn)]
      .join(' ')
      .toLowerCase()
      .replace(/\s+/g, ' ');
    return haystack.includes(q);
  });
}

/** РљР°СЂС‚РѕС‡РєР° РјР°С‚РµСЂРёР°Р»Р°: РїРѕР»СЏ СЃРїСЂР°РІРѕС‡РЅРёРєР° Рё С‚Р°Р±Р»РёС†Р° РІР°СЂРёР°С†РёР№ Р•РљРќ. */
class MaterialModal extends Modal {
  private draft: Material;

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
          unit: 'С€С‚',
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
    contentEl.createEl('h3', { text: this.draft.id ? 'РњР°С‚РµСЂРёР°Р»' : 'РќРѕРІС‹Р№ РјР°С‚РµСЂРёР°Р»' });

    // SKU РїРѕРєР°Р·С‹РІР°РµС‚СЃСЏ, РЅРѕ РЅРµ СЂРµРґР°РєС‚РёСЂСѓРµС‚СЃСЏ: СЌС‚Рѕ Р»РёС‡РЅРѕСЃС‚СЊ РјР°С‚РµСЂРёР°Р»Р°, Р° РЅРµ
    // СЏСЂР»С‹Рє. РЎРµСЂРІРµСЂ РЅР°Р·РЅР°С‡РёС‚ РєРѕРґ РїСЂРё РїРµСЂРІРѕРј СЃРѕС…СЂР°РЅРµРЅРёРё.
    contentEl.createDiv({
      cls: 'tn-offers-muted tn-offers-mono',
      text: this.draft.sku ? `РљРѕРґ: ${this.draft.sku}` : 'РљРѕРґ РјР°С‚РµСЂРёР°Р»Р° РЅР°Р·РЅР°С‡РёС‚ СЃРµСЂРІРµСЂ РїСЂРё СЃРѕС…СЂР°РЅРµРЅРёРё',
    });

    new Setting(contentEl).setName('РќР°Р·РІР°РЅРёРµ').addText((text) => {
      text.setValue(this.draft.name).onChange((v) => {
        this.draft.name = v;
      });
    });
    new Setting(contentEl)
      .setName('Р’РёРґ')
      .setDesc('Р Р°Р±РѕС‚Р° РїСЂРѕРґР°С‘С‚СЃСЏ С‚Р°Рє Р¶Рµ, РЅРѕ РІР°СЂРёР°С†РёР№ Р•РљРќ Сѓ РЅРµС‘ РЅРµ Р±С‹РІР°РµС‚.')
      .addDropdown((dd) => {
        dd.addOption('material', 'РњР°С‚РµСЂРёР°Р»');
        dd.addOption('work', 'Р Р°Р±РѕС‚Р°');
        dd.setValue(this.draft.kind).onChange((v) => {
          this.draft.kind = v === 'work' ? 'work' : 'material';
        });
      });
    new Setting(contentEl).setName('Р•РґРёРЅРёС†Р°').addText((text) => {
      text.setValue(this.draft.unit).onChange((v) => {
        this.draft.unit = v;
      });
    });
    new Setting(contentEl).setName('РћРїРёСЃР°РЅРёРµ').addTextArea((area) => {
      area.setValue(this.draft.description).onChange((v) => {
        this.draft.description = v;
      });
      area.inputEl.rows = 3;
    });

    if (!this.draft.id) {
      contentEl.createDiv({
        cls: 'tn-offers-muted',
        text: 'РР· РєР°РєРёС… Р±СѓРєРІ СЃРѕР±СЂР°С‚СЊ РєРѕРґ:',
      });
      new Setting(contentEl)
        .setName('РўРёРї')
        .setDesc('РљРѕСЂРѕС‚РєРѕРµ СЃР»РѕРІРѕ: РњР•РњР‘, РЈРўР•РџР›, РљР Р•Рџ, Р“Р РЈРќРў, РџР›РРў.')
        .addText((text) => {
          text.setValue(this.typeCode).onChange((v) => {
            this.typeCode = v;
          });
        });
      new Setting(contentEl)
        .setName('РЎРµСЂРёСЏ')
        .setDesc('РџР’РҐ, РџРР , Р­РџРџ вЂ” РїРµСЂРІС‹Рµ С‚СЂРё Р·РЅР°РєР° РїРѕРїР°РґСѓС‚ РІ РєРѕРґ.')
        .addText((text) => {
          text.setValue(this.series).onChange((v) => {
            this.series = v;
          });
        });
    }

    contentEl.createEl('h4', { text: 'Р’Р°СЂРёР°С†РёРё (Р•РљРќ)' });
    this.renderEkn(contentEl);

    new Setting(contentEl)
      .addButton((btn) =>
        btn
          .setButtonText('РЎРѕС…СЂР°РЅРёС‚СЊ')
          .setCta()
          .onClick(() => {
            void this.save();
          }),
      )
      .addButton((btn) =>
        btn.setButtonText('Р—Р°РєСЂС‹С‚СЊ').onClick(() => {
          this.close();
          this.after();
        }),
      );

    if (this.draft.id && !this.draft.archived) {
      new Setting(contentEl)
        .setName('РђСЂС…РёРІ')
        .setDesc('РЈРґР°Р»РµРЅРёРµ РЅРµ СЃС‚РёСЂР°РµС‚ РјР°С‚РµСЂРёР°Р»: РЅР° РЅРµРіРѕ СЃСЃС‹Р»Р°СЋС‚СЃСЏ СЃС‚Р°СЂС‹Рµ РїСЂРµРґР»РѕР¶РµРЅРёСЏ.')
        .addButton((btn) =>
          btn
            .setButtonText('рџ—„ РЈР±СЂР°С‚СЊ РІ Р°СЂС…РёРІ')
            .setWarning()
            .onClick(() => {
              void this.archive();
            }),
        );
    }
  }

  private typeCode = '';
  private series = '';

  private renderEkn(container: HTMLElement): void {
    if (!this.draft.id) {
      container.createDiv({
        cls: 'tn-offers-muted',
        text: 'Р’Р°СЂРёР°С†РёРё РґРѕР±Р°РІР»СЏСЋС‚СЃСЏ РїРѕСЃР»Рµ СЃРѕС…СЂР°РЅРµРЅРёСЏ РјР°С‚РµСЂРёР°Р»Р° Рё РїСЂРё Р·Р°РіСЂСѓР·РєРµ С‚Р°Р±Р»РёС†С‹ РјРµРЅРµРґР¶РµСЂР°.',
      });
      return;
    }
    if (this.draft.ekn.length === 0) {
      container.createDiv({ cls: 'tn-offers-muted', text: 'Р’Р°СЂРёР°С†РёР№ РїРѕРєР° РЅРµС‚.' });
    } else {
      const table = container.createEl('table', { cls: 'tn-offers-table' });
      const head = table.createEl('thead').createEl('tr');
      for (const title of ['Р•РљРќ', 'Р’Р°СЂРёР°РЅС‚', 'РћС‚РєСѓРґР°', 'РљРѕРіРґР° РІРёРґРµР»Рё', '']) {
        head.createEl('th', { text: title });
      }
      const body = table.createEl('tbody');
      for (const ekn of this.draft.ekn) {
        const row = body.createEl('tr');
        row.createEl('td', { cls: 'tn-offers-mono', text: ekn.ekn });
        row.createEl('td', { text: ekn.variant || 'вЂ”' });
        row.createEl('td', { text: ekn.source === 'excel' ? 'С‚Р°Р±Р»РёС†Р°' : 'РІСЂСѓС‡РЅСѓСЋ' });
        row.createEl('td', { text: formatShortDate(ekn.lastSeenAt) });
        const del = row.createEl('td').createEl('button', { text: 'вњ–', cls: 'tn-btn tn-btn-ghost' });
        del.addEventListener('click', () => {
          void this.removeEkn(ekn);
        });
      }
    }
    const addRow = container.createDiv({ cls: 'tn-offers-row tn-offers-ekn-add' });
    const eknInput = addRow.createEl('input', {
      cls: 'tn-offers-input tn-offers-ekn-code',
      attr: { placeholder: 'Р•РљРќ' },
    });
    const variantInput = addRow.createEl('input', {
      cls: 'tn-offers-input tn-offers-grow',
      attr: { placeholder: 'Р’Р°СЂРёР°РЅС‚: 1,5 РјРј, СЃРµСЂС‹Р№' },
    });
    const add = addRow.createEl('button', { text: 'вћ• Р”РѕР±Р°РІРёС‚СЊ', cls: 'tn-btn tn-btn-ghost' });
    add.addEventListener('click', () => {
      void this.addEkn(eknInput, variantInput);
    });
  }

  private async save(): Promise<void> {
    if (!this.draft.name.trim()) {
      new Notice('РќРµ Р·Р°РїРѕР»РЅРµРЅРѕ РЅР°Р·РІР°РЅРёРµ РјР°С‚РµСЂРёР°Р»Р°');
      return;
    }
    try {
      const saved = await this.view.plugin.offersService.saveMaterial({
        ...this.draft,
        typeCode: this.typeCode,
        series: this.series,
      });
      this.draft = { ...saved, ekn: saved.ekn ?? [] };
      new Notice(this.item ? 'РњР°С‚РµСЂРёР°Р» СЃРѕС…СЂР°РЅС‘РЅ' : `РњР°С‚РµСЂРёР°Р» Р·Р°РІРµРґС‘РЅ: ${saved.sku}`);
      this.after();
      this.render();
    } catch (e: unknown) {
      new Notice(`РќРµ СѓРґР°Р»РѕСЃСЊ СЃРѕС…СЂР°РЅРёС‚СЊ: ${errorMessage(e)}`);
    }
  }

  private async addEkn(codeInput: HTMLInputElement, variantInput: HTMLInputElement): Promise<void> {
    const code = codeInput.value.trim();
    if (!code) {
      new Notice('РќРµ СѓРєР°Р·Р°РЅ Р•РљРќ');
      return;
    }
    // Р—РЅР°С‡РµРЅРёСЏ Р·Р°Р±РёСЂР°РµРј Р”Рћ СЃР±СЂРѕСЃР°: input.value вЂ” Р¶РёРІРѕРµ РїРѕР»Рµ, Рё РѕС‡РёСЃС‚РєР° РЅРёР¶Рµ
    // РёРЅР°С‡Рµ РѕР±РЅСѓР»РёР»Р° Р±С‹ С‚Рѕ, С‡С‚Рѕ РµС‰С‘ РЅРµ СѓС€Р»Рѕ РЅР° СЃРµСЂРІРµСЂ.
    const variant = variantInput.value.trim();
    try {
      const saved = await this.view.plugin.offersService.addMaterialEkn(this.draft.id, code, variant);
      this.draft = { ...saved, ekn: saved.ekn ?? [] };
      this.after();
      this.render();
    } catch (e: unknown) {
      new Notice(`РќРµ СѓРґР°Р»РѕСЃСЊ РґРѕР±Р°РІРёС‚СЊ Р•РљРќ: ${errorMessage(e)}`);
    }
  }

  private async removeEkn(ekn: MaterialEkn): Promise<void> {
    try {
      const saved = await this.view.plugin.offersService.deleteMaterialEkn(this.draft.id, ekn.id);
      this.draft = { ...saved, ekn: saved.ekn ?? [] };
      this.after();
      this.render();
    } catch (e: unknown) {
      new Notice(`РќРµ СѓРґР°Р»РѕСЃСЊ СѓР±СЂР°С‚СЊ Р•РљРќ: ${errorMessage(e)}`);
    }
  }

  private async archive(): Promise<void> {
    try {
      await this.view.plugin.offersService.archiveMaterial(this.draft.id);
      new Notice(`РњР°С‚РµСЂРёР°Р» В«${this.draft.name}В» РІ Р°СЂС…РёРІРµ`);
      this.close();
      this.after();
    } catch (e: unknown) {
      new Notice(`РќРµ СѓРґР°Р»РѕСЃСЊ СѓР±СЂР°С‚СЊ РІ Р°СЂС…РёРІ: ${errorMessage(e)}`);
    }
  }

  onClose(): void {
    this.contentEl.empty();
  }
}


