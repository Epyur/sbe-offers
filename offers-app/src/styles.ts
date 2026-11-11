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