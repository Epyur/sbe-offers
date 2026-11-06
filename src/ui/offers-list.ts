import { Notice } from 'obsidian';
import type { OffersView } from './offers-view';
import { canEditOffers, type CatalogItem, type Offer, type OfferItem } from '../types/offers';
import { errorMessage } from '../../../sbe-core/src/utils/errors';
import { formatMoney, itemsTotal } from '../utils/money';
import { renderBriefCard } from './brief-editor';
import { renderSheetUpload } from './sheet-upload';
import { FORM_FLAG_BRIEF_SUMMARY, FORM_FLAG_SECTION_COPY } from '../constants';

/**
 * Блоки текста системы (2026-11-06): ровно два, и это решение, а не список
 * «всё, что есть». Состав не генерируется — он уже есть в карточке Базы знаний,
 * и пересказ моделью был бы второй правдой рядом со списком.
 */
const SYSTEM_TEXT_LABELS: Array<[string, string]> = [
  ['description', 'Обоснование применения'],
  ['advantages', 'Преимущества'],
];

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
      // Код ЕКН — рядом с наименованием: его печатает таблица документа, и
      // только здесь человек может его исправить или вписать (2026-11-02).
      const ekn = row.createEl('input', {
        cls: 'tn-offers-input tn-offers-mono tn-offers-ekn',
        attr: { placeholder: 'ЕКН' },
      });
      ekn.value = item.ekn ?? '';
      ekn.addEventListener('input', () => {
        item.ekn = ekn.value.trim();
      });
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

  // Выбор системы (Task 7, 2026-10-31; переработка 2026-11-03) работает с
  // сохранённым КП. Позиций он НЕ добавляет: в КП попадают обоснование
  // применения и описание продуктов, а состав с ценами менеджер заводит сам —
  // файлом или руками. До правки состав дописывался к таблице, и одни и те же
  // продукты стояли в ней дважды, с ценой и с нулём (КП 003/2026).
  if (current.id) {
    const systemsCard = itemsCard.createDiv({ cls: 'tn-offers-row tn-offers-system-add' });
    systemsCard.createSpan({ cls: 'tn-offers-muted', text: 'Система в КП:' });
    const systemsSelect = systemsCard.createEl('select', { cls: 'tn-offers-input tn-offers-grow' });
    systemsSelect.createEl('option', { value: '', text: '— выбрать систему —' });
    void view.plugin.offersService
      .listSystems()
      .then((res) => {
        if (!res.kbEnabled) {
          systemsCard.createSpan({ cls: 'tn-offers-warn', text: ' База знаний не настроена' });
          return;
        }
        for (const system of res.systems) {
          // Слои — нормальный состав, а не ошибка (2026-11-02): у живых
          // карточек почти в каждой есть работы без материала, и пометка
          // «есть ошибки» стояла у всех подряд.
          const broken = (system.problems ?? []).length > 0;
          // Значение — заметка, когда кода нет: у живых карточек
          // `system_sku` не заполнен, и вариант с пустым значением выбрать
          // нельзя (2026-11-02, КП 003: «Выберите систему» на любой выбор).
          systemsSelect.createEl('option', {
            value: `sys-${system.noteId}`,
            text: `${system.title}${broken ? ' (есть замечания к составу)' : ''}`,
          });
        }
        // Отмечаем уже выбранные системы: человек должен видеть текущее
        // состояние, а не пустой список.
        const chosen = current.systems ?? [];
        const chosenHandle = chosen[0]?.noteId ? `sys-${chosen[0].noteId}` : (chosen[0]?.sku ?? '');
        if (chosenHandle) systemsSelect.value = chosenHandle;
      })
      .catch((e: unknown) => {
        systemsCard.createSpan({ cls: 'tn-offers-warn', text: ` Список систем: ${errorMessage(e)}` });
      });
    const addSystemBtn = systemsCard.createEl('button', { text: '✔ Добавить', cls: 'tn-btn tn-btn-ghost' });
    addSystemBtn.addEventListener('click', () => {
      void addSystemPicked();
    });

    // Выбранные системы списком (2026-11-05). Раньше была одна, и человек не
    // видел даже её: подпись говорила только «выбрана». Теперь их может быть
    // несколько — для сравнения в документе, как СТИЛОБАТ V1.0 и V2.0.
    const chosenList = systemsCard.createDiv({ cls: 'tn-offers-chosen-systems' });

    function renderChosen(offer: Offer): void {
      chosenList.empty();
      const systems = offer.systems ?? [];
      if (systems.length === 0) {
        chosenList.createDiv({
          cls: 'tn-offers-muted',
          text: 'Системы не выбраны: КП соберётся без описания системы.',
        });
        return;
      }
      for (const chosen of systems) {
        const row = chosenList.createDiv({ cls: 'tn-offers-row tn-offers-chosen-system' });
        // Название берём из того же ответа, что и удаление: у выбора есть код
        // или заметка, а человеку нужно имя.
        const handle = chosen.noteId ? `sys-${chosen.noteId}` : (chosen.sku ?? '');
        const label = handle ? `Система: ${handleLabel(handle)}` : 'Система';
        row.createSpan({ cls: 'tn-offers-grow', text: label });
        // «Пересчитать тексты системы» (2026-11-06, шаг B): карточку в Базе
        // знаний могли поправить после выбора. Пересчёт — осознанное действие,
        // потому что снова зовёт модель; автоматически этого не делаем.
        if (chosen.noteId) {
          const recalcBtn = row.createEl('button', {
            text: '↻ Пересчитать',
            cls: 'tn-btn tn-btn-ghost',
          });
          recalcBtn.setAttr('title', 'Переписать описание и преимущества системы по текущей карточке');
          recalcBtn.addEventListener('click', () => {
            void recalculate(chosen.noteId!, recalcBtn);
          });
        }
        const removeBtn = row.createEl('button', { text: 'Убрать', cls: 'tn-btn tn-btn-ghost' });
        removeBtn.addEventListener('click', () => {
          void removeSystem(handle, removeBtn);
        });
      }
      if (systems.length === 1) {
        chosenList.createDiv({
          cls: 'tn-offers-muted',
          text: 'Одна система: сравнивать нечего. Добавьте вторую, чтобы в документ попала таблица сравнения.',
        });
      } else {
        chosenList.createDiv({
          cls: 'tn-offers-muted',
          text: `Выбрано систем: ${systems.length}. В документе они идут по порядку выбора, а сравнение — таблицей, если в форме есть этот блок.`,
        });
      }
    }

    // labelFor — подпись системы по хэндлу: имя из списка, иначе сам хэндл.
    function handleLabel(handle: string): string {
      const option = Array.from(systemsSelect.options).find((o) => o.value === handle);
      return option?.textContent ?? handle;
    }
    renderChosen(current);

    async function addSystemPicked(): Promise<void> {
      const choice = systemsSelect.value;
      if (!choice) {
        new Notice('Выберите систему');
        return;
      }
      addSystemBtn.setAttr('disabled', 'true');
      // Тексты системы пишет модель — ОДИН раз при выборе (2026-11-06). Это
      // занимает десятки секунд, и молчание здесь читается как поломка.
      new Notice('Добавляю систему и пишу тексты (описание и преимущества) — до минуты…', 8000);
      try {
        const res = await view.plugin.offersService.addSystem(current.id!, choice);
        // Позиции не трогаем: сервер их не меняет, а локальный список в
        // редакторе остаётся тем, что человек правил.
        current.systems = res.offer.systems ?? [];
        renderChosen(current);
        renderSystemTexts(res.texts ?? {}, res.missing ?? []);
        const written = Object.values(res.texts ?? {}).filter((t) => t.trim() !== '').length;
        new Notice(
          written > 0
            ? `Система добавлена: «${res.system?.title ?? ''}». Тексты готовы — в документ они попадут сразу. Позиции не добавлялись.`
            : `Система добавлена: «${res.system?.title ?? ''}». Тексты не записались — нажмите «Пересчитать тексты системы».`,
        );
      } catch (e: unknown) {
        new Notice(`Система выбрана, но тексты не записались: ${errorMessage(e)}`);
      } finally {
        addSystemBtn.removeAttribute('disabled');
      }
    }

    /** Показать, что за тексты системы записаны (2026-11-06). */
    function renderSystemTexts(texts: Record<string, string>, missing: string[]): void {
      chosenList.createDiv({ cls: 'tn-offers-system-texts-head', text: 'Тексты системы:' });
      for (const [block, label] of SYSTEM_TEXT_LABELS) {
        const text = (texts[block] ?? '').trim();
        const row = chosenList.createDiv({ cls: 'tn-offers-row tn-offers-system-text' });
        row.createSpan({ cls: 'tn-offers-muted', text: `${label}: ` });
        row.createSpan({
          cls: text ? 'tn-offers-grow' : 'tn-offers-warn',
          text: text ? `${text.slice(0, 160)}${text.length > 160 ? '…' : ''}` : 'не написано',
        });
      }
      for (const block of missing) {
        chosenList.createDiv({
          cls: 'tn-offers-warn',
          text: `Не удалось написать: ${SYSTEM_TEXT_LABELS.find(([id]) => id === block)?.[1] ?? block}. Документ соберётся без этого текста.`,
        });
      }
    }

    async function removeSystem(handle: string, button: HTMLButtonElement): Promise<void> {
      button.setAttr('disabled', 'true');
      try {
        const res = await view.plugin.offersService.removeSystem(current.id!, handle);
        current.systems = res.offer.systems ?? [];
        renderChosen(current);
        new Notice('Система убрана из предложения.');
      } catch (e: unknown) {
        new Notice(`Не удалось убрать систему: ${errorMessage(e)}`);
      } finally {
        button.removeAttribute('disabled');
      }
    }

    /**
     * Пересчитать тексты одной системы по текущей карточке (2026-11-06).
     *
     * Нужно, когда карточку в Базе знаний поправили после выбора: текст
     * сохранён и сам не обновится. Кнопка вызывает модель — поэтому она
     * ручная, а не фоновая.
     */
    async function recalculate(noteId: number, button: HTMLButtonElement): Promise<void> {
      button.setAttr('disabled', 'true');
      new Notice('Пересчитываю тексты системы — до минуты…', 8000);
      try {
        const res = await view.plugin.offersService.recalculateSystemTexts(current.id!, noteId);
        renderSystemTexts(res.texts, res.missing ?? []);
        const written = Object.values(res.texts).filter((t) => t.trim() !== '').length;
        new Notice(
          written > 0
            ? `Тексты системы «${res.title}» обновлены.`
            : `Тексты системы «${res.title}» не записались — попробуйте позже.`,
        );
      } catch (e: unknown) {
        new Notice(`Не удалось пересчитать тексты: ${errorMessage(e)}`);
      } finally {
        button.removeAttribute('disabled');
      }
    }
  }

  // Таблица менеджера — тоже только у сохранённого КП: она дописывает позиции
  // в конкретное предложение и двигает его updated_at.
  if (current.id) {
    renderSheetUpload(view, container, current.id, current.number, () => {
      void openEditor(view, container, current);
    });
  }

  const formCard = container.createDiv({ cls: 'tn-offers-card' });
  formCard.createDiv({ cls: 'tn-offers-muted', text: 'Поля формы' });  const formValues: Record<string, string> = { ...(current.formValues ?? {}) };
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

  // Бриф — только у сохранённого предложения: у нового ещё нет номера, а бриф
  // привязан к строке в базе.
  if (current.id) {
    await renderBriefCard(view, container, current.id);
  }

  // Что попадает в документ сверх формы (Task 11, 2026-10-30). Оба
  // переключателя по умолчанию выключены: у старых КП файл не меняется.
  const docCard = container.createDiv({ cls: 'tn-offers-card' });
  docCard.createDiv({ cls: 'tn-offers-muted', text: 'Дополнительно в документе' });
  const summaryBox = docCard.createEl('label', { cls: 'tn-offers-check' });
  const summaryInput = summaryBox.createEl('input', { attr: { type: 'checkbox' } });
  summaryInput.checked = formValues[FORM_FLAG_BRIEF_SUMMARY] === '1';
  summaryBox.createSpan({ text: ' Сводка брифа (что известно про объект)' });
  summaryInput.addEventListener('change', () => {
    formValues[FORM_FLAG_BRIEF_SUMMARY] = summaryInput.checked ? '1' : '';
  });
  const copyBox = docCard.createEl('label', { cls: 'tn-offers-check' });
  const copyInput = copyBox.createEl('input', { attr: { type: 'checkbox' } });
  copyInput.checked = formValues[FORM_FLAG_SECTION_COPY] === '1';
  copyBox.createSpan({ text: ' Тексты секций, согласованные в боте' });
  copyInput.addEventListener('change', () => {
    formValues[FORM_FLAG_SECTION_COPY] = copyInput.checked ? '1' : '';
  });

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
