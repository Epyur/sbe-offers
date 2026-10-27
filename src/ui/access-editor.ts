import type { OffersView } from './offers-view';

export async function renderAccessEditor(view: OffersView, container: HTMLElement): Promise<void> {
  container.createDiv({ cls: 'tn-offers-empty', text: 'Загрузка…' });
  void view;
}
