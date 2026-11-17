import { Notice } from 'obsidian';
import type { OffersView } from './offers-view';
import type { Block, ContentSection, OfferForm, Placeholder } from '../types/offers';
import { renderBlockEditor } from './block-editor';
import { pickAndUploadImage } from './image-picker';
import { renderFormPreview } from './form-preview';
import { DescribeBlockModal, applyDescribedBlock } from './describe-block-modal';
import { pickLayoutFile, renderLayoutCard } from './form-layout';
import { renderSectionsEditor, SECTIONS_EDITOR_TEXTS } from './sections-editor';
import { renderCollapsibleGroup } from './collapsible-group';
import { WINDOW_EDITOR_TEXTS, renderWindowEditor, type WindowEditorHandle } from './window-editor';
import { windowForEditor, windowForSave, windowProblem, windowSummary } from './window-model';
import { errorMessage } from '../../../sbe-core/src/utils/errors';

/**
 * Конфигуратор формы КП (раздел «⚙️ Форма КП», только admin): заголовок,
 * поля-плейсхолдеры, блоки документа и обязательные поля.
 *
 * `mode` (2026-11-08) — какой форме принадлежит редактор: `'client'` (КП) или
 * `'combat'` (боевой листок). Редактор ОДИН на оба документа: блоки,
 * подстановки и макет .docx у них одинаковой природы, и второй редактор
 * разошёлся бы с первым при первой же правке. Отличается только режим строки
 * в `offers_form` и подписи, которые показывает карточка документа.
 */
export function renderFormEditor(view: OffersView, container: HTMLElement, mode = 'client'): void {
  container.createDiv({ cls: 'tn-offers-muted', text: 'Загрузка формы…' });
  void load(view, container, mode);
}

async function load(view: OffersView, container: HTMLElement, mode: string): Promise<void> {
  let form: OfferForm;
  try {
    form = await view.plugin.offersService.getForm(mode);
  } catch (e: unknown) {
    container.empty();
    container.createDiv({ cls: 'tn-offers-error', text: `Не удалось загрузить форму: ${errorMessage(e)}` });
    return;
  }
  container.empty();
  render(view, container, form, mode);
}

function render(view: OffersView, container: HTMLElement, form: OfferForm, mode: string): void {
  const placeholders: Placeholder[] = form.placeholders.map((p) => ({ ...p }));
  const blocks = form.blocks.map((b) => ({ ...b, content: JSON.parse(JSON.stringify(b.content)) }));
  // Содержательные разделы (2026-11-14): на них ссылается метка `{ content: <id> }`.
  // Копия — как у блоков: правки в редакторе до сохранения не трогают форму.
  const sections: ContentSection[] = (form.sections ?? []).map((s) => ({
    ...s,
    subsections: s.subsections.map((sub) => ({ ...sub })),
  }));
  const required = new Set(form.requiredFields);

  // Базовый промт (2026-11-17): общая часть системного промта, которую видит
  // модель при написании каждого подраздела. Раньше этот текст жил в коде, и
  // правка формулировок требовала выпуска плагина. Сейчас — настройка формы:
  // живой листок 017/2026 дал противоречия между подразделами, и менеджеру
  // нужно править правила самому.
  const basePromptInitial = (form.basePrompt ?? '').trim();
  const basePromptDefault = (form.basePromptDefault ?? '').trim();

  // Группы формы (2026-11-14, дефект 0.10.2): каждая карточка — сворачиваемая
  // и по умолчанию свёрнута. Просьба пользователя. Один компонент на все
  // группы (`collapsible-group.ts`) — пять своих копий разъехались бы.
  // Кнопки «Сохранить/Отменить» остаются ВНЕ групп: иначе после сворачивания
  // всего к ним не добраться.
  const titleRow = container.createDiv({ cls: 'tn-offers-card' });
  titleRow.createDiv({ cls: 'tn-offers-muted', text: 'Заголовок документа' });
  const titleInput = titleRow.createEl('input', { cls: 'tn-offers-input' });
  titleInput.value = form.title;

  const fieldsGroup = renderCollapsibleGroup(container, {
    title: 'Поля формы',
    hint: 'подставляются вместо {{поле}}',
  });
  const fieldsCard = fieldsGroup.body;
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
        redrawFields();
      });
    });
  };
  drawFields();
  const addField = fieldsCard.createEl('button', { text: '➕ Поле', cls: 'tn-btn tn-btn-ghost' });
  addField.addEventListener('click', () => {
    placeholders.push({ id: `field_${placeholders.length + 1}`, label: 'Новое поле' });
    redrawFields();
  });

  // Окно КП в приложении (2026-11-15, шаг 4). Сразу после «Полей формы»: там
  // администратор заводит поля, и следующая карточка обязана отвечать на
  // вопрос «где они будут у менеджера». Пусто с сервера — «как сейчас».
  const windowItems = windowForEditor(form.window, placeholders);
  const windowGroup = renderCollapsibleGroup(container, {
    title: WINDOW_EDITOR_TEXTS.groupTitle,
    hint: windowSummary(windowItems),
  });
  const windowHint = windowGroup.body
    .closest('.tn-offers-group')
    ?.querySelector('.tn-offers-group-hint');
  let windowEditor: WindowEditorHandle | null = null;

  // Окно зависит от `placeholders`, поэтому перерисовывается при каждой правке
  // списка полей формы: убранное поле уходит из окна, новое дописывается.
  const drawWindow = (): void => {
    windowEditor = renderWindowEditor(windowGroup.body, windowItems, placeholders, {
      onChange: () => {
        // Заголовок группы несёт сводку: «видно N, обязательных M» — она
        // должна быть верной в СВЁРНУТОМ виде, а не только в раскрытом.
        if (windowHint instanceof HTMLElement) windowHint.setText(windowSummary(windowItems));
      },
    });
  };
  drawWindow();

  /** Правка списка полей: за ней обязано последовать окно.
   *
   *  Объявлена функцией, а не `const`: её зовут обработчики «✖»/«➕ Поле»,
   *  объявленные выше по коду, и `const` оказался бы ещё в TDZ в момент
   *  развешивания слушателей. */
  function redrawFields(): void {
    drawFields();
    windowEditor?.syncWithForm(placeholders);
  }

  // Блоки документа: заголовок группы несёт счётчик и кнопку «Опишите блок
  // словами» — она принадлежит блокам и должна быть видна, когда группа
  // свёрнута: это вход в редактор, а не его содержимое.
  const blockCount = blocks.length;
  const blocksGroup = renderCollapsibleGroup(container, {
    title: 'Блоки документа',
    hint: blockCount > 0 ? `блоков: ${blockCount}` : 'блоков нет',
  });
  const blocksCard = blocksGroup.body;
  const blocksHeader = blocksCard.createDiv({ cls: 'tn-offers-row-between' });
  renderDraftHints(blocksCard, blocks);

  const editorWrap = blocksCard.createDiv();

  // Содержательные разделы (2026-11-14, шаг 6). Блоки — вёрстка, написанная
  // руками; раздел — бриф для модели. Настройка на метку `{ content: <id> }`.
  //
  // Порядок карточек (2026-11-14, дефект 0.10.2): разделы выше предпросмотра.
  // Предпросмотр показывает, КАК напечатается то, что задано выше, — а задан
  // раздел. Стоя ниже, он читался как продолжение предпросмотра, и человек
  // искал настройку раздела после картинки готового документа.
  //
  // Макет без метки `{ content: <id> }` разделы не печатает. Счётчик в
  // заголовке группы говорит, задан ли хоть один раздел.
  const sectionsGroup = renderCollapsibleGroup(container, {
    title: SECTIONS_EDITOR_TEXTS.groupTitle,
    hint: sections.length > 0 ? `разделов: ${sections.length}` : 'разделов нет',
  });
  renderSectionsEditor(sectionsGroup.body, sections, {
    // Предложение здесь не открыто: редактор формы задаёт правила, а не
    // конкретный КП. Без предложения предпросмотр показывает форму рамки и
    // условия словами; условия, зависящие от предложения, видны как «попадут».
    onChange: () => {
      /* Предпросмотр разделов живёт внутри самого редактора. */
    },
  });

  // Базовый промт документа (2026-11-17): общая часть системного промта, которую
  // видит модель в КАЖДОМ подразделе. Пусто — встроенный текст (кнопка ниже его
  // возвращает). Здесь же правятся требования к согласованности разделов: сами
  // подразделы пишутся отдельными вызовами модели и друг друга не видят.
  const basePromptGroup = renderCollapsibleGroup(container, {
    title: mode === 'combat' ? 'Базовый промт боевого листка' : 'Базовый промт документа',
    hint: basePromptInitial ? 'свой текст' : 'встроенный',
  });
  const basePromptArea = basePromptGroup.body.createEl('textarea', {
    cls: 'tn-offers-input tn-offers-base-prompt',
    attr: { rows: '14', spellcheck: 'false' },
  });
  basePromptArea.value = basePromptInitial || basePromptDefault;
  basePromptGroup.body.createDiv({
    cls: 'tn-offers-muted',
    text:
      'Этот текст получает модель вместе с брифом каждого подраздела: формат ответа, ' +
      'правила сторон и формулировок. Пустое поле — встроенный текст.',
  });
  const basePromptReset = basePromptGroup.body.createEl('button', {
    cls: 'tn-offers-btn',
    text: '↩ Вернуть базовый текст',
  });
  basePromptReset.addEventListener('click', () => {
    basePromptArea.value = basePromptDefault;
  });

  const previewGroup = renderCollapsibleGroup(container, {
    title: 'Предпросмотр',
    hint: 'как увидит клиент',
    cls: 'tn-offers-preview-card',
  });
  const previewBody = previewGroup.body.createDiv({ cls: 'tn-offers-preview-body' });

  /** Предпросмотр перерисовывается при каждой правке: смысл его в том, что
   *  человек видит последствие правки сразу, а не в готовом DOCX. */
  const redrawPreview = (): void => {
    renderFormPreview(previewBody, blocks, { placeholders });
  };

  const redrawBlocks = (): void => {
    editorWrap.empty();
    renderBlockEditor(editorWrap, blocks, {
      placeholders,
      // Картинку можно выбрать прямо в блоке: файл из вольта уходит во вложения
      // Базы знаний, в блоке остаётся ключ (2026-11-01).
      pickImage: () => pickAndUploadImage(view),
      onChange: redrawPreview,
    });
    redrawPreview();
  };
  redrawBlocks();

  // «Опишите блок словами» (2026-11-05): настройка блока руками — это выбор
  // типа узла, таблицы через «|» и отступы. Администратор же знает, ЧТО должно
  // быть в блоке; описание превращается в узлы, а руками он дооформляет.
  const describeBtn = blocksHeader.createEl('button', {
    text: '✨ Опишите блок словами',
    cls: 'tn-btn tn-btn-primary',
  });
  describeBtn.addEventListener('click', () => {
    new DescribeBlockModal(view, placeholders, (title, nodes) => {
      applyDescribedBlock(blocks, `blk_${Date.now().toString(36)}`, title, nodes);
      redrawBlocks();
      new Notice('Блок добавлен в форму — поправьте его и сохраните');
    }).open();
  });

  // Макет .docx (2026-11-05): если он загружен, документ печатается по нему,
  // а блоки остаются в форме для возврата. Своя группа с состоянием в
  // заголовке: «какой макет сейчас печатает документ» — первое, что нужно
  // знать, и это должно быть видно СВЁРНУТЫМ.
  const layoutGroup = renderCollapsibleGroup(container, {
    title: 'Макет документа .docx',
    hint: form.hasLayout ? (form.layoutName || 'макет загружен') : 'макета нет',
  });
  renderLayoutCard(view, layoutGroup.body, form, {
    onChanged: () => {
      void load(view, container, mode);
    },
    onPickFile: pickLayoutFile,
    // Режим нужен, чтобы макет боевого листка загружался в свою форму, а не
    // подменял макет КП (2026-11-11).
    mode,
  });

  const actions = container.createDiv({ cls: 'tn-offers-actions' });
  const saveBtn = actions.createEl('button', { text: '💾 Сохранить форму', cls: 'tn-btn tn-btn-primary' });
  saveBtn.addEventListener('click', () => {
    void save();
  });
  const reloadBtn = actions.createEl('button', { text: '↻ Отменить правки', cls: 'tn-btn tn-btn-ghost' });
  reloadBtn.addEventListener('click', () => {
    void load(view, container, mode);
  });

  async function save(): Promise<void> {
    // Окно проверяется до запроса: неизвестная зона, поле-призрак или скрытое
    // обязательное отвергаются сервером, и человек должен узнать причину там,
    // где её создал, а не из общего отказа сохранения.
    const windowToSave = windowForSave(windowItems);
    const windowError = windowProblem(windowToSave, placeholders);
    if (windowError) {
      new Notice(`Окно КП: ${windowError}`);
      windowGroup.setOpen(true);
      return;
    }
    const payload: OfferForm = {
      ...form,
      title: titleInput.value.trim() || (mode === 'combat' ? 'Боевой листок' : 'Коммерческое предложение'),
      placeholders,
      blocks,
      sections,
      basePrompt: basePromptArea.value.trim(),
      window: windowToSave,
      requiredFields: Array.from(required),
    };
    try {
      await view.plugin.offersService.saveForm(payload);
      new Notice(mode === 'combat' ? 'Форма боевого листка сохранена' : 'Форма КП сохранена');
      await load(view, container, mode);
    } catch (e: unknown) {
      new Notice(`Не удалось сохранить форму: ${errorMessage(e)}`);
    }
  }
}

/** Показать замечания по блокам над редактором: пусто — замечаний нет. */
function renderDraftHints(container: HTMLElement, blocks: Block[]): void {
  const hints = draftBlockHints(blocks);
  if (hints.length === 0) return;
  const box = container.createDiv({ cls: 'tn-offers-warn tn-offers-form-hints' });
  box.createDiv({ text: 'В форме есть строки, которые клиент прочитает как черновик:' });
  const list = box.createEl('ul', { cls: 'tn-offers-problems' });
  for (const hint of hints) list.createEl('li', { text: hint });
}

/**
 * Что выглядит черновиком, но уйдёт клиенту (2026-11-02).
 *
 * Живой случай: в выгруженном КП над титулом стоял заголовок, которого никто
 * не заказывал, а в тексте — «Текст с полем» и английское задание модели.
 * Причина не в сборке документа, а в форме: блоки сохранились с этими
 * строками. Показываем их в редакторе, чтобы человек увидел до выгрузки.
 */
export function draftBlockHints(blocks: Block[]): string[] {
  const hints: string[] = [];
  for (const block of blocks) {
    if (!block.showInClient) continue;
    for (const node of block.content) {
      const texts = [node.text ?? '', ...(node.items ?? []), ...(node.rows ?? []).flat()];
      for (const text of texts) {
        if (text.trim() === '') continue;
        const reason = draftTextReason(text);
        if (reason) {
          hints.push(`Блок «${block.title || block.id}»: ${reason}`);
        }
      }
    }
  }
  return hints;
}

/** Почему строка выглядит недоделанной; пусто — выглядит готовой. */
function draftTextReason(text: string): string {
  if (/\btask\s*:/i.test(text) || /\bwrite\s+(one|a)\b/i.test(text)) {
    return 'похоже на задание модели, а не на текст для клиента';
  }
  if (text.includes('Текст с полем') || text.includes('Текст блока')) {
    return 'заводская заготовка редактора — её увидит клиент';
  }
  if (text.trim() === 'Условия:') {
    return 'подпись без условий — добавьте текст или уберите строку';
  }
  return '';
}
