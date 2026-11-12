/**
 * Карточка КП (2026-11-13, шаг S3).
 *
 * Каждая часть сохраняется своей ручкой, поэтому на экране три независимые
 * зоны — заказчик, системы, позиции. «Сохранить» нажимается у той зоны, где
 * человек правил: сервер меняет ровно это поле (см. `offer.ts`).
 *
 * Тексты разделов (вступление, описание, преимущества) — шаг S4: здесь их нет
 * намеренно, чтобы не завести второй способ сохранения одного и того же.
 */
import { useCallback, useEffect, useState, type JSX } from 'react';
import { ApiError } from './api.ts';
import {
  addMaterial,
  addSystem,
  chosenMaterials,
  chosenSystems,
  clientChanged,
  draftToItem,
  draftTotal,
  EMPTY_ITEM,
  itemDraftOf,
  itemsChanged,
  itemsProblem,
  loadOffer,
  loadSystemCatalog,
  money,
  removeMaterial,
  removeSystem,
  rowSum,
  saveClient,
  saveItems,
  searchMaterials,
  searchSystems,
  type ChosenSystem,
  type ItemDraft,
  type OfferClientDto,
  type OfferDto,
  type SystemCardDto,
} from './offer.ts';
import { shortDate, statusLabel } from './lists.ts';
import { SectionsZone } from './sections-screen.tsx';
import { SendToChatButton } from './sendchat-button.tsx';

type LoadState =
  | { kind: 'loading' }
  | { kind: 'error'; text: string }
  | { kind: 'ready'; offer: OfferDto; catalog: SystemCardDto[] };

function errorText(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 401 || e.status === 403) return 'Нет доступа к этому предложению.';
    return e.message;
  }
  return 'Не удалось связаться с сервером.';
}

export function OfferCard({ id }: { id: number }): JSX.Element {
  const [state, setState] = useState<LoadState>({ kind: 'loading' });
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);

  const reload = useCallback(() => {
    setState({ kind: 'loading' });
    void Promise.all([loadOffer(id), loadSystemCatalog().catch(() => [] as SystemCardDto[])])
      .then(([offer, catalog]) => {
        setState({ kind: 'ready', offer, catalog });
      })
      .catch((e: unknown) => setState({ kind: 'error', text: errorText(e) }));
  }, [id]);

  useEffect(reload, [reload]);

  /** Общий приём записи: занятость, ответ сервера, текст для человека. */
  const apply = useCallback(
    (action: () => Promise<OfferDto>, done: string) => {
      setBusy(true);
      setNote('');
      action()
        .then((offer) => {
          setState((prev) => (prev.kind === 'ready' ? { ...prev, offer } : prev));
          setNote(done);
        })
        .catch((e: unknown) => setNote(errorText(e)))
        .finally(() => setBusy(false));
    },
    [],
  );

  if (state.kind === 'loading') {
    return (
      <section>
        <p className="app-muted">Загружаем карточку…</p>
      </section>
    );
  }
  if (state.kind === 'error') {
    return (
      <section>
        <h1>КП #{id}</h1>
        <div className="app-card app-error">
          <p>{state.text}</p>
          <button className="app-button" type="button" onClick={reload}>
            Повторить
          </button>
        </div>
        <p className="app-muted">
          <a className="app-link" href="#/list/offers">
            К списку КП
          </a>
        </p>
      </section>
    );
  }

  const { offer, catalog } = state;
  return (
    <section>
      <h1>
        КП {offer.number} <span className="app-muted">· {statusLabel(offer.status)}</span>
      </h1>
      <p className="app-muted">
        изменено {shortDate(offer.updatedAt)} · итог {money(offer.total)} ₽
      </p>
      {note === '' ? null : (
        <p className="app-note" role="status">
          {note}
        </p>
      )}

      <ClientZone
        offer={offer}
        busy={busy}
        onSave={(client) => apply(() => saveClient(id, client), 'Заказчик сохранён.')}
      />

      <SystemsZone
        offer={offer}
        catalog={catalog}
        busy={busy}
        onAdd={(system) => {
          apply(async () => (await addSystem(id, system)).offer, `Система «${system.title}» добавлена.`);
        }}
        onRemove={(system) => {
          apply(async () => (await removeSystem(id, system)).offer, `Система «${system.title}» убрана.`);
        }}
      />

      <ItemsZone
        offer={offer}
        busy={busy}
        onSave={(drafts) =>
          apply(() => saveItems(id, drafts.map(draftToItem)), 'Позиции сохранены.')
        }
      />

      <MaterialsZone
        offer={offer}
        busy={busy}
        onAdd={(material) => {
          apply(
            async () => (await addMaterial(id, material)).offer,
            `Материал «${material.title}» добавлен описанием.`,
          );
        }}
        onRemove={(material) => {
          apply(async () => (await removeMaterial(id, material)).offer, 'Материал убран.');
        }}
      />

      <SectionsZone id={id} />

      <SendToChatButton kind="offer" id={id} />

      <p className="app-muted">
        <a className="app-link" href="#/list/offers">
          К списку КП
        </a>
      </p>
    </section>
  );
}

function ClientZone({
  offer,
  busy,
  onSave,
}: {
  offer: OfferDto;
  busy: boolean;
  onSave: (client: OfferClientDto) => void;
}): JSX.Element {
  const [client, setClient] = useState<OfferClientDto>({
    name: offer.client?.name ?? '',
    contact: offer.client?.contact ?? '',
  });

  useEffect(() => {
    setClient({ name: offer.client?.name ?? '', contact: offer.client?.contact ?? '' });
  }, [offer.client?.name, offer.client?.contact]);

  const dirty = clientChanged(client, offer.client);
  return (
    <div className="app-card">
      <h2 className="app-h2">Заказчик</h2>
      <label className="app-field">
        <span className="app-field-label">Название</span>
        <input
          className="app-input"
          value={client.name}
          placeholder="ООО «Ромашка»"
          onChange={(e) => setClient({ ...client, name: e.target.value })}
        />
      </label>
      <label className="app-field">
        <span className="app-field-label">Контакт</span>
        <input
          className="app-input"
          value={client.contact}
          placeholder="Имя, телефон"
          onChange={(e) => setClient({ ...client, contact: e.target.value })}
        />
      </label>
      <button className="app-button" type="button" disabled={busy || !dirty} onClick={() => onSave(client)}>
        {dirty ? 'Сохранить заказчика' : 'Сохранено'}
      </button>
    </div>
  );
}

function SystemsZone({
  offer,
  catalog,
  busy,
  onAdd,
  onRemove,
}: {
  offer: OfferDto;
  catalog: SystemCardDto[];
  busy: boolean;
  onAdd: (system: ChosenSystem) => void;
  onRemove: (system: ChosenSystem) => void;
}): JSX.Element {
  const chosen = chosenSystems(offer, catalog);
  return (
    <div className="app-card">
      <h2 className="app-h2">Системы</h2>
      {chosen.length === 0 ? (
        <p className="app-muted">Система ещё не выбрана: без неё в документе нет описания и ссылок.</p>
      ) : (
        <ul className="app-chips">
          {chosen.map((system) => (
            <li className="app-chip" key={`${system.sku}:${system.noteId}`}>
              <span>{system.title}</span>
              <button
                className="app-chip-x"
                type="button"
                aria-label={`Убрать систему ${system.title}`}
                disabled={busy}
                onClick={() => onRemove(system)}
              >
                ✕
              </button>
            </li>
          ))}
        </ul>
      )}
      <SystemSearch busy={busy} chosen={chosen} onPick={onAdd} />
      <p className="app-muted">
        Выбор системы не добавляет её материалы в позиции — цена строк вводится вручную.
      </p>
    </div>
  );
}

function SystemSearch({
  busy,
  chosen,
  onPick,
}: {
  busy: boolean;
  chosen: ChosenSystem[];
  onPick: (system: ChosenSystem) => void;
}): JSX.Element {
  const [query, setQuery] = useState('');
  const [found, setFound] = useState<Array<{ noteId: number; sku?: string; title: string }>>([]);
  const [hint, setHint] = useState('');

  useEffect(() => {
    if (query.trim() === '') {
      setFound([]);
      setHint('');
      return;
    }
    let alive = true;
    const timer = window.setTimeout(() => {
      searchSystems(query)
        .then((res) => {
          if (!alive) return;
          setFound(res.items ?? []);
          setHint(res.error ?? '');
        })
        .catch((e: unknown) => {
          if (alive) {
            setFound([]);
            setHint(errorText(e));
          }
        });
    }, 300);
    return () => {
      alive = false;
      window.clearTimeout(timer);
    };
  }, [query]);

  const taken = new Set(chosen.map((c) => `${c.sku.toUpperCase()}:${c.noteId}`));
  return (
    <div className="app-search-block">
      <input
        className="app-input"
        type="search"
        value={query}
        placeholder="Найти систему по названию"
        aria-label="Поиск системы"
        onChange={(e) => setQuery(e.target.value)}
      />
      {hint === '' ? null : <p className="app-muted">{hint}</p>}
      {found.length === 0 ? null : (
        <ul className="app-results">
          {found.map((item) => {
            const key = `${(item.sku ?? '').toUpperCase()}:${item.noteId}`;
            const isTaken = taken.has(key);
            return (
              <li key={key}>
                <button
                  className="app-result"
                  type="button"
                  disabled={busy || isTaken}
                  onClick={() => {
                    onPick({ sku: item.sku ?? '', noteId: item.noteId, title: item.title });
                    setQuery('');
                  }}
                >
                  <span>{item.title}</span>
                  <span className="app-muted">{isTaken ? 'уже выбрана' : 'добавить'}</span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

function ItemsZone({
  offer,
  busy,
  onSave,
}: {
  offer: OfferDto;
  busy: boolean;
  onSave: (drafts: ItemDraft[]) => void;
}): JSX.Element {
  const [drafts, setDrafts] = useState<ItemDraft[]>(() => (offer.items ?? []).map(itemDraftOf));
  const [problem, setProblem] = useState('');

  useEffect(() => {
    setDrafts((offer.items ?? []).map(itemDraftOf));
  }, [offer.items]);

  const dirty = itemsChanged(drafts, offer.items ?? []);
  const update = (index: number, patch: Partial<ItemDraft>): void => {
    setDrafts((prev) => prev.map((draft, i) => (i === index ? { ...draft, ...patch } : draft)));
    setProblem('');
  };

  const save = (): void => {
    const text = itemsProblem(drafts);
    setProblem(text);
    if (text === '') onSave(drafts);
  };

  return (
    <div className="app-card">
      <h2 className="app-h2">Позиции</h2>
      {drafts.length === 0 ? (
        <p className="app-muted">Позиций пока нет. Добавьте строку — цена вводится вручную.</p>
      ) : null}
      <ul className="app-items">
        {drafts.map((draft, index) => (
          <li className="app-item" key={index}>
            <label className="app-field">
              <span className="app-field-label">Наименование</span>
              <input
                className="app-input"
                value={draft.name}
                onChange={(e) => update(index, { name: e.target.value })}
              />
            </label>
            <div className="app-item-row">
              <label className="app-field app-field-small">
                <span className="app-field-label">Кол-во</span>
                <input
                  className="app-input"
                  inputMode="decimal"
                  value={draft.qty}
                  onChange={(e) => update(index, { qty: e.target.value })}
                />
              </label>
              <label className="app-field app-field-small">
                <span className="app-field-label">Ед.</span>
                <input
                  className="app-input"
                  value={draft.unit}
                  onChange={(e) => update(index, { unit: e.target.value })}
                />
              </label>
              <label className="app-field app-field-small">
                <span className="app-field-label">Цена</span>
                <input
                  className="app-input"
                  inputMode="decimal"
                  value={draft.price}
                  onChange={(e) => update(index, { price: e.target.value })}
                />
              </label>
            </div>
            <div className="app-item-row">
              <span className="app-muted">Сумма: {money(rowSum(draft))} ₽</span>
              <button
                className="app-link-button"
                type="button"
                disabled={busy}
                onClick={() => setDrafts((prev) => prev.filter((_, i) => i !== index))}
              >
                Удалить строку
              </button>
            </div>
          </li>
        ))}
      </ul>
      <div className="app-item-row">
        <button
          className="app-button app-button-ghost"
          type="button"
          disabled={busy}
          onClick={() => setDrafts((prev) => [...prev, { ...EMPTY_ITEM }])}
        >
          + Строка
        </button>
        <button className="app-button" type="button" disabled={busy || !dirty} onClick={save}>
          {dirty ? 'Сохранить позиции' : 'Сохранено'}
        </button>
      </div>
      {problem === '' ? null : <p className="app-error-text">{problem}</p>}
      <p className="app-muted">Итог: {money(draftTotal(drafts))} ₽ (считает сервер при сохранении)</p>
    </div>
  );
}

/**
 * Материалы-объекты предложения (2026-11-13, R3).
 *
 * Материал здесь попадает в документ ОПИСАНИЕМ, а не строкой с ценой. Подпись
 * говорит это дословно: рядом находятся позиции, и человек должен понимать
 * разницу до нажатия. Поиск тот же, что у выбора материала в позиции.
 */
function MaterialsZone({
  offer,
  busy,
  onAdd,
  onRemove,
}: {
  offer: OfferDto;
  busy: boolean;
  onAdd: (material: ChosenSystem) => void;
  onRemove: (material: ChosenSystem) => void;
}): JSX.Element {
  const chosen = chosenMaterials(offer);
  return (
    <div className="app-card">
      <h2 className="app-h2">Материалы (описанием)</h2>
      {chosen.length === 0 ? (
        <p className="app-muted">
          Материалов описанием нет: в документ идут их описание и ссылки, а цена — только в позициях.
        </p>
      ) : (
        <ul className="app-chips">
          {chosen.map((material) => (
            <li className="app-chip" key={`${material.sku}:${material.noteId}`}>
              <span>{material.title}</span>
              <button
                className="app-chip-x"
                type="button"
                aria-label={`Убрать материал ${material.title}`}
                disabled={busy}
                onClick={() => onRemove(material)}
              >
                ✕
              </button>
            </li>
          ))}
        </ul>
      )}
      <MaterialSearch busy={busy} chosen={chosen} onPick={onAdd} />
      <p className="app-muted">
        Выбор материала описанием не добавляет позицию — цена и количество вводятся в таблице.
      </p>
    </div>
  );
}

function MaterialSearch({
  busy,
  chosen,
  onPick,
}: {
  busy: boolean;
  chosen: ChosenSystem[];
  onPick: (material: ChosenSystem) => void;
}): JSX.Element {
  const [query, setQuery] = useState('');
  const [found, setFound] = useState<Array<{ noteId: number; sku?: string; name: string }>>([]);
  const [hint, setHint] = useState('');

  useEffect(() => {
    if (query.trim() === '') {
      setFound([]);
      setHint('');
      return;
    }
    let alive = true;
    const timer = window.setTimeout(() => {
      searchMaterials(query)
        .then((res) => {
          if (!alive) return;
          setFound(res.items ?? []);
          setHint(res.error ?? '');
        })
        .catch((e: unknown) => {
          if (alive) {
            setFound([]);
            setHint(errorText(e));
          }
        });
    }, 300);
    return () => {
      alive = false;
      window.clearTimeout(timer);
    };
  }, [query]);

  const taken = new Set(chosen.map((c) => `${c.sku.toUpperCase()}:${c.noteId}`));
  return (
    <div className="app-search-block">
      <input
        className="app-input"
        type="search"
        value={query}
        placeholder="Найти материал для предложения"
        aria-label="Поиск материала"
        onChange={(e) => setQuery(e.target.value)}
      />
      {hint === '' ? null : <p className="app-muted">{hint}</p>}
      {found.length === 0 ? null : (
        <ul className="app-results">
          {found.map((item) => {
            const key = `${(item.sku ?? '').toUpperCase()}:${item.noteId}`;
            const isTaken = taken.has(key);
            return (
              <li key={key}>
                <button
                  className="app-result"
                  type="button"
                  disabled={busy || isTaken}
                  onClick={() => {
                    onPick({ sku: item.sku ?? '', noteId: item.noteId, title: item.name });
                    setQuery('');
                  }}
                >
                  <span>{item.name}</span>
                  <span className="app-muted">{isTaken ? 'уже выбран' : 'добавить'}</span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}