import { Plugin, WorkspaceLeaf } from 'obsidian';
import { publishService, unpublishService, getService } from '../../sbe-core/src/bridge';
import type { SbeOpenableApi } from '../../sbe-core/src/types';
import { errorMessage } from '../../sbe-core/src/utils/errors';
import { OffersView } from './ui/offers-view';
import { OffersSettingsTab } from './ui/settings-tab';
import { OffersService } from './services/offers.service';
import { DEFAULT_SETTINGS, type OffersSettings } from './types/settings';
import { OFFERS_PLUGIN_ID, OFFERS_VIEW_TYPE } from './constants';

export default class SbeOffersPlugin extends Plugin {
  settings!: OffersSettings;
  offersService!: OffersService;

  async onload(): Promise<void> {
    await this.loadSettings();
    this.offersService = new OffersService(() => this.settings.apiUrl);

    this.registerView(
      OFFERS_VIEW_TYPE,
      (leaf: WorkspaceLeaf) => new OffersView(leaf, this),
    );

    this.addSettingTab(new OffersSettingsTab(this.app, this));

    publishService<SbeOpenableApi>(OFFERS_PLUGIN_ID, {
      open: async () => {
        await this.activateView();
      },
    }, {
      version: this.manifest.version,
      name: this.manifest.name,
    });

    void this.announceOnce();
  }

  onunload(): void {
    unpublishService(OFFERS_PLUGIN_ID);
  }

  async loadSettings(): Promise<void> {
    const data = (await this.loadData() as Partial<OffersSettings>) || {};
    this.settings = Object.assign({}, DEFAULT_SETTINGS, data);
  }

  async saveSettings(): Promise<void> {
    await this.saveData(this.settings);
  }

  async activateView(): Promise<void> {
    const { workspace } = this.app;
    const existing = workspace.getLeavesOfType(OFFERS_VIEW_TYPE)[0];
    if (existing) {
      workspace.revealLeaf(existing);
      return;
    }
    const leaf = workspace.getLeaf(true);
    await leaf.setViewState({ type: OFFERS_VIEW_TYPE, active: true });
    workspace.revealLeaf(leaf);
  }

  /**
   * Новость об обновлении — один раз на версию (правило канала «Новости» ЦУП).
   * Текст описан на понятном русском, без технических терминов.
   */
  private async announceOnce(): Promise<void> {
    if (this.settings.lastAnnouncedVersion === this.manifest.version) return;
    try {
      const apstore = await getService('sbe-apstore');
      await apstore.announceUpdate({
        appId: this.manifest.id,
        appName: this.manifest.name,
        version: this.manifest.version,
        summary:
          'Раздел «Бриф» теперь видно и в приложении, и в настройках формы. ' +
          'В «Форме КП» → «Окно КП в приложении» появилась строка «Бриф» с ' +
          'галочкой «виден» — включите её, и менеджер увидит в карточке КП то, ' +
          'что бот выяснил про объект: «Заполнено N из 14 тем», ответ на ' +
          'открытый вопрос прямо в приложении и кнопку «Продолжить бриф в ' +
          'боте». Список разделов и полей формы теперь разделён заголовками — ' +
          'раньше он шёл сплошным, и новая строка терялась в конце. Бриф ' +
          'встаёт сразу за «Заказчиком», а не под таблицей полей. ' +
          'Настройка больше не теряется: если окно сохраняет старый клиент, ' +
          'включённый раздел остаётся включённым.',
      });
      this.settings.lastAnnouncedVersion = this.manifest.version;
      await this.saveSettings();
    } catch (e: unknown) {
      console.warn('КП: не удалось опубликовать новость:', errorMessage(e));
    }
  }
}
