import type { Block, Placeholder, RichNode } from '../types/offers';

/**
 * Живой предпросмотр формы КП (2026-11-05).
 *
 * Зачем: редактор блоков показывал узлы списком полей, и администратор видел
 * «поле выбора типа» вместо «документа, который уйдёт клиенту». Оформление —
 * по ширине, красная строка, шрифт 14 пт, степени надстрочными — проверялось
 * только в готовом DOCX, то есть уже у менеджера.
 *
 * Предпросмотр повторяет правила печати (offers-service/docx.go):
 *   - основной текст по ширине, первая строка 1,25 см, шрифт 14 пт;
 *   - заголовки по центру, без красной строки, полужирные;
 *   - таблицы 10 пт, шапка по центру и с заливкой;
 *   - списки с маркерами и без отступа первой строки;
 *   - степени в единицах печатаются надстрочными (г/м², м²).
 *
 * Это не esbuild-копия сборки документа, а её читаемая модель: показать
 * человеку, что будет, а не повторить вёрстку Word до twips.
 */

export interface PreviewDeps {
  placeholders: Placeholder[];
  /** Имя/заголовок системы для `[Название системы]`. */
  systemTitle?: string;
  /** Значения формы для замены `{{поля}}`; без них — подписи полей в скобках. */
  values?: Record<string, string>;
}

/** Рисует предпросмотр документа в контейнер. */
export function renderFormPreview(container: HTMLElement, blocks: Block[], deps: PreviewDeps): void {
  container.empty();
  const page = container.createDiv({ cls: 'tn-offers-preview-page' });
  const shown = blocks.filter((b) => b.showInClient);
  if (shown.length === 0) {
    page.createDiv({ cls: 'tn-offers-muted', text: 'В форме нет блоков, которые увидит клиент.' });
    return;
  }
  for (const block of shown) {
    if (block.pageBreakBefore) {
      page.createDiv({ cls: 'tn-offers-preview-break', text: '── новая страница ──' });
    }
    const blockEl = page.createDiv({ cls: 'tn-offers-preview-block' });
    if (block.spacingBefore) blockEl.style.paddingTop = `${block.spacingBefore}px`;
    if (block.spacingAfter) blockEl.style.paddingBottom = `${block.spacingAfter}px`;
    for (const node of block.content) {
      renderPreviewNode(blockEl, node, deps);
    }
  }
}

/** Один узел так, как он напечатается. */
function renderPreviewNode(parent: HTMLElement, node: RichNode, deps: PreviewDeps): void {
  switch (node.type) {
    case 'page_break':
      parent.createDiv({ cls: 'tn-offers-preview-break', text: '── новая страница ──' });
      return;
    case 'heading': {
      const el = parent.createDiv({ cls: `tn-offers-preview-heading tn-offers-preview-h${node.level ?? 2}` });
      appendText(el, node.text ?? '', deps);
      return;
    }
    case 'bullet_list': {
      const list = parent.createEl('ul', { cls: 'tn-offers-preview-list' });
      for (const item of node.items ?? []) {
        const li = list.createEl('li');
        appendText(li, item, deps);
      }
      return;
    }
    case 'table': {
      const rows = node.rows ?? [];
      if (rows.length === 0) return;
      const table = parent.createEl('table', { cls: 'tn-offers-preview-table' });
      rows.forEach((cells, index) => {
        const tr = table.createEl('tr', { cls: index === 0 ? 'tn-offers-preview-thead' : '' });
        for (const cell of cells) {
          const td = tr.createEl(index === 0 ? 'th' : 'td');
          appendText(td, cell, deps);
        }
      });
      return;
    }
    case 'links':
      parent.createDiv({
        cls: 'tn-offers-preview-note',
        text: '🔗 Ссылки из карточек систем и материалов — печатаются отдельным разделом в конце.',
      });
      return;
    case 'prompt_text':
      parent.createDiv({
        cls: 'tn-offers-preview-note',
        text: `✨ Текст напишет модель по данным КП: «${node.instruction ?? ''}»` +
          (node.maxWords ? ` (до ${node.maxWords} слов)` : ''),
      });
      return;
    case 'image':
      renderPreviewImage(parent, node, deps);
      return;
    case 'compare_table':
      parent.createDiv({
        cls: 'tn-offers-preview-note',
        text: '📊 Таблица сравнения систем — печатается, если в КП выбрано две системы и больше.',
      });
      return;
    default: {
      const p = parent.createDiv({ cls: 'tn-offers-preview-para' });
      if (node.align) p.style.textAlign = node.align === 'justify' ? 'justify' : node.align;
      appendText(p, node.text ?? '', deps);
    }
  }
}

/** Картинка: рамка с подписью источника. */
function renderPreviewImage(parent: HTMLElement, node: RichNode, deps: PreviewDeps): void {
  const box = parent.createDiv({ cls: 'tn-offers-preview-image' });
  let label = '🖼 своя картинка';
  if (node.imageOf === 'system') {
    label = `🖼 картинка системы${node.systemIndex && node.systemIndex > 1 ? ` №${node.systemIndex}` : ''}` +
      (deps.systemTitle ? `: ${deps.systemTitle}` : ' из КП');
  } else if (node.imageOf === 'material') {
    label = `🖼 картинка материала из позиции №${node.itemIndex ?? 1}`;
  } else if (node.imageName) {
    label = `🖼 ${node.imageName}`;
  }
  box.createDiv({ cls: 'tn-offers-preview-image-name', text: label });
  box.createDiv({
    cls: 'tn-offers-muted',
    text: `Ширина ${node.widthCm ?? 8} см, обтекание: ${wrapLabel(node.wrap)}`,
  });
  if (node.caption) {
    box.createDiv({ cls: 'tn-offers-preview-caption', text: node.caption });
  }
}

/** Подпись для стороны обтекания — теми же словами, что в редакторе. */
function wrapLabel(wrap: RichNode['wrap']): string {
  switch (wrap) {
    case 'left':
      return 'картинка слева, текст справа';
    case 'right':
      return 'картинка справа, текст слева';
    case 'topAndBottom':
      return 'только сверху и снизу';
    case 'none':
      return 'в тексте (без обтекания)';
    default:
      return 'текст с обеих сторон';
  }
}

/**
 * Текст узла так, как его увидит клиент: поля подставлены, степени
 * надстрочные, вики-ссылки из заметок превращены в подпись.
 *
 * Без значений формы показываем подпись поля в угловых скобках — человек
 * видит, что здесь будет подставлено, и не путает с настоящим текстом.
 */
function appendText(target: HTMLElement, raw: string, deps: PreviewDeps): void {
  const text = plainText(raw);
  const re = /\{\{([a-zA-Z0-9_]+)\}\}|\[\[([^\[\]|]*)(?:\|([^\[\]]*))?\]\]/g;
  let last = 0;
  let match: RegExpExecArray | null;
  while ((match = re.exec(text)) !== null) {
    const m = match;
    if (m.index > last) appendSupsub(target, text.slice(last, m.index));
    if (m[1] !== undefined) {
      const value = deps.values?.[m[1]];
      const placeholder = deps.placeholders.find((p) => p.id === m[1]);
      if (value) {
        appendSupsub(target, value);
      } else {
        target.createSpan({ cls: 'tn-offers-preview-field', text: `〈${placeholder?.label ?? m[1]}〉` });
      }
    } else {
      appendSupsub(target, (m[3] || m[2] || '').trim());
    }
    last = match.index + match[0].length;
  }
  if (last < text.length) appendSupsub(target, text.slice(last));
}

/**
 * Степени из единиц измерения — надстрочными, как в документе.
 *
 * То же правило, что у сервера (docx_runs.go): «г/м2» и «м2» поднимают двойку,
 * «V-SL 2 мм» нет. Держать здесь копию — цена предпросмотра: он рисуется в
 * плагине, а серверный код в бандл не попадает.
 */
function appendSupsub(target: HTMLElement, text: string): void {
  if (text === '') return;
  const re = /(г\/м|кг\/м|т\/м|Н\/мм|мм|см|дм|м)([23])(?![0-9])/g;
  let last = 0;
  let match: RegExpExecArray | null;
  while ((match = re.exec(text)) !== null) {
    // Граница слева: «AL2» — марка, а не метры.
    const prev = match.index > 0 ? text[match.index - 1] : '';
    if (prev && /[А-Яа-яA-Za-z0-9]/.test(prev)) continue;
    const before = text.slice(last, match.index + match[1].length);
    if (before) target.createSpan({ text: before });
    const sup = target.createEl('sup', { text: match[2] });
    sup.addClass('tn-offers-preview-sup');
    last = match.index + match[0].length;
  }
  if (last < text.length) target.createSpan({ text: text.slice(last) });
}

/** Человеческий текст из разметки заметки — как на сервере (plainText). */
function plainText(raw: string): string {
  return raw
    .replace(/\*\*([^*]+)\*\*/g, '$1')
    .replace(/`/g, '');
}