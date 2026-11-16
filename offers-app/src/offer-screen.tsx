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
  readWindowSave,
  removeMaterial,
  removeSystem,
  rowSum,
  changedValues,
  saveClient,
  saveFormValues,
  saveItems,
  searchMaterials,
  searchSystems,
  type ChosenSystem,
  type ItemDraft,
  type OfferClientDto,
  type OfferDto,
  type SystemCardDto,
  type WindowSaveResponse,
} from './offer.ts';
import { shortDate, statusLabel } from './lists.ts';
import { SectionsZone } from './sections-screen.tsx';
import { SendToChatButton } from './sendchat-button.tsx';
import { askBot, askButtonText } from './ask.ts';
import {
  loadOfferWindow,
  requiredOf,
  screenItems,
  zonesForScreen,
  type MissingField,
  type OfferFormWindowDto,
  type ScreenItem,
  type WindowZoneId,
} from './window.ts';

type LoadState =
  | { kind: 'loading' }
  | { kind: 'error'; text: string }
  | {
      kind: 'ready';
      offer: OfferDto;
      catalog: SystemCardDto[];
      /**
       * Окно КП (2026-11-15, шаг 3). `null` — настройку прочитать не удалось:
       * показываем все зоны и строку об этом, а не пустой экран.
       */
      window: OfferFormWindowDto | null;
      /** `false`, когда форма не прочиталась (тогда `window === null`). */
      windowRead: boolean;
    };

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
  const [missing, setMissing] = useState<MissingField[]>([]);

  const reload = useCallback(() => {
    setState({ kind: 'loading' });
    void Promise.all([
      loadOffer(id),
      loadSystemCatalog().catch(() => [] as SystemCardDto[]),
      // Окно читается отдельным запросом: форма общая для всех КП, и её правка
      // не должна перезагружать карточку. Ошибка окна — `null`, не отказ.
      loadOfferWindow()
        .then((form) => ({ ok: form !== null, form }))
        .catch(() => ({ ok: false as const, form: null })),
    ])
      .then(([offer, catalog, windowLoaded]) => {
        setState({
          kind: 'ready',
          offer,
          catalog,
          // `zones === null` — не прочитали; иначе это настоящий список зон.
          window: windowLoaded.form,
          windowRead: windowLoaded.ok,
        });
        setMissing([]);
      })
      .catch((e: unknown) => setState({ kind: 'error', text: errorText(e) }));
  }, [id]);

  useEffect(reload, [reload]);

  /** Общий приём записи: занятость, ответ сервера, текст для человека. */
  const apply = useCallback(
    (action: () => Promise<WindowSaveResponse>, done: string) => {
      setBusy(true);
      setNote('');
      action()
        .then((res) => {
          setState((prev) => (prev.kind === 'ready' ? { ...prev, offer: res.offer } : prev));
          // Мягкая обязательность: сохранение уже прошло, список — что дополнить.
          setMissing(res.missingRequired ?? []);
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

  const { offer, catalog, window, windowRead } = state;
  // Экран одним проходом по окну: зоны И поля формы, в порядке окна (0.11.3).
  // До 0.11.3 поля отбрасывались насовсем — их не было в приложении вовсе.
  const items: ScreenItem[] = windowRead
    ? screenItems(window)
    : zonesForScreen(null).map((zone) => ({ kind: 'zone' as const, id: zone }));
  const showAllZones = !windowRead;
  // Подсветка мягкая: список незаполненного не блокирует сохранение.
  const softMissing = windowRead ? requiredOf(window, offer) : [];
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
      {showAllZones ? (
        <p className="app-window-note" role="status">
          Настройку окна прочитать не удалось — показаны все разделы.
        </p>
      ) : null}
      {missing.length === 0 ? null : <MissingNote items={missing} />}

      {items.map((item) =>
        item.kind === 'zone' ? (
          <Zone
            key={`zone:${item.id}`}
            zone={item.id}
            offer={offer}
            catalog={catalog}
            busy={busy}
            missing={softMissing}
            id={id}
            onSaveClient={(client) => apply(() => saveClient(id, client), 'Заказчик сохранён.')}
            onSaveItems={(drafts) => apply(() => saveItems(id, drafts.map(draftToItem)), 'Позиции сохранены.')}
            onAddSystem={(system) => {
              apply(async () => readWindowSave(await addSystem(id, system)), `Система «${system.title}» добавлена.`);
            }}
            onRemoveSystem={(system) => {
              apply(async () => readWindowSave(await removeSystem(id, system)), `Система «${system.title}» убрана.`);
            }}
            onAddMaterial={(material) => {
              apply(
                async () => readWindowSave(await addMaterial(id, material)),
                `Материал «${material.title}» добавлен описанием.`,
              );
            }}
            onRemoveMaterial={(material) => {
              apply(async () => readWindowSave(await removeMaterial(id, material)), 'Материал убран.');
            }}
          />
        ) : (
          <FormFieldZone
            key={`field:${item.id}`}
            fieldId={item.id}
            label={item.label}
            offer={offer}
            busy={busy}
            required={softMissing.some((m) => m.id === item.id)}
            onSave={(values) => apply(() => saveFormValues(id, values), 'Поле сохранено.')}
          />
        ),
      )}

      <SendToChatButton kind="offer" id={id} />

      <p className="app-muted">
        <a className="app-link" href="#/list/offers">
          К списку КП
        </a>
      </p>
    </section>
  );
}

/** Строка «Не заполнено обязательное: …» — мягкое предупреждение, не блок. */
function MissingNote({ items }: { items: MissingField[] }): JSX.Element {
  return (
    <p className="app-window-note app-required" role="status">
      Не заполнено обязательное: {items.map((item) => item.label).join(', ')}.
    </p>
  );
}

interface ZoneProps {
  zone: WindowZoneId;
  offer: OfferDto;
  catalog: SystemCardDto[];
  busy: boolean;
  missing: MissingField[];
  id: number;
  onSaveClient: (client: OfferClientDto) => void;
  onSaveItems: (drafts: ItemDraft[]) => void;
  onAddSystem: (system: ChosenSystem) => void;
  onRemoveSystem: (system: ChosenSystem) => void;
  onAddMaterial: (material: ChosenSystem) => void;
  onRemoveMaterial: (material: ChosenSystem) => void;
}

/** Одна зона окна. Невидимые сюда не доходят: их отсекает `visibleZones`. */
function Zone(props: ZoneProps): JSX.Element | null {
  const { zone, offer, catalog, busy, missing, id } = props;
  const lacks = (key: string): boolean => missing.some((item) => item.id === key);
  switch (zone) {
    case 'client':
      return (
        <ClientZone
          offer={offer}
          busy={busy}
          required={lacks('client')}
          onSave={props.onSaveClient}
        />
      );
    case 'systems':
      return (
        <SystemsZone
          offer={offer}
          catalog={catalog}
          busy={busy}
          onAdd={props.onAddSystem}
          onRemove={props.onRemoveSystem}
        />
      );
    case 'items':
      return <ItemsZone offer={offer} busy={busy} onSave={props.onSaveItems} />;
    case 'materials':
      return (
        <MaterialsZone
          offer={offer}
          busy={busy}
          onAdd={props.onAddMaterial}
          onRemove={props.onRemoveMaterial}
        />
      );
    case 'sections':
      return <SectionsZone id={id} />;
    default:
      return null;
  }
}

/**
 * Поле формы в карточке КП (0.11.3).
 *
 * Поля формы раньше не рисовались вовсе: `visibleZones` их отбрасывала, а
 * `FieldsZone`, обещанный комментарием, не был написан. Разбор 2026-11-16
 * (пять моделей) дал одно правило: экран собирается ОДНИМ проходом по окну, и
 * поле — такой же элемент окна, как зона. Отдельный блок полей сломал бы
 * порядок, который администратор задал (поле между зонами — законно).
 *
 * Ценные поля «Заказчик» (название и контакт) сюда не попадают: они живут в
 * `offer.client` и правятся зоной `client` (решение пользователя 2026-11-16:
 * «считать частью зоны»). Если такое поле стоит в окне отдельной строкой,
 * компонент его не рисует — иначе на экране было бы два поля на одно значение.
 *
 * Сохранение — только изменённые ключи (`changedValues` → PATCH
 * `/form-values`): `form_values` делят поля, тексты разделов и бриф, и полная
 * запись снесла бы их.
 */
const CLIENT_FIELDS = new Set(['client_name', 'client_contact']);

function FormFieldZone({
  fieldId,
  label,
  offer,
  busy,
  required,
  onSave,
}: {
  fieldId: string;
  label: string;
  offer: OfferDto;
  busy: boolean;
  /** Окно пометило поле обязательным, а оно пустое — мягкая подсветка. */
  required: boolean;
  onSave: (values: Record<string, string>) => void;
}): JSX.Element | null {
  const saved = offer.formValues ?? {};
  const [draft, setDraft] = useState(saved[fieldId] ?? '');

  useEffect(() => {
    setDraft(saved[fieldId] ?? '');
  }, [saved, fieldId]);

  if (CLIENT_FIELDS.has(fieldId)) {
    // Значение правится зоной «Заказчик»; отдельная строка была бы дублем.
    return null;
  }

  const dirty = draft !== (saved[fieldId] ?? '');
  const multiline = fieldId === 'conditions';
  return (
    <div className={required ? 'app-card app-required' : 'app-card'}>
      <h2 className="app-h2">{label}</h2>
      {multiline ? (
        <textarea
          className="app-textarea"
          rows={3}
          value={draft}
          aria-label={label}
          onChange={(e) => setDraft(e.target.value)}
        />
      ) : (
        <input
          className="app-input"
          value={draft}
          aria-label={label}
          onChange={(e) => setDraft(e.target.value)}
        />
      )}
      <button
        className="app-button"
        type="button"
        disabled={busy || !dirty}
        onClick={() => onSave(changedValues({ [fieldId]: draft }, saved))}
      >
        {dirty ? 'Сохранить поле' : 'Сохранено'}
      </button>
    </div>
  );
}

function ClientZone({
  offer,
  busy,
  required,
  onSave,
}: {
  offer: OfferDto;
  busy: boolean;
  /** Окно отметило зону обязательной, а имя и контакт пусты. */
  required: boolean;
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
    <div className={required ? 'app-card app-required' : 'app-card'}>
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
  const [fileNote, setFileNote] = useState('');
  const [asking, setAsking] = useState(false);

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

  // Разбор файла делает бот (двухшагово: «что понял» → подтверждение), и в
  // приложении такого пути нет вовсе. Кнопка просит бота открыть этот шаг в
  // чате; человек присылает .xlsx туда, и позиции приходят в то же КП.
  const askForFile = (): void => {
    setAsking(true);
    setFileNote('');
    askBot('positions', offer.id)
      .then((res) => setFileNote(res.problem === '' ? res.sent : res.problem))
      .finally(() => setAsking(false));
  };

  return (
    <div className="app-card">
      <h2 className="app-h2">Позиции</h2>
      <div className="app-item-row">
        <button className="app-button app-button-ghost" type="button" disabled={asking} onClick={askForFile}>
          {asking ? '⌛ Пишем в чат…' : askButtonText('positions')}
        </button>
      </div>
      <p className="app-muted">
        Файл .xlsx (наименование, количество, цена) разбирает бот: он покажет, что понял, и
        спросит подтверждение — само ничего не запишется.
      </p>
      {fileNote === '' ? null : (
        <p className="app-note" role="status">
          {fileNote}
        </p>
      )}
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