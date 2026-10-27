export interface OffersSettings {
  apiUrl: string;
  /** Версия, для которой уже опубликована новость в «Новости» ЦУП (однократно на версию). */
  lastAnnouncedVersion: string;
}

export const DEFAULT_SETTINGS: OffersSettings = {
  apiUrl: 'https://epyur.fvds.ru',
  lastAnnouncedVersion: '',
};
