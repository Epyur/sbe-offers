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
  combatAskQuery,
  combatIntro,
  combatListMessage,
  combatNotEmptyMessage,
  combatPickerKeyboard,
  combatPickerMessage,
  combatSheetMessage,
  combatSideAddedMessage,
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
  it('пустой список не показывает клавиатуру и объясняет, что делать', () => {
    const res = combatPickerMessage([], 'competitor');
    assert.equal(res.show, false);
    assert.match(res.text, /конкурент/i);
  });

  it('непустой список показывает кнопки и несёт индекс', () => {
    const items: CombatSystemItem[] = [
      { noteId: 10, title: 'Кровля' },
      { noteId: 20, title: 'Фасад', problems: ['нет ссылки'] },
    ];
    const res = combatPickerMessage(items, 'our');
    assert.equal(res.show, true);
    const kb = combatPickerKeyboard(items, 'oursel') as { inline_keyboard: Array<Array<{ callback_data: string }>> };
    assert.equal(kb.inline_keyboard[0][0].callback_data, 'oursel:0');
    assert.equal(kb.inline_keyboard[1][0].callback_data, 'oursel:1');
    // «Назад» — последняя строка, перед ней «▼ Все продукты».
    assert.equal(kb.inline_keyboard[kb.inline_keyboard.length - 1][0].callback_data, 'cb:back');
    assert.equal(kb.inline_keyboard[kb.inline_keyboard.length - 2][0].callback_data, 'cb:all');
  });

  it('при поиске кнопки «▼ Все» нет — список уже сужен', () => {
    const items: CombatSystemItem[] = [{ noteId: 10, title: 'Кровля' }];
    const kb = combatPickerKeyboard(items, 'oursel', true) as {
      inline_keyboard: Array<Array<{ callback_data: string }>>;
    };
    const ids = kb.inline_keyboard.flat().map((b) => b.callback_data);
    assert.ok(!ids.includes('cb:all'));
    assert.equal(ids[ids.length - 1], 'cb:back');
  });

  it('замечания к карточке видны в подписи кнопки', () => {
    const items: CombatSystemItem[] = [{ noteId: 20, title: 'Фасад', problems: ['нет ссылки'] }];
    const kb = combatPickerKeyboard(items, 'compsel') as { inline_keyboard: Array<Array<{ text: string }>> };
    assert.match(kb.inline_keyboard[0][0].text, /есть замечания/);
  });

  it('пустой поиск по запросу объясняет причину и не теряет путь назад', () => {
    const res = combatPickerMessage([], 'our', 'несуществующий');
    assert.equal(res.show, false);
    assert.match(res.text, /несуществующий/);
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
    assert.deepEqual(ids, ['cb:oursys', 'cb:comp', 'cb:doc', 'cb:done']);
  });
});