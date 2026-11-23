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

/*
 * «Технодайджест» — брендовая полоса (2026-11-23, по макету тёмной темы).
 *
 * Макет взят из письма ТЕХНОНИКОЛЬ: красная марка сверху, шапка с эмблемой и
 * плашкой месяца, вводный блок, ведущая новость кадром, остальные карточками,
 * баннер источника и подвал. Тёмная палитра живёт ТОЛЬКО в этом классе:
 * остальные экраны приложения остаются в теме Telegram — решение пользователя
 * 2026-11-23. Переменные локальные (--dg-*), чтобы бренд не протёк в
 * кнопки и ссылки других экранов.
 */
.app-digest {
  --dg-bg: #111722;
  --dg-deep: #090d14;
  --dg-card: #17202f;
  --dg-line: #1f2a3a;
  --dg-line-soft: #232f42;
  --dg-text: #e2e8f0;
  --dg-title: #f8fafc;
  --dg-muted: #94a3b8;
  --dg-body: #cbd5e1;
  --dg-red: #e30613;
  --dg-red-soft: #f87171;
  background: var(--dg-bg);
  color: var(--dg-text);
  border: 1px solid var(--dg-line);
  border-radius: var(--app-radius);
  overflow: hidden;
  margin-bottom: var(--app-gap);
  font-size: 14px;
}

.app-digest-bar {
  height: 4px;
  background: var(--dg-red);
}

.app-digest-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
  padding: 18px 16px 14px;
  border-bottom: 1px solid #1c2536;
}

.app-digest-brand {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.app-digest-mark {
  flex: none;
  background: var(--dg-red);
  color: #ffffff;
  font-weight: 900;
  font-size: 12px;
  line-height: 1;
  padding: 7px 8px;
  border-radius: 4px;
  letter-spacing: 0.5px;
}

.app-digest-brand-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.app-digest-word {
  font-size: 14px;
  font-weight: 900;
  color: var(--dg-title);
  letter-spacing: 0.3px;
  line-height: 1.15;
}

.app-digest-word-red {
  color: var(--dg-red);
}

.app-digest-word-tail {
  color: #fcfbfb;
}

.app-digest-tagline {
  font-size: 10px;
  font-weight: 600;
  color: var(--dg-muted);
  letter-spacing: 0.4px;
  text-transform: uppercase;
}

.app-digest-stamp {
  flex: none;
  background: #1a2332;
  border: 1px solid #283548;
  color: #cbd5e1;
  font-size: 11px;
  font-weight: 700;
  padding: 4px 9px;
  border-radius: 4px;
  white-space: nowrap;
}

.app-digest-months {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 12px 16px 0;
}

.app-digest .app-digest-month {
  background: #1a2332;
  border-color: #283548;
  color: #cbd5e1;
  font-size: 12px;
}

.app-digest .app-month-current {
  border-color: var(--dg-red);
  color: #fca5a5;
  background: #3b1419;
}

.app-digest-intro {
  padding: 18px 16px 6px;
}

.app-digest-eyebrow {
  font-size: 11px;
  font-weight: 800;
  color: var(--dg-red);
  text-transform: uppercase;
  letter-spacing: 0.8px;
  margin-bottom: 6px;
}

.app-digest-title {
  margin: 0 0 8px;
  font-size: 20px;
  line-height: 1.25;
  color: var(--dg-title);
  font-weight: 800;
  letter-spacing: -0.2px;
}

.app-digest-lead {
  margin: 0;
  font-size: 13.5px;
  line-height: 1.55;
  color: var(--dg-muted);
}

.app-digest-empty {
  padding: 8px 16px 20px;
  color: var(--dg-muted);
}

/* Ведущая новость: кадр, плашка рубрики поверх и текст под ним. */
.app-digest .app-news-lead {
  margin: 12px 16px 4px;
  padding: 0;
  border: 1px solid #253346;
  border-radius: 8px;
  background: var(--dg-card);
  overflow: hidden;
}

/* Новости выпуска — такие же карточки, как ведущая (2026-11-23: жалоба
 * «разъезжаются по всей ширине и почти наезжают друг на друга»). Раньше это
 * были строки во всю ширину полосы с пунктирным разделителем: на телефоне
 * текст одной новости читался как продолжение соседней. */
.app-digest .app-digest-news {
  margin: 10px 16px 0;
  padding: 14px;
  background: var(--dg-card);
  border: 1px solid #253346;
  border-radius: 8px;
}

.app-digest-hero-media {
  position: relative;
  height: 160px;
  background: linear-gradient(135deg, #0a0e17 0%, #161e2b 60%, #1f2b3e 100%);
  border-bottom: 3px solid var(--dg-red);
  display: flex;
  align-items: center;
  justify-content: center;
}

.app-digest-hero-media-empty {
  color: #ef4444;
}

.app-digest-hero-icon {
  width: 60px;
  height: 60px;
}

.app-digest-photo {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.app-digest-badge {
  position: absolute;
  top: 10px;
  left: 12px;
  background: rgba(227, 6, 19, 0.95);
  color: #ffffff;
  font-size: 9.5px;
  font-weight: 800;
  padding: 3px 7px;
  border-radius: 3px;
  letter-spacing: 0.5px;
  text-transform: uppercase;
}

.app-digest-hero-body {
  padding: 16px;
}

.app-digest .app-news-title {
  margin: 0 0 6px;
  font-size: 16px;
  line-height: 1.35;
  color: var(--dg-title);
  font-weight: 800;
}

.app-digest .app-news-lead .app-news-title {
  font-size: 18px;
}

.app-digest .app-news-title a {
  color: inherit;
  text-decoration: none;
}

.app-digest-byline {
  margin: 0 0 8px;
  color: var(--dg-muted);
  font-size: 12px;
}

.app-digest-hero-lead {
  margin: 0 0 10px;
  font-size: 13.5px;
  line-height: 1.5;
  color: var(--dg-body);
}

.app-digest-text {
  margin: 0 0 10px;
  font-size: 13.5px;
  line-height: 1.55;
  color: var(--dg-body);
}

.app-digest-hero-cta {
  margin: 0 0 10px;
}

.app-digest-button {
  display: inline-block;
  background: var(--dg-red);
  color: #ffffff !important;
  text-decoration: none;
  font-size: 13px;
  font-weight: 700;
  padding: 10px 16px;
  border-radius: 6px;
  letter-spacing: 0.2px;
}

/* Кнопка «Отправить в чат» в тёмной полосе: брендовая, а не синяя Telegram. */
.app-digest .app-button-ghost {
  background: transparent;
  color: var(--dg-red-soft);
  border: 1px solid #7f1d1d;
  font-size: 13px;
  padding: 7px 12px;
}

.app-digest .app-error-text {
  color: #fca5a5;
}

.app-digest-divider {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  margin: 6px 16px 0;
  padding-bottom: 8px;
  border-bottom: 2px solid var(--dg-line-soft);
}

.app-digest-divider-title {
  font-size: 11.5px;
  font-weight: 800;
  text-transform: uppercase;
  letter-spacing: 0.7px;
  color: #f1f5f9;
}

.app-digest-divider-side {
  font-size: 11px;
  font-weight: 700;
  color: var(--dg-red-soft);
}

.app-digest-thumb {
  display: block;
  width: 100%;
  height: 150px;
  object-fit: cover;
  border-radius: 6px;
  margin-bottom: 10px;
  background: #0a0e17;
}

.app-digest-chip {
  display: inline-block;
  background: #3b1419;
  color: #fca5a5;
  border: 1px solid #7f1d1d;
  font-size: 10px;
  font-weight: 700;
  padding: 2px 7px;
  border-radius: 3px;
  margin-bottom: 6px;
  text-transform: uppercase;
  letter-spacing: 0.3px;
}

.app-digest-news-lead {
  margin: 0 0 6px;
  font-size: 13.5px;
  line-height: 1.5;
  color: var(--dg-muted);
}

.app-digest-full {
  margin: 0 0 8px;
}

.app-digest-full summary {
  color: var(--dg-red-soft);
  font-size: 12.5px;
  font-weight: 700;
  cursor: pointer;
}

.app-digest-more {
  display: inline-block;
  margin-top: 2px;
  font-size: 12.5px;
  font-weight: 700;
  color: var(--dg-red-soft);
  text-decoration: none;
}

/* Нижний баннер: в макете здесь анонс Академии, у нас — источник выпуска. */
.app-digest-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin: 12px 16px 18px;
  padding: 14px 16px;
  background: #151d2a;
  border: 1px solid #253346;
  border-left: 4px solid var(--dg-red);
  border-radius: 6px;
}

.app-digest-banner-body {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.app-digest-banner-eyebrow {
  font-size: 10.5px;
  font-weight: 800;
  color: var(--dg-red-soft);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.app-digest-banner-title {
  font-size: 14px;
  font-weight: 700;
  color: var(--dg-title);
  line-height: 1.3;
}

.app-digest-banner-sub {
  font-size: 11.5px;
  color: var(--dg-muted);
}

.app-digest-outline {
  flex: none;
  display: inline-block;
  border: 1px solid #334155;
  background: #172030;
  color: #f1f5f9 !important;
  text-decoration: none;
  font-size: 12px;
  font-weight: 600;
  padding: 8px 14px;
  border-radius: 6px;
  white-space: nowrap;
}

.app-digest-foot {
  background: var(--dg-deep);
  border-top: 1px solid #1a2230;
  padding: 20px 16px;
}

.app-digest-foot-brand {
  font-size: 15px;
  font-weight: 900;
  color: #ffffff;
  letter-spacing: 0.5px;
}

.app-digest-foot-sub {
  font-size: 10.5px;
  font-weight: 600;
  color: var(--dg-muted);
  letter-spacing: 0.4px;
  text-transform: uppercase;
  margin-top: 2px;
}

@media screen and (max-width: 380px) {
  .app-digest-head {
    flex-wrap: wrap;
  }

  .app-digest-hero-media {
    height: 140px;
  }

  .app-digest-banner {
    flex-direction: column;
    align-items: flex-start;
  }
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