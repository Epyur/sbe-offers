import { Modal, Notice } from 'obsidian';
import type { OffersView } from './offers-view';
import type { Block, Placeholder, RichNode } from '../types/offers';
import { errorMessage } from '../../../sbe-core/src/utils/errors';

/**
 * «Опиши блок словами» (2026-11-05).
 *
 * Жалоба пользователя: «сейчас настройка блоков крайне не дружелюбная». Здесь
 * администратор пишет, ЧТО должно быть в блоке («заголовок, таблица с составом
 * и два абзаца о преимуществах»), а модель предлагает готовые узлы формы.
 *
 * Чего этот диалог НЕ делает: не сохраняет и не подменяет блок молча. Человек
 * видит предложение, решает сам и дальше дооформляет его в обычном редакторе —
 * правка остаётся за ним.
 */
export class DescribeBlockModal extends Modal {
  private description = '';
  private busy = false;
  private result: { title: string; nodes: RichNode[]; notes: string[] } | null = null;

  constructor(
    private readonly view: OffersView,
    private readonly placeholders: Placeholder[],
    private readonly onApply: (title: string, nodes: RichNode[]) => void,
  ) {
    super(view.app);
  }

  onOpen(): void {
    const { contentEl } = this;
    contentEl.empty();
    contentEl.createEl('h2', { text: 'Опишите блок словами' });
    contentEl.createEl('p', {
      cls: 'tn-offers-muted',
      text: 'Напишите, что должно быть в блоке и в каком порядке. Модель предложит ' +
        'готовые поля, а вы потом дооформите их в редакторе. Например: ' +
        '«заголовок «Условия поставки» и список из трёх пунктов об оплате, доставке и гарантии».',
    });

    const area = contentEl.createEl('textarea', { cls: 'tn-offers-input tn-offers-describe-input' });
    area.rows = 6;
    area.placeholder = 'Что должно быть в этом блоке…';
    area.value = this.description;
    area.addEventListener('input', () => {
      this.description = area.value;
    });

    const actions = contentEl.createDiv({ cls: 'tn-offers-actions' });
    const suggest = actions.createEl('button', { text: '✨ Предложить блок', cls: 'tn-btn tn-btn-primary' });
    const cancel = actions.createEl('button', { text: 'Отмена', cls: 'tn-btn tn-btn-ghost' });

    const resultBox = contentEl.createDiv({ cls: 'tn-offers-card tn-offers-describe-result' });

    suggest.addEventListener('click', () => {
      void (async () => {
        if (this.busy) return;
        if (!this.description.trim()) {
          new Notice('Опишите, что должно быть в блоке');
          return;
        }
        this.busy = true;
        suggest.setText('⏳ Думаю…');
        suggest.setAttribute('disabled', 'true');
        try {
          const res = await this.view.plugin.offersService.describeBlock(
            this.description,
            this.placeholders.map((p) => p.id),
          );
          this.result = { title: res.title, nodes: res.nodes, notes: res.notes ?? [] };
          drawResult(resultBox, this.result);
        } catch (e: unknown) {
          new Notice(`Не получилось: ${errorMessage(e)}`, 8000);
        } finally {
          this.busy = false;
          suggest.setText('✨ Предложить блок');
          suggest.removeAttribute('disabled');
        }
      })();
    });

    const applyRow = actions.createDiv({ cls: 'tn-offers-row' });
    const apply = applyRow.createEl('button', { text: '➕ Добавить блок в форму', cls: 'tn-btn tn-btn-primary' });
    apply.addEventListener('click', () => {
      if (!this.result) {
        new Notice('Сначала предложите блок');
        return;
      }
      this.onApply(this.result.title, this.result.nodes);
      this.close();
    });

    cancel.addEventListener('click', () => this.close());
  }

  onClose(): void {
    this.contentEl.empty();
  }
}

/** Что предложила модель: заголовок, узлы и замечания. */
function drawResult(box: HTMLElement, result: { title: string; nodes: RichNode[]; notes: string[] }): void {
  box.empty();
  box.createDiv({ cls: 'tn-offers-muted', text: 'Предложение модели (вы сможете его поправить):' });
  box.createEl('h3', { text: result.title });
  const list = box.createEl('ul', { cls: 'tn-offers-preview-list' });
  for (const node of result.nodes) {
    list.createEl('li', { text: describeNode(node) });
  }
  if (result.notes.length > 0) {
    const warn = box.createDiv({ cls: 'tn-offers-warn' });
    warn.createDiv({ text: 'На что обратить внимание:' });
    const ul = warn.createEl('ul', { cls: 'tn-offers-problems' });
    for (const note of result.notes) ul.createEl('li', { text: note });
  }
}

/** Человеческое описание узла для предварительного списка. */
function describeNode(node: RichNode): string {
  switch (node.type) {
    case 'heading':
      return `Заголовок ${node.level ?? 2}: ${node.text ?? ''}`;
    case 'bullet_list':
      return `Список: ${(node.items ?? []).join('; ')}`;
    case 'table':
      return `Таблица ${node.rows ? node.rows.length : 0} × ${node.rows && node.rows[0] ? node.rows[0].length : 0}`;
    case 'prompt_text':
      return `Текст модели: ${node.instruction ?? ''}`;
    case 'image':
      return node.imageOf === 'system'
        ? 'Картинка системы из КП'
        : node.imageOf === 'material'
          ? 'Картинка материала из КП'
          : 'Картинка (файл)';
    case 'links':
      return 'Ссылки из карточек';
    case 'page_break':
      return 'Разрыв страницы';
    case 'compare_table':
      return 'Таблица сравнения систем';
    default:
      return `Абзац: ${(node.text ?? '').slice(0, 80)}`;
  }
}

/** Применить предложение модели к блоку формы. */
export function applyDescribedBlock(
  blocks: Block[],
  id: string,
  title: string,
  nodes: RichNode[],
): void {
  blocks.push({
    id,
    title,
    content: nodes,
    showInClient: true,
    showInInternal: false,
  });
}