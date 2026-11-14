import { Notice } from 'obsidian';
import type { ContentSection, ContentSubsection, Offer, SubsectionCondition } from '../types/offers';
import { CONDITION_ORDER, conditionHoldsClient, conditionWords, sectionMarker } from './sections-model';
import { errorMessage } from '../../../sbe-core/src/utils/errors';

/**
 * Редактор содержательных разделов КП (2026-11-14, шаг 6).
 *
 * Раздел — не вёрстка, а БРИФ для модели: какие подразделы писать, при каком
 * условии и о чём. Оформление (шрифты, заголовки, номера, таблицы, подписи)
 * берётся из макета Word, поэтому настроек оформления здесь нет — только
 * правила и условия (дизайн §4).
 *
 * Предпросмотр рамки модель НЕ зовёт: он показывает форму (заголовок раздела
 * и серые боксы подразделов, которые попадут), а не текст. Это тот же принцип,
 * что у `form-preview.ts`: увидеть последствие правки до выгрузки.
 */

/** Что умеет редактор разделов снаружи. */
export interface SectionsEditorDeps {
  /** Что выбрано в редактируемом предложении — для предпросмотра условий. */
  offer?: Offer;
  /** Правка: перерисовать предпросмотр и вывод меток. */
  onChange?: () => void;
}

/** Тексты редактора — одна точка, чтобы проверка бандла нашла их все. */
export const SECTIONS_EDITOR_TEXTS = {
  cardTitle: 'Содержательные разделы (ИИ по инструкциям, оформление — из макета)',
  emptyHint:
    'Содержательных разделов нет. Раздел — это бриф для модели: подразделы, условия и инструкции. Оформление берётся из макета Word.',
  addSection: '➕ Раздел',
  removeSection: '✖ Удалить раздел',
  sectionID: 'id метки (латиницей)',
  sectionTitle: 'Заголовок раздела в документе',
  numbered: 'Нумеровать разделы',
  critique: 'Раздел (условие has_critique): критика альтернатив',
  subsections: 'Подразделы',
  addSubsection: '➕ Подраздел',
  removeSubsection: '✖',
  subsectionTitle: 'Заголовок подраздела',
  instruction: 'Что написать (инструкция модели)',
  condition: 'Условие',
  repeat: 'Повтор',
  maxWords: 'Предел слов',
  previewTitle: 'Предпросмотр рамки (модель не вызывается)',
  previewSkip: 'не попадёт — условие не выполнено',
  copyMarker: 'Скопировать метку { content: … }',
  addedSection: 'Раздел добавлен — настройте подразделы и сохраните форму',
  copied: 'Метка скопирована',
  copyFailed: 'Не удалось скопировать',
} as const;

/** Подписи условий — из `sections-model.ts`: одно место с серверной таблицей. */
export { conditionHoldsClient, conditionWords, sectionMarker };

/** Рисует редактор разделов прямо в контейнер. */
export function renderSectionsEditor(
  container: HTMLElement,
  sections: ContentSection[],
  deps: SectionsEditorDeps,
): void {
  container.empty();

  const list = container.createDiv({ cls: 'tn-offers-sections-list' });

  const redraw = (): void => {
    list.empty();
    if (sections.length === 0) {
      list.createDiv({ cls: 'tn-offers-muted', text: SECTIONS_EDITOR_TEXTS.emptyHint });
      return;
    }
    sections.forEach((section, index) => {
      renderSectionCard(list, sections, section, index, deps, redraw);
    });
  };

  const addBtn = container.createEl('button', { text: SECTIONS_EDITOR_TEXTS.addSection, cls: 'tn-btn' });
  addBtn.addEventListener('click', () => {
    sections.push({
      id: `section_${Date.now().toString(36)}`,
      title: 'Новый раздел',
      numbered: true,
      subsections: [
        { id: `sub_${Date.now().toString(36)}`, title: 'Введение', instruction: '', condition: 'always' },
      ],
    });
    new Notice(SECTIONS_EDITOR_TEXTS.addedSection);
    deps.onChange?.();
    redraw();
  });

  redraw();
}

/** Одна карточка раздела: заголовок, переключатели, подразделы, предпросмотр. */
function renderSectionCard(
  list: HTMLElement,
  sections: ContentSection[],
  section: ContentSection,
  index: number,
  deps: SectionsEditorDeps,
  redraw: () => void,
): void {
  const card = list.createDiv({ cls: 'tn-offers-card tn-offers-section-card' });
  const head = card.createDiv({ cls: 'tn-offers-row-between' });

  const titleInput = head.createEl('input', { cls: 'tn-offers-input tn-offers-grow' });
  titleInput.value = section.title;
  titleInput.placeholder = SECTIONS_EDITOR_TEXTS.sectionTitle;
  titleInput.addEventListener('input', () => {
    section.title = titleInput.value;
    deps.onChange?.();
    redraw();
  });

  copyChip(head, section);

  const upBtn = head.createEl('button', { text: '▲', cls: 'tn-btn tn-btn-ghost' });
  upBtn.disabled = index === 0;
  upBtn.addEventListener('click', () => {
    [sections[index - 1], sections[index]] = [sections[index], sections[index - 1]];
    deps.onChange?.();
    redraw();
  });
  const downBtn = head.createEl('button', { text: '▼', cls: 'tn-btn tn-btn-ghost' });
  downBtn.disabled = index === sections.length - 1;
  downBtn.addEventListener('click', () => {
    [sections[index], sections[index + 1]] = [sections[index + 1], sections[index]];
    deps.onChange?.();
    redraw();
  });
  const delBtn = head.createEl('button', { text: SECTIONS_EDITOR_TEXTS.removeSection, cls: 'tn-btn tn-btn-ghost' });
  delBtn.addEventListener('click', () => {
    sections.splice(index, 1);
    deps.onChange?.();
    redraw();
  });

  renderIdentityRow(card, section, deps);
  renderSectionFlags(card, section, deps, redraw);
  renderSubsections(card, section, deps, redraw);
  renderFramePreview(card, section, deps);
}

/** id метки и переключатели раздела. */
function renderIdentityRow(card: HTMLElement, section: ContentSection, deps: SectionsEditorDeps): void {
  const row = card.createDiv({ cls: 'tn-offers-row' });
  row.createSpan({ cls: 'tn-offers-muted', text: 'id:' });
  const idInput = row.createEl('input', { cls: 'tn-offers-input' });
  idInput.value = section.id;
  idInput.placeholder = SECTIONS_EDITOR_TEXTS.sectionID;
  idInput.addEventListener('change', () => {
    section.id = idInput.value.trim().replace(/[^a-zA-Z0-9_]/g, '_') || section.id;
    idInput.value = section.id;
    deps.onChange?.();
  });
}

/** Флаги раздела: нумерация и «критика». */
function renderSectionFlags(
  card: HTMLElement,
  section: ContentSection,
  deps: SectionsEditorDeps,
  redraw: () => void,
): void {
  const flags = card.createDiv({ cls: 'tn-offers-row' });
  const numberedFlag = flags.createEl('label');
  const numberedBox = numberedFlag.createEl('input', { attr: { type: 'checkbox' } });
  numberedBox.checked = section.numbered ?? false;
  numberedBox.addEventListener('change', () => {
    section.numbered = numberedBox.checked;
    deps.onChange?.();
  });
  numberedFlag.createSpan({ text: ` ${SECTIONS_EDITOR_TEXTS.numbered}` });

  const critiqueFlag = flags.createEl('label');
  const critiqueBox = critiqueFlag.createEl('input', { attr: { type: 'checkbox' } });
  critiqueBox.checked = section.critique ?? false;
  critiqueBox.addEventListener('change', () => {
    section.critique = critiqueBox.checked;
    deps.onChange?.();
    redraw();
  });
  critiqueFlag.createSpan({ text: ` ${SECTIONS_EDITOR_TEXTS.critique}` });
}

/** Таблица подразделов раздела. */
function renderSubsections(
  card: HTMLElement,
  section: ContentSection,
  deps: SectionsEditorDeps,
  redraw: () => void,
): void {
  const block = card.createDiv({ cls: 'tn-offers-subsections' });
  block.createDiv({ cls: 'tn-offers-muted', text: SECTIONS_EDITOR_TEXTS.subsections });

  section.subsections.forEach((sub, subIndex) => {
    renderSubsectionRow(block, section, sub, subIndex, deps, redraw);
  });

  const addSub = block.createEl('button', { text: SECTIONS_EDITOR_TEXTS.addSubsection, cls: 'tn-btn tn-btn-ghost' });
  addSub.addEventListener('click', () => {
    section.subsections.push({
      id: `sub_${Date.now().toString(36)}`,
      title: 'Новый подраздел',
      instruction: '',
      condition: 'always',
    });
    deps.onChange?.();
    redraw();
  });
}

/** Одна строка подраздела: заголовок, инструкция, условие, повтор, предел. */
function renderSubsectionRow(
  parent: HTMLElement,
  section: ContentSection,
  sub: ContentSubsection,
  subIndex: number,
  deps: SectionsEditorDeps,
  redraw: () => void,
): void {
  const row = parent.createDiv({ cls: 'tn-offers-card tn-offers-subsection-card' });
  const head = row.createDiv({ cls: 'tn-offers-row-between' });

  const titleInput = head.createEl('input', { cls: 'tn-offers-input tn-offers-grow' });
  titleInput.value = sub.title;
  titleInput.placeholder = SECTIONS_EDITOR_TEXTS.subsectionTitle;
  titleInput.addEventListener('input', () => {
    sub.title = titleInput.value;
    deps.onChange?.();
  });

  const idInput = head.createEl('input', { cls: 'tn-offers-input' });
  idInput.value = sub.id;
  idInput.placeholder = 'id';
  idInput.addEventListener('change', () => {
    sub.id = idInput.value.trim().replace(/[^a-zA-Z0-9_]/g, '_') || sub.id;
    idInput.value = sub.id;
    deps.onChange?.();
  });

  const upBtn = head.createEl('button', { text: '▲', cls: 'tn-btn tn-btn-ghost' });
  upBtn.disabled = subIndex === 0;
  upBtn.addEventListener('click', () => {
    [section.subsections[subIndex - 1], section.subsections[subIndex]] = [
      section.subsections[subIndex],
      section.subsections[subIndex - 1],
    ];
    deps.onChange?.();
    redraw();
  });
  const downBtn = head.createEl('button', { text: '▼', cls: 'tn-btn tn-btn-ghost' });
  downBtn.disabled = subIndex === section.subsections.length - 1;
  downBtn.addEventListener('click', () => {
    [section.subsections[subIndex], section.subsections[subIndex + 1]] = [
      section.subsections[subIndex + 1],
      section.subsections[subIndex],
    ];
    deps.onChange?.();
    redraw();
  });
  const delBtn = head.createEl('button', { text: SECTIONS_EDITOR_TEXTS.removeSubsection, cls: 'tn-btn tn-btn-ghost' });
  delBtn.addEventListener('click', () => {
    section.subsections.splice(subIndex, 1);
    deps.onChange?.();
    redraw();
  });

  const instruction = row.createEl('textarea', { cls: 'tn-offers-input tn-offers-instruction' });
  instruction.value = sub.instruction;
  instruction.placeholder = SECTIONS_EDITOR_TEXTS.instruction;
  instruction.addEventListener('input', () => {
    sub.instruction = instruction.value;
    deps.onChange?.();
  });

  renderSubsectionOptions(row, sub, deps);
}

/** Условие, повтор и предел слов подраздела. */
function renderSubsectionOptions(
  row: HTMLElement,
  sub: ContentSubsection,
  deps: SectionsEditorDeps,
): void {
  const opts = row.createDiv({ cls: 'tn-offers-row' });

  opts.createSpan({ cls: 'tn-offers-muted', text: `${SECTIONS_EDITOR_TEXTS.condition}:` });
  const condSel = opts.createEl('select', { cls: 'tn-offers-input' });
  for (const cond of CONDITION_ORDER) {
    condSel.createEl('option', { text: conditionWords(cond), value: cond });
  }
  condSel.value = sub.condition ?? 'always';
  condSel.addEventListener('change', () => {
    sub.condition = condSel.value as SubsectionCondition;
    deps.onChange?.();
  });

  opts.createSpan({ cls: 'tn-offers-muted', text: `${SECTIONS_EDITOR_TEXTS.repeat}:` });
  const repeatSel = opts.createEl('select', { cls: 'tn-offers-input' });
  repeatSel.createEl('option', { text: 'один раз', value: '' });
  repeatSel.createEl('option', { text: 'по каждой системе', value: 'systems' });
  repeatSel.value = sub.repeat ?? '';
  repeatSel.addEventListener('change', () => {
    sub.repeat = repeatSel.value === 'systems' ? 'systems' : '';
    deps.onChange?.();
  });

  opts.createSpan({ cls: 'tn-offers-muted', text: `${SECTIONS_EDITOR_TEXTS.maxWords}:` });
  const wordsInput = opts.createEl('input', { cls: 'tn-offers-input tn-offers-narrow', attr: { type: 'number' } });
  wordsInput.value = String(sub.maxWords ?? 0);
  wordsInput.addEventListener('change', () => {
    const parsed = Number.parseInt(wordsInput.value, 10);
    sub.maxWords = Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
  });
}

/** Предпросмотр рамки: заголовок и серые боксы подразделов «попадут / нет». */
function renderFramePreview(card: HTMLElement, section: ContentSection, deps: SectionsEditorDeps): void {
  const preview = card.createDiv({ cls: 'tn-offers-section-preview' });
  preview.createDiv({ cls: 'tn-offers-muted', text: SECTIONS_EDITOR_TEXTS.previewTitle });

  const frame = preview.createDiv({ cls: 'tn-offers-frame' });
  const titleEl = frame.createDiv({ cls: 'tn-offers-frame-title' });
  titleEl.textContent = section.title || '(без заголовка)';

  // Без предложения оценить «есть ли системы» нельзя — и это не значит «нет».
  // Показываем все подразделы с их условием словами; пометка «не попадёт»
  // появляется только когда предложение открыто и условие проверено.
  const canJudge = deps.offer !== undefined;

  for (const sub of section.subsections) {
    const holds = canJudge ? conditionHoldsClient(sub.condition, section, deps.offer) : true;
    if (!holds) {
      frame.createDiv({
        cls: 'tn-offers-frame-subsection tn-offers-frame-skipped',
        text: `${sub.title || '(без заголовка)'} — ${SECTIONS_EDITOR_TEXTS.previewSkip}`,
      });
      continue;
    }
    const box = frame.createDiv({ cls: 'tn-offers-frame-subsection' });
    const repeatNote = sub.repeat === 'systems' ? ' · по каждой системе' : '';
    box.createSpan({ cls: 'tn-offers-frame-sub-title', text: sub.title || '(без заголовка)' });
    box.createSpan({ cls: 'tn-offers-muted', text: ` (${conditionWords(sub.condition)}${repeatNote})` });
  }
}

/** Чип копирования метки `{ content: <id> }` — как у блоков `{{block:<id>}}`. */
function copyChip(parent: HTMLElement, section: ContentSection): void {
  const chip = parent.createSpan({ cls: 'tn-offers-chip tn-offers-chip-copy', text: sectionMarker(section) });
  chip.title = SECTIONS_EDITOR_TEXTS.copyMarker;
  chip.addEventListener('click', () => {
    void (async () => {
      try {
        await navigator.clipboard.writeText(sectionMarker(section));
        new Notice(SECTIONS_EDITOR_TEXTS.copied);
      } catch (e: unknown) {
        new Notice(`${SECTIONS_EDITOR_TEXTS.copyFailed}: ${errorMessage(e)}`);
      }
    })();
  });
}