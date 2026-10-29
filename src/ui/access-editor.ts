import { Notice } from 'obsidian';
import type { OffersView } from './offers-view';
import type { AccessEntry, BotStatusInfo, Role } from '../types/offers';
import { errorMessage } from '../../../sbe-core/src/utils/errors';

function roleLabel(role: string): string {
  if (role === 'superadmin') return 'Глобальный администратор';
  if (role === 'admin') return 'Администратор';
  if (role === 'editor') return 'Менеджер';
  if (role === 'viewer') return 'Просмотр';
  return '—';
}

function formatMoment(value: string): string {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString('ru-RU');
}

/** Что показать в строке состояния. Отдельно от отрисовки: одна ветка —
 *  одно состояние, проверять глазами проще, чем искать его в обработчике. */
interface BotStateView {
  cls: 'is-ok' | 'is-bad';
  text: string;
  hint: string;
  deletable: boolean;
}

function describeBotState(status: BotStatusInfo): BotStateView {
  if (!status.configured) {
    return {
      cls: 'is-bad',
      text: 'Токен не задан — бот не принимает сообщения.',
      hint: '',
      deletable: false,
    };
  }
  const who = status.botUsername ? `: @${status.botUsername}` : '';
  if (status.running) {
    return { cls: 'is-ok', text: `Бот работает${who}.`, hint: '', deletable: true };
  }
  const reason = status.lastError ? ` · последняя ошибка: ${status.lastError}` : '';
  return {
    cls: 'is-bad',
    text: `Бот не отвечает${reason}`,
    hint: 'Проверьте, запущен ли контейнер бота на сервере.',
    deletable: true,
  };
}

function paintState(view: BotStateView, stateEl: HTMLElement, metaEl: HTMLElement, deleteBtn: HTMLButtonElement): void {
  stateEl.removeClass('is-ok', 'is-bad');
  stateEl.addClass(view.cls);
  stateEl.setText(view.text);
  metaEl.setText(view.hint);
  deleteBtn.toggleClass('is-hidden', !view.deletable);
}

/** Кто и когда сохранял токен. Ошибка не критична — состояние уже показано. */
async function paintBotMeta(offersView: OffersView, metaEl: HTMLElement): Promise<void> {
  try {
    const config = await offersView.plugin.offersService.getBotConfig();
    if (!config.configured) return;
    const who = config.updatedBy ? ` (${config.updatedBy})` : '';
    metaEl.setText(`Сохранён ${formatMoment(config.updatedAt)}${who}`);
  } catch {
    // Метаданные необязательны: не показываем ничего, ничего не ломаем.
  }
}

interface BotCardRefs {
  stateEl: HTMLElement;
  metaEl: HTMLElement;
  deleteBtn: HTMLButtonElement;
}

async function refreshBotCard(view: OffersView, refs: BotCardRefs): Promise<void> {
  let status: BotStatusInfo;
  try {
    status = await view.plugin.offersService.getBotStatus();
  } catch (e: unknown) {
    paintState(
      { cls: 'is-bad', text: `Не удалось узнать состояние бота: ${errorMessage(e)}`, hint: '', deletable: false },
      refs.stateEl,
      refs.metaEl,
      refs.deleteBtn,
    );
    return;
  }
  paintState(describeBotState(status), refs.stateEl, refs.metaEl, refs.deleteBtn);
  if (status.configured) await paintBotMeta(view, refs.metaEl);
}

/**
 * Карточка «Telegram-бот»: единственное место, где вводится токен своего бота.
 * До 2026-10-29 токен жил только в .env на сервере — из плагина его нельзя было
 * ни задать, ни сменить, а при пустом токене бот молча не стартовал и раздел
 * «Бот и доступ» при этом выглядел вполне рабочим.
 */
async function renderBotCard(view: OffersView, container: HTMLElement): Promise<void> {
  const card = container.createDiv({ cls: 'tn-offers-card' });
  card.createDiv({ cls: 'tn-offers-card-title', text: 'Telegram-бот' });
  card.createDiv({
    cls: 'tn-offers-muted',
    text: 'Токен бота, который принимает заказы в Telegram. Выдаётся @BotFather командой /newbot.',
  });

  const stateEl = card.createDiv({ cls: 'tn-offers-bot-state', text: 'Проверка состояния…' });
  const hintEl = card.createDiv({ cls: 'tn-offers-muted' });
  const errorEl = card.createDiv({ cls: 'tn-offers-error' });
  errorEl.hide();

  const row = card.createDiv({ cls: 'tn-offers-row' });
  const tokenInput = row.createEl('input', {
    cls: 'tn-offers-input tn-offers-grow',
    attr: { type: 'password', placeholder: '123456789:AA…', autocomplete: 'off' },
  });
  const saveBtn = row.createEl('button', { text: 'Проверить и сохранить', cls: 'tn-btn tn-btn-primary' });
  const deleteBtn = row.createEl('button', { text: 'Удалить токен', cls: 'tn-btn tn-btn-ghost' });
  const refreshBtn = row.createEl('button', { text: '↻ Проверить сейчас', cls: 'tn-btn tn-btn-ghost' });

  const showError = (text: string): void => {
    errorEl.setText(text);
    errorEl.show();
  };
  const clearError = (): void => {
    errorEl.setText('');
    errorEl.hide();
  };

  const refs: BotCardRefs = { stateEl, metaEl: hintEl, deleteBtn };

  saveBtn.addEventListener('click', () => {
    void (async () => {
      const token = tokenInput.value.trim();
      if (!token) {
        showError('Сначала укажите токен бота');
        return;
      }
      clearError();
      saveBtn.disabled = true;
      saveBtn.setText('Проверяем…');
      try {
        const saved = await view.plugin.offersService.saveBotToken(token);
        tokenInput.value = '';
        new Notice(saved.botUsername ? `Токен сохранён: @${saved.botUsername}` : 'Токен сохранён');
        await refreshBotCard(view, refs);
      } catch (e: unknown) {
        showError(errorMessage(e));
      } finally {
        saveBtn.disabled = false;
        saveBtn.setText('Проверить и сохранить');
      }
    })();
  });

  deleteBtn.addEventListener('click', () => {
    void (async () => {
      deleteBtn.disabled = true;
      try {
        await view.plugin.offersService.deleteBotToken();
        new Notice('Токен удалён — бот больше не принимает сообщения');
        await refreshBotCard(view, refs);
      } catch (e: unknown) {
        showError(errorMessage(e));
      } finally {
        deleteBtn.disabled = false;
      }
    })();
  });

  refreshBtn.addEventListener('click', () => {
    void (async () => {
      refreshBtn.disabled = true;
      try {
        await refreshBotCard(view, refs);
      } finally {
        refreshBtn.disabled = false;
      }
    })();
  });

  await refreshBotCard(view, refs);
}

function renderCodeIssuer(view: OffersView, container: HTMLElement): void {
  const issue = container.createDiv({ cls: 'tn-offers-card' });
  issue.createDiv({ cls: 'tn-offers-muted', text: 'Код входа для бота (действует 7 дней)' });
  const codeRow = issue.createDiv({ cls: 'tn-offers-row' });
  const emailInput = codeRow.createEl('input', {
    cls: 'tn-offers-input tn-offers-grow',
    attr: { placeholder: 'e-mail человека' },
  });
  const issueBtn = codeRow.createEl('button', { text: '🔑 Выдать код', cls: 'tn-btn tn-btn-primary' });
  const codeResult = issue.createDiv({ cls: 'tn-offers-code-result' });

  const showCode = (email: string, code: string, expiresAt: string): void => {
    codeResult.empty();
    codeResult.createSpan({ cls: 'tn-offers-muted', text: `Код для ${email}: ` });
    codeResult.createSpan({ cls: 'tn-offers-code', text: code });
    const copy = codeResult.createEl('button', { text: '⧉', cls: 'tn-btn tn-btn-ghost' });
    copy.addEventListener('click', () => {
      void navigator.clipboard.writeText(code);
      new Notice('Код скопирован');
    });
    codeResult.createDiv({
      cls: 'tn-offers-muted',
      text: `Действует до ${new Date(expiresAt).toLocaleString('ru-RU')}`,
    });
  };

  issueBtn.addEventListener('click', () => {
    void (async () => {
      const email = emailInput.value.trim();
      if (!email) {
        new Notice('Укажите e-mail');
        return;
      }
      try {
        const res = await view.plugin.offersService.issueCode(email);
        showCode(email, res.code, res.expiresAt);
      } catch (e: unknown) {
        new Notice(`Не удалось выдать код: ${errorMessage(e)}`);
      }
    })();
  });
}

async function renderSignedInList(view: OffersView, container: HTMLElement): Promise<void> {
  const listCard = container.createDiv({ cls: 'tn-offers-card' });
  listCard.createDiv({ cls: 'tn-offers-muted', text: 'Кто вошёл в бота' });

  let entries: AccessEntry[] = [];
  try {
    entries = await view.plugin.offersService.listAccess();
  } catch (e: unknown) {
    listCard.createDiv({ cls: 'tn-offers-error', text: `Не удалось загрузить список: ${errorMessage(e)}` });
    return;
  }
  if (entries.length === 0) {
    listCard.createDiv({ cls: 'tn-offers-empty', text: 'Пока никто не входил.' });
    return;
  }

  const table = listCard.createEl('table', { cls: 'tn-offers-table' });
  const head = table.createEl('thead').createEl('tr');
  for (const title of ['E-mail', 'Роль', 'Вошёл']) head.createEl('th', { text: title });
  const body = table.createEl('tbody');
  for (const entry of entries) {
    const row = body.createEl('tr');
    row.createEl('td', { text: entry.email });
    const roleCell = row.createEl('td');
    roleCell.appendChild(renderRoleSelect(view, entry));
    row.createEl('td', {
      cls: 'tn-offers-muted',
      text: entry.updatedAt ? new Date(entry.updatedAt).toLocaleString('ru-RU') : '—',
    });
  }
}

/** Выбор роли человека в боте. superadmin приходит из центральной таблицы прав,
 *  а не из карты бота: показываем отдельным пунктом только для показа. */
function renderRoleSelect(view: OffersView, entry: AccessEntry): HTMLSelectElement {
  const select = document.createElement('select');
  select.addClass('tn-offers-input');
  for (const role of ['viewer', 'editor', 'admin'] as Role[]) {
    select.createEl('option', { value: role, text: roleLabel(role) });
  }
  const entryRole: string = entry.role;
  if (entryRole === 'superadmin') {
    select.createEl('option', { value: 'superadmin', text: roleLabel('superadmin') });
  }
  select.value = entryRole || 'viewer';
  select.addEventListener('change', () => {
    void (async () => {
      try {
        await view.plugin.offersService.setRole(entry.email, select.value as Role);
        new Notice(`Роль ${entry.email}: ${roleLabel(select.value)}`);
      } catch (e: unknown) {
        new Notice(`Не удалось изменить роль: ${errorMessage(e)}`);
      }
    })();
  });
  return select;
}

export async function renderAccessEditor(view: OffersView, container: HTMLElement): Promise<void> {
  container.empty();
  container.createDiv({ cls: 'tn-offers-muted', text: 'Загрузка…' });

  // Токен бота — первым блоком: без него всё остальное в этом разделе
  // бессмысленно (коды выдаются, а отправить их некому).
await renderBotCard(view, container);
  renderCodeIssuer(view, container);
  await renderSignedInList(view, container);
}