import { Modal, Notice, TFile } from 'obsidian';
import type { OffersView } from './offers-view';
import type { OfferForm } from '../types/offers';
import { errorMessage } from '../../../sbe-core/src/utils/errors';

/**
 * Карточка «Макет документа .docx» (2026-11-05).
 *
 * Пользователь просил: «пользователь мог бы загрузить в систему макет ТКП, где
 * он мог бы заполнить два типа плейсхолдеров». Здесь загрузка, скачивание для
 * правки и удаление.
 *
 * Метки макета показываются списком: человек должен видеть, что система поняла
 * из его документа, ДО первой сборки — иначе ошибка в метке обнаружится только
 * в готовом КП у менеджера.
 */

export interface LayoutCardDeps {
  onChanged: () => void;
  onPickFile: (view: OffersView) => Promise<{ name: string; data: ArrayBuffer } | null>;
}

/** Рисует карточку макета. */
export function renderLayoutCard(view: OffersView, container: HTMLElement, form: OfferForm, deps: LayoutCardDeps): void {
  const card = container.createDiv({ cls: 'tn-offers-card tn-offers-layout-card' });
  card.createDiv({ cls: 'tn-offers-muted', text: 'Макет документа .docx (необязательно)' });

  if (form.hasLayout) {
    const active = card.createDiv({ cls: 'tn-offers-warn' });
    active.createDiv({
      text: `Сейчас КП печатается по макету «${form.layoutName || 'макет.docx'}». ` +
        'Блоки ниже сохранены, но в документ не идут — удалите макет, чтобы вернуться к ним.',
    });
  } else {
    card.createDiv({
      cls: 'tn-offers-muted',
      text: 'Если макет загружен, КП собирается по вашему документу Word: свои стили, ' +
        'шрифты и колонтитулы. Метки в макете: [Название объекта] — подставить значение, ' +
        '{Опиши систему} — пусть текст напишет модель. Промт можно писать одним, двумя ' +
        'или тремя фигурными скобками: {…}, {{…}}, {{{…}}} — любая из форм понимается одинаково.',
    });
  }

  renderLayoutTokens(card, form);

  // Начать с рабочего образца (2026-11-06, шаг E): требование пользователя —
  // инструкция должна позволять начать с образца, а не со слов. Образец
  // собирает сервис, поэтому он не разойдётся с разбором.
  const guide = card.createDiv({ cls: 'tn-offers-muted' });
  guide.createDiv({
    text: 'Метки: [Поле] — значение, {промт} — текст модели, {{система}} — картинка ' +
      'и состав одной таблицей, {{текст: система: description}} — сохранённый текст ' +
      'системы, {{block:1}} — целый блок формы. Подробно — в разделе справки ' +
      '«Макет .docx: как расставить метки».',
  });
  const exampleBtn = guide.createEl('button', { text: '⬇ Скачать образец макета', cls: 'tn-btn tn-btn-ghost' });
  exampleBtn.addEventListener('click', () => {
    void (async () => {
      try {
        const data = await view.plugin.offersService.downloadExampleLayout();
        const path = await view.app.vault.createBinary(uniqueVaultPath(view, 'образец-макета-КП.docx'), data);
        new Notice(`Образец макета сохранён в вольт: ${path.path}`);
      } catch (e: unknown) {
        new Notice(`Не удалось скачать образец: ${errorMessage(e)}`);
      }
    })();
  });

  const actions = card.createDiv({ cls: 'tn-offers-row' });
  const upload = actions.createEl('button', {
    text: form.hasLayout ? '⬆ Заменить макет' : '⬆ Загрузить макет .docx',
    cls: 'tn-btn tn-btn-primary',
  });
  upload.addEventListener('click', () => {
    void (async () => {
      const picked = await deps.onPickFile(view);
      if (!picked) return;
      upload.setText('⏳ Загружаю…');
      try {
        const res = await view.plugin.offersService.uploadLayout(picked.data, picked.name);
        new Notice(
          `Макет «${res.layoutName}» загружен: полей ${res.layoutFields.length}, промтов ${res.layoutPrompts.length}`,
        );
        deps.onChanged();
      } catch (e: unknown) {
        new Notice(`Не удалось загрузить макет: ${errorMessage(e)}`, 8000);
        upload.setText('⬆ Загрузить макет .docx');
      }
    })();
  });

  if (form.hasLayout) {
    const download = actions.createEl('button', { text: '⬇ Скачать макет', cls: 'tn-btn tn-btn-ghost' });
    download.addEventListener('click', () => {
      void (async () => {
        try {
          const data = await view.plugin.offersService.downloadLayout();
          const name = form.layoutName || 'макет.docx';
          const path = await view.app.vault.createBinary(
            uniqueVaultPath(view, name),
            data,
          );
          new Notice(`Макет сохранён в вольт: ${path.path}`);
        } catch (e: unknown) {
          new Notice(`Не удалось скачать макет: ${errorMessage(e)}`);
        }
      })();
    });

    const remove = actions.createEl('button', { text: '✖ Удалить макет', cls: 'tn-btn tn-btn-ghost' });
    remove.addEventListener('click', () => {
      void (async () => {
        try {
          await view.plugin.offersService.deleteLayout();
          new Notice('Макет удалён — КП снова печатается по блокам формы');
          deps.onChanged();
        } catch (e: unknown) {
          new Notice(`Не удалось удалить макет: ${errorMessage(e)}`);
        }
      })();
    });
  }
}

/** Какие метки нашла система. */
function renderLayoutTokens(card: HTMLElement, form: OfferForm): void {
  const fields = form.layoutFields ?? [];
  const prompts = form.layoutPrompts ?? [];
  if (fields.length === 0 && prompts.length === 0) return;
  const box = card.createDiv({ cls: 'tn-offers-layout-tokens' });
  if (fields.length > 0) {
    box.createDiv({ cls: 'tn-offers-muted', text: 'Подставляемые поля [ … ]:' });
    const list = box.createDiv({ cls: 'tn-offers-row tn-offers-chip-row' });
    for (const field of fields) list.createSpan({ cls: 'tn-offers-chip', text: `[${field}]` });
  }
  if (prompts.length > 0) {
    box.createDiv({ cls: 'tn-offers-muted', text: 'Текст модели { … }:' });
    const list = box.createDiv({ cls: 'tn-offers-row tn-offers-chip-row' });
    for (const prompt of prompts) list.createSpan({ cls: 'tn-offers-chip', text: `{${prompt}}` });
    box.createDiv({
      cls: 'tn-offers-muted',
      text: 'Так систему поняла эта сборка. Промт можно писать и в {{…}}, и в {{{…}}}: ' +
        'главное — чтобы внутри были ваши слова, а не только имя поля формы.',
    });
  }
}

/** Путь в вольте, которого ещё нет: макет не должен затирать чужой файл. */
function uniqueVaultPath(view: OffersView, name: string): string {
  const base = name.replace(/[\\/:*?"<>|]/g, '_');
  let candidate = base;
  let n = 1;
  while (view.app.vault.getAbstractFileByPath(candidate) instanceof TFile) {
    candidate = base.replace(/(\.docx)?$/i, ` (${n++}).docx`);
  }
  return candidate;
}

/** Выбор .docx из вольта. */
export async function pickLayoutFile(view: OffersView): Promise<{ name: string; data: ArrayBuffer } | null> {
  const files = view.app.vault.getFiles().filter((f) => f.extension.toLowerCase() === 'docx');
  if (files.length === 0) {
    new Notice('В вольте нет файлов .docx. Положите макет в хранилище и повторите.');
    return null;
  }
  // Простой список через модальный выбор не нужен: файлов .docx в вольте
  // единицы (макеты и старые КП), и плагин предлагает их напрямую.
  return new Promise((resolve) => {
    new LayoutFileModal(view, files, (file) => {
      void (async () => {
        try {
          resolve({ name: file.name, data: await view.app.vault.readBinary(file) });
        } catch (e: unknown) {
          new Notice(`Не удалось прочитать файл: ${errorMessage(e)}`);
          resolve(null);
        }
      })();
    }, () => resolve(null)).open();
  });
}

/** Модалка выбора файла .docx из вольта. */
class LayoutFileModal extends Modal {
  private picked = false;

  constructor(
    private readonly view: OffersView,
    private readonly files: TFile[],
    private readonly onPick: (file: TFile) => void,
    private readonly onCancel: () => void,
  ) {
    super(view.app);
  }

  onOpen(): void {
    const { contentEl } = this;
    contentEl.empty();
    contentEl.createEl('h2', { text: 'Макет КП из хранилища' });
    contentEl.createEl('p', {
      cls: 'tn-offers-muted',
      text: 'Выберите документ Word (.docx) с метками [ … ] и { … }.',
    });
    const list = contentEl.createDiv({ cls: 'tn-offers-card' });
    for (const file of this.files) {
      const row = list.createDiv({ cls: 'tn-offers-row' });
      const btn = row.createEl('button', { text: file.path, cls: 'tn-btn tn-btn-ghost tn-offers-grow' });
      btn.addEventListener('click', () => {
        this.picked = true;
        this.onPick(file);
        this.close();
      });
    }
  }

  onClose(): void {
    this.contentEl.empty();
    // Закрыли без выбора (крестиком или Escape) — «не выбрал», иначе обещание
    // висело бы, и кнопка загрузки навсегда осталась бы в «Загружаю…».
    if (!this.picked) this.onCancel();
  }
}