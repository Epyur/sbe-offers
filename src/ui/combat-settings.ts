import { Notice } from 'obsidian';
import type { OffersView } from './offers-view';
import type { ContentSection, OfferForm } from '../types/offers';
import { errorMessage } from '../../../sbe-core/src/utils/errors';
import { copyMarker, pickLayoutFile, renderLayoutCard } from './form-layout';
import { COMBAT_MARKERS } from './markers';
import { renderSectionsEditor } from './sections-editor';
import { cloneSections, combatSectionsProblem } from './sections-model';

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
 * 2026-11-18 — четвёртый блок, СОДЕРЖАТЕЛЬНЫЕ РАЗДЕЛЫ (решение пользователя,
 * вариант A — «только разделы»). До этого структура листка задавалась заготовкой
 * в коде сервера (`combatDefaultSections`), и поменять её было нельзя без
 * выпуска. Редактор переиспользован у КП (`sections-editor.ts`), но в режиме
 * `combat`: id раздела не меняется (на нём держится кэш текстов) и метки
 * `{ content: … }` в макете листка нет.
 *
 * Настраиваемо ТОЛЬКО содержание разделов. Каркас документа — продукты
 * сравнения, две стороны, разделы, ссылки — остаётся в коде: это решение
 * пользователя 0.11.6 (реверс был бы новой настройкой, а не правкой).
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

  // 0. Содержательные разделы (2026-11-18). Первым блоком: это и есть содержание
  // листка — что именно сравнивать, а всё остальное (общая часть промта, ссылки,
  // макет) настраивает его вид.
  //
  // `sections` — отдельная переменная, а не `form.sections`: редактор правит
  // массив НА МЕСТЕ, а «↩ Вернуть заготовку» заменяет его целиком. Держать оба
  // пути в одном поле формы значило бы забыть, какой из них сейчас актуален.
  let sections: ContentSection[] = form.sections ?? [];
  const sectionsCard = container.createDiv({ cls: 'tn-offers-card' });
  sectionsCard.createDiv({ cls: 'tn-offers-muted', text: 'Содержательные разделы боевого листка' });
  sectionsCard.createDiv({
    cls: 'tn-offers-muted',
    text:
      'Раздел — это бриф для модели: подразделы, условия, инструкции и повтор по ' +
      'продуктам. Оформление берётся из макета листка, поэтому настроек шрифтов и ' +
      'заголовков здесь нет. Порядок разделов — порядок печати. ' +
      'id раздела после создания не меняется: на нём держится кэш уже написанных ' +
      'текстов.',
  });
  const sectionsBody = sectionsCard.createDiv();
  const drawSections = (): void => {
    sectionsBody.empty();
    if (sections.length === 0) {
      // Пустой список — состояние только внутри окна: удалили всё и ещё не
      // сохранили. Объясняем ДО нажатия «Сохранить», потому что пустую настройку
      // сервер отклоняет (у листка нет состояния «без разделов»: печать
      // подставляет заготовку, и чтение тоже).
      sectionsBody.createDiv({
        cls: 'tn-offers-muted',
        text: 'Разделов нет — такую настройку сервер не примет: добавьте раздел или верните заготовку.',
      });
    }
    renderSectionsEditor(sectionsBody, sections, {
      combat: true,
      onChange: () => {
        /* Предпросмотр рамки живёт внутри редактора, снаружи перерисовывать нечего. */
      },
    });
  };
  drawSections();
  const restoreSectionsBtn = sectionsCard.createEl('button', {
    cls: 'tn-btn tn-btn-ghost',
    text: '↩ Вернуть заготовку',
  });
  restoreSectionsBtn.setAttribute(
    'title',
    'Поставить разделы, с которыми листок печатается, пока администратор не задал свои',
  );
  restoreSectionsBtn.addEventListener('click', () => {
    const fallback = form.sectionsDefault ?? [];
    // Заготовки нет — сервер старой сборки (поле появилось 2026-11-18). Честно
    // сказать об этом лучше, чем очистить редактор: пустой список сохранить
    // нельзя, и человек искал бы причину в своих разделах.
    if (fallback.length === 0) {
      new Notice(
        'Сервер не отдал заготовку разделов — вероятно, обновлён только плагин. ' +
          'Обновите сервис (версия 0.11.9 и новее) или добавьте раздел вручную.',
      );
      return;
    }
    // Копия, а не сама заготовка: редактор правит разделы на месте, и вторая
    // правка испортила бы заготовку, пришедшую с сервера.
    sections = cloneSections(fallback);
    drawSections();
    new Notice('Заготовка разделов подставлена — не забудьте сохранить настройки');
  });

  // 1. Базовый промт документа.
  const promptCard = container.createDiv({ cls: 'tn-offers-card' });
  promptCard.createDiv({ cls: 'tn-offers-muted', text: 'Базовый промт боевого листка' });
  promptCard.createDiv({
    cls: 'tn-offers-muted',
    text:
      'Этот текст получает модель вместе с брифом каждого раздела: формат ответа, ' +
      'правила сторон и формулировок, требования к согласованности. Содержание листка ' +
      '(какие разделы сравниваются) правится блоком «Содержательные разделы» выше, ' +
      'здесь — общая часть промта. Пустое поле — встроенный текст.',
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
      // Пустое сохранение отклоняем ДО запроса, русским текстом (2026-11-18).
      // Сервер проверяет то же правило (`combatEmptySectionsProblem`): здесь
      // отказ приходит сразу, без круглого отказа, а текст один и тот же.
      const problem = combatSectionsProblem(sections);
      if (problem) {
        new Notice(problem);
        return;
      }
      try {
        // Полная форма: сервер записывает её целиком, и частичный payload стёр бы
        // разделы и поля листка. Меняются здесь разделы и базовый промт.
        //
        // Текст, равный встроенному, сохраняем ПУСТЫМ: «пусто = встроенный» —
        // правило формы, и оно же держит ключ кэша прежним. Иначе первое же
        // сохранение окна переписало бы все тексты разделов без единой правки.
        const promptValue = promptArea.value.trim();
        form = await view.plugin.offersService.saveForm({
          ...form,
          sections,
          basePrompt: promptValue === basePromptDefault ? '' : promptValue,
        });
        // `sections` НЕ перекладываем из ответа: редактор держит ЭТОТ массив и
        // правит его на месте. Подменив переменную ответом сервера, мы оставили
        // бы редактор на старом массиве, и следующее сохранение отправило бы
        // состояние до правки — тихая потеря введённого.
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