package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// Выбор системы в предложение (Task 7, 2026-10-31; переработка 2026-11-03).
//
// Выбор системы НЕ дописывает позиции. Он говорит, какая система предложена:
// отсюда в КП попадают обоснование применения, преимущества и описание
// основных продуктов (раздел «Системы» и карточка системы при печати).
// Позиции менеджер заводит сам — файлом или руками.
//
// Почему так (КП 003/2026, 2026-11-03): состав системы дописывался к позициям
// из таблицы менеджера, и в таблице оказывались одни и те же продукты дважды —
// с ценой из таблицы и с нулём из системы. Сматчить их нечем: названия в
// карточке и в таблице пишутся по-разному, а собранный состав был неотличим от
// остальных позиций. Теперь позиций от выбора системы в КП не появляется вовсе.
//
// Правило у плагина и у бота ОДНО — assembleSystem; разница только во входе
// (JWT против секрета сервиса с chat_id) и в том, кто отвечает за ошибку.

// ComposeSystemResponse — какая система выбрана и что о ней известно.
type ComposeSystemResponse struct {
	Offer Offer `json:"offer"`
	// System — выбранная карточка: её текст и печатается в КП.
	System SystemCard `json:"system"`
	// Title — название системы строкой: клиентам (плагин, бот) оно нужно
	// для ответа человеку, и вытаскивать его из карточки незачем.
	Title string `json:"title"`
	// Composition — сколько основных продуктов в составе системы. Позиций в
	// КП это не прибавляет: число нужно, чтобы человек увидел, о чём система.
	Composition int `json:"composition"`
	// Texts — тексты, которые модель написала при выборе системы (2026-11-06):
	// `description` и `advantages`. Сборка документа берёт их из таблицы.
	Texts map[string]string `json:"texts,omitempty"`
	// Missing — какие блоки не написались. Выбор при этом сохранён: пустой
	// блок честнее черновика, а терять выбор из-за аварии модели нельзя.
	Missing []string `json:"missing,omitempty"`
}

// composeError — отказ сборки, который вызывающий превращает в HTTP-статус.
type composeError struct {
	status int
	text   string
}

func (e composeError) Error() string { return e.text }

// SystemChoice — выбор системы человеком: код из карточки или заметка Базы
// знаний, по которой он её выбрал.
//
// Идентификатор заметки появился 2026-11-02 по живой жалобе: ни одна из 375
// живых карточек `commercial_propose` не несёт `system_sku`, поэтому список в
// карточке КП состоял из вариантов с пустым значением, и «Добавить систему»
// отвечало «Выберите систему» на любой выбор. Заметка есть у каждой карточки
// всегда — по ней выбор и передаётся.
type SystemChoice struct {
	SKU    string `json:"sku,omitempty"`
	NoteID int64  `json:"noteId,omitempty"`
}

// Choice — то, чем карточку выбирает список: хэндл и код.
//
// Хэндл — заметка, а когда её нет — код. Значение пустым не бывает, иначе
// список снова получил бы варианты, которые нельзя выбрать.
func (c SystemCard) Choice() SystemChoice {
	if c.NoteID > 0 {
		return SystemChoice{NoteID: c.NoteID}
	}
	return SystemChoice{SKU: c.SKU}
}

// Handle — значение варианта списка: номер заметки или код.
func (c SystemChoice) Handle() string {
	if c.NoteID > 0 {
		return systemChoiceNotePrefix + strconv.FormatInt(c.NoteID, 10)
	}
	return strings.TrimSpace(c.SKU)
}

// ParseSystemChoice разбирает значение варианта списка.
//
// `sys-<noteId>` — заметка, пусто — ничего не выбрано, остальное — код
// системы. Префикс нужен, чтобы номер заметки никогда не спутался с кодом:
// код свободный текст, и «12» в нём допустим.
func ParseSystemChoice(handle string) SystemChoice {
	value := strings.TrimSpace(handle)
	if value == "" {
		return SystemChoice{}
	}
	if rest, ok := strings.CutPrefix(value, systemChoiceNotePrefix); ok {
		if id, err := strconv.ParseInt(strings.TrimSpace(rest), 10, 64); err == nil && id > 0 {
			return SystemChoice{NoteID: id}
		}
		return SystemChoice{}
	}
	return SystemChoice{SKU: value}
}

const systemChoiceNotePrefix = "sys-"

// FindSystem — система по выбору из списка.
//
// Код главнее заметки и сравнивается без учёта регистра и краевых пробелов: в
// карточку его пишут руками, и `TN-roof-classic` с пробелом на конце — тот же
// код. Заметка ищется, когда кода нет: в живых карточках его почти никогда нет,
// и именно этот случай раньше был невыразим (2026-11-02).
func FindSystem(systems []SystemCard, choice SystemChoice) (SystemCard, bool) {
	want := strings.ToUpper(strings.TrimSpace(choice.SKU))
	if want != "" {
		for _, card := range systems {
			if strings.ToUpper(strings.TrimSpace(card.SKU)) == want {
				return card, true
			}
		}
		return SystemCard{}, false
	}
	if choice.NoteID <= 0 {
		return SystemCard{}, false
	}
	for _, card := range systems {
		if card.NoteID == choice.NoteID {
			return card, true
		}
	}
	return SystemCard{}, false
}

// assembleSystem помечает предложение выбранной системой.
//
// Система ищется в живой ленте Базы знаний, а не в кеше карточек: человек
// выбрал её из раздела «Системы», который читает базу знаний вживую, и кеш
// мог отстать от того, что он видел на экране.
//
// Возвращает обновлённое предложение и сводку. Позиции НЕ дописываются
// (2026-11-03): в КП попадают обоснование и описание, а состав цен и позиций
// менеджер ведёт сам. Если система не найдена — composeError со статусом 404,
// и предложение НЕ сохраняется.
func (s *Server) assembleSystem(ctx context.Context, offer Offer, choice SystemChoice) (Offer, ComposeSystemResponse, error) {
	materials, err := s.listMaterials(ctx, false)
	if err != nil {
		return offer, ComposeSystemResponse{}, composeError{
			status: http.StatusInternalServerError, text: "не удалось прочитать материалы",
		}
	}
	systems, err := SystemsFromKB(ctx, s.kb, materials)
	if err != nil {
		return offer, ComposeSystemResponse{}, composeError{
			status: http.StatusBadGateway, text: "База знаний недоступна: " + err.Error(),
		}
	}
	card, found := FindSystem(systems, choice)
	if !found {
		return offer, ComposeSystemResponse{}, composeError{
			status: http.StatusNotFound, text: "система не найдена",
		}
	}
	if card.IsEmpty() {
		return offer, ComposeSystemResponse{}, composeError{
			status: http.StatusBadRequest,
			text:   "в карточке системы нет ни описания, ни состава",
		}
	}
	before := len(offer.Systems)

	// Система запоминается в самом предложении: по ней печатаются описание,
	// преимущества и ссылки, и по ней же ищется карточка, когда менеджер потом
	// грузит таблицу — сопоставление позиций с системой больше не нужно.
	//
	// Систем теперь может быть несколько (2026-11-05): чтобы предложить и
	// сравнить два решения в одном КП. Add не дублирует и сохраняет порядок.
	offer.Systems = offer.Systems.Add(OfferSystem{SKU: card.SKU, NoteID: card.NoteID})
	resp := ComposeSystemResponse{
		Offer:       offer,
		System:      card,
		Title:       card.Title,
		Composition: countCompositionMaterials(card.CompositionLinks),
	}
	// Система уже выбрана — второй раз её не «добавляем».
	//
	// Это не только про дубли: тексты системы уже написаны и лежат в
	// offers_system_text, и повторный вызов модели на каждое нажатие «Добавить»
	// был бы ровно той тратой времени, ради устранения которой сделан шаг B
	// (2026-11-06). Переписать текст — кнопка «Пересчитать».
	alreadyChosen := len(offer.Systems) == before
	if err := s.persistOfferSystem(ctx, offer); err != nil {
		return offer, resp, composeError{
			status: http.StatusInternalServerError, text: "не удалось сохранить предложение",
		}
	}
	if alreadyChosen {
		resp.Texts = s.storedTextsOf(ctx, card)
		return offer, resp, nil
	}
	// Система сменилась — значит, текст блоков «по промту» про прежнюю систему
	// больше не годится. Кэш стираем здесь, при записи, а не при печати:
	// между ними человек может открыть КП и увидеть старый текст (2026-11-01).
	offer.FormValues = dropStaleBlockTexts(offer.FormValues)
	if err := s.persistOfferFormValues(ctx, offer); err != nil {
		log.Printf("add system: тексты блоков не сброшены у КП %s: %v", offer.Number, err)
	}
	resp.Offer = offer
	// Тексты системы — ПОСЛЕ сохранения выбора (2026-11-06, шаг B).
	//
	// Порядок обязателен: человек нажал «Добавить», система уже выбрана, и
	// даже если модель не ответит, КП не потеряет выбор. Модель вызывается
	// здесь один раз за жизнь системы (результат лежит в offers_system_text),
	// и сборка документа больше не ждёт её вовсе.
	texts, missing := s.reconcileSystemTexts(ctx, offer, card)
	resp.Texts = texts
	resp.Missing = missing
	return offer, resp, nil
}

// countCompositionMaterials — сколько в составе системы найдено материалов.
//
// Считаем только найденные: строка-слой («Подготовка основания») материалом не
// является, и обещать «12 продуктов» там, где их девять, значит врать.
func countCompositionMaterials(links []CompositionItem) int {
	n := 0
	for _, link := range links {
		if link.Found {
			n++
		}
	}
	return n
}

// dropStaleBlockTexts убирает хеш данных из form_values: при следующей сборке
// документа тексты блоков будут написаны заново.
//
// Сам текст не стирается: человек мог видеть его в плагине, и тихо удалять
// текст, который он читал, хуже, чем переписать его при печати. Хеш — то, по
// чему решается «этот текст ещё про это предложение».
func dropStaleBlockTexts(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	out := make(map[string]string, len(values))
	for k, v := range values {
		if k == genKeySection {
			continue
		}
		out[k] = v
	}
	return out
}

// writeComposeError отвечает отказом сборки: текст ошибки — для человека.
func writeComposeError(w http.ResponseWriter, err error) {
	if ce, ok := err.(composeError); ok {
		if ce.status == http.StatusInternalServerError {
			dbError(w, "add system", fmt.Errorf("%s", ce.text))
			return
		}
		writeJSON(w, ce.status, map[string]any{"error": ce.text})
		return
	}
	dbError(w, "add system", err)
}

// primaryEkn — номер для позиции КП: первый по справочнику.
//
// Пусто — тоже нормально: цены и позиции живут и без ЕКН, а выдуманный номер
// в документе хуже пустой ячейки — его начнут сверять.
func primaryEkn(m Material) string {
	for _, e := range m.Ekn {
		if v := strings.TrimSpace(e.Ekn); v != "" {
			return v
		}
	}
	return ""
}

// persistOfferSystem пишет выбранную систему и двигает updated_at.
//
// Отдельной ручкой, а не полным сохранением предложения: выбор системы — одно
// поле, и гонять через него validateOffer (состав, цены) значило бы отказать в
// выборе из-за позиции, которую менеджер ещё не завёл.
func (s *Server) persistOfferSystem(ctx context.Context, offer Offer) error {
	raw, err := json.Marshal(offer.Systems)
	if err != nil {
		return err
	}
	return s.persistOfferColumn(ctx, offer.ID, "system_choice", raw)
}

// handleAddSystem — «Добавить систему» в редакторе плагина.
func (s *Server) handleAddSystem(w http.ResponseWriter, r *http.Request) {
	s.systemWriteHandler(w, r, false)
}

// handleRemoveSystem — убрать систему из предложения (2026-11-05).
//
// Отдельная ручка, а не «сохранить весь список»: менеджер убирает одну систему,
// и полное сохранение предложения отказало бы из-за незаконченных позиций.
func (s *Server) handleRemoveSystem(w http.ResponseWriter, r *http.Request) {
	s.systemWriteHandler(w, r, true)
}

// systemWriteHandler — общий разбор тела и ответ для добавления и удаления.
//
// Тело одно: `sku` и/или `noteId`. Ошибка «система не найдена» здесь не
// проверяется: убрать можно и систему, которой больше нет в Базе знаний —
// именно такой случай и заставляет человека её убрать.
func (s *Server) systemWriteHandler(w http.ResponseWriter, r *http.Request, remove bool) {
	offer, ok := s.offerForEditor(w, r)
	if !ok {
		return
	}
	var req struct {
		SKU    string `json:"sku"`
		NoteID int64  `json:"noteId"`
	}
	if err := decodeJSON(w, r, &req, 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	choice := SystemChoice{SKU: req.SKU, NoteID: req.NoteID}
	if choice.Handle() == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "не выбрана система"})
		return
	}
	if remove {
		s.removeSystem(w, r, offer, choice)
		return
	}
	_, resp, err := s.assembleSystem(r.Context(), offer, choice)
	if err != nil {
		writeComposeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleGetSystemTexts — сохранённые тексты систем предложения.
//
// Плагин показывает их человеку и по признаку `stale` решает, показать ли
// кнопку «Пересчитать тексты системы»: карточку в Базе знаний могли изменить
// после выбора.
func (s *Server) handleGetSystemTexts(w http.ResponseWriter, r *http.Request) {
	offer, ok := s.offerForReader(w, r)
	if !ok {
		return
	}
	cards, _ := s.systemCardsForOffer(r.Context(), offer)
	writeJSON(w, http.StatusOK, map[string]any{"texts": s.systemTextsForSystems(r.Context(), cards)})
}

// handleRecalculateSystemTexts — кнопка «Пересчитать тексты системы».
//
// Пересчитывает ОДНУ систему: `noteId` в пути. Карточка читается из Базы
// знаний, а не из кеша: пересчёт нажимают как раз тогда, когда карточку
// поправили, и кеш мог не обновиться.
func (s *Server) handleRecalculateSystemTexts(w http.ResponseWriter, r *http.Request) {
	offer, ok := s.offerForEditor(w, r)
	if !ok {
		return
	}
	noteID, err := strconv.ParseInt(r.PathValue("noteId"), 10, 64)
	if err != nil || noteID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid noteId"})
		return
	}
	card, err := s.liveSystemCard(r.Context(), noteID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "База знаний недоступна: " + err.Error()})
		return
	}
	// Пустая карточка и «нет такой системы» — разные ответы: IsEmpty истинно у
	// карточки без содержания, а неизвестная заметка до этого места доходит с
	// NoteID = 0. Раньше оба случая отвечали 404 одинаково, и на пустой карточке
	// пересчёт молча «ничего не делал» вместо честного «нечего писать».
	if card.NoteID == 0 || card.IsEmpty() {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "система не найдена в Базе знаний"})
		return
	}
	texts, missing := s.recalculateSystem(r.Context(), offer, card)
	writeJSON(w, http.StatusOK, map[string]any{
		"title":   card.Title,
		"texts":   texts,
		"missing": missing,
	})
}

// liveSystemCard — одна карточка системы из Базы знаний по заметке.
func (s *Server) liveSystemCard(ctx context.Context, noteID int64) (SystemCard, error) {
	materials, err := s.listMaterials(ctx, false)
	if err != nil {
		return SystemCard{}, err
	}
	systems, err := SystemsFromKB(ctx, s.kb, materials)
	if err != nil {
		return SystemCard{}, err
	}
	card, found := FindSystem(systems, SystemChoice{NoteID: noteID})
	if !found {
		return SystemCard{}, nil
	}
	return card, nil
}

// removeSystem убирает систему и отвечает тем же телом, что добавление, чтобы
// клиент обновлял карточку одним разбором.
func (s *Server) removeSystem(w http.ResponseWriter, r *http.Request, offer Offer, choice SystemChoice) {
	before := len(offer.Systems)
	offer.Systems = offer.Systems.Remove(OfferSystem{SKU: choice.SKU, NoteID: choice.NoteID})
	if len(offer.Systems) == before {
		// Системы не было — это не ошибка, но и обновлять нечего. Отвечаем
		// 404: человек нажал «убрать» на строке, которой уже нет.
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "система не выбрана в предложении"})
		return
	}
	if err := s.persistOfferSystem(r.Context(), offer); err != nil {
		dbError(w, "remove system", err)
		return
	}
	// Состав систем изменился — текст блоков «по промту» про него устарел.
	offer.FormValues = dropStaleBlockTexts(offer.FormValues)
	if err := s.persistOfferFormValues(r.Context(), offer); err != nil {
		log.Printf("remove system: тексты блоков не сброшены у КП %s: %v", offer.Number, err)
	}
	writeJSON(w, http.StatusOK, ComposeSystemResponse{Offer: offer})
}
