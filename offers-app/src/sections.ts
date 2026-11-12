/**
 * Тексты разделов КП (2026-11-13, шаг S4).
 *
 * Модель (совпадает с ботом один в один):
 *   - слоты: вступление → описание каждой выбранной системы → преимущества →
 *     условия; ключ слота приходит с сервера (`copy.intro`, `copy.system.1`, …);
 *   - три действия: написать, выправить присланное, принять/пропустить;
 *   - «пропустить» — пустой текст: ключ стирается на сервере, и документ
 *     собирается без этого раздела.
 *
 * Здесь только транспорт и чистая часть (адреса, порядок показа, подписи).
 * Генерация и правка — на сервере, теми же функциями, что у бота.
 */
import { apiRequest } from './api.ts';

export interface OfferSectionDto {
  key: string;
  label: string;
  text: string;
  warnings: string[];
}

export type SectionKind = 'intro' | 'system' | 'advantages' | 'conditions';

/** Как секция выглядит в адресе: `copy.system.1` → `system` + индекс 1. */
export function sectionAddress(key: string): { kind: SectionKind; systemIndex: number } | null {
  const dot = key.indexOf('.');
  const rest = dot >= 0 ? key.slice(dot + 1) : key;
  if (rest === 'intro' || rest === 'advantages' || rest === 'conditions') {
    return { kind: rest, systemIndex: 0 };
  }
  const match = /^system\.(\d+)$/.exec(rest);
  if (match) {
    const index = Number(match[1]);
    if (Number.isInteger(index) && index >= 1) return { kind: 'system', systemIndex: index };
  }
  return null;
}

function pathOf(id: number, key: string, suffix = ''): string {
  const address = sectionAddress(key);
  if (address === null) return '';
  const query = address.kind === 'system' ? `?systemIndex=${address.systemIndex}` : '';
  return `/offers/${id}/sections/${address.kind}${suffix}${query}`;
}

export async function loadSections(id: number): Promise<OfferSectionDto[]> {
  const res = await apiRequest<{ sections?: OfferSectionDto[] }>(`/offers/${id}/sections`);
  return res.sections ?? [];
}

export async function writeSection(id: number, key: string): Promise<OfferSectionDto> {
  return apiRequest<OfferSectionDto>(pathOf(id, key), { method: 'POST', body: {} });
}

export async function editSection(id: number, key: string, text: string): Promise<OfferSectionDto> {
  return apiRequest<OfferSectionDto>(pathOf(id, key, '/edit'), { method: 'POST', body: { text } });
}

/** Принять или пропустить (пустой текст) — в ответ приходит всё предложение. */
export async function acceptSection(id: number, key: string, text: string): Promise<{ formValues: Record<string, string> }> {
  return apiRequest<{ formValues: Record<string, string> }>(pathOf(id, key), { method: 'PUT', body: { text } });
}

/**
 * Порядок на экране — серверный (`SectionSlots`), но пустые слоты остаются
 * на месте: менеджер должен видеть, что раздел ещё не написан, а не искать,
 * куда он пропал.
 */
export function withCurrent(sections: OfferSectionDto[], current: OfferSectionDto): OfferSectionDto[] {
  return sections.map((section) => (section.key === current.key ? { ...section, ...current } : section));
}

/** Сколько разделов уже имеет текст — для строки состояния. */
export function filledCount(sections: OfferSectionDto[]): number {
  return sections.filter((section) => section.text.trim() !== '').length;
}