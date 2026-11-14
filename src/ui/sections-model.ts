import type { ContentSection, Offer, SubsectionCondition } from '../types/offers';

/**
 * Правила содержательных разделов для ПРЕДПРОСМОТРА (2026-11-14, шаг 6).
 *
 * Файл чистый: ни Obsidian, ни сети, ни DOM. Поэтому его можно прогнать
 * отдельным скриптом (`scripts/check_sections_conditions.mjs`), и зеркало
 * серверных условий не уедет от сервера молча. Правила печати живут на
 * сервере (`offers-service/sections.go`); здесь — те же правила, но чтобы
 * показать человеку форму рамки до выгрузки, а не напечатать документ.
 */

/** Подписи условий — те же слова, что в дизайне §3.3. */
const CONDITION_LABELS: Record<SubsectionCondition, string> = {
  always: 'всегда',
  has_systems: 'есть выбранные системы',
  has_materials: 'есть выбранные материалы',
  multi_system: 'систем больше одной',
  has_critique: 'включён переключатель «критика»',
  has_items: 'есть позиции в предложении',
};

export const CONDITION_ORDER: SubsectionCondition[] = [
  'always',
  'has_systems',
  'has_materials',
  'multi_system',
  'has_critique',
  'has_items',
];

/** Что увидит администратор глазами: «выводится, если …». */
export function conditionWords(condition: SubsectionCondition | undefined): string {
  return CONDITION_LABELS[condition ?? 'always'] ?? CONDITION_LABELS.always;
}

/**
 * Выполняется ли условие на предложении — та же таблица, что `conditionHolds`
 * на сервере (`sections.go`), но без сети.
 *
 * `has_critique` читает ПЕРЕКЛЮЧАТЕЛЬ раздела, остальные — само предложение.
 * Незнакомое условие не печатает: сервер его тоже не печатает.
 */
export function conditionHoldsClient(
  condition: SubsectionCondition | undefined,
  section: Pick<ContentSection, 'critique'>,
  offer: Offer | undefined,
): boolean {
  const systems = offer?.systems?.length ?? 0;
  const materials = offer?.materials?.length ?? 0;
  const items = offer?.items?.length ?? 0;
  switch (condition ?? 'always') {
    case 'always':
      return true;
    case 'has_systems':
      return systems >= 1;
    case 'has_materials':
      return materials >= 1;
    case 'multi_system':
      return systems > 1;
    case 'has_critique':
      return section.critique === true;
    case 'has_items':
      return items >= 1;
    default:
      return false;
  }
}

/** Метка макета для раздела: `{ content: <id> }`. */
export function sectionMarker(section: Pick<ContentSection, 'id'>): string {
  return `{ content: ${section.id} }`;
}