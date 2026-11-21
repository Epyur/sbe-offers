import { Notice } from 'obsidian';
import type { OffersView } from './offers-view';
import {
  canEditOffers,
  type CombatNoteRef,
  type CombatSheet,
  type Material,
  type SystemCard,
} from '../types/offers';
import { errorMessage } from '../../../sbe-core/src/utils/errors';
import { renderCombatSettings } from './combat-settings';
import { PICKER_EMPTY_HINT, pickerRows, type PickerCandidate } from './picker-model';

/** Сторона выбора в боевом листке: наш продукт или конкурент. */
type CombatSide = 'our' | 'competitor';

/** Вариант выбора стороны: заметка Базы знаний и её подпись. */
type CombatCandidate = PickerCandidate;

/**
 * Раздел «⚙️ Боевой листок» (2026-11-08).
 *
 * Внутренний вид повторяет раздел «КП» (offers-list.ts): список документов,
 * карточка редактора, шапка с кнопками. Отличие — в источниках: наш продукт
 * выбирается из систем (или берётся из привязанного КП), продукты конкурентов —
 * из папки Базы знаний «материалы и системы конкурентов». Сборка DOCX
 * появится отдельной партией; сейчас листок сохраняется и показывается.
 */
/**
 * Ключ уточнения для разбора в `formValues` листка (2026-11-11).
 *
 * Один ключ на плагин и бота: у боевого листка из полей формы живой остаётся
 * только уточнение, и это единственное значение, которое человек вводит руками.
 * Совпадает с `combatHintField` в offers-service.
 */
export const COMBAT_HINT_FIELD = 'hint';

/**
 * Карточка макета боевого листка и настройки листка (2026-11-17).
 *
 * Раньше здесь стояла кнопка «📄 Макет боевого листка», которая открывала
 * карточку макета с метками и образцом КП. Пользователь попросил убрать её и
 * завести одно окно настроек: промт, ссылки, макет. Окно живёт отдельным
 * экраном (`combat-settings.ts`), потому что у листка нет полей формы и блоков.
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
  // Настройки боевого листка (2026-11-17, просьба пользователя): базовый промт,
  // ссылки и макет в одном окне. Раньше здесь была кнопка «Макет боевого
  // листка», и настройка промта жила в мёртвом пути общего редактора формы.
  if (view.admin) {
    const settingsBtn = actions.createEl('button', {
      text: '⚙️ Настройки боевого листка',
      cls: 'tn-btn tn-btn-ghost',
    });
    settingsBtn.addEventListener('click', () => {
      renderCombatSettings(view, container, () => {
        void load(view, container);
      });
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

  // Наш продукт: материалы каталога и системы Базы знаний (2026-11-09).
  // Каталог — 1500+ записей, поэтому выбор идёт через поиск, а не списком.
  const ourCard = container.createDiv({ cls: 'tn-offers-card' });
  const ourCandidates: CombatCandidate[] = [];
  renderSidePicker(view, ourCard, {
    side: 'our',
    chosen: ourNotes,
    candidates: () => ourCandidates,
    load: async () => {
      ourCandidates.length = 0;
      const [materials, systems] = await Promise.all([
        view.plugin.offersService.listMaterials(),
        view.plugin.offersService.listSystems(),
      ]);
      for (const m of materials as Material[]) {
        if (m.noteId && m.noteId > 0) {
          ourCandidates.push({ noteId: m.noteId, title: m.name, code: m.sku });
        }
      }
      for (const s of systems.systems) {
        if (s.noteId > 0) {
          ourCandidates.push({ noteId: s.noteId, title: s.title, code: s.sku });
        }
      }
      if (!systems.kbEnabled) {
        ourCard.createDiv({ cls: 'tn-offers-warn', text: 'База знаний не настроена — доступны только материалы каталога.' });
      }
    },
  });

  // Продукты конкурентов: всё поддерево папки Базы знаний (2026-11-09),
  // а по запросу — полнотекстовый поиск Базы знаний, как в веб-версии
  // (2026-11-12). Кэш нужен для «▼ Все» и для случая, когда поиск недоступен.
  const compCard = container.createDiv({ cls: 'tn-offers-card' });
  const compCandidates: CombatCandidate[] = [];
  renderSidePicker(view, compCard, {
    side: 'competitor',
    chosen: competitorNotes,
    candidates: () => compCandidates,
    // Поиск по Базе знаний: сервер отвечает заметками с тегом `competitor`.
    // Пустой ответ (или ошибка) — не тупик: показываем, что нашлось по кэшу.
    search: async (query) => {
      const res = await view.plugin.offersService.searchCompetitors(query);
      if (res.error) {
        new Notice(`Поиск по Базе знаний: ${res.error}`);
        return null;
      }
      return res.items.map((item) => ({ noteId: item.id, title: item.title }));
    },
    load: async () => {
      compCandidates.length = 0;
      const res = await view.plugin.offersService.listCompetitors();
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
        compCandidates.push({ noteId: card.noteId, title: card.title, code: card.sku });
      }
    },
  });

  // Уточнение для разбора — единственная настройка листка (2026-11-11).
  // Содержание документа фиксировано, а здесь менеджер говорит, на чём ИИ
  // должен сделать акцент в сравнении. Хранится в `formValues["hint"]`, как в
  // боте: один ключ на оба интерфейса.
  const formValues: Record<string, string> = { ...(current.formValues ?? {}) };
  const hintCard = container.createDiv({ cls: 'tn-offers-card' });
  hintCard.createDiv({ cls: 'tn-offers-muted', text: 'Уточнение для разбора' });
  hintCard.createDiv({
    cls: 'tn-offers-muted',
    text:
      'Содержание боевого листка всегда одинаковое: сначала перечень продуктов, ' +
      'затем их сравнение. Здесь — на чём сделать акцент, например «огнестойкость» ' +
      'или «стоимость владения».',
  });
  const hintInput = hintCard.createEl('textarea', { cls: 'tn-offers-input tn-offers-grow' });
  hintInput.rows = 3;
  hintInput.value = formValues[COMBAT_HINT_FIELD] ?? '';
  hintInput.addEventListener('input', () => {
    const value = hintInput.value.trim();
    if (value) formValues[COMBAT_HINT_FIELD] = value;
    else delete formValues[COMBAT_HINT_FIELD];
  });

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

/**
 * Выбор стороны боевого листка с поиском (2026-11-09).
 *
 * Раньше это был обычный `<select>`: в каталоге 1500+ записей, прокрутить его
 * до нужного продукта нельзя. Теперь — строка поиска и найденные варианты
 * кнопками; выбранные продукты остаются списком выше и убираются по одному.
 * Сторона может быть добавлена несколько раз: сравнение бывает и «три наших
 * продукта против двух конкурентов».
 */
function renderSidePicker(
  _view: OffersView,
  card: HTMLElement,
  opts: {
    side: CombatSide;
    chosen: CombatNoteRef[];
    candidates: () => CombatCandidate[];
    load: () => Promise<void>;
    /**
     * Поиск стороны на сервере (2026-11-12). Есть у конкурентов: полнотекстовый
     * поиск Базы знаний находит «Кровельная ПВХ мембрана FACHMANN …» по запросу
     * «ПВХ мембрана» и `Sikaplan®` — по смыслу, чего фильтр по подстроке в
     * кэше не умеет. `null` — поиск недоступен, остаётся фильтр по кэшу.
     */
    search?: (query: string) => Promise<CombatCandidate[] | null>;
  },
): void {
  const label = opts.side === 'our' ? 'Наш продукт' : 'Продукты конкурентов';
  card.createDiv({ cls: 'tn-offers-muted', text: label });

  const chosenList = card.createDiv();
  const redrawChosen = (): void => {
    chosenList.empty();
    if (opts.chosen.length === 0) {
      chosenList.createDiv({ cls: 'tn-offers-muted', text: 'Пока не выбран.' });
    }
    for (const note of opts.chosen) {
      const row = chosenList.createDiv({ cls: 'tn-offers-row tn-offers-chosen-system' });
      row.createSpan({ cls: 'tn-offers-grow', text: note.title });
      const remove = row.createEl('button', { text: 'Убрать', cls: 'tn-btn tn-btn-ghost' });
      remove.addEventListener('click', () => {
        const idx = opts.chosen.indexOf(note);
        if (idx >= 0) opts.chosen.splice(idx, 1);
        redrawChosen();
      });
    }
  };
  redrawChosen();

  const search = card.createEl('input', { cls: 'tn-offers-input', type: 'text' });
  search.placeholder =
    opts.side === 'our'
      ? 'Поиск продукта по названию или коду…'
      : 'Поиск продукта конкурента по названию…';
  const results = card.createDiv({ cls: 'tn-offers-row-list' });
  // Найденное сервером для текущего запроса; `null` — сервер не искал.
  let foundByServer: CombatCandidate[] | null = null;
  let searchSeq = 0;

  const shown = (query: string): CombatCandidate[] =>
    pickerRows(opts.candidates(), foundByServer, query);

  const renderItems = (items: CombatCandidate[]): void => {
    results.empty();
    if (items.length === 0) {
      // До ввода списка нет — то же правило, что в окне КП и в мини-приложении
      // (2026-11-21): человек начинает с названия, а не с чужого каталога.
      if (search.value.trim() === '') {
        results.createDiv({ cls: 'tn-offers-muted', text: PICKER_EMPTY_HINT });
        return;
      }
      if (opts.candidates().length === 0 && foundByServer === null) {
        results.createDiv({ cls: 'tn-offers-muted', text: 'Список пуст — нечего выбирать.' });
        return;
      }
      results.createDiv({ cls: 'tn-offers-muted', text: 'Ничего не нашлось. Уточните название.' });
      return;
    }
    for (const item of items) {
      const add = results.createEl('button', {
        cls: 'tn-btn tn-btn-ghost tn-offers-add-result',
        text: `➕ ${item.title}${item.code ? ` (${item.code})` : ''}`,
      });
      add.addEventListener('click', () => {
        addNoteRef(opts.chosen, { noteId: item.noteId, title: item.title });
        redrawChosen();
        search.value = '';
        foundByServer = null;
        redrawResults();
      });
    }
  };

  const redrawResults = (): void => {
    renderItems(shown(search.value));
  };

  // Поиск на сервере — с задержкой: человек набирает название, и запрос на
  // каждую букву превратил бы набор в очередь к Базе знаний.
  let timer: number | null = null;
  const runServerSearch = (query: string): void => {
    if (!opts.search || !query.trim()) {
      foundByServer = null;
      redrawResults();
      return;
    }
    const seq = ++searchSeq;
    void opts
      .search(query)
      .then((items) => {
        // Ответ на старый запрос не должен затирать свежий.
        if (seq !== searchSeq) return;
        foundByServer = items;
        redrawResults();
      })
      .catch((e: unknown) => {
        if (seq !== searchSeq) return;
        foundByServer = null;
        results.empty();
        results.createDiv({ cls: 'tn-offers-warn', text: `Поиск: ${errorMessage(e)}` });
      });
  };

  search.addEventListener('input', () => {
    const query = search.value;
    if (timer !== null) window.clearTimeout(timer);
    // До ответа сервера показываем локальные совпадения по кэшу — экран не
    // пустует, пока идёт запрос.
    foundByServer = null;
    redrawResults();
    if (!opts.search) return;
    timer = window.setTimeout(() => runServerSearch(query), 350);
  });
  redrawResults();

  void opts
    .load()
    .then(redrawResults)
    .catch((e: unknown) => {
      results.createDiv({ cls: 'tn-offers-warn', text: `Список: ${errorMessage(e)}` });
    });
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