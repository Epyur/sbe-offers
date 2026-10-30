import { ItemView, WorkspaceLeaf } from 'obsidian';
import type SbeOffersPlugin from '../main';
import { OFFERS_VIEW_TYPE } from '../constants';
import { OffersError } from '../services/offers.service';
import { errorMessage } from '../../../sbe-core/src/utils/errors';
import { renderHelpButton } from '../../../sbe-core/src/ui/help-modal';
import { OFFERS_HELP } from './help';
import { renderOffersList } from './offers-list';
import { renderCatalogEditor } from './catalog-editor';
import { renderFormEditor } from './form-editor';
import { renderAccessEditor } from './access-editor';
import { renderContextEditor } from './context-editor';
import { isAdminRole, type Role } from '../types/offers';

export type NavKey = 'offers' | 'catalog' | 'context' | 'form' | 'access';

export class OffersView extends ItemView {
  plugin: SbeOffersPlugin;
  private rootEl!: HTMLElement;
  private navEl!: HTMLElement;
  private pageTitleEl!: HTMLElement;
  private pageSubEl!: HTMLElement;
  private bodyEl!: HTMLElement;
  private key: NavKey = 'offers';
  private myRole: Role | '' = '';
  private myEmail = '';

  constructor(leaf: WorkspaceLeaf, plugin: SbeOffersPlugin) {
    super(leaf);
    this.plugin = plugin;
  }

  getViewType(): string {
    return OFFERS_VIEW_TYPE;
  }

  getDisplayText(): string {
    return 'LogicTEAM.КП';
  }

  getIcon(): string {
    return 'receipt';
  }

  async onOpen(): Promise<void> {
    const container = this.contentEl;
    container.empty();
    container.addClass('tn-offers-container');
    this.rootEl = container.createDiv({ cls: 'tn-offers-app' });

    // Каркас строится ДО запроса роли: иначе ошибку «Ключ недействителен»
    // некуда написать — подзаголовка страницы ещё нет.
    this.buildShell();
    await this.loadRole();
    this.buildNavForRole();
    this.renderPage();
  }

  refresh(): void {
    this.renderPage();
  }

  /** Перечитать роль с сервера (кнопка в шапке) — права могли изменить в ЦУП. */
  async reloadRole(): Promise<void> {
    await this.loadRole();
    this.renderPage();
  }

  private async loadRole(): Promise<void> {
    try {
      const me = await this.plugin.offersService.me();
      this.myRole = me.hasAccess ? me.role : '';
      this.myEmail = me.email || '';
    } catch (e: unknown) {
      console.warn('КП: не удалось получить роль:', errorMessage(e));
      this.myRole = '';
      this.myEmail = '';
      if (e instanceof OffersError && e.status !== 0) {
        this.pageSubEl.setText(e.message);
      }
    }
  }

  private get isAdmin(): boolean {
    return isAdminRole(this.myRole);
  }

  private buildShell(): void {
    const topbar = this.rootEl.createDiv({ cls: 'tn-offers-topbar' });
    topbar.createDiv({ cls: 'tn-offers-module-title', text: 'LogicTEAM.КП' });
    topbar.createDiv({ cls: 'tn-offers-spacer' });
    const reloadBtn = topbar.createEl('button', {
      text: '↻ Обновить права',
      cls: 'tn-btn tn-btn-ghost',
    });
    reloadBtn.addEventListener('click', () => {
      void this.reloadRole();
    });

    const main = this.rootEl.createDiv({ cls: 'tn-offers-main' });
    const sidebar = main.createDiv({ cls: 'tn-offers-sidebar' });
    this.navEl = sidebar.createDiv({ cls: 'tn-offers-nav' });

    const actions = sidebar.createDiv({ cls: 'tn-offers-sidebar-actions' });
    renderHelpButton({
      container: actions,
      app: this.app,
      cls: 'tn-offers-nav-action',
      labelCls: 'tn-offers-nav-lbl',
      help: OFFERS_HELP,
    });

    const content = main.createDiv({ cls: 'tn-offers-content' });
    this.pageTitleEl = content.createEl('h1', { cls: 'tn-offers-page-title' });
    this.pageSubEl = content.createDiv({ cls: 'tn-offers-page-sub' });
    this.bodyEl = content.createDiv({ cls: 'tn-offers-body' });
  }

  private buildNavForRole(): void {
    // Сменилась роль — если текущий раздел теперь недоступен, уходим на «КП».
    if (!this.isAdmin && (this.key === 'form' || this.key === 'access')) {
      this.key = 'offers';
    }
  }

  private buildNav(): void {
    this.navEl.empty();
    const items: Array<{ key: NavKey; icon: string; label: string }> = [
      { key: 'offers', icon: '🧾', label: 'КП' },
      { key: 'catalog', icon: '🗂', label: 'Каталог' },
      { key: 'context', icon: '📚', label: 'Контекст КП' },
    ];
    if (this.isAdmin) {
      items.push({ key: 'form', icon: '⚙️', label: 'Форма КП' });
      items.push({ key: 'access', icon: '👤', label: 'Бот и доступ' });
    }
    for (const item of items) {
      const btn = this.navEl.createEl('button', {
        cls: `tn-offers-nav-item${this.key === item.key ? ' is-active' : ''}`,
      });
      btn.createSpan({ cls: 'tn-offers-nav-icon', text: item.icon });
      btn.createSpan({ cls: 'tn-offers-nav-lbl', text: item.label });
      btn.addEventListener('click', () => {
        this.key = item.key;
        this.renderPage();
      });
    }
  }

  private renderPage(): void {
    this.buildNav();
    this.bodyEl.empty();
    const titles: Record<NavKey, string> = {
      offers: 'Коммерческие предложения',
      catalog: 'Каталог систем и материалов',
      context: 'Контекст КП',
      form: 'Форма КП',
      access: 'Бот и доступ',
    };
    this.pageTitleEl.setText(titles[this.key]);
    switch (this.key) {
      case 'offers':
        this.pageSubEl.setText(this.myEmail ? `Вы: ${this.myEmail}` : '');
        renderOffersList(this, this.bodyEl);
        break;
      case 'catalog':
        this.pageSubEl.setText('Карточки, из которых собираются предложения.');
        renderCatalogEditor(this, this.bodyEl);
        break;
      case 'context':
        this.pageSubEl.setText('Карточки систем из Базы знаний: текст, состав и картинка для КП.');
        renderContextEditor(this, this.bodyEl);
        break;
      case 'form':
        this.pageSubEl.setText('Поля и блоки документа — что увидит клиент.');
        renderFormEditor(this, this.bodyEl);
        break;
      case 'access':
        this.pageSubEl.setText('Кто вошёл в Telegram-бота и с какими правами.');
        void renderAccessEditor(this, this.bodyEl);
        break;
    }
  }

  get role(): Role | '' {
    return this.myRole;
  }

  get email(): string {
    return this.myEmail;
  }

  get admin(): boolean {
    return this.isAdmin;
  }
}
