import { Notice } from 'obsidian';
import type { OffersView } from './offers-view';
import {
  canEditOffers,
  type CombatNoteRef,
  type CombatSheet,
  type SystemCard,
} from '../types/offers';
import { errorMessage } from '../../../sbe-core/src/utils/errors';
import { renderFormEditor } from './form-editor';

/**
 * Раздел «⚙️ Боевой листок» (2026-11-08).
 *
 * Внутренний вид повторяет раздел «КП» (offers-list.ts): список документов,
 * карточка редактора, шапка с кнопками. Отличие — в источниках: наш продукт
 * выбирается из систем (или берётся из привязанного КП), продукты конкурентов —
 * из папки Базы знаний «материалы и системы конкурентов». Сборка DOCX
 * появится отдельной партией; сейчас листок сохраняется и показывается.
 */
export function renderCombatView(view: OffersView, container: HTMLElement): void {
  container.createDiv({ cls: 'tn-offers-muted', text: 'Загрузка боевых листков…' });
  void load(view, container);
}

async function load(view: OffersView, container: HTMLElement): Promise<void> {
  container.empty();
  const actions = container.createDiv({ cls: 'tn-offers-actions' });
  if (canEditOffers(view.role)) {
    const create = actions.createEl('button', { text: '➕ Новый боевой листок', cls: 'tn-btn tn-btn-primary' });
    create.addEventListener('click', () => {
      void openEditor(view, container, null);
    });
  }
  // Форма боевого листка — тот же конфигуратор, что у КП, под режимом combat.
  if (view.admin) {
    const formBtn = actions.createEl('button', { text: '⚙️ Форма боевого листка', cls: 'tn-btn tn-btn-ghost' });
    formBtn.addEventListener('click', () => {
      container.empty();
      const back = container.createEl('button', { text: '← К списку', cls: 'tn-btn tn-btn-ghost' });
      back.addEventListener('click', () => {
        void load(view, container);
      });
      renderFormEditor(view, container, 'combat');
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

  let sheets: CombatSheet[] = [];
  try {
    sheets = await view.plugin.offersService.listCombat();
  } catch (e: unknown) {
    container.createDiv({ cls: 'tn-offers-error', text: `Не удалось загрузить боевые листки: ${errorMessage(e)}` });
    return;
  }
  if (sheets.length === 0) {
    container.createDiv({ cls: 'tn-offers-empty', text: 'Боевых листков пока нет.' });
    return;
  }

  const table = container.createEl('table', { cls: 'tn-offers-table' });
  const head = table.createEl('thead').createEl('tr');
  for (const title of ['Номер', 'Название', 'Наш продукт', 'Конкуренты', 'Статус', 'Менеджер', '']) {
    head.createEl('th', { text: title });
  }
  const body = table.createEl('tbody');
  for (const sheet of sheets) {
    const row = body.createEl('tr');
    const numberCell = row.createEl('td');
    const link = numberCell.createEl('a', { text: sheet.number, cls: 'tn-offers-link' });
    link.addEventListener('click', () => {
      void openEditor(view, container, sheet);
    });
    row.createEl('td', { text: sheet.title || '—' });
    row.createEl('td', { text: ourLabel(sheet) });
    row.createEl('td', { cls: 'tn-offers-num', text: String((sheet.competitorNotes ?? []).length) });
    const statusCell = row.createEl('td');
    statusCell.createSpan({
      cls: 'tn-offers-chip',
      text: sheet.status === 'final' ? 'Готово' : 'Черновик',
    });
    row.createEl('td', { cls: 'tn-offers-muted', text: sheet.ownerEmail });
    const act = row.createEl('td');
    const openBtn = act.createEl('button', { text: '✏️ Открыть', cls: 'tn-btn tn-btn-ghost' });
    openBtn.addEventListener('click', () => {
      void openEditor(view, container, sheet);
    });
  }
}

/** Подпись нашего продукта в списке: названия выбранных систем. */
function ourLabel(sheet: CombatSheet): string {
  const names = (sheet.ourNotes ?? []).map((n) => n.title).filter((t) => t !== '');
  if (names.length === 0) return '—';
  return names.join(', ');
}

async function openEditor(view: OffersView, container: HTMLElement, sheet: CombatSheet | null): Promise<void> {
  let current: CombatSheet;
  try {
    if (sheet) {
      current = await view.plugin.offersService.getCombat(sheet.id);
    } else {
      current = {
        id: 0,
        number: '',
        ownerEmail: view.email,
        title: 'Боевой листок',
        offerId: 0,
        ourNotes: [],
        competitorNotes: [],
        formValues: {},
        status: 'draft',
        createdAt: '',
        updatedAt: '',
      };
    }
  } catch (e: unknown) {
    new Notice(`Не удалось открыть боевой листок: ${errorMessage(e)}`);
    return;
  }

  container.empty();
  const back = container.createEl('button', { text: '← К списку', cls: 'tn-btn tn-btn-ghost' });
  back.addEventListener('click', () => {
    void load(view, container);
  });

  const headCard = container.createDiv({ cls: 'tn-offers-card' });
  headCard.createDiv({ cls: 'tn-offers-muted', text: 'Название боевого листка' });
  const titleInput = headCard.createEl('input', { cls: 'tn-offers-input' });
  titleInput.value = current.title;
  titleInput.addEventListener('input', () => {
    current.title = titleInput.value;
  });

  // Наш продукт и продукты конкурентов — два независимых списка заметок.
  const ourNotes = current.ourNotes.map((n) => ({ ...n }));
  const competitorNotes = current.competitorNotes.map((n) => ({ ...n }));

  const ourCard = container.createDiv({ cls: 'tn-offers-card' });
  ourCard.createDiv({ cls: 'tn-offers-muted', text: 'Наш продукт (системы из КП)' });
  const ourList = ourCard.createDiv();
  const redrawOur = (): void => {
    ourList.empty();
    if (ourNotes.length === 0) {
      ourList.createDiv({ cls: 'tn-offers-muted', text: 'Наш продукт не выбран.' });
    }
    for (const note of ourNotes) {
      const row = ourList.createDiv({ cls: 'tn-offers-row tn-offers-chosen-system' });
      row.createSpan({ cls: 'tn-offers-grow', text: note.title });
      const remove = row.createEl('button', { text: 'Убрать', cls: 'tn-btn tn-btn-ghost' });
      remove.addEventListener('click', () => {
        const idx = ourNotes.indexOf(note);
        if (idx >= 0) ourNotes.splice(idx, 1);
        redrawOur();
      });
    }
  };
  const ourSelect = ourCard.createEl('select', { cls: 'tn-offers-input' });
  const pickedOur = ourCard.createEl('button', { text: '➕ Добавить нашу систему', cls: 'tn-btn tn-btn-ghost' });
  pickedOur.addEventListener('click', () => {
    const noteId = Number(ourSelect.value);
    const opt = Array.from(ourSelect.options).find((o) => o.value === String(noteId));
    if (!noteId || !opt) {
      new Notice('Выберите систему');
      return;
    }
    addNoteRef(ourNotes, { noteId, title: opt.textContent ?? '' });
    redrawOur();
  });
  void view.plugin.offersService
    .listSystems()
    .then((res) => {
      ourSelect.empty();
      ourSelect.createEl('option', { value: '', text: '— выбрать систему —' });
      if (!res.kbEnabled) {
        ourCard.createDiv({ cls: 'tn-offers-warn', text: 'База знаний не настроена' });
        return;
      }
      for (const system of res.systems) {
        ourSelect.createEl('option', { value: String(system.noteId), text: system.title });
      }
    })
    .catch((e: unknown) => {
      ourCard.createDiv({ cls: 'tn-offers-warn', text: `Список систем: ${errorMessage(e)}` });
    });
  redrawOur();

  const compCard = container.createDiv({ cls: 'tn-offers-card' });
  compCard.createDiv({ cls: 'tn-offers-muted', text: 'Продукты конкурентов (из Базы знаний)' });
  const compList = compCard.createDiv();
  const redrawComp = (): void => {
    compList.empty();
    if (competitorNotes.length === 0) {
      compList.createDiv({ cls: 'tn-offers-muted', text: 'Продукты конкурентов не выбраны.' });
    }
    for (const note of competitorNotes) {
      const row = compList.createDiv({ cls: 'tn-offers-row tn-offers-chosen-system' });
      row.createSpan({ cls: 'tn-offers-grow', text: note.title });
      const remove = row.createEl('button', { text: 'Убрать', cls: 'tn-btn tn-btn-ghost' });
      remove.addEventListener('click', () => {
        const idx = competitorNotes.indexOf(note);
        if (idx >= 0) competitorNotes.splice(idx, 1);
        redrawComp();
      });
    }
  };
  const compSelect = compCard.createEl('select', { cls: 'tn-offers-input' });
  const pickedComp = compCard.createEl('button', { text: '➕ Добавить конкурента', cls: 'tn-btn tn-btn-ghost' });
  pickedComp.addEventListener('click', () => {
    const noteId = Number(compSelect.value);
    const opt = Array.from(compSelect.options).find((o) => o.value === String(noteId));
    if (!noteId || !opt) {
      new Notice('Выберите продукт конкурента');
      return;
    }
    addNoteRef(competitorNotes, { noteId, title: opt.textContent ?? '' });
    redrawComp();
  });
  void view.plugin.offersService
    .listCompetitors()
    .then((res) => {
      compSelect.empty();
      compSelect.createEl('option', { value: '', text: '— выбрать продукт конкурента —' });
      if (!res.kbEnabled) {
        compCard.createDiv({ cls: 'tn-offers-warn', text: 'База знаний не настроена' });
        return;
      }
      if (!res.folderFound) {
        compCard.createDiv({
          cls: 'tn-offers-warn',
          text: 'В Базе знаний нет папки «материалы и системы конкурентов» — продукты конкурентов пока не заведены.',
        });
        return;
      }
      for (const card of res.cards as SystemCard[]) {
        compSelect.createEl('option', { value: String(card.noteId), text: card.title });
      }
    })
    .catch((e: unknown) => {
      compCard.createDiv({ cls: 'tn-offers-warn', text: `Список конкурентов: ${errorMessage(e)}` });
    });
  redrawComp();

  // Поля формы боевого листка — те же подстановки, что в форме КП, но под
  // режимом combat.
  const formValues: Record<string, string> = { ...(current.formValues ?? {}) };
  const formCard = container.createDiv({ cls: 'tn-offers-card' });
  formCard.createDiv({ cls: 'tn-offers-muted', text: 'Поля формы' });
  try {
    const form = await view.plugin.offersService.getForm('combat');
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
  docxBtn.addEventListener('click', () => {
    if (!current.id) {
      new Notice('Сначала сохраните боевой листок');
      return;
    }
    void downloadCombatDocx(view, current);
  });

  const collect = (status: CombatSheet['status']): CombatSheet => ({
    ...current,
    title: titleInput.value.trim(),
    ourNotes,
    competitorNotes,
    formValues,
    status,
  });

  saveBtn.addEventListener('click', () => {
    void doSave('draft');
  });
  finBtn.addEventListener('click', () => {
    void doSave('final');
  });

  async function doSave(status: CombatSheet['status']): Promise<void> {
    try {
      current = await view.plugin.offersService.saveCombat(collect(status));
      new Notice(`Боевой листок ${current.number} сохранён`);
      await load(view, container);
    } catch (e: unknown) {
      new Notice(`Не удалось сохранить: ${errorMessage(e)}`);
    }
  }
}

/** Добавить заметку в список без повторов: один продукт — одна строка. */
function addNoteRef(list: CombatNoteRef[], ref: CombatNoteRef): void {
  if (list.some((n) => n.noteId === ref.noteId)) return;
  list.push(ref);
}

/** Скачать DOCX боевого листка рядом с КП (та же папка выгрузок). */
async function downloadCombatDocx(view: OffersView, sheet: CombatSheet): Promise<void> {
  try {
    const fileName = `Боевой листок ${sheet.number.replace('/', '-')}.docx`;
    const bytes = await view.plugin.offersService.exportCombatDocx(sheet.id);
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