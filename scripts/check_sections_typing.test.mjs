/*
 * Сторож: ввод заголовка раздела не теряет фокус.
 *
 * Живой дефект 2026-11-15: в «Разделах» заголовок набирался «по одной букве» —
 * обработчик `input` звал `redraw()`, полная перерисовка списка уничтожала
 * поле ввода вместе с фокусом, и каждая следующая буква уходила в никуда.
 * Тот же класс, что `input.value = ''` из 0.1.18: тихая потеря введённого.
 *
 * Проверка не эмуляцией клавиатуры, а по исходнику — и намеренно: Obsidian в
 * тесте недоступен, а вопрос здесь ровно один, «зовёт ли обработчик `input`
 * перерисовку списка». Если ответ «да», фокус умирает всегда, и человек видит
 * ровно один символ. Проверка читает ОБА обработчика `input` в редакторе
 * разделов (заголовок раздела и заголовок подраздела) и требует, чтобы в их
 * телах `redraw()` не встречался.
 *
 * Устойчивость к переделке: обработчик ищется по `addEventListener('input'`,
 * тело берётся до закрывающей скобки на нулевом отступе. Если редактор
 * переделают на делегирование событий, проверка упадёт — и это правильно:
 * тогда правило «input не перерисовывает карточку» надо подтвердить заново,
 * а не унаследовать молча.
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const source = readFileSync(join(here, '..', 'src', 'ui', 'sections-editor.ts'), 'utf8');

/** Тела всех обработчиков `input` в файле. */
function inputHandlerBodies(src) {
  const bodies = [];
  const marker = "addEventListener('input'";
  let from = 0;
  for (;;) {
    const at = src.indexOf(marker, from);
    if (at < 0) break;
    const open = src.indexOf('{', at);
    assert.ok(open > 0, 'обработчик input без тела');
    // Скобочный баланс: тело может содержать вложенные блоки.
    let depth = 0;
    let i = open;
    for (; i < src.length; i += 1) {
      if (src[i] === '{') depth += 1;
      else if (src[i] === '}') {
        depth -= 1;
        if (depth === 0) break;
      }
    }
    bodies.push(src.slice(open, i + 1));
    from = i + 1;
  }
  return bodies;
}

test('обработчики ввода в редакторе разделов не перерисовывают список', () => {
  const bodies = inputHandlerBodies(source);
  assert.ok(bodies.length >= 2, `обработчиков input найдено ${bodies.length}, ожидали хотя бы два`);

  for (const [index, body] of bodies.entries()) {
    // Комментарии вырезаем перед проверкой: упоминание `redraw()` в пояснении
    // — это не вызов, и сторож обязан читать код, а не прозу (иначе он ловил
    // бы собственный комментарий и «зеленел» по неправильной причине).
    const code = body.replace(/\/\/[^\n]*/g, '').replace(/\/\*[\s\S]*?\*\//g, '');
    const redraw = /\bredraw\s*\(\s*\)/.test(code);
    assert.equal(
      redraw,
      false,
      `обработчик input #${index + 1} зовёт redraw(): поле ввода будет уничтожено на первой же букве:\n${code}`,
    );
  }
});

test('заголовок раздела обновляет рамку предпросмотра без перерисовки', () => {
  // Заголовок виден в рамке предпросмотра; раз полную перерисовку убрали,
  // должен существовать точечный путь обновить именно его — иначе человек
  // вводит заголовок и не видит, что он доходит до документа.
  assert.match(source, /function syncFramePreviewTitle\(/, 'нет syncFramePreviewTitle');
  const bodies = inputHandlerBodies(source);
  assert.ok(
    bodies.some((body) => body.includes('syncFramePreviewTitle(')),
    'перерисовка списка убрана, но заголовок рамки не обновляется — заголовок «не доходит» до предпросмотра',
  );
});