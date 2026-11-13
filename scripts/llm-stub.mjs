/**
 * Заглушка llm-service для локального стенда (2026-10-30).
 *
 * Отвечает на ту же ручку, что настоящий сервис:
 *
 *   POST /internal/llm/complete  {system, user, max_tokens}
 *     - X-Service-Secret обязателен;
 *     - система содержит «JSON» (шаг брифа) → ответ — {"payload": {…}};
 *     - иначе (описания секций)             → ответ — обычный текст;
 *     - в ПОЛЬЗОВАТЕЛЬСКОМ промпте есть метка STUB_FAIL → ответ 502,
 *       чтобы проверить деградацию. Метка в тексте, а не в заголовке: сервис
 *       не пересылает произвольные заголовки, и проверять надо то, что он
 *       действительно отправляет.
 *
 * Сеть и провайдер стенду не нужны: проверяется поведение offers-service,
 * а не модель. Ответы нарочно простые — важно, что сервис их разбирает.
 */
import http from 'node:http';

const PORT = Number(process.env.LLM_PORT ?? 3000);
const SECRET = process.env.LLM_SERVICE_SECRET ?? '';

const FAIL_MARKER = 'STUB_FAIL';

const BRIEF_REPLY = JSON.stringify({
  payload: {
    object_name: 'ЖК «Поэма»',
    customer_goals: ['ввод в эксплуатацию в срок'],
    customer_pains: ['срывы сроков подрядчиками'],
    project_risks: [],
    budget_signal: '',
    decision_maker: 'главный инженер',
    timeline: 'до конца квартала',
    why_us: '',
    objections: [],
    notes: 'ответ заглушки',
    skipped_topics: [],
  },
  reply: 'Записал. Расскажите про боли заказчика.',
  skip: [],
});

const COPY_REPLY =
  'Террасы и кровли «Поэма» получают систему, рассчитанную на нагрузку ' +
  'эксплуатации и на срок ввода объекта.';

// Ответ на промт макета (2026-11-06, 0.7.5): проверяется, что печать по макету
// делает адрес кликабельным и поднимает степень в единицах. Заглушка обязана
// содержать и адрес, и «м2» — иначе проверка стенда ничего не значит.
// Заголовок и список — тоже нарочно: печать по макету обязана превратить их
// в заголовок бланка и настоящий список.
const LAYOUT_REPLY = [
  '## Горизонтальная часть',
  'Площадь горизонтали 4800 м2, вертикали 1400 м2.',
  '- двойной сварной шов с контролем герметичности',
  '- ремонт секции без вскрытия соседних',
  'Технический лист: https://nav.tn.ru/systems/fundament/tn-fundament-drenazh-prof/',
].join('\n');

// Тексты системы (2026-11-07).
const SYSTEM_TEXT_DESCRIPTION =
  'Система закрывает подземный контур дома одной гидроизоляционной картой, ' +
  'швы свариваются горячим воздухом с воздушным каналом для контроля.';

const SYSTEM_TEXT_ADVANTAGES =
  'Проверяемая герметичность каждого шва, ремонт секции без вскрытия соседних, ' +
  'срок службы не менее пятидесяти лет.';

// «Опиши блок словами» (2026-11-05): модель отдаёт узлы формы, а не бриф.
// Отдельный ответ, потому что запрос тоже JSON, но структура другая —
// без этого стенд получал бриф вместо блока и describe отвечал 502.
const DESCRIBE_REPLY = JSON.stringify({
  title: 'Условия поставки',
  nodes: [
    { type: 'heading', text: 'Условия поставки', level: 3 },
    { type: 'bullet_list', items: ['Доставка на объект включена', 'Оплата по счёту', 'Гарантия 5 лет'] },
  ],
  notes: [],
});

// reply выбирает ответ по ОБОИМ половинам запроса.
//
// Почему не по одной системе (2026-11-08). Признак «бриф» искали словом «JSON»
// в системном промпте, а слово это живёт в user-промпте («ТЕКУЩИЙ БРИФ (JSON):
// …»); в системе стоит «ОТВЕТ ФОРМАТОМ JSON». В итоге на шаг брифа заглушка
// отвечала текстом копирайтера, разбор ответа падал, сервис отвечал «Не
// разобрал ответ», и стенд краснел на проверке «ответ менеджера попал в бриф».
// Правило: признак берётся там, где сервис его ДЕЙСТВИТЕЛЬНО пишет.
function reply(system, user) {
  // Помощник КП (2026-11-14, шаг P6): узнаётся по системному промту агента.
  // Первый ход — вызов поиска; второй (в транскрипте уже есть «Результат тула»)
  // — финальный ответ. Так стенд проходит настоящий цикл: инструмент вызван,
  // его результат вернулся модели, ход закончился текстом.
  // Помощник КП: варианты разбираются РАНЬШЕ общего (2026-11-14, P6-бис).
  // Общий адресат ниже отвечает всегда вызовом `get_our_products`, и пока он
  // стоял первым, узкие сценарии до него не доходили — стенд звал поиск на
  // «подбери аналог» и читал результат как «карточки нет».
  //
  // «Подбери аналог» — карточка материала (read_card по материалу — живой
  // дефект 0.9.22) и поиск конкурентов.
  if (/Ты — помощник менеджера отдела продаж/.test(system) && /подбери аналог/i.test(user)) {
    if (/\[Результат тула read_card/.test(user)) {
      return JSON.stringify({ type: 'tool_call', tool: 'search_competitors', arguments: { query: 'мембрана' } });
    }
    if (/\[Результат тула search_competitors/.test(user)) {
      return JSON.stringify({ type: 'final', text: 'Карточка материала прочитана, конкуренты найдены.' });
    }
    return JSON.stringify({ type: 'tool_call', tool: 'read_card', arguments: { noteId: 103 } });
  }
  // Боевой листок — завести и положить наш продукт. Сторона берётся из ответа
  // инструмента: первый ход `create_combat_sheet`, второй `set_combat_side`,
  // третий `final`.
  if (/Ты — помощник менеджера отдела продаж/.test(system) && /сравнение с конкурент/i.test(user)) {
    if (/\[Результат тула set_combat_side/.test(user)) {
      return JSON.stringify({ type: 'final', text: 'Листок заведён, наш продукт добавлен.' });
    }
    if (/\[Результат тула create_combat_sheet/.test(user)) {
      const id = (user.match(/\(id (\d+)\)/) ?? [])[1] ?? '0';
      return JSON.stringify({ type: 'tool_call', tool: 'set_combat_side', arguments: { sheetId: Number(id), noteId: 105, side: 'our' } });
    }
    return JSON.stringify({ type: 'tool_call', tool: 'create_combat_sheet', arguments: { title: 'Сравнение с конкурентами' } });
  }
  // Сценарий B: без открытого документа агент заводит КП сам и кладёт систему —
  // раньше он упирался в «предложение не найдено».
  if (/Ты — помощник менеджера отдела продаж/.test(system) && /собери кп/i.test(user)) {
    if (/\[Результат тула set_system/.test(user)) {
      const offerID = (user.match(/\(id (\d+)\)/) ?? [])[1] ?? '0';
      return JSON.stringify({ type: 'tool_call', tool: 'build_docx', arguments: { offerId: Number(offerID) } });
    }
    if (/\[Результат тула create_offer/.test(user)) {
      const id = (user.match(/\(id (\d+)\)/) ?? [])[1] ?? '0';
      return JSON.stringify({ type: 'tool_call', tool: 'set_system', arguments: { offerId: Number(id), noteId: 105 } });
    }
    if (/\[Результат тула build_docx/.test(user)) {
      return JSON.stringify({ type: 'final', text: 'КП собрано: система добавлена, документ готов.' });
    }
    return JSON.stringify({ type: 'tool_call', tool: 'create_offer', arguments: { clientName: 'ООО «Тест»' } });
  }
  if (/Ты — помощник менеджера отдела продаж/.test(system) && /письм/i.test(user)) {
    const asked = (user.match(/письм[аоы]?\s+(\d{3,})/i) ?? [])[1];
    if (/\[Результат тула read_email/.test(user)) {
      return JSON.stringify({ type: 'final', text: 'Письмо прочитано, сканы доступны ссылкой.' });
    }
    if (/\[Результат тула get_emails/.test(user)) {
      const id = (user.match(/"id":\s*(\d+)/) ?? [])[1] ?? '0';
      if (id === '0') {
        return JSON.stringify({ type: 'final', text: 'Писем, относящихся к продуктам, не нашлось.' });
      }
      return JSON.stringify({ type: 'tool_call', tool: 'read_email', arguments: { id: Number(id) } });
    }
    // «найди письма 862» — человек назвал номер: читаем именно его. Так стенд
    // проверяет, что письмо без карточки скана НЕ открывается: ручка отвечает
    // 404, и инструмент отказывает вместо пересказа пустоты.
    if (asked) {
      return JSON.stringify({ type: 'tool_call', tool: 'read_email', arguments: { id: Number(asked) } });
    }
    return JSON.stringify({
      type: 'tool_call',
      tool: 'get_emails',
      arguments: { product: 'LOGICROOF V-RP' },
    });
  }
  if (/Ты — помощник менеджера отдела продаж/.test(system)) {
    if (/\[Результат тула /.test(user)) {
      return JSON.stringify({ type: 'final', text: 'Нашёл в Базе знаний, что можно предложить.' });
    }
    const query = (user.match(/\[Пользователь\]\s*(.+)/) ?? [])[1]?.trim() ?? '';
    return JSON.stringify({ type: 'tool_call', tool: 'get_our_products', arguments: { query } });
  }
  // Боевой листок (2026-11-12): сравнение с уточнением менеджера.
  //
  // Заглушка обязана ОТРАЖАТЬ уточнение: сервер кэширует готовый DOCX по хэшу,
  // в который входит уточнение, и стенд проверяет, что файл пересобирается. С
  // одинаковым ответом на любой уточнение проверка ничего бы не доказывала.
  if (/аналитик коммерческой службы/.test(system)) {
    const hint = (user.match(/уточнение менеджера\):\s*(.+)/) ?? [])[1] ?? 'без акцента';
    return [
      '## Общее сравнение',
      `Разбор сделан с акцентом: ${hint}. Наш продукт и продукты конкурентов — разные решения одной задачи.`,
      '## По каждому продукту',
      '### Продукт конкурента',
      'Отличается составом и условиями применения; уточните у заказчика условия эксплуатации.',
    ].join('\n');
  }
  // Шаг брифа — по началу user-промпта (buildBriefUserPrompt, briefstep.go).
  if (/^ТЕКУЩИЙ БРИФ \(JSON\):/m.test(user)) return BRIEF_REPLY;
  // Промпт «опиши блок словами» проверяется раньше общего текста блока: он
  // тоже говорит про блок КП, и общий признак «БЛОК КОММЕРЧЕСКОГО
  // ПРЕДЛОЖЕНИЯ» ловил бы его раньше своего адресата. Из-за этого стенд отдавал
  // узлы формы вместо текста, и печать по макету получала пустой блок
  // (поймано разделом 22, 2026-11-06).
  if (/ПЕРЕВОДИШЬ ОПИСАНИЕ АДМИНИСТРАТОРА/.test(system)) return DESCRIBE_REPLY;
  // Тексты системы проверяются ПЕРЕД общим промтом блока: инструкцию обоих
  // собирает один RenderBlockSystem, и «ПО ПОДСКАЗКЕ АДМИНИСТРАТОРА» есть в
  // обоих. Признак — первые слова инструкции (systemtext.go), они уникальны.
  if (/задача заказчика/i.test(system)) return SYSTEM_TEXT_DESCRIPTION;
  if (/преимущества системы/i.test(system)) return SYSTEM_TEXT_ADVANTAGES;
  if (/ПО ПОДСКАЗКЕ АДМИНИСТРАТОРА/.test(system)) return LAYOUT_REPLY;
  // Остальные структурированные ответы — по системному промпту: так и работает
  // сопоставление «строка таблицы → материал справочника» (matcher.go).
  return /JSON/.test(system) ? BRIEF_REPLY : COPY_REPLY;
}

http
  .createServer((req, res) => {
    const send = (status, body) => {
      res.writeHead(status, { 'Content-Type': 'application/json' });
      res.end(typeof body === 'string' ? body : JSON.stringify(body));
    };
    if (req.method !== 'POST' || req.url !== '/internal/llm/complete') {
      send(404, { error: 'not found' });
      return;
    }
    if (!SECRET || req.headers['x-service-secret'] !== SECRET) {
      send(403, { error: 'forbidden' });
      return;
    }
    const chunks = [];
    req.on('data', (c) => chunks.push(c));
    req.on('end', () => {
      let system = '';
      let user = '';
      try {
        const body = JSON.parse(Buffer.concat(chunks).toString('utf8'));
        system = String(body.system ?? '');
        user = String(body.user ?? '');
      } catch {
        send(400, { error: 'invalid json' });
        return;
      }
      if (user.includes(FAIL_MARKER)) {
        send(502, { error: 'заглушка: ошибка провайдера по метке' });
        return;
      }
      if (process.env.LLM_STUB_LOG === '1') {
        const content = reply(system, user);
        console.log(`→ system: ${system.slice(0, 80)}\n→ user: ${user.slice(0, 120)}\n← content: ${content.slice(0, 100)}`);
      }
      send(200, { content: reply(system, user) });
    });
  })
  .listen(PORT, () => console.log(`llm-stub слушает :${PORT}`));
