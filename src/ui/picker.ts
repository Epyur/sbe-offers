/**
 * Выбор продукта поиском (2026-11-12).
 *
 * Раньше выбор стороны в боевом листке и выбор системы/материала в КП были
 * двумя разными механизмами: в листке — поиск с кнопками, в КП — дропдаун на
 * сотни строк, в котором нужную систему не найти. Здесь одно окно на все три
 * случая.
 *
 * Источник найденного задаёт вызывающий: боевой листок берёт кэш + поиск
 * конкурентов, КП — поиск систем и материалов. Окно о поиске не знает, оно
 * умеет только «покажи найденное кнопками и запомни выбор».
 */

/** Вариант выбора: заметка Базы знаний и её подпись. */
export interface PickerCandidate {
  noteId: number;
  title: string;
  code?: string;
}

/** Опции окна выбора. */
export interface PickerOptions {
  /** Заголовок окна, например «Наш продукт». */
  label: string;
  /** Подсказка в поле поиска. */
  placeholder: string;
  /** Что уже выбрано: строки списка и их кнопки «Убрать». */
  chosen: { title: string }[];
  /** Кэш найденного: показывается без запроса и когда поиск недоступен. */
  candidates: () => PickerCandidate[];
  /** Заполнить кэш (админский список из Базы знаний). */
  load: () => Promise<void>;
  /**
   * Поиск на сервере. `null` — поиск недоступен, остаётся фильтр по кэшу.
   * Без этой функции окно работает как фильтр по `candidates()`.
   */
  search?: (query: string) => Promise<PickerCandidate[] | null>;
  /**
   * Что сделать с выбранным. Окно само решает, как выглядит строка списка;
   * вызывающий решает, что значит выбор: добавить систему в КП, добавить
   * позицию, добавить сторону листка.
   */
  onPick: (candidate: PickerCandidate) => void | Promise<void>;
  /**
   * Убрать выбранное. Не задано — строки списка только показываются (в КП
   * системами управляет отдельный блок с «Убрать» и «Пересчитать»).
   */
  onRemove?: (index: number) => void | Promise<void>;
  /** Текст, когда ничего не выбрано. */
  emptyText?: string;
}

/**
 * Совпадение по запросу: слова ищутся по названию и коду, регистр и «ё» не
 * важны. То же правило, что в боте — одно место на все окна.
 */
export function filterCandidates(items: PickerCandidate[], query: string): PickerCandidate[] {
  const q = query.trim().toLowerCase().replace(/ё/g, 'е');
  if (!q) return items;
  const words = q.split(/\s+/).filter((w) => w !== '');
  return items.filter((item) => {
    const hay = `${item.title} ${item.code ?? ''}`.toLowerCase().replace(/ё/g, 'е');
    return words.every((w) => hay.includes(w));
  });
}

/**
 * Отрисовать окно выбора в `card`.
 *
 * Возвращает функцию перерисовки списка выбранного — вызывающему она нужна,
 * когда он меняет выбор не через окно (например, после сохранения КП).
 */
export function renderPicker(
  card: HTMLElement,
  opts: PickerOptions,
  errorMessage: (e: unknown) => string,
): () => void {
  card.createDiv({ cls: 'tn-offers-muted', text: opts.label });

  const chosenList = card.createDiv();
  const redrawChosen = (): void => {
    chosenList.empty();
    if (opts.chosen.length === 0) {
      chosenList.createDiv({ cls: 'tn-offers-muted', text: opts.emptyText ?? 'Пока не выбран.' });
    }
    opts.chosen.forEach((entry, index) => {
      const row = chosenList.createDiv({ cls: 'tn-offers-row tn-offers-chosen-system' });
      row.createSpan({ cls: 'tn-offers-grow', text: entry.title });
      if (!opts.onRemove) return;
      const remove = row.createEl('button', { text: 'Убрать', cls: 'tn-btn tn-btn-ghost' });
      remove.addEventListener('click', () => {
        void Promise.resolve(opts.onRemove?.(index)).then(redrawChosen);
      });
    });
  };
  redrawChosen();

  const search = card.createEl('input', { cls: 'tn-offers-input', type: 'text' });
  search.placeholder = opts.placeholder;
  const results = card.createDiv({ cls: 'tn-offers-row-list' });
  // Найденное сервером для текущего запроса; `null` — сервер не искал.
  let foundByServer: PickerCandidate[] | null = null;
  let searchSeq = 0;

  const shown = (query: string): PickerCandidate[] => {
    const found = foundByServer ?? filterCandidates(opts.candidates(), query);
    // Без запроса показываем небольшой срез: весь каталог — это тысячи строк,
    // которые человек всё равно не прочитает.
    return query.trim() ? found : found.slice(0, 20);
  };

  const renderItems = (items: PickerCandidate[]): void => {
    results.empty();
    if (items.length === 0) {
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
        void Promise.resolve(opts.onPick(item)).then(() => {
          redrawChosen();
          search.value = '';
          foundByServer = null;
          redrawResults();
        });
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

  return redrawChosen;
}

/** Добавить заметку в список без повторов: один продукт — одна строка. */
export function addUnique<T extends { noteId: number }>(list: T[], ref: T): void {
  if (list.some((item) => item.noteId === ref.noteId)) return;
  list.push(ref);
}