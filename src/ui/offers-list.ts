import { Notice } from 'obsidian';
import type { OffersView } from './offers-view';
import {
  canEditOffers,
  type MaterialSearchItem,
  type Offer,
  type OfferItem,
  type SystemSearchItem,
} from '../types/offers';
import { errorMessage } from '../../../sbe-core/src/utils/errors';
import { formatMoney, itemsTotal } from '../utils/money';
import { renderBriefCard } from './brief-editor';
import { renderSheetUpload } from './sheet-upload';
import { renderFormEditor } from './form-editor';
import { renderPicker, type PickerCandidate } from './picker';
import { FORM_FLAG_BRIEF_SUMMARY, FORM_FLAG_SECTION_COPY } from '../constants';

/**
 * Тексты системы (2026-11-06): ровно два блока, и это решение, а не список
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
  // «Форма КП» переехала внутрь модуля КП (2026-11-08, просьба пользователя):
  // кнопка рядом с «Новое предложение», а не отдельный пункт сайдбара. Один
  // путь к конфигуратору вместо двух.
  if (view.admin) {
    const formBtn = actions.createEl('button', { text: '⚙️ Форма КП', cls: 'tn-btn tn-btn-ghost' });
    formBtn.addEventListener('click', () => {
      container.empty();
      const back = container.createEl('button', { text: '← К списку', cls: 'tn-btn tn-btn-ghost' });
      back.addEventListener('click', () => {
        void load(view, container);
      });
      renderFormEditor(view, container, 'client');
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

  // Материалы — поиском по Базе знаний (2026-11-12). Раньше здесь был дропдаун
  // по `offers_catalog`: в нём не было ни новых материалов (таблица не
  // пополняется с 2026-10-31), ни поиска по живому каталогу на 1500+ записей.
  // Правило «выбор системы не добавляет позиции» (2026-11-03) не меняется:
  // позицией становится именно выбранный материал.
  const materialCandidates: Array<PickerCandidate & { item: OfferItem }> = [];
  const materialsBox = itemsCard.createDiv({ cls: 'tn-offers-card tn-offers-inline-card' });
  renderPicker(
    materialsBox,
    {
      label: 'Материал в позиции',
      placeholder: 'Название материала — часть слова или код…',
      chosen: [],
      emptyText: 'Найдите материал: он добавится позицией, количество поправите потом.',
      candidates: () => materialCandidates,
      load: async () => {
        materialCandidates.length = 0;
      },
      search: async (query) => {
        const res = await view.plugin.offersService.searchMaterials(query);
        if (res.error) {
          new Notice(`Поиск по Базе знаний: ${res.error}`);
          return null;
        }
        materialCandidates.length = 0;
        for (const item of res.items) {
          materialCandidates.push(materialCandidate(item));
        }
        return materialCandidates;
      },
      onPick: (candidate) => {
        const found = materialCandidates.find((c) => c.noteId === candidate.noteId);
        const item = found?.item;
        if (!item) return;
        // Позиции ещё не сохранены: редактор держит их локально и пишет в КП по
        // кнопке «Сохранить», поэтому здесь только локальный список.
        items.push(item);
        redrawItems();
      },
    },
    errorMessage,
  );

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
    // Система выбирается поиском по Базе знаний (2026-11-12). Раньше это был
    // дропдаун всех систем: на живых данных их сотни, и найти «ТН-Кровля
    // Гарант» прокруткой было нельзя. Поиск тот же, что в боевом листке.
    const systemsBox = itemsCard.createDiv({ cls: 'tn-offers-system-add' });
    const systemCandidates: PickerCandidate[] = [];
    const redrawSystems = renderPicker(
      systemsBox,
      {
        label: 'Система в КП',
        placeholder: 'Название системы — часть слова, например «Гарант» или «Кровля»…',
        chosen: [],
        emptyText: 'Систем не выбрано: КП соберётся без описания системы.',
        candidates: () => systemCandidates,
        load: async () => {
          // Кэш из Базы знаний нужен, пока идёт запрос, и когда модель поиска
          // недоступна: раздел «Системы» уже прочитанное показывает.
          systemCandidates.length = 0;
          const res = await view.plugin.offersService.listSystems();
          if (!res.kbEnabled) {
            systemsBox.createDiv({ cls: 'tn-offers-warn', text: 'База знаний не настроена.' });
            return;
          }
          for (const system of res.systems) {
            systemCandidates.push({
              noteId: system.noteId,
              title: system.title,
              code: system.sku,
            });
          }
        },
        search: async (query) => {
          const res = await view.plugin.offersService.searchSystems(query);
          if (res.error) {
            new Notice(`Поиск по Базе знаний: ${res.error}`);
            return null;
          }
          systemCandidates.length = 0;
          for (const item of res.items) {
            systemCandidates.push(systemCandidate(item));
          }
          return systemCandidates;
        },
        onPick: (candidate) => addSystemPicked(candidate.noteId),
      },
      errorMessage,
    );

    // Выбранные системы списком (2026-11-05). Раньше была одна, и человек не
    // видел даже её: подпись говорила только «выбрана». Теперь их может быть
    // несколько — для сравнения в документе, как СТИЛОБАТ V1.0 и V2.0.
    const chosenList = systemsBox.createDiv({ cls: 'tn-offers-chosen-systems' });

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

    /**
     * Подпись системы по хэндлу: имя из найденного, иначе сам хэндл.
     *
     * Список найденного теперь живёт в окне поиска, поэтому имя ищется и там,
     * и в кэше систем: выбранное могло прийти и поиском, и из кэша.
     */
    function handleLabel(handle: string): string {
      const noteId = Number(/^sys-(\d+)$/.exec(handle)?.[1] ?? 0);
      if (noteId > 0) {
        const found = systemCandidates.find((c) => c.noteId === noteId);
        if (found) return found.title;
      }
      return handle;
    }
    renderChosen(current);
    void redrawSystems;

    /**
     * Выбрать систему: сервер пишет её тексты один раз при выборе (2026-11-06).
     *
     * Позиций выбор не добавляет (2026-11-03): в КП попадают описание,
     * обоснование и ссылки, а состав с ценами менеджер заводит сам.
     */
    async function addSystemPicked(noteId: number): Promise<void> {
      if (!noteId) {
        new Notice('Выберите систему');
        return;
      }
      // Тексты системы пишет модель — ОДИН раз при выборе. Это занимает
      // десятки секунд, и молчание здесь читается как поломка.
      new Notice('Добавляю систему и пишу тексты (описание и преимущества) — до минуты…', 8000);
      try {
        const res = await view.plugin.offersService.addSystem(current.id!, `sys-${noteId}`);
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

  // Материалы-объекты предложения (2026-11-13) — отдельный блок, а не часть
  // предыдущего: «Материал в позиции» добавляет СТРОКУ с ценой, а здесь
  // материал попадает в документ описанием. Две подписи рядом, потому что один
  // неаккуратный выбор здесь стоит дорого: цена и описание — разные вещи.
  if (current.id) {
    renderChosenMaterials(view, itemsCard, current);
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

/**
 * «Материалы в предложении (описанием)» (2026-11-13, R3).
 *
 * Материал здесь — ОБЪЕКТ предложения, как система: в документ попадают его
 * описание и ссылки, а не цена. Подпись говорит это дословно: рядом стоит
 * «Материал в позиции», и человек должен понимать разницу до нажатия — иначе
 * материал либо станет строкой с ценой 0, либо пропадёт из документа вовсе.
 */
function renderChosenMaterials(view: OffersView, card: HTMLElement, offer: Offer): void {
  const box = card.createDiv({ cls: 'tn-offers-card tn-offers-inline-card' });
  box.createDiv({ cls: 'tn-offers-muted', text: 'Материалы в предложении (описанием)' });
  const chosenList = box.createDiv({ cls: 'tn-offers-chosen-systems' });
  const candidates: PickerCandidate[] = [];

  const renderChosen = (current: Offer): void => {
    chosenList.empty();
    const materials = current.materials ?? [];
    if (materials.length === 0) {
      chosenList.createDiv({
        cls: 'tn-offers-muted',
        text: 'Материалов описанием нет. В документ они попадут через блок формы с повтором «по материалам».',
      });
      return;
    }
    for (const chosen of materials) {
      const row = chosenList.createDiv({ cls: 'tn-offers-row tn-offers-chosen-system' });
      const handle = chosen.noteId ? `sys-${chosen.noteId}` : (chosen.sku ?? '');
      const found = candidates.find((c) => c.noteId === chosen.noteId);
      row.createSpan({ cls: 'tn-offers-grow', text: `Материал: ${found?.title ?? handle}` });
      const removeBtn = row.createEl('button', { text: 'Убрать', cls: 'tn-btn tn-btn-ghost' });
      removeBtn.addEventListener('click', () => {
        void removeMaterial(handle, removeBtn);
      });
    }
  };

  renderPicker(
    box,
    {
      label: 'Найти материал для предложения',
      placeholder: 'Название материала — часть слова или код…',
      chosen: [],
      emptyText: 'Найдите материал: он попадёт в документ описанием, без цены.',
      candidates: () => candidates,
      load: async () => {
        candidates.length = 0;
      },
      search: async (query) => {
        const res = await view.plugin.offersService.searchMaterials(query);
        if (res.error) {
          new Notice(`Поиск по Базе знаний: ${res.error}`);
          return null;
        }
        candidates.length = 0;
        for (const item of res.items) {
          candidates.push({ noteId: item.noteId, title: item.name, code: item.sku });
        }
        return candidates;
      },
      onPick: (candidate) => {
        void addMaterial(candidate.noteId);
      },
    },
    errorMessage,
  );
  renderChosen(offer);

  /** Выбор материала: сервер пишет описание один раз, при выборе. */
  async function addMaterial(noteId: number): Promise<void> {
    if (!noteId) {
      new Notice('Выберите материал');
      return;
    }
    new Notice('Добавляю материал и пишу описание — до минуты…', 8000);
    try {
      const res = await view.plugin.offersService.addMaterial(offer.id, `sys-${noteId}`);
      offer.materials = res.offer.materials ?? [];
      renderChosen(offer);
      const written = Object.values(res.texts ?? {}).filter((t) => t.trim() !== '').length;
      new Notice(
        written > 0
          ? `Материал «${res.title}» добавлен в предложение. Описание готово. Позиции не добавлялись.`
          : `Материал «${res.title}» добавлен. Описание не записалось — попробуйте позже.`,
      );
    } catch (e: unknown) {
      new Notice(`Материал не добавлен: ${errorMessage(e)}`);
    }
  }

  /** Снять материал с предложения. */
  async function removeMaterial(handle: string, button: HTMLButtonElement): Promise<void> {
    button.setAttr('disabled', 'true');
    try {
      const res = await view.plugin.offersService.removeMaterial(offer.id, handle);
      offer.materials = res.offer.materials ?? [];
      renderChosen(offer);
      new Notice('Материал убран из предложения.');
    } catch (e: unknown) {
      new Notice(`Не удалось убрать материал: ${errorMessage(e)}`);
    } finally {
      button.removeAttribute('disabled');
    }
  }
}

function totalText(items: OfferItem[]): string {
  return `Итого: ${formatMoney(itemsTotal(items))} ₽`;
}

/**
 * Найденная система → вариант окна выбора.
 *
 * Замечания к составу показываются прямо в кнопке: человек должен знать, что
 * у системы есть несвязанные строки, ДО того как она попадёт в документ.
 */
function systemCandidate(item: SystemSearchItem): PickerCandidate {
  const broken = (item.problems ?? []).length > 0;
  return {
    noteId: item.noteId,
    title: broken ? `${item.title} (есть замечания к составу)` : item.title,
    code: item.sku,
  };
}

/** Найденный материал → вариант окна выбора и сразу позиция КП. */
function materialCandidate(item: MaterialSearchItem): PickerCandidate & { item: OfferItem } {
  const price = item.priceMin > 0 ? item.priceMin : 0;
  return {
    noteId: item.noteId,
    title: item.name,
    code: item.sku,
    item: {
      catalogId: item.materialId > 0 ? item.materialId : undefined,
      ekn: (item.ekn ?? [])[0] ?? '',
      name: item.name,
      unit: item.unit || 'шт',
      qty: 1,
      price,
      sum: price,
    },
  };
}
