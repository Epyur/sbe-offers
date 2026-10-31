/**
 * Поиск заметки в вольте по данным карточки Базы знаний (2026-10-30).
 *
 * Живая жалоба: «Открыть заметку» отвечала «Заметка не найдена».
 *
 * Причина: kb-service отдаёт ЛОГИЧЕСКИЙ путь внутри Базы знаний
 * («Инженерная гидроизоляция/Системы/ТН-СТИЛОБАТ ЭКСПЕРТ ТРОТУАР.md»), а файл
 * в вольте лежит совсем в другой папке («База службы/Write it/…»). Открывать
 * файл по этому пути нельзя — он описывает место в базе знаний, не на диске.
 *
 * Поэтому настоящий путь ищется: сначала точное совпадение (на случай, если
 * структура всё-таки совпала), потом по названию заметки среди файлов вольта.
 * Чистая функция: ввод — список путей вольта и логический путь, вывод — путь
 * к открытию или пустая строка.
 */

/** Имя файла из логического пути базы знаний. */
export function noteTitleFromPath(kbPath: string): string {
  const trimmed = (kbPath || '').trim();
  if (!trimmed) return '';
  const last = trimmed.split('/').pop() ?? '';
  return last.replace(/\.md$/i, '').trim();
}

/**
 * Сравнение названий: без регистра, без ё/е, без лишних пробелов и кавычек.
 * Ровно то, чем различаются названия одной заметки в базе знаний и в файле.
 */
export function normalizeNoteTitle(raw: string): string {
  return (raw || '')
    .toLowerCase()
    .replace(/ё/g, 'е')
    .replace(/[«»"'`]/g, '')
    .replace(/[‐‑–—−]/g, '-')
    .replace(/\s+/g, ' ')
    .trim();
}

/**
 * Найти путь файла в вольте.
 *
 * Порядок решений — от самого надёжного к требующему догадки:
 * 1. точное совпадение логического пути (структура совпала) — берём его;
 * 2. единственный файл с тем же названием — берём его;
 * 3. несколько файлов с тем же названием — не угадываем, вернём пустую строку:
 *    открыть не ту заметку хуже, чем честно сказать «не нашли»;
 * 4. ничего не нашли — пустая строка.
 */
export function resolveNotePath(vaultPaths: string[], kbPath: string): string {
  const wanted = (kbPath || '').trim();
  if (!wanted) return '';
  const target = wanted.endsWith('.md') ? wanted : `${wanted}.md`;

  const exact = vaultPaths.find((p) => p === target);
  if (exact) return exact;

  const title = normalizeNoteTitle(noteTitleFromPath(wanted));
  if (!title) return '';

  const matches = vaultPaths.filter((p) => {
    const base = p.split('/').pop() ?? '';
    return normalizeNoteTitle(base.replace(/\.md$/i, '')) === title;
  });
  if (matches.length === 1) return matches[0];

  // Несколько одинаковых названий: предпочитаем тот, что заканчивается
  // логическим путём базы знаний — это та же заметка, просто лежит глубже.
  if (matches.length > 1) {
    const tail = wanted.toLowerCase();
    const byTail = matches.filter((p) => p.toLowerCase().endsWith(tail));
    if (byTail.length === 1) return byTail[0];
  }
  return '';
}

/**
 * Что сказать человеку, если файл не найден. Не повторяем логический путь:
 * он сбивает с толку — человек ищет заметку в этой папке и не находит.
 */
export function noteNotFoundMessage(kbPath: string): string {
  const title = noteTitleFromPath(kbPath);
  if (!title) return 'Заметка не найдена в хранилище.';
  return `Заметка «${title}» не найдена в хранилище Obsidian. Возможно, она ещё не синхронизирована с этой машиной.`;
}