import type { LinkGroupRule } from '../types/offers';

/**
 * Редактор правил группировки ссылок (2026-11-14, дефект 0.10.2, пункт 4).
 *
 * Просьба пользователя: «сделать блок ссылок редактируемым, т.е. в форме
 * настройки КП администратор должен иметь возможность настраивать правила
 * группировки ссылок. Сейчас чтобы поменять способ группировки нужно менять
 * код. Отдавать эту задачу ИИ я не хочу».
 *
 * Правило = группа ссылок в документе: как её узнать (по кускам адреса и/или
 * подписи) и как назвать. Порядок правил — порядок групп в готовом КП.
 * Последняя группа — «остальное»: в неё попадает всё, что не подошло ни одному
 * правилу выше. Без такой группы ссылка пропала бы из документа молча, поэтому
 * последнюю группу нельзя снабдить признаками (это проверяет и сервер).
 *
 * Правила по умолчанию приходят с сервера, поэтому старые формы открываются со
 * знакомым набором групп, а не с пустым списком.
 */
export const LINK_GROUPS_TEXTS = {
  groupTitle: 'Группы ссылок в документе',
  hint:
    'Ссылки печатаются группами в этом порядке. Группа узнаёт ссылку по кусочкам адреса и подписи; ' +
    'последняя группа — «остальное», в неё попадает всё, что не подошло ни одному правилу выше.',
  addRule: '➕ Группа',
  removeRule: '✖',
  moveUp: '▲ Выше',
  moveDown: '▼ Ниже',
  idPlaceholder: 'ключ (латиницей)',
  titlePlaceholder: 'Заголовок группы в документе',
  urlLabel: 'Куски адреса через запятую',
  urlPlaceholder: '/systems/, /catalog/',
  labelLabel: 'Куски подписи через запятую',
  labelPlaceholder: 'техлист, узл',
  lastGroupTitle: 'Последняя группа — «остальное»',
  lastGroupHint: 'Признаков у неё нет и быть не может: сюда попадает всё остальное.',
  emptyHint: 'Правил нет — ссылки печатаются группами по умолчанию.',
};

export interface LinkGroupsEditorDeps {
  /** Правка: редактор формы забирает правила при сохранении сам, здесь — хук. */
  onChange?: () => void;
}

/** Модель правил по умолчанию — копия серверной, чтобы новая группа не путала. */
export function defaultLinkGroupRules(): LinkGroupRule[] {
  return [
    { id: 'page', title: 'Страницы продуктов', urlContains: ['/systems/', '/catalog/', '/catalogue/'] },
    { id: 'sto', title: 'Стандарты организации (СТО)', labelContains: ['сто '] },
    { id: 'techlist', title: 'Технические листы', labelContains: ['техлист', 'тех. лист', 'тех.лист', 'технический лист'] },
    { id: 'uzly', title: 'Узлы', labelContains: ['узл'] },
    { id: 'cert', title: 'Сертификаты и декларации' },
  ];
}

/**
 * Проверка правил на клиенте — те же ограничения, что и на сервере, но до
 * сохранения: администратор должен видеть проблему там, где она возникла.
 * Пусто — правила годны.
 */
export function validateLinkGroupRules(rules: LinkGroupRule[]): string {
  if (rules.length === 0) return '';
  const seen = new Set<string>();
  for (const rule of rules) {
    const id = rule.id.trim();
    if (!id) return 'у группы ссылок пустой ключ';
    if (!rule.title.trim()) return `у группы «${id}» пустой заголовок`;
    if (seen.has(id)) return `группа «${id}» повторяется`;
    seen.add(id);
  }
  const last = rules[rules.length - 1];
  if ((last.urlContains ?? []).some((s) => s.trim()) || (last.labelContains ?? []).some((s) => s.trim())) {
    return 'последняя группа должна быть «остальное»: без признаков, иначе ссылка, не подошедшая правилам, пропадёт';
  }
  return '';
}

/** Строка «куски через запятую» ↔ массив. Пустые куски не сохраняются. */
function splitPieces(value: string): string[] {
  return value
    .split(',')
    .map((s) => s.trim())
    .filter((s) => s !== '');
}

export function renderLinkGroupsEditor(
  container: HTMLElement,
  rules: LinkGroupRule[],
  deps: LinkGroupsEditorDeps = {},
): void {
  container.empty();
  container.createDiv({ cls: 'tn-offers-muted tn-offers-link-groups-hint', text: LINK_GROUPS_TEXTS.hint });

  const list = container.createDiv({ cls: 'tn-offers-link-groups-list' });

  const redraw = (): void => {
    list.empty();
    if (rules.length === 0) {
      list.createDiv({ cls: 'tn-offers-muted', text: LINK_GROUPS_TEXTS.emptyHint });
    }
    rules.forEach((rule, index) => renderRuleCard(list, rules, rule, index, redraw, deps));
  };

  const add = container.createEl('button', { text: LINK_GROUPS_TEXTS.addRule, cls: 'tn-btn tn-btn-ghost' });
  add.addEventListener('click', () => {
    const id = nextRuleID(rules);
    rules.push({ id, title: 'Новая группа' });
    redraw();
    deps.onChange?.();
  });

  redraw();
}

function renderRuleCard(
  list: HTMLElement,
  rules: LinkGroupRule[],
  rule: LinkGroupRule,
  index: number,
  redraw: () => void,
  deps: LinkGroupsEditorDeps,
): void {
  const isLast = index === rules.length - 1;
  const card = list.createDiv({ cls: 'tn-offers-card tn-offers-link-group' });
  const head = card.createDiv({ cls: 'tn-offers-row-between' });
  head.createDiv({ cls: 'tn-offers-muted', text: `Группа ${index + 1}` });
  const tools = head.createDiv({ cls: 'tn-offers-row' });

  const up = tools.createEl('button', { text: LINK_GROUPS_TEXTS.moveUp, cls: 'tn-btn tn-btn-ghost' });
  up.disabled = index === 0;
  up.addEventListener('click', () => {
    [rules[index - 1], rules[index]] = [rules[index], rules[index - 1]];
    redraw();
    deps.onChange?.();
  });

  const down = tools.createEl('button', { text: LINK_GROUPS_TEXTS.moveDown, cls: 'tn-btn tn-btn-ghost' });
  down.disabled = isLast;
  down.addEventListener('click', () => {
    [rules[index + 1], rules[index]] = [rules[index], rules[index + 1]];
    redraw();
    deps.onChange?.();
  });

  const del = tools.createEl('button', {
    text: LINK_GROUPS_TEXTS.removeRule,
    cls: 'tn-btn tn-btn-ghost',
    attr: { 'aria-label': 'Удалить группу' },
  });
  // Последнюю группу удалить нельзя: без неё ссылка, не подошедшая правилам,
  // пропала бы из документа. Это тот же случай, что пустой заголовок, — тихая
  // потеря, которую человек увидит только в готовом файле.
  del.disabled = isLast;
  del.addEventListener('click', () => {
    rules.splice(index, 1);
    redraw();
    deps.onChange?.();
  });

  const row = card.createDiv({ cls: 'tn-offers-row' });
  const idInput = row.createEl('input', { cls: 'tn-offers-input', attr: { placeholder: LINK_GROUPS_TEXTS.idPlaceholder } });
  idInput.value = rule.id;
  idInput.disabled = isLast;
  idInput.addEventListener('change', () => {
    rule.id = idInput.value.trim().replace(/[^a-zA-Z0-9_]/g, '_');
    idInput.value = rule.id;
    deps.onChange?.();
  });

  const titleInput = row.createEl('input', {
    cls: 'tn-offers-input tn-offers-grow',
    attr: { placeholder: LINK_GROUPS_TEXTS.titlePlaceholder },
  });
  titleInput.value = rule.title;
  titleInput.addEventListener('input', () => {
    rule.title = titleInput.value;
    deps.onChange?.();
  });

  if (isLast) {
    card.createDiv({ cls: 'tn-offers-muted', text: LINK_GROUPS_TEXTS.lastGroupHint });
    return;
  }

  const urlRow = card.createDiv({ cls: 'tn-offers-row' });
  urlRow.createDiv({ cls: 'tn-offers-muted tn-offers-link-group-label', text: LINK_GROUPS_TEXTS.urlLabel });
  const urlInput = urlRow.createEl('input', {
    cls: 'tn-offers-input tn-offers-grow',
    attr: { placeholder: LINK_GROUPS_TEXTS.urlPlaceholder },
  });
  urlInput.value = (rule.urlContains ?? []).join(', ');
  urlInput.addEventListener('change', () => {
    rule.urlContains = splitPieces(urlInput.value);
    urlInput.value = (rule.urlContains ?? []).join(', ');
    deps.onChange?.();
  });

  const labelRow = card.createDiv({ cls: 'tn-offers-row' });
  labelRow.createDiv({ cls: 'tn-offers-muted tn-offers-link-group-label', text: LINK_GROUPS_TEXTS.labelLabel });
  const labelInput = labelRow.createEl('input', {
    cls: 'tn-offers-input tn-offers-grow',
    attr: { placeholder: LINK_GROUPS_TEXTS.labelPlaceholder },
  });
  labelInput.value = (rule.labelContains ?? []).join(', ');
  labelInput.addEventListener('change', () => {
    rule.labelContains = splitPieces(labelInput.value);
    labelInput.value = (rule.labelContains ?? []).join(', ');
    deps.onChange?.();
  });
}

/** Свободный ключ для новой группы: `group_1`, `group_2`, … */
function nextRuleID(rules: LinkGroupRule[]): string {
  const used = new Set(rules.map((r) => r.id));
  let n = 1;
  while (used.has(`group_${n}`)) n += 1;
  return `group_${n}`;
}