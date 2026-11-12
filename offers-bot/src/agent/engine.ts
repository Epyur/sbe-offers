/**
 * КОПИЯ канона: sbe-core/src/agent/engine.ts (2026-11-14).
 *
 * Бот КП собирается отдельным проектом и в sbe-core не заглядывает, поэтому
 * цикл агента живёт здесь копией. Правим ТОЛЬКО канон и копируем; расхождение
 * ловит \`scripts/check_agent_engine.mjs\` (он же сверяет константы правила).
 */
/**
 * Цикл агента LogicTEAM.007 — ЕДИНСТВЕННЫЙ источник правила
 * (2026-11-14, решение пользователя: «агент главный, шаги — его инструменты»).
 *
 * Потребителей три, и все три держат файл у себя:
 *   1. `sbe-core/src/agent/engine.ts` — канон (плагины Obsidian);
 *   2. `web/sbe-web/src/modules/agent/agentEngine.ts` — портал;
 *   3. `sbe-offers/offers-bot/src/agent/engine.ts` — агент бота КП.
 * Портал в `sbe-core` не заглядывает (правило 2026-09-17), поэтому копия, а не
 * импорт. Расхождение копий ловит `scripts/check_agent_engine.mjs`.
 *
 * Здесь НЕТ ни одного импорта: зависимости приходят аргументами
 * (`AgentEngineDeps`). Так один файл одинаково собирается и под Obsidian, и
 * под браузер, и под Node на сервере.
 *
 * Правила, ради которых этот файл вообще существует (каждое — живой дефект):
 *  - неразобранная разметка вызова пользователю НЕ показывается, модель
 *    получает просьбу повторить ход (2026-09-11, gpt-5-nano, 8 из 41);
 *  - оборванный на одну скобку JSON достраивается (2026-09-07);
 *  - если человек просил файл, а ход кончается текстом без единой выдачи —
 *    один переспрос модели, промежуточный текст не показывается (2026-10-07);
 *  - защита от зацикливания и лимит шагов не выбрасывают уже найденное:
 *    один ход синтеза ответа по транскрипту (2026-09-06).
 */

/* ============================ типы ============================ */

export type AgentRole = 'user' | 'assistant' | 'tool' | 'summary';

export interface AgentMessageLink {
  url: string;
  label: string;
}

export interface AgentMessage {
  role: AgentRole;
  content: string;
  files?: string[];
  tool?: string;
  toolOk?: boolean;
  link?: AgentMessageLink;
  created_at: string;
}

export interface AgentDialog {
  id: string;
  title: string;
  messages: AgentMessage[];
  created_at: string;
  updated_at: string;
}

export interface AgentToolSchema {
  name: string;
  description: string;
  input_schema: Record<string, unknown>;
}

export interface ToolCallResult {
  ok: boolean;
  summary: string;
  data?: unknown;
  error?: string;
  link?: AgentMessageLink;
}

/** Тул, как его видит движок: схема для модели и исполнитель. */
export interface AgentTool {
  schema: AgentToolSchema;
  execute: (arguments_: Record<string, unknown>) => Promise<ToolCallResult>;
}

export type LlmTurn =
  | { type: 'final'; text: string }
  | { type: 'tool_call'; tool: string; arguments: Record<string, unknown> }
  /** Ответ похож на вызов инструмента, но в форме, которую мы не привели к
   *  своей. Показывать такое пользователю нельзя — движок переспрашивает. */
  | { type: 'protocol_error'; raw: string };

/* ============================ всё, что приходит снаружи ============================ */

export interface AgentEngineDeps {
  /** Один запрос к модели. Ошибку бросает — движок её покажет. */
  complete: (system: string, user: string, options?: { model?: string }) => Promise<string>;
  /** Кого модель получает в «Сожми историю». */
  completeForCompaction: (system: string, user: string) => Promise<string>;
  /** Полный системный промпт (шаблон + источники + схемы тулов + правила). */
  buildSystemPrompt: (tools: AgentTool[]) => Promise<string>;
  /** Переписать историю после сжатия. Необязательно: без него сжатие только
   *  меняет массив в памяти диалога. */
  persistHistory?: (dialog: AgentDialog) => Promise<void>;
  /** Отдать сообщение в чат. */
  emit: (message: AgentMessage) => void;
  /** Статус работы для индикатора. */
  progress: (status: string) => void;
  /** Текущее время метки сообщения. По умолчанию — `new Date().toISOString()`. */
  now?: () => string;
  /** Журнал хода (2026-11-14, разбор переписки менеджеров). Необязательно. */
  logTurn?: (entry: AgentTurnLog) => void;
  /** Тихая запись предупреждения; по умолчанию console.warn. */
  warn?: (message: string, detail?: unknown) => void;
}

/** Что движок записывает в журнал за один запуск. Отдельный тип, а не
 *  свободный объект: у него есть потребитель (разбор менеджеров). */
export interface AgentTurnLog {
  userText: string;
  tools: Array<{ name: string; arguments: Record<string, unknown>; ok: boolean; summary: string }>;
  answer: string;
  reason: 'final' | 'synthesis' | 'protocol' | 'aborted' | 'error';
  latencyMs: number;
}

export interface RunAgentParams {
  dialog: AgentDialog;
  userMessage: string;
  model?: string;
  /** Отмена извне (Esc/кнопка «Стоп»). */
  signal?: AbortSignal;
}

/* ============================ константы правила ============================ */

const DEFAULT_MAX_ITERATIONS = 15;

/** Кап на JSON одного результата тула, отправляемый модели В ТЕКУЩЕМ ходу.
 *  При превышении — честный отказ, не обрезка посередине JSON. */
const MAX_TOOL_DATA_CHARS = 30000;

/** Сколько символов одного результата тула остаётся в истории диалога. */
const TOOL_HISTORY_CHARS = 15000;

/** Порог суммарной длины истории, после которого старая часть сжимается. */
const HISTORY_COMPACT_THRESHOLD_CHARS = 50000;

/** Сколько последних сообщений всегда остаются несжатыми. */
const HISTORY_KEEP_RECENT = 8;

const COMPACTION_SYSTEM_PROMPT = [
  'Сожми историю диалога агента в компактный структурированный дайджест на русском.',
  'ОБЯЗАТЕЛЬНО сохрани ВСЕ числа, id, даты, названия и принятые решения — списком',
  'или таблицей, не прозой. Не выдумывай ничего, чего нет в истории.',
  'Если в начале истории уже есть дайджест прошлого сжатия (помечен как',
  '"[Свёрнутая история]") — ОБНОВИ его, не потеряв ни одного факта из него,',
  'и добавь факты из новых сообщений. Верни ТОЛЬКО текст дайджеста, без пояснений.',
].join(' ');

/** Идемпотентные тулы: повтор с теми же аргументами не считается
 *  зацикливанием. Ведро общее на всех потребителей: `fetch_url` здесь
 *  намеренно ОТСУТСТВУЕТ — повтор даёт тот же ответ, и это ровно цикл. */
const IDEMPOTENT_TOOLS = new Set<string>([
  'browser_open', 'browser_wait', 'browser_extract', 'browser_links', 'browser_screenshot',
]);

/** Сколько раз подряд можно вызвать ОДИН И ТОТ ЖЕ инструмент с РАЗНЫМИ
 *  аргументами прежде чем остановиться и спросить человека. */
const CONSECUTIVE_SAME_TOOL_LIMIT = 5;

/** Сколько раз просим модель повторить ход в нашем формате. */
const PROTOCOL_RETRY_LIMIT = 2;

const PROTOCOL_REMINDER = 'Твой предыдущий ответ не распознан: он похож на вызов инструмента в чужом формате. '
  + 'Ответь ЗАНОВО ОДНИМ JSON-объектом ровно одной из двух форм (поле "type" обязательно): '
  + '{"type":"tool_call","tool":"<имя тула>","arguments":{…}} или {"type":"final","text":"<ответ пользователю>"}. '
  + 'Формы без "type" и любая разметка вызовов (DSML, XML-теги, function_call) системой не принимаются.';

/** Тулы, которые ФАКТИЧЕСКИ выдают файл пользователю. Их наличие снимает
 *  переспрос про файл. Разведка (`get_kb_notes`, `get_documents`, `get_emails`)
 *  сюда НЕ входит — она файла не отдаёт. */
const FILE_DELIVERY_TOOLS = new Set<string>([
  'get_kb_file_link', 'get_document_link', 'get_document_links_page', 'read_document_file',
  'get_photo_link', 'get_photo_links_page',
  'create_docx', 'create_xlsx', 'create_pdf', 'create_json', 'create_mermaid',
  'create_png', 'create_html', 'create_presentation', 'create_markdown', 'create_md',
  'build_docx', 'build_combat_docx',
]);

/* ============================ «человек просил файл?» ============================ */

export function asksForFile(text: string): boolean {
  const t = (text || '').toLowerCase();
  if (!t.trim()) return false;
  if (/(\bpdf\b|\.pdf|скач|прикреп|приложен|во вложени|вложени)/.test(t)) return true;
  if (/(файл|документ|картинк|изображени|фото|сертификат|протокол|заключени|схем|чертёж|чертеж)/.test(t)
      && /(дай|дайте|давай|пришли|вышли|отправь|открой|откройте|найди|найти|покажи|приложи|скачан|получ|хочу|нужен|нужна|нужно|скинь)/.test(t)) {
    return true;
  }
  return false;
}

export function lastUserText(dialog: AgentDialog): string {
  for (let i = dialog.messages.length - 1; i >= 0; i--) {
    const m = dialog.messages[i];
    if (m.role === 'user') return m.content || '';
  }
  return '';
}

/* ============================ разбор чужих форматов ============================ */

// Разметка вызовов DeepSeek (DSML) встречается в нескольких начертаниях:
// блоком `<…tool_calls>` и без него; висящие чёрточки (U+FF5C) стоят то слева,
// то справа от `DSML`.
//
// ГРАБЛИ (2026-11-14, стоило сессии): тег DSML разбирается НЕ регулярным
// выражением, а руками (см. `dsmlTagEnd`). Регулярка с закрывающим `>` в
// конце не находит тег, если сразу за ним идёт текст без `>` после («<…DSML…>
// invoke>»), — проверено пробами в Node для КАЖДОГО способа записи класса
// U+FF5C, включая `\uFF5C` в разных позициях. При этом тот же шаблон без
// последнего `>` находит нормально. Вместо погони за этим поведением — тег
// ищется посимвольно, а имя инструмента берётся подстрокой.
// Одиночный `<invoke>` без пары закрывающих тегов разбор не берёт: такой ответ
// уходит модели на переспрос (`protocol_error`), а не показывается человеку.
const DSML_CHAR = String.fromCharCode(0xff5c);

/** Позиция после тега `<…DSML…>`, начиная с `at`.
 *
 *  Два начертания, оба живые (2026-11-14):
 *  - `<…DSML…>tool_calls>`, `<…DSML…>invoke name="…">` — имя блока СРАЗУ после
 *    открывающей скобки, ПЕРЕД `>`;
 *  - `</…DSML…>parameter>`, `</…DSML…>invoke>` — то же у закрывающих.
 *  Тег — это `<` (и `/` для закрывающего), 0..4 чёрточки/палки, `DSML`,
 *  0..4 чёрточки/палки. Имя блока сюда не входит: оно разбирается отдельно
 *  (`blockNameEnd`), иначе `>` внутри тега не найти. */
function dsmlTagEnd(text: string, at: number): number {
  let i = at;
  if (text[i] !== '<') return -1;
  i++;
  if (text[i] === '/') i++;
  let bars = 0;
  while (bars < 4 && (text[i] === DSML_CHAR || text[i] === '|')) { i++; bars++; }
  if (!text.startsWith('DSML', i)) return -1;
  i += 4;
  bars = 0;
  while (bars < 4 && (text[i] === DSML_CHAR || text[i] === '|')) { i++; bars++; }
  return i;
}

/** Имя блока после тега: `>tool_calls>`, `>invoke name="…">`, `>parameter …>`.
 *  Возвращает `{ name, end }`, где `end` — позиция после закрывающей `>`.
 *  Если имени нет вовсе (`<…DSML…>` и сразу текст) — `name` пустой. */
function blockNameEnd(text: string, afterTag: number): { name: string; end: number } {
  let i = afterTag;
  while (i < text.length && /\s/.test(text[i])) i++;
  const names = ['tool_calls', 'invoke', 'parameter'];
  for (const name of names) {
    if (!text.startsWith(name, i)) continue;
    const close = text.indexOf('>', i + name.length);
    return { name, end: close === -1 ? text.length : close + 1 };
  }
  if (text[i] === '>') return { name: '', end: i + 1 };
  // Имени нет и `>` нет — тег не наш.
  return { name: '', end: -1 };
}

/** Все места, где начинается `<…DSML…>` — входные точки разбора. */
function dsmlTags(text: string): number[] {
  const found: number[] = [];
  for (let i = text.indexOf('<'); i >= 0; i = text.indexOf('<', i + 1)) {
    if (dsmlTagEnd(text, i) >= 0) found.push(i);
  }
  return found;
}

function looseJson(value: string): unknown {
  try { return JSON.parse(value); } catch { return value; }
}

function parseJsonTolerant(objStr: string): unknown {
  try { return JSON.parse(objStr); } catch { /* чиним ниже */ }
  let out = '';
  let inStr = false;
  let esc = false;
  for (const ch of objStr) {
    if (!inStr) {
      if (ch === '"') inStr = true;
      out += ch;
      continue;
    }
    if (esc) { esc = false; out += ch; continue; }
    if (ch === '\\') { esc = true; out += ch; continue; }
    if (ch === '"') { inStr = false; out += ch; continue; }
    if (ch === '\n') { out += '\\n'; continue; }
    if (ch === '\r') { out += '\\r'; continue; }
    if (ch === '\t') { out += '\\t'; continue; }
    out += ch.charCodeAt(0) < 0x20 ? `\\u${ch.charCodeAt(0).toString(16).padStart(4, '0')}` : ch;
  }
  try { return JSON.parse(out); } catch { return undefined; }
}

/** Все вызовы из DSML-блока. Ищутся посимвольно от каждого `<…DSML…>`:
 *  `[invoke name="…"] [parameter name="…" …]текст[/parameter]* [/invoke]`.
 *  Блок может быть один на весь ответ (`<…tool_calls>`), а может — сразу
 *  `invoke`; поэтому имя ищется в любом месте после тега, а не только в его
 *  начале. */
function parseDsmlCalls(text: string): LlmTurn[] {
  const turns: LlmTurn[] = [];
  let cursor = 0;
  while (cursor < text.length) {
    const invoke = nextDsmlInvoke(text, cursor);
    if (!invoke) break;
    const args: Record<string, unknown> = {};
    let bodyCursor = invoke.bodyStart;
    while (bodyCursor < invoke.bodyEnd) {
      const param = nextDsmlParam(text, bodyCursor, invoke.bodyEnd);
      if (!param) break;
      const parsed = readDsmlParam(text, param.start);
      if (!parsed) break;
      args[parsed.name] = /string\s*=\s*"true"/i.test(parsed.attrs)
        ? parsed.value.trim()
        : looseJson(parsed.value.trim());
      bodyCursor = parsed.end;
    }
    turns.push({ type: 'tool_call', tool: invoke.tool, arguments: args });
    cursor = invoke.bodyEnd;
  }
  return turns;
}

/** Ближайший вызов (`invoke name="…"`) от `from`; тело — до `</invoke>`.
 *
 *  Имя стоит по-разному в двух живых начертаниях (2026-11-14):
 *  - обёртка: `<…DSML…>tool_calls>` затем `<…DSML…>invoke name="x">` —
 *    имя в СЛЕДУЮЩЕМ теге;
 *  - без обёртки: `<…DSML…>invoke name="x">` — имя в том же теге.
 *  Поэтому: для обёртки идём от `block.end`; иначе восстанавливаем позицию
 *  сразу после чёрточек (`tagEnd`) и читаем `invoke name=` оттуда. */
function nextDsmlInvoke(text: string, from: number): { tool: string; bodyStart: number; bodyEnd: number } | null {
  for (let i = text.indexOf('<', from); i >= 0; i = text.indexOf('<', i + 1)) {
    const tagEnd = dsmlTagEnd(text, i);
    if (tagEnd < 0) continue;
    const block = blockNameEnd(text, tagEnd);
    if (block.name === 'tool_calls') {
      const after = block.end;
      const match = /^\s*invoke\s+name="([^"]+)"\s*>/.exec(text.slice(after));
      if (!match) continue;
      const bodyStart = after + match[0].length;
      return { tool: match[1], bodyStart, bodyEnd: closingTagEnd(text, bodyStart, 'invoke') };
    }
    // Без обёртки: тег и есть `<…DSML…>invoke name="x">`.
    const match = /^invoke\s+name="([^"]+)"\s*>/.exec(text.slice(tagEnd));
    if (!match) continue;
    const bodyStart = tagEnd + match[0].length;
    return { tool: match[1], bodyStart, bodyEnd: closingTagEnd(text, bodyStart, 'invoke') };
  }
  return null;
}

/** Позиция после `</…DSML…>invoke>` (или `</…DSML…>parameter>`), начиная с
 *  `from`; если закрывающего тега нет — конец строки. */
function closingTagEnd(text: string, from: number, tag: string): number {
  for (let i = text.indexOf('</', from); i >= 0; i = text.indexOf('</', i + 1)) {
    const end = dsmlTagEnd(text, i);
    if (end < 0) continue;
    const block = blockNameEnd(text, end);
    if (block.end < 0 || block.name !== tag) continue;
    return block.end;
  }
  return text.length;
}

/** Ближайший параметр (`parameter name=…`) до `until` — та же логика
 *  начертаний, что у `nextDsmlInvoke`. */
function nextDsmlParam(text: string, from: number, until: number): { start: number } | null {
  for (let i = text.indexOf('<', from); i >= 0 && i < until; i = text.indexOf('<', i + 1)) {
    const end = dsmlTagEnd(text, i);
    if (end < 0) continue;
    const block = blockNameEnd(text, end);
    if (block.name === 'parameter') return { start: i };
    if (block.name === 'tool_calls' || block.name === 'invoke') continue;
    if (/^parameter\s+name="/.test(text.slice(end))) return { start: i };
  }
  return null;
}

/** Один параметр DSML: имя, атрибуты (в т.ч. `string="true"`) и текст. */
function readDsmlParam(text: string, at: number): { name: string; attrs: string; value: string; end: number } | null {
  const tagEnd = dsmlTagEnd(text, at);
  if (tagEnd < 0) return null;
  const block = blockNameEnd(text, tagEnd);
  const from = block.name === 'parameter' ? tagEnd : block.end;
  if (from < 0) return null;
  const match = /^\s*parameter\s+name="([^"]+)"([^>]*)>/.exec(text.slice(from));
  if (!match) return null;
  const valueStart = from + match[0].length;
  const end = closingTagEnd(text, valueStart, 'parameter');
  const closeAt = text.lastIndexOf('</', end);
  const value = closeAt > valueStart ? text.slice(valueStart, closeAt) : text.slice(valueStart, end);
  return { name: match[1], attrs: match[2], value, end };
}

function asArguments(value: unknown): Record<string, unknown> | null {
  if (value && typeof value === 'object' && !Array.isArray(value)) return value as Record<string, unknown>;
  if (typeof value === 'string' && value.trim()) {
    const parsed = looseJson(value);
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) return parsed as Record<string, unknown>;
  }
  return null;
}

const LOOSE_CALL_KEYS = new Set(['tool', 'tool_call', 'tool_name', 'name', 'function', 'function_call', 'arguments', 'parameters', 'args']);

function toolCallFromLooseShape(obj: Record<string, unknown>): LlmTurn | null {
  const nested = obj.function_call ?? obj.function ?? obj.tool_call;
  if (nested && typeof nested === 'object' && !Array.isArray(nested)) {
    const inner = nested as Record<string, unknown>;
    const innerName = inner.name ?? inner.tool;
    if (typeof innerName === 'string' && innerName) {
      return { type: 'tool_call', tool: innerName, arguments: asArguments(inner.arguments ?? inner.parameters) ?? {} };
    }
  }
  const name = obj.tool_call ?? obj.tool ?? obj.tool_name ?? obj.name;
  if (typeof name !== 'string' || !name) return null;
  const args = asArguments(obj.arguments ?? obj.parameters ?? obj.args);
  if (args) return { type: 'tool_call', tool: name, arguments: args };
  // Тул без аргументов — принимаем, только если в объекте нет постороннего.
  if (Object.keys(obj).every(k => LOOSE_CALL_KEYS.has(k))) {
    return { type: 'tool_call', tool: name, arguments: {} };
  }
  return null;
}

function salvageFinalText(text: string): string | null {
  const match = text.match(/"text"\s*:\s*"([\s\S]*)$/);
  if (!match) return null;
  const body = match[1].replace(/"\s*\}*\s*$/, '');
  const unescaped = body
    .replace(/\\n/g, '\n')
    .replace(/\\t/g, '\t')
    .replace(/\\"/g, '"')
    .replace(/\\\\/g, '\\');
  return unescaped.trim() || null;
}

function looksLikeToolMarkup(text: string): boolean {
  const trimmed = text.trim();
  if (!trimmed) return false;
  if (dsmlTags(trimmed).length > 0) return true;
  // Блок вызовов без DSML-тега (модель начала сразу с `<tool_calls>`).
  if (/<tool_calls\b/i.test(trimmed)) return true;
  if (!trimmed.startsWith('{') && !trimmed.startsWith('[') && !trimmed.startsWith('<')) return false;
  return /"(tool|tool_call|tool_name|function|function_call|arguments|parameters)"\s*:/.test(trimmed);
}

/** Ленивый разбор хода модели: все подряд идущие JSON-объекты; обычный текст —
 *  законный финальный ответ, разметка вызова — нет. Экспортируется: это
 *  правило проверяется отдельно, без сети и без движка. */
export function parseTurns(text: string): LlmTurn[] {
  const turns: LlmTurn[] = [];
  let rest = text.trim();

  const dsml = parseDsmlCalls(rest);
  if (dsml.length > 0) return dsml;

  const fence = rest.match(/```(?:json)?\s*([\s\S]*?)```/i);
  if (fence) rest = fence[1].trim();

  while (true) {
    const start = rest.indexOf('{');
    if (start === -1) break;
    let depth = 0; let inStr = false; let esc = false; let objEnd = -1;
    for (let i = start; i < rest.length; i++) {
      const ch = rest[i];
      if (inStr) {
        if (esc) { esc = false; continue; }
        if (ch === '\\') { esc = true; continue; }
        if (ch === '"') inStr = false;
        continue;
      }
      if (ch === '"') { inStr = true; continue; }
      if (ch === '{') depth++;
      else if (ch === '}') { depth--; if (depth === 0) { objEnd = i; break; } }
    }
    let objStr: string;
    if (objEnd === -1) {
      // Провайдер обрывает генерацию ровно на одну «}» раньше конца, а
      // finish_reason при этом честно врёт «stop» (2026-09-07). Достраиваем.
      if (depth <= 0) break;
      objStr = rest.substring(start) + '}'.repeat(depth);
    } else {
      objStr = rest.substring(start, objEnd + 1);
      rest = rest.slice(objEnd + 1);
    }

    const parsed = parseJsonTolerant(objStr);
    if (parsed === undefined) { if (objEnd === -1) break; continue; }
    if (parsed && typeof parsed === 'object') {
      const obj = parsed as Record<string, unknown>;
      if (obj.type === 'final') {
        turns.push({ type: 'final', text: typeof obj.text === 'string' ? obj.text : '' });
        return turns;
      }
      if (obj.type === 'tool_call') {
        turns.push({
          type: 'tool_call',
          tool: typeof obj.tool === 'string' ? obj.tool : '',
          arguments: obj.arguments && typeof obj.arguments === 'object' ? (obj.arguments as Record<string, unknown>) : {},
        });
        if (objEnd === -1) break;
        continue;
      }
      const loose = toolCallFromLooseShape(obj);
      if (loose) {
        turns.push(loose);
        if (objEnd === -1) break;
        continue;
      }
      if (typeof (obj as { text?: unknown }).text === 'string') {
        turns.push({ type: 'final', text: (obj as { text: string }).text });
        return turns;
      }
    }
    if (objEnd === -1) break;
  }

  if (turns.length === 0) {
    const trimmed = text.trim();
    if (looksLikeToolMarkup(trimmed)) return [{ type: 'protocol_error', raw: trimmed }];
    const salvaged = trimmed.startsWith('{') ? salvageFinalText(trimmed) : null;
    turns.push({ type: 'final', text: salvaged ?? trimmed });
  }
  return turns;
}

/** Сжатие старой части истории. Экспортируется ради теста правила: порог,
 *  сохранение хвоста и порядок не должны зависеть от потребителя. */
export function shouldCompact(dialog: AgentDialog): boolean {
  const totalLength = dialog.messages.reduce((sum, m) => sum + m.content.length, 0);
  return totalLength > HISTORY_COMPACT_THRESHOLD_CHARS && dialog.messages.length > HISTORY_KEEP_RECENT;
}

/* ============================ движок ============================ */

export class AgentEngine {
  private deps: AgentEngineDeps;
  private tools: AgentTool[];
  private maxIterations: number;
  private rpm?: (status: string) => void;
  private rulesBlock: string | null = null;

  constructor(deps: AgentEngineDeps, tools: AgentTool[], maxIterations = DEFAULT_MAX_ITERATIONS) {
    this.deps = deps;
    this.tools = tools;
    this.maxIterations = maxIterations;
  }

  /** Правила пользователя — один запрос на весь диалог (кэш в rulesBlock). */
  private async buildSystemPrompt(): Promise<string> {
    if (this.rulesBlock === null) {
      this.rulesBlock = await this.deps.buildSystemPrompt(this.tools);
    }
    return this.rulesBlock;
  }

  private now(): string {
    return this.deps.now ? this.deps.now() : new Date().toISOString();
  }

  private warn(message: string, detail?: unknown): void {
    if (this.deps.warn) this.deps.warn(message, detail);
    else console.warn(message, detail);
  }

  serializeHistory(dialog: AgentDialog): string {
    const lines: string[] = [];
    for (const m of dialog.messages) {
      if (m.role === 'user') {
        const files = m.files?.length ? ` (прикреплён файл: ${m.files.join(', ')})` : '';
        lines.push(`[Пользователь] ${m.content}${files}`);
      } else if (m.role === 'assistant') {
        lines.push(`[Ассистент] ${m.content}`);
      } else if (m.role === 'tool') {
        lines.push(`[Результат тула ${m.tool || ''} (${m.toolOk ? 'ok' : 'ошибка'})] ${m.content.slice(0, TOOL_HISTORY_CHARS)}`);
      } else if (m.role === 'summary') {
        lines.push(`[Свёрнутая история] ${m.content}`);
      }
    }
    lines.push('');
    lines.push('Твой ход (только JSON):');
    return lines.join('\n');
  }

  private async compactHistoryIfNeeded(dialog: AgentDialog): Promise<void> {
    if (!shouldCompact(dialog)) return;
    const older = dialog.messages.slice(0, -HISTORY_KEEP_RECENT);
    const recent = dialog.messages.slice(-HISTORY_KEEP_RECENT);
    const oldPart = this.serializeHistory({ ...dialog, messages: older })
      .replace(/\n\nТвой ход \(только JSON\):$/, '');
    try {
      const digest = await this.deps.completeForCompaction(COMPACTION_SYSTEM_PROMPT, oldPart);
      const digestText = digest.trim();
      if (!digestText) return;
      dialog.messages = [
        { role: 'summary', content: digestText, created_at: this.now() },
        ...recent,
      ];
      if (this.deps.persistHistory) await this.deps.persistHistory(dialog);
    } catch (e: unknown) {
      this.warn('LogicTEAM.007: не удалось сжать историю диалога, продолжаю с полной:', e);
    }
  }

  private findTool(name: string): AgentTool | undefined {
    return this.tools.find(t => t.schema.name === name);
  }

  private toolMessage(tool: string, ok: boolean, content: string, link?: AgentMessageLink): AgentMessage {
    return { role: 'tool', tool, toolOk: ok, content, link, created_at: this.now() };
  }

  private summaryForLlm(summary: string, data?: unknown): string {
    if (data === undefined) return summary || '';
    let json = '';
    try {
      json = JSON.stringify(data);
    } catch { json = String(data); }
    if (json.length > MAX_TOOL_DATA_CHARS) {
      return `${summary}\nДанные слишком большие (${json.length} символов, лимит ${MAX_TOOL_DATA_CHARS}) — не показаны. Уточни фильтр/лимит или используй агрегирующий тул, если он есть.`;
    }
    return `${summary}\nДанные: ${json}`;
  }

  async run(params: RunAgentParams): Promise<void> {
    this.rpm = this.deps.progress;
    const startedAt = Date.now();
    const log = {
      userText: params.userMessage || lastUserText(params.dialog),
      tools: [] as AgentTurnLog['tools'],
      answer: '',
      reason: 'final' as AgentTurnLog['reason'],
      latencyMs: 0,
    };
    const finish = (answer: string, reason: AgentTurnLog['reason']): void => {
      log.answer = answer;
      log.reason = reason;
      log.latencyMs = Date.now() - startedAt;
      if (this.deps.logTurn) {
        try { this.deps.logTurn(log); } catch (e: unknown) { this.warn('LogicTEAM.007: журнал хода не записан:', e); }
      }
    };

    const system = await this.buildSystemPrompt();
    await this.compactHistoryIfNeeded(params.dialog);
    let transcript = this.serializeHistory(params.dialog);
    const seenCalls = new Map<string, number>();
    let lastToolName: string | null = null;
    let consecutiveToolCount = 0;
    let protocolRetries = 0;
    const wantsFile = asksForFile(lastUserText(params.dialog));
    let deliveredFile = false;
    let fileNudgeUsed = false;
    const aborted = (): boolean => Boolean(params.signal?.aborted);

    for (let i = 0; i < this.maxIterations; i++) {
      if (aborted()) {
        const answer = 'Остановлено пользователем.';
        this.deps.emit({ role: 'assistant', content: answer, created_at: this.now() });
        finish(answer, 'aborted');
        return;
      }
      this.rpm('Агент думает…');
      let turns: LlmTurn[];
      try {
        const raw = await this.deps.complete(system, transcript, params.model ? { model: params.model } : undefined);
        turns = parseTurns(raw);
      } catch (e: unknown) {
        if (aborted()) {
          const answer = 'Остановлено пользователем.';
          this.deps.emit({ role: 'assistant', content: answer, created_at: this.now() });
          finish(answer, 'aborted');
          return;
        }
        const answer = `Ошибка обращения к LLM: ${errorText(e)}`;
        this.deps.emit({ role: 'assistant', content: answer, created_at: this.now() });
        finish(answer, 'error');
        return;
      }

      const malformed = turns.find(t => t.type === 'protocol_error');
      if (malformed) {
        protocolRetries++;
        this.warn('LogicTEAM.007: ответ модели не в формате протокола:', malformed.raw.slice(0, 300));
        if (protocolRetries > PROTOCOL_RETRY_LIMIT) {
          const answer = await this.finishWithSynthesis(transcript, system, params, log,
            'Модель не смогла вернуть вызов инструмента в нужном формате.');
          finish(answer, 'protocol');
          return;
        }
        this.rpm('Переспрашиваю модель…');
        transcript += `\n[Система] ${PROTOCOL_REMINDER}\nТвой ход (только JSON):`;
        continue;
      }

      for (const turn of turns) {
        if (turn.type === 'protocol_error') continue; // отсеяно выше
        if (aborted()) {
          const answer = 'Остановлено пользователем.';
          this.deps.emit({ role: 'assistant', content: answer, created_at: this.now() });
          finish(answer, 'aborted');
          return;
        }
        if (turn.type === 'final') {
          if (wantsFile && !deliveredFile && !fileNudgeUsed) {
            fileNudgeUsed = true;
            this.rpm('Файл ещё не выдан — довожу ход…');
            transcript += `\n[Система] Пользователь просил ФАЙЛ (документ/PDF/вложение/картинку), но ты заканчиваешь ответом текстом и не выдал ни одного файла. `
              + 'Не заканчивай ход: если файл лежит в заметке базы знаний — сперва прочитай заметку (если её текст ещё не прочитан), затем выдай ссылку на вложение; '
              + 'если это документ из базы документов — возьми ссылку на файл (или прочитай его) по названию. '
              + 'Передай ссылку пользователю кнопкой в результате тула. Если файла в системе действительно нет — тогда прямо скажи об этом и заверши ход.'
              + '\nТвой ход (только JSON):';
            continue;
          }
          const answer = turn.text || 'Готово.';
          this.deps.emit({ role: 'assistant', content: answer, created_at: this.now() });
          finish(answer, 'final');
          return;
        }

        this.rpm(`Вызываю инструмент «${turn.tool || '…'}»…`);
        const tool = this.findTool(turn.tool || '');
        if (!tool) {
          this.deps.emit(this.toolMessage(turn.tool || '?', false, `Неизвестный инструмент «${turn.tool}»`));
          log.tools.push({ name: turn.tool || '?', arguments: {}, ok: false, summary: 'Неизвестный инструмент' });
          transcript += `\n[Результат тула ${turn.tool} (ошибка)] Неизвестный инструмент\nТвой ход (только JSON):`;
          continue;
        }

        const callKey = `${turn.tool}:${JSON.stringify(turn.arguments || {})}`;
        const count = (seenCalls.get(callKey) || 0) + 1;
        seenCalls.set(callKey, count);
        if (!IDEMPOTENT_TOOLS.has(turn.tool) && count > 2) {
          const answer = await this.finishWithSynthesis(transcript, system, params, log,
            `Защита от зацикливания: инструмент «${turn.tool}» вызван одинаково ${count} раз.`);
          finish(answer, 'synthesis');
          return;
        }

        if (turn.tool === lastToolName) {
          consecutiveToolCount++;
        } else {
          lastToolName = turn.tool;
          consecutiveToolCount = 1;
        }
        if (!IDEMPOTENT_TOOLS.has(turn.tool) && consecutiveToolCount > CONSECUTIVE_SAME_TOOL_LIMIT) {
          const answer = await this.finishWithSynthesis(transcript, system, params, log,
            `Не получилось подобрать нужный результат: инструмент «${turn.tool}» вызван ${consecutiveToolCount} раз подряд с разными параметрами.`);
          finish(answer, 'synthesis');
          return;
        }

        const result = await tool.execute(turn.arguments || {});
        if (result.ok && FILE_DELIVERY_TOOLS.has(turn.tool)) deliveredFile = true;
        log.tools.push({
          name: turn.tool,
          arguments: turn.arguments || {},
          ok: result.ok,
          summary: result.ok ? result.summary.slice(0, 500) : (result.error || 'ошибка').slice(0, 500),
        });
        this.deps.emit(this.toolMessage(turn.tool, result.ok, result.ok ? result.summary : (result.error || 'ошибка'), result.link));

        transcript += `\n${result.ok
          ? `[Результат тула ${turn.tool} (ok)] ${this.summaryForLlm(result.summary, result.data)}`
          : `[Результат тула ${turn.tool} (ошибка)] ${result.error || 'ошибка'}`}\nТвой ход (только JSON):`;
      }
    }

    const answer = await this.finishWithSynthesis(transcript, system, params, log,
      `Превышено число шагов агента (${this.maxIterations}).`);
    finish(answer, 'synthesis');
  }

  /** Остановка посреди работы — не обрыв «на полуслове»: модель получает ОДИН
   *  ход без тулов и собирает ответ из уже найденного. */
  private async finishWithSynthesis(
    transcript: string, system: string, params: RunAgentParams, log: { tools: AgentTurnLog['tools'] }, reason: string,
  ): Promise<string> {
    void log;
    if (params.signal?.aborted) return 'Остановлено пользователем.';
    const note = `\n[Система] ${reason} Дальнейшие вызовы инструментов недоступны. Сформулируй итоговый ответ пользователю ТОЛЬКО на основе данных, уже полученных выше в этом диалоге (результаты тулов) — не выдумывай новые данные. Если реально ничего полезного не найдено — честно скажи об этом и предложи, как уточнить запрос. Ответь обычным текстом, без JSON.\n`;
    try {
      const raw = await this.deps.complete(system, transcript + note, params.model ? { model: params.model } : undefined);
      const turns = parseTurns(raw);
      const finalTurn = turns.find(t => t.type === 'final');
      const answer = (finalTurn?.text || '').trim() || reason;
      this.deps.emit({ role: 'assistant', content: answer, created_at: this.now() });
      return answer;
    } catch (e: unknown) {
      this.deps.emit({ role: 'assistant', content: reason, created_at: this.now() });
      return reason;
    }
  }
}

/** Ошибку в текст: движок не знает про классы ошибок конкретного потребителя. */
function errorText(e: unknown): string {
  if (e instanceof Error) return e.message;
  return String(e);
}