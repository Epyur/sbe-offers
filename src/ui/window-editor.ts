import type { Placeholder, WindowItem } from '../types/offers';
import {
  ZONE_TITLES,
  appendNewFields,
  isKnownZone,
  moveWindowItem,
  windowPreview,
  windowTitle,
} from './window-model';

/**
 * Редактор окна КП в приложении (2026-11-15, шаг 4 конфигуратора окна).
 *
 * Просьба пользователя: «в плагине добавить конфигуратор окна КП в приложении,
 * чтобы администратор в плагине мог настраивать порядок отображения полей,
 * обязательные и добровольные поля». Настраивается не только форма, но и ЗОНЫ
 * экрана приложения: заказчик, системы, позиции, материалы, тексты разделов.
 *
 * Правила (все — решение пользователя, не догадка):
 *   - пустое окно = «как сейчас»: старые формы ничего не меняют;
 *   - скрытое не может быть обязательным: пока элемент скрыт, он не обязателен;
 *   - обязательность МЯГКАЯ: она подсвечивает, а не блокирует сохранение КП;
 *   - новое поле формы дописывается в конец видимым, убранное из формы
 *     исчезает и из окна (иначе сохранение формы упадёт на ссылке в пустоту).
 *
 * Перерисовка списка — ТОЛЬКО по кнопкам ▲/▼/✖. Обработчик `input` в
 * текстовом поле ничего не пересобирает: в 0.10.4 именно `redraw()` внутри
 * `input` уничтожал поле ввода на первой же букве. Здесь текстовых полей у
 * строки нет вовсе — подпись поля только читается.
 */
export const WINDOW_EDITOR_TEXTS = {
  groupTitle: 'Окно КП в приложении',
  hint:
    'Порядок, видимость и обязательность разделов экрана менеджера и полей формы. ' +
    'Скрытое не может быть обязательным. Обязательное только подсвечивается — сохранить КП можно и без него.',
  zonesTitle: 'Разделы экрана',
  fieldsTitle: 'Поля формы',
  moveUp: '▲',
  moveDown: '▼',
  removeField: '✖',
  removeFieldAria: 'Убрать поле из окна',
  visible: 'виден',
  required: 'обязателен',
  hiddenHint: 'скрыт — обязательным быть не может',
  requiredZoneOnly: 'обязательным бывает только раздел «Заказчик»',
  unknownZone: 'этот раздел не нарисован в приложении: уберите его из окна',
  previewTitle: 'Что увидит менеджер',
  previewEmpty: 'Ничего: все разделы скрыты.',
  emptyFields: 'Полей формы нет — заведите их в группе «Поля формы».',
  addAllFields: '↩ Вернуть все поля формы',
};

export interface WindowEditorDeps {
  /**
   * Хук на каждую правку: редактор формы ставит сюда перерисовку предпросмотра
   * окна. Вызывается и на галочках, и на перестановке — предпросмотр обязан
   * показывать то, что уйдёт на сервер.
   */
  onChange?: () => void;
}

export interface WindowEditorHandle {
  /** Текущее окно — то, что редактор формы заберёт при сохранении. */
  items(): WindowItem[];
  /** Свести окно с формой: убрать убранные поля, дописать новые в конец. */
  syncWithForm(placeholders: Placeholder[]): void;
  /** Перерисовать (после правки списка полей формы). */
  refresh(): void;
}

export function renderWindowEditor(
  container: HTMLElement,
  items: WindowItem[],
  placeholders: Placeholder[],
  deps: WindowEditorDeps = {},
): WindowEditorHandle {
  container.empty();
  container.createDiv({ cls: 'tn-offers-muted tn-offers-window-hint', text: WINDOW_EDITOR_TEXTS.hint });

  const list = container.createDiv({ cls: 'tn-offers-window-list' });
  const preview = container.createDiv({ cls: 'tn-offers-window-preview' });

  const redraw = (): void => {
    list.empty();
    if (items.length === 0) {
      list.createDiv({ cls: 'tn-offers-muted', text: WINDOW_EDITOR_TEXTS.emptyFields });
    }
    // Разделы экрана и поля формы — двумя подписанными блоками (2026-11-16).
    // Подписи были объявлены с 0.11.0, но не рисовались: список шёл сплошным,
    // и при 12 строках новая зона в конце сливалась с полями — отсюда живая
    // жалоба «не вижу зоны бриф». Заголовок остаётся и у пустой половины:
    // человек должен видеть, где заканчиваются разделы.
    const zones = items.filter((item) => item.kind === 'zone');
    const fields = items.filter((item) => item.kind === 'field');
    if (zones.length > 0) {
      list.createDiv({ cls: 'tn-offers-window-group', text: WINDOW_EDITOR_TEXTS.zonesTitle });
      zones.forEach((item) => {
        const index = items.indexOf(item);
        renderItemCard(list, items, item, index, placeholders, redraw, deps);
      });
    }
    list.createDiv({ cls: 'tn-offers-window-group', text: WINDOW_EDITOR_TEXTS.fieldsTitle });
    fields.forEach((item) => {
      const index = items.indexOf(item);
      renderItemCard(list, items, item, index, placeholders, redraw, deps);
    });
    redrawPreview();
  };

  const redrawPreview = (): void => {
    preview.empty();
    preview.createDiv({ cls: 'tn-offers-window-preview-title', text: WINDOW_EDITOR_TEXTS.previewTitle });
    const lines = windowPreview(items, placeholders);
    if (lines.length === 0) {
      preview.createDiv({ cls: 'tn-offers-muted', text: WINDOW_EDITOR_TEXTS.previewEmpty });
      return;
    }
    const ol = preview.createEl('ol', { cls: 'tn-offers-window-preview-list' });
    for (const line of lines) ol.createEl('li', { text: line });
  };

  const allFieldsPresent = (): boolean => {
    const inWindow = new Set(items.filter((item) => item.kind === 'field').map((item) => item.id));
    return placeholders.every((p) => inWindow.has(p.id));
  };

  const restore = container.createEl('button', { text: WINDOW_EDITOR_TEXTS.addAllFields, cls: 'tn-btn tn-btn-ghost' });
  restore.addEventListener('click', () => {
    // Кнопка полезна только когда что-то убрано; показываем это состоянием, а
    // не второй подписью — иначе две причины «нажать» на одном экране.
    const next = appendNewFields(items, placeholders);
    items.splice(0, items.length, ...next);
    redraw();
    restore.hidden = allFieldsPresent();
    deps.onChange?.();
  });
  restore.hidden = allFieldsPresent();

  redraw();

  return {
    items: () => items,
    syncWithForm: (next: Placeholder[]) => {
      // Поле убрали из формы — убираем и из окна, иначе сохранение упадёт на
      // ссылке в пустоту. Новое поле дописываем видимым в конец.
      const kept = items.filter((item) => item.kind !== 'field' || next.some((p) => p.id === item.id));
      items.splice(0, items.length, ...appendNewFields(kept, next));
      redraw();
      restore.hidden = allFieldsPresent();
    },
    refresh: () => {
      redraw();
      restore.hidden = allFieldsPresent();
    },
  };
}

function renderItemCard(
  list: HTMLElement,
  items: WindowItem[],
  item: WindowItem,
  index: number,
  placeholders: Placeholder[],
  redraw: () => void,
  deps: WindowEditorDeps,
): void {
  const isZone = item.kind === 'zone';
  const zoneKnown = isZone && isKnownZone(item.id);
  const classes = ['tn-offers-card', 'tn-offers-window-item'];
  if (isZone) classes.push('tn-offers-window-item-zone');
  if (item.visible === false) classes.push('is-hidden');
  const card = list.createDiv({ cls: classes.join(' ') });

  const head = card.createDiv({ cls: 'tn-offers-row-between' });
  const title = head.createDiv({ cls: 'tn-offers-window-title' });
  title.createSpan({ cls: isZone ? 'tn-offers-window-zone' : 'tn-offers-window-field', text: windowTitle(item, placeholders) });
  const zoneLabel = isZone ? (zoneKnown ? null : WINDOW_EDITOR_TEXTS.unknownZone) : null;
  if (zoneLabel) title.createSpan({ cls: 'tn-offers-muted', text: ` ${zoneLabel}` });

  const tools = head.createDiv({ cls: 'tn-offers-row' });
  const up = tools.createEl('button', {
    text: WINDOW_EDITOR_TEXTS.moveUp,
    cls: 'tn-btn tn-btn-ghost',
    attr: { 'aria-label': 'Выше' },
  });
  up.disabled = index === 0;
  up.addEventListener('click', () => {
    if (!moveWindowItem(items, index, -1)) return;
    redraw();
    deps.onChange?.();
  });

  const down = tools.createEl('button', {
    text: WINDOW_EDITOR_TEXTS.moveDown,
    cls: 'tn-btn tn-btn-ghost',
    attr: { 'aria-label': 'Ниже' },
  });
  down.disabled = index === items.length - 1;
  down.addEventListener('click', () => {
    if (!moveWindowItem(items, index, 1)) return;
    redraw();
    deps.onChange?.();
  });

  if (!isZone) {
    const del = tools.createEl('button', {
      text: WINDOW_EDITOR_TEXTS.removeField,
      cls: 'tn-btn tn-btn-ghost',
      attr: { 'aria-label': WINDOW_EDITOR_TEXTS.removeFieldAria },
    });
    del.addEventListener('click', () => {
      items.splice(index, 1);
      redraw();
      deps.onChange?.();
    });
  }

  const checks = card.createDiv({ cls: 'tn-offers-row tn-offers-window-checks' });

  const visibleBox = checks.createEl('label', { cls: 'tn-offers-check' });
  const visibleInput = visibleBox.createEl('input', { attr: { type: 'checkbox' } });
  visibleInput.checked = item.visible !== false;
  visibleBox.createSpan({ text: ` ${WINDOW_EDITOR_TEXTS.visible}` });
  visibleInput.addEventListener('change', () => {
    item.visible = visibleInput.checked;
    // Скрытое необязательно (решение пользователя): галочку снимаем сразу,
    // чтобы человек видел последствие, а не получал отказ сервера.
    if (!visibleInput.checked && item.required === true) item.required = false;
    redraw();
    deps.onChange?.();
  });

  const requiredAllowed = !isZone || item.id === 'client';
  const requiredBox = checks.createEl('label', { cls: 'tn-offers-check' });
  const requiredInput = requiredBox.createEl('input', { attr: { type: 'checkbox' } });
  requiredInput.checked = item.required === true;
  requiredInput.disabled = !requiredAllowed || item.visible === false;
  requiredBox.createSpan({ text: ` ${WINDOW_EDITOR_TEXTS.required}` });
  requiredInput.addEventListener('change', () => {
    item.required = requiredInput.checked;
    redraw();
    deps.onChange?.();
  });

  if (isZone && !requiredAllowed) {
    card.createDiv({ cls: 'tn-offers-muted', text: WINDOW_EDITOR_TEXTS.requiredZoneOnly });
  }
  if (item.visible === false) {
    card.createDiv({ cls: 'tn-offers-muted', text: WINDOW_EDITOR_TEXTS.hiddenHint });
  }
}

/** Подписи зон — для справки в самой группе (что вообще можно настроить). */
export function knownZoneTitles(): string[] {
  return Object.values(ZONE_TITLES);
}