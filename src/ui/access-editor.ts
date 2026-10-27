import { Notice } from 'obsidian';
import type { OffersView } from './offers-view';
import type { AccessEntry, Role } from '../types/offers';
import { errorMessage } from '../../../sbe-core/src/utils/errors';

function roleLabel(role: string): string {
  if (role === 'superadmin') return 'Глобальный администратор';
  if (role === 'admin') return 'Администратор';
  if (role === 'editor') return 'Менеджер';
  if (role === 'viewer') return 'Просмотр';
  return '—';
}

export async function renderAccessEditor(view: OffersView, container: HTMLElement): Promise<void> {
  container.empty();
  container.createDiv({ cls: 'tn-offers-muted', text: 'Загрузка…' });

  const issue = container.createDiv({ cls: 'tn-offers-card' });
  issue.createDiv({ cls: 'tn-offers-muted', text: 'Код входа для бота (действует 7 дней)' });
  const codeRow = issue.createDiv({ cls: 'tn-offers-row' });
  const emailInput = codeRow.createEl('input', {
    cls: 'tn-offers-input tn-offers-grow',
    attr: { placeholder: 'e-mail человека' },
  });
  const issueBtn = codeRow.createEl('button', { text: '🔑 Выдать код', cls: 'tn-btn tn-btn-primary' });
  const codeResult = issue.createDiv({ cls: 'tn-offers-code-result' });
  issueBtn.addEventListener('click', () => {
    void (async () => {
      const email = emailInput.value.trim();
      if (!email) {
        new Notice('Укажите e-mail');
        return;
      }
      try {
        const res = await view.plugin.offersService.issueCode(email);
        codeResult.empty();
        codeResult.createSpan({ cls: 'tn-offers-muted', text: `Код для ${email}: ` });
        const code = codeResult.createSpan({ cls: 'tn-offers-code', text: res.code });
        const copy = codeResult.createEl('button', { text: '⧉', cls: 'tn-btn tn-btn-ghost' });
        copy.addEventListener('click', () => {
          void navigator.clipboard.writeText(res.code);
          new Notice('Код скопирован');
        });
        codeResult.createDiv({ cls: 'tn-offers-muted', text: `Действует до ${new Date(res.expiresAt).toLocaleString('ru-RU')}` });
      } catch (e: unknown) {
        new Notice(`Не удалось выдать код: ${errorMessage(e)}`);
      }
    })();
  });

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
    const select = roleCell.createEl('select', { cls: 'tn-offers-input' });
    for (const role of ['viewer', 'editor', 'admin'] as Role[]) {
      select.createEl('option', { value: role, text: roleLabel(role) });
    }
    // superadmin приходит из центральной таблицы прав, а не из карты бота:
    // показываем отдельным пунктом только для показа.
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
    row.createEl('td', {
      cls: 'tn-offers-muted',
      text: entry.updatedAt ? new Date(entry.updatedAt).toLocaleString('ru-RU') : '—',
    });
  }
}