import { Notice } from 'obsidian';
import type { OffersView } from './offers-view';
import type { OfferForm } from '../types/offers';
import { errorMessage } from '../../../sbe-core/src/utils/errors';
import { copyMarker, pickLayoutFile, renderLayoutCard } from './form-layout';
import { COMBAT_MARKERS } from './markers';

/**
 * «⚙️ Настройки боевого листка» (2026-11-17).
 *
 * Просьба пользователя: убрать кнопку «Макет боевого листка» и дать одно окно
 * настроек листка из трёх блоков:
 *   1) базовый промт документа с копированием меток для макета;
 *   2) управление ссылками — в листок идут все ссылки наших продуктов, включая
 *      документы из приложения «Документы» (файлы в S3), на которые есть ссылка
 *      в заметке Базы знаний;
 *   3) макет: скачать и загрузить (и удалить — иначе нельзя вернуться к
 *      встроенному оформлению).
 *
 * Содержательные разделы листка (что именно сравнивать) остаются заготовкой в
 * коде: решение пользователя 2026-11-17 — «оставить как есть, без окна».
 *
 * Окно СВОЁ, а не общий редактор формы КП: у листка нет полей формы, блоков и
 * окна приложения, и показывать их значило бы давать настройки, которые ни на
 * что не влияют (ровно на это жаловался пользователь в 0.11.5).
 */
export function renderCombatSettings(view: OffersView, container: HTMLElement, onBack: () => void): void {
  container.createDiv({ cls: 'tn-offers-muted', text: 'Загрузка настроек боевого листка…' });
  void load(view, container, onBack);
}

async function load(view: OffersView, container: HTMLElement, onBack: () => void): Promise<void> {
  let form: OfferForm;
  try {
    form = await view.plugin.offersService.getForm('combat');
  } catch (e: unknown) {
    container.empty();
    container.createDiv({
      cls: 'tn-offers-error',
      text: `Не удалось загрузить настройки боевого листка: ${errorMessage(e)}`,
    });
    return;
  }
  container.empty();
  render(view, container, form, onBack);
}

function render(view: OffersView, container: HTMLElement, form: OfferForm, onBack: () => void): void {
  const back = container.createEl('button', { text: '← К списку', cls: 'tn-btn tn-btn-ghost' });
  back.addEventListener('click', () => {
    onBack();
  });

  const basePromptInitial = (form.basePrompt ?? '').trim();
  const basePromptDefault = (form.basePromptDefault ?? '').trim();

  // 1. Базовый промт документа.
  const promptCard = container.createDiv({ cls: 'tn-offers-card' });
  promptCard.createDiv({ cls: 'tn-offers-muted', text: 'Базовый промт боевого листка' });
  promptCard.createDiv({
    cls: 'tn-offers-muted',
    text:
      'Этот текст получает модель вместе с брифом каждого раздела: формат ответа, ' +
      'правила сторон и формулировок, требования к согласованности. Содержание листка ' +
      '(какие разделы сравниваются) задано заготовкой, здесь правится общая часть промта. ' +
      'Пустое поле — встроенный текст.',
  });
  const promptArea = promptCard.createEl('textarea', {
    cls: 'tn-offers-input tn-offers-base-prompt',
    attr: { rows: '14', spellcheck: 'false' },
  });
  promptArea.value = basePromptInitial || basePromptDefault;
  const resetBtn = promptCard.createEl('button', { cls: 'tn-btn tn-btn-ghost', text: '↩ Вернуть базовый текст' });
  resetBtn.addEventListener('click', () => {
    promptArea.value = basePromptDefault;
  });
  renderCombatMarkers(promptCard, 'Метки для макета боевого листка — нажмите, чтобы скопировать:');

  // 2. Ссылки: что именно попадает в листок.
  const linksCard = container.createDiv({ cls: 'tn-offers-card' });
  linksCard.createDiv({ cls: 'tn-offers-muted', text: 'Ссылки в боевом листке' });
  linksCard.createDiv({
    cls: 'tn-offers-muted',
    text:
      'В листок идут ВСЕ ссылки наших продуктов из их карточек Базы знаний: страницы на ' +
      'сайте, техлисты, узлы, сертификаты — и документы из приложения «Документы» ' +
      '(файлы в S3), если на них есть ссылка в заметке. Порядок — как в заметке, повторы ' +
      'по адресу убираются. Ссылки конкурентов не печатаются: листок читают как нашу ' +
      'рекомендацию.',
  });
  linksCard.createDiv({
    cls: 'tn-offers-muted',
    text:
      'Где напечатать список, задаёт макет: поставьте метку {{ссылки}} в нужное место. ' +
      'Без макета листок дописывает ссылки в конце сам.',
  });
  linksCard.createDiv({
    cls: 'tn-offers-muted',
    text:
      'Карточки для листка читаются из кэша «Контекст КП», а не из раздела «Системы»: ' +
      'если документ добавили в заметку только что, откройте «Контекст КП» и нажмите там ' +
      '«↻ Перечитать из Базы знаний» — до этого в листке будет прежний набор ссылок. ' +
      'После перечитывания соберите листок заново.',
  });
  renderCombatMarkers(linksCard, 'Метка списка ссылок:');

  // 3. Макет листка: скачать, загрузить, удалить. Карточку рисует общий
  // `renderLayoutCard` в режиме `combat` — свой заголовок она несёт сама, а
  // метки КП и образец КП в этом режиме не показываются (2026-11-17).
  const layoutBody = container.createDiv();
  const drawLayout = (): void => {
    layoutBody.empty();
    renderLayoutCard(view, layoutBody, form, {
      // Перерисовываем ТОЛЬКО карточку макета: правка промта в поле выше ещё не
      // сохранена, и перезагрузка экрана потеряла бы её.
      onChanged: () => {
        void view.plugin.offersService
          .getForm('combat')
          .then((fresh) => {
            form = fresh;
            drawLayout();
          })
          .catch((e: unknown) => {
            new Notice(`Не удалось перечитать макет: ${errorMessage(e)}`);
          });
      },
      onPickFile: pickLayoutFile,
      mode: 'combat',
    });
  };
  drawLayout();

  const actions = container.createDiv({ cls: 'tn-offers-actions' });
  const saveBtn = actions.createEl('button', { text: '💾 Сохранить настройки', cls: 'tn-btn tn-btn-primary' });
  saveBtn.addEventListener('click', () => {
    void (async () => {
      try {
        // Полная форма: сервер записывает её целиком, и частичный payload стёр бы
        // разделы и поля листка. Меняется здесь только базовый промт.
        //
        // Текст, равный встроенному, сохраняем ПУСТЫМ: «пусто = встроенный» —
        // правило формы, и оно же держит ключ кэша прежним. Иначе первое же
        // сохранение окна переписало бы все тексты разделов без единой правки.
        const promptValue = promptArea.value.trim();
        form = await view.plugin.offersService.saveForm({
          ...form,
          basePrompt: promptValue === basePromptDefault ? '' : promptValue,
        });
        new Notice('Настройки боевого листка сохранены');
      } catch (e: unknown) {
        new Notice(`Не удалось сохранить настройки: ${errorMessage(e)}`);
      }
    })();
  });
  const cancelBtn = actions.createEl('button', { text: '↻ Отменить правки', cls: 'tn-btn tn-btn-ghost' });
  cancelBtn.addEventListener('click', () => {
    void load(view, container, onBack);
  });
}

/** Метки макета листка: подпись и копирование по клику. */
function renderCombatMarkers(container: HTMLElement, title: string): void {
  container.createDiv({ cls: 'tn-offers-muted', text: title });
  const row = container.createDiv({ cls: 'tn-offers-row tn-offers-chip-row tn-offers-chip-wrap' });
  for (const variant of COMBAT_MARKERS) {
    const chip = row.createEl('button', { cls: 'tn-offers-chip tn-offers-chip-btn', text: variant.label });
    chip.setAttribute('title', `${variant.marker} — нажмите, чтобы скопировать`);
    chip.addEventListener('click', (event) => {
      event.preventDefault();
      void copyMarker(variant.marker);
    });
  }
}