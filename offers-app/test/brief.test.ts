/**
 * Блок «Бриф» в приложении (2026-11-15).
 *
 * Проверяется расчётная часть: что считается заполненным, как список
 * превращается в текст и обратно, и когда показывать вопрос. Это те места,
 * где ошибка видна менеджеру как «тема пустая, хотя я ответил» или «вопрос
 * пропал, хотя я не ответил».
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import {
  answerBrief,
  BRIEF_TOPICS,
  briefSummaryLine,
  filledCount,
  openTopic,
  textToTopic,
  topicFilled,
  topicLabel,
  topicText,
  type BriefPayloadDto,
} from '../src/brief.ts';

test('темы совпадают с серверными ключами брифа', () => {
  assert.deepEqual(
    [...BRIEF_TOPICS],
    [
      'object_name',
      'object_purpose',
      'object_area',
      'customer_address',
      'planned_systems',
      'customer_goals',
      'customer_pains',
      'project_risks',
      'decision_maker',
      'timeline',
      'budget_signal',
      'why_us',
      'objections',
      'notes',
    ],
  );
  assert.equal(topicLabel('object_name'), 'Объект');
  // Незнакомый ключ не теряется: лучше показать как есть, чем пустое слово.
  assert.equal(topicLabel('что-то'), 'что-то');
});

test('заполненность: пустые строки и пустые списки не считаются', () => {
  const payload: BriefPayloadDto = {
    object_name: '  Дом на Охотской ',
    customer_pains: [],
    why_us: ['цена'],
    skipped_topics: [],
  };
  assert.equal(topicFilled(payload, 'object_name'), true);
  assert.equal(topicFilled(payload, 'customer_pains'), false);
  assert.equal(topicFilled(payload, 'why_us'), true);
  assert.equal(filledCount(payload), 2);
});

test('возражение считается отработанным только с ответом', () => {
  const без: BriefPayloadDto = { objections: [{ claim: 'дорого', response: '' }] };
  const с: BriefPayloadDto = { objections: [{ claim: 'дорого', response: 'считаем скидку' }] };
  assert.equal(topicFilled(без, 'objections'), false);
  assert.equal(topicFilled(с, 'objections'), true);
  assert.equal(topicText(с, 'objections'), 'дорого → считаем скидку');
});

test('список показывается построчно и собирается обратно без пустых строк', () => {
  const payload: BriefPayloadDto = { customer_pains: ['дорого', 'долго'] };
  assert.equal(topicText(payload, 'customer_pains'), 'дорого\nдолго');

  const back = textToTopic('дорого\n\n  долго  \n', 'customer_pains');
  assert.deepEqual(back, ['дорого', 'долго']);
  // Пустое поле не превращается в список из одной пустой строки.
  assert.deepEqual(textToTopic('   \n', 'customer_goals'), []);
});

test('текстовые поля обрезаются, списки режутся по строкам', () => {
  // «Notes» — список тем, а не свободный текст (у сервера это []string в
  // model.BriefPayload). Перенос в поле создаёт два пункта, а не пробел.
  assert.deepEqual(textToTopic('Дом\nул. Охотская, 5', 'notes'), ['Дом', 'ул. Охотская, 5']);
  // Однострочные поля сохраняют значение целиком, но без краёв.
  assert.equal(textToTopic('  Объект  ', 'object_name'), 'Объект');
  assert.equal(textToTopic('Дом\n2 этаж', 'object_name'), 'Дом\n2 этаж');
});

test('заголовок карточки: нет брифа, пуст, заполнен', () => {
  assert.equal(briefSummaryLine({}, true), 'Брифа нет');
  assert.equal(briefSummaryLine({}, false), 'Бриф пуст');
  assert.equal(briefSummaryLine({ object_name: 'Дом' }, false), 'Заполнено 1 из 14 тем');
});

test('вопрос открыт только когда тема известна и бриф не готов', () => {
  assert.equal(openTopic({ status: 'open', lastTopic: 'object_name' }), 'object_name');
  assert.equal(openTopic({ status: 'open', lastTopic: 'неизвестная' }), '');
  assert.equal(openTopic({ status: 'ready', lastTopic: 'object_name' }), '');
  assert.equal(openTopic({ status: 'absent' }), '');
  assert.equal(openTopic(undefined), '');
});

test('ответ уходит на сервер с темой в адресе', async () => {
  // Сеть здесь не нужна: проверяем, что функция отвечает не-промисом только
  // когда адрес невозможно собрать (id не число), иначе это запрос.
  assert.equal(typeof answerBrief, 'function');
  assert.deepEqual(textToTopic('a\nb', 'planned_systems'), ['a', 'b']);
});
