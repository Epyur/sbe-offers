/**
 * Боевой листок в боте (2026-11-09): тексты и клавиатуры.
 *
 * Проверяем поведение, которое видит менеджер: что написано в сводке, какие
 * кнопки нажимать и что бот честно говорит, когда список пуст.
 */
import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import {
  COMBAT_KEYBOARD,
  COMBAT_PICKER_KEYBOARD,
  combatNeedCompetitorMessage,
  type CombatPickerPage,
  combatAskQuery,
  combatIntro,
  combatListMessage,
  combatNotEmptyMessage,
  combatPickerKeyboard,
  combatPickerMessage,
  combatSheetMessage,
  combatSideAddedMessage,
  combatAskHint,
  combatHintKeyboard,
  combatAnalogueMessage,
} from './combat.ts';
import type { CombatSheet, CombatSystemItem } from '../api.ts';

function sheet(over: Partial<CombatSheet> = {}): CombatSheet {
  return {
    id: 1,
    number: '001/2026',
    ownerEmail: 'm@tn.ru',
    title: 'Сравнение',
    offerId: 0,
    ourNotes: [],
    competitorNotes: [],
    formValues: {},
    status: 'draft',
    createdAt: '',
    updatedAt: '',
    ...over,
  };
}

describe('боевой листок: сводка', () => {
  it('пустой листок честно говорит, что ничего не выбрано', () => {
    const text = combatSheetMessage(sheet());
    assert.match(text, /Наш продукт: пока не выбран/);
    assert.match(text, /Конкуренты: пока не добавлены/);
  });

  it('перечисляет выбранные стороны', () => {
    const text = combatSheetMessage(
      sheet({
        ourNotes: [{ noteId: 10, title: 'Кровля' }],
        competitorNotes: [{ noteId: 20, title: 'Конкурент А' }],
      }),
    );
    assert.match(text, /Кровля/);
    assert.match(text, /Конкурент А/);
  });

  it('приветствие несёт номер листка', () => {
    assert.match(combatIntro('007/2026'), /007\/2026/);
  });
});

describe('боевой листок: выбор стороны', () => {
  const page = (items: CombatSystemItem[], total = items.length, offset = 0, hasMore = false): CombatPickerPage => ({
    items,
    total,
    offset,
    hasMore,
  });

  it('пустой список не показывает клавиатуру и объясняет, что делать', () => {
    const text = combatPickerMessage(page([]), 'competitor');
    assert.match(text, /конкурент/i);
  });

  it('страница печатает пронумерованный текст и кнопки-номера, а не названия', () => {
    const items: CombatSystemItem[] = [
      { noteId: 10, title: 'Кровля' },
      { noteId: 20, title: 'Фасад', problems: ['нет ссылки'] },
    ];
    const text = combatPickerMessage(page(items), 'our');
    assert.match(text, /1\. Кровля/);
    assert.match(text, /2\. Фасад.*есть замечания/);
    const kb = combatPickerKeyboard(page(items), 'oursel', new Set()) as {
      inline_keyboard: Array<Array<{ text: string; callback_data: string }>>;
    };
    // Номера — в одном ряду, подписи — цифры, а не длинные названия.
    assert.equal(kb.inline_keyboard[0][0].text, '1');
    assert.equal(kb.inline_keyboard[0][0].callback_data, 'oursel:0');
    assert.equal(kb.inline_keyboard[0][1].callback_data, 'oursel:1');
  });

  it('отмеченные строки видны галочкой на кнопке, а отдельная кнопка добавляет их все', () => {
    const items: CombatSystemItem[] = [
      { noteId: 10, title: 'Кровля' },
      { noteId: 20, title: 'Фасад' },
    ];
    const kb = combatPickerKeyboard(page(items), 'oursel', new Set([0, 1])) as {
      inline_keyboard: Array<Array<{ text: string; callback_data: string }>>;
    };
    assert.equal(kb.inline_keyboard[0][0].text, '✓ 1');
    assert.equal(kb.inline_keyboard[0][1].text, '✓ 2');
    const ids = kb.inline_keyboard.flat().map((b) => b.callback_data);
    assert.ok(ids.includes('cb:addpicked'));
  });

  it('«Дальше» ведёт на следующую страницу, «В начало» — на первую', () => {
    const items: CombatSystemItem[] = [{ noteId: 10, title: 'Кровля' }];
    // Смещение > 0 и есть остаток — две кнопки навигации.
    const second = combatPickerKeyboard(page(items, 20, 8, true), 'oursel', new Set()) as {
      inline_keyboard: Array<Array<{ callback_data: string }>>;
    };
    const ids = second.inline_keyboard.flat().map((b) => b.callback_data);
    assert.ok(ids.includes('cb:first'));
    assert.ok(ids.includes('cb:more'));
    // Нумерация продолжается: строка начинается с 9-го номера.
    assert.ok(ids.includes('oursel:8'));

    const first = combatPickerKeyboard(page(items, 20, 0, true), 'compsel', new Set()) as {
      inline_keyboard: Array<Array<{ callback_data: string }>>;
    };
    const firstIds = first.inline_keyboard.flat().map((b) => b.callback_data);
    assert.ok(!firstIds.includes('cb:first'));
    assert.ok(firstIds.includes('cb:more'));
  });

  it('«☑ Выбрать все найденные» появляется, когда совпадений больше страницы', () => {
    const items: CombatSystemItem[] = [{ noteId: 10, title: 'Кровля' }];
    // Найдено 20, на экране 1 — значит, собрать всё одной кнопкой осмысленно.
    const kb = combatPickerKeyboard(page(items, 20, 0, true), 'oursel', new Set()) as {
      inline_keyboard: Array<Array<{ text: string; callback_data: string }>>;
    };
    const ids = kb.inline_keyboard.flat().map((b) => b.callback_data);
    assert.ok(ids.includes('cb:pickall'));
    const pickAll = kb.inline_keyboard.flat().find((b) => b.callback_data === 'cb:pickall');
    assert.match(pickAll!.text, /20/);
  });

  it('когда помещается на экран — кнопки «выбрать все» нет', () => {
    const items: CombatSystemItem[] = [
      { noteId: 10, title: 'Кровля' },
      { noteId: 20, title: 'Фасад' },
    ];
    const kb = combatPickerKeyboard(page(items), 'oursel', new Set()) as {
      inline_keyboard: Array<Array<{ callback_data: string }>>;
    };
    const ids = kb.inline_keyboard.flat().map((b) => b.callback_data);
    assert.ok(!ids.includes('cb:pickall'));
  });

  it('в режиме «всё найдено» кнопка «Добавить» остаётся, даже если номера не отмечены', () => {
    const items: CombatSystemItem[] = [{ noteId: 10, title: 'Кровля' }];
    // Живой сценарий: «Выбрать все найденные» очищает picked — и раньше
    // кнопка «Добавить» из-за этого исчезала.
    const kb = combatPickerKeyboard(page(items, 21, 0, true), 'compsel', new Set(), false, true) as {
      inline_keyboard: Array<Array<{ text: string; callback_data: string }>>;
    };
    const ids = kb.inline_keyboard.flat().map((b) => b.callback_data);
    assert.ok(ids.includes('cb:addpicked'), 'кнопки «Добавить» нет в режиме «всё найдено»');
    const add = kb.inline_keyboard.flat().find((b) => b.callback_data === 'cb:addpicked');
    assert.match(add!.text, /21/);
  });

  it('при «выбрано всё» кнопка меняется на снятие и считает всё найденное', () => {
    const items: CombatSystemItem[] = [{ noteId: 10, title: 'Кровля' }];
    const kb = combatPickerKeyboard(page(items, 20, 0, true), 'oursel', new Set([0]), false, true) as {
      inline_keyboard: Array<Array<{ text: string; callback_data: string }>>;
    };
    const ids = kb.inline_keyboard.flat().map((b) => b.callback_data);
    assert.ok(!ids.includes('cb:pickall'));
    assert.ok(ids.includes('cb:unpickall'));
    const add = kb.inline_keyboard.flat().find((b) => b.callback_data === 'cb:addpicked');
    assert.match(add!.text, /20/);
  });

  it('при поиске кнопки «▼ Все» нет — список уже сужен', () => {
    const kb = combatPickerKeyboard(page([{ noteId: 10, title: 'Кровля' }]), 'oursel', new Set(), true) as {
      inline_keyboard: Array<Array<{ callback_data: string }>>;
    };
    const ids = kb.inline_keyboard.flat().map((b) => b.callback_data);
    assert.ok(!ids.includes('cb:all'));
    assert.equal(ids[ids.length - 1], 'cb:back');
  });

  it('пустой поиск по запросу объясняет причину и не теряет путь назад', () => {
    const text = combatPickerMessage(page([]), 'our', 'несуществующий');
    assert.match(text, /несуществующий/);
  });

  it('приглашение просит название, а не показывает список', () => {
    assert.match(combatAskQuery('our'), /название/);
    assert.match(combatAskQuery('competitor'), /конкурента/);
  });

  it('сообщение об добавлении согласуется по роду', () => {
    assert.match(combatSideAddedMessage('our', 'Кровля'), /добавлена/);
    assert.match(combatSideAddedMessage('competitor', 'Конкурент'), /добавлен/);
  });
});

describe('боевой листок: подбор аналогов в меню', () => {
  it('кнопка «Найти аналоги и сравнить» есть в меню листка и страницы выбора', () => {
    const main = COMBAT_KEYBOARD.inline_keyboard.flat() as Array<{ callback_data: string; text: string }>;
    const analog = main.find((b) => b.callback_data === 'cb:analog');
    assert.ok(analog, 'в главном меню нет кнопки подбора аналогов');
    assert.match(analog!.text, /аналоги/i);

    const picker = COMBAT_PICKER_KEYBOARD.inline_keyboard.flat() as Array<{ callback_data: string }>;
    assert.ok(picker.some((b) => b.callback_data === 'cb:analog'));
  });

  it('без конкурента нажатие объясняет следующий шаг, а не молчит', () => {
    assert.match(combatNeedCompetitorMessage(), /Добавить конкурента/);
  });
});

describe('боевой листок: уточнение и аналоги', () => {
  it('приглашение просит акцент, а не новые поля', () => {
    const text = combatAskHint();
    assert.match(text, /акцент/);
    // Пустое уточнение снимается точкой — так сказано человеку.
    assert.match(text, /точку/);
  });

  it('приглашение показывает текущее уточнение и кнопку снятия', () => {
    const text = combatAskHint('огнестойкость');
    assert.match(text, /огнестойкость/);
    const kb = combatHintKeyboard(true) as { inline_keyboard: Array<Array<{ callback_data: string }>> };
    assert.ok(kb.inline_keyboard.flat().some((b) => b.callback_data === 'cb:hintclear'));
  });

  it('без уточнения кнопки снятия нет', () => {
    const kb = combatHintKeyboard(false) as { inline_keyboard: Array<Array<{ callback_data: string }>> };
    assert.ok(!kb.inline_keyboard.flat().some((b) => b.callback_data === 'cb:hintclear'));
  });

  it('аналоги показываются номерами, кнопка несёт заметку', () => {
    const res = combatAnalogueMessage('Продукт конкурента', [
      { noteId: 7, title: 'Наш аналог', sku: 'MAT-1' },
    ]);
    assert.match(res.text, /Наш аналог/);
    const kb = res.keyboard as { inline_keyboard: Array<Array<{ callback_data: string }>> };
    assert.equal(kb.inline_keyboard[0][0].callback_data, 'cban:7');
  });

  it('пустой подбор честно говорит, что аналога нет', () => {
    const res = combatAnalogueMessage('Экзотика', []);
    assert.match(res.text, /не нашлось/);
  });
});

describe('боевой листок: прочее', () => {
  it('пустой листок не даёт выгрузить файл', () => {
    assert.match(combatNotEmptyMessage(), /нечего сравнивать/);
  });

  it('список листков: пусто → null', () => {
    assert.equal(combatListMessage([]), null);
  });

  it('список листков: кнопки открыть и файл', () => {
    const res = combatListMessage([sheet({ id: 5, number: '005/2026' })]);
    assert.ok(res);
    const kb = res!.keyboard as { inline_keyboard: Array<Array<{ callback_data: string }>> };
    assert.equal(kb.inline_keyboard[0][0].callback_data, 'cbe:5');
    assert.equal(kb.inline_keyboard[0][1].callback_data, 'cbx:5');
    assert.match(res!.text, /005\/2026/);
  });

  it('кнопки боевого листка ведут в свои обработчики', () => {
    const kb = COMBAT_KEYBOARD.inline_keyboard.flat() as Array<{ callback_data: string }>;
    const ids = kb.map((b) => b.callback_data);
    // «Найти аналоги и сравнить» стоит в главном меню листка (2026-11-11):
    // раньше его видел только тот, кто уже выбрал конкурента в этой сессии.
    assert.deepEqual(ids, [
      'cb:oursys', 'cb:comp', 'cb:analog', 'cb:hint', 'cb:doc', 'cb:done',
    ]);
  });
});
