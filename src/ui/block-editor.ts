import type { Block, Placeholder, RichNode } from '../types/offers';

/**
 * Блок-редактор формы КП — по образцу конфигуратора протоколов в ЛИМС
 * (sbe-lims/src/ui/block-editor.ts): блоки документа с чипами-плейсхолдерами,
 * которые подставляет сервер при печати.
 *
 * С 2026-11-01 узлы блока шире текста: разрыв страницы, ссылки из карточек,
 * текст по промту и картинка с обтеканием. Размещение блока (отступы, «с новой
 * страницы», «не разрывать») — это то, что человек называет «расположением
 * блока на странице»: абсолютного позиционирования здесь нет намеренно, оно не
 * переживает правку текста в Word.
 *
 * Модель узлов нарочно простая (текст+список+таблица+служебные), и хранится она
 * как RichNode[] — сервер разбирает её тем же описанием (offers-service/docx.go).
 */

export interface BlockEditorDeps {
  placeholders: Placeholder[];
  onChange?: () => void;
  /**
   * Выбор картинки для узла `image`. Возвращает ключ и имя файла в хранилище
   * Базы знаний; `null` — человек отказался.
   *
   * Живёт снаружи: у редактора нет доступа к файлам вольта и к хранилищу, а
   * тащить туда весь плагин ради одного диалога — лишняя связность.
   */
  pickImage?: () => Promise<{ key: string; name: string } | null>;
}

function uid(prefix: string): string {
  return `${prefix}_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
}

const NODE_LABELS: Array<[RichNode['type'], string]> = [
  ['paragraph', 'Абзац'],
  ['heading', 'Заголовок'],
  ['bullet_list', 'Список'],
  ['table', 'Таблица'],
  ['page_break', 'Разрыв страницы'],
  ['prompt_text', 'Текст по промту'],
  ['links', 'Ссылки'],
  ['image', 'Картинка'],
];

export function newBlock(placeholders: Placeholder[]): Block {
  const first = placeholders[0];
  const content: RichNode[] = [
    { type: 'paragraph', text: first ? `Текст с полем {{${first.id}}}` : 'Текст блока' },
  ];
  return {
    id: uid('blk'),
    title: 'Новый блок',
    content,
    showInClient: true,
    showInInternal: false,
  };
}

/** Заводская настройка нового узла: поля по типу, чтобы редактор не мигал пустотой. */
export function newNode(type: RichNode['type']): RichNode {
  const node: RichNode = { type };
  onNodeKindChanged(node);
  return node;
}

function nodeRow(
  container: HTMLElement,
  nodes: RichNode[],
  node: RichNode,
  index: number,
  deps: BlockEditorDeps,
  redraw: () => void,
): void {
  const row = container.createDiv({ cls: 'tn-offers-row tn-offers-block-node' });
  const kind = row.createEl('select', { cls: 'tn-offers-input tn-offers-node-kind' });
  for (const [value, label] of NODE_LABELS) {
    kind.createEl('option', { value, text: label });
  }
  kind.value = node.type;
  kind.addEventListener('change', () => {
    node.type = kind.value as RichNode['type'];
    onNodeKindChanged(node);
    deps.onChange?.();
    redraw();
  });

  const body = row.createDiv({ cls: 'tn-offers-grow' });
  renderNodeBody(body, node, deps);

  if (node.type === 'heading') {
    const level = row.createEl('select', { cls: 'tn-offers-input tn-offers-node-level' });
    for (const value of [2, 3, 4]) {
      level.createEl('option', { value: String(value), text: `Уровень ${value}` });
    }
    level.value = String(node.level ?? 2);
    level.addEventListener('change', () => {
      node.level = Number(level.value) as RichNode['level'];
      deps.onChange?.();
    });
  }

  if (node.type === 'paragraph' || node.type === 'heading' || node.type === 'prompt_text') {
    const align = row.createEl('select', { cls: 'tn-offers-input tn-offers-node-align' });
    for (const [value, label] of [
      ['', 'По ширине'],
      ['left', 'Слева'],
      ['center', 'По центру'],
      ['right', 'Справа'],
    ]) {
      align.createEl('option', { value, text: label });
    }
    align.value = node.align ?? '';
    align.addEventListener('change', () => {
      node.align = (align.value || undefined) as RichNode['align'];
      deps.onChange?.();
    });
  }

  const moveUp = row.createEl('button', { text: '▲', cls: 'tn-btn tn-btn-ghost' });
  moveUp.disabled = index === 0;
  moveUp.addEventListener('click', () => {
    [nodes[index - 1], nodes[index]] = [nodes[index], nodes[index - 1]];
    redraw();
  });
  const moveDown = row.createEl('button', { text: '▼', cls: 'tn-btn tn-btn-ghost' });
  moveDown.addEventListener('click', () => {
    if (index + 1 >= nodes.length) return;
    [nodes[index], nodes[index + 1]] = [nodes[index + 1], nodes[index]];
    redraw();
  });
  const remove = row.createEl('button', { text: '✖', cls: 'tn-btn tn-btn-ghost' });
  remove.addEventListener('click', () => {
    nodes.splice(index, 1);
    redraw();
  });
}

/** Тело узла: зависит от типа. Вынесено из nodeRow — у того и так пять веток. */
function renderNodeBody(body: HTMLElement, node: RichNode, deps: BlockEditorDeps): void {
  switch (node.type) {
    case 'page_break':
      body.createDiv({ cls: 'tn-offers-muted', text: '⏎ Содержимое после этого узла начнётся с новой страницы.' });
      return;
    case 'links':
      body.createDiv({
        cls: 'tn-offers-muted',
        text: '🔗 Ссылки собираются из карточек систем и материалов («## Ссылки»): страница на сайте и техдокументация на навигаторе.',
      });
      return;
    case 'prompt_text':
      renderPromptBody(body, node, deps);
      return;
    case 'image':
      renderImageBody(body, node, deps);
      return;
    case 'bullet_list': {
      const area = body.createEl('textarea', { cls: 'tn-offers-input' });
      area.rows = 3;
      area.value = (node.items ?? []).join('\n');
      area.addEventListener('input', () => {
        node.items = area.value.split('\n').map((line) => line.trimEnd()).filter((line) => line !== '');
        deps.onChange?.();
      });
      return;
    }
    case 'table': {
      const area = body.createEl('textarea', { cls: 'tn-offers-input' });
      area.rows = 3;
      area.value = (node.rows ?? []).map((cells) => cells.join(' | ')).join('\n');
      area.addEventListener('input', () => {
        node.rows = area.value
          .split('\n')
          .filter((line) => line.trim() !== '')
          .map((line) => line.split('|').map((cell) => cell.trim()));
        deps.onChange?.();
      });
      body.createDiv({ cls: 'tn-offers-muted', text: 'Первая строка — шапка. Ячейки через « | ».' });
      return;
    }
    default: {
      const input = body.createEl('textarea', { cls: 'tn-offers-input' });
      input.rows = 2;
      input.value = node.text ?? '';
      input.addEventListener('input', () => {
        node.text = input.value;
        deps.onChange?.();
      });
      // Поля вставляются туда, где стоит курсор: админ не должен помнить
      // синтаксис {{...}} наизусть.
      renderPlaceholderPicker(deps.placeholders, input);
    }
  }
}

/** Текст по промту: подсказка, уточнение, объём. */
function renderPromptBody(body: HTMLElement, node: RichNode, deps: BlockEditorDeps): void {
  body.createDiv({ cls: 'tn-offers-muted', text: 'Что написать (подсказка для модели):' });
  const instruction = body.createEl('textarea', { cls: 'tn-offers-input' });
  instruction.rows = 3;
  instruction.value = node.instruction ?? '';
  instruction.addEventListener('input', () => {
    node.instruction = instruction.value;
    deps.onChange?.();
  });
  body.createDiv({ cls: 'tn-offers-muted', text: 'Уточнение (необязательно):' });
  const hint = body.createEl('input', { cls: 'tn-offers-input' });
  hint.value = node.hint ?? '';
  hint.addEventListener('input', () => {
    node.hint = hint.value;
    deps.onChange?.();
  });
  const wordsRow = body.createDiv({ cls: 'tn-offers-row' });
  wordsRow.createSpan({ cls: 'tn-offers-muted', text: 'Слов не больше:' });
  const words = wordsRow.createEl('input', {
    cls: 'tn-offers-input tn-offers-num-input',
    attr: { type: 'number', min: '20', max: '800', step: '10' },
  });
  words.value = String(node.maxWords ?? 150);
  words.addEventListener('input', () => {
    node.maxWords = Number(words.value) || 0;
    deps.onChange?.();
  });
  body.createDiv({
    cls: 'tn-offers-muted',
    text: 'Текст пишет модель при сборке файла по данным КП. Модель недоступна — блок не печатается.',
  });
  renderPlaceholderPicker(deps.placeholders, instruction);
}

/** Картинка: выбор файла, ширина, сторона обтекания, подпись. */
function renderImageBody(body: HTMLElement, node: RichNode, deps: BlockEditorDeps): void {
  const pickRow = body.createDiv({ cls: 'tn-offers-row' });
  const pick = pickRow.createEl('button', {
    text: node.imageKey ? `🖼 ${node.imageName ?? node.imageKey}` : '🖼 Выбрать картинку',
    cls: 'tn-btn tn-btn-ghost',
  });
  pick.addEventListener('click', () => {
    void (async () => {
      const picked = await deps.pickImage?.();
      if (!picked) return;
      node.imageKey = picked.key;
      node.imageName = picked.name;
      deps.onChange?.();
      pick.setText(`🖼 ${picked.name}`);
    })();
  });

  const sizeRow = body.createDiv({ cls: 'tn-offers-row' });
  sizeRow.createSpan({ cls: 'tn-offers-muted', text: 'Ширина, см:' });
  const width = sizeRow.createEl('input', {
    cls: 'tn-offers-input tn-offers-num-input',
    attr: { type: 'number', min: '3', max: '18', step: '1' },
  });
  width.value = String(node.widthCm ?? 8);
  width.addEventListener('input', () => {
    node.widthCm = Number(width.value) || 0;
    deps.onChange?.();
  });

  const wrapRow = body.createDiv({ cls: 'tn-offers-row' });
  wrapRow.createSpan({ cls: 'tn-offers-muted', text: 'Обтекание:' });
  const wrap = wrapRow.createEl('select', { cls: 'tn-offers-input' });
  for (const [value, label] of [
    ['both', 'Текст с обеих сторон'],
    ['left', 'Картинка слева, текст справа'],
    ['right', 'Картинка справа, текст слева'],
    ['topAndBottom', 'Только сверху и снизу'],
    ['none', 'В тексте (без обтекания)'],
  ]) {
    wrap.createEl('option', { value, text: label });
  }
  wrap.value = node.wrap ?? 'both';
  wrap.addEventListener('change', () => {
    node.wrap = wrap.value as RichNode['wrap'];
    deps.onChange?.();
  });

  const caption = body.createEl('input', { cls: 'tn-offers-input', attr: { placeholder: 'Подпись (необязательно)' } });
  caption.value = node.caption ?? '';
  caption.addEventListener('input', () => {
    node.caption = caption.value;
    deps.onChange?.();
  });
  body.createDiv({
    cls: 'tn-offers-muted',
    text: 'Картинка плавающая: текст идёт рядом с ней, строка не разрывается.',
  });
}

function onNodeKindChanged(node: RichNode): void {
  switch (node.type) {
    case 'bullet_list':
      node.items = node.items ?? [node.text ?? ''];
      node.text = undefined;
      break;
    case 'table':
      node.rows = node.rows ?? [
        ['Колонка 1', 'Колонка 2'],
        ['', ''],
      ];
      break;
    case 'prompt_text':
      node.instruction = node.instruction ?? '';
      node.maxWords = node.maxWords ?? 150;
      break;
    case 'image':
      node.widthCm = node.widthCm ?? 8;
      node.wrap = node.wrap ?? 'both';
      break;
    default:
      node.text = node.text ?? '';
  }
}

export function renderPlaceholderPicker(placeholders: Placeholder[], target: HTMLTextAreaElement): void {
  if (placeholders.length === 0) return;
  const picker = target.parentElement?.createDiv({ cls: 'tn-offers-row tn-offers-chip-row' });
  if (!picker) return;
  picker.createSpan({ cls: 'tn-offers-muted', text: 'Вставить поле:' });
  for (const p of placeholders) {
    const chip = picker.createEl('button', { cls: 'tn-offers-chip tn-offers-chip-btn', text: p.label });
    chip.addEventListener('click', () => {
      const insert = `{{${p.id}}}`;
      const start = target.selectionStart ?? target.value.length;
      const end = target.selectionEnd ?? start;
      target.value = target.value.slice(0, start) + insert + target.value.slice(end);
      target.dispatchEvent(new Event('input'));
      target.focus();
    });
  }
}

export interface BlockEditorHandle {
  getBlocks: () => Block[];
}

/** Рисует редактор блоков прямо в контейнер. */
export function renderBlockEditor(
  container: HTMLElement,
  blocks: Block[],
  deps: BlockEditorDeps,
): BlockEditorHandle {
  const list = container.createDiv({ cls: 'tn-offers-block-list' });

  const redraw = (): void => {
    list.empty();
    blocks.forEach((block, blockIndex) => {
      const card = list.createDiv({ cls: 'tn-offers-card' });
      const head = card.createDiv({ cls: 'tn-offers-row-between' });

      const title = head.createEl('input', { cls: 'tn-offers-input tn-offers-grow' });
      title.value = block.title;
      title.addEventListener('input', () => {
        block.title = title.value;
        deps.onChange?.();
      });

      const upBtn = head.createEl('button', { text: '▲', cls: 'tn-btn tn-btn-ghost' });
      upBtn.disabled = blockIndex === 0;
      upBtn.addEventListener('click', () => {
        [blocks[blockIndex - 1], blocks[blockIndex]] = [blocks[blockIndex], blocks[blockIndex - 1]];
        deps.onChange?.();
        redraw();
      });
      const downBtn = head.createEl('button', { text: '▼', cls: 'tn-btn tn-btn-ghost' });
      downBtn.disabled = blockIndex === blocks.length - 1;
      downBtn.addEventListener('click', () => {
        [blocks[blockIndex], blocks[blockIndex + 1]] = [blocks[blockIndex + 1], blocks[blockIndex]];
        deps.onChange?.();
        redraw();
      });
      const delBtn = head.createEl('button', { text: '✖ Удалить блок', cls: 'tn-btn tn-btn-ghost' });
      delBtn.addEventListener('click', () => {
        blocks.splice(blockIndex, 1);
        deps.onChange?.();
        redraw();
      });

      renderBlockFlags(card, block, deps);
      renderBlockPlacement(card, block, deps);

      const nodesWrap = card.createDiv();
      block.content.forEach((node, nodeIndex) => {
        nodeRow(nodesWrap, block.content, node, nodeIndex, deps, () => {
          deps.onChange?.();
          redraw();
        });
      });

      const addRow = card.createDiv({ cls: 'tn-offers-row' });
      for (const [type, label] of NODE_LABELS) {
        const btn = addRow.createEl('button', { text: `➕ ${label}`, cls: 'tn-btn tn-btn-ghost' });
        btn.addEventListener('click', () => {
          block.content.push(newNode(type));
          deps.onChange?.();
          redraw();
        });
      }

      const picker = card.createDiv({ cls: 'tn-offers-row tn-offers-chip-row' });
      picker.createSpan({ cls: 'tn-offers-muted', text: 'Поля:' });
      for (const p of deps.placeholders) {
        picker.createSpan({ cls: 'tn-offers-chip', text: `{{${p.id}}} · ${p.label}` });
      }
    });
  };

  redraw();

  const addBlock = container.createEl('button', { text: '➕ Блок', cls: 'tn-btn' });
  addBlock.addEventListener('click', () => {
    blocks.push(newBlock(deps.placeholders));
    deps.onChange?.();
    redraw();
  });

  return { getBlocks: () => blocks };
}

/** Флаги показа: клиенту / внутренний. */
function renderBlockFlags(card: HTMLElement, block: Block, deps: BlockEditorDeps): void {
  const flags = card.createDiv({ cls: 'tn-offers-row' });
  const clientFlag = flags.createEl('label');
  const clientBox = clientFlag.createEl('input', { attr: { type: 'checkbox' } });
  clientBox.checked = block.showInClient;
  clientBox.addEventListener('change', () => {
    block.showInClient = clientBox.checked;
    deps.onChange?.();
  });
  clientFlag.createSpan({ text: ' Показывать клиенту' });

  const internalFlag = flags.createEl('label');
  const internalBox = internalFlag.createEl('input', { attr: { type: 'checkbox' } });
  internalBox.checked = block.showInInternal;
  internalBox.addEventListener('change', () => {
    block.showInInternal = internalBox.checked;
    deps.onChange?.();
  });
  internalFlag.createSpan({ text: ' Внутренний (не печатается)' });

  const breakFlag = flags.createEl('label');
  const breakBox = breakFlag.createEl('input', { attr: { type: 'checkbox' } });
  breakBox.checked = block.pageBreakBefore ?? false;
  breakBox.addEventListener('change', () => {
    block.pageBreakBefore = breakBox.checked;
    deps.onChange?.();
  });
  breakFlag.createSpan({ text: ' С новой страницы' });

  const keepFlag = flags.createEl('label');
  const keepBox = keepFlag.createEl('input', { attr: { type: 'checkbox' } });
  keepBox.checked = block.keepTogether ?? false;
  keepBox.addEventListener('change', () => {
    block.keepTogether = keepBox.checked;
    deps.onChange?.();
  });
  keepFlag.createSpan({ text: ' Не разрывать между страницами' });
}

/** Отступы блока в пунктах — «расположение на странице». */
function renderBlockPlacement(card: HTMLElement, block: Block, deps: BlockEditorDeps): void {
  const row = card.createDiv({ cls: 'tn-offers-row' });
  const field = (label: string, value: number, set: (v: number) => void): void => {
    row.createSpan({ cls: 'tn-offers-muted', text: label });
    const input = row.createEl('input', {
      cls: 'tn-offers-input tn-offers-num-input',
      attr: { type: 'number', min: '0', max: '200', step: '6' },
    });
    input.value = String(value);
    input.addEventListener('input', () => {
      const v = Math.max(0, Math.min(200, Number(input.value) || 0));
      set(v);
      deps.onChange?.();
    });
  };
  field('Отступ до, пт:', block.spacingBefore ?? 0, (v) => {
    block.spacingBefore = v;
  });
  field('Отступ после, пт:', block.spacingAfter ?? 0, (v) => {
    block.spacingAfter = v;
  });
}
