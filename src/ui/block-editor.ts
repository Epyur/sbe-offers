import type { Block, Placeholder, RichNode } from '../types/offers';

/**
 * Блок-редактор формы КП — по образцу конфигуратора протоколов в ЛИМС
 * (sbe-lims/src/ui/block-editor.ts): блоки документа с чипами-плейсхолдерами,
 * которые подставляет сервер при печати.
 *
 * Модель узлов нарочно простая (текст+список+таблица), и хранится она как
 * RichNode[] — сервер разбирает её тем же описанием (offers-service/docx.go).
 */

export interface BlockEditorDeps {
  placeholders: Placeholder[];
  onChange?: () => void;
}

function uid(prefix: string): string {
  return `${prefix}_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
}

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

function nodeRow(container: HTMLElement, nodes: RichNode[], node: RichNode, index: number, deps: BlockEditorDeps, redraw: () => void): void {
  const row = container.createDiv({ cls: 'tn-offers-row tn-offers-block-node' });
  const kind = row.createEl('select', { cls: 'tn-offers-input tn-offers-node-kind' });
  for (const [value, label] of [
    ['paragraph', 'Абзац'],
    ['heading', 'Заголовок'],
    ['bullet_list', 'Список'],
    ['table', 'Таблица'],
  ]) {
    kind.createEl('option', { value, text: label });
  }
  kind.value = node.type;
  kind.addEventListener('change', () => {
    node.type = kind.value as RichNode['type'];
    onNodeKindChanged(node);
    redraw();
  });

  const body = row.createDiv({ cls: 'tn-offers-grow' });
  if (node.type === 'bullet_list') {
    const area = body.createEl('textarea', { cls: 'tn-offers-input' });
    area.rows = 3;
    area.value = (node.items ?? []).join('\n');
    area.addEventListener('input', () => {
      node.items = area.value.split('\n').map((line) => line.trimEnd()).filter((line) => line !== '');
      deps.onChange?.();
    });
  } else if (node.type === 'table') {
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
  } else {
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

function onNodeKindChanged(node: RichNode): void {
  if (node.type === 'bullet_list') {
    node.items = node.items ?? [node.text ?? ''];
    node.text = undefined;
  } else if (node.type === 'table') {
    node.rows = node.rows ?? [
      ['Колонка 1', 'Колонка 2'],
      ['', ''],
    ];
  } else {
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

      const nodesWrap = card.createDiv();
      block.content.forEach((node, nodeIndex) => {
        nodeRow(nodesWrap, block.content, node, nodeIndex, deps, () => {
          deps.onChange?.();
          redraw();
        });
      });

      const addRow = card.createDiv({ cls: 'tn-offers-row' });
      for (const [type, label] of [
        ['paragraph', '➕ Абзац'],
        ['heading', '➕ Заголовок'],
        ['bullet_list', '➕ Список'],
        ['table', '➕ Таблица'],
      ] as Array<[RichNode['type'], string]>) {
        const btn = addRow.createEl('button', { text: label, cls: 'tn-btn tn-btn-ghost' });
        btn.addEventListener('click', () => {
          const node: RichNode = { type };
          onNodeKindChanged(node);
          block.content.push(node);
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