/**
 * Дата в коротком виде для таблиц: 29.10.2026.
 *
 * Одна функция на плагин: раньше она была скопирована в разделе карточек и
 * понадобилась материалам и системам — три копии разошлись бы в мелочах вроде
 * пустого значения.
 */
export function formatShortDate(raw: string): string {
  if (!raw) return '—';
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return raw;
  const pad = (n: number): string => String(n).padStart(2, '0');
  return `${pad(d.getDate())}.${pad(d.getMonth() + 1)}.${d.getFullYear()}`;
}