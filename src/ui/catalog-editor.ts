import type { OffersView } from './offers-view';

export function renderCatalogEditor(view: OffersView, container: HTMLElement): void {
  container.createDiv({ cls: 'tn-offers-empty', text: 'Загрузка…' });
  void view;
}
