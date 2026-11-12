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
`;

/** Идемпотентно: повторный вызов (StrictMode) не плодит второй <style>. */
export function registerStyles(): void {
  if (document.getElementById(STYLE_ID) !== null) return;
  const style = document.createElement('style');
  style.id = STYLE_ID;
  style.textContent = CSS;
  document.head.append(style);
}