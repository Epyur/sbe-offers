import { Notice } from 'obsidian';
import type { OffersView } from './offers-view';
import type { MatchResult, SheetApplyRow, SheetPreview } from '../types/offers';
import { errorMessage } from '../../../sbe-core/src/utils/errors';
import { formatMoney } from '../utils/money';

/**
 * Загрузка таблицы менеджера в двух шагах (2026-10-31).
 *
 * Первый шаг ничего не пишет: сервис разбирает файл и говорит, ЧТО ПОНЯЛ —
 * сколько строк нашлось по ЕКН, по названию, догадался ли модели и что не
 * сопоставлено. Менеджер проверяет и только потом жмёт «Применить».
 *
 * Разделение не формальность: файл бывает черновиком со старыми ценами, а
 * молчаливое применение роняет КП в глазах заказчика. Поэтому же строки,
 * сопоставленные моделью, по умолчанию НЕ отмечены: их добавляет человек.
 */

/** Рисует блок загрузки в контейнер и возвращает reload для перерисовки КП. */
export function renderSheetUpload(
  view: OffersView,
  container: HTMLElement,
  offerId: number,
  offerNumber: string,
  onApplied: () => void,
): void {
  const card = container.createDiv({ cls: 'tn-offers-card' });
  card.createDiv({ cls: 'tn-offers-muted', text: 'Таблица менеджера (.xlsx)' });
  const hint = card.createDiv({ cls: 'tn-offers-muted' });
  hint.setText(
    'Колонки ищутся по заголовку: Наименование (обязательно), ЕКН, Ед., Кол-во, ' +
      'Цена (обязательно), Сумма. Цена из таблицы — источник правды для этого КП.',
  );

  const hidden = card.createEl('input', {
    cls: 'tn-offers-hidden',
    attr: { type: 'file', accept: '.xlsx' },
  });
  const button = card.createEl('button', { text: '📄 Загрузить таблицу', cls: 'tn-btn' });
  button.addEventListener('click', () => hidden.click());

  const result = card.createDiv({ cls: 'tn-offers-sheet-result' });

  hidden.addEventListener('change', async () => {
    const files = hidden.files;
    if (!files || files.length === 0) return;
    // Файл забираем ДО сброса поля: input.files — живой список, и сброс ниже
    // опустошает его (грабля 2026-09-18 в «Письмах»).
    const file = files[0];
    hidden.value = '';
    if (!file.name.toLowerCase().endsWith('.xlsx')) {
      new Notice('Нужен файл .xlsx: старый .xls нужно пересохранить в Excel');
      return;
    }
    button.setAttr('disabled', 'true');
    button.setText('⏳ Разбираю…');
    try {
      const buffer = await file.arrayBuffer();
      const preview = await view.plugin.offersService.previewSheet(offerId, file.name, buffer);
      renderPreview(view, result, offerId, offerNumber, preview, onApplied);
    } catch (e: unknown) {
      result.empty();
      result.createDiv({ cls: 'tn-offers-error', text: `Не удалось разобрать таблицу: ${errorMessage(e)}` });
    } finally {
      button.removeAttribute('disabled');
      button.setText('📄 Загрузить таблицу');
    }
  });
}

function renderPreview(
  view: OffersView,
  container: HTMLElement,
  offerId: number,
  offerNumber: string,
  preview: SheetPreview,
  onApplied: () => void,
): void {
  container.empty();
  const counts = preview.counts;
  container.createDiv({
    cls: 'tn-offers-sheet-summary',
    text:
      `Файл «${preview.fileName || 'без имени'}»: строк ${counts.total}, ` +
      `по ЕКН ${counts.byEkn}, по названию ${counts.byName}, ` +
      `догадка модели ${counts.byAi}, не сопоставлено ${counts.none}.`,
  });
  if (preview.problems.length > 0) {
    const list = container.createEl('ul', { cls: 'tn-offers-warn tn-offers-problems' });
    for (const p of preview.problems) list.createEl('li', { text: p });
  }

  const table = container.createEl('table', { cls: 'tn-offers-table' });
  const head = table.createEl('thead').createEl('tr');
  for (const title of ['', 'Строка', 'Из таблицы', 'Материал', 'Кол-во', 'Цена', 'Сумма']) {
    head.createEl('th', { text: title });
  }
  const body = table.createEl('tbody');

  // Отметки: сопоставленное точно — отмечено, догадка модели — НЕТ.
  const chosen = new Set<number>();
  const rows = preview.rows.map((r) => ({ result: r, row: applyRow(r) }));
  rows.forEach((entry, index) => {
    const r = entry.result;
    const canApply = entry.row !== null;
    if (canApply && !r.needsConfirm) chosen.add(index);

    const tr = body.createEl('tr');
    const boxCell = tr.createEl('td');
    let box: HTMLInputElement | null = null;
    if (canApply) {
      box = boxCell.createEl('input', { attr: { type: 'checkbox' } });
      box.checked = chosen.has(index);
      box.addEventListener('change', () => {
        if (box?.checked) chosen.add(index);
        else chosen.delete(index);
      });
    }
    tr.createEl('td', { text: String(r.row.line) });
    tr.createEl('td', { text: r.row.name });
    const matCell = tr.createEl('td');
    if (r.materialId > 0) {
      matCell.createSpan({ text: r.name });
      matCell.createSpan({ cls: 'tn-offers-muted tn-offers-mono', text: ` ${r.sku}` });
      if (r.needsConfirm) matCell.createSpan({ cls: 'tn-offers-warn', text: ' (догадка модели)' });
      else if (r.method === 'ekn') matCell.createSpan({ cls: 'tn-offers-muted', text: ' (по ЕКН)' });
    } else {
      matCell.createSpan({ cls: 'tn-offers-warn', text: r.reason || 'не сопоставлено' });
    }
    tr.createEl('td', {
      cls: 'tn-offers-num',
      text: r.row.hasQty ? formatNumber(r.row.qty) : '1',
    });
    tr.createEl('td', {
      cls: 'tn-offers-num',
      text: r.row.hasPrice ? `${formatMoney(r.row.price)} ₽` : '—',
    });
    tr.createEl('td', {
      cls: 'tn-offers-num',
      text: r.row.hasSum ? `${formatMoney(r.row.sum)} ₽` : '—',
    });
  });

  const actions = container.createDiv({ cls: 'tn-offers-actions' });
  const addEkn = actions.createEl('label', { cls: 'tn-offers-check' });
  const addEknInput = addEkn.createEl('input', { attr: { type: 'checkbox' } });
  addEknInput.checked = true;
  addEkn.createSpan({ text: ' Дописывать новые ЕКН в справочник' });

  const apply = actions.createEl('button', { text: '✅ Применить', cls: 'tn-btn tn-btn-primary' });
  const cancel = actions.createEl('button', { text: 'Отменить', cls: 'tn-btn tn-btn-ghost' });
  cancel.addEventListener('click', () => {
    container.empty();
  });

  apply.addEventListener('click', async () => {
    const picked: SheetApplyRow[] = [];
    chosen.forEach((index) => {
      const row = rows[index]?.row;
      if (row) picked.push(row);
    });
    if (picked.length === 0) {
      new Notice('Не выбрано ни одной строки');
      return;
    }
    apply.setAttr('disabled', 'true');
    apply.setText('⏳ Применяю…');
    try {
      const res = await view.plugin.offersService.applySheet(offerId, picked, addEknInput.checked);
      new Notice(
        `Позиции обновлены по таблице: ${res.applied}. Итого ${formatMoney(res.offer.total)} ₽ — ` +
          `предложение ${offerNumber || res.offer.number}.`,
      );
      onApplied();
    } catch (e: unknown) {
      new Notice(`Не удалось применить таблицу: ${errorMessage(e)}`);
      apply.removeAttribute('disabled');
      apply.setText('✅ Применить');
    }
  });
}

/**
 * Строка к отправке. Берём то, что человек видел в превью; количество по
 * умолчанию — 1, цена — из таблицы.
 */
function applyRow(r: MatchResult): SheetApplyRow | null {
  if (r.materialId <= 0) return null;
  return {
    line: r.row.line,
    materialId: r.materialId,
    ekn: r.row.ekn ?? '',
    variant: r.row.name,
    qty: r.row.hasQty && r.row.qty > 0 ? r.row.qty : 1,
    price: r.row.hasPrice ? r.row.price : 0,
  };
}

function formatNumber(value: number): string {
  return new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 3 }).format(value);
}