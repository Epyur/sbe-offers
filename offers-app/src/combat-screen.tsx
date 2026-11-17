/**
 * Экран боевого листка (2026-11-13, шаг S6; источники конкурента — 2026-11-17).
 *
 * Состав: номер и кто автор → наши продукты → продукты конкурентов →
 * уточнение для разбора. Правка позиционная: одна карточка за нажатие,
 * поэтому параллельная правка в боте не затирается.
 *
 * Блок конкурентов получил ВЫБОР ИСТОЧНИКА (просьба пользователя 2026-11-17):
 *   - «База знаний» — поиск карточек (пустое поле не ищет, не больше 5 строк) и
 *     кнопка «Подобрать» — подбор конкурентов того же класса от нашего продукта
 *     (бывший отдельный блок «Аналоги», он убран: одна работа — одно место);
 *   - «Файл» — файл принимает бот (приложение отправить его не может), сервис
 *     разбирает и кладёт конкурента с текстом;
 *   - «URL» — сервер читает страницу и добавляет конкурента с текстом.
 *
 * Чужой листок открывается на чтение: кнопки правки не показываются вовсе —
 * так человек не получает 403 на нажатие, которое выглядело доступным.
 */
import { useCallback, useEffect, useState, type JSX } from 'react';
import { ApiError } from './api.ts';
import { askBot, askButtonText } from './ask.ts';
import {
  addCompetitorFromUrl,
  addSide,
  canEdit,
  COMBAT_SEARCH_LIMIT,
  findAnalogues,
  hintOf,
  loadSheet,
  removeSide,
  removeTextCompetitor,
  saveHint,
  searchSide,
  sheetLabelOf,
  shouldSearchSide,
  sideSearchResults,
  sourceLabel,
  type CombatPickerItem,
  type CombatSheetDto,
  type CombatSide,
  type CombatTextRefDto,
} from './combat.ts';
import { shortDate, statusLabel } from './lists.ts';
import { SendToChatButton } from './sendchat-button.tsx';

function errorText(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 401 || e.status === 403) return 'Нет доступа к этому листку.';
    return e.message;
  }
  return 'Не удалось связаться с сервером.';
}

export function CombatCard({
  id,
  email,
  role,
}: {
  id: number;
  email: string;
  role: string;
}): JSX.Element {
  const [sheet, setSheet] = useState<CombatSheetDto | null>(null);
  const [error, setError] = useState('');
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);

  const reload = useCallback(() => {
    setSheet(null);
    loadSheet(id)
      .then((loaded) => setSheet(loaded))
      .catch((e: unknown) => setError(errorText(e)));
  }, [id]);

  useEffect(reload, [reload]);

  const run = useCallback((action: () => Promise<CombatSheetDto>, done: string) => {
    setBusy(true);
    setNote('');
    action()
      .then((next) => {
        setSheet(next);
        setNote(done);
      })
      .catch((e: unknown) => setNote(errorText(e)))
      .finally(() => setBusy(false));
  }, []);

  if (error !== '') {
    return (
      <section>
        <h1>Боевой листок #{id}</h1>
        <div className="app-card app-error">
          <p>{error}</p>
          <button className="app-button" type="button" onClick={reload}>
            Повторить
          </button>
        </div>
      </section>
    );
  }
  if (sheet === null) {
    return (
      <section>
        <p className="app-muted">Загружаем листок…</p>
      </section>
    );
  }

  const editable = canEdit(sheet, email, role);
  return (
    <section>
      <h1>
        БЛ {sheet.number} <span className="app-muted">· {statusLabel(sheet.status)}</span>
      </h1>
      <p className="app-muted">
        {sheetLabelOf(sheet)} · автор {sheet.ownerEmail} · {shortDate(sheet.updatedAt)}
      </p>
      {editable ? null : (
        <p className="app-note">
          Это листок коллеги. Смотреть и печатать можно, править — только автору.
        </p>
      )}
      {note === '' ? null : (
        <p className="app-note" role="status">
          {note}
        </p>
      )}

      <SideZone
        title="Наш продукт"
        side="our"
        refs={sheet.ourNotes}
        editable={editable}
        busy={busy}
        onAdd={(card) => run(() => addSide(id, 'our', card), `Добавлено: ${card.title}`)}
        onRemove={(noteId) => run(() => removeSide(id, 'our', noteId), 'Убрано с нашей стороны.')}
      />

      <CompetitorZone
        sheet={sheet}
        editable={editable}
        busy={busy}
        onAddKb={(card) => run(() => addSide(id, 'competitor', card), `Добавлено: ${card.title}`)}
        onRemoveKb={(noteId) => run(() => removeSide(id, 'competitor', noteId), 'Конкурент убран.')}
        onAddUrl={(url) =>
          run(() => addCompetitorFromUrl(id, url), 'Страница прочитана — конкурент добавлен.')
        }
        onRemoveText={(key) => run(() => removeTextCompetitor(id, key), 'Конкурент убран.')}
      />

      <HintZone
        sheet={sheet}
        editable={editable}
        busy={busy}
        onSave={(hint) => run(() => saveHint(id, hint), 'Уточнение сохранено.')}
      />

      <SendToChatButton kind="combat" id={id} />

      <p className="app-muted">
        <a className="app-link" href="#/list/sheets">
          К списку листков
        </a>
      </p>
    </section>
  );
}

/** Общий список выбранного стороны: чипы с «убрать». */
function Chips({
  refs,
  editable,
  busy,
  onRemove,
}: {
  refs: Array<{ noteId: number; title: string }>;
  editable: boolean;
  busy: boolean;
  onRemove: (noteId: number) => void;
}): JSX.Element {
  if (refs.length === 0) return <p className="app-muted">Пока пусто.</p>;
  return (
    <ul className="app-chips">
      {refs.map((ref) => (
        <li className="app-chip" key={ref.noteId}>
          <span>{ref.title}</span>
          {editable ? (
            <button
              className="app-chip-x"
              type="button"
              aria-label={`Убрать ${ref.title}`}
              disabled={busy}
              onClick={() => onRemove(ref.noteId)}
            >
              ✕
            </button>
          ) : null}
        </li>
      ))}
    </ul>
  );
}

function SideZone({
  title,
  side,
  refs,
  editable,
  busy,
  onAdd,
  onRemove,
}: {
  title: string;
  side: CombatSide;
  refs: Array<{ noteId: number; title: string }>;
  editable: boolean;
  busy: boolean;
  onAdd: (card: CombatPickerItem) => void;
  onRemove: (noteId: number) => void;
}): JSX.Element {
  return (
    <div className="app-card">
      <h2 className="app-h2">{title}</h2>
      <Chips refs={refs} editable={editable} busy={busy} onRemove={onRemove} />
      {editable ? <SideSearch side={side} busy={busy} onPick={onAdd} taken={refs.map((r) => r.noteId)} /> : null}
    </div>
  );
}

/** Источник конкурента: база знаний, файл (через бота) или ссылка. */
type CompetitorSource = 'kb' | 'file' | 'url';

const SOURCE_TITLES: Array<{ id: CompetitorSource; title: string }> = [
  { id: 'kb', title: 'База знаний' },
  { id: 'file', title: 'Файл' },
  { id: 'url', title: 'URL' },
];

function CompetitorZone({
  sheet,
  editable,
  busy,
  onAddKb,
  onRemoveKb,
  onAddUrl,
  onRemoveText,
}: {
  sheet: CombatSheetDto;
  editable: boolean;
  busy: boolean;
  onAddKb: (card: CombatPickerItem) => void;
  onRemoveKb: (noteId: number) => void;
  onAddUrl: (url: string) => void;
  onRemoveText: (key: string) => void;
}): JSX.Element {
  const [source, setSource] = useState<CompetitorSource>('kb');
  const [url, setUrl] = useState('');
  const [ask, setAsk] = useState('');
  const texts: CombatTextRefDto[] = sheet.textCompetitors ?? [];
  const taken = sheet.competitorNotes.map((ref) => ref.noteId);

  const askFile = (): void => {
    setAsk('⌛ Просим бота написать в чат…');
    void askBot('competitor-file', 'sheet', sheet.id).then((outcome) => {
      setAsk(outcome.problem !== '' ? outcome.problem : `Бот написал в чат: ${outcome.sent} Пришлите файл там.`);
    });
  };

  return (
    <div className="app-card">
      <h2 className="app-h2">Конкуренты</h2>
      {sheet.competitorNotes.length === 0 && texts.length === 0 ? (
        <p className="app-muted">Пока пусто.</p>
      ) : (
        <ul className="app-chips">
          {sheet.competitorNotes.map((ref) => (
            <li className="app-chip" key={`note-${ref.noteId}`}>
              <span>{ref.title}</span>
              {editable ? (
                <button
                  className="app-chip-x"
                  type="button"
                  aria-label={`Убрать ${ref.title}`}
                  disabled={busy}
                  onClick={() => onRemoveKb(ref.noteId)}
                >
                  ✕
                </button>
              ) : null}
            </li>
          ))}
          {texts.map((ref) => (
            <li className="app-chip" key={`text-${ref.key}`}>
              <span>
                {ref.title} <span className="app-muted">· {sourceLabel(ref.source)}</span>
              </span>
              {editable ? (
                <button
                  className="app-chip-x"
                  type="button"
                  aria-label={`Убрать ${ref.title}`}
                  disabled={busy}
                  onClick={() => onRemoveText(ref.key)}
                >
                  ✕
                </button>
              ) : null}
            </li>
          ))}
        </ul>
      )}

      {editable ? (
        <>
          <div className="app-row-3">
            {SOURCE_TITLES.map((item) => (
              <button
                key={item.id}
                className={`app-button ${source === item.id ? '' : 'app-button-ghost'}`}
                type="button"
                aria-pressed={source === item.id}
                onClick={() => setSource(item.id)}
              >
                {item.title}
              </button>
            ))}
          </div>
          {source === 'kb' ? (
            <>
              <SideSearch side="competitor" busy={busy} onPick={onAddKb} taken={taken} />
              <AnaloguePicker sheet={sheet} busy={busy} taken={taken} onPick={onAddKb} />
            </>
          ) : null}
          {source === 'url' ? (
            <>
              <p className="app-muted">Адрес страницы конкурента — сервер прочитает её и добавит конкурента.</p>
              <input
                className="app-input"
                type="url"
                value={url}
                placeholder="https://example.ru/catalog/"
                aria-label="Адрес страницы конкурента"
                onChange={(e) => setUrl(e.target.value)}
              />
              <button
                className="app-button"
                type="button"
                disabled={busy || url.trim() === ''}
                onClick={() => onAddUrl(url)}
              >
                {busy ? '⌛ Читаем…' : 'Прочитать'}
              </button>
            </>
          ) : null}
          {source === 'file' ? (
            <>
              <p className="app-muted">
                Файл отправляется в чат с ботом: приложение не может приложить его само. Бот прочитает
                файл и добавит конкурента в этот листок.
              </p>
              <button className="app-button" type="button" disabled={busy} onClick={askFile}>
                {askButtonText('competitor-file')}
              </button>
              {ask === '' ? null : <p className="app-muted">{ask}</p>}
            </>
          ) : null}
        </>
      ) : null}
    </div>
  );
}

/**
 * Подбор конкурентов того же класса от нашего продукта.
 *
 * Раньше это был отдельный блок «Аналоги» под уточнением: чтобы подобрать
 * соперника, человек уходил из блока конкурентов вниз и возвращался. Теперь
 * подбор стоит там, где он нужен, — под поиском (просьба пользователя
 * 2026-11-17). Кнопка гаснет, когда нашего продукта нет: ручка
 * `/combat/analogues?noteId=` подбирает от НАШЕГО продукта, и без него ей
 * нечего сравнивать.
 */
function AnaloguePicker({
  sheet,
  busy,
  taken,
  onPick,
}: {
  sheet: CombatSheetDto;
  busy: boolean;
  taken: number[];
  onPick: (card: CombatPickerItem) => void;
}): JSX.Element {
  const [cards, setCards] = useState<CombatPickerItem[] | null>(null);
  const [hint, setHint] = useState('');
  const [loading, setLoading] = useState(false);
  const source = sheet.ourNotes?.[0];
  const set = new Set(taken);

  const search = (): void => {
    if (!source) return;
    setLoading(true);
    setHint('');
    findAnalogues(source.noteId)
      .then((found) => setCards(found))
      .catch((e: unknown) => setHint(errorText(e)))
      .finally(() => setLoading(false));
  };

  return (
    <div className="app-search-block">
      {source === undefined ? (
        <p className="app-muted">«Подобрать» ищет конкурентов от нашего продукта — сначала добавьте его.</p>
      ) : (
        <>
          <p className="app-muted">Подобрать конкурентов того же класса, что и «{source.title}».</p>
          <button className="app-button app-button-ghost" type="button" disabled={busy || loading} onClick={search}>
            {loading ? '⌛ Подбираем…' : '🔍 Подобрать'}
          </button>
        </>
      )}
      {hint === '' ? null : <p className="app-error-text">{hint}</p>}
      {cards !== null && cards.length === 0 ? (
        <p className="app-muted">Подходящих конкурентов не нашлось — уточните наш продукт.</p>
      ) : null}
      {cards === null || cards.length === 0 ? null : (
        <ul className="app-results">
          {cards.map((card) => (
            <li key={card.noteId}>
              <button
                className="app-result"
                type="button"
                disabled={busy || set.has(card.noteId)}
                onClick={() => onPick(card)}
              >
                <span>{card.title}</span>
                <span className="app-muted">{set.has(card.noteId) ? 'уже в листке' : 'в конкуренты'}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function SideSearch({
  side,
  busy,
  taken,
  onPick,
}: {
  side: CombatSide;
  busy: boolean;
  taken: number[];
  onPick: (card: CombatPickerItem) => void;
}): JSX.Element {
  const [query, setQuery] = useState('');
  const [items, setItems] = useState<CombatPickerItem[]>([]);
  const [hint, setHint] = useState('');
  // Запрос, на который уже пришёл ответ: пока его нет, «ничего не нашлось»
  // показывать нельзя — человек ещё печатает.
  const [answered, setAnswered] = useState('');

  useEffect(() => {
    // Пустое поле — не поиск (2026-11-17): списка нет вовсе, а не весь кеш.
    if (!shouldSearchSide(query)) {
      setItems([]);
      setHint('');
      setAnswered('');
      return;
    }
    let alive = true;
    const asked = query.trim();
    const timer = window.setTimeout(() => {
      searchSide(side, asked, 0, COMBAT_SEARCH_LIMIT)
        .then((page) => {
          if (!alive) return;
          setItems(sideSearchResults(page.items));
          setHint('');
          setAnswered(asked);
        })
        .catch((e: unknown) => {
          if (!alive) return;
          setItems([]);
          setHint(errorText(e));
          setAnswered(asked);
        });
    }, 300);
    return () => {
      alive = false;
      window.clearTimeout(timer);
    };
  }, [side, query]);

  const set = new Set(taken);
  const typed = shouldSearchSide(query);
  const nothingFound = typed && hint === '' && answered === query.trim() && items.length === 0;
  return (
    <div className="app-search-block">
      <input
        className="app-input"
        type="search"
        value={query}
        placeholder={side === 'our' ? 'Найти наш продукт' : 'Найти продукт конкурента'}
        aria-label={side === 'our' ? 'Поиск нашего продукта' : 'Поиск конкурента'}
        onChange={(e) => setQuery(e.target.value)}
      />
      {typed ? null : <p className="app-muted">Начните вводить название — покажем до 5 совпадений.</p>}
      {hint === '' ? null : <p className="app-muted">{hint}</p>}
      {nothingFound ? <p className="app-muted">Ничего не нашлось — уточните название.</p> : null}
      {items.length === 0 ? null : (
        <ul className="app-results">
          {items.map((item) => (
            <li key={item.noteId}>
              <button
                className="app-result"
                type="button"
                disabled={busy || set.has(item.noteId)}
                onClick={() => onPick(item)}
              >
                <span>{item.title}</span>
                <span className="app-muted">{set.has(item.noteId) ? 'уже в листке' : 'добавить'}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function HintZone({
  sheet,
  editable,
  busy,
  onSave,
}: {
  sheet: CombatSheetDto;
  editable: boolean;
  busy: boolean;
  onSave: (hint: string) => void;
}): JSX.Element {
  const [hint, setHint] = useState(hintOf(sheet));
  useEffect(() => setHint(hintOf(sheet)), [sheet]);
  if (!editable) {
    return (
      <div className="app-card">
        <h2 className="app-h2">Уточнение для разбора</h2>
        <p className={hint === '' ? 'app-muted' : ''}>{hint === '' ? 'Не задано.' : hint}</p>
      </div>
    );
  }
  const dirty = hint !== hintOf(sheet);
  return (
    <div className="app-card">
      <h2 className="app-h2">Уточнение для разбора</h2>
      <p className="app-muted">На чём сравнение должно сделать акцент.</p>
      <textarea
        className="app-textarea"
        rows={3}
        value={hint}
        aria-label="Уточнение для разбора"
        onChange={(e) => setHint(e.target.value)}
      />
      <button className="app-button" type="button" disabled={busy || !dirty} onClick={() => onSave(hint)}>
        {dirty ? 'Сохранить уточнение' : 'Сохранено'}
      </button>
    </div>
  );
}