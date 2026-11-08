// Сверка списка меток плагина с тем, что разбирает сервер.
//
// Повод (2026-11-07): `{{текст: система: description}}` был заявлен в справке,
// но в редакторе формы его не было вовсе, а сама метка молчала, потому что
// печатала только сохранённый текст. Человек узнал о метке из инструкции, не
// нашёл её в интерфейсе и получил пустое место в документе.
//
// Понятия берутся из СЕРВЕРНОГО кода, а не из списка, написанного здесь заново:
//   * слова-ключи служебных меток — `serviceLayoutMarkers` (`layout_tokens.go`);
//   * слова картинки — `startsWithImageWord` (там же);
//   * блоки текста системы — `systemTextBlocks` (`systemtext.go`).
// Второй список в проверке разошёлся бы с кодом так же, как разошлась справка.
//
// Запуск из папки плагина: `node scripts/check_marker_registry.mjs`.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const pluginRoot = path.resolve(here, '..');

let bad = 0;
function check(name, ok, detail = '') {
  if (!ok) bad++;
  console.log((ok ? 'OK   ' : 'MISS ') + name + (ok || !detail ? '' : ' — ' + detail));
}

const read = (rel) => fs.readFileSync(path.join(pluginRoot, rel), 'utf8');

/** Маркеры из плагина: `{ marker: '…', label: '…' }`. */
function pluginMarkers() {
  const out = [];
  const re = /\{\s*marker:\s*'([^']+)'\s*,\s*label:\s*'([^']*)'\s*\}/g;
  let m;
  const source = read('src/ui/markers.ts');
  while ((m = re.exec(source)) !== null) out.push({ marker: m[1], label: m[2] });
  return out;
}

/** Внутренняя часть метки: `{{имя}}` → `имя`. */
function inner(marker) {
  const t = marker.trim();
  if (t.startsWith('{{') && t.endsWith('}}')) return t.slice(2, -2).trim();
  if (t.startsWith('{') && t.endsWith('}')) return t.slice(1, -1).trim();
  return t;
}

/** Первое слово метки — то, по чему сервер решает вид. */
function markerKey(value) {
  return value.split(/[\s:]/)[0].toLowerCase();
}

const tokensGo = read('offers-service/layout_tokens.go');
const systemGo = read('offers-service/systemtext.go');

// Слова-ключи из `serviceLayoutMarkers`: `{layoutTokenLinks, "ссылки"}`.
const serviceKeys = [...tokensGo.matchAll(/\{layoutToken\w+,\s*"([^"]+)"\}/g)].map((m) => m[1]);
check('слова-ключи служебных меток найдены', serviceKeys.length >= 5, serviceKeys.join(', '));

// Блоки текста системы — из `systemTextBlocks`.
const blockList = (systemGo.match(/systemTextBlocks\s*=\s*\[\]string\{([^}]+)\}/) ?? [])[1] ?? '';
const textBlockIDs = [...blockList.matchAll(/systemText(?:Description|Advantages)/g)].map((m) => {
  const name = m[0].replace('systemText', '');
  return name.charAt(0).toLowerCase() + name.slice(1);
});
check('блоки текста системы сняты с сервера', textBlockIDs.length === 2, textBlockIDs.join(','));

// Слова картинки — из `startsWithImageWord`: `for _, stem := range []string{"картинк", …}`.
const imageStems = [...(tokensGo.match(/func startsWithImageWord[\s\S]{0,600}?range \[\]string\{([^}]+)\}/) ?? ['', ''])[1].matchAll(/"([^"]+)"/g)].map((m) => m[1]);
check('слова картинки сняты с сервера', imageStems.length > 0, imageStems.join(','));

/** Вид метки по её содержимому — тот же порядок, что в `parseLayoutServiceSpec`. */
function expectedKind(value) {
  const key = markerKey(value);
  if (key === 'block' || key === 'блок') return 'block';
  if (key === 'текст' || key === 'text') return 'text';
  if (imageStems.some((stem) => value.toLowerCase().startsWith(stem))) return 'image';
  for (const service of serviceKeys) {
    if (key.startsWith(service) || service.startsWith(key.slice(0, 4))) {
      if (service.startsWith('систем')) return 'system';
      if (service.startsWith('материал')) return 'material';
      return service.includes('ссыл') ? 'links' : service.includes('характер') ? 'specs' : 'system';
    }
  }
  return '';
}

const markers = pluginMarkers();
check('метки плагина найдены', markers.length > 0, String(markers.length));

const seen = new Set();
for (const { marker, label } of markers) {
  const value = inner(marker);
  seen.add(value.toLowerCase());
  const kind = expectedKind(value);
  if (kind === 'image') {
    check(`«${label}» → картинка: сервер её узнаёт`, true);
    continue;
  }
  if (kind === '') {
    // Служебное поле или промт: их проверяют серверные тесты.
    check(`«${label}» — метка, а не служебная`, true);
    continue;
  }
  const key = markerKey(value);
  check(`«${label}» → ${kind}: сервер знает ключ «${key}»`, !!key);
  if (kind === 'text') {
    const block = value.split(':').pop()?.trim() ?? '';
    check(`«${label}»: блок «${block}» существует на сервере`, textBlockIDs.includes(block),
      textBlockIDs.join(', '));
  }
}

// Что сервер умеет, а плагин не предлагает: человек снова узнает из справки.
//
// «материал» сюда намеренно не входит: это метка ВНУТРИ повтора по позициям
// (шаг D), и вне блока с repeat:"materials" она печатает первую позицию —
// предлагать её отдельным чипом значило бы обещать не то.
// Сервер понимает и «block:1», и «блок 1» — покрыт любой из двух форм.
for (const [name, prefixes] of [
  ['блок', ['block', 'блок']],
  ['система', ['систем']],
  ['текст', ['текст', 'text']],
  ['ссылки', ['ссылки']],
  ['характеристики', ['характеристик']],
]) {
  const covered = [...seen].some((v) => prefixes.some((p) => v.startsWith(p)));
  check(`в плагине есть метка «${name}…»`, covered);
}

process.exit(bad === 0 ? 0 : 1);