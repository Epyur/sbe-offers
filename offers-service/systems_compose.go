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

	// Система запоминается в самом предложении: по ней печатаются описание,
	// преимущества и ссылки, и по ней же ищется карточка, когда менеджер потом
	// грузит таблицу — сопоставление позиций с системой больше не нужно.
	offer.System = OfferSystem{SKU: card.SKU, NoteID: card.NoteID}
	resp := ComposeSystemResponse{
		Offer:       offer,
		System:      card,
		Title:       card.Title,
		Composition: countCompositionMaterials(card.CompositionLinks),
	}
	if err := s.persistOfferSystem(ctx, offer); err != nil {
		return offer, resp, composeError{
			status: http.StatusInternalServerError, text: "не удалось сохранить предложение",
		}
	}
	// Система сменилась — значит, текст блоков «по промту» про прежнюю систему
	// больше не годится. Кэш стираем здесь, при записи, а не при печати:
	// между ними человек может открыть КП и увидеть старый текст (2026-11-01).
	offer.FormValues = dropStaleBlockTexts(offer.FormValues)
	if err := s.persistOfferFormValues(ctx, offer); err != nil {
		log.Printf("add system: тексты блоков не сброшены у КП %s: %v", offer.Number, err)
	}
	resp.Offer = offer
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
	raw, err := json.Marshal(offer.System)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
UPDATE offers_offer SET system_choice = $2, updated_at = now() WHERE id = $1`,
		offer.ID, raw)
	return err
}

// handleAddSystem — «Добавить систему» в редакторе плагина.
func (s *Server) handleAddSystem(w http.ResponseWriter, r *http.Request) {
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
	_, resp, err := s.assembleSystem(r.Context(), offer, SystemChoice{SKU: req.SKU, NoteID: req.NoteID})
	if err != nil {
		writeComposeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
