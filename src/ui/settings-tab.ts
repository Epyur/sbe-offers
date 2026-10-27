import { App, PluginSettingTab, Setting } from 'obsidian';
import type SbeOffersPlugin from '../main';

export class OffersSettingsTab extends PluginSettingTab {
  plugin: SbeOffersPlugin;

  constructor(app: App, plugin: SbeOffersPlugin) {
    super(app, plugin);
    this.plugin = plugin;
  }

  display(): void {
    const { containerEl } = this;
    containerEl.empty();

    new Setting(containerEl)
      .setName('Адрес сервера')
      .setDesc('Адрес offers-service, откуда плагин берёт каталог, форму и предложения.')
      .addText((text) =>
        text
          .setPlaceholder('https://epyur.fvds.ru')
          .setValue(this.plugin.settings.apiUrl)
          .onChange(async (value) => {
            this.plugin.settings.apiUrl = value.trim() || 'https://epyur.fvds.ru';
            await this.plugin.saveSettings();
          }),
      );
  }
}
