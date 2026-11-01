import { FuzzySuggestModal, Notice, requestUrl, TFile } from 'obsidian';
import { getService } from '../../../sbe-core/src/bridge';
import { errorMessage } from '../../../sbe-core/src/utils/errors';
import { arrayBufferToBase64, mimeForName, sha256Hex } from '../../../sbe-core/src/utils/files';
import type { OffersView } from './offers-view';

/**
 * Выбор картинки для блока формы КП (2026-11-01).
 *
 * Изображение берётся из вольта: техдокументация и фото продуктов и так лежат в
 * заметках. Файл заливается во вложения Базы знаний — там уже есть хранилище и
 * раздача, второй не нужен, — а в блоке остаётся ключ. Документ собирает
 * сервис, и ему нужны байты по ссылке, а не base64 в JSON формы.
 *
 * Контракт загрузки — как у самой Базы знаний: POST /api/kb/attachments
 * {content_hash, filename, mime, data_base64} → {key}. Key берём из ответа, а
 * не собираем по хэшу: формат ключа — дело хранилища.
 */

export interface PickedImage {
  key: string;
  name: string;
}

/** Модалка выбора картинки вольта: подсказывает полный путь файла. */
class ImageSuggestModal extends FuzzySuggestModal<TFile> {
  private readonly onPick: (file: TFile) => void;

  constructor(app: OffersView['app'], readonly files: TFile[], onPick: (file: TFile) => void) {
    super(app);
    this.onPick = onPick;
    this.setPlaceholder('Начните вводить имя картинки…');
  }

  getItems(): TFile[] {
    return this.files;
  }

  getItemText(file: TFile): string {
    return file.path;
  }

  onChooseItem(file: TFile): void {
    this.onPick(file);
  }
}

/** Диалог выбора файла вольта — только картинки. */
export function pickImageFile(view: OffersView): Promise<TFile | null> {
  return new Promise((resolve) => {
    const files = view.app.vault
      .getFiles()
      .filter((f) => /\.(png|jpe?g|gif|webp)$/i.test(f.path))
      .sort((a, b) => a.path.localeCompare(b.path, 'ru'));
    if (files.length === 0) {
      new Notice('В вольте нет картинок — добавьте файл png или jpg.');
      resolve(null);
      return;
    }
    new ImageSuggestModal(view.app, files, (file) => resolve(file)).open();
  });
}

/**
 * Залить картинку во вложения Базы знаний и вернуть ключ.
 *
 * Бросает понятную ошибку: без Базы знаний картинка в блок не попадёт, и об
 * этом надо сказать человеку, а не проглотить молча.
 */
export async function uploadImageToKb(view: OffersView, file: TFile): Promise<PickedImage> {
  const data = await view.app.vault.readBinary(file);
  const hash = await sha256Hex(data);
  const apstore = await getService('sbe-apstore');
  const token = await apstore.auth.getToken('kb');
  const base = view.plugin.settings.apiUrl.replace(/\/+$/, '');
  const res = await requestUrl({
    url: `${base}/api/kb/attachments`,
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({
      content_hash: hash,
      filename: file.name,
      mime: mimeForName(file.name),
      data_base64: arrayBufferToBase64(data),
    }),
    throw: false,
  });
  if (res.status < 200 || res.status >= 300) {
    throw new Error(kbErrorText(res.status, res.text));
  }
  const parsed = JSON.parse(res.text) as { key?: string };
  if (!parsed.key) {
    throw new Error('База знаний не вернула ключ файла');
  }
  return { key: parsed.key, name: file.name };
}

/** Ответ Базы знаний об ошибке — для человека, а не код статуса. */
function kbErrorText(status: number, text: string): string {
  try {
    const parsed = JSON.parse(text) as { error?: string };
    if (parsed.error) return parsed.error;
  } catch (e: unknown) {
    console.warn('КП: ответ Базы знаний не JSON:', errorMessage(e));
  }
  return `База знаний не приняла файл (${status})`;
}

/** Выбор и загрузка одним действием — то, что ждёт от кнопки администратор. */
export async function pickAndUploadImage(view: OffersView): Promise<PickedImage | null> {
  const file = await pickImageFile(view);
  if (!file) return null;
  try {
    return await uploadImageToKb(view, file);
  } catch (e: unknown) {
    new Notice(`Не удалось загрузить картинку: ${errorMessage(e)}`);
    return null;
  }
}