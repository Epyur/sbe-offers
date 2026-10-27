import type { OffersView } from './offers-view';

export function renderOffersList(view: OffersView, container: HTMLElement): void {
  container.createDiv({ cls: 'tn-offers-empty', text: 'Загрузка…' });
  void view;
}
