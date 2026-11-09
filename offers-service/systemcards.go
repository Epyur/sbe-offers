package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	"mailers/offers/skills"
)

// Карточки систем из Базы знаний (шаг 7c плана 2026-10-29).
//
// Источник правды о тексте системы — заметка в базе знаний. Здесь:
//   - чтение карточек через kbclient.go,
//   - связь с каталогом по system_sku,
//   - кеш в offers_context, чтобы плагин получал список без похода в базу
//     знаний на каждый экран.
//
// Кеш НЕ источник правды: его всегда можно сбросить, и без базы знаний КП
// собирается как раньше — каталог и форма работают.

// --- Ручки для плагина -------------------------------------------------

// handleGetSystemCards — список карточек для раздела «Контекст КП».
//
// Отдаёт КЕШ, а не живой поход в базу знаний: экран плагина открывается часто,
// а карточки меняются редко. Обновление — отдельной ручкой.
func (s *Server) handleGetSystemCards(w http.ResponseWriter, r *http.Request) {
	cards, err := s.loadCachedSystemCards(r.Context())
	if err != nil {
		dbError(w, "get system cards", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cards":     cards,
		"kbEnabled": s.kb != nil && s.kb.configured(),
	})
}

// handleRefreshSystemCards — перечитать карточки из базы знаний (admin).
//
// Ошибка базы знаний возвращается полем `error`, но статус 200 и прежние
// карточки: плагин покажет предупреждение, а раздел не станет пустым.
func (s *Server) handleRefreshSystemCards(w http.ResponseWriter, r *http.Request) {
	email := actorEmailFromCtx(r)
	cards, err := s.refreshSystemCards(r.Context(), email)
	resp := map[string]any{
		"cards":     cards,
		"kbEnabled": s.kb != nil && s.kb.configured(),
		"count":     len(cards),
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleListSystems — системы из Базы знаний с составом (раздел «Системы»).
//
// Читает КЕШ, а не базу знаний (2026-11-01, требование пользователя: «загрузились
// и используем то, что знаем, при обновлении обновляем данные»).
//
// Раньше каждый показ раздела и каждый показ списка материалов ходили в
// kb-service вживую. Один такой заход — это сотни запросов (страницы ленты плюс
// полная заметка на каждую карточку), а любой сбой посреди захода показывал
// человеку «Ключ доступа недействителен» на пустом месте. Кеш от этого и
// страхует: он переживает и недоступную базу знаний, и неудачный токен.
//
// Кеш пуст — читаем базу знаний один раз и заполняем его; не вышло — пустой
// список и `error`, раздел честно скажет, что именно не так.
func (s *Server) handleListSystems(w http.ResponseWriter, r *http.Request) {
	cards, err := s.loadCachedSystems(r.Context())
	if err == nil && len(cards) == 0 {
		cards, err = s.refreshSystemsCache(r.Context(), "")
	}
	if err != nil {
		log.Printf("системы: кеш не прочитан: %v", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"systems":   cards,
		"kbEnabled": s.kb != nil && s.kb.configured(),
		"count":     len(cards),
		"error":     errorText(err),
	})
}

// handleRefreshSystems — перечитать системы из Базы знаний (admin).
//
// Ошибка базы знаний возвращается полем `error`, но статус 200 и прежний кеш:
// плагин покажет предупреждение, а раздел не станет пустым.
func (s *Server) handleRefreshSystems(w http.ResponseWriter, r *http.Request) {
	email := actorEmailFromCtx(r)
	cards, err := s.refreshSystemsCache(r.Context(), email)
	writeJSON(w, http.StatusOK, map[string]any{
		"systems":   cards,
		"kbEnabled": s.kb != nil && s.kb.configured(),
		"count":     len(cards),
		"error":     errorText(err),
	})
}

// errorText — текст ошибки для поля `error`; nil превращается в пустую строку.
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// SystemCard — карточка системы, как её видит offers-service.
type SystemCard struct {
	// NoteID — заметка в базе знаний. По ней открывается карточка в плагине.
	NoteID int64 `json:"noteId"`
	// Path — путь заметки в вольте: плагин показывает его человеку.
	Path string `json:"path"`
	// SKU — код системы из frontmatter; по нему связь с каталогом.
	SKU string `json:"sku"`
	// Title — название системы.
	Title string `json:"title"`
	// Description — описание системы.
	Description string `json:"description"`
	// Composition — состав (позиции без цен: цены в каталоге).
	Composition []string `json:"composition"`
	// CompositionLinks — тот же состав, разрешённый против справочника
	// материалов: строка, код из неё, найденный материал. Строка, которой нет
	// в справочнике, остаётся с Found=false и попадает в Problems.
	CompositionLinks []CompositionItem `json:"compositionLinks,omitempty"`
	// Problems — что не так с составом. Показывается человеку явно: молча
	// терять строку состава нельзя.
	Problems []string `json:"problems,omitempty"`
	// Specifications — таблица характеристик.
	Specifications [][]string `json:"specifications,omitempty"`
	// Refs — ссылки из раздела «Ссылки» карточки (2026-11-01): страница на
	// корпоративном сайте и техдокументация на навигаторе. Идут в конец КП.
	Refs []skills.CardRef `json:"refs,omitempty"`
	// LinkedRefs — ссылки материалов, на которые ведёт состав системы
	// (2026-11-09). Читаются на один уровень: разбирается строка состава,
	// находится материал по ссылке, берётся его раздел «Ссылки». В документ
	// идут вместе с Refs, повторы по адресу схлопываются. Пусто — у составных
	// материалов ссылок нет или карточка прочитана до этой правки.
	LinkedRefs []skills.CardRef `json:"linkedRefs,omitempty"`
	// ImageName — имя файла картинки в заметке.
	ImageName string `json:"imageName,omitempty"`
	// ImageKey — ключ вложения в хранилище Базы знаний. По нему картинка
	// читается для вставки в DOCX (FileLink отдаёт ссылку, а нам нужны байты).
	ImageKey string `json:"imageKey,omitempty"`
	// CatalogID — система каталога, с которой связана карточка. 0 — не связана.
	CatalogID int64 `json:"catalogId"`
	// UpdatedAt — когда заметку правили.
	UpdatedAt string `json:"updatedAt"`
	// HasDescription / HasComposition — есть ли обязательные блоки стандарта.
	HasDescription bool `json:"hasDescription"`
	HasComposition bool `json:"hasComposition"`
	// Empty — карточка без содержания.
	Empty bool `json:"empty"`
}

// CompositionItem — строка состава с разрешённым материалом.
//
// Raw — строка, как её написал человек. SKU — код из строки (пусто, если
// ссылку не проставили). Found — материал нашёлся в справочнике; тогда
// заполнены MaterialID, Name, Unit и Ekn. Ekn нужен, чтобы плагин мог собрать
// позицию по конкретному варианту, не делая ещё один запрос.
type CompositionItem struct {
	Raw string `json:"raw"`
	// Wiki — цель вики-ссылки из строки (`[[Название]]`), пусто, если ссылки
	// нет. По ней ищется материал: в живых карточках кода нет, а ссылка есть.
	Wiki       string   `json:"wiki,omitempty"`
	SKU        string   `json:"sku,omitempty"`
	MaterialID int64    `json:"materialId"`
	Name       string   `json:"name,omitempty"`
	Unit       string   `json:"unit,omitempty"`
	Ekn        []string `json:"ekn,omitempty"`
	Found      bool     `json:"found"`
	// Layer — строка называет слой или работу, а не материал: «Подготовка
	// основания», «Уплотнённый грунт». Материала у неё нет и не будет, поэтому
	// в Problems она не попадает — иначе у живых карточек (1273 такие строки)
	// каждая система помечалась бы «есть ошибки в составе» (2026-11-02).
	Layer bool `json:"layer,omitempty"`
}

// resolveComposition разрешает строки состава против справочника материалов.
//
// Слой — не проблема: «Подготовка основания», «Уплотнённый грунт» называют
// работу, а не товар, и материалом никогда не станут. Такая строка помечается
// Layer и в замечания не идёт (2026-11-02: 1273 такие строки на живых
// карточках помечали КАЖДУЮ систему «есть ошибки в составе», хотя ни одна не
// была сломана). Замечание остаётся для того, что действительно можно
// исправить: ссылка ведёт не туда, код незнаком, материал снят с продажи.
func resolveComposition(card skills.SystemCard, materials []Material) ([]CompositionItem, []string) {
	links := card.Links
	// Старый кеш карточки мог не нести Links: тогда разбираем строки сами.
	if len(links) == 0 && len(card.Composition) > 0 {
		links = make([]skills.CompositionLink, 0, len(card.Composition))
		for _, raw := range card.Composition {
			links = append(links, skills.CompositionLink{Raw: raw, SKU: skills.ExtractSku(raw)})
		}
	}
	if len(links) == 0 {
		return []CompositionItem{}, []string{"состав системы не заполнен"}
	}

	// Архивные материалы тоже в картах: ссылка на снятый с продажи материал —
	// отдельная причина, и она понятнее, чем «не найден».
	bySku := make(map[string]Material, len(materials))
	byNote := make(map[string]Material, len(materials))
	for _, m := range materials {
		bySku[strings.ToUpper(strings.TrimSpace(m.SKU))] = m
		if key := normalizeWikiTarget(m.NoteTitle); key != "" {
			byNote[key] = m
		}
	}

	items := make([]CompositionItem, 0, len(links))
	problems := make([]string, 0)
	for _, link := range links {
		item := CompositionItem{Raw: link.Raw, SKU: link.SKU}
		item.Wiki = wikiTarget(link.Raw)

		// Ссылка — главный способ связи (2026-10-31): её ставил человек руками
		// на нужную заметку. Код остаётся запасным, если ссылки нет.
		m, resolvedBy := Material{}, ""
		if item.Wiki != "" {
			m, resolvedBy = byNote[item.Wiki], "ссылке"
		}
		if resolvedBy == "" && strings.TrimSpace(link.SKU) != "" {
			m, resolvedBy = bySku[strings.ToUpper(strings.TrimSpace(link.SKU))], "коду"
		}
		if resolvedBy == "" {
			if item.Layer = isLayerLine(link.Raw); !item.Layer {
				problems = append(problems, fmt.Sprintf("%q: не указан ни код, ни ссылка на материал", shortenRaw(link.Raw)))
			}
			items = append(items, item)
			continue
		}
		if m.ID == 0 {
			items = append(items, item)
			problems = append(problems, notFoundProblem(item, resolvedBy))
			continue
		}
		if m.Archived {
			items = append(items, item)
			problems = append(problems, fmt.Sprintf("%s: материал удалён из справочника", m.Name))
			continue
		}
		item.MaterialID, item.Name, item.Unit, item.Found = m.ID, m.Name, m.Unit, true
		item.Ekn = materialEknNumbers(m)
		items = append(items, item)
	}
	return items, problems
}

// notFoundProblem — почему строка не связалась. Называет то, чем искали:
// «не найден материал» без указания, по чему искали, отправляет человека
// проверять не то место.
func notFoundProblem(item CompositionItem, resolvedBy string) string {
	if resolvedBy == "ссылке" {
		return fmt.Sprintf("[[%s]]: по ссылке материал не найден в справочнике", shortenRaw(item.Wiki))
	}
	return fmt.Sprintf("%s: материал не найден в справочнике", item.SKU)
}

// reWorkWord — слово, по которому видно работу, а не товар.
//
// Границы заданы явно: `\b` в Go считается по ASCII, и на кириллице он не
// находит ничего (грабля ловилась трижды — см. AGENTS.md).
var reWorkWord = regexp.MustCompile(`(?:^|[^А-ЯЁA-Z])(?:подготовк|устройств|уплотнени|разборк|демонтаж|монтаж|укладк|нанесени|грунтовк|очистк|выравнивани|засыпк|испытани|работ)`)

// isLayerLine — строка называет слой или работу, а не материал.
//
// Проверяется только строка БЕЗ кода и БЕЗ ссылки: если человек проставил
// ссылку или код, это его утверждение о материале, и спорить с ним нечем —
// несвязанная ссылка остаётся замечанием. Здесь же решается лишь одно:
// ругать ли карточку за строку, которая материалом никогда не была.
//
// Формулировка продиктована живыми данными (2026-11-02): на 373 карточках
// 1273 строки без ссылки, и все они — слои и общестроительные работы.
func isLayerLine(raw string) bool {
	if wikiTarget(raw) != "" || strings.TrimSpace(skills.ExtractSku(raw)) != "" {
		return false
	}
	return reWorkWord.MatchString(strings.ToLower(raw))
}

// materialEknNumbers — номера ЕКН материала в порядке справочника.
func materialEknNumbers(m Material) []string {
	if len(m.Ekn) == 0 {
		return nil
	}
	out := make([]string, 0, len(m.Ekn))
	for _, e := range m.Ekn {
		out = append(out, e.Ekn)
	}
	return out
}

// shortenRaw обрезает длинную строку состава для текста проблемы: человеку
// нужен узнаваемый кусок, а не вся строка в списке замечаний.
func shortenRaw(raw string) string {
	runes := []rune(strings.TrimSpace(raw))
	if len(runes) <= 60 {
		return string(runes)
	}
	return string(runes[:60]) + "…"
}

// SystemsFromKB — системы из Базы знаний с составом, разрешённым против
// материалов. Материальные карточки в список не попадают: у них свой раздел.
//
// kb == nil — «база знаний не настроена»: пустой список без ошибки.
func SystemsFromKB(ctx context.Context, kb KBReader, materials []Material) ([]SystemCard, error) {
	if kb == nil {
		return []SystemCard{}, nil
	}
	tagID, err := kb.TagID(ctx, commercialPropose)
	if err != nil {
		return []SystemCard{}, err
	}
	notes, err := kb.Notes(ctx, tagID)
	if err != nil {
		return []SystemCard{}, err
	}
	// Деление на системы и материалы — по тегам ПОЛНОЙ заметки, а не элемента
	// ленты (2026-10-31). Живой дефект: kb-service в списке `/api/kb/notes`
	// тегов не отдаёт вовсе, `splitCardsByTags` видел пустой список и записывал
	// ВСЕ 1541 карточку в системы — включая 1165 материалов. Проверено на бою:
	// в списке ключей нет `tags`, в полной заметке `tags` есть.
	//
	// Цена — один запрос заметки на карточку; он всё равно нужен ради текста.
	cards := make([]SystemCard, 0, len(notes))
	for _, summary := range notes {
		note, err := kb.Note(ctx, summary.ID)
		// Одна сломанная карточка не скрывает остальные.
		if err != nil {
			continue
		}
		// Тег берём из полной заметки: в ленте его нет.
		if noteHasTag(KBNoteSummary{Tags: note.Tags}, materialTagName) {
			continue // это материал, а не система
		}
		parsed := skills.ParseSystemCard(note.ContentMD)
		card := systemCardFromParsed(summary, note, parsed)
		card.CompositionLinks, card.Problems = resolveComposition(parsed, materials)
		cards = append(cards, card)
	}
	// Картинки — как в SystemCardsFromKB и SystemsWithCompositionFromKB: без
	// ключа вложения метка `{{система}}` не находит байтов, и карточка печатается
	// без картинки. Живая жалоба 2026-11-07.
	return attachImageKeys(ctx, kb, cards), nil
}

// SystemWithComposition — карточка системы вместе с разрешённым составом.
//
// Отдельный тип, а не поля в SystemCard: `SystemCard` уходит в «Контекст КП»
// и в кеш карточек, где состава нет вовсе. Общий тип заставлял бы кеш карточек
// нести пустые поля состава и путать два раздела.
type SystemWithComposition struct {
	SystemCard
	CompositionLinks []CompositionItem `json:"compositionLinks,omitempty"`
	Problems         []string          `json:"problems,omitempty"`
}

// IsEmpty — карточка без содержания: ни описания, ни состава.
//
// Метод, а не поле: у `SystemCard` уже есть поле `Empty`, и одноимённый метод
// на нём не скомпилируется.
func (c SystemCard) IsEmpty() bool {
	return c.Empty
}

// SystemsWithCompositionFromKB — системы из Базы знаний с составом,
// разрешённым против справочника материалов. Материальные карточки сюда не
// попадают: у них свой раздел.
//
// kb == nil — «база знаний не настроена»: пустой список без ошибки.
func SystemsWithCompositionFromKB(ctx context.Context, kb KBReader, materials []Material) ([]SystemWithComposition, error) {
	if kb == nil {
		return []SystemWithComposition{}, nil
	}
	tagID, err := kb.TagID(ctx, commercialPropose)
	if err != nil {
		return []SystemWithComposition{}, err
	}
	notes, err := kb.Notes(ctx, tagID)
	if err != nil {
		return []SystemWithComposition{}, err
	}
	cards := make([]SystemWithComposition, 0, len(notes))
	for _, summary := range notes {
		note, err := kb.Note(ctx, summary.ID)
		// Одна сломанная карточка не скрывает остальные.
		if err != nil {
			continue
		}
		// Тег берём из полной заметки: в ленте его нет. Материал в этом
		// разделе — не система.
		if noteHasTag(KBNoteSummary{Tags: note.Tags}, materialTagName) {
			continue
		}
		parsed := skills.ParseSystemCard(note.ContentMD)
		card := SystemWithComposition{SystemCard: systemCardFromParsed(summary, note, parsed)}
		card.CompositionLinks, card.Problems = resolveComposition(parsed, materials)
		cards = append(cards, card)
	}
	// Картинки — так же, как в SystemCardsFromKB: раздел «Системы» показывает
	// карточку с картинкой, и ключ должен быть уже разрешён.
	withKeys := attachImageKeys(ctx, kb, systemCardsOf(cards))
	for i := range cards {
		cards[i].SystemCard = withKeys[i]
	}
	return cards, nil
}

// systemCardsOf — карточки из списка систем с составом (для общей привязки
// картинок, которая знает только SystemCard).
func systemCardsOf(cards []SystemWithComposition) []SystemCard {
	out := make([]SystemCard, len(cards))
	for i, card := range cards {
		out[i] = card.SystemCard
	}
	return out
}

// systemCardFromParsed собирает карточку для плагина из разобранной заметки.
func systemCardFromParsed(summary KBNoteSummary, note KBNote, parsed skills.SystemCard) SystemCard {
	title := parsed.Title
	// Заголовок заметки — запасной источник названия: разбор карточки требует
	// оформления по стандарту, а в живых карточках H1 часто отсутствует.
	// Без этого все системы назывались бы «(без названия)».
	if strings.TrimSpace(title) == "" {
		title = summary.Title
	}
	return SystemCard{
		NoteID:         summary.ID,
		Path:           note.Path,
		SKU:            parsed.SKU,
		Title:          title,
		Description:    parsed.Description,
		Composition:    parsed.Composition,
		Specifications: parsed.Specifications,
		Refs:           parsed.Refs,
		ImageName:      parsed.ImageName,
		UpdatedAt:      note.UpdatedAt,
		HasDescription: strings.TrimSpace(parsed.Description) != "",
		HasComposition: len(parsed.Composition) > 0,
		Empty:          parsed.Empty(),
	}
}

// KBReader — то, что нужно от базы знаний. Интерфейс, а не *KBClient: тесты
// подставляют свою реализацию и не ходят по сети.
type KBReader interface {
	TagID(ctx context.Context, name string) (int64, error)
	Notes(ctx context.Context, tagID int64) ([]KBNoteSummary, error)
	Note(ctx context.Context, id int64) (KBNote, error)
	Attachments(ctx context.Context, noteID int64) ([]KBAttachment, error)
	FileLink(ctx context.Context, key string) (string, error)
}

// SystemCardsFromKB читает карточки и связывает их с каталогом по system_sku.
//
// kb == nil означает «база знаний не настроена»: пустой список без ошибки.
// Ошибка базы знаний возвращается наверх вместе с пустым списком — вызывающий
// решает, показать предупреждение или работать без карточек.
func SystemCardsFromKB(ctx context.Context, kb KBReader, catalog []CatalogItem) ([]SystemCard, error) {
	if kb == nil {
		return []SystemCard{}, nil
	}
	tagID, err := kb.TagID(ctx, commercialPropose)
	if err != nil {
		return []SystemCard{}, err
	}
	notes, err := kb.Notes(ctx, tagID)
	if err != nil {
		return []SystemCard{}, err
	}
	cards := make([]SystemCard, 0, len(notes))
	for _, summary := range notes {
		note, err := kb.Note(ctx, summary.ID)
		// Одна сломанная карточка не должна скрывать остальные.
		if err != nil {
			continue
		}
		parsed := skills.ParseSystemCard(note.ContentMD)
		card := systemCardFromParsed(summary, note, parsed)
		card.CatalogID = linkCatalogID(card.SKU, catalog)
		cards = append(cards, card)
	}
	// Картинки карточек (2026-11-05): по имени файла из `![[…]]` находим
	// вложение и запоминаем его ключ. Без этого узел «картинка системы» не
	// нашёл бы байтов: ключ вложения — не имя файла в заметке.
	return attachImageKeys(ctx, kb, cards), nil
}

// linkCatalogID ищет систему каталога по коду. Только по коду, не по имени:
// имя правится свободно, а код заводится один раз и не меняется.
func linkCatalogID(sku string, catalog []CatalogItem) int64 {
	sku = strings.TrimSpace(sku)
	if sku == "" {
		return 0
	}
	for _, item := range catalog {
		if strings.EqualFold(strings.TrimSpace(item.SKU), sku) {
			return item.ID
		}
	}
	return 0
}

// FindSystemCard — карточка по коду системы (для сборки документа и добавления
// состава в КП). Тонкая обёртка: правило сравнения кодов живёт в одном месте,
// иначе поиск в документе и поиск в редакторе разошлись бы в мелочах.
func FindSystemCard(cards []SystemCard, sku string) (SystemCard, bool) {
	return FindSystem(cards, ParseSystemChoice(sku))
}

// --- Кеш систем в offers_context ---------------------------------------

// systemsCacheMode — ключ строки кеша раздела «Системы». Отдельно от
// system_cards: там карточки для «Контекста КП» с привязкой к каталогу, здесь —
// системы с составом, разрешённым против справочника материалов.
const systemsCacheMode = "systems"

// loadCachedSystems читает системы из кеша. Нет строки — пустой список.
func (s *Server) loadCachedSystems(ctx context.Context) ([]SystemWithComposition, error) {
	var raw []byte
	if err := s.pool.QueryRow(ctx,
		`SELECT sections FROM offers_context WHERE mode = $1`, systemsCacheMode).Scan(&raw); err != nil {
		// Строки нет — это не авария: кеш пуст, его заполнит первый показ.
		return []SystemWithComposition{}, nil
	}
	var out []SystemWithComposition
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("кеш систем повреждён: %w", err)
	}
	if out == nil {
		return []SystemWithComposition{}, nil
	}
	return out, nil
}

// saveCachedSystems пишет системы в кеш. Строка одна: это кеш, а не набор.
func (s *Server) saveCachedSystems(ctx context.Context, systems []SystemWithComposition, updatedBy string) error {
	if systems == nil {
		systems = []SystemWithComposition{}
	}
	raw, err := json.Marshal(systems)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO offers_context (mode, sections, learnings, updated_by, updated_at)
VALUES ($1, $2, '[]', $3, now())
ON CONFLICT (mode) DO UPDATE SET
	sections = EXCLUDED.sections, updated_by = EXCLUDED.updated_by, updated_at = now()`,
		systemsCacheMode, raw, updatedBy)
	return err
}

// refreshSystemsCache читает системы из базы знаний и обновляет кеш.
//
// Ошибка базы знаний НЕ стирает прежний кеш: временный сбой не должен
// выключать раздел. Возвращается прежний кеш и ошибка — вызывающий сам решает,
// показать предупреждение или молчать.
func (s *Server) refreshSystemsCache(ctx context.Context, updatedBy string) ([]SystemWithComposition, error) {
	// Справочник берём без синхронизации с карточками: она сама ходит в базу
	// знаний, а второй заход туда же нужен только для повтора той же ошибки.
	materials, err := s.listMaterialsOnce(ctx, false)
	if err != nil {
		return nil, err
	}
	fresh, err := SystemsWithCompositionFromKB(ctx, s.kb, materials)
	if err != nil {
		cached, _ := s.loadCachedSystems(ctx)
		return cached, err
	}
	if err := s.saveCachedSystems(ctx, fresh, updatedBy); err != nil {
		return fresh, err
	}
	return fresh, nil
}

// --- Кеш карточек в offers_context -------------------------------------

// loadCachedSystemCards читает карточки из кеша. Нет строки — пустой список.
func (s *Server) loadCachedSystemCards(ctx context.Context) ([]SystemCard, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx,
		`SELECT sections FROM offers_context WHERE mode = 'system_cards'`).Scan(&raw)
	if err != nil {
		return []SystemCard{}, nil
	}
	return unmarshalSystemCards(raw)
}

// saveCachedSystemCards пишет карточки в кеш. Строка одна (mode =
// 'system_cards'): это кеш, а не набор записей.
func (s *Server) saveCachedSystemCards(ctx context.Context, cards []SystemCard, updatedBy string) error {
	raw, err := marshalSystemCards(cards)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO offers_context (mode, sections, learnings, updated_by, updated_at)
VALUES ('system_cards', $1, '[]', $2, now())
ON CONFLICT (mode) DO UPDATE SET
	sections = EXCLUDED.sections, updated_by = EXCLUDED.updated_by, updated_at = now()`,
		raw, updatedBy)
	return err
}

// refreshSystemCards читает карточки из базы знаний и обновляет кеш.
//
// Ошибка базы знаний НЕ стирает прежний кеш: временный сбой не должен
// выключать описания во всех КП. Возвращается прежний кеш и ошибка — вызывающий
// сам решает, показать предупреждение или молчать.
func (s *Server) refreshSystemCards(ctx context.Context, updatedBy string) ([]SystemCard, error) {
	catalog, _ := s.listCatalogItems(ctx, true)
	fresh, err := SystemCardsFromKB(ctx, s.kb, catalog)
	if err != nil {
		cached, _ := s.loadCachedSystemCards(ctx)
		return cached, err
	}
	// Ссылки материалов состава (2026-11-09) читаются здесь, а не при печати:
	// сборка документа смотрит только кеш. Справочник берём без синхронизации
	// с карточками — она уже прошла выше; нужен он лишь чтобы по названию
	// заметки найти материал и его note_id.
	if materials, mErr := s.listMaterialsOnce(ctx, false); mErr == nil {
		fresh = enrichLinkedRefs(ctx, s.kb, fresh, materials)
	} else {
		log.Printf("ссылки материалов: справочник не прочитан: %v", mErr)
	}
	if err := s.saveCachedSystemCards(ctx, fresh, updatedBy); err != nil {
		return fresh, err
	}
	return fresh, nil
}

// PromptText — текст карточки для промпта модели: название, описание, состав.
// Таблица характеристик сюда не идёт: модели нужно то, о чём писать.
//
// Формат текста живёт в skills — там же, где разбор карточки. Сервисная копия
// дублировала его символ в символ: правка в одном месте молча оставляла бы
// вторую старую, и промпт собирался бы из двух разных шаблонов.
func (c SystemCard) PromptText() string {
	return skills.SystemCard{
		Title:       c.Title,
		Description: c.Description,
		Composition: c.Composition,
	}.PromptText()
}

// marshalSystemCards — кеш в JSONB.
func marshalSystemCards(cards []SystemCard) ([]byte, error) {
	if cards == nil {
		cards = []SystemCard{}
	}
	return json.Marshal(cards)
}

// unmarshalSystemCards читает кеш. Старый формат (ProductContext с разделами)
// не ломает чтение: карточек в нём нет, и это нормально.
func unmarshalSystemCards(raw []byte) ([]SystemCard, error) {
	if len(raw) == 0 {
		return []SystemCard{}, nil
	}
	var cards []SystemCard
	if err := json.Unmarshal(raw, &cards); err == nil && cards != nil {
		return cards, nil
	}
	return []SystemCard{}, nil
}
