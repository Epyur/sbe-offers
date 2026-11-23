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

    // Идемпотентная регистрация (2026-11-17). Живой дефект при перезагрузке
    // плагина (HotReload): `onload` звался повторно в том же процессе при
    // уже существующей регистрации, и Obsidian бросал
    // «Attempting to register an existing view type "sbe-offers-view"» —
    // плагин не поднимался. Причина — HotReload перезагружает плагин без
    // полного `onunload` прежнего экземпляра. Проглатываем имено этот случай:
    // повторную регистрацию того же типа второй раз не делаем. Любую другую
    // ошибку проброса не маскируем — она должна быть видна.
    try {
      this.registerView(
        OFFERS_VIEW_TYPE,
        (leaf: WorkspaceLeaf) => new OffersView(leaf, this),
      );
    } catch (e: unknown) {
      if (!/already.*registered|existing view type/i.test(errorMessage(e))) {
        throw e;
      }
      console.warn('КП: вид уже зарегистрирован — повторную регистрацию пропускаю:', errorMessage(e));
    }

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
    // Листья вида надо снять при выгрузке (2026-11-17). Без этого регистрация
    // типа и листья остаются на месте, и повторный `onload` при перезагрузке
    // плагина падал на «view type already registered». Снимаем листья СВОЕГО
    // вида — чужие трогать нельзя; сам тип плагин отдаёт Obsidian штатно.
    this.app.workspace.detachLeavesOfType(OFFERS_VIEW_TYPE);
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
          '«Технодайджест» теперь выглядит как фирменный выпуск: красная марка, ' +
          'шапка с месяцем, ведущая новость с картинкой, остальные — карточками, ' +
          'внизу подвал технической дирекции. Картинки новостей показываются в ' +
          'приложении, а «Отправить в чат» присылает новость фотографией с подписью ' +
          'и следом — полный текст и источник.',
      });
      this.settings.lastAnnouncedVersion = this.manifest.version;
      await this.saveSettings();
    } catch (e: unknown) {
      console.warn('КП: не удалось опубликовать новость:', errorMessage(e));
    }
  }
}
