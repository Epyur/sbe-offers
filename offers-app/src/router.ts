/**
 * Маршрутизация по хэшу (2026-11-13, S0).
 *
 * Своя, без библиотеки: маршрутов на всё приложение перечислено девять строк,
 * а `react-router` в бандле стоит заметно больше. Разбор — чистая функция
 * `parseRoute`, чтобы маршруты проверялись тестом, а не глазами в браузере.
 */

export type Route =
  | { name: 'home' }
  | { name: 'ambulance' }
  | { name: 'list'; kind: 'offers' | 'sheets'; query: string }
  | { name: 'offer'; id: number }
  | { name: 'sheet'; id: number }
  | { name: 'notFound'; hash: string };

const EMPTY_HASH = '';

/**
 * Параметры, которые дописывает в хэш сам Telegram при открытии Mini App.
 * Открывая приложение, платформа приклеивает к адресу `tgWebAppData`,
 * `tgWebAppVersion`, `tgWebAppPlatform`, `tgWebAppThemeParams` — и без их
 * отбрасывания маршрут не распознаётся: человек видит «Экран не найден»
 * (живой дефект 2026-11-13, самый первый запуск).
 */

/**
 * `#/offer/12` → `{name:'offer', id:12}`; мусор → notFound, а не исключение.
 *
 * Корень (`''`, `#/`, `#`, только параметры Telegram) — приветственная
 * страница: три карточки разделов (2026-11-14).
 *
 * Хэш приходит в двух видах:
 *   - от бота: `#/offer/12`, `#/list/offers?q=…`;
 *   - от Telegram: `#tgWebAppData=…&tgWebAppVersion=9.6` (маршрута нет —
 *     открылся тот адрес, что задан кнопкой меню, то есть приветствие).
 * Поэтому сначала отбрасываем параметры платформы и берём путь.
 */
export function parseRoute(hash: string): Route {
  const raw = hash.trim();
  if (raw === EMPTY_HASH || raw === '#/' || raw === '#') {
    return { name: 'home' };
  }

  const withoutHash = raw.startsWith('#') ? raw.slice(1) : raw;

  // Хэш приходит в двух видах:
  //   - от бота:      `/offer/12`, `/list/offers?q=мембрана`;
  //   - от Telegram:  `tgWebAppData=…&tgWebAppVersion=9.6&tgWebAppPlatform=…`.
  // Второй вид маршрута не содержит (кнопка меню ведёт на корень приложения),
  // поэтому его параметры надо отбросить, а не разбирать как путь.
  //
  // Простой признак: маршрут всегда начинается с `/`, а параметры платформы
  // идут парами `k=v`. Режем по `&` только когда первая часть — не маршрут.
  const firstAmp = withoutHash.indexOf('&');
  const firstSegment = firstAmp >= 0 ? withoutHash.slice(0, firstAmp) : withoutHash;
  const looksLikeParams = !firstSegment.startsWith('/');

  let pathPart = '';
  let searchPart = '';

  if (looksLikeParams) {
    // Параметры Telegram: своего маршрута здесь нет. Возможен `startapp` —
    // это единственное, что платформа несёт осмысленного (будущие ссылки).
    const params = new URLSearchParams(withoutHash);
    const startParam = params.get('tgWebAppStartParam') ?? '';
    if (startParam.startsWith('/')) {
      pathPart = startParam;
    }
  } else {
    const queryAt = firstSegment.indexOf('?');
    pathPart = queryAt >= 0 ? firstSegment.slice(0, queryAt) : firstSegment;
    searchPart = queryAt >= 0 ? firstSegment.slice(queryAt + 1) : '';
    // Остаток после `&` в обычном хэше — дополнительные параметры маршрута.
    if (firstAmp >= 0) {
      const rest = withoutHash.slice(firstAmp + 1);
      searchPart = searchPart === '' ? rest : `${searchPart}&${rest}`;
    }
  }

  const parts = pathPart.split('/').filter((p) => p !== '');

  if (parts.length === 2 && parts[0] === 'list') {
    const kind = parts[1];
    if (kind === 'offers' || kind === 'sheets') {
      return { name: 'list', kind, query: new URLSearchParams(searchPart).get('q') ?? '' };
    }
  }

  if (parts.length === 1 && parts[0] === 'ambulance') {
    return { name: 'ambulance' };
  }

  if (parts.length === 2) {
    const id = Number(parts[1]);
    if (Number.isInteger(id) && id > 0) {
      if (parts[0] === 'offer') return { name: 'offer', id };
      if (parts[0] === 'sheet') return { name: 'sheet', id };
    }
  }

  // Хэш есть, но маршрута в нём нет: Telegram дописал только свои параметры
  // (значит, открыли адрес кнопки меню без маршрута) — показываем приветствие,
  // а не «Экран не найден». Ошибкой остаётся только настоящий мусор в пути.
  if (pathPart === '' && withoutHash !== '') {
    return { name: 'home' };
  }

  return { name: 'notFound', hash: raw };
}

/** Собрать адрес для бота: `#/offer/12`, `#/list/sheets?q=…`, `#/` — приветствие. */
export function routeHash(route: Exclude<Route, { name: 'notFound' }>, base = ''): string {
  const prefix = base.endsWith('/') || base === '' ? base : `${base}/`;
  switch (route.name) {
    case 'home':
      return `${prefix}#/`;
    case 'ambulance':
      return `${prefix}#/ambulance`;
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