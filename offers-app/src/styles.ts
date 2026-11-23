/**
 * Визуальная основа (2026-11-13, S0).
 *
 * Цвета берутся из переменных Telegram (`--tg-theme-*`), которые платформа
 * выставляет на <html> для каждой темы пользователя. Своих палитр минимум:
 * только то, чего в Telegram нет (внутренние отступы, радиусы).
 *
 * Плагинные токены из `sbe-core` сюда намеренно не копируются (§7.5 плана):
 * у приложения другая тема — тема человека в Telegram, а не вольта.
 *
 * Стили вставляются из TS, а не импортируются CSS-файлом: `cssCodeSplit`
 * выключен, файл всё равно попадает в один бандл, а так у него ровно одна
 * точка подключения и нет второго источника правды.
 */

const STYLE_ID = 'offers-app-styles';

const CSS = `
:root {
  color-scheme: light dark;
  --tg-bg: var(--tg-theme-bg-color, #ffffff);
  --tg-secondary-bg: var(--tg-theme-secondary-bg-color, #f2f2f7);
  --tg-text: var(--tg-theme-text-color, #000000);
  --tg-hint: var(--tg-theme-hint-color, #8e8e93);
  --tg-link: var(--tg-theme-link-color, #2481cc);
  --app-gap: 12px;
  --app-radius: 12px;
}

*,
*::before,
*::after {
  box-sizing: border-box;
}

html,
body {
  margin: 0;
  padding: 0;
  min-height: 100%;
}

body {
  background: var(--tg-bg);
  color: var(--tg-text);
  font: 16px/1.45 -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  -webkit-font-smoothing: antialiased;
  padding: var(--app-gap);
  padding-bottom: max(var(--app-gap), env(safe-area-inset-bottom));
}

h1 {
  font-size: 20px;
  margin: 0 0 var(--app-gap);
}

p {
  margin: 0 0 var(--app-gap);
}

code {
  font-size: 13px;
  word-break: break-all;
}

.app-card {
  background: var(--tg-secondary-bg);
  border-radius: var(--app-radius);
  padding: var(--app-gap);
  margin-bottom: var(--app-gap);
}

.app-muted {
  color: var(--tg-hint);
  font-size: 14px;
}

.app-nav {
  display: flex;
  gap: 8px;
  margin-top: var(--app-gap);
}

.app-link {
  color: var(--tg-link);
  text-decoration: none;
  padding: 8px 0;
  font-size: 15px;
}

.app-search {
  position: relative;
  margin-bottom: var(--app-gap);
}

.app-input {
  width: 100%;
  box-sizing: border-box;
  padding: 10px 36px 10px 12px;
  border: 1px solid var(--tg-hint);
  border-radius: var(--app-radius);
  background: var(--tg-secondary-bg);
  color: var(--tg-text);
  font: inherit;
}

.app-input:focus {
  outline: 2px solid var(--tg-link);
  outline-offset: -1px;
}

.app-clear {
  position: absolute;
  right: 4px;
  top: 50%;
  transform: translateY(-50%);
  border: 0;
  background: transparent;
  color: var(--tg-hint);
  font-size: 16px;
  padding: 8px;
  cursor: pointer;
}

.app-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: var(--app-gap);
}

.app-row {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 2px 8px;
  padding: var(--app-gap);
  border-radius: var(--app-radius);
  background: var(--tg-secondary-bg);
  color: var(--tg-text);
  text-decoration: none;
}

.app-row-main {
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
}

.app-row-side {
  color: var(--tg-hint);
  font-size: 14px;
  white-space: nowrap;
}

.app-row-meta {
  grid-column: 1 / -1;
  color: var(--tg-hint);
  font-size: 13px;
}

.app-error {
  border: 1px solid var(--tg-link);
}

.app-button {
  padding: 8px 14px;
  border: 0;
  border-radius: var(--app-radius);
  background: var(--tg-link);
  color: #ffffff;
  font: inherit;
  cursor: pointer;
}

.app-h2 {
  font-size: 16px;
  margin: 0 0 var(--app-gap);
  color: var(--tg-text);
}

.app-field {
  display: block;
  margin-bottom: 8px;
}

.app-field-label {
  display: block;
  font-size: 12px;
  color: var(--tg-hint);
  margin-bottom: 2px;
}

.app-field-small {
  flex: 1 1 0;
  min-width: 0;
}

.app-item-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.app-button-ghost {
  background: transparent;
  color: var(--tg-link);
  border: 1px solid var(--tg-link);
}

.app-button:disabled {
  opacity: 0.5;
}

.app-link-button {
  border: 0;
  background: transparent;
  color: var(--tg-link);
  font: inherit;
  font-size: 14px;
  padding: 0;
  cursor: pointer;
}

.app-note {
  color: var(--tg-text);
  background: var(--tg-secondary-bg);
  border-left: 3px solid var(--tg-link);
  padding: 8px 12px;
  border-radius: 4px;
}

.app-error-text {
  color: #d9534f;
  font-size: 14px;
}

.app-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  list-style: none;
  margin: 0 0 var(--app-gap);
  padding: 0;
}

.app-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 8px 6px 10px;
  border-radius: 999px;
  background: var(--tg-bg);
  border: 1px solid var(--tg-hint);
  font-size: 14px;
}

.app-chip-x {
  border: 0;
  background: transparent;
  color: var(--tg-hint);
  font-size: 13px;
  padding: 2px 4px;
  cursor: pointer;
}

.app-search-block {
  margin-bottom: var(--app-gap);
}

.app-results {
  list-style: none;
  margin: 6px 0 0;
  padding: 0;
}

.app-result {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  width: 100%;
  text-align: left;
  padding: 10px;
  margin-bottom: 6px;
  border: 1px solid var(--tg-hint);
  border-radius: var(--app-radius);
  background: var(--tg-bg);
  color: var(--tg-text);
  font: inherit;
  cursor: pointer;
}

.app-result:disabled {
  opacity: 0.55;
}

.app-items {
  list-style: none;
  margin: 0 0 var(--app-gap);
  padding: 0;
}

.app-item {
  border-top: 1px solid var(--tg-hint);
  padding-top: var(--app-gap);
  margin-bottom: var(--app-gap);
}

.app-item:first-child {
  border-top: 0;
  padding-top: 0;
}

.app-section {
  border-top: 1px solid var(--tg-hint);
  padding-top: var(--app-gap);
  margin-top: var(--app-gap);
}

.app-section:first-of-type {
  border-top: 0;
  padding-top: 0;
  margin-top: 0;
}

.app-section-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 6px;
}

.app-section-label {
  font-weight: 600;
}

.app-textarea {
  width: 100%;
  box-sizing: border-box;
  padding: 10px;
  border: 1px solid var(--tg-hint);
  border-radius: var(--app-radius);
  background: var(--tg-bg);
  color: var(--tg-text);
  font: inherit;
  font-size: 14px;
  resize: vertical;
  margin-bottom: 8px;
}

.app-textarea:focus {
  outline: 2px solid var(--tg-link);
  outline-offset: -1px;
}

.app-warnings {
  margin: 0 0 6px;
  padding-left: 18px;
  color: #d9534f;
  font-size: 13px;
}

.app-placeholder {
  border: 1px dashed var(--tg-hint);
  border-radius: var(--app-radius);
  padding: calc(var(--app-gap) * 2);
  text-align: center;
  color: var(--tg-hint);
}

/*
 * Окно КП (2026-11-15, шаги 2–3 конфигуратора окна).
 *
 * app-window-note — строка состояния экрана: настройку окна не прочитали,
 * показаны все разделы. app-required — МЯГКАЯ подсветка незаполненного
 * обязательного: рамка, а не блокирующая кнопка. Сохранение обязано проходить
 * (решение пользователя 2026-11-15).
 */
.app-window-note {
  color: var(--tg-hint);
  background: var(--tg-secondary-bg);
  border-left: 3px solid var(--tg-hint);
  padding: 8px 12px;
  border-radius: 4px;
  font-size: 14px;
}

.app-required {
  border: 1px solid #d9534f;
}

p.app-required {
  border: 0;
  border-left: 3px solid #d9534f;
}

/* Приветствие: три карточки-заглушки разделов (2026-11-14). */
.app-sections {
  display: flex;
  flex-direction: column;
  gap: var(--app-gap);
  margin-bottom: var(--app-gap);
}

.app-section-tile {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-radius: var(--app-radius);
  background: var(--tg-secondary-bg);
  color: var(--tg-text);
  text-decoration: none;
}

.app-section-tile:active {
  opacity: 0.85;
}

.app-section-image {
  display: block;
  width: 100%;
  aspect-ratio: 1376 / 768;
  object-fit: cover;
  background: var(--tg-bg);
}

/* Пока картинка раздела не вшита (section-images.ts): узор вместо пустоты. */
.app-section-image-offers {
  background: linear-gradient(135deg, #2f6db5 0%, #7fb2e5 100%);
}

.app-section-image-sheets {
  background: linear-gradient(135deg, #8c3a2e 0%, #d98a65 100%);
}

.app-section-image-ambulance {
  background: linear-gradient(135deg, #b3261e 0%, #f2a900 100%);
}

/*
 * «Технодайджест» — макет квадратный (144x157), а карточка широкая: cover от
 * него оставил бы узкую полосу. Показываем картинку целиком на подложке.
 */
.app-section-image-digest {
  object-fit: contain;
  border-radius: var(--app-radius);
  background: #0f1b2d;
}

.app-section-body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--app-gap);
}

.app-section-title {
  font-weight: 600;
  font-size: 17px;
}

.app-section-caption {
  color: var(--tg-hint);
  font-size: 13px;
}

/*
 * Разделы строкой на экране списка (2026-11-14): приветствие осталось только
 * на главной, и без этой строки из КП не видно, что есть боевые листки.
 */
.app-sections-compact {
  flex-direction: row;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 8px;
}

.app-section-chip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px 6px 6px;
  border-radius: 999px;
  border: 1px solid var(--tg-hint);
  background: var(--tg-secondary-bg);
  color: var(--tg-text);
  text-decoration: none;
  font-size: 14px;
}

.app-section-chip-current {
  border-color: var(--tg-link);
  color: var(--tg-link);
  background: transparent;
}

.app-section-chip-image {
  display: block;
  width: 30px;
  height: 30px;
  border-radius: 999px;
  object-fit: cover;
  background: var(--tg-bg);
}

/*
 * Блок «Бриф» (2026-11-15): темы читаются как список «подпись — значение».
 * Значение может быть многострочным (список болей), поэтому это колонка, а не
 * строка: длинная фраза в узком телефоне иначе растянет карточку вбок.
 */
/*
 * Два ряда кнопок в карточке раздела (0.11.3). В один ряд четыре кнопки не
 * помещались, и «Пропустить» уезжала за правый край экрана — её и убрали.
 * Равные колонки не дают кнопке раздуться от длинной подписи.
 */
.app-row-2 {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 8px;
  margin-bottom: 8px;
}

/* Три кнопки в один ряд (2026-11-17): выбор источника конкурента —
   «База знаний / Файл / URL». Подпись «База знаний» длинная, поэтому в
   узкой колонке уменьшены поля и кегль, а не текст: сокращать подписи
   значило бы заставлять человека догадываться, что за источник. */
.app-row-3 {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 6px;
  margin-bottom: 8px;
}

.app-row-3 .app-button {
  padding: 8px 6px;
  font-size: 13px;
}

/*
 * Шторка брифа (0.11.3): бриф вызывается из карточки раздела, а не живёт
 * отдельным блоком экрана. На узком телефоне это единственное место, где
 * общий бриф доступен без прокрутки наверх.
 */
.app-sheet-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: flex-end;
  z-index: 10;
}

.app-sheet {
  background: var(--tg-bg);
  color: var(--tg-text);
  width: 100%;
  max-height: 85vh;
  overflow-y: auto;
  border-radius: var(--app-radius) var(--app-radius) 0 0;
  padding: var(--app-gap);
  padding-bottom: max(var(--app-gap), env(safe-area-inset-bottom));
}

.app-sheet-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 6px;
}

.app-sheet-title {
  font-size: 17px;
  font-weight: 600;
}

/*
 * «Технодайджест» (2026-11-23): лента новостей выпуска из Базы знаний.
 * Вёрстка простая и «газетная» настолько, насколько это уместно в телефоне:
 * рубрика — над заголовком, лид курсивом, текст абзацами. Своей полосы A4
 * (как в Базе знаний) здесь нет намеренно — на телефоне она нечитаема.
 */
.app-months {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: var(--app-gap);
}

.app-month {
  padding: 6px 10px;
  border-radius: 999px;
  border: 1px solid var(--tg-hint);
  color: var(--tg-text);
  text-decoration: none;
  font-size: 14px;
  background: var(--tg-secondary-bg);
}

.app-month-current {
  border-color: var(--tg-link);
  color: var(--tg-link);
  background: transparent;
}

.app-news {
  padding: var(--app-gap) 0;
  border-top: 1px solid var(--tg-hint);
}

.app-news:first-of-type {
  border-top: 0;
}

.app-news-rubric {
  margin: 0 0 2px;
  color: var(--tg-link);
  font-size: 12px;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.app-news-title {
  font-size: 18px;
  margin: 0 0 4px;
}

.app-news-lead .app-news-title {
  font-size: 21px;
}

.app-news-byline {
  color: var(--tg-hint);
  font-size: 13px;
  margin: 0 0 6px;
}

.app-news-lead-text {
  font-style: italic;
  color: var(--tg-hint);
}

.app-news-send {
  margin-top: 8px;
}

.app-brief-fields {
  list-style: none;
  margin: 0 0 var(--app-gap);
  padding: 0;
}

.app-brief-field {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 6px 0;
  border-top: 1px solid var(--tg-hint);
}

.app-brief-field:first-child {
  border-top: 0;
  padding-top: 0;
}

.app-brief-label {
  font-size: 12px;
  color: var(--tg-hint);
}

.app-brief-value {
  white-space: pre-wrap;
  word-break: break-word;
}

.app-brief-answer {
  margin-bottom: var(--app-gap);
}

.app-brief-turns {
  margin-top: var(--app-gap);
}

.app-brief-turns summary {
  color: var(--tg-link);
  font-size: 14px;
  cursor: pointer;
}

.app-brief-turns-list {
  list-style: none;
  margin: 8px 0 0;
  padding: 0;
  font-size: 14px;
}

.app-brief-turn {
  padding: 4px 0;
  white-space: pre-wrap;
  word-break: break-word;
}
`;

/** Идемпотентно: повторный вызов (StrictMode) не плодит второй <style>. */
export function registerStyles(): void {
  if (document.getElementById(STYLE_ID) !== null) return;
  const style = document.createElement('style');
  style.id = STYLE_ID;
  style.textContent = CSS;
  document.head.append(style);
}