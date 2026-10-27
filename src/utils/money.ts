/**
 * Деньги в приложении: округление копеек и формат для показа.
 * ОДНО место на плагин — раздел КП и каталог пользуются этими функциями, а не
 * своими копиями (в трёх копиях уже разошлось поведение).
 */

/** Округление до копеек без двоичных хвостов. Множитель 1000 снимает артефакт
 *  представления: 1.005 * 100 = 100.49999999999999 дал бы 1.00 вместо 1.01.
 *  Тот же приём на сервере (`roundMoney`) и в боте. */
export function roundCents(value: number): number {
  return Math.round(Math.round(value * 1000) / 10) / 100;
}

/** Сумма для показа: «1 234,56». Знак — перед числом. */
export function formatMoney(value: number): string {
  const rounded = roundCents(value);
  const cents = Math.round(Math.abs(rounded) * 100);
  const sign = rounded < 0 ? '-' : '';
  const whole = Math.floor(cents / 100)
    .toString()
    .replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
  const frac = (cents % 100).toString().padStart(2, '0');
  return `${sign}${whole},${frac}`;
}

/** Итог по позициям: сумма строк, каждая округлена до копеек. */
export function itemsTotal(items: Array<{ qty: number; price: number }>): number {
  let total = 0;
  for (const item of items) total += roundCents(item.qty * item.price);
  return roundCents(total);
}