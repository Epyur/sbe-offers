/**
 * Маршрутизация по хэшу (2026-11-13, S0).
 *
 * Своя, без библиотеки: маршрутов на всё приложение перечислено девять строк,
 * а `react-router` в бандле стоит заметно больше. Разбор — чистая функция
 * `parseRoute`, чтобы маршруты проверялись тестом, а не глазами в браузере.
 */

export type Route =
  | { name: 'list'; kind: 'offers' | 'sheets'; query: string }
  | { name: 'offer'; id: number }
  | { name: 'sheet'; id: number }
  | { name: 'notFound'; hash: string };

const EMPTY_HASH = '';

/** `#/offer/12` → `{name:'offer', id:12}`; мусор → notFound, а не исключение. */
export function parseRoute(hash: string): Route {
  const raw = hash.trim();
  if (raw === EMPTY_HASH || raw === '#/' || raw === '#') {
    return { name: 'list', kind: 'offers', query: '' };
  }

  const withoutHash = raw.startsWith('#') ? raw.slice(1) : raw;
  const [pathPart = '', searchPart = ''] = withoutHash.split('?');
  const parts = pathPart.split('/').filter((p) => p !== '');

  if (parts.length === 2 && parts[0] === 'list') {
    const kind = parts[1];
    if (kind === 'offers' || kind === 'sheets') {
      return { name: 'list', kind, query: new URLSearchParams(searchPart).get('q') ?? '' };
    }
  }

  if (parts.length === 2) {
    const id = Number(parts[1]);
    if (Number.isInteger(id) && id > 0) {
      if (parts[0] === 'offer') return { name: 'offer', id };
      if (parts[0] === 'sheet') return { name: 'sheet', id };
    }
  }

  return { name: 'notFound', hash: raw };
}

/** Собрать адрес для бота: `#/offer/12`, `#/list/sheets?q=…`. */
export function routeHash(route: Exclude<Route, { name: 'notFound' }>, base = ''): string {
  const prefix = base.endsWith('/') || base === '' ? base : `${base}/`;
  switch (route.name) {
    case 'list': {
      const q = route.query.trim();
      const suffix = q === '' ? '' : `?q=${encodeURIComponent(q)}`;
      return `${prefix}#/list/${route.kind}${suffix}`;
    }
    case 'offer':
      return `${prefix}#/offer/${route.id}`;
    case 'sheet':
      return `${prefix}#/sheet/${route.id}`;
  }
}